package application

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"portico/internal/mcpgateway/domain"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// defaultConnector 按类型构造真实上游连接。onToolsChanged 在上游发出
// notifications/tools/list_changed 时回调（nil 则不订阅）。
func defaultConnector(appVersion string, onToolsChanged func(serverID int64)) Connector {
	if appVersion == "" {
		appVersion = "dev"
	}
	return func(ctx context.Context, cfg domain.McpServer) (UpstreamSession, error) {
		var opts *mcp.ClientOptions
		if onToolsChanged != nil {
			opts = &mcp.ClientOptions{
				ToolListChangedHandler: func(context.Context, *mcp.ToolListChangedRequest) {
					onToolsChanged(cfg.ID)
				},
			}
		}
		client := mcp.NewClient(&mcp.Implementation{Name: "portico", Version: appVersion}, opts)
		var transport mcp.Transport
		switch cfg.Type {
		case domain.ServerTypeStdio:
			cmd := exec.Command(cfg.Command, cfg.Args...)
			cmd.Env = stdioEnvironment(os.Environ(), cfg.Env)
			transport = &mcp.CommandTransport{Command: cmd}
		case domain.ServerTypeHTTP:
			transport = &mcp.StreamableClientTransport{
				Endpoint:   cfg.URL,
				HTTPClient: &http.Client{Transport: newStaticHeaderRoundTripper(cfg.URL, cfg.Headers)},
			}
		default:
			return nil, fmt.Errorf("未知上游类型: %q", cfg.Type)
		}
		return client.Connect(ctx, transport, nil)
	}
}

func envPairs(env map[string]string) []string {
	pairs := make([]string, 0, len(env))
	for k, v := range env {
		pairs = append(pairs, k+"="+v)
	}
	return pairs
}

// staticHeaderRoundTripper 为上游 HTTP 请求注入静态头（仿官方 examples/server/proxy）。
//
// 只在目标与配置的端点同 host 时注入：Go http.Client 的重定向跨域凭证剥离发生
// 在 transport 之前，若此处无条件重放 Authorization，上游一个 302 就能把凭证
// 引向任意域。
type staticHeaderRoundTripper struct {
	base       http.RoundTripper
	targetHost string
	headers    map[string]string
}

func newStaticHeaderRoundTripper(endpoint string, headers map[string]string) http.RoundTripper {
	rt := staticHeaderRoundTripper{base: http.DefaultTransport, headers: headers}
	if u, err := url.Parse(endpoint); err == nil {
		rt.targetHost = u.Host
	}
	return rt
}

func (rt staticHeaderRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if len(rt.headers) == 0 || rt.targetHost == "" || !strings.EqualFold(req.URL.Host, rt.targetHost) {
		return rt.base.RoundTrip(req)
	}
	cloned := req.Clone(req.Context())
	for k, v := range rt.headers {
		cloned.Header.Set(k, v)
	}
	return rt.base.RoundTrip(cloned)
}
