// Package lp 解析与生成 Android 动态分区（super）的 lp_metadata。
//
// 结构定义以 AOSP system/core/fs_mgr/liblp/metadata_format.h 为准：
//
//	super 布局（单位：字节）
//	[0, 4096)        保留给 bootloader
//	[4096, 8192)     主 geometry
//	[8192, 12288)    备 geometry
//	[12288, ...)     主 metadata 槽位（每个 metadata_max_size）
//	[...]            备 metadata 槽位
//	[first_logical_sector*512, ...) 各逻辑分区的数据
//
// 这里只做读取与「按分区切分 / 按目录合成」，不涉及设备写入。
package lp

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

const (
	geometryMagic = 0x616c4467
	metadataMagic = 0x414c5030

	// GeometryOffset 主 geometry 的偏移（前 4096 字节保留）。
	GeometryOffset       = 4096
	backupGeometryOffset = GeometryOffset + geometrySize
	geometrySize         = 4096

	// primaryMetadataOffset 主 metadata 起始偏移。
	primaryMetadataOffset = GeometryOffset + geometrySize*2

	headerSize = 256
	sectorSize = 512

	// LP_PARTITION_ATTR_*
	attrReadonly     = 1 << 0
	attrSlotSuffixed = 1 << 1
	attrUpdated      = 1 << 2
	attrDisabled     = 1 << 3

	// 各表项的固定长度。厂商写过更大的 entry_size 时按 stride 跳过，字段偏移不变。
	entryPartition   = 52
	entryExtent      = 24
	entryGroup       = 48
	entryBlockDevice = 64
)

// Extent 是分区在物理设备上的一段连续扇区。
type Extent struct {
	NumSectors uint64 `json:"numSectors"`
	// TargetType 0 = linear（读物理扇区），1 = zero（填零）。
	TargetType   uint32 `json:"targetType"`
	TargetData   uint64 `json:"targetData"`
	TargetSource uint32 `json:"targetSource"`
}

// Partition 是一个逻辑分区。
type Partition struct {
	Name       string `json:"name"`
	Attributes uint32 `json:"attributes"`
	Group      string `json:"group"`
	GroupIndex uint32 `json:"groupIndex"`
	SizeBytes  int64  `json:"sizeBytes"`
	// Extents 按逻辑顺序排列，可能对应多段不连续的物理扇区。
	Extents      []Extent `json:"extents"`
	ReadOnly     bool     `json:"readOnly"`
	SlotSuffixed bool     `json:"slotSuffixed"`
	Disabled     bool     `json:"disabled"`
}

// Group 是分区组（A/B 设备上常见 default / qti_dynamic_partitions 等）。
type Group struct {
	Name    string `json:"name"`
	Flags   uint32 `json:"flags"`
	MaxSize uint64 `json:"maxSize"`
}

// BlockDevice 描述 super 所在的物理块设备。
type BlockDevice struct {
	Name               string `json:"name"`
	FirstLogicalSector uint64 `json:"firstLogicalSector"`
	Alignment          uint32 `json:"alignment"`
	Size               uint64 `json:"size"`
}

// Geometry 是 lp_metadata 的几何参数，决定后面 metadata 槽位怎么排。
type Geometry struct {
	MetadataMaxSize   uint32 `json:"metadataMaxSize"`
	MetadataSlotCount uint32 `json:"metadataSlotCount"`
	LogicalBlockSize  uint32 `json:"logicalBlockSize"`
}

// Metadata 是对一个 super 镜像解析出的完整描述。
type Metadata struct {
	Path         string        `json:"path"`
	Major        uint16        `json:"major"`
	Minor        uint16        `json:"minor"`
	Slot         int           `json:"slot"`
	Geometry     Geometry      `json:"geometry"`
	Partitions   []Partition   `json:"partitions"`
	Groups       []Group       `json:"groups"`
	BlockDevices []BlockDevice `json:"blockDevices"`
	TotalSize    int64         `json:"totalSize"`
	// ChecksumOK 为 false 说明厂商改过表内容但没重算校验和，一般仍可正常使用。
	ChecksumOK bool `json:"checksumOk"`
	// Sparse 表示镜像本体是 Android sparse 格式。
	Sparse bool   `json:"sparse"`
	Note   string `json:"note"`

	// allExtents 解析期暂存全量 extent，分区解析完即释放，不参与序列化。
	allExtents []Extent
}

