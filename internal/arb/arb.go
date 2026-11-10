// Package arb 做防回滚（Anti-Rollback）相关信息分析。
//
// 需要说清的边界：ARB 版本号没有统一的跨厂商存储格式，高通把它编进
// xbl/abl 的某个结构里，联发科放在 preloader 的段中，各机型位置还不同。
// 所以这里采用**启发式扫描**——在引导镜像里找 anti-rollback 相关标记，
// 再读标记附近的整数。结果只能作为参考，不能当成唯一依据。
//
// 真正确定的部分交给包 avb：vbmeta 的 rollback_index 是标准字段，
// 两个镜像之间可以精确对比。
package arb

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// 常见标记。大小写与下划线变体都收，厂商各写各的。
var markers = []string{
	"anti-rollback",
	"anti_rollback",
	"AntiRollback",
	"ANTI_ROLLBACK",
	"anti rollback",
	"rollback_index",
	"rollback-index",
	"RollbackIndex",
	"min_rollback",
	"ARB_VERSION",
	"arb_version",
}

// utf16 变体在 PE/EFI 里很常见。
var markersUTF16 = func() [][]byte {
	out := make([][]byte, 0, len(markers))
	for _, m := range markers {
		b := make([]byte, 0, len(m)*2)
		for _, r := range m {
			b = append(b, byte(r), 0)
		}
		out = append(out, b)
	}
	return out
}()

// Hit 是一次标记命中。
type Hit struct {
	Offset      int64  `json:"offset"`
	Marker      string `json:"marker"`
	Encoding    string `json:"encoding"` // ascii / utf16le
	Value       uint32 `json:"value"`
	ValueOffset int64  `json:"valueOffset"`
	// Delta 是数值相对标记起点的字节距离，用来判断可信度（紧邻的更可信）。
	Delta int `json:"delta"`
}

// FileReport 是单个镜像的分析结果。
type FileReport struct {
	Path      string   `json:"path"`
	Size      int64    `json:"size"`
	Scanned   int64    `json:"scanned"`
	Format    string   `json:"format"` // elf64-arm64 / elf32-arm / pe / raw
	Hits      []Hit    `json:"hits"`
	Values    []uint32 `json:"values"`
	Confident string   `json:"confidence"` // high / medium / low / none
	Note      string   `json:"note"`
}

// markerWindow 是标记之后要找数值的范围。
const markerWindow = 512

// maxScan 限制单文件扫描量。引导镜像通常几 MB，超过这个量级的多半是整包，
// 扫全盘既慢又容易误报。
const maxScan = 128 << 20

// ScanFile 扫描一个镜像。
func ScanFile(path string) (*FileReport, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	rep := &FileReport{Path: path, Size: st.Size()}

	head := make([]byte, 4096)
	n, _ := io.ReadFull(f, head)
	rep.Format = detectFormat(head[:n])
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	limit := st.Size()
	if limit > maxScan {
		limit = maxScan
		rep.Note = fmt.Sprintf("文件 %.1f MB，只扫描前 %d MB，尾部可能有遗漏", float64(st.Size())/(1<<20), maxScan>>20)
	}

	// 分块读，块间留出 markerWindow+4 的重叠，避免标记被块边界切断。
	const chunk = 1 << 20
	overlap := markerWindow + 64
	buf := make([]byte, chunk+overlap)
	var base int64
	for base < limit {
		want := int64(chunk)
		if base+want > limit {
			want = limit - base
		}
		n, err := f.ReadAt(buf[:want+int64(overlap)], base)
		if n <= 0 {
			break
		}
		scanChunk(rep, buf[:n], base)
		rep.Scanned += int64(n)
		if base+want >= limit {
			break
		}
		base += want
		if err != nil && err != io.EOF {
			return nil, err
		}
	}

	finalize(rep)
	return rep, nil
}

func scanChunk(rep *FileReport, b []byte, base int64) {
	for _, m := range markers {
		needle := []byte(m)
		for from := 0; ; {
			i := bytes.Index(b[from:], needle)
			if i < 0 {
				break
			}
			pos := from + i
			record(rep, b, pos, m, "ascii", len(needle), base)
			from = pos + len(needle)
		}
	}
	for i, needle := range markersUTF16 {
		name := markers[i]
		for from := 0; ; {
			j := bytes.Index(b[from:], needle)
			if j < 0 {
				break
			}
			pos := from + j
			record(rep, b, pos, name, "utf16le", len(needle), base)
			from = pos + len(needle)
		}
	}
}

