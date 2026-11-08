// Portico 桌面版：只负责窗口、托盘与 Dock 的 UI 壳。
// 后端（relay、CLI 配置接管、MCP 网关、管理 API 与 Web UI）是独立进程 portico-server，
// 桌面版启动时复用已在运行的实例，否则拉起随应用分发的 sidecar；退出界面不会停止后端。
package main

import (
	"embed"
	"log"
	"os"

	appdesktop "portico/internal/app/desktop"
	"portico/internal/app/sidecar"
	"portico/internal/shared/buildinfo"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/dock"
)

//go:embed assets/icon.png assets/icon-dark.png
var trayIcons embed.FS

//go:embed build/appicon.png
var appIcon []byte

func main() {
	dockService := dock.New()
	app := application.New(application.Options{
		Name:        "Portico",
		Description: "Claude Code and Codex provider manager",
		Icon:        appIcon,
		Services:    []application.Service{application.NewService(dockService)},
		Assets: application.AssetOptions{
			Handler: appdesktop.LoadingAssets(),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})

	backend := sidecar.New(sidecar.Options{Version: buildinfo.Version})
	appdesktop.NewShell(app, dockService, trayIcons, backend).Configure()

	if err := app.Run(); err != nil {
		log.Printf("应用运行失败: %v", err)
		os.Exit(1)
	}
}
