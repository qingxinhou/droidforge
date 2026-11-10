package adb

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"droidforge/internal/runner"
)

// AppDetail 是单个应用的详细信息。
type AppDetail struct {
	Package      string   `json:"package"`
	VersionName  string   `json:"versionName"`
	VersionCode  int64    `json:"versionCode"`
	MinSDK       string   `json:"minSdk"`
	TargetSDK    string   `json:"targetSdk"`
	FirstInstall string   `json:"firstInstall"`
	LastUpdate   string   `json:"lastUpdate"`
	DataDir      string   `json:"dataDir"`
	UID          string   `json:"uid"`
	CodePath     string   `json:"codePath"`
	SplitPaths   []string `json:"splitPaths"`
	Flags        []string `json:"flags"`
	Enabled      bool     `json:"enabled"`
	System       bool     `json:"system"`
}

// ListPackagesDetail 列出应用，并标记禁用状态。
func (c *Client) ListPackagesDetail(ctx context.Context, serial string, includeSystem bool) ([]AppInfo, error) {
	cmd := "pm list packages -f"
	if !includeSystem {
		cmd += " -3"
	}
	out, err := c.Shell(ctx, serial, cmd)
	if err != nil {
		return nil, err
	}
	disabled := map[string]bool{}
	if dOut, err := c.Shell(ctx, serial, "pm list packages -d"); err == nil {
		for _, line := range strings.Split(dOut, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "package:") {
				disabled[strings.TrimPrefix(line, "package:")] = true
			}
		}
	}

	apps := make([]AppInfo, 0, 96)
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "package:") {
			continue
		}
		path, pkg, ok := splitPkgLine(line)
		if !ok {
			continue
		}
		system := strings.HasPrefix(path, "/system") || strings.HasPrefix(path, "/product") ||
			strings.HasPrefix(path, "/vendor") || strings.HasPrefix(path, "/system_ext") ||
			strings.HasPrefix(path, "/odm") || strings.HasPrefix(path, "/apex")
		apps = append(apps, AppInfo{
			Package: pkg,
			APKPath: path,
			System:  system,
			Enabled: !disabled[pkg],
		})
	}
	sort.SliceStable(apps, func(i, j int) bool { return apps[i].Package < apps[j].Package })
	return apps, nil
}

// SetAppEnabled 启用或禁用应用（禁用使用 disable-user，可逆且不会破坏系统分区）。
func (c *Client) SetAppEnabled(ctx context.Context, serial, pkg string, enabled bool) error {
	if err := validPkg(pkg); err != nil {
		return err
	}
	cmd := "pm disable-user --user 0 " + pkg
	if enabled {
		cmd = "pm enable " + pkg
	}
	_, err := c.Shell(ctx, serial, cmd)
	return err
}

// ForceStopApp 强行停止应用。
func (c *Client) ForceStopApp(ctx context.Context, serial, pkg string) error {
	if err := validPkg(pkg); err != nil {
		return err
	}
	_, err := c.Shell(ctx, serial, "am force-stop "+pkg)
	return err
}

// LaunchApp 启动应用。
//
// 真机上三种情况会让朴素的 `monkey -p <pkg> -c LAUNCHER` 失败：
//  1. 应用只有文件打开的 Activity（WPS lite 就是这种插件包），压根没有
//     MAIN/LAUNCHER 入口 —— monkey 回 "No activities found"；
//  2. 应用被 disable-user 禁用了；
//  3. monkey 本身在部分 ROM 上不可靠，但 am start 能起。
//
// 所以按「先解析出具体组件 → am start → monkey 兜底 → 精确报错」走，
// 而不是把所有失败都压成一句「未找到可启动的 Activity」。
func (c *Client) LaunchApp(ctx context.Context, serial, pkg string) error {
	if err := validPkg(pkg); err != nil {
		return err
	}

	// 1) 让系统告诉我们启动组件是谁，拿到后直接 am start。
	out, err := c.Shell(ctx, serial,
		"cmd package resolve-activity --brief -a android.intent.action.MAIN -c android.intent.category.LAUNCHER "+pkg)
	if err == nil {
		if comp, ok := ParseResolveActivity(out); ok {
			startOut, serr := c.Shell(ctx, serial, "am start -n "+comp)
			if serr == nil && !startFailed(startOut) {
				return nil
			}
		}
	}

	// 2) 兜底：monkey。它不看组件名，对别名 / 动态注册的入口更宽容。
	if mOut, merr := c.Shell(ctx, serial, "monkey -p "+pkg+" -c android.intent.category.LAUNCHER 1"); merr == nil {
		if !strings.Contains(mOut, "No activities found") && !strings.Contains(mOut, "monkey aborted") {
			return nil
		}
	}

	// 3) 全失败：给出能行动的原因，而不是笼统的「未找到」。
	//    先分清「没装」「被禁了」「装了但没入口」三种情况 —— 它们的
	//    处理办法完全不同，混成一句话等于没说。
	if pkgs, perr := c.Shell(ctx, serial, "pm list packages "+pkg); perr == nil && !hasPackage(pkgs, pkg) {
		return fmt.Errorf("设备上没有安装这个应用（%s）", pkg)
	}
	if dis, derr := c.Shell(ctx, serial, "pm list packages -d "+pkg); derr == nil && hasPackage(dis, pkg) {
		return fmt.Errorf("应用已被禁用，先在应用列表里启用再启动")
	}
	return fmt.Errorf("该应用没有可启动的入口（可能只提供文件打开/编辑的 Activity，如插件型应用）")
}

