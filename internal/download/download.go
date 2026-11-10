// Package download 实现多线程分段下载与断点续传。
//
// 面向 OTA 全量包这类 1–2 GB 的文件：先探测服务端是否支持 Range，
// 支持则切成若干段并发下载，并把每段进度写进 .dpart 状态文件；
// 中断后再次调用同一个目标路径即可从断点继续。服务端不支持 Range 时
// 自动降级为单流下载，不静默失败。
package download

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ErrRangeUnsupported 表示服务端不支持分段请求，调用方应改用单流下载。
var ErrRangeUnsupported = errors.New("服务端不支持 Range 请求")

// LinkError 表示链接本身已不可用（常见于 OTA 直链过期）。
type LinkError struct {
	Status int
	URL    string
}

func (e *LinkError) Error() string {
	switch e.Status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Sprintf("下载链接被拒绝（HTTP %d），通常是直链已过期或需要新的鉴权参数", e.Status)
	case http.StatusNotFound, http.StatusGone:
		return fmt.Sprintf("下载链接已失效（HTTP %d），请重新获取", e.Status)
	default:
		return fmt.Sprintf("下载失败（HTTP %d）", e.Status)
	}
}

// Options 控制下载行为。
type Options struct {
	// Threads 为并发分段数，<=1 时按 4 处理。
	Threads   int
	UserAgent string
	Referer   string
	// Resume 为 false 时丢弃已有进度重新下载。
	Resume bool
	// OnProgress 会被高频调用，内部已按时间节流到约 5 次/秒。
	OnProgress func(done, total int64)
}

func (o Options) normalized() Options {
	if o.Threads < 1 {
		o.Threads = 4
	}
	if o.Threads > 16 {
		o.Threads = 16
	}
	if o.UserAgent == "" {
		o.UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) DroidForge"
	}
	return o
}

// Result 是一次下载的结果。
type Result struct {
	Path          string `json:"path"`
	Size          int64  `json:"size"`
	Threads       int    `json:"threads"`
	SupportsRange bool   `json:"supportsRange"`
	Resumed       bool   `json:"resumed"`
	Note          string `json:"note"`
}

// Probe 探测远端文件大小与分段支持情况。
type Probe struct {
	URL           string `json:"url"`
	FileName      string `json:"fileName"`
	Size          int64  `json:"size"`
	SupportsRange bool   `json:"supportsRange"`
	AcceptRanges  string `json:"acceptRanges"`
	ETag          string `json:"etag"`
	LastModified  string `json:"lastModified"`
}

// Detect 用一次 Range 请求拿到大小与分段能力，不下载正文。
func Detect(ctx context.Context, rawURL string, opt Options) (*Probe, error) {
	opt = opt.normalized()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Range", "bytes=0-0")
	req.Header.Set("User-Agent", opt.UserAgent)
	if opt.Referer != "" {
		req.Header.Set("Referer", opt.Referer)
	}

	client := &http.Client{Timeout: 45 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("连接下载地址失败：%w", err)
	}
	defer resp.Body.Close()
	_, _ = io.CopyN(io.Discard, resp.Body, 512)

	if resp.StatusCode >= 400 {
		return nil, &LinkError{Status: resp.StatusCode, URL: rawURL}
	}

	p := &Probe{
		URL:          rawURL,
		AcceptRanges: resp.Header.Get("Accept-Ranges"),
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
	}
	switch resp.StatusCode {
	case http.StatusPartialContent:
		p.SupportsRange = true
		p.Size = parseContentRangeTotal(resp.Header.Get("Content-Range"))
	case http.StatusOK:
		p.SupportsRange = strings.EqualFold(p.AcceptRanges, "bytes")
		p.Size = resp.ContentLength
	}
	p.FileName = fileName(resp, rawURL)
	return p, nil
}

// state 是断点续传的落盘状态。
type state struct {
	URL          string    `json:"url"`
	Size         int64     `json:"size"`
	ETag         string    `json:"etag"`
	LastModified string    `json:"lastModified"`
	Segments     []segment `json:"segments"`
}

type segment struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"` // 含
	Done  int64 `json:"done"`
}

func (s *segment) remaining() int64 { return s.End - s.Start + 1 - s.Done }

