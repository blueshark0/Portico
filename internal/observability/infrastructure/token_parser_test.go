package infrastructure

import (
	"testing"

	observabilitydomain "portico/internal/observability/domain"
	"portico/internal/shared/kernel"
)

func TestGetTokenParserSelectsPlatformParser(t *testing.T) {
	if _, ok := GetTokenParser(kernel.PlatformClaude).(*ClaudeTokenParser); !ok {
		t.Fatal("expected ClaudeTokenParser for claude platform")
	}
	if _, ok := GetTokenParser(kernel.PlatformCodex).(*CodexTokenParser); !ok {
		t.Fatal("expected CodexTokenParser for codex platform")
	}
	if _, ok := GetTokenParser(kernel.PlatformGemini).(*GeminiTokenParser); !ok {
		t.Fatal("expected GeminiTokenParser for gemini platform")
	}
}

func TestClaudeTokenParserStreamDoesNotDoubleCount(t *testing.T) {
	parser := &ClaudeTokenParser{}
	usage := &observabilitydomain.RequestLog{}

	// message_start：初始 usage（input 已定，output 只有起始的 1-3 个）
	parser.Parse(`{"type":"message_start","message":{"usage":{"input_tokens":1000,"output_tokens":2,"cache_creation_input_tokens":300,"cache_read_input_tokens":400}}}`, usage)
	if usage.InputTokens != 1000 || usage.OutputTokens != 2 {
		t.Fatalf("after message_start: input=%d output=%d, want 1000/2", usage.InputTokens, usage.OutputTokens)
	}
	if usage.CacheCreateTokens != 300 || usage.CacheReadTokens != 400 {
		t.Fatalf("after message_start: cacheCreate=%d cacheRead=%d, want 300/400", usage.CacheCreateTokens, usage.CacheReadTokens)
	}

	// 中间事件不带 usage：不改变任何值
	parser.Parse(`{"type":"content_block_delta","delta":{"type":"text_delta","text":"hi"}}`, usage)
	if usage.InputTokens != 1000 || usage.OutputTokens != 2 {
		t.Fatalf("content_block_delta should not change usage: input=%d output=%d", usage.InputTokens, usage.OutputTokens)
	}

	// message_delta：usage 为累计值，覆盖而非累加
	parser.Parse(`{"type":"message_delta","usage":{"output_tokens":57}}`, usage)
	if usage.OutputTokens != 57 {
		t.Fatalf("message_delta must overwrite output tokens: got %d, want 57", usage.OutputTokens)
	}
	if usage.InputTokens != 1000 {
		t.Fatalf("message_delta without input_tokens must keep input: got %d, want 1000", usage.InputTokens)
	}

	// 新版 API 的 message_delta 会携带累计 input_tokens：同样覆盖，不能翻倍
	parser.Parse(`{"type":"message_delta","usage":{"input_tokens":1000,"output_tokens":80}}`, usage)
	if usage.InputTokens != 1000 || usage.OutputTokens != 80 {
		t.Fatalf("cumulative message_delta must not double input: input=%d output=%d, want 1000/80", usage.InputTokens, usage.OutputTokens)
	}
}

func TestClaudeTokenParserNonStreamResponse(t *testing.T) {
	parser := &ClaudeTokenParser{}
	usage := &observabilitydomain.RequestLog{}
	parser.Parse(`{"id":"msg_1","usage":{"input_tokens":500,"output_tokens":60,"cache_creation_input_tokens":10,"cache_read_input_tokens":20}}`, usage)
	if usage.InputTokens != 500 || usage.OutputTokens != 60 {
		t.Fatalf("non-stream response: input=%d output=%d, want 500/60", usage.InputTokens, usage.OutputTokens)
	}
	if usage.CacheCreateTokens != 10 || usage.CacheReadTokens != 20 {
		t.Fatalf("non-stream response: cacheCreate=%d cacheRead=%d, want 10/20", usage.CacheCreateTokens, usage.CacheReadTokens)
	}
}

func TestCodexTokenParserSplitsCachedInput(t *testing.T) {
	parser := &CodexTokenParser{}
	usage := &observabilitydomain.RequestLog{}
	parser.Parse(`{"type":"response.completed","response":{"usage":{"input_tokens":1000,"input_tokens_details":{"cached_tokens":400},"output_tokens":80,"output_tokens_details":{"reasoning_tokens":25}}}}`, usage)
	if usage.InputTokens != 600 {
		t.Fatalf("non-cached input = %d, want 600", usage.InputTokens)
	}
	if usage.CacheReadTokens != 400 {
		t.Fatalf("cache read = %d, want 400", usage.CacheReadTokens)
	}
	if usage.OutputTokens != 80 || usage.ReasoningTokens != 25 {
		t.Fatalf("output=%d reasoning=%d, want 80/25", usage.OutputTokens, usage.ReasoningTokens)
	}
}

