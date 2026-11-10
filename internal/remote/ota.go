package remote

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"droidforge/internal/image"
)

// Partition 是远程 payload 中的一个分区。
type Partition struct {
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	Compressed  int64  `json:"compressed"` // 实际需要下载的压缩数据量
	Operations  int    `json:"operations"`
	Incremental bool   `json:"incremental"`
}

// Info 是远程 OTA 的解析结果。
type Info struct {
	URL           string      `json:"url"`
	FileName      string      `json:"fileName"`
	TotalSize     int64       `json:"totalSize"`
	SupportsRange bool        `json:"supportsRange"`
	Container     string      `json:"container"` // zip / payload
	PayloadOffset int64       `json:"payloadOffset"`
	PayloadSize   int64       `json:"payloadSize"`
	Version       int         `json:"version"`
	BlockSize     int64       `json:"blockSize"`
	Full          bool        `json:"full"`
	Partitions    []Partition `json:"partitions"`
	Note          string      `json:"note"`
}

// payloadHandle 保存已定位的 payload 位置与索引。
type payloadHandle struct {
	file      *File
	offset    int64
	size      int64
	index     *image.PayloadIndex
	container string
}

func (h *payloadHandle) dataStart() int64 {
	return h.offset + h.index.DataStart()
}

// locatePayload 在远程文件中定位 payload.bin。
func locatePayload(f *File) (offset, size int64, container string, err error) {
	// 直接就是 payload.bin
	head := make([]byte, 4)
	if f.Size >= 4 {
		if _, rerr := f.ReadAt(head, 0); rerr == nil && string(head) == "CrAU" {
			return 0, f.Size, "payload", nil
		}
	}

	entry, err := f.FindEntry("payload.bin")
	if err != nil {
		return 0, 0, "", fmt.Errorf("%w（也可能是线刷包，请下载后用「刷机包」页处理）", err)
	}
	if !entry.Stored() {
		return 0, 0, "", errors.New("payload.bin 在压缩包内被 deflate 压缩，无法远程分段解析，请改用「下载整包」")
	}
	size = entry.UncompressedSize
	if size <= 0 {
		size = entry.CompressedSize
	}
	return entry.DataOffset, size, "zip", nil
}

// openPayload 打开远程 payload 并解析清单。
func openPayload(f *File) (*payloadHandle, error) {
	offset, size, container, err := locatePayload(f)
	if err != nil {
		return nil, err
	}
	if size < 20 {
		return nil, errors.New("payload 数据过小，不是有效文件")
	}

	readLen := int64(24)
	if readLen > size {
		readLen = size
	}
	head := make([]byte, readLen)
	if _, err := f.ReadAt(head, offset); err != nil {
		return nil, fmt.Errorf("读取 payload 头部失败：%w", err)
	}
	version, manifestSize, metaSig, err := image.ParsePayloadHead(head)
	if err != nil {
		return nil, err
	}

	headerLen := image.HeadLen(version)
	if offset+headerLen+manifestSize > f.Size {
		return nil, errors.New("payload 清单超出文件范围，链接可能不完整")
	}
	manifest := make([]byte, manifestSize)
	if _, err := f.ReadAt(manifest, offset+headerLen); err != nil {
		return nil, fmt.Errorf("读取 payload 清单失败：%w", err)
	}
	idx, err := image.NewPayloadIndex(version, manifestSize, metaSig, manifest)
	if err != nil {
		return nil, err
	}
	return &payloadHandle{file: f, offset: offset, size: size, index: idx, container: container}, nil
}

func buildInfo(f *File, h *payloadHandle) *Info {
	info := &Info{
		URL:           f.URL,
		FileName:      f.Name,
		TotalSize:     f.Size,
		SupportsRange: f.Range,
		Container:     h.container,
		PayloadOffset: h.offset,
		PayloadSize:   h.size,
		Version:       h.index.Version,
		BlockSize:     h.index.BlockSize,
		Full:          true,
	}
	incremental := false
	for _, p := range h.index.Partitions {
		part := Partition{
			Name:       p.Name,
			Size:       p.Size,
			Operations: len(p.Ops),
		}
		if part.Size == 0 {
			part.Size = image.PartitionSizeFromOps(p.Ops, h.index.BlockSize)
		}
		for _, op := range p.Ops {
			part.Compressed += op.DataLength
			switch op.Type {
			case image.OpReplace, image.OpReplaceBz, image.OpReplaceXz, image.OpZero, image.OpDiscard:
			default:
				part.Incremental = true
				incremental = true
			}
		}
		info.Partitions = append(info.Partitions, part)
	}
	sort.SliceStable(info.Partitions, func(i, j int) bool {
		return info.Partitions[i].Name < info.Partitions[j].Name
	})
	info.Full = !incremental
	switch {
	case incremental:
		info.Note = "检测到增量 OTA，部分分区需要原厂底包才能还原"
	case h.container == "zip":
		info.Note = "已远程定位 payload.bin，只会下载选中分区实际用到的数据"
	default:
		info.Note = "直接以 payload.bin 解析"
	}
	return info
}

// Inspect 远程解析 OTA 链接，不下载整包。
func Inspect(ctx context.Context, rawURL string, opt Options) (*Info, error) {
	f, err := Open(ctx, rawURL, opt)
	if err != nil {
		return nil, err
	}
	info := &Info{
		URL:           f.URL,
		FileName:      f.Name,
		TotalSize:     f.Size,
		SupportsRange: f.Range,
	}
	if !f.Range {
		info.Note = "服务器不支持 Range 请求（常见于重定向到镜像站后），无法远程分段解析，请使用「下载整包」"
		return info, nil
	}
	h, err := openPayload(f)
	if err != nil {
		return nil, err
	}
	return buildInfo(f, h), nil
}

// Extract 从远程 OTA 中按需提取指定分区（names 为空表示全部）。
func Extract(ctx context.Context, rawURL string, names []string, outDir string, opt Options, onProgress func(string, int)) ([]string, error) {
	f, err := Open(ctx, rawURL, opt)
	if err != nil {
		return nil, err
	}
	if !f.Range {
		return nil, ErrNoRange
	}
	h, err := openPayload(f)
	if err != nil {
		return nil, err
	}
	base := h.dataStart()
	open := func(offset, length int64) (io.ReadCloser, error) {
		return f.Reader(base+offset, length), nil
	}
	return image.ExtractWith(h.index, open, names, outDir, onProgress)
}

// Download 下载整包到 outDir，返回本地路径。
func Download(ctx context.Context, rawURL, outDir string, opt Options, onProgress func(string, int)) (string, error) {
	f, err := Open(ctx, rawURL, opt)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(outDir, sanitizeName(f.Name))
	report := func(p int) {
		if onProgress != nil {
			onProgress(filepath.Base(dst), p)
		}
	}
	if _, err := downloadTo(ctx, rawURL, dst, opt, report); err != nil {
		return "", err
	}
	return dst, nil
}

func sanitizeName(name string) string {
	name = strings.TrimSpace(filepath.Base(name))
	name = strings.ReplaceAll(name, ":", "_")
	if name == "" || name == "." || name == ".." {
		return "ota.zip"
	}
	return name
}
