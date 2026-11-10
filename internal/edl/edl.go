package edl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

// Transport 是 EDL 的字节通道。WinUSB 实现见 usb_windows.go，
// 测试里用 net.Pipe 模拟设备。
type Transport interface {
	io.Reader
	io.Writer
	io.Closer
}

// LogFunc 接收过程中的可读日志。
type LogFunc func(format string, args ...any)

func noopLog(string, ...any) {}

// UploadLoader 按 Sahara 协议把 loader 送进设备内存并让它跑起来。
//
// 时序（host 视角）：读 Hello → 回 Hello Response → 循环
// [发 ReadData 请求 → 收设备的数据请求 → 发这一块数据] → EndOfImage → 收 Done → 回 DoneResp。
//
// 最后**不发 Reset**：reset 会让设备重启，而我们要它停在 Firehose 模式。
func UploadLoader(t Transport, image []byte, imageID uint32, logf LogFunc) (*HelloInfo, error) {
	if logf == nil {
		logf = noopLog
	}
	if len(image) == 0 {
		return nil, errors.New("loader 文件是空的")
	}

	hdr := make([]byte, packetHdrLen)
	if err := readFull(t, hdr, controlIdle, "读取 Hello"); err != nil {
		return nil, fmt.Errorf("%w（设备可能不在 9008 模式）", err)
	}
	cmd, length, err := ParsePacketHeader(hdr)
	if err != nil {
		return nil, err
	}
	if cmd != SaharaHello {
		return nil, fmt.Errorf("期望 Hello(0x01)，实际收到 0x%02x。检查设备是否真的在 EDL 模式", cmd)
	}
	if length > 4096 || length < packetHdrLen {
		return nil, fmt.Errorf("Hello 报文长度不合理：%d", length)
	}
	full := make([]byte, length)
	copy(full, hdr)
	if err := readFull(t, full[packetHdrLen:], controlIdle, "读取 Hello 正文"); err != nil {
		return nil, err
	}
	hello, err := ParseHello(full)
	if err != nil {
		return nil, err
	}
	logf("设备握手：协议 v%d（最低 v%d），单包上限 %d 字节",
		hello.Version, hello.VersionMin, hello.MaxPacketSize)

	chunk := int(hello.MaxPacketSize)
	if chunk <= 0 {
		chunk = 4096
	}
	if chunk > 1<<20 {
		chunk = 1 << 20
	}

	if _, err := t.Write(EncodeHelloResp(hello, SaharaModeImageTxPending)); err != nil {
		return nil, fmt.Errorf("回 Hello 失败：%w", err)
	}

	total := len(image)
	offset := 0
	for offset < total {
		n := total - offset
		if n > chunk {
			n = chunk
		}
		if _, err := t.Write(EncodeReadData(imageID, uint32(offset), uint32(n))); err != nil {
			return nil, fmt.Errorf("请求发送数据失败：%w", err)
		}

		rh := make([]byte, packetHdrLen)
		if err := readFull(t, rh, controlIdle, fmt.Sprintf("偏移 %d 处等待数据请求", offset)); err != nil {
			return nil, err
		}
		rcmd, rlen, err := ParsePacketHeader(rh)
		if err != nil {
			return nil, err
		}
		if rcmd != SaharaReadData {
			return nil, fmt.Errorf("偏移 %d 处期望数据请求(0x03)，实际 0x%02x", offset, rcmd)
		}
		if rlen < readDataLen || rlen > 64 {
			return nil, fmt.Errorf("数据请求长度异常：%d", rlen)
		}
		body := make([]byte, rlen)
		copy(body, rh)
		if err := readFull(t, body[packetHdrLen:], controlIdle, "读取数据请求正文"); err != nil {
			return nil, err
		}
		_, devOffset, want, err := ParseDataRequest(body)
		if err != nil {
			return nil, err
		}
		if devOffset != uint32(offset) {
			logf("注意：设备请求的偏移是 %d，与本地 %d 不一致，以设备的为准", devOffset, offset)
			offset = int(devOffset)
			if offset >= total {
				break
			}
			n = total - offset
		}
		if want > 0 && int(want) < n {
			n = int(want)
		}
		if _, err := t.Write(image[offset : offset+n]); err != nil {
			return nil, fmt.Errorf("发送数据失败（偏移 %d）：%w", offset, err)
		}
		offset += n
	}

	if _, err := t.Write(EncodeEndOfImage(imageID, SaharaStatusSuccess)); err != nil {
		return nil, fmt.Errorf("结束传输失败：%w", err)
	}

	dh := make([]byte, packetHdrLen)
	if err := readFull(t, dh, controlIdle, "等待 Done"); err != nil {
		return nil, fmt.Errorf("%w（loader 可能是坏的或不匹配该机型）", err)
	}
	dcmd, dlen, err := ParsePacketHeader(dh)
	if err != nil {
		return nil, err
	}
	if dcmd != SaharaDone {
		return nil, fmt.Errorf("期望 Done(0x05)，实际 0x%02x", dcmd)
	}
	if dlen > packetHdrLen {
		skip := make([]byte, dlen-packetHdrLen)
		_ = readFull(t, skip, controlIdle, "读取 Done 正文")
	}
	if _, err := t.Write(EncodeDoneResp(SaharaStatusSuccess)); err != nil {
		return nil, fmt.Errorf("回 Done 失败：%w", err)
	}
	logf("loader 已加载（%d 字节），等待切换到 Firehose", total)
	return hello, nil
}

