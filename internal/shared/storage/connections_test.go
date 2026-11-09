package storage

import (
	"errors"
	"os"
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

// A read-only pool cannot checkpoint, so if it closes last the writer's final
// checkpoint is skipped and -wal/-shm stay behind. Map iteration would pick that
// order about half the time, hence the repetition.
func TestCloseConnectionsClosesInReverseRegistrationOrder(t *testing.T) {
	for i := range 16 {
		path := filepath.Join(t.TempDir(), "app.db")
		writerName := "storage_test_writer_" + t.Name()
		readerName := "storage_test_reader_" + t.Name()
		t.Cleanup(func() {
			_ = CloseConnection(writerName)
			_ = CloseConnection(readerName)
		})
		if err := OpenConnection(ConnectionConfig{Name: writerName, Driver: "sqlite", DSN: SQLiteDSN(path), MaxOpenConn: 1, MaxIdleConn: 1}); err != nil {
			t.Fatalf("open writer: %v", err)
		}
		writer, err := DB(writerName)
		if err != nil {
			t.Fatalf("get writer: %v", err)
		}
		if _, err := writer.Exec("CREATE TABLE t(v INTEGER); INSERT INTO t VALUES (1)"); err != nil {
			t.Fatalf("seed: %v", err)
		}
		if err := OpenConnection(ConnectionConfig{Name: readerName, Driver: "sqlite", DSN: SQLiteReadOnlyDSN(path), MaxOpenConn: 1, MaxIdleConn: 1}); err != nil {
			t.Fatalf("open reader: %v", err)
		}
		reader, err := DB(readerName)
		if err != nil {
			t.Fatalf("get reader: %v", err)
		}
		var v int
		if err := reader.QueryRow("SELECT v FROM t").Scan(&v); err != nil {
			t.Fatalf("read: %v", err)
		}

		if err := CloseConnections(); err != nil {
			t.Fatalf("close connections: %v", err)
		}
		for _, suffix := range []string{"-wal", "-shm"} {
			if _, err := os.Stat(path + suffix); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("iteration %d: %s left behind after shutdown (stat err=%v)", i, suffix, err)
			}
		}
	}
}

func TestCloseConnectionForgetsRegistrationOrder(t *testing.T) {
	name := "storage_test_" + t.Name()
	config := ConnectionConfig{Name: name, Driver: "sqlite", DSN: SQLiteDSN(filepath.Join(t.TempDir(), "app.db"))}
	t.Cleanup(func() { _ = CloseConnection(name) })
	for range 2 {
		if err := OpenConnection(config); err != nil {
			t.Fatalf("open: %v", err)
		}
		if err := CloseConnection(name); err != nil {
			t.Fatalf("close: %v", err)
		}
	}
	if err := CloseConnections(); err != nil {
		t.Fatalf("close connections: %v", err)
	}
}
