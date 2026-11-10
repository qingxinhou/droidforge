// 临时联调工具：验证远程 OTA 解析（不参与发布构建）。
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"droidforge/internal/remote"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: remotecheck <ota-url> [output-dir] [partition...]")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	start := time.Now()
	info, err := remote.Inspect(ctx, os.Args[1], remote.Options{})
	if err != nil {
		fmt.Println("Inspect error:", err)
		os.Exit(1)
	}
	fmt.Printf("file=%s size=%.2fMB range=%v container=%s payloadOff=%d payloadSize=%.2fMB v%d blockSize=%d full=%v\n",
		info.FileName, float64(info.TotalSize)/1048576, info.SupportsRange, info.Container,
		info.PayloadOffset, float64(info.PayloadSize)/1048576, info.Version, info.BlockSize, info.Full)
	fmt.Printf("note=%s\npartitions=%d\n", info.Note, len(info.Partitions))
	for i, p := range info.Partitions {
		if i >= 12 {
			fmt.Printf("  ... 共 %d 个\n", len(info.Partitions))
			break
		}
		fmt.Printf("  %-16s size=%.2fMB dl=%.2fMB ops=%d inc=%v\n", p.Name,
			float64(p.Size)/1048576, float64(p.Compressed)/1048576, p.Operations, p.Incremental)
	}
	fmt.Printf("解析耗时 %s\n", time.Since(start).Round(time.Millisecond))

	if len(os.Args) < 3 {
		return
	}
	outDir := os.Args[2]
	names := os.Args[3:]
	start = time.Now()
	files, err := remote.Extract(ctx, os.Args[1], names, outDir, remote.Options{}, func(name string, pct int) {
		fmt.Printf("\r  %s %3d%%", name, pct)
	})
	fmt.Println()
	if err != nil {
		fmt.Println("Extract error:", err)
		os.Exit(1)
	}
	for _, f := range files {
		st, _ := os.Stat(f)
		fmt.Printf("  -> %s (%.2fMB)\n", f, float64(st.Size())/1048576)
	}
	fmt.Printf("提取耗时 %s\n", time.Since(start).Round(time.Second))
}
