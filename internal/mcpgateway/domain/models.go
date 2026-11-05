package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

// namePattern 约束服务名落在 MCP 工具名字符合法集 [A-Za-z0-9_-] 内，
// 这样拼接公共工具名时无需二次消毒。
var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,31}$`)

const (
	DefaultTimeoutSec = 30
	MaxTimeoutSec     = 300
)

// McpServer 一个上游 MCP 服务的配置。
type McpServer struct {
	ID         int64             `json:"id"`
	Name       string            `json:"name"` // 全局唯一，同时作为工具命名空间前缀
	Type       ServerType        `json:"type"`
	Command    string            `json:"command,omitempty"` // stdio：可执行文件（exec.Command 直启，不经 shell）
	Args       []string          `json:"args,omitempty"`    // stdio
	Env        map[string]string `json:"env,omitempty"`     // stdio：追加到进程环境
	URL        string            `json:"url,omitempty"`     // http：streamable HTTP 端点
	Headers    map[string]string `json:"headers,omitempty"` // http：静态请求头
	Enabled    bool              `json:"enabled"`
	Position   int               `json:"position"`
	TimeoutSec int               `json:"timeoutSec"` // tools/call 超时；0 → Normalize 为 DefaultTimeoutSec
	CreatedAt  time.Time         `json:"createdAt"`
	UpdatedAt  time.Time         `json:"updatedAt"`
}

// Normalize 规整字段：按类型清理无关字段、补默认值。
func (s *McpServer) Normalize() {
	if s.Type == ServerTypeStdio {
		s.URL = ""
		s.Headers = nil
	} else if s.Type == ServerTypeHTTP {
		s.Command = ""
		s.Args = nil
		s.Env = nil
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
	default:
		return fmt.Errorf("未知类型：%q（支持 stdio / http）", s.Type)
	}
	return nil
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
	}{s.Name, s.Type, s.Command, s.Args, s.Env, s.URL, s.Headers, s.TimeoutSec}
	data, _ := json.Marshal(payload) // 字段均为可序列化类型，不会失败
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:16])
}

// TestResult TestConnection 的结果。
type TestResult struct {
	Success    bool   `json:"success"`
	ToolCount  int    `json:"toolCount"`
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"durationMs"`
}
