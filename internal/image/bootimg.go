// Package image 负责 Android 镜像解析：boot.img / payload.bin / sparse。
package image

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	bootMagic       = "ANDROID!"
	vendorBootMagic = "VNDRBOOT"
)

// BootImageInfo 是 boot / init_boot / vendor_boot 镜像的头部信息。
type BootImageInfo struct {
	Path          string   `json:"path"`
	Kind          string   `json:"kind"` // boot / init_boot / vendor_boot
	HeaderVersion int      `json:"headerVersion"`
	PageSize      int      `json:"pageSize"`
	KernelSize    int64    `json:"kernelSize"`
	RamdiskSize   int64    `json:"ramdiskSize"`
	SecondSize    int64    `json:"secondSize"`
	DtbSize       int64    `json:"dtbSize"`
	RecoveryDtbo  int64    `json:"recoveryDtboSize"`
	SignatureSize int64    `json:"signatureSize"`
	OSVersion     string   `json:"osVersion"`
	Name          string   `json:"name"`
	Cmdline       string   `json:"cmdline"`
	FileSize      int64    `json:"fileSize"`
	Sections      []string `json:"sections"`
}

// IdentifyImage 粗略识别文件类型，便于前端展示。
func IdentifyImage(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return "unknown"
	}
	defer f.Close()

	head := make([]byte, 8)
	if _, err := io.ReadFull(f, head); err != nil {
		return "unknown"
	}
	switch {
	case string(head) == bootMagic:
		return "boot"
	case string(head) == vendorBootMagic:
		return "vendor_boot"
	case string(head[:4]) == "CrAU":
		return "payload"
	case string(head[:2]) == "PK":
		return "zip"
	case len(head) >= 4 && binary.LittleEndian.Uint32(head[:4]) == 0xED26FF3A:
		return "sparse"
	case len(head) >= 4 && binary.LittleEndian.Uint32(head[0:4]) == 0x53EF:
		return "ext4"
	}
	// erofs magic 0xE0F5E1E2 at offset 1024
	if _, err := f.Seek(1024, io.SeekStart); err == nil {
		buf := make([]byte, 4)
		if _, err := io.ReadFull(f, buf); err == nil && binary.LittleEndian.Uint32(buf) == 0xE0F5E1E2 {
			return "erofs"
		}
	}
	return "unknown"
}

// ParseBoot 解析 boot 镜像头部。
func ParseBoot(path string) (*BootImageInfo, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	head := make([]byte, 4096)
	n, _ := io.ReadFull(f, head)
	if n < 64 {
		return nil, fmt.Errorf("文件过小，不是有效的 boot 镜像")
	}

	info := &BootImageInfo{Path: path, FileSize: st.Size(), Kind: "boot"}
	magic := string(head[:8])

	if magic == vendorBootMagic {
		return parseVendorBoot(head, info)
	}
	if magic != bootMagic {
		return nil, fmt.Errorf("不是 Android boot 镜像（magic=%q）", strings.TrimRight(magic, "\x00"))
	}

	le := binary.LittleEndian
	info.KernelSize = int64(le.Uint32(head[8:12]))
	info.RamdiskSize = int64(le.Uint32(head[16:20]))
	info.SecondSize = int64(le.Uint32(head[24:28]))
	info.PageSize = int(le.Uint32(head[36:40]))
	headerVersion := int(le.Uint32(head[40:44]))
	osVer := le.Uint32(head[44:48])

	if headerVersion >= 3 {
		// v3/v4 布局
		info.HeaderVersion = headerVersion
		info.PageSize = 4096
		info.KernelSize = int64(le.Uint32(head[8:12]))
		info.RamdiskSize = int64(le.Uint32(head[12:16]))
		info.OSVersion = formatOSVersion(le.Uint32(head[16:20]))
		info.Cmdline = cstr(head[44 : 44+1536])
		if headerVersion >= 4 && n >= 1584 {
			info.SignatureSize = int64(le.Uint32(head[1580:1584]))
		}
		if info.RamdiskSize == 0 {
			info.Kind = "init_boot"
		}
	} else {
		info.HeaderVersion = headerVersion
		info.OSVersion = formatOSVersion(osVer)
		info.Name = cstr(head[48:64])
		info.Cmdline = cstr(head[64 : 64+512])
		if n >= 1624 {
			info.RecoveryDtbo = int64(le.Uint32(head[1608:1612]))
			info.DtbSize = int64(le.Uint32(head[1624:1628]))
		}
	}
	if info.PageSize <= 0 || info.PageSize > 65536 {
		info.PageSize = 2048
	}

	info.Sections = bootSections(info)
	return info, nil
}

