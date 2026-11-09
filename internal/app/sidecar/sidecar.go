// Package sidecar 负责桌面壳与 portico-server 之间的连接：发现已运行的后端、
// 必要时以分离进程拉起随应用分发的 portico-server、以及请求其停止。
// 该包不依赖 Wails，便于在无图形环境下测试。
package sidecar

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"portico/internal/shared/serverinfo"
)

const (
	defaultStartTimeout = 15 * time.Second
	pollInterval        = 200 * time.Millisecond
	requestTimeout      = 2 * time.Second
)

// Options 主要供测试替换外部依赖；零值即生产配置。
type Options struct {
	// Version 是桌面壳版本；与后端不一致时会重启后端，保证升级后 UI 与后端同步更新。
	Version string
	// ServerBinary 为空时取可执行文件同目录下的 portico-server。
	ServerBinary string
	// AppImageDir 是 AppImage 的挂载目录，为空时取 APPDIR 环境变量。
	AppImageDir string
	// ServerInstallDir 是 AppImage 内后端的复制目标，为空时使用 ~/.portico/bin。
	ServerInstallDir string
	// RuntimeInfoPath / TokenPath 为空时使用 ~/.portico 下的默认位置。
	RuntimeInfoPath string
	TokenPath       string
	// DefaultURL 是没有运行时信息时探测的地址。
	DefaultURL   string
	StartTimeout time.Duration
	// Spawn 启动后端进程并返回一个在进程退出时关闭的 channel。
	Spawn func(binary string) (<-chan error, error)
}

type Manager struct {
	opts   Options
	client *http.Client
}

func New(opts Options) *Manager {
	if opts.DefaultURL == "" {
		opts.DefaultURL = serverinfo.AdminURL(serverinfo.DefaultAdminAddr, false)
	}
	if opts.StartTimeout <= 0 {
		opts.StartTimeout = defaultStartTimeout
	}
	if opts.Spawn == nil {
		opts.Spawn = spawnDetached
	}
	return &Manager{opts: opts, client: &http.Client{Timeout: requestTimeout}}
}

// Connection 描述一个可用的后端。
type Connection struct {
	AdminURL string
	// Spawned 表示后端由本次调用拉起。
	Spawned bool
}

// WindowURL 返回桌面窗口应加载的地址。后端开启 always-require-token 时本机访问也需要令牌，
// 此时通过 ?token= 交给前端（前端读取后立即从地址栏移除）。
func (m *Manager) WindowURL(ctx context.Context, conn Connection) string {
	base := strings.TrimRight(conn.AdminURL, "/") + "/"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"api/session", nil)
	if err != nil {
		return base
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return base
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		return base
	}
	token := m.readToken()
	if token == "" {
		return base
	}
	return base + "?token=" + url.QueryEscape(token)
}

// Ensure 返回一个可用的后端：优先复用已在运行且版本一致的实例；
// 版本不一致时请求旧实例停止并重新拉起；未运行时拉起 sidecar。
func (m *Manager) Ensure(ctx context.Context) (Connection, error) {
	adminURL := m.discoverURL()
	health, err := m.health(ctx, adminURL)
	if err == nil {
		if health.Version == m.opts.Version || m.opts.Version == "" {
			return Connection{AdminURL: adminURL}, nil
		}
		log.Printf("sidecar: 后端版本 %s 与桌面版 %s 不一致，正在重启后端", health.Version, m.opts.Version)
		if err := m.Shutdown(ctx, adminURL); err != nil {
			return Connection{}, fmt.Errorf("停止旧版本后端失败: %w", err)
		}
		if err := m.waitUntilDown(ctx, adminURL); err != nil {
			return Connection{}, err
		}
	}

	binary, err := m.serverBinary()
	if err != nil {
		return Connection{}, err
	}
	exited, err := m.opts.Spawn(binary)
	if err != nil {
		return Connection{}, fmt.Errorf("启动后端失败: %w", err)
	}
	// 新实例总是监听默认地址（除非环境变量覆盖），因此重新发现地址。
	adminURL = m.discoverURL()
	if err := m.waitUntilUp(ctx, &adminURL, exited); err != nil {
		return Connection{}, err
	}
	return Connection{AdminURL: adminURL, Spawned: true}, nil
}

