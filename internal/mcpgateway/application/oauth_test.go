package application

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"portico/internal/mcpgateway/domain"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const testRedirectURL = "http://127.0.0.1:18100/mcp/oauth/callback"

// ---- 假凭证存储 ----

type memCredentialStore struct {
	mu        sync.Mutex
	creds     map[int64]domain.OAuthCredential
	deleteErr error
}

func (s *memCredentialStore) failDelete(fail bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteErr = nil
	if fail {
		s.deleteErr = errors.New("delete failed")
	}
}

func newMemCredentialStore() *memCredentialStore {
	return &memCredentialStore{creds: make(map[int64]domain.OAuthCredential)}
}

func (s *memCredentialStore) GetOAuthCredential(_ context.Context, id int64) (*domain.OAuthCredential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cred, ok := s.creds[id]
	if !ok {
		return nil, nil
	}
	return &cred, nil
}

func (s *memCredentialStore) SaveOAuthCredential(_ context.Context, cred domain.OAuthCredential) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.creds[cred.ServerID] = cred
	return nil
}

func (s *memCredentialStore) DeleteOAuthCredential(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deleteErr != nil {
		return s.deleteErr
	}
	delete(s.creds, id)
	return nil
}

func (s *memCredentialStore) get(id int64) (domain.OAuthCredential, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cred, ok := s.creds[id]
	return cred, ok
}

func (s *memCredentialStore) mutate(id int64, fn func(*domain.OAuthCredential)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cred := s.creds[id]
	fn(&cred)
	s.creds[id] = cred
}

// ---- 假的“受保护 MCP 服务 + 授权服务器” ----

type fakeOAuthUpstream struct {
	t   *testing.T
	srv *httptest.Server

	mu              sync.Mutex
	registerStatus  int  // 0 → 201
	noRegistration  bool // 元数据不含 registration_endpoint
	noAuth          bool // /mcp 不要求授权
	rejectAllTokens bool // /mcp 对任何令牌都返回 401
	expiresIn       int  // 0 → 响应里不带 expires_in
	registeredNames []string
	registerCalls   int
	refreshCalls    int
	tokenClientIDs  []string
	codes           map[string]string // code → PKCE challenge
	accessTokens    map[string]bool
	refreshTokens   map[string]bool
	seq             int
}

func newFakeOAuthUpstream(t *testing.T) *fakeOAuthUpstream {
	t.Helper()
	f := &fakeOAuthUpstream{
		t:             t,
		expiresIn:     3600,
		codes:         make(map[string]string),
		accessTokens:  make(map[string]bool),
		refreshTokens: make(map[string]bool),
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "protected", Version: "v0"}, nil)
	server.AddTool(&mcp.Tool{Name: "echo", InputSchema: objectSchema()},
		func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(req.Params.Arguments)}}}, nil
		})
	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)

	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		allowed := f.noAuth || (f.accessTokens[token] && !f.rejectAllTokens)
		f.mu.Unlock()
		if !allowed {
			w.Header().Set("WWW-Authenticate",
				fmt.Sprintf(`Bearer resource_metadata="%s/.well-known/oauth-protected-resource"`, f.srv.URL))
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		mcpHandler.ServeHTTP(w, r)
	})
	mux.HandleFunc("/.well-known/oauth-protected-resource", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"resource":              f.srv.URL + "/mcp",
			"authorization_servers": []string{f.srv.URL},
		})
	})
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		noRegistration := f.noRegistration
		f.mu.Unlock()
		meta := map[string]any{
			"issuer":                                f.srv.URL,
			"authorization_endpoint":                f.srv.URL + "/authorize",
			"token_endpoint":                        f.srv.URL + "/token",
			"response_types_supported":              []string{"code"},
			"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
			"code_challenge_methods_supported":      []string{"S256"},
			"token_endpoint_auth_methods_supported": []string{"none", "client_secret_post"},
		}
		if !noRegistration {
			meta["registration_endpoint"] = f.srv.URL + "/register"
		}
		writeJSON(w, http.StatusOK, meta)
	})
	mux.HandleFunc("/register", f.handleRegister)
	mux.HandleFunc("/authorize", f.handleAuthorize)
	mux.HandleFunc("/token", f.handleToken)

	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func (f *fakeOAuthUpstream) handleRegister(w http.ResponseWriter, r *http.Request) {
	var meta struct {
		ClientName   string   `json:"client_name"`
		RedirectURIs []string `json:"redirect_uris"`
	}
	_ = json.NewDecoder(r.Body).Decode(&meta)
	f.mu.Lock()
	f.registerCalls++
	f.registeredNames = append(f.registeredNames, meta.ClientName)
	status := f.registerStatus
	clientID := fmt.Sprintf("dyn-client-%d", f.registerCalls)
	f.mu.Unlock()
	if status != 0 {
		http.Error(w, "client not allowed", status)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"client_id":                  clientID,
		"client_name":                meta.ClientName,
		"redirect_uris":              meta.RedirectURIs,
		"token_endpoint_auth_method": "none",
	})
}

