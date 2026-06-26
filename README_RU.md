# jacpro

JacPro - HTTP-прокси для JacRed с совместимыми API Torznab и Jackett. Сервис
принимает запросы от клиентов, обращается к JacRed v2 и при необходимости v1,
убирает дубликаты по infohash и возвращает результаты в Torznab RSS XML или
Jackett JSON.

Встроенный веб-интерфейс настроек доступен по адресу `/settings`.

## Возможности

- Torznab RSS: `/api`
- Jackett-style Torznab: `/api/v2.0/indexers/<status>/results/torznab/api`
- Jackett JSON: `/api/v2.0/indexers/<status>/results`
- Поиск через JacRed v2 с опциональным объединением результатов v1
- Разделение двуязычных запросов вида `локальное / оригинальное`
- Поддержка Lampa-style metadata search
- Фильтрация по Torznab-категории, году, сезону и эпизоду
- Сохранение настроек через веб-интерфейс в JSON-файл
- Ротация лог-файла без внешних зависимостей

## Запуск

```bash
go run ./cmd/jacpro
```

Адрес по умолчанию:

```text
http://127.0.0.1:5002/settings
```

Сборка локального бинарника:

```bash
go build -o jacpro .
./jacpro
```

Сборка release-бинарников для популярных платформ:

```bash
./scripts/build-dist.sh
```

Файлы создаются в `./Dist` и называются по схеме:

```text
jacpro-linux-amd64
jacpro-darwin-arm64
jacpro-windows-amd64.exe
```

Платформы по умолчанию:

```text
linux/amd64 linux/arm64 linux/arm
darwin/amd64 darwin/arm64
windows/amd64 windows/arm64
```

Список можно переопределить:

```bash
TARGETS="freebsd/amd64 linux/ppc64le" ./scripts/build-dist.sh
```

При запуске бинарник пишет в лог версию, платформу, commit и дату сборки.
Метаданные релиза можно передать при сборке:

```bash
VERSION=v1.2.3 COMMIT=abc123 BUILD_DATE=2026-06-26T00:00:00Z ./scripts/build-dist.sh
```

## Настройки

При старте JacPro берет значения по умолчанию из переменных окружения, затем
переопределяет их из JSON-файла настроек.

Приоритет поиска существующего файла:

1. `config.json` в текущей рабочей папке
2. путь из `JACPRO_CONFIG`
3. `config.json` рядом с бинарником

Если файл не найден, JacPro создает файл с дефолтными значениями. Если задан
`JACPRO_CONFIG` и существующего файла нет, новый файл создается по этому пути;
иначе создается `config.json` в рабочей папке.

Если выбранный JSON поврежден или содержит невалидные значения, JacPro
перемещает его в `*.bad-YYYYMMDD-HHMMSS`, создает новый дефолтный конфиг и
продолжает запуск.

| Переменная | По умолчанию | Описание |
| --- | --- | --- |
| `JACRED_BASE_URL` | `http://127.0.0.1:9117` | Базовый URL JacRed |
| `JACRED_APIKEY` | пусто | API key JacRed |
| `JACRED_TIMEOUT` | `20` | Таймаут backend-запроса в секундах |
| `JACRED_MERGE_V1` | `true` | Добавлять результаты v1 `/api/v1.0/torrents` |
| `JACRED_STRIP_YEAR` | `false` | Убирать год в конце fuzzy-запроса |
| `JACRED_ENRICH_TITLES` | `true` | Добавлять voice tags / `[].rus` в Torznab title |
| `JACRED_SKIP_CAT_FILTER` | `false` | Не применять post-filter по Torznab category |
| `HOST` | `0.0.0.0` | Адрес bind |
| `PORT` | `5002` | Порт bind |
| `LOG_LEVEL` | `INFO` | `DEBUG`, `INFO`, `WARNING`, `ERROR`, `CRITICAL` |
| `LOG_FILE` | `/tmp/jacpro.log` | Путь к лог-файлу |
| `JACPRO_CONFIG` | пусто | Опциональный путь к JSON-настройкам |

Пример `config.json`:

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

Системные endpoints:

- `GET /settings` - веб-интерфейс настроек
- `GET /api/settings` - текущие настройки в JSON
- `POST /api/settings` - сохранить настройки
- `GET /api/backend/status` - проверить JacRed `/version`
- `GET /version` - версия backend, если доступна
- `GET /lastupdatedb` - время последнего обновления базы backend

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

В Docker Compose укажите `JACRED_BASE_URL`, доступный из контейнера, например:

```text
http://jacred:9117
```

Релизный workflow публикует Docker images в GitHub Container Registry:

```text
ghcr.io/<owner>/<repo>:<release-tag>
ghcr.io/<owner>/<repo>:latest
```

## Проверка

```bash
go test ./...
go vet ./...
```
