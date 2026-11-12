package application

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"portico/internal/mcpgateway/domain"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"
)

// ErrAuthorizationRequired 上游要求 OAuth 授权而网关手里没有可用令牌。
// 聚合器据此把上游置为 needs_auth：等用户授权，而不是按故障退避重连。
var ErrAuthorizationRequired = errors.New("需要完成 OAuth 授权")

const (
	// oauthFlowTimeout 用户在浏览器里完成登录的最长等待时间。
	oauthFlowTimeout = 5 * time.Minute
	// oauthPrepareTimeout 拿到授权 URL 之前（探测、发现、注册）的总时限。
	oauthPrepareTimeout = 45 * time.Second
	// oauthHTTPTimeout 单次发现/注册/换令牌请求的时限。
	oauthHTTPTimeout = 30 * time.Second
	// oauthCallbackWait 回调页等待换令牌结果的时限，超时则只告知“已收到”。
	oauthCallbackWait = 20 * time.Second
	// oauthMessageLimit 展示授权服务器回传的错误文本时的截断长度。
	oauthMessageLimit = 200
	// oauthFreshTokenWindow 刚签发的访问令牌在这段时间内仍被上游拒绝，说明问题
	// 不在令牌过期，再刷新也无济于事（还会白白消耗轮换的刷新令牌）。
	oauthFreshTokenWindow = time.Minute
)

// CredentialStore OAuth 凭证的存储接口，infrastructure.SQLiteStore 结构性满足。
type CredentialStore interface {
	// GetOAuthCredential 不存在时返回 (nil, nil)。
	GetOAuthCredential(ctx context.Context, serverID int64) (*domain.OAuthCredential, error)
	SaveOAuthCredential(ctx context.Context, cred domain.OAuthCredential) error
	DeleteOAuthCredential(ctx context.Context, serverID int64) error
}

// OAuthManagerOptions OAuth 管理器选项。
type OAuthManagerOptions struct {
	// RedirectURL 注册给授权服务器的回调地址，须指向 CallbackHandler 的挂载点。
	RedirectURL string
	AppVersion  string
	// Transport 发现、注册、换令牌及探测请求使用的底层传输；nil → http.DefaultTransport。
	Transport http.RoundTripper
}

// OAuthManager 代持上游 MCP 服务的 OAuth 凭证，并驱动交互式授权。
//
// 两条路径严格分开：后台连接（reconciler）只使用已保存的令牌，绝不发起注册
// 或等待用户；只有用户显式触发的 Begin 才走完整的授权码流程。
type OAuthManager struct {
	store       CredentialStore
	redirectURL string
	appVersion  string
	transport   http.RoundTripper
	httpClient  *http.Client

	mu      sync.Mutex
	sources map[int64]*storedTokenSource // 每个服务唯一：刷新令牌会轮换，不能各持一份
	flows   map[string]*oauthFlow        // state → 进行中的授权
	active  map[int64]*oauthFlow         // serverID → 进行中的授权
	notify  func(serverID int64, err error)
}

func NewOAuthManager(store CredentialStore, opts OAuthManagerOptions) *OAuthManager {
	transport := opts.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	appVersion := opts.AppVersion
	if appVersion == "" {
		appVersion = "dev"
	}
	return &OAuthManager{
		store:       store,
		redirectURL: opts.RedirectURL,
		appVersion:  appVersion,
		transport:   transport,
		httpClient:  &http.Client{Transport: transport, Timeout: oauthHTTPTimeout},
		sources:     make(map[int64]*storedTokenSource),
		flows:       make(map[string]*oauthFlow),
		active:      make(map[int64]*oauthFlow),
	}
}

// RedirectURL 注册给授权服务器的回调地址，供 UI 展示（预注册客户端需在第三方后台登记）。
func (m *OAuthManager) RedirectURL() string { return m.redirectURL }

// setNotifier 注册授权结果回调（由聚合器在构造时设置）。err 为 nil 表示授权成功。
func (m *OAuthManager) setNotifier(fn func(serverID int64, err error)) {
	m.mu.Lock()
	m.notify = fn
	m.mu.Unlock()
}

// ---- 后台连接：只用已保存的令牌 ----

