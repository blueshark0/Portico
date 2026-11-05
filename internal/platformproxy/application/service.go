package application

import (
	"fmt"

	platformproxydomain "portico/internal/platformproxy/domain"
	"portico/internal/shared/kernel"
)

type Manager interface {
	ProxyStatus() (platformproxydomain.Status, error)
	EnableProxy() error
	DisableProxy() error
}

// McpInjector 可选能力：向 CLI 配置外科式注入/移除 MCP 网关条目。
// 只有 Claude Code 与 Codex 的 Manager 实现它（Gemini 暂不支持）。
type McpInjector interface {
	McpStatus() (bool, error)
	InjectMcp() error
	RemoveMcp() error
}

type Service struct {
	managers map[kernel.Platform]Manager
}

func NewService(managers map[kernel.Platform]Manager) *Service {
	return &Service{managers: managers}
}

func (s *Service) GetStatus(platform string) (platformproxydomain.Status, error) {
	manager, err := s.manager(platform)
	if err != nil {
		return platformproxydomain.Status{}, err
	}
	return manager.ProxyStatus()
}

func (s *Service) Enable(platform string) error {
	manager, err := s.manager(platform)
	if err != nil {
		return err
	}
	return manager.EnableProxy()
}

func (s *Service) Disable(platform string) error {
	manager, err := s.manager(platform)
	if err != nil {
		return err
	}
	return manager.DisableProxy()
}

// McpGatewayStatus 报告目标平台是否已注入 MCP 网关条目。
func (s *Service) McpGatewayStatus(platform string) (bool, error) {
	injector, err := s.mcpInjector(platform)
	if err != nil {
		return false, err
	}
	return injector.McpStatus()
}

// InjectMcpGateway 向目标平台配置注入 MCP 网关条目（幂等）。
func (s *Service) InjectMcpGateway(platform string) error {
	injector, err := s.mcpInjector(platform)
	if err != nil {
		return err
	}
	return injector.InjectMcp()
}

// RemoveMcpGateway 从目标平台配置移除 MCP 网关条目（幂等）。
func (s *Service) RemoveMcpGateway(platform string) error {
	injector, err := s.mcpInjector(platform)
	if err != nil {
		return err
	}
	return injector.RemoveMcp()
}

func (s *Service) mcpInjector(platform string) (McpInjector, error) {
	manager, err := s.manager(platform)
	if err != nil {
		return nil, err
	}
	injector, ok := manager.(McpInjector)
	if !ok {
		return nil, fmt.Errorf("平台 %s 暂不支持 MCP 注入", platform)
	}
	return injector, nil
}

func (s *Service) manager(platform string) (Manager, error) {
	parsed, err := kernel.ParsePlatform(platform)
	if err != nil {
		return nil, err
	}
	manager := s.managers[parsed]
	if manager == nil {
		return nil, fmt.Errorf("proxy manager not configured: %s", platform)
	}
	return manager, nil
}
