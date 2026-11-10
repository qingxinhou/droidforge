package runner

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// Stream 启动一个进程并返回它的 stdout 管道。
//
// 与 RunStream 的区别：这里不做按行切分，调用方拿到的是原始字节流，
// 适合 `adb exec-out tar -cf -` 这类二进制输出。读完后必须调用 wait 回收
// 进程，否则会留下僵尸句柄——这一点在 Windows 上尤其明显。
func Stream(ctx context.Context, dir, bin string, args ...string) (io.ReadCloser, func() error, error) {
	if strings.TrimSpace(bin) == "" {
		return nil, nil, ErrNotFound
	}
	if _, err := exec.LookPath(bin); err != nil {
		return nil, nil, fmt.Errorf("%w: %s", ErrNotFound, bin)
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	applySysProcAttr(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	wait := func() error {
		err := cmd.Wait()
		if err != nil && stderr.Len() > 0 {
			return fmt.Errorf("%w：%s", err, strings.TrimSpace(stderr.String()))
		}
		return err
	}
	return stdout, wait, nil
}
