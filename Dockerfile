# syntax=docker/dockerfile:1

FROM golang:1.22-bookworm AS build

WORKDIR /src
COPY go.mod ./
COPY internal ./internal
ARG VERSION=dev
ARG COMMIT=
ARG BUILD_DATE=
RUN <<EOF
set -eu
module_path="$(go list -m -f '{{.Path}}')"
mkdir -p .docker-main /out
cat > .docker-main/main.go <<GOEOF
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"${module_path}/internal/buildinfo"
	"${module_path}/internal/proxy"
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
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

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
GOEOF
CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags="-s -w -X ${module_path}/internal/buildinfo.Version=${VERSION} -X ${module_path}/internal/buildinfo.Commit=${COMMIT} -X ${module_path}/internal/buildinfo.Date=${BUILD_DATE}" \
    -o /out/jacpro ./.docker-main
EOF

FROM debian:bookworm-slim

WORKDIR /app
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --system --uid 10001 --home-dir /app jacpro \
    && mkdir -p /config \
    && chown -R jacpro:jacpro /app /config
COPY --from=build /out/jacpro /app/jacpro
ENV HOST=0.0.0.0 \
    PORT=5002 \
    JACPRO_CONFIG=/config/config.json
USER jacpro
VOLUME ["/config"]
EXPOSE 5002
ENTRYPOINT ["/app/jacpro"]
