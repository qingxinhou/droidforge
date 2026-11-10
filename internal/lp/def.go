package lp

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Definition 是散包目录里动态分区定义的解析结果。
//
// 欧加系线刷包通常带一份 super_def / dynamic_partitions_info.txt，内容是
// fastboot 打 super 时用的 key=value 列表：
//
//	super_partition_groups=qti_dynamic_partitions
//	qti_dynamic_partitions_partition_list=system vendor product
//	qti_dynamic_partitions_size=9126805504
//
// 有它就能把每个分区归到正确的组，合成的 super 与原厂一致。
type Definition struct {
	// Groups 组名 → 该组下的分区名。
	Groups map[string][]string `json:"groups"`
	// Sizes 组名 → 组容量上限（字节），0 表示未声明。
	Sizes map[string]int64 `json:"sizes"`
	// Source 是解析的文件名。
	Source string `json:"source"`
}

var defNames = []string{
	"super_def", "super_def.txt",
	"dynamic_partitions_info.txt", "super_empty.txt",
}

// LoadDefinition 在目录里找动态分区定义文件，找不到返回 nil, nil。
func LoadDefinition(dir string) (*Definition, error) {
	for _, name := range defNames {
		p := filepath.Join(dir, name)
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		def, perr := parseDefinition(f)
		f.Close()
		if perr != nil {
			return nil, perr
		}
		if def == nil {
			continue
		}
		def.Source = name
		return def, nil
	}
	return nil, nil
}

func parseDefinition(r io.Reader) (*Definition, error) {
	def := &Definition{Groups: map[string][]string{}, Sizes: map[string]int64{}}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := splitKV(line)
		if !ok {
			continue
		}
		switch {
		case key == "super_partition_groups":
			for _, g := range strings.Fields(value) {
				if _, exists := def.Groups[g]; !exists {
					def.Groups[g] = nil
				}
			}
		case strings.HasSuffix(key, "_partition_list"):
			group := strings.TrimSuffix(key, "_partition_list")
			def.Groups[group] = append(def.Groups[group], strings.Fields(value)...)
		case strings.HasSuffix(key, "_size"):
			group := strings.TrimSuffix(key, "_size")
			if group == "super" {
				continue
			}
			if n, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil {
				def.Sizes[group] = n
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(def.Groups) == 0 {
		return nil, nil
	}
	return def, nil
}

// splitKV 兼容 key=value 与 key value 两种写法。
func splitKV(line string) (string, string, bool) {
	if i := strings.IndexByte(line, '='); i > 0 {
		return strings.TrimSpace(line[:i]), strings.TrimSpace(line[i+1:]), true
	}
	fields := strings.Fields(line)
	if len(fields) >= 2 {
		return fields[0], strings.Join(fields[1:], " "), true
	}
	return "", "", false
}

// GroupOf 返回分区所属的组名，未声明时返回空串。
func (d *Definition) GroupOf(partition string) string {
	if d == nil {
		return ""
	}
	for g, list := range d.Groups {
		for _, p := range list {
			if p == partition {
				return g
			}
		}
	}
	return ""
}

// MaxSizeOf 返回组容量上限，未声明返回 0。
func (d *Definition) MaxSizeOf(group string) int64 {
	if d == nil {
		return 0
	}
	return d.Sizes[group]
}

// ApplyDefinition 用定义文件给散包补齐组名。
func ApplyDefinition(parts []BuildPartition, def *Definition) []BuildPartition {
	if def == nil {
		return parts
	}
	out := make([]BuildPartition, len(parts))
	copy(out, parts)
	for i := range out {
		// 文件名里的 _a/_b 不影响分组判断。
		base := strings.TrimSuffix(strings.TrimSuffix(out[i].Name, "_a"), "_b")
		if g := def.GroupOf(out[i].Name); g != "" {
			out[i].Group = g
		} else if g := def.GroupOf(base); g != "" {
			out[i].Group = g
		}
	}
	return out
}
