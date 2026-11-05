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
