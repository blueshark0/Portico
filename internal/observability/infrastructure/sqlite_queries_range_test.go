package infrastructure

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	modelpricing "portico/resources/model-pricing"
)

func TestListRequestLogs_FiltersByNaturalDayRange(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*60*60)
	now := time.Date(2026, time.March, 16, 12, 0, 0, 0, loc)
	db := setupRangeQueryTestDB(t)
	setRangeTestClock(t, now)

	seedRequestLog(t, db, time.Date(2026, time.March, 16, 9, 0, 0, 0, loc), "claude", "alpha", "gpt-4.1")
	seedRequestLog(t, db, time.Date(2026, time.March, 15, 11, 0, 0, 0, loc), "claude", "alpha", "gpt-4.1")
	seedRequestLog(t, db, time.Date(2026, time.March, 14, 8, 0, 0, 0, loc), "claude", "beta", "gpt-4.1")
	seedRequestLog(t, db, time.Date(2026, time.March, 10, 10, 0, 0, 0, loc), "codex", "alpha", "gpt-4.1")
	seedRequestLog(t, db, time.Date(2026, time.March, 9, 10, 0, 0, 0, loc), "claude", "alpha", "gpt-4.1")

	queries := NewSQLiteQueries()

	todayLogs, err := queries.ListRequestLogs("", "", "today", "all", "all", 1, 100)
	if err != nil {
		t.Fatalf("ListRequestLogs today: %v", err)
	}
	if got := len(todayLogs.Items); got != 1 {
		t.Fatalf("expected 1 today log, got %d", got)
	}
	if todayLogs.Total != 1 {
		t.Fatalf("expected today total 1, got %d", todayLogs.Total)
	}

	last3DaysLogs, err := queries.ListRequestLogs("", "", "last3days", "all", "all", 1, 100)
	if err != nil {
		t.Fatalf("ListRequestLogs last3days: %v", err)
	}
	if got := len(last3DaysLogs.Items); got != 3 {
		t.Fatalf("expected 3 logs in last3days, got %d", got)
	}

	last7DaysLogs, err := queries.ListRequestLogs("", "", "last7days", "all", "all", 1, 100)
	if err != nil {
		t.Fatalf("ListRequestLogs last7days: %v", err)
	}
	if got := len(last7DaysLogs.Items); got != 4 {
		t.Fatalf("expected 4 logs in last7days, got %d", got)
	}

	filteredLogs, err := queries.ListRequestLogs("claude", "alpha", "last3days", "all", "all", 1, 100)
	if err != nil {
		t.Fatalf("ListRequestLogs filtered last3days: %v", err)
	}
	if got := len(filteredLogs.Items); got != 2 {
		t.Fatalf("expected 2 filtered logs in last3days, got %d", got)
	}
}

func TestListRequestLogs_PaginatesWithTotal(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*60*60)
	now := time.Date(2026, time.March, 16, 12, 0, 0, 0, loc)
	db := setupRangeQueryTestDB(t)
	setRangeTestClock(t, now)

	for i := 0; i < 5; i++ {
		seedRequestLog(t, db, time.Date(2026, time.March, 16, 9, i, 0, 0, loc), "claude", "alpha", "gpt-4.1")
	}

	queries := NewSQLiteQueries()
	page, err := queries.ListRequestLogs("", "", "today", "all", "all", 2, 2)
	if err != nil {
		t.Fatalf("ListRequestLogs page 2: %v", err)
	}
	if page.Total != 5 {
		t.Fatalf("expected total 5, got %d", page.Total)
	}
	if page.Page != 2 || page.PageSize != 15 {
		t.Fatalf("expected invalid page size to normalize to page=2 pageSize=15, got page=%d pageSize=%d", page.Page, page.PageSize)
	}

	page, err = queries.ListRequestLogs("", "", "today", "all", "all", 2, 30)
	if err != nil {
		t.Fatalf("ListRequestLogs normalized page 2: %v", err)
	}
	if page.Total != 5 || len(page.Items) != 0 {
		t.Fatalf("expected empty second page with total 5, got total=%d items=%d", page.Total, len(page.Items))
	}

	page, err = queries.ListRequestLogs("", "", "today", "all", "all", 1, 30)
	if err != nil {
		t.Fatalf("ListRequestLogs page 1: %v", err)
	}
	if page.Page != 1 || page.PageSize != 30 || len(page.Items) != 5 {
		t.Fatalf("unexpected first page: page=%d pageSize=%d items=%d", page.Page, page.PageSize, len(page.Items))
	}
	page, err = queries.ListRequestLogs("", "", "today", "all", "all", 1, 200)
	if err != nil {
		t.Fatalf("ListRequestLogs page size 200: %v", err)
	}
	if page.PageSize != 200 || len(page.Items) != 5 {
		t.Fatalf("expected 200-row page size, got pageSize=%d items=%d", page.PageSize, len(page.Items))
	}
}

