package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"droidforge/internal/edl"
)

// ---------------------------------------------------------------------------
// EDL 9008
// ---------------------------------------------------------------------------

// edlRuntime 是一次已建立的 EDL 会话。
type edlRuntime struct {
	t      edl.Transport
	fh     *edl.Firehose
	gpt    *edl.GPT
	loader string
	device string
	sector int
}

// EdlScanDevices 列出处于 9008 模式的设备。
func (a *App) EdlScanDevices() ([]edl.Device, error) {
	devs, err := edl.FindEDL()
	if err != nil {
		a.log("err", err.Error())
		return nil, err
	}
	if len(devs) == 0 {
		a.log("warn", "没有找到 9008 设备。确认设备已进入 EDL 模式；Windows 上还需要用 Zadig 把驱动换成 WinUSB")
		return []edl.Device{}, nil
	}
	for _, d := range devs {
		a.log("ok", "找到 EDL 设备："+d.Name)
	}
	return devs, nil
}

// EdlInspectLoader 检查 loader 文件。
func (a *App) EdlInspectLoader(path string) (*edl.LoaderInfo, error) {
	info, err := edl.InspectLoader(path)
	if err != nil {
		a.log("err", err.Error())
		return nil, err
	}
	if info.Valid {
		a.log("ok", fmt.Sprintf("loader：%s（%d 字节，按 %s 处理）", info.Name, info.Size, info.Memory))
	} else {
		a.log("warn", info.Note)
	}
	return info, nil
}

