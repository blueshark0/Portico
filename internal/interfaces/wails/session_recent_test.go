package wails

import (
	"path/filepath"
	"testing"
	"time"

	sessionapp "portico/internal/sessions/application"
	sessioninfra "portico/internal/sessions/infrastructure"
	"portico/internal/shared/storage"
)

func TestRecentSessionsSkipUsageAndRespectVisibilityAndExpiry(t *testing.T) {
	name := "session_recent_" + t.Name()
	if err := storage.OpenConnection(storage.ConnectionConfig{
		Name: name, Driver: "sqlite", DSN: storage.SQLiteDSN(filepath.Join(t.TempDir(), "session.db")), MaxOpenConn: 1, MaxIdleConn: 1,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.CloseConnection(name) })
	repo := sessioninfra.NewSQLiteRepository(name)
	if err := repo.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	for _, s := range []struct{ platform, id string }{{"claude", "main"}, {"claude", "expired"}, {"codex", "main"}, {"codex", "hidden"}, {"codex", "unclassified"}} {
		if err := repo.BindSessionToProvider(s.platform, s.id, "provider"); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.RecordSessionBackground("codex", "main", false); err != nil {
		t.Fatal(err)
	}
	if err := repo.RecordSessionBackground("codex", "hidden", true); err != nil {
		t.Fatal(err)
	}
	db, _ := storage.DB(name)
	if _, err := db.Exec("UPDATE session_provider_binding SET last_success_at = ? WHERE session_id = 'expired'", time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	usage := &sessionUsageSpy{}
	facade := NewSessionFacade(sessionapp.NewService(repo), usage)
	items, err := facade.ListRecent(5)
	if err != nil || len(items) != 2 || usage.calls != 0 {
		t.Fatalf("recent metadata: items=%+v usage_calls=%d err=%v", items, usage.calls, err)
	}
	for _, item := range items {
		if item.SessionID != "main" {
			t.Fatalf("unexpected visible session: %+v", item)
		}
	}
}
