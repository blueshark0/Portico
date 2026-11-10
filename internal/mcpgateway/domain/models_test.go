package domain

import (
	"strings"
	"testing"
)

func TestMcpServerValidate(t *testing.T) {
	cases := []struct {
		name    string
		server  McpServer
		wantErr bool
	}{
		{"stdio ok", McpServer{Name: "fs", Type: ServerTypeStdio, Command: "npx", Args: []string{"-y", "srv"}}, false},
		{"http ok", McpServer{Name: "remote-1", Type: ServerTypeHTTP, URL: "https://example.com/mcp"}, false},
		{"empty name", McpServer{Name: "", Type: ServerTypeStdio, Command: "x"}, true},
		{"name bad char", McpServer{Name: "my server", Type: ServerTypeStdio, Command: "x"}, true},
		{"name dot", McpServer{Name: "my.server", Type: ServerTypeStdio, Command: "x"}, true},
		{"name too long", McpServer{Name: strings.Repeat("a", 33), Type: ServerTypeStdio, Command: "x"}, true},
		{"name max ok", McpServer{Name: strings.Repeat("a", 32), Type: ServerTypeStdio, Command: "x"}, false},
		{"name leading dash", McpServer{Name: "-abc", Type: ServerTypeStdio, Command: "x"}, true},
		{"name double underscore", McpServer{Name: "a__b", Type: ServerTypeStdio, Command: "x"}, true},
		{"stdio no command", McpServer{Name: "fs", Type: ServerTypeStdio}, true},
		{"http no url", McpServer{Name: "r", Type: ServerTypeHTTP}, true},
		{"http bad scheme", McpServer{Name: "r", Type: ServerTypeHTTP, URL: "ftp://x/mcp"}, true},
		{"http no host", McpServer{Name: "r", Type: ServerTypeHTTP, URL: "http://"}, true},
		{"unknown type", McpServer{Name: "r", Type: "sse"}, true},
		{"stdio empty env key", McpServer{Name: "fs", Type: ServerTypeStdio, Command: "x", Env: map[string]string{"": "v"}}, true},
		{"http empty header key", McpServer{Name: "r", Type: ServerTypeHTTP, URL: "http://x/mcp", Headers: map[string]string{"": "v"}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.server.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() err=%v, wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestMcpServerNormalize(t *testing.T) {
	stdio := McpServer{Name: "a", Type: ServerTypeStdio, Command: "c", URL: "http://x", Headers: map[string]string{"A": "b"}}
	stdio.Normalize()
	if stdio.URL != "" || stdio.Headers != nil {
		t.Fatalf("stdio 应清理 http 字段: %+v", stdio)
	}
	if stdio.TimeoutSec != DefaultTimeoutSec {
		t.Fatalf("默认超时: %d", stdio.TimeoutSec)
	}

	httpSrv := McpServer{Name: "b", Type: ServerTypeHTTP, URL: "http://x", Command: "c", TimeoutSec: 9999}
	httpSrv.Normalize()
	if httpSrv.Command != "" {
		t.Fatalf("http 应清理 stdio 字段: %+v", httpSrv)
	}
	if httpSrv.TimeoutSec != MaxTimeoutSec {
		t.Fatalf("超时应被 clamp: %d", httpSrv.TimeoutSec)
	}
}

func TestFingerprint(t *testing.T) {
	a := McpServer{Name: "s", Type: ServerTypeStdio, Command: "npx", Args: []string{"x"}}
	b := a
	if a.Fingerprint() != b.Fingerprint() {
		t.Fatal("相同配置指纹应一致")
	}
	b.Args = []string{"y"}
	if a.Fingerprint() == b.Fingerprint() {
		t.Fatal("配置变化指纹应变化")
	}
	// Enabled/Position 不影响连接，不应进入指纹
	c := a
	c.Enabled = !a.Enabled
	c.Position = a.Position + 1
	if a.Fingerprint() != c.Fingerprint() {
		t.Fatal("Enabled/Position 不应影响指纹")
	}
}

func TestSanitizeToolSegment(t *testing.T) {
	cases := map[string]string{
		"read_file":     "read_file",
		"read file":     "read_file",
		"中文.tool/v2":    "___tool_v2",
		"":              "x",
		"a.b:c":         "a_b_c",
		"already__ok-1": "already__ok-1",
	}
	for in, want := range cases {
		if got := SanitizeToolSegment(in); got != want {
			t.Errorf("SanitizeToolSegment(%q) = %q, want %q", in, got, want)
		}
	}
	// 幂等
	for in := range cases {
		once := SanitizeToolSegment(in)
		if twice := SanitizeToolSegment(once); twice != once {
			t.Errorf("不幂等: %q -> %q -> %q", in, once, twice)
		}
	}
}

func TestPublicToolName(t *testing.T) {
	if got := PublicToolName("fs", "read file"); got != "fs__read_file" {
		t.Fatalf("got %q", got)
	}
	// 超长：确定性 + 64 上限
	longTool := strings.Repeat("t", 100)
	got := PublicToolName("srv", longTool)
	if len(got) != MaxPublicToolNameLen {
		t.Fatalf("len=%d, want %d", len(got), MaxPublicToolNameLen)
	}
	if got != PublicToolName("srv", longTool) {
		t.Fatal("同输入应得到同输出")
	}
	other := PublicToolName("srv", longTool+"x")
	if other == got {
		t.Fatal("不同输入哈希后缀应不同")
	}
	// 恰好边界
	exact := strings.Repeat("t", MaxPublicToolNameLen-len("srv")-2)
	if got := PublicToolName("srv", exact); len(got) != MaxPublicToolNameLen || strings.Count(got, "_") > len(exact)+2 {
		t.Fatalf("边界长度不应触发截断: %q", got)
	}
}

func TestMcpServerValidateOAuth(t *testing.T) {
	base := func(mutate func(*McpServer)) McpServer {
		srv := McpServer{Name: "remote", Type: ServerTypeHTTP, URL: "https://example.com/mcp", AuthType: AuthTypeOAuth}
		mutate(&srv)
		return srv
	}
	cases := []struct {
		name    string
		server  McpServer
		wantErr bool
	}{
		{"oauth https ok", base(func(*McpServer) {}), false},
		{"oauth loopback http ok", base(func(s *McpServer) { s.URL = "http://127.0.0.1:3845/mcp" }), false},
		{"oauth localhost http ok", base(func(s *McpServer) { s.URL = "http://localhost:3845/mcp" }), false},
		{"oauth plain http rejected", base(func(s *McpServer) { s.URL = "http://example.com/mcp" }), true},
		{"oauth with client id", base(func(s *McpServer) { s.OAuthClientID = "abc" }), false},
		{"oauth with id and secret", base(func(s *McpServer) { s.OAuthClientID = "abc"; s.OAuthClientSecret = "s" }), false},
		{"oauth secret without id", base(func(s *McpServer) { s.OAuthClientSecret = "s" }), true},
		{"oauth client name max ok", base(func(s *McpServer) { s.OAuthClientName = strings.Repeat("n", MaxOAuthClientNameLen) }), false},
		{"oauth client name too long", base(func(s *McpServer) { s.OAuthClientName = strings.Repeat("n", MaxOAuthClientNameLen+1) }), true},
		{"unknown auth type", base(func(s *McpServer) { s.AuthType = "basic" }), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.server.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() err=%v, wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestMcpServerNormalizeOAuth(t *testing.T) {
	// stdio 没有 OAuth：相关字段一律清空。
	stdio := McpServer{Name: "a", Type: ServerTypeStdio, Command: "c", AuthType: AuthTypeOAuth, OAuthClientName: "x", OAuthClientID: "id"}
	stdio.Normalize()
	if stdio.AuthType != AuthTypeNone || stdio.OAuthClientName != "" || stdio.OAuthClientID != "" {
		t.Fatalf("stdio 应清理 OAuth 字段: %+v", stdio)
	}

	// 认证方式不是 oauth 时不保留客户端身份，避免切换回来时带着过期的值。
	plain := McpServer{Name: "b", Type: ServerTypeHTTP, URL: "https://x/mcp", OAuthClientName: "x", OAuthClientID: "id", OAuthClientSecret: "s"}
	plain.Normalize()
	if plain.OAuthClientName != "" || plain.OAuthClientID != "" || plain.OAuthClientSecret != "" {
		t.Fatalf("非 oauth 应清理客户端身份: %+v", plain)
	}

	oauth := McpServer{Name: "c", Type: ServerTypeHTTP, URL: "https://x/mcp", AuthType: AuthTypeOAuth, OAuthClientName: "  Mine  ", OAuthClientID: " id "}
	oauth.Normalize()
	if oauth.OAuthClientName != "Mine" || oauth.OAuthClientID != "id" {
		t.Fatalf("应去除首尾空白: %+v", oauth)
	}
	if oauth.EffectiveOAuthClientName() != "Mine" {
		t.Fatalf("自定义名称应生效: %q", oauth.EffectiveOAuthClientName())
	}
	oauth.OAuthClientName = ""
	if oauth.EffectiveOAuthClientName() != DefaultOAuthClientName {
		t.Fatalf("留空应回落到默认名称: %q", oauth.EffectiveOAuthClientName())
	}
}

func TestFingerprintOAuth(t *testing.T) {
	// 未启用 OAuth 的既有配置：指纹与新增字段之前保持一致，升级不会触发重连。
	legacy := McpServer{Name: "s", Type: ServerTypeHTTP, URL: "https://x/mcp", TimeoutSec: 30}
	// 该值取自引入 OAuth 字段之前的实现。
	const legacyFingerprint = "3b91a623590b28c419b987fe2088cd4d"
	if got := legacy.Fingerprint(); got != legacyFingerprint {
		t.Fatalf("既有配置的指纹不应变化: got %s want %s", got, legacyFingerprint)
	}

	oauth := legacy
	oauth.AuthType = AuthTypeOAuth
	if oauth.Fingerprint() == legacy.Fingerprint() {
		t.Fatal("切换认证方式指纹应变化")
	}
	renamed := oauth
	renamed.OAuthClientName = "Other"
	if renamed.Fingerprint() == oauth.Fingerprint() {
		t.Fatal("客户端名称变化指纹应变化")
	}
	withID := oauth
	withID.OAuthClientID = "id"
	if withID.Fingerprint() == oauth.Fingerprint() {
		t.Fatal("Client ID 变化指纹应变化")
	}
}

func TestOAuthIdentityEquals(t *testing.T) {
	a := McpServer{Name: "s", Type: ServerTypeHTTP, URL: "https://x/mcp", AuthType: AuthTypeOAuth, TimeoutSec: 30}
	b := a
	b.TimeoutSec = 60
	b.Name = "renamed"
	if !a.OAuthIdentityEquals(b) {
		t.Fatal("超时与名称不属于 OAuth 身份")
	}
	for name, mutate := range map[string]func(*McpServer){
		"url":         func(s *McpServer) { s.URL = "https://y/mcp" },
		"auth type":   func(s *McpServer) { s.AuthType = AuthTypeNone },
		"client name": func(s *McpServer) { s.OAuthClientName = "n" },
		"client id":   func(s *McpServer) { s.OAuthClientID = "id" },
		"secret":      func(s *McpServer) { s.OAuthClientSecret = "x" },
	} {
		c := a
		mutate(&c)
		if a.OAuthIdentityEquals(c) {
			t.Fatalf("%s 变化应视为不同身份", name)
		}
	}
}
