package image

import (
	"compress/bzip2"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/ulikunitz/xz"
)

var errBadProto = errors.New("payload manifest 解析失败（protobuf 格式异常）")

// 安装操作类型（对应 update_metadata.proto InstallOperation.Type）
const (
	OpReplace    = 0
	OpReplaceBz  = 1
	OpMove       = 2
	OpBsdiff     = 3
	OpSourceCopy = 4
	OpSourceDiff = 5
	OpZero       = 6
	OpDiscard    = 7
	OpReplaceXz  = 8
	OpPuffdiff   = 9
	OpBrotliBsd  = 10
	OpZucchini   = 11
	OpLz4Bsdiff  = 12
	OpLz4Puff    = 13
)

// PayloadPartition 描述 payload.bin 内的一个分区（对外 JSON 结构）。
type PayloadPartition struct {
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	Operations  int    `json:"operations"`
	Incremental bool   `json:"incremental"`
}

// PayloadInfo 是 payload.bin 的概览。
type PayloadInfo struct {
	Path       string             `json:"path"`
	Version    int                `json:"version"`
	BlockSize  int64              `json:"blockSize"`
	Partitions []PayloadPartition `json:"partitions"`
	Full       bool               `json:"full"`
}

// PayloadExtent 是目标分区内的一段连续块区间。
type PayloadExtent struct {
	StartBlock int64
	NumBlocks  int64
}

// PayloadOp 是一条安装操作。
type PayloadOp struct {
	Type       int
	DataOffset int64
	DataLength int64
	Dst        []PayloadExtent
}

// PayloadPart 是一个分区的完整操作列表。
type PayloadPart struct {
	Name string
	Size int64
	Ops  []PayloadOp
}

// PayloadIndex 是 payload 头部 + 清单的解析结果，不依赖数据源类型。
type PayloadIndex struct {
	Version      int
	BlockSize    int64
	HeaderLen    int64
	ManifestSize int64
	MetaSigSize  int64
	Partitions   []PayloadPart
}

// DataStart 返回数据区相对 payload 起始位置的偏移。
func (idx *PayloadIndex) DataStart() int64 {
	return idx.HeaderLen + idx.ManifestSize + idx.MetaSigSize
}

// Partition 按名称查找分区。
func (idx *PayloadIndex) Partition(name string) (PayloadPart, bool) {
	for _, p := range idx.Partitions {
		if p.Name == name {
			return p, true
		}
	}
	return PayloadPart{}, false
}

// SegmentOpener 按需打开 payload 数据区中的一段（offset 相对数据区起点）。
// 本地文件用 SectionReader，远程 OTA 用 HTTP Range，二者共用同一套提取逻辑。
type SegmentOpener func(offset, length int64) (io.ReadCloser, error)

// ---------------------------------------------------------------------------
// 极简 protobuf wire 解码
// ---------------------------------------------------------------------------

func walkProto(b []byte, fn func(num, wire int, v uint64, data []byte)) error {
	i := 0
	for i < len(b) {
		key, n := binary.Uvarint(b[i:])
		if n <= 0 {
			return errBadProto
		}
		i += n
		num := int(key >> 3)
		wire := int(key & 7)
		switch wire {
		case 0:
			v, n := binary.Uvarint(b[i:])
			if n <= 0 {
				return errBadProto
			}
			i += n
			fn(num, wire, v, nil)
		case 1:
			if i+8 > len(b) {
				return errBadProto
			}
			fn(num, wire, 0, b[i:i+8])
			i += 8
		case 2:
			l, n := binary.Uvarint(b[i:])
			if n <= 0 {
				return errBadProto
			}
			i += n
			if i+int(l) > len(b) {
				return errBadProto
			}
			fn(num, wire, 0, b[i:i+int(l)])
			i += int(l)
		case 5:
			if i+4 > len(b) {
				return errBadProto
			}
			fn(num, wire, 0, b[i:i+4])
			i += 4
		default:
			return errBadProto
		}
	}
	return nil
}

func parseExtent(b []byte) (PayloadExtent, error) {
	var e PayloadExtent
	err := walkProto(b, func(num, wire int, v uint64, data []byte) {
		switch num {
		case 1:
			e.StartBlock = int64(v)
		case 2:
			e.NumBlocks = int64(v)
		}
	})
	return e, err
}

func parseOperation(b []byte) (PayloadOp, error) {
	var op PayloadOp
	err := walkProto(b, func(num, wire int, v uint64, data []byte) {
		switch num {
		case 1:
			op.Type = int(v)
		case 2:
			op.DataOffset = int64(v)
		case 3:
			op.DataLength = int64(v)
		case 6:
			if e, err := parseExtent(data); err == nil {
				op.Dst = append(op.Dst, e)
			}
		}
	})
	return op, err
}

