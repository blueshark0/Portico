package application

import (
	"context"
	"testing"
	"time"

	"portico/internal/routing/domain"
	"portico/internal/shared/kernel"
)

type routingRepoStub struct {
	getProfileCalls  int
	saveProfileCalls int
	profiles         map[kernel.Platform]domain.RouteProfile
}

func (r *routingRepoStub) GetProfile(_ context.Context, platform kernel.Platform) (domain.RouteProfile, error) {
	r.getProfileCalls++
	return r.profiles[platform], nil
}

func (r *routingRepoStub) SaveProfile(_ context.Context, profile domain.RouteProfile) (domain.RouteProfile, error) {
	r.saveProfileCalls++
	r.profiles[profile.Platform] = profile.Normalize()
	return r.profiles[profile.Platform], nil
}

func (r *routingRepoStub) GetAppPreferences(_ context.Context) (domain.AppPreferences, error) {
	return domain.DefaultAppPreferences(), nil
}

func (r *routingRepoStub) SaveAppPreferences(_ context.Context, preferences domain.AppPreferences) (domain.AppPreferences, error) {
	return preferences, nil
}

func (r *routingRepoStub) ReplaceAll(_ context.Context, _ domain.AppPreferences, profiles []domain.RouteProfile) error {
	for _, profile := range profiles {
		r.profiles[profile.Platform] = profile.Normalize()
	}
	return nil
}

func TestServiceGetProfileUsesCache(t *testing.T) {
	archivedAt := time.Date(2026, 10, 22, 14, 32, 0, 0, time.UTC)
	repo := &routingRepoStub{
		profiles: map[kernel.Platform]domain.RouteProfile{
			kernel.PlatformClaude: {
				Platform: kernel.PlatformClaude,
				Providers: []domain.Provider{
					{ID: 1, Name: "alpha", Position: 1, Archived: true, ArchivedAt: &archivedAt},
				},
			},
		},
	}
	service := NewService(repo)

	first, err := service.GetProfile(context.Background(), kernel.PlatformClaude.String())
	if err != nil {
		t.Fatalf("first get profile: %v", err)
	}
	first.Providers[0].Name = "mutated"
	*first.Providers[0].ArchivedAt = first.Providers[0].ArchivedAt.Add(time.Hour)

	second, err := service.GetProfile(context.Background(), kernel.PlatformClaude.String())
	if err != nil {
		t.Fatalf("second get profile: %v", err)
	}

	if repo.getProfileCalls != 1 {
		t.Fatalf("expected 1 repo call, got %d", repo.getProfileCalls)
	}
	if second.Providers[0].Name != "alpha" {
		t.Fatalf("expected cached profile clone, got %+v", second.Providers[0])
	}
	if second.Providers[0].ArchivedAt == nil || !second.Providers[0].ArchivedAt.Equal(archivedAt) {
		t.Fatalf("expected cached archived timestamp clone, got %v", second.Providers[0].ArchivedAt)
	}
}

func TestServiceSaveProfileRefreshesCache(t *testing.T) {
	repo := &routingRepoStub{
		profiles: map[kernel.Platform]domain.RouteProfile{
			kernel.PlatformClaude: {
				Platform:          kernel.PlatformClaude,
				DefaultProviderID: intPtr(1),
				Providers: []domain.Provider{
					{ID: 1, Name: "alpha", Position: 1},
				},
			},
		},
	}
	service := NewService(repo)

	saved, err := service.SaveProfile(context.Background(), domain.RouteProfile{
		Platform:          kernel.PlatformClaude,
		DefaultProviderID: intPtr(1),
		Providers: []domain.Provider{
			{ID: 1, Name: "alpha", Position: 1, APIURL: "https://example.com", APIKey: "secret"},
		},
	})
	if err != nil {
		t.Fatalf("save profile: %v", err)
	}
	if repo.saveProfileCalls != 1 {
		t.Fatalf("expected save to hit repo once, got %d", repo.saveProfileCalls)
	}
	if repo.getProfileCalls != 1 {
		t.Fatalf("expected save validation to read repo once, got %d", repo.getProfileCalls)
	}

	profile, err := service.GetProfile(context.Background(), kernel.PlatformClaude.String())
	if err != nil {
		t.Fatalf("get cached saved profile: %v", err)
	}
	if repo.getProfileCalls != 1 {
		t.Fatalf("expected cached read after save, got %d repo reads", repo.getProfileCalls)
	}
	if profile.Providers[0].APIURL != saved.Providers[0].APIURL {
		t.Fatalf("expected cache to be refreshed with saved profile, got %+v", profile.Providers[0])
	}
}

func intPtr(value int) *int {
	return &value
}

