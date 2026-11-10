// Package power 读取与模拟电池状态。
package power

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Exec 执行一条设备端 shell 命令。
type Exec = func(ctx context.Context, command string) (string, error)

// Info 是一次电池读数。
type Info struct {
	Level            int     `json:"level"`  // 0–100
	Scale            int     `json:"scale"`  // 通常 100
	Status           int     `json:"status"` // 1 unknown / 2 charging / 3 discharging / 4 not charging / 5 full
	StatusText       string  `json:"statusText"`
	Health           int     `json:"health"`
	HealthText       string  `json:"healthText"`
	Present          bool    `json:"present"`
	ACPowered        bool    `json:"acPowered"`
	USBPowered       bool    `json:"usbPowered"`
	WirelessPowered  bool    `json:"wirelessPowered"`
	VoltageMV        int     `json:"voltageMv"`
	VoltageV         float64 `json:"voltageV"`
	TemperatureC     float64 `json:"temperatureC"`
	CurrentUA        int64   `json:"currentUa"` // 负值表示放电
	CurrentMA        float64 `json:"currentMa"`
	PowerW           float64 `json:"powerW"`
	ChargeCounterMAh float64 `json:"chargeCounterMah"`
	MaxChargingMA    float64 `json:"maxChargingMa"`
	Technology       string  `json:"technology"`
	CapacityLevel    string  `json:"capacityLevel"`
}

const readScript = `dumpsys battery
echo '##CUR'
cat /sys/class/power_supply/battery/current_now 2>/dev/null
echo '##VOLT'
cat /sys/class/power_supply/battery/voltage_now 2>/dev/null
echo '##CAP'
cat /sys/class/power_supply/battery/capacity 2>/dev/null`

// Read 读取当前电池状态。
func Read(ctx context.Context, exec Exec) (Info, error) {
	out, err := exec(ctx, readScript)
	if err != nil {
		return Info{}, err
	}
	var info Info
	body := out
	extra := map[string]string{}

	if idx := strings.Index(out, "##CUR"); idx >= 0 {
		body = out[:idx]
		rest := out[idx:]
		parts := strings.Split(rest, "##")
		for _, p := range parts {
			line := strings.TrimSpace(p)
			switch {
			case strings.HasPrefix(line, "CUR"):
				extra["cur"] = strings.TrimSpace(strings.TrimPrefix(line, "CUR"))
			case strings.HasPrefix(line, "VOLT"):
				extra["volt"] = strings.TrimSpace(strings.TrimPrefix(line, "VOLT"))
			case strings.HasPrefix(line, "CAP"):
				extra["cap"] = strings.TrimSpace(strings.TrimPrefix(line, "CAP"))
			}
		}
	}

	kv := map[string]string{}
	for _, line := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		kv[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}

	info.Level = atoi(kv["level"])
	info.Scale = atoiDefault(kv["scale"], 100)
	info.Status = atoi(kv["status"])
	info.StatusText = statusText(info.Status)
	info.Health = atoi(kv["health"])
	info.HealthText = healthText(info.Health)
	info.Present = kv["present"] == "true"
	info.ACPowered = kv["AC powered"] == "true"
	info.USBPowered = kv["USB powered"] == "true"
	info.WirelessPowered = kv["Wireless powered"] == "true"
	info.VoltageMV = atoi(kv["voltage"])
	info.VoltageV = float64(info.VoltageMV) / 1000
	info.TemperatureC = float64(atoi(kv["temperature"])) / 10
	info.Technology = kv["technology"]
	info.CapacityLevel = kv["capacity level"]
	info.ChargeCounterMAh = float64(atoi(kv["Charge counter"])) / 1000
	info.MaxChargingMA = float64(atoi(kv["Max charging current"])) / 1000

	// sysfs 的 current_now 单位是 µA；部分机型读不到，退回 dumpsys 电压估算。
	cur := parseInt64(extra["cur"])
	if cur != 0 {
		info.CurrentUA = cur
		info.CurrentMA = float64(cur) / 1000
		if info.VoltageMV > 0 {
			info.PowerW = float64(info.VoltageMV) / 1000 * (float64(cur) / 1e6)
			if info.PowerW < 0 {
				info.PowerW = -info.PowerW
			}
		}
	}
	if v := parseInt64(extra["volt"]); v > 0 && info.VoltageMV == 0 {
		info.VoltageMV = int(v / 1000)
		info.VoltageV = float64(info.VoltageMV) / 1000
	}
	return info, nil
}

// Mock 是电池模拟参数；Level 为 0 表示不修改。
type Mock struct {
	Level       int   `json:"level"`
	AC          *bool `json:"ac"`
	USB         *bool `json:"usb"`
	Wireless    *bool `json:"wireless"`
	Unplug      bool  `json:"unplug"`
	FullReset   bool  `json:"fullReset"`
	Discharging bool  `json:"discharging"`
}

// ApplyMock 应用电池模拟；恢复请用 Reset。
func ApplyMock(ctx context.Context, exec Exec, m Mock) error {
	var cmds []string
	if m.FullReset {
		cmds = append(cmds, "dumpsys battery reset")
	}
	if m.Unplug {
		cmds = append(cmds, "dumpsys battery unplug")
	}
	if m.Level > 0 {
		if m.Level > 100 {
			return fmt.Errorf("电量应在 1–100 之间")
		}
		cmds = append(cmds, fmt.Sprintf("dumpsys battery set level %d", m.Level))
	}
	if m.AC != nil {
		cmds = append(cmds, "dumpsys battery set ac "+boolStr(*m.AC))
	}
	if m.USB != nil {
		cmds = append(cmds, "dumpsys battery set usb "+boolStr(*m.USB))
	}
	if m.Wireless != nil {
		cmds = append(cmds, "dumpsys battery set wireless "+boolStr(*m.Wireless))
	}
	switch {
	case m.Discharging:
		cmds = append(cmds, "dumpsys battery set status 3")
	case m.Level >= 100:
		cmds = append(cmds, "dumpsys battery set status 5")
	}
	if len(cmds) == 0 {
		return fmt.Errorf("没有需要应用的参数")
	}
	_, err := exec(ctx, strings.Join(cmds, "; "))
	return err
}

// Reset 恢复真实电池状态。
func Reset(ctx context.Context, exec Exec) error {
	_, err := exec(ctx, "dumpsys battery reset")
	return err
}

func statusText(s int) string {
	switch s {
	case 2:
		return "充电中"
	case 3:
		return "放电中"
	case 4:
		return "未充电"
	case 5:
		return "已充满"
	default:
		return "未知"
	}
}

func healthText(h int) string {
	switch h {
	case 2:
		return "良好"
	case 3:
		return "过热"
	case 4:
		return "已损坏"
	case 5:
		return "电压过高"
	case 6:
		return "故障"
	case 7:
		return "温度过低"
	default:
		return "未知"
	}
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func atoiDefault(s string, def int) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return def
	}
	return n
}

func parseInt64(s string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n
}

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