func (f *fakeOAuthUpstream) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f.mu.Lock()
	f.seq++
	code := fmt.Sprintf("code-%d", f.seq)
	f.codes[code] = q.Get("code_challenge")
	f.mu.Unlock()
	redirect, _ := url.Parse(q.Get("redirect_uri"))
	back := redirect.Query()
	back.Set("code", code)
	back.Set("state", q.Get("state"))
	redirect.RawQuery = back.Encode()
	http.Redirect(w, r, redirect.String(), http.StatusFound)
}

func (f *fakeOAuthUpstream) handleToken(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tokenClientIDs = append(f.tokenClientIDs, r.PostForm.Get("client_id"))
	switch r.PostForm.Get("grant_type") {
	case "authorization_code":
		challenge, ok := f.codes[r.PostForm.Get("code")]
		delete(f.codes, r.PostForm.Get("code"))
		sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
		if !ok || base64.RawURLEncoding.EncodeToString(sum[:]) != challenge {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_grant"})
			return
		}
	case "refresh_token":
		f.refreshCalls++
		token := r.PostForm.Get("refresh_token")
		if !f.refreshTokens[token] {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_grant"})
			return
		}
		delete(f.refreshTokens, token) // 轮换：旧刷新令牌立即作废
	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unsupported_grant_type"})
		return
	}
	f.seq++
	access := fmt.Sprintf("access-%d", f.seq)
	refresh := fmt.Sprintf("refresh-%d", f.seq)
	f.accessTokens[access] = true
	f.refreshTokens[refresh] = true
	body := map[string]any{"access_token": access, "refresh_token": refresh, "token_type": "Bearer"}
	if f.expiresIn > 0 {
		body["expires_in"] = f.expiresIn
	}
	writeJSON(w, http.StatusOK, body)
}

func (f *fakeOAuthUpstream) set(fn func(*fakeOAuthUpstream)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

func (f *fakeOAuthUpstream) snapshot() (registerCalls, refreshCalls int, names, tokenClientIDs []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.registerCalls, f.refreshCalls, append([]string{}, f.registeredNames...), append([]string{}, f.tokenClientIDs...)
}

func (f *fakeOAuthUpstream) revokeAccessTokens() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.accessTokens = make(map[string]bool)
}

func (f *fakeOAuthUpstream) revokeRefreshTokens() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refreshTokens = make(map[string]bool)
}

// ---- 测试装配 ----

type oauthFixture struct {
	t        *testing.T
	upstream *fakeOAuthUpstream
	repo     *fakeRepo
	store    *memCredentialStore
	manager  *OAuthManager
	agg      *Aggregator
	svc      *Service
	serverID int64
}

