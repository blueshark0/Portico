package infrastructure

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"portico/internal/shared/storage"
)

func paginationFixture(t testing.TB, count int) (*sql.DB, *SQLiteQueries, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "request_log.db")
	name := "pagination_" + t.Name()
	if err := storage.OpenConnection(storage.ConnectionConfig{Name: name, Driver: "sqlite", DSN: storage.SQLiteDSN(path), MaxOpenConn: 1, MaxIdleConn: 1}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.CloseConnection(name) })
	db, err := storage.DB(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := EnsureRequestLogTableWithDB(db); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`WITH RECURSIVE n(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x<?)
		INSERT INTO request_log(platform,model,provider,http_code,input_tokens,output_tokens,cache_create_tokens,cache_read_tokens,reasoning_tokens,is_stream,is_fast,duration_sec,ttft_sec,created_at,total_cost,has_pricing,session_id)
		SELECT CASE WHEN x%2=0 THEN 'claude' ELSE 'codex' END,'gpt-4.1',
		CASE WHEN x%3=0 THEN 'alpha' ELSE 'beta' END,CASE WHEN x%7=0 THEN 500 ELSE 200 END,
		1000,100,0,0,0,1,0,1.0,0.1,datetime(?,'-'||(x%168)||' hour'),
		CASE WHEN x%3=0 THEN 0.01 WHEN x%3=1 THEN 0.2 ELSE 0.8 END,1,'s-'||(x%1000) FROM n`,
		count, timeNow().UTC().Format(timeLayout))
	if err != nil {
		t.Fatal(err)
	}
	return db, newSQLiteQueriesOn(name), path
}

func TestDeferredPaginationMatchesOriginalOrderingAndFilters(t *testing.T) {
	setRangeTestClock(t, time.Date(2026, 11, 6, 19, 0, 0, 0, time.FixedZone("UTC+8", 8*3600)))
	db, q, _ := paginationFixture(t, 2500)
	for _, platform := range []string{"", "claude", "codex"} {
		for _, provider := range []string{"", "alpha"} {
			for _, rangeKey := range []string{"today", "last3days", "last7days"} {
				for _, cost := range []string{"all", "low", "medium", "high"} {
					for _, status := range []string{"all", "success", "failed"} {
						for _, page := range []int{1, 3, 40} {
							where, args := requestLogListWhere(platform, provider, rangeKey, cost, status)
							var total int64
							if err := db.QueryRow("SELECT COUNT(*) FROM request_log"+where, args...).Scan(&total); err != nil {
								t.Fatal(err)
							}
							args = append(args, 30, (page-1)*30)
							original, err := q.queryRequestLogs(db, "SELECT "+requestLogSelectColumns+" FROM request_log"+where+" ORDER BY id DESC LIMIT ? OFFSET ?", args...)
							if err != nil {
								t.Fatal(err)
							}
							actual, err := q.ListRequestLogs(platform, provider, rangeKey, cost, status, page, 30)
							if err != nil || int64(actual.Total) != total || !reflect.DeepEqual(actual.Items, original) {
								t.Fatalf("pagination differs (%s,%s,%s,%s,%s,page=%d): total=%d/%d err=%v", platform, provider, rangeKey, cost, status, page, actual.Total, total, err)
							}
						}
					}
				}
			}
		}
	}
}

func TestLogQueriesUseIndependentReadPool(t *testing.T) {
	db, _, path := paginationFixture(t, 10)
	readName := "pagination_reader_" + t.Name()
	if err := storage.OpenConnection(storage.ConnectionConfig{Name: readName, Driver: "sqlite", DSN: storage.SQLiteReadOnlyDSN(path), MaxOpenConn: 4, MaxIdleConn: 2}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.CloseConnection(readName) })
	// 占住整个写入池，查询仍必须能经只读池完成。
	conn, err := db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	finished := make(chan error, 1)
	go func() {
		_, err := newSQLiteQueriesOn(readName).ListRequestLogs("", "", "last7days", "all", "all", 1, 200)
		finished <- err
	}()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("read query queued behind the occupied writer connection")
	}
}

