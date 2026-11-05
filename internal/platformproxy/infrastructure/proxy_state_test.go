package infrastructure

import (
	"os"
	"strings"
	"testing"
)

func TestProxyStateStoreInactiveClearsOriginalValues(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	store := newProxyStateStore("claude")
	if err := store.save(proxyState{Active: true, Files: map[string]proxyFileSnapshot{
		"settings": {Fields: map[string][]string{"secret": {"original-secret"}}},
	}}); err != nil {
		t.Fatal(err)
	}
	if state, exists, err := store.load(); err != nil || !exists || !state.Active {
		t.Fatalf("active 快照应可跨重启读取: %#v, %v, %v", state, exists, err)
	}
	if err := store.save(proxyState{Active: false}); err != nil {
		t.Fatal(err)
	}
	state, exists, err := store.load()
	if err != nil || !exists || state.Active || len(state.Files) != 0 {
		t.Fatalf("关闭状态应清除原值: %#v, %v, %v", state, exists, err)
	}
	path, err := store.path()
	if err != nil {
		t.Fatal(err)
	}
	data := readTestConfig(t, path)
	if strings.Contains(string(data), "original-secret") {
		t.Fatalf("关闭后的状态文件不得残留敏感原值: %s", data)
	}
}

func TestProxyStateStoreRejectsUnknownVersion(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	store := newProxyStateStore("claude")
	path, err := store.path()
	if err != nil {
		t.Fatal(err)
	}
	writeTestConfig(t, path, []byte(`{"version":99,"active":false}`))
	if _, _, err := store.load(); err == nil {
		t.Fatal("未知快照版本必须报错")
	}
}

func TestClaudeEnableDoesNotOverwriteCorruptSnapshot(t *testing.T) {
	m := newClaudeManager(t)
	statePath, err := m.stateStore.path()
	if err != nil {
		t.Fatal(err)
	}
	corrupt := []byte("{ corrupt state")
	writeTestConfig(t, statePath, corrupt)
	configPath := claudeSettingsPathForTest(t)
	config := []byte(`{"keep":1}`)
	writeTestConfig(t, configPath, config)
	if err := m.EnableProxy(); err == nil {
		t.Fatal("损坏快照必须报错，不能覆盖恢复依据")
	}
	if got := readTestConfig(t, statePath); string(got) != string(corrupt) {
		t.Fatalf("损坏快照不得被新周期覆盖: %s", got)
	}
	if got := readTestConfig(t, configPath); string(got) != string(config) {
		t.Fatalf("快照错误时不得修改 CLI 文件: %s", got)
	}
	if _, err := os.Stat(statePath); err != nil {
		t.Fatal(err)
	}
}
