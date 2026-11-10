package lp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// 默认布局参数，与 AOSP liblp 的默认值一致。
const (
	DefaultBlockSize   = 4096
	DefaultAlignment   = 1 << 20 // 1 MB
	DefaultSlotCount   = 2
	DefaultMetadataMax = 65536
	DefaultLabel       = "super"
)

// BuildPartition 是合成 super 时的一个输入分区。
type BuildPartition struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Group string `json:"group"`
}

// Candidate 是扫描目录得到的候选分区，额外带上文件大小供界面展示。
type Candidate struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Group     string `json:"group"`
	SizeBytes int64  `json:"sizeBytes"`
}

// BuildOptions 控制合成结果。
type BuildOptions struct {
	// Label 是块设备名字，一般就叫 super。
	Label string `json:"label"`
	// DefaultGroup 是未指定组的分区归到哪个组，默认 default。
	DefaultGroup string `json:"defaultGroup"`
	BlockSize    uint32 `json:"blockSize"`
	Alignment    uint64 `json:"alignment"`
	SlotCount    uint32 `json:"slotCount"`
	// OutputSize 大于 0 时按该大小输出（单位字节），便于对齐原厂 super 尺寸。
	OutputSize int64 `json:"outputSize"`
}

func (o BuildOptions) normalized() BuildOptions {
	if o.Label == "" {
		o.Label = DefaultLabel
	}
	if o.DefaultGroup == "" {
		o.DefaultGroup = "default"
	}
	if o.BlockSize == 0 {
		o.BlockSize = DefaultBlockSize
	}
	if o.Alignment == 0 {
		o.Alignment = DefaultAlignment
	}
	if o.SlotCount == 0 {
		o.SlotCount = DefaultSlotCount
	}
	return o
}

// Build 按给定文件合成 super 镜像，并返回写入后的元数据。
//
// 布局策略：各分区按传入顺序依次排布，每个分区起点按 Alignment 对齐，
// 只生成一段线性 extent（不做快照 / COW）。这与官方 super_empty.img
// 扩容后的形态一致，可直接 fastboot flash super 使用。
func Build(ctx context.Context, parts []BuildPartition, out string, opt BuildOptions, onProgress func(string, int)) (*Metadata, error) {
	if len(parts) == 0 {
		return nil, errors.New("没有可合成的分区")
	}
	opt = opt.normalized()

	type layout struct {
		BuildPartition
		size    int64
		sectors uint64
		start   int64
		sector  uint64
		group   uint32
	}
	items := make([]layout, 0, len(parts))
	seen := map[string]bool{}
	for _, p := range parts {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			return nil, fmt.Errorf("%s 缺少分区名", filepath.Base(p.Path))
		}
		if seen[name] {
			return nil, fmt.Errorf("分区名重复：%s", name)
		}
		seen[name] = true
		st, err := os.Stat(p.Path)
		if err != nil {
			return nil, fmt.Errorf("读取 %s 失败：%w", filepath.Base(p.Path), err)
		}
		if st.IsDir() {
			return nil, fmt.Errorf("%s 是目录，不是镜像", p.Path)
		}
		items = append(items, layout{
			BuildPartition: BuildPartition{Name: name, Path: p.Path, Group: p.Group},
			size:           st.Size(),
			sectors:        uint64((st.Size() + sectorSize - 1) / sectorSize),
		})
	}

	// 组：default 必须是 0 号，AOSP 要求该组固定存在。
	groupIndex := map[string]uint32{"default": 0}
	groups := []Group{{Name: "default"}}
	for i := range items {
		g := items[i].Group
		if g == "" {
			g = opt.DefaultGroup
		}
		items[i].Group = g
		idx, ok := groupIndex[g]
		if !ok {
			idx = uint32(len(groups))
			groupIndex[g] = idx
			groups = append(groups, Group{Name: g})
		}
		items[i].group = idx
	}

	// 排布：从头依次放，起点按 alignment 对齐。
	cursor := int64(firstLogicalSector(opt)) * sectorSize
	for i := range items {
		start := alignUp(cursor, int64(opt.Alignment))
		items[i].start = start
		items[i].sector = uint64(start / sectorSize)
		cursor = start + int64(items[i].sectors)*sectorSize
	}
	total := alignUp(cursor, int64(opt.Alignment))
	if opt.OutputSize > total {
		total = alignUp(opt.OutputSize, int64(opt.Alignment))
	}
	if opt.OutputSize > 0 && opt.OutputSize < cursor {
		return nil, fmt.Errorf("指定大小 %d 小于数据总量 %d", opt.OutputSize, cursor)
	}

	// 先把文件撑到目标大小，再按偏移写数据，避免边写边扩容。
	f, err := os.Create(out)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := f.Truncate(total); err != nil {
		return nil, err
	}

	buf := make([]byte, 4<<20)
	for i, it := range items {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("已中止：%w", err)
		}
		if onProgress != nil {
			onProgress(it.Name, i*100/len(items))
		}
		if err := copyInto(f, it.Path, it.start, buf); err != nil {
			return nil, fmt.Errorf("写入 %s 失败：%w", it.Name, err)
		}
	}
	if onProgress != nil {
		onProgress("写入元数据", 96)
	}

	meta := &Metadata{
		Major:    10,
		Minor:    2,
		Geometry: Geometry{MetadataMaxSize: DefaultMetadataMax, MetadataSlotCount: opt.SlotCount, LogicalBlockSize: opt.BlockSize},
		Groups:   groups,
		BlockDevices: []BlockDevice{{
			Name:               opt.Label,
			FirstLogicalSector: uint64(firstLogicalSector(opt)),
			Alignment:          uint32(opt.Alignment),
			Size:               uint64(total),
		}},
	}
	for _, it := range items {
		// 带槽位后缀的分区在元数据里要打 SLOT_SUFFIXED，否则 HAL 不会拼 _a/_b。
		attrs := uint32(attrReadonly)
		suffixed := strings.HasSuffix(it.Name, "_a") || strings.HasSuffix(it.Name, "_b")
		if suffixed {
			attrs |= attrSlotSuffixed
		}
		p := Partition{
			Name:         it.Name,
			Attributes:   attrs,
			Group:        it.Group,
			GroupIndex:   it.group,
			SizeBytes:    int64(it.sectors) * sectorSize,
			ReadOnly:     true,
			SlotSuffixed: suffixed,
			Extents: []Extent{{
				NumSectors:   it.sectors,
				TargetType:   0,
				TargetData:   it.sector,
				TargetSource: 0,
			}},
		}
		meta.Partitions = append(meta.Partitions, p)
	}

	if err := writeMetadata(f, meta); err != nil {
		return nil, err
	}
	if err := f.Sync(); err != nil {
		return nil, err
	}
	if onProgress != nil {
		onProgress("完成", 100)
	}

	built, err := Parse(out)
	if err != nil {
		return meta, fmt.Errorf("已写出镜像但回读校验失败：%w", err)
	}
	return built, nil
}

