package tweaks

import (
	"context"
	"strings"
	"testing"
)

func fakeExec(out string) Exec {
	return func(ctx context.Context, command string) (string, error) { return out, nil }
}

func TestReadParsesSections(t *testing.T) {
	out := `##SIZE
Physical size: 1080x2400
Override size: 1080x2280
##DENSITY
Physical density: 420
Override density: 440
##FONT
1.15
##WANIM
0.5
##TANIM
0.5
##AANIM
0.5
##TIMEOUT
60000
##ICONS
rotate,bluetooth
##CLOCK
1
##BATT
0
##QQS
5
##IMM
null*
##CAPTIVE
0`
	d, s, err := Read(context.Background(), fakeExec(out))
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if d.PhysicalSize != "1080x2400" || d.OverrideSize != "1080x2280" {
		t.Fatalf("分辨率解析错误: %+v", d)
	}
	if d.PhysicalDensity != "420" || d.OverrideDensity != "440" {
		t.Fatalf("密度解析错误: %+v", d)
	}
	if d.FontScale != 1.15 || d.WindowAnim != 0.5 || d.ScreenTimeoutMS != 60000 {
		t.Fatalf("设置解析错误: %+v", d)
	}
	if len(s.IconBlacklist) != 2 || s.IconBlacklist[0] != "rotate" {
		t.Fatalf("图标列表解析错误: %+v", s.IconBlacklist)
	}
	if !s.ClockSeconds || s.BatteryPercent {
		t.Fatalf("开关解析错误: %+v", s)
	}
	if s.QuickCount != 5 || !s.CaptiveDisabled || s.Immersive {
		t.Fatalf("系统界面解析错误: %+v", s)
	}
}

func TestReadHandlesMissingValues(t *testing.T) {
	out := `##SIZE
Physical size: 1440x3200
##FONT
null
##TIMEOUT
null
##ICONS
null`
	d, s, err := Read(context.Background(), fakeExec(out))
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if d.FontScale != 1 || d.ScreenTimeoutMS != 30000 {
		t.Fatalf("默认值不正确: %+v", d)
	}
	if len(s.IconBlacklist) != 0 {
		t.Fatalf("null 应解析为空列表: %+v", s.IconBlacklist)
	}
}

func TestParseSize(t *testing.T) {
	if w, h, err := ParseSize("1080x2400"); err != nil || w != 1080 || h != 2400 {
		t.Fatalf("正常分辨率解析失败: %d %d %v", w, h, err)
	}
	for _, bad := range []string{"1080", "10x20", "abcxdef", "1080x99999"} {
		if _, _, err := ParseSize(bad); err == nil {
			t.Fatalf("非法分辨率应报错: %s", bad)
		}
	}
}

func TestSetCommands(t *testing.T) {
	var last string
	exec := func(ctx context.Context, command string) (string, error) {
		last = command
		return "", nil
	}
	if err := SetDensity(context.Background(), exec, 0); err != nil || last != "wm density reset" {
		t.Fatalf("恢复密度命令错误: %s %v", last, err)
	}
	if err := SetDensity(context.Background(), exec, 440); err != nil || last != "wm density 440" {
		t.Fatalf("设置密度命令错误: %s %v", last, err)
	}
	if err := SetIconBlacklist(context.Background(), exec, nil); err != nil || !strings.Contains(last, "delete secure icon_blacklist") {
		t.Fatalf("清空图标命令错误: %s %v", last, err)
	}
	if err := SetAnimationScales(context.Background(), exec, 0, 0, 0); err != nil || !strings.Contains(last, "animator_duration_scale 0") {
		t.Fatalf("动画命令错误: %s %v", last, err)
	}
}
