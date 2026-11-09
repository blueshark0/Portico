package admin

import (
	"crypto/subtle"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// TokenSource 提供当前有效的访问令牌。
type TokenSource interface {
	Token() string
}

// StaticToken 是固定令牌（测试或嵌入式场景）。
type StaticToken string

func (t StaticToken) Token() string { return string(t) }

// FileToken 从令牌文件读取并按 mtime 缓存：`portico-server token reset`
// 改写文件后无需重启，正在运行的服务在下一次请求时即切换到新令牌。
type FileToken struct {
	path string

	mu      sync.Mutex
	token   string
	modTime time.Time
}

func NewFileToken(path, initial string) *FileToken {
	t := &FileToken{path: path, token: initial}
	if info, err := os.Stat(path); err == nil {
		t.modTime = info.ModTime()
	}
	return t
}

func (t *FileToken) Token() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	info, err := os.Stat(t.path)
	if err != nil || info.ModTime().Equal(t.modTime) {
		return t.token
	}
	data, err := os.ReadFile(t.path)
	if err != nil {
		return t.token
	}
	if token := strings.TrimSpace(string(data)); token != "" {
		t.token = token
		t.modTime = info.ModTime()
	}
	return t.token
}

// authMiddleware 保护 /api 下除 health 外的所有接口。
//
// 规则：
//   - 跨站请求一律拒绝：Origin 存在且与 Host 不同源，或 Sec-Fetch-Site 为 cross-site。
//     同时状态变更接口要求 application/json，迫使浏览器发起 CORS 预检，而本服务从不应答预检。
//   - 来自回环地址、且 Host 头也是回环主机名的请求免令牌（本机桌面窗口与浏览器）。
//     校验 Host 是为了防御 DNS rebinding：恶意域名解析到 127.0.0.1 时 Host 仍是该域名。
//     带有 Forwarded / X-Forwarded-* / X-Real-IP 头的请求视为经代理转发，不享受该豁免。
//     同机反向代理默认会把 Host 改写为上游地址（如 nginx 的 $proxy_host），
//     仅凭 RemoteAddr 与 Host 无法区分，因此反向代理部署应开启 alwaysRequireToken。
//   - 其他请求必须携带 Authorization: Bearer <token>。
func authMiddleware(tokens TokenSource, alwaysRequireToken bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		r := c.Request
		if isCrossSite(r) {
			abortJSON(c, http.StatusForbidden, "cross-site requests are not allowed")
			return
		}
		if r.Method == http.MethodPost && !isJSONContentType(r.Header.Get("Content-Type")) {
			abortJSON(c, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
			return
		}
		if !alwaysRequireToken && isLocalRequest(r) {
			c.Next()
			return
		}
		if !validBearer(r.Header.Get("Authorization"), tokens.Token()) {
			c.Header("WWW-Authenticate", `Bearer realm="portico"`)
			abortJSON(c, http.StatusUnauthorized, "missing or invalid access token")
			return
		}
		c.Next()
	}
}

func isCrossSite(r *http.Request) bool {
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return true
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return true
	}
	return !strings.EqualFold(parsed.Host, r.Host)
}

func isJSONContentType(value string) bool {
	mediaType, _, _ := strings.Cut(value, ";")
	return strings.EqualFold(strings.TrimSpace(mediaType), "application/json")
}

var forwardedHeaders = []string{"Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Real-Ip"}

func isLocalRequest(r *http.Request) bool {
	for _, name := range forwardedHeaders {
		if r.Header.Get(name) != "" {
			return false
		}
	}
	remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	remoteIP := net.ParseIP(remoteHost)
	if remoteIP == nil || !remoteIP.IsLoopback() {
		return false
	}
	return isLoopbackHost(r.Host)
}

func isLoopbackHost(hostHeader string) bool {
	host := hostHeader
	if h, _, err := net.SplitHostPort(hostHeader); err == nil {
		host = h
	}
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func validBearer(header, token string) bool {
	if token == "" {
		return false
	}
	scheme, value, ok := strings.Cut(strings.TrimSpace(header), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return false
	}
	value = strings.TrimSpace(value)
	return subtle.ConstantTimeCompare([]byte(value), []byte(token)) == 1
}

func abortJSON(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": message})
}
