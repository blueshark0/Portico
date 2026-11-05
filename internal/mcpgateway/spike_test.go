package mcpgateway

// Phase 0 spike：验证 go-sdk v1.8.0 的关键行为假设。
// 正式实现落地后，其中仍有价值的断言会迁移到 application 层的正式测试。

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// spike 1：gin.WrapH 挂载 + 客户端自连 initialize/tools/list；
// 同时验证 SupportedProtocolVersions 收窄与 SessionTimeout 字段可用。
func TestSpikeGinMountAndSelfConnect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := mcp.NewServer(&mcp.Implementation{Name: "spike", Version: "v0"}, &mcp.ServerOptions{
		HasTools: true,
		SupportedProtocolVersions: []string{
			"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05",
		},
	})
	server.AddTool(&mcp.Tool{Name: "echo", Description: "echo", InputSchema: map[string]any{"type": "object"}},
		func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: string(req.Params.Arguments)}},
			}, nil
		})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{SessionTimeout: 30 * time.Minute})

	r := gin.New()
	r.Any("/mcp", gin.WrapH(handler))
	ts := httptest.NewServer(r)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "spike-client", Version: "v0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: ts.URL + "/mcp"}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(tools.Tools) != 1 || tools.Tools[0].Name != "echo" {
		t.Fatalf("unexpected tools: %+v", tools.Tools)
	}

	// spike 2：CallToolParamsRaw.Arguments(json.RawMessage) 原样转发给上游 CallToolParams.Arguments(any)
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "echo",
		Arguments: json.RawMessage(`{"a":1,"b":[true,null]}`),
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	got := res.Content[0].(*mcp.TextContent).Text
	if got != `{"a":1,"b":[true,null]}` {
		t.Fatalf("arguments not forwarded verbatim: %s", got)
	}
}

// spike 3：stateful 模式下 AddTool 触发已连接客户端的 ToolListChangedHandler。
func TestSpikeListChangedNotification(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "spike", Version: "v0"},
		&mcp.ServerOptions{HasTools: true})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	var notified atomic.Int32
	client := mcp.NewClient(&mcp.Implementation{Name: "spike-client", Version: "v0"}, &mcp.ClientOptions{
		ToolListChangedHandler: func(context.Context, *mcp.ToolListChangedRequest) {
			notified.Add(1)
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: ts.URL}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	server.AddTool(&mcp.Tool{Name: "late", Description: "added after connect", InputSchema: map[string]any{"type": "object"}},
		func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{}, nil
		})

	deadline := time.Now().Add(5 * time.Second)
	for notified.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if notified.Load() == 0 {
		t.Fatal("client did not receive tools/list_changed after AddTool")
	}
}