func TestListRequestLogs_FiltersByCostTier(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*60*60)
	now := time.Date(2026, time.March, 16, 12, 0, 0, 0, loc)
	db := setupRangeQueryTestDB(t)
	setRangeTestClock(t, now)

	seedRequestLogWithUsage(t, db, time.Date(2026, time.March, 16, 9, 0, 0, 0, loc), "claude", "alpha", "gpt-4.1", 1, 1, 0)
	seedRequestLogWithUsage(t, db, time.Date(2026, time.March, 16, 10, 0, 0, 0, loc), "claude", "alpha", "gpt-4.1", 1, 100000, 0)

	queries := newRangeTestQueriesWithPricing(t)
	// seed 直接 INSERT 模拟补列前的历史行（total_cost 为 NULL）：
	// tier 过滤已下推到落库列，必须先经回填才可见。
	if err := backfillRequestLogCosts(db, queries.pricing); err != nil {
		t.Fatalf("backfillRequestLogCosts: %v", err)
	}
	highLogs, err := queries.ListRequestLogs("claude", "alpha", "today", "high", "all", 1, 15)
	if err != nil {
		t.Fatalf("ListRequestLogs high: %v", err)
	}
	if highLogs.Total != 1 || len(highLogs.Items) != 1 {
		t.Fatalf("expected one high-cost log, got total=%d items=%d", highLogs.Total, len(highLogs.Items))
	}
	if highLogs.Items[0].TotalCost <= 0.5 {
		t.Fatalf("expected high-cost log above 0.5, got %.4f", highLogs.Items[0].TotalCost)
	}
	if highLogs.Items[0].CostTier != "high" {
		t.Fatalf("expected cost_tier high, got %q", highLogs.Items[0].CostTier)
	}

	lowLogs, err := queries.ListRequestLogs("claude", "alpha", "today", "low", "all", 1, 15)
	if err != nil {
		t.Fatalf("ListRequestLogs low: %v", err)
	}
	if lowLogs.Total != 1 || len(lowLogs.Items) != 1 || lowLogs.Items[0].CostTier != "low" {
		t.Fatalf("expected one low-cost log, got total=%d items=%d", lowLogs.Total, len(lowLogs.Items))
	}

	highStats, err := queries.StatsSince("claude", "alpha", "today", "high", "all")
	if err != nil {
		t.Fatalf("StatsSince high: %v", err)
	}
	if highStats.TotalRequests != 1 {
		t.Fatalf("expected high-cost stats total 1, got %d", highStats.TotalRequests)
	}
}

