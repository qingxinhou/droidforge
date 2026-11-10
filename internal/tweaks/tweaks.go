// Package tweaks 通过 ADB Shell 读写显示、动画与系统界面设置。
//
// 所有操作都走 settings / wm 这类系统命令，不需要 root；
// 是否生效取决于 Android 版本、ROM 与 Shell 权限。
package tweaks

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Exec 执行一条设备端 shell 命令。
type Exec = func(ctx context.Context, command string) (string, error)

// Display 是显示与动画相关的当前状态。
type Display struct {
	PhysicalSize    string  `json:"physicalSize"`
	OverrideSize    string  `json:"overrideSize"`
	PhysicalDensity string  `json:"physicalDensity"`
	OverrideDensity string  `json:"overrideDensity"`
	FontScale       float64 `json:"fontScale"`
	WindowAnim      float64 `json:"windowAnim"`
	TransitionAnim  float64 `json:"transitionAnim"`
	AnimatorAnim    float64 `json:"animatorAnim"`
	ScreenTimeoutMS int     `json:"screenTimeoutMs"`
}

// SysUI 是状态栏 / 系统界面相关的当前状态。
type SysUI struct {
	IconBlacklist   []string `json:"iconBlacklist"`
	ClockSeconds    bool     `json:"clockSeconds"`
	BatteryPercent  bool     `json:"batteryPercent"`
	QuickCount      int      `json:"quickCount"`
	Immersive       bool     `json:"immersive"`
	CaptiveDisabled bool     `json:"captiveDisabled"`
}

const stateScript = `echo '##SIZE'; wm size
echo '##DENSITY'; wm density
echo '##FONT'; settings get system font_scale
echo '##WANIM'; settings get global window_animation_scale
echo '##TANIM'; settings get global transition_animation_scale
echo '##AANIM'; settings get global animator_duration_scale
echo '##TIMEOUT'; settings get system screen_off_timeout
echo '##ICONS'; settings get secure icon_blacklist
echo '##CLOCK'; settings get secure clock_seconds
echo '##BATT'; settings get system status_bar_show_battery_percent
echo '##QQS'; settings get secure sysui_qqs_count
echo '##IMM'; settings get global policy_control
echo '##CAPTIVE'; settings get global captive_portal_mode`

// Read 读取当前显示与系统界面设置。
func Read(ctx context.Context, exec Exec) (Display, SysUI, error) {
	out, err := exec(ctx, stateScript)
	if err != nil {
		return Display{}, SysUI{}, err
	}
	sections := splitSections(out)
	var d Display
	var s SysUI

	parseWM(sections["SIZE"], &d)
	parseWMDensity(sections["DENSITY"], &d)
	d.FontScale = firstFloat(sections["FONT"], 1)
	d.WindowAnim = firstFloat(sections["WANIM"], 1)
	d.TransitionAnim = firstFloat(sections["TANIM"], 1)
	d.AnimatorAnim = firstFloat(sections["AANIM"], 1)
	d.ScreenTimeoutMS = int(firstFloat(sections["TIMEOUT"], 30000))

	s.IconBlacklist = splitList(sections["ICONS"])
	s.ClockSeconds = valueIsOn(sections["CLOCK"])
	s.BatteryPercent = valueIsOn(sections["BATT"])
	s.QuickCount = int(firstFloat(sections["QQS"], 0))
	s.Immersive = strings.Contains(strings.TrimSpace(sections["IMM"]), "immersive")
	s.CaptiveDisabled = strings.TrimSpace(sections["CAPTIVE"]) == "0"
	return d, s, nil
}

// ParseSize 解析 "1080x2400"。
func ParseSize(v string) (int, int, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	a, b, ok := strings.Cut(v, "x")
	if !ok {
		return 0, 0, fmt.Errorf("分辨率格式应为 宽x高，例如 1080x2400")
	}
	w, err1 := strconv.Atoi(strings.TrimSpace(a))
	h, err2 := strconv.Atoi(strings.TrimSpace(b))
	if err1 != nil || err2 != nil || w < 240 || h < 240 || w > 10000 || h > 10000 {
		return 0, 0, fmt.Errorf("分辨率数值不合法：%s", v)
	}
	return w, h, nil
}

