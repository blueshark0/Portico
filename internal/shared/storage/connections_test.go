package storage

import (
	"path/filepath"
	"testing"
)

func TestConnectionLifecycle(t *testing.T) {
	name := "storage_test_" + t.Name()
	path := filepath.Join(t.TempDir(), "app.db")
	config := ConnectionConfig{
		Name:        name,
		Driver:      "sqlite",
		DSN:         SQLiteDSN(path),
		MaxOpenConn: 1,
		MaxIdleConn: 1,
	}
	t.Cleanup(func() { _ = CloseConnection(name) })

	if _, err := DB(name); err == nil {
		t.Fatal("expected an unregistered connection to fail")
	}
	if err := OpenConnections([]ConnectionConfig{config}); err != nil {
		t.Fatalf("open connection: %v", err)
	}
	db, err := DB(name)
	if err != nil {
		t.Fatalf("get connection: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("ping connection: %v", err)
	}
	if err := OpenConnection(config); err == nil {
		t.Fatal("expected duplicate connection registration to fail")
	}

	if err := CloseConnection(name); err != nil {
		t.Fatalf("close connection: %v", err)
	}
	if _, err := DB(name); err == nil {
		t.Fatal("expected closed connection to be unregistered")
	}
	if err := CloseConnection(name); err != nil {
		t.Fatalf("repeated close should be idempotent: %v", err)
	}
}
