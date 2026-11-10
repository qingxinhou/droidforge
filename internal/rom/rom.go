// Package rom 解析各类刷机包（OTA zip / 小米 tgz / 三星 tar.md5 / 文件夹）。
package rom

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pierrec/lz4/v4"

	"droidforge/internal/image"
)

// Entry 描述包内一个文件。
type Entry struct {
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	Kind      string `json:"kind"` // image / payload / metadata / other
	Partition string `json:"partition"`
}

// PackageInfo 是刷机包概览。
type PackageInfo struct {
	Path       string   `json:"path"`
	Type       string   `json:"type"` // zip / tar / folder / image
	Kind       string   `json:"kind"` // ota / fastboot / odin / unknown
	TotalSize  int64    `json:"totalSize"`
	Entries    []Entry  `json:"entries"`
	Partitions []string `json:"partitions"`
	HasPayload bool     `json:"hasPayload"`
	Note       string   `json:"note"`
}

var imageExts = []string{".img", ".img.lz4", ".bin"}

// PartitionFromFilename 从镜像文件名推断分区名。
func PartitionFromFilename(name string) string {
	base := filepath.Base(name)
	base = strings.TrimSuffix(base, ".lz4")
	for _, ext := range []string{".img", ".bin"} {
		base = strings.TrimSuffix(base, ext)
	}
	// 去掉槽位后缀
	base = strings.TrimSuffix(base, "_a")
	base = strings.TrimSuffix(base, "_b")
	return base
}

func entryKind(name string) (string, string) {
	lower := strings.ToLower(filepath.Base(name))
	switch {
	case lower == "payload.bin":
		return "payload", "payload"
	case lower == "payload_properties.txt":
		return "metadata", ""
	case lower == "care_map.pb", lower == "care_map.txt", lower == "metadata":
		return "metadata", ""
	}
	for _, ext := range imageExts {
		if strings.HasSuffix(lower, ext) {
			return "image", PartitionFromFilename(lower)
		}
	}
	return "other", ""
}

// Inspect 识别并列出刷机包内容。
func Inspect(path string) (*PackageInfo, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	info := &PackageInfo{Path: path}

	if st.IsDir() {
		info.Type = "folder"
		info.Kind = "fastboot"
		return inspectFolder(info, path)
	}

	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		return inspectZip(info, path, st.Size())
	case strings.HasSuffix(lower, ".ofp"):
		// OFP 是欧加系（OPPO / 一加 / 真我）的刷机包，公开的 OFP 就是个 zip。
		// 加密变体在 zip 头之前还有一段私有数据，zip.OpenReader 会直接失败，
		// 这里如实说明而不是硬猜密钥。
		pi, err := inspectZip(info, path, st.Size())
		if err != nil {
			return nil, fmt.Errorf("这个 .ofp 不是标准 zip 容器（可能被加密或加了私有头部），本工具不解析加密包：%w", err)
		}
		if pi.Kind == "unknown" {
			pi.Kind = "ofp"
		}
		pi.Note = "OFP 刷机包（欧加系）：" + pi.Note
		return pi, nil
	case strings.HasSuffix(lower, ".ops"):
		return nil, fmt.Errorf("OPS 是 OPPO 早期的私有打包格式，没有公开规范，无法保证解析正确。请改用 OFP 包，或先用官方工具转成镜像再提取")
	case strings.HasSuffix(lower, ".tar"), strings.HasSuffix(lower, ".tar.md5"),
		strings.HasSuffix(lower, ".tgz"), strings.HasSuffix(lower, ".tar.gz"):
		return inspectTar(info, path, st.Size())
	case isImageFile(lower):
		info.Type = "image"
		info.Kind = "image"
		kind, part := entryKind(lower)
		info.Entries = append(info.Entries, Entry{
			Name: filepath.Base(path), Size: st.Size(), Kind: kind, Partition: part,
		})
		if part != "" {
			info.Partitions = append(info.Partitions, part)
		}
		info.TotalSize = st.Size()
		return info, nil
	}
	return nil, fmt.Errorf("暂不支持的文件类型：%s", filepath.Ext(path))
}

func isImageFile(p string) bool {
	for _, ext := range imageExts {
		if strings.HasSuffix(p, ext) {
			return true
		}
	}
	return false
}

func inspectFolder(info *PackageInfo, dir string) (*PackageInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		kind, part := entryKind(name)
		if kind != "image" {
			continue
		}
		fi, _ := e.Info()
		var size int64
		if fi != nil {
			size = fi.Size()
		}
		info.Entries = append(info.Entries, Entry{Name: name, Size: size, Kind: kind, Partition: part})
		if part != "" {
			info.Partitions = appendUnique(info.Partitions, part)
		}
		info.TotalSize += size
	}
	info.Note = fmt.Sprintf("识别到 %d 个镜像文件", len(info.Entries))
	return info, nil
}

