package infrastructure

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestEnsureRequestLogTableWithDB_MigratesLegacySchema(t *testing.T) {
	db, err := sql.Open("sqlite", t.TempDir()+"/request_log.db")
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	if _, err := db.Exec(`
		CREATE TABLE request_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			platform TEXT,
			model TEXT,
			provider TEXT,
			http_code INTEGER,
			input_tokens INTEGER,
			output_tokens INTEGER,
			cache_create_tokens INTEGER,
			cache_read_tokens INTEGER,
			reasoning_tokens INTEGER
		);
		INSERT INTO request_log (
			platform,
			model,
			provider,
			http_code,
			input_tokens,
			output_tokens,
			cache_create_tokens,
			cache_read_tokens,
			reasoning_tokens
		) VALUES ('desktop', 'gpt-4.1', 'openai', 200, 10, 20, 0, 0, 0);
	`); err != nil {
		t.Fatalf("seed legacy request_log: %v", err)
	}

	if err := EnsureRequestLogTableWithDB(db); err != nil {
		t.Fatalf("ensure request_log schema: %v", err)
	}

	for _, column := range []string{"created_at", "is_stream", "is_fast", "duration_sec", "ttft_sec", "session_id"} {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('request_log') WHERE name = ?", column).Scan(&count); err != nil {
			t.Fatalf("query column %s: %v", column, err)
		}
		if count != 1 {
			t.Fatalf("expected column %s to exist once, got %d", column, count)
		}
	}

	var createdAt sql.NullString
	var isStream sql.NullInt64
	var isFast sql.NullInt64
	var durationSec sql.NullFloat64
	if err := db.QueryRow(
		"SELECT strftime('%Y-%m-%d %H:%M:%S', created_at), is_stream, is_fast, duration_sec FROM request_log WHERE id = 1",
	).Scan(&createdAt, &isStream, &isFast, &durationSec); err != nil {
		t.Fatalf("query migrated row: %v", err)
	}
	if !createdAt.Valid || createdAt.String == "" {
		t.Fatal("expected migrated row to have created_at populated")
	}
	if _, err := time.Parse(timeLayout, createdAt.String); err != nil {
		t.Fatalf("parse created_at %q: %v", createdAt.String, err)
	}
	if !isStream.Valid || isStream.Int64 != 0 {
		t.Fatalf("expected is_stream to default to 0, got %+v", isStream)
	}
	if !isFast.Valid || isFast.Int64 != 0 {
		t.Fatalf("expected is_fast to default to 0, got %+v", isFast)
	}
	if !durationSec.Valid || durationSec.Float64 != 0 {
		t.Fatalf("expected duration_sec to default to 0, got %+v", durationSec)
	}
}

func TestRequestLogCreatedAt_SetsCreatedAt(t *testing.T) {
	value := requestLogCreatedAt("")

	if value == "" {
		t.Fatal("expected generated created_at string")
	}
	if _, err := time.Parse(timeLayout, value); err != nil {
		t.Fatalf("parse generated created_at %q: %v", value, err)
	}
}

func TestRequestLogCreatedAt_PreservesCreatedAt(t *testing.T) {
	const createdAt = "2026-03-16 08:00:00"

	if got := requestLogCreatedAt(createdAt); got != createdAt {
		t.Fatalf("expected created_at %q, got %q", createdAt, got)
	}
}
