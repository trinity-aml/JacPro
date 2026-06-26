#!/usr/bin/env bash
set -uo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="${DIST_DIR:-"$ROOT_DIR/Dist"}"
PACKAGE="${PACKAGE:-}"
BINARY_NAME="${BINARY_NAME:-jacpro}"
CGO_ENABLED="${CGO_ENABLED:-0}"
LDFLAGS="${LDFLAGS:--s -w}"
VERSION="${VERSION:-}"
COMMIT="${COMMIT:-}"
BUILD_DATE="${BUILD_DATE:-}"
TARGETS="${TARGETS:-}"
BUILD_TAGS="${BUILD_TAGS:-}"
SKIP_EXTERNAL_LINK_TARGETS="${SKIP_EXTERNAL_LINK_TARGETS:-1}"
STRICT="${STRICT:-0}"
CLEAN_DIST="${CLEAN_DIST:-1}"
DEFAULT_TARGETS=(
  linux/amd64
  linux/arm64
  linux/arm
  darwin/amd64
  darwin/arm64
  windows/amd64
  windows/arm64
)

if ! command -v go >/dev/null 2>&1; then
  echo "go not found in PATH" >&2
  exit 127
fi

if [[ -z "${GOCACHE:-}" ]]; then
  export GOCACHE="$ROOT_DIR/.gocache"
fi

if [[ -z "$VERSION" ]]; then
  if git -C "$ROOT_DIR" describe --tags --exact-match >/dev/null 2>&1; then
    VERSION="$(git -C "$ROOT_DIR" describe --tags --exact-match)"
  elif git -C "$ROOT_DIR" describe --tags --always --dirty >/dev/null 2>&1; then
    VERSION="$(git -C "$ROOT_DIR" describe --tags --always --dirty)"
  else
    VERSION="dev"
  fi
fi

if [[ -z "$COMMIT" ]]; then
  COMMIT="$(git -C "$ROOT_DIR" rev-parse --short HEAD 2>/dev/null || true)"
fi

if [[ -z "$BUILD_DATE" ]]; then
  BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
fi

resolve_package() {
  if [[ -n "$PACKAGE" ]]; then
    echo "$PACKAGE"
    return 0
  fi

  if [[ -f "$ROOT_DIR/main.go" ]]; then
    echo "."
    return 0
  fi

  if [[ -d "$ROOT_DIR/cmd/jacpro" ]]; then
    echo "./cmd/jacpro"
    return 0
  fi

  local packages
  packages="$(cd "$ROOT_DIR" && go list -f '{{if eq .Name "main"}}{{.ImportPath}}{{end}}' ./... 2>/dev/null | sed '/^$/d' || true)"
  if [[ -n "$packages" ]]; then
    echo "$packages" | head -n 1
    return 0
  fi

  if ! create_fallback_main; then
    return 1
  fi
  echo "./.build/jacpro-main"
}

create_fallback_main() {
  local module_path
  module_path="$(cd "$ROOT_DIR" && go list -m -f '{{.Path}}')"
  if [[ ! -d "$ROOT_DIR/internal/proxy" || ! -d "$ROOT_DIR/internal/buildinfo" ]]; then
    echo "could not find a Go main package and fallback packages are missing." >&2
    echo "Set PACKAGE=./path/to/main-package." >&2
    echo "Go files found:" >&2
    find "$ROOT_DIR" -maxdepth 4 -type f -name '*.go' -print | sed "s#^$ROOT_DIR/#  #" >&2
    return 1
  fi

  mkdir -p "$ROOT_DIR/.build/jacpro-main"
  cat > "$ROOT_DIR/.build/jacpro-main/main.go" <<EOF
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"$module_path/internal/buildinfo"
	"$module_path/internal/proxy"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "jacpro: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	config := proxy.ResolveConfigPath()
	for _, warning := range config.Warnings {
		fmt.Fprintf(os.Stderr, "jacpro: %s\n", warning)
	}

	store, warnings, err := proxy.NewSettingsStore(config.Path)
	if err != nil {
		return err
	}

	logger, err := proxy.NewLogger(store.Get())
	if err != nil {
		return err
	}
	defer logger.Close()
	logger.Infof(
		"jacpro version=%s platform=%s commit=%s build_date=%s",
		buildinfo.Version,
		buildinfo.Platform(),
		valueOrUnknown(buildinfo.Commit),
		valueOrUnknown(buildinfo.Date),
	)
	logger.Infof("settings file: %s (%s, found=%v)", config.Path, config.Source, config.Found)
	for _, warning := range warnings {
		logger.Warningf("%s", warning)
	}

	app := proxy.NewServer(store, logger)
	settings := store.Get()
	addr := fmt.Sprintf("%s:%d", settings.Host, settings.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           app,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Infof("listening on http://%s", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	select {
	case sig := <-sigCh:
		logger.Infof("received %s, shutting down", sig)
	case err := <-errCh:
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		return err
	}
	return <-errCh
}

func valueOrUnknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}
EOF
  echo "No main package found; generated fallback entrypoint at .build/jacpro-main" >&2
}

