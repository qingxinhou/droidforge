package lp

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

// readerAt 是 os.File 与本包 sparse 读取器的公共接口。
type readerAt interface {
	io.ReaderAt
	Close() error
}

const (
	sparseMagic = 0xED26FF3A

	chunkRaw      = 0xCAC1
	chunkFill     = 0xCAC2
	chunkDontCare = 0xCAC3
	chunkCRC32    = 0xCAC4
)

// openImage 打开镜像，自动识别 Android sparse 并给出逻辑大小。
//
// super.img 从线刷包里解出来经常是 sparse 的，直接按物理偏移读 geometry 会读空。
func openImage(path string) (readerAt, int64, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, false, err
	}
	head := make([]byte, 28)
	if _, err := io.ReadFull(f, head); err != nil {
		f.Close()
		return nil, 0, false, fmt.Errorf("文件过小，不是有效镜像：%w", err)
	}
	if binary.LittleEndian.Uint32(head[0:4]) != sparseMagic {
		st, err := f.Stat()
		if err != nil {
			f.Close()
			return nil, 0, false, err
		}
		return f, st.Size(), false, nil
	}

	sp, err := buildSparseIndex(f, head)
	if err != nil {
		f.Close()
		return nil, 0, false, err
	}
	return sp, sp.size, true, nil
}

// sparseChunk 记录一个 sparse 块在文件里的位置与逻辑范围。
type sparseChunk struct {
	logicalStart int64
	logicalLen   int64
	fileOffset   int64
	kind         uint16
	fill         [4]byte
}

type sparseReader struct {
	f      *os.File
	size   int64
	chunks []sparseChunk
}

// buildSparseIndex 顺序扫一遍块头，建立「逻辑地址 → 文件偏移」的映射。
func buildSparseIndex(f *os.File, head []byte) (*sparseReader, error) {
	le := binary.LittleEndian
	fileHdrSize := int64(le.Uint16(head[8:10]))
	chunkHdrSize := int64(le.Uint16(head[10:12]))
	blkSize := int64(le.Uint32(head[12:16]))
	totalChunks := int(le.Uint32(head[20:24]))
	if blkSize <= 0 || totalChunks <= 0 {
		return nil, errors.New("sparse 头字段异常")
	}

	sp := &sparseReader{f: f}
	offset := fileHdrSize
	hdr := make([]byte, chunkHdrSize)
	var logical int64

	for i := 0; i < totalChunks; i++ {
		if _, err := f.ReadAt(hdr, offset); err != nil {
			return nil, fmt.Errorf("读取 sparse 块头 %d 失败：%w", i, err)
		}
		kind := le.Uint16(hdr[0:2])
		blocks := int64(le.Uint32(hdr[4:8]))
		totalSize := int64(le.Uint32(hdr[8:12]))
		if totalSize < chunkHdrSize {
			return nil, errors.New("sparse 块长度异常")
		}
		logicalLen := blocks * blkSize

		c := sparseChunk{
			logicalStart: logical,
			logicalLen:   logicalLen,
			fileOffset:   offset + chunkHdrSize,
			kind:         kind,
		}
		if kind == chunkFill {
			if _, err := f.ReadAt(c.fill[:], c.fileOffset); err != nil {
				return nil, err
			}
		}
		sp.chunks = append(sp.chunks, c)
		logical += logicalLen
		offset += totalSize
	}

	sp.size = logical
	return sp, nil
}

// ReadAt 把逻辑地址映射回 sparse 块。跨块读取时逐段拼接。
func (s *sparseReader) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, errors.New("偏移为负")
	}
	if off >= s.size {
		return 0, io.EOF
	}
	want := len(p)
	if int64(want) > s.size-off {
		want = int(s.size - off)
	}

	done := 0
	for done < want {
		cur := off + int64(done)
		idx := s.find(cur)
		if idx < 0 {
			// 落在块与块之间的空隙：补零。
			remain := want - done
			if remain > 4096 {
				remain = 4096
			}
			for i := 0; i < remain; i++ {
				p[done+i] = 0
			}
			done += remain
			continue
		}
		c := s.chunks[idx]
		inner := cur - c.logicalStart
		n := int(c.logicalLen - inner)
		if n > want-done {
			n = want - done
		}
		switch c.kind {
		case chunkRaw:
			if _, err := s.f.ReadAt(p[done:done+n], c.fileOffset+inner); err != nil && err != io.EOF {
				return done, err
			}
		case chunkFill:
			for i := 0; i < n; i++ {
				p[done+i] = c.fill[(inner+int64(i))%4]
			}
		default: // dontcare / crc / 未知块一律当作零
			for i := 0; i < n; i++ {
				p[done+i] = 0
			}
		}
		done += n
	}

	if done < len(p) {
		return done, io.EOF
	}
	return done, nil
}

// find 二分查找包含逻辑地址的块。
func (s *sparseReader) find(off int64) int {
	lo, hi := 0, len(s.chunks)-1
	for lo <= hi {
		mid := (lo + hi) / 2
		c := s.chunks[mid]
		switch {
		case off < c.logicalStart:
			hi = mid - 1
		case off >= c.logicalStart+c.logicalLen:
			lo = mid + 1
		default:
			return mid
		}
	}
	return -1
}

func (s *sparseReader) Close() error { return s.f.Close() }
