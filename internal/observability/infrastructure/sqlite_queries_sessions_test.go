package infrastructure

import (
	"fmt"
	"path/filepath"
	"testing"

	observabilitydomain "portico/internal/observability/domain"
	modelpricing "portico/resources/model-pricing"
)

func TestSessionUsageForSessions_AggregatesPerSession(t *testing.T) {
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
		{Platform: "claude", Model: "claude-sonnet-4", Provider: "alpha", SessionID: "s-1", HttpCode: 200, InputTokens: 10, OutputTokens: 20},
		{Platform: "claude", Model: "claude-opus-4", Provider: "alpha", SessionID: "s-1", HttpCode: 200, InputTokens: 30, OutputTokens: 40},
		{Platform: "claude", Model: "claude-sonnet-4", Provider: "beta", SessionID: "s-2", HttpCode: 200, InputTokens: 5, OutputTokens: 6},
		{Platform: "codex", Model: "gpt-5", Provider: "ice", SessionID: "c-1", HttpCode: 200, InputTokens: 7, OutputTokens: 8},
		// 无会话 ID 的行（历史数据）不参与聚合。
		{Platform: "claude", Model: "claude-sonnet-4", Provider: "alpha", HttpCode: 200, InputTokens: 100, OutputTokens: 100},
	})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}

	selected, err := NewSQLiteQueries().SessionUsageForSessions("claude", []string{"s-1", "s-1", "", "c-1", "missing"})
	if err != nil || len(selected) != 1 || selected["s-1"].Requests != 2 {
		t.Fatalf("must include only requested platform/session pairs without double counting: %+v, %v", selected, err)
	}
	// Cross a parameter batch boundary; the requested session in the last batch
	// must still be returned and a repeated ID must not double-count usage.
	ids := make([]string, 0, 1003)
	for i := 0; i < 1000; i++ {
		ids = append(ids, fmt.Sprintf("missing-%d", i))
	}
	ids = append(ids, "s-1", "s-2", "s-1")
	batched, err := NewSQLiteQueries().SessionUsageForSessions("claude", ids)
	if err != nil || len(batched) != 2 || batched["s-1"].Requests != 2 || batched["s-2"].Requests != 1 {
		t.Fatalf("batched session usage: %+v, %v", batched, err)
	}
	usage, err := NewSQLiteQueries().SessionUsageForSessions("claude", []string{"s-1", "s-2"})
	if err != nil {
		t.Fatalf("SessionUsageForSessions: %v", err)
	}
	if len(usage) != 2 {
		t.Fatalf("expected 2 claude sessions, got %d", len(usage))
	}

	s1 := usage["s-1"]
	if s1.Requests != 2 {
		t.Fatalf("expected s-1 requests 2, got %d", s1.Requests)
	}
	if s1.InputTokens != 40 || s1.OutputTokens != 60 {
		t.Fatalf("expected s-1 tokens 40/60, got %d/%d", s1.InputTokens, s1.OutputTokens)
	}
	if s1.TotalCost <= 0 {
		t.Fatalf("expected s-1 total cost > 0, got %f", s1.TotalCost)
	}
	if len(s1.Models) != 2 {
		t.Fatalf("expected s-1 to have 2 distinct models, got %v", s1.Models)
	}

	logs, err := NewSQLiteQueries().ListSessionRequestLogs("claude", "s-1")
	if err != nil {
		t.Fatalf("ListSessionRequestLogs: %v", err)
	}
	if len(logs) != 2 || logs[0].SessionID != "s-1" || logs[1].SessionID != "s-1" {
		t.Fatalf("expected two s-1 logs with session IDs preserved, got %+v", logs)
	}
	if logs[0].Model != "claude-opus-4" || logs[1].Model != "claude-sonnet-4" {
		t.Fatalf("expected newest session request first, got %q then %q", logs[0].Model, logs[1].Model)
	}
	otherPlatform, err := NewSQLiteQueries().ListSessionRequestLogs("codex", "s-1")
	if err != nil || len(otherPlatform) != 0 {
		t.Fatalf("session query must be platform scoped: logs=%+v err=%v", otherPlatform, err)
	}

	if usage["s-2"].Requests != 1 {
		t.Fatalf("expected s-2 requests 1, got %d", usage["s-2"].Requests)
	}

	codexUsage, err := NewSQLiteQueries().SessionUsageForSessions("codex", []string{"c-1", "s-1"})
	if err != nil {
		t.Fatalf("SessionUsageForSessions codex: %v", err)
	}
	if len(codexUsage) != 1 || codexUsage["c-1"].Requests != 1 {
		t.Fatalf("expected 1 codex session with 1 request, got %+v", codexUsage)
	}
}

func TestSessionUsageForSessionsEmptyInputDoesNotAccessDB(t *testing.T) {
	// The preceding tests may have left a closed registry connection. Empty IDs
	// must succeed regardless of database availability.
	for _, ids := range [][]string{nil, {}, {"", ""}} {
		result, err := (&SQLiteQueries{}).SessionUsageForSessions("claude", ids)
		if err != nil || len(result) != 0 {
			t.Fatalf("empty query: %+v, %v", result, err)
		}
	}
}
