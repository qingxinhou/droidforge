// Package backup 把设备上的常用数据拉到本机。
//
// 走 `adb exec-out tar -cf - | base64` 而不是 adb pull 的原因：
//  1. 一个连接传完整棵目录，小文件极多时比逐个 pull 快一个量级；
//  2. 设备端先 base64 编码，管道里只跑 ASCII，才能同时躲开 adb 的两个毛病：
//     把设备端 stderr 合并进 stdout、在 Windows 上把 \n 改写成 \r\n。
//     裸二进制流会被这两点毁掉（tar 头直接错位）；
//  3. 解包时能逐文件判断「已存在且大小一致就跳过」，天然支持增量。
//
// 通讯录与短信走 content provider 查询，导出成 CSV——它们不在文件系统里，
// 直接 pull /data/data 需要 root 且容易撞上 SELinux。
package backup

import (
	"archive/tar"
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"droidforge/internal/runner"
)

// Category 是一个备份类别。
type Category struct {
	ID    string   `json:"id"`
	Label string   `json:"label"`
	Desc  string   `json:"desc"`
	Kind  string   `json:"kind"` // files / contacts / sms
	Paths []string `json:"paths"`
}

var categories = []Category{
	{
		ID: "photos", Label: "照片与视频", Kind: "files",
		Desc:  "相机、截图、录屏、下载的视频",
		Paths: []string{"/sdcard/DCIM", "/sdcard/Pictures", "/sdcard/Movies"},
	},
	{
		ID: "documents", Label: "下载与文档", Kind: "files",
		Desc:  "浏览器下载、WPS/Office 文档、电子书",
		Paths: []string{"/sdcard/Download", "/sdcard/Documents"},
	},
	{
		ID: "audio", Label: "音乐与录音", Kind: "files",
		Desc:  "本地音乐、通话录音、语音备忘录",
		Paths: []string{"/sdcard/Music", "/sdcard/Recordings", "/sdcard/Podcasts"},
	},
	{
		ID: "pictures", Label: "其它图片", Kind: "files",
		Desc:  "第三方 App 保存到公共目录的图片",
		Paths: []string{"/sdcard/Android/media", "/sdcard/Pictures/WeiXin"},
	},
	{
		ID: "contacts", Label: "通讯录", Kind: "contacts",
		Desc: "导出为 CSV，含姓名、号码、邮箱与类型",
	},
	{
		ID: "sms", Label: "短信", Kind: "sms",
		Desc: "导出为 CSV，含号码、时间、收发方向与正文",
	},
}

// Categories 返回可备份类别。
func Categories() []Category { return categories }

// CategoryPlan 是单个类别的扫描结果。
type CategoryPlan struct {
	Category
	Size   int64  `json:"size"`
	Files  int    `json:"files"`
	Exists bool   `json:"exists"`
	Note   string `json:"note"`
}

// Plan 是整体扫描结果。
type Plan struct {
	Serial     string         `json:"serial"`
	Categories []CategoryPlan `json:"categories"`
	TotalSize  int64          `json:"totalSize"`
	TotalFiles int            `json:"totalFiles"`
}

const markerContacts = "@@@CONTACTS"
const markerSMS = "@@@SMS"
const markerEnd = "@@@END"

// scanScript 用一次 adb 往返拿到所有类别的大小与数量。
//
// du -sk 输出「KB<TAB>路径」，不存在的路径不会出现在 stdout，靠这一点判断可用性。
func scanScript() string {
	var sb strings.Builder
	for _, c := range categories {
		if c.Kind != "files" {
			continue
		}
		sb.WriteString("du -sk")
		for _, p := range c.Paths {
			sb.WriteString(" '" + p + "'")
		}
		sb.WriteString(" 2>/dev/null\n")
	}
	sb.WriteString("echo " + markerContacts + "\n")
	sb.WriteString("content query --uri content://com.android.contacts/raw_contacts --projection _id 2>/dev/null | wc -l\n")
	sb.WriteString("echo " + markerSMS + "\n")
	sb.WriteString("content query --uri content://sms --projection _id 2>/dev/null | wc -l\n")
	sb.WriteString("echo " + markerEnd + "\n")
	return sb.String()
}

