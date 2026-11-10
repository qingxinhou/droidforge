//go:build windows

package runner

import (
	"os/exec"
	"syscall"
)

// applySysProcAttr 在 Windows 下隐藏子进程控制台窗口。
func applySysProcAttr(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= 0x08000000 // CREATE_NO_WINDOW
}
