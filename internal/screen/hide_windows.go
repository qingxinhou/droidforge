//go:build windows

package screen

import (
	"os/exec"
	"syscall"
)

// applyHidden 隐藏 scrcpy 的控制台窗口。
// scrcpy 的 Windows 官方构建是控制台程序，不隐藏的话每次投屏都会弹一个黑框；
// 它自己的 SDL 画面窗口不受这个标志影响。
func applyHidden(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}