// Scan 统计各类别占用。
func Scan(ctx context.Context, bin, serial string) (*Plan, error) {
	out, err := shell(ctx, bin, serial, scanScript())
	if err != nil {
		return nil, err
	}

	sizes := map[string]int64{}
	contacts, sms := 0, 0
	section := "du"
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		switch line {
		case markerContacts:
			section = "contacts"
			continue
		case markerSMS:
			section = "sms"
			continue
		case markerEnd:
			section = "end"
			continue
		}
		if line == "" {
			continue
		}
		switch section {
		case "du":
			parts := strings.SplitN(line, "\t", 2)
			if len(parts) != 2 {
				continue
			}
			kb, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
			if err != nil {
				continue
			}
			sizes[strings.TrimSpace(parts[1])] = kb * 1024
		case "contacts":
			contacts = atoiSafe(line)
		case "sms":
			sms = atoiSafe(line)
		}
	}

	plan := &Plan{Serial: serial}
	for _, c := range categories {
		cp := CategoryPlan{Category: c}
		switch c.Kind {
		case "files":
			for _, p := range c.Paths {
				if sz, ok := sizes[p]; ok {
					cp.Size += sz
					cp.Exists = true
				}
			}
			if cp.Exists {
				cp.Note = fmt.Sprintf("%d 个目录中至少一个可读", len(c.Paths))
			} else {
				cp.Note = "设备上没有这些目录，或当前系统版本限制了访问"
			}
		case "contacts":
			cp.Files = contacts
			cp.Exists = contacts > 0
			if !cp.Exists {
				cp.Note = "查询不到通讯录，可能被系统限制（部分 ROM 会拦 adb shell 的 provider 访问）"
			}
		case "sms":
			cp.Files = sms
			cp.Exists = sms > 0
			if !cp.Exists {
				cp.Note = "查询不到短信，可能是纯数据机型或系统限制了访问"
			}
		}
		plan.Categories = append(plan.Categories, cp)
		plan.TotalSize += cp.Size
		plan.TotalFiles += cp.Files
	}
	return plan, nil
}

// Options 是备份参数。
type Options struct {
	OutDir      string   `json:"outDir"`
	CategoryIDs []string `json:"categoryIds"`
	// SkipExisting 为 true 时，本地已存在且大小一致的文件直接跳过。
	SkipExisting bool `json:"skipExisting"`
}

// CategoryResult 是单个类别的实际结果。
type CategoryResult struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Files   int    `json:"files"`
	Bytes   int64  `json:"bytes"`
	Skipped int    `json:"skipped"`
	Error   string `json:"error"`
}

// Result 是整体结果。
type Result struct {
	OutDir     string           `json:"outDir"`
	Files      int              `json:"files"`
	Bytes      int64            `json:"bytes"`
	Skipped    int              `json:"skipped"`
	Categories []CategoryResult `json:"categories"`
	Manifest   string           `json:"manifest"`
}

// manifestEntry 写入 manifest.json，方便事后核对。
type manifestEntry struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	ModTime string `json:"modTime"`
}

