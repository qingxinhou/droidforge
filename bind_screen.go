package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"droidforge/internal/screen"
)

// ---------------------------------------------------------------------------
// 投屏（scrcpy）
// ---------------------------------------------------------------------------

// DetectScrcpy 查找本机的 scrcpy。
func (a *App) DetectScrcpy() screen.Binary {
	b := screen.Detect(a.config().CustomScrcpy)
	if !b.Found {
		a.log("warn", b.Hint)
		return b
	}
	if b.Version != "" {
		a.log("ok", "scrcpy："+b.Version)
	} else if b.Hint != "" {
		a.log("warn", b.Hint)
	}
	return b
}

// StartScreen 启动投屏。返回一句状态文案，前端用来更新按钮。
func (a *App) StartScreen(opt screen.Options) (string, error) {
	a.screenMu.Lock()
	defer a.screenMu.Unlock()

	if a.screenProc != nil && a.screenProc.Running() {
		return "", errors.New("投屏已经在运行，先停止再启动新的")
	}

	b := screen.Detect(a.config().CustomScrcpy)
	if !b.Found {
		return "", errors.New(b.Hint)
	}
	if opt.AdbPath == "" {
		opt.AdbPath = a.toolset().AdbPath
	}
	if opt.Serial == "" {
		opt.Serial = a.config().LastSerial
	}
	if opt.Record != "" {
		if err := os.MkdirAll(filepath.Dir(opt.Record), 0o755); err != nil {
			return "", fmt.Errorf("录制目录不可用：%w", err)
		}
	}

	p, err := screen.Start(b.Path, opt)
	if err != nil {
		a.log("err", err.Error())
		return "", err
	}
	a.screenProc = p
	label := opt.Serial
	if label == "" {
		label = "当前设备"
	}
	a.log("ok", "投屏已启动："+label)
	if opt.Record != "" {
		a.log("info", "同时录制到 "+opt.Record)
	}
	return "运行中", nil
}

// StopScreen 停止投屏。
func (a *App) StopScreen() error {
	a.screenMu.Lock()
	defer a.screenMu.Unlock()
	if a.screenProc == nil {
		return nil
	}
	err := a.screenProc.Stop()
	a.screenProc = nil
	if err != nil {
		a.log("err", "停止投屏失败："+err.Error())
		return err
	}
	a.log("info", "投屏已停止")
	return nil
}

// ScreenStatus 查询投屏进程状态。
type ScreenStatus struct {
	Running bool   `json:"running"`
	Tail    string `json:"tail"`
}

// ScreenStatus 返回当前投屏状态与最近输出。
func (a *App) ScreenStatus() ScreenStatus {
	a.screenMu.Lock()
	defer a.screenMu.Unlock()
	if a.screenProc == nil || !a.screenProc.Running() {
		return ScreenStatus{}
	}
	return ScreenStatus{Running: true, Tail: a.screenProc.Tail(8)}
}
