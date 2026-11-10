package tweaks

import (
	"context"
	"os"
	"strings"
	"testing"
)

func loadFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("读取夹具 %s 失败: %v", name, err)
	}
	return string(b)
}

// TestFailureMessageFoldsStackTrace 用真机日志里的完整堆栈验证：
// 17 行 Java 异常必须折成一行，并且带上用户能行动的信息。
func TestFailureMessageFoldsStackTrace(t *testing.T) {
	out := loadFixture(t, "settings_denied_secure.txt")
	msg := failureMessage(out)

	if msg == "" {
		t.Fatal("被拒的输出没有被判成失败")
	}
	if strings.Contains(msg, "\n") {
		t.Errorf("错误信息里还有换行（没折叠干净）: %q", msg)
	}
	if strings.Contains(msg, "at com.android") {
		t.Errorf("错误信息里混进了堆栈帧: %q", msg)
	}
	if !strings.Contains(msg, "WRITE_SECURE_SETTINGS") {
		t.Errorf("没指出缺的是哪个权限: %q", msg)
	}
	if !strings.Contains(msg, "root") {
		t.Errorf("没告诉用户怎么解决: %q", msg)
	}
	if len([]rune(msg)) > 120 {
		t.Errorf("错误信息太长（%d 字）: %q", len([]rune(msg)), msg)
	}
}

// TestFailureMessageSystemNamespace 真机日志里 system 命名空间报的是
// WRITE_SETTINGS（不带 SECURE），两条路径要分开翻译。
func TestFailureMessageSystemNamespace(t *testing.T) {
	msg := failureMessage(loadFixture(t, "settings_denied_system.txt"))
	if !strings.Contains(msg, "WRITE_SETTINGS") {
		t.Errorf("没识别出 WRITE_SETTINGS: %q", msg)
	}
	if strings.Contains(msg, "WRITE_SECURE_SETTINGS") {
		t.Errorf("把 WRITE_SETTINGS 误判成 WRITE_SECURE_SETTINGS: %q", msg)
	}
}

// TestFailureMessageIgnoresNormalOutput 正常输出不能被误判成失败。
func TestFailureMessageIgnoresNormalOutput(t *testing.T) {
	for _, out := range []string{
		"",
		"null",
		"1",
		"1080x2400\n",
		"Deleted 0 rows",
		"Physical size: 1080x2400\n",
	} {
		if msg := failureMessage(out); msg != "" {
			t.Errorf("正常输出 %q 被误判成失败: %q", out, msg)
		}
	}
}

// TestFailureMessageKeepsUnknownErrors 非权限类错误保留一行原文，不能吞掉。
func TestFailureMessageKeepsUnknownErrors(t *testing.T) {
	out := "Error: unknown command 'foo'\nusage: settings [--user NUM] get|put|delete ..."
	msg := failureMessage(out)
	if !strings.Contains(msg, "unknown command") {
		t.Errorf("丢了原始错误: %q", msg)
	}
}

// fakeNsExec 按命令里的命名空间返回不同结果，并记录执行过的命令。
func fakeNsExec(t *testing.T, denied map[string]string) (Exec, *[]string) {
	t.Helper()
	var cmds []string
	exec := func(ctx context.Context, command string) (string, error) {
		cmds = append(cmds, command)
		for ns, out := range denied {
			if strings.Contains(command, "settings put "+ns+" ") {
				return out, nil
			}
		}
		return "", nil
	}
	return exec, &cmds
}

// TestProbeCapsAllDenied 复现真机状态：三个命名空间全被拒。
func TestProbeCapsAllDenied(t *testing.T) {
	secure := loadFixture(t, "settings_denied_secure.txt")
	system := loadFixture(t, "settings_denied_system.txt")
	exec, cmds := fakeNsExec(t, map[string]string{"system": system, "secure": secure, "global": secure})

	caps := ProbeCaps(context.Background(), exec)
	if caps.SystemWritable || caps.SecureWritable || caps.GlobalWritable {
		t.Errorf("全部被拒却报可写: %+v", caps)
	}
	if caps.AnyWritable() {
		t.Error("AnyWritable 应为 false")
	}
	if !strings.Contains(caps.Note, "root") {
		t.Errorf("说明里没给出路: %q", caps.Note)
	}
	if len(*cmds) != 3 {
		t.Errorf("探测了 %d 次，应为 3 次", len(*cmds))
	}
	// 探测必须写完就删，不能在用户设备上留垃圾键。
	for _, c := range *cmds {
		if !strings.Contains(c, probeKey) || !strings.Contains(c, "settings delete") {
			t.Errorf("探测命令没有收尾删除: %q", c)
		}
	}
}

// TestProbeCapsAllWritable 原生系统（AOSP）应当三个全可写，且不产生提示。
func TestProbeCapsAllWritable(t *testing.T) {
	exec, _ := fakeNsExec(t, nil)
	caps := ProbeCaps(context.Background(), exec)
	if !caps.SystemWritable || !caps.SecureWritable || !caps.GlobalWritable {
		t.Errorf("可写却报不可写: %+v", caps)
	}
	if caps.Note != "" {
		t.Errorf("全部可写时不该有提示: %q", caps.Note)
	}
}

// TestProbeCapsPartial 部分可写时，说明里只点名被拒的命名空间。
func TestProbeCapsPartial(t *testing.T) {
	secure := loadFixture(t, "settings_denied_secure.txt")
	exec, _ := fakeNsExec(t, map[string]string{"secure": secure, "global": secure})

	caps := ProbeCaps(context.Background(), exec)
	if !caps.SystemWritable {
		t.Error("system 应可写")
	}
	if caps.SecureWritable || caps.GlobalWritable {
		t.Error("secure / global 应不可写")
	}
	if !strings.Contains(caps.Note, "secure") || strings.Contains(caps.Note, "system、") {
		t.Errorf("说明点错了命名空间: %q", caps.Note)
	}
}
