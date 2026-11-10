package magisk

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeZip(t *testing.T, entries map[string]string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "module.zip")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
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
	f.Close()
	return p
}

func TestInspectModule(t *testing.T) {
	prop := "id=fakegps\nname=虚拟定位\nversion=v1.2.3\nversionCode=12\nauthor=someone\ndescription=测试用模块\n"
	p := writeZip(t, map[string]string{
		"module.prop":  prop,
		"system/bin/x": "bin",
	})
	m, err := Inspect(p)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Valid {
		t.Fatalf("应当识别为有效模块：%s", m.Error)
	}
	if m.ID != "fakegps" || m.Name != "虚拟定位" {
		t.Errorf("ID/名称解析错误：%+v", m)
	}
	if m.Version != "v1.2.3" || m.VersionCode != "12" || m.Author != "someone" {
		t.Errorf("版本/作者解析错误：%+v", m)
	}
}

func TestInspectNestedProp(t *testing.T) {
	// 有些模块把内容放在一级子目录里。
	p := writeZip(t, map[string]string{
		"fakegps/module.prop": "id=nested\nname=嵌套模块\nversion=1\n",
	})
	m, err := Inspect(p)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Valid || m.ID != "nested" {
		t.Errorf("子目录里的 module.prop 未被识别：%+v", m)
	}
}

func TestInspectRootPropWins(t *testing.T) {
	p := writeZip(t, map[string]string{
		"module.prop":     "id=root\nname=根目录\nversion=1\n",
		"sub/module.prop": "id=sub\nname=子目录\nversion=2\n",
	})
	m, _ := Inspect(p)
	if m.ID != "root" {
		t.Errorf("应当优先取根目录的 module.prop，实际 %q", m.ID)
	}
}

func TestInspectRejectsNonModule(t *testing.T) {
	p := writeZip(t, map[string]string{"boot.img": "not a module"})
	m, err := Inspect(p)
	if err != nil {
		t.Fatal(err)
	}
	if m.Valid {
		t.Error("普通 zip 不该被识别为模块")
	}
	if !strings.Contains(m.Error, "module.prop") {
		t.Errorf("错误信息应说明缺少 module.prop，实际 %q", m.Error)
	}
}

func TestInspectMissingID(t *testing.T) {
	p := writeZip(t, map[string]string{"module.prop": "name=无名\nversion=1\n"})
	m, _ := Inspect(p)
	if m.Valid {
		t.Error("缺少 id 的模块不该通过校验")
	}
}

func TestParseProp(t *testing.T) {
	kv := parseProp("# 注释\n\nid=a\nname = 带空格的名字 \nversion=1.0\n坏行\n")
	if kv["id"] != "a" {
		t.Errorf("id = %q", kv["id"])
	}
	if kv["name"] != "带空格的名字" {
		t.Errorf("name 未 trim：%q", kv["name"])
	}
	if _, ok := kv["坏行"]; ok {
		t.Error("没有等号的行不该入表")
	}
}

func TestSafeName(t *testing.T) {
	cases := map[string]string{
		"模块 v1.zip":    "___v1.zip",
		"normal_1.zip": "normal_1.zip",
		"a;b c.zip":    "a_b_c.zip",
		"":             "module.zip",
		"..":           "module.zip",
	}
	for in, want := range cases {
		if got := safeName(in); got != want {
			t.Errorf("safeName(%q) = %q，期望 %q", in, got, want)
		}
	}
}

func TestInstallCommand(t *testing.T) {
	if !strings.Contains(installCommand("magisk", "/tmp/a.zip"), "magisk --install-module") {
		t.Error("magisk 命令不正确")
	}
	if !strings.Contains(installCommand("kernelsu", "/tmp/a.zip"), "ksud module install") {
		t.Error("KernelSU 命令不正确")
	}
	if !strings.Contains(installCommand("apatch", "/tmp/a.zip"), "apd module install") {
		t.Error("APatch 命令不正确")
	}
}

func TestLooksLikeFailure(t *testing.T) {
	if looksLikeFailure("Installing module...\nDone") {
		t.Error("成功输出被误判为失败")
	}
	if !looksLikeFailure("Failed to install module") {
		t.Error("失败输出未被识别")
	}
	if !looksLikeFailure("su: permission denied") {
		t.Error("权限拒绝未被识别")
	}
}
