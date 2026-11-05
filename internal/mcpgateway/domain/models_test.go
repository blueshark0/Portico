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
