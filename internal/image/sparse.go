package image

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

const sparseMagic = 0xED26FF3A

const (
	chunkRaw      = 0xCAC1
	chunkFill     = 0xCAC2
	chunkDontCare = 0xCAC3
	chunkCRC32    = 0xCAC4
)

// IsSparse 判断文件是否为 Android sparse image。
func IsSparse(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, 4)
	if _, err := io.ReadFull(f, head); err != nil {
		return false
	}
	return binary.LittleEndian.Uint32(head) == sparseMagic
}

// Unsparse 将 sparse image 还原为 raw image。
func Unsparse(srcPath, dstPath string, onProgress func(percent int)) error {
	in, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer in.Close()

	hdr := make([]byte, 28)
	if _, err := io.ReadFull(in, hdr); err != nil {
		return fmt.Errorf("读取 sparse 头失败：%w", err)
	}
	if binary.LittleEndian.Uint32(hdr[0:4]) != sparseMagic {
		return fmt.Errorf("不是 sparse 镜像")
	}
	fileHdrSize := int(binary.LittleEndian.Uint16(hdr[8:10]))
	chunkHdrSize := int(binary.LittleEndian.Uint16(hdr[10:12]))
	blkSize := int64(binary.LittleEndian.Uint32(hdr[12:16]))
	totalChunks := int(binary.LittleEndian.Uint32(hdr[20:24]))

	if fileHdrSize > 28 {
		if _, err := in.Seek(int64(fileHdrSize), io.SeekStart); err != nil {
			return err
		}
	}

	out, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer out.Close()

	outBuf := make([]byte, 1<<20)
	chunkHdr := make([]byte, chunkHdrSize)

	for i := 0; i < totalChunks; i++ {
		if _, err := io.ReadFull(in, chunkHdr); err != nil {
			return fmt.Errorf("读取 chunk 头失败：%w", err)
		}
		ctype := binary.LittleEndian.Uint16(chunkHdr[0:2])
		chunkBlocks := int64(binary.LittleEndian.Uint32(chunkHdr[4:8]))
		totalSize := int64(binary.LittleEndian.Uint32(chunkHdr[8:12]))
		dataSize := totalSize - int64(chunkHdrSize)

		switch ctype {
		case chunkRaw:
			if _, err := io.CopyN(out, in, dataSize); err != nil {
				return err
			}
		case chunkFill:
			fill := make([]byte, 4)
			if _, err := io.ReadFull(in, fill); err != nil {
				return err
			}
			size := chunkBlocks * blkSize
			pattern := make([]byte, len(outBuf))
			for j := 0; j < len(outBuf); j += 4 {
				copy(pattern[j:j+4], fill)
			}
			for size > 0 {
				n := int64(len(pattern))
				if size < n {
					n = size
				}
				if _, err := out.Write(pattern[:n]); err != nil {
					return err
				}
				size -= n
			}
		case chunkDontCare:
			size := chunkBlocks * blkSize
			zero := make([]byte, len(outBuf))
			for size > 0 {
				n := int64(len(zero))
				if size < n {
					n = size
				}
				if _, err := out.Write(zero[:n]); err != nil {
					return err
				}
				size -= n
			}
		case chunkCRC32:
			if _, err := io.CopyN(io.Discard, in, dataSize); err != nil {
				return err
			}
		default:
			// 未知 chunk 类型，按声明长度跳过
			if _, err := io.CopyN(io.Discard, in, dataSize); err != nil {
				return err
			}
		}

		if onProgress != nil && totalChunks > 0 {
			onProgress((i + 1) * 100 / totalChunks)
		}
	}
	return out.Sync()
}
