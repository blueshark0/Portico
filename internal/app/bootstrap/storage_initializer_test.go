package bootstrap

import (
	"testing"

	"portico/internal/shared/storage"
)

// Initialize 是应用启动的第一步，失败时 main 直接 log.Fatalf，窗口不会出现。
func TestInitializeOpensReadOnlyRequestLogPool(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Cleanup(func() { _ = storage.CloseConnections() })

	if err := NewInitializer().Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	writer, err := storage.DB(storage.RequestLogDBName)
	if err != nil {
		t.Fatalf("写入池未注册: %v", err)
	}
	if _, err := writer.Exec("INSERT INTO request_log(platform, created_at) VALUES ('claude', '2026-11-09 00:00:00')"); err != nil {
		t.Fatalf("写入池应可写: %v", err)
	}
	reader, err := storage.DB(storage.RequestLogReadDBName)
	if err != nil {
		t.Fatalf("只读池未注册: %v", err)
	}
	var count int
	if err := reader.QueryRow("SELECT COUNT(*) FROM request_log").Scan(&count); err != nil {
		t.Fatalf("只读池查询失败: %v", err)
	}
	if count != 1 {
		t.Fatalf("只读池应能读到写入池的数据，got %d", count)
	}
	if _, err := reader.Exec("DELETE FROM request_log"); err == nil {
		t.Fatal("只读池不应允许写入")
	}
}
