// Package flashscript 把线刷步骤渲染成可直接运行的脚本。
//
// 输出三种形式，内容等价：
//
//	flash-all.bat  Windows 双击运行
//	flash-all.sh   macOS / Linux
//	flash-all.txt  纯命令清单，方便逐条核对或粘贴进现有脚本
//
// 脚本里的镜像路径优先用相对路径，脚本与镜像放在同一目录即可整体拷贝。
package flashscript

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"droidforge/internal/rom"
)

// Options 控制脚本内容。
type Options struct {
	// Slot 非空时给带槽位的分区补上 _a / _b。
	Slot string `json:"slot"`
	// Reboot 为 true 时脚本末尾重启到系统。
	Reboot bool `json:"reboot"`
	// Wipe 为 true 时在刷写前追加 fastboot -w 清空用户数据。
	Wipe bool `json:"wipe"`
}

// 允许带槽位后缀的分区，其余（super / userdata / modem 等）保持原名。
var slottedPartitions = map[string]bool{
	"boot": true, "init_boot": true, "vendor_boot": true, "recovery": true, "dtbo": true,
	"vbmeta": true, "vbmeta_system": true, "vbmeta_vendor": true,
	"system": true, "system_ext": true, "vendor": true, "product": true, "odm": true,
	"odm_dlkm": true, "vendor_dlkm": true, "system_dlkm": true, "my_product": true,
	"abl": true, "xbl": true, "xbl_config": true, "tz": true, "hyp": true, "aop": true,
	"devcfg": true, "dsp": true, "featenabler": true, "imagefv": true, "keymaster": true,
	"keystore": true, "qupfw": true, "uefisecapp": true, "cmnlib": true, "cmnlib64": true,
	"multiimgoem": true,
}

// SlotName 按需给分区名补槽位后缀。
func SlotName(partition, slot string) string {
	if slot == "" {
		return partition
	}
	if strings.HasSuffix(partition, "_a") || strings.HasSuffix(partition, "_b") {
		return partition
	}
	if !slottedPartitions[partition] {
		return partition
	}
	return partition + "_" + slot
}

// Generate 由线刷步骤生成脚本文件，返回写出的路径。
func Generate(steps []rom.FlashStep, outDir string, opt Options) ([]string, error) {
	if len(steps) == 0 {
		return nil, fmt.Errorf("没有可生成的刷机步骤")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}

	files := []struct {
		name    string
		kind    string
		mode    os.FileMode
		content string
	}{
		{"flash-all.bat", "bat", 0o644, Render(steps, outDir, opt, "bat")},
		{"flash-all.sh", "sh", 0o755, Render(steps, outDir, opt, "sh")},
		{"flash-all.txt", "txt", 0o644, Render(steps, outDir, opt, "txt")},
	}

	var out []string
	for _, f := range files {
		p := filepath.Join(outDir, f.name)
		if err := os.WriteFile(p, []byte(f.content), f.mode); err != nil {
			return out, err
		}
		out = append(out, p)
	}
	return out, nil
}

// GenerateFromDir 扫描镜像目录并生成脚本。
func GenerateFromDir(imagesDir, outDir string, opt Options) ([]string, error) {
	steps, err := rom.BuildFlashPlan(imagesDir)
	if err != nil {
		return nil, err
	}
	return Generate(steps, outDir, opt)
}

// Render 渲染脚本文本。kind 取 bat / sh / txt。
func Render(steps []rom.FlashStep, scriptDir string, opt Options, kind string) string {
	var b strings.Builder
	eol := "\n"
	if kind == "bat" {
		eol = "\r\n"
	}
	line := func(s string) {
		b.WriteString(s)
		b.WriteString(eol)
	}
	comment := func(s string) {
		if kind == "bat" {
			line("rem " + s)
		} else {
			line("# " + s)
		}
	}
	echo := func(s string) {
		if kind == "bat" {
			line("echo " + s)
		} else if kind == "sh" {
			line("echo \"" + s + "\"")
		}
	}

	switch kind {
	case "bat":
		line("@echo off")
		line("chcp 65001 >nul")
		line("cd /d \"%~dp0\"")
	case "sh":
		line("#!/usr/bin/env bash")
		line("set -e")
		line("cd \"$(dirname \"$0\")\"")
	}

	line("")
	comment("由 DroidForge 生成。刷写前请核对机型、分区名与槽位。")
	comment("需要 Bootloader 已解锁，且手机已进入 fastboot 模式。")
	line("")

	echo("检查设备连接...")
	line("fastboot devices")
	if kind == "bat" {
		line("if errorlevel 1 goto :fail")
	}
	line("")

	if opt.Wipe {
		echo("清空用户数据（fastboot -w）...")
		line("fastboot -w")
		if kind == "bat" {
			line("if errorlevel 1 goto :fail")
		}
		line("")
	}

	inFastbootd := false
	for _, s := range steps {
		name := SlotName(s.Partition, opt.Slot)
		if s.NeedsFastbootd && !inFastbootd {
			echo("切换到 fastbootd 以写入动态分区...")
			line("fastboot reboot fastboot")
			if kind == "bat" {
				line("timeout /t 8 /nobreak >nul")
			} else {
				line("sleep 8")
			}
			line("")
			inFastbootd = true
		}
		cmd := fmt.Sprintf("fastboot flash %s %s", name, quotePath(scriptDir, s.Image, kind))
		line(cmd)
		if kind == "bat" {
			line("if errorlevel 1 goto :fail")
		}
	}

	if opt.Reboot {
		line("")
		echo("刷写完成，重启到系统...")
		line("fastboot reboot")
	}

	switch kind {
	case "bat":
		line("")
		echo("全部完成。")
		line("goto :eof")
		line("")
		line(":fail")
		echo("刷写失败，请向上滚动查看具体是哪一步出错。")
		line("exit /b 1")
	case "sh":
		line("")
		echo("全部完成。")
	default:
		line("")
		line("# 以上命令可逐条粘贴执行")
	}
	return b.String()
}

// quotePath 生成脚本里的镜像路径，含空格时加引号。
func quotePath(scriptDir, image, kind string) string {
	p := image
	if rel, err := filepath.Rel(scriptDir, image); err == nil && !strings.HasPrefix(rel, "..") {
		p = rel
	} else if abs, err := filepath.Abs(image); err == nil {
		p = abs
	}
	if kind == "bat" {
		p = strings.ReplaceAll(p, "/", `\`)
	} else {
		p = filepath.ToSlash(p)
	}
	if strings.ContainsAny(p, " \t") {
		return `"` + p + `"`
	}
	return p
}
