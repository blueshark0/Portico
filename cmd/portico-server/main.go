// Command portico-server 是 Portico 的后端服务：本地 relay（127.0.0.1:18100）、
// CLI 配置接管、MCP 网关，以及管理 API 与 Web UI（默认 127.0.0.1:18101）。
// 桌面版 Portico 会按需以 sidecar 方式拉起它；在无桌面的服务器上可直接运行。
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"portico/frontend"
	appbootstrap "portico/internal/app/bootstrap"
	"portico/internal/interfaces/http/admin"
	"portico/internal/interfaces/tui"
	"portico/internal/shared/buildinfo"
	"portico/internal/shared/serverinfo"
)

const usage = `用法: portico-server [命令] [参数]

命令:
  serve          启动服务（默认）
  tui            终端管理界面（连接本机或远程的 portico-server）
  token show     显示访问令牌与带令牌的访问地址
  token reset    重新生成访问令牌（运行中的服务立即生效，旧令牌失效）
  version        显示版本

serve 参数（也可用对应环境变量设置，命令行参数优先）:
  --listen ADDR             管理 API / Web UI 监听地址（PORTICO_ADMIN_LISTEN，默认 ` + serverinfo.DefaultAdminAddr + `）
  --tls-cert FILE           HTTPS 证书（PORTICO_ADMIN_TLS_CERT）
  --tls-key FILE            HTTPS 私钥（PORTICO_ADMIN_TLS_KEY）
  --always-require-token    本机请求也要求令牌，部署在反向代理之后时必须开启（PORTICO_ADMIN_ALWAYS_TOKEN=1）

tui 参数:
  --url URL                 管理地址（PORTICO_ADMIN_URL，默认读取本机运行时信息，否则 http://` + serverinfo.DefaultAdminAddr + `）
  --token TOKEN             访问令牌（PORTICO_ADMIN_TOKEN；本机地址自动读取令牌文件。
                            建议用环境变量传入，命令行参数会出现在进程列表中）
`

func main() {
	args := os.Args[1:]
	command := "serve"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		command, args = args[0], args[1:]
	}

	var err error
	switch command {
	case "serve":
		err = runServe(args)
	case "tui":
		err = runTUI(args)
	case "token":
		err = runToken(args)
	case "version":
		fmt.Println(buildinfo.Version)
	case "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "未知命令 %q\n\n%s", command, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "portico-server: %v\n", err)
		os.Exit(1)
	}
}

type serveConfig struct {
	listen             string
	tlsCert            string
	tlsKey             string
	alwaysRequireToken bool
}

func parseServeFlags(args []string) (serveConfig, error) {
	cfg := serveConfig{
		listen:             envOr("PORTICO_ADMIN_LISTEN", serverinfo.DefaultAdminAddr),
		tlsCert:            os.Getenv("PORTICO_ADMIN_TLS_CERT"),
		tlsKey:             os.Getenv("PORTICO_ADMIN_TLS_KEY"),
		alwaysRequireToken: os.Getenv("PORTICO_ADMIN_ALWAYS_TOKEN") == "1",
	}
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	fs.StringVar(&cfg.listen, "listen", cfg.listen, "")
	fs.StringVar(&cfg.tlsCert, "tls-cert", cfg.tlsCert, "")
	fs.StringVar(&cfg.tlsKey, "tls-key", cfg.tlsKey, "")
	fs.BoolVar(&cfg.alwaysRequireToken, "always-require-token", cfg.alwaysRequireToken, "")
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if (cfg.tlsCert == "") != (cfg.tlsKey == "") {
		return cfg, errors.New("--tls-cert 与 --tls-key 必须同时提供")
	}
	return cfg, nil
}

