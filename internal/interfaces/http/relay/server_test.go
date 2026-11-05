package relay

import (
	"bytes"
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	observabilitydomain "portico/internal/observability/domain"
	routingapp "portico/internal/routing/application"
	routingdomain "portico/internal/routing/domain"
	sessionapp "portico/internal/sessions/application"
	sessiondomain "portico/internal/sessions/domain"
	sessioninfra "portico/internal/sessions/infrastructure"
	"portico/internal/shared/kernel"
	"portico/internal/shared/worker"
)

type relayRoutingRepoStub struct {
	profiles map[kernel.Platform]routingdomain.RouteProfile
}

func (r *relayRoutingRepoStub) GetProfile(_ context.Context, platform kernel.Platform) (routingdomain.RouteProfile, error) {
	if profile, ok := r.profiles[platform]; ok {
		return profile, nil
	}
	return routingdomain.RouteProfile{Platform: platform}, nil
}

func (r *relayRoutingRepoStub) SaveProfile(_ context.Context, profile routingdomain.RouteProfile) (routingdomain.RouteProfile, error) {
	if r.profiles == nil {
		r.profiles = make(map[kernel.Platform]routingdomain.RouteProfile)
	}
	r.profiles[profile.Platform] = profile
	return profile, nil
}

func (r *relayRoutingRepoStub) GetAppPreferences(_ context.Context) (routingdomain.AppPreferences, error) {
	return routingdomain.DefaultAppPreferences(), nil
}

func (r *relayRoutingRepoStub) SaveAppPreferences(_ context.Context, preferences routingdomain.AppPreferences) (routingdomain.AppPreferences, error) {
	return preferences, nil
}

func (r *relayRoutingRepoStub) ReplaceAll(_ context.Context, _ routingdomain.AppPreferences, profiles []routingdomain.RouteProfile) error {
	if r.profiles == nil {
		r.profiles = make(map[kernel.Platform]routingdomain.RouteProfile)
	}
	for _, profile := range profiles {
		r.profiles[profile.Platform] = profile
	}
	return nil
}

type relaySessionRepoStub struct {
	bindings          map[string]string
	backgroundRecords map[string]bool
	bindCalls         int
	updateCalls       int
	unbindCalls       int
}

func (r *relaySessionRepoStub) GetSessionProvider(platform, sessionID string) (string, error) {
	if r.bindings == nil {
		return "", nil
	}
	return r.bindings[r.key(platform, sessionID)], nil
}

func (r *relaySessionRepoStub) BindSessionToProvider(platform, sessionID, providerName string) error {
	if r.bindings == nil {
		r.bindings = make(map[string]string)
	}
	r.bindCalls++
	r.bindings[r.key(platform, sessionID)] = providerName
	return nil
}

func (r *relaySessionRepoStub) UpdateSessionSuccess(platform, sessionID string) error {
	r.updateCalls++
	return nil
}

func (r *relaySessionRepoStub) RecordSessionBackground(platform, sessionID string, background bool) error {
	if r.backgroundRecords == nil {
		r.backgroundRecords = make(map[string]bool)
	}
	r.backgroundRecords[r.key(platform, sessionID)] = background
	return nil
}

func (r *relaySessionRepoStub) GetProviderSessions(platform, providerName string) ([]sessiondomain.SessionBinding, error) {
	return nil, nil
}

func (r *relaySessionRepoStub) GetPlatformSessions(platform string) ([]sessiondomain.SessionBinding, error) {
	return nil, nil
}

func (r *relaySessionRepoStub) UnbindSession(platform, sessionID string) error {
	if r.bindings != nil {
		delete(r.bindings, r.key(platform, sessionID))
	}
	r.unbindCalls++
	return nil
}

func (r *relaySessionRepoStub) CleanExpiredSessions() error {
	return nil
}

func (r *relaySessionRepoStub) key(platform, sessionID string) string {
	return platform + ":" + sessionID
}

type requestLogNoopProcessor struct{}

func (p *requestLogNoopProcessor) Process(batch []*observabilitydomain.RequestLog) error {
	return nil
}

func (p *requestLogNoopProcessor) ProcessSingle(item *observabilitydomain.RequestLog) error {
	return nil
}

func TestServerStartReturnsErrorWhenAddressIsInUse(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on ephemeral port: %v", err)
	}
	defer listener.Close()

	server := &Server{
		addr: listener.Addr().String(),
		routingService: routingapp.NewService(&relayRoutingRepoStub{
			profiles: map[kernel.Platform]routingdomain.RouteProfile{
				kernel.PlatformClaude: {Platform: kernel.PlatformClaude},
				kernel.PlatformCodex:  {Platform: kernel.PlatformCodex},
				kernel.PlatformGemini: {Platform: kernel.PlatformGemini},
			},
		}),
		shutdownCh: make(chan struct{}),
	}
	defer func() {
		_ = server.Stop()
	}()

	if err := server.Start(); err == nil {
		t.Fatal("expected relay start to fail when address is already in use")
	}
}

