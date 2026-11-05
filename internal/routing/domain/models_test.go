package domain

import (
	"strings"
	"testing"
	"time"

	"portico/internal/shared/kernel"
)

func TestRouteProfileValidateRejectsDuplicateProviderNames(t *testing.T) {
	profile := RouteProfile{
		Platform:          kernel.PlatformClaude,
		DefaultProviderID: intPtr(1),
		Providers: []Provider{
			{ID: 1, Name: "Alpha", Position: 1, APIURL: "https://alpha.example", APIKey: "secret"},
			{ID: 2, Name: " alpha ", Position: 2, APIURL: "https://beta.example", APIKey: "secret"},
		},
	}

	err := profile.Normalize().Validate()
	if err == nil {
		t.Fatal("expected duplicate provider name validation error")
	}
	if !strings.Contains(err.Error(), "duplicate provider name") {
		t.Fatalf("expected duplicate provider name error, got %v", err)
	}
}

func intPtr(value int) *int {
	return &value
}

func TestRouteProfileValidateArchivedDefault(t *testing.T) {
	profile := RouteProfile{
		Platform:          kernel.PlatformClaude,
		DefaultProviderID: intPtr(1),
		Providers: []Provider{
			{ID: 1, Name: "Alpha", Position: 1, APIURL: "https://alpha.example", APIKey: "secret", Archived: true},
			{ID: 2, Name: "Beta", Position: 2, APIURL: "https://beta.example", APIKey: "secret"},
		},
	}

	err := profile.Normalize().Validate()
	if err == nil {
		t.Fatal("expected archived default provider validation error")
	}
	if !strings.Contains(err.Error(), "已归档") {
		t.Fatalf("expected archived default provider error, got %v", err)
	}
}

func TestRouteProfileValidateAllowsArchivedNonDefault(t *testing.T) {
	profile := RouteProfile{
		Platform:          kernel.PlatformClaude,
		DefaultProviderID: intPtr(1),
		Providers: []Provider{
			{ID: 1, Name: "Alpha", Position: 1, APIURL: "https://alpha.example", APIKey: "secret"},
			{ID: 2, Name: "Beta", Position: 2, APIURL: "https://beta.example", APIKey: "secret", Archived: true},
		},
	}

	if err := profile.Normalize().Validate(); err != nil {
		t.Fatalf("archived non-default provider should validate, got %v", err)
	}
}

func TestRouteProfileNormalizeClearsArchivedAtWhenRestored(t *testing.T) {
	archivedAt := time.Date(2026, 10, 22, 14, 32, 0, 0, time.FixedZone("test", 8*60*60))
	profile := RouteProfile{
		Platform: kernel.PlatformClaude,
		Providers: []Provider{{
			ID: 1, Name: "Alpha", Position: 1, APIURL: "https://alpha.example",
			ArchivedAt: &archivedAt,
		}},
	}

	normalized := profile.Normalize()
	if normalized.Providers[0].ArchivedAt != nil {
		t.Fatalf("expected active provider timestamp to be cleared, got %v", normalized.Providers[0].ArchivedAt)
	}

	profile.Providers[0].Archived = true
	normalized = profile.Normalize()
	if normalized.Providers[0].ArchivedAt == nil {
		t.Fatal("expected archived timestamp to be preserved")
	}
	if normalized.Providers[0].ArchivedAt.Location() != time.UTC {
		t.Fatalf("expected archived timestamp in UTC, got %v", normalized.Providers[0].ArchivedAt.Location())
	}
	normalized.Providers[0].ArchivedAt = nil
	if profile.Providers[0].ArchivedAt == nil {
		t.Fatal("expected normalize to clone archived timestamp pointer")
	}
}

func TestMatchWildcard(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		text    string
		want    bool
	}{
		{name: "exact match without wildcard", pattern: "claude-sonnet-4", text: "claude-sonnet-4", want: true},
		{name: "exact mismatch without wildcard", pattern: "claude-sonnet-4", text: "claude-opus-4", want: false},
		{name: "prefix wildcard hit", pattern: "claude-*", text: "claude-sonnet-4", want: true},
		{name: "prefix wildcard miss", pattern: "claude-*", text: "gpt-4", want: false},
		{name: "suffix wildcard hit", pattern: "*-sonnet", text: "claude-sonnet", want: true},
		{name: "prefix and suffix wildcard hit", pattern: "claude-*-v2", text: "claude-sonnet-v2", want: true},
		{name: "prefix and suffix wildcard miss", pattern: "claude-*-v2", text: "claude-sonnet-v3", want: false},
		{name: "bare wildcard matches everything", pattern: "*", text: "anything", want: true},
		{name: "multiple wildcards are unsupported", pattern: "*sonnet*", text: "claude-sonnet-4", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchWildcard(tt.pattern, tt.text); got != tt.want {
				t.Fatalf("matchWildcard(%q, %q) = %v, want %v", tt.pattern, tt.text, got, tt.want)
			}
		})
	}
}

