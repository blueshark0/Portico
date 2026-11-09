package infrastructure

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func TestCostBackfillBatchesResumeAndPreserveExistingCosts(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "backfill.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if err := EnsureRequestLogTableWithDB(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`WITH RECURSIVE n(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x<1205)
		INSERT INTO request_log(model,input_tokens,output_tokens) SELECT 'gpt-4.1',100,10 FROM n;
		UPDATE request_log SET total_cost=99,has_pricing=1 WHERE id=1;
		CREATE TRIGGER interrupt_backfill BEFORE UPDATE OF total_cost ON request_log
		WHEN NEW.id>501 BEGIN SELECT RAISE(ABORT,'simulated interruption'); END;`); err != nil {
		t.Fatal(err)
	}
	pricing := newRangeTestQueriesWithPricing(t).pricing
	if err := backfillRequestLogCosts(db, pricing); err == nil {
		t.Fatal("expected second batch interruption")
	}
	var priced int
	if err := db.QueryRow("SELECT COUNT(*) FROM request_log WHERE total_cost IS NOT NULL").Scan(&priced); err != nil || priced != 501 {
		t.Fatalf("first batch was not committed independently: priced=%d err=%v", priced, err)
	}
	if _, err := db.Exec("DROP TRIGGER interrupt_backfill"); err != nil {
		t.Fatal(err)
	}
	if err := backfillRequestLogCosts(db, nil); err != nil {
		t.Fatal(err)
	}
	var pending int
	if err := db.QueryRow("SELECT COUNT(*) FROM request_log WHERE total_cost IS NULL").Scan(&pending); err != nil || pending != 704 {
		t.Fatalf("nil pricing changed pending rows: %d, %v", pending, err)
	}
	if err := backfillRequestLogCosts(db, pricing); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM request_log WHERE total_cost IS NULL").Scan(&pending); err != nil || pending != 0 {
		t.Fatalf("resume left pending rows: %d, %v", pending, err)
	}
	var preserved float64
	if err := db.QueryRow("SELECT total_cost FROM request_log WHERE id=1").Scan(&preserved); err != nil || preserved != 99 {
		t.Fatalf("existing cost overwritten: %f, %v", preserved, err)
	}
	if err := EnsureRequestLogTableWithDB(db); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query("EXPLAIN QUERY PLAN SELECT id,model FROM request_log WHERE total_cost IS NULL AND id>? ORDER BY id LIMIT ?", 0, requestLogCostBatchSize)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	indexed := false
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		indexed = indexed || strings.Contains(detail, "idx_request_log_unpriced")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !indexed {
		t.Fatal("startup backfill does not use the pending-only index")
	}
}
