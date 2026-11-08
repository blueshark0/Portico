// Package serverinfo 定义 portico-server 与 UI 壳之间的本地约定：默认管理地址、
// 访问令牌文件与运行时信息文件。它只依赖标准库与 storage 路径，桌面壳可放心引用。
package serverinfo

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"portico/internal/shared/storage"
)

const (
	// DefaultAdminAddr 是管理 API 与 Web UI 的默认监听地址（与 relay 的 18100 相邻）。
	DefaultAdminAddr = "127.0.0.1:18101"

	tokenFileName       = "admin_token"
	runtimeInfoFileName = "server.json"
	tokenBytes          = 32
)

// RuntimeInfo 由运行中的 portico-server 写入 ~/.portico/server.json，退出时删除，
// 供桌面壳发现已在运行的后端。文件不含令牌。
type RuntimeInfo struct {
	PID       int    `json:"pid"`
	AdminAddr string `json:"adminAddr"`
	AdminURL  string `json:"adminUrl"`
	Version   string `json:"version"`
}

// Health 是 GET /api/health 的响应体（免认证，因此只含非敏感信息）。
type Health struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	PID     int    `json:"pid"`
}

func TokenPath() (string, error) {
	return storage.AppDataPath(tokenFileName)
}

func RuntimeInfoPath() (string, error) {
	return storage.AppDataPath(runtimeInfoFileName)
}

func AppLogPath() (string, error) {
	return storage.AppDataPath("app.log")
}

// LoadOrCreateToken 读取访问令牌；文件不存在或为空时生成新令牌并以 0600 写入。
func LoadOrCreateToken(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		if token := strings.TrimSpace(string(data)); token != "" {
			return token, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read admin token: %w", err)
	}
	return ResetToken(path)
}

// ResetToken 生成新令牌并原子替换令牌文件；旧令牌随即失效。
func ResetToken(path string) (string, error) {
	buf := make([]byte, tokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate admin token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(buf)
	if err := writeFileAtomic(path, []byte(token+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("write admin token: %w", err)
	}
	return token, nil
}

func WriteRuntimeInfo(path string, info RuntimeInfo) error {
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(data, '\n'), 0o644)
}

func ReadRuntimeInfo(path string) (RuntimeInfo, error) {
	var info RuntimeInfo
	data, err := os.ReadFile(path)
	if err != nil {
		return info, err
	}
	if err := json.Unmarshal(data, &info); err != nil {
		return info, fmt.Errorf("parse %s: %w", path, err)
	}
	return info, nil
}

// RemoveRuntimeInfo 只在文件仍属于当前进程时删除，避免误删新实例写入的信息。
func RemoveRuntimeInfo(path string, pid int) {
	info, err := ReadRuntimeInfo(path)
	if err != nil || info.PID != pid {
		return
	}
	_ = os.Remove(path)
}

// AdminURL 由监听地址推导浏览器访问地址：通配地址（0.0.0.0 / ::）替换为回环地址，
// 方便本机直接打开；远程访问时用户自行替换为服务器的主机名。
func AdminURL(listenAddr string, tls bool) string {
	scheme := "http"
	if tls {
		scheme = "https"
	}
	host, port, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return scheme + "://" + listenAddr
	}
	if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
		host = "127.0.0.1"
	}
	return scheme + "://" + net.JoinHostPort(host, port)
}

// IsLoopbackListen 判断监听地址是否只绑定回环接口。
func IsLoopbackListen(listenAddr string) bool {
	host, _, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