// Firehose 是加载完 loader 后的 XML 会话。
type Firehose struct {
	t          Transport
	MaxPayload int
	Timeout    time.Duration
	logf       LogFunc
}

// NewFirehose 创建一个会话。
func NewFirehose(t Transport, logf LogFunc) *Firehose {
	if logf == nil {
		logf = noopLog
	}
	return &Firehose{t: t, MaxPayload: 1 << 20, Timeout: 30 * time.Second, logf: logf}
}

// Send 发一条命令并等响应。
func (f *Firehose) Send(ctx context.Context, xml string) (FHResponse, error) {
	if err := f.checkCtx(ctx); err != nil {
		return FHResponse{}, err
	}
	if _, err := f.t.Write([]byte(xml)); err != nil {
		return FHResponse{}, fmt.Errorf("发送命令失败：%w", err)
	}
	resp, err := f.readResponse(ctx)
	if err != nil {
		return resp, err
	}
	for _, l := range resp.Logs {
		f.logf("设备日志：%s", l)
	}
	if !resp.OK() {
		return resp, fmt.Errorf("设备返回 %s", firstNonEmpty(resp.Value, "未知响应"))
	}
	return resp, nil
}

// readResponse 累积读取直到出现 response 标签。
func (f *Firehose) readResponse(ctx context.Context) (FHResponse, error) {
	var sb strings.Builder
	buf := make([]byte, 4096)
	deadline := time.Now().Add(f.Timeout)
	for {
		if err := f.checkCtx(ctx); err != nil {
			return FHResponse{Raw: sb.String()}, err
		}
		if sb.Len() > 0 && strings.Contains(sb.String(), "<response") {
			r := ParseFHResponse(sb.String())
			if r.Value != "" {
				return r, nil
			}
		}
		if time.Now().After(deadline) {
			return FHResponse{Raw: sb.String()}, fmt.Errorf("等待响应超时，已收到：%s", truncate(sb.String(), 200))
		}
		n, err := f.t.Read(buf)
		if n > 0 {
			sb.Write(buf[:n])
			continue
		}
		if err != nil {
			return FHResponse{Raw: sb.String()}, fmt.Errorf("读取响应失败：%w", err)
		}
	}
}