func TestListRequestLogs_FiltersByHTTPStatus(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*60*60)
	now := time.Date(2026, time.March, 16, 12, 0, 0, 0, loc)
	db := setupRangeQueryTestDB(t)
	setRangeTestClock(t, now)

	seedRequestLogWithHTTPCode(t, db, time.Date(2026, time.March, 16, 9, 0, 0, 0, loc), "claude", "alpha", "gpt-4.1", 1, 1, 0, false, 200)
	seedRequestLogWithHTTPCode(t, db, time.Date(2026, time.March, 16, 10, 0, 0, 0, loc), "claude", "alpha", "gpt-4.1", 0, 0, 0, false, 500)

	queries := newRangeTestQueriesWithPricing(t)

	successLogs, err := queries.ListRequestLogs("claude", "alpha", "today", "all", "success", 1, 15)
	if err != nil {
		t.Fatalf("ListRequestLogs success: %v", err)
	}
	if successLogs.Total != 1 || len(successLogs.Items) != 1 || successLogs.Items[0].HttpCode != 200 {
		t.Fatalf("expected one 200 log, got total=%d items=%d", successLogs.Total, len(successLogs.Items))
	}

	// 空串/非法值都按默认的 success 处理。
	defaultLogs, err := queries.ListRequestLogs("claude", "alpha", "today", "all", "", 1, 15)
	if err != nil {
		t.Fatalf("ListRequestLogs default: %v", err)
	}
	if defaultLogs.Total != 1 {
		t.Fatalf("expected default filter to keep only 200 logs, got total=%d", defaultLogs.Total)
	}

	allLogs, err := queries.ListRequestLogs("claude", "alpha", "today", "all", "all", 1, 15)
	if err != nil {
		t.Fatalf("ListRequestLogs all: %v", err)
	}
	if allLogs.Total != 2 || len(allLogs.Items) != 2 {
		t.Fatalf("expected two logs with all filter, got total=%d items=%d", allLogs.Total, len(allLogs.Items))
	}

	failedLogs, err := queries.ListRequestLogs("claude", "alpha", "today", "all", "failed", 1, 15)
	if err != nil {
		t.Fatalf("ListRequestLogs failed: %v", err)
	}
	if failedLogs.Total != 1 || len(failedLogs.Items) != 1 || failedLogs.Items[0].HttpCode != 500 {
		t.Fatalf("expected one failed log, got %+v", failedLogs)
	}

	successStats, err := queries.StatsSince("claude", "alpha", "today", "all", "success")
	if err != nil {
		t.Fatalf("StatsSince success: %v", err)
	}
	if successStats.TotalRequests != 1 {
		t.Fatalf("expected success stats total 1, got %d", successStats.TotalRequests)
	}
	allStats, err := queries.StatsSince("claude", "alpha", "today", "all", "all")
	if err != nil {
		t.Fatalf("StatsSince all: %v", err)
	}
	if allStats.TotalRequests != 2 {
		t.Fatalf("expected all stats total 2, got %d", allStats.TotalRequests)
	}
}

func TestStatsSinceCountsCacheHitRequests(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*60*60)
	now := time.Date(2026, time.March, 16, 12, 0, 0, 0, loc)
	db := setupRangeQueryTestDB(t)
	setRangeTestClock(t, now)
	seedRequestLogWithHTTPCode(t, db, time.Date(2026, time.March, 16, 9, 0, 0, 0, loc), "claude", "alpha", "gpt-4.1", 1, 1, 0, false, 200)
	seedRequestLogWithHTTPCode(t, db, time.Date(2026, time.March, 16, 10, 0, 0, 0, loc), "claude", "alpha", "gpt-4.1", 1, 1, 0, false, 500)
	if _, err := db.Exec("UPDATE request_log SET cache_read_tokens = 64 WHERE http_code = 200"); err != nil {
		t.Fatalf("seed cache read: %v", err)
	}

	stats, err := newRangeTestQueriesWithPricing(t).StatsSince("claude", "alpha", "today", "all", "all")
	if err != nil {
		t.Fatalf("StatsSince: %v", err)
	}
	if stats.CacheHitRequests != 1 {
		t.Fatalf("expected one request with cache reads, got %d", stats.CacheHitRequests)
	}
}

