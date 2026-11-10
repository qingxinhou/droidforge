package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"droidforge/internal/rom"
)

// ---------------------------------------------------------------------------
// Fastboot 操作
// ---------------------------------------------------------------------------

// FastbootDevices 列出 fastboot 设备。
func (a *App) FastbootDevices() ([]string, error) {
	if err := a.requireFastboot(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return a.fbClient().Devices(ctx)
}

// FastbootReboot 通过 fastboot 重启。
func (a *App) FastbootReboot(serial, target string) error {
	if err := a.requireFastboot(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	a.log("cmd", "fastboot reboot "+target)
	if err := a.fbClient().Reboot(ctx, serial, target); err != nil {
		a.log("err", err.Error())
		return err
	}
	a.log("ok", "重启命令已发送")
	return nil
}

// FlashPartition 通过 fastboot 刷入单个分区。
func (a *App) FlashPartition(serial, partition, imagePath string) error {
	if err := a.requireFastboot(); err != nil {
		return err
	}
	if _, err := os.Stat(imagePath); err != nil {
		return fmt.Errorf("镜像文件不存在：%s", imagePath)
	}
	ctx, cancel := a.opCtx()
	defer cancel()

	size := fileSize(imagePath)
	a.log("cmd", fmt.Sprintf("fastboot flash %s %s (%.1f MB)", partition, filepath.Base(imagePath), float64(size)/1024/1024))
	a.progress("刷入 "+partition, 5, false)
	if err := a.fbClient().Flash(ctx, serial, partition, imagePath, a.lineHandler()); err != nil {
		a.log("err", err.Error())
		a.progress("刷入 "+partition, 0, true)
		return err
	}
	a.progress("刷入 "+partition, 100, true)
	a.log("ok", "分区 "+partition+" 刷入完成")
	return nil
}

// ErasePartition 擦除分区。
func (a *App) ErasePartition(serial, partition string) error {
	if err := a.requireFastboot(); err != nil {
		return err
	}
	ctx, cancel := a.opCtx()
	defer cancel()
	a.log("cmd", "fastboot erase "+partition)
	if err := a.fbClient().Erase(ctx, serial, partition, a.lineHandler()); err != nil {
		a.log("err", err.Error())
		return err
	}
	a.log("ok", "已擦除 "+partition)
	return nil
}

// SetActiveSlot 切换 A/B 槽位。
func (a *App) SetActiveSlot(serial, slot string) error {
	if err := a.requireFastboot(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	a.log("cmd", "fastboot --set-active="+slot)
	if err := a.fbClient().SetActive(ctx, serial, slot); err != nil {
		a.log("err", err.Error())
		return err
	}
	a.log("ok", "已切换到槽位 "+slot)
	return nil
}

// FlashPlan 按步骤执行线刷。
func (a *App) FlashPlan(serial string, steps []rom.FlashStep) error {
	if err := a.requireFastboot(); err != nil {
		return err
	}
	ctx, cancel := a.opCtx()
	defer cancel()

	var normal, dynamic []rom.FlashStep
	for _, s := range steps {
		if s.NeedsFastbootd {
			dynamic = append(dynamic, s)
		} else {
			normal = append(normal, s)
		}
	}

	total := len(normal) + len(dynamic)
	done := 0
	flash := func(s rom.FlashStep) error {
		a.log("cmd", fmt.Sprintf("fastboot flash %s %s", s.Partition, filepath.Base(s.Image)))
		if err := a.fbClient().Flash(ctx, serial, s.Partition, s.Image, a.lineHandler()); err != nil {
			a.log("err", fmt.Sprintf("%s 刷入失败：%v", s.Partition, err))
			return err
		}
		done++
		a.progress("线刷", done*100/total, false)
		a.log("ok", s.Partition+" 完成")
		return nil
	}

	for _, s := range normal {
		if err := flash(s); err != nil {
			return err
		}
	}
	if len(dynamic) > 0 {
		a.log("info", "进入 fastbootd 刷入动态分区…")
		if err := a.fbClient().Reboot(ctx, serial, "fastboot"); err != nil {
			a.log("warn", "进入 fastbootd 失败，尝试继续："+err.Error())
		} else {
			time.Sleep(3 * time.Second)
		}
		for _, s := range dynamic {
			if err := flash(s); err != nil {
				return err
			}
		}
	}
	a.progress("线刷", 100, true)
	a.log("ok", "线刷流程完成，建议重启到系统")
	return nil
}