func BenchmarkRequestLogPagination(b *testing.B) {
	for _, count := range []int{100000, 600000} {
		b.Run(fmt.Sprintf("rows=%d", count), func(b *testing.B) {
			db, q, _ := paginationFixture(b, count)
			// Each platform has count/2 rows; also measure a page near its end.
			for _, page := range []int{1, count / 500} {
				b.Run(fmt.Sprintf("page=%d", page), func(b *testing.B) {
					for _, legacy := range []bool{true, false} {
						name := "ids_first"
						if legacy {
							name = "original"
						}
						b.Run(name, func(b *testing.B) {
							b.ReportAllocs()
							for b.Loop() {
								if !legacy {
									if _, err := q.ListRequestLogs("claude", "", "last7days", "all", "all", page, 200); err != nil {
										b.Fatal(err)
									}
									continue
								}
								where, args := requestLogListWhere("claude", "", "last7days", "all", "all")
								var total int
								if err := db.QueryRow("SELECT COUNT(*) FROM request_log"+where, args...).Scan(&total); err != nil {
									b.Fatal(err)
								}
								args = append(args, 200, (page-1)*200)
								if _, err := q.queryRequestLogs(db, "SELECT "+requestLogSelectColumns+" FROM request_log"+where+" ORDER BY id DESC LIMIT ? OFFSET ?", args...); err != nil {
									b.Fatal(err)
								}
							}
						})
					}
				})
			}
			b.Run("startup_original", func(b *testing.B) {
				for b.Loop() {
					rows, err := db.Query("SELECT id,model,input_tokens,output_tokens,cache_create_tokens,cache_read_tokens,is_fast FROM request_log NOT INDEXED WHERE total_cost IS NULL")
					if err != nil {
						b.Fatal(err)
					}
					for rows.Next() {
						b.Fatal("fixture should contain no pending costs")
					}
					if err := rows.Err(); err != nil {
						b.Fatal(err)
					}
					_ = rows.Close()
				}
			})
			b.Run("startup_indexed", func(b *testing.B) {
				for b.Loop() {
					if err := backfillRequestLogCosts(db, q.pricing); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}

func TestExportRequestLogsAppliesFiltersOrderAndCap(t *testing.T) {
	setRangeTestClock(t, time.Date(2026, 11, 6, 19, 0, 0, 0, time.FixedZone("UTC+8", 8*3600)))
	db, q, _ := paginationFixture(t, requestLogExportMaxRows+500)
	all, err := q.ExportRequestLogs("", "", "last7days", "all", "all")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != requestLogExportMaxRows {
		t.Fatalf("expected cap %d, got %d", requestLogExportMaxRows, len(all))
	}
	seen := make(map[int64]bool, len(all))
	for i, item := range all {
		if seen[item.ID] {
			t.Fatalf("duplicate row %d", item.ID)
		}
		seen[item.ID] = true
		if i > 0 && (item.CreatedAt > all[i-1].CreatedAt || (item.CreatedAt == all[i-1].CreatedAt && item.ID > all[i-1].ID)) {
			t.Fatalf("not ordered by created_at, id desc at %d", i)
		}
	}
	filtered, err := q.ExportRequestLogs("claude", "alpha", "last7days", "all", "failed")
	if err != nil {
		t.Fatal(err)
	}
	where, args := requestLogListWhere("claude", "alpha", "last7days", "all", "failed")
	var want int
	if err := db.QueryRow("SELECT COUNT(*) FROM request_log"+where, args...).Scan(&want); err != nil {
		t.Fatal(err)
	}
	if want == 0 || len(filtered) != want {
		t.Fatalf("filtered export: got %d want %d", len(filtered), want)
	}
	for _, item := range filtered {
		if item.Platform != "claude" || item.Provider != "alpha" || item.HttpCode < 400 {
			t.Fatalf("filter leaked %+v", item)
		}
	}
}

func TestListProvidersSkipsAlongIndex(t *testing.T) {
	db, q, _ := paginationFixture(t, 500)
	if _, err := db.Exec(`INSERT INTO request_log(platform, provider, created_at) VALUES
		('claude', '', '2026-01-01 00:00:00'), ('claude', NULL, '2026-01-01 00:00:00'),
		('gemini', 'zeta', '2026-01-01 00:00:00'), ('claude', 'gamma', '2026-01-01 00:00:00')`); err != nil {
		t.Fatal(err)
	}
	for platform, want := range map[string][]string{
		"":        {"alpha", "beta", "gamma", "zeta"},
		"claude":  {"alpha", "beta", "gamma"},
		"gemini":  {"zeta"},
		"missing": {},
	} {
		got, err := q.ListProviders(platform)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("platform %q: got %v want %v", platform, got, want)
		}
		query, args := requestLogProvidersSQL(platform)
		rows, err := db.Query("EXPLAIN QUERY PLAN "+query, args...)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(detail, "SCAN request_log") {
				t.Fatalf("platform %q scans the whole table or index: %s", platform, detail)
			}
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
