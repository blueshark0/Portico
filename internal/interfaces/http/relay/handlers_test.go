package relay

import "testing"

func TestClaudeHandlerExtractRequestMeta(t *testing.T) {
	handler := NewClaudeHandler("/v1/messages")
	meta := handler.ExtractRequestMeta("/v1/messages", []byte(`{
		"model":"claude-sonnet",
		"stream":true,
		"metadata":{"user_id":"user-1"}
	}`), nil, nil)

	if meta.Endpoint != "/v1/messages" {
		t.Fatalf("unexpected endpoint: %s", meta.Endpoint)
	}
	if meta.RequestedModel != "claude-sonnet" || !meta.IsStream || meta.SessionID != "user-1" {
		t.Fatalf("unexpected meta: %+v", meta)
	}
	if !meta.BodyHasModelField {
		t.Fatal("expected model field to be detected")
	}
}

func TestCodexHandlerExtractRequestMeta(t *testing.T) {
	handler := NewCodexHandler("/responses")
	meta := handler.ExtractRequestMeta("/responses", []byte(`{"model":"gpt-5","stream":false,"service_tier":"fast"}`), nil, map[string]string{
		"session-id": "session-1",
	})

	if meta.Endpoint != "/responses" {
		t.Fatalf("unexpected endpoint: %s", meta.Endpoint)
	}
	if meta.RequestedModel != "gpt-5" || meta.IsStream || meta.SessionID != "session-1" {
		t.Fatalf("unexpected meta: %+v", meta)
	}
	if !meta.IsFast {
		t.Fatalf("expected fast request, got %+v", meta)
	}
}

func TestCodexHandlerExtractRequestMetaTreatsPriorityAsFast(t *testing.T) {
	handler := NewCodexHandler("/responses")
	meta := handler.ExtractRequestMeta("/responses", []byte(`{"model":"gpt-5","service_tier":"priority"}`), nil, nil)

	if !meta.IsFast {
		t.Fatalf("expected priority request to be fast, got %+v", meta)
	}
}

func TestCodexHandlerExtractRequestMetaFallsBackToLegacySessionHeader(t *testing.T) {
	handler := NewCodexHandler("/responses")
	meta := handler.ExtractRequestMeta("/responses", []byte(`{"model":"gpt-5","stream":false}`), nil, map[string]string{
		"session_id": "legacy-session-1",
	})

	if meta.SessionID != "legacy-session-1" {
		t.Fatalf("unexpected session id: %s", meta.SessionID)
	}
}

func TestCodexHandlerClassifiesBackgroundTasks(t *testing.T) {
	handler := NewCodexHandler("/responses")
	tests := []struct {
		name       string
		body       string
		headers    map[string]string
		background bool
	}{
		{name: "main conversation", body: `{"model":"gpt-5"}`, headers: map[string]string{"session-id": "main"}},
		{
			name:       "memory consolidation subagent",
			body:       `{"model":"gpt-5"}`,
			headers:    map[string]string{"X-OpenAI-Subagent": "memory_consolidation"},
			background: true,
		},
		{
			name:       "temporary structured title",
			body:       `{"model":"gpt-5","text":{"format":{"name":"codex_output_schema","type":"json_schema"}}}`,
			background: true,
		},
		{
			name:       "memory request metadata",
			body:       `{"model":"gpt-5"}`,
			headers:    map[string]string{"x-codex-turn-metadata": `{"request_kind":"memory"}`},
			background: true,
		},
		{
			name:    "custom structured output is not enough to hide a session",
			body:    `{"model":"gpt-5","text":{"format":{"name":"customer_schema","type":"json_schema"}}}`,
			headers: map[string]string{"session-id": "main"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta := handler.ExtractRequestMeta("/responses", []byte(tt.body), nil, tt.headers)
			if meta.IsBackgroundTask != tt.background {
				t.Fatalf("unexpected background classification: got=%v want=%v", meta.IsBackgroundTask, tt.background)
			}
		})
	}
}

func TestGeminiHandlerExtractRequestMeta(t *testing.T) {
	handler := NewGeminiHandler()
	meta := handler.ExtractRequestMeta(
		"/gemini/v1beta/models/gemini-2.5-pro:streamGenerateContent",
		[]byte(`{"model":"models/gemini-2.5-pro"}`),
		map[string]string{"alt": "sse"},
		map[string]string{"x-session-id": "gem-session"},
	)

	if meta.Endpoint != "/v1beta/models/gemini-2.5-pro:streamGenerateContent" {
		t.Fatalf("unexpected endpoint: %s", meta.Endpoint)
	}
	if meta.RequestedModel != "gemini-2.5-pro" || !meta.IsStream || meta.SessionID != "gem-session" {
		t.Fatalf("unexpected meta: %+v", meta)
	}
	if !meta.BodyHasModelField {
		t.Fatal("expected gemini body model field")
	}
	if meta.RouteOptions == nil || !meta.RouteOptions.forceBodyRewrite {
		t.Fatalf("expected gemini route options to require body rewrite, got %+v", meta.RouteOptions)
	}
}
