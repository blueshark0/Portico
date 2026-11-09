# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Portico (Go backend + Vue 3/TypeScript frontend) runs a loopback relay on `127.0.0.1:18100` (fixed port) in front of Claude Code, Codex and Gemini CLI. It rewrites each CLI's config to point at the relay, routes requests to a user-chosen provider, records usage and cost, and aggregates MCP servers behind `/mcp`. All data lives in `~/.portico/`.

It runs as two processes:
- **`portico-server`** (`cmd/portico-server`, pure Go, no cgo) owns everything: the relay, CLI config patching, the MCP gateway, the databases, and the management API + embedded web UI on `127.0.0.1:18101` (configurable, token-protected for non-local clients). It runs headless on servers.
- **The desktop app** (`main.go`, Wails 3) is only a window, tray and dock shell. On launch it reuses a running backend (found via `~/.portico/server.json`) or spawns the bundled `portico-server` next to its executable, then loads the backend's web UI in its window. Quitting the window does not stop the backend. The desktop shell links none of the backend packages.

## Commands

Prerequisites: Go 1.26+, Node 20.19+/22.12+ with pnpm 9, Wails CLI `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0`. On Linux, the desktop shell needs `libgtk-4-dev libwebkitgtk-6.0-dev`; `portico-server` does not.

```sh
wails3 task build:server                # backend + embedded web UI → bin/portico-server (CGO_ENABLED=0)
wails3 task run:server                  # build and run portico-server in the foreground
wails3 task dev                         # desktop shell with hot reload; it spawns bin/portico-server as its backend
wails3 task build                       # desktop app + backend for the current OS
wails3 task package                     # production packaging
wails3 task common:generate:bindings    # regenerate frontend/bindings after changing facade methods/structs
```

CI checks (run these before a PR):

```sh
go vet ./...
go test -race -count=1 ./...
CGO_ENABLED=0 go build -o /dev/null ./cmd/portico-server   # the backend must stay cgo-free (CI also cross-builds windows/amd64 and linux/arm64)
cd frontend && pnpm install --frozen-lockfile && pnpm run build   # vue-tsc type-check + Vite build
```

Single package / single test:

```sh
go test -race -count=1 ./internal/interfaces/http/relay/ -run TestName
```

Build gotchas:
- `frontend/embed.go` embeds `frontend/dist` via `//go:embed all:dist` (served by portico-server), so `go vet`/`go test ./...` fail to compile `portico/frontend` and `cmd/portico-server` until the frontend has been built at least once.
- Without GTK/WebKit dev packages, only `main.go`, `internal/app/desktop` and `cmd/bindgen` fail to build; everything else (including `internal/app/bootstrap`) builds and tests with `CGO_ENABLED=0`.
- `frontend/bindings/` is generated and gitignored; the frontend won't type-check until bindings are generated. They are generated from `cmd/bindgen` with `-names`, so calls go by method name, not ID.
- Frontend dev: `cd frontend && pnpm dev` (port 9245). Vite proxies `/api` to a running backend (`PORTICO_ADMIN_URL`, default `http://127.0.0.1:18101`). In dev mode `main.ts` probes `/api/health`; if no backend answers, or with `VITE_PORTICO_MOCK=1`, it installs `src/mocks/installMockTransport.ts`, which answers binding calls by `{Facade}.{Method}` name with data from `src/mocks/demoData.ts`. New binding methods return `null` there until a mock handler is added.
- An optional `.env.local` (see `.env.local.example`) is loaded by Taskfile for local build proxies.

## Architecture

### Backend layout (DDD-style bounded contexts)

Each context under `internal/` (`routing`, `sessions`, `observability`, `platformproxy`, `mcpgateway`, `configsync`, `hotkeys`) is split into `domain / application / infrastructure`, with dependencies pointing inward. Adapter layers sit on top:
- `internal/interfaces/wails/`: `*Facade` types exposed to the UI. Despite the package name they are no longer Wails services at runtime: the admin server dispatches to them by reflection. Their exported methods become the TS functions in `frontend/bindings/portico/internal/interfaces/wails/<facade>`.
- `internal/interfaces/http/admin/`: the management server on its own listener (default `127.0.0.1:18101`). `GET /api/health` (no auth), `POST /api/rpc/{Facade}.{Method}` (JSON array of args), `POST /api/shutdown`, and the embedded web UI. `auth.go`: non-local requests need `Authorization: Bearer <token>` (`~/.portico/admin_token`); loopback address *and* loopback `Host` are exempt unless `--always-require-token`; cross-site browser requests are always rejected.
- `internal/interfaces/http/relay/`: Gin HTTP server for the CLI relay, which also mounts the MCP gateway at `/mcp`. It must only ever listen on loopback, otherwise LAN hosts could use the configured paid API keys. Only the admin port may be exposed.

`internal/app/bootstrap/container.go` is the composition root. It builds every store/service/facade and wires the relay with injected options (session cache, request-log sink, token parser per platform, MCP gateway). `Container.Facades()` lists what the admin RPC exposes. New facades must be added there **and** to `cmd/bindgen/main.go`, which exists only so `wails3 generate bindings` can discover the types.

