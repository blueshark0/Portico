package infrastructure

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexProxyTogglePreservesStandardTablesAndAuthFields(t *testing.T) {
	m := newCodexManager(t)
	configPath := codexConfigPath(t)
	authPath := codexAuthPathForTest(t)
	originalConfig := []byte(`# top comment
model = "gpt-5"
model_provider = "openai"

[model_providers.portico]
name = "old-name"
base_url = "http://old"
wire_api = "chat"
requires_openai_auth = true
custom = "keep"

[model_providers.other]
name = "other"
base_url = "http://other"

[mcp_servers.user-own]
url = "http://mcp"
`)
	originalAuth := []byte(`{"tokens":{"access":"a","refresh":"r"},"OPENAI_API_KEY":"original","other":1}`)
	writeTestConfig(t, configPath, originalConfig)
	writeTestConfig(t, authPath, originalAuth)

	if err := m.EnableProxy(); err != nil {
		t.Fatal(err)
	}
	enabledConfig := readTestConfig(t, configPath)
	expectedEnabled := string(originalConfig)
	for _, replacement := range [][2]string{
		{`model_provider = "openai"`, `model_provider = "portico"`},
		{`name = "old-name"`, `name = "portico"`},
		{`base_url = "http://old"`, `base_url = "http://127.0.0.1:18100"`},
		{`wire_api = "chat"`, `wire_api = "responses"`},
		{`requires_openai_auth = true`, `requires_openai_auth = false`},
	} {
		expectedEnabled = strings.Replace(expectedEnabled, replacement[0], replacement[1], 1)
	}
	if string(enabledConfig) != expectedEnabled {
		t.Fatalf("普通表只能替换五个 literal:\n--- got ---\n%s\n--- want ---\n%s", enabledConfig, expectedEnabled)
	}
	expectedAuth := strings.Replace(string(originalAuth), `"original"`, `"portico"`, 1)
	if got := readTestConfig(t, authPath); string(got) != expectedAuth {
		t.Fatalf("auth.json 只能修改 OPENAI_API_KEY: got %s want %s", got, expectedAuth)
	}

	changedConfig := strings.Replace(string(enabledConfig), `custom = "keep"`, `custom = "changed"`, 1)
	changedConfig = strings.Replace(changedConfig, `model = "gpt-5"`, `model = "gpt-6"`, 1)
	writeTestConfig(t, configPath, []byte(changedConfig))
	changedAuth := strings.Replace(expectedAuth, `"access":"a"`, `"access":"b"`, 1)
	writeTestConfig(t, authPath, []byte(changedAuth))
	if err := m.DisableProxy(); err != nil {
		t.Fatal(err)
	}

	expectedRestored := changedConfig
	for _, replacement := range [][2]string{
		{`model_provider = "portico"`, `model_provider = "openai"`},
		{`name = "portico"`, `name = "old-name"`},
		{`base_url = "http://127.0.0.1:18100"`, `base_url = "http://old"`},
		{`wire_api = "responses"`, `wire_api = "chat"`},
		{`requires_openai_auth = false`, `requires_openai_auth = true`},
	} {
		expectedRestored = strings.Replace(expectedRestored, replacement[0], replacement[1], 1)
	}
	if got := readTestConfig(t, configPath); string(got) != expectedRestored {
		t.Fatalf("Disable 应恢复原 literal 并保留其他当前修改:\n--- got ---\n%s\n--- want ---\n%s", got, expectedRestored)
	}
	expectedAuthRestored := strings.Replace(changedAuth, `"portico"`, `"original"`, 1)
	if got := readTestConfig(t, authPath); string(got) != expectedAuthRestored {
		t.Fatalf("auth 原 key 应恢复且 tokens 当前修改应保留: got %s want %s", got, expectedAuthRestored)
	}
}

