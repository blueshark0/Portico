package application

import (
	"context"
	"testing"
	"time"

	configsyncdomain "portico/internal/configsync/domain"
	routingapp "portico/internal/routing/application"
	routingdomain "portico/internal/routing/domain"
	"portico/internal/shared/kernel"
)

type fakeSettingsRepo struct {
	settings configsyncdomain.SyncSettings
}

func (r *fakeSettingsRepo) GetSettings(_ context.Context) (configsyncdomain.SyncSettings, error) {
	return r.settings, nil
}

func (r *fakeSettingsRepo) SaveSettings(_ context.Context, settings configsyncdomain.SyncSettings) (configsyncdomain.SyncSettings, error) {
	r.settings = settings
	return settings, nil
}

type fakeRemote struct {
	files   map[string][]byte
	uploads int
}

func newFakeRemote() *fakeRemote {
	return &fakeRemote{files: map[string][]byte{}}
}

func (f *fakeRemote) TestConnection(_ context.Context, _ configsyncdomain.SyncSettings) error {
	return nil
}

func (f *fakeRemote) Upload(_ context.Context, _ configsyncdomain.SyncSettings, filename string, payload []byte) error {
	f.uploads++
	f.files[filename] = append([]byte(nil), payload...)
	return nil
}

func (f *fakeRemote) Download(_ context.Context, _ configsyncdomain.SyncSettings, filename string) ([]byte, error) {
	if data, ok := f.files[filename]; ok {
		return data, nil
	}
	return nil, ErrRemoteSnapshotNotFound
}

type syncRoutingRepoStub struct {
	prefs           routingdomain.AppPreferences
	profiles        map[kernel.Platform]routingdomain.RouteProfile
	replaceAllCalls int
	saveProfileHits int
	savePrefHits    int
}

func (r *syncRoutingRepoStub) GetProfile(_ context.Context, platform kernel.Platform) (routingdomain.RouteProfile, error) {
	if profile, ok := r.profiles[platform]; ok {
		return profile, nil
	}
	return routingdomain.RouteProfile{Platform: platform}, nil
}

func (r *syncRoutingRepoStub) SaveProfile(_ context.Context, profile routingdomain.RouteProfile) (routingdomain.RouteProfile, error) {
	r.saveProfileHits++
	r.profiles[profile.Platform] = profile
	return profile, nil
}

func (r *syncRoutingRepoStub) GetAppPreferences(_ context.Context) (routingdomain.AppPreferences, error) {
	return r.prefs, nil
}

func (r *syncRoutingRepoStub) SaveAppPreferences(_ context.Context, preferences routingdomain.AppPreferences) (routingdomain.AppPreferences, error) {
	r.savePrefHits++
	r.prefs = preferences
	return preferences, nil
}

func (r *syncRoutingRepoStub) ReplaceAll(_ context.Context, preferences routingdomain.AppPreferences, profiles []routingdomain.RouteProfile) error {
	r.replaceAllCalls++
	r.prefs = preferences
	for _, profile := range profiles {
		r.profiles[profile.Platform] = profile
	}
	return nil
}

func newSyncTestService(t *testing.T, remote *fakeRemote) (*Service, *syncRoutingRepoStub) {
	t.Helper()
	routingRepo := &syncRoutingRepoStub{
		prefs: routingdomain.AppPreferences{ShowHeatmap: true, ProxyEnabled: true, ProxyURL: "http://127.0.0.1:7890"},
		profiles: map[kernel.Platform]routingdomain.RouteProfile{
			kernel.PlatformClaude: {
				Platform: kernel.PlatformClaude,
				Providers: []routingdomain.Provider{
					{ID: 1, Name: "alpha", APIURL: "https://api.example.com", APIKey: "sk", Position: 1},
				},
			},
			kernel.PlatformCodex:  {Platform: kernel.PlatformCodex},
			kernel.PlatformGemini: {Platform: kernel.PlatformGemini},
		},
	}
	settingsRepo := &fakeSettingsRepo{settings: configsyncdomain.SyncSettings{
		Endpoint:       "https://dav.example.com",
		Username:       "user",
		AppPassword:    "app-pass",
		RemotePath:     "/Portico",
		SyncPassphrase: "sync-secret",
	}}
	service := NewService(settingsRepo, remote, routingapp.NewService(routingRepo), "1.0.0")
	service.now = func() time.Time { return time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC) }
	service.hostname = func() (string, error) { return "test-device", nil }
	service.rollbackDir = t.TempDir()
	return service, routingRepo
}

func TestLegacyRemoteSnapshotCanBeRead(t *testing.T) {
	remote := newFakeRemote()
	service, _ := newSyncTestService(t, remote)
	ctx := context.Background()
	if _, err := service.PushSnapshot(ctx, configsyncdomain.UIPreferences{Theme: "dark", Locale: "zh"}); err != nil {
		t.Fatal(err)
	}
	remote.files[configsyncdomain.LegacyRemoteSnapshotFile] = remote.files[configsyncdomain.RemoteSnapshotFile]
	delete(remote.files, configsyncdomain.RemoteSnapshotFile)

	meta, err := service.GetRemoteSnapshotMeta(ctx)
	if err != nil {
		t.Fatalf("read legacy snapshot meta: %v", err)
	}
	if !meta.Exists || meta.DeviceName != "test-device" {
		t.Fatalf("unexpected legacy snapshot meta: %+v", meta)
	}
	if _, err := service.PullSnapshot(ctx); err != nil {
		t.Fatalf("pull legacy snapshot: %v", err)
	}
}

