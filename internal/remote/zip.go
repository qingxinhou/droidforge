package remote

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"path"
	"strings"
)

// zip 魔数
const (
	sigEOCD     = 0x06054b50 // PK\005\006 目录结束记录
	sigEOCD64   = 0x06064b50 // PK\006\006 ZIP64 目录结束记录
	sigEOCD64Lo = 0x07064b50 // PK\006\007 ZIP64 定位器
	sigCEN      = 0x02014b50 // PK\001\002 中央目录条目
	sigLOC      = 0x04034b50 // PK\003\004 本地文件头
)

const (
	eocdLen     = 22
	zip64LocLen = 20
	maxCDSize   = 128 << 20 // 中央目录上限，防止异常文件导致巨额分配
)

// ZipEntry 是 zip 中一个条目的关键信息。
type ZipEntry struct {
	Name             string
	Method           uint16
	CompressedSize   int64
	UncompressedSize int64
	LocalOffset      int64
	DataOffset       int64
}

// Stored 表示条目未压缩，可以按字节区间直接读取。
func (e *ZipEntry) Stored() bool { return e.Method == 0 }

func be32(b []byte) uint32 { return binary.LittleEndian.Uint32(b) }
func be16(b []byte) uint16 { return binary.LittleEndian.Uint16(b) }

// centralDirectory 远程定位 zip 的中央目录位置与大小。
func (f *File) centralDirectory() (int64, int64, error) {
	if f.Size < eocdLen {
		return 0, 0, errors.New("文件太小，不是有效的 zip")
	}
	tailLen := int64(65557) // 22 字节 EOCD + 最大 65535 字节注释
	if tailLen > f.Size {
		tailLen = f.Size
	}
	tail := make([]byte, tailLen)
	if _, err := f.ReadAt(tail, f.Size-tailLen); err != nil {
		return 0, 0, fmt.Errorf("读取 zip 尾部失败：%w", err)
	}

	idx := bytes.LastIndex(tail, []byte{'P', 'K', 0x05, 0x06})
	if idx < 0 {
		return 0, 0, errors.New("文件不是 zip 格式（未找到目录结束记录）")
	}

	e := tail[idx:]
	if len(e) < eocdLen {
		return 0, 0, errors.New("zip 目录结束记录不完整")
	}
	entries := int64(be16(e[10:12]))
	cdSize := int64(be32(e[12:16]))
	cdOffset := int64(be32(e[16:20]))

	// ZIP64：字段被置为 0xFFFFFFFF，真实值在 ZIP64 结构里。
	if cdSize == 0xFFFFFFFF || cdOffset == 0xFFFFFFFF || entries == 0xFFFF {
		size, offset, err := f.zip64Directory(tail, idx)
		if err != nil {
			return 0, 0, err
		}
		cdSize, cdOffset = size, offset
	}

	if cdSize <= 0 || cdSize > maxCDSize {
		return 0, 0, fmt.Errorf("zip 中央目录尺寸异常：%d", cdSize)
	}
	if cdOffset < 0 || cdOffset+cdSize > f.Size {
		return 0, 0, errors.New("zip 中央目录位置超出文件范围（可能是分卷压缩包）")
	}
	return cdOffset, cdSize, nil
}

func (f *File) zip64Directory(tail []byte, eocdIdx int) (int64, int64, error) {
	locIdx := eocdIdx - zip64LocLen
	if locIdx < 0 || be32(tail[locIdx:]) != sigEOCD64Lo {
		return 0, 0, errors.New("zip64 定位器缺失，无法解析该压缩包")
	}
	z64Off := int64(binary.LittleEndian.Uint64(tail[locIdx+8 : locIdx+16]))
	if z64Off <= 0 || z64Off+56 > f.Size {
		return 0, 0, errors.New("zip64 目录结束记录位置异常")
	}
	rec := make([]byte, 56)
	if _, err := f.ReadAt(rec, z64Off); err != nil {
		return 0, 0, fmt.Errorf("读取 zip64 目录结束记录失败：%w", err)
	}
	if be32(rec) != sigEOCD64 {
		return 0, 0, errors.New("zip64 目录结束记录签名不匹配")
	}
	cdSize := int64(binary.LittleEndian.Uint64(rec[40:48]))
	cdOffset := int64(binary.LittleEndian.Uint64(rec[48:56]))
	return cdSize, cdOffset, nil
}

