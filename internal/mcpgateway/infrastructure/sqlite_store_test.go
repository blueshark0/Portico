package infrastructure

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"portico/internal/mcpgateway/domain"
	"portico/internal/shared/storage"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()

	dbName := "mcpgateway_test_" + strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	dbPath := filepath.Join(t.TempDir(), "app.db")

	if err := storage.OpenConnection(storage.ConnectionConfig{
		Name:   dbName,
		Driver: "sqlite",
		DSN:    dbPath,
	}); err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = storage.CloseConnection(dbName) })

	store := NewSQLiteStoreWithDBName(dbName)
	if err := store.EnsureSchema(); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	return store
}

func TestEnsureSchemaIdempotent(t *testing.T) {
	store := newTestStore(t)
	if err := store.EnsureSchema(); err != nil {
		t.Fatalf("二次 EnsureSchema: %v", err)
	}
}

func TestCRUDRoundTrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	srv := domain.McpServer{
		Name:    "fs",
		Type:    domain.ServerTypeStdio,
		Command: "npx",
		Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"},
		Env:     map[string]string{"DEBUG": "1"},
		Enabled: true, Position: 2, TimeoutSec: 45,
	}
	if err := store.Create(ctx, &srv); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if srv.ID == 0 || srv.CreatedAt.IsZero() {
		t.Fatalf("Create 应回写 ID/时间戳: %+v", srv)
	}

	remote := domain.McpServer{
		Name: "remote", Type: domain.ServerTypeHTTP,
		URL: "https://example.com/mcp", Headers: map[string]string{"Authorization": "Bearer x"},
		Enabled: false, Position: 1,
	}
	if err := store.Create(ctx, &remote); err != nil {
		t.Fatalf("Create remote: %v", err)
	}

	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("List 数量: %d", len(list))
	}
	// 按 position 排序：remote(1) 在 fs(2) 前
	if list[0].Name != "remote" || list[1].Name != "fs" {
		t.Fatalf("排序错误: %+v", list)
	}
	got := list[1]
	if got.Command != "npx" || len(got.Args) != 3 || got.Env["DEBUG"] != "1" ||
		!got.Enabled || got.TimeoutSec != 45 {
		t.Fatalf("stdio 往返失真: %+v", got)
	}
	if list[0].Headers["Authorization"] != "Bearer x" || list[0].Enabled {
		t.Fatalf("http 往返失真: %+v", list[0])
	}

	// Update
	got.Name = "fs2"
	got.Enabled = false
	if err := store.Update(ctx, got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	// SetEnabled
	if err := store.SetEnabled(ctx, remote.ID, true); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	list, err = store.List(ctx)
	if err != nil {
		t.Fatalf("List2: %v", err)
	}
	byID := map[int64]domain.McpServer{}
	for _, s := range list {
		byID[s.ID] = s
	}
	if byID[got.ID].Name != "fs2" || byID[got.ID].Enabled {
		t.Fatalf("Update 未生效: %+v", byID[got.ID])
	}
	if !byID[remote.ID].Enabled {
		t.Fatalf("SetEnabled 未生效: %+v", byID[remote.ID])
	}

	// Delete
	if err := store.Delete(ctx, got.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	list, _ = store.List(ctx)
	if len(list) != 1 {
		t.Fatalf("Delete 后数量: %d", len(list))
	}
}

func TestNameUniqueConstraint(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	srv := domain.McpServer{Name: "dup", Type: domain.ServerTypeStdio, Command: "x"}
	if err := store.Create(ctx, &srv); err != nil {
		t.Fatalf("Create: %v", err)
	}
	dup := domain.McpServer{Name: "dup", Type: domain.ServerTypeStdio, Command: "y"}
	if err := store.Create(ctx, &dup); err == nil {
		t.Fatal("重名应报 UNIQUE 冲突")
	}
}

func TestNullJSONDefaults(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	// nil Args/Env/Headers 应落库为默认值且读回不报错
	srv := domain.McpServer{Name: "bare", Type: domain.ServerTypeHTTP, URL: "http://x/mcp"}
	if err := store.Create(ctx, &srv); err != nil {
		t.Fatalf("Create: %v", err)
	}
	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Name != "bare" {
		t.Fatalf("往返失真: %+v", list)
	}
}