// parsePartition 解析 PartitionUpdate。
// 字段号对照 update_metadata.proto：
//
//	1 partition_name, 7 new_partition_info, 8 operations
func parsePartition(b []byte) (PayloadPart, error) {
	var p PayloadPart
	err := walkProto(b, func(num, wire int, v uint64, data []byte) {
		switch num {
		case 1:
			p.Name = string(data)
		case 7:
			// PartitionInfo: 1 size, 2 hash
			_ = walkProto(data, func(n, w int, vv uint64, dd []byte) {
				if n == 1 {
					p.Size = int64(vv)
				}
			})
		case 8:
			if op, err := parseOperation(data); err == nil {
				p.Ops = append(p.Ops, op)
			}
		}
	})
	return p, err
}

// parseManifest 返回 block_size 与分区列表。
func parseManifest(b []byte) (int64, []PayloadPart, error) {
	blockSize := int64(4096)
	var parts []PayloadPart
	err := walkProto(b, func(num, wire int, v uint64, data []byte) {
		switch num {
		case 3:
			if v > 0 {
				blockSize = int64(v)
			}
		case 13:
			if p, err := parsePartition(data); err == nil && p.Name != "" {
				parts = append(parts, p)
			}
		}
	})
	return blockSize, parts, err
}

// ---------------------------------------------------------------------------
// payload 头部
// ---------------------------------------------------------------------------

const payloadMagic = "CrAU"

// MaxManifestSize 是清单大小的上限，防止损坏文件导致巨额分配。
const MaxManifestSize = 64 * 1024 * 1024

// HeadLen 根据版本返回头部长度：v1 为 20 字节，v2 起为 24 字节。
func HeadLen(version int) int64 {
	if version >= 2 {
		return 24
	}
	return 20
}

// ParsePayloadHead 解析 payload 头部（head 至少 20 字节）。
// v2 及以上的元数据签名长度位于第 20–24 字节，调用方若只传 20 字节，
// 需要自行读取这 4 字节并回填 metaSigSize。
func ParsePayloadHead(head []byte) (version int, manifestSize int64, metaSigSize int64, err error) {
	if len(head) < 20 {
		return 0, 0, 0, fmt.Errorf("payload 头部不完整")
	}
	if string(head[:4]) != payloadMagic {
		return 0, 0, 0, fmt.Errorf("不是 payload.bin（magic=%q）", string(head[:4]))
	}
	version = int(binary.BigEndian.Uint64(head[4:12]))
	manifestSize = int64(binary.BigEndian.Uint64(head[12:20]))
	if version >= 2 && len(head) >= 24 {
		metaSigSize = int64(binary.BigEndian.Uint32(head[20:24]))
	}
	if manifestSize <= 0 || manifestSize > MaxManifestSize {
		return 0, 0, 0, fmt.Errorf("payload 清单尺寸异常：%d", manifestSize)
	}
	return version, manifestSize, metaSigSize, nil
}

// NewPayloadIndex 用已读出的头部信息与清单构造索引。
func NewPayloadIndex(version int, manifestSize, metaSigSize int64, manifest []byte) (*PayloadIndex, error) {
	blockSize, parts, err := parseManifest(manifest)
	if err != nil {
		return nil, err
	}
	return &PayloadIndex{
		Version:      version,
		BlockSize:    blockSize,
		HeaderLen:    HeadLen(version),
		ManifestSize: manifestSize,
		MetaSigSize:  metaSigSize,
		Partitions:   parts,
	}, nil
}

func readPayloadHeader(path string) (*PayloadIndex, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	head := make([]byte, 24)
	n, err := io.ReadFull(f, head)
	if err != nil && err != io.ErrUnexpectedEOF {
		return nil, fmt.Errorf("不是有效的 payload.bin（读取头部失败）")
	}
	head = head[:n]

	version, manifestSize, metaSig, err := ParsePayloadHead(head)
	if err != nil {
		return nil, err
	}
	if _, err := f.Seek(HeadLen(version), io.SeekStart); err != nil {
		return nil, err
	}
	manifest := make([]byte, manifestSize)
	if _, err := io.ReadFull(f, manifest); err != nil {
		return nil, fmt.Errorf("读取 payload 清单失败：%w", err)
	}
	return NewPayloadIndex(version, manifestSize, metaSig, manifest)
}

// ParsePayload 读取 payload.bin 概览。
func ParsePayload(path string) (*PayloadInfo, error) {
	idx, err := readPayloadHeader(path)
	if err != nil {
		return nil, err
	}
	return buildPayloadInfo(path, idx), nil
}

