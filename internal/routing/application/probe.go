package application

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"portico/internal/routing/domain"
)

// ProviderProbeResult 供应商连通性探测结果。
type ProviderProbeResult struct {
	Success    bool   `json:"success"`
	HTTPStatus int    `json:"httpStatus"`
	DurationMs int64  `json:"durationMs"`
	Error      string `json:"error,omitempty"`
}

const providerProbeTimeout = 10 * time.Second

// ProbeProvider 以供应商的代理/TLS 设置对其 API Base URL 做一次 GET 探测。
// 只验证网络可达性（DNS/拨号/TLS/首个响应头），不携带凭证、不消耗 API 额度：
// 上游返回任何 HTTP 状态码（含 401/403/404）都视为网络层连通；仅网络错误判失败。
func (s *Service) ProbeProvider(ctx context.Context, provider domain.Provider) (ProviderProbeResult, error) {
	apiURL := provider.APIURL
	if apiURL == "" {
		return ProviderProbeResult{Success: false, Error: "API Base URL 为空"}, nil
	}
	if _, err := url.ParseRequestURI(apiURL); err != nil {
		return ProviderProbeResult{Success: false, Error: fmt.Sprintf("API Base URL 无效: %v", err)}, nil
	}

	preferences, err := s.repo.GetAppPreferences(ctx)
	if err != nil {
		return ProviderProbeResult{}, err
	}
	proxyURL := provider.ResolveProxyURL(preferences.ProxyEnabled, preferences.ProxyURL)

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   providerProbeTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   providerProbeTimeout,
		ResponseHeaderTimeout: providerProbeTimeout,
	}
	if proxyURL != "" {
		parsed, err := url.Parse(proxyURL)
		if err != nil {
			return ProviderProbeResult{Success: false, Error: fmt.Sprintf("代理地址无效: %v", err)}, nil
		}
		transport.Proxy = http.ProxyURL(parsed)
	}
	if provider.InsecureSkipVerify {
		// 仅当 provider 显式开启时才跳过证书校验（如本地抓包代理调试）。
		// #nosec G402 -- provider 级显式开关。
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	client := &http.Client{Transport: transport, Timeout: providerProbeTimeout}
	defer transport.CloseIdleConnections()

	ctx, cancel := context.WithTimeout(ctx, providerProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return ProviderProbeResult{Success: false, Error: err.Error()}, nil
	}

	start := time.Now()
	resp, err := client.Do(req)
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		return ProviderProbeResult{Success: false, DurationMs: durationMs, Error: err.Error()}, nil
	}
	defer func() { _ = resp.Body.Close() }()
	return ProviderProbeResult{Success: true, HTTPStatus: resp.StatusCode, DurationMs: durationMs}, nil
}
