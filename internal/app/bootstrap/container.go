package bootstrap

import (
	"context"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	configsyncapp "portico/internal/configsync/application"
	configsyncinfra "portico/internal/configsync/infrastructure"
	hotkeyinfra "portico/internal/hotkeys/infrastructure"
	relayhttp "portico/internal/interfaces/http/relay"
	facades "portico/internal/interfaces/wails"
	mcpgatewayapp "portico/internal/mcpgateway/application"
	mcpgatewayinfra "portico/internal/mcpgateway/infrastructure"
	observabilityinfra "portico/internal/observability/infrastructure"
	platformproxyapp "portico/internal/platformproxy/application"
	platformproxyinfra "portico/internal/platformproxy/infrastructure"
	routingapp "portico/internal/routing/application"
	routinginfra "portico/internal/routing/infrastructure"
	sessionapp "portico/internal/sessions/application"
	sessioninfra "portico/internal/sessions/infrastructure"
	"portico/internal/shared/kernel"
	"portico/internal/shared/logging"
	"portico/internal/shared/storage"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/dock"
)

const defaultSessionCleanupInterval = 5 * time.Minute

type Container struct {
	logFile       io.WriteCloser
	relayServer   *relayhttp.Server
	cleanupRunner *sessionapp.CleanupRunner
	hotkeyService *facades.HotkeyFacade
	routingFacade *facades.RoutingFacade
	syncFacade    *facades.ConfigSyncFacade
	sessionFacade *facades.SessionFacade
	obsFacade     *facades.ObservabilityFacade
	appLogFacade  *facades.AppLogFacade
	proxyFacade   *facades.PlatformProxyFacade
	mcpFacade     *facades.McpGatewayFacade
	mcpAggregator *mcpgatewayapp.Aggregator
	dockService   *dock.DockService
	shutdownOnce  sync.Once
}

func Initialize(appVersion string) (*Container, error) {
	if err := storage.MigrateLegacyAppDataDir(); err != nil {
		return nil, fmt.Errorf("迁移旧版应用数据失败: %w", err)
	}
	logFile := logging.Setup()
	if err := NewInitializer().Initialize(); err != nil {
		if logFile != nil {
			_ = logFile.Close()
		}
		return nil, err
	}

	hotkeyStore, err := hotkeyinfra.NewSQLiteStore()
	if err != nil {
		if logFile != nil {
			_ = logFile.Close()
		}
		return nil, err
	}

	sessionRepo := sessioninfra.NewSQLiteRepository("")
	sessionService := sessionapp.NewService(sessionRepo)
	routingService := routingapp.NewService(routinginfra.NewSQLiteStore())
	// MCP 聚合网关需在 relay 之前构造，经 Options 注入并由 relay 挂载 /mcp。
	mcpStore := mcpgatewayinfra.NewSQLiteStore()
	mcpAggregator := mcpgatewayapp.NewAggregator(mcpStore, mcpgatewayapp.AggregatorOptions{AppVersion: appVersion})
	mcpService := mcpgatewayapp.NewService(mcpStore, mcpAggregator)
	// 仅监听回环地址：绑定 0.0.0.0 会让同一局域网任意主机免费使用已配置的付费 API key。
	relayServer := relayhttp.NewServer(routingService, sessionService, "127.0.0.1:18100", relayhttp.Options{
		SessionCache:   sessioninfra.NewCache(sessionService),
		RequestLogSink: &observabilityinfra.RequestLogWriter{},
		TokenParserFor: func(platform kernel.Platform) relayhttp.TokenUsageParser {
			return observabilityinfra.GetTokenParser(platform)
		},
		RetentionCleanup: func() error {
			return observabilityinfra.CleanupOldRequestLogs(observabilityinfra.RequestLogRetentionDays)
		},
		MCPGateway: mcpAggregator,
	})
	// 偏好保存（含 configsync 快照恢复）后由 application 层统一推送代理配置到 relay。
	routingService.RegisterProxyConfigSink(relayServer)
	platformProxyService := platformproxyapp.NewService(map[kernel.Platform]platformproxyapp.Manager{
		kernel.PlatformClaude: platformproxyinfra.NewClaudeManager(relayServer.Addr()),
		kernel.PlatformCodex:  platformproxyinfra.NewCodexManager(relayServer.Addr()),
		kernel.PlatformGemini: platformproxyinfra.NewGeminiManager(relayServer.Addr()),
	})
	configSyncService := configsyncapp.NewService(
		configsyncinfra.NewSQLiteStore(),
		configsyncinfra.NewWebDAVClient(),
		routingService,
		appVersion,
	)

	// 路径失败时留空即可：reader 对不存在的文件返回空日志而非报错。
	appLogPath, _ := storage.AppDataPath("app.log")

	// 会话页需要 request_log 的会话级聚合，与观测 facade 共用同一查询实例。
	obsQueries := observabilityinfra.NewSQLiteQueries()

	return &Container{
		logFile:       logFile,
		relayServer:   relayServer,
		cleanupRunner: sessionapp.NewCleanupRunner(sessionService, defaultSessionCleanupInterval),
		hotkeyService: facades.NewHotkeyFacade(hotkeyStore),
		routingFacade: facades.NewRoutingFacade(routingService),
		syncFacade:    facades.NewConfigSyncFacade(configSyncService),
		sessionFacade: facades.NewSessionFacade(sessionService, obsQueries),
		obsFacade:     facades.NewObservabilityFacade(obsQueries),
		appLogFacade:  facades.NewAppLogFacade(observabilityinfra.NewAppLogReader(appLogPath)),
		proxyFacade:   facades.NewPlatformProxyFacade(platformProxyService),
		mcpFacade:     facades.NewMcpGatewayFacade(mcpService, platformProxyService, "http://"+relayServer.Addr()+"/mcp"),
		mcpAggregator: mcpAggregator,
		dockService:   dock.New(),
	}, nil
}

func (c *Container) ApplicationServices(extra ...application.Service) []application.Service {
	services := []application.Service{
		application.NewService(c.hotkeyService),
		application.NewService(c.routingFacade),
		application.NewService(c.syncFacade),
		application.NewService(c.sessionFacade),
		application.NewService(c.obsFacade),
		application.NewService(c.appLogFacade),
		application.NewService(c.proxyFacade),
		application.NewService(c.mcpFacade),
		application.NewService(c.dockService),
	}
	services = append(services, extra...)
	return services
}

func (c *Container) DockService() *dock.DockService {
	return c.dockService
}

func (c *Container) StartBackground() error {
	if c == nil {
		return nil
	}
	if err := c.relayServer.Start(); err != nil {
		return err
	}
	if c.cleanupRunner != nil {
		c.cleanupRunner.Start()
	}
	// relay 起监听后再启动 MCP 聚合器：Start 只起 reconciler goroutine，
	// 上游连接异步进行，不拖慢启动。
	if c.mcpAggregator != nil {
		c.mcpAggregator.Start(context.Background())
	}
	return nil
}

func (c *Container) Shutdown() {
	if c == nil {
		return
	}
	c.shutdownOnce.Do(func() {
		if c.relayServer != nil {
			_ = c.relayServer.Stop()
		}
		if c.cleanupRunner != nil {
			c.cleanupRunner.Stop()
		}
		if c.hotkeyService != nil {
			_ = c.hotkeyService.Close()
		}
		if err := storage.CloseConnections(); err != nil {
			log.Printf("关闭数据库连接失败: %v", err)
		}
		if c.logFile != nil {
			_ = c.logFile.Close()
		}
	})
}
