package infrastructure

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

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

func TestOAuthFieldsRoundTrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	srv := domain.McpServer{
		Name: "remote", Type: domain.ServerTypeHTTP, URL: "https://example.com/mcp",
		AuthType: domain.AuthTypeOAuth, OAuthClientName: "Mine", OAuthClientID: "cid", OAuthClientSecret: "sec",
		Enabled: true,
	}
	if err := store.Create(ctx, &srv); err != nil {
		t.Fatalf("Create: %v", err)
	}
	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	got := list[0]
	if got.AuthType != domain.AuthTypeOAuth || got.OAuthClientName != "Mine" ||
		got.OAuthClientID != "cid" || got.OAuthClientSecret != "sec" {
		t.Fatalf("OAuth 字段往返失真: %+v", got)
	}

	got.AuthType = domain.AuthTypeNone
	got.OAuthClientName, got.OAuthClientID, got.OAuthClientSecret = "", "", ""
	if err := store.Update(ctx, got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	list, _ = store.List(ctx)
	if list[0].AuthType != domain.AuthTypeNone || list[0].OAuthClientID != "" {
		t.Fatalf("Update 未清空 OAuth 字段: %+v", list[0])
	}
}

// 老版本建的表没有 OAuth 列：EnsureSchema 补列后既有数据照常可读。
func TestEnsureSchemaUpgradesLegacyTable(t *testing.T) {
	dbName := "mcpgateway_test_" + strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	if err := storage.OpenConnection(storage.ConnectionConfig{
		Name: dbName, Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "app.db"),
	}); err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = storage.CloseConnection(dbName) })
	db, err := storage.DB(dbName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE mcp_servers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		type TEXT NOT NULL DEFAULT 'stdio',
		command TEXT NOT NULL DEFAULT '',
		args_json TEXT NOT NULL DEFAULT '[]',
		env_json TEXT NOT NULL DEFAULT '{}',
		url TEXT NOT NULL DEFAULT '',
		headers_json TEXT NOT NULL DEFAULT '{}',
		enabled INTEGER NOT NULL DEFAULT 1,
		position INTEGER NOT NULL DEFAULT 0,
		timeout_sec INTEGER NOT NULL DEFAULT 30,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO mcp_servers (name, type, url) VALUES ('legacy', 'http', 'https://example.com/mcp')`); err != nil {
		t.Fatal(err)
	}

	store := NewSQLiteStoreWithDBName(dbName)
	if err := store.EnsureSchema(); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	list, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Name != "legacy" || list[0].AuthType != domain.AuthTypeNone {
		t.Fatalf("升级后既有数据异常: %+v", list)
	}
}

func TestOAuthCredentialCRUD(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if cred, err := store.GetOAuthCredential(ctx, 1); err != nil || cred != nil {
		t.Fatalf("不存在时应返回 (nil, nil)，实际 %+v, %v", cred, err)
	}

	expiry := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	want := domain.OAuthCredential{
		ServerID: 1, ClientID: "cid", ClientSecret: "sec", AuthStyle: 1,
		AuthURL: "https://as/authorize", TokenURL: "https://as/token", RedirectURL: "http://127.0.0.1:18100/mcp/oauth/callback",
		Scopes: []string{"read", "write"}, AccessToken: "at", RefreshToken: "rt", TokenType: "Bearer", Expiry: expiry,
	}
	if err := store.SaveOAuthCredential(ctx, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := store.GetOAuthCredential(ctx, 1)
	if err != nil || got == nil {
		t.Fatalf("Get: %+v, %v", got, err)
	}
	if !reflect.DeepEqual(*got, want) {
		t.Fatalf("凭证往返失真:\n got %+v\nwant %+v", *got, want)
	}

	// 覆盖写：刷新后的令牌替换旧值；零值有效期往返后仍是零值。
	want.AccessToken, want.RefreshToken, want.Expiry, want.Scopes = "at2", "rt2", time.Time{}, nil
	if err := store.SaveOAuthCredential(ctx, want); err != nil {
		t.Fatalf("Save 覆盖: %v", err)
	}
	got, _ = store.GetOAuthCredential(ctx, 1)
	if got.AccessToken != "at2" || got.RefreshToken != "rt2" || !got.Expiry.IsZero() || len(got.Scopes) != 0 {
		t.Fatalf("覆盖写未生效: %+v", *got)
	}

	if err := store.DeleteOAuthCredential(ctx, 1); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := store.DeleteOAuthCredential(ctx, 1); err != nil {
		t.Fatalf("重复 Delete 应幂等: %v", err)
	}
	if cred, _ := store.GetOAuthCredential(ctx, 1); cred != nil {
		t.Fatalf("Delete 后仍可读到凭证: %+v", cred)
	}
}

// 删除服务必须连带删除它的令牌，不留孤儿凭证。
func TestDeleteServerRemovesOAuthCredential(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	keep := domain.McpServer{Name: "keep", Type: domain.ServerTypeHTTP, URL: "https://a/mcp", AuthType: domain.AuthTypeOAuth}
	drop := domain.McpServer{Name: "drop", Type: domain.ServerTypeHTTP, URL: "https://b/mcp", AuthType: domain.AuthTypeOAuth}
	for _, srv := range []*domain.McpServer{&keep, &drop} {
		if err := store.Create(ctx, srv); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := store.SaveOAuthCredential(ctx, domain.OAuthCredential{ServerID: srv.ID, AccessToken: "at-" + srv.Name}); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	if err := store.Delete(ctx, drop.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if cred, _ := store.GetOAuthCredential(ctx, drop.ID); cred != nil {
		t.Fatalf("被删服务的凭证应一并删除: %+v", cred)
	}
	if cred, _ := store.GetOAuthCredential(ctx, keep.ID); cred == nil || cred.AccessToken != "at-keep" {
		t.Fatalf("其他服务的凭证不应受影响: %+v", cred)
	}
}
