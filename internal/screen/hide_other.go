//go:build !windows

package screen

import "os/exec"

// applyHidden 在非 Windows 平台不需要处理窗口。
func applyHidden(cmd *exec.Cmd) {}