func TestPushThenPullSnapshotEndToEnd(t *testing.T) {
	remote := newFakeRemote()
	service, _ := newSyncTestService(t, remote)
	ctx := context.Background()

	meta, err := service.PushSnapshot(ctx, configsyncdomain.UIPreferences{Theme: "dark", Locale: "zh"})
	if err != nil {
		t.Fatalf("push snapshot: %v", err)
	}
	if remote.uploads != 1 {
		t.Fatalf("expected exactly one upload, got %d", remote.uploads)
	}
	if meta.DeviceName != "test-device" || meta.UpdatedAt != "2026-07-07T12:00:00Z" {
		t.Fatalf("unexpected push meta: %+v", meta)
	}

	snapshot, err := service.PullSnapshot(ctx)
	if err != nil {
		t.Fatalf("pull snapshot: %v", err)
	}
	if !snapshot.Backend.AppPreferences.ProxyEnabled || snapshot.Backend.AppPreferences.ProxyURL != "http://127.0.0.1:7890" {
		t.Fatalf("preferences lost in round trip: %+v", snapshot.Backend.AppPreferences)
	}
	providers := snapshot.Backend.ClaudeProfile.Providers
	if len(providers) != 1 || providers[0].Name != "alpha" || providers[0].APIKey != "sk" {
		t.Fatalf("claude providers lost in round trip: %+v", providers)
	}
	if snapshot.UI.Theme != "dark" || snapshot.UI.Locale != "zh" {
		t.Fatalf("ui preferences lost in round trip: %+v", snapshot.UI)
	}
}

func TestPullSnapshotFailsWithWrongPassphrase(t *testing.T) {
	remote := newFakeRemote()
	service, _ := newSyncTestService(t, remote)
	ctx := context.Background()

	if _, err := service.PushSnapshot(ctx, configsyncdomain.UIPreferences{}); err != nil {
		t.Fatalf("push snapshot: %v", err)
	}
	service.repo.(*fakeSettingsRepo).settings.SyncPassphrase = "different-secret"
	if _, err := service.PullSnapshot(ctx); err == nil {
		t.Fatal("expected pull with wrong passphrase to fail")
	}
}

func TestGetRemoteSnapshotMetaWhenRemoteEmpty(t *testing.T) {
	service, _ := newSyncTestService(t, newFakeRemote())
	meta, err := service.GetRemoteSnapshotMeta(context.Background())
	if err != nil {
		t.Fatalf("expected missing remote snapshot to be reported without error, got %v", err)
	}
	if meta.Exists {
		t.Fatalf("expected meta.Exists=false, got %+v", meta)
	}
}

func TestApplySnapshotUsesSingleAtomicReplace(t *testing.T) {
	service, routingRepo := newSyncTestService(t, newFakeRemote())
	snapshot := configsyncdomain.BackendSnapshot{
		AppPreferences: routingdomain.AppPreferences{ShowHeatmap: true, ProxyURL: ""},
		ClaudeProfile: routingdomain.RouteProfile{
			Platform: kernel.PlatformClaude,
			Providers: []routingdomain.Provider{
				{ID: 2, Name: "beta", APIURL: "https://beta.example.com", APIKey: "sk2", Position: 1},
			},
		},
		CodexProfile:  routingdomain.RouteProfile{Platform: kernel.PlatformCodex},
		GeminiProfile: routingdomain.RouteProfile{Platform: kernel.PlatformGemini},
	}

	if err := service.ApplySnapshot(context.Background(), snapshot); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}
	// 必须走单事务 ReplaceAll，而不是一次偏好写入加三次独立 profile 写入
	// （那种路径中途失败会留下混合状态）。
	if routingRepo.replaceAllCalls != 1 {
		t.Fatalf("expected exactly one ReplaceAll call, got %d", routingRepo.replaceAllCalls)
	}
	if routingRepo.saveProfileHits != 0 || routingRepo.savePrefHits != 0 {
		t.Fatalf("expected no piecemeal writes, got profiles=%d prefs=%d", routingRepo.saveProfileHits, routingRepo.savePrefHits)
	}
	if got := routingRepo.profiles[kernel.PlatformClaude].Providers[0].Name; got != "beta" {
		t.Fatalf("claude profile not replaced, got provider %q", got)
	}
}

func TestApplySnapshotRejectsMismatchedPlatform(t *testing.T) {
	service, _ := newSyncTestService(t, newFakeRemote())
	snapshot := configsyncdomain.BackendSnapshot{
		ClaudeProfile: routingdomain.RouteProfile{Platform: kernel.PlatformCodex},
		CodexProfile:  routingdomain.RouteProfile{Platform: kernel.PlatformCodex},
		GeminiProfile: routingdomain.RouteProfile{Platform: kernel.PlatformGemini},
	}
	if err := service.ApplySnapshot(context.Background(), snapshot); err == nil {
		t.Fatal("expected mismatched platform snapshot to be rejected")
	}
}
