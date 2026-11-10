package tweaks

import (
	"context"
	"fmt"
	"strings"
)

// Caps 是 adb shell 在三个 settings 命名空间上的写权限。
//
// 为什么必须探测：AOSP 默认给 shell 角色 WRITE_SECURE_SETTINGS，所以
// `settings put secure ...` 在任何原生系统上都能用。但 MIUI / HyperOS
// 及部分定制 ROM 把这几个权限从 shell 上收回去了，结果是每一条
// `settings put` 都抛 SecurityException —— 而 UI 上完全看不出来，
// 按钮照常可点，用户只能一次次点、一次次看 Java 堆栈。
//
// 真机日志里的证据（Redmi K60 至尊版 / Android 13）：
//
//	settings put system  → com.android.shell was not granted WRITE_SETTINGS
//	settings put secure  → requires:android.permission.WRITE_SECURE_SETTINGS
//	settings put global  → requires:android.permission.WRITE_SECURE_SETTINGS
//
// 三个命名空间全灭。
type Caps struct {
	SystemWritable bool `json:"systemWritable"`
	SecureWritable bool `json:"secureWritable"`
	GlobalWritable bool `json:"globalWritable"`
	// Note 是给用户看的一句话解释；全部可写时为空。
	Note string `json:"note"`
}

// AnyWritable 表示至少有一个命名空间能写。
func (c Caps) AnyWritable() bool {
	return c.SystemWritable || c.SecureWritable || c.GlobalWritable
}

const probeKey = "droidforge_probe"

// ProbeCaps 用一次写入 + 一次删除探测各命名空间是否可写。
//
// 探测键是本工具自己的名字，写完立刻删掉，不碰用户的任何设置。
// 注意 settings 命令即使被拒也返回退出码 0，只能读输出文本判断。
func ProbeCaps(ctx context.Context, exec Exec) Caps {
	var c Caps
	var denied []string

	check := func(ns string) bool {
		cmd := fmt.Sprintf("settings put %s %s 1; settings delete %s %s", ns, probeKey, ns, probeKey)
		out, err := exec(ctx, cmd)
		if err != nil || failureMessage(out) != "" {
			denied = append(denied, ns)
			return false
		}
		return true
	}

	c.SystemWritable = check("system")
	c.SecureWritable = check("secure")
	c.GlobalWritable = check("global")

	switch len(denied) {
	case 3:
		c.Note = "本机 ROM 禁止 adb shell 写入系统设置（MIUI / HyperOS 常见）：system、secure、global 三个命名空间全部被拒。" +
			"下面这些开关点了也不会生效，需要 root 才能改。"
	case 0:
		c.Note = ""
	default:
		c.Note = "本机 ROM 禁止 adb shell 写入 " + strings.Join(denied, "、") + " 设置，依赖它的开关点了也不会生效。"
	}
	return c
}

// FailureMessage 从命令输出里挑出失败原因，没失败迹象时返回空串。
//
// 只保留一行：`settings` 被拒时会吐 17 行 Java 堆栈，整段抛到界面上
// 既占满 toast 又淹没真正的原因。权限类的失败再翻译成能行动的话。
func FailureMessage(out string) string { return failureMessage(out) }

func failureMessage(out string) string {
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return ""
	}
	low := strings.ToLower(trimmed)
	if !strings.Contains(low, "exception") &&
		!strings.Contains(low, "error:") &&
		!strings.Contains(low, "unknown command") {
		return ""
	}

	// 权限问题优先按缺失的权限名翻译 —— 这是用户唯一能采取行动的信息。
	switch {
	case strings.Contains(trimmed, "WRITE_SECURE_SETTINGS"):
		return "本机 ROM 限制：adb shell 没有 WRITE_SECURE_SETTINGS 权限（MIUI / HyperOS 常见），" +
			"这个开关在本机改不了，需要 root。"
	case strings.Contains(trimmed, "WRITE_SETTINGS"):
		return "本机 ROM 限制：adb shell 没有 WRITE_SETTINGS 权限，这个开关在本机改不了，需要 root。"
	}
	return truncateLine(firstErrorLine(trimmed), 200)
}

// firstErrorLine 取最能说明问题的那一行：优先 `java.lang.XxxException: ...`，
// 而不是 `Exception occurred while executing 'put':` 这种只有动作没有原因的壳。
func firstErrorLine(trimmed string) string {
	var shellLine, javaLine string
	for _, raw := range strings.Split(trimmed, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" || strings.HasPrefix(line, "at ") {
			continue
		}
		if strings.Contains(line, "Exception") {
			if javaLine == "" && strings.Contains(line, "Exception:") {
				javaLine = line
			}
			if shellLine == "" {
				shellLine = line
			}
			continue
		}
		if shellLine == "" {
			l := strings.ToLower(line)
			if strings.Contains(l, "error:") || strings.Contains(l, "unknown command") {
				shellLine = line
			}
		}
	}
	if javaLine != "" {
		return javaLine
	}
	if shellLine != "" {
		return shellLine
	}
	return strings.TrimSpace(strings.Split(trimmed, "\n")[0])
}

func truncateLine(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}