// Fetch 下载 rawURL 到 dst（dst 为具体文件路径）。
func Fetch(ctx context.Context, rawURL, dst string, opt Options) (*Result, error) {
	opt = opt.normalized()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return nil, err
	}

	probe, err := Detect(ctx, rawURL, opt)
	if err != nil {
		return nil, err
	}
	res := &Result{Path: dst, Size: probe.Size, Threads: 1, SupportsRange: probe.SupportsRange}

	if !probe.SupportsRange || probe.Size <= 0 {
		res.Note = "服务端不支持分段，已改为单流下载"
		if err := fetchStream(ctx, rawURL, dst, opt, probe.Size); err != nil {
			return nil, err
		}
		if probe.Size <= 0 {
			if st, err := os.Stat(dst); err == nil {
				res.Size = st.Size()
			}
		}
		return res, nil
	}

	res.Threads = opt.Threads
	st, resumed, err := loadOrCreateState(dst, rawURL, probe, opt)
	if err != nil {
		return nil, err
	}
	res.Resumed = resumed

	if err := runSegments(ctx, dst, rawURL, st, opt); err != nil {
		return nil, err
	}
	res.Size = st.Size
	return res, nil
}

// loadOrCreateState 读取或初始化状态文件，并保证目标文件已预分配。
func loadOrCreateState(dst, rawURL string, probe *Probe, opt Options) (*state, bool, error) {
	statePath := dst + ".dpart"

	if opt.Resume {
		if b, err := os.ReadFile(statePath); err == nil {
			var s state
			if json.Unmarshal(b, &s) == nil && s.URL == rawURL && s.Size == probe.Size && len(s.Segments) > 0 {
				if fi, err := os.Stat(dst); err == nil && fi.Size() == s.Size {
					return &s, true, nil
				}
			}
		}
	}

	_ = os.Remove(statePath)

	f, err := os.Create(dst)
	if err != nil {
		return nil, false, err
	}
	if err := f.Truncate(probe.Size); err != nil {
		f.Close()
		return nil, false, err
	}
	f.Close()

	n := opt.Threads
	if probe.Size < 1<<20 {
		n = 1
	} else if int64(n) > probe.Size>>20 {
		n = int(probe.Size >> 20)
	}
	s := &state{URL: rawURL, Size: probe.Size, ETag: probe.ETag, LastModified: probe.LastModified}
	chunk := probe.Size / int64(n)
	for i := 0; i < n; i++ {
		start := int64(i) * chunk
		end := start + chunk - 1
		if i == n-1 {
			end = probe.Size - 1
		}
		s.Segments = append(s.Segments, segment{Start: start, End: end})
	}
	return s, false, nil
}

// runSegments 并发下载所有未完成分段，失败时把进度落盘。
func runSegments(ctx context.Context, dst, rawURL string, st *state, opt Options) error {
	f, err := os.OpenFile(dst, os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	var totalDone atomic.Int64
	for i := range st.Segments {
		totalDone.Add(st.Segments[i].Done)
	}

	// 进度节流：避免每读 32KB 就触发一次界面刷新。
	stop := make(chan struct{})
	var wgProg sync.WaitGroup
	if opt.OnProgress != nil {
		wgProg.Add(1)
		go func() {
			defer wgProg.Done()
			ticker := time.NewTicker(200 * time.Millisecond)
			defer ticker.Stop()
			var last int64 = -1
			for {
				select {
				case <-stop:
					opt.OnProgress(totalDone.Load(), st.Size)
					return
				case <-ticker.C:
					d := totalDone.Load()
					if d != last {
						last = d
						opt.OnProgress(d, st.Size)
					}
				}
			}
		}()
	}

	// stateMu 保护 Segments 的 Done 字段：worker 持续更新，落盘协程同时读。
	var stateMu sync.Mutex
	persist := func() {
		stateMu.Lock()
		defer stateMu.Unlock()
		_ = writeState(dst+".dpart", st)
	}

	// 每 2 秒落盘一次进度，进程被杀掉也能续。
	persistStop := make(chan struct{})
	var wgPersist sync.WaitGroup
	wgPersist.Add(1)
	go func() {
		defer wgPersist.Done()
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-persistStop:
				return
			case <-ticker.C:
				persist()
			}
		}
	}()

	errCh := make(chan error, len(st.Segments))
	var wg sync.WaitGroup
	for i := range st.Segments {
		seg := &st.Segments[i]
		if seg.remaining() <= 0 {
			continue
		}
		wg.Add(1)
		go func(seg *segment) {
			defer wg.Done()
			start := seg.Start + seg.Done
			written, err := fetchRange(ctx, rawURL, f, start, seg.End, opt, func(n int64) {
				stateMu.Lock()
				seg.Done += n
				stateMu.Unlock()
				totalDone.Add(n)
			})
			if err != nil {
				errCh <- err
				return
			}
			if start+written <= seg.End {
				errCh <- fmt.Errorf("分段 %d-%d 未下载完整", start, seg.End)
			}
		}(seg)
	}
	wg.Wait()

	close(stop)
	wgProg.Wait()
	close(persistStop)
	wgPersist.Wait()

	select {
	case err := <-errCh:
		persist()
		if ctx.Err() != nil {
			return fmt.Errorf("已中止，进度已保存：%w", ctx.Err())
		}
		return err
	default:
	}

	_ = os.Remove(dst + ".dpart")
	return nil
}