// hasPackage 判断 `pm list packages` 的输出里是否有这个包。
//
// 不能用 strings.Contains：`pm list packages com.foo` 是模糊过滤，
// com.foobar 也会出现在结果里，必须逐行精确匹配 "package:<pkg>"。
func hasPackage(out, pkg string) bool {
	want := "package:" + pkg
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) == want {
			return true
		}
	}
	return false
}

// ParseResolveActivity 从 `resolve-activity --brief` 的输出里取组件名。
//
// 真机输出（两行，末行是答案）：
//
//	priority=0 preferredOrder=0 match=0x108000 specificIndex=-1 isDefault=true
//	com.android.settings/.MainSettings
//
// 解析不出来时回 "No activity found"。
func ParseResolveActivity(out string) (string, bool) {
	lines := strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if strings.Contains(line, "No activity found") {
			return "", false
		}
		// 组件名形如 com.foo/.Bar 或 com.foo/com.foo.Bar，没有空格和等号。
		if strings.Contains(line, "/") && !strings.ContainsAny(line, " =") {
			return line, true
		}
	}
	return "", false
}

// startFailed 判断 am start 的输出是否失败。
//
// am start 的失败不会体现在退出码上（仍然是 0），只能读文本：
// "Error: Activity not started, unable to resolve Intent"。
func startFailed(out string) bool {
	low := strings.ToLower(out)
	return strings.Contains(low, "error:") || strings.Contains(low, "unable to resolve") ||
		strings.Contains(low, "does not exist")
}

// ClearAppData 清除应用数据。
func (c *Client) ClearAppData(ctx context.Context, serial, pkg string) error {
	if err := validPkg(pkg); err != nil {
		return err
	}
	out, err := c.Shell(ctx, serial, "pm clear "+pkg)
	if err != nil {
		return err
	}
	if strings.Contains(out, "Failed") {
		return fmt.Errorf("%s", strings.TrimSpace(out))
	}
	return nil
}

// RestoreSystemApp 恢复被卸载的系统应用（Android 7+）。
func (c *Client) RestoreSystemApp(ctx context.Context, serial, pkg string) error {
	if err := validPkg(pkg); err != nil {
		return err
	}
	out, err := c.Shell(ctx, serial, "cmd package install-existing "+pkg)
	if err != nil {
		return err
	}
	if strings.Contains(strings.ToLower(out), "not found") || strings.Contains(out, "Error") {
		return fmt.Errorf("%s", strings.TrimSpace(out))
	}
	return nil
}

// PackagePaths 返回应用的所有 APK 路径（含 split）。
func (c *Client) PackagePaths(ctx context.Context, serial, pkg string) ([]string, error) {
	if err := validPkg(pkg); err != nil {
		return nil, err
	}
	out, err := c.Shell(ctx, serial, "pm path "+pkg)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "package:") {
			paths = append(paths, strings.TrimPrefix(line, "package:"))
		}
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("未找到该应用的 APK 路径")
	}
	return paths, nil
}

// ExtractApp 把应用的所有 APK 拉取到 outDir。
func (c *Client) ExtractApp(ctx context.Context, serial, pkg, outDir string, onLine runner.LineHandler) ([]string, error) {
	paths, err := c.PackagePaths(ctx, serial, pkg)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}
	var saved []string
	for i, p := range paths {
		name := pkg + ".apk"
		if i > 0 {
			name = fmt.Sprintf("%s-%d.apk", pkg, i)
		}
		// split 类型尽量保留原始文件名
		if b := filepath.Base(p); strings.HasPrefix(b, "split_") || strings.Contains(b, "config.") {
			name = pkg + "-" + b
		}
		dst := filepath.Join(outDir, name)
		if err := c.Pull(ctx, serial, p, dst, onLine); err != nil {
			return saved, err
		}
		saved = append(saved, dst)
	}
	return saved, nil
}

