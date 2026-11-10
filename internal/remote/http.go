// Package remote 提供基于 HTTP Range 的远程文件读取能力，
// 用于在完全不下载整个 OTA 的前提下解析 zip / payload.bin 并抽取指定分区。
package remote

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

// ErrNoRange 表示服务器不支持 Range 请求，无法做远程定位。
var ErrNoRange = errors.New("服务器不支持 Range 断点请求，无法远程解析；可改用「下载整包」后再提取")

// Options 是远程请求的可选项。
type Options struct {
	UserAgent string
	Referer   string
	Timeout   time.Duration
}

// DefaultUserAgent 是默认 UA（多数 CDN 都接受）。
const DefaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36 DroidForge/1.0"

// AndroidUserAgent 模拟 Android DownloadManager，部分厂商 OTA 服务器认这个 UA。
const AndroidUserAgent = "AndroidDownloadManager/14 (Linux; U; Android 14; Build/AP1A)"

// File 是一个可随机读取的远程文件。
type File struct {
	URL    string
	Name   string
	Size   int64
	Range  bool
	ctx    context.Context
	client *http.Client
	header http.Header
}

func normalizeOptions(opt Options) Options {
	if strings.TrimSpace(opt.UserAgent) == "" {
		opt.UserAgent = DefaultUserAgent
	}
	if opt.Timeout <= 0 {
		opt.Timeout = 60 * time.Second
	}
	return opt
}

// Open 探测远程文件大小、文件名与 Range 支持情况。
func Open(ctx context.Context, rawURL string, opt Options) (*File, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, errors.New("链接不能为空")
	}
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		return nil, errors.New("只支持 http / https 链接")
	}
	opt = normalizeOptions(opt)

	f := &File{
		ctx:    ctx,
		header: http.Header{},
		client: &http.Client{
			Timeout: opt.Timeout,
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				DisableCompression:    true, // 必须关闭，否则字节偏移会错位
				MaxIdleConnsPerHost:   4,
				ResponseHeaderTimeout: 30 * time.Second,
			},
		},
	}
	f.URL = rawURL
	f.header.Set("User-Agent", opt.UserAgent)
	f.header.Set("Accept", "*/*")
	f.header.Set("Accept-Encoding", "identity")
	if opt.Referer != "" {
		f.header.Set("Referer", opt.Referer)
	}

	resp, err := f.do("bytes=0-0")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))

	if resp.Request != nil && resp.Request.URL != nil {
		f.URL = resp.Request.URL.String()
	}

	switch resp.StatusCode {
	case http.StatusPartialContent:
		total, err := parseContentRangeTotal(resp.Header.Get("Content-Range"))
		if err != nil {
			return nil, err
		}
		f.Size = total
		f.Range = true
	case http.StatusOK:
		// 服务器忽略了 Range（或重定向到了不支持的镜像），只能整段读取。
		f.Range = false
		f.Size = resp.ContentLength
		if f.Size < 0 {
			f.Size = 0
		}
	default:
		return nil, fmt.Errorf("请求失败：%s（%s · %s）", resp.Status, resp.Header.Get("Server"), f.URL)
	}

	f.Name = fileName(resp, f.URL)
	return f, nil
}

func (f *File) do(rangeHeader string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(f.ctx, http.MethodGet, f.URL, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range f.header {
		req.Header[k] = v
	}
	if rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}
	return f.client.Do(req)
}

// fetch 读取 [off, off+len(p)) 区间。
func (f *File) fetch(p []byte, off int64) error {
	if !f.Range {
		return ErrNoRange
	}
	if len(p) == 0 {
		return nil
	}
	start := off
	end := off + int64(len(p)) - 1
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if err := f.ctx.Err(); err != nil {
			return err
		}
		n, err := f.fetchOnce(p, start, end)
		if err == nil {
			if n < len(p) {
				return io.ErrUnexpectedEOF
			}
			return nil
		}
		lastErr = err
		if errors.Is(err, ErrNoRange) {
			return err
		}
		select {
		case <-f.ctx.Done():
			return f.ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 400 * time.Millisecond):
		}
	}
	return lastErr
}

func (f *File) fetchOnce(p []byte, start, end int64) (int, error) {
	resp, err := f.do(fmt.Sprintf("bytes=%d-%d", start, end))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusPartialContent:
	case http.StatusOK:
		return 0, ErrNoRange
	default:
		return 0, fmt.Errorf("分段读取失败：%s", resp.Status)
	}
	return io.ReadFull(resp.Body, p)
}

