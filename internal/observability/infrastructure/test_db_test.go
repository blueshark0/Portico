package infrastructure

import (
	"database/sql"
	"testing"

	"portico/internal/shared/storage"
)

// newSQLiteQueriesOn 让查询走指定连接，隔离并行存在的测试库。
func newSQLiteQueriesOn(dbName string) *SQLiteQueries {
	q := NewSQLiteQueries()
	q.dbName = dbName
	return q
}

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