func TestRouteToManualProviderClearsInvalidBoundSessionAndFallsBackToDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var requestCount int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("unexpected upstream path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	sessionRepo := &relaySessionRepoStub{
		bindings: map[string]string{
			"claude:session-1": "missing-provider",
		},
	}
	sessionService := sessionapp.NewService(sessionRepo)
	requestLogWorker := worker.New[*observabilitydomain.RequestLog](worker.Config{
		Name:          "request_log_test",
		BufferSize:    1,
		BatchSize:     1,
		FlushInterval: 5 * time.Millisecond,
	}, &requestLogNoopProcessor{})
	requestLogWorker.Start()
	defer requestLogWorker.Stop()

	server := &Server{
		sessionService:   sessionService,
		sessionCache:     sessioninfra.NewCache(sessionService),
		requestLogWorker: requestLogWorker,
	}

	body := []byte(`{"model":"claude-sonnet-4"}`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	ginCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	ok, err := server.routeToManualProvider(&RelayContext{
		GinCtx:    ginCtx,
		Platform:  kernel.PlatformClaude,
		BodyBytes: body,
		Query:     map[string]string{},
		Headers:   map[string]string{"Accept": "application/json"},
		Profile: routingdomain.RouteProfile{
			Platform:          kernel.PlatformClaude,
			DefaultProviderID: intPtr(1),
			Providers: []routingdomain.Provider{
				{
					ID:     1,
					Name:   "default-provider",
					APIURL: upstream.URL,
					APIKey: "test-key",

					Position: 1,
				},
			},
		},
		RequestMeta: RequestMeta{
			Endpoint:          "/v1/messages",
			RequestedModel:    "claude-sonnet-4",
			SessionID:         "session-1",
			BodyHasModelField: true,
		},
	})
	if err != nil {
		t.Fatalf("route to manual provider: %v", err)
	}
	if !ok {
		t.Fatal("expected route to succeed after falling back to default provider")
	}
	if requestCount != 1 {
		t.Fatalf("expected exactly one upstream request, got %d", requestCount)
	}
	if sessionRepo.unbindCalls != 1 {
		t.Fatalf("expected invalid session binding to be cleared once, got %d", sessionRepo.unbindCalls)
	}
	if sessionRepo.bindCalls != 1 {
		t.Fatalf("expected fallback provider to be rebound once, got %d", sessionRepo.bindCalls)
	}
	if got := sessionRepo.bindings[sessionRepo.key("claude", "session-1")]; got != "default-provider" {
		t.Fatalf("expected session to be rebound to default provider, got %q", got)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected proxy response code 200, got %d", recorder.Code)
	}
}

func TestRouteToManualProviderIgnoresStaleSuccessAfterUnbindAndRebindsNextRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	releaseFirst := make(chan struct{})
	firstStarted := make(chan struct{})
	requestCount := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			close(firstStarted)
			<-releaseFirst
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	sessionRepo := &relaySessionRepoStub{
		bindings: map[string]string{
			"claude:session-race": "provider-a",
		},
	}
	sessionService := sessionapp.NewService(sessionRepo)
	requestLogWorker := worker.New[*observabilitydomain.RequestLog](worker.Config{
		Name:          "request_log_test",
		BufferSize:    2,
		BatchSize:     1,
		FlushInterval: 5 * time.Millisecond,
	}, &requestLogNoopProcessor{})
	requestLogWorker.Start()
	defer requestLogWorker.Stop()

	server := &Server{
		sessionService:   sessionService,
		sessionCache:     sessioninfra.NewCache(sessionService),
		requestLogWorker: requestLogWorker,
	}
	profile := routingdomain.RouteProfile{
		Platform:          kernel.PlatformClaude,
		DefaultProviderID: intPtr(2),
		Providers: []routingdomain.Provider{
			{
				ID:       1,
				Name:     "provider-a",
				APIURL:   upstream.URL,
				APIKey:   "test-key-a",
				Position: 1,
			},
			{
				ID:       2,
				Name:     "provider-b",
				APIURL:   upstream.URL,
				APIKey:   "test-key-b",
				Position: 2,
			},
		},
	}

	done := make(chan error, 1)
	go func() {
		_, err := server.routeToManualProvider(newClaudeRelayContext(profile, "session-race"))
		done <- err
	}()

	<-firstStarted
	if err := sessionService.Unbind("claude", "session-race"); err != nil {
		t.Fatalf("unbind session: %v", err)
	}
	close(releaseFirst)
	if err := <-done; err != nil {
		t.Fatalf("first route: %v", err)
	}
	if got := sessionRepo.bindings[sessionRepo.key("claude", "session-race")]; got != "" {
		t.Fatalf("expected stale in-flight success not to restore binding, got %q", got)
	}
	if sessionRepo.updateCalls != 0 {
		t.Fatalf("expected stale in-flight success not to update DB, got %d calls", sessionRepo.updateCalls)
	}

	ok, err := server.routeToManualProvider(newClaudeRelayContext(profile, "session-race"))
	if err != nil {
		t.Fatalf("second route: %v", err)
	}
	if !ok {
		t.Fatal("expected second route to succeed")
	}
	if got := sessionRepo.bindings[sessionRepo.key("claude", "session-race")]; got != "provider-b" {
		t.Fatalf("expected next request to rebind default provider, got %q", got)
	}
}

