#!/usr/bin/env bash
set -uo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="${DIST_DIR:-"$ROOT_DIR/Dist"}"
PACKAGE="${PACKAGE:-./cmd/jacpro}"
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

BUILD_LDFLAGS=(
  "$LDFLAGS"
  "-X" "jacpro/internal/buildinfo.Version=$VERSION"
  "-X" "jacpro/internal/buildinfo.Commit=$COMMIT"
  "-X" "jacpro/internal/buildinfo.Date=$BUILD_DATE"
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
