package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"droidforge/internal/adb"
	"droidforge/internal/fastboot"
	"droidforge/internal/platform"
	"droidforge/internal/runner"
	"droidforge/internal/session"
	"droidforge/internal/store"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App 是暴露给前端的应用对象。
//
// 并发模型：Wails 的每个绑定调用可能落在不同的 goroutine 上，子进程的输出
// 回调也在独立 goroutine 中执行。所有可变状态都收敛到 session 包里加锁的
// 对象上，App 自身只保留一个启动期写入的运行时 context。
type App struct {
	ctxMu sync.RWMutex
	ctx   context.Context

	state *session.State
	ops   *session.Ops
}

// NewApp 创建应用。
func NewApp() *App {
	return &App{
		state: session.NewState(store.Load(), platform.Tools{}),
		ops:   &session.Ops{},
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctxMu.Lock()
	a.ctx = ctx
	a.ctxMu.Unlock()

	cfg := store.Load()
	tools := platform.Detect(cfg.CustomAdb, cfg.CustomFastboot)
	a.state.SetConfig(cfg)
	a.state.SetTools(tools)

	a.log("info", "搞机工坊已启动")
	if !tools.Ready {
		a.log("warn", tools.Hint)
	} else {
		a.log("ok", fmt.Sprintf("平台工具就绪（%s）", tools.Source))
	}
}

func (a *App) shutdown(ctx context.Context) {
	a.ops.Cancel()
}

// ---------------------------------------------------------------------------
// 状态访问器（并发安全）
// ---------------------------------------------------------------------------

// appCtx 返回 Wails 运行时上下文，未初始化时返回 nil。
func (a *App) appCtx() context.Context {
	a.ctxMu.RLock()
	defer a.ctxMu.RUnlock()
	return a.ctx
}

// config 返回当前配置的快照，调用方可自由读取。
func (a *App) config() store.Config { return a.state.Config() }

// toolset 返回当前工具链探测结果。
func (a *App) toolset() platform.Tools { return a.state.Tools() }

// updateConfig 在写锁内修改配置，返回修改后的快照。
func (a *App) updateConfig(fn func(*store.Config)) store.Config { return a.state.Update(fn) }

// setTools 更新工具链探测结果。
func (a *App) setTools(t platform.Tools) { a.state.SetTools(t) }

// ---------------------------------------------------------------------------
// 基础设施
// ---------------------------------------------------------------------------

type LogEntry struct {
	Level string `json:"level"`
	Text  string `json:"text"`
	Time  string `json:"time"`
}

type Progress struct {
	Label   string `json:"label"`
	Percent int    `json:"percent"`
	Done    bool   `json:"done"`
}

func (a *App) log(level, text string) {
	ctx := a.appCtx()
	if ctx == nil {
		return
	}
	wailsruntime.EventsEmit(ctx, "log", LogEntry{
		Level: level,
		Text:  text,
		Time:  time.Now().Format("15:04:05"),
	})
}

func (a *App) progress(label string, percent int, done bool) {
	ctx := a.appCtx()
	if ctx == nil {
		return
	}
	wailsruntime.EventsEmit(ctx, "progress", Progress{Label: label, Percent: percent, Done: done})
}

func (a *App) lineHandler() runner.LineHandler {
	return func(line string, kind runner.LineKind) {
		level := "out"
		if kind == runner.Stderr {
			level = "err"
		}
		a.log(level, line)
	}
}

// opCtx 创建一个可被取消的操作上下文。
//
// 同一时刻只允许一个长任务处于「当前」状态：新任务会取消上一个。
func (a *App) opCtx() (context.Context, context.CancelFunc) {
	return a.ops.Begin()
}

// CancelOperation 中止当前长任务。
func (a *App) CancelOperation() {
	if a.ops.Cancel() {
		a.log("warn", "已请求中止当前操作")
	}
}

func (a *App) adbClient() *adb.Client     { return adb.New(a.toolset().AdbPath) }
func (a *App) fbClient() *fastboot.Client { return fastboot.New(a.toolset().FastbootPath) }

func (a *App) requireAdb() error {
	if a.toolset().AdbPath == "" {
		return fmt.Errorf("未找到 adb，请先在「设置」中下载或指定 platform-tools")
	}
	return nil
}

func (a *App) requireFastboot() error {
	if a.toolset().FastbootPath == "" {
		return fmt.Errorf("未找到 fastboot，请先在「设置」中下载或指定 platform-tools")
	}
	return nil
}

const appVersion = "1.6.0"
