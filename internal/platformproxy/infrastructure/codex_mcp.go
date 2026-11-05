package infrastructure

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"

	"github.com/pelletier/go-toml/v2"
)

// Codex 的 MCP 条目写入 ~/.codex/config.toml 的 [mcp_servers.portico]。
// 注意 config.toml 不做环境变量插值，所以鉴权头用静态 http_headers 而不是
// bearer_token_env_var——占位 token 不是真密钥，静态写入最简单可靠。

func (m *CodexManager) mcpEntryPayload() map[string]any {
	return map[string]any{
		"url": m.baseURL() + "/mcp",
		"http_headers": map[string]any{
			"Authorization": "Bearer " + codexTokenValue,
		},
	}
}

// McpStatus 报告网关 MCP 条目是否已注入 config.toml。
func (m *CodexManager) McpStatus() (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	raw, err := m.readRawConfig()
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	servers, _ := raw["mcp_servers"].(map[string]any)
	_, ok := servers[mcpEntryName]
	return ok, nil
}

// InjectMcp 外科式注入 MCP 条目（幂等：内容一致则跳过写盘）。
// 与模型代理的 Enable/Disable 完全解耦，由用户在前端显式触发。
func (m *CodexManager) InjectMcp() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	settingsPath, err := m.configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		return err
	}
	raw, err := m.readRawConfig()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			raw = make(map[string]any)
		} else {
			return err
		}
	}
	servers, _ := raw["mcp_servers"].(map[string]any)
	if servers == nil {
		servers = make(map[string]any)
	}
	payload := m.mcpEntryPayload()
	_, hasLegacy := servers[legacyMcpEntryName]
	if reflect.DeepEqual(servers[mcpEntryName], payload) && !hasLegacy {
		return nil
	}
	delete(servers, legacyMcpEntryName)
	servers[mcpEntryName] = payload
	raw["mcp_servers"] = servers
	return m.writeRawConfig(settingsPath, raw)
}

// RemoveMcp 外科式移除 MCP 条目；条目不存在或文件不存在均为 no-op。
func (m *CodexManager) RemoveMcp() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	settingsPath, err := m.configPath()
	if err != nil {
		return err
	}
	raw, err := m.readRawConfig()
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	servers, _ := raw["mcp_servers"].(map[string]any)
	_, hasCurrent := servers[mcpEntryName]
	_, hasLegacy := servers[legacyMcpEntryName]
	if !hasCurrent && !hasLegacy {
		return nil
	}
	delete(servers, mcpEntryName)
	delete(servers, legacyMcpEntryName)
	if len(servers) == 0 {
		delete(raw, "mcp_servers")
	} else {
		raw["mcp_servers"] = servers
	}
	return m.writeRawConfig(settingsPath, raw)
}

// readRawConfig 把 config.toml 读成 raw map，保留全部未知字段。
// TOML 解析失败时返回错误而不是覆盖损坏文件。
func (m *CodexManager) readRawConfig() (map[string]any, error) {
	settingsPath, err := m.configPath()
	if err != nil {
		return nil, err
	}
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		return nil, err
	}
	raw := make(map[string]any)
	if err := toml.Unmarshal(content, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func (m *CodexManager) writeRawConfig(settingsPath string, raw map[string]any) error {
	data, err := toml.Marshal(raw)
	if err != nil {
		return err
	}
	return atomicWriteFile(settingsPath, stripRedundantTableHeaders(data, "model_providers", "mcp_servers"), 0o600)
}
