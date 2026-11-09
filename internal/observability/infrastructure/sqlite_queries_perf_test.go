package infrastructure

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	observabilitydomain "portico/internal/observability/domain"
	"portico/internal/shared/storage"
)

func TestListRequestLogs_OrdersByCreatedAtDescWithIDTieBreak(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*60*60)
	now := time.Date(2026, time.March, 16, 12, 0, 0, 0, loc)
	db := setupRangeQueryTestDB(t)
	setRangeTestClock(t, now)

	// 插入顺序与 created_at 顺序故意不一致；同一秒两条以 id 倒序排列。
	seedRequestLog(t, db, time.Date(2026, time.March, 16, 9, 0, 0, 0, loc), "claude", "alpha", "m-0900")
	seedRequestLog(t, db, time.Date(2026, time.March, 16, 11, 0, 0, 0, loc), "claude", "alpha", "m-1100-a")
	seedRequestLog(t, db, time.Date(2026, time.March, 16, 10, 0, 0, 0, loc), "claude", "alpha", "m-1000")
	seedRequestLog(t, db, time.Date(2026, time.March, 16, 11, 0, 0, 0, loc), "claude", "alpha", "m-1100-b")

	page, err := NewSQLiteQueries().ListRequestLogs("", "", "today", "all", "all", 1, 15)
	if err != nil {
		t.Fatalf("ListRequestLogs: %v", err)
	}
	got := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		got = append(got, item.Model)
	}
	want := []string{"m-1100-b", "m-1100-a", "m-1000", "m-0900"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected order: got %v want %v", got, want)
	}
}

// 默认视图（不按平台/供应商过滤）的分页查询必须沿 created_at 索引直接取前 N 行，
// 不能退化为把整个时间范围的行取出后再排序。
func TestRequestLogListPageQuery_UsesIndexOrder(t *testing.T) {
	db := setupRangeQueryTestDB(t)
	where, args := requestLogListWhere("", "", "last7days", "all", "all")
	args = append(args, 200, 0)
	rows, err := db.Query("EXPLAIN QUERY PLAN "+requestLogListPageQuery(where), args...)
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatalf("scan plan: %v", err)
		}
		plan = append(plan, detail)
	}
	joined := strings.Join(plan, "\n")
	if strings.Contains(joined, "TEMP B-TREE FOR ORDER BY") {
		t.Fatalf("list query sorts the whole range instead of using index order:\n%s", joined)
	}
	if !strings.Contains(joined, "idx_request_log_created_at") {
		t.Fatalf("expected list query to use idx_request_log_created_at:\n%s", joined)
	}
}

func TestStatsSince_CacheInvalidatedByRequestLogWrite(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*60*60)
	now := time.Date(2026, time.March, 16, 12, 0, 0, 0, loc)
	db := setupRangeQueryTestDB(t)
	setRangeTestClock(t, now)
	seedRequestLog(t, db, time.Date(2026, time.March, 16, 9, 0, 0, 0, loc), "claude", "alpha", "gpt-4.1")

	queries := NewSQLiteQueries()
	first, err := queries.StatsSince("claude", "", "today", "all", "all")
	if err != nil || first.TotalRequests != 1 {
		t.Fatalf("first StatsSince: total=%d err=%v", first.TotalRequests, err)
	}

	// 绕过写入路径的直接插入不会递增数据版本：命中缓存，证明缓存确实生效。
	seedRequestLog(t, db, time.Date(2026, time.March, 16, 10, 0, 0, 0, loc), "claude", "alpha", "gpt-4.1")
	cached, err := queries.StatsSince("claude", "", "today", "all", "all")
	if err != nil || cached.TotalRequests != 1 {
		t.Fatalf("expected cached total 1, got %d err=%v", cached.TotalRequests, err)
	}

	// 经由写入路径落库后缓存立即失效，不必等 TTL。
	if err := insertRequestLogs([]*observabilitydomain.RequestLog{{
		Platform:  "claude",
		Provider:  "alpha",
		Model:     "gpt-4.1",
		HttpCode:  200,
		CreatedAt: time.Date(2026, time.March, 16, 11, 0, 0, 0, loc).UTC().Format(timeLayout),
	}}); err != nil {
		t.Fatalf("insertRequestLogs: %v", err)
	}
	fresh, err := queries.StatsSince("claude", "", "today", "all", "all")
	if err != nil || fresh.TotalRequests != 3 {
		t.Fatalf("expected fresh total 3 after write, got %d err=%v", fresh.TotalRequests, err)
	}
}

func TestQueriesUseReadPoolWhenRegistered(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*60*60)
	now := time.Date(2026, time.March, 16, 12, 0, 0, 0, loc)
	dbPath := filepath.Join(t.TempDir(), "request_log.db")
	db := openRequestLogTestDB(t, storage.SQLiteDSN(dbPath), 1, 1)
	if err := EnsureRequestLogTableWithDB(db); err != nil {
		t.Fatalf("EnsureRequestLogTableWithDB: %v", err)
	}
	if err := storage.OpenConnection(storage.ConnectionConfig{
		Name: storage.RequestLogReadDBName, Driver: "sqlite", DSN: storage.SQLiteDSN(dbPath), MaxOpenConn: 4, MaxIdleConn: 2,
	}); err != nil {
		t.Fatalf("open read pool: %v", err)
	}
	t.Cleanup(func() { _ = storage.CloseConnection(storage.RequestLogReadDBName) })
	setRangeTestClock(t, now)

	readDB, err := requestLogReadDB()
	if err != nil {
		t.Fatalf("requestLogReadDB: %v", err)
	}
	if readDB == db {
		t.Fatal("expected queries to use the dedicated read pool")
	}

	// 写入池写入的数据对只读池可见。
	if err := insertRequestLogs([]*observabilitydomain.RequestLog{{
		Platform:  "claude",
		Provider:  "alpha",
		Model:     "gpt-4.1",
		HttpCode:  200,
		CreatedAt: time.Date(2026, time.March, 16, 9, 0, 0, 0, loc).UTC().Format(timeLayout),
	}}); err != nil {
		t.Fatalf("insertRequestLogs: %v", err)
	}
	page, err := NewSQLiteQueries().ListRequestLogs("", "", "today", "all", "all", 1, 15)
	if err != nil || page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("expected 1 log via read pool, got total=%d items=%d err=%v", page.Total, len(page.Items), err)
	}
}
