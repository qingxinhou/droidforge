// Package edl 实现高通 9008（EDL / QDL）模式下的刷写协议。
//
// 分三层：
//   - protocol：Sahara 与 Firehose 的报文编解码，纯函数，可单测；
//   - session：按协议时序编排握手、加载 loader、读写分区；
//   - usb：Windows 下的 WinUSB 传输实现。
//
// 关于驱动：Windows 上 9008 设备默认挂的是「Qualcomm HS-USB QDLoader 9008」
// 串口驱动，这个驱动不提供批量端点，无法走本包。需要先用 Zadig 把它换成
// WinUSB，本包才能通过 SetupAPI + WinUSB 访问。
package edl

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

// Sahara 命令字，定义来自高通的 sahara_protocol.h。
const (
	SaharaHello       = 0x01
	SaharaHelloResp   = 0x02
	SaharaReadData    = 0x03
	SaharaEndOfImage  = 0x04
	SaharaDone        = 0x05
	SaharaDoneResp    = 0x06
	SaharaReset       = 0x07
	SaharaResetResp   = 0x08
	SaharaCommandRdy  = 0x09
	SaharaCommandExec = 0x0A
	SaharaCmdExecResp = 0x0B
	SaharaSwitchMode  = 0x0C
	SaharaExecData    = 0x0D
	SaharaMemoryDebug = 0x0E
	SaharaMemoryRead  = 0x0F
)

// Sahara 传输模式。
const (
	SaharaModeImageTxPending  = 0
	SaharaModeCommand         = 1
	SaharaModeImageTxComplete = 2
)

// Sahara 状态码。
const (
	SaharaStatusSuccess    = 0x00
	SaharaStatusInvalidCmd = 0x1D
)

// HelloInfo 是设备上电后主动发来的能力声明。
type HelloInfo struct {
	Version       uint32
	VersionMin    uint32
	MaxPacketSize uint32
	Mode          uint32
}

// 各类 Sahara 报文的固定长度。
const (
	helloLen      = 48
	helloRespLen  = 48
	readDataLen   = 20
	endOfImageLen = 16
	doneRespLen   = 12
	packetHdrLen  = 8
)

// ParsePacketHeader 读出 command 与 length。
func ParsePacketHeader(b []byte) (cmd, length uint32, err error) {
	if len(b) < packetHdrLen {
		return 0, 0, errors.New("报文不足 8 字节")
	}
	cmd = binary.LittleEndian.Uint32(b[0:])
	length = binary.LittleEndian.Uint32(b[4:])
	if length < packetHdrLen {
		return cmd, length, fmt.Errorf("报文声明的长度为 %d，小于头部长度", length)
	}
	return cmd, length, nil
}

// ParseHello 解析 Hello 报文（含 8 字节头，共 48 字节）。
func ParseHello(b []byte) (*HelloInfo, error) {
	cmd, length, err := ParsePacketHeader(b)
	if err != nil {
		return nil, err
	}
	if cmd != SaharaHello {
		return nil, fmt.Errorf("期望 Hello(0x01)，实际是 0x%02x", cmd)
	}
	if length < helloLen || len(b) < helloLen {
		return nil, fmt.Errorf("Hello 报文长度异常：声明 %d，实际 %d", length, len(b))
	}
	return &HelloInfo{
		Version:       binary.LittleEndian.Uint32(b[8:]),
		VersionMin:    binary.LittleEndian.Uint32(b[12:]),
		MaxPacketSize: binary.LittleEndian.Uint32(b[16:]),
		Mode:          binary.LittleEndian.Uint32(b[20:]),
	}, nil
}

// EncodeHelloResp 生成 Hello 响应。
//
// 设备在 Hello 里报的 max_packet_size 决定了后续数据块的上限，
// 这里原样回填，避免设备按更小的值截断。
func EncodeHelloResp(hello *HelloInfo, mode uint32) []byte {
	b := make([]byte, helloRespLen)
	binary.LittleEndian.PutUint32(b[0:], SaharaHelloResp)
	binary.LittleEndian.PutUint32(b[4:], helloRespLen)
	binary.LittleEndian.PutUint32(b[8:], hello.Version)
	binary.LittleEndian.PutUint32(b[12:], hello.VersionMin)
	binary.LittleEndian.PutUint32(b[16:], hello.MaxPacketSize)
	binary.LittleEndian.PutUint32(b[20:], mode)
	return b
}