// Run 执行备份。
func Run(ctx context.Context, bin, serial string, plan *Plan, opt Options, onProgress func(label string, percent int), onLine func(string)) (*Result, error) {
	if opt.OutDir == "" {
		return nil, fmt.Errorf("未指定备份目录")
	}
	want := map[string]bool{}
	for _, id := range opt.CategoryIDs {
		want[id] = true
	}
	if len(want) == 0 {
		for _, c := range categories {
			want[c.ID] = true
		}
	}

	res := &Result{OutDir: opt.OutDir}
	manifest := make([]manifestEntry, 0, 256)
	byID := map[string]CategoryPlan{}
	for _, c := range plan.Categories {
		byID[c.ID] = c
	}

	var doneBytes, totalBytes int64
	for _, c := range plan.Categories {
		totalBytes += c.Size
	}
	if totalBytes <= 0 {
		totalBytes = 1
	}

	sel := make([]CategoryPlan, 0, len(want))
	for _, c := range plan.Categories {
		if want[c.ID] {
			sel = append(sel, c)
		}
	}

	for _, c := range sel {
		cr := CategoryResult{ID: c.ID, Label: c.Label}
		if !c.Exists && c.Kind == "files" {
			cr.Error = "目录不可读，跳过"
			res.Categories = append(res.Categories, cr)
			continue
		}

		dst := filepath.Join(opt.OutDir, c.ID)
		switch c.Kind {
		case "files":
			n, skipped, bytes, entries, err := pullTree(ctx, bin, serial, c, c.Paths, dst, opt.SkipExisting, func(delta int64) {
				doneBytes += delta
				if onProgress != nil {
					onProgress(c.Label, int(doneBytes*100/totalBytes))
				}
			}, onLine)
			cr.Files, cr.Skipped, cr.Bytes = n, skipped, bytes
			manifest = append(manifest, entries...)
			if err != nil {
				cr.Error = err.Error()
			}
		case "contacts":
			p, err := exportContacts(ctx, bin, serial, dst)
			if err != nil {
				cr.Error = err.Error()
			} else if p != "" {
				cr.Files = 1
				if fi, e := os.Stat(p); e == nil {
					cr.Bytes = fi.Size()
				}
				manifest = append(manifest, manifestEntry{Path: p, Size: cr.Bytes, ModTime: time.Now().Format(time.RFC3339)})
			}
		case "sms":
			p, err := exportSMS(ctx, bin, serial, dst)
			if err != nil {
				cr.Error = err.Error()
			} else if p != "" {
				cr.Files = 1
				if fi, e := os.Stat(p); e == nil {
					cr.Bytes = fi.Size()
				}
				manifest = append(manifest, manifestEntry{Path: p, Size: cr.Bytes, ModTime: time.Now().Format(time.RFC3339)})
			}
		}
		if cr.Error != "" && onLine != nil {
			onLine(fmt.Sprintf("%s：%s", c.Label, cr.Error))
		}
		res.Files += cr.Files
		res.Bytes += cr.Bytes
		res.Skipped += cr.Skipped
		res.Categories = append(res.Categories, cr)
	}

	// manifest 放在备份根目录，方便整目录搬走后仍能核对。
	mp := filepath.Join(opt.OutDir, "manifest.json")
	doc := map[string]any{
		"createdAt":  time.Now().Format(time.RFC3339),
		"serial":     serial,
		"categories": res.Categories,
		"files":      manifest,
	}
	if data, err := json.MarshalIndent(doc, "", "  "); err == nil {
		if err := os.MkdirAll(opt.OutDir, 0o755); err == nil {
			if err := os.WriteFile(mp, data, 0o644); err == nil {
				res.Manifest = mp
			}
		}
	}

	if onProgress != nil {
		onProgress("备份完成", 100)
	}
	return res, nil
}

// pullTree 用 tar 流把一个类别的所有目录拉到 dst。
//
// 裸 tar 流经 adb 传到 Windows 上必坏，两个原因叠在一起：
//  1. adb 会把设备端 stderr 合并进 stdout，tar 对绝对路径打的
//     "removing leading '/' from member names" 警告会插进数据流里；
//  2. Windows 版 adb 会把 \n 改写成 \r\n，tar 的 512 字节头随之错位。
//
// 因此改为在设备端补上 2>/dev/null 并用 base64 编码，让管道里只跑纯 ASCII。
// Go 的 base64 解码器会忽略 \r 与 \n，不需要额外剥离。
func pullTree(ctx context.Context, bin, serial string, c CategoryPlan, paths []string, dst string, skipExisting bool, onBytes func(int64), onLine func(string)) (files, skipped int, bytes int64, entries []manifestEntry, err error) {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return 0, 0, 0, nil, err
	}

	args := []string{}
	if serial != "" {
		args = append(args, "-s", serial)
	}
	args = append(args, "exec-out", "tar -cf - "+shellJoin(paths)+" 2>/dev/null | base64")

	stdout, wait, err := runner.Stream(ctx, "", bin, args...)
	if err != nil {
		return 0, 0, 0, nil, err
	}
	// 解包中途失败会提前 return，这里必须关掉管道顺带杀掉 adb，
	// 否则进程堵在写 stdout 上不退出，设备连接一直被占着。
	defer stdout.Close()

	files, skipped, bytes, entries, err = extractTar(base64.NewDecoder(base64.StdEncoding, stdout), dst, skipExisting, onBytes, onLine)
	if werr := wait(); werr != nil && files == 0 {
		return files, skipped, bytes, entries, fmt.Errorf("设备侧打包失败（%v），请确认设备支持 tar 与 base64", werr)
	}
	return files, skipped, bytes, entries, err
}

