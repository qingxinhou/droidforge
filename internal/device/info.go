// Package device 采集设备软硬件信息与分区表。
package device

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"droidforge/internal/adb"
)

// RootStatus 描述设备的 root 状态。
type RootStatus struct {
	Rooted          bool     `json:"rooted"`
	Solutions       []string `json:"solutions"` // magisk / kernelsu / apatch / supersu
	SuPath          string   `json:"suPath"`
	MagiskVersion   string   `json:"magiskVersion"`
	KernelSUVersion string   `json:"kernelSUVersion"`
	APatchVersion   string   `json:"apatchVersion"`
	Zygisk          bool     `json:"zygisk"`
	Detail          string   `json:"detail"`
}

// DeviceInfo 是设备信息总览。
type DeviceInfo struct {
	Serial           string     `json:"serial"`
	Model            string     `json:"model"`
	Brand            string     `json:"brand"`
	Device           string     `json:"device"`
	ProductName      string     `json:"productName"`
	AndroidVersion   string     `json:"androidVersion"`
	SDK              int        `json:"sdk"`
	SecurityPatch    string     `json:"securityPatch"`
	BuildID          string     `json:"buildId"`
	BuildFingerprint string     `json:"buildFingerprint"`
	BuildType        string     `json:"buildType"`
	BuildTags        string     `json:"buildTags"`
	ABI              string     `json:"abi"`
	ABIList          string     `json:"abiList"`
	SoC              string     `json:"soc"`
	Kernel           string     `json:"kernel"`
	Arch             string     `json:"arch"`
	Slot             string     `json:"slot"`
	IsAB             bool       `json:"isAb"`
	HasDynamicParts  bool       `json:"hasDynamicPartitions"`
	VerifiedBoot     string     `json:"verifiedBoot"`
	FlashLocked      string     `json:"flashLocked"`
	Debuggable       bool       `json:"debuggable"`
	SELinux          string     `json:"selinux"`
	Uptime           string     `json:"uptime"`
	MemoryTotalKB    int64      `json:"memoryTotalKb"`
	StorageTotalKB   int64      `json:"storageTotalKb"`
	StorageFreeKB    int64      `json:"storageFreeKb"`
	BatteryLevel     int        `json:"batteryLevel"`
	Root             RootStatus `json:"root"`
}

// Collect 采集指定设备的完整信息。
func Collect(ctx context.Context, a *adb.Client, serial string) (*DeviceInfo, error) {
	info := &DeviceInfo{Serial: serial}

	keys := []string{
		"ro.product.model", "ro.product.brand", "ro.product.device", "ro.product.name",
		"ro.build.version.release", "ro.build.version.sdk", "ro.build.version.security_patch",
		"ro.build.display.id", "ro.build.fingerprint", "ro.build.type", "ro.build.tags",
		"ro.product.cpu.abi", "ro.product.cpu.abilist", "ro.board.platform",
		"ro.boot.slot_suffix", "ro.boot.verifiedbootstate", "ro.boot.flash.locked",
		"ro.build.ab_update", "ro.debuggable", "ro.secure", "ro.build.characteristics",
	}
	props := a.BatchGetProp(ctx, serial, keys)

	info.Model = props["ro.product.model"]
	info.Brand = props["ro.product.brand"]
	info.Device = props["ro.product.device"]
	info.ProductName = props["ro.product.name"]
	info.AndroidVersion = props["ro.build.version.release"]
	info.SDK, _ = strconv.Atoi(props["ro.build.version.sdk"])
	info.SecurityPatch = props["ro.build.version.security_patch"]
	info.BuildID = props["ro.build.display.id"]
	info.BuildFingerprint = props["ro.build.fingerprint"]
	info.BuildType = props["ro.build.type"]
	info.BuildTags = props["ro.build.tags"]
	info.ABI = props["ro.product.cpu.abi"]
	info.ABIList = props["ro.product.cpu.abilist"]
	info.SoC = props["ro.board.platform"]
	info.Slot = strings.TrimPrefix(props["ro.boot.slot_suffix"], "_")
	info.IsAB = info.Slot != "" || props["ro.build.ab_update"] == "true"
	info.VerifiedBoot = props["ro.boot.verifiedbootstate"]
	info.FlashLocked = props["ro.boot.flash.locked"]
	info.Debuggable = props["ro.debuggable"] == "1"

	// 内核与架构
	if out, err := a.Shell(ctx, serial, "uname -r"); err == nil {
		info.Kernel = strings.TrimSpace(out)
	}
	if out, err := a.Shell(ctx, serial, "uname -m"); err == nil {
		info.Arch = strings.TrimSpace(out)
	}

	// SELinux
	if out, err := a.Shell(ctx, serial, "getenforce"); err == nil {
		info.SELinux = strings.TrimSpace(out)
	}

	// 动态分区检测
	if out, err := a.Shell(ctx, serial, "ls /dev/block/mapper"); err == nil && strings.TrimSpace(out) != "" {
		info.HasDynamicParts = true
	}

	// 运行时长
	if out, err := a.Shell(ctx, serial, "cat /proc/uptime"); err == nil {
		fields := strings.Fields(out)
		if len(fields) > 0 {
			if secs, err := strconv.ParseFloat(fields[0], 64); err == nil {
				info.Uptime = humanDuration(int64(secs))
			}
		}
	}

	// 内存
	if out, err := a.Shell(ctx, serial, "cat /proc/meminfo"); err == nil {
		re := regexp.MustCompile(`MemTotal:\s+(\d+)`)
		if m := re.FindStringSubmatch(out); m != nil {
			info.MemoryTotalKB, _ = strconv.ParseInt(m[1], 10, 64)
		}
	}

	// 存储
	if out, err := a.Shell(ctx, serial, "df -k /data"); err == nil {
		lines := strings.Split(strings.TrimSpace(out), "\n")
		if len(lines) >= 2 {
			f := strings.Fields(lines[len(lines)-1])
			if len(f) >= 4 {
				info.StorageTotalKB, _ = strconv.ParseInt(f[1], 10, 64)
				info.StorageFreeKB, _ = strconv.ParseInt(f[3], 10, 64)
			}
		}
	}

	// 电量
	if out, err := a.Shell(ctx, serial, "dumpsys battery"); err == nil {
		re := regexp.MustCompile(`level:\s*(\d+)`)
		if m := re.FindStringSubmatch(out); m != nil {
			info.BatteryLevel, _ = strconv.Atoi(m[1])
		}
	}

	info.Root = DetectRoot(ctx, a, serial)
	return info, nil
}

