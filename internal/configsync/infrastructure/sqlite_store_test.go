package infrastructure

import (
	"context"
	"path/filepath"
	"testing"

	configsyncdomain "portico/internal/configsync/domain"
	"portico/internal/shared/storage"
)

func TestSQLiteStoreEnsureSchema_MigratesLegacySyncSettingsColumns(t *testing.T) {
	dbName := "configsync_test_" + t.Name()
	dbPath := filepath.Join(t.TempDir(), "app.db")

	if err := storage.OpenConnection(storage.ConnectionConfig{
		Name:   dbName,
		Driver: "sqlite",
		DSN:    dbPath,
	}); err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = storage.CloseConnection(dbName) })

	db, err := storage.DB(dbName)
	if err != nil {
		t.Fatalf("get test database: %v", err)
	}

	if _, err := db.Exec(`
		CREATE TABLE sync_settings (
			id INTEGER PRIMARY KEY CHECK (id = 1)
		);
		INSERT INTO sync_settings (id) VALUES (1);
	`); err != nil {
		t.Fatalf("seed legacy sync_settings: %v", err)
	}

	store := NewSQLiteStoreWithDBName(dbName)
	if err := store.EnsureSchema(); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}

	for _, column := range []string{"endpoint", "username", "app_password", "remote_path", "sync_passphrase", "updated_at"} {
		var count int
		if err := db.QueryRow(
			"SELECT COUNT(*) FROM pragma_table_info('sync_settings') WHERE name = ?",
			column,
		).Scan(&count); err != nil {
			t.Fatalf("query column %s: %v", column, err)
		}
		if count != 1 {
			t.Fatalf("expected column %s to exist once, got %d", column, count)
		}
	}

	settings, err := store.GetSettings(context.Background())
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if settings.RemotePath != configsyncdomain.DefaultRemotePath {
		t.Fatalf("expected default remote path %q, got %q", configsyncdomain.DefaultRemotePath, settings.RemotePath)
	}

	saved, err := store.SaveSettings(context.Background(), configsyncdomain.SyncSettings{
		Endpoint:       "https://dav.example.com",
		Username:       "user",
		AppPassword:    "password",
		RemotePath:     "/Portico",
		SyncPassphrase: "secret",
	})
	if err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	if saved.Endpoint != "https://dav.example.com" {
		t.Fatalf("expected saved endpoint, got %q", saved.Endpoint)
	}
}
