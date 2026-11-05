package relay

import (
	"context"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	observabilitydomain "portico/internal/observability/domain"
	routingapp "portico/internal/routing/application"
	sessionapp "portico/internal/sessions/application"
	"portico/internal/shared/kernel"
	"portico/internal/shared/worker"
)

const (
	requestLogBufferSize      = 4096
	requestLogBatchSize       = 100
	requestLogFlushInterval   = 15 * time.Second
	requestLogCleanupInterval = 6 * time.Hour

	sessionUpdateBufferSize    = 512
	sessionUpdateBatchSize     = 50
	sessionUpdateFlushInterval = 500 * time.Millisecond
)

type sessionUpdateRequest struct {
	platform   string
	sessionID  string
	generation uint64
}

// SessionBinder 是 relay 对会话粘滞缓存的窄依赖（由 sessions/infrastructure.Cache
// 实现、组合根装配注入）：relay 不再直接 import 其他上下文的 infrastructure。
type SessionBinder interface {
	GetSessionProviderSnapshot(platform, sessionID string) (string, uint64, error)
	SessionGeneration(platform, sessionID string) uint64
	InvalidateSession(platform, sessionID string)
	BindSessionToProviderGeneration(platform, sessionID, providerName string, generation uint64) error
	RecordSessionSuccessGeneration(platform, sessionID, providerName string, generation uint64) (current, persist bool)
	UpdateSessionSuccessGeneration(platform, sessionID string, generation uint64) error
	ClearPendingSessionSuccess(platform, sessionID string)
}

// TokenUsageParser 与 observability 解析器签名一致（消费端定义接口，
// observability 的实现结构性满足）。
type TokenUsageParser interface {
	Parse(data string, usage *observabilitydomain.RequestLog)
}

// MCPGateway 聚合 MCP 端点（mcpgateway.Aggregator 结构性满足）；nil 时不挂 /mcp。
type MCPGateway interface {
	Handler() http.Handler
	Close(ctx context.Context) error
}

// Options 汇集 relay 的出上下文依赖，由 app/bootstrap 组合根装配。
type Options struct {
	// SessionCache 提供会话-供应商粘滞绑定。
	SessionCache SessionBinder
	// RequestLogSink 承接请求日志落盘（observability 写路径实现）；nil 时日志被丢弃。
	RequestLogSink worker.BatchProcessor[*observabilitydomain.RequestLog]
	// TokenParserFor 返回平台对应的 token 用量解析器；nil 时不统计用量。
	TokenParserFor func(platform kernel.Platform) TokenUsageParser
	// RetentionCleanup 执行请求日志保留期清理；nil 时不启动定时任务。
	RetentionCleanup func() error
	// MCPGateway 聚合 MCP 网关；nil 时不暴露 /mcp。
	MCPGateway MCPGateway
}

type Server struct {
	routingService      *routingapp.Service
	sessionService      *sessionapp.Service
	sessionCache        SessionBinder
	tokenParserFor      func(platform kernel.Platform) TokenUsageParser
	retentionCleanup    func() error
	requestLogSink      worker.BatchProcessor[*observabilitydomain.RequestLog]
	listener            net.Listener
	server              *http.Server
	addr                string
	requestLogWorker    *worker.BackgroundWorker[*observabilitydomain.RequestLog]
	sessionUpdateWorker *worker.BackgroundWorker[sessionUpdateRequest]
	mcpGateway          MCPGateway
	shutdownCh          chan struct{}
	backgroundWG        sync.WaitGroup
	shutdownOnce        sync.Once
	proxyMu             sync.RWMutex
	proxyEnabled        bool
	proxyURL            string
	upstreamMu          sync.Mutex
	upstreamClients     map[upstreamClientOptions]*http.Client
}

func NewServer(routingService *routingapp.Service, sessionService *sessionapp.Service, addr string, opts Options) *Server {
	if addr == "" {
		addr = "127.0.0.1:18100"
	}
	server := &Server{
		routingService:   routingService,
		sessionService:   sessionService,
		sessionCache:     opts.SessionCache,
		tokenParserFor:   opts.TokenParserFor,
		retentionCleanup: opts.RetentionCleanup,
		requestLogSink:   opts.RequestLogSink,
		mcpGateway:       opts.MCPGateway,
		addr:             addr,
		shutdownCh:       make(chan struct{}),
	}
	if server.requestLogSink != nil {
		server.requestLogWorker = worker.New[*observabilitydomain.RequestLog](worker.Config{
			Name:          "request_log",
			BufferSize:    requestLogBufferSize,
			BatchSize:     requestLogBatchSize,
			FlushInterval: requestLogFlushInterval,
			DropWhenFull:  true,
		}, server.requestLogSink)
		server.requestLogWorker.Start()
	}
	if server.sessionCache != nil {
		server.sessionUpdateWorker = worker.New[sessionUpdateRequest](worker.Config{
			Name:          "session_update",
			BufferSize:    sessionUpdateBufferSize,
			BatchSize:     sessionUpdateBatchSize,
			FlushInterval: sessionUpdateFlushInterval,
		}, newSessionUpdateProcessor(server.sessionCache))
		server.sessionUpdateWorker.Start()
	}
	server.startRequestLogRetentionTask()
	server.initProxyConfig()
	return server
}

func (s *Server) Start() error {
	if warnings := s.validateConfig(); len(warnings) > 0 {
		log.Println("======== Provider 配置验证警告 ========")
		for _, warn := range warnings {
			log.Printf("⚠️  %s\n", warn)
		}
		log.Println("========================================")
	}
	gin.SetMode(gin.ReleaseMode)
	// The relay has its own request statistics and warning logs. Avoid Gin's
	// per-request access log, which otherwise writes once for every proxied call.
	router := gin.New()
	router.Use(gin.Recovery())
	s.registerRoutes(router)
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.server = &http.Server{
		Addr:    s.addr,
		Handler: router,
	}
	s.listener = listener
	log.Printf("provider relay server listening on %s\n", s.addr)
	go func() {
		if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("provider relay server error: %v\n", err)
		}
	}()
	return nil
}

func (s *Server) Stop() error {
	var err error
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err = s.server.Shutdown(ctx)
		s.listener = nil
	}
	// 先停收新请求，再关闭 MCP 上游会话/stdio 子进程
	if s.mcpGateway != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if closeErr := s.mcpGateway.Close(ctx); closeErr != nil {
			log.Printf("mcp gateway close: %v\n", closeErr)
		}
	}
	s.stopBackgroundWorkers()
	return err
}

func (s *Server) Addr() string {
	return s.addr
}