func TestServiceModelMappingSchemeLifecycle(t *testing.T) {
	repo := &routingRepoStub{
		profiles: map[kernel.Platform]domain.RouteProfile{
			kernel.PlatformClaude: {
				Platform: kernel.PlatformClaude,
				Providers: []domain.Provider{
					{ID: 1, Name: "alpha", Position: 1},
				},
			},
		},
	}
	service := NewService(repo)
	ctx := context.Background()

	saved, err := service.SaveModelMappingScheme(ctx, "claude", 1, "", domain.ModelMappingScheme{
		Name: "Daily", Mapping: map[string]string{"claude-*": "vendor/claude-*"},
	}, false)
	if err != nil {
		t.Fatalf("create inactive scheme: %v", err)
	}
	if saved.Providers[0].ActiveModelMappingScheme != "" || len(saved.Providers[0].ModelMapping) != 0 {
		t.Fatalf("new scheme must not activate itself: %+v", saved.Providers[0])
	}

	saved, err = service.SetActiveModelMappingScheme(ctx, "claude", 1, "Daily")
	if err != nil {
		t.Fatalf("activate scheme: %v", err)
	}
	if got := saved.Providers[0].ModelMapping["claude-*"]; got != "vendor/claude-*" {
		t.Fatalf("active scheme mirror not updated: %q", got)
	}

	_, err = service.SaveModelMappingScheme(ctx, "claude", 1, "", domain.ModelMappingScheme{
		Name: "daily", Mapping: map[string]string{},
	}, false)
	if err == nil {
		t.Fatal("expected case-insensitive duplicate scheme name to fail")
	}

	saved, err = service.SaveModelMappingScheme(ctx, "claude", 1, "", domain.ModelMappingScheme{
		Name: "Deep", Mapping: map[string]string{"claude-sonnet-4-5": "vendor/opus"},
	}, true)
	if err != nil {
		t.Fatalf("save and activate second scheme: %v", err)
	}
	if saved.Providers[0].ActiveModelMappingScheme != "Deep" || saved.Providers[0].ModelMapping["claude-sonnet-4-5"] != "vendor/opus" {
		t.Fatalf("second scheme was not activated: %+v", saved.Providers[0])
	}

	saved, err = service.SaveModelMappingScheme(ctx, "claude", 1, "Deep", domain.ModelMappingScheme{
		Name: "Deep Reasoning", Mapping: map[string]string{"claude-sonnet-4-5": "vendor/opus-next"},
	}, false)
	if err != nil {
		t.Fatalf("rename active scheme: %v", err)
	}
	provider := saved.Providers[0]
	if provider.ActiveModelMappingScheme != "Deep Reasoning" || provider.ModelMapping["claude-sonnet-4-5"] != "vendor/opus-next" {
		t.Fatalf("active scheme rename/edit not mirrored: %+v", provider)
	}

	beforeFailedDelete := repo.saveProfileCalls
	if _, err := service.DeleteModelMappingScheme(ctx, "claude", 1, "Deep Reasoning"); err == nil {
		t.Fatal("expected deleting active scheme to fail")
	}
	if repo.saveProfileCalls != beforeFailedDelete {
		t.Fatal("failed delete must not persist a mutation")
	}

	saved, err = service.SetActiveModelMappingScheme(ctx, "claude", 1, "")
	if err != nil {
		t.Fatalf("deactivate mapping: %v", err)
	}
	if saved.Providers[0].ActiveModelMappingScheme != "" || len(saved.Providers[0].ModelMapping) != 0 {
		t.Fatalf("deactivation must clear only the active mirror: %+v", saved.Providers[0])
	}
	if len(saved.Providers[0].ModelMappingSchemes) != 2 {
		t.Fatalf("deactivation must preserve schemes: %+v", saved.Providers[0].ModelMappingSchemes)
	}

	saved, err = service.DeleteModelMappingScheme(ctx, "claude", 1, "Deep Reasoning")
	if err != nil {
		t.Fatalf("delete inactive scheme: %v", err)
	}
	if len(saved.Providers[0].ModelMappingSchemes) != 1 || saved.Providers[0].ModelMappingSchemes[0].Name != "Daily" {
		t.Fatalf("unexpected schemes after delete: %+v", saved.Providers[0].ModelMappingSchemes)
	}

	// Returned profiles and cached profiles must not share scheme rule maps.
	saved.Providers[0].ModelMappingSchemes[0].Mapping["claude-*"] = "mutated/*"
	cached, err := service.GetProfile(ctx, "claude")
	if err != nil {
		t.Fatalf("get cached profile: %v", err)
	}
	if got := cached.Providers[0].ModelMappingSchemes[0].Mapping["claude-*"]; got != "vendor/claude-*" {
		t.Fatalf("cached scheme map was mutated through returned profile: %q", got)
	}
}

func TestServiceDefaultProviderSwitchPreservesPerProviderSchemes(t *testing.T) {
	repo := &routingRepoStub{
		profiles: map[kernel.Platform]domain.RouteProfile{
			kernel.PlatformClaude: {
				Platform:          kernel.PlatformClaude,
				DefaultProviderID: intPtr(1),
				Providers: []domain.Provider{
					{ID: 1, Name: "alpha", Position: 1, ModelMappingSchemes: []domain.ModelMappingScheme{
						{Name: "Alpha Daily", Mapping: map[string]string{"claude-*": "alpha/*"}},
					}, ActiveModelMappingScheme: "Alpha Daily"},
					{ID: 2, Name: "beta", Position: 2, ModelMappingSchemes: []domain.ModelMappingScheme{
						{Name: "Beta Daily", Mapping: map[string]string{"claude-*": "beta/*"}},
					}, ActiveModelMappingScheme: "Beta Daily"},
				},
			},
		},
	}
	service := NewService(repo)
	profile, err := service.GetProfile(context.Background(), "claude")
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	profile.DefaultProviderID = intPtr(2)
	saved, err := service.SaveProfile(context.Background(), profile)
	if err != nil {
		t.Fatalf("switch default provider: %v", err)
	}
	if saved.Providers[0].ActiveModelMappingScheme != "Alpha Daily" || saved.Providers[1].ActiveModelMappingScheme != "Beta Daily" {
		t.Fatalf("default switch changed provider schemes: %+v", saved.Providers)
	}
	if saved.Providers[0].ModelMapping["claude-*"] != "alpha/*" || saved.Providers[1].ModelMapping["claude-*"] != "beta/*" {
		t.Fatalf("default switch changed active mirrors: %+v", saved.Providers)
	}
}
