package lp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// SplitResult 是拆分出的单个镜像。
type SplitResult struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// Split 把 super 镜像里的逻辑分区按需导出为独立 .img。
//
// names 为空时导出全部分区。导出的镜像是 raw（非 sparse），
// 大小严格等于分区 extents 之和，可直接用于 fastboot flash。
func Split(ctx context.Context, srcPath, outDir string, names []string, onProgress func(label string, percent int)) ([]SplitResult, error) {
	meta, err := Parse(srcPath)
	if err != nil {
		return nil, err
	}
	if len(meta.Partitions) == 0 {
		return nil, errors.New("super 里没有任何逻辑分区")
	}

	wanted := map[string]bool{}
	for _, n := range names {
		wanted[strings.TrimSpace(n)] = true
	}

	ra, _, _, err := openImage(srcPath)
	if err != nil {
		return nil, err
	}
	defer ra.Close()

	if outDir != "" {
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			return nil, err
		}
	}

	var out []SplitResult
	buf := make([]byte, 4<<20)
	total := len(meta.Partitions)
	done := 0

	for _, p := range meta.Partitions {
		if len(wanted) > 0 && !wanted[p.Name] {
			continue
		}
		if len(p.Extents) == 0 {
			continue
		}
		if err := ctx.Err(); err != nil {
			return out, fmt.Errorf("已中止：%w", err)
		}
		if onProgress != nil {
			onProgress(p.Name, done*100/total)
		}
		dst := filepath.Join(outDir, p.Name+".img")
		if err := writePartition(ra, p, dst, buf); err != nil {
			return out, fmt.Errorf("导出 %s 失败：%w", p.Name, err)
		}
		out = append(out, SplitResult{Name: p.Name, Path: dst, Size: p.SizeBytes})
		done++
	}
	if len(out) == 0 {
		return nil, errors.New("选中的分区在镜像里不存在")
	}
	if onProgress != nil {
		onProgress("完成", 100)
	}
	return out, nil
}

// writePartition 把分区的各段 extent 按逻辑顺序拼接写出。
func writePartition(ra readerAt, p Partition, dst string, buf []byte) error {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, e := range p.Extents {
		length := int64(e.NumSectors) * sectorSize
		switch e.TargetType {
		case 0:
			// 多块设备（虚拟 A/B 的 _b 槽位落在另一块设备上）目前不处理。
			if e.TargetSource != 0 {
				return fmt.Errorf("分区 %s 引用了第 %d 块设备，暂不支持", p.Name, e.TargetSource)
			}
			sr := io.NewSectionReader(ra, int64(e.TargetData)*sectorSize, length)
			if _, err := io.CopyBuffer(f, sr, buf); err != nil {
				return err
			}
		case 1:
			zero := make([]byte, 64<<10)
			for length > 0 {
				n := int64(len(zero))
				if length < n {
					n = length
				}
				if _, err := f.Write(zero[:n]); err != nil {
					return err
				}
				length -= n
			}
		default:
			return fmt.Errorf("分区 %s 含未知 extent 类型 %d", p.Name, e.TargetType)
		}
	}
	return f.Sync()
}
