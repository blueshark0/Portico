package desktop

import (
	"context"
	"io/fs"
	"log"
	"runtime"
	"sync"
	"time"

	"portico/internal/app/sidecar"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/dock"
)

const connectTimeout = 30 * time.Second

// Shell 是桌面 UI 壳：窗口、托盘与 Dock。业务界面与后端逻辑都在 portico-server 中，
// 窗口先显示本地启动页，连接（必要时拉起）后端后再加载其管理页面。
type Shell struct {
	app         *application.App
	dockService *dock.DockService
	trayIcons   fs.ReadFileFS
	backend     *sidecar.Manager

	mainWindow         *application.WebviewWindow
	mainWindowCentered bool

	mu         sync.Mutex
	adminURL   string
	connecting bool
}

func NewShell(app *application.App, dockService *dock.DockService, trayIcons fs.ReadFileFS, backend *sidecar.Manager) *Shell {
	return &Shell{
		app:         app,
		dockService: dockService,
		trayIcons:   trayIcons,
		backend:     backend,
	}
}

func (s *Shell) Configure() {
	s.mainWindow = s.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "Portico",
		Width:     1150,
		Height:    800,
		MinWidth:  600,
		MinHeight: 300,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(27, 38, 54),
		URL:              "/",
	})

	s.showMainWindow(false)
	s.mainWindow.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		s.mainWindow.Hide()
		s.handleDockVisibility(false)
		e.Cancel()
	})
	s.app.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(event *application.ApplicationEvent) {
		s.showMainWindow(true)
	})
	s.app.Event.OnApplicationEvent(events.Mac.ApplicationDidBecomeActive, func(event *application.ApplicationEvent) {
		if s.mainWindow.IsVisible() {
			s.mainWindow.Focus()
			return
		}
		s.showMainWindow(true)
	})
	s.app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(event *application.ApplicationEvent) {
		go s.connect()
	})
	s.configureTray()
}

// connect 确保后端可用并让窗口加载其管理页面；失败时在窗口内显示原因。
func (s *Shell) connect() {
	s.mu.Lock()
	if s.connecting {
		s.mu.Unlock()
		return
	}
	s.connecting = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.connecting = false
		s.mu.Unlock()
	}()

	s.mainWindow.SetURL("/")
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	conn, err := s.backend.Ensure(ctx)
	if err != nil {
		log.Printf("连接 Portico 服务失败: %v", err)
		s.mainWindow.SetURL(errorPageURL(err.Error()))
		return
	}
	if conn.Spawned {
		log.Printf("已启动 Portico 服务: %s", conn.AdminURL)
	}
	s.mu.Lock()
	s.adminURL = conn.AdminURL
	s.mu.Unlock()
	s.mainWindow.SetURL(s.backend.WindowURL(ctx, conn))
}

// quitAndStopBackend 请求后端退出后再退出桌面壳；后端不可达时直接退出。
func (s *Shell) quitAndStopBackend() {
	s.mu.Lock()
	adminURL := s.adminURL
	s.mu.Unlock()
	if adminURL != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := s.backend.Shutdown(ctx, adminURL); err != nil {
			log.Printf("停止 Portico 服务失败: %v", err)
		}
		cancel()
	}
	s.app.Quit()
}

func (s *Shell) focusMainWindow() {
	if runtime.GOOS == "windows" {
		s.mainWindow.SetAlwaysOnTop(true)
		s.mainWindow.Focus()
		go func() {
			time.Sleep(150 * time.Millisecond)
			s.mainWindow.SetAlwaysOnTop(false)
		}()
		return
	}
	s.mainWindow.Focus()
}

func (s *Shell) showMainWindow(withFocus bool) {
	if !s.mainWindowCentered {
		s.mainWindow.Center()
		s.mainWindowCentered = true
	}
	if s.mainWindow.IsMinimised() {
		s.mainWindow.UnMinimise()
	}
	s.mainWindow.Show()
	if withFocus {
		s.focusMainWindow()
	}
	s.handleDockVisibility(true)
}

func (s *Shell) configureTray() {
	systray := s.app.SystemTray.New()
	systray.SetTooltip("Portico")
	if lightIcon := s.loadTrayIcon("assets/icon.png"); len(lightIcon) > 0 {
		systray.SetIcon(lightIcon)
	}
	if darkIcon := s.loadTrayIcon("assets/icon-dark.png"); len(darkIcon) > 0 {
		systray.SetDarkModeIcon(darkIcon)
	}

	trayMenu := application.NewMenu()
	trayMenu.Add("显示主窗口").OnClick(func(ctx *application.Context) {
		s.showMainWindow(true)
	})
	trayMenu.Add("重新连接服务").OnClick(func(ctx *application.Context) {
		go s.connect()
	})
	trayMenu.AddSeparator()
	// 后端是独立进程：退出界面不影响 CLI 通过 relay 继续工作。
	trayMenu.Add("退出界面（服务继续运行）").OnClick(func(ctx *application.Context) {
		s.app.Quit()
	})
	trayMenu.Add("退出并停止服务").OnClick(func(ctx *application.Context) {
		go s.quitAndStopBackend()
	})
	systray.SetMenu(trayMenu)

	handleTrayActivate := func() {
		if !s.mainWindow.IsVisible() {
			s.showMainWindow(true)
			return
		}
		if !s.mainWindow.IsFocused() {
			s.focusMainWindow()
		}
	}
	systray.OnClick(handleTrayActivate)
	if runtime.GOOS == "linux" {
		systray.OnDoubleClick(handleTrayActivate)
	}
}

func (s *Shell) loadTrayIcon(path string) []byte {
	if s.trayIcons == nil {
		return nil
	}
	data, err := s.trayIcons.ReadFile(path)
	if err != nil {
		log.Printf("failed to load tray icon %s: %v", path, err)
		return nil
	}
	return data
}

func (s *Shell) handleDockVisibility(show bool) {
	if runtime.GOOS != "darwin" || s.dockService == nil {
		return
	}
	if show {
		s.dockService.ShowAppIcon()
		return
	}
	s.dockService.HideAppIcon()
}
