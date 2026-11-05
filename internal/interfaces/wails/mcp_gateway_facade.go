package wails

import (
	"context"

	mcpapp "portico/internal/mcpgateway/application"
	mcpdomain "portico/internal/mcpgateway/domain"
	platformproxyapp "portico/internal/platformproxy/application"
)

// McpGatewayFacade 暴露 MCP 网关管理与 CLI 注入能力给前端。
type McpGatewayFacade struct {
	mcpService   *mcpapp.Service
	proxyService *platformproxyapp.Service
	endpoint     string
}

func NewMcpGatewayFacade(mcpService *mcpapp.Service, proxyService *platformproxyapp.Service, endpoint string) *McpGatewayFacade {
	return &McpGatewayFacade{mcpService: mcpService, proxyService: proxyService, endpoint: endpoint}
}

func (f *McpGatewayFacade) List() ([]mcpdomain.McpServer, error) {
	return f.mcpService.List(context.Background())
}

// Save 新建（ID==0）或更新一条 MCP 服务配置。
func (f *McpGatewayFacade) Save(server mcpdomain.McpServer) (mcpdomain.McpServer, error) {
	return f.mcpService.Save(context.Background(), server)
}

func (f *McpGatewayFacade) Delete(id int64) error {
	return f.mcpService.Delete(context.Background(), id)
}

func (f *McpGatewayFacade) SetEnabled(id int64, enabled bool) error {
	return f.mcpService.SetEnabled(context.Background(), id, enabled)
}

// TestConnection 探测一份（未落库的）配置是否可用。
func (f *McpGatewayFacade) TestConnection(server mcpdomain.McpServer) (mcpdomain.TestResult, error) {
	return f.mcpService.TestConnection(context.Background(), server)
}

// Statuses 各上游的实时连接状态（供前端轮询）。
func (f *McpGatewayFacade) Statuses() []mcpapp.UpstreamStatus {
	return f.mcpService.Statuses()
}

// ListTools 全部上游已发布工具的快照（含公共名、上游名、描述与下发的 InputSchema）。
func (f *McpGatewayFacade) ListTools() []mcpapp.ToolInfo {
	return f.mcpService.ToolCatalog()
}

// Endpoint 网关聚合端点地址，供 UI 展示/复制。
func (f *McpGatewayFacade) Endpoint() string {
	return f.endpoint
}

// GetInjectionStatus 目标平台（claude/codex）是否已注入网关条目。
func (f *McpGatewayFacade) GetInjectionStatus(platform string) (bool, error) {
	return f.proxyService.McpGatewayStatus(platform)
}

// Inject 向目标平台配置注入网关条目（幂等）。
func (f *McpGatewayFacade) Inject(platform string) error {
	return f.proxyService.InjectMcpGateway(platform)
}

// RemoveInjection 从目标平台配置移除网关条目（幂等）。
func (f *McpGatewayFacade) RemoveInjection(platform string) error {
	return f.proxyService.RemoveMcpGateway(platform)
}