if ! PACKAGE="$(resolve_package)"; then
  exit 1
fi
if [[ "$PACKAGE" == ./* && ! -d "$ROOT_DIR/${PACKAGE#./}" ]]; then
  echo "package path $PACKAGE does not exist under $ROOT_DIR" >&2
  exit 1
fi
BUILDINFO_PACKAGE="$(cd "$ROOT_DIR" && go list -f '{{.ImportPath}}' ./internal/buildinfo 2>/dev/null || true)"
if [[ -z "$BUILDINFO_PACKAGE" ]]; then
  echo "could not resolve ./internal/buildinfo package" >&2
  exit 1
fi

BUILD_LDFLAGS=(
  "$LDFLAGS"
  "-X" "$BUILDINFO_PACKAGE.Version=$VERSION"
  "-X" "$BUILDINFO_PACKAGE.Commit=$COMMIT"
  "-X" "$BUILDINFO_PACKAGE.Date=$BUILD_DATE"
)

mkdir -p "$DIST_DIR" "$GOCACHE"

if [[ -z "$TARGETS" ]]; then
  TARGETS="${DEFAULT_TARGETS[*]}"
fi

if [[ "$CLEAN_DIST" == "1" ]]; then
  find "$DIST_DIR" -maxdepth 1 -type f \( -name "$BINARY_NAME-*" -o -name "$BINARY_NAME-*.exe" \) -delete
fi

failures=()
skipped=()
count=0

echo "Output: $DIST_DIR"
echo "Package: $PACKAGE"
echo "Targets: $TARGETS"
echo "Version: $VERSION"

for target in $TARGETS; do
  goos="${target%%/*}"
  goarch="${target##*/}"
  output="$DIST_DIR/$BINARY_NAME-$goos-$goarch"
  if [[ "$goos" == "windows" ]]; then
    output="$output.exe"
  fi

  args=(build -trimpath -ldflags "${BUILD_LDFLAGS[*]}" -o "$output")
  if [[ -n "$BUILD_TAGS" ]]; then
    args+=(-tags "$BUILD_TAGS")
  fi
  args+=("$PACKAGE")

  printf 'Building %-18s -> %s\n' "$target" "$(basename "$output")"
  log_file="$(mktemp)"
  if env CGO_ENABLED="$CGO_ENABLED" GOOS="$goos" GOARCH="$goarch" go "${args[@]}" 2>"$log_file"; then
    count=$((count + 1))
  else
    if [[ "$SKIP_EXTERNAL_LINK_TARGETS" == "1" ]] && grep -q "requires external (cgo) linking" "$log_file"; then
      skipped+=("$target")
      sed 's/^/  skip: /' "$log_file"
    else
      cat "$log_file" >&2
      failures+=("$target")
    fi
  fi
  rm -f "$log_file"
done

if (( ${#failures[@]} > 0 )); then
  echo
  echo "Failed targets:" >&2
  printf '  %s\n' "${failures[@]}" >&2
  exit 1
fi

if (( ${#skipped[@]} > 0 )); then
  echo
  echo "Skipped targets requiring an external cgo/platform linker:"
  printf '  %s\n' "${skipped[@]}"
  if [[ "$STRICT" == "1" ]]; then
    exit 1
  fi
fi

echo
echo "Built $count binaries in $DIST_DIR"
