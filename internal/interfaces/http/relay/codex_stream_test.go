package relay

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	routingdomain "portico/internal/routing/domain"
	sessionapp "portico/internal/sessions/application"
	sessioninfra "portico/internal/sessions/infrastructure"
	"portico/internal/shared/kernel"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type terminalErrorReader struct {
	err error
}

func (r terminalErrorReader) Read([]byte) (int, error) {
	return 0, r.err
}

func TestCodexStreamOutcome(t *testing.T) {
	tests := []struct {
		name      string
		payload   string
		want      codexStreamStatus
		wantEvent string
	}{
		{
			name:      "completed",
			payload:   `data: {"type":"response.completed","response":{"status":"completed","error":null,"incomplete_details":null}}`,
			want:      codexStreamCompleted,
			wantEvent: "response.completed",
		},
		{
			name:    "completed event with error is not success",
			payload: `data: {"type":"response.completed","response":{"status":"completed","error":{"message":"failed"}}}`,
			want:    codexStreamPending,
		},
		{
			name:      "failed",
			payload:   `data: {"type":"response.failed","response":{"status":"failed"}}`,
			want:      codexStreamFailed,
			wantEvent: "response.failed",
		},
		{
			name:      "incomplete",
			payload:   `data: {"type":"response.incomplete","response":{"status":"incomplete"}}`,
			want:      codexStreamFailed,
			wantEvent: "response.incomplete",
		},
		{
			name:    "ordinary delta",
			payload: `data: {"type":"response.output_text.delta","delta":"hello"}`,
			want:    codexStreamPending,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outcome := &codexStreamOutcome{}
			outcome.Observe([]byte(tt.payload))
			if got := outcome.Status(); got != tt.want {
				t.Fatalf("status = %v, want %v", got, tt.want)
			}
			if got := outcome.EventType(); got != tt.wantEvent {
				t.Fatalf("event type = %q, want %q", got, tt.wantEvent)
			}
		})
	}
}

func TestCodexSessionBindingUsesCompletedEventBeforeTransportCancellation(t *testing.T) {
	payload := codexTestSSE("response.completed", "completed", "null")
	ok, err, repo := routeCodexTestStream(t, payload, context.Canceled)
	if err != nil {
		t.Fatalf("route completed stream: %v", err)
	}
	if !ok {
		t.Fatal("expected completed Codex stream to succeed despite trailing context cancellation")
	}
	if got := repo.bindings[repo.key("codex", "session-1")]; got != "provider-1" {
		t.Fatalf("session binding = %q, want provider-1", got)
	}
}

func TestCodexSessionBindingRejectsFailedEventWithHTTP200(t *testing.T) {
	payload := codexTestSSE("response.failed", "failed", `{"message":"upstream failed"}`)
	ok, err, repo := routeCodexTestStream(t, payload, nil)
	if ok {
		t.Fatal("expected response.failed stream not to succeed")
	}
	if err == nil || !strings.Contains(err.Error(), "response.failed") {
		t.Fatalf("expected response.failed error, got %v", err)
	}
	if got := repo.bindings[repo.key("codex", "session-1")]; got != "" {
		t.Fatalf("failed stream must not bind session, got %q", got)
	}
}

func TestCodexSessionBindingRejectsCancellationBeforeTerminalEvent(t *testing.T) {
	padding := strings.Repeat("x", 2048)
	payload := fmt.Sprintf("event: response.output_text.delta\ndata: %s\n\n", `{"type":"response.output_text.delta","delta":"`+padding+`"}`)
	ok, err, repo := routeCodexTestStream(t, payload, context.Canceled)
	if ok {
		t.Fatal("expected cancellation before a terminal event not to succeed")
	}
	if err == nil || !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	if got := repo.bindings[repo.key("codex", "session-1")]; got != "" {
		t.Fatalf("cancelled stream must not bind session, got %q", got)
	}
}

func routeCodexTestStream(t *testing.T, payload string, trailingErr error) (bool, error, *relaySessionRepoStub) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	bodyReader := io.Reader(strings.NewReader(payload))
	if trailingErr != nil {
		bodyReader = io.MultiReader(bodyReader, terminalErrorReader{err: trailingErr})
	}
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     http.StatusText(http.StatusOK),
			Header: http.Header{
				"Content-Type": []string{"text/event-stream"},
			},
			Body:    io.NopCloser(bodyReader),
			Request: req,
		}, nil
	})}

	repo := &relaySessionRepoStub{bindings: make(map[string]string)}
	service := sessionapp.NewService(repo)
	server := &Server{
		sessionService: service,
		sessionCache:   sessioninfra.NewCache(service),
		upstreamClients: map[upstreamClientOptions]*http.Client{
			{}: client,
		},
	}

	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	ginCtx.Request = httptest.NewRequest(http.MethodPost, "/responses", nil)
	profile := routingdomain.RouteProfile{
		Platform:          kernel.PlatformCodex,
		DefaultProviderID: intPtr(1),
		Providers: []routingdomain.Provider{
			{
				ID:       1,
				Name:     "provider-1",
				APIURL:   "https://provider.example",
				APIKey:   "test-key",
				Position: 1,
			},
		},
	}

	ok, err := server.routeToManualProvider(&RelayContext{
		GinCtx:    ginCtx,
		Platform:  kernel.PlatformCodex,
		BodyBytes: []byte(`{"model":"gpt-5","stream":true}`),
		Query:     map[string]string{},
		Headers:   map[string]string{"Accept": "text/event-stream"},
		Profile:   profile,
		RequestMeta: RequestMeta{
			Endpoint:          "/responses",
			RequestedModel:    "gpt-5",
			SessionID:         "session-1",
			IsStream:          true,
			BodyHasModelField: true,
		},
	})
	return ok, err, repo
}

func codexTestSSE(eventType, status, errorJSON string) string {
	padding := strings.Repeat("x", 2048)
	data := fmt.Sprintf(
		`{"type":%q,"response":{"status":%q,"error":%s,"incomplete_details":null,"output":[{"text":%q}]}}`,
		eventType,
		status,
		errorJSON,
		padding,
	)
	return fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, data)
}
