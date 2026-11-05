package relay

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	observabilitydomain "portico/internal/observability/domain"
	routingdomain "portico/internal/routing/domain"
	"portico/internal/shared/kernel"
)

func (s *Server) readRequestBody(c *gin.Context) ([]byte, error) {
	if c.Request.Body == nil {
		return nil, nil
	}
	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil, err
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(data))
	return data, nil
}

func (s *Server) registerRoutes(router gin.IRouter) {
	router.Use(s.inboundAuthMiddleware())
	router.POST("/v1/messages", s.proxyHandler(kernel.PlatformClaude, "/v1/messages"))
	router.POST("/responses", s.proxyHandler(kernel.PlatformCodex, "/responses"))
	router.POST("/responses/compact", s.proxyHandler(kernel.PlatformCodex, "/responses/compact"))
	router.POST("/gemini/v1beta/*proxyPath", s.proxyHandler(kernel.PlatformGemini, ""))
	router.POST("/gemini/v1/*proxyPath", s.proxyHandler(kernel.PlatformGemini, ""))
	if s.mcpGateway != nil {
		// MCP streamable HTTP 使用 POST（消息）/GET（SSE 流）/DELETE（会话终止）；
		// SDK handler 自管 body 与 SSE 写出，不走 readRequestBody/proxyHandler。
		router.Any("/mcp", gin.WrapH(s.mcpGateway.Handler()))
	}
}

