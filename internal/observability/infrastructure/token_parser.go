package infrastructure

import (
	observabilitydomain "portico/internal/observability/domain"
	"portico/internal/shared/kernel"

	"github.com/tidwall/gjson"
)

type TokenUsageParser interface {
	Parse(data string, usage *observabilitydomain.RequestLog)
}

func GetTokenParser(platform kernel.Platform) TokenUsageParser {
	switch platform {
	case kernel.PlatformCodex:
		return &CodexTokenParser{}
	case kernel.PlatformGemini:
		return &GeminiTokenParser{}
	default:
		return &ClaudeTokenParser{}
	}
}

// applyTokenField 只在字段存在时覆盖目标值。三个平台的 usage 均为绝对值
// （初始值或累计值），必须用"最后值覆盖"而非 += 累加，否则 message_start
// 与 message_delta 双事件、或逐 chunk 携带的固定值会被重复计入。
func applyTokenField(data, path string, target *int) {
	if result := gjson.Get(data, path); result.Exists() {
		*target = int(result.Int())
	}
}

type ClaudeTokenParser struct{}

// Parse 处理三种输入：message_start 事件（message.usage.* 初始值）、
// message_delta 事件（顶层 usage.* 累计值）、非流式响应（顶层 usage.*）。
func (p *ClaudeTokenParser) Parse(data string, usage *observabilitydomain.RequestLog) {
	applyTokenField(data, "message.usage.input_tokens", &usage.InputTokens)
	applyTokenField(data, "message.usage.output_tokens", &usage.OutputTokens)
	applyTokenField(data, "message.usage.cache_creation_input_tokens", &usage.CacheCreateTokens)
	applyTokenField(data, "message.usage.cache_read_input_tokens", &usage.CacheReadTokens)
	applyTokenField(data, "usage.input_tokens", &usage.InputTokens)
	applyTokenField(data, "usage.output_tokens", &usage.OutputTokens)
	applyTokenField(data, "usage.cache_creation_input_tokens", &usage.CacheCreateTokens)
	applyTokenField(data, "usage.cache_read_input_tokens", &usage.CacheReadTokens)
}

type CodexTokenParser struct{}

// Parse 处理 SSE 的 response.completed 事件（response.usage）与非流式响应（顶层 usage）。
func (p *CodexTokenParser) Parse(data string, usage *observabilitydomain.RequestLog) {
	p.applyUsage(gjson.Get(data, "response.usage"), usage)
	p.applyUsage(gjson.Get(data, "usage"), usage)
}

func (p *CodexTokenParser) applyUsage(node gjson.Result, usage *observabilitydomain.RequestLog) {
	if !node.Exists() {
		return
	}
	totalInput := int(node.Get("input_tokens").Int())
	cachedInput := int(node.Get("input_tokens_details.cached_tokens").Int())
	nonCachedInput := totalInput - cachedInput
	if nonCachedInput < 0 {
		nonCachedInput = 0
	}
	usage.InputTokens = nonCachedInput
	usage.OutputTokens = int(node.Get("output_tokens").Int())
	usage.CacheReadTokens = cachedInput
	usage.ReasoningTokens = int(node.Get("output_tokens_details.reasoning_tokens").Int())
}

type GeminiTokenParser struct{}

// Parse 以最后一个携带 usageMetadata 的 chunk 为准：Gemini 流式响应的每个
// chunk 都带 usageMetadata，promptTokenCount 是固定值、candidatesTokenCount
// 是累计值，逐 chunk 累加会把 input 放大数十倍。
func (p *GeminiTokenParser) Parse(data string, usage *observabilitydomain.RequestLog) {
	meta := gjson.Get(data, "usageMetadata")
	if !meta.Exists() {
		return
	}
	if v := meta.Get("promptTokenCount"); v.Exists() {
		usage.InputTokens = int(v.Int())
	}
	if v := meta.Get("candidatesTokenCount"); v.Exists() {
		usage.OutputTokens = int(v.Int())
	}
	if v := meta.Get("cachedContentTokenCount"); v.Exists() {
		usage.CacheReadTokens = int(v.Int())
	}
	// totalTokenCount 包含 thoughts tokens：当 total 比 prompt+candidates 大时，
	// 把差值（思考 token）计入 output。
	if total := meta.Get("totalTokenCount").Int(); total > 0 {
		remaining := int(total) - usage.InputTokens
		if remaining > usage.OutputTokens {
			usage.OutputTokens = remaining
		}
	}
}
