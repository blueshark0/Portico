package relay

// 透传回归测试：锁定 relay 对 Claude Code auto-mode 服务端分类器机制的透明性。
// 该机制要求网关原样转发请求头与请求体字段（含未知字段如 safeguards），
// 响应/SSE 事件不得丢 key（如 safeguard_results）或改写 tool-use ID，
// 否则 Claude Code 会退回本地分类器请求并计入用户 token 计费。
// 参见 https://code.claude.com/docs/en/auto-mode-classifier-billing

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	routingapp "portico/internal/routing/application"
	routingdomain "portico/internal/routing/domain"
	"portico/internal/shared/kernel"
)

func TestCloneHeadersJoinsMultiValueHeaders(t *testing.T) {
	// HTTP/1.1 语义上同名多值请求头（Cookie 除外，而 Cookie 在转发前会被剥离）
	// 等价于逗号连接的单值头；anthropic-beta 这类 list-valued 头不允许丢值。
	cloned := cloneHeaders(http.Header{
		"Anthropic-Beta": []string{"tools-2025", "fine-grained"},
		"User-Agent":     []string{"claude-cli/2.1.281"},
		"X-Empty":        []string{},
	})
	if got := cloned["Anthropic-Beta"]; got != "tools-2025, fine-grained" {
		t.Fatalf("multi-value header = %q, want comma-joined", got)
	}
	if got := cloned["User-Agent"]; got != "claude-cli/2.1.281" {
		t.Fatalf("single-value header = %q, want unchanged", got)
	}
	if _, ok := cloned["X-Empty"]; ok {
		t.Fatal("header with no values must be skipped")
	}
}

func TestRouteToManualProviderPreservesUnknownBodyFieldsByteIdentical(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var gotBody []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	server := &Server{}
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	ginCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	// 刻意使用乱序键、非常规空白与未知字段：auto-mode 的 safeguards
	// 字段与未来新增字段都必须逐字节透传到上游。
	body := []byte(`{ "safeguards" : [ {"type":"command" , "cmd":"rm -rf /tmp/x"} ],` +
		`"future_unknown_field":{"nested":[1,2,3]},"model":"claude-sonnet-4",` +
		`"stream":true,"metadata":{"user_id":"u1"}}`)

	ok, err := server.routeToManualProvider(&RelayContext{
		GinCtx:    ginCtx,
		Platform:  kernel.PlatformClaude,
		BodyBytes: body,
		Query:     map[string]string{},
		Headers:   map[string]string{"Accept": "application/json"},
		Profile: routingdomain.RouteProfile{
			Platform:          kernel.PlatformClaude,
			DefaultProviderID: intPtr(1),
			// 无 ModelMapping / RequestBodyReplacements：body 不应经过任何改写。
			Providers: []routingdomain.Provider{
				{ID: 1, Name: "p", APIURL: upstream.URL, APIKey: "sk-provider", Position: 1},
			},
		},
		RequestMeta: RequestMeta{
			Endpoint:          "/v1/messages",
			RequestedModel:    "claude-sonnet-4",
			SessionID:         "u1",
			IsStream:          true,
			BodyHasModelField: true,
		},
	})
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if !ok {
		t.Fatal("expected route to succeed")
	}
	if !bytes.Equal(gotBody, body) {
		t.Fatalf("upstream body not byte-identical:\n got: %s\nwant: %s", gotBody, body)
	}
}

