package flashscript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"droidforge/internal/rom"
)

func sampleSteps(dir string) []rom.FlashStep {
	return []rom.FlashStep{
		{Partition: "boot", Image: filepath.Join(dir, "boot.img"), SizeBytes: 1},
		{Partition: "vbmeta", Image: filepath.Join(dir, "vbmeta.img"), SizeBytes: 1},
		{Partition: "super", Image: filepath.Join(dir, "super.img"), SizeBytes: 1, NeedsFastbootd: true},
		{Partition: "userdata", Image: filepath.Join(dir, "userdata.img"), SizeBytes: 1},
	}
}

func TestRenderBat(t *testing.T) {
	dir := t.TempDir()
	got := Render(sampleSteps(dir), dir, Options{Slot: "b", Reboot: true}, "bat")

	if !strings.Contains(got, "@echo off") || !strings.Contains(got, "chcp 65001") {
		t.Fatal("bat 应包含编码设置")
	}
	if !strings.Contains(got, "fastboot flash boot_b boot.img") {
		t.Fatalf("槽位后缀未生效：\n%s", got)
	}
	if !strings.Contains(got, "fastboot flash vbmeta_b vbmeta.img") {
		t.Fatal("vbmeta 应带槽位")
	}
	if !strings.Contains(got, "fastboot flash userdata userdata.img") {
		t.Fatal("userdata 不应带槽位")
	}
	if strings.Contains(got, "super_b") {
		t.Fatal("super 不应带槽位后缀")
	}
	if strings.Count(got, "fastboot reboot fastboot") != 1 {
		t.Fatal("进入 fastbootd 只应出现一次")
	}
	if !strings.Contains(got, "timeout /t 8") {
		t.Fatal("bat 应用 timeout 等待")
	}
	if !strings.Contains(got, "fastboot reboot") {
		t.Fatal("应包含末尾重启")
	}
	if !strings.Contains(got, ":fail") || strings.Count(got, "if errorlevel 1 goto :fail") < 3 {
		t.Fatal("每一步都应有错误中断")
	}
	if !strings.Contains(got, "\r\n") {
		t.Fatal("bat 应使用 CRLF 换行")
	}
}

func TestRenderSh(t *testing.T) {
	dir := t.TempDir()
	got := Render(sampleSteps(dir), dir, Options{}, "sh")
	if !strings.HasPrefix(got, "#!/usr/bin/env bash") {
		t.Fatal("sh 缺少 shebang")
	}
	if !strings.Contains(got, "set -e") {
		t.Fatal("sh 应启用 set -e")
	}
	if !strings.Contains(got, "sleep 8") {
		t.Fatal("sh 应用 sleep 等待 fastbootd")
	}
	if strings.Contains(got, "rem ") {
		t.Fatal("sh 里不应出现 batch 注释")
	}
	if !strings.Contains(got, "fastboot flash super super.img") {
		t.Fatal("缺少 super 刷写命令")
	}
}

func TestRenderWipeAndTxt(t *testing.T) {
	dir := t.TempDir()
	got := Render(sampleSteps(dir), dir, Options{Wipe: true}, "txt")
	if !strings.Contains(got, "fastboot -w") {
		t.Fatal("wipe 选项未生效")
	}
	if strings.Contains(got, "@echo off") || strings.Contains(got, "set -e") {
		t.Fatal("txt 不应包含脚本头")
	}
	if !strings.Contains(got, "fastboot flash boot boot.img") {
		t.Fatal("txt 缺少刷写命令")
	}
}

func TestGenerateFiles(t *testing.T) {
	dir := t.TempDir()
	imgDir := t.TempDir()
	for _, n := range []string{"boot.img", "super.img"} {
		if err := os.WriteFile(filepath.Join(imgDir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	steps := []rom.FlashStep{
		{Partition: "boot", Image: filepath.Join(imgDir, "boot.img")},
		{Partition: "super", Image: filepath.Join(imgDir, "super.img"), NeedsFastbootd: true},
	}
	files, err := Generate(steps, dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("应生成 3 个文件，实际 %d", len(files))
	}
	for _, f := range files {
		if fi, err := os.Stat(f); err != nil || fi.Size() == 0 {
			t.Fatalf("%s 生成失败", f)
		}
	}
}

func TestSlotName(t *testing.T) {
	cases := []struct{ in, slot, want string }{
		{"boot", "a", "boot_a"},
		{"boot_a", "a", "boot_a"},
		{"super", "b", "super"},
		{"userdata", "a", "userdata"},
		{"boot", "", "boot"},
	}
	for _, c := range cases {
		if got := SlotName(c.in, c.slot); got != c.want {
			t.Errorf("SlotName(%q,%q)=%q，期望 %q", c.in, c.slot, got, c.want)
		}
	}
}