// FindEntry 在远程 zip 中查找指定文件名（按 basename 不区分大小写匹配）。
func (f *File) FindEntry(name string) (*ZipEntry, error) {
	cdOffset, cdSize, err := f.centralDirectory()
	if err != nil {
		return nil, err
	}
	cd := make([]byte, cdSize)
	if _, err := f.ReadAt(cd, cdOffset); err != nil {
		return nil, fmt.Errorf("读取 zip 中央目录失败：%w", err)
	}

	target := strings.ToLower(name)
	var fallback *ZipEntry
	p := 0
	for p+46 <= len(cd) {
		if be32(cd[p:]) != sigCEN {
			break
		}
		method := be16(cd[p+10:])
		csize := int64(be32(cd[p+20:]))
		usize := int64(be32(cd[p+24:]))
		nameLen := int(be16(cd[p+28:]))
		extraLen := int(be16(cd[p+30:]))
		commentLen := int(be16(cd[p+32:]))
		localOff := int64(be32(cd[p+42:]))

		if p+46+nameLen > len(cd) {
			break
		}
		entryName := string(cd[p+46 : p+46+nameLen])
		extraStart := p + 46 + nameLen
		if extraStart+extraLen > len(cd) {
			break
		}
		extra := cd[extraStart : extraStart+extraLen]

		if csize == 0xFFFFFFFF || usize == 0xFFFFFFFF || localOff == 0xFFFFFFFF {
			usize, csize, localOff = applyZip64Extra(extra, usize, csize, localOff)
		}

		if strings.ToLower(path.Base(entryName)) == target {
			entry := &ZipEntry{
				Name:             entryName,
				Method:           method,
				CompressedSize:   csize,
				UncompressedSize: usize,
				LocalOffset:      localOff,
			}
			// 根目录下的优先
			if entryName == name || !strings.Contains(entryName, "/") {
				if err := f.resolveDataOffset(entry); err != nil {
					return nil, err
				}
				return entry, nil
			}
			if fallback == nil {
				fallback = entry
			}
		}
		p += 46 + nameLen + extraLen + commentLen
	}
	if fallback != nil {
		if err := f.resolveDataOffset(fallback); err != nil {
			return nil, err
		}
		return fallback, nil
	}
	return nil, fmt.Errorf("压缩包内未找到 %s", name)
}

// applyZip64Extra 按 ZIP64 扩展字段修正被置位的尺寸与偏移。
// 字段顺序固定：原始尺寸、压缩尺寸、本地头偏移、磁盘号，只包含被置位的那些。
func applyZip64Extra(extra []byte, usize, csize, localOff int64) (int64, int64, int64) {
	p := 0
	for p+4 <= len(extra) {
		id := be16(extra[p:])
		size := int(be16(extra[p+2:]))
		p += 4
		if p+size > len(extra) {
			return usize, csize, localOff
		}
		if id != 0x0001 {
			p += size
			continue
		}
		data := extra[p : p+size]
		q := 0
		next := func() (int64, bool) {
			if q+8 > len(data) {
				return 0, false
			}
			v := int64(binary.LittleEndian.Uint64(data[q : q+8]))
			q += 8
			return v, true
		}
		if usize == 0xFFFFFFFF {
			if v, ok := next(); ok {
				usize = v
			}
		}
		if csize == 0xFFFFFFFF {
			if v, ok := next(); ok {
				csize = v
			}
		}
		if localOff == 0xFFFFFFFF {
			if v, ok := next(); ok {
				localOff = v
			}
		}
		return usize, csize, localOff
	}
	return usize, csize, localOff
}

// resolveDataOffset 计算条目数据区的真实起始偏移（跳过本地文件头）。
func (f *File) resolveDataOffset(entry *ZipEntry) error {
	if entry.LocalOffset < 0 || entry.LocalOffset+30 > f.Size {
		return errors.New("zip 条目本地头位置异常")
	}
	hdr := make([]byte, 30)
	if _, err := f.ReadAt(hdr, entry.LocalOffset); err != nil {
		return fmt.Errorf("读取 zip 本地头失败：%w", err)
	}
	if be32(hdr) != sigLOC {
		return errors.New("zip 本地文件头签名不匹配，压缩包可能已损坏")
	}
	nameLen := int64(be16(hdr[26:28]))
	extraLen := int64(be16(hdr[28:30]))
	entry.DataOffset = entry.LocalOffset + 30 + nameLen + extraLen
	if entry.DataOffset > f.Size {
		return errors.New("zip 条目数据超出文件范围")
	}
	return nil
}
