package root

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"droidforge/internal/adb"
)

// buildAPK 生成一个最小的合成 APK，用于验证补丁工具的解包规则。
func buildAPK(t *testing.T, entries map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.apk")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	for name, body := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSolutionsIntegrity(t *testing.T) {
	list := Solutions()
	if len(list) != 6 {
		t.Fatalf("内置方案数量应为 6，实际 %d", len(list))
	}

	seen := map[string]bool{}
	validFamily := map[string]bool{"magisk": true, "kernelsu": true, "apatch": true}

	for _, s := range list {
		if s.ID == "" || s.Name == "" || s.Repo == "" || s.Family == "" {
			t.Fatalf("方案字段不完整：%+v", s)
		}
		if seen[s.ID] {
			t.Fatalf("方案 ID 重复：%s", s.ID)
		}
		seen[s.ID] = true

		if !validFamily[s.Family] {
			t.Fatalf("方案 %s 的 family 非法：%s", s.ID, s.Family)
		}
		if _, _, err := patchCommandSafe(s.Family); err != nil {
			t.Fatalf("方案 %s 没有对应的补丁命令：%v", s.ID, err)
		}
	}
}

func TestFindSolution(t *testing.T) {
	if s, ok := FindSolution("magisk"); !ok || s.Family != "magisk" {
		t.Fatalf("未找到 magisk 方案：%+v %v", s, ok)
	}
	if _, ok := FindSolution("does-not-exist"); ok {
		t.Fatal("不存在的方案不应命中")
	}
}

func TestPatchCommand(t *testing.T) {
	cases := []struct{ family, wantOut string }{
		{"magisk", "new-boot.img"},
		{"kernelsu", "patched.img"},
		{"apatch", "patched.img"},
	}
	for _, c := range cases {
		cmd, out := patchCommand(c.family)
		if cmd == "" || out != c.wantOut {
			t.Fatalf("%s 补丁命令异常：cmd=%q out=%q", c.family, cmd, out)
		}
	}

	// 三个方案的命令里都必须真的带上输入镜像，否则补丁会在设备上失败。
	if !strings.Contains(mustCmd("magisk"), "boot.img") {
		t.Fatal("magisk 补丁命令缺少输入镜像")
	}
	if !strings.Contains(mustCmd("kernelsu"), "boot.img") {
		t.Fatal("kernelsu 补丁命令缺少输入镜像")
	}
	if !strings.Contains(mustCmd("apatch"), "boot.img") {
		t.Fatal("apatch 补丁命令缺少输入镜像")
	}

	if cmd, out := patchCommand("nonsense"); cmd != "" || out != "" {
		t.Fatalf("未知方案应返回空命令：%q %q", cmd, out)
	}
}

func mustCmd(family string) string {
	cmd, _ := patchCommand(family)
	return cmd
}

// patchCommandSafe 判断某 family 是否被支持。
func patchCommandSafe(family string) (string, string, error) {
	cmd, out := patchCommand(family)
	if cmd == "" {
		return "", "", os.ErrInvalid
	}
	return cmd, out, nil
}