// tableDesc 是 metadata 里四张表的定位信息。
type tableDesc struct {
	Offset     uint32
	NumEntries uint32
	EntrySize  uint32
}

// Parse 读取 super 镜像（raw 或 sparse）里的 lp_metadata。
func Parse(path string) (*Metadata, error) {
	ra, size, sparse, err := openImage(path)
	if err != nil {
		return nil, err
	}
	defer ra.Close()

	meta, err := parseAt(ra, size)
	if err != nil {
		return nil, err
	}
	meta.Path = path
	meta.TotalSize = size
	meta.Sparse = sparse
	if sparse {
		meta.Note = "镜像为 sparse 格式，已按逻辑地址解析"
	}
	return meta, nil
}

// parseAt 依次尝试主槽位、备槽位与冗余槽位，避免单一槽位损坏时直接失败。
func parseAt(ra readerAt, size int64) (*Metadata, error) {
	geo, err := readGeometry(ra)
	if err != nil {
		return nil, err
	}

	primary := int64(primaryMetadataOffset)
	backup := primary + int64(geo.MetadataSlotCount)*int64(geo.MetadataMaxSize)

	var lastErr error
	for slot := 0; slot < int(geo.MetadataSlotCount); slot++ {
		off := primary + int64(slot)*int64(geo.MetadataMaxSize)
		if m, err := readMetadata(ra, off, geo); err == nil {
			m.Slot = slot
			m.Geometry = geo
			return m, nil
		} else {
			lastErr = err
		}
	}
	// 主槽位全部失败时再试备槽位。
	for slot := 0; slot < int(geo.MetadataSlotCount); slot++ {
		off := backup + int64(slot)*int64(geo.MetadataMaxSize)
		if off+int64(geo.MetadataMaxSize) > size {
			break
		}
		if m, err := readMetadata(ra, off, geo); err == nil {
			m.Slot = slot
			m.Geometry = geo
			m.Note = "主 metadata 校验失败，已改用备份槽位"
			return m, nil
		} else {
			lastErr = err
		}
	}
	if lastErr == nil {
		lastErr = errors.New("未找到可用的 lp_metadata")
	}
	return nil, fmt.Errorf("解析 lp_metadata 失败：%w", lastErr)
}

// readGeometry 读取并校验 geometry。
func readGeometry(ra readerAt) (Geometry, error) {
	buf := make([]byte, geometrySize)
	if _, err := ra.ReadAt(buf, GeometryOffset); err != nil {
		// 有些工具会把 geometry 写在 0 偏移处，兼容一下。
		if _, err2 := ra.ReadAt(buf, 0); err2 != nil {
			return Geometry{}, fmt.Errorf("读取 geometry 失败：%w", err)
		}
	}
	if binary.LittleEndian.Uint32(buf[0:4]) != geometryMagic {
		return Geometry{}, errors.New("不是 super 镜像（缺少 lp_metadata geometry）")
	}
	structSize := binary.LittleEndian.Uint32(buf[4:8])
	if structSize < 52 || structSize > geometrySize {
		return Geometry{}, fmt.Errorf("geometry 长度异常：%d", structSize)
	}
	return Geometry{
		MetadataMaxSize:   binary.LittleEndian.Uint32(buf[40:44]),
		MetadataSlotCount: binary.LittleEndian.Uint32(buf[44:48]),
		LogicalBlockSize:  binary.LittleEndian.Uint32(buf[48:52]),
	}, nil
}

