package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"droidforge/internal/remote"
	"droidforge/internal/store"
)

// ---------------------------------------------------------------------------
// 远程 OTA（按 HTTP Range 直接读取链接里的 payload）
// ---------------------------------------------------------------------------

func (a *App) remoteOptions() remote.Options {
	return remote.Options{UserAgent: a.config().OtaUserAgent, Referer: a.config().OtaReferer}
}

// SaveOtaRequest 保存远程 OTA 请求使用的 UA / Referer。
func (a *App) SaveOtaRequest(ua, referer string) {
	saved := a.updateConfig(func(c *store.Config) {
		c.OtaUserAgent = strings.TrimSpace(ua)
		c.OtaReferer = strings.TrimSpace(referer)
	})
	_ = store.Save(saved)
	a.log("info", "已更新远程请求头")
}

// OtaOutputDir 根据文件名给出默认输出目录。
func (a *App) OtaOutputDir(fileName string) string {
	base := strings.TrimSuffix(filepath.Base(fileName), filepath.Ext(fileName))
	if base == "" || base == "." {
		base = "ota"
	}
	return filepath.Join(a.config().OutputDir, base)
}

// InspectRemoteOta 远程解析 OTA 链接（只读取 zip 目录与 payload 清单）。
func (a *App) InspectRemoteOta(link string) (*remote.Info, error) {
	ctx, cancel := a.opCtx()
	defer cancel()
	ctx, cancelTimeout := context.WithTimeout(ctx, 120*time.Second)
	defer cancelTimeout()

	a.log("cmd", "解析远程 OTA："+link)
	info, err := remote.Inspect(ctx, link, a.remoteOptions())
	if err != nil {
		a.log("err", err.Error())
		return nil, err
	}
	if !info.SupportsRange {
		a.log("warn", info.Note)
		return info, nil
	}
	a.log("ok", fmt.Sprintf("%s · %.2f GB · %d 个分区（payload v%d）",
		info.FileName, float64(info.TotalSize)/1024/1024/1024, len(info.Partitions), info.Version))
	if !info.Full {
		a.log("warn", "该 OTA 含增量操作，部分分区无法独立还原")
	}
	return info, nil
}

// ExtractRemoteOta 从远程 OTA 按需提取分区（只下载选中分区的数据块）。
func (a *App) ExtractRemoteOta(link string, names []string, outDir string) ([]string, error) {
	if outDir == "" {
		outDir = a.config().OutputDir
	}
	ctx, cancel := a.opCtx()
	defer cancel()

	a.log("info", fmt.Sprintf("开始远程提取 %d 个分区到 %s", len(names), outDir))
	files, err := remote.Extract(ctx, link, names, outDir, a.remoteOptions(), func(name string, percent int) {
		a.progress(name, percent, false)
	})
	if err != nil {
		a.log("err", err.Error())
		a.progress("远程提取", 0, true)
		return files, err
	}
	a.progress("远程提取", 100, true)
	a.log("ok", fmt.Sprintf("已提取 %d 个镜像", len(files)))
	return files, nil
}

// DownloadOta 下载整包，返回本地路径。
func (a *App) DownloadOta(link string) (string, error) {
	ctx, cancel := a.opCtx()
	defer cancel()
	a.log("info", "开始下载整包…")
	path, err := remote.Download(ctx, link, a.config().OutputDir, a.remoteOptions(), func(name string, percent int) {
		a.progress("下载 "+name, percent, false)
	})
	if err != nil {
		a.log("err", err.Error())
		a.progress("下载", 0, true)
		return "", err
	}
	a.progress("下载完成", 100, true)
	a.log("ok", "已保存："+path)
	return path, nil
}
