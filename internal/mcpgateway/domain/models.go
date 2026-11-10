package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// ServerType 上游 MCP 服务的传输类型。
type ServerType string

const (
	ServerTypeStdio ServerType = "stdio" // 本地子进程
	ServerTypeHTTP  ServerType = "http"  // streamable HTTP 远程服务
)

// AuthType 上游 HTTP 服务的认证方式。
type AuthType string

const (
	AuthTypeNone  AuthType = ""      // 无认证或仅静态请求头
	AuthTypeOAuth AuthType = "oauth" // MCP 授权规范：OAuth 2.1 授权码 + PKCE
)

// DefaultOAuthClientName 动态客户端注册时默认上报的客户端名称。
const DefaultOAuthClientName = "Portico"

// MaxOAuthClientNameLen 自定义客户端名称的长度上限。
const MaxOAuthClientNameLen = 64

// namePattern 约束服务名落在 MCP 工具名字符合法集 [A-Za-z0-9_-] 内，
// 这样拼接公共工具名时无需二次消毒。
var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,31}$`)

const (
	DefaultTimeoutSec = 30
	MaxTimeoutSec     = 300
)

// McpServer 一个上游 MCP 服务的配置。
type McpServer struct {
	ID      int64             `json:"id"`
	Name    string            `json:"name"` // 全局唯一，同时作为工具命名空间前缀
	Type    ServerType        `json:"type"`
	Command string            `json:"command,omitempty"` // stdio：可执行文件（exec.Command 直启，不经 shell）
	Args    []string          `json:"args,omitempty"`    // stdio
	Env     map[string]string `json:"env,omitempty"`     // stdio：追加到进程环境
	URL     string            `json:"url,omitempty"`     // http：streamable HTTP 端点
	Headers map[string]string `json:"headers,omitempty"` // http：静态请求头
	// http：认证方式。oauth 时令牌由网关代持，以下三项均可留空：
	// 客户端名称用于动态注册（空 → DefaultOAuthClientName）；填了 ClientID
	// 则跳过动态注册，直接使用预注册的客户端。
	AuthType          AuthType  `json:"authType,omitempty"`
	OAuthClientName   string    `json:"oauthClientName,omitempty"`
	OAuthClientID     string    `json:"oauthClientId,omitempty"`
	OAuthClientSecret string    `json:"oauthClientSecret,omitempty"`
	Enabled           bool      `json:"enabled"`
	Position          int       `json:"position"`
	TimeoutSec        int       `json:"timeoutSec"` // tools/call 超时；0 → Normalize 为 DefaultTimeoutSec
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

// Normalize 规整字段：按类型清理无关字段、补默认值。
func (s *McpServer) Normalize() {
	if s.Type == ServerTypeStdio {
		s.URL = ""
		s.Headers = nil
		s.AuthType = AuthTypeNone
	} else if s.Type == ServerTypeHTTP {
		s.Command = ""
		s.Args = nil
		s.Env = nil
	}
	s.OAuthClientName = strings.TrimSpace(s.OAuthClientName)
	s.OAuthClientID = strings.TrimSpace(s.OAuthClientID)
	s.OAuthClientSecret = strings.TrimSpace(s.OAuthClientSecret)
	if s.AuthType != AuthTypeOAuth {
		s.OAuthClientName = ""
		s.OAuthClientID = ""
		s.OAuthClientSecret = ""
	}
	if s.TimeoutSec <= 0 {
		s.TimeoutSec = DefaultTimeoutSec
	}
	if s.TimeoutSec > MaxTimeoutSec {
		s.TimeoutSec = MaxTimeoutSec
	}
}

// Validate 校验配置合法性。
func (s *McpServer) Validate() error {
	if !namePattern.MatchString(s.Name) {
		return fmt.Errorf("名称需匹配 ^[A-Za-z0-9][A-Za-z0-9_-]{0,31}$：%q", s.Name)
	}
	// 公共工具名是 <name>__<tool>：name 中禁止出现连续下划线 "__"，
	// 否则 (a, b__c) 与 (a__b, c) 撞名，命名空间失去单射性。
	if strings.Contains(s.Name, "__") {
		return fmt.Errorf("名称不能包含连续下划线 \"__\"（与工具命名空间分隔符冲突）：%q", s.Name)
	}
	switch s.Type {
	case ServerTypeStdio:
		if s.Command == "" {
			return fmt.Errorf("stdio 类型必须填写 command")
		}
		for k := range s.Env {
			if k == "" {
				return fmt.Errorf("环境变量名不能为空")
			}
		}
	case ServerTypeHTTP:
		if s.URL == "" {
			return fmt.Errorf("http 类型必须填写 url")
		}
		u, err := url.Parse(s.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("url 必须是合法的 http/https 地址：%q", s.URL)
		}
		for k := range s.Headers {
			if k == "" {
				return fmt.Errorf("请求头名称不能为空")
			}
		}
		if err := s.validateAuth(u); err != nil {
			return err
		}
	default:
		return fmt.Errorf("未知类型：%q（支持 stdio / http）", s.Type)
	}
	return nil
}

func (s *McpServer) validateAuth(endpoint *url.URL) error {
	switch s.AuthType {
	case AuthTypeNone:
		return nil
	case AuthTypeOAuth:
	default:
		return fmt.Errorf("未知认证方式：%q（支持留空 / oauth）", s.AuthType)
	}
	// 令牌只能走加密通道；回环地址例外，便于连接本机的开发服务。
	if endpoint.Scheme != "https" && !isLoopbackHost(endpoint.Hostname()) {
		return fmt.Errorf("OAuth 认证要求 https 地址（回环地址除外）：%q", s.URL)
	}
	if len(s.OAuthClientName) > MaxOAuthClientNameLen {
		return fmt.Errorf("OAuth 客户端名称不能超过 %d 个字符", MaxOAuthClientNameLen)
	}
	if s.OAuthClientSecret != "" && s.OAuthClientID == "" {
		return fmt.Errorf("填写 Client Secret 时必须同时填写 Client ID")
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// EffectiveOAuthClientName 动态注册时实际上报的客户端名称。
func (s *McpServer) EffectiveOAuthClientName() string {
	if s.OAuthClientName != "" {
		return s.OAuthClientName
	}
	return DefaultOAuthClientName
}

// OAuthIdentityEquals 两份配置是否共用同一套 OAuth 凭证：端点或客户端身份
// 任一变化，已保存的客户端注册与令牌都不再适用。
func (s *McpServer) OAuthIdentityEquals(other McpServer) bool {
	return s.AuthType == other.AuthType &&
		s.URL == other.URL &&
		s.OAuthClientName == other.OAuthClientName &&
		s.OAuthClientID == other.OAuthClientID &&
		s.OAuthClientSecret == other.OAuthClientSecret
}

// Fingerprint 配置指纹：影响上游连接与工具集的字段变化时指纹变化，
// 聚合器据此判断是否需要重建上游会话。
func (s *McpServer) Fingerprint() string {
	payload := struct {
		Name       string            `json:"n"`
		Type       ServerType        `json:"t"`
		Command    string            `json:"c"`
		Args       []string          `json:"a"`
		Env        map[string]string `json:"e"`
		URL        string            `json:"u"`
		Headers    map[string]string `json:"h"`
		TimeoutSec int               `json:"to"`
		// omitempty：未启用 OAuth 的既有配置指纹保持不变，升级后不会无故重连。
		AuthType          AuthType `json:"at,omitempty"`
		OAuthClientName   string   `json:"ocn,omitempty"`
		OAuthClientID     string   `json:"oci,omitempty"`
		OAuthClientSecret string   `json:"ocs,omitempty"`
	}{s.Name, s.Type, s.Command, s.Args, s.Env, s.URL, s.Headers, s.TimeoutSec,
		s.AuthType, s.OAuthClientName, s.OAuthClientID, s.OAuthClientSecret}
	data, _ := json.Marshal(payload) // 字段均为可序列化类型，不会失败
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:16])
}

// TestResult TestConnection 的结果。NeedsAuth 表示服务可达但尚未完成 OAuth 授权。
type TestResult struct {
	Success    bool   `json:"success"`
	ToolCount  int    `json:"toolCount"`
	Error      string `json:"error,omitempty"`
	NeedsAuth  bool   `json:"needsAuth,omitempty"`
	DurationMs int64  `json:"durationMs"`
}

// OAuthCredential 一个上游服务的 OAuth 客户端注册与令牌，由网关代持。
// 端点与客户端身份随令牌一起保存，重启后无需重新发现或注册即可刷新令牌。
type OAuthCredential struct {
	ServerID     int64
	ClientID     string
	ClientSecret string
	AuthStyle    int // oauth2.AuthStyle：令牌端点的客户端认证方式
	AuthURL      string
	TokenURL     string
	RedirectURL  string
	Scopes       []string
	AccessToken  string
	RefreshToken string
	TokenType    string
	Expiry       time.Time // 零值表示授权服务器未给出有效期
}
