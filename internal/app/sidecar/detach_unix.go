//go:build !windows

package sidecar

import (
	"os/exec"
	"syscall"
)

// configureDetached 让后端成为新会话的首进程，脱离桌面壳的进程组与控制终端，
// 桌面壳退出（或终端关闭）时不会收到 SIGHUP/SIGINT。
func configureDetached(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