// AppDetailOf 读取单个应用的详细信息。
func (c *Client) AppDetailOf(ctx context.Context, serial, pkg string) (AppDetail, error) {
	if err := validPkg(pkg); err != nil {
		return AppDetail{}, err
	}
	d := AppDetail{Package: pkg}
	if out, err := c.Shell(ctx, serial, "dumpsys package "+pkg); err == nil {
		kv := map[string]string{}
		re := regexp.MustCompile(`^\s*([A-Za-z][A-Za-z0-9 _]*?)=(.*)$`)
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimRight(line, "\r")
			if m := re.FindStringSubmatch(line); m != nil {
				key := strings.TrimSpace(m[1])
				if _, exists := kv[key]; !exists {
					kv[key] = strings.TrimSpace(m[2])
				}
			}
			trimmed := strings.TrimSpace(line)
			switch {
			case strings.HasPrefix(trimmed, "pkgFlags="):
				raw := strings.Trim(strings.TrimPrefix(trimmed, "pkgFlags="), "[]")
				d.Flags = append(d.Flags, strings.Fields(raw)...)
			case strings.HasPrefix(trimmed, "codePath="):
				d.CodePath = strings.TrimSpace(strings.TrimPrefix(trimmed, "codePath="))
			case strings.HasPrefix(trimmed, "enabled="):
				if kv["__enabled"] == "" {
					kv["__enabled"] = strings.TrimSpace(strings.TrimPrefix(trimmed, "enabled="))
				}
			}
		}
		d.VersionName = kv["versionName"]
		d.VersionCode = parseInt64(kv["versionCode"])
		d.MinSDK = firstToken(kv["minSdk"])
		d.TargetSDK = firstToken(kv["targetSdk"])
		d.FirstInstall = kv["firstInstallTime"]
		d.LastUpdate = kv["lastUpdateTime"]
		d.DataDir = kv["dataDir"]
		d.UID = kv["userId"]
		if d.CodePath == "" {
			d.CodePath = kv["codePath"]
		}
		enabled := kv["__enabled"]
		d.Enabled = enabled == "" || enabled == "0" || enabled == "1" || enabled == "true"
		for _, f := range d.Flags {
			if f == "SYSTEM" {
				d.System = true
			}
		}
	}
	if paths, err := c.PackagePaths(ctx, serial, pkg); err == nil {
		d.SplitPaths = paths
		if d.CodePath == "" {
			d.CodePath = paths[0]
		}
	}
	return d, nil
}

// Run 直接执行任意 adb 参数（供脚本使用）。
func (c *Client) Run(ctx context.Context, serial string, args []string) (string, error) {
	full := make([]string, 0, len(args)+2)
	if serial != "" {
		full = append(full, "-s", serial)
	}
	full = append(full, args...)
	res, err := runner.Run(ctx, "", c.Bin, full...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(res.Combined()), nil
}

// InstallMultiple 安装拆分 APK（base.apk + split）。
func (c *Client) InstallMultiple(ctx context.Context, serial string, apks []string, reinstall bool, onLine runner.LineHandler) error {
	if len(apks) == 0 {
		return fmt.Errorf("未选择 APK")
	}
	args := []string{}
	if serial != "" {
		args = append(args, "-s", serial)
	}
	args = append(args, "install-multiple")
	if reinstall {
		args = append(args, "-r")
	}
	args = append(args, apks...)
	res, err := runner.RunStream(ctx, "", onLine, c.Bin, args...)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 || strings.Contains(res.Combined(), "Failure") {
		return fmt.Errorf("%s", strings.TrimSpace(res.Combined()))
	}
	return nil
}

// InstallBatch 逐个安装多个 APK，返回失败列表。
func (c *Client) InstallBatch(ctx context.Context, serial string, apks []string, reinstall bool, onLine runner.LineHandler) ([]string, error) {
	var failed []string
	for _, apk := range apks {
		if err := c.Install(ctx, serial, apk, reinstall, onLine); err != nil {
			failed = append(failed, filepath.Base(apk)+": "+err.Error())
		}
	}
	if len(failed) > 0 {
		return failed, fmt.Errorf("%d 个安装失败", len(failed))
	}
	return nil, nil
}

func validPkg(pkg string) error {
	pkg = strings.TrimSpace(pkg)
	if pkg == "" {
		return fmt.Errorf("包名不能为空")
	}
	for _, r := range pkg {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '.' || r == '_' || r == '-'
		if !ok {
			return fmt.Errorf("包名包含非法字符：%s", pkg)
		}
	}
	return nil
}

func firstToken(s string) string {
	f := strings.Fields(s)
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

func parseInt64(s string) int64 {
	s = firstToken(s)
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}
