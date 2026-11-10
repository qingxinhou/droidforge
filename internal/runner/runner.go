// Package runner 统一封装外部进程调用（adb / fastboot / tar 等）。
//
// 设计要点：
//  1. Windows 下不弹出黑色控制台窗口；
//  2. 支持按行流式回调，便于前端实时显示刷机日志；
//  3. 总是同时收集 stdout / stderr，即使调用方只关心最终结果。
package runner

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

// Result 保存一次进程执行的完整结果。
type Result struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exitCode"`
}

// Combined 返回合并后的输出，适合直接展示给用户。
func (r *Result) Combined() string {
	parts := make([]string, 0, 2)
	if s := strings.TrimRight(r.Stdout, "\r\n"); s != "" {
		parts = append(parts, s)
	}
	if s := strings.TrimRight(r.Stderr, "\r\n"); s != "" {
		parts = append(parts, s)
	}
	return strings.Join(parts, "\n")
}

// LineKind 表示一行输出来自 stdout 还是 stderr。
type LineKind int

const (
	Stdout LineKind = iota
	Stderr
)

// LineHandler 收到一行输出（已去掉行尾换行符）。
type LineHandler func(line string, kind LineKind)

// ErrNotFound 表示可执行文件不存在。
var ErrNotFound = errors.New("executable not found")

// Run 同步执行命令并返回结果。dir 为空时继承当前工作目录。
func Run(ctx context.Context, dir, bin string, args ...string) (*Result, error) {
	return RunStream(ctx, dir, nil, bin, args...)
}

// RunStream 执行命令，并将每一行输出交给 onLine 回调（可为 nil）。
func RunStream(ctx context.Context, dir string, onLine LineHandler, bin string, args ...string) (*Result, error) {
	if strings.TrimSpace(bin) == "" {
		return nil, ErrNotFound
	}
	if _, err := exec.LookPath(bin); err != nil {
		// LookPath 对绝对路径同样有效；返回更友好的错误。
		if strings.ContainsAny(bin, `/\`) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, bin)
		}
		return nil, fmt.Errorf("%w: %s", ErrNotFound, bin)
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	applySysProcAttr(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	var (
		mu     sync.Mutex
		outBuf strings.Builder
		errBuf strings.Builder
		wg     sync.WaitGroup
	)

	consume := func(r io.Reader, kind LineKind, sink *strings.Builder) {
		defer wg.Done()
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			mu.Lock()
			sink.WriteString(line)
			sink.WriteString("\n")
			mu.Unlock()
			if onLine != nil {
				onLine(line, kind)
			}
		}
	}

	wg.Add(2)
	go consume(stdout, Stdout, &outBuf)
	go consume(stderr, Stderr, &errBuf)

	waitErr := cmd.Wait()
	wg.Wait()

	res := &Result{
		Stdout:   outBuf.String(),
		Stderr:   errBuf.String(),
		ExitCode: 0,
	}
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			res.ExitCode = exitErr.ExitCode()
			return res, nil
		}
		if ctx.Err() != nil {
			return res, ctx.Err()
		}
		return res, waitErr
	}
	return res, nil
}

// MustRun 在命令失败（退出码非 0）时构造一个包含输出内容的 error。
func MustRun(ctx context.Context, dir, bin string, args ...string) (*Result, error) {
	res, err := Run(ctx, dir, bin, args...)
	if err != nil {
		return res, err
	}
	if res.ExitCode != 0 {
		msg := res.Combined()
		if msg == "" {
			msg = fmt.Sprintf("命令执行失败，退出码 %d", res.ExitCode)
		}
		return res, fmt.Errorf("%s", msg)
	}
	return res, nil
}