// readMetadata 读取一个 metadata 槽位。
func readMetadata(ra readerAt, off int64, geo Geometry) (*Metadata, error) {
	limit := int64(geo.MetadataMaxSize)
	if limit <= 0 || limit > 4<<20 {
		limit = 1 << 20
	}
	buf := make([]byte, limit)
	n, err := ra.ReadAt(buf, off)
	if err != nil && n < headerSize {
		return nil, err
	}
	buf = buf[:n]

	if binary.LittleEndian.Uint32(buf[0:4]) != metadataMagic {
		return nil, errors.New("metadata magic 不匹配")
	}
	le := binary.LittleEndian
	major := le.Uint16(buf[4:6])
	minor := le.Uint16(buf[6:8])
	hsize := le.Uint32(buf[8:12])
	if hsize < 128 || int(hsize) > len(buf) {
		return nil, fmt.Errorf("metadata 头长度异常：%d", hsize)
	}
	storedHeaderSum := append([]byte(nil), buf[12:44]...)
	tablesSize := le.Uint32(buf[44:48])
	storedTablesSum := append([]byte(nil), buf[48:80]...)

	m := &Metadata{Major: major, Minor: minor}

	// 校验和：头校验时把 checksum 字段清零，表校验针对 tables 区域。
	head := append([]byte(nil), buf[:hsize]...)
	for i := 12; i < 44; i++ {
		head[i] = 0
	}
	m.ChecksumOK = bytes.Equal(sha256Sum(head), storedHeaderSum)

	if int(hsize)+int(tablesSize) > len(buf) {
		return nil, errors.New("metadata 表区越界")
	}
	tables := buf[hsize : hsize+tablesSize]
	if len(tables) > 0 {
		m.ChecksumOK = m.ChecksumOK && bytes.Equal(sha256Sum(tables), storedTablesSum)
	} else {
		m.ChecksumOK = false
	}

	descs := make([]tableDesc, 4)
	for i := 0; i < 4; i++ {
		p := 80 + i*12
		descs[i] = tableDesc{
			Offset:     le.Uint32(buf[p : p+4]),
			NumEntries: le.Uint32(buf[p+4 : p+8]),
			EntrySize:  le.Uint32(buf[p+8 : p+12]),
		}
	}
	partDesc, extDesc, groupDesc, devDesc := descs[0], descs[1], descs[2], descs[3]

	tableBytes := func(d tableDesc, min int) ([]byte, error) {
		start := int(d.Offset)
		if d.EntrySize < uint32(min) {
			return nil, fmt.Errorf("表项长度 %d 小于预期 %d", d.EntrySize, min)
		}
		end := start + int(d.NumEntries)*int(d.EntrySize)
		if start < 0 || end > len(tables) {
			return nil, errors.New("表区越界")
		}
		return tables[start:end], nil
	}

	// groups 要先读，分区里存的是组索引。
	if groupDesc.NumEntries > 0 {
		raw, err := tableBytes(groupDesc, entryGroup)
		if err != nil {
			return nil, fmt.Errorf("读取分区组失败：%w", err)
		}
		for i := 0; i < int(groupDesc.NumEntries); i++ {
			e := raw[i*int(groupDesc.EntrySize):]
			m.Groups = append(m.Groups, Group{
				Name:    cstr(e[0:36]),
				Flags:   le.Uint32(e[36:40]),
				MaxSize: le.Uint64(e[40:48]),
			})
		}
	}

	if extDesc.NumEntries > 0 {
		raw, err := tableBytes(extDesc, entryExtent)
		if err != nil {
			return nil, fmt.Errorf("读取 extent 失败：%w", err)
		}
		for i := 0; i < int(extDesc.NumEntries); i++ {
			e := raw[i*int(extDesc.EntrySize):]
			m.appendExtent(Extent{
				NumSectors:   le.Uint64(e[0:8]),
				TargetType:   le.Uint32(e[8:12]),
				TargetData:   le.Uint64(e[12:20]),
				TargetSource: le.Uint32(e[20:24]),
			})
		}
	}

	if partDesc.NumEntries > 0 {
		raw, err := tableBytes(partDesc, entryPartition)
		if err != nil {
			return nil, fmt.Errorf("读取分区表失败：%w", err)
		}
		for i := 0; i < int(partDesc.NumEntries); i++ {
			e := raw[i*int(partDesc.EntrySize):]
			p := Partition{
				Name:       cstr(e[0:36]),
				Attributes: le.Uint32(e[36:40]),
				GroupIndex: le.Uint32(e[48:52]),
			}
			first := le.Uint32(e[40:44])
			count := le.Uint32(e[44:48])
			p.ReadOnly = p.Attributes&attrReadonly != 0
			p.SlotSuffixed = p.Attributes&attrSlotSuffixed != 0
			p.Disabled = p.Attributes&attrDisabled != 0
			if int(first+count) <= len(m.allExtents) {
				p.Extents = append([]Extent(nil), m.allExtents[first:first+count]...)
			}
			for _, ex := range p.Extents {
				p.SizeBytes += int64(ex.NumSectors) * sectorSize
			}
			if int(p.GroupIndex) < len(m.Groups) {
				p.Group = m.Groups[p.GroupIndex].Name
			}
			m.Partitions = append(m.Partitions, p)
		}
	}

	if devDesc.NumEntries > 0 {
		raw, err := tableBytes(devDesc, entryBlockDevice)
		if err != nil {
			return nil, fmt.Errorf("读取块设备表失败：%w", err)
		}
		for i := 0; i < int(devDesc.NumEntries); i++ {
			e := raw[i*int(devDesc.EntrySize):]
			m.BlockDevices = append(m.BlockDevices, BlockDevice{
				FirstLogicalSector: le.Uint64(e[0:8]),
				Alignment:          le.Uint32(e[8:12]),
				Size:               le.Uint64(e[16:24]),
				Name:               cstr(e[24:60]),
			})
		}
	}

	// allExtents 只是临时中转，避免每个分区都复制一遍后再截断。
	m.allExtents = nil
	return m, nil
}

