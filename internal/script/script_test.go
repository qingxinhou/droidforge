package script

import (
	"context"
	"strings"
	"testing"
)

type recorder struct {
	lines   []string
	shells  []string
	adbArgs [][]string
}

func (r *recorder) host() Host {
	return Host{
		Shell: func(ctx context.Context, command string) (string, error) {
			r.shells = append(r.shells, command)
			return "Pixel 9", nil
		},
		Adb: func(ctx context.Context, args []string) (string, error) {
			r.adbArgs = append(r.adbArgs, args)
			return "ok", nil
		},
		Log: func(kind, text string) {
			r.lines = append(r.lines, kind+"|"+text)
		},
	}
}

func joined(r *recorder) string { return strings.Join(r.lines, "\n") }

func TestEchoSetAndFor(t *testing.T) {
	rec := &recorder{}
	src := `@echo off
set /a n=2+3*2
echo n=%n%
for /l %%i in (1,1,%n%) do (
  echo i=%%i
)
`
	res, err := Run(context.Background(), rec.host(), src)
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}
	if res.Failures != 0 {
		t.Fatalf("不应有失败: %s", joined(rec))
	}
	if !strings.Contains(joined(rec), "out|n=8") {
		t.Fatalf("表达式计算错误: %s", joined(rec))
	}
	for i := 1; i <= 8; i++ {
		want := "out|i=" + itoa(i)
		if !strings.Contains(joined(rec), want) {
			t.Fatalf("缺少循环输出 %s: %s", want, joined(rec))
		}
	}
}

func TestAdbShellDispatch(t *testing.T) {
	rec := &recorder{}
	src := "adb shell getprop ro.product.model\nadb install -r C:\\tmp\\a.apk\n"
	if _, err := Run(context.Background(), rec.host(), src); err != nil {
		t.Fatalf("执行失败: %v", err)
	}
	if len(rec.shells) != 1 || rec.shells[0] != "getprop ro.product.model" {
		t.Fatalf("shell 命令错误: %v", rec.shells)
	}
	if len(rec.adbArgs) != 1 || rec.adbArgs[0][0] != "install" {
		t.Fatalf("install 未透传: %v", rec.adbArgs)
	}
	if !strings.Contains(joined(rec), "out|Pixel 9") {
		t.Fatalf("命令输出未回显: %s", joined(rec))
	}
}

func TestIfErrorlevelAndGoto(t *testing.T) {
	rec := &recorder{}
	src := `@echo off
set /a v=3
if %v%==3 goto hit
echo miss
goto end
:hit
echo matched
:end
echo done
`
	res, err := Run(context.Background(), rec.host(), src)
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}
	if res.Failures != 0 {
		t.Fatalf("不应有失败: %s", joined(rec))
	}
	out := joined(rec)
	if !strings.Contains(out, "out|matched") || strings.Contains(out, "out|miss") {
		t.Fatalf("跳转逻辑错误: %s", out)
	}
	if !strings.Contains(out, "out|done") {
		t.Fatalf("未执行到结尾: %s", out)
	}
}

func TestUnsupportedCommandCountsFailure(t *testing.T) {
	rec := &recorder{}
	res, err := Run(context.Background(), rec.host(), "call foo.bat\npowershell -c 1\n")
	if err != nil {
		t.Fatalf("不应返回致命错误: %v", err)
	}
	if res.Failures == 0 {
		t.Fatalf("不支持的命令应计入失败")
	}
	if !strings.Contains(joined(rec), "不支持的命令") {
		t.Fatalf("缺少错误提示: %s", joined(rec))
	}
}

func TestEvalExpr(t *testing.T) {
	cases := map[string]int64{
		"1+2*3":     7,
		"(1+2)*3":   9,
		"10/3":      3,
		"10%3":      1,
		"2+3*4-5/5": 13,
	}
	for expr, want := range cases {
		got, err := evalExpr(expr)
		if err != nil {
			t.Fatalf("%s 计算失败: %v", expr, err)
		}
		if got != want {
			t.Fatalf("%s = %d，期望 %d", expr, got, want)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
