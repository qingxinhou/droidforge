// Command fbcheck 在真机 fastboot / bootloader 模式下自检 fastboot 包。
//
// 用法：go run ./tools/fbcheck <fastboot.exe 路径>
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"droidforge/internal/fastboot"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("用法: fbcheck <fastboot 路径>")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	c := fastboot.New(os.Args[1])

	devs, err := c.Devices(ctx)
	if err != nil {
		fmt.Println("Devices 失败:", err)
		os.Exit(1)
	}
	fmt.Printf("=== fastboot devices ===\n%v（共 %d 台）\n", devs, len(devs))
	if len(devs) == 0 {
		fmt.Println("没有设备处于 fastboot 模式")
		os.Exit(1)
	}
	serial := devs[0]

	fmt.Println("\n=== 单项 GetVar ===")
	for _, name := range []string{
		"product", "current-slot", "is-userspace", "max-download-size",
		"partition-size:boot_a", "partition-size:super", "version-bootloader",
	} {
		v, err := c.GetVar(ctx, serial, name)
		fmt.Printf("%-24s -> %-34q err=%v\n", name, v, err)
	}

	fmt.Println("\n=== 不存在的变量（真机表现：键回显、值为空、退出码 0）===")
	v, err := c.GetVar(ctx, serial, "partition-size:not_a_real_partition")
	short := v
	if len(short) > 50 {
		short = short[:50] + "..."
	}
	fmt.Printf("值=%q 长度=%d\nerr=%v（空值且无 err 即为真机实际语义）\n", short, len(v), err)

	fmt.Println("\n=== Partitions() 正式代码路径 ===")
	parts, err := c.Partitions(ctx, serial)
	if err != nil {
		fmt.Println("失败:", err)
	} else {
		fmt.Printf("共 %d 个分区：\n", len(parts))
		for _, p := range parts {
			fmt.Printf("  %-16s base=%-10s slot=%-2s %s\n",
				p.Name, p.Base, p.Slot, humanSize(p.SizeBytes))
		}
	}

	fmt.Println("\n=== 模式判断 ===")
	us, _ := c.GetVar(ctx, serial, "is-userspace")
	if strings.EqualFold(strings.TrimSpace(us), "yes") {
		fmt.Println("当前是 fastbootd（用户态），逻辑分区可见")
	} else {
		fmt.Println("当前是 bootloader（内核态），逻辑分区不可见")
	}
}

func humanSize(n int64) string {
	if n <= 0 {
		return "—"
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	f := float64(n)
	i := 0
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	return fmt.Sprintf("%.2f %s", f, units[i])
}
