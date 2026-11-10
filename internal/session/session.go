// Package session 管理应用的运行时状态：配置、工具链探测结果与长任务取消登记。
//
// Wails 的每个绑定方法都可能落在不同 goroutine 上，子进程输出回调也在独立
// goroutine 中执行。把可变状态收敛到这里，上层（app.go）就不必关心同步细节，
// 而且可以在无 GUI、无 cgo 的构建标签下做并发测试。
package session

import (
	"context"
	"sync"

	"droidforge/internal/platform"
	"droidforge/internal/store"
)

// State 保存配置与工具链状态，读写均加锁。
type State struct {
	mu    sync.RWMutex
	cfg   store.Config
	tools platform.Tools
}

// NewState 创建状态对象。
func NewState(cfg store.Config, tools platform.Tools) *State {
	return &State{cfg: cfg, tools: tools}
}

// Config 返回配置快照，调用方可自由读取而不必持锁。
func (s *State) Config() store.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// Tools 返回工具链快照。
func (s *State) Tools() platform.Tools {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tools
}

// Update 在写锁内修改配置，返回修改后的快照。
func (s *State) Update(fn func(*store.Config)) store.Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.cfg)
	return s.cfg
}

// SetConfig 整体替换配置。
func (s *State) SetConfig(cfg store.Config) {
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
}

// SetTools 更新工具链探测结果。
func (s *State) SetTools(t platform.Tools) {
	s.mu.Lock()
	s.tools = t
	s.mu.Unlock()
}

// Ops 保证同一时刻只有一个长任务处于「当前」状态。
//
// 新任务接管时会取消上一个；每个任务的释放函数只在它仍是当前任务时才清空
// 登记，避免先结束的旧任务把后启动的新任务一并抹掉。
type Ops struct {
	mu     sync.Mutex
	cancel context.CancelFunc
	seq    uint64
}

// Begin 开始一个新任务，返回其 context 与释放函数。
func (o *Ops) Begin() (context.Context, func()) {
	ctx, cancel := context.WithCancel(context.Background())

	o.mu.Lock()
	prev := o.cancel
	o.seq++
	seq := o.seq
	o.cancel = cancel
	o.mu.Unlock()

	// 先登记新 cancel 再取消旧的：顺序反过来会与并发调用交错，
	// 可能导致刚创建的新任务被立刻取消。
	if prev != nil {
		prev()
	}

	release := func() {
		cancel()
		o.mu.Lock()
		if o.seq == seq {
			o.cancel = nil
		}
		o.mu.Unlock()
	}
	return ctx, release
}

// Cancel 取消当前任务，返回是否确实存在进行中的任务。
func (o *Ops) Cancel() bool {
	o.mu.Lock()
	cancel := o.cancel
	o.cancel = nil
	o.mu.Unlock()

	if cancel == nil {
		return false
	}
	cancel()
	return true
}
