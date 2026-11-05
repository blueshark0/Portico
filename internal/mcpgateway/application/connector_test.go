package application

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"portico/internal/mcpgateway/domain"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// 重定向往不同 host 时不得重放凭证头（Go 的跨域剥离发生在 transport 之前，
// 只能靠 transport 自己判断目标 host）。
func TestStaticHeaderRoundTripperRedirectSafety(t *testing.T) {
	var targetSawAuth atomic.Value
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetSawAuth.Store(r.Header.Get("Authorization"))
		_, _ = w.Write([]byte("ok"))
	}))
	defer target.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, target.URL, http.StatusFound)
			return
		}
		// 同源路径：应带上注入的凭证
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("同源请求应注入 Authorization")
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer redirector.Close()

	client := &http.Client{Transport: newStaticHeaderRoundTripper(redirector.URL, map[string]string{
		"Authorization": "Bearer secret",
	})}

	resp, err := client.Get(redirector.URL + "/same")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	resp, err = client.Get(redirector.URL + "/redirect")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	got, _ := targetSawAuth.Load().(string)
	if got != "" {
		t.Fatalf("跨 host 重定向不应携带凭证，实际: %q", got)
	}
}

// 上游发出 tools/list_changed 时，connector 的回调应携带正确的 serverID。
func TestDefaultConnectorListChangedCallback(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "up", Version: "v0"}, &mcp.ServerOptions{HasTools: true})
	ts := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil))
	defer ts.Close()

	var notified atomic.Int64
	connect := defaultConnector("test", func(id int64) { notified.Store(id) })
	cfg := domain.McpServer{ID: 42, Name: "up", Type: domain.ServerTypeHTTP, URL: ts.URL}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	session, err := connect(ctx, cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	server.AddTool(&mcp.Tool{Name: "late", InputSchema: map[string]any{"type": "object"}},
		func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{}, nil
		})

	deadline := time.Now().Add(5 * time.Second)
	for notified.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if notified.Load() != 42 {
		t.Fatalf("应收到 serverID=42 的 list_changed 回调，实际 %d", notified.Load())
	}
}
