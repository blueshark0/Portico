package infrastructure

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"portico/internal/shared/kernel"
)

func newLegacyImporterTestStore(t *testing.T) (*SQLiteStore, *sql.DB) {
	t.Helper()

	dbName := "routing_test_" + t.Name()
	dbPath := filepath.Join(t.TempDir(), "app.db")

	db := openRoutingTestDB(t, dbName, dbPath)

	store := NewSQLiteStoreWithDBName(dbName)
	if err := store.EnsureSchema(); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	return store, db
}

func writeLegacyFixture(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture %s: %v", name, err)
	}
}

const legacyClaudeFixture = `{
	"providers": [
		{
			"id": 1,
			"name": "Alpha",
			"apiUrl": "https://alpha.example/api",
			"apiKey": "sk-alpha",
			"officialSite": "https://alpha.example",
			"icon": "alpha.png",
			"tint": "#112233",
			"accent": "#445566",
			"enabled": true,
			"position": 5,
			"supportedModels": {"claude-sonnet-4.5": true},
			"modelMapping": {"claude-*": "alpha/claude-*"}
		},
		{
			"id": 2,
			"name": "Beta",
			"apiUrl": "https://beta.example/api",
			"apiKey": "sk-beta",
			"enabled": false,
			"position": 0
		}
	]
}`

const legacyCodexFixture = `{
	"providers": [
		{"id": 7, "name": "Gamma", "apiUrl": "https://gamma.example/api", "apiKey": "sk-gamma", "enabled": true, "position": 1}
	]
}`

const legacyAppFixture = `{
	"show_heatmap": false,
	"show_home_title": true,
	"default_claude_provider": "Beta",
	"default_codex_provider": "Missing"
}`

func TestLegacyImporterEnsureImported_MigratesLegacyFiles(t *testing.T) {
	store, db := newLegacyImporterTestStore(t)
	legacyDir := t.TempDir()
	ctx := context.Background()

	writeLegacyFixture(t, legacyDir, "claude-code.json", legacyClaudeFixture)
	writeLegacyFixture(t, legacyDir, "codex.json", legacyCodexFixture)
	writeLegacyFixture(t, legacyDir, "app.json", legacyAppFixture)

	if err := NewLegacyImporterWithDir(store, legacyDir).EnsureImported(ctx); err != nil {
		t.Fatalf("EnsureImported: %v", err)
	}

	imported, err := store.HasLegacyImport(ctx)
	if err != nil {
		t.Fatalf("HasLegacyImport: %v", err)
	}
	if !imported {
		t.Fatal("expected legacy import to be marked done")
	}

	// Claude 平台：两个 provider，默认供应商按名字 "Beta" 回填为其 ID。
	claude, err := store.GetProfile(ctx, kernel.PlatformClaude)
	if err != nil {
		t.Fatalf("GetProfile claude: %v", err)
	}
	if claude.DefaultProviderID == nil || *claude.DefaultProviderID != 2 {
		t.Fatalf("expected default claude provider id 2 (Beta), got %v", claude.DefaultProviderID)
	}
	if len(claude.Providers) != 2 {
		t.Fatalf("expected 2 claude providers, got %d", len(claude.Providers))
	}
	// Beta 旧 position=0 → 兜底 idx+1=2；Normalize 后排在 Alpha(5) 之前并重排为 1。
	beta := claude.Providers[0]
	if beta.ID != 2 || beta.Name != "Beta" || beta.Position != 1 {
		t.Fatalf("unexpected first claude provider: %+v", beta)
	}
	if beta.APIURL != "https://beta.example/api" || beta.APIKey != "sk-beta" {
		t.Fatalf("unexpected Beta connection fields: %+v", beta)
	}
	if len(beta.SupportedModels) != 0 || len(beta.ModelMapping) != 0 {
		t.Fatalf("expected Beta to have empty model config, got %+v", beta)
	}
	alpha := claude.Providers[1]
	if alpha.ID != 1 || alpha.Name != "Alpha" || alpha.Position != 2 {
		t.Fatalf("unexpected second claude provider: %+v", alpha)
	}
	if alpha.APIURL != "https://alpha.example/api" || alpha.APIKey != "sk-alpha" ||
		alpha.OfficialSite != "https://alpha.example" ||
		alpha.Tint != "#112233" || alpha.Accent != "#445566" {
		t.Fatalf("unexpected Alpha fields: %+v", alpha)
	}
	if len(alpha.SupportedModels) != 1 || !alpha.SupportedModels["claude-sonnet-4.5"] {
		t.Fatalf("unexpected Alpha supported models: %+v", alpha.SupportedModels)
	}
	if len(alpha.ModelMapping) != 1 || alpha.ModelMapping["claude-*"] != "alpha/claude-*" {
		t.Fatalf("unexpected Alpha model mapping: %+v", alpha.ModelMapping)
	}

	// 迁移语义锁定：旧 enabled 字段被丢弃，落库列保持默认 0。
	var enabledCount int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM providers WHERE platform = 'claude' AND enabled != 0",
	).Scan(&enabledCount); err != nil {
		t.Fatalf("query enabled column: %v", err)
	}
	if enabledCount != 0 {
		t.Fatalf("expected legacy enabled flag to be discarded, got %d enabled rows", enabledCount)
	}

	// Codex 平台：默认供应商名 "Missing" 不存在 → 不设置默认 ID。
	codex, err := store.GetProfile(ctx, kernel.PlatformCodex)
	if err != nil {
		t.Fatalf("GetProfile codex: %v", err)
	}
	if len(codex.Providers) != 1 || codex.Providers[0].ID != 7 || codex.Providers[0].Name != "Gamma" {
		t.Fatalf("unexpected codex providers: %+v", codex.Providers)
	}
	if codex.DefaultProviderID != nil {
		t.Fatalf("expected no default codex provider, got %v", *codex.DefaultProviderID)
	}

	// gemini.json 不存在 → 平台跳过。
	gemini, err := store.GetProfile(ctx, kernel.PlatformGemini)
	if err != nil {
		t.Fatalf("GetProfile gemini: %v", err)
	}
	if len(gemini.Providers) != 0 {
		t.Fatalf("expected no gemini providers, got %+v", gemini.Providers)
	}

	prefs, err := store.GetAppPreferences(ctx)
	if err != nil {
		t.Fatalf("GetAppPreferences: %v", err)
	}
	if prefs.ShowHeatmap || !prefs.ShowHomeTitle {
		t.Fatalf("expected app preferences show_heatmap=false show_home_title=true, got %+v", prefs)
	}

	// 备份目录：单个时间戳目录，含且仅含存在过的 3 个旧文件，内容与原文件一致。
	backupRoot := filepath.Join(legacyDir, "legacy-backup")
	entries, err := os.ReadDir(backupRoot)
	if err != nil {
		t.Fatalf("read backup root: %v", err)
	}
	if len(entries) != 1 || !entries[0].IsDir() {
		t.Fatalf("expected exactly one backup dir, got %+v", entries)
	}
	backupDir := filepath.Join(backupRoot, entries[0].Name())
	backedUp, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("read backup dir: %v", err)
	}
	if len(backedUp) != 3 {
		t.Fatalf("expected 3 backed up files, got %d", len(backedUp))
	}
	for name, want := range map[string]string{
		"claude-code.json": legacyClaudeFixture,
		"codex.json":       legacyCodexFixture,
		"app.json":         legacyAppFixture,
	} {
		data, err := os.ReadFile(filepath.Join(backupDir, name))
		if err != nil {
			t.Fatalf("read backup %s: %v", name, err)
		}
		if string(data) != want {
			t.Fatalf("backup %s content mismatch", name)
		}
	}
}

