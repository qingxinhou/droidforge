package main

import (
	"context"
	"path/filepath"
	"runtime"
	"time"

	"droidforge/internal/adb"
	"droidforge/internal/device"
	"droidforge/internal/platform"
	"droidforge/internal/store"
)

// ---------------------------------------------------------------------------
// 环境与设备
// ---------------------------------------------------------------------------

type Environment struct {
	Tools           platform.Tools `json:"tools"`
	AdbDevices      []adb.Device   `json:"adbDevices"`
	FastbootDevices []string       `json:"fastbootDevices"`
	Config          store.Config   `json:"config"`
	OS              string         `json:"os"`
	Arch            string         `json:"arch"`
	AppVersion      string         `json:"appVersion"`
}

// GetEnvironment 返回工具链状态与当前设备列表。
func (a *App) GetEnvironment() Environment {
	env := Environment{
		Tools:      a.toolset(),
		Config:     a.config(),
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		AppVersion: appVersion,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if a.toolset().AdbPath != "" {
		if list, err := a.adbClient().Devices(ctx); err == nil {
			env.AdbDevices = list
		}
	}
	if a.toolset().FastbootPath != "" {
		if list, err := a.fbClient().Devices(ctx); err == nil {
			env.FastbootDevices = list
		}
	}
	return env
}

// RefreshDevices 重新扫描设备。
func (a *App) RefreshDevices() Environment {
	return a.GetEnvironment()
}

// SetToolsPaths 手动指定 adb / fastboot 路径。
func (a *App) SetToolsPaths(adbPath, fastbootPath string) Environment {
	saved := a.updateConfig(func(c *store.Config) {
		c.CustomAdb = adbPath
		c.CustomFastboot = fastbootPath
	})
	a.setTools(platform.Detect(adbPath, fastbootPath))
	_ = store.Save(saved)
	a.log("info", "已更新工具路径："+a.toolset().Hint)
	return a.GetEnvironment()
}

// DownloadPlatformTools 下载官方 platform-tools 到缓存目录。
func (a *App) DownloadPlatformTools() (string, error) {
	ctx, cancel := a.opCtx()
	defer cancel()
	dest := filepath.Join(a.config().CacheDir, "platform-tools")
	a.log("info", "开始下载 platform-tools…")
	err := platform.DownloadPlatformTools(ctx, dest, func(line string) {
		a.log("out", line)
	}, func(p int) {
		a.progress("下载 platform-tools", p, false)
	})
	if err != nil {
		a.log("err", err.Error())
		return "", err
	}
	a.progress("下载 platform-tools", 100, true)
	a.setTools(platform.Detect(a.config().CustomAdb, a.config().CustomFastboot))
	a.log("ok", "platform-tools 安装完成")
	return dest, nil
}

// GetDeviceInfo 采集设备信息。
func (a *App) GetDeviceInfo(serial string) (*device.DeviceInfo, error) {
	if err := a.requireAdb(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return device.Collect(ctx, a.adbClient(), serial)
}

// GetPartitions 列出分区表。
func (a *App) GetPartitions(serial string) ([]device.Partition, error) {
	if err := a.requireAdb(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return device.ListPartitions(ctx, a.adbClient(), serial)
}
