package main

import (
	"context"
	"fmt"
	"time"

	"droidforge/internal/magisk"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// ---------------------------------------------------------------------------
// Magisk / KernelSU 模块
// ---------------------------------------------------------------------------

// InspectModules 解析指定的模块包。
func (a *App) InspectModules(paths []string) []magisk.Module {
	out := make([]magisk.Module, 0, len(paths))
	for _, p := range paths {
		m, err := magisk.Inspect(p)
		if err != nil {
			a.log("err", fmt.Sprintf("读取 %s 失败：%v", p, err))
			continue
		}
		if !m.Valid {
			a.log("warn", fmt.Sprintf("%s：%s", m.FileName, m.Error))
		} else {
			a.log("ok", fmt.Sprintf("%s %s（%s）", m.Name, m.Version, m.FileName))
		}
		out = append(out, *m)
	}
	return out
}

// ScanModuleDir 扫描目录下的模块包。
func (a *App) ScanModuleDir(dir string) ([]magisk.Module, error) {
	mods, err := magisk.ScanDir(dir)
	if err != nil {
		a.log("err", err.Error())
		return nil, err
	}
	a.log("ok", fmt.Sprintf("找到 %d 个模块包", len(mods)))
	return mods, nil
}

// PickModule 选一个模块包。
//
// Wails 2.12 的打开对话框不支持多选，所以批量场景走「扫目录」，
// 单个场景走这里，前端把结果追加到列表里。
func (a *App) PickModule() (*magisk.Module, error) {
	p, err := wailsruntime.OpenFileDialog(a.appCtx(), wailsruntime.OpenDialogOptions{
		Title: "选择模块包",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "Magisk 模块", Pattern: "*.zip"},
		},
	})
	if err != nil || p == "" {
		return nil, err
	}
	m, err := magisk.Inspect(p)
	if err != nil {
		a.log("err", fmt.Sprintf("读取 %s 失败：%v", p, err))
		return nil, err
	}
	if !m.Valid {
		a.log("warn", fmt.Sprintf("%s：%s", m.FileName, m.Error))
	} else {
		a.log("ok", fmt.Sprintf("%s %s（%s）", m.Name, m.Version, m.FileName))
	}
	return m, nil
}

// DetectRootManager 探测设备上的 root 方案与模块管理器。
func (a *App) DetectRootManager(serial string) (magisk.Manager, error) {
	if err := a.requireAdb(); err != nil {
		return magisk.Manager{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	m := magisk.DetectManager(ctx, a.toolset().AdbPath, serial)
	switch {
	case !m.RootOK:
		a.log("warn", m.Note)
	case m.Kind == "unknown":
		a.log("warn", m.Note)
	default:
		a.log("ok", m.Note)
	}
	return m, nil
}

// InstallModules 批量安装模块。
func (a *App) InstallModules(serial string, mods []magisk.Module) ([]magisk.InstallResult, error) {
	if err := a.requireAdb(); err != nil {
		return nil, err
	}
	ctx, cancel := a.opCtx()
	defer cancel()

	mgr := magisk.DetectManager(ctx, a.toolset().AdbPath, serial)
	if !mgr.RootOK {
		return nil, fmt.Errorf("设备没有 root 权限，无法安装模块：%s", mgr.Note)
	}

	a.log("info", fmt.Sprintf("开始安装 %d 个模块（%s）", len(mods), mgr.Kind))
	res, err := magisk.InstallBatch(ctx, a.toolset().AdbPath, serial, mgr, mods, func(line string) {
		a.log("info", line)
	})
	if err != nil {
		a.log("err", err.Error())
		return res, err
	}

	ok := 0
	for _, r := range res {
		if r.OK {
			ok++
		} else {
			a.log("err", fmt.Sprintf("%s：%s", r.Module.FileName, r.Error))
		}
	}
	a.log("ok", fmt.Sprintf("安装结束：%d/%d 成功，重启后生效", ok, len(res)))
	return res, nil
}
