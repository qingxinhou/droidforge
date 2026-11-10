package device

import (
	"context"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"droidforge/internal/adb"
)

// Partition 描述一个分区。
type Partition struct {
	Name       string `json:"name"`
	Block      string `json:"block"`
	SizeBytes  int64  `json:"sizeBytes"`
	Slot       string `json:"slot"`
	Dynamic    bool   `json:"dynamic"`
	MountPoint string `json:"mountPoint"`
	FSType     string `json:"fsType"`
}

// ListPartitions 通过 /dev/block/by-name、/proc/partitions、mount 汇总分区信息。
func ListPartitions(ctx context.Context, a *adb.Client, serial string) ([]Partition, error) {
	parts := make(map[string]*Partition)

	// 1. by-name 符号链接：拿到分区名与块设备路径
	linkRe := regexp.MustCompile(`(\S+)\s*->\s*(\S+)`)
	if out, err := a.Shell(ctx, serial, "ls -l /dev/block/by-name"); err == nil {
		for _, line := range strings.Split(out, "\n") {
			m := linkRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			name := strings.TrimSuffix(m[1], "_a")
			name = strings.TrimSuffix(name, "_b")
			name = strings.TrimPrefix(name, "by-name/")
			p := &Partition{Name: m[1], Block: m[2]}
			if strings.HasSuffix(m[1], "_a") {
				p.Slot = "a"
			} else if strings.HasSuffix(m[1], "_b") {
				p.Slot = "b"
			}
			parts[m[1]] = p
		}
	}

	// 2. /proc/partitions：拿块设备容量（单位 KB）
	if out, err := a.Shell(ctx, serial, "cat /proc/partitions"); err == nil {
		for _, line := range strings.Split(out, "\n") {
			f := strings.Fields(strings.TrimSpace(line))
			if len(f) != 4 {
				continue
			}
			kb, err := strconv.ParseInt(f[2], 10, 64)
			if err != nil {
				continue
			}
			devName := f[3]
			for _, p := range parts {
				if strings.HasSuffix(p.Block, "/"+devName) {
					p.SizeBytes = kb * 1024
				}
			}
		}
	}

	// 3. mount：拿挂载点与文件系统
	mountRe := regexp.MustCompile(`^(\S+) on (\S+) type (\S+)`)
	if out, err := a.Shell(ctx, serial, "mount"); err == nil {
		for _, line := range strings.Split(out, "\n") {
			m := mountRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			src, dst, fs := m[1], m[2], m[3]
			for _, p := range parts {
				if p.Block == src {
					p.MountPoint = dst
					p.FSType = fs
				}
			}
		}
	}

	// 4. 动态分区（/dev/block/mapper）
	if out, err := a.Shell(ctx, serial, "ls /dev/block/mapper"); err == nil {
		for _, name := range strings.Fields(out) {
			p, ok := parts[name]
			if !ok {
				p = &Partition{Name: name, Block: "/dev/block/mapper/" + name}
				parts[name] = p
			}
			p.Dynamic = true
			if strings.HasSuffix(name, "_a") {
				p.Slot = "a"
			} else if strings.HasSuffix(name, "_b") {
				p.Slot = "b"
			}
		}
	}

	// 5. 补充容量：du 兜底（部分设备 /proc/partitions 不含 dm）
	list := make([]Partition, 0, len(parts))
	for _, p := range parts {
		list = append(list, *p)
	}
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Name == list[j].Name {
			return list[i].Block < list[j].Block
		}
		return list[i].Name < list[j].Name
	})
	return list, nil
}