// fetchRange 下载 [start,end] 区间并写入文件对应偏移，返回本次写入字节数。
func fetchRange(ctx context.Context, rawURL string, f *os.File, start, end int64, opt Options, onWrite func(int64)) (int64, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		n, err := fetchRangeOnce(ctx, rawURL, f, start, end, opt, onWrite)
		if err == nil {
			return n, nil
		}
		lastErr = err
		if errors.Is(err, ErrRangeUnsupported) {
			return 0, err
		}
		select {
		case <-ctx.Done():
			return n, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 800 * time.Millisecond):
		}
	}
	return 0, lastErr
}

func fetchRangeOnce(ctx context.Context, rawURL string, f *os.File, start, end int64, opt Options, onWrite func(int64)) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	req.Header.Set("User-Agent", opt.UserAgent)
	if opt.Referer != "" {
		req.Header.Set("Referer", opt.Referer)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		// 服务端忽略了 Range，继续写会把文件写乱。
		return 0, ErrRangeUnsupported
	}
	if resp.StatusCode != http.StatusPartialContent {
		return 0, &LinkError{Status: resp.StatusCode, URL: rawURL}
	}

	buf := make([]byte, 256<<10)
	pos := start
	var written int64
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.WriteAt(buf[:n], pos); werr != nil {
				return written, werr
			}
			pos += int64(n)
			written += int64(n)
			onWrite(int64(n))
			if pos > end+1 {
				return written, fmt.Errorf("服务端返回的数据超出请求区间")
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return written, rerr
		}
	}
	return written, nil
}

// fetchStream 单流下载，不支持续传。
func fetchStream(ctx context.Context, rawURL, dst string, opt Options, expect int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", opt.UserAgent)
	if opt.Referer != "" {
		req.Header.Set("Referer", opt.Referer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return &LinkError{Status: resp.StatusCode, URL: rawURL}
	}

	tmp := dst + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer f.Close()

	buf := make([]byte, 1<<20)
	var done int64
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return werr
			}
			done += int64(n)
			if opt.OnProgress != nil && expect > 0 {
				opt.OnProgress(done, expect)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

func writeState(path string, st *state) error {
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func parseContentRangeTotal(v string) int64 {
	// 形如 bytes 0-0/1234567
	slash := strings.LastIndex(v, "/")
	if slash < 0 {
		return 0
	}
	var total int64
	_, err := fmt.Sscanf(v[slash+1:], "%d", &total)
	if err != nil {
		return 0
	}
	return total
}

func fileName(resp *http.Response, rawURL string) string {
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if i := strings.Index(cd, "filename="); i >= 0 {
			name := strings.Trim(cd[i+len("filename="):], `"; `)
			if name != "" {
				return name
			}
		}
	}
	if u, err := url.Parse(rawURL); err == nil {
		base := filepath.Base(u.Path)
		if base != "" && base != "." && base != "/" {
			return base
		}
	}
	return "download.bin"
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
