package main

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"droidforge/internal/backup"
)

// ---------------------------------------------------------------------------
// 备份助手
// ---------------------------------------------------------------------------

// BackupCategories 返回可备份的数据类别。
func (a *App) BackupCategories() []backup.Category {
	return backup.Categories()
}

// ScanBackup 统计设备上各类数据的占用，用于备份前确认。
func (a *App) ScanBackup(serial string) (*backup.Plan, error) {
	if err := a.requireAdb(); err != nil {
		return nil, err
	}
	a.log("info", "统计设备数据占用…")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	plan, err := backup.Scan(ctx, a.toolset().AdbPath, serial)
	if err != nil {
		a.log("err", "扫描失败："+err.Error())
		return nil, err
	}
	// 这里原先把 TotalFiles 当成「项数」，而文件类别的 Files 一直是 0，
	// 于是真机日志里出现「共 1 项可备份，预估 0.2 MB」这种自相矛盾的数字。
	// 「项」应该是类别数，文件数单独说。
	cats := 0
	for _, c := range plan.Categories {
		if c.Exists {
			cats++
		}
	}
	a.log("ok", fmt.Sprintf("共 %d 项可备份（%d 个文件），预估 %.1f MB",
		cats, plan.TotalFiles, float64(plan.TotalSize)/(1<<20)))
	for _, c := range plan.Categories {
		if !c.Exists && c.Note != "" {
			a.log("warn", c.Label+"："+c.Note)
		}
	}
	return plan, nil
}

// RunBackup 执行备份。
//
// 目录为空时落在 <输出目录>/backup_<时间戳>。
func (a *App) RunBackup(serial string, opt backup.Options) (*backup.Result, error) {
	if err := a.requireAdb(); err != nil {
		return nil, err
	}
	if opt.OutDir == "" {
		opt.OutDir = filepath.Join(a.config().OutputDir, "backup_"+time.Now().Format("20060102_150405"))
	}

	ctx, cancel := a.opCtx()
	defer cancel()

	a.log("info", "开始备份到 "+opt.OutDir)
	plan, err := backup.Scan(ctx, a.toolset().AdbPath, serial)
	if err != nil {
		a.log("err", "扫描失败："+err.Error())
		return nil, err
	}
	if len(opt.CategoryIDs) > 0 {
		a.log("info", fmt.Sprintf("已选 %d 个类别，预估 %.1f MB",
			len(opt.CategoryIDs), float64(plan.TotalOf(opt.CategoryIDs))/(1<<20)))
	}

	res, err := backup.Run(ctx, a.toolset().AdbPath, serial, plan, opt,
		func(label string, percent int) {
			a.progress(label, percent, percent >= 100)
		},
		func(line string) {
			a.log("out", line)
		})
	if err != nil {
		a.log("err", "备份中断："+err.Error())
		return res, err
	}

	a.log("ok", fmt.Sprintf("备份完成：%d 个文件，共 %.1f MB（跳过 %d 个已存在文件）",
		res.Files, float64(res.Bytes)/(1<<20), res.Skipped))
	for _, c := range res.Categories {
		if c.Error != "" {
			a.log("warn", c.Label+"："+c.Error)
		}
	}
	if res.Manifest != "" {
		a.log("info", "清单已写入 "+res.Manifest)
	}
	return res, nil
}
