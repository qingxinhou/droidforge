package fastboot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"droidforge/internal/runner"
)

// loadFixture 读取真机抓下来的原始输出。
func loadFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("读夹具失败: %v", err)
	}
	return string(b)
}

// TestParseVarsRealBootloader 用真机夹具锁住解析规则。
//
// 夹具来自 Redmi K60 至尊版（rembrandt）bootloader 下的
// `fastboot getvar all`，原样保留 CRLF 和 `(bootloader)\t` 前缀。
func TestParseVarsRealBootloader(t *testing.T) {
	vars := ParseVars(loadFixture(t, "getvar_all_bootloader.txt"))

	cases := map[string]string{
		"product":                 "rembrandt",
		"current-slot":            "a",
		"is-userspace":            "no",
		"unlocked":                "yes",
		"max-download-size":       "0x10000000",
		"version-bootloader":      "rembrandt_a1e1c0d97_202311281507",
		"partition-size:super":    "220000000",
		"partition-size:userdata": "73ff7f8000",
		// 键自身含冒号：必须用最后一个 ": " 切分，否则值会变成 "boot_a: 4000000"
		"partition-size:boot_a": "4000000",
		"slot-retry-count:a":    "1",
		"slot-successful:b":     "no",
	}
	for k, want := range cases {
		got, ok := vars[k]
		if !ok {
			t.Errorf("缺少变量 %q", k)
			continue
		}
		if got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}

	// 尾部噪声不能进变量表
	for _, junk := range []string{"all", "Finished. Total time", "all: Done!!"} {
		if _, ok := vars[junk]; ok {
			t.Errorf("噪声行被当成变量: %q = %q", junk, vars[junk])
		}
	}
}

// TestPartitionsFromRealVars 验证真机变量表能还原成正确容量的分区列表。
func TestPartitionsFromRealVars(t *testing.T) {
	vars := ParseVars(loadFixture(t, "getvar_all_bootloader.txt"))
	ps := PartitionsFromVars(vars)

	idx := map[string]Partition{}
	for _, p := range ps {
		idx[p.Name] = p
	}

	// 真机实测值（裸十六进制，不带 0x）
	want := map[string]int64{
		"boot_a":        0x4000000, // 64 MiB
		"boot_b":        0x4000000,
		"super":         0x220000000,  // 8.5 GiB
		"userdata":      0x73ff7f8000, // 464 GiB
		"vbmeta_a":      0x800000,
		"dtbo_a":        0x2000000,
		"vendor_boot_b": 0x4000000,
		"lk_a":          0x800000,
		"md1img_a":      0xc800000,
	}
	for name, size := range want {
		p, ok := idx[name]
		if !ok {
			t.Errorf("缺少分区 %q", name)
			continue
		}
		if p.SizeBytes != size {
			t.Errorf("%s 容量 = %d, want %d", name, p.SizeBytes, size)
		}
	}

	// 槽位拆分
	if p := idx["boot_a"]; p.Base != "boot" || p.Slot != "a" {
		t.Errorf("boot_a 拆成 base=%q slot=%q", p.Base, p.Slot)
	}
	if p := idx["userdata"]; p.Slot != "" || p.Base != "userdata" {
		t.Errorf("userdata 不该有槽位: base=%q slot=%q", p.Base, p.Slot)
	}
	if p := idx["userdata"]; p.FSType != "f2fs" {
		t.Errorf("userdata fsType = %q, want f2fs", p.FSType)
	}

	// raw 不该被当成文件系统
	if p := idx["boot_a"]; p.FSType != "" {
		t.Errorf("boot_a fsType = %q, want 空（raw 不算文件系统）", p.FSType)
	}

	// a/b 必须相邻
	seen := map[string]int{}
	for i, p := range ps {
		if prev, ok := seen[p.Base]; ok && i-prev != 1 {
			t.Errorf("%s 的两个槽位不相邻（%d 与 %d）", p.Base, prev, i)
		}
		seen[p.Base] = i
	}
}

