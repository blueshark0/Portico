# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Portico is a Wails 3 desktop app (Go backend + Vue 3/TypeScript frontend) that runs a loopback relay on `127.0.0.1:18100` (fixed port) in front of Claude Code, Codex and Gemini CLI. It rewrites each CLI's config to point at the relay, routes requests to a user-chosen provider, records usage and cost, and aggregates MCP servers behind `/mcp`. All data lives in `~/.portico/`.

## Commands

Prerequisites: Go 1.26+, Node 20.19+/22.12+ with pnpm 9, Wails CLI `go install -tags gtk3 github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0`. On Linux: `libgtk-3-dev libwebkit2gtk-4.1-dev`.

```sh
wails3 task dev                         # desktop app with hot reload (Vite on port 9245, override via WAILS_VITE_PORT)
wails3 task build                       # build for current OS
wails3 task package                     # production packaging
wails3 task common:generate:bindings    # regenerate frontend/bindings after changing Go methods/structs exposed to the UI
```

CI checks (run these before a PR):

```sh
go vet -tags gtk3 ./...
go test -tags gtk3 -race -count=1 ./...
cd frontend && pnpm install --frozen-lockfile && pnpm run build   # vue-tsc type-check + Vite build
```

Single package / single test:

```sh
go test -tags gtk3 -race -count=1 ./internal/interfaces/http/relay/ -run TestName
```

Build gotchas:
- `main.go` embeds `frontend/dist` via `//go:embed all:frontend/dist`, so `go vet`/`go test ./...` fail to compile the root package until the frontend has been built at least once.
- `frontend/bindings/` is generated and gitignored; the frontend won't type-check until bindings are generated (needs the Go side to compile).
- Browser-only UI preview without the Go backend: `cd frontend && pnpm dev`. `main.ts` probes `/wails/runtime`, and if it's missing installs `src/mocks/installMockTransport.ts`, which answers binding calls by methodID with data from `src/mocks/demoData.ts`. New binding methods return `null` there until a mock handler is added.
- Linux builds use Wails' GTK3 backend (`-tags gtk3`, set in `build/linux/Taskfile.yml`) because the minimum supported system is Ubuntu 22.04, whose GTK4 is too old. Without the tag, Go commands that compile the root package look for GTK4.
- An optional `.env.local` (see `.env.local.example`) is loaded by Taskfile for local build proxies.

## Architecture

### Backend layout (DDD-style bounded contexts)

Each context under `internal/` (`routing`, `sessions`, `observability`, `platformproxy`, `mcpgateway`, `configsync`, `hotkeys`) is split into `domain / application / infrastructure`, with dependencies pointing inward. Two adapter layers sit on top:
- `internal/interfaces/wails/`: `*Facade` types registered as Wails services. Their exported methods become the TS functions in `frontend/bindings/portico/internal/interfaces/wails/<facade>`.
- `internal/interfaces/http/relay/`: Gin HTTP server for the CLI relay, which also mounts the MCP gateway at `/mcp`.

`internal/app/bootstrap/container.go` is the composition root. It builds every store/service/facade, wires the relay with injected options (session cache, request-log sink, token parser per platform, MCP gateway), and registers services with Wails. New services and facades go here, and new facades must also be added to `ApplicationServices()`.

### Storage

SQLite via `modernc.org/sqlite` (pure Go). Connections are registered by name in a global registry (`internal/shared/storage`): `default` (`app.db`), `request_log`, `session`, plus a separate hotkeys DB. Schema creation and lightweight forward migrations (`storage.EnsureColumns`) run in `internal/app/bootstrap/storage_initializer.go` / `storage_migration.go`. Tests open isolated named connections (see `*_test.go` helpers such as `openRoutingTestDB`) and must never touch the real `~/.portico`.

### Relay request flow (`internal/interfaces/http/relay`)

`http.go` registers routes per platform (`/v1/messages` → Claude, `/responses[/compact]` → Codex, `/gemini/v1beta/*` and `/gemini/v1/*` → Gemini). Each request goes through these steps:
1. `inboundAuthMiddleware` (`auth.go`) accepts only the placeholder token `portico`.
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

Vue 3 + vue-router + vue-i18n + Tailwind 4, with no store framework: state lives in feature composables (`components/<Feature>/use*.ts`, `composables/`). `services/*.ts` wrap the generated Wails bindings, so components call services rather than bindings directly. Pages are under `components/<Feature>/Index.vue`, routed from `router/index.ts`.

## Conventions

- User-facing strings go in **both** `frontend/src/locales/en.json` and `zh.json` with symmetric key sets.
- Code comments and many log/error messages are written in Chinese. Follow the surrounding style.
- Commits use Conventional Commit prefixes (`feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`).
- Run `go test -race` after touching the relay, session cache, or MCP gateway.
- Releases: pushing a `v*` tag triggers `.github/workflows/release.yml`; `scripts/publish_release.sh <tag> [notes-file]` is the local equivalent.