// record 在标记附近找第一个像 ARB 版本号的 4 字节小端整数。
//
// needleLen 必须是**实际匹配到的字节数**——UTF-16 标记比它的字符数长一倍，
// 用字符数算起点会整体错位，读出来的是半截数据。
func record(rep *FileReport, b []byte, pos int, marker, enc string, needleLen int, base int64) {
	start := pos + needleLen
	for _, align := range []int{0, 4, 8} {
		for d := align; d < markerWindow; d += 4 {
			p := start + d
			if p+4 > len(b) {
				return
			}
			v := binary.LittleEndian.Uint32(b[p:])
			// ARB 版本号是个很小的正整数；0 通常是占位。
			if v == 0 || v > 0xffff {
				continue
			}
			rep.Hits = append(rep.Hits, Hit{
				Offset:      base + int64(pos),
				Marker:      marker,
				Encoding:    enc,
				Value:       v,
				ValueOffset: base + int64(p),
				Delta:       d,
			})
			return
		}
	}
}

func detectFormat(b []byte) string {
	if len(b) >= 20 && bytes.Equal(b[:4], []byte{0x7f, 'E', 'L', 'F'}) {
		switch b[4] {
		case 2:
			return "elf64"
		case 1:
			return "elf32"
		}
		return "elf"
	}
	if len(b) >= 2 && b[0] == 'M' && b[1] == 'Z' {
		return "pe"
	}
	if bytes.HasPrefix(b, []byte("ANDROID!")) {
		return "android-boot"
	}
	return "raw"
}

// finalize 汇总候选值并给出置信度。
func finalize(rep *FileReport) {
	counts := map[uint32]int{}
	for _, h := range rep.Hits {
		counts[h.Value]++
	}
	vals := make([]uint32, 0, len(counts))
	for v := range counts {
		vals = append(vals, v)
	}
	sort.Slice(vals, func(i, j int) bool {
		if counts[vals[i]] != counts[vals[j]] {
			return counts[vals[i]] > counts[vals[j]]
		}
		return vals[i] < vals[j]
	})
	rep.Values = vals

	switch {
	case len(rep.Hits) == 0:
		rep.Confident = "none"
		if rep.Note == "" {
			rep.Note = "没有找到 anti-rollback 标记。可能该机型不使用字符串标记（高通常把版本编进二进制结构），也可能是被压缩过的镜像"
		}
	case len(vals) == 1 && len(rep.Hits) >= 2:
		rep.Confident = "high"
		rep.Note = fmt.Sprintf("%d 处标记指向同一个值 %d", len(rep.Hits), vals[0])
	case len(vals) == 1:
		rep.Confident = "medium"
		rep.Note = "只有一处标记，且数值唯一，建议再拿同机型的其它版本对比"
	default:
		rep.Confident = "low"
		rep.Note = fmt.Sprintf("命中 %d 处标记但得到 %d 个不同数值（%s），需要人工判断哪个是真正的 ARB 版本",
			len(rep.Hits), len(vals), joinValues(vals, 6))
	}
}

func joinValues(v []uint32, max int) string {
	strs := make([]string, 0, len(v))
	for i, x := range v {
		if i >= max {
			strs = append(strs, "…")
			break
		}
		strs = append(strs, fmt.Sprint(x))
	}
	return strings.Join(strs, ", ")
}

// Verdict 是对「能不能降级」的判断结论。
type Verdict struct {
	Level  string `json:"level"` // ok / warn / block / unknown
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

// Compare 对比当前设备侧与目标固件的 ARB 值，给出降级建议。
//
// current 取设备上已有固件的值，target 取准备刷入的固件值。拿不到时传 -1。
func Compare(current, target int64) Verdict {
	switch {
	case current < 0 || target < 0:
		return Verdict{
			Level:  "unknown",
			Title:  "缺少对比基准",
			Detail: "需要同时拿到「设备当前固件」与「目标固件」的 ARB 版本才能判断。可以先分别对两个包里的 xbl/abl 做一次扫描。",
		}
	case target > current:
		return Verdict{
			Level: "warn",
			Title: "目标固件 ARB 更高，刷入后无法回退",
			Detail: fmt.Sprintf("目标为 %d，当前为 %d。刷入后熔断计数被抬到 %d，之后任何 ARB 低于 %d 的固件都会被引导程序拒绝。",
				target, current, target, target),
		}
	case target == current:
		return Verdict{
			Level:  "ok",
			Title:  "ARB 版本相同，可以互刷",
			Detail: fmt.Sprintf("两侧都是 %d，降级不会触发熔断。", target),
		}
	default:
		return Verdict{
			Level:  "ok",
			Title:  "目标固件 ARB 更低，可正常降级",
			Detail: fmt.Sprintf("目标为 %d，当前为 %d。只要设备没有被更高的固件抬过熔断计数，这次降级是安全的。", target, current),
		}
	}
}
