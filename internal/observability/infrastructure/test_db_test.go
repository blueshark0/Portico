package infrastructure

import (
	"database/sql"
	"testing"

	"portico/internal/shared/storage"
)

func openRequestLogTestDB(t *testing.T, dsn string, maxOpen, maxIdle int) *sql.DB {
	t.Helper()
	if err := storage.OpenConnection(storage.ConnectionConfig{
		Name:        storage.RequestLogDBName,
		Driver:      "sqlite",
		DSN:         dsn,
		MaxOpenConn: maxOpen,
		MaxIdleConn: maxIdle,
	}); err != nil {
		t.Fatalf("open test request log database: %v", err)
	}
	t.Cleanup(func() { _ = storage.CloseConnection(storage.RequestLogDBName) })
	db, err := storage.DB(storage.RequestLogDBName)
	if err != nil {
		t.Fatalf("get test request log database: %v", err)
	}
	return db
}
