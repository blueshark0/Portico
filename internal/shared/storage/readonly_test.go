package storage

import (
	"context"
	"database/sql"
	"net/url"
	"path/filepath"
	"testing"
	"time"
)

func TestReadOnlyPoolUsesWALSnapshotsWithoutBlockingWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spaces # ? session.db")
	// 夹具文件名含字面量 '?'，写入端和只读端都要用 URI 打开。
	uri := url.URL{Scheme: "file", Path: path}
	writer, err := sql.Open("sqlite", uri.String()+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	writer.SetMaxOpenConns(1)
	if _, err := writer.Exec("CREATE TABLE sample(id INTEGER PRIMARY KEY); INSERT INTO sample VALUES (1)"); err != nil {
		t.Fatal(err)
	}
	reader, err := sql.Open("sqlite", SQLiteReadOnlyDSN(path))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	reader.SetMaxOpenConns(4)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var conns []*sql.Conn
	defer func() {
		for _, conn := range conns {
			_ = conn.Close()
		}
	}()
	for range 4 {
		conn, err := reader.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		conns = append(conns, conn)
		var queryOnly, timeout int
		if err := conn.QueryRowContext(ctx, "PRAGMA query_only").Scan(&queryOnly); err != nil || queryOnly != 1 {
			t.Fatalf("query_only=%d: %v", queryOnly, err)
		}
		if err := conn.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&timeout); err != nil || timeout != 15000 {
			t.Fatalf("busy_timeout=%d: %v", timeout, err)
		}
		if _, err := conn.ExecContext(ctx, "INSERT INTO sample VALUES (3)"); err == nil {
			t.Fatal("read pool allowed a write")
		}
	}
	for _, conn := range conns {
		_ = conn.Close()
	}
	conns = nil
	tx, err := reader.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var count int
	if err := tx.QueryRow("SELECT COUNT(*) FROM sample").Scan(&count); err != nil || count != 1 {
		t.Fatalf("initial count=%d: %v", count, err)
	}
	if _, err := writer.ExecContext(ctx, "INSERT INTO sample VALUES (2)"); err != nil {
		t.Fatalf("WAL reader blocked writer: %v", err)
	}
	if err := tx.QueryRow("SELECT COUNT(*) FROM sample").Scan(&count); err != nil || count != 1 {
		t.Fatalf("snapshot changed: count=%d err=%v", count, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := reader.QueryRow("SELECT COUNT(*) FROM sample").Scan(&count); err != nil || count != 2 {
		t.Fatalf("new snapshot count=%d: %v", count, err)
	}
}
