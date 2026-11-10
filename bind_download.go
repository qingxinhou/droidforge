package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"droidforge/internal/download"
)

// ---------------------------------------------------------------------------
// 多线程断点续传下载
// ---------------------------------------------------------------------------

// ProbeDownload 探测直链的文件名、大小与是否支持分段。
func (a *App) ProbeDownload(link string) (*download.Probe, error) {
	ctx, cancel := a.opCtx()
	defer cancel()
	ctx, timeout := context.WithTimeout(ctx, 60*time.Second)
	defer timeout()

	p, err := download.Detect(ctx, link, a.downloadOptions(0))
	if err != nil {
		a.log("err", err.Error())
		return nil, err
	}
	mode := "单流"
	if p.SupportsRange {
		mode = "支持分段"
	}
	a.log("ok", fmt.Sprintf("%s，%.2f GB，%s", p.FileName, float64(p.Size)/(1<<30), mode))
	return p, nil
}

// DownloadFile 下载直链到本地，支持多线程与断点续传。
//
// dst 为空时按远端文件名落到输出目录。中断后再次点击下载会从断点继续。
func (a *App) DownloadFile(link, dst string, threads int) (*download.Result, error) {
	ctx, cancel := a.opCtx()
	defer cancel()

	opt := a.downloadOptions(threads)
	if dst == "" {
		probe, err := download.Detect(ctx, link, opt)
		if err != nil {
			a.log("err", err.Error())
			return nil, err
		}
		name := sanitizeName(probe.FileName)
		if name == "" {
			name = "download_" + time.Now().Format("20060102_150405") + ".bin"
		}
		dst = filepath.Join(a.config().OutputDir, name)
	}

	a.log("info", fmt.Sprintf("开始下载 %s → %s", link, dst))
	res, err := download.Fetch(ctx, link, dst, opt)
	if err != nil {
		a.log("err", err.Error())
		return nil, err
	}

	a.progress("下载完成", 100, true)
	if res.Resumed {
		a.log("ok", fmt.Sprintf("已续传完成，共 %.2f GB（%d 线程）", float64(res.Size)/(1<<30), res.Threads))
	} else {
		a.log("ok", fmt.Sprintf("下载完成，共 %.2f GB（%d 线程）", float64(res.Size)/(1<<30), res.Threads))
	}
	if res.Note != "" {
		a.log("warn", res.Note)
	}
	return res, nil
}

// downloadOptions 复用远程 OTA 的 UA / Referer 设置。
func (a *App) downloadOptions(threads int) download.Options {
	cfg := a.config()
	return download.Options{
		Threads:   threads,
		UserAgent: cfg.OtaUserAgent,
		Referer:   cfg.OtaReferer,
		Resume:    true,
		OnProgress: func(done, total int64) {
			if total <= 0 {
				return
			}
			percent := int(done * 100 / total)
			a.progress(fmt.Sprintf("下载 %d%%", percent), percent, false)
		},
	}
}

// sanitizeName 去掉远端文件名里不适合落盘的字符。
//
// 远端给的 Content-Disposition 完全不可信，除了非法字符还要挡掉
// 全是点、或者带路径分隔的名字——否则可能落到输出目录之外。
func sanitizeName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range name {
		switch r {
		case '\\', '/', ':', '*', '?', '"', '<', '>', '|':
			b.WriteRune('_')
		default:
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	trimmed := strings.Trim(out, ".")
	if trimmed == "" {
		return ""
	}
	if rs := []rune(out); len(rs) > 120 {
		out = string(rs[:120])
	}
	return out
}