func runServe(args []string) error {
	cfg, err := parseServeFlags(args)
	if err != nil {
		return err
	}

	container, err := appbootstrap.Initialize(buildinfo.Version)
	if err != nil {
		return fmt.Errorf("初始化失败: %w", err)
	}
	defer container.Shutdown()

	tokenPath, err := serverinfo.TokenPath()
	if err != nil {
		return err
	}
	token, err := serverinfo.LoadOrCreateToken(tokenPath)
	if err != nil {
		return err
	}
	registry, err := admin.NewRegistry(container.Facades()...)
	if err != nil {
		return fmt.Errorf("注册管理接口失败: %w", err)
	}

	stop := make(chan struct{})
	var stopOnce sync.Once
	requestStop := func() { stopOnce.Do(func() { close(stop) }) }

	adminServer := admin.NewServer(admin.Options{
		Addr:               cfg.listen,
		TLSCertFile:        cfg.tlsCert,
		TLSKeyFile:         cfg.tlsKey,
		Tokens:             admin.NewFileToken(tokenPath, token),
		AlwaysRequireToken: cfg.alwaysRequireToken,
		Registry:           registry,
		Assets:             frontend.Assets(),
		Version:            buildinfo.Version,
		OnShutdown:         requestStop,
	})

	// 先启动 relay：18100 被占用说明已有实例在运行，此时不应再占用管理端口。
	if err := container.StartBackground(); err != nil {
		return fmt.Errorf("启动本地代理失败（%s 是否已被其他 Portico 实例占用？）: %w", container.RelayAddr(), err)
	}
	if err := adminServer.Start(); err != nil {
		return fmt.Errorf("启动管理服务失败: %w", err)
	}
	defer func() {
		if err := adminServer.Stop(); err != nil {
			log.Printf("关闭管理服务失败: %v", err)
		}
	}()

	if !serverinfo.IsLoopbackListen(adminServer.Addr()) {
		if !adminServer.TLSEnabled() {
			log.Printf("⚠️  管理端口 %s 对外开放且未启用 TLS，令牌将以明文传输；建议配置 --tls-cert/--tls-key 或置于 HTTPS 反向代理之后", adminServer.Addr())
		}
		log.Println("远程访问需要令牌，运行 `portico-server token show` 查看")
	}

	runtimePath, err := serverinfo.RuntimeInfoPath()
	if err != nil {
		return err
	}
	pid := os.Getpid()
	adminURL := serverinfo.AdminURL(adminServer.Addr(), adminServer.TLSEnabled())
	if err := serverinfo.WriteRuntimeInfo(runtimePath, serverinfo.RuntimeInfo{
		PID:       pid,
		AdminAddr: adminServer.Addr(),
		AdminURL:  adminURL,
		Version:   buildinfo.Version,
	}); err != nil {
		log.Printf("写入运行时信息失败: %v", err)
	}
	defer serverinfo.RemoveRuntimeInfo(runtimePath, pid)
	log.Printf("Portico %s 已启动：relay %s，管理界面 %s", buildinfo.Version, container.RelayAddr(), adminURL)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	select {
	case sig := <-signals:
		log.Printf("收到信号 %s，正在退出", sig)
	case <-stop:
		log.Println("收到停止请求，正在退出")
	}
	return nil
}

func runTUI(args []string) error {
	var flagURL, flagToken string
	fs := flag.NewFlagSet("tui", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	fs.StringVar(&flagURL, "url", "", "")
	fs.StringVar(&flagToken, "token", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return tui.Run(tui.ResolveTarget(flagURL, flagToken))
}

func runToken(args []string) error {
	if len(args) != 1 || (args[0] != "show" && args[0] != "reset") {
		return errors.New("用法: portico-server token show|reset")
	}
	tokenPath, err := serverinfo.TokenPath()
	if err != nil {
		return err
	}
	var token string
	if args[0] == "reset" {
		token, err = serverinfo.ResetToken(tokenPath)
	} else {
		token, err = serverinfo.LoadOrCreateToken(tokenPath)
	}
	if err != nil {
		return err
	}

	adminURL := serverinfo.AdminURL(serverinfo.DefaultAdminAddr, false)
	if runtimePath, err := serverinfo.RuntimeInfoPath(); err == nil {
		if info, err := serverinfo.ReadRuntimeInfo(runtimePath); err == nil && info.AdminURL != "" {
			adminURL = info.AdminURL
		}
	}
	fmt.Printf("令牌: %s\n", token)
	fmt.Printf("访问地址: %s/?token=%s\n", adminURL, token)
	fmt.Println("（远程访问时把地址中的主机替换为服务器的主机名或 IP）")
	return nil
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