func (f *Firehose) checkCtx(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

// Configure 建立存储会话。
//
// 设备可以在响应里把 MaxPayloadSizeToTargetInBytes 降到比请求更小的值，
// 必须按它给的值切段，否则后续 program 会被拒或直接截断。
func (f *Firehose) Configure(ctx context.Context, opt ConfigureOptions) error {
	if opt.MaxPayload > 0 {
		f.MaxPayload = opt.MaxPayload
	}
	f.logf("配置存储会话：%s，单次上限 %d 字节", opt.MemoryName, f.MaxPayload)
	resp, err := f.Send(ctx, Configure(opt))
	if err != nil {
		return fmt.Errorf("configure 失败：%w。确认 loader 与机型匹配，必要时重新插拔设备再试", err)
	}
	if v := attrValueLast(resp.Raw, "MaxPayloadSizeToTargetInBytes"); v != "" {
		if n, perr := strconv.Atoi(strings.TrimSpace(v)); perr == nil && n > 0 && n < f.MaxPayload {
			f.logf("设备把单次上限降到 %d 字节", n)
			f.MaxPayload = n
		}
	}
	return nil
}

// WriteOptions 控制写入行为。
type WriteOptions struct {
	SectorSize  int
	LUN         int
	StartSector int64
	Sectors     int64
}

// WritePartition 把 r 的内容写进指定分区。
//
// 大镜像按 MaxPayload 对齐切段，每段一条 program，start_sector 递增。
func (f *Firehose) WritePartition(ctx context.Context, label string, r io.Reader, opt WriteOptions, onProgress func(int64)) error {
	if opt.SectorSize <= 0 {
		opt.SectorSize = 4096
	}
	if opt.Sectors <= 0 {
		return errors.New("未指定要写入的扇区数")
	}
	if onProgress == nil {
		onProgress = func(int64) {}
	}

	perRound := int64(f.MaxPayload / opt.SectorSize)
	if perRound <= 0 {
		perRound = 1
	}
	if perRound > opt.Sectors {
		perRound = opt.Sectors
	}

	buf := make([]byte, perRound*int64(opt.SectorSize))
	var written int64
	for done := int64(0); done < opt.Sectors; done += perRound {
		if err := f.checkCtx(ctx); err != nil {
			return err
		}
		n := perRound
		if done+n > opt.Sectors {
			n = opt.Sectors - done
		}
		want := n * int64(opt.SectorSize)

		chunk := buf[:want]
		// 镜像长度不一定按扇区对齐：最后一段短读是正常的，用零补满再写，
		// 否则非对齐镜像每次都在最后一块失败。buf 是复用的，尾部必须显式清零。
		nRead, rerr := io.ReadFull(r, chunk)
		if rerr != nil && !errors.Is(rerr, io.ErrUnexpectedEOF) && !errors.Is(rerr, io.EOF) {
			return fmt.Errorf("读取本地镜像在偏移 %d 处失败：%w", done*int64(opt.SectorSize), rerr)
		}
		for i := nRead; i < len(chunk); i++ {
			chunk[i] = 0
		}

		req := ProgramRequest{
			Label:            label,
			Filename:         "droidforge.bin",
			StartSector:      opt.StartSector + done,
			Sectors:          n,
			SectorSize:       opt.SectorSize,
			LUN:              opt.LUN,
			FileSectorOffset: done,
		}
		if _, err := f.Send(ctx, Program(req)); err != nil {
			return fmt.Errorf("program 请求被拒（分区 %s，扇区 %d）：%w", label, req.StartSector, err)
		}
		if _, err := f.t.Write(chunk); err != nil {
			return fmt.Errorf("发送分区数据失败：%w", err)
		}
		resp, err := f.readResponse(ctx)
		if err != nil {
			return fmt.Errorf("等待写入确认失败：%w", err)
		}
		if !resp.OK() {
			return fmt.Errorf("写入被拒绝：%s", firstNonEmpty(resp.Value, "NAK"))
		}
		written += want
		onProgress(written)
	}
	return nil
}

// ReadPartition 读取分区的原始内容。
func (f *Firehose) ReadPartition(ctx context.Context, label string, w io.Writer, opt WriteOptions, onProgress func(int64)) error {
	if opt.SectorSize <= 0 {
		opt.SectorSize = 4096
	}
	if opt.Sectors <= 0 {
		return errors.New("未指定要读取的扇区数")
	}
	if onProgress == nil {
		onProgress = func(int64) {}
	}

	perRound := int64(f.MaxPayload / opt.SectorSize)
	if perRound <= 0 {
		perRound = 1
	}
	if perRound > opt.Sectors {
		perRound = opt.Sectors
	}

	var total int64
	for done := int64(0); done < opt.Sectors; done += perRound {
		if err := f.checkCtx(ctx); err != nil {
			return err
		}
		n := perRound
		if done+n > opt.Sectors {
			n = opt.Sectors - done
		}
		req := ReadRequest{
			Label:       label,
			StartSector: opt.StartSector + done,
			Sectors:     n,
			SectorSize:  opt.SectorSize,
			LUN:         opt.LUN,
		}
		if _, err := f.Send(ctx, Read(req)); err != nil {
			return fmt.Errorf("read 请求被拒（分区 %s）：%w", label, err)
		}
		want := n * int64(opt.SectorSize)
		copied, err := copyN(ctx, w, f.t, want, dataIdle, fmt.Sprintf("读取分区 %s 的数据", label))
		if err != nil {
			return fmt.Errorf("已读 %d/%d 字节：%w", copied, want, err)
		}
		// 数据之后设备还会发一条 ACK。
		if _, err := f.readResponse(ctx); err != nil {
			return fmt.Errorf("读取数据后的确认失败：%w", err)
		}
		total += want
		onProgress(total)
	}
	return nil
}

// ErasePartition 擦除分区。
func (f *Firehose) ErasePartition(ctx context.Context, label string, e EraseRequest) error {
	if e.SectorSize <= 0 {
		e.SectorSize = 4096
	}
	if e.Sectors <= 0 {
		return errors.New("未指定要擦除的扇区数")
	}
	e.Label = label
	if _, err := f.Send(ctx, Erase(e)); err != nil {
		return fmt.Errorf("erase 请求被拒（分区 %s）：%w", label, err)
	}
	return nil
}

// Power 发送 reset 让设备重启。
func (f *Firehose) Power(ctx context.Context, action string) error {
	if action == "" {
		action = "reset"
	}
	xml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" ?><data><power value="%s" /></data>`, escapeXML(action))
	_, err := f.Send(ctx, xml)
	return err
}

// LoaderInfo 是从 loader 文件名里推出来的信息。
type LoaderInfo struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	Memory  string `json:"memory"`  // ufs / emmc
	Program string `json:"program"` // 机型代号
	Valid   bool   `json:"valid"`
	Note    string `json:"note"`
}