func TestForwardRequestPassthroughSSEByteIdentical(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 负载覆盖：safeguard_results、SSE 注释行、空行、>1024 字节的长行
	// （xrequest 的 Peek(1024) 边界）以及末尾不带换行的一行（EOF 分支）。
	padding := strings.Repeat("y", 1500)
	payload := "event: message_start\n" +
		`data: {"type":"message_start","message":{"id":"msg_1"},"safeguard_results":[{"category":"cyber","decision":"allow"}]}` + "\n" +
		"\n" +
		": keep-alive\n" +
		"\n" +
		"event: content_block_delta\n" +
		`data: {"type":"content_block_delta","delta":{"text":"` + padding + `"}}` + "\n" +
		"\n" +
		`data: {"type":"message_stop","safeguard_results":[{"category":"cyber","decision":"allow"}]}`

	server := &Server{
		upstreamClients: map[upstreamClientOptions]*http.Client{
			{}: {Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header: http.Header{
						"Content-Type":         []string{"text/event-stream"},
						"X-Safeguard-Decision": []string{"allow"},
					},
					Body:    io.NopCloser(strings.NewReader(payload)),
					Request: req,
				}, nil
			})},
		},
	}
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	ginCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	ok, err := server.routeToManualProvider(&RelayContext{
		GinCtx:    ginCtx,
		Platform:  kernel.PlatformClaude,
		BodyBytes: []byte(`{"model":"claude-sonnet-4","stream":true}`),
		Query:     map[string]string{},
		Headers:   map[string]string{"Accept": "text/event-stream"},
		Profile: routingdomain.RouteProfile{
			Platform:          kernel.PlatformClaude,
			DefaultProviderID: intPtr(1),
			Providers: []routingdomain.Provider{
				{ID: 1, Name: "p", APIURL: "https://provider.example", APIKey: "sk-provider", Position: 1},
			},
		},
		RequestMeta: RequestMeta{
			Endpoint:          "/v1/messages",
			RequestedModel:    "claude-sonnet-4",
			IsStream:          true,
			BodyHasModelField: true,
		},
	})
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if !ok {
		t.Fatal("expected SSE route to succeed")
	}
	if got := recorder.Body.Bytes(); !bytes.Equal(got, []byte(payload)) {
		t.Fatalf("SSE stream not byte-identical:\n got: %q\nwant: %q", got, payload)
	}
	if got := recorder.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}
	if got := recorder.Header().Get("X-Safeguard-Decision"); got != "allow" {
		t.Fatalf("custom response header lost: %q", got)
	}
}

func TestForwardRequestForwardsCustomHeadersToUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		headers    map[string]string
		wantAccept string
	}{
		{
			name: "client accept preserved",
			headers: map[string]string{
				"Accept":              "text/event-stream",
				"Anthropic-Beta":      "tools-2025, fine-grained",
				"X-Stainless-Timeout": "600",
				"User-Agent":          "claude-cli/2.1.281",
			},
			wantAccept: "text/event-stream",
		},
		{
			name:       "default accept injected when absent",
			headers:    map[string]string{"Anthropic-Beta": "tools-2025"},
			wantAccept: "application/json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotHeaders http.Header
			server := &Server{
				upstreamClients: map[upstreamClientOptions]*http.Client{
					{}: {Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
						gotHeaders = req.Header.Clone()
						return &http.Response{
							StatusCode: http.StatusOK,
							Status:     "200 OK",
							Header:     http.Header{"Content-Type": []string{"application/json"}},
							Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
							Request:    req,
						}, nil
					})},
				},
			}
			recorder := httptest.NewRecorder()
			ginCtx, _ := gin.CreateTestContext(recorder)
			ginCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

			ok, err := server.routeToManualProvider(&RelayContext{
				GinCtx:    ginCtx,
				Platform:  kernel.PlatformClaude,
				BodyBytes: []byte(`{"model":"claude-sonnet-4"}`),
				Query:     map[string]string{},
				Headers:   tt.headers,
				Profile: routingdomain.RouteProfile{
					Platform:          kernel.PlatformClaude,
					DefaultProviderID: intPtr(1),
					Providers: []routingdomain.Provider{
						{ID: 1, Name: "p", APIURL: "https://provider.example", APIKey: "sk-provider", Position: 1},
					},
				},
				RequestMeta: RequestMeta{
					Endpoint:          "/v1/messages",
					RequestedModel:    "claude-sonnet-4",
					BodyHasModelField: true,
				},
			})
			if err != nil {
				t.Fatalf("route: %v", err)
			}
			if !ok {
				t.Fatal("expected route to succeed")
			}
			// 网关兼容性要求：客户端自定义头必须原样到达上游。
			if got := gotHeaders.Get("Anthropic-Beta"); got != tt.headers["Anthropic-Beta"] {
				t.Fatalf("Anthropic-Beta = %q, want %q", got, tt.headers["Anthropic-Beta"])
			}
			if want := tt.headers["X-Stainless-Timeout"]; want != "" && gotHeaders.Get("X-Stainless-Timeout") != want {
				t.Fatalf("X-Stainless-Timeout = %q, want %q", gotHeaders.Get("X-Stainless-Timeout"), want)
			}
			if want := tt.headers["User-Agent"]; want != "" && gotHeaders.Get("User-Agent") != want {
				t.Fatalf("User-Agent = %q, want %q", gotHeaders.Get("User-Agent"), want)
			}
			if got := gotHeaders.Get("Accept"); got != tt.wantAccept {
				t.Fatalf("Accept = %q, want %q", got, tt.wantAccept)
			}
		})
	}
}

