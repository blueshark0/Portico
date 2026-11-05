package infrastructure

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestAppLog(t *testing.T, content string) (*AppLogReader, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.log")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return NewAppLogReader(path), path
}

func appendToFile(t *testing.T, path string, content string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
}

func TestAppLogReaderParsesEntriesAndLevels(t *testing.T) {
	reader, _ := newTestAppLog(t,
		"2026/07/08 18:27:22 logging.go:23: [WARN] [relay] 请求失败: provider=anyrouter\n"+
			"2026/07/08 18:27:23 service.go:105: provider relay server listening on :18100\n"+
			"2026/07/08 18:27:24 cleanup_runner.go:43: [ERROR] cleanup failed\n")

	chunk, err := reader.Tail(0, 0, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(chunk.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(chunk.Entries))
	}
	first := chunk.Entries[0]
	if first.Time != "2026/07/08 18:27:22" || first.Source != "logging.go:23" || first.Level != "WARN" {
		t.Fatalf("unexpected first entry: %+v", first)
	}
	if first.Message != "[relay] 请求失败: provider=anyrouter" {
		t.Fatalf("level tag not stripped: %q", first.Message)
	}
	if chunk.Entries[1].Level != "" {
		t.Fatalf("expected empty level for untagged line, got %q", chunk.Entries[1].Level)
	}
	if chunk.Entries[2].Level != "ERROR" {
		t.Fatalf("expected ERROR level, got %q", chunk.Entries[2].Level)
	}
	if chunk.Truncated || chunk.Reset {
		t.Fatalf("unexpected flags: %+v", chunk)
	}
}

func TestAppLogReaderMergesContinuationLines(t *testing.T) {
	reader, _ := newTestAppLog(t,
		"2026/07/08 18:27:22 http.go:10: [WARN] upstream error {\n"+
			"  \"error\": \"forbidden\"\n"+
			"} duration=1.34s\n"+
			"2026/07/08 18:27:25 http.go:10: [INFO] ok\n")

	chunk, err := reader.Tail(0, 0, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(chunk.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(chunk.Entries))
	}
	msg := chunk.Entries[0].Message
	if !strings.Contains(msg, "\n  \"error\": \"forbidden\"\n} duration=1.34s") {
		t.Fatalf("continuation lines not merged: %q", msg)
	}
}