// InspectLoader 校验 loader 文件并猜出目标存储类型。
//
// 高通 loader 常见命名：prog_firehose_ddr.elf、prog_firehose_lite.elf、
// prog_emmc_firehose_*.mbn。UFS 机型的 loader 文件名里通常带 ufs 或 ddr。
func InspectLoader(path string) (*LoaderInfo, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	info := &LoaderInfo{Path: path, Name: st.Name(), Size: st.Size()}
	low := strings.ToLower(st.Name())

	if st.Size() < 1024 {
		info.Note = "文件太小，不像 loader"
		return info, nil
	}
	if !strings.HasSuffix(low, ".elf") && !strings.HasSuffix(low, ".mbn") && !strings.HasSuffix(low, ".bin") {
		info.Note = "扩展名不是 .elf / .mbn / .bin，仍会尝试发送"
	}
	switch {
	case strings.Contains(low, "emmc"):
		info.Memory = "emmc"
	case strings.Contains(low, "ufs"), strings.Contains(low, "ddr"):
		info.Memory = "ufs"
	default:
		info.Memory = "ufs"
		info.Note = "文件名里没有存储类型线索，按 UFS 处理；若 configure 失败可手动改成 emmc"
	}

	// prog_firehose_ddr.elf → ddr；prog_firehose_lite.elf → lite
	base := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(low, ".elf"), ".mbn"), ".bin")
	parts := strings.Split(base, "_")
	if len(parts) >= 2 {
		info.Program = parts[len(parts)-1]
	}
	info.Valid = true
	return info, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ParseSector 解析用户输入的扇区数，支持十进制与 0x 前缀。
func ParseSector(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("扇区数不能为空")
	}
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		return strconv.ParseInt(s[2:], 16, 64)
	}
	return strconv.ParseInt(s, 10, 64)
}