func TestRelayEndToEndPassthroughThroughGinRouter(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 全链路：readRequestBody → cloneHeaders → flattenQuery → ExtractRequestMeta
	// → 路由 → forward。任何一环引入 body 重序列化或字段剥离都会在此失败。
	var gotBody []byte
	var gotHeaders http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotHeaders = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Safeguard-Decision", "allow")
		_, _ = w.Write([]byte(`{"id":"msg_1","safeguard_results":[{"category":"cyber","decision":"allow"}],"future_field":true}`))
	}))
	defer upstream.Close()

	server := &Server{
		routingService: routingapp.NewService(&relayRoutingRepoStub{
			profiles: map[kernel.Platform]routingdomain.RouteProfile{
				kernel.PlatformClaude: {
					Platform:          kernel.PlatformClaude,
					DefaultProviderID: intPtr(1),
					Providers: []routingdomain.Provider{
						{ID: 1, Name: "p", APIURL: upstream.URL, APIKey: "sk-provider", Position: 1},
					},
				},
			},
		}),
	}
	router := gin.New()
	server.registerRoutes(router)

	body := []byte(`{"model":"claude-sonnet-4","stream":false,"metadata":{"user_id":"u1"},` +
		`"safeguards":[{"type":"command","cmd":"ls"}],"future_unknown_field":{"a":1}}`)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer portico")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Anthropic-Beta", "tools-2025")
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", recorder.Code, recorder.Body.String())
	}
	if !bytes.Equal(gotBody, body) {
		t.Fatalf("upstream body not byte-identical:\n got: %s\nwant: %s", gotBody, body)
	}
	if got := gotHeaders.Get("Anthropic-Beta"); got != "tools-2025" {
		t.Fatalf("Anthropic-Beta not forwarded: %q", got)
	}
	if got := gotHeaders.Get("Authorization"); got != "Bearer sk-provider" {
		t.Fatalf("Authorization = %q, want provider key (placeholder must not leak)", got)
	}
	wantResp := `{"id":"msg_1","safeguard_results":[{"category":"cyber","decision":"allow"}],"future_field":true}`
	if !bytes.Equal(recorder.Body.Bytes(), []byte(wantResp)) {
		t.Fatalf("response body not byte-identical:\n got: %s\nwant: %s", recorder.Body.Bytes(), wantResp)
	}
	if got := recorder.Header().Get("X-Safeguard-Decision"); got != "allow" {
		t.Fatalf("custom response header lost: %q", got)
	}
}

func TestForwardRequestPassthroughJSONResponsePreservesUnknownKeys(t *testing.T) {
	gin.SetMode(gin.TestMode)

	payload := `{"id":"msg_1","safeguard_results":[{"category":"cyber","decision":"allow"}],"unknown_future_key":{"x":[1,2]}}`
	server := &Server{
		upstreamClients: map[upstreamClientOptions]*http.Client{
			{}: {Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header: http.Header{
						"Content-Type":         []string{"application/json"},
						"X-Safeguard-Decision": []string{"allow"},
					},
					Body:    io.NopCloser(strings.NewReader(payload)),
					Request: req,
				}, nil
			})},
		},
	}
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	ginCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	ok, err := server.routeToManualProvider(&RelayContext{
		GinCtx:    ginCtx,
		Platform:  kernel.PlatformClaude,
		BodyBytes: []byte(`{"model":"claude-sonnet-4","stream":false}`),
		Query:     map[string]string{},
		Headers:   map[string]string{"Accept": "application/json"},
		Profile: routingdomain.RouteProfile{
			Platform:          kernel.PlatformClaude,
			DefaultProviderID: intPtr(1),
			Providers: []routingdomain.Provider{
				{ID: 1, Name: "p", APIURL: "https://provider.example", APIKey: "sk-provider", Position: 1},
			},
		},
		RequestMeta: RequestMeta{
			Endpoint:          "/v1/messages",
			RequestedModel:    "claude-sonnet-4",
			BodyHasModelField: true,
		},
	})
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if !ok {
		t.Fatal("expected route to succeed")
	}
	if got := recorder.Body.Bytes(); !bytes.Equal(got, []byte(payload)) {
		t.Fatalf("JSON response not byte-identical:\n got: %s\nwant: %s", got, payload)
	}
	if got := recorder.Header().Get("X-Safeguard-Decision"); got != "allow" {
		t.Fatalf("custom response header lost: %q", got)
	}
}