func (s *Server) proxyHandler(platform kernel.Platform, defaultEndpoint string) gin.HandlerFunc {
	handler := GetPlatformHandler(platform, defaultEndpoint)
	return func(c *gin.Context) {
		bodyBytes, err := s.readRequestBody(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
		query := flattenQuery(c.Request.URL.Query())
		headers := cloneHeaders(c.Request.Header)
		requestMeta := handler.ExtractRequestMeta(c.Request.URL.Path, bodyBytes, query, headers)
		if requestMeta.RequestedModel == "" {
			relayDebugf("request missing model: platform=%s path=%s", platform, c.Request.URL.Path)
		}
		profile, err := s.loadProfile(platform.String())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		ctx := &RelayContext{
			GinCtx:      c,
			Platform:    platform,
			BodyBytes:   bodyBytes,
			Query:       query,
			Headers:     headers,
			Profile:     profile,
			RequestMeta: requestMeta,
		}
		if ok, err := s.routeToManualProvider(ctx); !ok {
			// 响应已开始写出（SSE 中途失败，或上游错误已原样透传）时不能
			// 再追加 JSON——那会往已发出的字节流里插入非法内容。
			if c.Writer.Written() {
				c.Abort()
				return
			}
			errorMsg := "路由失败"
			if err != nil {
				errorMsg = err.Error()
			}
			c.JSON(http.StatusBadGateway, gin.H{"error": errorMsg})
		}
	}
}

// inboundCredentialHeaderNames 是客户端可能携带的认证/敏感头：其中要么是
// 本地占位 token，要么是用户环境里的真实密钥，任何一种都不能转发给上游。
var inboundCredentialHeaderNames = []string{"Authorization", "X-Api-Key", "X-Goog-Api-Key", "Cookie"}

func stripInboundCredentialHeaders(headers map[string]string) {
	for key := range headers {
		for _, name := range inboundCredentialHeaderNames {
			if strings.EqualFold(key, name) {
				delete(headers, key)
				break
			}
		}
	}
}

// applyProviderCredentials 先剥离客户端凭证再无条件注入供应商凭证。
// 旧行为（仅当客户端带 Authorization 才替换）在客户端改发 x-api-key 时
// 会既不注入供应商 key、又把用户真实 key 原样发给第三方。
func applyProviderCredentials(headers map[string]string, platform kernel.Platform, provider *routingdomain.Provider) {
	stripInboundCredentialHeaders(headers)
	if provider.APIKey == "" {
		return
	}
	switch platform {
	case kernel.PlatformGemini:
		headers["X-Goog-Api-Key"] = provider.APIKey
	case kernel.PlatformClaude:
		// Anthropic 官方 API 认 x-api-key，多数中转商认 Authorization：同时提供。
		headers["Authorization"] = "Bearer " + provider.APIKey
		headers["X-Api-Key"] = provider.APIKey
	default:
		headers["Authorization"] = "Bearer " + provider.APIKey
	}
}

func (s *Server) forwardRequest(fwdCtx *ForwardContext) (bool, error) {
	c := fwdCtx.GinCtx
	provider := fwdCtx.Provider
	targetURL := joinURL(provider.APIURL, fwdCtx.Endpoint)
	headers := cloneMap(fwdCtx.Headers)
	applyProviderCredentials(headers, fwdCtx.Platform, provider)
	if _, ok := headers["Accept"]; !ok {
		headers["Accept"] = "application/json"
	}
	if _, ok := headers["Content-Type"]; !ok {
		// xrequest 原来会为带 body 的请求补这个默认值；保留该兼容行为。
		headers["Content-Type"] = "application/json"
	}
	requestLog := &observabilitydomain.RequestLog{
		Platform:  fwdCtx.Platform.String(),
		Provider:  provider.Name,
		Model:     fwdCtx.Model,
		SessionID: fwdCtx.SessionID,
		IsStream:  fwdCtx.IsStream,
		IsFast:    fwdCtx.IsFast,
	}
	start := time.Now()
	defer func() {
		requestLog.DurationSec = time.Since(start).Seconds()
		s.enqueueRequestLog(requestLog)
	}()
	client, err := s.upstreamClientFor(upstreamClientOptions{
		proxyURL:           s.getProxyURLForProvider(provider),
		insecureSkipVerify: provider.InsecureSkipVerify,
	})
	if err != nil {
		return false, fmt.Errorf("invalid proxy URL: %w", err)
	}
	// 使用客户端请求 context，让断开/取消立即取消上游请求；不设置整体超时，
	// 避免掐断超长 SSE 流。直接使用共享 client，避免第三方库为每个请求构造
	// 完整 curl 调试串（其中包含请求体和凭证）。
	req, err := http.NewRequestWithContext(
		c.Request.Context(),
		http.MethodPost,
		targetURL,
		bytes.NewReader(fwdCtx.BodyBytes),
	)
	if err != nil {
		return false, fmt.Errorf("创建上游请求失败: %w", err)
	}
	if fwdCtx.Query != nil {
		query := req.URL.Query()
		for key, value := range fwdCtx.Query {
			query.Add(key, value)
		}
		req.URL.RawQuery = query.Encode()
	}
	for key, value := range headers {
		req.Header[key] = []string{value}
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("请求上游失败: %w", err)
	}
	if resp == nil {
		return false, fmt.Errorf("empty response")
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	status := resp.StatusCode
	requestLog.HttpCode = status
	if status >= http.StatusOK && status < http.StatusMultipleChoices {
		responseHook := withFirstTokenTiming(start, requestLog, s.requestLogHook(fwdCtx.Platform, requestLog))
		var codexOutcome *codexStreamOutcome
		if fwdCtx.Platform == kernel.PlatformCodex {
			codexOutcome = &codexStreamOutcome{}
			logHook := responseHook
			responseHook = func(data []byte) (bool, []byte) {
				codexOutcome.Observe(data)
				return logHook(data)
			}
		}
		_, copyErr := writeUpstreamResponse(c.Writer, resp, responseHook)
		if codexOutcome != nil {
			switch codexOutcome.Status() {
			case codexStreamCompleted:
				// Codex 把 response.completed 当作业务终点，可能在收到该事件后
				// 立即关闭下游连接，不再等待 HTTP chunked 的 0 chunk。此时
				// 上游读取会因请求 context 传播返回 context.Canceled，但供应商
				// 已经成功服务本次会话，仍应绑定或续期。
				if copyErr == nil {
					return true, nil
				}
				if errors.Is(copyErr, context.Canceled) {
					relayDebugf("Codex 已完成，忽略流收尾取消: %v", copyErr)
					return true, nil
				}
				return false, copyErr
			case codexStreamFailed:
				if copyErr != nil {
					return false, copyErr
				}
				return false, fmt.Errorf("upstream codex stream ended with %s", codexOutcome.EventType())
			}
		}
		return copyErr == nil, copyErr
	}
	// 非 2xx：把上游状态码、响应头与错误体原样透传给客户端（400 的参数
	// 错误详情、429 的限流原因与 Retry-After 都不能丢），同时返回失败，
	// 让会话粘滞不绑定该供应商。错误体另经 collector 截断后打进应用日志，
	// 否则 UI 里只剩一个状态码，排查不到真实原因。
	var errBody upstreamErrorCollector
	if _, copyErr := writeBufferedUpstreamResponse(c.Writer, resp, errBody.hook); copyErr != nil {
		relayWarnf("透传上游错误响应失败: %v", copyErr)
	}
	errBody.logWith(provider.Name, fwdCtx.Model, status)
	return false, fmt.Errorf("upstream status %d", status)
}

func joinURL(base string, endpoint string) string {
	base = strings.TrimSuffix(base, "/")
	endpoint = "/" + strings.TrimPrefix(endpoint, "/")
	return base + endpoint
}
