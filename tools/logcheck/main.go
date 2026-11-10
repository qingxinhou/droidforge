// logcheck 用真机验证「读取日志里发现的问题」修复是否生效。
//
// 只做只读探测与失败路径验证，不写任何设备设置、不启动会打扰人的界面。
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"droidforge/internal/adb"
	"droidforge/internal/backup"
	"droidforge/internal/tweaks"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("用法: logcheck <adb 路径>")
		os.Exit(2)
	}
	bin := os.Args[1]
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	c := adb.New(bin)
	devs, err := c.Devices(ctx)
	if err != nil || len(devs) == 0 {
		fmt.Println("没有设备:", err)
		os.Exit(1)
	}
	d := devs[0]
	fmt.Printf("设备 %s (%s)\n", d.Serial, d.State)

	exec := func(ctx context.Context, command string) (string, error) {
		return c.Shell(ctx, d.Serial, command)
	}

	fmt.Println("\n=== 1. 设置写入权限探测 ===")
	caps := tweaks.ProbeCaps(ctx, exec)
	fmt.Printf("system=%v secure=%v global=%v\n", caps.SystemWritable, caps.SecureWritable, caps.GlobalWritable)
	fmt.Printf("说明: %q\n", caps.Note)

	fmt.Println("\n=== 2. 真机写入失败的报错折叠 ===")
	out, _ := exec(ctx, "settings put secure clock_seconds 1")
	msg := tweaks.FailureMessage(out)
	fmt.Printf("原始输出 %d 字节 / %d 行\n", len(out), countLines(out))
	fmt.Printf("折叠后: %s\n", msg)
	fmt.Printf("单行=%v 含换行=%v 长度=%d\n", !containsNewline(msg), containsNewline(msg), len([]rune(msg)))

	fmt.Println("\n=== 3. 启动组件的解析（只解析，不点开应用）===")
	for _, pkg := range []string{"com.android.settings", "com.mi.health", "cn.wps.moffice_eng.xiaomi.lite"} {
		raw, _ := exec(ctx, "cmd package resolve-activity --brief -a android.intent.action.MAIN -c android.intent.category.LAUNCHER "+pkg)
		comp, ok := adb.ParseResolveActivity(raw)
		fmt.Printf("%-38s → %v %s\n", pkg, ok, comp)
	}

	fmt.Println("\n=== 4. 插件型应用的启动报错（不产生副作用）===")
	err = c.LaunchApp(ctx, d.Serial, "cn.wps.moffice_eng.xiaomi.lite")
	fmt.Printf("err=%v\n", err)

	fmt.Println("\n=== 5. 不存在的包 ===")
	err = c.LaunchApp(ctx, d.Serial, "com.definitely.not.installed")
	fmt.Printf("err=%v\n", err)

	fmt.Println("\n=== 6. 备份扫描（文件数是否真实）===")
	plan, perr := backup.Scan(ctx, bin, d.Serial)
	if perr != nil {
		fmt.Println("扫描失败:", perr)
		return
	}
	cats := 0
	for _, cp := range plan.Categories {
		if cp.Exists {
			cats++
		}
		fmt.Printf("%-10s exists=%-5v size=%-9d files=%-4d %s\n",
			cp.ID, cp.Exists, cp.Size, cp.Files, cp.Note)
	}
	fmt.Printf("汇总: %d 项可备份, %d 个文件, %.1f MB\n", cats, plan.TotalFiles, float64(plan.TotalSize)/(1<<20))
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	n := 1
	for _, r := range s {
		if r == '\n' {
			n++
		}
	}
	return n
}

func containsNewline(s string) bool {
	for _, r := range s {
		if r == '\n' || r == '\r' {
			return true
		}
	}
	return false
}
