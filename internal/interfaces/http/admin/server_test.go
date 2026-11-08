package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

const testToken = "s3cret-token"

func newTestServer(t *testing.T, opts Options) (*Server, *SampleFacade) {
	t.Helper()
	facade := &SampleFacade{}
	registry, err := NewRegistry(facade)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if opts.Tokens == nil {
		opts.Tokens = StaticToken(testToken)
	}
	opts.Registry = registry
	if opts.Version == "" {
		opts.Version = "v9.9.9"
	}
	return NewServer(opts), facade
}

type reqSpec struct {
	method      string
	path        string
	body        string
	remoteAddr  string
	host        string
	contentType string
	headers     map[string]string
}

func do(t *testing.T, handler http.Handler, spec reqSpec) *httptest.ResponseRecorder {
	t.Helper()
	if spec.method == "" {
		spec.method = http.MethodPost
	}
	req := httptest.NewRequest(spec.method, spec.path, strings.NewReader(spec.body))
	req.RemoteAddr = spec.remoteAddr
	if req.RemoteAddr == "" {
		req.RemoteAddr = "127.0.0.1:50000"
	}
	req.Host = spec.host
	if req.Host == "" {
		req.Host = "127.0.0.1:18101"
	}
	if spec.contentType == "" && spec.method == http.MethodPost {
		spec.contentType = "application/json"
	}
	if spec.contentType != "-" {
		req.Header.Set("Content-Type", spec.contentType)
	}
	for k, v := range spec.headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestAuthRules(t *testing.T) {
	server, _ := newTestServer(t, Options{})
	handler := server.Handler()
	bearer := map[string]string{"Authorization": "Bearer " + testToken}

	cases := []struct {
		name string
		spec reqSpec
		want int
	}{
		{"loopback without token", reqSpec{path: "/api/rpc/SampleFacade.Ping"}, http.StatusOK},
		{"loopback via localhost host", reqSpec{path: "/api/rpc/SampleFacade.Ping", host: "localhost:18101"}, http.StatusOK},
		{"ipv6 loopback", reqSpec{path: "/api/rpc/SampleFacade.Ping", remoteAddr: "[::1]:50000", host: "[::1]:18101"}, http.StatusOK},
		{"dns rebinding host", reqSpec{path: "/api/rpc/SampleFacade.Ping", host: "evil.example:18101"}, http.StatusUnauthorized},
		{"proxied loopback", reqSpec{path: "/api/rpc/SampleFacade.Ping", headers: map[string]string{"X-Forwarded-For": "203.0.113.9"}}, http.StatusUnauthorized},
		{"remote without token", reqSpec{path: "/api/rpc/SampleFacade.Ping", remoteAddr: "203.0.113.9:40000", host: "portico.example:18101"}, http.StatusUnauthorized},
		{"remote wrong token", reqSpec{path: "/api/rpc/SampleFacade.Ping", remoteAddr: "203.0.113.9:40000", host: "portico.example:18101", headers: map[string]string{"Authorization": "Bearer nope"}}, http.StatusUnauthorized},
		{"remote basic scheme", reqSpec{path: "/api/rpc/SampleFacade.Ping", remoteAddr: "203.0.113.9:40000", host: "portico.example:18101", headers: map[string]string{"Authorization": "Basic " + testToken}}, http.StatusUnauthorized},
		{"remote valid token", reqSpec{path: "/api/rpc/SampleFacade.Ping", remoteAddr: "203.0.113.9:40000", host: "portico.example:18101", headers: bearer}, http.StatusOK},
		{"cross-site origin", reqSpec{path: "/api/rpc/SampleFacade.Ping", headers: map[string]string{"Origin": "https://evil.example"}}, http.StatusForbidden},
		{"cross-site fetch metadata", reqSpec{path: "/api/rpc/SampleFacade.Ping", headers: map[string]string{"Sec-Fetch-Site": "cross-site"}}, http.StatusForbidden},
		{"same origin", reqSpec{path: "/api/rpc/SampleFacade.Ping", headers: map[string]string{"Origin": "http://127.0.0.1:18101"}}, http.StatusOK},
		{"text/plain post", reqSpec{path: "/api/rpc/SampleFacade.Ping", contentType: "text/plain"}, http.StatusUnsupportedMediaType},
		{"missing content type", reqSpec{path: "/api/rpc/SampleFacade.Ping", contentType: "-"}, http.StatusUnsupportedMediaType},
		{"json with charset", reqSpec{path: "/api/rpc/SampleFacade.Ping", contentType: "application/json; charset=utf-8"}, http.StatusOK},
		{"session probe remote", reqSpec{method: http.MethodGet, path: "/api/session", remoteAddr: "203.0.113.9:40000", host: "portico.example"}, http.StatusUnauthorized},
		{"session probe remote token", reqSpec{method: http.MethodGet, path: "/api/session", remoteAddr: "203.0.113.9:40000", host: "portico.example", headers: bearer}, http.StatusOK},
		{"health is public", reqSpec{method: http.MethodGet, path: "/api/health", remoteAddr: "203.0.113.9:40000", host: "portico.example"}, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, handler, tc.spec)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d, body=%s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestAlwaysRequireTokenDisablesLoopbackExemption(t *testing.T) {
	server, _ := newTestServer(t, Options{AlwaysRequireToken: true})
	handler := server.Handler()
	if rec := do(t, handler, reqSpec{path: "/api/rpc/SampleFacade.Ping"}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("loopback without token: status = %d, want 401", rec.Code)
	}
	rec := do(t, handler, reqSpec{path: "/api/rpc/SampleFacade.Ping", headers: map[string]string{"Authorization": "Bearer " + testToken}})
	if rec.Code != http.StatusOK {
		t.Fatalf("loopback with token: status = %d, want 200", rec.Code)
	}
}

func TestEmptyTokenNeverAuthenticates(t *testing.T) {
	server, _ := newTestServer(t, Options{Tokens: StaticToken("")})
	rec := do(t, server.Handler(), reqSpec{
		path:       "/api/rpc/SampleFacade.Ping",
		remoteAddr: "203.0.113.9:40000",
		host:       "portico.example",
		headers:    map[string]string{"Authorization": "Bearer "},
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestRPCResponses(t *testing.T) {
	server, _ := newTestServer(t, Options{})
	handler := server.Handler()

	rec := do(t, handler, reqSpec{path: "/api/rpc/SampleFacade.Save", body: `[{"name":"x"}]`})
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"id":1,"name":"x"}` {
		t.Fatalf("Save: %d %s", rec.Code, rec.Body.String())
	}

	rec = do(t, handler, reqSpec{path: "/api/rpc/SampleFacade.Save", body: `[{"name":""}]`})
	if rec.Code != http.StatusInternalServerError || errorMessage(t, rec) != "name is required" {
		t.Fatalf("Save error: %d %s", rec.Code, rec.Body.String())
	}

	rec = do(t, handler, reqSpec{path: "/api/rpc/SampleFacade.Missing"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing: %d %s", rec.Code, rec.Body.String())
	}

	rec = do(t, handler, reqSpec{path: "/api/rpc/SampleFacade.Touch"})
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "null" {
		t.Fatalf("Touch: %d %s", rec.Code, rec.Body.String())
	}
}

func TestHealthReportsVersion(t *testing.T) {
	server, _ := newTestServer(t, Options{Version: "v1.2.3"})
	rec := do(t, server.Handler(), reqSpec{method: http.MethodGet, path: "/api/health"})
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ok" || body["version"] != "v1.2.3" || body["pid"] == nil {
		t.Fatalf("health = %v", body)
	}
}

func TestShutdownEndpoint(t *testing.T) {
	called := make(chan struct{})
	server, _ := newTestServer(t, Options{OnShutdown: func() { close(called) }})
	handler := server.Handler()

	rec := do(t, handler, reqSpec{path: "/api/shutdown", remoteAddr: "203.0.113.9:40000", host: "portico.example"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("remote shutdown without token: %d", rec.Code)
	}
	rec = do(t, handler, reqSpec{path: "/api/shutdown"})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("shutdown: %d %s", rec.Code, rec.Body.String())
	}
	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("OnShutdown was not called")
	}
}

func TestStaticAssets(t *testing.T) {
	assets := fstest.MapFS{
		"index.html":    {Data: []byte("<html>portico</html>")},
		"assets/app.js": {Data: []byte("console.log(1)")},
	}
	server, _ := newTestServer(t, Options{Assets: assets})
	handler := server.Handler()

	remote := reqSpec{method: http.MethodGet, remoteAddr: "203.0.113.9:40000", host: "portico.example"}

	spec := remote
	spec.path = "/"
	rec := do(t, handler, spec)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "portico") {
		t.Fatalf("index: %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("index should not be cached, got %q", rec.Header().Get("Cache-Control"))
	}

	spec.path = "/assets/app.js"
	rec = do(t, handler, spec)
	if rec.Code != http.StatusOK || rec.Body.String() != "console.log(1)" {
		t.Fatalf("asset: %d %s", rec.Code, rec.Body.String())
	}

	spec.path = "/some/deep/link"
	rec = do(t, handler, spec)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "portico") {
		t.Fatalf("fallback: %d %s", rec.Code, rec.Body.String())
	}

	spec.path = "/api/unknown"
	rec = do(t, handler, spec)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown api should 404, got %d", rec.Code)
	}
}

func TestFileTokenPicksUpReset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "admin_token")
	if err := os.WriteFile(path, []byte("first\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tokens := NewFileToken(path, "first")
	if got := tokens.Token(); got != "first" {
		t.Fatalf("token = %q", got)
	}
	if err := os.WriteFile(path, []byte("second\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// 部分文件系统 mtime 精度较粗，显式推进以避免偶发失败。
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	if got := tokens.Token(); got != "second" {
		t.Fatalf("token after reset = %q", got)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if got := tokens.Token(); got != "second" {
		t.Fatalf("token should be kept when file disappears, got %q", got)
	}
}

func TestServerListensAndServes(t *testing.T) {
	server, _ := newTestServer(t, Options{Addr: "127.0.0.1:0"})
	if err := server.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = server.Stop() })

	req, _ := http.NewRequest(http.MethodPost, "http://"+server.Addr()+"/api/rpc/SampleFacade.Echo", strings.NewReader(`["a",1]`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || strings.TrimSpace(string(body)) != `["a"]` {
		t.Fatalf("Echo over TCP: %d %s", resp.StatusCode, body)
	}
}

func errorMessage(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body %q: %v", rec.Body.String(), err)
	}
	return body.Error
}
