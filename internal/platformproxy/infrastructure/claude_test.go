package infrastructure

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func TestClaudeProxyToggleRestoresOnlyManagedFields(t *testing.T) {
	m := newClaudeManager(t)
	path := claudeSettingsPathForTest(t)
	original := []byte(`{
  "z": 1,
  "env": {
    "OTHER": "keep",
    "ANTHROPIC_AUTH_TOKEN": "old-token",
    "ANTHROPIC_BASE_URL": "http://old"
  },
  "after": {"nested": true}
}`)
	writeTestConfig(t, path, original)

	if err := m.EnableProxy(); err != nil {
		t.Fatal(err)
	}
	enabled := readTestConfig(t, path)
	expectedEnabled := strings.Replace(string(original), `"old-token"`, `"portico"`, 1)
	expectedEnabled = strings.Replace(expectedEnabled, `"http://old"`, `"http://127.0.0.1:18100"`, 1)
	if string(enabled) != expectedEnabled {
		t.Fatalf("Enable 只能替换两个值:\n--- got ---\n%s\n--- want ---\n%s", enabled, expectedEnabled)
	}

	// 模拟接入期间用户修改非受管字段，关闭时不得被快照覆盖。
	changed := strings.Replace(string(enabled), `"keep"`, `"changed-by-user"`, 1)
	changed = strings.Replace(changed, `"z": 1`, `"z": 2`, 1)
	writeTestConfig(t, path, []byte(changed))
	if err := m.DisableProxy(); err != nil {
		t.Fatal(err)
	}
	restored := readTestConfig(t, path)
	expectedRestored := strings.Replace(changed, `"portico"`, `"old-token"`, 1)
	expectedRestored = strings.Replace(expectedRestored, `"http://127.0.0.1:18100"`, `"http://old"`, 1)
	if string(restored) != expectedRestored {
		t.Fatalf("Disable 应恢复原值并保留其他当前修改:\n--- got ---\n%s\n--- want ---\n%s", restored, expectedRestored)
	}
}

func TestClaudeProxyToggleRemovesOnlyNewFields(t *testing.T) {
	m := newClaudeManager(t)
	path := claudeSettingsPathForTest(t)
	original := []byte(`{"before":1,"env":{"keep":2},"after":3}`)
	writeTestConfig(t, path, original)
	if err := m.EnableProxy(); err != nil {
		t.Fatal(err)
	}
	if err := m.DisableProxy(); err != nil {
		t.Fatal(err)
	}
	if got := readTestConfig(t, path); string(got) != string(original) {
		t.Fatalf("新增字段移除后应恢复原结构和顺序: got %s want %s", got, original)
	}
}