func TestCodexProxyToggleInlineTableRoundTrip(t *testing.T) {
	m := newCodexManager(t)
	configPath := codexConfigPath(t)
	authPath := codexAuthPathForTest(t)
	original := []byte("model_provider = \"openai\"\nmodel_providers = { other = { name = \"other\" }, portico = { name = \"old\", custom = \"keep\", base_url = \"http://old\", wire_api = \"chat\", requires_openai_auth = true } }\n")
	writeTestConfig(t, configPath, original)
	writeTestConfig(t, authPath, []byte(`{"tokens":{"access":"a"}}`))
	if err := m.EnableProxy(); err != nil {
		t.Fatal(err)
	}
	enabled := readTestConfig(t, configPath)
	if strings.Contains(string(enabled), "[model_providers") {
		t.Fatalf("inline table 不得展开成标准表:\n%s", enabled)
	}
	if strings.Index(string(enabled), `other =`) > strings.Index(string(enabled), `portico =`) {
		t.Fatalf("已有 inline 条目顺序不得改变:\n%s", enabled)
	}
	if err := m.DisableProxy(); err != nil {
		t.Fatal(err)
	}
	if got := readTestConfig(t, configPath); string(got) != string(original) {
		t.Fatalf("inline 配置应精确恢复: got %q want %q", got, original)
	}
	if got := readTestConfig(t, authPath); string(got) != `{"tokens":{"access":"a"}}` {
		t.Fatalf("auth 新增 key 应删除且其他字段不动: got %s", got)
	}
}

func TestCodexProxyToggleDottedKeysRoundTrip(t *testing.T) {
	m := newCodexManager(t)
	configPath := codexConfigPath(t)
	original := []byte("model_provider = \"openai\"\nmodel_providers.portico.name = \"old\"\nmodel_providers.portico.base_url = \"http://old\"\nmodel_providers.portico.wire_api = \"chat\"\nmodel_providers.portico.requires_openai_auth = true\nmodel_providers.portico.custom = \"keep\"\n")
	writeTestConfig(t, configPath, original)
	if err := m.EnableProxy(); err != nil {
		t.Fatal(err)
	}
	enabled := readTestConfig(t, configPath)
	if strings.Contains(string(enabled), "[model_providers") {
		t.Fatalf("点分键不得改写成表:\n%s", enabled)
	}
	if err := m.DisableProxy(); err != nil {
		t.Fatal(err)
	}
	if got := readTestConfig(t, configPath); string(got) != string(original) {
		t.Fatalf("点分键配置应精确恢复: got %q want %q", got, original)
	}
}

func TestCodexProxyToggleRemovesNewProviderAndAuthFile(t *testing.T) {
	m := newCodexManager(t)
	configPath := codexConfigPath(t)
	authPath := codexAuthPathForTest(t)
	original := []byte("# keep\nmodel = \"gpt-5\"\n")
	writeTestConfig(t, configPath, original)
	if err := m.EnableProxy(); err != nil {
		t.Fatal(err)
	}
	if err := m.DisableProxy(); err != nil {
		t.Fatal(err)
	}
	if got := readTestConfig(t, configPath); string(got) != string(original) {
		t.Fatalf("新增 provider 和字段应移除，已有内容不变: got %q want %q", got, original)
	}
	if _, err := os.Stat(authPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Enable 新建的 auth.json 应在无其他内容时移除: %v", err)
	}
}

func TestCodexLegacyDisableLeavesBothFilesAndBackupsUntouched(t *testing.T) {
	m := newCodexManager(t)
	configPath := codexConfigPath(t)
	authPath := codexAuthPathForTest(t)
	config := []byte("model_provider = \"portico\"\n[model_providers.portico]\nname = \"portico\"\nbase_url = \"http://127.0.0.1:18100\"\nwire_api = \"responses\"\nrequires_openai_auth = false\n")
	auth := []byte(`{"OPENAI_API_KEY":"portico","keep":1}`)
	writeTestConfig(t, configPath, config)
	writeTestConfig(t, authPath, auth)
	configBackup := filepath.Join(filepath.Dir(configPath), "cc-studio.back.config.toml")
	authBackup := filepath.Join(filepath.Dir(configPath), "cc-studio.back.auth.json")
	writeTestConfig(t, configBackup, []byte("old config backup"))
	writeTestConfig(t, authBackup, []byte("old auth backup"))

	if err := m.DisableProxy(); err != nil {
		t.Fatal(err)
	}
	if got := readTestConfig(t, configPath); string(got) != string(config) {
		t.Fatalf("旧版关闭不得修改 config.toml: %s", got)
	}
	if got := readTestConfig(t, authPath); string(got) != string(auth) {
		t.Fatalf("旧版关闭不得修改 auth.json: %s", got)
	}
	if got := readTestConfig(t, configBackup); string(got) != "old config backup" {
		t.Fatalf("旧 config 备份应不动: %s", got)
	}
	if got := readTestConfig(t, authBackup); string(got) != "old auth backup" {
		t.Fatalf("旧 auth 备份应不动: %s", got)
	}
	if err := m.EnableProxy(); err != nil {
		t.Fatal(err)
	}
	if got := readTestConfig(t, configPath); string(got) != string(config) {
		t.Fatalf("已正确的 TOML 值不应重写: %s", got)
	}
}