// EdlConnect 连接设备、加载 loader 并建立 Firehose 会话。
func (a *App) EdlConnect(devicePath, loaderPath string) (*edl.GPT, error) {
	a.edlMu.Lock()
	defer a.edlMu.Unlock()

	if a.edlRT != nil {
		_ = a.edlRT.t.Close()
		a.edlRT = nil
	}
	if loaderPath == "" {
		return nil, fmt.Errorf("需要先指定 loader 文件（prog_firehose_*.elf）")
	}
	loader, err := os.ReadFile(loaderPath)
	if err != nil {
		a.log("err", "读取 loader 失败："+err.Error())
		return nil, err
	}
	info, _ := edl.InspectLoader(loaderPath)

	devs, err := edl.FindEDL()
	if err != nil {
		a.log("err", err.Error())
		return nil, err
	}
	var dev edl.Device
	found := false
	for _, d := range devs {
		if devicePath == "" || d.Path == devicePath {
			dev, found = d, true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("没有找到指定的 9008 设备，先点一次「重新扫描」")
	}

	a.log("info", "打开设备："+dev.Name)
	t, err := edl.Open(dev)
	if err != nil {
		a.log("err", err.Error())
		return nil, err
	}

	ctx, cancel := a.opCtx()
	defer cancel()

	a.log("info", "按 Sahara 协议发送 loader…")
	if _, err := edl.UploadLoader(t, loader, edl.ImageIDFlashProgrammer, func(f string, args ...any) {
		a.log("info", fmt.Sprintf(f, args...))
	}); err != nil {
		t.Close()
		a.log("err", "loader 加载失败："+err.Error())
		return nil, err
	}

	fh := edl.NewFirehose(t, func(f string, args ...any) {
		a.log("info", fmt.Sprintf(f, args...))
	})
	fh.MaxPayload = 1 << 20

	// 设备从 Sahara 切到 Firehose 需要一点时间。
	time.Sleep(600 * time.Millisecond)

	mem := "ufs"
	if info != nil && info.Memory != "" {
		mem = info.Memory
	}
	if err := fh.Configure(ctx, edl.ConfigureOptions{MemoryName: mem, MaxPayload: 1 << 20}); err != nil {
		t.Close()
		a.log("err", "configure 失败："+err.Error())
		return nil, err
	}

	rt := &edlRuntime{t: t, fh: fh, loader: loaderPath, device: dev.Name}
	gpt, gerr := readGPT(ctx, rt, 0)
	if gerr != nil {
		a.log("warn", "已连上设备，但没读到 GPT："+gerr.Error())
	} else {
		rt.gpt = gpt
		rt.sector = gpt.SectorSize
		a.log("ok", fmt.Sprintf("分区表已读取：%d 个分区，扇区大小 %d", len(gpt.Partitions), gpt.SectorSize))
	}
	a.edlRT = rt
	return gpt, nil
}

// EdlInitialized 判断会话是否已建立。
func (a *App) edlSession() (*edlRuntime, error) {
	if a.edlRT == nil {
		return nil, fmt.Errorf("还没有连接 EDL 设备")
	}
	return a.edlRT, nil
}

// EdlGPT 重新读取指定 LUN 的分区表。
func (a *App) EdlGPT(lun int) (*edl.GPT, error) {
	a.edlMu.Lock()
	defer a.edlMu.Unlock()
	rt, err := a.edlSession()
	if err != nil {
		return nil, err
	}
	ctx, cancel := a.opCtx()
	defer cancel()

	a.log("info", fmt.Sprintf("读取 LUN %d 的分区表…", lun))
	g, err := readGPT(ctx, rt, lun)
	if err != nil {
		a.log("err", err.Error())
		return nil, err
	}
	rt.gpt = g
	rt.sector = g.SectorSize
	a.log("ok", fmt.Sprintf("LUN %d：%d 个分区", lun, len(g.Partitions)))
	return g, nil
}

// readGPT 逐种扇区大小尝试解析分区表。
func readGPT(ctx context.Context, rt *edlRuntime, lun int) (*edl.GPT, error) {
	var lastErr error
	for _, ss := range []int{4096, 512} {
		const lbas = 34
		buf := &bytes.Buffer{}
		err := rt.fh.ReadPartition(ctx, "primary_gpt", buf, edl.WriteOptions{
			SectorSize:  ss,
			LUN:         lun,
			StartSector: 0,
			Sectors:     lbas,
		}, nil)
		if err != nil {
			lastErr = err
			continue
		}
		g, perr := edl.ParseGPT(buf.Bytes(), ss, lun)
		if perr != nil {
			lastErr = perr
			continue
		}
		return g, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("读不到分区表")
	}
	return nil, lastErr
}

// EdlReadPartition 把分区读成镜像文件。
func (a *App) EdlReadPartition(label, outPath string, lun int) (string, error) {
	a.edlMu.Lock()
	defer a.edlMu.Unlock()
	rt, err := a.edlSession()
	if err != nil {
		return "", err
	}
	if rt.gpt == nil {
		return "", fmt.Errorf("分区表还没读出来，先点一次「读取分区表」")
	}
	p, ok := rt.gpt.Find(label)
	if !ok {
		return "", fmt.Errorf("分区表里没有 %s", label)
	}
	if outPath == "" {
		outPath = filepath.Join(a.config().OutputDir, label+".img")
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return "", err
	}
	f, err := os.Create(outPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	ctx, cancel := a.opCtx()
	defer cancel()

	sector := rt.sector
	if sector <= 0 {
		sector = 4096
	}
	useLUN := lun
	if useLUN < 0 {
		useLUN = rt.gpt.LUN
	}

	a.log("info", fmt.Sprintf("读取 %s：%d 扇区（%.1f MB）", label, p.Sectors, float64(p.ByteSize(sector))/(1<<20)))
	total := p.ByteSize(sector)
	err = rt.fh.ReadPartition(ctx, label, f, edl.WriteOptions{
		SectorSize:  sector,
		LUN:         useLUN,
		StartSector: int64(p.FirstLBA),
		Sectors:     int64(p.Sectors),
	}, func(n int64) {
		if total > 0 {
			a.progress("读取 "+label, int(n*100/total), false)
		}
	})
	if err != nil {
		a.log("err", err.Error())
		return outPath, err
	}
	a.progress("读取完成", 100, true)
	a.log("ok", fmt.Sprintf("%s 已保存到 %s", label, outPath))
	return outPath, nil
}

// EdlWritePartition 把镜像写进分区。
func (a *App) EdlWritePartition(label, imgPath string, lun int) error {
	a.edlMu.Lock()
	defer a.edlMu.Unlock()
	rt, err := a.edlSession()
	if err != nil {
		return err
	}
	if rt.gpt == nil {
		return fmt.Errorf("分区表还没读出来，先点一次「读取分区表」")
	}
	p, ok := rt.gpt.Find(label)
	if !ok {
		return fmt.Errorf("分区表里没有 %s", label)
	}
	st, err := os.Stat(imgPath)
	if err != nil {
		return err
	}

	sector := rt.sector
	if sector <= 0 {
		sector = 4096
	}
	needSectors := (st.Size() + int64(sector) - 1) / int64(sector)
	if needSectors > int64(p.Sectors) {
		return fmt.Errorf("镜像 %d 字节需要 %d 个扇区，超过分区容量 %d 个扇区，写不下",
			st.Size(), needSectors, p.Sectors)
	}

	f, err := os.Open(imgPath)
	if err != nil {
		return err
	}
	defer f.Close()

	ctx, cancel := a.opCtx()
	defer cancel()

	useLUN := lun
	if useLUN < 0 {
		useLUN = rt.gpt.LUN
	}

	a.log("warn", fmt.Sprintf("开始写入 %s：%d 字节 / %d 扇区", label, st.Size(), needSectors))
	err = rt.fh.WritePartition(ctx, label, f, edl.WriteOptions{
		SectorSize:  sector,
		LUN:         useLUN,
		StartSector: int64(p.FirstLBA),
		Sectors:     needSectors,
	}, func(n int64) {
		if st.Size() > 0 {
			a.progress("写入 "+label, int(n*100/st.Size()), false)
		}
	})
	if err != nil {
		a.log("err", "写入失败："+err.Error())
		return err
	}
	a.progress("写入完成", 100, true)
	a.log("ok", fmt.Sprintf("%s 写入完成，共 %d 字节", label, st.Size()))
	return nil
}

// EdlErasePartition 擦除分区。
func (a *App) EdlErasePartition(label string, lun int) error {
	a.edlMu.Lock()
	defer a.edlMu.Unlock()
	rt, err := a.edlSession()
	if err != nil {
		return err
	}
	if rt.gpt == nil {
		return fmt.Errorf("分区表还没读出来")
	}
	p, ok := rt.gpt.Find(label)
	if !ok {
		return fmt.Errorf("分区表里没有 %s", label)
	}
	ctx, cancel := a.opCtx()
	defer cancel()

	useLUN := lun
	if useLUN < 0 {
		useLUN = rt.gpt.LUN
	}
	sector := rt.sector
	if sector <= 0 {
		sector = 4096
	}

	a.log("warn", "擦除 "+label)
	if err := rt.fh.ErasePartition(ctx, label, edl.EraseRequest{
		StartSector: int64(p.FirstLBA),
		Sectors:     int64(p.Sectors),
		SectorSize:  sector,
		LUN:         useLUN,
	}); err != nil {
		a.log("err", err.Error())
		return err
	}
	a.log("ok", label+" 已擦除")
	return nil
}

// EdlReboot 让设备退出 EDL 并重启。
func (a *App) EdlReboot() error {
	a.edlMu.Lock()
	defer a.edlMu.Unlock()
	rt, err := a.edlSession()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	a.log("info", "发送重启指令")
	if err := rt.fh.Power(ctx, "reset"); err != nil {
		a.log("warn", "设备没有回确认，通常会直接断开："+err.Error())
	}
	_ = rt.t.Close()
	a.edlRT = nil
	a.log("ok", "已发送重启，设备会重新枚举")
	return nil
}

// EdlDisconnect 主动断开并释放句柄。
func (a *App) EdlDisconnect() {
	a.edlMu.Lock()
	defer a.edlMu.Unlock()
	if a.edlRT == nil {
		return
	}
	_ = a.edlRT.t.Close()
	a.edlRT = nil
	a.log("info", "EDL 会话已断开")
}

// EdlStatus 返回当前会话的简要状态。
type EdlStatus struct {
	Connected  bool   `json:"connected"`
	Device     string `json:"device"`
	Loader     string `json:"loader"`
	Sector     int    `json:"sectorSize"`
	Partitions int    `json:"partitions"`
}

// EdlState 查询会话状态。
func (a *App) EdlState() EdlStatus {
	a.edlMu.Lock()
	defer a.edlMu.Unlock()
	if a.edlRT == nil {
		return EdlStatus{}
	}
	st := EdlStatus{Connected: true, Device: a.edlRT.device, Loader: filepath.Base(a.edlRT.loader), Sector: a.edlRT.sector}
	if a.edlRT.gpt != nil {
		st.Partitions = len(a.edlRT.gpt.Partitions)
	}
	return st
}