func TestAppLogReaderLevelFilter(t *testing.T) {
	content := "2026/07/08 18:27:22 a.go:1: [WARN] warn line\n" +
		"2026/07/08 18:27:23 b.go:2: untagged line\n" +
		"2026/07/08 18:27:24 c.go:3: [ERROR] error line\n"

	reader, _ := newTestAppLog(t, content)

	chunk, err := reader.Tail(0, 0, "ERROR", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(chunk.Entries) != 1 || chunk.Entries[0].Level != "ERROR" {
		t.Fatalf("ERROR filter failed: %+v", chunk.Entries)
	}

	chunk, err = reader.Tail(0, 0, "none", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(chunk.Entries) != 1 || chunk.Entries[0].Message != "untagged line" {
		t.Fatalf("none filter failed: %+v", chunk.Entries)
	}
}

func TestAppLogReaderKeywordFilterIsCaseInsensitiveAndCoversContinuation(t *testing.T) {
	reader, _ := newTestAppLog(t,
		"2026/07/08 18:27:22 a.go:1: [WARN] upstream error {\n"+
			"  \"type\": \"Forbidden\"\n"+
			"}\n"+
			"2026/07/08 18:27:23 b.go:2: [WARN] other\n")

	chunk, err := reader.Tail(0, 0, "", "forbidden")
	if err != nil {
		t.Fatal(err)
	}
	if len(chunk.Entries) != 1 || !strings.Contains(chunk.Entries[0].Message, "Forbidden") {
		t.Fatalf("keyword filter failed: %+v", chunk.Entries)
	}
}

func TestAppLogReaderMaxLinesKeepsTail(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 10; i++ {
		sb.WriteString("2026/07/08 18:27:2")
		sb.WriteByte(byte('0' + i%10))
		sb.WriteString(" a.go:1: line\n")
	}
	reader, _ := newTestAppLog(t, sb.String())

	chunk, err := reader.Tail(0, 3, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(chunk.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(chunk.Entries))
	}
	if chunk.Entries[2].Time != "2026/07/08 18:27:29" {
		t.Fatalf("expected newest entries kept, got %+v", chunk.Entries)
	}
	if !chunk.Truncated {
		t.Fatal("expected Truncated when maxLines trims entries")
	}
}

func TestAppLogReaderIncrementalOffset(t *testing.T) {
	reader, path := newTestAppLog(t,
		"2026/07/08 18:27:22 a.go:1: [INFO] first\n")

	chunk, err := reader.Tail(0, 0, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(chunk.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(chunk.Entries))
	}

	// 无新内容时 NextOffset 不变、无条目。
	again, err := reader.Tail(chunk.NextOffset, 0, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Entries) != 0 || again.NextOffset != chunk.NextOffset || again.Reset {
		t.Fatalf("expected no-op follow, got %+v", again)
	}

	appendToFile(t, path, "2026/07/08 18:27:30 b.go:2: [WARN] second\n")
	follow, err := reader.Tail(chunk.NextOffset, 0, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(follow.Entries) != 1 || follow.Entries[0].Message != "second" {
		t.Fatalf("expected only new entry, got %+v", follow.Entries)
	}
	if follow.Reset {
		t.Fatal("unexpected Reset on normal follow")
	}
}

func TestAppLogReaderDoesNotConsumeUnfinishedLine(t *testing.T) {
	reader, path := newTestAppLog(t,
		"2026/07/08 18:27:22 a.go:1: [INFO] complete\n"+
			"2026/07/08 18:27:23 b.go:2: [WARN] partial without newline")

	chunk, err := reader.Tail(0, 0, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(chunk.Entries) != 1 || chunk.Entries[0].Message != "complete" {
		t.Fatalf("partial line should not be consumed: %+v", chunk.Entries)
	}

	appendToFile(t, path, " now done\n")
	follow, err := reader.Tail(chunk.NextOffset, 0, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(follow.Entries) != 1 || follow.Entries[0].Message != "partial without newline now done" {
		t.Fatalf("expected completed line on next poll, got %+v", follow.Entries)
	}
}

func TestAppLogReaderResetOnRotation(t *testing.T) {
	reader, path := newTestAppLog(t,
		"2026/07/08 18:27:22 a.go:1: [INFO] old content that is fairly long\n"+
			"2026/07/08 18:27:23 a.go:1: [INFO] old content line two\n")

	chunk, err := reader.Tail(0, 0, "", "")
	if err != nil {
		t.Fatal(err)
	}

	// 模拟 lumberjack 轮转：文件被替换成更小的新文件。
	if err := os.WriteFile(path, []byte("2026/07/08 18:30:00 a.go:1: [INFO] fresh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	follow, err := reader.Tail(chunk.NextOffset, 0, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !follow.Reset {
		t.Fatal("expected Reset after rotation")
	}
	if len(follow.Entries) != 1 || follow.Entries[0].Message != "fresh" {
		t.Fatalf("expected fresh content after rotation, got %+v", follow.Entries)
	}
}

func TestAppLogReaderResetWhenBacklogExceedsFollowLimit(t *testing.T) {
	reader, path := newTestAppLog(t, "2026/07/08 18:27:22 a.go:1: [INFO] first\n")
	reader.followLimit = 16

	chunk, err := reader.Tail(0, 0, "", "")
	if err != nil {
		t.Fatal(err)
	}
	appendToFile(t, path,
		"2026/07/08 18:27:23 a.go:1: [INFO] a much longer backlog line exceeding the follow limit\n")
	follow, err := reader.Tail(chunk.NextOffset, 0, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !follow.Reset {
		t.Fatal("expected Reset when backlog exceeds follow limit")
	}
	if len(follow.Entries) == 0 {
		t.Fatal("expected tail window entries after reset")
	}
}

func TestAppLogReaderScanWindowTruncatesAndKeepsOrphanContinuation(t *testing.T) {
	content := "2026/07/08 18:27:22 a.go:1: [WARN] head {\n" +
		"orphan continuation body\n" +
		"2026/07/08 18:27:23 b.go:2: [INFO] tail entry\n"
	reader, _ := newTestAppLog(t, content)
	// 窗口只覆盖到第一行中间：残行被丢弃后，续行成为 Time=="" 的裸条目。
	reader.scanBytes = int64(len(content)) - 20

	chunk, err := reader.Tail(0, 0, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !chunk.Truncated {
		t.Fatal("expected Truncated when window smaller than file")
	}
	if len(chunk.Entries) != 2 {
		t.Fatalf("expected orphan + tail entries, got %+v", chunk.Entries)
	}
	if chunk.Entries[0].Time != "" || chunk.Entries[0].Message != "orphan continuation body" {
		t.Fatalf("orphan continuation mishandled: %+v", chunk.Entries[0])
	}
}

func TestAppLogReaderMissingFileReturnsEmpty(t *testing.T) {
	reader := NewAppLogReader(filepath.Join(t.TempDir(), "does-not-exist.log"))
	chunk, err := reader.Tail(0, 0, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(chunk.Entries) != 0 || chunk.FileSize != 0 || chunk.NextOffset != 0 {
		t.Fatalf("expected empty chunk, got %+v", chunk)
	}
}
