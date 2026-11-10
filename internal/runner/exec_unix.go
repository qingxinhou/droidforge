//go:build !windows

package runner

import "os/exec"

// applySysProcAttr 在类 Unix 系统上无需处理窗口。
func applySysProcAttr(cmd *exec.Cmd) {}