func inspectZip(info *PackageInfo, path string, size int64) (*PackageInfo, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("打开 zip 失败：%w", err)
	}
	defer zr.Close()
	info.Type = "zip"
	info.TotalSize = size

	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		kind, part := entryKind(f.Name)
		info.Entries = append(info.Entries, Entry{Name: f.Name, Size: int64(f.UncompressedSize64), Kind: kind, Partition: part})
		if kind == "payload" {
			info.HasPayload = true
		}
		if part != "" {
			info.Partitions = appendUnique(info.Partitions, part)
		}
	}
	switch {
	case info.HasPayload:
		info.Kind = "ota"
		info.Note = "Android OTA 整包（含 payload.bin），可从 payload 中提取各分区"
		// 只读取 payload 清单，不解压数据区，快速获知分区列表
		for _, f := range zr.File {
			if !strings.EqualFold(filepath.Base(f.Name), "payload.bin") {
				continue
			}
			if rc, err := f.Open(); err == nil {
				if pi, perr := image.ParsePayloadStream(rc); perr == nil {
					for _, p := range pi.Partitions {
						info.Partitions = appendUnique(info.Partitions, p.Name)
						if p.Incremental {
							info.Note = "该 OTA 为增量包，部分分区无法独立还原，建议使用整包（full OTA）"
						}
					}
				}
				rc.Close()
			}
			break
		}
	case len(info.Partitions) > 0:
		info.Kind = "fastboot"
		info.Note = "包含分区镜像的 zip 包"
	default:
		info.Kind = "unknown"
		info.Note = "未在压缩包内找到可识别的镜像"
	}
	return info, nil
}

func inspectTar(info *PackageInfo, path string, size int64) (*PackageInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var r io.Reader = f
	for _, suffix := range []string{".tgz", ".tar.gz"} {
		if strings.HasSuffix(strings.ToLower(path), suffix) {
			gz, err := gzip.NewReader(f)
			if err != nil {
				return nil, err
			}
			defer gz.Close()
			r = gz
			break
		}
	}

	tr := tar.NewReader(r)
	info.Type = "tar"
	info.TotalSize = size
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("读取 tar 失败：%w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		kind, part := entryKind(hdr.Name)
		info.Entries = append(info.Entries, Entry{Name: hdr.Name, Size: hdr.Size, Kind: kind, Partition: part})
		if part != "" {
			info.Partitions = appendUnique(info.Partitions, part)
		}
	}
	if strings.HasSuffix(strings.ToLower(path), ".tar.md5") {
		info.Kind = "odin"
		info.Note = "三星 ODIN 固件包（tar.md5）"
	} else {
		info.Kind = "fastboot"
		info.Note = "线刷包（tar），可直接按分区刷入"
	}
	return info, nil
}

func appendUnique(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}

// Extract 从包中提取指定分区镜像。entries 匹配 Entry.Name 或分区名；为空则全部。
func Extract(path string, names []string, outDir string, onProgress func(string, int)) ([]string, error) {
	info, err := Inspect(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	match := func(e Entry) bool {
		if len(want) == 0 {
			return e.Kind == "image"
		}
		return want[e.Name] || want[e.Partition]
	}

	switch info.Type {
	case "image":
		dst := filepath.Join(outDir, filepath.Base(path))
		if err := copyFile(path, dst); err != nil {
			return nil, err
		}
		return []string{dst}, nil

	case "folder":
		var out []string
		for _, e := range info.Entries {
			if !match(e) {
				continue
			}
			src := filepath.Join(path, e.Name)
			dst := filepath.Join(outDir, filepath.Base(e.Name))
			if err := copyFile(src, dst); err != nil {
				return out, err
			}
			out = append(out, dst)
		}
		return out, nil

	case "zip":
		return extractZip(path, info, names, match, outDir, onProgress)

	case "tar":
		return extractTar(path, info, match, outDir, onProgress)
	}
	return nil, fmt.Errorf("不支持的包类型：%s", info.Type)
}

func extractZip(path string, info *PackageInfo, names []string, match func(Entry) bool, outDir string, onProgress func(string, int)) ([]string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	// 优先处理 payload.bin
	var payloadEntry *zip.File
	for _, f := range zr.File {
		if strings.EqualFold(filepath.Base(f.Name), "payload.bin") {
			payloadEntry = f
			break
		}
	}

	var out []string
	if payloadEntry != nil && len(info.Partitions) > 0 {
		// 判断是否需要从 payload 提取：用户指定了分区，或包内没有独立 img
		hasStandaloneImgs := false
		for _, f := range zr.File {
			if k, _ := entryKind(f.Name); k == "image" {
				hasStandaloneImgs = true
				break
			}
		}
		if !hasStandaloneImgs {
			tmp := filepath.Join(os.TempDir(), fmt.Sprintf("droidforge_payload_%d.bin", os.Getpid()))
			if err := extractZipFile(payloadEntry, tmp); err != nil {
				return nil, err
			}
			defer os.Remove(tmp)

			// 未指定时提取全部；指定时按用户选择
			payloadNames := names
			if len(payloadNames) == 0 {
				payloadNames = info.Partitions
			}
			return image.ExtractPayload(tmp, payloadNames, outDir, onProgress)
		}
	}

	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		kind, part := entryKind(f.Name)
		e := Entry{Name: f.Name, Size: int64(f.UncompressedSize64), Kind: kind, Partition: part}
		if !match(e) {
			continue
		}
		dst := filepath.Join(outDir, filepath.Base(f.Name))
		if err := extractZipFile(f, dst); err != nil {
			return out, err
		}
		if strings.HasSuffix(strings.ToLower(dst), ".lz4") {
			if err := unlz4File(dst, strings.TrimSuffix(dst, ".lz4")); err != nil {
				return out, err
			}
			_ = os.Remove(dst)
			dst = strings.TrimSuffix(dst, ".lz4")
		}
		out = append(out, dst)
		if onProgress != nil {
			onProgress(filepath.Base(dst), 100)
		}
	}
	return out, nil
}

