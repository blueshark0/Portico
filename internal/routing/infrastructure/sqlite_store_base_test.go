package infrastructure

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

func TestSQLiteStoreEnsureSchema_MigratesLegacyAppPreferencesColumns(t *testing.T) {
	dbName := "routing_test_" + t.Name()
	dbPath := filepath.Join(t.TempDir(), "app.db")

	db := openRoutingTestDB(t, dbName, dbPath)

	if _, err := db.Exec(`
		CREATE TABLE app_preferences (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			show_heatmap INTEGER NOT NULL DEFAULT 1,
			show_home_title INTEGER NOT NULL DEFAULT 1,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		INSERT INTO app_preferences (id, show_heatmap, show_home_title)
		VALUES (1, 0, 1);
	`); err != nil {
		t.Fatalf("seed legacy app_preferences: %v", err)
	}

	store := NewSQLiteStoreWithDBName(dbName)
	if err := store.EnsureSchema(); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}

	for _, column := range []string{"proxy_enabled", "proxy_url"} {
		var count int
		if err := db.QueryRow(
			"SELECT COUNT(*) FROM pragma_table_info('app_preferences') WHERE name = ?",
			column,
		).Scan(&count); err != nil {
			t.Fatalf("query column %s: %v", column, err)
		}
		if count != 1 {
			t.Fatalf("expected column %s to exist once, got %d", column, count)
		}
	}

	prefs, err := store.GetAppPreferences(context.Background())
	if err != nil {
		t.Fatalf("GetAppPreferences: %v", err)
	}
	if prefs.ShowHeatmap {
		t.Fatal("expected existing show_heatmap value to be preserved")
	}
	if !prefs.ShowHomeTitle {
		t.Fatal("expected existing show_home_title value to be preserved")
	}
	if prefs.ProxyEnabled {
		t.Fatal("expected proxy_enabled default false after migration")
	}
	if prefs.ProxyURL != "" {
		t.Fatalf("expected empty proxy_url after migration, got %q", prefs.ProxyURL)
	}
}

func TestSQLiteStoreEnsureSchema_CreatesDefaultAppPreferencesRow(t *testing.T) {
	dbName := "routing_test_" + t.Name()
	dbPath := filepath.Join(t.TempDir(), "app.db")

	db := openRoutingTestDB(t, dbName, dbPath)

	if _, err := db.Exec(`
		CREATE TABLE app_preferences (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			show_heatmap INTEGER NOT NULL DEFAULT 1,
			show_home_title INTEGER NOT NULL DEFAULT 1,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`); err != nil {
		t.Fatalf("create legacy app_preferences: %v", err)
	}

	store := NewSQLiteStoreWithDBName(dbName)
	if err := store.EnsureSchema(); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}

	var (
		id            int
		showHeatmap   int
		showHomeTitle int
		proxyEnabled  int
		proxyURL      string
	)
	if err := db.QueryRow(
		`SELECT id, show_heatmap, show_home_title, proxy_enabled, proxy_url
		 FROM app_preferences WHERE id = 1`,
	).Scan(&id, &showHeatmap, &showHomeTitle, &proxyEnabled, &proxyURL); err != nil {
		t.Fatalf("query default app_preferences row: %v", err)
	}

	if got := fmt.Sprintf("%d:%d:%d:%d:%s", id, showHeatmap, showHomeTitle, proxyEnabled, proxyURL); got != "1:1:1:0:" {
		t.Fatalf("unexpected default row: %s", got)
	}
}

func TestSQLiteStoreEnsureSchema_MigratesLegacyProviderColumns(t *testing.T) {
	dbName := "routing_test_" + t.Name()
	dbPath := filepath.Join(t.TempDir(), "app.db")

	db := openRoutingTestDB(t, dbName, dbPath)

	if _, err := db.Exec(`
		CREATE TABLE providers (
			platform TEXT NOT NULL,
			id INTEGER NOT NULL,
			name TEXT NOT NULL,
			api_url TEXT NOT NULL DEFAULT '',
			api_key TEXT NOT NULL DEFAULT '',
			official_site TEXT NOT NULL DEFAULT '',
			icon TEXT NOT NULL DEFAULT '',
			tint TEXT NOT NULL DEFAULT '',
			accent TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 0,
			position INTEGER NOT NULL,
			supported_models_json TEXT NOT NULL DEFAULT '{}',
			model_mapping_json TEXT NOT NULL DEFAULT '{}',
			PRIMARY KEY(platform, id),
			UNIQUE(platform, name)
		);
	`); err != nil {
		t.Fatalf("seed legacy providers: %v", err)
	}

	store := NewSQLiteStoreWithDBName(dbName)
	if err := store.EnsureSchema(); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}

	for _, column := range []string{"request_body_replacements_json", "archived", "archived_at"} {
		var count int
		if err := db.QueryRow(
			"SELECT COUNT(*) FROM pragma_table_info('providers') WHERE name = ?",
			column,
		).Scan(&count); err != nil {
			t.Fatalf("query %s column: %v", column, err)
		}
		if count != 1 {
			t.Fatalf("expected %s column to exist once, got %d", column, count)
		}
	}

	// 升级前已经归档的记录没有可靠时间，保持 NULL 而不是回填迁移时间。
	if _, err := db.Exec(`INSERT INTO providers(platform, id, name, position, archived)
		VALUES ('claude', 1, 'legacy-archived', 1, 1)`); err != nil {
		t.Fatalf("seed legacy archived provider: %v", err)
	}
	profile, err := store.GetProfile(context.Background(), "claude")
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if len(profile.Providers) != 1 || !profile.Providers[0].Archived {
		t.Fatalf("expected legacy archived provider, got %+v", profile.Providers)
	}
	if profile.Providers[0].ArchivedAt != nil {
		t.Fatalf("expected legacy archived timestamp to remain unknown, got %v", profile.Providers[0].ArchivedAt)
	}
}
