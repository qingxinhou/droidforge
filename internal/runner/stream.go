package runner

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

// streamReader 把 stdout 管道与进程绑定，Close 时会顺手结束进程。
//
// 单有管道不够：调用方中途放弃（解包失败、用户退回上一页）时没人读管道，
// adb 会一直堵在写 stdout 上不退出，设备连接也跟着占着。所以这里把
// 「关管道」和「回收进程」绑在一起。
type streamReader struct {
	r   io.ReadCloser
	cmd *exec.Cmd

	mu     sync.Mutex
	waited bool
	err    error
}

func (s *streamReader) Read(p []byte) (int, error) { return s.r.Read(p) }

// wait 回收进程，可重复调用。
func (s *streamReader) wait() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.waited {
		return s.err
	}
	s.waited = true
	s.err = s.cmd.Wait()
	return s.err
}

// Close 结束进程并回收。已经 wait 过就是空操作。
func (s *streamReader) Close() error {
	s.mu.Lock()
	done := s.waited
	s.mu.Unlock()
	if done {
		return nil
	}
	if s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	_ = s.wait()
	return nil
}

// Stream 启动一个进程并返回它的 stdout 管道。
//
// 与 RunStream 的区别：这里不做按行切分，调用方拿到的是原始字节流，
// 适合 `adb exec-out tar -cf -` 这类二进制输出。返回的 ReadCloser 必须关闭
// （defer 即可），否则进程与句柄会留在系统里；正常读完再调 wait 可以拿到退出码。
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

	sr := &streamReader{r: stdout, cmd: cmd}
	wait := func() error {
		err := sr.wait()
		if err != nil && stderr.Len() > 0 {
			return fmt.Errorf("%w：%s", err, strings.TrimSpace(stderr.String()))
		}
		return err
	}
	return sr, wait, nil
}
