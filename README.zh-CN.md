<div align="center">

# Portico

**Claude Code、Codex 与 Gemini CLI 的本地供应商控制中心。**

集中管理 API 供应商、控制会话路由、查看用量与成本、聚合 MCP 服务，无需改变原有 CLI 的使用方式。

[下载](https://github.com/blueshark0/Portico/releases) · [快速开始](#快速开始) · [工作原理](#工作原理) · [本地开发](#本地开发) · [English](README.md)

<img src="resources/images/portico.png" alt="Portico 工作台" width="900">

</div>

## 功能特性

- **统一的供应商管理**：在一个桌面应用中维护 Claude Code、Codex 和 Gemini CLI 的 API 供应商。
- **会话感知路由**：每个平台可设置默认供应商，活跃会话则持续绑定到成功处理其请求的供应商。
- **平滑切换**：切换 Claude Code 和 Codex 的供应商时无需重启 CLI。
- **灵活的模型控制**：配置模型白名单和模型名称映射，支持 `claude-*` → `anthropic/claude-*` 等通配符规则。
- **清晰的用量与成本**：查看请求日志、Token 用量、模型价格、费用和会话活动。
- **MCP 聚合网关**：将多个 stdio 或远程 MCP Server 的工具聚合到统一的本地 `/mcp` 端点。
- **独立的网络配置**：为每个供应商单独设置代理，并可为本地 TLS 拦截场景显式启用跳过证书校验。
- **加密配置同步**：通过 WebDAV 加密同步配置。
- **完善的桌面体验**：支持亮色/暗色主题、中英文界面、全局快捷键和快速切换。

基于 [Wails 3](https://v3.wails.io/)、Go、Vue 3 和 TypeScript 构建。

## 下载

前往 [GitHub Releases](https://github.com/blueshark0/Portico/releases) 下载适用于 **macOS** 或 **Windows** 的最新安装包。

## 快速开始

1. **添加供应商**：在供应商页面填写接口地址、API Key，并按需配置模型规则。
2. **设置默认供应商**：为正在使用的每个 CLI 平台选择默认供应商。
3. **启用平台接入**：在工作台开启对应平台。Portico 只修改 CLI 配置中的受管字段，并将其指向本地 Relay。
4. **照常使用 CLI**：请求会经过 Portico，自动应用供应商路由和模型映射。
5. **可选：配置 MCP**：在 MCP 页面添加上游服务，并将聚合端点注入 Claude Code 或 Codex。

如需断开某个 CLI，在 Portico 中关闭对应的平台接入即可。本轮接入管理的字段会恢复原值，其余 CLI 配置保持不变。

## 工作原理

Portico 在 `127.0.0.1:18100` 启动本地 HTTP Relay。启用平台接入后，CLI 会将请求发送到这个固定的本地地址，而不再直接连接供应商。

```text
Claude Code  ── /v1/messages ───────┐
Codex        ── /responses ─────────┼──▶ Portico 本地 Relay ──▶ 已配置的供应商
Gemini CLI   ── /gemini/v1beta/* ───┘          │
                                               ├─ 默认供应商选择
                                               ├─ 会话粘性
                                               ├─ 模型校验与映射
                                               └─ 用量与成本记录

MCP 客户端   ─────────────── /mcp ─────────────▶ 聚合后的上游 MCP 工具
```

路由采用**默认供应商 + 会话粘性**，不提供多级供应商回退链。新会话首先使用平台默认供应商，随后保持绑定到成功处理请求的供应商。如果该供应商被删除、禁用或不支持当前模型，Portico 会清理失效绑定，并改用当前默认供应商。

Relay 透明转发请求体和响应，移除客户端凭据请求头，并注入所选供应商的凭据。上游错误会原样返回给 CLI。

## 更多截图

<details>
<summary>工作台、供应商、日志、会话与 MCP</summary>
<br>

| 暗色工作台 | 供应商管理 |
| --- | --- |
| ![暗色工作台](resources/images/portico-dark.png) | ![供应商管理](resources/images/portico-providers.png) |

| 用量日志 | 暗色用量日志 |
| --- | --- |
| ![用量日志](resources/images/portico-logs.png) | ![暗色用量日志](resources/images/portico-logs-dark.png) |

| 会话列表 | MCP 服务 |
| --- | --- |
| ![会话列表](resources/images/portico-sessions.png) | ![MCP 服务管理](resources/images/portico-mcp.png) |

</details>

## 本地数据与隐私

Portico 将应用状态保存在 `~/.portico/`：

| 路径 | 内容 |
| --- | --- |
| `app.db` | 供应商、路由配置、偏好设置、同步设置和 MCP Server |
| `request_log.db` | 请求用量与成本日志 |
| `session.db` | 会话与供应商的绑定关系 |
| `hotkeys.db` | 全局快捷键配置 |
| `platformproxy/*.json` | 受管 CLI 配置字段的原值和接入状态 |

供应商 API Key 保存在本地应用数据库中。Relay 仅监听回环地址，并要求使用 Portico 的本地占位 Token。请勿将 Relay 暴露到公共网络。

## 本地开发

### 环境要求

- Go **1.26+**
- Node.js **18+** 和 [pnpm](https://pnpm.io/)
- Wails **3.1.0** CLI：

  ```bash
  go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0
  ```

- Linux 构建依赖：`libgtk-4-dev`、`libwebkitgtk-6.0-dev`

### 运行桌面应用

```bash
git clone https://github.com/blueshark0/Portico.git
cd Portico
wails3 task dev
```

开发模式支持热更新。

### 在浏览器中预览前端

无需 Go 后端，Mock Wails Transport 会提供演示数据。

```bash
cd frontend
pnpm install
pnpm dev
```

打开 `http://localhost:9245`。

### 检查与测试

提交改动前，请运行与 CI 一致的核心检查：

```bash
go vet ./...
go test -race -count=1 ./...

cd frontend
pnpm install
pnpm run build
```

### 构建与打包

```bash
wails3 task common:update:build-assets  # 更新构建元数据
wails3 task build                       # 构建当前平台版本
wails3 task package                     # 生成生产安装包
```

发布带标签的版本可使用 `scripts/publish_release.sh v0.1.0`。GitHub Actions 也会根据 `v*` 标签构建发布产物，详见 [`.github/workflows/release.yml`](.github/workflows/release.yml)。

## 项目结构

```text
internal/
  app/                 组合根与桌面外壳
  interfaces/          HTTP Relay 与 Wails 接口层
  routing/             供应商、路由配置与模型映射
  sessions/            会话与供应商的粘性绑定
  observability/       请求日志、Token 解析与统计
  mcpgateway/          MCP 聚合与上游连接
  platformproxy/       CLI 配置的精确字段更新
  configsync/          WebDAV 加密同步
  shared/              存储、日志与后台任务
frontend/              Vue 3 + TypeScript 桌面界面
resources/             图片与模型定价数据
```

架构细节和开发约束请参阅 [CLAUDE.md](CLAUDE.md)。

## 参与贡献

欢迎提交 Issue 和 Pull Request。请保持改动聚焦，为行为变化补充测试，运行上述检查；修改界面文案时，请同步更新中英文语言文件。

安全漏洞请按照 [SECURITY.md](SECURITY.md) 中的流程私下报告，不要创建公开 Issue。

## 致谢

- 桌面应用框架：[Wails](https://wails.io/)
- MCP 支持：[Model Context Protocol Go SDK](https://github.com/modelcontextprotocol/go-sdk)
- 模型定价数据：[BerriAI/litellm](https://github.com/BerriAI/litellm)

## 许可证

Portico 基于 [Apache License 2.0](LICENSE) 开源。
