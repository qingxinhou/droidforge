// Package root 负责 root 方案识别与「一键打补丁」流程编排。
//
// 设计思路：所有补丁工具（magiskboot / ksud / apd）本质上都是 Android ARM 二进制，
// 直接放在 Windows/macOS 上跑不了。因此统一采用：
//
//	下载官方 APK -> 解出 ARM 二进制 -> push 到 /data/local/tmp -> 在手机上执行 -> pull 回补丁镜像
//
// 这样跨平台一致，也不依赖本机 gcc / WSL。
package root

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"droidforge/internal/adb"
	"droidforge/internal/runner"
)

// Solution 描述一种 root 方案。
type Solution struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Vendor      string `json:"vendor"`
	Description string `json:"description"`
	Repo        string `json:"repo"`
	AssetGlob   string `json:"assetGlob"`
	Website     string `json:"website"`
	Family      string `json:"family"` // magisk / kernelsu / apatch
}

// Solutions 返回内置支持的 root 方案。
func Solutions() []Solution {
	return []Solution{
		{
			ID: "kernelsu", Name: "KernelSU", Vendor: "tiann",
			Description: "内核级 root，基于 GKI 内核，隐蔽性最好，支持模块（Zygisk Next 需另装）",
			Repo:        "tiann/KernelSU", AssetGlob: "KernelSU_.*\\.apk", Website: "https://kernelsu.org",
			Family: "kernelsu",
		},
		{
			ID: "kernelsu-next", Name: "KernelSU Next", Vendor: "KernelSU-Next",
			Description: "KernelSU 社区分支，支持更多内核版本与非 GKI 设备",
			Repo:        "KernelSU-Next/KernelSU-Next", AssetGlob: "KernelSU_.*\\.apk", Website: "https://github.com/KernelSU-Next",
			Family: "kernelsu",
		},
		{
			ID: "magisk", Name: "Magisk 官方", Vendor: "topjohnwu",
			Description: "最主流的系统级 root，模块生态最完整，支持 Zygisk",
			Repo:        "topjohnwu/Magisk", AssetGlob: "Magisk-.*\\.apk", Website: "https://github.com/topjohnwu/Magisk",
			Family: "magisk",
		},
		{
			ID: "magisk-alpha", Name: "Magisk Alpha（阿尔法面具）", Vendor: "vvb2060",
			Description: "Magisk 增强分支，内置更强的隐藏能力，适合对抗检测",
			Repo:        "vvb2060/Magisk", AssetGlob: ".*\\.apk", Website: "https://github.com/vvb2060/Magisk",
			Family: "magisk",
		},
		{
			ID: "magisk-delta", Name: "Magisk Delta（德尔塔面具）", Vendor: "HuskyDG",
			Description: "Magisk 分支，保留 MagiskHide 与 SUList，模块兼容性好",
			Repo:        "HuskyDG/magisk-files", AssetGlob: ".*\\.apk", Website: "https://huskydg.github.io/magisk-files/",
			Family: "magisk",
		},
		{
			ID: "apatch", Name: "APatch", Vendor: "bmax121",
			Description: "基于 KernelPatch 的内核 root，免 GKI 限制，支持 KPM 模块",
			Repo:        "bmax121/APatch", AssetGlob: "APatch_.*\\.apk", Website: "https://apatch.dev",
			Family: "apatch",
		},
	}
}

// FindSolution 按 ID 查找方案。
func FindSolution(id string) (Solution, bool) {
	for _, s := range Solutions() {
		if s.ID == id {
			return s, true
		}
	}
	return Solution{}, false
}

// 设备端工作目录
const deviceWorkDir = "/data/local/tmp/droidforge"

// InstallOptions 是打补丁参数。
type InstallOptions struct {
	Serial    string
	Solution  string
	BootImage string // 本地 boot / init_boot 镜像
	WorkDir   string // 本地工作目录（下载与解包）
	APKPath   string // 可选：使用本地已下载的 APK，跳过网络
	Mirror    string // 可选：GitHub 加速前缀，如 https://ghfast.top/
}

// InstallResult 是打补丁结果。
type InstallResult struct {
	Solution     string   `json:"solution"`
	PatchedImage string   `json:"patchedImage"`
	Steps        []string `json:"steps"`
}

