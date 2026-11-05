package wails

import (
	"log"
	"time"

	observabilitydomain "portico/internal/observability/domain"
	sessionapp "portico/internal/sessions/application"
)

// SessionUsageQuerier 返回指定平台有效会话的 request_log 聚合，见
// observability/infrastructure.SessionUsageForSessions。
type SessionUsageQuerier interface {
	SessionUsageForSessions(platform string, sessionIDs []string) (map[string]observabilitydomain.SessionUsage, error)
}

// SessionSummary 是会话绑定 + 用量聚合的视图模型：前端用它把不可读的
// 原始 session_id 换成请求数/tokens/费用/模型等可读字段。
type SessionSummary struct {
	Platform      string    `json:"platform"`
	SessionID     string    `json:"session_id"`
	ProviderName  string    `json:"provider_name"`
	LastSuccessAt time.Time `json:"last_success_at"`
	CreatedAt     time.Time `json:"created_at"`
	Requests      int       `json:"requests"`
	InputTokens   int       `json:"input_tokens"`
	OutputTokens  int       `json:"output_tokens"`
	TotalCost     float64   `json:"total_cost"`
	Models        []string  `json:"models"`
}

type SessionFacade struct {
	service *sessionapp.Service
	usage   SessionUsageQuerier
}

func NewSessionFacade(service *sessionapp.Service, usage SessionUsageQuerier) *SessionFacade {
	return &SessionFacade{service: service, usage: usage}
}

func (f *SessionFacade) ListByPlatform(platform string) ([]SessionSummary, error) {
	bindings, err := f.service.ListByPlatform(platform)
	if err != nil {
		return nil, err
	}
	// 用量查询失败不应拖垮会话列表：绑定信息才是主体，降级为无用量。
	usageBySession := map[string]observabilitydomain.SessionUsage{}
	if f.usage != nil && len(bindings) > 0 {
		sessionIDs := make([]string, 0, len(bindings))
		for _, binding := range bindings {
			sessionIDs = append(sessionIDs, binding.SessionID)
		}
		if usage, err := f.usage.SessionUsageForSessions(platform, sessionIDs); err != nil {
			log.Printf("查询会话用量聚合失败: platform=%s err=%v", platform, err)
		} else {
			usageBySession = usage
		}
	}
	summaries := make([]SessionSummary, 0, len(bindings))
	for _, binding := range bindings {
		usage := usageBySession[binding.SessionID]
		models := usage.Models
		if models == nil {
			models = []string{}
		}
		summaries = append(summaries, SessionSummary{
			Platform:      binding.Platform,
			SessionID:     binding.SessionID,
			ProviderName:  binding.ProviderName,
			LastSuccessAt: binding.LastSuccessAt,
			CreatedAt:     binding.CreatedAt,
			Requests:      usage.Requests,
			InputTokens:   usage.InputTokens,
			OutputTokens:  usage.OutputTokens,
			TotalCost:     usage.TotalCost,
			Models:        models,
		})
	}
	return summaries, nil
}

func (f *SessionFacade) Unbind(platform, sessionID string) error {
	return f.service.Unbind(platform, sessionID)
}