// firstLogicalSector 返回第一个逻辑分区的起始扇区（默认 1 MB 处）。
func firstLogicalSector(opt BuildOptions) int64 {
	s := int64(opt.Alignment) / sectorSize
	if s < 2048 {
		s = 2048
	}
	return s
}

func alignUp(v, align int64) int64 {
	if align <= 1 {
		return v
	}
	if r := v % align; r != 0 {
		v += align - r
	}
	return v
}

// copyInto 把 src 写入 dst 的指定偏移。
func copyInto(dst *os.File, src string, offset int64, buf []byte) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	pos := offset
	for {
		n, rerr := in.Read(buf)
		if n > 0 {
			if _, werr := dst.WriteAt(buf[:n], pos); werr != nil {
				return werr
			}
			pos += int64(n)
		}
		if rerr == io.EOF {
			return nil
		}
		if rerr != nil {
			return rerr
		}
	}
}

// writeMetadata 写入两份 geometry、各槽位 metadata 与对应备份。
func writeMetadata(f *os.File, m *Metadata) error {
	geo := m.Geometry
	if geo.MetadataMaxSize == 0 {
		geo.MetadataMaxSize = DefaultMetadataMax
	}
	if geo.MetadataSlotCount == 0 {
		geo.MetadataSlotCount = DefaultSlotCount
	}
	if geo.LogicalBlockSize == 0 {
		geo.LogicalBlockSize = DefaultBlockSize
	}

	geoBuf := encodeGeometry(geo)
	if _, err := f.WriteAt(geoBuf, GeometryOffset); err != nil {
		return err
	}
	if _, err := f.WriteAt(geoBuf, backupGeometryOffset); err != nil {
		return err
	}

	blob := encodeMetadata(m)
	if int64(len(blob)) > int64(geo.MetadataMaxSize) {
		return fmt.Errorf("metadata 体积 %d 超过上限 %d", len(blob), geo.MetadataMaxSize)
	}

	primary := int64(primaryMetadataOffset)
	backup := primary + int64(geo.MetadataSlotCount)*int64(geo.MetadataMaxSize)
	for slot := 0; slot < int(geo.MetadataSlotCount); slot++ {
		if _, err := f.WriteAt(blob, primary+int64(slot)*int64(geo.MetadataMaxSize)); err != nil {
			return err
		}
		if _, err := f.WriteAt(blob, backup+int64(slot)*int64(geo.MetadataMaxSize)); err != nil {
			return err
		}
	}
	return nil
}

