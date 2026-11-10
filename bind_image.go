package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"droidforge/internal/image"
	"droidforge/internal/rom"
)

// ---------------------------------------------------------------------------
// 镜像 / 刷机包
// ---------------------------------------------------------------------------

// InspectPackage 解析刷机包。
func (a *App) InspectPackage(path string) (*rom.PackageInfo, error) {
	a.log("info", "解析刷机包："+filepath.Base(path))
	info, err := rom.Inspect(path)
	if err != nil {
		a.log("err", err.Error())
		return nil, err
	}
	a.log("ok", fmt.Sprintf("识别为 %s，共 %d 个条目", info.Kind, len(info.Entries)))
	return info, nil
}

// ExtractPackage 提取刷机包内的分区镜像。
func (a *App) ExtractPackage(path string, names []string, outDir string) ([]string, error) {
	if outDir == "" {
		outDir = a.config().OutputDir
	}
	a.log("info", "开始提取到 "+outDir)
	files, err := rom.Extract(path, names, outDir, func(name string, percent int) {
		a.progress(name, percent, false)
	})
	if err != nil {
		a.log("err", err.Error())
		return files, err
	}
	a.progress("提取完成", 100, true)
	a.log("ok", fmt.Sprintf("已提取 %d 个镜像", len(files)))
	return files, nil
}

// IdentifyImage 粗略识别镜像类型（boot / payload / sparse / zip / ext4 / erofs）。
func (a *App) IdentifyImage(path string) string {
	kind := image.IdentifyImage(path)
	a.log("info", fmt.Sprintf("%s → %s", filepath.Base(path), kind))
	return kind
}

// InspectBootImage 解析 boot / init_boot / vendor_boot 镜像。
func (a *App) InspectBootImage(path string) (*image.BootImageInfo, error) {
	info, err := image.ParseBoot(path)
	if err != nil {
		a.log("err", err.Error())
		return nil, err
	}
	a.log("ok", fmt.Sprintf("boot 镜像 v%d，内核 %d 字节，ramdisk %d 字节", info.HeaderVersion, info.KernelSize, info.RamdiskSize))
	return info, nil
}

// UnpackBootImage 拆分 boot 镜像。
func (a *App) UnpackBootImage(path, outDir string) ([]string, error) {
	if outDir == "" {
		outDir = filepath.Join(a.config().OutputDir, "boot_unpack_"+time.Now().Format("20060102_150405"))
	}
	files, err := image.UnpackBoot(path, outDir)
	if err != nil {
		a.log("err", err.Error())
		return nil, err
	}
	a.log("ok", fmt.Sprintf("已拆分出 %d 个文件到 %s", len(files), outDir))
	return files, nil
}

// InspectPayload 解析 payload.bin。
func (a *App) InspectPayload(path string) (*image.PayloadInfo, error) {
	info, err := image.ParsePayload(path)
	if err != nil {
		a.log("err", err.Error())
		return nil, err
	}
	a.log("ok", fmt.Sprintf("payload 版本 %d，共 %d 个分区", info.Version, len(info.Partitions)))
	return info, nil
}

// UnsparseImage 将 sparse 镜像还原为 raw。
func (a *App) UnsparseImage(src, dst string) (string, error) {
	if dst == "" {
		base := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
		dst = filepath.Join(a.config().OutputDir, base+"_raw.img")
	}
	_ = os.MkdirAll(filepath.Dir(dst), 0o755)
	a.log("info", "转换 sparse → raw")
	if err := image.Unsparse(src, dst, func(p int) { a.progress("转换镜像", p, false) }); err != nil {
		a.log("err", err.Error())
		return "", err
	}
	a.progress("转换镜像", 100, true)
	a.log("ok", "已输出 "+dst)
	return dst, nil
}

// BuildFlashPlan 扫描镜像目录生成线刷步骤。
func (a *App) BuildFlashPlan(dir string) ([]rom.FlashStep, error) {
	steps, err := rom.BuildFlashPlan(dir)
	if err != nil {
		a.log("err", err.Error())
		return nil, err
	}
	a.log("ok", fmt.Sprintf("生成 %d 步线刷流程", len(steps)))
	return steps, nil
}
