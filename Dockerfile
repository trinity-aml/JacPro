FROM golang:1.22-bookworm AS build

WORKDIR /src
COPY go.mod ./
COPY main.go ./
COPY internal ./internal
ARG VERSION=dev
ARG COMMIT=
ARG BUILD_DATE=
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags="-s -w -X jacpro/internal/buildinfo.Version=${VERSION} -X jacpro/internal/buildinfo.Commit=${COMMIT} -X jacpro/internal/buildinfo.Date=${BUILD_DATE}" \
    -o /out/jacpro .

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