func extractZipFile(f *zip.File, dst string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}

func extractTar(path string, info *PackageInfo, match func(Entry) bool, outDir string, onProgress func(string, int)) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var r io.Reader = f
	for _, suffix := range []string{".tgz", ".tar.gz"} {
		if strings.HasSuffix(strings.ToLower(path), suffix) {
			gz, err := gzip.NewReader(f)
			if err != nil {
				return nil, err
			}
			defer gz.Close()
			r = gz
			break
		}
	}

	tr := tar.NewReader(r)
	var out []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return out, err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		kind, part := entryKind(hdr.Name)
		e := Entry{Name: hdr.Name, Size: hdr.Size, Kind: kind, Partition: part}
		if !match(e) {
			continue
		}
		dst := filepath.Join(outDir, filepath.Base(hdr.Name))
		if err := writeReader(tr, dst); err != nil {
			return out, err
		}
		if strings.HasSuffix(strings.ToLower(dst), ".lz4") {
			if err := unlz4File(dst, strings.TrimSuffix(dst, ".lz4")); err != nil {
				return out, err
			}
			_ = os.Remove(dst)
			dst = strings.TrimSuffix(dst, ".lz4")
		}
		out = append(out, dst)
		if onProgress != nil {
			onProgress(filepath.Base(dst), 100)
		}
	}
	return out, nil
}

func writeReader(r io.Reader, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, r)
	return err
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	return writeReader(in, dst)
}

func unlz4File(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	r := lz4.NewReader(in)
	return writeReader(r, dst)
}

// FlashStep 是线刷流程的一步。
type FlashStep struct {
	Partition      string `json:"partition"`
	Image          string `json:"image"`
	SizeBytes      int64  `json:"sizeBytes"`
	NeedsFastbootd bool   `json:"needsFastbootd"`
	Note           string `json:"note"`
}

// 线刷顺序：先引导与校验相关，再系统分区。
var flashOrder = []string{
	"boot", "init_boot", "vendor_boot", "dtbo", "vbmeta", "vbmeta_system", "vbmeta_vendor",
	"recovery", "modem", "abl", "xbl", "xbl_config", "tz", "hyp", "keymaster", "keystore",
	"super", "system", "system_ext", "vendor", "product", "odm", "odm_dlkm", "vendor_dlkm",
	"cache", "userdata",
}

// BuildFlashPlan 根据镜像目录生成线刷步骤。
func BuildFlashPlan(imagesDir string) ([]FlashStep, error) {
	entries, err := os.ReadDir(imagesDir)
	if err != nil {
		return nil, err
	}
	dynamic := map[string]bool{
		"system": true, "system_ext": true, "vendor": true, "product": true, "odm": true,
		"odm_dlkm": true, "vendor_dlkm": true, "super": true,
	}
	var steps []FlashStep
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		kind, part := entryKind(name)
		if kind != "image" || part == "" {
			continue
		}
		fi, _ := e.Info()
		var size int64
		if fi != nil {
			size = fi.Size()
		}
		step := FlashStep{
			Partition: part,
			Image:     filepath.Join(imagesDir, name),
			SizeBytes: size,
		}
		if dynamic[part] {
			step.NeedsFastbootd = true
			step.Note = "动态分区，需要进入 fastbootd 刷入"
		}
		// 默认不刷 userdata，避免误清空用户数据
		if part == "userdata" {
			continue
		}
		steps = append(steps, step)
	}

	rank := func(p string) int {
		for i, v := range flashOrder {
			if v == p {
				return i
			}
		}
		return len(flashOrder)
	}
	sort.SliceStable(steps, func(i, j int) bool { return rank(steps[i].Partition) < rank(steps[j].Partition) })
	if len(steps) == 0 {
		return nil, fmt.Errorf("目录中没有可刷入的镜像")
	}
	return steps, nil
}
