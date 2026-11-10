package main

import (
	"fmt"
	"path/filepath"

	"droidforge/internal/arb"
	"droidforge/internal/avb"
)

// ---------------------------------------------------------------------------
// vbmeta / AVB 分析
// ---------------------------------------------------------------------------

// VBMetaReport 是给前端的 vbmeta 概览 + 一句话结论。
type VBMetaReport struct {
	Image   *avb.Image `json:"image"`
	Summary string     `json:"summary"`
	// Level 用于前端着色：ok / warn / risk
	Level string `json:"level"`
}

// InspectVBMeta 解析 vbmeta 镜像（支持裸 vbmeta 与带 AVBf footer 的 boot/system 镜像）。
func (a *App) InspectVBMeta(path string) (*VBMetaReport, error) {
	a.log("info", "解析 vbmeta："+filepath.Base(path))
	img, err := avb.Parse(path)
	if err != nil {
		a.log("err", err.Error())
		return nil, err
	}

	rep := &VBMetaReport{Image: img, Level: "ok"}
	verified := !img.VerificationDisabled && img.AlgorithmType != 0 && !img.HashtreeDisabled
	switch {
	case img.AlgorithmType == 0:
		rep.Level = "risk"
		rep.Summary = "未签名的 vbmeta，刷入后启动校验不会拦下任何改动"
	case img.VerificationDisabled:
		rep.Level = "warn"
		rep.Summary = "校验已被关闭，分区改动不会导致启动失败，也不会被发现"
	case img.HashtreeDisabled:
		rep.Level = "warn"
		rep.Summary = "dm-verity 已关闭，system/vendor 的改动无法检测"
	case verified:
		rep.Summary = fmt.Sprintf("已签名（%s），校验开启，rollback index %d",
			img.Algorithm, img.RollbackIndex)
	}

	a.log("ok", fmt.Sprintf("vbmeta v%d.%d，%s，rollback index %d，%d 个描述符",
		img.VersionMajor, img.VersionMinor, img.Algorithm, img.RollbackIndex, len(img.Descriptors)))
	for _, n := range img.Notes {
		a.log("warn", n)
	}
	return rep, nil
}

// ---------------------------------------------------------------------------
// ARB 防回滚
// ---------------------------------------------------------------------------

// ARBReport 是扫描结果 + 结论。
type ARBReport struct {
	File    *arb.FileReport `json:"file"`
	Verdict arb.Verdict     `json:"verdict"`
	// Guest 是启发式猜出来的 ARB 版本，取候选值里出现次数最多的。
	Guest   int64 `json:"guest"`
	GuestOK bool  `json:"guestOk"`
}

// ScanARBImage 扫描引导镜像里的 anti-rollback 信息。
func (a *App) ScanARBImage(path string) (*ARBReport, error) {
	a.log("info", "扫描 ARB 标记："+filepath.Base(path))
	file, err := arb.ScanFile(path)
	if err != nil {
		a.log("err", err.Error())
		return nil, err
	}

	rep := &ARBReport{File: file}
	if len(file.Values) > 0 {
		rep.Guest = int64(file.Values[0])
		rep.GuestOK = file.Confident == "high" || file.Confident == "medium"
	}
	rep.Verdict = arb.Compare(-1, rep.Guest)
	if rep.GuestOK {
		rep.Verdict = arb.Verdict{
			Level:  "ok",
			Title:  fmt.Sprintf("疑似 ARB 版本 %d", rep.Guest),
			Detail: "这是从引导镜像里按标记启发式读出的值，置信度 " + confidLabel(file.Confident) + "。要判断能不能降级，请再扫一次设备当前固件的同类镜像做对比。",
		}
	}

	switch file.Confident {
	case "none":
		a.log("warn", "未找到 anti-rollback 标记")
	case "low":
		a.log("warn", file.Note)
	default:
		a.log("ok", fmt.Sprintf("疑似 ARB 版本 %d（置信度 %s）", rep.Guest, confidLabel(file.Confident)))
	}
	return rep, nil
}

// CompareARB 对比两侧的 ARB 版本，给出降级结论。
func (a *App) CompareARB(current, target int64) arb.Verdict {
	v := arb.Compare(current, target)
	a.log("info", fmt.Sprintf("ARB 对比：当前 %d、目标 %d → %s", current, target, v.Title))
	return v
}

func confidLabel(c string) string {
	switch c {
	case "high":
		return "高"
	case "medium":
		return "中"
	case "low":
		return "低"
	default:
		return "无"
	}
}
