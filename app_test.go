package main

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"droidforge/internal/store"
)

// 这些测试验证 App 对 session 的委托是否正确。
// 锁本身的正确性由 internal/session 的 -race 测试覆盖。

func TestAppOpCtxCancelsPrevious(t *testing.T) {
	a := NewApp()

	ctx1, release1 := a.opCtx()
	defer release1()

	ctx2, release2 := a.opCtx()
	defer release2()

	select {
	case <-ctx1.Done():
	case <-time.After(time.Second):
		t.Fatal("启动新操作后，旧操作的 context 应被取消")
	}
	if err := ctx2.Err(); err != nil {
		t.Fatalf("新操作不应被取消：%v", err)
	}
}

func TestAppCancelOperation(t *testing.T) {
	a := NewApp()

	ctx, release := a.opCtx()
	defer release()

	a.CancelOperation()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("CancelOperation 应取消当前操作")
	}

	a.CancelOperation() // 无进行中操作时不应 panic
}

func TestAppReleaseDoesNotClearNewer(t *testing.T) {
	a := NewApp()

	_, release1 := a.opCtx()
	ctx2, release2 := a.opCtx()
	defer release2()

	release1() // 旧操作先收尾

	if err := ctx2.Err(); err != nil {
		t.Fatalf("旧操作收尾不应影响新操作：%v", err)
	}
}

func TestAppConcurrentStateAccess(t *testing.T) {
	a := NewApp()

	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(5)
		go func() {
			defer wg.Done()
			_, release := a.opCtx()
			release()
		}()
		go func() {
			defer wg.Done()
			a.CancelOperation()
		}()
		go func() {
			defer wg.Done()
			_ = a.config()
		}()
		go func() {
			defer wg.Done()
			_ = a.toolset()
		}()
		go func(i int) {
			defer wg.Done()
			a.updateConfig(func(c *store.Config) { c.CpuIntervalMS = 500 + i })
		}(i)
	}
	wg.Wait()

	if got := a.config().CpuIntervalMS; got < 500 || got >= 540 {
		t.Fatalf("配置写入丢失：CpuIntervalMS = %d", got)
	}
}

func TestAppConfigSnapshotIsolation(t *testing.T) {
	a := NewApp()
	a.updateConfig(func(c *store.Config) { c.OutputDir = "/original" })

	snap := a.config()
	snap.OutputDir = "/mutated"

	if got := a.config().OutputDir; got != "/original" {
		t.Fatalf("config() 应返回副本，内部值被外部改写：%s", got)
	}
}

// TestAppLogWithoutRuntime 验证运行时未就绪时写日志静默丢弃而不 panic，
// 因为后台子进程回调可能在 startup 之前或 shutdown 之后触发。
func TestAppLogWithoutRuntime(t *testing.T) {
	a := NewApp()

	var wg sync.WaitGroup
	var count atomic.Int64
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a.log("out", "line")
			a.progress("p", 10, false)
			count.Add(1)
		}()
	}
	wg.Wait()

	if count.Load() != 50 {
		t.Fatalf("回调次数异常：%d", count.Load())
	}
}

func TestAppShutdownSafe(t *testing.T) {
	a := NewApp()
	a.shutdown(context.Background())
	a.shutdown(context.Background())

	ctx, release := a.opCtx()
	defer release()
	a.shutdown(context.Background())

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("shutdown 应取消进行中的操作")
	}
}