func TestCodexEnablePreflightsBothFiles(t *testing.T) {
	t.Run("invalid auth leaves config unchanged", func(t *testing.T) {
		m := newCodexManager(t)
		configPath := codexConfigPath(t)
		authPath := codexAuthPathForTest(t)
		config := []byte("model = \"keep\"\n")
		auth := []byte("{ invalid json")
		writeTestConfig(t, configPath, config)
		writeTestConfig(t, authPath, auth)
		if err := m.EnableProxy(); err == nil {
			t.Fatal("损坏 auth.json 应报错")
		}
		if got := readTestConfig(t, configPath); string(got) != string(config) {
			t.Fatalf("预检失败不得先写 config.toml: %s", got)
		}
		if got := readTestConfig(t, authPath); string(got) != string(auth) {
			t.Fatalf("损坏 auth 不得覆盖: %s", got)
		}
	})
	t.Run("invalid config leaves auth unchanged", func(t *testing.T) {
		m := newCodexManager(t)
		configPath := codexConfigPath(t)
		authPath := codexAuthPathForTest(t)
		config := []byte("[ invalid toml")
		auth := []byte(`{"tokens":{"access":"a"}}`)
		writeTestConfig(t, configPath, config)
		writeTestConfig(t, authPath, auth)
		if err := m.EnableProxy(); err == nil {
			t.Fatal("损坏 config.toml 应报错")
		}
		if got := readTestConfig(t, authPath); string(got) != string(auth) {
			t.Fatalf("预检失败不得先写 auth.json: %s", got)
		}
	})
}

func TestCodexProxyTogglePrunesOnlyNewInlineProvider(t *testing.T) {
	m := newCodexManager(t)
	configPath := codexConfigPath(t)
	original := []byte("model_provider = \"openai\"\nmodel_providers = { other = { name = \"other\" } }\n")
	writeTestConfig(t, configPath, original)
	if err := m.EnableProxy(); err != nil {
		t.Fatal(err)
	}
	enabled := readTestConfig(t, configPath)
	if !strings.Contains(string(enabled), `portico = {`) || strings.Contains(string(enabled), "[model_providers") {
		t.Fatalf("应在原 inline parent 内创建 provider: %s", enabled)
	}
	if err := m.DisableProxy(); err != nil {
		t.Fatal(err)
	}
	if got := readTestConfig(t, configPath); string(got) != string(original) {
		t.Fatalf("只应移除新建 inline provider: got %q want %q", got, original)
	}
}

func TestCodexDisableKeepsUserContentAddedToNewProvider(t *testing.T) {
	m := newCodexManager(t)
	configPath := codexConfigPath(t)
	writeTestConfig(t, configPath, []byte("# keep\n"))
	if err := m.EnableProxy(); err != nil {
		t.Fatal(err)
	}
	withUserContent := append(readTestConfig(t, configPath), []byte("custom = \"user\"\n")...)
	writeTestConfig(t, configPath, withUserContent)
	if err := m.DisableProxy(); err != nil {
		t.Fatal(err)
	}
	restored := string(readTestConfig(t, configPath))
	if !strings.Contains(restored, "[model_providers.portico]") || !strings.Contains(restored, `custom = "user"`) {
		t.Fatalf("用户加入新 provider 的内容必须保留: %s", restored)
	}
	for _, managed := range []string{"model_provider =", "base_url =", "wire_api =", "requires_openai_auth ="} {
		if strings.Contains(restored, managed) {
			t.Fatalf("受管字段仍应删除 %s: %s", managed, restored)
		}
	}
}

func TestCodexProxyDisableRemovesBothFilesCreatedByEnable(t *testing.T) {
	m := newCodexManager(t)
	if err := m.EnableProxy(); err != nil {
		t.Fatal(err)
	}
	if err := m.DisableProxy(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{codexConfigPath(t), codexAuthPathForTest(t)} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("新建文件 %s 应在空内容恢复后移除: %v", path, err)
		}
	}
}

func codexAuthPathForTest(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(home, ".codex", "auth.json")
}