func TestPrepareToolFilesMagisk(t *testing.T) {
	apk := buildAPK(t, map[string]string{
		"lib/arm64-v8a/libmagiskboot.so": "magiskboot-binary",
		"lib/arm64-v8a/libbusybox.so":    "busybox-binary",
		"assets/boot_patch.sh":           "#!/system/bin/sh",
		"assets/util_functions.sh":       "util",
		"assets/stub.apk":                "stub",
		"classes.dex":                    "ignored",
	})

	outDir := t.TempDir()
	files, err := prepareToolFiles(apk, "magisk", outDir)
	if err != nil {
		t.Fatalf("解包失败：%v", err)
	}

	names := map[string]bool{}
	for _, f := range files {
		names[filepath.Base(f)] = true
	}
	for _, want := range []string{"magiskboot", "busybox", "boot_patch.sh", "util_functions.sh", "stub.apk"} {
		if !names[want] {
			t.Fatalf("缺少解出的文件 %s，实际：%v", want, names)
		}
	}
	if names["libmagiskboot"] || names["magiskboot.so"] {
		t.Fatalf("二进制命名未去掉 lib/.so 前后缀：%v", names)
	}

	// 内容必须完整复制，不能是空文件
	body, err := os.ReadFile(filepath.Join(outDir, "magiskboot"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "magiskboot-binary" {
		t.Fatalf("文件内容不正确：%q", body)
	}
}

func TestPrepareToolFilesKernelSU(t *testing.T) {
	apk := buildAPK(t, map[string]string{
		"lib/arm64-v8a/libksud.so": "ksud-binary",
	})

	files, err := prepareToolFiles(apk, "kernelsu", t.TempDir())
	if err != nil {
		t.Fatalf("解包失败：%v", err)
	}
	if len(files) != 1 || filepath.Base(files[0]) != "ksud" {
		t.Fatalf("KernelSU 应只解出 ksud，实际：%v", files)
	}
}

func TestPrepareToolFilesAPatch(t *testing.T) {
	apk := buildAPK(t, map[string]string{
		"lib/arm64-v8a/libapd.so":    "apd-binary",
		"lib/arm64-v8a/libkpatch.so": "kpatch-binary",
	})

	files, err := prepareToolFiles(apk, "apatch", t.TempDir())
	if err != nil {
		t.Fatalf("解包失败：%v", err)
	}
	names := map[string]bool{}
	for _, f := range files {
		names[filepath.Base(f)] = true
	}
	if !names["apd"] || !names["kpatch"] {
		t.Fatalf("APatch 应解出 apd 与 kpatch，实际：%v", names)
	}
}

// TestPrepareToolFilesRejectsWrongPackage 传入普通 APK 时必须明确报错，
// 而不是推一堆用不了的文件到设备上。
func TestPrepareToolFilesRejectsWrongPackage(t *testing.T) {
	apk := buildAPK(t, map[string]string{
		"classes.dex":             "dex",
		"AndroidManifest.xml":     "xml",
		"lib/arm64-v8a/libfoo.so": "foo",
	})

	if _, err := prepareToolFiles(apk, "magisk", t.TempDir()); err == nil {
		t.Fatal("不含 magiskboot 的 APK 应报错")
	}
	if _, err := prepareToolFiles(apk, "kernelsu", t.TempDir()); err == nil {
		t.Fatal("不含 ksud 的 APK 应报错")
	}
	if _, err := prepareToolFiles(apk, "apatch", t.TempDir()); err == nil {
		t.Fatal("不含 apd 的 APK 应报错")
	}
}

func TestPrepareToolFilesUnknownFamily(t *testing.T) {
	apk := buildAPK(t, map[string]string{"a": "b"})
	if _, err := prepareToolFiles(apk, "unknown", t.TempDir()); err == nil {
		t.Fatal("未知方案族应报错")
	}
}

func TestPrepareToolFilesMissingAPK(t *testing.T) {
	if _, err := prepareToolFiles(filepath.Join(t.TempDir(), "nope.apk"), "magisk", t.TempDir()); err == nil {
		t.Fatal("APK 不存在时应报错")
	}
}

// TestInstallValidatesInput 参数校验必须发生在接触设备之前。
func TestInstallValidatesInput(t *testing.T) {
	client := adb.New("")
	ctx := context.Background()

	if _, err := Install(ctx, client, InstallOptions{Solution: "nope"}, nil); err == nil {
		t.Fatal("未知方案应报错")
	}

	_, err := Install(ctx, client, InstallOptions{Solution: "magisk"}, nil)
	if err == nil || !strings.Contains(err.Error(), "boot") {
		t.Fatalf("未选镜像时应提示选择 boot 镜像，实际：%v", err)
	}
}
