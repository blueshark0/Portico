package desktop

import (
	"io/fs"
	"log"
	"runtime"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/dock"
)

type Shell struct {
	app         *application.App
	dockService *dock.DockService
	trayIcons   fs.ReadFileFS

	mainWindow         *application.WebviewWindow
	mainWindowCentered bool
}

func NewShell(app *application.App, dockService *dock.DockService, trayIcons fs.ReadFileFS) *Shell {
	return &Shell{
		app:         app,
		dockService: dockService,
		trayIcons:   trayIcons,
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
	s.configureTray()
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
	trayMenu.Add("退出").OnClick(func(ctx *application.Context) {
		s.app.Quit()
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
