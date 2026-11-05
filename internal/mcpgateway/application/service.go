package application

import (
	"context"

	"portico/internal/mcpgateway/domain"
)

// Service MCP 服务配置的 CRUD 入口，写操作成功后触发聚合器同步。
type Service struct {
	repo Repository
	agg  *Aggregator
}

func NewService(repo Repository, agg *Aggregator) *Service {
	return &Service{repo: repo, agg: agg}
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
		if err := s.repo.Update(ctx, server); err != nil {
			return server, err
		}
	}
	s.agg.NotifyChanged()
	return server, nil
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.agg.NotifyChanged()
	return nil
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