// ParsePayloadStream 只读取 payload 清单（不读取数据区），
// 用于在不解压整个 OTA 的情况下获知分区列表。
func ParsePayloadStream(r io.Reader) (*PayloadInfo, error) {
	head := make([]byte, 20)
	if _, err := io.ReadFull(r, head); err != nil {
		return nil, fmt.Errorf("读取 payload 头部失败：%w", err)
	}
	version, manifestSize, metaSig, err := ParsePayloadHead(head)
	if err != nil {
		return nil, err
	}
	if HeadLen(version) > 20 {
		extra := make([]byte, 4)
		if _, err := io.ReadFull(r, extra); err != nil {
			return nil, fmt.Errorf("读取 payload 头部失败：%w", err)
		}
		metaSig = int64(binary.BigEndian.Uint32(extra))
	}
	manifest := make([]byte, manifestSize)
	if _, err := io.ReadFull(r, manifest); err != nil {
		return nil, fmt.Errorf("读取 payload 清单失败：%w", err)
	}
	idx, err := NewPayloadIndex(version, manifestSize, metaSig, manifest)
	if err != nil {
		return nil, err
	}
	return buildPayloadInfo("", idx), nil
}

func buildPayloadInfo(path string, idx *PayloadIndex) *PayloadInfo {
	info := &PayloadInfo{
		Path:      path,
		Version:   idx.Version,
		BlockSize: idx.BlockSize,
		Full:      true,
	}
	for _, p := range idx.Partitions {
		pp := PayloadPartition{
			Name:       p.Name,
			Size:       p.Size,
			Operations: len(p.Ops),
		}
		for _, op := range p.Ops {
			switch op.Type {
			case OpMove, OpBsdiff, OpSourceCopy, OpSourceDiff, OpPuffdiff, OpBrotliBsd, OpZucchini, OpLz4Bsdiff, OpLz4Puff:
				pp.Incremental = true
				info.Full = false
			}
		}
		if pp.Size == 0 {
			pp.Size = partitionSizeFromOps(p.Ops, idx.BlockSize)
		}
		info.Partitions = append(info.Partitions, pp)
	}
	sort.SliceStable(info.Partitions, func(i, j int) bool {
		return info.Partitions[i].Name < info.Partitions[j].Name
	})
	return info
}

func partitionSizeFromOps(ops []PayloadOp, blockSize int64) int64 {
	var maxEnd int64
	for _, op := range ops {
		for _, e := range op.Dst {
			end := (e.StartBlock + e.NumBlocks) * blockSize
			if end > maxEnd {
				maxEnd = end
			}
		}
	}
	return maxEnd
}

// PartitionSizeFromOps 由操作的目标区间推算分区大小（清单未声明时使用）。
func PartitionSizeFromOps(ops []PayloadOp, blockSize int64) int64 {
	return partitionSizeFromOps(ops, blockSize)
}

// OpsSize 返回分区需要读取的数据总量（压缩后）。
func OpsSize(ops []PayloadOp) int64 {
	var total int64
	for _, op := range ops {
		total += op.DataLength
	}
	return total
}

