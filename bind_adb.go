package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"droidforge/internal/adb"
)

// ---------------------------------------------------------------------------
// ADB 操作
// ---------------------------------------------------------------------------

// AdbShell 执行任意 shell 命令。
func (a *App) AdbShell(serial, command string) (string, error) {
	if err := a.requireAdb(); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	a.log("cmd", "$ adb shell "+command)
	out, err := a.adbClient().Shell(ctx, serial, command)
	if err != nil {
		a.log("err", err.Error())
	}
	return out, err
}

// AdbReboot 重启设备。
func (a *App) AdbReboot(serial, target string) error {
	if err := a.requireAdb(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	a.log("cmd", "重启到 "+describeTarget(target))
	err := a.adbClient().Reboot(ctx, serial, target)
	if err != nil {
		a.log("err", err.Error())
		return err
	}
	a.log("ok", "重启命令已发送")
	return nil
}

func describeTarget(target string) string {
	switch target {
	case "":
		return "系统"
	case "recovery":
		return "Recovery"
	case "bootloader":
		return "Bootloader (fastboot)"
	case "fastboot":
		return "fastbootd"
	case "edl":
		return "EDL (9008)"
	case "download":
		return "Download 模式"
	case "sideload":
		return "Sideload"
	case "userspace":
		return "用户空间重启"
	default:
		return target
	}
}

// AdbPush 推送文件。
func (a *App) AdbPush(serial, local, remote string) error {
	if err := a.requireAdb(); err != nil {
		return err
	}
	ctx, cancel := a.opCtx()
	defer cancel()
	a.log("cmd", fmt.Sprintf("push %s -> %s", filepath.Base(local), remote))
	return a.adbClient().Push(ctx, serial, local, remote, a.lineHandler())
}

// AdbPull 拉取文件。
func (a *App) AdbPull(serial, remote, local string) error {
	if err := a.requireAdb(); err != nil {
		return err
	}
	ctx, cancel := a.opCtx()
	defer cancel()
	a.log("cmd", fmt.Sprintf("pull %s -> %s", remote, local))
	return a.adbClient().Pull(ctx, serial, remote, local, a.lineHandler())
}

// AdbInstall 安装 APK。
func (a *App) AdbInstall(serial, apk string, reinstall bool) error {
	if err := a.requireAdb(); err != nil {
		return err
	}
	ctx, cancel := a.opCtx()
	defer cancel()
	a.log("cmd", "安装 "+filepath.Base(apk))
	return a.adbClient().Install(ctx, serial, apk, reinstall, a.lineHandler())
}

// ListApps 列出应用。
func (a *App) ListApps(serial string, includeSystem bool) ([]adb.AppInfo, error) {
	if err := a.requireAdb(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return a.adbClient().ListPackages(ctx, serial, includeSystem)
}

// UninstallApp 卸载应用。
func (a *App) UninstallApp(serial, pkg string, keepData bool) error {
	if err := a.requireAdb(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	a.log("cmd", "卸载 "+pkg)
	if err := a.adbClient().Uninstall(ctx, serial, pkg, keepData); err != nil {
		a.log("err", err.Error())
		return err
	}
	a.log("ok", "已卸载 "+pkg)
	return nil
}

// Screenshot 截屏并保存到输出目录。
func (a *App) Screenshot(serial string) (string, error) {
	if err := a.requireAdb(); err != nil {
		return "", err
	}
	outDir := a.config().OutputDir
	_ = os.MkdirAll(outDir, 0o755)
	dst := filepath.Join(outDir, fmt.Sprintf("screenshot_%s.png", time.Now().Format("20060102_150405")))
	ctx, cancel := a.opCtx()
	defer cancel()
	if err := a.adbClient().Screenshot(ctx, serial, dst); err != nil {
		a.log("err", err.Error())
		return "", err
	}
	a.log("ok", "截图已保存："+dst)
	return dst, nil
}

// WirelessConnect 连接无线调试设备。
func (a *App) WirelessConnect(addr string) (string, error) {
	if err := a.requireAdb(); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := a.adbClient().Connect(ctx, addr)
	if err != nil {
		return "", err
	}
	a.log("ok", out)
	return out, nil
}

// EnableWireless 将 USB 设备切换到无线调试。
func (a *App) EnableWireless(serial string, port int) (string, error) {
	if err := a.requireAdb(); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := a.adbClient().EnableTcpip(ctx, serial, port)
	if err != nil {
		return "", err
	}
	a.log("ok", out)
	return out, nil
}
