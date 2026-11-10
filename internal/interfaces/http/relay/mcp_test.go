package relay

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type stubMcpGateway struct {
	handler http.Handler
	closed  bool
}

func (g *stubMcpGateway) Handler() http.Handler       { return g.handler }
func (g *stubMcpGateway) Close(context.Context) error { g.closed = true; return nil }

type bearerRoundTripper struct{}

func (bearerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.Header.Set("Authorization", "Bearer portico")
	return http.DefaultTransport.RoundTrip(cloned)
}

func newStubMcpGateway() *stubMcpGateway {
	server := mcp.NewServer(&mcp.Implementation{Name: "stub", Version: "v0"}, &mcp.ServerOptions{HasTools: true})
	server.AddTool(&mcp.Tool{Name: "ping", InputSchema: map[string]any{"type": "object"}},
		func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "pong"}}}, nil
		})
	return &stubMcpGateway{handler: mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)}
}

func TestMcpRouteMountedWithAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gateway := newStubMcpGateway()
	server := &Server{mcpGateway: gateway}
	router := gin.New()
	server.registerRoutes(router)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// 无 token → 401（入站鉴权中间件拦在 MCP handler 之前）
	resp, err := http.Post(ts.URL+"/mcp", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("无 token 应 401，实际 %d", resp.StatusCode)
	}

	// 带占位 token → 完成 initialize + tools/list + tools/call 全链路
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   ts.URL + "/mcp",
		HTTPClient: &http.Client{Transport: bearerRoundTripper{}},
	}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools.Tools) != 1 || tools.Tools[0].Name != "ping" {
		t.Fatalf("tools: %+v", tools.Tools)
	}
	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "ping"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if got := res.Content[0].(*mcp.TextContent).Text; got != "pong" {
		t.Fatalf("got %q", got)
	}
}

func TestMcpRouteAbsentWhenGatewayNil(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := &Server{}
	router := gin.New()
	server.registerRoutes(router)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer portico")
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("未装配网关时 /mcp 应 404，实际 %d", recorder.Code)
	}
}

// OAuth 回调由浏览器跳转触发，带不了占位 token：它必须绕过入站鉴权，
// 而其余路由的鉴权不能因此松动。
func TestMcpOAuthCallbackBypassesInboundAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var gotQuery string
	server := &Server{
		mcpGateway: newStubMcpGateway(),
		mcpOAuthCallback: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotQuery = r.URL.RawQuery
			w.WriteHeader(http.StatusNoContent)
		}),
	}
	router := gin.New()
	server.registerRoutes(router)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, MCPOAuthCallbackPath+"?code=c&state=s", nil))
	if recorder.Code != http.StatusNoContent || gotQuery != "code=c&state=s" {
		t.Fatalf("回调应无需 token 直达 handler，实际 %d query=%q", recorder.Code, gotQuery)
	}

	for _, probe := range []struct{ method, path string }{
		{http.MethodPost, "/mcp"},
		{http.MethodGet, "/mcp"},
		{http.MethodPost, "/v1/messages"},
		{http.MethodPost, "/responses"},
	} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(probe.method, probe.path, nil))
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s 无 token 应 401，实际 %d", probe.method, probe.path, recorder.Code)
		}
	}

	// 回调只接受 GET，其他方法不会落到 handler 上。
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, MCPOAuthCallbackPath, nil))
	if recorder.Code == http.StatusNoContent {
		t.Fatal("回调路由不应接受 POST")
	}
}

func TestMcpOAuthCallbackAbsentWhenNotConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := &Server{mcpGateway: newStubMcpGateway()}
	router := gin.New()
	server.registerRoutes(router)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, MCPOAuthCallbackPath, nil)
	req.Header.Set("Authorization", "Bearer portico")
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("未注入回调时该路由应 404，实际 %d", recorder.Code)
	}
}
