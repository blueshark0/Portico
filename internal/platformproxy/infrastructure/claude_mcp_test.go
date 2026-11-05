package infrastructure

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newClaudeManager(t *testing.T) *ClaudeManager {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	return NewClaudeManager("127.0.0.1:18100")
}

func claudeConfigPath(t *testing.T) string {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(home, ".claude.json")
}

func readClaudeConfig(t *testing.T) map[string]any {
	t.Helper()
	content, err := os.ReadFile(claudeConfigPath(t))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(content, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return raw
}

func TestClaudeInjectIntoMissingFile(t *testing.T) {
	m := newClaudeManager(t)
	if ok, err := m.McpStatus(); err != nil || ok {
		t.Fatalf("未注入时 McpStatus: %v, %v", ok, err)
	}
	if err := m.InjectMcp(); err != nil {
		t.Fatalf("InjectMcp: %v", err)
	}
	raw := readClaudeConfig(t)
	servers, _ := raw["mcpServers"].(map[string]any)
	entry, _ := servers["portico"].(map[string]any)
	if entry["type"] != "http" || entry["url"] != "http://127.0.0.1:18100/mcp" {
		t.Fatalf("条目内容: %v", entry)
	}
	headers, _ := entry["headers"].(map[string]any)
	if headers["Authorization"] != "Bearer portico" {
		t.Fatalf("headers: %v", headers)
	}
	if ok, _ := m.McpStatus(); !ok {
		t.Fatal("注入后 McpStatus 应为 true")
	}
}

func TestClaudeInjectPreservesOtherContent(t *testing.T) {
	m := newClaudeManager(t)
	seed := `{
		"theme": "dark",
		"mcpServers": {"user-own": {"type": "stdio", "command": "x"}},
		"projects": {"/some/path": {"history": []}}
	}`
	if err := os.WriteFile(claudeConfigPath(t), []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := m.InjectMcp(); err != nil {
		t.Fatalf("InjectMcp: %v", err)
	}
	raw := readClaudeConfig(t)
	if raw["theme"] != "dark" {
		t.Fatalf("其他顶层键丢失: %v", raw)
	}
	if _, ok := raw["projects"]; !ok {
		t.Fatal("projects 键丢失")
	}
	servers := raw["mcpServers"].(map[string]any)
	if _, ok := servers["user-own"]; !ok {
		t.Fatal("用户既有 MCP 条目被覆盖")
	}
	if _, ok := servers["portico"]; !ok {
		t.Fatal("网关条目未注入")
	}
}

func TestClaudeInjectReplacesLegacyEntry(t *testing.T) {
	m := newClaudeManager(t)
	seed := `{"mcpServers":{"code-switch":{"type":"http","url":"http://127.0.0.1:18100/mcp"},"user-own":{"type":"stdio","command":"x"}}}`
	if err := os.WriteFile(claudeConfigPath(t), []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := m.InjectMcp(); err != nil {
		t.Fatal(err)
	}
	servers := readClaudeConfig(t)["mcpServers"].(map[string]any)
	if _, ok := servers[legacyMcpEntryName]; ok {
		t.Fatal("旧 MCP 条目应被移除")
	}
	if _, ok := servers[mcpEntryName]; !ok {
		t.Fatal("新 MCP 条目应被注入")
	}
	if _, ok := servers["user-own"]; !ok {
		t.Fatal("用户条目应保留")
	}
}

func TestClaudeInjectIdempotentSkipsWrite(t *testing.T) {
	m := newClaudeManager(t)
	if err := m.InjectMcp(); err != nil {
		t.Fatal(err)
	}
	// 把 home 目录设为只读：若第二次注入仍尝试写盘（tmp+rename）会失败，
	// 幂等跳过则安然无恙。
	home, _ := os.UserHomeDir()
	if err := os.Chmod(home, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(home, 0o755) })
	if err := m.InjectMcp(); err != nil {
		t.Fatalf("内容一致时不应写盘: %v", err)
	}
}

func TestClaudeRemoveOnlyOwnEntry(t *testing.T) {
	m := newClaudeManager(t)
	seed := `{"mcpServers": {"user-own": {"type": "stdio", "command": "x"}}}`
	if err := os.WriteFile(claudeConfigPath(t), []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := m.InjectMcp(); err != nil {
		t.Fatal(err)
	}
	if err := m.RemoveMcp(); err != nil {
		t.Fatalf("RemoveMcp: %v", err)
	}
	raw := readClaudeConfig(t)
	servers := raw["mcpServers"].(map[string]any)
	if _, ok := servers["portico"]; ok {
		t.Fatal("网关条目应被移除")
	}
	if _, ok := servers["user-own"]; !ok {
		t.Fatal("用户条目应保留")
	}
	// 再移除用户条目后整个 mcpServers 键应消失
	if err := m.RemoveMcp(); err != nil {
		t.Fatal(err)
	}
	if ok, _ := m.McpStatus(); ok {
		t.Fatal("移除后 McpStatus 应为 false")
	}
}

func TestClaudeRemoveEmptiesServersKey(t *testing.T) {
	m := newClaudeManager(t)
	if err := m.InjectMcp(); err != nil {
		t.Fatal(err)
	}
	if err := m.RemoveMcp(); err != nil {
		t.Fatal(err)
	}
	raw := readClaudeConfig(t)
	if _, ok := raw["mcpServers"]; ok {
		t.Fatal("mcpServers 空了应整键移除")
	}
	// 文件不存在时 Remove 为 no-op
	_ = os.Remove(claudeConfigPath(t))
	if err := m.RemoveMcp(); err != nil {
		t.Fatalf("缺失文件 Remove 应无错误: %v", err)
	}
}

func TestClaudeCorruptFileRefusesOverwrite(t *testing.T) {
	m := newClaudeManager(t)
	corrupt := []byte(`{"mcpServers": {broken`)
	path := claudeConfigPath(t)
	if err := os.WriteFile(path, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := m.InjectMcp(); err == nil {
		t.Fatal("损坏文件应报错")
	}
	if err := m.RemoveMcp(); err == nil {
		t.Fatal("损坏文件 Remove 也应报错")
	}
	content, _ := os.ReadFile(path)
	if string(content) != string(corrupt) {
		t.Fatal("损坏文件不应被改写")
	}
	if _, err := m.McpStatus(); err == nil {
		t.Fatal("损坏文件 McpStatus 应报错")
	}
}

// EnableProxy/DisableProxy（操作 ~/.claude/settings.json）不应触碰
// ~/.claude.json 里的 MCP 条目——两者生命周期正交。
func TestClaudeProxyToggleKeepsMcpEntry(t *testing.T) {
	m := newClaudeManager(t)
	if err := m.InjectMcp(); err != nil {
		t.Fatal(err)
	}
	if err := m.EnableProxy(); err != nil {
		t.Fatal(err)
	}
	if err := m.DisableProxy(); err != nil {
		t.Fatal(err)
	}
	if ok, err := m.McpStatus(); err != nil || !ok {
		t.Fatalf("代理开关不应影响 MCP 条目: %v, %v", ok, err)
	}
}

func TestStripRedundantTableHeaders(t *testing.T) {
	input := "[model_providers]\n[mcp_servers]\n[mcp_servers.portico]\nurl = \"x\"\n[other]\nkey = 1\n"
	out := string(stripRedundantTableHeaders([]byte(input), "model_providers", "mcp_servers"))
	if strings.Contains(out, "[model_providers]") || strings.Contains(out, "\n[mcp_servers]\n") {
		t.Fatalf("冗余父表头未剔除: %q", out)
	}
	if !strings.Contains(out, "[mcp_servers.portico]") || !strings.Contains(out, "[other]") {
		t.Fatalf("正常表头应保留: %q", out)
	}
}