func newOAuthFixture(t *testing.T, mutate func(*domain.McpServer)) *oauthFixture {
	t.Helper()
	upstream := newFakeOAuthUpstream(t)
	cfg := domain.McpServer{
		Name:     "protected",
		Type:     domain.ServerTypeHTTP,
		URL:      upstream.srv.URL + "/mcp",
		AuthType: domain.AuthTypeOAuth,
		Enabled:  true,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("测试配置不合法: %v", err)
	}
	repo := &fakeRepo{}
	if err := repo.Create(context.Background(), &cfg); err != nil {
		t.Fatal(err)
	}
	f := &oauthFixture{t: t, upstream: upstream, repo: repo, store: newMemCredentialStore(), serverID: cfg.ID}
	f.boot()
	return f
}

// boot 用同一份存储重建管理器与聚合器，相当于应用重启。
func (f *oauthFixture) boot() {
	f.t.Helper()
	if f.agg != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = f.agg.Close(ctx)
		cancel()
	}
	f.manager = NewOAuthManager(f.store, OAuthManagerOptions{RedirectURL: testRedirectURL, AppVersion: "test"})
	agg := NewAggregator(f.repo, AggregatorOptions{AppVersion: "test", OAuth: f.manager})
	f.t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = agg.Close(ctx)
	})
	f.agg = agg
	f.svc = NewService(f.repo, agg, f.manager)
	// 授权完成后的重连由 reconciler 驱动，必须让它真的跑起来。
	agg.Start(context.Background())
}

func (f *oauthFixture) sync() {
	f.t.Helper()
	if err := f.agg.Sync(context.Background()); err != nil {
		f.t.Fatalf("Sync: %v", err)
	}
}

// browse 扮演浏览器：打开授权 URL，把授权服务器的重定向交给回调 handler。
func (f *oauthFixture) browse(authURL string) *httptest.ResponseRecorder {
	f.t.Helper()
	return f.deliver(f.redirectTarget(authURL))
}

func (f *oauthFixture) redirectTarget(authURL string) *url.URL {
	f.t.Helper()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Get(authURL)
	if err != nil {
		f.t.Fatalf("打开授权 URL: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		f.t.Fatalf("授权端点应重定向，实际 %d", resp.StatusCode)
	}
	target, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		f.t.Fatal(err)
	}
	if got := target.Scheme + "://" + target.Host + target.Path; got != testRedirectURL {
		f.t.Fatalf("重定向目标 %q 不是注册的回调地址", got)
	}
	return target
}

func (f *oauthFixture) deliver(target *url.URL) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	f.manager.CallbackHandler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target.RequestURI(), nil))
	return recorder
}

func (f *oauthFixture) authorize() {
	f.t.Helper()
	authURL, err := f.svc.StartOAuth(context.Background(), f.serverID)
	if err != nil {
		f.t.Fatalf("StartOAuth: %v", err)
	}
	if recorder := f.browse(authURL); recorder.Code != http.StatusOK {
		f.t.Fatalf("回调页应 200，实际 %d: %s", recorder.Code, recorder.Body.String())
	}
}

// ---- 测试 ----

// 完整链路：待授权 → 用户授权 → 自动重连 → 工具可用；重启后无需再次授权。
func TestOAuthAuthorizeConnectAndSurviveRestart(t *testing.T) {
	f := newOAuthFixture(t, nil)

	f.sync()
	waitForState(t, f.agg, f.serverID, StateNeedsAuth)
	if registerCalls, _, _, _ := f.upstream.snapshot(); registerCalls != 0 {
		t.Fatalf("后台连接不得注册客户端，实际注册 %d 次", registerCalls)
	}
	if _, retry := f.agg.nextRetryDelay(); retry {
		t.Fatal("待授权状态不应进入退避重连")
	}

	f.authorize()
	waitForState(t, f.agg, f.serverID, StateConnected)
	if !hasTool(f.agg, "protected__echo") {
		t.Fatal("授权后应注册上游工具")
	}
	res := callThrough(t, f.agg, "protected__echo", `{"a":1}`)
	if res.IsError || res.Content[0].(*mcp.TextContent).Text != `{"a":1}` {
		t.Fatalf("工具调用结果异常: %+v", res)
	}

	cred, ok := f.store.get(f.serverID)
	if !ok || cred.AccessToken == "" || cred.RefreshToken == "" || cred.ClientID == "" {
		t.Fatalf("凭证未完整落库: %+v", cred)
	}
	if cred.RedirectURL != testRedirectURL || cred.TokenURL != f.upstream.srv.URL+"/token" {
		t.Fatalf("凭证端点信息异常: %+v", cred)
	}
	registerCalls, _, names, _ := f.upstream.snapshot()
	if registerCalls != 1 || names[0] != domain.DefaultOAuthClientName {
		t.Fatalf("应以默认名称注册一次，实际 %d 次 %v", registerCalls, names)
	}

	// 重启：只凭已保存的凭证连接。
	f.boot()
	f.sync()
	waitForState(t, f.agg, f.serverID, StateConnected)
	if registerCalls, _, _, _ := f.upstream.snapshot(); registerCalls != 1 {
		t.Fatalf("重启后不应重新注册，累计 %d 次", registerCalls)
	}
}

