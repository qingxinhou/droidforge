package device

import (
	"context"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"droidforge/internal/adb"
)

// PartitionKind 区分分区的来源，前端可据此过滤。
type PartitionKind string

const (
	// KindPhysical 是 /dev/block/by-name 下的物理分区。
	KindPhysical PartitionKind = "physical"
	// KindLogical 是 /dev/block/mapper 下由 super 映射出的逻辑分区。
	KindLogical PartitionKind = "logical"
	// KindUnknown 用于无法归类但希望展示的条目（目前不产出）。
	KindUnknown PartitionKind = "unknown"
)

// Partition 描述一个分区。
type Partition struct {
	Name       string        `json:"name"`
	Block      string        `json:"block"`
	SizeBytes  int64         `json:"sizeBytes"` // -1 表示当前权限下无法获取，0 表示真实为 0
	Slot       string        `json:"slot"`
	Dynamic    bool          `json:"dynamic"`
	MountPoint string        `json:"mountPoint"`
	FSType     string        `json:"fsType"`
	Kind       PartitionKind `json:"kind"`
}

// ListPartitions 汇总分区信息。
//
// 真机上这三类信息用的是**三套不同的设备名**，必须统一后才能互相关联：
//
//	by-name  →  /dev/block/sdc43        （物理分区，指向存储节点）
//	mapper   →  system_a -> dm-3        （逻辑分区，指向 device-mapper）
//	mount    →  /dev/block/dm-9 on /     （实际挂载的是 *-verity 层，不是 dm-3）
//
// 因此这里先建立 dm 编号到分区名的映射，再用 mount / df 回填。
// 容量在 Android 10 以上对 shell 全部拒绝（/proc/partitions、/sys/class/block、
// blockdev 都是 Permission denied），拿不到时如实返回 -1，不伪装成 0。
func ListPartitions(ctx context.Context, a *adb.Client, serial string) ([]Partition, error) {
	parts := make(map[string]*Partition)
	// dm 编号 → 分区名，用于 mount / df 反查
	dmToName := make(map[string]string)

	slotSuffix := "_a"
	if out, err := a.Shell(ctx, serial, "getprop ro.boot.slot_suffix"); err == nil {
		if v := strings.TrimSpace(out); v == "_a" || v == "_b" {
			slotSuffix = v
		}
	}

	// 1. by-name：物理分区
	linkRe := regexp.MustCompile(`(\S+)\s+->\s+(\S+)`)
	if out, err := a.Shell(ctx, serial, "ls -l /dev/block/by-name"); err == nil {
		for _, line := range strings.Split(out, "\n") {
			if !strings.HasPrefix(strings.TrimSpace(line), "l") {
				continue
			}
			m := linkRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			name, target := m[1], m[2]
			// 少数分区是链到 by-name 内部的二级软链（如 bootloader1 -> lk1），
			// 这里只登记名字，Block 保留原始目标，避免编造不存在的节点。
			p := &Partition{Name: name, Block: target, Slot: slotOf(name), Kind: KindPhysical, SizeBytes: -1}
			parts[name] = p
		}
	}

	// 2. mapper：逻辑分区与 verity 层
	if out, err := a.Shell(ctx, serial, "ls -l /dev/block/mapper"); err == nil {
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			// 目录行（by-uuid）不是分区，早先会被当成一条动态分区混进列表
			if !strings.HasPrefix(line, "l") {
				continue
			}
			m := linkRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			name, target := m[1], m[2]
			// APEX 也挂在 device-mapper 下（com.android.*），不属于分区表
			if strings.Contains(name, ".") {
				continue
			}
			if strings.HasSuffix(name, "-verity") {
				owner := strings.TrimSuffix(name, "-verity")
				// verity 层挂在激活槽上，数据层 dm 与它同属一个分区
				dmToName[target] = owner + slotSuffix
				continue
			}
			if p, ok := parts[name]; ok {
				// 同名物理分区（少见）以逻辑映射为准
				p.Dynamic = true
				p.Kind = KindLogical
				p.Block = target
			} else {
				parts[name] = &Partition{Name: name, Block: target, Slot: slotOf(name), Dynamic: true, Kind: KindLogical, SizeBytes: -1}
			}
			dmToName[target] = name
		}
	}

	// 3. /proc/partitions：仅 root 可读，有则补物理分区容量
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

	// 4. df：补已挂载分区的文件系统容量。
	// 注意 df 报的是文件系统可用块数，erofs 压缩后会小于分区物理大小，
	// 因此只作为「已知的容量下界」，不覆盖已经拿到的物理值。
	if out, err := a.Shell(ctx, serial, "df -k"); err == nil {
		for _, line := range strings.Split(out, "\n") {
			f := strings.Fields(strings.TrimSpace(line))
			if len(f) < 6 {
				continue
			}
			kb, err := strconv.ParseInt(f[1], 10, 64)
			if err != nil {
				continue
			}
			src := f[0]
			name := dmToName[src]
			if name == "" {
				// 有的 ROM df 直接给 mapper 路径
				if strings.HasPrefix(src, "/dev/block/mapper/") {
					name = strings.TrimPrefix(src, "/dev/block/mapper/")
				}
			}
			if p, ok := parts[name]; ok && p.SizeBytes <= 0 {
				p.SizeBytes = kb * 1024
			}
		}
	}

	// 5. mount：补挂载点与文件系统
	mountRe := regexp.MustCompile(`^(\S+) on (\S+) type (\S+)`)
	if out, err := a.Shell(ctx, serial, "mount"); err == nil {
		for _, line := range strings.Split(out, "\n") {
			m := mountRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			src, dst, fs := m[1], m[2], m[3]
			name := dmToName[src]
			if name == "" {
				for n, p := range parts {
					if p.Block == src {
						name = n
						break
					}
				}
			}
			if name == "" {
				continue
			}
			if p, hit := parts[name]; hit {
				// verity 与数据层同属一个分区，后写不覆盖先写
				if p.MountPoint == "" {
					p.MountPoint = dst
					p.FSType = fs
				}
			}
		}
	}

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

func slotOf(name string) string {
	if strings.HasSuffix(name, "_a") {
		return "a"
	}
	if strings.HasSuffix(name, "_b") {
		return "b"
	}
	return ""
}
