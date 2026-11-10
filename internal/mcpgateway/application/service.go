package application

import (
	"context"
	"errors"
	"fmt"

	"portico/internal/mcpgateway/domain"
)

// Service MCP 服务配置的 CRUD 入口，写操作成功后触发聚合器同步。
type Service struct {
	repo  Repository
	agg   *Aggregator
	oauth *OAuthManager // nil 时不支持 OAuth 认证的上游
}

func NewService(repo Repository, agg *Aggregator, oauth *OAuthManager) *Service {
	return &Service{repo: repo, agg: agg, oauth: oauth}
}

func (s *Service) List(ctx context.Context) ([]domain.McpServer, error) {
	return s.repo.List(ctx)
}

// Save 新建（ID==0）或更新；Normalize+Validate 后落库，成功后通知聚合器。
func (s *Service) Save(ctx context.Context, server domain.McpServer) (domain.McpServer, error) {
	server.Normalize()
	if err := server.Validate(); err != nil {
		return server, err
	}
	if server.ID == 0 {
		if err := s.repo.Create(ctx, &server); err != nil {
			return server, err
		}
	} else {
		previous, found, err := s.find(ctx, server.ID)
		if err != nil {
			return server, err
		}
		// 端点或客户端身份变了，旧的注册与令牌属于另一个对象，不能带过去。
		// 必须先清除再更新：反过来的话，清除一旦失败，旧令牌就会被发往新地址。
		if s.oauth != nil && found && !previous.OAuthIdentityEquals(server) {
			if err := s.oauth.Clear(ctx, server.ID); err != nil {
				return server, fmt.Errorf("清除旧的 OAuth 凭证失败: %w", err)
			}
		}
		if err := s.repo.Update(ctx, server); err != nil {
			return server, err
		}
	}
	s.agg.NotifyChanged()
	return server, nil
}

func (s *Service) find(ctx context.Context, id int64) (domain.McpServer, bool, error) {
	servers, err := s.repo.List(ctx)
	if err != nil {
		return domain.McpServer{}, false, err
	}
	for _, srv := range servers {
		if srv.ID == id {
			return srv, true, nil
		}
	}
	return domain.McpServer{}, false, nil
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	if s.oauth != nil {
		s.oauth.Forget(id) // 凭证行已随服务一并删除，这里只清内存态
	}
	s.agg.NotifyChanged()
	return nil
}

// StartOAuth 为一个已保存的服务发起 OAuth 授权，返回需要在浏览器中打开的授权 URL。
func (s *Service) StartOAuth(ctx context.Context, id int64) (string, error) {
	if s.oauth == nil {
		return "", errors.New("未启用 OAuth 支持")
	}
	server, found, err := s.find(ctx, id)
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("MCP 服务 #%d 不存在", id)
	}
	return s.oauth.Begin(ctx, server)
}

// ClearOAuth 删除一个服务已保存的 OAuth 凭证，使其回到待授权状态。
func (s *Service) ClearOAuth(ctx context.Context, id int64) error {
	if s.oauth == nil {
		return errors.New("未启用 OAuth 支持")
	}
	if err := s.oauth.Clear(ctx, id); err != nil {
		return err
	}
	s.agg.Reconnect(id)
	return nil
}

// OAuthRedirectURL 注册给授权服务器的回调地址；未启用 OAuth 时为空。
func (s *Service) OAuthRedirectURL() string {
	if s.oauth == nil {
		return ""
	}
	return s.oauth.RedirectURL()
}

func (s *Service) SetEnabled(ctx context.Context, id int64, enabled bool) error {
	if err := s.repo.SetEnabled(ctx, id, enabled); err != nil {
		return err
	}
	s.agg.NotifyChanged()
	return nil
}

// TestConnection 探测一份（未落库的）配置是否可用。
func (s *Service) TestConnection(ctx context.Context, server domain.McpServer) (domain.TestResult, error) {
	server.Normalize()
	if err := server.Validate(); err != nil {
		return domain.TestResult{Success: false, Error: err.Error()}, nil
	}
	return s.agg.TestConnection(ctx, server)
}

func (s *Service) Statuses() []UpstreamStatus {
	return s.agg.Statuses()
}

// ToolCatalog 全部上游已发布工具的快照，供 UI 工具清单与诊断页使用。
func (s *Service) ToolCatalog() []ToolInfo {
	return s.agg.ToolCatalog()
}

func (s *Service) RuntimeSnapshot(knownRevision uint64) GatewayRuntimeSnapshot {
	return s.agg.RuntimeSnapshot(knownRevision)
}
