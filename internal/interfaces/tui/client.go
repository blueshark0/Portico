package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"portico/internal/interfaces/wails"
	mcpapp "portico/internal/mcpgateway/application"
	mcpdomain "portico/internal/mcpgateway/domain"
	observabilitydomain "portico/internal/observability/domain"
	platformproxydomain "portico/internal/platformproxy/domain"
	routingapp "portico/internal/routing/application"
	routingdomain "portico/internal/routing/domain"
	"portico/internal/shared/serverinfo"
)

// requestTimeout 需大于后端供应商探测的 10s 超时。
const requestTimeout = 30 * time.Second

// ErrUnauthorized 表示管理接口要求令牌而当前令牌缺失或无效。
var ErrUnauthorized = errors.New("需要有效的访问令牌")

// APIError 是管理接口返回的非 2xx 响应。
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("HTTP %d", e.Status)
	}
	return e.Message
}

// Client 通过 portico-server 的管理 API（/api/rpc）调用后端 facade，
// 与 Web UI 走同一套接口与认证规则。
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: requestTimeout},
	}
}

func (c *Client) BaseURL() string { return c.baseURL }

// Target 是 TUI 连接的后端地址与令牌。
type Target struct {
	URL   string
	Token string
}

// ResolveTarget 按「参数 > 环境变量 > 本机运行时信息 / 令牌文件 > 默认值」确定连接目标。
// 本机令牌文件只在目标为回环地址时使用，避免把本机令牌发给远程服务器。
func ResolveTarget(flagURL, flagToken string) Target {
	target := Target{URL: firstNonEmpty(flagURL, os.Getenv("PORTICO_ADMIN_URL"))}
	if target.URL == "" {
		target.URL = serverinfo.AdminURL(serverinfo.DefaultAdminAddr, false)
		if path, err := serverinfo.RuntimeInfoPath(); err == nil {
			if info, err := serverinfo.ReadRuntimeInfo(path); err == nil && info.AdminURL != "" {
				target.URL = info.AdminURL
			}
		}
	}
	target.Token = firstNonEmpty(flagToken, os.Getenv("PORTICO_ADMIN_TOKEN"))
	if target.Token == "" && isLoopbackURL(target.URL) {
		if path, err := serverinfo.TokenPath(); err == nil {
			if data, err := os.ReadFile(path); err == nil {
				target.Token = strings.TrimSpace(string(data))
			}
		}
	}
	return target
}

func isLoopbackURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

// Health 调用免认证的健康检查。
func (c *Client) Health(ctx context.Context) (serverinfo.Health, error) {
	var health serverinfo.Health
	resp, err := c.do(ctx, http.MethodGet, "/api/health", nil)
	if err != nil {
		return health, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return health, readAPIError(resp)
	}
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return health, fmt.Errorf("解析健康检查响应: %w", err)
	}
	return health, nil
}

// CheckSession 验证当前令牌（或本机免令牌）是否可访问管理接口。
func (c *Client) CheckSession(ctx context.Context) error {
	resp, err := c.do(ctx, http.MethodGet, "/api/session", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return readAPIError(resp)
	}
	return nil
}

// Call 调用 "{Facade}.{Method}"，参数按位置编码为 JSON 数组；out 为 nil 时忽略返回值。
func (c *Client) Call(ctx context.Context, method string, out any, args ...any) error {
	if args == nil {
		args = []any{}
	}
	body, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("编码参数: %w", err)
	}
	resp, err := c.do(ctx, http.MethodPost, "/api/rpc/"+method, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return readAPIError(resp)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("解析 %s 响应: %w", method, err)
	}
	return nil
}

func (c *Client) do(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	return c.http.Do(req)
}