// EncodeReadData 请求设备接收一段数据。
func EncodeReadData(imageID, offset, length uint32) []byte {
	b := make([]byte, readDataLen)
	binary.LittleEndian.PutUint32(b[0:], SaharaReadData)
	binary.LittleEndian.PutUint32(b[4:], readDataLen)
	binary.LittleEndian.PutUint32(b[8:], imageID)
	binary.LittleEndian.PutUint32(b[12:], offset)
	binary.LittleEndian.PutUint32(b[16:], length)
	return b
}

// EncodeEndOfImage 结束一个镜像的传输。
func EncodeEndOfImage(imageID, status uint32) []byte {
	b := make([]byte, endOfImageLen)
	binary.LittleEndian.PutUint32(b[0:], SaharaEndOfImage)
	binary.LittleEndian.PutUint32(b[4:], endOfImageLen)
	binary.LittleEndian.PutUint32(b[8:], imageID)
	binary.LittleEndian.PutUint32(b[12:], status)
	return b
}

// EncodeDoneResp 回应设备的 Done。
func EncodeDoneResp(status uint32) []byte {
	b := make([]byte, doneRespLen)
	binary.LittleEndian.PutUint32(b[0:], SaharaDoneResp)
	binary.LittleEndian.PutUint32(b[4:], doneRespLen)
	binary.LittleEndian.PutUint32(b[8:], status)
	return b
}

// EncodeReset 让设备复位到正常启动。
func EncodeReset() []byte {
	b := make([]byte, packetHdrLen)
	binary.LittleEndian.PutUint32(b[0:], SaharaReset)
	binary.LittleEndian.PutUint32(b[4:], packetHdrLen)
	return b
}

// ImageID 是 loader 的镜像编号。1–11 是高通内部保留，13 是
// 「闪存编程器」（prog_firehose_*.elf），其余自定义镜像用它。
const ImageIDFlashProgrammer = 13

// ParseDataRequest 解析设备回的数据请求，返回它愿意接收的长度。
func ParseDataRequest(b []byte) (imageID, offset, length uint32, err error) {
	cmd, _, err := ParsePacketHeader(b)
	if err != nil {
		return 0, 0, 0, err
	}
	if cmd != SaharaReadData {
		return 0, 0, 0, fmt.Errorf("期望 ReadData(0x03)，实际是 0x%02x", cmd)
	}
	if len(b) < readDataLen {
		return 0, 0, 0, fmt.Errorf("数据请求报文只有 %d 字节", len(b))
	}
	return binary.LittleEndian.Uint32(b[8:]),
		binary.LittleEndian.Uint32(b[12:]),
		binary.LittleEndian.Uint32(b[16:]), nil
}

// ---------------------------------------------------------------------------
// Firehose
// ---------------------------------------------------------------------------

// FHResponse 是一条 Firehose 响应。
type FHResponse struct {
	Value string   `json:"value"` // ACK / NAK
	Logs  []string `json:"logs"`
	Raw   string   `json:"raw"`
}

// OK 表示设备确认执行。
func (r FHResponse) OK() bool { return strings.EqualFold(r.Value, "ACK") }

// ParseFHResponse 从设备返回的 XML 片段里取 response 与 log。
//
// Firehose 的输出可能一次返回多行（log 在前、response 在后），
// 也可能被拆成多个包，所以这里按字符串扫描而不是严格解析 XML。
func ParseFHResponse(raw string) FHResponse {
	resp := FHResponse{Raw: raw}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if v := attrValue(line, "value"); v != "" && strings.Contains(line, "<response") {
			resp.Value = v
		}
		if strings.Contains(line, "<log") {
			if v := attrValue(line, "value"); v != "" {
				resp.Logs = append(resp.Logs, v)
			}
		}
	}
	return resp
}

