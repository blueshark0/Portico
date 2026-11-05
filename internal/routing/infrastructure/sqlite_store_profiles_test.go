package infrastructure

import (
	"context"
	"path/filepath"
	"testing"

	"portico/internal/routing/domain"
	"portico/internal/shared/kernel"
)

func TestSQLiteStoreSaveProfilePersistsRequestBodyReplacements(t *testing.T) {
	dbName := "routing_test_" + t.Name()
	dbPath := filepath.Join(t.TempDir(), "app.db")

	openRoutingTestDB(t, dbName, dbPath)

	store := NewSQLiteStoreWithDBName(dbName)
	if err := store.EnsureSchema(); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}

	saved, err := store.SaveProfile(context.Background(), domain.RouteProfile{
		Platform:          kernel.PlatformClaude,
		DefaultProviderID: intPtr(1),
		Providers: []domain.Provider{
			{
				ID:       1,
				Name:     "alpha",
				APIURL:   "https://alpha.example",
				APIKey:   "secret",
				Position: 1,
				RequestBodyReplacements: []domain.RequestBodyReplacement{
					{Path: "metadata.user_id", Value: `"fixed-user"`},
					{Path: "temperature", Value: `0.2`},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	if got := saved.Providers[0].RequestBodyReplacements; len(got) != 2 {
		t.Fatalf("expected saved replacements, got %+v", got)
	}

	loaded, err := store.GetProfile(context.Background(), kernel.PlatformClaude)
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	got := loaded.Providers[0].RequestBodyReplacements
	if len(got) != 2 {
		t.Fatalf("expected 2 replacements, got %+v", got)
	}
	if got[0].Path != "metadata.user_id" || got[0].Value != `"fixed-user"` {
		t.Fatalf("unexpected first replacement: %+v", got[0])
	}
	if got[1].Path != "temperature" || got[1].Value != `0.2` {
		t.Fatalf("unexpected second replacement: %+v", got[1])
	}
}

func intPtr(value int) *int {
	return &value
}

func TestSQLiteStoreSaveProfilePersistsArchivedFlag(t *testing.T) {
	dbName := "routing_test_" + t.Name()
	dbPath := filepath.Join(t.TempDir(), "app.db")

	openRoutingTestDB(t, dbName, dbPath)

	store := NewSQLiteStoreWithDBName(dbName)
	if err := store.EnsureSchema(); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}

	profile := domain.RouteProfile{
		Platform:          kernel.PlatformClaude,
		DefaultProviderID: intPtr(1),
		Providers: []domain.Provider{
			{ID: 1, Name: "alpha", APIURL: "https://alpha.example", APIKey: "secret", Position: 1},
			{ID: 2, Name: "beta", APIURL: "https://beta.example", APIKey: "secret", Position: 2},
		},
	}
	if _, err := store.SaveProfile(context.Background(), profile); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	loaded, err := store.GetProfile(context.Background(), kernel.PlatformClaude)
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	loaded.Providers[1].Archived = true
	archived, err := store.SaveProfile(context.Background(), loaded)
	if err != nil {
		t.Fatalf("SaveProfile archive: %v", err)
	}
	if archived.Providers[0].Archived {
		t.Fatalf("provider alpha should not be archived, got %+v", archived.Providers[0])
	}
	if !archived.Providers[1].Archived {
		t.Fatalf("provider beta should stay archived, got %+v", archived.Providers[1])
	}
	firstArchivedAt := archived.Providers[1].ArchivedAt
	if firstArchivedAt == nil {
		t.Fatal("expected archiving to record a timestamp")
	}

	// 编辑归档配置不能刷新归档时间。
	archived.Providers[1].APIURL = "https://beta-updated.example"
	resaved, err := store.SaveProfile(context.Background(), archived)
	if err != nil {
		t.Fatalf("SaveProfile archived edit: %v", err)
	}
	if resaved.Providers[1].ArchivedAt == nil || !resaved.Providers[1].ArchivedAt.Equal(*firstArchivedAt) {
		t.Fatalf("expected archived timestamp to stay %v, got %v", firstArchivedAt, resaved.Providers[1].ArchivedAt)
	}

	resaved.Providers[1].Archived = false
	restored, err := store.SaveProfile(context.Background(), resaved)
	if err != nil {
		t.Fatalf("SaveProfile restore: %v", err)
	}
	if restored.Providers[1].Archived {
		t.Fatalf("provider beta should be restored, got %+v", restored.Providers[1])
	}
	if restored.Providers[1].ArchivedAt != nil {
		t.Fatalf("expected restore to clear archived timestamp, got %v", restored.Providers[1].ArchivedAt)
	}

	restored.Providers[1].Archived = true
	rearchived, err := store.SaveProfile(context.Background(), restored)
	if err != nil {
		t.Fatalf("SaveProfile re-archive: %v", err)
	}
	if rearchived.Providers[1].ArchivedAt == nil {
		t.Fatal("expected re-archiving to record a new timestamp")
	}
}