func TestListRequestLogs_FastCostDoublesBaseCost(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*60*60)
	now := time.Date(2026, time.March, 16, 12, 0, 0, 0, loc)
	db := setupRangeQueryTestDB(t)
	setRangeTestClock(t, now)

	createdAt := time.Date(2026, time.March, 16, 9, 0, 0, 0, loc)
	seedRequestLogWithUsage(t, db, createdAt, "codex", "alpha", "gpt-5", 100, 50, 0)
	seedFastRequestLogWithUsage(t, db, createdAt.Add(time.Minute), "codex", "alpha", "gpt-5", 100, 50, 0)

	queries := newRangeTestQueriesWithPricing(t)
	page, err := queries.ListRequestLogs("codex", "alpha", "today", "all", "all", 1, 15)
	if err != nil {
		t.Fatalf("ListRequestLogs: %v", err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("expected 2 logs, got %d", len(page.Items))
	}

	var normalCost, fastCost float64
	for _, item := range page.Items {
		if item.IsFast {
			fastCost = item.TotalCost
			continue
		}
		normalCost = item.TotalCost
	}
	if normalCost <= 0 || fastCost <= 0 {
		t.Fatalf("expected positive costs, normal=%.8f fast=%.8f", normalCost, fastCost)
	}
	if fastCost != normalCost*2 {
		t.Fatalf("expected fast cost to double normal cost, normal=%.8f fast=%.8f", normalCost, fastCost)
	}

	stats, err := queries.StatsSince("codex", "alpha", "today", "all", "all")
	if err != nil {
		t.Fatalf("StatsSince: %v", err)
	}
	if stats.CostTotal != normalCost+fastCost {
		t.Fatalf("expected stats cost %.8f, got %.8f", normalCost+fastCost, stats.CostTotal)
	}
}

func TestStatsSince_UsesAdaptiveBucketsForRange(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*60*60)
	now := time.Date(2026, time.March, 16, 12, 0, 0, 0, loc)
	db := setupRangeQueryTestDB(t)
	setRangeTestClock(t, now)

	seedRequestLogWithUsage(t, db, time.Date(2026, time.March, 16, 9, 15, 0, 0, loc), "claude", "alpha", "gpt-4.1", 10, 5, 2)
	seedRequestLogWithUsage(t, db, time.Date(2026, time.March, 16, 9, 45, 0, 0, loc), "claude", "alpha", "gpt-4.1", 2, 3, 1)
	seedRequestLogWithUsage(t, db, time.Date(2026, time.March, 15, 11, 0, 0, 0, loc), "claude", "alpha", "gpt-4.1", 7, 1, 0)
	seedRequestLogWithUsage(t, db, time.Date(2026, time.March, 14, 8, 0, 0, 0, loc), "claude", "beta", "gpt-4.1", 4, 6, 0)
	seedRequestLogWithUsage(t, db, time.Date(2026, time.March, 13, 20, 0, 0, 0, loc), "claude", "alpha", "gpt-4.1", 8, 1, 0)

	queries := NewSQLiteQueries()

	todayStats, err := queries.StatsSince("claude", "alpha", "today", "all", "all")
	if err != nil {
		t.Fatalf("StatsSince today: %v", err)
	}
	if got := len(todayStats.Series); got != 24 {
		t.Fatalf("expected 24 hourly buckets for today, got %d", got)
	}
	if todayStats.TotalRequests != 2 {
		t.Fatalf("expected 2 today requests, got %d", todayStats.TotalRequests)
	}
	if todayStats.InputTokens != 12 || todayStats.OutputTokens != 8 || todayStats.ReasoningTokens != 3 {
		t.Fatalf("unexpected today totals: input=%d output=%d reasoning=%d", todayStats.InputTokens, todayStats.OutputTokens, todayStats.ReasoningTokens)
	}
	if bucket := todayStats.Series[9]; bucket.TotalRequests != 2 {
		t.Fatalf("expected 09:00 bucket to have 2 requests, got %d", bucket.TotalRequests)
	}

	last3DaysStats, err := queries.StatsSince("claude", "", "last3days", "all", "all")
	if err != nil {
		t.Fatalf("StatsSince last3days: %v", err)
	}
	if got := len(last3DaysStats.Series); got != 3 {
		t.Fatalf("expected 3 daily buckets for last3days, got %d", got)
	}
	if last3DaysStats.TotalRequests != 4 {
		t.Fatalf("expected 4 requests in last3days, got %d", last3DaysStats.TotalRequests)
	}
	if got := last3DaysStats.Series[0].Day; got != "2026-03-14 00:00:00" {
		t.Fatalf("unexpected first last3days bucket: %s", got)
	}
	if last3DaysStats.Series[0].TotalRequests != 1 || last3DaysStats.Series[1].TotalRequests != 1 || last3DaysStats.Series[2].TotalRequests != 2 {
		t.Fatalf("unexpected last3days bucket counts: %+v", last3DaysStats.Series)
	}

	last7DaysStats, err := queries.StatsSince("claude", "alpha", "last7days", "all", "all")
	if err != nil {
		t.Fatalf("StatsSince last7days: %v", err)
	}
	if got := len(last7DaysStats.Series); got != 7 {
		t.Fatalf("expected 7 daily buckets for last7days, got %d", got)
	}
	if last7DaysStats.TotalRequests != 4 {
		t.Fatalf("expected 4 alpha requests in last7days, got %d", last7DaysStats.TotalRequests)
	}
}