// Shutdown 请求后端优雅退出（POST /api/shutdown）。
func (m *Manager) Shutdown(ctx context.Context, adminURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(adminURL, "/")+"/api/shutdown", bytes.NewReader(nil))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token := m.readToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("shutdown: HTTP %d", resp.StatusCode)
	}
	return nil
}

func (m *Manager) discoverURL() string {
	path := m.opts.RuntimeInfoPath
	if path == "" {
		path, _ = serverinfo.RuntimeInfoPath()
	}
	if path != "" {
		if info, err := serverinfo.ReadRuntimeInfo(path); err == nil && info.AdminURL != "" {
			return info.AdminURL
		}
	}
	return m.opts.DefaultURL
}

func (m *Manager) readToken() string {
	path := m.opts.TokenPath
	if path == "" {
		path, _ = serverinfo.TokenPath()
	}
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func (m *Manager) health(ctx context.Context, adminURL string) (serverinfo.Health, error) {
	var health serverinfo.Health
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(adminURL, "/")+"/api/health", nil)
	if err != nil {
		return health, err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return health, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return health, fmt.Errorf("health: HTTP %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return health, fmt.Errorf("health: %w", err)
	}
	if health.Status != "ok" {
		return health, fmt.Errorf("health: status %q", health.Status)
	}
	return health, nil
}

func (m *Manager) waitUntilUp(ctx context.Context, adminURL *string, exited <-chan error) error {
	ctx, cancel := context.WithTimeout(ctx, m.opts.StartTimeout)
	defer cancel()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		if _, err := m.health(ctx, *adminURL); err == nil {
			return nil
		}
		select {
		case err := <-exited:
			if err == nil {
				err = errors.New("进程已退出")
			}
			return fmt.Errorf("后端启动后立即退出（%v），详见 %s", err, appLogHint())
		case <-ctx.Done():
			return fmt.Errorf("等待后端启动超时（%s），详见 %s", m.opts.StartTimeout, appLogHint())
		case <-ticker.C:
			*adminURL = m.discoverURL()
		}
	}
}

func (m *Manager) waitUntilDown(ctx context.Context, adminURL string) error {
	ctx, cancel := context.WithTimeout(ctx, m.opts.StartTimeout)
	defer cancel()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		if _, err := m.health(ctx, adminURL); err != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("等待旧版本后端退出超时（%s）", m.opts.StartTimeout)
		case <-ticker.C:
		}
	}
}

func (m *Manager) serverBinary() (string, error) {
	binary, err := m.locateServerBinary()
	if err != nil {
		return "", err
	}
	appDir := m.opts.AppImageDir
	if appDir == "" {
		appDir = os.Getenv("APPDIR")
	}
	if appDir != "" && isWithinDir(binary, appDir) {
		return m.installBundledServer(binary)
	}
	return binary, nil
}

func (m *Manager) locateServerBinary() (string, error) {
	if m.opts.ServerBinary != "" {
		return m.opts.ServerBinary, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	name := "portico-server"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(filepath.Dir(exe), name)
	if _, err := os.Stat(binary); err != nil {
		return "", fmt.Errorf("未找到后端程序 %s（桌面版需与 portico-server 一同安装）: %w", binary, err)
	}
	return binary, nil
}

// spawnDetached 以独立会话启动后端：桌面壳退出后后端继续运行。
// 输出丢弃即可，后端自身写入 ~/.portico/app.log。
func spawnDetached(binary string) (<-chan error, error) {
	cmd := exec.Command(binary, "serve")
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	if appDir := os.Getenv("APPDIR"); appDir != "" {
		// AppRun 把工作目录切到了挂载点内，后端不能留在那里。
		cmd.Dir = filepath.Dir(binary)
		cmd.Env = append(cleanAppImageEnv(os.Environ(), appDir), "PWD="+cmd.Dir)
	}
	configureDetached(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	exited := make(chan error, 1)
	// Wait 回收子进程，避免其提前退出时成为僵尸进程；桌面壳先退出时子进程由系统接管。
	go func() { exited <- cmd.Wait() }()
	return exited, nil
}

func appLogHint() string {
	if path, err := serverinfo.AppLogPath(); err == nil {
		return path
	}
	return "~/.portico/app.log"
}
