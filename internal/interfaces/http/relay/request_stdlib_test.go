package relay

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	routingdomain "portico/internal/routing/domain"
	"portico/internal/shared/kernel"
)

func TestForwardRequestMergesQueryAndDefaultsContentType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var gotQuery url.Values
	var gotContentType string
	var gotBody []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	ginCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	server := &Server{}
	body := []byte(`{"model":"claude-sonnet-4"}`)
	ok, err := server.forwardRequest(&ForwardContext{
		GinCtx:    ginCtx,
		Platform:  kernel.PlatformClaude,
		Provider:  &routingdomain.Provider{Name: "p", APIURL: upstream.URL, APIKey: "sk-provider"},
		Endpoint:  "/v1/messages",
		Query:     map[string]string{"alt": "sse", "q": "hello world"},
		Headers:   map[string]string{"Accept": "application/json"},
		BodyBytes: body,
		Model:     "claude-sonnet-4",
	})
	if err != nil || !ok {
		t.Fatalf("forward = (%v, %v), want success", ok, err)
	}
	if gotQuery.Get("alt") != "sse" || gotQuery.Get("q") != "hello world" {
		t.Fatalf("query = %v, want alt=sse&q=hello world", gotQuery)
	}
	if gotContentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", gotContentType)
	}
	if string(gotBody) != string(body) {
		t.Fatalf("body = %s, want %s", gotBody, body)
	}
}
