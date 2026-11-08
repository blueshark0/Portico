// Package frontend 内嵌 Web UI 构建产物，由 portico-server 的 admin 服务托管。
// 构建 Go 代码前必须先生成 dist（pnpm run build），否则 go:embed 编译失败。
package frontend

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Assets 返回以 dist 为根的文件系统（根目录下即 index.html）。
func Assets() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
