package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"droidforge/internal/device"
	"droidforge/internal/root"
)

// ---------------------------------------------------------------------------
// Root
// ---------------------------------------------------------------------------

// GetRootSolutions 返回支持的 root 方案。
func (a *App) GetRootSolutions() []root.Solution {
	return root.Solutions()
}

// GetRootStatus 检测设备 root 状态。
func (a *App) GetRootStatus(serial string) (device.RootStatus, error) {
	if err := a.requireAdb(); err != nil {
		return device.RootStatus{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	st := device.DetectRoot(ctx, a.adbClient(), serial)
	if st.Rooted {
		a.log("ok", "已检测到 root："+strings.Join(st.Solutions, ", "))
	} else {
		a.log("info", "设备未 root")
	}
	return st, nil
}

// ExtractBoot 从设备导出 boot 分区（需要 root）。
func (a *App) ExtractBoot(serial, partition, outPath string) (string, error) {
	if err := a.requireAdb(); err != nil {
		return "", err
	}
	if outPath == "" {
		outPath = filepath.Join(a.config().OutputDir, partition+"_"+time.Now().Format("20060102_150405")+".img")
	}
	_ = os.MkdirAll(filepath.Dir(outPath), 0o755)
	ctx, cancel := a.opCtx()
	defer cancel()
	a.log("info", "从设备导出 "+partition+" 分区…")
	if err := root.ExtractBootFromDevice(ctx, a.adbClient(), serial, partition, outPath, a.lineHandler()); err != nil {
		a.log("err", err.Error())
		return "", err
	}
	a.log("ok", "已导出 "+outPath)
	return outPath, nil
}

// InstallRoot 执行一键打补丁。
func (a *App) InstallRoot(serial, solution, bootImage, apkPath string) (*root.InstallResult, error) {
	if err := a.requireAdb(); err != nil {
		return nil, err
	}
	ctx, cancel := a.opCtx()
	defer cancel()
	opts := root.InstallOptions{
		Serial:    serial,
		Solution:  solution,
		BootImage: bootImage,
		WorkDir:   a.config().CacheDir,
		APKPath:   apkPath,
		Mirror:    a.config().Mirror,
	}
	res, err := root.Install(ctx, a.adbClient(), opts, a.lineHandler())
	if err != nil {
		a.log("err", err.Error())
		return nil, err
	}
	a.log("ok", "补丁镜像已生成："+res.PatchedImage)
	a.log("info", "下一步：重启到 bootloader，用 fastboot 把它刷入 boot / init_boot 分区")
	return res, nil
}
