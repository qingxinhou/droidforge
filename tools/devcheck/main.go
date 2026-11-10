// devcheck 用真机数据跑一遍 device 包的采集与分区解析，输出诊断结论。
// 仅用于开发期验证，不参与打包。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"droidforge/internal/adb"
	"droidforge/internal/backup"
	"droidforge/internal/cpu"
	"droidforge/internal/device"
	"droidforge/internal/power"
)

func main() {
	bin := os.Args[1]
	a := adb.New(bin)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	devs, err := a.Devices(ctx)
	if err != nil {
		fmt.Println("Devices 失败:", err)
		return
	}
	if len(devs) == 0 {
		fmt.Println("没有设备")
		return
	}
	d := devs[0]
	fmt.Printf("设备: %s  state=%s  product=%s\n\n", d.Serial, d.State, d.Product)

	info, err := device.Collect(ctx, a, d.Serial)
	if err != nil {
		fmt.Println("Collect 失败:", err)
		return
	}
	b, _ := json.MarshalIndent(info, "", "  ")
	fmt.Println("=== DeviceInfo ===")
	fmt.Println(string(b))

	parts, err := device.ListPartitions(ctx, a, d.Serial)
	if err != nil {
		fmt.Println("ListPartitions 失败:", err)
		return
	}

	var noSize, noMount, noFS, dynamic, fake, known int
	for _, p := range parts {
		if p.SizeBytes > 0 {
			known++
		}
		if p.SizeBytes == -1 {
			noSize++
		}
		if p.MountPoint == "" {
			noMount++
		}
		if p.FSType == "" {
			noFS++
		}
		if p.Dynamic {
			dynamic++
		}
		if p.Name == "by-uuid" || p.Name == "by-name" {
			fake++
		}
	}
	fmt.Printf("\n=== ListPartitions 统计 ===\n")
	fmt.Printf("总数 %d | 容量已知 %d | 容量未知 %d | 无挂载点 %d | 无文件系统 %d | 标记动态 %d | 伪分区 %d\n",
		len(parts), known, noSize, noMount, noFS, dynamic, fake)

	fmt.Println("\n=== 逻辑分区（全部）===")
	for _, p := range parts {
		if p.Kind != "logical" {
			continue
		}
		fmt.Printf("%-22s block=%-22s size=%-12d mount=%-24s fs=%s\n",
			p.Name, p.Block, p.SizeBytes, p.MountPoint, p.FSType)
	}

	fmt.Println("\n=== 物理分区样本（前 6）===")
	shown := 0
	for _, p := range parts {
		if p.Kind != "physical" || shown >= 6 {
			continue
		}
		shown++
		fmt.Printf("%-22s block=%-22s size=%-12d slot=%s\n", p.Name, p.Block, p.SizeBytes, p.Slot)
	}

	// 直接对照 mount 的真实设备节点，看能否与分区对上
	out, _ := a.Shell(ctx, d.Serial, "mount")
	dmCount, matched := 0, 0
	blockSet := map[string]bool{}
	for _, p := range parts {
		blockSet[p.Block] = true
	}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 3 || !strings.HasPrefix(f[0], "/dev/block/dm-") {
			continue
		}
		dmCount++
		if blockSet[f[0]] {
			matched++
		}
	}
	fmt.Printf("\n=== 挂载节点匹配 ===\nmount 中 dm-* 设备 %d 个，能被 partition.Block 命中 %d 个\n", dmCount, matched)

	// 动态分区名是否与 dm 节点有映射
	mapOut, _ := a.Shell(ctx, d.Serial, "ls -l /dev/block/mapper")
	fmt.Printf("mapper 的 ls -l 行数=%d（若为 0 说明拿不到 symlink 目标）\n", len(strings.Split(strings.TrimSpace(mapOut), "\n")))

	exec := func(c context.Context, cmd string) (string, error) { return a.Shell(c, d.Serial, cmd) }
	snap, err := cpu.Read(ctx, exec)
	if err != nil {
		fmt.Println("\nCPU 读取失败:", err)
	} else {
		b2, _ := json.Marshal(snap)
		fmt.Printf("\n=== CPU ===\n%s\n", string(b2))
	}

	bat, err := power.Read(ctx, exec)
	if err != nil {
		fmt.Println("电池读取失败:", err)
	} else {
		b3, _ := json.Marshal(bat)
		fmt.Printf("\n=== 电池 ===\n%s\n", string(b3))
	}

	apps, err := a.ListPackagesDetail(ctx, d.Serial, false)
	if err != nil {
		fmt.Println("应用列表失败:", err)
	} else {
		fmt.Printf("\n=== 应用 ===\n第三方 %d 个，前 3 条：\n", len(apps))
		for i, ap := range apps {
			if i >= 3 {
				break
			}
			fmt.Printf("  %-44s %s\n", ap.Package, ap.APKPath)
		}
	}

	plan, err := backup.Scan(ctx, bin, d.Serial)
	if err != nil {
		fmt.Println("\n备份扫描失败:", err)
	} else {
		b4, _ := json.Marshal(plan)
		fmt.Printf("\n=== 备份扫描 ===\n%s\n", string(b4))

		outDir := filepath.Join(os.TempDir(), "df-devcheck-backup")
		_ = os.RemoveAll(outDir)
		res, err := backup.Run(ctx, bin, d.Serial, plan,
			backup.Options{OutDir: outDir, CategoryIDs: []string{"photos", "documents"}},
			func(label string, pct int) {}, func(string) {})
		if err != nil {
			fmt.Println("备份执行失败:", err)
		} else {
			b5, _ := json.Marshal(res)
			fmt.Printf("\n=== 备份结果 ===\n%s\n", string(b5))
			_ = filepath.Walk(outDir, func(p string, fi os.FileInfo, e error) error {
				if e == nil && fi != nil && !fi.IsDir() {
					fmt.Printf("  文件 %d B  %s\n", fi.Size(), strings.TrimPrefix(p, outDir))
				}
				return nil
			})
		}
	}
}
