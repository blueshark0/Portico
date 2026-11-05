package relay

import (
	"strings"

	"github.com/tidwall/gjson"
	routingdomain "portico/internal/routing/domain"
	"portico/internal/shared/kernel"
)

type ClaudeHandler struct {
	defaultEndpoint string
}

func NewClaudeHandler(endpoint string) *ClaudeHandler {
	return &ClaudeHandler{defaultEndpoint: endpoint}
}

func (h *ClaudeHandler) ExtractRequestMeta(path string, bodyBytes []byte, query, headers map[string]string) RequestMeta {
	modelResult := gjson.GetBytes(bodyBytes, "model")
	return RequestMeta{
		Endpoint:          h.defaultEndpoint,
		RequestedModel:    modelResult.String(),
		SessionID:         gjson.GetBytes(bodyBytes, "metadata.user_id").String(),
		IsStream:          gjson.GetBytes(bodyBytes, "stream").Bool(),
		BodyHasModelField: modelResult.Exists(),
	}
}

type CodexHandler struct {
	defaultEndpoint string
}

func NewCodexHandler(endpoint string) *CodexHandler {
	return &CodexHandler{defaultEndpoint: endpoint}
}

func (h *CodexHandler) ExtractRequestMeta(path string, bodyBytes []byte, query, headers map[string]string) RequestMeta {
	modelResult := gjson.GetBytes(bodyBytes, "model")
	return RequestMeta{
		Endpoint:          h.defaultEndpoint,
		RequestedModel:    modelResult.String(),
		SessionID:         codexSessionID(headers),
		IsStream:          gjson.GetBytes(bodyBytes, "stream").Bool(),
		IsFast:            isCodexFastRequest(bodyBytes),
		IsBackgroundTask:  isCodexBackgroundTask(bodyBytes, headers),
		BodyHasModelField: modelResult.Exists(),
	}
}

func codexSessionID(headers map[string]string) string {
	if sessionID := headerValue(headers, "session-id"); sessionID != "" {
		return sessionID
	}
	return headerValue(headers, "session_id")
}

// isCodexBackgroundTask 识别 Codex 自动创建的非用户会话。子代理和记忆整理
// 会带专用元数据；自动标题等 temporary structured 请求使用内部输出 schema。
// 这些请求仍按原 session-id 路由和记录日志，只是不在会话管理中占一个用户会话。
func isCodexBackgroundTask(bodyBytes []byte, headers map[string]string) bool {
	if strings.TrimSpace(headerValue(headers, "x-openai-subagent")) != "" {
		return true
	}
	if gjson.GetBytes(bodyBytes, "text.format.name").String() == "codex_output_schema" {
		return true
	}
	turnMetadata := headerValue(headers, "x-codex-turn-metadata")
	return gjson.Get(turnMetadata, "request_kind").String() == "memory"
}

func isCodexFastRequest(bodyBytes []byte) bool {
	switch strings.ToLower(strings.TrimSpace(gjson.GetBytes(bodyBytes, "service_tier").String())) {
	case "fast", "priority":
		return true
	default:
		return false
	}
}

type GeminiHandler struct{}

func NewGeminiHandler() *GeminiHandler {
	return &GeminiHandler{}
}

func (h *GeminiHandler) ExtractRequestMeta(path string, bodyBytes []byte, query, headers map[string]string) RequestMeta {
	endpoint := strings.TrimPrefix(path, "/gemini")
	var bodyModel string
	requestedModel, bodyModel := extractGeminiModel(endpoint, bodyBytes)
	sessionID := headerValue(headers, "x-gemini-api-privileged-user-id")
	if sessionID == "" {
		sessionID = headerValue(headers, "x-session-id")
	}
	return RequestMeta{
		Endpoint:          endpoint,
		RequestedModel:    requestedModel,
		SessionID:         sessionID,
		IsStream:          detectGeminiStream(endpoint, query, headers, bodyBytes),
		BodyHasModelField: strings.TrimSpace(bodyModel) != "",
		RouteOptions:      h.createRouteOptions(bodyModel != ""),
	}
}

func (h *GeminiHandler) createRouteOptions(forceBodyRewrite bool) *relayRouteOptions {
	return &relayRouteOptions{
		endpointMutator: func(endpoint, _, effectiveModel string) (string, error) {
			if effectiveModel == "" {
				return endpoint, nil
			}
			return replaceGeminiModelInPath(endpoint, effectiveModel), nil
		},
		queryMutator: func(q map[string]string, provider routingdomain.Provider) map[string]string {
			if q == nil {
				return nil
			}
			// 认证统一由 forwardRequest 注入 x-goog-api-key 头：清掉客户端
			// 带来的 key 参数，避免占位符或密钥落入 URL 与上游访问日志。
			delete(q, "key")
			return q
		},
		bodyModelFormatter: formatGeminiModelForBody,
		forceBodyRewrite:   forceBodyRewrite,
	}
}

func GetPlatformHandler(platform kernel.Platform, defaultEndpoint string) PlatformHandler {
	switch platform {
	case kernel.PlatformClaude:
		return NewClaudeHandler(defaultEndpoint)
	case kernel.PlatformCodex:
		return NewCodexHandler(defaultEndpoint)
	case kernel.PlatformGemini:
		return NewGeminiHandler()
	default:
		return NewClaudeHandler(defaultEndpoint)
	}
}

func extractGeminiModel(path string, bodyBytes []byte) (string, string) {
	bodyModel := strings.TrimSpace(gjson.GetBytes(bodyBytes, "model").String())

	modelFromPath := ""
	if idx := strings.Index(path, "/models/"); idx >= 0 {
		rest := path[idx+len("/models/"):]
		delimiter := strings.IndexAny(rest, ":?")
		if delimiter >= 0 {
			modelFromPath = rest[:delimiter]
		} else {
			modelFromPath = rest
		}
	}

	requested := normalizeGeminiModel(bodyModel)
	if requested == "" {
		requested = normalizeGeminiModel(modelFromPath)
	}
	return requested, bodyModel
}

func normalizeGeminiModel(model string) string {
	trimmed := strings.TrimSpace(model)
	if strings.Contains(trimmed, "/models/") {
		parts := strings.SplitN(trimmed, "/models/", 2)
		trimmed = parts[1]
	}
	trimmed = strings.TrimPrefix(trimmed, "models/")
	return trimmed
}

func formatGeminiModelForBody(model string) string {
	if model == "" {
		return ""
	}
	if strings.HasPrefix(model, "models/") {
		return model
	}
	return "models/" + model
}

func replaceGeminiModelInPath(path, effectiveModel string) string {
	if path == "" || effectiveModel == "" {
		return path
	}
	idx := strings.Index(path, "/models/")
	if idx < 0 {
		return path
	}
	start := idx + len("/models/")
	rest := path[start:]
	delimiter := strings.IndexAny(rest, ":?")
	if delimiter < 0 {
		return path[:start] + effectiveModel
	}
	return path[:start] + effectiveModel + rest[delimiter:]
}

func detectGeminiStream(path string, query map[string]string, headers map[string]string, body []byte) bool {
	if gjson.GetBytes(body, "stream").Bool() {
		return true
	}
	lowerPath := strings.ToLower(path)
	if strings.Contains(lowerPath, ":stream") || strings.Contains(lowerPath, "/stream") {
		return true
	}
	if alt, ok := query["alt"]; ok && strings.EqualFold(alt, "sse") {
		return true
	}
	accept := headers["Accept"]
	if accept == "" {
		accept = headers["accept"]
	}
	return strings.Contains(strings.ToLower(accept), "event-stream")
}
