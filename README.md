<div align="center">

<img src="build/appicon-128.png" alt="Portico" width="88" height="88" />

# Portico

**The API provider control center for AI coding CLIs.**

[Claude Code](https://github.com/anthropics/claude-code) · [Codex](https://github.com/openai/codex) · [Gemini CLI](https://github.com/google-gemini/gemini-cli)

[Download](https://github.com/blueshark0/Portico/releases/latest) · [Quick start](#quick-start) · [Headless server](#headless-server) · [How it works](#how-it-works) · [MCP gateway](#mcp-gateway) · [Contributing](#contributing)

**English** · [简体中文](README.zh-CN.md)

[![CI](https://github.com/blueshark0/Portico/actions/workflows/test.yml/badge.svg)](https://github.com/blueshark0/Portico/actions/workflows/test.yml)
[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue)](LICENSE)
![Go 1.26](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)
![Wails v3](https://img.shields.io/badge/Wails-v3-DF0000)
![Vue 3 + TypeScript](https://img.shields.io/badge/Vue-3%20%2B%20TypeScript-42b883?logo=vue.js&logoColor=white)

</div>

Portico sits between your AI coding CLIs and their API providers. A background service, `portico-server`, runs a local relay on `127.0.0.1:18100`, points each CLI at it, and forwards requests to the provider you choose — with session-aware routing, model remapping, per-request usage and cost tracking, and an aggregated MCP endpoint. You manage it from the desktop app, or from a browser on machines without a desktop.

- **One local address for every CLI.** Switch providers in the app or with the <kbd>⌘K</kbd> / <kbd>Ctrl+K</kbd> palette, without editing CLI config files again.
- **Routing you can reason about.** Each CLI has a current provider, and a session stays with the provider that served it. No hidden retry chains, no silent fallbacks.
- **Know what you spend.** Tokens, cache reads and writes, estimated cost, time to first response and total duration for every request.
- **Transparent by design.** Requests and responses — including SSE streams — are forwarded byte for byte. Only the `model` field and your own body-rewrite rules touch the payload.
- **Careful with your configs.** Portico patches only the fields it manages, keeps their previous values, and restores them when you turn integration off.
- **Desktop or headless.** The same backend serves the desktop window and a token-protected web UI, so Portico also runs on servers with no desktop environment.

Built with [Wails 3](https://v3.wails.io) (Go) and Vue 3 + TypeScript. All data stays on your machine.

## Preview

<img src="resources/images/portico.png" alt="Portico dashboard" width="100%" />

<details>
<summary>More screenshots — dark mode, providers, usage logs, sessions, MCP servers</summary>
<br />

<img src="resources/images/portico-dark.png" alt="Dashboard, dark mode" width="100%" />
<img src="resources/images/portico-providers.png" alt="Provider management" width="100%" />
<img src="resources/images/portico-logs.png" alt="Usage logs" width="100%" />
<img src="resources/images/portico-logs-dark.png" alt="Usage logs, dark mode" width="100%" />
<img src="resources/images/portico-sessions.png" alt="Sessions" width="100%" />
<img src="resources/images/portico-mcp.png" alt="MCP servers" width="100%" />
</details>

## Features

| | |
| --- | --- |
| **Provider management** | Separate provider profiles per CLI, quick switching with <kbd>⌘K</kbd> / <kbd>Ctrl+K</kbd>, connection tests, copying profiles across CLIs, and archive / restore. |
| **Predictable routing** | A session is bound to the provider that served it. Switching the current provider affects new sessions; an invalid binding (provider deleted, archived, or unable to serve the model) is cleared and re-bound automatically. |
| **Model control** | Per-provider model allowlists (exact or wildcard), named **model mapping schemes** you can save and switch between (`claude-*` → `anthropic/claude-*`), and request-body override / conditional-replace rules. |
| **Networking** | Global HTTP proxy with per-provider inherit / custom / direct overrides, and an explicit, per-provider TLS-skip option for local intercepting proxies. |
| **Usage & cost** | Per-request log with tokens, cache, cost, first response time, duration and status; an overview dashboard with success rate, token breakdown and a 7-day trend; per-session traces; application logs. |
| **MCP gateway** | Aggregate stdio and Streamable HTTP MCP servers behind one endpoint (`/mcp`) with namespaced tools (`<server>__<tool>`), per-upstream call timeouts, connection tests and diagnostics, and one-click injection into Claude Code and Codex. |
| **Sync & preferences** | Optional WebDAV sync of provider profiles and appearance preferences, encrypted client-side (scrypt + AES-256-GCM) with local backups before a restore; light / dark / system theme, English and Chinese UI. |
| **Desktop shell** | System tray with light and dark icons, and dock visibility control on macOS. |

## Supported clients

| CLI | Relay endpoints | Managed config fields | MCP injection |
| --- | --- | --- | --- |
| **Claude Code** | `POST /v1/messages` | `~/.claude/settings.json` → `env.ANTHROPIC_BASE_URL`, `env.ANTHROPIC_AUTH_TOKEN` | `~/.claude.json` → `mcpServers.portico` |
| **Codex** | `POST /responses`, `POST /responses/compact` | `~/.codex/config.toml` → `model_provider`, `model_providers.portico.*`; `~/.codex/auth.json` → `OPENAI_API_KEY` | `~/.codex/config.toml` → `[mcp_servers.portico]` |
| **Gemini CLI** | `POST /gemini/v1beta/*`, `POST /gemini/v1/*` | `~/.gemini/.env` → `GOOGLE_GEMINI_BASE_URL`, `GEMINI_API_KEY` | — (connect it manually, see [MCP gateway](#mcp-gateway)) |

The CLIs authenticate to the relay with the placeholder token `portico`, never with your real key. Portico strips client credentials and injects the selected provider's API key on the outbound request only. The relay listens on loopback and is not exposed to your network.

## Installation

Grab a build from [**Releases**](https://github.com/blueshark0/Portico/releases/latest).

| Platform | Asset |
| --- | --- |
| macOS · Apple Silicon | `portico-macos-arm64.zip` |
| macOS · Intel | `portico-macos-amd64.zip` |
| Windows · x64 installer | `Portico-amd64-installer.exe` |
| Windows · x64 portable | `portico-windows-amd64-portable.zip` |
| Linux server (headless) · x64 / arm64 | `portico-server-linux-amd64.tar.gz` / `portico-server-linux-arm64.tar.gz` — see [Headless server](#headless-server) |
| Linux desktop | [Build from source](#building-from-source) |

**macOS.** Unzip and move `Portico.app` to `/Applications`. Release builds are ad-hoc signed, not notarized, so Gatekeeper may refuse the first launch:

```sh
xattr -cr /Applications/Portico.app
```

…or approve the app once under *System Settings → Privacy & Security*.

**Windows.** The NSIS installer bootstraps the WebView2 runtime if needed; the portable build requires WebView2 to be installed already. Keep `Portico.exe` and `portico-server.exe` from the portable zip in the same folder — the desktop app starts the backend from there.

**Upgrading from Code Switch.** On first launch Portico renames `~/.code-switch` to `~/.portico`, keeping your profiles, request logs and proxy state (an existing `~/.portico` is never touched). The legacy `code-switch` placeholder token is still accepted, so an existing CLI integration keeps working; re-enable it once to move fully to the new placeholder.

## Quick start

1. **Add a provider** under *Providers → Add provider*: a name, the CLI it belongs to, its API base URL and API key. *Test connection* checks reachability without sending credentials or consuming API quota.
2. **Make it current** with *Set as current*, or pick it from the <kbd>⌘K</kbd> / <kbd>Ctrl+K</kbd> palette.
3. **Enable integration** for that CLI on the Dashboard with the *Managed by Portico* toggle. Portico rewrites only the fields listed above and stores their previous values.
4. **Restart the CLI** if it was already running, then work as usual. Every request now flows through Portico.

After that, switching providers is an in-app action — no config editing, no restart. Note that an **existing session keeps its provider**; switching affects new sessions. To move a live session, use *Sessions → Unbind session*, or start a new session in the CLI.

Turning the toggle off restores the previous values. Provider names and API keys are stored locally in `~/.portico/app.db`.

**Quitting the desktop app does not stop the relay.** The backend is a separate process, so your CLIs keep working while the window is closed or the app has quit. Use the tray menu's *Quit and stop service* to stop it as well.

## Headless server

On a machine without a desktop, run the backend directly and manage it from a browser or the terminal UI. Run it as the same user that runs the CLIs: Portico edits that user's `~/.claude`, `~/.codex` and `~/.gemini`.

```sh
tar -xzf portico-server-linux-amd64.tar.gz
cd portico-server-linux-amd64
./portico-server --listen 0.0.0.0:18101   # relay stays on 127.0.0.1:18100
./portico-server token show               # prints the access token and a sign-in link
```

Open `http://<server>:18101/?token=<token>` (or open `http://<server>:18101` and paste the token). The token is saved in that browser and removed from the address bar.

### Terminal UI

`portico-server tui` opens a full-screen terminal UI over the same management API — handy over SSH. On the server itself it finds the running backend and needs no token:

```sh
./portico-server tui
PORTICO_ADMIN_TOKEN=<token> ./portico-server tui --url https://<server>:18101   # from another machine
```

It covers the day-to-day tasks: turning CLI integration on and off, switching the default provider, adding/editing/archiving providers and testing connections, browsing request logs, unbinding sessions, toggling MCP servers and gateway injection, and following the app log. Model mapping, request-body rewrites, proxy settings and config sync are only in the web UI. Press `?` for key bindings; quitting the TUI leaves the backend running.

| | |
| --- | --- |
| **Management port** | `127.0.0.1:18101` by default; `--listen` / `PORTICO_ADMIN_LISTEN` to change it. The relay always stays on loopback. |
| **Authentication** | Requests from another host need `Authorization: Bearer <token>`. Requests from the same machine (loopback address *and* loopback `Host`) are exempt, so an SSH tunnel such as `ssh -L 18101:127.0.0.1:18101 server` works without a token. Cross-site browser requests are always rejected. |
| **Token** | Generated on first start in `~/.portico/admin_token` (`0600`). `portico-server token reset` rotates it; the running server picks up the new token immediately. |
| **TLS** | `--tls-cert` / `--tls-key` (or `PORTICO_ADMIN_TLS_CERT` / `PORTICO_ADMIN_TLS_KEY`). Without TLS the token travels in plain text — use HTTPS or a tunnel when exposing the port. |
| **Reverse proxy** | Keep `--listen` on loopback and set `--always-require-token` (`PORTICO_ADMIN_ALWAYS_TOKEN=1`). A proxy on the same host would otherwise look like a local client. |

To keep it running, install the bundled systemd **user** unit:

```sh
sudo install -m 0755 portico-server /usr/local/bin/
mkdir -p ~/.config/systemd/user && cp portico-server.service ~/.config/systemd/user/
systemctl --user daemon-reload && systemctl --user enable --now portico-server
loginctl enable-linger "$USER"   # keep running after you log out
```

Override the listen address or TLS settings with `systemctl --user edit portico-server` (see the comments in the unit file).

## How it works

```mermaid
flowchart LR
    CC["Claude Code"]
    CX["Codex"]
    GM["Gemini CLI"]

    subgraph Portico["Portico · 127.0.0.1:18100"]
        direction TB
        AUTH["Placeholder-token auth"]
        ROUTE["Session + current provider routing"]
        MAP["Model mapping and body rules"]
        FWD["Key injection and forwarding"]
        MCP["MCP gateway /mcp"]
        LOG[("Usage and cost · SQLite")]
        AUTH --> ROUTE --> MAP --> FWD
        ROUTE -.->|usage| LOG
    end

    CC --> AUTH
    CX --> AUTH
    GM --> AUTH
    FWD --> P1["Claude provider"]
    FWD --> P2["Codex provider"]
    FWD --> P3["Gemini provider"]
    CC -.->|MCP tools| MCP
    CX -.->|MCP tools| MCP
    MCP --> U1["stdio MCP servers"]
    MCP --> U2["HTTP MCP servers"]
```

Portico runs as two processes. **`portico-server`** owns everything that matters — the relay, CLI config patching, the MCP gateway, the databases — and serves the management API and web UI on `127.0.0.1:18101`. **The desktop app** is only a window, tray icon and dock entry: on launch it reuses a running backend or starts the bundled one, then loads the web UI in its window. Browsers on other machines use the same UI.

Each request goes through five steps:

1. **Authenticate.** The inbound token must be the `portico` placeholder (`Authorization`, `X-Api-Key`, `X-Goog-Api-Key` or `?key=`); anything else gets a `401`.
2. **Resolve.** The session's bound provider wins; otherwise the CLI's current provider is used. A request whose model the provider does not allow is rejected rather than silently rerouted. Upstream errors are returned verbatim and never bind a session.
3. **Rewrite.** The requested model is mapped to the provider's own model name, and your body-rewrite rules are applied. The body stays raw bytes throughout — unknown fields are preserved.
4. **Forward.** Client credentials are stripped, the provider key is injected, and the request goes out through a shared HTTP client using the provider's proxy and TLS settings. Cancelling the CLI request cancels the upstream one.
5. **Record.** Token usage is parsed from SSE `data:` lines or the response JSON without altering the stream, and written to `request_log.db` in the background. Cost is estimated from the bundled pricing table.

### What changes on disk

Integration is opt-in per CLI and reversible. Portico uses field-level patches — raw JSON edits for Claude Code, AST-offset edits for Codex TOML, line edits for Gemini dotenv — so unrelated keys, ordering and comments survive. Previous values live in `~/.portico/platformproxy/<platform>.json` (mode `0600`, since a snapshot may contain an old API key) and are restored when you disable integration.

## MCP gateway

Portico also aggregates your MCP servers into a single Streamable HTTP endpoint, so every CLI shares one tool inventory instead of each running its own copies.

| | |
| --- | --- |
| **Endpoint** | `http://127.0.0.1:18100/mcp` (POST / GET / DELETE) |
| **Auth** | `Authorization: Bearer portico` |
| **Upstreams** | `stdio` (command, args, env) or `http` (Streamable HTTP URL, headers) |
| **Tool naming** | `<server>__<tool>`, e.g. `context7__resolve-library-id`; names over 64 chars get a hash suffix |
| **Safety** | Upstream headers are only sent to the host they were configured for, so a redirect cannot exfiltrate credentials. Server names may not contain `__`. |

1. Add servers on the **MCP Servers** page, test the connection, and enable them.
2. Turn on CLI injection for **Claude Code** and **Codex**; only the `portico` entry is touched, your own MCP entries are left alone. Restart the CLI to pick it up.
3. Use **Gateway diagnostics** to inspect upstream health, latency, tool schemas and downstream naming.

For any other MCP client that supports Streamable HTTP, connect manually to the endpoint above with the bearer header. Enabled upstreams and their tools are visible downstream immediately, and upstream `tools/list_changed` notifications refresh the registry without a reconnect.

## Data & privacy

Everything lives in `~/.portico/` on your machine:

| Path | Contents |
| --- | --- |
| `app.db` | Provider profiles and API keys, preferences, WebDAV settings, MCP servers |
| `request_log.db` | Per-request usage records, retained for **30 days** |
| `session.db` | Session → provider bindings |
| `hotkeys.db` | Hotkey settings |
| `platformproxy/<platform>.json` | Previous values of managed CLI config fields (`0600`) |
| `admin_token` | Access token for remote management (`0600`) |
| `server.json` | Address, PID and version of the running backend (removed on exit) |
| `app.log` | Rotating application log (10 MB, 5 backups, 14 days) |
| `sync-backups/` | Local snapshots taken before a cloud restore |

- **API keys are stored in plaintext** in the local SQLite database, and the UI displays them so you can copy and edit them. Protect your user account accordingly.
- **No telemetry.** Portico has no analytics, update pinging or hard-coded service endpoints. Network traffic goes only to the providers, MCP upstreams and WebDAV server you configure.
- **No request contents.** Only structured metrics are logged — never prompts, completions or raw SSE events. Costs are estimates in USD from the bundled pricing table, not your provider's invoice.
- **The management port listens on loopback by default.** Exposing it is opt-in and requires the access token from other hosts.
- **WebDAV sync is optional** and off until you configure it. Snapshots are encrypted on your machine with your passphrase before upload.

Report vulnerabilities privately — see [SECURITY.md](SECURITY.md).

## Troubleshooting

<details>
<summary><strong>The CLI gets 401 from the relay</strong></summary>

Its config no longer carries the `portico` placeholder — usually because another tool rewrote the file. Toggle the CLI's integration off and on again on the Dashboard. This only fixes the managed fields; your other settings are untouched.
</details>

<details>
<summary><strong>The window says it cannot connect to the Portico service</strong></summary>

The backend needs `127.0.0.1:18100` (relay, fixed) and `127.0.0.1:18101` (management). Check whether a different relay or another program is holding them, then check `~/.portico/app.log` and use *Reconnect service* in the tray menu. On Windows and macOS, make sure `portico-server` sits next to the Portico executable.
</details>

<details>
<summary><strong>The CLIs still work after I quit Portico</strong></summary>

That is expected: the relay runs in the background service. Choose *Quit and stop service* in the tray menu, or run `curl -X POST -H 'Content-Type: application/json' http://127.0.0.1:18101/api/shutdown`.
</details>

<details>
<summary><strong>The web UI keeps asking for a token</strong></summary>

The token was rotated or mistyped. Run `portico-server token show` on the server and use the printed link. Behind a reverse proxy, every request needs the token, including your own.
</details>

<details>
<summary><strong>I switched providers but the session still uses the old one</strong></summary>

That is session stickiness working as intended. Use *Sessions → Unbind session* to re-route it to the current provider, or start a new session in the CLI.
</details>

<details>
<summary><strong>A provider is skipped or a model is rejected</strong></summary>

Check the provider's supported-model allowlist and the active mapping scheme. An empty allowlist with no mappings accepts every model; a populated one only accepts what it lists.
</details>

<details>
<summary><strong>Costs or token counts look off</strong></summary>

Costs are derived from the bundled LiteLLM price table and may differ from your provider's billing, especially for cached or reasoning tokens. Total tokens = input + output; cache and reasoning tokens may already be included in those numbers.
</details>

**Debug logging:** set `PORTICO_DEBUG_RELAY=1` before launching Portico for verbose per-request relay logs. *Usage logs → Application logs* shows startup, MCP connection and upstream errors.

## Development

### Prerequisites

- **Go 1.26+**
- **Node.js 20.19+ or 22.12+** (required by Vite 7) with **pnpm 9**
- **Wails 3.1.0 CLI**

  ```sh
  go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0
  ```

- **Linux only:** `libgtk-4-dev` and `libwebkitgtk-6.0-dev`

  ```sh
  sudo apt-get install -y libgtk-4-dev libwebkitgtk-6.0-dev
  ```

### Building from source

```sh
git clone https://github.com/blueshark0/Portico.git
cd Portico

wails3 task build:server   # backend + embedded web UI → bin/portico-server (pure Go, no cgo)
wails3 task run:server     # build and run it in the foreground
wails3 task dev            # desktop shell; it starts bin/portico-server as its backend
wails3 task build          # desktop app + backend for the current platform
wails3 task package        # production packaging (.app / NSIS / AppImage + deb + rpm)
```

Frontend dependencies and bindings are generated by the build tasks. The frontend calls the backend through Wails-generated TypeScript bindings, but the calls go over HTTP to `POST /api/rpc/{Facade}.{Method}` on `portico-server`, not through Wails IPC. To regenerate bindings by hand after changing Go structs or facade methods:

```sh
wails3 task common:generate:bindings
```

New facades must be registered in both `internal/app/bootstrap/container.go` (`Facades()`) and `cmd/bindgen` (which exists only for binding generation).

**Frontend development** — Vite proxies `/api` to a running backend (`PORTICO_ADMIN_URL`, default `http://127.0.0.1:18101`). With no backend running, or with `VITE_PORTICO_MOCK=1`, the UI serves demo data through a mock transport:

```sh
wails3 task run:server     # optional: real data
cd frontend
pnpm install
pnpm dev                   # http://localhost:9245
```

### Checks

These are exactly what CI runs on every push and pull request:

```sh
go vet ./...
go test -race -count=1 ./...

cd frontend
pnpm install --frozen-lockfile
pnpm run build        # vue-tsc type-check + production Vite build
```

Go tests must never touch the real `~/.portico`; they use isolated per-test databases. Run `go test -race` whenever you change the relay, the session cache or the MCP gateway.

### Packaging and releasing

Update build metadata before packaging:

```sh
wails3 task common:update:build-assets
wails3 task package
```

Cross-compile Windows from macOS with `mingw-w64` (`brew install mingw-w64`):

```sh
env ARCH=amd64 wails3 task windows:build
env ARCH=amd64 wails3 task windows:package   # requires makensis
```

Pushing a `v*` tag triggers [`.github/workflows/release.yml`](.github/workflows/release.yml), which builds macOS (arm64, amd64) and Windows (amd64) desktop artifacts, Linux (amd64, arm64) server archives, and publishes a release. `scripts/publish_release.sh <tag> [notes-file]` is the equivalent local entry point.

## Project structure

```
main.go                              desktop shell entry point (window, tray, dock)
cmd/portico-server/                  backend entry point: relay, management API, web UI
cmd/bindgen/                         registers facades for TS binding generation only
internal/
  app/bootstrap/                     composition root, storage init and migrations
  app/desktop/                       window, tray and dock shell
  app/sidecar/                       desktop ↔ backend: discover, spawn, stop portico-server
  interfaces/http/relay/             loopback relay: auth, routing, forwarding, MCP mount
  interfaces/http/admin/             management API: token auth, facade RPC, static web UI
  interfaces/tui/                    terminal UI (portico-server tui), a client of the management API
  interfaces/wails/                  facades exposed to the UI (served over the admin RPC)
  routing/                           providers, route profiles, model mapping schemes, probes
  sessions/                          session → provider stickiness (LRU + SQLite)
  observability/                     request logs, token parsing, stats, app log reader
  platformproxy/                     surgical CLI config edits, MCP injection, restore
  mcpgateway/                        upstream pool, tool registry, aggregation
  configsync/                        encrypted WebDAV config sync
  hotkeys/                           hotkey storage
  shared/                            kernel types, storage, logging, server info, background worker
resources/model-pricing/             bundled pricing table and cost calculation
frontend/                            Vue 3 + TypeScript + Vite + Tailwind UI
build/                               per-platform packaging configuration
```

The backend is organized into bounded contexts under `internal/`, each split into `domain / application / infrastructure`, with dependencies pointing inward (`interfaces → application → domain`). New services are wired in `internal/app/bootstrap/container.go`. Frontend state lives in feature composables; there is no store framework. The desktop shell links none of the backend packages; it only talks to `portico-server` over HTTP.

## Contributing

Contributions are welcome.

- **Bugs and ideas:** use the [bug report](https://github.com/blueshark0/Portico/issues/new?template=bug_report.yml) and [feature request](https://github.com/blueshark0/Portico/issues/new?template=feature_request.yml) templates. For anything non-trivial, open an issue first so the approach can be discussed before you invest time.
- **Before opening a PR:** run the [checks](#checks) above. Keep commits focused and use [Conventional Commits](https://www.conventionalcommits.org/) prefixes (`feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`).
- **User-facing text** must be added to both `frontend/src/locales/en.json` and `zh.json`, keeping the key sets symmetric.
- **Relay changes** must preserve byte-level passthrough of requests and responses; extend the existing passthrough tests rather than weakening them.
- **Security issues** should be reported privately, not as public issues — see [SECURITY.md](SECURITY.md).

## Acknowledgements

- [Wails](https://v3.wails.io) — desktop application framework
- [BerriAI/litellm](https://github.com/BerriAI/litellm) — source of the bundled [model pricing data](resources/model-pricing/model_prices_and_context_window.json)
- [modelcontextprotocol/go-sdk](https://github.com/modelcontextprotocol/go-sdk) — MCP server and client implementation behind the gateway

Portico is an independent open-source project and is not affiliated with, endorsed by, or sponsored by Anthropic, OpenAI or Google. Claude Code, Codex and Gemini are trademarks of their respective owners.

## License

[Apache License 2.0](LICENSE) © Portico contributors
