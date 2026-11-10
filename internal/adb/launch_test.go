package adb

import "testing"

// TestParseResolveActivity 用真机原始输出（含 CRLF）验证组件名提取。
//
// 真机输出：
//
//	priority=0 preferredOrder=0 match=0x108000 specificIndex=-1 isDefault=true
//	com.android.settings/.MainSettings
func TestParseResolveActivity(t *testing.T) {
	cases := []struct {
		name string
		out  string
		want string
		ok   bool
	}{
		{
			name: "系统应用（短组件名）",
			out:  "priority=0 preferredOrder=0 match=0x108000 specificIndex=-1 isDefault=true\r\ncom.android.settings/.MainSettings\r\n",
			want: "com.android.settings/.MainSettings",
			ok:   true,
		},
		{
			name: "三方应用（全限定）",
			out:  "priority=0 preferredOrder=0 match=0x108000 specificIndex=-1 isDefault=false\r\ncom.mi.health/com.xiaomi.fitness.login.SplashActivity\r\n",
			want: "com.mi.health/com.xiaomi.fitness.login.SplashActivity",
			ok:   true,
		},
		{
			name: "插件型应用没有启动入口",
			out:  "No activity found\r\n",
			ok:   false,
		},
		{
			name: "空输出",
			out:  "",
			ok:   false,
		},
	}
	for _, c := range cases {
		got, ok := ParseResolveActivity(c.out)
		if ok != c.ok {
			t.Errorf("%s: ok = %v, want %v", c.name, ok, c.ok)
			continue
		}
		if ok && got != c.want {
			t.Errorf("%s: 组件 = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestStartFailed 锁住 am start 的失败判定。
//
// 真机刷入不存在的包时输出：
// "Error: Activity not started, unable to resolve Intent { ... }"，退出码仍是 0。
func TestStartFailed(t *testing.T) {
	fail := []string{
		"Starting: Intent { act=android.intent.action.MAIN cat=[android.intent.category.LAUNCHER] pkg=cn.wps.moffice_eng.xiaomi.lite }\r\nError: Activity not started, unable to resolve Intent { act=android.intent.action.MAIN }",
		"Error: Activity class {com.foo/.Bar} does not exist.",
	}
	for _, out := range fail {
		if !startFailed(out) {
			t.Errorf("失败输出没被识别: %q", out)
		}
	}

	ok := []string{
		"Starting: Intent { cmp=com.android.settings/.MainSettings }\r\n",
		"Warning: Activity not started, intent has been delivered to currently running top-most instance.",
	}
	for _, out := range ok {
		if startFailed(out) {
			t.Errorf("成功输出被误判成失败: %q", out)
		}
	}
}

// TestHasPackage 锁住精确匹配：`pm list packages com.foo` 是模糊过滤，
// com.foobar 也会出现在结果里，不能用子串判断。
func TestHasPackage(t *testing.T) {
	out := "package:com.foo\r\npackage:com.foobar\r\n"
	if !hasPackage(out, "com.foo") {
		t.Error("com.foo 应判定为已安装")
	}
	if hasPackage(out, "com.fo") {
		t.Error("com.fo 只是前缀，不该判定为已安装")
	}
	if hasPackage("", "com.foo") {
		t.Error("空输出不该判定为已安装")
	}
	// 真机上 -d 过滤没有结果时输出为空字符串
	if hasPackage("\r\n", "com.foo") {
		t.Error("只有空行时不该判定为已安装")
	}
}