// handlerFor 返回后台连接使用的 OAuth handler；没有已保存的凭证时返回
// ErrAuthorizationRequired，且不发任何网络请求。
func (m *OAuthManager) handlerFor(ctx context.Context, cfg domain.McpServer) (auth.OAuthHandler, error) {
	src, err := m.tokenSource(ctx, cfg.ID)
	if err != nil {
		return nil, err
	}
	if src == nil {
		return nil, ErrAuthorizationRequired
	}
	return backgroundOAuthHandler{src: src}, nil
}

func (m *OAuthManager) tokenSource(ctx context.Context, serverID int64) (*storedTokenSource, error) {
	if serverID == 0 {
		return nil, nil // 尚未落库的配置不可能有凭证
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if src, ok := m.sources[serverID]; ok {
		return src, nil
	}
	cred, err := m.store.GetOAuthCredential(ctx, serverID)
	if err != nil {
		return nil, fmt.Errorf("读取 OAuth 凭证失败: %w", err)
	}
	if cred == nil {
		return nil, nil
	}
	src := m.newStoredTokenSource(*cred, time.Time{})
	m.sources[serverID] = src
	return src, nil
}

// issuedAt 为该令牌的签发时刻；从数据库加载的旧令牌传零值。
func (m *OAuthManager) newStoredTokenSource(cred domain.OAuthCredential, issuedAt time.Time) *storedTokenSource {
	return &storedTokenSource{
		cred:     cred,
		store:    m.store,
		issuedAt: issuedAt,
		// 刷新发生在连接的整个生命周期里，不能绑定某次请求的 ctx。
		refreshCtx: context.WithValue(context.Background(), oauth2.HTTPClient, m.httpClient),
	}
}

// storedTokenSource 以数据库中的凭证为准的 token source：过期时刷新，
// 刷新结果（含轮换后的刷新令牌）立即落库。
type storedTokenSource struct {
	store      CredentialStore
	refreshCtx context.Context

	mu       sync.Mutex
	cred     domain.OAuthCredential
	issuedAt time.Time // 当前访问令牌由本进程换得/刷新的时刻；零值表示来自数据库
}

func (s *storedTokenSource) Token() (*oauth2.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if tok := s.tokenLocked(); tok.Valid() {
		return tok, nil
	}
	return s.refreshLocked()
}

func (s *storedTokenSource) tokenLocked() *oauth2.Token {
	return &oauth2.Token{
		AccessToken:  s.cred.AccessToken,
		RefreshToken: s.cred.RefreshToken,
		TokenType:    s.cred.TokenType,
		Expiry:       s.cred.Expiry,
	}
}

// refreshAfterRejection 上游以 401 拒绝了 rejected 这枚访问令牌时强制刷新一次
// （令牌可能被吊销，或授权服务器没给有效期）。别的请求已经换过令牌时直接复用。
func (s *storedTokenSource) refreshAfterRejection(rejected string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rejected != "" && rejected != s.cred.AccessToken {
		return nil
	}
	if !s.issuedAt.IsZero() && time.Since(s.issuedAt) < oauthFreshTokenWindow {
		return fmt.Errorf("%w：刚签发的访问令牌仍被上游拒绝", ErrAuthorizationRequired)
	}
	_, err := s.refreshLocked()
	return err
}

func (s *storedTokenSource) refreshLocked() (*oauth2.Token, error) {
	if s.cred.RefreshToken == "" {
		return nil, fmt.Errorf("%w：访问令牌已失效且没有刷新令牌", ErrAuthorizationRequired)
	}
	cfg := &oauth2.Config{
		ClientID:     s.cred.ClientID,
		ClientSecret: s.cred.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:   s.cred.AuthURL,
			TokenURL:  s.cred.TokenURL,
			AuthStyle: oauth2.AuthStyle(s.cred.AuthStyle),
		},
		RedirectURL: s.cred.RedirectURL,
		Scopes:      s.cred.Scopes,
	}
	// 只带刷新令牌，迫使 oauth2 走刷新而不是复用旧的访问令牌。
	tok, err := cfg.TokenSource(s.refreshCtx, &oauth2.Token{RefreshToken: s.cred.RefreshToken}).Token()
	if err != nil {
		var retrieveErr *oauth2.RetrieveError
		if errors.As(err, &retrieveErr) && isAuthorizationRejected(retrieveErr) {
			// 刷新令牌或客户端注册已失效，只能重新授权。
			return nil, fmt.Errorf("%w：刷新令牌被拒绝（%s）", ErrAuthorizationRequired, retrieveErrorSummary(retrieveErr))
		}
		return nil, fmt.Errorf("刷新 OAuth 令牌失败: %w", err)
	}
	s.issuedAt = time.Now()
	s.cred.AccessToken = tok.AccessToken
	s.cred.TokenType = tok.TokenType
	s.cred.Expiry = tok.Expiry
	if tok.RefreshToken != "" {
		s.cred.RefreshToken = tok.RefreshToken
	}
	if err := s.store.SaveOAuthCredential(context.Background(), s.cred); err != nil {
		// 内存里的令牌仍可用；但若刷新令牌已轮换，重启后需要重新授权。
		log.Printf("mcpgateway: 保存上游 #%d 刷新后的 OAuth 令牌失败: %v", s.cred.ServerID, err)
	}
	return s.tokenLocked(), nil
}

