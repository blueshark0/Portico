// Command bindgen 仅用于生成前端 TS bindings，不随产品发布，也不应被运行。
//
// 后端 facade 由 portico-server 的 admin HTTP API 暴露，不再注册到任何
// Wails 应用；但 wails3 binding 生成器需要从 application.NewService 调用
// 中发现被绑定类型，因此在这里集中登记一次。生成命令见 build/Taskfile.yml
// 的 generate:bindings（-names：按 "<pkg>.{Facade}.{Method}" 调用，
// 前端 HTTP transport 截取末两段转发到 /api/rpc/{Facade}.{Method}）。
//
// 新增 facade 时需同时登记到这里与 bootstrap.Container.Facades()。
package main

import (
	facades "portico/internal/interfaces/wails"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func main() {
	_ = application.New(application.Options{
		Services: []application.Service{
			application.NewService(&facades.HotkeyFacade{}),
			application.NewService(&facades.RoutingFacade{}),
			application.NewService(&facades.ConfigSyncFacade{}),
			application.NewService(&facades.SessionFacade{}),
			application.NewService(&facades.ObservabilityFacade{}),
			application.NewService(&facades.AppLogFacade{}),
			application.NewService(&facades.PlatformProxyFacade{}),
			application.NewService(&facades.McpGatewayFacade{}),
		},
	})
}
