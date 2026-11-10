package edl

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"unicode/utf16"
)

// Partition 是 GPT 里的一个分区。
type Partition struct {
	Index      int    `json:"index"`
	Name       string `json:"name"`
	TypeGUID   string `json:"typeGuid"`
	UniqueGUID string `json:"uniqueGuid"`
	FirstLBA   uint64 `json:"firstLba"`
	LastLBA    uint64 `json:"lastLba"`
	Sectors    uint64 `json:"sectors"`
	Attributes uint64 `json:"attributes"`
}

// SectorSize 返回分区占用的字节数（按给定扇区大小）。
func (p Partition) ByteSize(sectorSize int) int64 {
	return int64(p.Sectors) * int64(sectorSize)
}

// GPT 是解析后的分区表。
type GPT struct {
	SectorSize  int         `json:"sectorSize"`
	LUN         int         `json:"lun"`
	DiskGUID    string      `json:"diskGuid"`
	FirstUsable uint64      `json:"firstUsableLba"`
	LastUsable  uint64      `json:"lastUsableLba"`
	Partitions  []Partition `json:"partitions"`
	// ChecksumOK 表示 header 的 CRC 校验通过。不通过时仍返回结果，只是提示。
	ChecksumOK bool   `json:"checksumOk"`
	Note       string `json:"note"`
}

// errNoGPT 表示这块数据里没有 GPT。
var errNoGPT = errors.New("没有找到 GPT 头（EFI PART）")

// ParseGPT 从一段以 LBA0 开头的数据里解析 GPT。
//
// data 至少要覆盖 header 所在扇区与分区表项区。sectorSize 常见 4096（UFS）
// 或 512（eMMC）。header 在 LBA1，所以先在 sectorSize 偏移处找签名。
func ParseGPT(data []byte, sectorSize int, lun int) (*GPT, error) {
	if sectorSize <= 0 {
		return nil, errors.New("扇区大小必须大于 0")
	}
	if len(data) < 2*sectorSize {
		return nil, fmt.Errorf("数据只有 %d 字节，不足以容纳 GPT 头（需要至少 %d 字节）", len(data), 2*sectorSize)
	}

	hdrOff := sectorSize
	if !bytes.Equal(data[hdrOff:hdrOff+8], []byte("EFI PART")) {
		// 有些设备把 header 放在 512 偏移，即使逻辑扇区是 4096。
		if len(data) >= 1024 && bytes.Equal(data[512:520], []byte("EFI PART")) {
			hdrOff = 512
			sectorSize = 512
		} else {
			return nil, errNoGPT
		}
	}

	hdr := data[hdrOff : hdrOff+512]
	g := &GPT{SectorSize: sectorSize, LUN: lun}
	g.FirstUsable = binary.LittleEndian.Uint64(hdr[40:])
	g.LastUsable = binary.LittleEndian.Uint64(hdr[48:])
	g.DiskGUID = guidString(hdr[56:72])
	entriesLBA := binary.LittleEndian.Uint64(hdr[72:])
	numEntries := binary.LittleEndian.Uint32(hdr[80:])
	entrySize := binary.LittleEndian.Uint32(hdr[84:])
	entriesCRC := binary.LittleEndian.Uint32(hdr[88:])

	if numEntries == 0 || numEntries > 4096 {
		return nil, fmt.Errorf("分区表项数量异常：%d", numEntries)
	}
	if entrySize < 128 || entrySize > 4096 {
		return nil, fmt.Errorf("分区表项大小异常：%d", entrySize)
	}

	// 校验 header CRC：把 CRC 字段本身清零后计算。
	headerSize := binary.LittleEndian.Uint32(hdr[12:])
	if headerSize >= 92 && headerSize <= 512 {
		tmp := make([]byte, headerSize)
		copy(tmp, hdr[:headerSize])
		binary.LittleEndian.PutUint32(tmp[16:], 0)
		g.ChecksumOK = crc32IEEE(tmp) == binary.LittleEndian.Uint32(hdr[16:])
	}

	entriesOff := int(entriesLBA) * sectorSize
	need := int(numEntries) * int(entrySize)
	if entriesOff < 0 || entriesOff+need > len(data) {
		return nil, fmt.Errorf("分区表项区越界：offset=%d need=%d len=%d", entriesOff, need, len(data))
	}
	entries := data[entriesOff : entriesOff+need]

	if !g.ChecksumOK {
		g.Note = "GPT 头 CRC 不匹配，表可能被改写过；解析结果仅供参考"
	}
	if want := crc32IEEE(entries); want != entriesCRC {
		if g.Note != "" {
			g.Note += "；分区表项 CRC 也不匹配"
		} else {
			g.Note = "分区表项 CRC 不匹配，表可能被改写过"
		}
	}

	for i := 0; i < int(numEntries); i++ {
		e := entries[i*int(entrySize) : (i+1)*int(entrySize)]
		if isZeroGUID(e[0:16]) {
			continue
		}
		p := Partition{
			Index:      i,
			TypeGUID:   guidString(e[0:16]),
			UniqueGUID: guidString(e[16:32]),
			FirstLBA:   binary.LittleEndian.Uint64(e[32:]),
			LastLBA:    binary.LittleEndian.Uint64(e[40:]),
			Attributes: binary.LittleEndian.Uint64(e[48:]),
			Name:       utf16Name(e[56:128]),
		}
		if p.LastLBA >= p.FirstLBA {
			p.Sectors = p.LastLBA - p.FirstLBA + 1
		}
		g.Partitions = append(g.Partitions, p)
	}
	return g, nil
}

// Find 按名称查分区，不区分大小写。
func (g *GPT) Find(name string) (Partition, bool) {
	for _, p := range g.Partitions {
		if strings.EqualFold(p.Name, name) {
			return p, true
		}
	}
	return Partition{}, false
}

func isZeroGUID(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
}

// guidString 按 UEFI 的混合字节序格式化 GUID。
func guidString(b []byte) string {
	if len(b) < 16 {
		return ""
	}
	return fmt.Sprintf("%08X-%04X-%04X-%02X%02X-%02X%02X%02X%02X%02X%02X",
		binary.LittleEndian.Uint32(b[0:4]),
		binary.LittleEndian.Uint16(b[4:6]),
		binary.LittleEndian.Uint16(b[6:8]),
		b[8], b[9], b[10], b[11], b[12], b[13], b[14], b[15])
}

func utf16Name(b []byte) string {
	u := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		v := binary.LittleEndian.Uint16(b[i:])
		if v == 0 {
			break
		}
		u = append(u, v)
	}
	return string(utf16.Decode(u))
}

// crc32IEEE 用表驱动实现，避免为了一次校验引入额外依赖。
func crc32IEEE(data []byte) uint32 {
	crc := ^uint32(0)
	for _, b := range data {
		crc ^= uint32(b)
		for i := 0; i < 8; i++ {
			if crc&1 != 0 {
				crc = (crc >> 1) ^ 0xEDB88320
			} else {
				crc >>= 1
			}
		}
	}
	return ^crc
}
