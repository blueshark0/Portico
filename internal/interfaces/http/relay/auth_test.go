package relay

import (
	"net/http/httptest"
	"testing"

	routingdomain "portico/internal/routing/domain"
	"portico/internal/shared/kernel"
)

func TestIsAuthorizedInboundRequest(t *testing.T) {
	tests := []struct {
		name   string
		header map[string]string
		query  string
		want   bool
	}{
		{name: "claude bearer placeholder", header: map[string]string{"Authorization": "Bearer portico"}, want: true},
		{name: "bearer prefix case-insensitive", header: map[string]string{"Authorization": "bearer CODE-SWITCH"}, want: true},
		{name: "raw token in authorization", header: map[string]string{"Authorization": "portico"}, want: true},
		{name: "x-api-key placeholder", header: map[string]string{"X-Api-Key": "portico"}, want: true},
		{name: "gemini header placeholder", header: map[string]string{"X-Goog-Api-Key": "portico"}, want: true},
		{name: "gemini query key placeholder", query: "key=portico", want: true},
		{name: "no credentials", want: false},
		{name: "foreign real key is rejected", header: map[string]string{"Authorization": "Bearer sk-real-key"}, want: false},
		{name: "foreign x-api-key is rejected", header: map[string]string{"X-Api-Key": "sk-user-key"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/v1/messages"
			if tt.query != "" {
				url += "?" + tt.query
			}
			req := httptest.NewRequest("POST", url, nil)
			for key, value := range tt.header {
				req.Header.Set(key, value)
			}
			if got := isAuthorizedInboundRequest(req); got != tt.want {
				t.Fatalf("isAuthorizedInboundRequest() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStripInboundCredentialHeaders(t *testing.T) {
	headers := map[string]string{
		"Authorization":  "Bearer portico",
		"X-Api-Key":      "user-real-key",
		"x-goog-api-key": "lowercase-variant",
		"Cookie":         "session=1",
		"Content-Type":   "application/json",
		"Accept":         "application/json",
	}
	stripInboundCredentialHeaders(headers)
	for _, forbidden := range []string{"Authorization", "X-Api-Key", "x-goog-api-key", "Cookie"} {
		if _, ok := headers[forbidden]; ok {
			t.Fatalf("expected %s to be stripped", forbidden)
		}
	}
	if headers["Content-Type"] != "application/json" || headers["Accept"] != "application/json" {
		t.Fatal("non-credential headers must be preserved")
	}
}

func TestApplyProviderCredentialsInjectsUnconditionally(t *testing.T) {
	provider := &routingdomain.Provider{APIKey: "sk-provider"}

	// 客户端只带 x-api-key（未带 Authorization）时：旧实现既不注入供应商
	// key、又把用户真实 key 透传给第三方——新实现必须两者都纠正。
	claudeHeaders := map[string]string{"X-Api-Key": "sk-user-real"}
	applyProviderCredentials(claudeHeaders, kernel.PlatformClaude, provider)
	if claudeHeaders["Authorization"] != "Bearer sk-provider" {
		t.Fatalf("Authorization = %q, want provider bearer", claudeHeaders["Authorization"])
	}
	if claudeHeaders["X-Api-Key"] != "sk-provider" {
		t.Fatalf("X-Api-Key = %q, want provider key", claudeHeaders["X-Api-Key"])
	}

	// Gemini：只注入 canonical 的 X-Goog-Api-Key，客户端占位头必须被清除，
	// 不能出现两个大小写不同、取值不同的认证头并存。
	geminiHeaders := map[string]string{"X-Goog-Api-Key": "portico"}
	applyProviderCredentials(geminiHeaders, kernel.PlatformGemini, provider)
	if geminiHeaders["X-Goog-Api-Key"] != "sk-provider" {
		t.Fatalf("X-Goog-Api-Key = %q, want provider key", geminiHeaders["X-Goog-Api-Key"])
	}
	if _, ok := geminiHeaders["Authorization"]; ok {
		t.Fatal("gemini must not receive Authorization header")
	}

	// 供应商未配置 key：仅剥离客户端凭证，不注入空值。
	emptyProvider := &routingdomain.Provider{}
	headers := map[string]string{"Authorization": "Bearer portico"}
	applyProviderCredentials(headers, kernel.PlatformCodex, emptyProvider)
	if len(headers) != 0 {
		t.Fatalf("expected all credential headers stripped, got %v", headers)
	}
}
