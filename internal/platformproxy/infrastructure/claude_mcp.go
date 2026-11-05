package infrastructure

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
)

// mcpEntryName 注入到各 CLI 配置中的 MCP 条目名，CLI 内工具显示为
// mcp__portico__<工具名>。
const (
	mcpEntryName       = "portico"
	legacyMcpEntryName = "code-switch"
)

// claudeConfigFileName 是 Claude Code 的用户级配置文件（home 根下，
// 注意不是 ~/.claude/ 目录），顶层 mcpServers 即 user scope，全项目生效
// 且无需逐项目批准。
const claudeConfigFileName = ".claude.json"

func (m *ClaudeManager) claudeConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, claudeConfigFileName), nil
}

func (m *ClaudeManager) mcpEndpoint() string {
	return m.baseURL() + "/mcp"
}

func (m *ClaudeManager) mcpEntryPayload() map[string]any {
	return map[string]any{
		"type": "http",
		"url":  m.mcpEndpoint(),
		"headers": map[string]any{
			"Authorization": "Bearer " + claudeAuthTokenValue,
		},
	}
}

// McpStatus 报告网关 MCP 条目是否已注入 ~/.claude.json。
func (m *ClaudeManager) McpStatus() (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	path, err := m.claudeConfigPath()
	if err != nil {
		return false, err
	}
	raw, err := readJSONMapFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	servers, _ := raw["mcpServers"].(map[string]any)
	_, ok := servers[mcpEntryName]
	return ok, nil
}

// InjectMcp 外科式注入 MCP 条目。
//
// ~/.claude.json 是 Claude Code 持续写入的活文件（会话状态等），这里绝不
// 做整文件备份-恢复，只读-改-写 mcpServers 单键：解析失败拒绝覆盖、内容
// 一致跳过写入、原子写避免半截文件。残余的并发写 last-writer-wins 窗口
// 可接受（最坏情况是条目被 CLI 覆写消失，用户在 UI 重新注入即可）。
func (m *ClaudeManager) InjectMcp() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	path, err := m.claudeConfigPath()
	if err != nil {
		return err
	}
	raw, err := readJSONMapFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			raw = make(map[string]any)
		} else {
			return err
		}
	}
	servers, _ := raw["mcpServers"].(map[string]any)
	if servers == nil {
		servers = make(map[string]any)
	}
	payload := m.mcpEntryPayload()
	_, hasLegacy := servers[legacyMcpEntryName]
	if reflect.DeepEqual(servers[mcpEntryName], payload) && !hasLegacy {
		return nil // 幂等：内容一致不写盘，缩小并发写冲突窗口
	}
	delete(servers, legacyMcpEntryName)
	servers[mcpEntryName] = payload
	raw["mcpServers"] = servers
	return writeJSONMapFile(path, raw)
}

// RemoveMcp 外科式移除 MCP 条目；条目不存在或文件不存在均为 no-op。
func (m *ClaudeManager) RemoveMcp() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	path, err := m.claudeConfigPath()
	if err != nil {
		return err
	}
	raw, err := readJSONMapFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	servers, _ := raw["mcpServers"].(map[string]any)
	_, hasCurrent := servers[mcpEntryName]
	_, hasLegacy := servers[legacyMcpEntryName]
	if !hasCurrent && !hasLegacy {
		return nil
	}
	delete(servers, mcpEntryName)
	delete(servers, legacyMcpEntryName)
	if len(servers) == 0 {
		delete(raw, "mcpServers")
	} else {
		raw["mcpServers"] = servers
	}
	return writeJSONMapFile(path, raw)
}

// readJSONMapFile 读取 JSON 对象文件；解析失败返回错误而不是覆盖损坏文件。
func readJSONMapFile(path string) (map[string]any, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(content, &raw); err != nil {
		return nil, fmt.Errorf("解析 %s 失败（文件可能损坏），拒绝覆盖: %w", path, err)
	}
	return raw, nil
}

func writeJSONMapFile(path string, raw map[string]any) error {
	payload, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(path, payload, 0o600)
}