func TestRouteToManualProviderRewritesMappedModelAndInjectsCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var gotBody []byte
	var gotAuth, gotAPIKey string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotAuth = r.Header.Get("Authorization")
		gotAPIKey = r.Header.Get("X-Api-Key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	server := &Server{}
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	ginCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	ok, err := server.routeToManualProvider(&RelayContext{
		GinCtx:    ginCtx,
		Platform:  kernel.PlatformClaude,
		BodyBytes: []byte(`{"model":"claude-sonnet-4","stream":false}`),
		Query:     map[string]string{},
		Headers: map[string]string{
			"Accept":        "application/json",
			"Authorization": "Bearer portico",
			"X-Api-Key":     "sk-user-real",
		},
		Profile: routingdomain.RouteProfile{
			Platform:          kernel.PlatformClaude,
			DefaultProviderID: intPtr(1),
			Providers: []routingdomain.Provider{
				{
					ID:       1,
					Name:     "mapped-provider",
					APIURL:   upstream.URL,
					APIKey:   "sk-provider",
					Position: 1,
					ModelMapping: map[string]string{
						"claude-*": "vendor/claude-*",
					},
				},
			},
		},
		RequestMeta: RequestMeta{
			Endpoint:          "/v1/messages",
			RequestedModel:    "claude-sonnet-4",
			BodyHasModelField: true,
		},
	})
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if !ok {
		t.Fatal("expected mapped-model route to succeed")
	}
	if got := string(gotBody); !strings.Contains(got, `"model":"vendor/claude-sonnet-4"`) {
		t.Fatalf("upstream body model not rewritten: %s", got)
	}
	if gotAuth != "Bearer sk-provider" {
		t.Fatalf("Authorization = %q, want provider bearer", gotAuth)
	}
	if gotAPIKey != "sk-provider" {
		t.Fatalf("X-Api-Key = %q, want provider key (client key must not leak)", gotAPIKey)
	}
}

func TestRouteToManualProviderPassesThroughUpstreamError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var logBuf bytes.Buffer
	log.SetOutput(&logBuf)
	defer log.SetOutput(os.Stderr)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "17")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"type":"rate_limit_error","message":"slow down"}}`))
	}))
	defer upstream.Close()

	server := &Server{}
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	ginCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	profile := routingdomain.RouteProfile{
		Platform:          kernel.PlatformClaude,
		DefaultProviderID: intPtr(1),
		Providers: []routingdomain.Provider{
			{ID: 1, Name: "p", APIURL: upstream.URL, APIKey: "k", Position: 1},
		},
	}
	ok, err := server.routeToManualProvider(&RelayContext{
		GinCtx:    ginCtx,
		Platform:  kernel.PlatformClaude,
		BodyBytes: []byte(`{"model":"claude-sonnet-4"}`),
		Query:     map[string]string{},
		Headers:   map[string]string{"Accept": "application/json"},
		Profile:   profile,
		RequestMeta: RequestMeta{
			Endpoint:          "/v1/messages",
			RequestedModel:    "claude-sonnet-4",
			BodyHasModelField: true,
		},
	})
	if ok {
		t.Fatal("expected non-2xx upstream to be reported as failure (no session binding)")
	}
	if err == nil {
		t.Fatal("expected upstream status error")
	}
	// 上游错误必须原样透传：状态码、限流头与错误体都不能丢。
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("client status = %d, want 429", recorder.Code)
	}
	if recorder.Header().Get("Retry-After") != "17" {
		t.Fatalf("Retry-After lost: %q", recorder.Header().Get("Retry-After"))
	}
	if !strings.Contains(recorder.Body.String(), "rate_limit_error") {
		t.Fatalf("upstream error body lost: %s", recorder.Body.String())
	}
	// 错误体须同步落入应用日志（WARN 级别），供 UI"应用日志"页查看。
	logged := logBuf.String()
	for _, want := range []string{"[WARN] [relay]", "provider=p", "status=429", "slow down"} {
		if !strings.Contains(logged, want) {
			t.Fatalf("应用日志缺少 %q: %s", want, logged)
		}
	}
}

func newClaudeRelayContext(profile routingdomain.RouteProfile, sessionID string) *RelayContext {
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	ginCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	return &RelayContext{
		GinCtx:    ginCtx,
		Platform:  kernel.PlatformClaude,
		BodyBytes: []byte(`{"model":"claude-sonnet-4"}`),
		Query:     map[string]string{},
		Headers:   map[string]string{"Accept": "application/json"},
		Profile:   profile,
		RequestMeta: RequestMeta{
			Endpoint:          "/v1/messages",
			RequestedModel:    "claude-sonnet-4",
			SessionID:         sessionID,
			BodyHasModelField: true,
		},
	}
}

func intPtr(value int) *int {
	return &value
}