// encodeGeometry 生成 4096 字节的 geometry 块。
func encodeGeometry(g Geometry) []byte {
	buf := make([]byte, geometrySize)
	le := binary.LittleEndian
	le.PutUint32(buf[0:4], geometryMagic)
	le.PutUint32(buf[4:8], 52) // struct_size：到 reserved 为止
	le.PutUint32(buf[40:44], g.MetadataMaxSize)
	le.PutUint32(buf[44:48], g.MetadataSlotCount)
	le.PutUint32(buf[48:52], g.LogicalBlockSize)
	sum := sha256.Sum256(buf[:52])
	copy(buf[8:40], sum[:])
	return buf
}

// encodeMetadata 生成 header + 四张表。
func encodeMetadata(m *Metadata) []byte {
	le := binary.LittleEndian

	var partTab, extTab, groupTab, devTab bytes.Buffer
	firstExtent := uint32(0)
	for _, p := range m.Partitions {
		var name [36]byte
		copy(name[:], p.Name)
		partTab.Write(name[:])
		writeU32(&partTab, p.Attributes)
		writeU32(&partTab, firstExtent)
		writeU32(&partTab, uint32(len(p.Extents)))
		writeU32(&partTab, p.GroupIndex)
		firstExtent += uint32(len(p.Extents))
		for _, e := range p.Extents {
			writeU64(&extTab, e.NumSectors)
			writeU32(&extTab, e.TargetType)
			writeU64(&extTab, e.TargetData)
			writeU32(&extTab, e.TargetSource)
		}
	}
	for _, g := range m.Groups {
		var name [36]byte
		copy(name[:], g.Name)
		groupTab.Write(name[:])
		writeU32(&groupTab, g.Flags)
		writeU64(&groupTab, g.MaxSize)
	}
	for _, d := range m.BlockDevices {
		var name [36]byte
		copy(name[:], d.Name)
		writeU64(&devTab, d.FirstLogicalSector)
		writeU32(&devTab, d.Alignment)
		writeU32(&devTab, 0) // alignment_offset
		writeU64(&devTab, d.Size)
		devTab.Write(name[:])
		writeU32(&devTab, 0) // flags
	}

	// 表区顺序必须与 descriptor 的顺序一致。
	tables := bytes.Join([][]byte{partTab.Bytes(), extTab.Bytes(), groupTab.Bytes(), devTab.Bytes()}, nil)

	partOff := 0
	extOff := partOff + partTab.Len()
	groupOff := extOff + extTab.Len()
	devOff := groupOff + groupTab.Len()

	buf := make([]byte, headerSize+len(tables))
	le.PutUint32(buf[0:4], metadataMagic)
	le.PutUint16(buf[4:6], m.Major)
	le.PutUint16(buf[6:8], m.Minor)
	le.PutUint32(buf[8:12], headerSize)
	le.PutUint32(buf[44:48], uint32(len(tables)))

	descs := []struct{ off, num, size int }{
		{partOff, int(partTab.Len() / entryPartition), entryPartition},
		{extOff, int(extTab.Len() / entryExtent), entryExtent},
		{groupOff, int(groupTab.Len() / entryGroup), entryGroup},
		{devOff, int(devTab.Len() / entryBlockDevice), entryBlockDevice},
	}
	for i, d := range descs {
		p := 80 + i*12
		le.PutUint32(buf[p:p+4], uint32(d.off))
		le.PutUint32(buf[p+4:p+8], uint32(d.num))
		le.PutUint32(buf[p+8:p+12], uint32(d.size))
	}
	copy(buf[headerSize:], tables)

	tableSum := sha256.Sum256(tables)
	copy(buf[48:80], tableSum[:])

	headSum := sha256.Sum256(buf[:headerSize])
	copy(buf[12:44], headSum[:])
	return buf
}

func writeU32(b *bytes.Buffer, v uint32) {
	var tmp [4]byte
	binary.LittleEndian.PutUint32(tmp[:], v)
	b.Write(tmp[:])
}

func writeU64(b *bytes.Buffer, v uint64) {
	var tmp [8]byte
	binary.LittleEndian.PutUint64(tmp[:], v)
	b.Write(tmp[:])
}

// SortPartitions 按名称排序，保证同一目录多次合成结果稳定。
func SortPartitions(list []BuildPartition) {
	sort.SliceStable(list, func(i, j int) bool { return list[i].Name < list[j].Name })
}
