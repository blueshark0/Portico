package tui

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"portico/internal/interfaces/http/admin"
	routingdomain "portico/internal/routing/domain"
	"portico/internal/shared/kernel"
)

// 测试用 facade：类型名决定 RPC 方法名（RoutingFacade.GetProfile 等）。
type RoutingFacade struct {
	saved []routingdomain.RouteProfile
}

func (f *RoutingFacade) GetProfile(platform string) (routingdomain.RouteProfile, error) {
	if platform == "broken" {
		return routingdomain.RouteProfile{}, errors.New("profile store unavailable")
	}
	return routingdomain.RouteProfile{
		Platform:  kernel.Platform(platform),
		Providers: []routingdomain.Provider{{ID: 7, Name: "p7", APIURL: "https://p7.example.com"}},
	}, nil
}

func (f *RoutingFacade) SaveProfile(profile routingdomain.RouteProfile) (routingdomain.RouteProfile, error) {
	f.saved = append(f.saved, profile)
	return profile, nil
}

type PlatformProxyFacade struct{ enabled map[string]bool }

func (f *PlatformProxyFacade) Enable(platform string) error  { f.enabled[platform] = true; return nil }
func (f *PlatformProxyFacade) Disable(platform string) error { f.enabled[platform] = false; return nil }

func newTestServer(t *testing.T, token string, requireToken bool) (*httptest.Server, *RoutingFacade, *PlatformProxyFacade) {
	t.Helper()
	routing := &RoutingFacade{}
	proxy := &PlatformProxyFacade{enabled: map[string]bool{}}
	registry, err := admin.NewRegistry(routing, proxy)
	if err != nil {
		t.Fatal(err)
	}
	server := admin.NewServer(admin.Options{
		Tokens:             admin.StaticToken(token),
		AlwaysRequireToken: requireToken,
		Registry:           registry,
		Version:            "test",
	})
	srv := httptest.NewServer(server.Handler())
	t.Cleanup(srv.Close)
	return srv, routing, proxy
}

func TestClientCallRoundTrip(t *testing.T) {
	srv, routing, proxy := newTestServer(t, "secret", false)
	client := NewClient(srv.URL+"/", "")
	ctx := context.Background()

	health, err := client.Health(ctx)
	if err != nil || health.Version != "test" {
		t.Fatalf("health = %+v, %v", health, err)
	}
	profile, err := client.Profile(ctx, "codex")
	if err != nil {
		t.Fatal(err)
	}
	if profile.Platform != kernel.PlatformCodex || len(profile.Providers) != 1 || profile.Providers[0].Name != "p7" {
		t.Fatalf("profile = %+v", profile)
	}
	profile.DefaultProviderID = intPtr(7)
	if _, err := client.SaveProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	if len(routing.saved) != 1 || *routing.saved[0].DefaultProviderID != 7 {
		t.Fatalf("saved = %+v", routing.saved)
	}
	if err := client.SetPlatformProxy(ctx, "claude", true); err != nil || !proxy.enabled["claude"] {
		t.Fatalf("enable: %v %v", err, proxy.enabled)
	}
	if err := client.SetPlatformProxy(ctx, "claude", false); err != nil || proxy.enabled["claude"] {
		t.Fatalf("disable: %v %v", err, proxy.enabled)
	}
}

func TestClientErrors(t *testing.T) {
	srv, _, _ := newTestServer(t, "secret", false)
	client := NewClient(srv.URL, "")
	ctx := context.Background()

	_, err := client.Profile(ctx, "broken")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Message != "profile store unavailable" {
		t.Fatalf("want facade error message, got %v", err)
	}
	if err := client.Call(ctx, "RoutingFacade.Missing", nil); !errors.As(err, &apiErr) || apiErr.Status != 404 {
		t.Fatalf("want 404 for unknown method, got %v", err)
	}
}

func TestClientToken(t *testing.T) {
	srv, _, _ := newTestServer(t, "secret", true)
	ctx := context.Background()
	if err := NewClient(srv.URL, "").CheckSession(ctx); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("no token: %v", err)
	}
	if _, err := NewClient(srv.URL, "wrong").Profile(ctx, "claude"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("wrong token: %v", err)
	}
	client := NewClient(srv.URL, "secret")
	if err := client.CheckSession(ctx); err != nil {
		t.Fatalf("valid token: %v", err)
	}
	if _, err := client.Profile(ctx, "claude"); err != nil {
		t.Fatalf("valid token rpc: %v", err)
	}
}

func TestResolveTarget(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PORTICO_ADMIN_URL", "")
	t.Setenv("PORTICO_ADMIN_TOKEN", "")
	dir := filepath.Join(home, ".portico")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "admin_token"), []byte("local-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// 默认：本机地址 + 本机令牌文件
	if got := ResolveTarget("", ""); got.URL != "http://127.0.0.1:18101" || got.Token != "local-token" {
		t.Fatalf("default = %+v", got)
	}
	// 运行时信息覆盖默认地址
	if err := os.WriteFile(filepath.Join(dir, "server.json"), []byte(`{"adminUrl":"https://127.0.0.1:19000"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ResolveTarget("", ""); got.URL != "https://127.0.0.1:19000" {
		t.Fatalf("runtime info = %+v", got)
	}
	// 远程地址不发送本机令牌
	if got := ResolveTarget("http://10.0.0.5:18101", ""); got.Token != "" {
		t.Fatalf("remote target must not use local token file: %+v", got)
	}
	// 环境变量与参数优先
	t.Setenv("PORTICO_ADMIN_URL", "http://env.example:1")
	t.Setenv("PORTICO_ADMIN_TOKEN", "env-token")
	if got := ResolveTarget("", ""); got.URL != "http://env.example:1" || got.Token != "env-token" {
		t.Fatalf("env = %+v", got)
	}
	if got := ResolveTarget("http://flag.example:2", "flag-token"); got.URL != "http://flag.example:2" || got.Token != "flag-token" {
		t.Fatalf("flags = %+v", got)
	}
}