// 访问令牌过期：连接时刷新，轮换后的刷新令牌落库。
func TestOAuthRefreshRotationPersisted(t *testing.T) {
	f := newOAuthFixture(t, nil)
	f.sync()
	waitForState(t, f.agg, f.serverID, StateNeedsAuth)
	f.authorize()
	waitForState(t, f.agg, f.serverID, StateConnected)
	before, _ := f.store.get(f.serverID)

	f.store.mutate(f.serverID, func(c *domain.OAuthCredential) { c.Expiry = time.Now().Add(-time.Hour) })
	f.boot()
	f.sync()
	waitForState(t, f.agg, f.serverID, StateConnected)

	after, _ := f.store.get(f.serverID)
	if after.RefreshToken == before.RefreshToken || after.AccessToken == before.AccessToken {
		t.Fatalf("刷新后的令牌未落库: before=%+v after=%+v", before, after)
	}
	if _, refreshCalls, _, _ := f.upstream.snapshot(); refreshCalls != 1 {
		t.Fatalf("应恰好刷新一次，实际 %d 次", refreshCalls)
	}
}

// 授权服务器没给有效期、访问令牌被吊销：上游 401 触发一次强制刷新后恢复。
func TestOAuthRefreshOnRejectedAccessToken(t *testing.T) {
	f := newOAuthFixture(t, nil)
	f.upstream.set(func(u *fakeOAuthUpstream) { u.expiresIn = 0 })
	f.sync()
	waitForState(t, f.agg, f.serverID, StateNeedsAuth)
	f.authorize()
	waitForState(t, f.agg, f.serverID, StateConnected)

	f.upstream.revokeAccessTokens()
	f.boot()
	f.sync()
	waitForState(t, f.agg, f.serverID, StateConnected)
	if _, refreshCalls, _, _ := f.upstream.snapshot(); refreshCalls != 1 {
		t.Fatalf("应恰好刷新一次，实际 %d 次", refreshCalls)
	}
}

// 刷新令牌失效：回到待授权而不是按故障反复重连。
func TestOAuthRefreshRejectedFallsBackToNeedsAuth(t *testing.T) {
	f := newOAuthFixture(t, nil)
	f.sync()
	waitForState(t, f.agg, f.serverID, StateNeedsAuth)
	f.authorize()
	waitForState(t, f.agg, f.serverID, StateConnected)

	f.upstream.revokeRefreshTokens()
	f.store.mutate(f.serverID, func(c *domain.OAuthCredential) { c.Expiry = time.Now().Add(-time.Hour) })
	f.boot()
	f.sync()
	waitForState(t, f.agg, f.serverID, StateNeedsAuth)
	if _, retry := f.agg.nextRetryDelay(); retry {
		t.Fatal("刷新令牌失效后不应进入退避重连")
	}

	// 重新授权即可恢复。
	f.authorize()
	waitForState(t, f.agg, f.serverID, StateConnected)
}

