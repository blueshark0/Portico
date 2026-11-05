package relay

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// upstreamClientOptions 决定复用哪个共享 client：相同 proxyURL 与 TLS 选项
// 组合复用同一个 http.Client，使 keep-alive 连接得以复用。每请求新建
// Transport 会导致连接零复用、goroutine 与 TCP 连接随请求速率线性泄漏。
type upstreamClientOptions struct {
	proxyURL           string
	insecureSkipVerify bool
}

func (s *Server) upstreamClientFor(opts upstreamClientOptions) (*http.Client, error) {
	s.upstreamMu.Lock()
	defer s.upstreamMu.Unlock()
	if client, ok := s.upstreamClients[opts]; ok {
		return client, nil
	}
	transport := newUpstreamTransport()
	if opts.proxyURL != "" {
		proxyURL, err := url.Parse(opts.proxyURL)
		if err != nil {
			return nil, err
		}
		transport.Proxy = relayProxyWithNoProxy(proxyURL)
	}
	if opts.insecureSkipVerify {
		// 仅当 provider 显式开启 insecureSkipVerify 时才跳过证书校验
		// （如本地抓包代理调试），绝不因"配置了代理"而全局关闭。
		// #nosec G402 -- provider 级显式开关。
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	// 不设置 client.Timeout：整体超时会掐断超长的 SSE 流（此前 900s 上限
	// 会截断超过 15 分钟的流），请求取消由客户端 context 传播驱动，
	// 各阶段期限由 Transport 的拨号/TLS/响应头超时控制。
	client := &http.Client{Transport: transport}
	if s.upstreamClients == nil {
		s.upstreamClients = make(map[upstreamClientOptions]*http.Client)
	}
	s.upstreamClients[opts] = client
	return client, nil
}

// resetUpstreamClients 在代理配置变更后丢弃缓存的 client，关闭其空闲连接。
func (s *Server) resetUpstreamClients() {
	s.upstreamMu.Lock()
	clients := s.upstreamClients
	s.upstreamClients = nil
	s.upstreamMu.Unlock()
	for _, client := range clients {
		if transport, ok := client.Transport.(*http.Transport); ok {
			transport.CloseIdleConnections()
		}
	}
}

func newUpstreamTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 300 * time.Second,
	}
}

func relayProxyWithNoProxy(proxyURL *url.URL) func(*http.Request) (*url.URL, error) {
	noProxy := relayNoProxyList()
	return func(req *http.Request) (*url.URL, error) {
		if relayShouldBypassProxy(req.URL.Hostname(), noProxy) {
			return nil, nil
		}
		return proxyURL, nil
	}
}

func relayNoProxyList() []string {
	merged := os.Getenv("NO_PROXY")
	if lower := os.Getenv("no_proxy"); lower != "" {
		if merged == "" {
			merged = lower
		} else {
			merged += "," + lower
		}
	}
	if merged == "" {
		return nil
	}
	parts := strings.Split(merged, ",")
	list := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			list = append(list, part)
		}
	}
	return list
}

func relayShouldBypassProxy(host string, noProxyList []string) bool {
	if host == "" || len(noProxyList) == 0 {
		return false
	}
	for _, rule := range noProxyList {
		rule = strings.TrimSpace(rule)
		if rule == "" {
			continue
		}
		if rule == "*" {
			return true
		}
		if idx := strings.IndexByte(rule, ':'); idx >= 0 {
			rule = rule[:idx]
		}
		rule = strings.TrimPrefix(rule, ".")
		if strings.EqualFold(host, rule) {
			return true
		}
		if len(host) > len(rule) && strings.HasSuffix(strings.ToLower(host), "."+strings.ToLower(rule)) {
			return true
		}
	}
	return false
}
