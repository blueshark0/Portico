package infrastructure

import (
	"path/filepath"
	"testing"

	"portico/internal/shared/storage"
)

func TestSQLiteRepositoryOnlyListsMainSessions(t *testing.T) {
	repo, cleanup := newSessionRepositoryForTest(t)
	defer cleanup()

	for _, sessionID := range []string{"main", "background", "promoted"} {
		if err := repo.BindSessionToProvider("codex", sessionID, "provider-a"); err != nil {
			t.Fatalf("bind %s: %v", sessionID, err)
		}
	}
	for _, sessionID := range []string{"background", "promoted"} {
		if err := repo.RecordSessionBackground("codex", sessionID, true); err != nil {
			t.Fatalf("hide %s: %v", sessionID, err)
		}
	}
	// 记忆提取可以复用主会话 ID，后台请求不能覆盖主会话的可见状态。
	if err := repo.RecordSessionBackground("codex", "main", false); err != nil {
		t.Fatalf("mark main session: %v", err)
	}
	if err := repo.RecordSessionBackground("codex", "main", true); err != nil {
		t.Fatalf("record background traffic on main session: %v", err)
	}

	sessions, err := repo.GetPlatformSessions("codex")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].SessionID != "main" {
		t.Fatalf("expected only the main session, got %+v", sessions)
	}
	provider, err := repo.GetSessionProvider("codex", "background")
	if err != nil || provider != "provider-a" {
		t.Fatalf("hidden sessions must retain routing state: provider=%q err=%v", provider, err)
	}

	// 同一 ID 后续出现主会话请求时，恢复展示。
	if err := repo.RecordSessionBackground("codex", "promoted", false); err != nil {
		t.Fatalf("promote session: %v", err)
	}
	sessions, err = repo.GetPlatformSessions("codex")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected main and promoted sessions, got %+v", sessions)
	}

	if err := repo.BindSessionToProvider("claude", "claude-main", "provider-a"); err != nil {
		t.Fatalf("bind Claude session: %v", err)
	}
	claudeSessions, err := repo.GetPlatformSessions("claude")
	if err != nil || len(claudeSessions) != 1 {
		t.Fatalf("other platforms must remain visible without Codex classification: sessions=%+v err=%v", claudeSessions, err)
	}
}

func TestSQLiteRepositorySchemaUpgradeKeepsLegacySessionsVisible(t *testing.T) {
	dbName := "sessions_test_" + t.Name()
	dbPath := filepath.Join(t.TempDir(), "session.db")
	if err := storage.OpenConnection(storage.ConnectionConfig{
		Name: dbName, Driver: "sqlite", DSN: dbPath,
	}); err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = storage.CloseConnection(dbName) })
	db, err := storage.DB(dbName)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(`CREATE TABLE session_provider_binding (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		platform TEXT NOT NULL,
		session_id TEXT NOT NULL,
		provider_name TEXT NOT NULL,
		last_success_at DATETIME NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(platform, session_id)
	)`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO session_provider_binding
		(platform, session_id, provider_name, last_success_at) VALUES ('codex', 'legacy', 'provider-a', CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}

	repo := NewSQLiteRepository(dbName)
	if err = repo.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	sessions, err := repo.GetPlatformSessions("codex")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].SessionID != "legacy" {
		t.Fatalf("legacy binding should remain visible after upgrade: %+v", sessions)
	}
}

func newSessionRepositoryForTest(t *testing.T) (*SQLiteRepository, func()) {
	t.Helper()
	dbName := "sessions_test_" + t.Name()
	dbPath := filepath.Join(t.TempDir(), "session.db")
	if err := storage.OpenConnection(storage.ConnectionConfig{
		Name: dbName, Driver: "sqlite", DSN: dbPath,
	}); err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = storage.CloseConnection(dbName) })
	repo := NewSQLiteRepository(dbName)
	if err := repo.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	return repo, func() { _ = storage.CloseConnection(dbName) }
}