// SetResolution 设置显示分辨率；传空字符串表示恢复默认。
func SetResolution(ctx context.Context, exec Exec, size string) error {
	if strings.TrimSpace(size) == "" {
		_, err := exec(ctx, "wm size reset")
		return err
	}
	if _, _, err := ParseSize(size); err != nil {
		return err
	}
	return run(ctx, exec, "wm size "+strings.TrimSpace(size))
}

// SetDensity 设置显示密度；0 表示恢复默认。
func SetDensity(ctx context.Context, exec Exec, density int) error {
	if density <= 0 {
		_, err := exec(ctx, "wm density reset")
		return err
	}
	if density < 72 || density > 1000 {
		return fmt.Errorf("密度应在 72–1000 之间")
	}
	return run(ctx, exec, fmt.Sprintf("wm density %d", density))
}

// SetFontScale 设置字体缩放倍数；0 表示恢复默认。
func SetFontScale(ctx context.Context, exec Exec, scale float64) error {
	if scale <= 0 {
		scale = 1
	}
	if scale < 0.5 || scale > 2 {
		return fmt.Errorf("字体缩放应在 0.5–2.0 之间")
	}
	return run(ctx, exec, fmt.Sprintf("settings put system font_scale %s", trimFloat(scale)))
}

// SetAnimationScales 设置窗口 / 过渡 / 动画时长倍率，0 表示关闭动画。
func SetAnimationScales(ctx context.Context, exec Exec, window, transition, animator float64) error {
	for _, v := range []float64{window, transition, animator} {
		if v < 0 || v > 10 {
			return fmt.Errorf("动画倍率应在 0–10 之间")
		}
	}
	cmd := fmt.Sprintf(
		"settings put global window_animation_scale %s; settings put global transition_animation_scale %s; settings put global animator_duration_scale %s",
		trimFloat(window), trimFloat(transition), trimFloat(animator))
	return run(ctx, exec, cmd)
}

// SetScreenTimeout 设置自动锁屏时间（毫秒）。
func SetScreenTimeout(ctx context.Context, exec Exec, ms int) error {
	if ms < 15000 || ms > 1800000 {
		return fmt.Errorf("锁屏时间应在 15 秒–30 分钟之间")
	}
	return run(ctx, exec, fmt.Sprintf("settings put system screen_off_timeout %d", ms))
}

// SetIconBlacklist 设置状态栏隐藏图标列表；传空切片表示全部显示。
func SetIconBlacklist(ctx context.Context, exec Exec, icons []string) error {
	clean := make([]string, 0, len(icons))
	for _, v := range icons {
		v = strings.TrimSpace(v)
		if v != "" && !strings.ContainsAny(v, " \t;'\"") {
			clean = append(clean, v)
		}
	}
	if len(clean) == 0 {
		_, err := exec(ctx, "settings delete secure icon_blacklist")
		return err
	}
	return run(ctx, exec, "settings put secure icon_blacklist "+strings.Join(clean, ","))
}

// SetClockSeconds 状态栏时钟是否显示秒。
func SetClockSeconds(ctx context.Context, exec Exec, on bool) error {
	return run(ctx, exec, "settings put secure clock_seconds "+boolStr(on))
}

// SetBatteryPercent 状态栏是否显示电量百分比。
func SetBatteryPercent(ctx context.Context, exec Exec, on bool) error {
	// 不同 ROM 用不同键，两个都写，失败不影响另一个。
	cmd := "settings put system status_bar_show_battery_percent " + boolStr(on) +
		"; settings put secure battery_percentage " + boolStr(on)
	return run(ctx, exec, cmd)
}

