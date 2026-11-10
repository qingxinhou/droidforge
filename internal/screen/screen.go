// Package screen 负责启动与管理 scrcpy 投屏进程。
//
// 不做画面内嵌：scrcpy 自己开一个原生窗口，把它的画面抓到 Wails 的
// WebView 里需要一整套视频解码链路，收益远小于维护成本。这里负责的是
// 参数面板、二进制定位与进程生命周期。
package screen

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"droidforge/internal/runner"
)

// Options 是投屏参数。
type Options struct {
	Serial        string `json:"serial"`
	AdbPath       string `json:"adbPath"`
	BitRate       string `json:"bitRate"` // 如 8M / 16M
	MaxSize       int    `json:"maxSize"` // 长边上限，0 表示原生分辨率
	MaxFPS        int    `json:"maxFps"`
	TurnScreenOff bool   `json:"turnScreenOff"`
	StayAwake     bool   `json:"stayAwake"`
	NoAudio       bool   `json:"noAudio"`
	AlwaysOnTop   bool   `json:"alwaysOnTop"`
	Record        string `json:"record"`
}

// Binary 是 scrcpy 的探测结果。
type Binary struct {
	Path    string `json:"path"`
	Found   bool   `json:"found"`
	Version string `json:"version"`
	Hint    string `json:"hint"`
}

// Detect 查找 scrcpy。
//
// 顺序：用户指定路径 → 程序自带 tools 目录 → PATH。
func Detect(custom string) Binary {
	if custom != "" {
		if _, err := os.Stat(custom); err == nil {
			return probe(custom)
		}
	}
	exe := "scrcpy"
	if runtime.GOOS == "windows" {
		exe = "scrcpy.exe"
	}
	for _, dir := range toolDirs() {
		p := filepath.Join(dir, exe)
		if _, err := os.Stat(p); err == nil {
			return probe(p)
		}
	}
	if p, err := exec.LookPath(exe); err == nil {
		return probe(p)
	}
	return Binary{
		Hint: "没找到 scrcpy。到 github.com/Genymobile/scrcpy/releases 下载对应平台的压缩包，" +
			"解压后在设置里指定 scrcpy 可执行文件路径，或把解压目录加进 PATH",
	}
}

func toolDirs() []string {
	var dirs []string
	if cfg, err := os.UserConfigDir(); err == nil {
		dirs = append(dirs, filepath.Join(cfg, "DroidForge", "tools", "scrcpy"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, "DroidForge", "tools", "scrcpy"))
	}
	return dirs
}

func probe(p string) Binary {
	b := Binary{Path: p, Found: true}
	ctx, cancel := context.WithTimeout(context.Background(), 5e9)
	defer cancel()
	if res, err := runner.Run(ctx, "", p, "--version"); err == nil {
		line := strings.TrimSpace(res.Stdout)
		if i := strings.IndexByte(line, '\n'); i >= 0 {
			line = line[:i]
		}
		b.Version = line
	} else {
		b.Hint = "找到了文件但执行失败，可能缺少同级 dll 或被安全软件拦截"
	}
	return b
}

// Process 是一个正在运行的投屏进程。
type Process struct {
	cmd  *exec.Cmd
	mu   sync.Mutex
	done bool
	out  *ringLines
}

// ringLines 保存最近若干行输出，用于失败时给出原因。
type ringLines struct {
	mu    sync.Mutex
	lines []string
}

func (r *ringLines) add(s string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = append(r.lines, s)
	if len(r.lines) > 50 {
		r.lines = r.lines[len(r.lines)-50:]
	}
}

// Tail 返回最近几行输出。
func (p *Process) Tail(n int) string {
	if p == nil || p.out == nil {
		return ""
	}
	p.out.mu.Lock()
	defer p.out.mu.Unlock()
	if n <= 0 || n > len(p.out.lines) {
		n = len(p.out.lines)
	}
	return strings.Join(p.out.lines[len(p.out.lines)-n:], "\n")
}

// Running 判断进程是否还活着。
func (p *Process) Running() bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return !p.done
}

// BuildArgs 生成 scrcpy 参数，单独抽出来便于测试。
func BuildArgs(opt Options) []string {
	args := []string{}
	if opt.Serial != "" {
		args = append(args, "--serial="+opt.Serial)
	}
	if opt.AdbPath != "" {
		args = append(args, "--adb="+opt.AdbPath)
	}
	if opt.BitRate != "" {
		args = append(args, "--video-bit-rate="+opt.BitRate)
	}
	if opt.MaxSize > 0 {
		args = append(args, fmt.Sprintf("--max-size=%d", opt.MaxSize))
	}
	if opt.MaxFPS > 0 {
		args = append(args, fmt.Sprintf("--max-fps=%d", opt.MaxFPS))
	}
	if opt.TurnScreenOff {
		args = append(args, "--turn-screen-off")
	}
	if opt.StayAwake {
		args = append(args, "--stay-awake")
	}
	if opt.NoAudio {
		args = append(args, "--no-audio")
	}
	if opt.AlwaysOnTop {
		args = append(args, "--always-on-top")
	}
	if opt.Record != "" {
		args = append(args, "--record="+opt.Record)
	}
	return args
}

// Start 启动投屏。
func Start(bin string, opt Options) (*Process, error) {
	if bin == "" {
		return nil, errors.New("没有指定 scrcpy 路径")
	}
	if opt.BitRate != "" && !strings.HasSuffix(opt.BitRate, "M") && !strings.HasSuffix(opt.BitRate, "K") {
		return nil, fmt.Errorf("码率要带单位，比如 8M 或 2000K，当前是 %q", opt.BitRate)
	}

	cmd := exec.Command(bin, BuildArgs(opt)...)
	applyHidden(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("启动 scrcpy 失败：%w", err)
	}

	p := &Process{cmd: cmd, out: &ringLines{}}
	var wg sync.WaitGroup
	collect := func(r interface {
		Read([]byte) (int, error)
	}) {
		defer wg.Done()
		buf := make([]byte, 4096)
		var carry string
		for {
			n, err := r.Read(buf)
			if n > 0 {
				chunk := carry + string(buf[:n])
				parts := strings.Split(chunk, "\n")
				for _, line := range parts[:len(parts)-1] {
					line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
					if line != "" {
						p.out.add(line)
					}
				}
				carry = parts[len(parts)-1]
			}
			if err != nil {
				if carry != "" {
					p.out.add(strings.TrimSpace(carry))
				}
				return
			}
		}
	}
	wg.Add(2)
	go collect(stdout)
	go collect(stderr)

	go func() {
		wg.Wait()
		_ = cmd.Wait()
		p.mu.Lock()
		p.done = true
		p.mu.Unlock()
	}()
	return p, nil
}

// Stop 结束投屏进程。
func (p *Process) Stop() error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.done {
		return nil
	}
	if err := p.cmd.Process.Kill(); err != nil {
		return err
	}
	return nil
}
