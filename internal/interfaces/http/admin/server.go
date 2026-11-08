// Package admin 是 portico-server 的管理接口：对外暴露 facade 的 RPC、
// 健康检查与内嵌的 Web UI。它与 relay 使用独立的 listener：relay 必须只监听回环
// （否则局域网主机可免费使用已配置的付费 API key），而管理端口可按需对外开放并由令牌保护。
package admin

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"portico/internal/shared/serverinfo"
)

const (
	maxRPCBodyBytes = 8 << 20
	shutdownTimeout = 5 * time.Second
)

type Options struct {
	// Addr 是监听地址，如 127.0.0.1:18101 或 0.0.0.0:18101。
	Addr string
	// TLSCertFile / TLSKeyFile 同时提供时启用 HTTPS。
	TLSCertFile string
	TLSKeyFile  string
	// Tokens 提供访问令牌；AlwaysRequireToken 为 true 时本机请求也需要令牌（反向代理部署）。
	Tokens             TokenSource
	AlwaysRequireToken bool
	Registry           *Registry
	// Assets 是 Web UI 的静态文件根目录（包含 index.html）；nil 时不托管 UI。
	Assets  fs.FS
	Version string
	// OnShutdown 在收到 POST /api/shutdown 后异步调用，由宿主进程负责优雅退出。
	OnShutdown func()
}

type Server struct {
	opts     Options
	server   *http.Server
	listener net.Listener
}

func NewServer(opts Options) *Server {
	return &Server{opts: opts}
}

func (s *Server) TLSEnabled() bool {
	return s.opts.TLSCertFile != "" && s.opts.TLSKeyFile != ""
}

// Addr 返回实际监听地址（Addr 使用 :0 时可取得系统分配的端口）。
func (s *Server) Addr() string {
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return s.opts.Addr
}

func (s *Server) Handler() http.Handler {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())

	router.GET("/api/health", s.handleHealth)
	api := router.Group("/api", authMiddleware(s.opts.Tokens, s.opts.AlwaysRequireToken))
	api.POST("/rpc/:method", s.handleRPC)
	api.POST("/shutdown", s.handleShutdown)
	// 认证探针：前端登录页用它校验令牌，不触发任何业务调用。
	api.GET("/session", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	if s.opts.Assets != nil {
		router.NoRoute(s.handleStatic())
	}
	return router
}

func (s *Server) Start() error {
	listener, err := net.Listen("tcp", s.opts.Addr)
	if err != nil {
		return err
	}
	s.listener = listener
	s.server = &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		IdleTimeout:       120 * time.Second,
		// 不设 WriteTimeout：连接测试、WebDAV 同步等 RPC 可能耗时较长，由各自的上游超时兜底。
	}
	scheme := "http"
	if s.TLSEnabled() {
		scheme = "https"
	}
	log.Printf("admin server listening on %s://%s\n", scheme, listener.Addr())
	go func() {
		var serveErr error
		if s.TLSEnabled() {
			serveErr = s.server.ServeTLS(listener, s.opts.TLSCertFile, s.opts.TLSKeyFile)
		} else {
			serveErr = s.server.Serve(listener)
		}
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			log.Printf("admin server error: %v\n", serveErr)
		}
	}()
	return nil
}

func (s *Server) Stop() error {
	if s.server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return s.server.Shutdown(ctx)
}

func (s *Server) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, serverinfo.Health{Status: "ok", Version: s.opts.Version, PID: os.Getpid()})
}

func (s *Server) handleRPC(c *gin.Context) {
	method := c.Param("method")
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, maxRPCBodyBytes))
	if err != nil {
		abortJSON(c, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	result, err := s.opts.Registry.Call(method, body)
	if err != nil {
		status := http.StatusInternalServerError
		var rpcErr *RPCError
		if errors.As(err, &rpcErr) {
			status = rpcErr.Status
		}
		if status >= http.StatusInternalServerError {
			log.Printf("admin rpc %s failed: %v\n", method, err)
		}
		abortJSON(c, status, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) handleShutdown(c *gin.Context) {
	if s.opts.OnShutdown == nil {
		abortJSON(c, http.StatusNotImplemented, "shutdown is not supported")
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"ok": true})
	log.Println("admin: shutdown requested")
	go s.opts.OnShutdown()
}

// handleStatic 托管 Web UI。前端使用 hash 路由，未知路径回落到 index.html 仅作兜底。
func (s *Server) handleStatic() gin.HandlerFunc {
	fileServer := http.FileServer(http.FS(s.opts.Assets))
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			abortJSON(c, http.StatusNotFound, "not found")
			return
		}
		path := strings.TrimPrefix(c.Request.URL.Path, "/")
		if strings.HasPrefix(path, "api/") {
			abortJSON(c, http.StatusNotFound, "not found")
			return
		}
		if path == "" || path == "index.html" || !assetExists(s.opts.Assets, path) {
			// index.html 不缓存，保证升级后立即加载新的带哈希资源。
			c.Header("Cache-Control", "no-cache")
			c.Request.URL.Path = "/"
		}
		fileServer.ServeHTTP(c.Writer, c.Request)
	}
}

func assetExists(assets fs.FS, path string) bool {
	info, err := fs.Stat(assets, path)
	return err == nil && !info.IsDir()
}
