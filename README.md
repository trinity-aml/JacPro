# jacpro

[Русская документация](README_RU.md)

JacPro is a Torznab and Jackett-compatible HTTP proxy for JacRed. It translates
client requests, queries JacRed v2 and optionally v1, deduplicates torrent
results by infohash, and returns Torznab RSS XML or Jackett JSON.

The service also includes a built-in settings UI at `/settings`.

## Features

- Torznab RSS endpoint: `/api`
- Jackett-style Torznab endpoint: `/api/v2.0/indexers/<status>/results/torznab/api`
- Jackett JSON endpoint: `/api/v2.0/indexers/<status>/results`
- JacRed v2 search plus optional v1 merge
- Bilingual `local / original` query splitting
- Lampa-style card metadata search parameters
- Torznab category, year, season and episode filtering
- Runtime settings UI with JSON config persistence
- Rotating file logging without external dependencies

## Run

```bash
go run ./cmd/jacpro
```

Default listen address: `0.0.0.0:5002`.

Open:

```text
http://127.0.0.1:5002/settings
```

Build a binary:

```bash
go build -o jacpro .
./jacpro
```

Build release binaries for popular Linux, macOS and Windows targets:

```bash
./scripts/build-dist.sh
```

Artifacts are written to `./Dist` and named like `jacpro-linux-amd64`
or `jacpro-windows-amd64.exe`.

Default targets:

```text
linux/amd64 linux/arm64 linux/arm
darwin/amd64 darwin/arm64
windows/amd64 windows/arm64
```

Override the list when needed:

```bash
TARGETS="freebsd/amd64 linux/ppc64le" ./scripts/build-dist.sh
```

The binary logs its version, platform, commit and build date at startup. Release
metadata can be injected during builds:

```bash
VERSION=v1.2.3 COMMIT=abc123 BUILD_DATE=2026-06-26T00:00:00Z ./scripts/build-dist.sh
```

## Configuration

At startup, defaults are read from environment variables and then overridden by
the JSON config file.

Config lookup priority for existing files:

1. `config.json` in the current working directory
2. path from `JACPRO_CONFIG`
3. `config.json` next to the executable

If no config file exists, JacPro creates one with default values. When
`JACPRO_CONFIG` is set and no existing config is found, that path is used as the
creation target; otherwise `config.json` is created in the working directory.
If the selected config contains invalid JSON or invalid values, JacPro moves it
to `*.bad-YYYYMMDD-HHMMSS`, creates a fresh default config, and continues
startup.

| Variable | Default | Description |
| --- | --- | --- |
| `JACRED_BASE_URL` | `http://127.0.0.1:9117` | JacRed base URL |
| `JACRED_APIKEY` | empty | JacRed API key |
| `JACRED_TIMEOUT` | `20` | Backend timeout in seconds |
| `JACRED_MERGE_V1` | `true` | Merge v1 `/api/v1.0/torrents` results |
| `JACRED_STRIP_YEAR` | `false` | Strip trailing year from fuzzy queries |
| `JACRED_ENRICH_TITLES` | `true` | Add voice tags / `[].rus` to Torznab titles |
| `JACRED_SKIP_CAT_FILTER` | `false` | Skip Torznab category post-filtering |
| `HOST` | `0.0.0.0` | Bind host |
| `PORT` | `5002` | Bind port |
| `LOG_LEVEL` | `INFO` | `DEBUG`, `INFO`, `WARNING`, `ERROR`, `CRITICAL` |
| `LOG_FILE` | `/tmp/jacpro.log` | Rotating log file path |
| `JACPRO_CONFIG` | empty | Optional JSON settings path |

Example `config.json`:

```json
{
  "base_url": "http://127.0.0.1:9117",
  "apikey": "",
  "version": "1.0.0",
  "strip_trailing_year": false,
  "merge_v1": true,
  "enrich_titles": true,
  "skip_cat_filter": false,
  "request_timeout": 20,
  "log_level": "INFO",
  "log_file": "/tmp/jacpro.log",
  "host": "0.0.0.0",
  "port": 5002
}
```

## HTTP API

System:

- `GET /settings` - web settings UI
- `GET /api/settings` - current settings as JSON
- `POST /api/settings` - update settings
- `GET /api/backend/status` - check JacRed `/version`
- `GET /version` - backend `/version` when available, otherwise proxy version
- `GET /lastupdatedb` - backend DB timestamp when available

Torznab:

- `GET /api?t=caps`
- `GET /api?t=indexers&configured=true`
- `GET /api?t=search&q=...`
- `GET /api?t=moviesearch&q=...`
- `GET /api?t=tvsearch&q=...&season=1&ep=2`
- `GET /api/v2.0/indexers/all/results/torznab/api?...`

Jackett JSON:

- `GET /api/v2.0/indexers`
- `GET /api/v2.0/indexers/all/results?Query=...`
- `GET /api/v1/indexer`

## Docker

```bash
docker build -t jacpro:latest .
docker run --rm -p 5002:5002 \
  -e JACRED_BASE_URL=http://host.docker.internal:9117 \
  -v jacpro-config:/config \
  jacpro:latest
```

In Docker Compose, set `JACRED_BASE_URL` to a service name reachable from the
container, for example `http://jacred:9117`.

The release workflow publishes Docker images to GitHub Container Registry:

```text
ghcr.io/<owner>/<repo>:<release-tag>
ghcr.io/<owner>/<repo>:latest
```

## Test

```bash
go test ./...
```
