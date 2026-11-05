package infrastructure

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func newCodexManager(t *testing.T) *CodexManager {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	return NewCodexManager("127.0.0.1:18100")
}

func codexConfigPath(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(home, ".codex", "config.toml")
}

func readCodexConfig(t *testing.T) string {
	t.Helper()
	content, err := os.ReadFile(codexConfigPath(t))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return string(content)
}

func TestCodexInjectIntoMissingFile(t *testing.T) {
	m := newCodexManager(t)
	if ok, err := m.McpStatus(); err != nil || ok {
		t.Fatalf("未注入时 McpStatus: %v, %v", ok, err)
	}
	if err := m.InjectMcp(); err != nil {
		t.Fatalf("InjectMcp: %v", err)
	}
	content := readCodexConfig(t)
	if !strings.Contains(content, "[mcp_servers.portico]") {
		t.Fatalf("缺少 mcp_servers 条目: %q", content)
	}
	if !strings.Contains(content, "http://127.0.0.1:18100/mcp") {
		t.Fatalf("缺少 url: %q", content)
	}
	if !strings.Contains(content, "Bearer portico") {
		t.Fatalf("缺少鉴权头: %q", content)
	}
	if strings.Contains(content, "\n[mcp_servers]\n") || strings.HasPrefix(content, "[mcp_servers]\n") {
		t.Fatalf("不应出现裸 [mcp_servers] 父表头: %q", content)
	}
	if ok, _ := m.McpStatus(); !ok {
		t.Fatal("注入后 McpStatus 应为 true")
	}
}

func TestCodexInjectPreservesExistingContent(t *testing.T) {
	m := newCodexManager(t)
	seed := "model = \"gpt-5\"\n\n[mcp_servers.user-own]\ncommand = \"x\"\n"
	dir := filepath.Dir(codexConfigPath(t))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(codexConfigPath(t), []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := m.InjectMcp(); err != nil {
		t.Fatal(err)
	}
	// 结构性断言（go-toml 用单引号字面量输出，不做字符串样式假设）
	var raw map[string]any
	content, err := os.ReadFile(codexConfigPath(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := toml.Unmarshal(content, &raw); err != nil {
		t.Fatalf("注入后的文件应为合法 TOML: %v", err)
	}
	if raw["model"] != "gpt-5" {
		t.Fatalf("既有顶层配置丢失: %v", raw)
	}
	servers, _ := raw["mcp_servers"].(map[string]any)
	if _, ok := servers["user-own"]; !ok {
		t.Fatalf("用户既有 MCP 条目丢失: %v", servers)
	}
	entry, _ := servers["portico"].(map[string]any)
	if entry["url"] != "http://127.0.0.1:18100/mcp" {
		t.Fatalf("网关条目失真: %v", entry)
	}
}

func TestCodexInjectReplacesLegacyEntry(t *testing.T) {
	m := newCodexManager(t)
	seed := "[mcp_servers.code-switch]\nurl = \"http://127.0.0.1:18100/mcp\"\n\n[mcp_servers.user-own]\ncommand = \"x\"\n"
	if err := os.MkdirAll(filepath.Dir(codexConfigPath(t)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(codexConfigPath(t), []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := m.InjectMcp(); err != nil {
		t.Fatal(err)
	}
	raw, err := m.readRawConfig()
	if err != nil {
		t.Fatal(err)
	}
	servers := raw["mcp_servers"].(map[string]any)
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

func TestCodexInjectIdempotentSkipsWrite(t *testing.T) {
	m := newCodexManager(t)
	if err := m.InjectMcp(); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(codexConfigPath(t))
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if err := m.InjectMcp(); err != nil {
		t.Fatalf("内容一致时不应写盘: %v", err)
	}
}

func TestCodexRemove(t *testing.T) {
	m := newCodexManager(t)
	if err := m.InjectMcp(); err != nil {
		t.Fatal(err)
	}
	if err := m.RemoveMcp(); err != nil {
		t.Fatal(err)
	}
	content := readCodexConfig(t)
	if strings.Contains(content, "mcp_servers") {
		t.Fatalf("条目移除后不应残留: %q", content)
	}
	if ok, _ := m.McpStatus(); ok {
		t.Fatal("移除后 McpStatus 应为 false")
	}
	// 幂等：再次移除、文件不存在时移除都是 no-op
	if err := m.RemoveMcp(); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(codexConfigPath(t))
	if err := m.RemoveMcp(); err != nil {
		t.Fatal(err)
	}
}

func TestCodexCorruptConfigRefusesOverwrite(t *testing.T) {
	m := newCodexManager(t)
	corrupt := []byte("[mcp_servers\nbroken")
	dir := filepath.Dir(codexConfigPath(t))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(codexConfigPath(t), corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := m.InjectMcp(); err == nil {
		t.Fatal("损坏 TOML 应报错")
	}
	content, _ := os.ReadFile(codexConfigPath(t))
	if string(content) != string(corrupt) {
		t.Fatal("损坏文件不应被改写")
	}
}

// 关键路径：Enable 代理 → 手动注入 MCP → Disable 代理。
// 代理关闭只恢复自己的字段，不得回滚后来注入的 MCP 条目。
func TestCodexDisableProxyKeepsMcpEntry(t *testing.T) {
	m := newCodexManager(t)
	if err := m.EnableProxy(); err != nil {
		t.Fatal(err)
	}
	if err := m.InjectMcp(); err != nil {
		t.Fatal(err)
	}
	if err := m.DisableProxy(); err != nil {
		t.Fatal(err)
	}
	if ok, err := m.McpStatus(); err != nil || !ok {
		t.Fatalf("Disable 代理后 MCP 条目应存续: %v, %v", ok, err)
	}
	raw, err := m.readRawConfig()
	if err != nil {
		t.Fatal(err)
	}
	servers, _ := raw["mcp_servers"].(map[string]any)
	if !reflect.DeepEqual(servers[mcpEntryName], m.mcpEntryPayload()) {
		t.Fatalf("Disable 不得改写 MCP 条目内容: %#v", servers[mcpEntryName])
	}
	// 模型代理字段已恢复为接入前的缺失状态，MCP 条目独立保留。
	status, err := m.ProxyStatus()
	if err != nil {
		t.Fatal(err)
	}
	if status.Enabled {
		t.Fatal("Disable 后代理应为关闭状态")
	}
}

// 反向场景：注入 → Enable → 用户移除 → Disable。
// 字段级恢复不得重新注入用户已经移除的 MCP 条目。
func TestCodexDisableProxyRespectsMcpRemoval(t *testing.T) {
	m := newCodexManager(t)
	if err := m.InjectMcp(); err != nil {
		t.Fatal(err)
	}
	if err := m.EnableProxy(); err != nil {
		t.Fatal(err)
	}
	if err := m.RemoveMcp(); err != nil {
		t.Fatal(err)
	}
	if err := m.DisableProxy(); err != nil {
		t.Fatal(err)
	}
	if ok, err := m.McpStatus(); err != nil || ok {
		t.Fatalf("用户移除后 Disable 不得恢复条目: %v, %v", ok, err)
	}
	// 代理也确实被禁用
	status, err := m.ProxyStatus()
	if err != nil {
		t.Fatal(err)
	}
	if status.Enabled {
		t.Fatal("Disable 后代理应为关闭状态")
	}
}