func newRangeTestQueriesWithPricing(t *testing.T) *SQLiteQueries {
	t.Helper()
	pricing, err := modelpricing.NewService()
	if err != nil {
		t.Fatalf("modelpricing.NewService: %v", err)
	}
	queries := NewSQLiteQueries()
	queries.pricing = pricing
	return queries
}

func setupRangeQueryTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "request_log.db")
	db := openRequestLogTestDB(t, dbPath, 0, 0)
	if err := EnsureRequestLogTableWithDB(db); err != nil {
		t.Fatalf("EnsureRequestLogTableWithDB: %v", err)
	}
	return db
}

func setRangeTestClock(t *testing.T, now time.Time) {
	t.Helper()

	originalLocal := time.Local
	originalNow := timeNow
	time.Local = now.Location()
	timeNow = func() time.Time { return now }
	t.Cleanup(func() {
		time.Local = originalLocal
		timeNow = originalNow
	})
}

func seedRequestLog(t *testing.T, db *sql.DB, createdAt time.Time, platform string, provider string, model string) {
	t.Helper()
	seedRequestLogWithUsage(t, db, createdAt, platform, provider, model, 1, 1, 0)
}

func seedRequestLogWithUsage(
	t *testing.T,
	db *sql.DB,
	createdAt time.Time,
	platform string,
	provider string,
	model string,
	inputTokens int,
	outputTokens int,
	reasoningTokens int,
) {
	seedRequestLogWithUsageAndFast(t, db, createdAt, platform, provider, model, inputTokens, outputTokens, reasoningTokens, false)
}

func seedFastRequestLogWithUsage(
	t *testing.T,
	db *sql.DB,
	createdAt time.Time,
	platform string,
	provider string,
	model string,
	inputTokens int,
	outputTokens int,
	reasoningTokens int,
) {
	seedRequestLogWithUsageAndFast(t, db, createdAt, platform, provider, model, inputTokens, outputTokens, reasoningTokens, true)
}

func seedRequestLogWithUsageAndFast(
	t *testing.T,
	db *sql.DB,
	createdAt time.Time,
	platform string,
	provider string,
	model string,
	inputTokens int,
	outputTokens int,
	reasoningTokens int,
	isFast bool,
) {
	t.Helper()
	seedRequestLogWithHTTPCode(t, db, createdAt, platform, provider, model, inputTokens, outputTokens, reasoningTokens, isFast, 200)
}

func seedRequestLogWithHTTPCode(
	t *testing.T,
	db *sql.DB,
	createdAt time.Time,
	platform string,
	provider string,
	model string,
	inputTokens int,
	outputTokens int,
	reasoningTokens int,
	isFast bool,
	httpCode int,
) {
	t.Helper()

	_, err := db.Exec(
		`INSERT INTO request_log (
			platform,
			model,
			provider,
			http_code,
			input_tokens,
			output_tokens,
			cache_create_tokens,
			cache_read_tokens,
			reasoning_tokens,
			is_stream,
			is_fast,
			duration_sec,
			created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		platform,
		model,
		provider,
		httpCode,
		inputTokens,
		outputTokens,
		0,
		0,
		reasoningTokens,
		0,
		boolToInt(isFast),
		0.5,
		createdAt.UTC().Format(timeLayout),
	)
	if err != nil {
		t.Fatalf("seed request_log: %v", err)
	}
}
