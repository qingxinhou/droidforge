package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"droidforge/internal/adb"
	"droidforge/internal/cpu"
	"droidforge/internal/power"
	"droidforge/internal/script"
	"droidforge/internal/tweaks"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// ---------------------------------------------------------------------------
// 显示 / 系统界面调校
// ---------------------------------------------------------------------------

// TweaksState 汇总显示与系统界面的当前设置。
type TweaksState struct {
	Display tweaks.Display `json:"display"`
	SysUI   tweaks.SysUI   `json:"sysui"`
}

func (a *App) tweakExec(serial string) tweaks.Exec {
	client := a.adbClient()
	return func(ctx context.Context, command string) (string, error) {
		return client.Shell(ctx, serial, command)
	}
}

// GetTweaks 读取当前的显示与系统界面设置。
func (a *App) GetTweaks(serial string) (*TweaksState, error) {
	if err := a.requireAdb(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	d, s, err := tweaks.Read(ctx, a.tweakExec(serial))
	if err != nil {
		a.log("err", err.Error())
		return nil, err
	}
	return &TweaksState{Display: d, SysUI: s}, nil
}

// SetResolution 设置分辨率，size 为空表示恢复默认。
func (a *App) SetResolution(serial, size string) error {
	return a.tweakOp(serial, "分辨率 → "+defaultText(size, "默认"), func(ctx context.Context, e tweaks.Exec) error {
		return tweaks.SetResolution(ctx, e, size)
	})
}

// SetDensity 设置显示密度，0 表示恢复默认。
func (a *App) SetDensity(serial string, density int) error {
	label := "显示密度 → 默认"
	if density > 0 {
		label = fmt.Sprintf("显示密度 → %d", density)
	}
	return a.tweakOp(serial, label, func(ctx context.Context, e tweaks.Exec) error {
		return tweaks.SetDensity(ctx, e, density)
	})
}

// SetFontScale 设置字体缩放。
func (a *App) SetFontScale(serial string, scale float64) error {
	return a.tweakOp(serial, fmt.Sprintf("字体缩放 → %.2f", scale), func(ctx context.Context, e tweaks.Exec) error {
		return tweaks.SetFontScale(ctx, e, scale)
	})
}

// SetAnimationScales 设置窗口 / 过渡 / 动画时长倍率。
func (a *App) SetAnimationScales(serial string, window, transition, animator float64) error {
	label := fmt.Sprintf("动画倍率 → %.2f / %.2f / %.2f", window, transition, animator)
	return a.tweakOp(serial, label, func(ctx context.Context, e tweaks.Exec) error {
		return tweaks.SetAnimationScales(ctx, e, window, transition, animator)
	})
}

// SetScreenTimeout 设置自动锁屏时间（毫秒）。
func (a *App) SetScreenTimeout(serial string, ms int) error {
	return a.tweakOp(serial, fmt.Sprintf("锁屏时间 → %d 秒", ms/1000), func(ctx context.Context, e tweaks.Exec) error {
		return tweaks.SetScreenTimeout(ctx, e, ms)
	})
}

// SetIconBlacklist 设置状态栏隐藏图标。
func (a *App) SetIconBlacklist(serial string, icons []string) error {
	return a.tweakOp(serial, "状态栏图标 → "+defaultText(strings.Join(icons, ","), "全部显示"), func(ctx context.Context, e tweaks.Exec) error {
		return tweaks.SetIconBlacklist(ctx, e, icons)
	})
}

// SetClockSeconds 状态栏时钟显示秒。
func (a *App) SetClockSeconds(serial string, on bool) error {
	return a.tweakOp(serial, "时钟显秒 → "+onOff(on), func(ctx context.Context, e tweaks.Exec) error {
		return tweaks.SetClockSeconds(ctx, e, on)
	})
}

// SetBatteryPercent 状态栏显示电量百分比。
func (a *App) SetBatteryPercent(serial string, on bool) error {
	return a.tweakOp(serial, "电量百分比 → "+onOff(on), func(ctx context.Context, e tweaks.Exec) error {
		return tweaks.SetBatteryPercent(ctx, e, on)
	})
}

// SetQuickCount 设置快捷开关数量。
func (a *App) SetQuickCount(serial string, n int) error {
	return a.tweakOp(serial, fmt.Sprintf("快捷开关数 → %d", n), func(ctx context.Context, e tweaks.Exec) error {
		return tweaks.SetQuickCount(ctx, e, n)
	})
}

// SetImmersive 全局沉浸模式。
func (a *App) SetImmersive(serial string, on bool) error {
	return a.tweakOp(serial, "沉浸模式 → "+onOff(on), func(ctx context.Context, e tweaks.Exec) error {
		return tweaks.SetImmersive(ctx, e, on)
	})
}

// SetCaptivePortal 关闭 / 恢复 Wi-Fi 网络检测。
func (a *App) SetCaptivePortal(serial string, disabled bool) error {
	return a.tweakOp(serial, "Wi-Fi 网络检测 → "+onOff(!disabled), func(ctx context.Context, e tweaks.Exec) error {
		return tweaks.SetCaptivePortal(ctx, e, disabled)
	})
}

// SendKeyEvent 发送按键事件。
func (a *App) SendKeyEvent(serial, code string) error {
	return a.tweakOp(serial, "按键 "+code, func(ctx context.Context, e tweaks.Exec) error {
		return tweaks.KeyEvent(ctx, e, code)
	})
}

// SendInputText 输入文本。
func (a *App) SendInputText(serial, text string) error {
	return a.tweakOp(serial, "输入文本", func(ctx context.Context, e tweaks.Exec) error {
		return tweaks.InputText(ctx, e, text)
	})
}

func (a *App) tweakOp(serial, label string, fn func(ctx context.Context, e tweaks.Exec) error) error {
	if err := a.requireAdb(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	a.log("cmd", label)
	if err := fn(ctx, a.tweakExec(serial)); err != nil {
		a.log("err", err.Error())
		return err
	}
	a.log("ok", label)
	return nil
}

// ---------------------------------------------------------------------------
// 电池
// ---------------------------------------------------------------------------

// ReadBattery 读取电池状态。
func (a *App) ReadBattery(serial string) (*power.Info, error) {
	if err := a.requireAdb(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	info, err := power.Read(ctx, a.tweakExec(serial))
	if err != nil {
		a.log("err", err.Error())
		return nil, err
	}
	return &info, nil
}

// MockBattery 应用电池模拟参数。
func (a *App) MockBattery(serial string, m power.Mock) error {
	if err := a.requireAdb(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	a.log("warn", "正在模拟电池状态")
	if err := power.ApplyMock(ctx, a.tweakExec(serial), m); err != nil {
		a.log("err", err.Error())
		return err
	}
	a.log("ok", "电池模拟已应用（记得稍后恢复）")
	return nil
}

// ResetBattery 恢复真实电池状态。
func (a *App) ResetBattery(serial string) error {
	if err := a.requireAdb(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := power.Reset(ctx, a.tweakExec(serial)); err != nil {
		a.log("err", err.Error())
		return err
	}
	a.log("ok", "电池状态已恢复")
	return nil
}

// ---------------------------------------------------------------------------
// CPU
// ---------------------------------------------------------------------------

// ReadCpu 读取各核心实时频率。
func (a *App) ReadCpu(serial string) (*cpu.Snapshot, error) {
	if err := a.requireAdb(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	snap, err := cpu.Read(ctx, a.tweakExec(serial))
	if err != nil {
		return nil, err
	}
	return &snap, nil
}

// ---------------------------------------------------------------------------
// 应用管理增强
// ---------------------------------------------------------------------------

// ListAppsDetail 列出应用并标记禁用状态。
func (a *App) ListAppsDetail(serial string, includeSystem bool) ([]adb.AppInfo, error) {
	if err := a.requireAdb(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	return a.adbClient().ListPackagesDetail(ctx, serial, includeSystem)
}

// AppAction 执行应用操作：enable / disable / force-stop / clear / launch / restore。
func (a *App) AppAction(serial, pkg, action string) error {
	if err := a.requireAdb(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	client := a.adbClient()
	var err error
	switch action {
	case "enable":
		a.log("cmd", "启用 "+pkg)
		err = client.SetAppEnabled(ctx, serial, pkg, true)
	case "disable":
		a.log("cmd", "禁用 "+pkg)
		err = client.SetAppEnabled(ctx, serial, pkg, false)
	case "force-stop":
		a.log("cmd", "强行停止 "+pkg)
		err = client.ForceStopApp(ctx, serial, pkg)
	case "clear":
		a.log("warn", "清除数据 "+pkg)
		err = client.ClearAppData(ctx, serial, pkg)
	case "launch":
		a.log("cmd", "启动 "+pkg)
		err = client.LaunchApp(ctx, serial, pkg)
	case "restore":
		a.log("cmd", "恢复系统应用 "+pkg)
		err = client.RestoreSystemApp(ctx, serial, pkg)
	default:
		err = fmt.Errorf("未知操作：%s", action)
	}
	if err != nil {
		a.log("err", err.Error())
		return err
	}
	a.log("ok", pkg+" → "+action)
	return nil
}

// GetAppDetail 读取应用详情。
func (a *App) GetAppDetail(serial, pkg string) (*adb.AppDetail, error) {
	if err := a.requireAdb(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	d, err := a.adbClient().AppDetailOf(ctx, serial, pkg)
	if err != nil {
		a.log("err", err.Error())
		return nil, err
	}
	return &d, nil
}

// ExtractApps 提取应用安装包到输出目录。
func (a *App) ExtractApps(serial string, pkgs []string) ([]string, error) {
	if err := a.requireAdb(); err != nil {
		return nil, err
	}
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("未选择应用")
	}
	outDir := filepath.Join(a.config().OutputDir, "apk")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}
	ctx, cancel := a.opCtx()
	defer cancel()
	var all []string
	for _, pkg := range pkgs {
		files, err := a.adbClient().ExtractApp(ctx, serial, pkg, outDir, a.lineHandler())
		if err != nil {
			a.log("err", pkg+": "+err.Error())
			return all, err
		}
		all = append(all, files...)
	}
	a.log("ok", fmt.Sprintf("已提取 %d 个安装包到 %s", len(all), outDir))
	return all, nil
}

// InstallApks 安装一个或多个 APK。split 为真时使用 install-multiple。
func (a *App) InstallApks(serial string, apks []string, reinstall, split bool) error {
	if err := a.requireAdb(); err != nil {
		return err
	}
	if len(apks) == 0 {
		return fmt.Errorf("未选择 APK")
	}
	ctx, cancel := a.opCtx()
	defer cancel()
	client := a.adbClient()
	if split || len(apks) > 1 {
		a.log("cmd", fmt.Sprintf("安装 %d 个 APK", len(apks)))
		return client.InstallMultiple(ctx, serial, apks, reinstall, a.lineHandler())
	}
	return client.Install(ctx, serial, apks[0], reinstall, a.lineHandler())
}

// PickFiles 多选文件。
func (a *App) PickFiles(title, pattern string) ([]string, error) {
	ctx := a.appCtx()
	if ctx == nil {
		return nil, fmt.Errorf("应用尚未就绪")
	}
	return wailsruntime.OpenMultipleFilesDialog(ctx, wailsruntime.OpenDialogOptions{
		Title: title,
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "匹配文件", Pattern: pattern},
			{DisplayName: "所有文件", Pattern: "*.*"},
		},
	})
}

// ReadTextFile 读取文本文件（用于导入设置与脚本）。
func (a *App) ReadTextFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ---------------------------------------------------------------------------
// 控机脚本
// ---------------------------------------------------------------------------

// RunScript 执行 BAT 子集脚本，adb 操作会真实下发到设备。
func (a *App) RunScript(serial, source string) (*script.Result, error) {
	if err := a.requireAdb(); err != nil {
		return nil, err
	}
	ctx, cancel := a.opCtx()
	defer cancel()
	client := a.adbClient()
	host := script.Host{
		Shell: func(ctx context.Context, command string) (string, error) {
			return client.Shell(ctx, serial, command)
		},
		Adb: func(ctx context.Context, args []string) (string, error) {
			return client.Run(ctx, serial, args)
		},
		Log: func(kind, text string) {
			a.log(kind, text)
		},
	}
	a.log("info", "开始执行脚本")
	res, err := script.Run(ctx, host, source)
	if err != nil {
		a.log("err", err.Error())
		return &res, err
	}
	a.log("ok", fmt.Sprintf("脚本执行完成：%d 步，%d 个错误", res.Steps, res.Failures))
	return &res, nil
}

func onOff(v bool) string {
	if v {
		return "开"
	}
	return "关"
}

func defaultText(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
