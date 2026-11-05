package infrastructure

import (
	"path/filepath"
	"testing"

	observabilitydomain "portico/internal/observability/domain"
	modelpricing "portico/resources/model-pricing"
)

func TestRequestLogWriter_ProcessWritesBatch(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "request_log.db")
	db := openRequestLogTestDB(t, dbPath, 1, 1)
	if err := EnsureRequestLogTableWithDB(db); err != nil {
		t.Fatalf("EnsureRequestLogTableWithDB: %v", err)
	}

	pricing, err := modelpricing.NewService()
	if err != nil {
		t.Fatalf("modelpricing.NewService: %v", err)
	}
	writer := &RequestLogWriter{pricing: pricing}
	err = writer.Process([]*observabilitydomain.RequestLog{
		{
			Platform:          "claude",
			Model:             "claude-sonnet-4",
			Provider:          "alpha-temp",
			HttpCode:          502,
			InputTokens:       11,
			OutputTokens:      22,
			CacheCreateTokens: 3,
			CacheReadTokens:   4,
			ReasoningTokens:   5,
			IsStream:          true,
			IsFast:            true,
			DurationSec:       1.25,
			TTFTSec:           0.42,
			CreatedAt:         "2026-05-29 06:30:15",
		},
		nil,
		{
			Platform:    "codex",
			Model:       "gpt-5",
			Provider:    "ice",
			HttpCode:    200,
			DurationSec: 0.75,
		},
	})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM request_log").Scan(&count); err != nil {
		t.Fatalf("count request_log: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 request_log rows, got %d", count)
	}

	var provider string
	var httpCode int
	var inputTokens int
	var isStream int
	var isFast int
	var durationSec float64
	var ttftSec float64
	var createdAt string
	var totalCost float64
	var hasPricing int
	if err := db.QueryRow(`
		SELECT provider, http_code, input_tokens, is_stream, is_fast, duration_sec, ttft_sec, strftime('%Y-%m-%d %H:%M:%S', created_at), total_cost, has_pricing
		FROM request_log
		WHERE platform = 'claude'
	`).Scan(&provider, &httpCode, &inputTokens, &isStream, &isFast, &durationSec, &ttftSec, &createdAt, &totalCost, &hasPricing); err != nil {
		t.Fatalf("query written row: %v", err)
	}
	if provider != "alpha-temp" {
		t.Fatalf("expected provider alpha-temp, got %q", provider)
	}
	if httpCode != 502 {
		t.Fatalf("expected http_code 502, got %d", httpCode)
	}
	if inputTokens != 11 {
		t.Fatalf("expected input_tokens 11, got %d", inputTokens)
	}
	if isStream != 1 {
		t.Fatalf("expected is_stream 1, got %d", isStream)
	}
	if isFast != 1 {
		t.Fatalf("expected is_fast 1, got %d", isFast)
	}
	if durationSec != 1.25 {
		t.Fatalf("expected duration_sec 1.25, got %f", durationSec)
	}
	if ttftSec != 0.42 {
		t.Fatalf("expected ttft_sec 0.42, got %f", ttftSec)
	}
	if createdAt != "2026-05-29 06:30:15" {
		t.Fatalf("expected created_at to be preserved, got %q", createdAt)
	}
	// 写入侧按当时定价物化费用列，tier 过滤/统计依赖它下推 SQL。
	if hasPricing != 1 || totalCost <= 0 {
		t.Fatalf("expected materialized cost columns, got total_cost=%f has_pricing=%d", totalCost, hasPricing)
	}
}
