package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"droidforge/internal/flashscript"
	"droidforge/internal/lp"
)

// ---------------------------------------------------------------------------
// 动态分区（super）
// ---------------------------------------------------------------------------

// InspectSuper 解析 super 镜像里的 lp_metadata。
func (a *App) InspectSuper(path string) (*lp.Metadata, error) {
	a.log("info", "解析 super 镜像："+filepath.Base(path))
	meta, err := lp.Parse(path)
	if err != nil {
		a.log("err", err.Error())
		return nil, err
	}
	a.log("ok", fmt.Sprintf("lp_metadata v%d.%d，%d 个逻辑分区，%d 个组",
		meta.Major, meta.Minor, len(meta.Partitions), len(meta.Groups)))
	if !meta.ChecksumOK {
		a.log("warn", "metadata 校验和与内容不符，镜像可能被修改过或由第三方工具生成")
	}
	return meta, nil
}

// SplitSuper 把 super 里的逻辑分区导出为独立镜像。
func (a *App) SplitSuper(path string, names []string, outDir string) ([]lp.SplitResult, error) {
	if outDir == "" {
		outDir = filepath.Join(a.config().OutputDir, "super_split_"+time.Now().Format("20060102_150405"))
	}
	ctx, cancel := a.opCtx()
	defer cancel()

	a.log("info", fmt.Sprintf("拆分 super，输出到 %s", outDir))
	res, err := lp.Split(ctx, path, outDir, names, func(label string, percent int) {
		a.progress("拆分 "+label, percent, false)
	})
	if err != nil {
		a.log("err", err.Error())
		return res, err
	}
	var total int64
	for _, r := range res {
		total += r.Size
	}
	a.progress("拆分完成", 100, true)
	a.log("ok", fmt.Sprintf("已导出 %d 个分区，共 %.1f MB", len(res), float64(total)/(1<<20)))
	return res, nil
}

// ScanSuperParts 扫描目录里的分区镜像，作为合成 super 的候选项。
//
// 目录里若带 super_def / dynamic_partitions_info.txt，会按它自动填好分区组。
func (a *App) ScanSuperParts(dir string) ([]lp.Candidate, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		a.log("err", err.Error())
		return nil, err
	}

	var parts []lp.BuildPartition
	sizes := map[string]int64{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, ".img") && !strings.HasSuffix(lower, ".bin") {
			continue
		}
		// super.img 本身是产物，不作为输入。
		if strings.HasPrefix(lower, "super.") || lower == "super.img" {
			continue
		}
		base := name
		for _, ext := range []string{".img", ".bin", ".IMG", ".BIN"} {
			base = strings.TrimSuffix(base, ext)
		}
		p := filepath.Join(dir, name)
		parts = append(parts, lp.BuildPartition{Name: base, Path: p})
		if fi, err := e.Info(); err == nil {
			sizes[base] = fi.Size()
		}
	}
	if len(parts) == 0 {
		return nil, fmt.Errorf("目录里没有可用的分区镜像")
	}
	sort.SliceStable(parts, func(i, j int) bool { return parts[i].Name < parts[j].Name })

	def, err := lp.LoadDefinition(dir)
	if err != nil {
		a.log("warn", "读取动态分区定义失败："+err.Error())
	}
	if def != nil {
		parts = lp.ApplyDefinition(parts, def)
		a.log("ok", fmt.Sprintf("已读取 %s，按其中的分组信息归类", def.Source))
	} else {
		a.log("info", "目录里没有 super_def，分区将统一归入 default 组")
	}

	out := make([]lp.Candidate, 0, len(parts))
	for _, p := range parts {
		out = append(out, lp.Candidate{Name: p.Name, Path: p.Path, Group: p.Group, SizeBytes: sizes[p.Name]})
	}
	a.log("ok", fmt.Sprintf("找到 %d 个候选分区", len(out)))
	return out, nil
}

// MergeSuper 把散包分区合成为一个 super.img。
func (a *App) MergeSuper(parts []lp.BuildPartition, outPath string, opt lp.BuildOptions) (*lp.Metadata, error) {
	if outPath == "" {
		outPath = filepath.Join(a.config().OutputDir, "super.img")
	}
	ctx, cancel := a.opCtx()
	defer cancel()

	_ = os.MkdirAll(filepath.Dir(outPath), 0o755)
	a.log("info", fmt.Sprintf("合成 super：%d 个分区 → %s", len(parts), outPath))

	meta, err := lp.Build(ctx, parts, outPath, opt, func(label string, percent int) {
		a.progress("写入 "+label, percent, false)
	})
	if err != nil {
		a.log("err", err.Error())
		return nil, err
	}
	a.progress("合成完成", 100, true)
	st, _ := os.Stat(outPath)
	size := int64(0)
	if st != nil {
		size = st.Size()
	}
	a.log("ok", fmt.Sprintf("已生成 super.img，%.2f GB，%d 个分区", float64(size)/(1<<30), len(meta.Partitions)))
	return meta, nil
}

// ---------------------------------------------------------------------------
// 线刷脚本
// ---------------------------------------------------------------------------

// GenerateFlashScript 扫描镜像目录并生成 bat / sh / txt 三种线刷脚本。
func (a *App) GenerateFlashScript(imagesDir, outDir, slot string, reboot, wipe bool) ([]string, error) {
	if outDir == "" {
		outDir = imagesDir
	}
	a.log("info", "生成线刷脚本："+outDir)
	files, err := flashscript.GenerateFromDir(imagesDir, outDir, flashscript.Options{
		Slot:   slot,
		Reboot: reboot,
		Wipe:   wipe,
	})
	if err != nil {
		a.log("err", err.Error())
		return nil, err
	}
	a.log("ok", fmt.Sprintf("已生成 %d 个脚本文件", len(files)))
	return files, nil
}