func TestLegacyImporterEnsureImported_SecondCallIsNoOp(t *testing.T) {
	store, _ := newLegacyImporterTestStore(t)
	legacyDir := t.TempDir()
	ctx := context.Background()

	original := `{"providers":[{"id":1,"name":"Alpha","apiUrl":"https://alpha.example/api","apiKey":"sk-alpha","position":1}]}`
	writeLegacyFixture(t, legacyDir, "claude-code.json", original)

	importer := NewLegacyImporterWithDir(store, legacyDir)
	if err := importer.EnsureImported(ctx); err != nil {
		t.Fatalf("first EnsureImported: %v", err)
	}

	// 篡改旧文件后再次调用：不得重新导入，也不得再新建备份。
	writeLegacyFixture(t, legacyDir, "claude-code.json",
		`{"providers":[{"id":9,"name":"Intruder","apiUrl":"https://intruder.example","apiKey":"sk-intruder","position":1}]}`)
	if err := importer.EnsureImported(ctx); err != nil {
		t.Fatalf("second EnsureImported: %v", err)
	}

	claude, err := store.GetProfile(ctx, kernel.PlatformClaude)
	if err != nil {
		t.Fatalf("GetProfile claude: %v", err)
	}
	if len(claude.Providers) != 1 || claude.Providers[0].ID != 1 || claude.Providers[0].Name != "Alpha" {
		t.Fatalf("expected second call to keep first import untouched, got %+v", claude.Providers)
	}

	backupRoot := filepath.Join(legacyDir, "legacy-backup")
	entries, err := os.ReadDir(backupRoot)
	if err != nil {
		t.Fatalf("read backup root: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly one backup dir after second call, got %d", len(entries))
	}
	data, err := os.ReadFile(filepath.Join(backupRoot, entries[0].Name(), "claude-code.json"))
	if err != nil {
		t.Fatalf("read backup claude-code.json: %v", err)
	}
	if string(data) != original {
		t.Fatalf("expected backup to keep original content, got %s", data)
	}
}

func TestLegacyImporterEnsureImported_NoLegacyFiles(t *testing.T) {
	store, _ := newLegacyImporterTestStore(t)
	legacyDir := t.TempDir()
	ctx := context.Background()

	if err := NewLegacyImporterWithDir(store, legacyDir).EnsureImported(ctx); err != nil {
		t.Fatalf("EnsureImported: %v", err)
	}

	// 现状行为：即使没有任何旧文件也会标记 imported（避免每次启动重复扫描）。
	imported, err := store.HasLegacyImport(ctx)
	if err != nil {
		t.Fatalf("HasLegacyImport: %v", err)
	}
	if !imported {
		t.Fatal("expected import marker to be set even without legacy files (current behavior)")
	}

	if _, err := os.Stat(filepath.Join(legacyDir, "legacy-backup")); !os.IsNotExist(err) {
		t.Fatalf("expected no backup dir, stat err = %v", err)
	}

	for _, platform := range kernel.AllPlatforms() {
		profile, err := store.GetProfile(ctx, platform)
		if err != nil {
			t.Fatalf("GetProfile %s: %v", platform, err)
		}
		if len(profile.Providers) != 0 {
			t.Fatalf("expected no providers for %s, got %+v", platform, profile.Providers)
		}
	}

	prefs, err := store.GetAppPreferences(ctx)
	if err != nil {
		t.Fatalf("GetAppPreferences: %v", err)
	}
	if !prefs.ShowHeatmap || !prefs.ShowHomeTitle {
		t.Fatalf("expected default app preferences, got %+v", prefs)
	}
}
