# Portico

**English** | [中文](#中文)

A desktop app that centrally manages API providers for the [Claude Code](https://docs.anthropic.com/en/docs/claude-code), [Codex](https://github.com/openai/codex), and [Gemini](https://github.com/google-gemini/gemini-cli) CLIs.

- Switch providers for cc & codex **without restarting** them
- Per-platform default provider with **session stickiness** — routing behavior is explicit and controllable
- **Per-request usage & cost tracking** — always know what you spent

Built with [Wails 3](https://v3.wails.io) (Go + Vue 3).

## How it works

On launch, Portico starts a local HTTP relay on `127.0.0.1:18100` and rewrites the Claude Code, Codex, and Gemini CLI configs to point at it. The relay exposes only the endpoints the CLIs need:

- `/v1/messages` → your configured Claude provider
- `/responses` → your configured Codex provider
- `/gemini/v1beta/*` and `/gemini/v1/*` → your configured Gemini provider (Gemini API format)

Each request is routed to the provider bound to its session; sessions without a binding go to the platform's default provider. If a bound provider is deleted, disabled, or no longer supports the requested model, Portico clears the stale binding and re-binds to the current default.

To the CLIs, everything looks like one fixed local address — Portico transparently routes real requests to whichever providers you maintain in the app. It also includes a **MCP aggregation gateway** at `/mcp` that merges multiple upstream MCP servers into one endpoint for all three CLIs.

## Download

[macOS](https://github.com/blueshark0/Portico/releases) | [Windows](https://github.com/blueshark0/Portico/releases)

## Preview

![Dashboard (light)](resources/images/portico.png)
![Dashboard (dark)](resources/images/portico-dark.png)
![Providers](resources/images/portico-providers.png)
![Usage logs (light)](resources/images/portico-logs.png)
![Usage logs (dark)](resources/images/portico-logs-dark.png)
![Sessions](resources/images/portico-sessions.png)
![MCP servers](resources/images/portico-mcp.png)

## Development

Prerequisites:
- Go 1.26+
- Node.js 18+ with pnpm
- Wails 3.1.0 CLI: `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0`
- Linux build deps: `libgtk-4-dev`, `libwebkitgtk-6.0-dev`

```bash
wails3 task dev     # run with hot reload
wails3 task build   # build for current platform
go test -race -count=1 ./...
```

Browser-only frontend preview (no Go backend needed; serves demo data via a mock Wails transport):

```bash
cd frontend
pnpm install
pnpm dev            # http://localhost:9245
```

## Release

`scripts/publish_release.sh v0.1.0` builds and uploads `portico-macos-arm64.zip`, `portico-macos-amd64.zip`, `portico-arm64-installer.exe`, and `portico.exe`. GitHub Actions also builds releases from `v*` tags (see `.github/workflows/release.yml`).

## Model pricing source

Model pricing data is derived from [BerriAI/litellm](https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json).

---

# 中文

集中管理 Claude Code、Codex 与 Gemini 供应商的桌面应用。

- 无需重启 cc & codex, 平滑切换不同供应商
- 支持按平台维护默认供应商与会话绑定, 路由行为清晰可控
- 支持请求级别的用量统计, 花费多少清晰可见

基于 [Wails 3](https://v3.wails.io)

## 实现原理

应用启动时会初始化 在本地 18100 端口创建一个 HTTP 代理服务器, 默认绑定 :18100

并自动更新 Claude Code、Codex 配置, 指向 http://127.0.0.1:18100 服务

代理内部只暴露兼容的关键端点：

- /v1/messages 转发到配置的 Claude 供应商
- /responses 转发到 Codex 供应商；
- /gemini/v1beta/* 以及 /gemini/v1/* 以 Gemini API 格式转发到配置的 Gemini 供应商

请求会优先命中当前会话已绑定的 provider；如果没有会话绑定，则转发到该平台的默认 provider。
当已绑定 provider 已被删除、禁用或与当前请求不兼容时，Portico 会自动清理这条失效绑定，并回退到当前默认 provider 重新建立绑定。

以上流程让 cli 看到的是一个固定的本地地址，而真实请求会被 Portico 透明地路由到你在应用里维护的供应商列表。此外还内置 MCP 聚合网关（`/mcp`），把多个上游 MCP server 合并成一个端点供三个 CLI 共用。

## 下载

[macOS](https://github.com/blueshark0/Portico/releases) | [windows](https://github.com/blueshark0/Portico/releases)

## 预览
![亮色主界面](resources/images/portico.png)
![暗色主界面](resources/images/portico-dark.png)
![供应商管理](resources/images/portico-providers.png)
![用量日志亮色](resources/images/portico-logs.png)
![用量日志暗色](resources/images/portico-logs-dark.png)
![会话列表](resources/images/portico-sessions.png)
![MCP 服务](resources/images/portico-mcp.png)

## 开发准备
- Go 1.26+
- Node.js 18+
- npm / pnpm / yarn
- Wails 3.1.0 CLI：`go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0`
- Linux 构建依赖：`libgtk-4-dev`、`libwebkitgtk-6.0-dev`

## 开发运行
```bash
wails3 task dev
```

纯浏览器预览前端（无需 Go 后端，内置演示数据，通过 mock Wails transport 提供）：
```bash
cd frontend
pnpm install
pnpm dev            # http://localhost:9245
```

## 构建流程
1. 同步 build metadata：
   ```bash
   wails3 task common:update:build-assets
   ```
2. 打包 macOS `.app`：
   ```bash
   wails3 task package
   ```

### 交叉编译 Windows (macOS 环境)
1. 安装 `mingw-w64`：
   ```bash
   brew install mingw-w64
   ```
2. 运行 Windows 任务：
   ```bash
   env ARCH=amd64 wails3 task windows:build
   # 生成安装器
   env ARCH=amd64 wails3 task windows:package
   ```

## 发布
脚本 `scripts/publish_release.sh v0.1.0` 将自动打包并上传以下资产（macOS 会分别构建 arm64 与 amd64）：
- `portico-macos-arm64.zip`
- `portico-macos-amd64.zip`
- `portico-arm64-installer.exe`
- `portico.exe`

若要手动发布，可执行：
```bash
wails3 task package
env ARCH=amd64 wails3 task windows:package
scripts/publish_release.sh
```

## 常见问题
- 若 `.app` 无法打开，先执行 `wails3 task common:update:build-assets` 后再构建。
- macOS 交叉编译需要终端拥有完全磁盘访问权限，否则 `~/Library/Caches/go-build` 会报 *operation not permitted*。

## 模型价格参考源
//URL Source: http://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json

## License

[Apache 2.0](LICENSE)