// ReadAt 实现 io.ReaderAt。
func (f *File) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, errors.New("偏移不能为负")
	}
	if off >= f.Size {
		return 0, io.EOF
	}
	if off+int64(len(p)) > f.Size {
		p = p[:f.Size-off]
	}
	if err := f.fetch(p, off); err != nil {
		return 0, err
	}
	return len(p), nil
}

const streamChunk = 4 << 20

// Reader 返回从 offset 起、长度为 length 的顺序读取器。
// 内部按 4 MiB 分块请求，足以让 xz / bzip2 之类的解压器高效工作。
func (f *File) Reader(offset, length int64) io.ReadCloser {
	if offset < 0 {
		offset = 0
	}
	if max := f.Size - offset; length > max {
		length = max
	}
	if length < 0 {
		length = 0
	}
	return &rangeStream{f: f, off: offset, remain: length}
}

type rangeStream struct {
	f      *File
	off    int64
	remain int64
	buf    []byte
	pos    int
}

func (s *rangeStream) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if s.pos >= len(s.buf) {
		if s.remain <= 0 {
			return 0, io.EOF
		}
		n := int64(streamChunk)
		if n > s.remain {
			n = s.remain
		}
		if int64(cap(s.buf)) >= n {
			s.buf = s.buf[:n]
		} else {
			s.buf = make([]byte, n)
		}
		if err := s.f.fetch(s.buf, s.off); err != nil {
			return 0, err
		}
		s.off += n
		s.remain -= n
		s.pos = 0
	}
	n := copy(p, s.buf[s.pos:])
	s.pos += n
	return n, nil
}

func (s *rangeStream) Close() error { return nil }

func parseContentRangeTotal(v string) (int64, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, errors.New("服务器未返回 Content-Range，无法确定文件大小")
	}
	idx := strings.LastIndex(v, "/")
	if idx < 0 {
		return 0, fmt.Errorf("无法解析 Content-Range：%s", v)
	}
	total := strings.TrimSpace(v[idx+1:])
	if total == "*" {
		return 0, errors.New("服务器未返回文件大小")
	}
	n, err := strconv.ParseInt(total, 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("无法解析文件大小：%s", v)
	}
	return n, nil
}

// fileName 依次尝试 Content-Disposition、URL 路径，最后兜底。
func fileName(resp *http.Response, rawURL string) string {
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if _, params, err := mime.ParseMediaType(cd); err == nil {
			if v := params["filename"]; v != "" {
				if decoded, derr := url.QueryUnescape(v); derr == nil {
					return decoded
				}
				return v
			}
		}
		// 兼容 filename*=UTF-8''xxx 这种不规范写法
		if i := strings.Index(cd, "filename="); i >= 0 {
			v := strings.TrimSpace(cd[i+len("filename="):])
			v = strings.Trim(v, `"'`)
			if v != "" {
				return v
			}
		}
	}
	if u, err := url.Parse(rawURL); err == nil {
		if base := path.Base(u.Path); base != "" && base != "/" && base != "." {
			if decoded, derr := url.QueryUnescape(base); derr == nil {
				return decoded
			}
			return base
		}
	}
	return "ota.zip"
}

// downloadTo 把整个远程文件下载到 dst。
func downloadTo(ctx context.Context, rawURL, dst string, opt Options, onProgress func(int)) (string, error) {
	opt = normalizeOptions(opt)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(rawURL), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", opt.UserAgent)
	req.Header.Set("Accept", "*/*")
	if opt.Referer != "" {
		req.Header.Set("Referer", opt.Referer)
	}

	client := &http.Client{Timeout: 0}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return "", fmt.Errorf("下载失败：%s", resp.Status)
	}
	return WriteStream(resp.Body, resp.ContentLength, dst, onProgress)
}

// WriteStream 把数据流写入文件，并按 total 报告进度（total<=0 时不报百分比）。
func WriteStream(r io.Reader, total int64, dst string, onProgress func(int)) (string, error) {
	out, err := createFile(dst)
	if err != nil {
		return "", err
	}
	defer out.Close()

	buf := make([]byte, 1<<20)
	var done int64
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				return "", werr
			}
			done += int64(n)
			if onProgress != nil && total > 0 {
				pct := int(done * 100 / total)
				if pct > 100 {
					pct = 100
				}
				onProgress(pct)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}
	if onProgress != nil {
		onProgress(100)
	}
	return dst, out.Sync()
}