func readAPIError(resp *http.Response) error {
	if resp.StatusCode == http.StatusUnauthorized {
		return ErrUnauthorized
	}
	var payload struct {
		Error string `json:"error"`
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	_ = json.Unmarshal(data, &payload)
	return &APIError{Status: resp.StatusCode, Message: payload.Error}
}

// ---- 类型化封装：与前端 services/*.ts 使用的 facade 方法一一对应 ----

func (c *Client) PlatformStatus(ctx context.Context, platform string) (platformproxydomain.Status, error) {
	var out platformproxydomain.Status
	err := c.Call(ctx, "PlatformProxyFacade.GetStatus", &out, platform)
	return out, err
}

func (c *Client) SetPlatformProxy(ctx context.Context, platform string, enabled bool) error {
	method := "PlatformProxyFacade.Disable"
	if enabled {
		method = "PlatformProxyFacade.Enable"
	}
	return c.Call(ctx, method, nil, platform)
}

func (c *Client) Profile(ctx context.Context, platform string) (routingdomain.RouteProfile, error) {
	var out routingdomain.RouteProfile
	err := c.Call(ctx, "RoutingFacade.GetProfile", &out, platform)
	return out, err
}

func (c *Client) SaveProfile(ctx context.Context, profile routingdomain.RouteProfile) (routingdomain.RouteProfile, error) {
	var out routingdomain.RouteProfile
	err := c.Call(ctx, "RoutingFacade.SaveProfile", &out, profile)
	return out, err
}

func (c *Client) TestProvider(ctx context.Context, provider routingdomain.Provider) (routingapp.ProviderProbeResult, error) {
	var out routingapp.ProviderProbeResult
	err := c.Call(ctx, "RoutingFacade.TestProvider", &out, provider)
	return out, err
}

// Stats 返回统计（含失败请求，httpStatus 固定为 all）。
func (c *Client) Stats(ctx context.Context, platform, rangeKey string) (observabilitydomain.LogStats, error) {
	var out observabilitydomain.LogStats
	err := c.Call(ctx, "ObservabilityFacade.StatsSince", &out, platform, "", rangeKey, "", "all")
	return out, err
}

func (c *Client) RequestLogs(ctx context.Context, platform, rangeKey, httpStatus string, page, pageSize int) (observabilitydomain.RequestLogPage, error) {
	var out observabilitydomain.RequestLogPage
	err := c.Call(ctx, "ObservabilityFacade.ListRequestLogs", &out, platform, "", rangeKey, "", httpStatus, page, pageSize)
	return out, err
}

func (c *Client) Sessions(ctx context.Context, platform string) ([]wails.SessionSummary, error) {
	var out []wails.SessionSummary
	err := c.Call(ctx, "SessionFacade.ListByPlatform", &out, platform)
	return out, err
}

func (c *Client) UnbindSession(ctx context.Context, platform, sessionID string) error {
	return c.Call(ctx, "SessionFacade.Unbind", nil, platform, sessionID)
}

func (c *Client) McpServers(ctx context.Context) ([]mcpdomain.McpServer, error) {
	var out []mcpdomain.McpServer
	err := c.Call(ctx, "McpGatewayFacade.List", &out)
	return out, err
}

func (c *Client) McpStatuses(ctx context.Context) ([]mcpapp.UpstreamStatus, error) {
	var out []mcpapp.UpstreamStatus
	err := c.Call(ctx, "McpGatewayFacade.Statuses", &out)
	return out, err
}

func (c *Client) McpEndpoint(ctx context.Context) (string, error) {
	var out string
	err := c.Call(ctx, "McpGatewayFacade.Endpoint", &out)
	return out, err
}

func (c *Client) McpSetEnabled(ctx context.Context, id int64, enabled bool) error {
	return c.Call(ctx, "McpGatewayFacade.SetEnabled", nil, id, enabled)
}

func (c *Client) McpInjection(ctx context.Context, platform string) (bool, error) {
	var out bool
	err := c.Call(ctx, "McpGatewayFacade.GetInjectionStatus", &out, platform)
	return out, err
}

func (c *Client) McpSetInjection(ctx context.Context, platform string, inject bool) error {
	method := "McpGatewayFacade.RemoveInjection"
	if inject {
		method = "McpGatewayFacade.Inject"
	}
	return c.Call(ctx, method, nil, platform)
}

func (c *Client) TailAppLog(ctx context.Context, offset int64, maxLines int, level, keyword string) (observabilitydomain.AppLogChunk, error) {
	var out observabilitydomain.AppLogChunk
	err := c.Call(ctx, "AppLogFacade.TailAppLog", &out, offset, maxLines, level, keyword)
	return out, err
}