// attrValue 取 XML 片段里某个属性的值，支持单双引号。
func attrValue(s, name string) string {
	for _, prefix := range []string{name + `="`, name + `='`} {
		i := strings.Index(s, prefix)
		if i < 0 {
			continue
		}
		rest := s[i+len(prefix):]
		quote := prefix[len(prefix)-1]
		j := strings.IndexByte(rest, quote)
		if j < 0 {
			continue
		}
		return rest[:j]
	}
	return ""
}

// ConfigureOptions 是 Firehose 的会话配置。
type ConfigureOptions struct {
	MemoryName      string // ufs / emmc / nand
	MaxPayload      int    // 单次传输上限，常见 1 MB
	SkipStorageInit bool
	ZlpAwareHost    bool
}

// Configure 生成 configure 请求。
func Configure(opt ConfigureOptions) string {
	if opt.MemoryName == "" {
		opt.MemoryName = "ufs"
	}
	if opt.MaxPayload <= 0 {
		opt.MaxPayload = 1 << 20
	}
	return fmt.Sprintf(
		`<?xml version="1.0" encoding="UTF-8" ?><data>`+
			`<configure MemoryName="%s" Verbose="0" AlwaysValidate="0" MaxDigestTableSizeInBytes="8192" `+
			`MaxPayloadSizeToTargetInBytes="%d" ZlpAwareHost="%d" SkipStorageInit="%d" />`+
			`</data>`,
		opt.MemoryName, opt.MaxPayload, b2i(opt.ZlpAwareHost), b2i(opt.SkipStorageInit))
}

// ProgramRequest 是一次分区写入。
type ProgramRequest struct {
	Label       string
	Filename    string
	StartSector int64
	Sectors     int64
	SectorSize  int
	LUN         int
	// FileSectorOffset 用于把一个大镜像拆成多段写入，单位是扇区。
	FileSectorOffset int64
}

// Program 生成 program 请求。
func Program(p ProgramRequest) string {
	if p.SectorSize <= 0 {
		p.SectorSize = 4096
	}
	return fmt.Sprintf(
		`<?xml version="1.0" encoding="UTF-8" ?><data>`+
			`<program SECTOR_SIZE_IN_BYTES="%d" file_sector_offset="%d" filename="%s" label="%s" `+
			`num_partition_sectors="%d" physical_partition_number="%d" start_sector="%d" />`+
			`</data>`,
		p.SectorSize, p.FileSectorOffset, escapeXML(p.Filename), escapeXML(p.Label),
		p.Sectors, p.LUN, p.StartSector)
}

// ReadRequest 是一次分区读取。
type ReadRequest struct {
	Label       string
	StartSector int64
	Sectors     int64
	SectorSize  int
	LUN         int
}

// Read 生成 read 请求。
func Read(r ReadRequest) string {
	if r.SectorSize <= 0 {
		r.SectorSize = 4096
	}
	return fmt.Sprintf(
		`<?xml version="1.0" encoding="UTF-8" ?><data>`+
			`<read SECTOR_SIZE_IN_BYTES="%d" label="%s" num_partition_sectors="%d" `+
			`physical_partition_number="%d" start_sector="%d" />`+
			`</data>`,
		r.SectorSize, escapeXML(r.Label), r.Sectors, r.LUN, r.StartSector)
}

// EraseRequest 是一次分区擦除。
type EraseRequest struct {
	Label       string
	StartSector int64
	Sectors     int64
	SectorSize  int
	LUN         int
}

// Erase 生成 erase 请求。
func Erase(e EraseRequest) string {
	if e.SectorSize <= 0 {
		e.SectorSize = 4096
	}
	return fmt.Sprintf(
		`<?xml version="1.0" encoding="UTF-8" ?><data>`+
			`<erase SECTOR_SIZE_IN_BYTES="%d" label="%s" num_partition_sectors="%d" `+
			`physical_partition_number="%d" start_sector="%d" />`+
			`</data>`,
		e.SectorSize, escapeXML(e.Label), e.Sectors, e.LUN, e.StartSector)
}

// Reset 让设备退出 EDL 并重启。
func Reset() string {
	return `<?xml version="1.0" encoding="UTF-8" ?><data><power value="reset" /></data>`
}

func escapeXML(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