func parseVendorBoot(head []byte, info *BootImageInfo) (*BootImageInfo, error) {
	le := binary.LittleEndian
	info.Kind = "vendor_boot"
	info.HeaderVersion = int(le.Uint32(head[8:12]))
	info.PageSize = int(le.Uint32(head[12:16]))
	info.KernelSize = int64(le.Uint32(head[16:20]))
	info.RamdiskSize = int64(le.Uint32(head[20:24]))
	info.Cmdline = cstr(head[28 : 28+2048])
	info.Name = "vendor_boot"
	info.Sections = []string{"vendor_ramdisk", "dtb"}
	return info, nil
}

func bootSections(info *BootImageInfo) []string {
	var s []string
	if info.KernelSize > 0 {
		s = append(s, "kernel")
	}
	if info.RamdiskSize > 0 {
		s = append(s, "ramdisk")
	}
	if info.SecondSize > 0 {
		s = append(s, "second")
	}
	if info.RecoveryDtbo > 0 {
		s = append(s, "recovery_dtbo")
	}
	if info.DtbSize > 0 {
		s = append(s, "dtb")
	}
	if info.SignatureSize > 0 {
		s = append(s, "signature")
	}
	return s
}

func cstr(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

func formatOSVersion(v uint32) string {
	if v == 0 {
		return ""
	}
	a := (v >> 14) & 0x7f
	b := (v >> 7) & 0x7f
	c := v & 0x7f
	year := (v >> 25) + 2000
	month := (v >> 21) & 0xf
	return fmt.Sprintf("%d.%d.%d (%d-%02d)", a, b, c, year, month)
}

func align(v, page int64) int64 {
	if page <= 0 {
		return v
	}
	if r := v % page; r != 0 {
		return v + page - r
	}
	return v
}

// UnpackBoot 拆分 boot 镜像为 kernel / ramdisk / second / dtb 等文件。
func UnpackBoot(path, outDir string) ([]string, error) {
	info, err := ParseBoot(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	page := int64(info.PageSize)
	var offset int64
	var plan []struct {
		name string
		size int64
	}

	if info.HeaderVersion >= 3 {
		offset = 4096
		plan = []struct {
			name string
			size int64
		}{{"kernel", info.KernelSize}, {"ramdisk", info.RamdiskSize}}
	} else {
		offset = page
		plan = []struct {
			name string
			size int64
		}{{"kernel", info.KernelSize}, {"ramdisk", info.RamdiskSize}, {"second", info.SecondSize}}
	}

	var written []string
	for _, item := range plan {
		if item.size <= 0 {
			continue
		}
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return written, err
		}
		dst := filepath.Join(outDir, item.name+".img")
		if err := copyN(f, dst, item.size); err != nil {
			return written, err
		}
		written = append(written, dst)
		offset += align(item.size, page)
	}

	// v0-v2 的 dtb 位于 recovery_dtbo 之后
	if info.HeaderVersion < 3 && info.DtbSize > 0 {
		dtbOffset := offset + align(info.RecoveryDtbo, page)
		if _, err := f.Seek(dtbOffset, io.SeekStart); err == nil {
			dst := filepath.Join(outDir, "dtb.img")
			if err := copyN(f, dst, info.DtbSize); err == nil {
				written = append(written, dst)
			}
		}
	}
	if info.HeaderVersion < 3 && info.RecoveryDtbo > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err == nil {
			dst := filepath.Join(outDir, "recovery_dtbo.img")
			if err := copyN(f, dst, info.RecoveryDtbo); err == nil {
				written = append(written, dst)
			}
		}
	}
	return written, nil
}

func copyN(r io.Reader, dst string, n int64) error {
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.CopyN(out, r, n)
	return err
}
