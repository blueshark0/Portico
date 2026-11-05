package infrastructure

import (
	"database/sql"
	"testing"

	"portico/internal/shared/storage"
)

func openRoutingTestDB(t *testing.T, name, dsn string) *sql.DB {
	t.Helper()
	if err := storage.OpenConnection(storage.ConnectionConfig{
		Name:   name,
		Driver: "sqlite",
		DSN:    dsn,
	}); err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = storage.CloseConnection(name) })
	db, err := storage.DB(name)
	if err != nil {
		t.Fatalf("get test database: %v", err)
	}
	return db
}
