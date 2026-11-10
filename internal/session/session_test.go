package session

import (
	"sync"
	"testing"
	"time"

	"droidforge/internal/platform"
	"droidforge/internal/store"
)

// 这些测试不依赖 GUI 与 cgo，CI 里可直接用 -race 跑。

func TestStateSnapshotIsolation(t *testing.T) {
	s := NewState(store.Config{OutputDir: "/original"}, platform.Tools{})

	snap := s.Config()
	snap.OutputDir = "/mutated"

	if got := s.Config().OutputDir; got != "/original" {
		t.Fatalf("Config() 应返回副本，内部值被外部改写：%s", got)
	}
}

func TestStateUpdate(t *testing.T) {
	s := NewState(store.Config{}, platform.Tools{})

	got := s.Update(func(c *store.Config) {
		c.CacheDir = "/cache"
		c.CpuIntervalMS = 800
	})

	if got.CacheDir != "/cache" || got.CpuIntervalMS != 800 {
		t.Fatalf("Update 返回值不正确：%+v", got)
	}
	if s.Config().CacheDir != "/cache" {
		t.Fatal("Update 修改未落到内部状态")
	}
}

func TestStateTools(t *testing.T) {
	s := NewState(store.Config{}, platform.Tools{Ready: false})

	s.SetTools(platform.Tools{Ready: true, AdbPath: "/usr/bin/adb"})

	if got := s.Tools(); !got.Ready || got.AdbPath != "/usr/bin/adb" {
		t.Fatalf("工具链状态未更新：%+v", got)
	}
}

func TestOpsBeginCancelsPrevious(t *testing.T) {
	var o Ops

	ctx1, release1 := o.Begin()
	defer release1()

	ctx2, release2 := o.Begin()
	defer release2()

	select {
	case <-ctx1.Done():
	case <-time.After(time.Second):
		t.Fatal("启动新任务后，旧任务的 context 应被取消")
	}
	if err := ctx2.Err(); err != nil {
		t.Fatalf("新任务不应被取消：%v", err)
	}
}

func TestOpsReleaseDoesNotClearNewer(t *testing.T) {
	var o Ops

	_, release1 := o.Begin()
	ctx2, _ := o.Begin()

	release1() // 旧任务先收尾

	if err := ctx2.Err(); err != nil {
		t.Fatalf("旧任务收尾不应影响新任务：%v", err)
	}
	if !o.Cancel() {
		t.Fatal("旧任务收尾时不应清空新任务的登记")
	}
}

func TestOpsCancel(t *testing.T) {
	var o Ops

	ctx, release := o.Begin()
	defer release()

	if !o.Cancel() {
		t.Fatal("存在进行中任务时 Cancel 应返回 true")
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("Cancel 应取消当前任务")
	}

	if o.Cancel() {
		t.Fatal("已取消后再次 Cancel 应返回 false")
	}
}

// TestOpsConcurrent 在 -race 下验证 Begin / Cancel / release 交错无竞争。
func TestOpsConcurrent(t *testing.T) {
	var o Ops
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, release := o.Begin()
			release()
		}()
		go func() {
			defer wg.Done()
			o.Cancel()
		}()
	}
	wg.Wait()
}

// TestStateConcurrent 在 -race 下验证读写配置与工具链无竞争。
func TestStateConcurrent(t *testing.T) {
	s := NewState(store.Config{OutputDir: "/out"}, platform.Tools{})
	var wg sync.WaitGroup

	for i := 0; i < 60; i++ {
		wg.Add(3)
		go func(i int) {
			defer wg.Done()
			s.Update(func(c *store.Config) { c.CpuIntervalMS = 500 + i })
		}(i)
		go func() {
			defer wg.Done()
			_ = s.Config()
		}()
		go func() {
			defer wg.Done()
			s.SetTools(platform.Tools{AdbPath: "/adb"})
			_ = s.Tools()
		}()
	}
	wg.Wait()

	if got := s.Config().CpuIntervalMS; got < 500 || got >= 560 {
		t.Fatalf("配置写入异常：CpuIntervalMS = %d", got)
	}
}

// TestOpsReleaseIdempotent 释放函数重复调用不应 panic 或误清状态。
func TestOpsReleaseIdempotent(t *testing.T) {
	var o Ops

	_, release := o.Begin()
	release()
	release()

	ctx2, release2 := o.Begin()
	defer release2()

	if err := ctx2.Err(); err != nil {
		t.Fatalf("重复释放影响了后续任务：%v", err)
	}
}

// TestOpsContextNotLeaked 验证未释放的任务交给下一次 Begin 后会被取消。
func TestOpsContextNotLeaked(t *testing.T) {
	var o Ops

	ctx1, _ := o.Begin() // 故意不释放
	ctx2, release2 := o.Begin()
	defer release2()

	if ctx1.Err() == nil {
		t.Fatal("被接管的任务应被取消")
	}
	if ctx2.Err() != nil {
		t.Fatalf("当前任务不应被取消：%v", ctx2.Err())
	}
}