// 上游连刚签发的令牌都不认：再刷新也没用，应停在待授权并说明原因，
// 而不是每次重连都消耗一枚刷新令牌。
func TestOAuthFreshTokenRejectedDoesNotLoopRefreshing(t *testing.T) {
	f := newOAuthFixture(t, nil)
	f.sync()
	waitForState(t, f.agg, f.serverID, StateNeedsAuth)

	// 授权服务器照常发令牌，但资源服务器一律不认。
	f.upstream.set(func(u *fakeOAuthUpstream) { u.rejectAllTokens = true })
	f.authorize()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && statusOf(f.agg, f.serverID).Error == "" {
		time.Sleep(10 * time.Millisecond)
	}
	st := statusOf(f.agg, f.serverID)
	if st.State != StateNeedsAuth || !strings.Contains(st.Error, "仍被上游拒绝") {
		t.Fatalf("应停在待授权并给出原因: %+v", st)
	}
	if _, refreshCalls, _, _ := f.upstream.snapshot(); refreshCalls != 0 {
		t.Fatalf("不应为刚签发的令牌再发起刷新，实际 %d 次", refreshCalls)
	}
	if _, retry := f.agg.nextRetryDelay(); retry {
		t.Fatal("不应进入退避重连")
	}
}

// 运行中令牌失效且无法刷新：工具调用报错，上游转为待授权。
func TestOAuthExpiryDuringSessionMovesToNeedsAuth(t *testing.T) {
	f := newOAuthFixture(t, nil)
	f.upstream.set(func(u *fakeOAuthUpstream) { u.expiresIn = 0 })
	f.sync()
	waitForState(t, f.agg, f.serverID, StateNeedsAuth)
	f.authorize()
	waitForState(t, f.agg, f.serverID, StateConnected)

	f.upstream.revokeAccessTokens()
	f.upstream.revokeRefreshTokens()
	res := callThrough(t, f.agg, "protected__echo", `{}`)
	if !res.IsError {
		t.Fatalf("令牌失效后的调用应报错: %+v", res)
	}
	waitForState(t, f.agg, f.serverID, StateNeedsAuth)
}

// 注册被拒（只放行白名单客户端的服务）：错误直接返回并给出可操作的提示；
// 自定义客户端名称原样出现在注册请求里。
func TestOAuthRegistrationRejected(t *testing.T) {
	f := newOAuthFixture(t, func(cfg *domain.McpServer) { cfg.OAuthClientName = "Custom Client" })
	f.upstream.set(func(u *fakeOAuthUpstream) { u.registerStatus = http.StatusForbidden })
	f.sync()
	waitForState(t, f.agg, f.serverID, StateNeedsAuth)

	_, err := f.svc.StartOAuth(context.Background(), f.serverID)
	if err == nil || !strings.Contains(err.Error(), "白名单") {
		t.Fatalf("应返回注册被拒的提示，实际: %v", err)
	}
	if _, _, names, _ := f.upstream.snapshot(); len(names) != 1 || names[0] != "Custom Client" {
		t.Fatalf("注册请求应使用自定义客户端名称，实际 %v", names)
	}
	if _, ok := f.store.get(f.serverID); ok {
		t.Fatal("注册失败不应留下凭证")
	}
	if st := statusOf(f.agg, f.serverID); st.State != StateNeedsAuth {
		t.Fatalf("状态应保持待授权: %+v", st)
	}
}