func TestClaudeProxyDisableRemovesFileCreatedByEnable(t *testing.T) {
	m := newClaudeManager(t)
	path := claudeSettingsPathForTest(t)
	if err := m.EnableProxy(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	if err := m.DisableProxy(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Enable 新建且没有其他内容的文件应在 Disable 后移除: %v", err)
	}
}

func TestClaudeLegacyDisableDoesNotTouchCurrentFileOrBackup(t *testing.T) {
	m := newClaudeManager(t)
	path := claudeSettingsPathForTest(t)
	current := []byte("{\n  \"env\": {\"ANTHROPIC_AUTH_TOKEN\": \"portico\", \"ANTHROPIC_BASE_URL\": \"http://127.0.0.1:18100\"},\n  \"keep\": 1\n}")
	writeTestConfig(t, path, current)
	backupPath := filepath.Join(filepath.Dir(path), "cc-studio.back.settings.json")
	backup := []byte("legacy backup must not be read")
	writeTestConfig(t, backupPath, backup)

	if err := m.DisableProxy(); err != nil {
		t.Fatal(err)
	}
	if got := readTestConfig(t, path); string(got) != string(current) {
		t.Fatalf("旧版首次关闭不得修改 CLI 文件: got %s", got)
	}
	if got := readTestConfig(t, backupPath); string(got) != string(backup) {
		t.Fatalf("旧备份必须保持不动: got %s", got)
	}
	if status, err := m.ProxyStatus(); err != nil || status.Enabled {
		t.Fatalf("旧版关闭状态应可跨刷新保持: %#v, %v", status, err)
	}

	if err := m.EnableProxy(); err != nil {
		t.Fatal(err)
	}
	if got := readTestConfig(t, path); string(got) != string(current) {
		t.Fatalf("值已正确时下一次开启不应重写文件: got %s", got)
	}
	if err := m.DisableProxy(); err != nil {
		t.Fatal(err)
	}
	if got := readTestConfig(t, path); string(got) != string(current) {
		t.Fatalf("当前配置作为原值时后续关闭也不应改写: got %s", got)
	}
}

func TestClaudeRepeatedEnableKeepsInitialSnapshotAcrossManagerRestart(t *testing.T) {
	m := newClaudeManager(t)
	path := claudeSettingsPathForTest(t)
	original := []byte(`{"env":{"ANTHROPIC_AUTH_TOKEN":"original","ANTHROPIC_BASE_URL":"http://original"},"keep":1}`)
	writeTestConfig(t, path, original)
	if err := m.EnableProxy(); err != nil {
		t.Fatal(err)
	}
	// 活跃周期内即使受管字段被改过，再次 Enable 也不得把它采成新原值。
	writeTestConfig(t, path, []byte(`{"env":{"ANTHROPIC_AUTH_TOKEN":"changed","ANTHROPIC_BASE_URL":"http://changed"},"keep":2}`))
	if err := m.EnableProxy(); err != nil {
		t.Fatal(err)
	}
	restarted := NewClaudeManager("127.0.0.1:18100")
	if err := restarted.DisableProxy(); err != nil {
		t.Fatal(err)
	}
	expected := `{"env":{"ANTHROPIC_AUTH_TOKEN":"original","ANTHROPIC_BASE_URL":"http://original"},"keep":2}`
	if got := readTestConfig(t, path); string(got) != expected {
		t.Fatalf("重启后仍应使用最初快照: got %s want %s", got, expected)
	}
}

func TestClaudeEnableRejectsInvalidJSONWithoutOverwriting(t *testing.T) {
	m := newClaudeManager(t)
	path := claudeSettingsPathForTest(t)
	corrupt := []byte("{ not json")
	writeTestConfig(t, path, corrupt)
	if err := m.EnableProxy(); err == nil {
		t.Fatal("损坏 JSON 应报错")
	}
	if got := readTestConfig(t, path); string(got) != string(corrupt) {
		t.Fatalf("损坏文件不得被覆盖: got %s", got)
	}
}

func TestClaudeEnableRejectsNonObjectEnv(t *testing.T) {
	m := newClaudeManager(t)
	path := claudeSettingsPathForTest(t)
	content := []byte(`{"env":"keep-this-string"}`)
	writeTestConfig(t, path, content)
	if err := m.EnableProxy(); err == nil {
		t.Fatal("env 不是对象时应报错")
	}
	if got := readTestConfig(t, path); string(got) != string(content) {
		t.Fatalf("不得通过替换整个 env 修复类型: got %s", got)
	}
}

func TestClaudeDisableKeepsUserFieldsAddedToNewEnv(t *testing.T) {
	m := newClaudeManager(t)
	path := claudeSettingsPathForTest(t)
	writeTestConfig(t, path, []byte(`{}`))
	if err := m.EnableProxy(); err != nil {
		t.Fatal(err)
	}
	updated, err := sjson.SetBytes(readTestConfig(t, path), "env.user", "keep")
	if err != nil {
		t.Fatal(err)
	}
	writeTestConfig(t, path, updated)
	if err := m.DisableProxy(); err != nil {
		t.Fatal(err)
	}
	restored := readTestConfig(t, path)
	if got := gjson.GetBytes(restored, "env.user").String(); got != "keep" {
		t.Fatalf("用户加入新 env 的字段必须保留: %s", restored)
	}
	if gjson.GetBytes(restored, "env.ANTHROPIC_AUTH_TOKEN").Exists() || gjson.GetBytes(restored, "env.ANTHROPIC_BASE_URL").Exists() {
		t.Fatalf("受管字段仍应删除: %s", restored)
	}
}

func claudeSettingsPathForTest(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(home, ".claude", "settings.json")
}

func writeTestConfig(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readTestConfig(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