// Install 执行完整打补丁流程，返回本地补丁镜像路径。
func Install(ctx context.Context, a *adb.Client, opts InstallOptions, onLine runner.LineHandler) (*InstallResult, error) {
	sol, ok := FindSolution(opts.Solution)
	if !ok {
		return nil, fmt.Errorf("未知 root 方案：%s", opts.Solution)
	}
	if opts.BootImage == "" {
		return nil, fmt.Errorf("请先选择 boot / init_boot 镜像")
	}
	if opts.WorkDir == "" {
		opts.WorkDir = filepath.Join(".", "cache")
	}

	res := &InstallResult{Solution: sol.ID}
	logf := func(format string, args ...any) {
		line := fmt.Sprintf(format, args...)
		res.Steps = append(res.Steps, line)
		if onLine != nil {
			onLine(line, runner.Stdout)
		}
	}

	workDir := filepath.Join(opts.WorkDir, "root-"+sol.ID)
	if err := ensureDir(workDir); err != nil {
		return nil, err
	}

	// 1. 准备补丁工具包
	apk := opts.APKPath
	if apk == "" {
		logf("正在获取 %s 最新版本…", sol.Name)
		path, err := DownloadSolutionAPK(ctx, sol, workDir, opts.Mirror, func(p int) {
			logf("下载进度 %d%%", p)
		})
		if err != nil {
			return nil, err
		}
		apk = path
	}
	logf("使用安装包：%s", filepath.Base(apk))

	toolDir := filepath.Join(workDir, "tool")
	files, err := prepareToolFiles(apk, sol.Family, toolDir)
	if err != nil {
		return nil, err
	}
	logf("已解出补丁工具 %d 个文件", len(files))

	// 2. 推送到设备
	logf("初始化设备目录 %s", deviceWorkDir)
	if _, err := a.Shell(ctx, opts.Serial, "mkdir -p "+deviceWorkDir+" && rm -f "+deviceWorkDir+"/*"); err != nil {
		return nil, fmt.Errorf("初始化设备目录失败：%w", err)
	}
	for _, f := range files {
		remote := deviceWorkDir + "/" + filepath.Base(f)
		if err := a.Push(ctx, opts.Serial, f, remote, onLine); err != nil {
			return nil, fmt.Errorf("推送 %s 失败：%w", filepath.Base(f), err)
		}
	}
	if err := a.Push(ctx, opts.Serial, opts.BootImage, deviceWorkDir+"/boot.img", onLine); err != nil {
		return nil, fmt.Errorf("推送 boot 镜像失败：%w", err)
	}
	logf("已推送 boot 镜像与补丁工具")

	// 3. 在设备上执行补丁
	cmd, outName := patchCommand(sol.Family)
	if cmd == "" {
		return nil, fmt.Errorf("暂不支持该方案的自动补丁：%s", sol.Family)
	}
	logf("执行补丁：%s", cmd)
	shellCmd := "cd " + deviceWorkDir + " && chmod -R 755 . && " + cmd
	if out, err := a.ShellStream(ctx, opts.Serial, shellCmd, onLine); err != nil {
		return nil, fmt.Errorf("补丁执行失败：%w", err)
	} else if out != nil && out.ExitCode != 0 {
		return nil, fmt.Errorf("补丁执行失败，退出码 %d", out.ExitCode)
	}

	// 4. 校验输出并拉回
	checkCmd := "ls -l " + deviceWorkDir + "/" + outName
	out, err := a.Shell(ctx, opts.Serial, checkCmd)
	if err != nil || strings.Contains(out, "No such file") || strings.TrimSpace(out) == "" {
		return nil, fmt.Errorf("补丁未生成预期文件 %s，请查看日志确认工具输出", outName)
	}
	localOut := filepath.Join(workDir, outName)
	if err := a.Pull(ctx, opts.Serial, deviceWorkDir+"/"+outName, localOut, onLine); err != nil {
		return nil, fmt.Errorf("拉取补丁镜像失败：%w", err)
	}

	// 清理
	a.Shell(context.Background(), opts.Serial, "rm -rf "+deviceWorkDir)

	logf("补丁镜像已生成：%s", localOut)
	res.PatchedImage = localOut
	return res, nil
}

// patchCommand 返回设备端执行的补丁命令与输出文件名。
func patchCommand(family string) (cmd, outName string) {
	switch family {
	case "magisk":
		return "sh ./boot_patch.sh boot.img", "new-boot.img"
	case "kernelsu":
		return "./ksud boot-patch -b boot.img -o patched.img", "patched.img"
	case "apatch":
		return "./apd -p -i boot.img -o patched.img -k ./kpatch -s droidforge", "patched.img"
	}
	return "", ""
}

// ExtractBootFromDevice 在已 root 的设备上导出 boot / init_boot 分区。
func ExtractBootFromDevice(ctx context.Context, a *adb.Client, serial, partition, localOut string, onLine runner.LineHandler) error {
	if partition == "" {
		partition = "boot"
	}
	remote := "/sdcard/droidforge_" + partition + ".img"
	cmd := fmt.Sprintf("su -c 'dd if=/dev/block/by-name/%s of=%s bs=4096'", partition, remote)
	if onLine != nil {
		onLine("执行："+cmd, runner.Stdout)
	}
	if _, err := a.ShellStream(ctx, serial, cmd, onLine); err != nil {
		return fmt.Errorf("导出分区失败（需要 root）：%w", err)
	}
	defer a.Shell(context.Background(), serial, "rm -f "+remote)
	return a.Pull(ctx, serial, remote, localOut, onLine)
}

func ensureDir(dir string) error {
	return osMkdirAll(dir)
}
