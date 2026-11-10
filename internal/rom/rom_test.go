package rom

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestPartitionFromFilename(t *testing.T) {
	cases := map[string]string{
		"boot.img":          "boot",
		"init_boot.img":     "init_boot",
		"vendor_boot_a.img": "vendor_boot",
		"system.img.lz4":    "system",
		"dtbo.bin":          "dtbo",
		"super.img":         "super",
	}
	for in, want := range cases {
		if got := PartitionFromFilename(in); got != want {
			t.Errorf("PartitionFromFilename(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestInspectAndExtractZipImages(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "images.zip")

	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, content := range map[string][]byte{
		"boot.img":   {1, 2, 3, 4},
		"dtbo.img":   {5, 6, 7},
		"readme.txt": []byte("hello"),
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()

	info, err := Inspect(zipPath)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if info.Kind != "fastboot" {
		t.Errorf("kind = %q, want fastboot", info.Kind)
	}
	if len(info.Partitions) != 2 {
		t.Errorf("partitions = %v", info.Partitions)
	}

	outDir := t.TempDir()
	files, err := Extract(zipPath, []string{"boot"}, outDir, nil)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("files = %v", files)
	}
	got, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 || got[0] != 1 {
		t.Errorf("unexpected content: %v", got)
	}
}

func TestBuildFlashPlan(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"system.img", "boot.img", "dtbo.img", "userdata.img", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte{0}, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	steps, err := BuildFlashPlan(dir)
	if err != nil {
		t.Fatalf("BuildFlashPlan: %v", err)
	}

	var order []string
	for _, s := range steps {
		order = append(order, s.Partition)
		if s.Partition == "userdata" {
			t.Error("userdata should be excluded by default")
		}
	}
	if len(order) != 3 {
		t.Fatalf("order = %v, want 3 steps", order)
	}
	// boot 应排在 system 之前
	if order[0] != "boot" {
		t.Errorf("first step = %q, want boot", order[0])
	}
	for _, s := range steps {
		if s.Partition == "system" && !s.NeedsFastbootd {
			t.Error("system should require fastbootd")
		}
	}
}

// TestBuildFlashPlanDynamicLast 保证动态分区排在最后：
// 没登记在 flashOrder 里的物理分区（z开头的各种 bootloader 小分区）
// 也必须排在 super/system 前面，否则脚本会在 fastbootd 里刷它们。
func TestBuildFlashPlanDynamicLast(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"system.img", "super.img", "boot.img", "zap.img", "xbl.img"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte{0}, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	steps, err := BuildFlashPlan(dir)
	if err != nil {
		t.Fatal(err)
	}
	seenDynamic := false
	for _, s := range steps {
		if s.NeedsFastbootd {
			seenDynamic = true
			continue
		}
		if seenDynamic {
			t.Fatalf("物理分区 %s 排在了动态分区之后（%v）", s.Partition, planOrder(steps))
		}
	}
}

func planOrder(steps []FlashStep) []string {
	out := make([]string, 0, len(steps))
	for _, s := range steps {
		out = append(out, s.Partition)
	}
	return out
}

func TestBuildFlashPlanEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildFlashPlan(dir); err == nil {
		t.Error("expected error for empty plan")
	}
}
