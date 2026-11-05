package relay

import (
	"testing"
	"time"

	observabilitydomain "portico/internal/observability/domain"
)

func TestEnqueueRequestLogStampsCompletionTime(t *testing.T) {
	server := &Server{}
	entry := &observabilitydomain.RequestLog{}
	before := time.Now().UTC().Add(-time.Second)
	server.enqueueRequestLog(entry)
	if entry.CreatedAt == "" {
		t.Fatal("入队时必须写入 CreatedAt，不能留给延迟批量落盘补时间")
	}
	parsed, err := time.ParseInLocation(time.DateTime, entry.CreatedAt, time.UTC)
	if err != nil {
		t.Fatalf("CreatedAt = %q 不是 UTC time.DateTime: %v", entry.CreatedAt, err)
	}
	if parsed.Before(before) || parsed.After(time.Now().UTC().Add(time.Second)) {
		t.Fatalf("CreatedAt = %v 不在入队时间附近", parsed)
	}
}

func TestEnqueueRequestLogPreservesExistingTimestamp(t *testing.T) {
	server := &Server{}
	const createdAt = "2026-01-02 03:04:05"
	entry := &observabilitydomain.RequestLog{CreatedAt: createdAt}
	server.enqueueRequestLog(entry)
	if entry.CreatedAt != createdAt {
		t.Fatalf("CreatedAt = %q, want preserved %q", entry.CreatedAt, createdAt)
	}
}