// CountIncremental 判断 payload 是否包含增量操作。
func CountIncremental(info *PayloadInfo) bool {
	for _, p := range info.Partitions {
		if p.Incremental {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// 提取
// ---------------------------------------------------------------------------

// ExtractPayload 从本地 payload.bin 提取指定分区（names 为空表示全部）。
func ExtractPayload(path string, names []string, outDir string, onProgress func(name string, percent int)) ([]string, error) {
	idx, err := readPayloadHeader(path)
	if err != nil {
		return nil, err
	}
	src, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer src.Close()

	open := func(offset, length int64) (io.ReadCloser, error) {
		return io.NopCloser(io.NewSectionReader(src, idx.DataStart()+offset, length)), nil
	}
	return ExtractWith(idx, open, names, outDir, onProgress)
}

// ExtractWith 用给定的数据源提取分区。
// open 的 offset 相对 payload 数据区起点，length 为该操作的数据长度。
func ExtractWith(idx *PayloadIndex, open SegmentOpener, names []string, outDir string, onProgress func(name string, percent int)) ([]string, error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}

	var selected []PayloadPart
	var totalBytes int64
	var unsupported string
	for _, p := range idx.Partitions {
		if len(want) > 0 && !want[p.Name] {
			continue
		}
		selected = append(selected, p)
		for _, op := range p.Ops {
			totalBytes += op.DataLength
			switch op.Type {
			case OpReplace, OpReplaceBz, OpReplaceXz, OpZero, OpDiscard:
			default:
				unsupported = p.Name
			}
		}
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("没有匹配到可提取的分区")
	}
	if unsupported != "" {
		return nil, fmt.Errorf("分区 %s 含增量操作，需要原厂底包才能还原，暂不支持", unsupported)
	}

	var doneBytes int64
	var written []string
	for _, p := range selected {
		size := p.Size
		if size == 0 {
			size = partitionSizeFromOps(p.Ops, idx.BlockSize)
		}
		outPath := filepath.Join(outDir, p.Name+".img")
		if err := extractOnePartition(idx, open, p, size, outPath, &doneBytes, totalBytes, onProgress); err != nil {
			return written, fmt.Errorf("提取 %s 失败：%w", p.Name, err)
		}
		written = append(written, outPath)
	}
	return written, nil
}

func extractOnePartition(idx *PayloadIndex, open SegmentOpener, p PayloadPart, size int64, outPath string, doneBytes *int64, totalBytes int64, onProgress func(string, int)) error {
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()

	// 预置为完整尺寸，未写入的区域天然是全零（稀疏文件）。
	if size > 0 {
		if err := out.Truncate(size); err != nil {
			return err
		}
	}

	for _, op := range p.Ops {
		if err := applyOperation(idx, open, out, op, p.Name, doneBytes, totalBytes, onProgress); err != nil {
			return err
		}
	}
	return out.Sync()
}

func applyOperation(idx *PayloadIndex, open SegmentOpener, dst *os.File, op PayloadOp, part string, doneBytes *int64, totalBytes int64, onProgress func(string, int)) error {
	switch op.Type {
	case OpZero, OpDiscard:
		// 目标区域本身就是全零，无需写入。
		return nil

	case OpReplace, OpReplaceBz, OpReplaceXz:
	default:
		return fmt.Errorf("该包为增量 OTA（操作类型 %d），需要原厂底包才能还原，暂不支持", op.Type)
	}

	if len(op.Dst) == 0 {
		return fmt.Errorf("操作缺少目标区间")
	}

	rc, err := open(op.DataOffset, op.DataLength)
	if err != nil {
		return err
	}
	defer rc.Close()

	// 用压缩数据的读取量做进度，解压比未知也不影响百分比准确度。
	// 解压器会频繁小块读取，这里只在百分比变化时回调，避免刷屏。
	lastPct := -1
	counting := &countingReader{r: rc, onRead: func(n int64) {
		*doneBytes += n
		if onProgress == nil || totalBytes <= 0 {
			return
		}
		pct := int(*doneBytes * 100 / totalBytes)
		if pct > 99 && *doneBytes < totalBytes {
			pct = 99
		}
		if pct != lastPct {
			lastPct = pct
			onProgress(part, pct)
		}
	}}

	var r io.Reader = counting
	switch op.Type {
	case OpReplaceBz:
		r = bzip2.NewReader(counting)
	case OpReplaceXz:
		xr, err := xz.NewReader(counting)
		if err != nil {
			return err
		}
		r = xr
	}

	w := &extentWriter{f: dst, extents: op.Dst, blockSize: idx.BlockSize}
	if _, err := io.Copy(w, r); err != nil {
		return err
	}
	if onProgress != nil && totalBytes > 0 && *doneBytes >= totalBytes && lastPct < 100 {
		onProgress(part, 100)
	}
	return nil
}

// countingReader 统计已读取的字节数。
type countingReader struct {
	r      io.Reader
	onRead func(int64)
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n > 0 && c.onRead != nil {
		c.onRead(int64(n))
	}
	return n, err
}

// extentWriter 把一段连续的数据流按 dst extents 分散写入文件。
type extentWriter struct {
	f         *os.File
	extents   []PayloadExtent
	blockSize int64
	idx       int
	pos       int64 // 当前 extent 内已写入的字节数
}

func (w *extentWriter) Write(p []byte) (int, error) {
	total := 0
	for len(p) > 0 {
		if w.idx >= len(w.extents) {
			return total, fmt.Errorf("分区数据比清单声明的目标区间更长")
		}
		e := w.extents[w.idx]
		limit := e.NumBlocks * w.blockSize
		if w.pos >= limit {
			w.idx++
			w.pos = 0
			continue
		}
		n := int64(len(p))
		if n > limit-w.pos {
			n = limit - w.pos
		}
		offset := e.StartBlock*w.blockSize + w.pos
		if _, err := w.f.WriteAt(p[:n], offset); err != nil {
			return total, err
		}
		total += int(n)
		w.pos += n
		p = p[n:]
		if w.pos >= limit {
			w.idx++
			w.pos = 0
		}
	}
	return total, nil
}
