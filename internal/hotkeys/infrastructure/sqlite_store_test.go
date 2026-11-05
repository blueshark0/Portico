package infrastructure

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestSQLiteStore_MigrateLegacyHotkeys(t *testing.T) {
	tempDir := t.TempDir()
	homeDir := filepath.Join(tempDir, "home")
	configDir := filepath.Join(tempDir, "config")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_CONFIG_HOME", configDir)

	legacyDir := filepath.Join(configDir, "SuiNest")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("mkdir legacy dir: %v", err)
	}
	legacyPath := filepath.Join(legacyDir, "suidemo.db")
	legacyDB, err := sql.Open("sqlite", legacyPath)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	t.Cleanup(func() {
		_ = legacyDB.Close()
	})

	if _, err := legacyDB.Exec(`
		CREATE TABLE hotkeys (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			keycode INTEGER NOT NULL,
			modifiers INTEGER NOT NULL,
			description TEXT,
			target TEXT
		);
		INSERT INTO hotkeys (id, keycode, modifiers, description, target) VALUES
			(1, 34, 768, 'legacy-open', 'main'),
			(2, 46, 512, 'legacy-settings', 'settings');
	`); err != nil {
		t.Fatalf("seed legacy db: %v", err)
	}

	store, err := NewSQLiteStore()
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	hotkeys, err := store.List()
	if err != nil {
		t.Fatalf("list hotkeys: %v", err)
	}
	if len(hotkeys) != 2 {
		t.Fatalf("expected 2 migrated hotkeys, got %d", len(hotkeys))
	}
	if hotkeys[0].ID != 1 || hotkeys[0].KeyCode != 34 || hotkeys[0].Modifiers != 768 {
		t.Fatalf("unexpected first hotkey: %+v", hotkeys[0])
	}
	if hotkeys[1].ID != 2 || hotkeys[1].KeyCode != 46 || hotkeys[1].Modifiers != 512 {
		t.Fatalf("unexpected second hotkey: %+v", hotkeys[1])
	}

	newDBPath := filepath.Join(homeDir, ".portico", hotkeysDBFile)
	if _, err := os.Stat(newDBPath); err != nil {
		t.Fatalf("expected new hotkeys db at %s: %v", newDBPath, err)
	}
}
