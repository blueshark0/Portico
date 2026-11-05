package domain

import (
	"strings"
	"testing"

	"portico/internal/shared/kernel"
)

func resolverTestProfile() RouteProfile {
	defaultID := 1
	return RouteProfile{
		Platform:          kernel.PlatformClaude,
		DefaultProviderID: &defaultID,
		Providers: []Provider{
			{ID: 1, Name: "default-p", APIURL: "https://a.example.com", APIKey: "k1", Position: 1},
			{ID: 2, Name: "limited-p", APIURL: "https://b.example.com", APIKey: "k2", Position: 2,
				SupportedModels: map[string]bool{"claude-opus-4": true}},
			{ID: 3, Name: "broken-p", APIURL: "", APIKey: "", Position: 3},
			{ID: 4, Name: "archived-p", APIURL: "https://c.example.com", APIKey: "k4", Position: 4, Archived: true},
		},
	}
}

func TestResolveProvider(t *testing.T) {
	profile := resolverTestProfile()

	tests := []struct {
		name         string
		providerName string
		model        string
		wantProvider string
		wantErr      string
	}{
		{name: "empty name resolves default", providerName: "", model: "claude-sonnet-4", wantProvider: "default-p"},
		{name: "bound provider by name", providerName: "limited-p", model: "claude-opus-4", wantProvider: "limited-p"},
		{name: "unknown provider", providerName: "ghost", wantErr: "不存在"},
		{name: "incomplete configuration", providerName: "broken-p", wantErr: "配置不完整"},
		{name: "unsupported model", providerName: "limited-p", model: "claude-sonnet-4", wantErr: "不支持模型"},
		{name: "empty model skips support check", providerName: "limited-p", model: "", wantProvider: "limited-p"},
		{name: "archived provider is unreachable", providerName: "archived-p", wantErr: "已归档"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := profile.ResolveProvider(tt.providerName, tt.model)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if provider.Name != tt.wantProvider {
				t.Fatalf("provider = %s, want %s", provider.Name, tt.wantProvider)
			}
		})
	}
}

func TestResolveProviderWithoutDefault(t *testing.T) {
	profile := RouteProfile{Platform: kernel.PlatformClaude}
	if _, err := profile.ResolveProvider("", "any"); err == nil || !strings.Contains(err.Error(), "默认供应商") {
		t.Fatalf("expected missing default provider error, got %v", err)
	}
}

func TestResolveProviderArchivedDefault(t *testing.T) {
	defaultID := 9
	profile := RouteProfile{
		Platform:          kernel.PlatformClaude,
		DefaultProviderID: &defaultID,
		Providers: []Provider{
			{ID: 9, Name: "default-p", APIURL: "https://a.example.com", APIKey: "k1", Position: 1, Archived: true},
		},
	}
	if _, err := profile.ResolveProvider("", "claude-sonnet-4"); err == nil || !strings.Contains(err.Error(), "已归档") {
		t.Fatalf("expected archived default provider error, got %v", err)
	}
}

func TestPlanModelRewrite(t *testing.T) {
	mapped := Provider{ModelMapping: map[string]string{"claude-*": "vendor/claude-*"}}
	plain := Provider{}

	tests := []struct {
		name          string
		provider      *Provider
		model         string
		bodyHasModel  bool
		forceRewrite  bool
		wantEffective string
		wantRewrite   bool
	}{
		{name: "mapped model rewrites body", provider: &mapped, model: "claude-sonnet-4", bodyHasModel: true,
			wantEffective: "vendor/claude-sonnet-4", wantRewrite: true},
		{name: "mapped model without body field", provider: &mapped, model: "claude-sonnet-4", bodyHasModel: false,
			wantEffective: "vendor/claude-sonnet-4", wantRewrite: false},
		{name: "unmapped model passes through", provider: &plain, model: "claude-sonnet-4", bodyHasModel: true,
			wantEffective: "claude-sonnet-4", wantRewrite: false},
		{name: "force rewrite normalizes even unmapped", provider: &plain, model: "gemini-2.5-pro", bodyHasModel: true, forceRewrite: true,
			wantEffective: "gemini-2.5-pro", wantRewrite: true},
		{name: "empty model never rewrites", provider: &mapped, model: "", bodyHasModel: true, forceRewrite: true,
			wantEffective: "", wantRewrite: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.provider.PlanModelRewrite(tt.model, tt.bodyHasModel, tt.forceRewrite)
			if got.EffectiveModel != tt.wantEffective || got.ShouldRewriteBody != tt.wantRewrite {
				t.Fatalf("PlanModelRewrite = %+v, want effective=%q rewrite=%v", got, tt.wantEffective, tt.wantRewrite)
			}
		})
	}
}