// 授权服务器不支持动态注册：未填 Client ID 时给出提示；填了则直接用预注册客户端。
func TestOAuthPreregisteredClient(t *testing.T) {
	f := newOAuthFixture(t, nil)
	f.upstream.set(func(u *fakeOAuthUpstream) { u.noRegistration = true })
	f.sync()
	waitForState(t, f.agg, f.serverID, StateNeedsAuth)

	if _, err := f.svc.StartOAuth(context.Background(), f.serverID); err == nil || !strings.Contains(err.Error(), "Client ID") {
		t.Fatalf("应提示填写预注册的 Client ID，实际: %v", err)
	}

	servers, _ := f.repo.List(context.Background())
	cfg := servers[0]
	cfg.OAuthClientID = "pre-registered"
	if _, err := f.svc.Save(context.Background(), cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	f.sync()
	waitForState(t, f.agg, f.serverID, StateNeedsAuth)

	f.authorize()
	waitForState(t, f.agg, f.serverID, StateConnected)
	registerCalls, _, _, clientIDs := f.upstream.snapshot()
	if registerCalls != 0 {
		t.Fatalf("预注册客户端不应触发动态注册，实际 %d 次", registerCalls)
	}
	if len(clientIDs) == 0 || clientIDs[len(clientIDs)-1] != "pre-registered" {
		t.Fatalf("换令牌应使用预注册的 client_id，实际 %v", clientIDs)
	}
}

// state 是回调的唯一凭据：未知与重复使用都必须拒绝。
func TestOAuthCallbackStateIsSingleUse(t *testing.T) {
	f := newOAuthFixture(t, nil)
	f.sync()
	waitForState(t, f.agg, f.serverID, StateNeedsAuth)

	unknown, _ := url.Parse(testRedirectURL + "?code=x&state=not-a-real-state")
	if recorder := f.deliver(unknown); recorder.Code != http.StatusBadRequest {
		t.Fatalf("未知 state 应 400，实际 %d", recorder.Code)
	}
	missing, _ := url.Parse(testRedirectURL + "?code=x")
	if recorder := f.deliver(missing); recorder.Code != http.StatusBadRequest {
		t.Fatalf("缺少 state 应 400，实际 %d", recorder.Code)
	}

	authURL, err := f.svc.StartOAuth(context.Background(), f.serverID)
	if err != nil {
		t.Fatalf("StartOAuth: %v", err)
	}
	target := f.redirectTarget(authURL)
	if recorder := f.deliver(target); recorder.Code != http.StatusOK {
		t.Fatalf("首次回调应成功，实际 %d", recorder.Code)
	}
	if recorder := f.deliver(target); recorder.Code != http.StatusBadRequest {
		t.Fatalf("重复使用 state 应 400，实际 %d", recorder.Code)
	}
	waitForState(t, f.agg, f.serverID, StateConnected)
}

// 用户在授权页拒绝：回调页报失败，原因挂到待授权的上游上，且页面不回显查询参数。
func TestOAuthDeniedByUser(t *testing.T) {
	f := newOAuthFixture(t, nil)
	f.sync()
	waitForState(t, f.agg, f.serverID, StateNeedsAuth)

	authURL, err := f.svc.StartOAuth(context.Background(), f.serverID)
	if err != nil {
		t.Fatalf("StartOAuth: %v", err)
	}
	parsed, _ := url.Parse(authURL)
	denied, _ := url.Parse(testRedirectURL)
	query := url.Values{}
	query.Set("state", parsed.Query().Get("state"))
	query.Set("error", "access_denied")
	query.Set("error_description", "<script>alert(1)</script>")
	denied.RawQuery = query.Encode()

	recorder := f.deliver(denied)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("被拒绝的授权应返回 400，实际 %d", recorder.Code)
	}
	if strings.Contains(recorder.Body.String(), "<script>") {
		t.Fatal("回调页不得原样输出授权服务器回传的内容")
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && statusOf(f.agg, f.serverID).Error == "" {
		time.Sleep(10 * time.Millisecond)
	}
	st := statusOf(f.agg, f.serverID)
	if st.State != StateNeedsAuth || !strings.Contains(st.Error, "access_denied") {
		t.Fatalf("应保持待授权并带上失败原因: %+v", st)
	}
	if _, ok := f.store.get(f.serverID); ok {
		t.Fatal("被拒绝的授权不应留下凭证")
	}
}

// 端点或客户端身份变更后旧凭证作废；清除授权后回到待授权。
func TestOAuthCredentialLifecycle(t *testing.T) {
	f := newOAuthFixture(t, nil)
	f.sync()
	waitForState(t, f.agg, f.serverID, StateNeedsAuth)
	f.authorize()
	waitForState(t, f.agg, f.serverID, StateConnected)

	// 与身份无关的修改（超时）不动凭证。
	servers, _ := f.repo.List(context.Background())
	cfg := servers[0]
	cfg.TimeoutSec = 60
	if _, err := f.svc.Save(context.Background(), cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, ok := f.store.get(f.serverID); !ok {
		t.Fatal("修改超时不应清除凭证")
	}
	f.sync()
	waitForState(t, f.agg, f.serverID, StateConnected)

	if err := f.svc.ClearOAuth(context.Background(), f.serverID); err != nil {
		t.Fatalf("ClearOAuth: %v", err)
	}
	if _, ok := f.store.get(f.serverID); ok {
		t.Fatal("ClearOAuth 后凭证应被删除")
	}
	f.sync()
	waitForState(t, f.agg, f.serverID, StateNeedsAuth)

	f.authorize()
	waitForState(t, f.agg, f.serverID, StateConnected)
	cfg.OAuthClientName = "Renamed"
	if _, err := f.svc.Save(context.Background(), cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, ok := f.store.get(f.serverID); ok {
		t.Fatal("客户端身份变更后旧凭证应被清除")
	}
	f.sync()
	waitForState(t, f.agg, f.serverID, StateNeedsAuth)
}

// 改了服务地址：旧令牌绝不能被带到新地址上。即使清除凭证失败，也不能先把
// 地址改掉再说。
func TestOAuthTokenNeverFollowsURLChange(t *testing.T) {
	f := newOAuthFixture(t, nil)
	f.sync()
	waitForState(t, f.agg, f.serverID, StateNeedsAuth)
	f.authorize()
	waitForState(t, f.agg, f.serverID, StateConnected)

	var leaked []string
	var mu sync.Mutex
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "" {
			mu.Lock()
			leaked = append(leaked, auth)
			mu.Unlock()
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer other.Close()

	servers, _ := f.repo.List(context.Background())
	moved := servers[0]
	moved.URL = other.URL + "/mcp"

	// 清除失败：保存必须整体失败，配置保持原样。
	f.store.failDelete(true)
	if _, err := f.svc.Save(context.Background(), moved); err == nil {
		t.Fatal("凭证清除失败时 Save 应报错")
	}
	if after, _ := f.repo.List(context.Background()); after[0].URL != servers[0].URL {
		t.Fatalf("凭证未清除时不应更新地址: %s", after[0].URL)
	}

	f.store.failDelete(false)
	if _, err := f.svc.Save(context.Background(), moved); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, ok := f.store.get(f.serverID); ok {
		t.Fatal("地址变更后旧凭证应被清除")
	}
	f.sync()
	waitForState(t, f.agg, f.serverID, StateNeedsAuth)
	mu.Lock()
	defer mu.Unlock()
	if len(leaked) != 0 {
		t.Fatalf("旧令牌被发往了新地址: %v", leaked)
	}
}

// 不要求授权的服务被误配成 OAuth：给出明确说明而不是卡在待授权。
func TestOAuthBeginOnServerWithoutAuth(t *testing.T) {
	f := newOAuthFixture(t, nil)
	f.upstream.set(func(u *fakeOAuthUpstream) { u.noAuth = true })

	_, err := f.svc.StartOAuth(context.Background(), f.serverID)
	if err == nil || !strings.Contains(err.Error(), "未要求授权") {
		t.Fatalf("应提示该服务未要求授权，实际: %v", err)
	}
}

// 未落库的配置无法持有凭证：测试连接如实报告需要授权。
func TestOAuthTestConnectionReportsNeedsAuth(t *testing.T) {
	f := newOAuthFixture(t, nil)
	result, err := f.svc.TestConnection(context.Background(), domain.McpServer{
		Name:     "probe",
		Type:     domain.ServerTypeHTTP,
		URL:      f.upstream.srv.URL + "/mcp",
		AuthType: domain.AuthTypeOAuth,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Success || !result.NeedsAuth {
		t.Fatalf("应报告需要授权: %+v", result)
	}
}
