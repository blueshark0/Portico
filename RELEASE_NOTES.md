# Portico v1.0.0

Portico 的首个正式版本。它在 `127.0.0.1:18100` 启动本地 relay，位于 Claude Code、Codex、Gemini CLI 与 API 供应商之间，所有数据都保存在本机。

## 主要功能

- **供应商管理**：每个 CLI 有独立的供应商档案，可用 `Ctrl+K` 快速切换，支持连通性测试。
- **可预期的路由**：会话绑定到首次成功服务它的供应商，切换当前供应商只影响新会话。
- **模型控制**：支持模型白名单、模型映射方案和请求体改写规则。
- **用量与成本**：记录每个请求的 Token、缓存、估算成本、首响应耗时和总耗时。
- **MCP 网关**：把 stdio 与 Streamable HTTP 的 MCP server 聚合到 `/mcp`，并可一键注入 Claude Code 与 Codex。
- **WebDAV 同步**：可选，同步前在本地加密（scrypt + AES-256-GCM）。

## 安装

本版本只提供 x86_64 Linux 安装包，需要 GTK 4.14 或更新版本和 WebKitGTK 6.0，例如 Ubuntu 24.04、Debian 13 或 Fedora 的较新版本。Ubuntu 22.04 和 Debian 12 的 GTK 版本过低，无法运行。macOS、Windows 和其他架构暂未提供安装包，可以从源码构建。

| 发行版 | 文件 | 安装命令 |
| --- | --- | --- |
| Debian / Ubuntu | `portico-linux-amd64.deb` | `sudo apt install ./portico-linux-amd64.deb` |
| Fedora / RHEL | `portico-linux-amd64.rpm` | `sudo dnf install ./portico-linux-amd64.rpm` |
| Arch Linux | `portico-linux-amd64.pkg.tar.zst` | `sudo pacman -U ./portico-linux-amd64.pkg.tar.zst` |
| 其他发行版 | `portico-linux-amd64.AppImage` | `chmod +x portico-linux-amd64.AppImage` 后直接运行，系统需要已有 OpenGL ES（`libGLESv2`）、X11、Wayland 等常见图形库 |
