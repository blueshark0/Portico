package relay

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	routingdomain "portico/internal/routing/domain"
	sessionapp "portico/internal/sessions/application"
	sessioninfra "portico/internal/sessions/infrastructure"
	"portico/internal/shared/kernel"
)

func TestCodexBackgroundTaskKeepsBindingAndRecordsVisibility(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	sessionRepo := &relaySessionRepoStub{}
	sessionService := sessionapp.NewService(sessionRepo)
	server := &Server{
		sessionService: sessionService,
		sessionCache:   sessioninfra.NewCache(sessionService),
	}
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	ginCtx.Request = httptest.NewRequest(http.MethodPost, "/responses", nil)

	ok, err := server.routeToManualProvider(&RelayContext{
		GinCtx:    ginCtx,
		Platform:  kernel.PlatformCodex,
		BodyBytes: []byte(`{"model":"gpt-5"}`),
		Query:     map[string]string{},
		Headers:   map[string]string{},
		Profile: routingdomain.RouteProfile{
			Platform:          kernel.PlatformCodex,
			DefaultProviderID: intPtr(1),
			Providers: []routingdomain.Provider{{
				ID:       1,
				Name:     "provider-a",
				APIURL:   upstream.URL,
				APIKey:   "test-key",
				Position: 1,
			}},
		},
		RequestMeta: RequestMeta{
			Endpoint:          "/responses",
			RequestedModel:    "gpt-5",
			SessionID:         "background-session",
			BodyHasModelField: true,
			IsBackgroundTask:  true,
		},
	})
	if err != nil || !ok {
		t.Fatalf("route failed: ok=%v err=%v", ok, err)
	}
	if got := sessionRepo.bindings["codex:background-session"]; got != "provider-a" {
		t.Fatalf("background task must retain provider stickiness, got %q", got)
	}
	background, recorded := sessionRepo.backgroundRecords["codex:background-session"]
	if !recorded || !background {
		t.Fatalf("background visibility was not recorded: %+v", sessionRepo.backgroundRecords)
	}
}
