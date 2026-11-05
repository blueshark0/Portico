package relay

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// relayInboundToken 与 platformproxy 写入各 CLI 配置的占位 token 保持一致
// （ANTHROPIC_AUTH_TOKEN / OPENAI_API_KEY / GEMINI_API_KEY = "portico"）。
// 即使监听地址已收敛到 127.0.0.1，仍校验入站凭证，防止本机其他用户或
// 恶意程序直接借用代理消耗已配置的付费 API key。
const (
	relayInboundToken       = "portico"
	legacyRelayInboundToken = "code-switch"
)

func (s *Server) inboundAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if isAuthorizedInboundRequest(c.Request) {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error": "unauthorized: expected the Portico placeholder token; re-enable the platform proxy in Portico to refresh the CLI config",
		})
	}
}

func isAuthorizedInboundRequest(r *http.Request) bool {
	candidates := []string{
		r.Header.Get("Authorization"),
		r.Header.Get("X-Api-Key"),
		r.Header.Get("X-Goog-Api-Key"),
		r.URL.Query().Get("key"),
	}
	for _, value := range candidates {
		if matchesInboundToken(value) {
			return true
		}
	}
	return false
}

func matchesInboundToken(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.TrimSpace(strings.TrimPrefix(normalized, "bearer "))
	return normalized == relayInboundToken || normalized == legacyRelayInboundToken
}
