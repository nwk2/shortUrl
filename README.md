# shortUrl

A URL shortener REST API in Go. Create a short link, look it up, or follow it
to the original URL. Links expire 30 days after creation by default.

The frontend lives in [shortUrl-UI](https://github.com/nwk2/shortUrl-UI).

## Run

```bash
go run ./cmd/shorturl
```

Listens on `:8080` and stores links in `test.db` (SQLite) in the working
directory. `go vet ./...`, `go test ./...`, `go build ./...` — CI runs all
three on every push to master.

## Layout

```
cmd/shorturl/            entry point: load config, open store, wire routes
internal/config/         Config struct + viper loader (loaded once at startup)
internal/model/          ShortUrl entity + create input (DB row and JSON body)
internal/store/          Store: all SQLite access, one *gorm.DB, context-aware
internal/service/        domain: URL/expiry validation, hash minting, expiry rules
internal/handler/        HTTP layer: bind JSON, call service, map errors to codes
internal/middleware/     CORS, per-IP rate limiting, X-Api-Key auth
configs/app_config.yml   runtime config (see below)
swagger.json             OpenAPI contract for the routes
```

Dependencies flow one way: handler -> service -> store -> model. Config is
loaded once in `main` and passed down; nothing re-reads the file per request,
and there is no package-level database global — tests build their own `Store`
on an in-memory database.

## API

| Method | Route | Auth | Description |
| ------ | ----- | ---- | ----------- |
| GET | `/ping` | — | `{"res":"pong"}` health check |
| POST | `/short-urls` | API key | Create a link. Body: `{"original_url": "...", "expiry_date": "YYYY-MM-DD HH:MM"}` (expiry optional, defaults to +30 days, interpreted in the server's local timezone; body capped at 1MB). Only `http://`/`https://` URLs are accepted (a bare host is treated as `http`). Returns `201` with the link. |
| GET | `/short-urls` | API key | List live links, paginated: `?limit=50&offset=0` (limit 1–100, default 50). Expired links are excluded |
| GET | `/short-urls/:hashKey` | — | One link by hash. `404` unknown, `410` expired |
| GET | `/redirect/:hashKey` | — | `302` to the original URL. `404` unknown, `410` expired |

`GET /short-urls` and `POST /short-urls` require the `X-Api-Key` header when
`API_KEY` is set; an empty key disables auth (the server logs a warning).
Hashes are 11-character base62 tokens minted from `crypto/rand`.

Expired links are removed by an hourly background sweep; until the sweep
runs, a single expired link still answers `410` and is excluded from the
list. The server sets 10s read/write and 60s idle timeouts and shuts down
gracefully on SIGINT/SIGTERM. Rate-limit windows for IPs that stop calling
are pruned once a minute, so the in-memory map can't grow forever.
Server-side failures answer a generic `500 {"error": "Internal server
error"}`; the detail goes to the server log, not the client.

## Config

`configs/app_config.yml`; every key can be overridden by an environment
variable of the same name. Missing file or key falls back to the defaults
shown here.

| Key | Default | Notes |
| --- | ------- | ----- |
| `APP_HOSTNAME` | `http://localhost:8080/redirect/` | Prefix for generated short URLs |
| `CORS_ALLOWED_ORIGINS` | `["*"]` | `["*"]` allows any origin without credentials; explicit origins are echoed back with credentials |
| `RATE_LIMIT_PER_MINUTE` | `100` | Per client IP, all routes. `0` or negative disables |
| `API_KEY` | `""` | Shared secret for create/list. Empty = auth off |
| `DB_PATH` | `test.db` | SQLite file, relative to the working directory |
| `LOG_LEVEL` | `info` | SQL log level: `info`, `warn`, `error`, `silent` |