func TestCodexTokenParserClampsNegativeNonCachedInput(t *testing.T) {
	parser := &CodexTokenParser{}
	usage := &observabilitydomain.RequestLog{}
	parser.Parse(`{"response":{"usage":{"input_tokens":100,"input_tokens_details":{"cached_tokens":250},"output_tokens":5}}}`, usage)
	if usage.InputTokens != 0 {
		t.Fatalf("non-cached input must clamp to 0, got %d", usage.InputTokens)
	}
	if usage.CacheReadTokens != 250 {
		t.Fatalf("cache read = %d, want 250", usage.CacheReadTokens)
	}
}

func TestCodexTokenParserNonStreamTopLevelUsage(t *testing.T) {
	parser := &CodexTokenParser{}
	usage := &observabilitydomain.RequestLog{}
	parser.Parse(`{"id":"resp_1","usage":{"input_tokens":300,"input_tokens_details":{"cached_tokens":100},"output_tokens":40}}`, usage)
	if usage.InputTokens != 200 || usage.CacheReadTokens != 100 || usage.OutputTokens != 40 {
		t.Fatalf("top-level usage: input=%d cacheRead=%d output=%d, want 200/100/40", usage.InputTokens, usage.CacheReadTokens, usage.OutputTokens)
	}
}

func TestGeminiTokenParserUsesLastChunkNotSum(t *testing.T) {
	parser := &GeminiTokenParser{}
	usage := &observabilitydomain.RequestLog{}

	// 每个 chunk 都携带 usageMetadata：promptTokenCount 是固定值，
	// candidatesTokenCount 是累计值——逐 chunk 累加会把 input 放大 N 倍。
	for i := 0; i < 50; i++ {
		parser.Parse(`{"candidates":[{"content":{}}],"usageMetadata":{"promptTokenCount":10000,"candidatesTokenCount":5}}`, usage)
	}
	parser.Parse(`{"usageMetadata":{"promptTokenCount":10000,"candidatesTokenCount":50,"totalTokenCount":10080}}`, usage)

	if usage.InputTokens != 10000 {
		t.Fatalf("input must stay at prompt count from last chunk, got %d", usage.InputTokens)
	}
	// totalTokenCount(10080) - prompt(10000) = 80 > candidates(50)：思考 token 计入 output
	if usage.OutputTokens != 80 {
		t.Fatalf("output = %d, want 80 (total-derived including thoughts)", usage.OutputTokens)
	}
}

func TestGeminiTokenParserChunkWithoutUsageMetadataKeepsState(t *testing.T) {
	parser := &GeminiTokenParser{}
	usage := &observabilitydomain.RequestLog{InputTokens: 123, OutputTokens: 45}
	parser.Parse(`{"candidates":[{"content":{"parts":[{"text":"hi"}]}}]}`, usage)
	if usage.InputTokens != 123 || usage.OutputTokens != 45 {
		t.Fatalf("chunk without usageMetadata must not reset usage: input=%d output=%d", usage.InputTokens, usage.OutputTokens)
	}
}

func TestGeminiTokenParserReadsCachedTokens(t *testing.T) {
	parser := &GeminiTokenParser{}
	usage := &observabilitydomain.RequestLog{}
	parser.Parse(`{"usageMetadata":{"promptTokenCount":800,"candidatesTokenCount":30,"cachedContentTokenCount":500,"totalTokenCount":830}}`, usage)
	if usage.InputTokens != 800 || usage.OutputTokens != 30 || usage.CacheReadTokens != 500 {
		t.Fatalf("input=%d output=%d cacheRead=%d, want 800/30/500", usage.InputTokens, usage.OutputTokens, usage.CacheReadTokens)
	}
}

func TestParsersIgnoreInvalidJSON(t *testing.T) {
	for _, parser := range []TokenUsageParser{&ClaudeTokenParser{}, &CodexTokenParser{}, &GeminiTokenParser{}} {
		usage := &observabilitydomain.RequestLog{InputTokens: 7, OutputTokens: 8}
		parser.Parse(`[DONE]`, usage)
		parser.Parse(``, usage)
		if usage.InputTokens != 7 || usage.OutputTokens != 8 {
			t.Fatalf("%T: invalid payload must not change usage", parser)
		}
	}
}