Process plumbing:
- `cmd/portico-server/main.go`: subcommands `serve` (default), `token show|reset`, `version`. Admin flags have env equivalents: `--listen`/`PORTICO_ADMIN_LISTEN`, `--tls-cert`/`--tls-key`/`PORTICO_ADMIN_TLS_*`, `--always-require-token`/`PORTICO_ADMIN_ALWAYS_TOKEN=1`.
- `internal/shared/serverinfo`: the shared contract between server and shell (default admin address, token file, `~/.portico/server.json` runtime info written on start and removed on exit).
- `internal/app/sidecar` (no Wails dependency, testable headless): the shell discovers a running backend, otherwise spawns `portico-server` from its own directory as a detached process, and can ask it to stop.
- `internal/app/desktop`: window/tray/dock shell plus the local loading/error page shown before the backend is reachable.

### Storage

SQLite via `modernc.org/sqlite` (pure Go). Connections are registered by name in a global registry (`internal/shared/storage`): `default` (`app.db`), `request_log`, `request_log_read` (a read-only pool on the same file so list/stats queries don't queue behind log writes), `session`, plus a separate hotkeys DB. Schema creation and lightweight forward migrations (`storage.EnsureColumns`) run in `internal/app/bootstrap/storage_initializer.go` / `storage_migration.go`. Tests open isolated named connections (see `*_test.go` helpers such as `openRoutingTestDB`) and must never touch the real `~/.portico`.

### Relay request flow (`internal/interfaces/http/relay`)

`http.go` registers routes per platform (`/v1/messages` → Claude, `/responses[/compact]` → Codex, `/gemini/v1beta/*` and `/gemini/v1/*` → Gemini). Each request goes through these steps:
1. `inboundAuthMiddleware` (`auth.go`) accepts only the placeholder token `portico` (the legacy `code-switch` is also accepted).
2. A per-platform handler extracts request meta (model, session id).
3. `routing.go` resolves the provider: the session-bound provider wins, otherwise the CLI's current provider. Disallowed models are rejected, not rerouted. Upstream errors never bind a session.
4. Model mapping and request-body replacement rules (`request_body_replacements.go`) are applied to the raw body bytes using gjson/sjson. The body is never unmarshalled into structs, so unknown fields survive.
5. Inbound credential headers are stripped, the provider key is injected, and the request is forwarded through the shared client in `proxy_client.go` (per-provider proxy/TLS settings).
6. The response is streamed back byte for byte while token usage is parsed from SSE `data:` lines or the JSON response. Request logs are written asynchronously via `observability/infrastructure.RequestLogWriter`. For Codex, `codex_stream.go` decides success or failure from the `response.completed/failed/incomplete` events, not from the HTTP 200 status.

**Invariant:** request/response passthrough must stay byte-exact, and the only allowed edits are the `model` field and the user's body rules. Extend the passthrough and response-boundary tests rather than weakening them. Once the response writer has started writing, never append JSON error bodies. Set `PORTICO_DEBUG_RELAY=1` for verbose relay logs.

### Other contexts worth knowing

- `routing`: providers, per-platform route profiles, model mapping schemes, connectivity probes. `RegisterProxyConfigSink` pushes global proxy settings to the relay when preferences are saved, including after a configsync restore.
- `sessions`: session → provider stickiness, an LRU cache in front of SQLite, and a periodic cleanup runner.
- `platformproxy`: surgical, reversible edits to CLI config files. It uses raw JSON edits (`json_edit.go`) for Claude, TOML AST-offset edits (`toml_edit.go`) for Codex, and dotenv line edits (`env_edit.go`) for Gemini, so unrelated keys, ordering and comments survive. Previous values are snapshotted to `~/.portico/platformproxy/<platform>.json` (0600) and written with atomic file writes. It also injects the `portico` MCP entry into Claude and Codex configs.
- `mcpgateway`: the aggregator runs a reconciler that keeps upstream (stdio / streamable HTTP) connections in sync with the DB and exposes tools named `<server>__<tool>` (hash suffix past 64 chars).
- `observability`: request-log queries, stats and cost tiers. Cost comes from `resources/model-pricing` (bundled LiteLLM price table). Logs are retained for 30 days.
- `configsync`: WebDAV sync of profiles and preferences, encrypted client-side with scrypt + AES-256-GCM.

### Frontend (`frontend/src`)

Vue 3 + vue-router + vue-i18n + Tailwind 4, with no store framework: state lives in feature composables (`components/<Feature>/use*.ts`, `composables/`). `services/*.ts` wrap the generated Wails bindings, so components call services rather than bindings directly. The bindings don't use Wails IPC: `transport/httpTransport.ts` replaces the Wails transport and forwards each `$Call.ByName` to `POST /api/rpc/{Facade}.{Method}`. `services/auth.ts` + `components/Auth/Login.vue` handle the access token (read from `?token=` once, then kept in the browser). The same UI is served to the desktop window and to remote browsers. Pages are under `components/<Feature>/Index.vue`, routed from `router/index.ts`.

## Conventions

- User-facing strings go in **both** `frontend/src/locales/en.json` and `zh.json` with symmetric key sets.
- Code comments and many log/error messages are written in Chinese. Follow the surrounding style.
- Commits use Conventional Commit prefixes (`feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`).
- Run `go test -race` after touching the relay, session cache, or MCP gateway.
- Keep `portico-server` and everything it imports free of cgo and Wails; only `main.go`, `internal/app/desktop` and `cmd/bindgen` may depend on Wails.
- Releases: pushing a `v*` tag triggers `.github/workflows/release.yml` (macOS/Windows desktop artifacts bundling `portico-server`, plus Linux amd64/arm64 server archives); `scripts/publish_release.sh <tag> [notes-file]` is the local equivalent.