// allExtents 在解析过程中暂存全量 extent，分区解析完即释放。
// 它不参与 JSON 序列化，也不应被外部使用。
func (m *Metadata) appendExtent(e Extent) { m.allExtents = append(m.allExtents, e) }

func sha256Sum(b []byte) []byte {
	sum := sha256.Sum256(b)
	return sum[:]
}

// cstr 读取定长字段里的 NUL 结尾字符串。
func cstr(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return strings.TrimSpace(string(b))
}

// GroupNames 返回去重后的组名，供界面下拉使用。
func (m *Metadata) GroupNames() []string {
	seen := map[string]bool{}
	var out []string
	for _, g := range m.Groups {
		if g.Name == "" || seen[g.Name] {
			continue
		}
		seen[g.Name] = true
		out = append(out, g.Name)
	}
	return out
}

// Partition 按名称查找分区。
func (m *Metadata) Partition(name string) (Partition, bool) {
	for _, p := range m.Partitions {
		if p.Name == name {
			return p, true
		}
	}
	return Partition{}, false
}

// IsSuper 判断文件是否带 lp_metadata。
func IsSuper(path string) bool {
	ra, _, _, err := openImage(path)
	if err != nil {
		return false
	}
	defer ra.Close()
	_, err = readGeometry(ra)
	return err == nil
}

// Dump 把分区表渲染成便于粘贴的文本。
func (m *Metadata) Dump() string {
	var b strings.Builder
	fmt.Fprintf(&b, "lp_metadata v%d.%d  slot %d  块大小 %d\n", m.Major, m.Minor, m.Slot, m.Geometry.LogicalBlockSize)
	fmt.Fprintf(&b, "镜像大小 %.2f GB  分区 %d 个  组 %d 个\n\n", float64(m.TotalSize)/(1<<30), len(m.Partitions), len(m.Groups))
	for _, p := range m.Partitions {
		fmt.Fprintf(&b, "%-24s %10.2f MB  组 %-24s extent %d\n", p.Name, float64(p.SizeBytes)/(1<<20), p.Group, len(p.Extents))
	}
	return b.String()
}
