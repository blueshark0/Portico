package infrastructure

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeminiProxyTogglePreservesEnvLinesAndUserChanges(t *testing.T) {
	m := newGeminiManagerForTest(t)
	path := geminiSettingsPathForTest(t)
	original := []byte("# heading\nGEMINI_API_KEY=\"original\"\nOTHER=keep\nexport GOOGLE_GEMINI_BASE_URL='http://old' # url comment\nTAIL=1\n")
	writeTestConfig(t, path, original)
	if err := m.EnableProxy(); err != nil {
		t.Fatal(err)
	}
	enabled := readTestConfig(t, path)
	expectedEnabled := strings.Replace(string(original), `"original"`, "portico", 1)
	expectedEnabled = strings.Replace(expectedEnabled, `'http://old'`, "http://127.0.0.1:18100/gemini", 1)
	if string(enabled) != expectedEnabled {
		t.Fatalf("Enable 只能修改值区间: got %q want %q", enabled, expectedEnabled)
	}

	changed := strings.Replace(string(enabled), "OTHER=keep", "OTHER=changed", 1)
	changed += "ADDED_DURING_ENABLE=yes\n"
	writeTestConfig(t, path, []byte(changed))
	if err := m.DisableProxy(); err != nil {
		t.Fatal(err)
	}
	expectedRestored := strings.Replace(changed, "GEMINI_API_KEY=portico", `GEMINI_API_KEY="original"`, 1)
	expectedRestored = strings.Replace(expectedRestored, "export GOOGLE_GEMINI_BASE_URL=http://127.0.0.1:18100/gemini", "export GOOGLE_GEMINI_BASE_URL='http://old'", 1)
	if got := readTestConfig(t, path); string(got) != expectedRestored {
		t.Fatalf("Disable 应恢复原始行并保留其他当前行: got %q want %q", got, expectedRestored)
	}
}

func TestGeminiProxyToggleRestoresDuplicateAssignments(t *testing.T) {
	m := newGeminiManagerForTest(t)
	path := geminiSettingsPathForTest(t)
	original := []byte("GEMINI_API_KEY=first\nOTHER=1\nexport GEMINI_API_KEY=\"second\" # keep\n")
	writeTestConfig(t, path, original)
	if err := m.EnableProxy(); err != nil {
		t.Fatal(err)
	}
	if err := m.DisableProxy(); err != nil {
		t.Fatal(err)
	}
	if got := readTestConfig(t, path); string(got) != string(original) {
		t.Fatalf("重复 key 应逐行恢复: got %q want %q", got, original)
	}
}

func TestGeminiProxyToggleMissingFieldsAndFile(t *testing.T) {
	t.Run("existing file", func(t *testing.T) {
		m := newGeminiManagerForTest(t)
		path := geminiSettingsPathForTest(t)
		original := []byte("# keep\nA=1\n")
		writeTestConfig(t, path, original)
		if err := m.EnableProxy(); err != nil {
			t.Fatal(err)
		}
		if err := m.DisableProxy(); err != nil {
			t.Fatal(err)
		}
		if got := readTestConfig(t, path); string(got) != string(original) {
			t.Fatalf("新增 key 删除后应恢复原行: got %q want %q", got, original)
		}
	})
	t.Run("created file", func(t *testing.T) {
		m := newGeminiManagerForTest(t)
		path := geminiSettingsPathForTest(t)
		if err := m.EnableProxy(); err != nil {
			t.Fatal(err)
		}
		if err := m.DisableProxy(); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("没有原始文件和其他内容时应移除新文件: %v", err)
		}
	})
}

func TestGeminiLegacyDisableLeavesCurrentFileUntouched(t *testing.T) {
	m := newGeminiManagerForTest(t)
	path := geminiSettingsPathForTest(t)
	current := []byte("# current is baseline\nGEMINI_API_KEY=portico\nGOOGLE_GEMINI_BASE_URL=http://127.0.0.1:18100/gemini")
	writeTestConfig(t, path, current)
	if err := m.DisableProxy(); err != nil {
		t.Fatal(err)
	}
	if got := readTestConfig(t, path); string(got) != string(current) {
		t.Fatalf("旧版无快照关闭不得修改文件: got %q", got)
	}
	if err := m.EnableProxy(); err != nil {
		t.Fatal(err)
	}
	if got := readTestConfig(t, path); string(got) != string(current) {
		t.Fatalf("已正确的值不应重写: got %q", got)
	}
}

func TestGeminiEnableRejectsMalformedManagedLine(t *testing.T) {
	m := newGeminiManagerForTest(t)
	path := geminiSettingsPathForTest(t)
	content := []byte("GEMINI_API_KEY=\"unterminated\nKEEP=1\n")
	writeTestConfig(t, path, content)
	if err := m.EnableProxy(); err == nil {
		t.Fatal("损坏的受管赋值应报错")
	}
	if got := readTestConfig(t, path); string(got) != string(content) {
		t.Fatalf("失败不得改写文件: got %q", got)
	}
}

func newGeminiManagerForTest(t *testing.T) *GeminiManager {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	return NewGeminiManager("127.0.0.1:18100")
}

func geminiSettingsPathForTest(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(home, ".gemini", ".env")
}