func TestIsModelSupported(t *testing.T) {
	tests := []struct {
		name     string
		provider Provider
		model    string
		want     bool
	}{
		{
			name:     "empty whitelist and mapping accept everything",
			provider: Provider{},
			model:    "any-model",
			want:     true,
		},
		{
			name:     "exact whitelist hit",
			provider: Provider{SupportedModels: map[string]bool{"claude-sonnet-4": true}},
			model:    "claude-sonnet-4",
			want:     true,
		},
		{
			name:     "whitelist wildcard hit",
			provider: Provider{SupportedModels: map[string]bool{"claude-*": true}},
			model:    "claude-opus-4",
			want:     true,
		},
		{
			name:     "explicitly disabled model is not supported",
			provider: Provider{SupportedModels: map[string]bool{"claude-sonnet-4": false}},
			model:    "claude-sonnet-4",
			want:     false,
		},
		{
			name:     "disabled wildcard does not match",
			provider: Provider{SupportedModels: map[string]bool{"claude-*": false}},
			model:    "claude-sonnet-4",
			want:     false,
		},
		{
			name:     "mapping key counts as supported",
			provider: Provider{ModelMapping: map[string]string{"claude-sonnet-4": "vendor/sonnet"}},
			model:    "claude-sonnet-4",
			want:     true,
		},
		{
			name:     "mapping wildcard counts as supported",
			provider: Provider{ModelMapping: map[string]string{"claude-*": "vendor/*"}},
			model:    "claude-sonnet-4",
			want:     true,
		},
		{
			name:     "no hit anywhere",
			provider: Provider{SupportedModels: map[string]bool{"gpt-4": true}, ModelMapping: map[string]string{"gemini-*": "g/*"}},
			model:    "claude-sonnet-4",
			want:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.provider.IsModelSupported(tt.model); got != tt.want {
				t.Fatalf("IsModelSupported(%q) = %v, want %v", tt.model, got, tt.want)
			}
		})
	}
}

func TestGetEffectiveModel(t *testing.T) {
	tests := []struct {
		name     string
		provider Provider
		model    string
		want     string
	}{
		{
			name:     "no mapping passes through",
			provider: Provider{},
			model:    "claude-sonnet-4",
			want:     "claude-sonnet-4",
		},
		{
			name:     "exact mapping wins over wildcard",
			provider: Provider{ModelMapping: map[string]string{"claude-sonnet-4": "exact-target", "claude-*": "wild/*"}},
			model:    "claude-sonnet-4",
			want:     "exact-target",
		},
		{
			name:     "wildcard mapping expands captured segment",
			provider: Provider{ModelMapping: map[string]string{"claude-*": "anthropic/claude-*"}},
			model:    "claude-sonnet-4.5",
			want:     "anthropic/claude-sonnet-4.5",
		},
		{
			name:     "wildcard mapping to literal replacement",
			provider: Provider{ModelMapping: map[string]string{"claude-*": "fixed-model"}},
			model:    "claude-opus-4",
			want:     "fixed-model",
		},
		{
			name:     "unmatched request passes through",
			provider: Provider{ModelMapping: map[string]string{"gpt-*": "openai/*"}},
			model:    "claude-sonnet-4",
			want:     "claude-sonnet-4",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.provider.GetEffectiveModel(tt.model); got != tt.want {
				t.Fatalf("GetEffectiveModel(%q) = %q, want %q", tt.model, got, tt.want)
			}
		})
	}
}

func TestGetEffectiveModelPrefersMostSpecificWildcardDeterministically(t *testing.T) {
	provider := Provider{ModelMapping: map[string]string{
		"claude-*":        "generic/*",
		"claude-sonnet-*": "sonnet/*",
	}}
	// 多个通配符同时命中时必须确定性地选最特异的一条，
	// 不能随 map 遍历序漂移；重复多次以捕捉随机性回归。
	for i := 0; i < 100; i++ {
		if got := provider.GetEffectiveModel("claude-sonnet-4.5"); got != "sonnet/4.5" {
			t.Fatalf("iteration %d: GetEffectiveModel = %q, want %q", i, got, "sonnet/4.5")
		}
		if got := provider.GetEffectiveModel("claude-opus-4"); got != "generic/opus-4" {
			t.Fatalf("iteration %d: GetEffectiveModel = %q, want %q", i, got, "generic/opus-4")
		}
	}
}

func TestNormalizeRequestBodyReplacements(t *testing.T) {
	normalized := normalizeRequestBodyReplacements([]RequestBodyReplacement{
		{Path: " model ", Value: ` "vendor/model" `},
		{Path: "metadata.tier", MatchValue: ` "pro" `, Value: `"enterprise"`},
		{Path: "   ", Value: `"dropped"`},
		{Path: "temperature", Value: "   "},
	})

	if len(normalized) != 2 {
		t.Fatalf("expected 2 normalized replacements, got %d: %+v", len(normalized), normalized)
	}
	if normalized[0].Path != "model" || normalized[0].MatchValue != "" || normalized[0].Value != `"vendor/model"` {
		t.Fatalf("expected override entry with trimmed fields, got %+v", normalized[0])
	}
	if normalized[1].Path != "metadata.tier" || normalized[1].MatchValue != `"pro"` || normalized[1].Value != `"enterprise"` {
		t.Fatalf("expected conditional entry with trimmed match value, got %+v", normalized[1])
	}
}
