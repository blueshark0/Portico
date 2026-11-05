package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateLegacyAppDataDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	legacyDir := filepath.Join(home, legacyAppDirName)
	if err := os.MkdirAll(filepath.Join(legacyDir, "platformproxy"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "app.db"), []byte("profiles"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "platformproxy", "claude.json"), []byte("state"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := MigrateLegacyAppDataDir(); err != nil {
		t.Fatalf("migrate legacy data: %v", err)
	}
	currentDir := filepath.Join(home, appDirName)
	content, err := os.ReadFile(filepath.Join(currentDir, "app.db"))
	if err != nil || string(content) != "profiles" {
		t.Fatalf("migrated app.db = %q, %v", content, err)
	}
	if _, err := os.Stat(filepath.Join(currentDir, "platformproxy", "claude.json")); err != nil {
		t.Fatalf("migrated proxy state: %v", err)
	}
	if _, err := os.Stat(legacyDir); !os.IsNotExist(err) {
		t.Fatalf("legacy directory still exists: %v", err)
	}
	if err := MigrateLegacyAppDataDir(); err != nil {
		t.Fatalf("second migration should be a no-op: %v", err)
	}
}

func TestMigrateLegacyAppDataDirDoesNotOverwriteCurrentData(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	currentDir := filepath.Join(home, appDirName)
	legacyDir := filepath.Join(home, legacyAppDirName)
	for _, dir := range []string{currentDir, legacyDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(currentDir, "app.db"), []byte("current"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "app.db"), []byte("legacy"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := MigrateLegacyAppDataDir(); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(currentDir, "app.db"))
	if err != nil || string(content) != "current" {
		t.Fatalf("current data was changed: %q, %v", content, err)
	}
	if _, err := os.Stat(filepath.Join(legacyDir, "app.db")); err != nil {
		t.Fatalf("legacy data should remain untouched when current data exists: %v", err)
	}
}