// SetQuickCount 设置下拉快捷开关的行列数量（部分 ROM 生效）。
func SetQuickCount(ctx context.Context, exec Exec, n int) error {
	if n < 1 || n > 12 {
		return fmt.Errorf("快捷开关数量应在 1–12 之间")
	}
	return run(ctx, exec, fmt.Sprintf("settings put secure sysui_qqs_count %d", n))
}

// SetImmersive 开启 / 关闭全局沉浸模式。
func SetImmersive(ctx context.Context, exec Exec, on bool) error {
	if !on {
		_, err := exec(ctx, "settings delete global policy_control")
		return err
	}
	return run(ctx, exec, "settings put global policy_control immersive.full=*")
}

// SetCaptivePortal 关闭 / 恢复 Wi-Fi 网络检测（关闭后不再出现 x 或 ! 标记）。
func SetCaptivePortal(ctx context.Context, exec Exec, disabled bool) error {
	if !disabled {
		_, err := exec(ctx, "settings delete global captive_portal_mode")
		return err
	}
	return run(ctx, exec, "settings put global captive_portal_mode 0")
}

// KeyEvent 发送按键事件。
func KeyEvent(ctx context.Context, exec Exec, code string) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return fmt.Errorf("按键码不能为空")
	}
	if strings.ContainsAny(code, " ;|&'\"") {
		return fmt.Errorf("按键码不合法")
	}
	return run(ctx, exec, "input keyevent "+code)
}

// InputText 向当前焦点输入文本（仅支持 ASCII）。
func InputText(ctx context.Context, exec Exec, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("文本不能为空")
	}
	if strings.ContainsAny(text, "\"'`$\\") {
		return fmt.Errorf("文本包含不支持的字符")
	}
	escaped := strings.ReplaceAll(text, " ", "%s")
	return run(ctx, exec, "input text "+escaped)
}

func run(ctx context.Context, exec Exec, cmd string) error {
	out, err := exec(ctx, cmd)
	if err != nil {
		return err
	}
	low := strings.ToLower(out)
	if strings.Contains(low, "exception") || strings.Contains(low, "error:") || strings.Contains(low, "unknown command") {
		return fmt.Errorf("%s", strings.TrimSpace(out))
	}
	return nil
}

func splitSections(out string) map[string]string {
	result := make(map[string]string, 13)
	var key string
	var buf []string
	flush := func() {
		if key != "" {
			result[key] = strings.TrimSpace(strings.Join(buf, "\n"))
		}
		buf = buf[:0]
	}
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, "##") {
			flush()
			key = strings.TrimSpace(strings.TrimPrefix(line, "##"))
			continue
		}
		if key != "" {
			buf = append(buf, line)
		}
	}
	flush()
	return result
}

func parseWM(v string, d *Display) {
	for _, line := range strings.Split(v, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Physical size:"):
			d.PhysicalSize = strings.TrimSpace(strings.TrimPrefix(line, "Physical size:"))
		case strings.HasPrefix(line, "Override size:"):
			d.OverrideSize = strings.TrimSpace(strings.TrimPrefix(line, "Override size:"))
		}
	}
}

func parseWMDensity(v string, d *Display) {
	for _, line := range strings.Split(v, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Physical density:"):
			d.PhysicalDensity = strings.TrimSpace(strings.TrimPrefix(line, "Physical density:"))
		case strings.HasPrefix(line, "Override density:"):
			d.OverrideDensity = strings.TrimSpace(strings.TrimPrefix(line, "Override density:"))
		}
	}
}

func firstFloat(v string, fallback float64) float64 {
	v = strings.TrimSpace(strings.SplitN(v, "\n", 2)[0])
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return f
}

func splitList(v string) []string {
	v = strings.TrimSpace(v)
	if v == "" || v == "null" {
		return []string{}
	}
	parts := strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' })
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func valueIsOn(v string) bool {
	switch strings.TrimSpace(v) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func boolStr(on bool) string {
	if on {
		return "1"
	}
	return "0"
}

func trimFloat(f float64) string {
	s := strconv.FormatFloat(f, 'f', 2, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" {
		return "0"
	}
	return s
}