// DetectRoot 检测 root 状态与方案。
func DetectRoot(ctx context.Context, a *adb.Client, serial string) RootStatus {
	var st RootStatus

	// 1. 尝试 su -c id
	if out, err := a.Shell(ctx, serial, "su -c id"); err == nil && strings.Contains(out, "uid=0") {
		st.Rooted = true
		st.Detail = "su 可用，已获取 root（uid=0）"
	}

	// 2. su 路径
	if out, err := a.Shell(ctx, serial, "which su 2>/dev/null"); err == nil {
		st.SuPath = strings.TrimSpace(out)
	}
	if st.SuPath == "" {
		if out, err := a.Shell(ctx, serial, "ls /system/bin/su /system/xbin/su /sbin/su 2>/dev/null"); err == nil {
			st.SuPath = strings.TrimSpace(strings.Split(out, "\n")[0])
		}
	}

	// 3. su -v 版本（Magisk 返回 "25.2:MAGISK"）
	if out, err := a.Shell(ctx, serial, "su -v"); err == nil {
		v := strings.TrimSpace(out)
		lower := strings.ToLower(v)
		switch {
		case strings.Contains(lower, "magisk"):
			st.MagiskVersion = v
			st.Solutions = appendUnique(st.Solutions, "magisk")
		case strings.Contains(lower, "kernelsu"):
			st.KernelSUVersion = v
			st.Solutions = appendUnique(st.Solutions, "kernelsu")
		case strings.Contains(lower, "apatch"):
			st.APatchVersion = v
			st.Solutions = appendUnique(st.Solutions, "apatch")
		}
	}

	// 4. 目录特征
	if out, err := a.Shell(ctx, serial, "ls /data/adb/magisk 2>/dev/null"); err == nil && strings.TrimSpace(out) != "" {
		st.Solutions = appendUnique(st.Solutions, "magisk")
		if st.MagiskVersion == "" {
			if v, err := a.Shell(ctx, serial, "cat /data/adb/magisk/util_functions.sh 2>/dev/null | grep MAGISK_VER= | head -1"); err == nil {
				st.MagiskVersion = strings.TrimSpace(v)
			}
		}
	}
	if out, err := a.Shell(ctx, serial, "ls /data/adb/ksu 2>/dev/null"); err == nil && strings.TrimSpace(out) != "" {
		st.Solutions = appendUnique(st.Solutions, "kernelsu")
	}
	if out, err := a.Shell(ctx, serial, "ls /data/adb/ap 2>/dev/null"); err == nil && strings.TrimSpace(out) != "" {
		st.Solutions = appendUnique(st.Solutions, "apatch")
	}

	// 5. 已安装的管理器包名
	if out, err := a.Shell(ctx, serial, "pm list packages"); err == nil {
		switch {
		case strings.Contains(out, "com.topjohnwu.magisk"):
			st.Solutions = appendUnique(st.Solutions, "magisk")
		case strings.Contains(out, "me.weishu.kernelsu"):
			st.Solutions = appendUnique(st.Solutions, "kernelsu")
		case strings.Contains(out, "me.bmax.apatch"):
			st.Solutions = appendUnique(st.Solutions, "apatch")
		}
	}

	if len(st.Solutions) > 0 {
		st.Rooted = true
	}
	if !st.Rooted && st.Detail == "" {
		st.Detail = "未检测到 root"
	}
	return st
}

func appendUnique(s []string, v string) []string {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}

func humanDuration(seconds int64) string {
	days := seconds / 86400
	hours := (seconds % 86400) / 3600
	mins := (seconds % 3600) / 60
	switch {
	case days > 0:
		return fmt.Sprintf("%d天 %d小时", days, hours)
	case hours > 0:
		return fmt.Sprintf("%d小时 %d分", hours, mins)
	default:
		return fmt.Sprintf("%d分", mins)
	}
}