// extractTar 把 tar 流解到 dst，只写普通文件。
func extractTar(r io.Reader, dst string, skipExisting bool, onBytes func(int64), onLine func(string)) (files, skipped int, bytes int64, entries []manifestEntry, err error) {
	tr := tar.NewReader(r)
	for {
		hdr, terr := tr.Next()
		if terr == io.EOF {
			break
		}
		if terr != nil {
			break
		}
		if hdr.Typeflag != tar.TypeReg || hdr.Size < 0 {
			continue
		}
		rel, ok := safeRel(hdr.Name)
		if !ok || rel == "" {
			continue
		}
		target := filepath.Join(dst, filepath.FromSlash(rel))
		if !strings.HasPrefix(target, dst+string(os.PathSeparator)) && target != dst {
			continue
		}

		if skipExisting {
			if fi, serr := os.Stat(target); serr == nil && fi.Size() == hdr.Size {
				skipped++
				onBytes(hdr.Size)
				continue
			}
		}

		if merr := os.MkdirAll(filepath.Dir(target), 0o755); merr != nil {
			continue
		}
		f, ferr := os.Create(target)
		if ferr != nil {
			if onLine != nil {
				onLine("跳过（无法写入）：" + rel)
			}
			continue
		}
		n, cerr := io.Copy(f, tr)
		f.Close()
		if cerr != nil {
			return files, skipped, bytes, entries, cerr
		}
		files++
		bytes += n
		onBytes(n)
		entries = append(entries, manifestEntry{
			Path:    rel,
			Size:    n,
			ModTime: hdr.ModTime.Format(time.RFC3339),
		})
	}
	return files, skipped, bytes, entries, nil
}

// shellJoin 把若干路径拼成安全的 shell 参数串。
func shellJoin(paths []string) string {
	parts := make([]string, 0, len(paths))
	for _, p := range paths {
		parts = append(parts, "'"+strings.ReplaceAll(p, "'", "'\\''")+"'")
	}
	return strings.Join(parts, " ")
}

// safeRel 把 tar 里的路径变成安全的相对路径。
//
// 设备上的路径通常是 /sdcard/DCIM/...，tar 会去掉前导斜杠，
// 解出来是 sdcard/DCIM/...。这里剥掉 sdcard 前缀，让结果更像人整理过的备份。
func safeRel(name string) (string, bool) {
	p := strings.TrimPrefix(strings.ReplaceAll(name, "\\", "/"), "/")
	if p == "" {
		return "", false
	}
	// 拒掉盘符与任何上跳段，避免写到备份目录之外。
	if len(p) >= 2 && p[1] == ':' {
		return "", false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return "", false
		}
	}
	p = path.Clean(p)
	if strings.HasPrefix(p, "sdcard/") {
		p = strings.TrimPrefix(p, "sdcard/")
	}
	return p, p != "." && p != ""
}

func shell(ctx context.Context, bin, serial, script string) (string, error) {
	args := []string{}
	if serial != "" {
		args = append(args, "-s", serial)
	}
	args = append(args, "shell", script)
	res, err := runner.Run(ctx, "", bin, args...)
	if err != nil {
		return "", err
	}
	return res.Stdout, nil
}

func atoiSafe(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}

// SortBySize 按占用从大到小排序，扫描结果展示用。
func (p *Plan) SortBySize() {
	sort.SliceStable(p.Categories, func(i, j int) bool {
		return p.Categories[i].Size > p.Categories[j].Size
	})
}

// TotalOf 返回选中类别的预估大小。
func (p *Plan) TotalOf(ids []string) int64 {
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	var sum int64
	for _, c := range p.Categories {
		if len(want) == 0 || want[c.ID] {
			sum += c.Size
		}
	}
	return sum
}

// csvWriter 带 UTF-8 BOM，Excel 才会按 UTF-8 解析。
func csvWriter(f *os.File) *bufio.Writer {
	w := bufio.NewWriter(f)
	_, _ = w.WriteString("\xEF\xBB\xBF")
	return w
}