// TestParseVarsSingleVarForm 单变量查询没有 (bootloader) 前缀。
func TestParseVarsSingleVarForm(t *testing.T) {
	out := "partition-size:boot_a: 4000000\r\nFinished. Total time: 0.010s\r\n"
	vars := ParseVars(out)
	if got := vars["partition-size:boot_a"]; got != "4000000" {
		t.Errorf("= %q, want 4000000", got)
	}
	if _, ok := vars["Finished. Total time"]; ok {
		t.Error("Finished 行不该进变量表")
	}
}

// TestParseHexSize 覆盖 bootloader 的三种容量写法。
func TestParseHexSize(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"4000000", 0x4000000, true},
		{"0x10000000", 0x10000000, true},
		{"0X800", 0x800, true},
		{"73ff7f8000", 0x73ff7f8000, true},
		{"", 0, false},
		{"-", 0, false},
		{"zz", 0, false},
	}
	for _, c := range cases {
		got, ok := parseHexSize(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("parseHexSize(%q) = (%d, %v), want (%d, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

// TestSplitSlot 只有 has-slot 明确为 yes 时才拆槽位，
// 避免把名字里真带下划线的分区（如 foo_b）误拆。
func TestSplitSlot(t *testing.T) {
	hasSlot := map[string]bool{"boot": true, "vbmeta": true, "super": false}
	cases := []struct{ in, base, slot string }{
		{"boot_a", "boot", "a"},
		{"boot_b", "boot", "b"},
		{"super", "super", ""},
		{"foo_b", "foo_b", ""}, // 不在 has-slot 表里，不动
		{"ab", "ab", ""},
	}
	for _, c := range cases {
		b, s := splitSlot(c.in, hasSlot)
		if b != c.base || s != c.slot {
			t.Errorf("splitSlot(%q) = (%q, %q), want (%q, %q)", c.in, b, s, c.base, c.slot)
		}
	}
}

// TestParseVarsEmptyValue 真机：不存在的变量会「键照常回显、值为空」。
// 解析后键必须存在、值为空串，不能当成噪声丢掉。
func TestParseVarsEmptyValue(t *testing.T) {
	out := "partition-size:not_a_real_partition: \r\nFinished. Total time: 0.011s\r\n"
	vars := ParseVars(out)
	got, ok := vars["partition-size:not_a_real_partition"]
	if !ok {
		t.Fatal("空值变量被丢掉了")
	}
	if got != "" {
		t.Errorf("= %q, want 空串", got)
	}
	// 空容量进 PartitionsFromVars 时，分区不该出现（没有 size 也没有 type）
	if ps := PartitionsFromVars(vars); len(ps) != 0 {
		t.Errorf("空值变量产出了 %d 个分区: %+v", len(ps), ps)
	}
}

// TestFailureText 变量不存在时 fastboot 退出码仍是 0，
// 必须靠文本识别，否则调用方拿到空串还以为成功。
func TestFailureText(t *testing.T) {
	out := "getvar:partition-size:nope FAILED (remote: 'GetVar Variable not found')\r\nFinished. Total time: 0.002s\r\n"
	msg, ok := failureText(out)
	if !ok {
		t.Fatal("未识别到失败")
	}
	if msg == "" {
		t.Error("失败原因不该为空")
	}

	if _, ok := failureText("product: rembrandt\r\nFinished. Total time: 0.010s\r\n"); ok {
		t.Error("正常输出被判成失败")
	}
}

// TestFlashErrorFromRealFailure 用真机刷入不存在分区的输出，锁住错误信息。
//
// 真机输出（stdout 空、全部走 stderr）：
//
//	Warning: skip copying ... avb footer (...)
//	Sending '...' (1024 KB)  OKAY
//	Writing '...'            FAILED (remote: 'partition does not exist')
//	fastboot: error: Command failed
//
// 关键点：错误信息里只该出现失败原因，不能把进度噪声整段抛给 UI。
func TestFlashErrorFromRealFailure(t *testing.T) {
	out := loadFixture(t, "flash_failed.txt")
	res := &runner.Result{Stderr: out, ExitCode: 1}
	err := flashError(res, "刷入 boot_a")
	if err == nil {
		t.Fatal("失败输出没有产生 error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "partition does not exist") {
		t.Errorf("错误信息丢了真正原因: %q", msg)
	}
	if strings.Contains(msg, "Warning:") || strings.Contains(msg, "Sending") {
		t.Errorf("错误信息混进了进度噪声: %q", msg)
	}
	if len(msg) > 120 {
		t.Errorf("错误信息过长（%d 字）: %q", len(msg), msg)
	}
}

// TestFlashErrorSuccess 正常输出不该报错。
func TestFlashErrorSuccess(t *testing.T) {
	out := "Sending 'boot_a' (65536 KB)  OKAY [  1.4s]\r\nWriting 'boot_a'  OKAY [  0.3s]\r\nFinished. Total time: 1.8s\r\n"
	res := &runner.Result{Stderr: out, ExitCode: 0}
	if err := flashError(res, "刷入 boot_a"); err != nil {
		t.Errorf("成功的刷入被判成失败: %v", err)
	}
}

// TestFastbootdRealDump 用真机 fastbootd 的完整输出（424 行、106 个分区）验证：
//   - 冒号后无空格的格式（`partition-size:system_a:0x32EBA000`）也能解析；
//   - 容量带 0x 前缀时按十六进制读（bootloader 是裸十六进制，两者都得认）；
//   - is-logical 能逐分区标出动态分区，而不是一律当物理分区。
func TestFastbootdRealDump(t *testing.T) {
	vars := ParseVars(loadFixture(t, "getvar_all_fastbootd.txt"))

	if got := vars["is-userspace"]; got != "yes" {
		t.Fatalf("is-userspace = %q, want yes", got)
	}

	ps := PartitionsFromVars(vars)
	// 106 个带 partition-size + sda/sdb 只有 partition-type 没有容量 = 108
	if len(ps) != 108 {
		t.Errorf("解析出 %d 个分区，真机是 108 个", len(ps))
	}

	idx := map[string]Partition{}
	logical := 0
	for _, p := range ps {
		idx[p.Name] = p
		if p.Kind == KindLogical {
			logical++
		}
	}
	if logical != 16 {
		t.Errorf("逻辑分区 %d 个，真机 is-logical:yes 是 16 个", logical)
	}

	// 带 0x 前缀的容量
	want := map[string]int64{
		"system_a":  0x32EBA000,
		"vendor_a":  0x723C9000,
		"product_a": 0xF69BD000,
		"super":     0x220000000,
		"userdata":  0x73FF7F8000,
	}
	for name, size := range want {
		p, ok := idx[name]
		if !ok {
			t.Errorf("缺少分区 %q", name)
			continue
		}
		if p.SizeBytes != size {
			t.Errorf("%s 容量 = %#x, want %#x", name, p.SizeBytes, size)
		}
	}

	// system_a 是动态分区，boot_a 不是
	if p := idx["system_a"]; p.Kind != KindLogical {
		t.Errorf("system_a kind = %q, want logical", p.Kind)
	}
	if p := idx["boot_a"]; p.Kind != KindPhysical {
		t.Errorf("boot_a kind = %q, want physical", p.Kind)
	}

	// 真机事实：非活动槽位的逻辑分区确实报 0x0（super 里没给它们分配空间），
	// 这是「真实为 0」，不是「读不到」，必须原样保留成 0 而不是 -1。
	if p := idx["vendor_b"]; p.SizeBytes != 0 {
		t.Errorf("vendor_b 容量 = %d, want 0（真机报 0x0）", p.SizeBytes)
	}
	// sda/sdb 只有 partition-type、没有 partition-size → 容量未知，记 -1
	for _, name := range []string{"sda", "sdb"} {
		p, ok := idx[name]
		if !ok {
			t.Errorf("缺少块设备 %q", name)
			continue
		}
		if p.SizeBytes != -1 {
			t.Errorf("%s 容量 = %d, want -1（真机没报容量）", name, p.SizeBytes)
		}
		if p.FSType != "" {
			t.Errorf("%s fsType = %q, want 空（raw 不算文件系统）", name, p.FSType)
		}
	}
}