func isAuthorizationRejected(err *oauth2.RetrieveError) bool {
	switch err.ErrorCode {
	case "invalid_grant", "invalid_client", "unauthorized_client":
		return true
	}
	return err.Response != nil &&
		(err.Response.StatusCode == http.StatusBadRequest || err.Response.StatusCode == http.StatusUnauthorized)
}

func retrieveErrorSummary(err *oauth2.RetrieveError) string {
	if err.ErrorCode != "" {
		return err.ErrorCode
	}
	if err.Response != nil {
		return err.Response.Status
	}
	return "unknown"
}

// backgroundOAuthHandler 后台连接使用的 auth.OAuthHandler：从不弹浏览器。
type backgroundOAuthHandler struct {
	src *storedTokenSource
}

func (h backgroundOAuthHandler) TokenSource(context.Context) (oauth2.TokenSource, error) {
	return h.src, nil
}

// Authorize 在上游返回 401/403 时被传输层调用；返回 nil 表示可以重试原请求。
func (h backgroundOAuthHandler) Authorize(_ context.Context, req *http.Request, resp *http.Response) error {
	defer resp.Body.Close()
	defer io.Copy(io.Discard, resp.Body)

	if resp.StatusCode == http.StatusForbidden {
		challenges, _ := oauthex.ParseWWWAuthenticate(resp.Header[http.CanonicalHeaderKey("WWW-Authenticate")])
		if !hasInsufficientScope(challenges) {
			// 与令牌无关的 403：交回传输层按普通响应处理。
			return nil
		}
		return fmt.Errorf("%w：已授权的权限范围不足", ErrAuthorizationRequired)
	}
	rejected := strings.TrimSpace(strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer "))
	return h.src.refreshAfterRejection(rejected)
}

func hasInsufficientScope(challenges []oauthex.Challenge) bool {
	for _, c := range challenges {
		if c.Scheme == "bearer" && c.Params["error"] == "insufficient_scope" {
			return true
		}
	}
	return false
}

// authorizationRequiredDetail 取出 ErrAuthorizationRequired 之后附带的原因；
// 单纯“还没授权过”时返回空串，UI 不需要为此展示一条错误。
func authorizationRequiredDetail(err error) string {
	msg := err.Error()
	idx := strings.Index(msg, ErrAuthorizationRequired.Error())
	if idx < 0 {
		return ""
	}
	detail := msg[idx+len(ErrAuthorizationRequired.Error()):]
	return strings.TrimLeft(detail, "：: ")
}

// ---- 交互授权 ----

type callbackResult struct {
	code    string
	state   string
	iss     string
	errCode string
	errDesc string
}

type oauthFlow struct {
	serverID int64
	cancel   context.CancelFunc
	result   chan callbackResult // 容量 1：回调至多投递一次
	done     chan struct{}       // Authorize 返回后关闭
	err      error               // done 关闭后有效

	published bool // 授权 URL 已交给调用方（受 OAuthManager.mu 保护）
}

// Begin 为一个已落库的服务发起交互授权，返回需要在浏览器中打开的授权 URL。
// 之后的换令牌在后台完成，结果经 notifier 通知聚合器。
func (m *OAuthManager) Begin(ctx context.Context, cfg domain.McpServer) (string, error) {
	if cfg.ID == 0 {
		return "", errors.New("请先保存服务配置再授权")
	}
	if cfg.Type != domain.ServerTypeHTTP || cfg.AuthType != domain.AuthTypeOAuth {
		return "", errors.New("该服务未启用 OAuth 认证")
	}
	if m.redirectURL == "" {
		return "", errors.New("未配置 OAuth 回调地址")
	}

	flowCtx, cancel := context.WithTimeout(context.Background(), oauthFlowTimeout)
	flow := &oauthFlow{
		serverID: cfg.ID,
		cancel:   cancel,
		result:   make(chan callbackResult, 1),
		done:     make(chan struct{}),
	}
	m.mu.Lock()
	if prev := m.active[cfg.ID]; prev != nil {
		prev.cancel() // 同一服务只保留最新一次授权
	}
	m.active[cfg.ID] = flow
	m.mu.Unlock()

	req, resp, err := m.probe(ctx, cfg)
	if err != nil {
		m.finishFlow(flow, err)
		return "", err
	}

	urlCh := make(chan string, 1)
	handler, err := auth.NewAuthorizationCodeHandler(m.interactiveConfig(cfg, flow, urlCh))
	if err != nil {
		resp.Body.Close()
		m.finishFlow(flow, err)
		return "", fmt.Errorf("初始化 OAuth 流程失败: %w", err)
	}

	go func() {
		// Authorize 负责关闭 resp.Body。
		m.finishFlow(flow, handler.Authorize(flowCtx, req, resp))
	}()

	prepare := time.NewTimer(oauthPrepareTimeout)
	defer prepare.Stop()
	select {
	case authURL := <-urlCh:
		return authURL, nil
	case <-flow.done:
		return "", describeAuthorizeError(flow.err)
	case <-prepare.C:
		cancel()
		return "", errors.New("获取授权地址超时")
	case <-ctx.Done():
		cancel()
		return "", ctx.Err()
	}
}

// probe 发一次不带凭证的 initialize，取得上游的 401 质询（其中指向资源元数据）。
func (m *OAuthManager) probe(ctx context.Context, cfg domain.McpServer) (*http.Request, *http.Response, error) {
	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"portico","version":%q}}}`, m.appVersion)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.URL, strings.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("构造探测请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	client := &http.Client{
		Transport: newStaticHeaderRoundTripper(m.transport, cfg.URL, cfg.Headers),
		Timeout:   oauthHTTPTimeout,
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("连接上游失败: %w", err)
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return req, resp, nil
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		resp.Body.Close()
		return nil, nil, errors.New("该服务未要求授权（不带凭证的请求已成功），请把认证方式改为“无”")
	default:
		resp.Body.Close()
		return nil, nil, fmt.Errorf("上游未返回授权质询（HTTP %d），无法发起 OAuth 授权", resp.StatusCode)
	}
}

func (m *OAuthManager) interactiveConfig(cfg domain.McpServer, flow *oauthFlow, urlCh chan<- string) *auth.AuthorizationCodeHandlerConfig {
	hc := &auth.AuthorizationCodeHandlerConfig{
		RedirectURL:         m.redirectURL,
		RequestRefreshToken: true,
		// 回传的 iss 与预期一致即可接受，不要求授权服务器在元数据里声明支持。
		AcceptUnadvertisedIss: true,
		Client:                m.httpClient,
		AuthorizationCodeFetcher: func(ctx context.Context, args *auth.AuthorizationArgs) (*auth.AuthorizationResult, error) {
			return m.awaitAuthorizationCode(ctx, flow, args.URL, urlCh)
		},
		NewTokenSource: func(_ context.Context, oc *oauth2.Config, tok *oauth2.Token) (oauth2.TokenSource, error) {
			return m.adoptToken(cfg.ID, oc, tok)
		},
	}
	if cfg.OAuthClientID != "" {
		creds := &oauthex.ClientCredentials{ClientID: cfg.OAuthClientID}
		if cfg.OAuthClientSecret != "" {
			creds.ClientSecretAuth = &oauthex.ClientSecretAuth{ClientSecret: cfg.OAuthClientSecret}
		}
		hc.PreregisteredClient = creds
		return hc
	}
	hc.DynamicClientRegistrationConfig = &auth.DynamicClientRegistrationConfig{
		Metadata: &oauthex.ClientRegistrationMetadata{
			ClientName:              cfg.EffectiveOAuthClientName(),
			RedirectURIs:            []string{m.redirectURL},
			GrantTypes:              []string{"authorization_code", "refresh_token"},
			ResponseTypes:           []string{"code"},
			TokenEndpointAuthMethod: "none", // 本机应用保守不住密钥，按公共客户端注册
		},
	}
	return hc
}

// awaitAuthorizationCode 把授权 URL 交给调用方，然后等浏览器回调。
func (m *OAuthManager) awaitAuthorizationCode(ctx context.Context, flow *oauthFlow, authURL string, urlCh chan<- string) (*auth.AuthorizationResult, error) {
	parsed, err := url.Parse(authURL)
	if err != nil {
		return nil, fmt.Errorf("授权地址不合法: %w", err)
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		// 该 URL 会交给系统浏览器打开，不能是 file:、javascript: 之类的协议。
		return nil, fmt.Errorf("授权地址协议不受支持: %q", parsed.Scheme)
	}
	state := parsed.Query().Get("state")
	if state == "" {
		return nil, errors.New("授权地址缺少 state 参数")
	}

	m.mu.Lock()
	m.flows[state] = flow
	flow.published = true
	m.mu.Unlock()
	urlCh <- authURL

	select {
	case res := <-flow.result:
		if res.errCode != "" {
			return nil, fmt.Errorf("授权未通过：%s", truncateMessage(strings.TrimSpace(res.errCode+" "+res.errDesc)))
		}
		if res.code == "" {
			return nil, errors.New("回调缺少授权码")
		}
		return &auth.AuthorizationResult{Code: res.code, State: res.state, Iss: res.iss}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// adoptToken 授权码换到令牌后落库，并成为该服务唯一的 token source。
func (m *OAuthManager) adoptToken(serverID int64, oc *oauth2.Config, tok *oauth2.Token) (oauth2.TokenSource, error) {
	cred := domain.OAuthCredential{
		ServerID:     serverID,
		ClientID:     oc.ClientID,
		ClientSecret: oc.ClientSecret,
		AuthStyle:    int(oc.Endpoint.AuthStyle),
		AuthURL:      oc.Endpoint.AuthURL,
		TokenURL:     oc.Endpoint.TokenURL,
		RedirectURL:  oc.RedirectURL,
		Scopes:       oc.Scopes,
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		TokenType:    tok.TokenType,
		Expiry:       tok.Expiry,
	}
	if err := m.store.SaveOAuthCredential(context.Background(), cred); err != nil {
		return nil, fmt.Errorf("保存 OAuth 凭证失败: %w", err)
	}
	src := m.newStoredTokenSource(cred, time.Now())
	m.mu.Lock()
	m.sources[serverID] = src
	m.mu.Unlock()
	return src, nil
}

// finishFlow 收尾一次授权：注销 state、释放资源，并在用户已看到授权 URL 的
// 情况下把结果通知聚合器。
func (m *OAuthManager) finishFlow(flow *oauthFlow, err error) {
	m.mu.Lock()
	for state, f := range m.flows {
		if f == flow {
			delete(m.flows, state)
		}
	}
	superseded := m.active[flow.serverID] != flow
	if !superseded {
		delete(m.active, flow.serverID)
	}
	published := flow.published
	notify := m.notify
	m.mu.Unlock()

	flow.err = err
	close(flow.done)
	flow.cancel()

	if notify == nil || !published {
		return // URL 尚未交出时，错误已由 Begin 直接返回给调用方
	}
	if err != nil && superseded {
		return // 被新的授权或清除操作取代，不是用户需要关心的失败
	}
	notify(flow.serverID, describeAuthorizeError(err))
}

// describeAuthorizeError 把 SDK 的英文错误补上用户能据此行动的说明。
func describeAuthorizeError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return errors.New("授权超时，请重新发起")
	case errors.Is(err, context.Canceled):
		return errors.New("授权已取消")
	case strings.Contains(msg, "failed to register client"):
		return fmt.Errorf("授权服务器拒绝了客户端注册：该服务可能只允许白名单内的客户端，或需要在高级选项里填写预注册的 Client ID（%s）", truncateMessage(msg))
	case strings.Contains(msg, "no configured client registration methods"):
		return errors.New("授权服务器不支持动态客户端注册，请在高级选项里填写预注册的 Client ID")
	}
	return fmt.Errorf("OAuth 授权失败: %s", truncateMessage(msg))
}

func truncateMessage(msg string) string {
	runes := []rune(msg)
	if len(runes) <= oauthMessageLimit {
		return msg
	}
	return string(runes[:oauthMessageLimit]) + "…"
}

// Clear 删除某个服务的 OAuth 凭证并取消进行中的授权。
func (m *OAuthManager) Clear(ctx context.Context, serverID int64) error {
	m.Forget(serverID)
	return m.store.DeleteOAuthCredential(ctx, serverID)
}

// Forget 丢弃某个服务的内存态（缓存的 token source、进行中的授权），不动数据库。
func (m *OAuthManager) Forget(serverID int64) {
	m.mu.Lock()
	delete(m.sources, serverID)
	flow := m.active[serverID]
	delete(m.active, serverID)
	m.mu.Unlock()
	if flow != nil {
		flow.cancel()
	}
}

// ---- 浏览器回调 ----

// CallbackHandler 授权服务器重定向回来的入口。
//
// 该路由无法携带 relay 的入站凭证（它由浏览器跳转触发），因此防护完全依赖
// 一次性的随机 state：未知或已用过的 state 一律拒绝，并且页面内容固定，
// 不回显任何查询参数。
func (m *OAuthManager) CallbackHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		query := r.URL.Query()
		flow := m.takeFlow(query.Get("state"))
		if flow == nil {
			writeCallbackPage(w, http.StatusBadRequest, callbackPage{
				Title:  "授权请求不存在或已过期",
				Detail: "Authorization request not found or expired. 请回到 Portico 重新发起授权。",
			})
			return
		}
		flow.result <- callbackResult{
			code:    query.Get("code"),
			state:   query.Get("state"),
			iss:     query.Get("iss"),
			errCode: query.Get("error"),
			errDesc: query.Get("error_description"),
		}

		wait := time.NewTimer(oauthCallbackWait)
		defer wait.Stop()
		select {
		case <-flow.done:
			if flow.err != nil {
				writeCallbackPage(w, http.StatusBadRequest, callbackPage{
					Title:  "授权失败",
					Detail: "Authorization failed. " + describeAuthorizeError(flow.err).Error(),
				})
				return
			}
			writeCallbackPage(w, http.StatusOK, callbackPage{
				OK:     true,
				Title:  "授权成功",
				Detail: "Authorization complete. 可以关闭此页面并回到 Portico。",
			})
		case <-wait.C:
			writeCallbackPage(w, http.StatusOK, callbackPage{
				OK:     true,
				Title:  "已收到授权结果",
				Detail: "Authorization received. 请回到 Portico 查看连接状态。",
			})
		case <-r.Context().Done():
		}
	})
}

// takeFlow 取出并注销 state 对应的授权：每个 state 只能被回调使用一次。
func (m *OAuthManager) takeFlow(state string) *oauthFlow {
	if state == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	flow := m.flows[state]
	delete(m.flows, state)
	return flow
}

type callbackPage struct {
	OK     bool
	Title  string
	Detail string
}

var callbackPageTemplate = template.Must(template.New("oauth-callback").Parse(`<!doctype html>
<html lang="zh">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Portico</title>
<style>
body{margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;font-family:system-ui,sans-serif;background:#f6f7f9;color:#1f2937}
main{max-width:28rem;padding:2rem;text-align:center}
h1{font-size:1.25rem;margin:0 0 .75rem}
p{margin:0;line-height:1.6;color:#4b5563;word-break:break-word}
.ok h1{color:#047857}.fail h1{color:#b91c1c}
@media (prefers-color-scheme:dark){body{background:#111827;color:#f3f4f6}p{color:#d1d5db}.ok h1{color:#34d399}.fail h1{color:#f87171}}
</style>
</head>
<body>
<main class="{{if .OK}}ok{{else}}fail{{end}}">
<h1>{{.Title}}</h1>
<p>{{.Detail}}</p>
</main>
</body>
</html>
`))

func writeCallbackPage(w http.ResponseWriter, status int, page callbackPage) {
	header := w.Header()
	header.Set("Content-Type", "text/html; charset=utf-8")
	header.Set("Cache-Control", "no-store")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	w.WriteHeader(status)
	_ = callbackPageTemplate.Execute(w, page)
}
