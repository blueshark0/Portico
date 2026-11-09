package sidecar

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"portico/internal/shared/serverinfo"
)

// appImageOnlyEnv 是 AppImage 运行时和 AppRun 设置的变量，只对桌面壳本身有意义。
var appImageOnlyEnv = []string{"APPDIR", "APPIMAGE", "ARGV0", "OWD", "PWD", "OLDPWD", "GSETTINGS_SCHEMA_DIR", "WEBKIT_INJECTED_BUNDLE_PATH"}

// appImagePathEnv 是 AppRun 追加过 AppDir 路径的列表型变量，需要还原其余条目。
var appImagePathEnv = []string{"PATH", "LD_LIBRARY_PATH", "XDG_DATA_DIRS"}

func isWithinDir(path, dir string) bool {
	return path == dir || strings.HasPrefix(path, strings.TrimRight(dir, "/")+"/")
}

// installBundledServer 把 AppImage 内的后端复制到 ~/.portico/bin。后端在桌面壳退出后
// 继续运行，而 AppImage 的挂载点会随桌面壳一起消失，不能直接从挂载点运行。
// 先写临时文件再重命名：覆盖正在运行的旧版本是安全的。
func (m *Manager) installBundledServer(source string) (string, error) {
	dir := m.opts.ServerInstallDir
	if dir == "" {
		tokenPath, err := serverinfo.TokenPath()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(filepath.Dir(tokenPath), "bin")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("创建后端目录失败: %w", err)
	}
	src, err := os.Open(source)
	if err != nil {
		return "", fmt.Errorf("读取内置后端失败: %w", err)
	}
	defer src.Close()
	tmp, err := os.CreateTemp(dir, ".portico-server-*")
	if err != nil {
		return "", fmt.Errorf("写入后端失败: %w", err)
	}
	defer os.Remove(tmp.Name())
	_, err = io.Copy(tmp, src)
	if err == nil {
		err = tmp.Chmod(0o755)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", fmt.Errorf("写入后端失败: %w", err)
	}
	target := filepath.Join(dir, filepath.Base(source))
	if err := os.Rename(tmp.Name(), target); err != nil {
		return "", fmt.Errorf("写入后端失败: %w", err)
	}
	return target, nil
}

// cleanAppImageEnv 去掉 AppImage 注入的环境。后端会继承环境并传给 MCP stdio 子进程，
// 残留的 LD_LIBRARY_PATH 会让 node 等外部程序加载 AppImage 内的库。
func cleanAppImageEnv(environ []string, appDir string) []string {
	cleaned := make([]string, 0, len(environ))
	for _, entry := range environ {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || slices.Contains(appImageOnlyEnv, key) {
			continue
		}
		if slices.Contains(appImagePathEnv, key) {
			parts := slices.DeleteFunc(strings.Split(value, ":"), func(part string) bool {
				return isWithinDir(part, appDir)
			})
			if len(parts) == 0 {
				continue
			}
			entry = key + "=" + strings.Join(parts, ":")
		}
		cleaned = append(cleaned, entry)
	}
	return cleaned
}
