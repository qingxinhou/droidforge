// Package magisk 处理 Magisk / KernelSU 模块的批量安装。
//
// 只做「把模块装进设备」这一步，不内置任何模块，也不从网络拉取——
// 模块来源由用户自己负责，页面上会明确提示这一点。
package magisk

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"droidforge/internal/runner"
)

// Module 是一个模块包的基本信息。
type Module struct {
	Path        string `json:"path"`
	FileName    string `json:"fileName"`
	Size        int64  `json:"size"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	VersionCode string `json:"versionCode"`
	Author      string `json:"author"`
	Description string `json:"description"`
	// Valid 为 false 时 Error 说明原因。
	Valid bool   `json:"valid"`
	Error string `json:"error"`
}

// Inspect 读 zip 里的 module.prop。模块包可能在根目录，也可能在某个子目录里。
func Inspect(p string) (*Module, error) {
	st, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	m := &Module{Path: p, FileName: filepath.Base(p), Size: st.Size()}

	zr, err := zip.OpenReader(p)
	if err != nil {
		m.Error = "不是有效的 zip：" + err.Error()
		return m, nil
	}
	defer zr.Close()

	var prop *zip.File
	for _, f := range zr.File {
		base := strings.ToLower(path.Base(f.Name))
		if base != "module.prop" {
			continue
		}
		// 越浅越好：根目录的 module.prop 优先。
		if prop == nil || strings.Count(f.Name, "/") < strings.Count(prop.Name, "/") {
			prop = f
		}
	}
	if prop == nil {
		m.Error = "包里没有 module.prop，不是 Magisk 模块（也可能是个普通刷机包）"
		return m, nil
	}
	if prop.UncompressedSize64 > 1<<20 {
		m.Error = "module.prop 异常大，拒绝解析"
		return m, nil
	}

	rc, err := prop.Open()
	if err != nil {
		m.Error = "读取 module.prop 失败：" + err.Error()
		return m, nil
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, 1<<20))
	if err != nil {
		m.Error = "读取 module.prop 失败：" + err.Error()
		return m, nil
	}

	kv := parseProp(string(data))
	m.ID = kv["id"]
	m.Name = kv["name"]
	m.Version = kv["version"]
	m.VersionCode = kv["versionCode"]
	m.Author = kv["author"]
	m.Description = kv["description"]
	if m.Name == "" {
		m.Name = m.FileName
	}
	if m.ID == "" {
		m.Error = "module.prop 缺少 id 字段"
		return m, nil
	}
	m.Valid = true
	return m, nil
}

// parseProp 解析 key=value 格式，忽略注释与空行。
func parseProp(s string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.IndexByte(line, '=')
		if i <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:i])
		val := strings.TrimSpace(line[i+1:])
		out[key] = val
	}
	return out
}

// Manager 是设备上的模块管理器类型。
type Manager struct {
	// Kind 是 magisk / kernelsu / apatch / unknown
	Kind string `json:"kind"`
	// Version 是管理器版本，取不到时为空。
	Version string `json:"version"`
	// RootOK 表示 su 可用。
	RootOK bool   `json:"rootOk"`
	Note   string `json:"note"`
}

// DetectManager 探测设备上装的是哪个 root 方案。
func DetectManager(ctx context.Context, bin, serial string) Manager {
	m := Manager{Kind: "unknown"}

	sh := func(cmd string) string {
		args := []string{}
		if serial != "" {
			args = append(args, "-s", serial)
		}
		args = append(args, "shell", cmd)
		res, err := runner.Run(ctx, "", bin, args...)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(res.Stdout)
	}

	// su -c id 是最直接的 root 判据。
	if uid := sh(`su -c id 2>/dev/null`); strings.Contains(uid, "uid=0") {
		m.RootOK = true
	}

	if v := sh(`su -c 'magisk -V' 2>/dev/null`); v != "" {
		m.Kind = "magisk"
		m.Version = v
	} else if v := sh(`su -c 'ksud -V' 2>/dev/null`); v != "" {
		m.Kind = "kernelsu"
		m.Version = v
	} else if v := sh(`su -c 'apd -V' 2>/dev/null`); v != "" {
		m.Kind = "apatch"
		m.Version = v
	}

	switch {
	case !m.RootOK:
		m.Note = "su 不可用，无法安装模块。请先在「Root 方案」里完成 root"
	case m.Kind == "unknown":
		m.Note = "su 可用但没有识别到模块管理器，安装会直接失败"
	default:
		m.Note = "已识别 " + m.Kind + " " + m.Version
	}
	return m
}

// InstallResult 是单个模块的安装结果。
type InstallResult struct {
	Module Module `json:"module"`
	OK     bool   `json:"ok"`
	Output string `json:"output"`
	Error  string `json:"error"`
}

const remoteTmp = "/data/local/tmp/droidforge"

// InstallBatch 逐个推送并安装模块。
func InstallBatch(ctx context.Context, bin, serial string, mgr Manager, mods []Module, onLine func(string)) ([]InstallResult, error) {
	if len(mods) == 0 {
		return nil, errors.New("没有选择任何模块")
	}
	if !mgr.RootOK {
		return nil, errors.New("当前设备没有 root 权限，无法安装模块")
	}
	if mgr.Kind == "unknown" {
		return nil, errors.New("没有识别到 Magisk / KernelSU / APatch，无法确定安装命令")
	}

	adb := func(args ...string) []string {
		out := []string{}
		if serial != "" {
			out = append(out, "-s", serial)
		}
		return append(out, args...)
	}

	// 目录不存在时 push 会失败，先建。
	if _, err := runner.Run(ctx, "", bin, adb("shell", "mkdir -p "+remoteTmp)...); err != nil {
		return nil, fmt.Errorf("创建临时目录失败：%w", err)
	}

	results := make([]InstallResult, 0, len(mods))
	for i, m := range mods {
		r := InstallResult{Module: m}
		if !m.Valid {
			r.Error = m.Error
			results = append(results, r)
			continue
		}
		if onLine != nil {
			onLine(fmt.Sprintf("[%d/%d] 推送 %s", i+1, len(mods), m.FileName))
		}

		remote := remoteTmp + "/" + safeName(m.FileName)
		if _, err := runner.Run(ctx, "", bin, adb("push", m.Path, remote)...); err != nil {
			r.Error = "推送失败：" + err.Error()
			results = append(results, r)
			continue
		}

		cmd := installCommand(mgr.Kind, remote)
		res, err := runner.Run(ctx, "", bin, adb("shell", cmd)...)
		out := ""
		if res != nil {
			out = strings.TrimSpace(res.Combined())
		}
		r.Output = out
		if err != nil {
			r.Error = err.Error()
		} else if looksLikeFailure(out) {
			r.Error = "安装命令返回失败：" + firstLine(out)
		} else {
			r.OK = true
			if onLine != nil {
				onLine(fmt.Sprintf("已安装 %s %s", m.Name, m.Version))
			}
		}
		// 装完删掉设备上的临时包，避免占 /data/local/tmp。
		_, _ = runner.Run(ctx, "", bin, adb("shell", "rm -f "+remote)...)
		results = append(results, r)
	}

	okCount := 0
	for _, r := range results {
		if r.OK {
			okCount++
		}
	}
	if onLine != nil {
		onLine(fmt.Sprintf("完成：%d 个成功，%d 个失败。模块需要重启后生效", okCount, len(results)-okCount))
	}
	return results, nil
}

func installCommand(kind, remote string) string {
	switch kind {
	case "kernelsu":
		return "su -c 'ksud module install " + remote + "'"
	case "apatch":
		return "su -c 'apd module install " + remote + "'"
	default:
		return "su -c 'magisk --install-module " + remote + "'"
	}
}

func looksLikeFailure(out string) bool {
	low := strings.ToLower(out)
	for _, bad := range []string{"failed", "error", "not found", "no such", "denied", "unable"} {
		if strings.Contains(low, bad) {
			return true
		}
	}
	return false
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

// safeName 去掉文件名里可能破坏 shell 命令的字符。
func safeName(name string) string {
	base := filepath.Base(name)
	var sb strings.Builder
	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			sb.WriteRune(r)
		case r == '.' || r == '-' || r == '_':
			sb.WriteRune(r)
		default:
			sb.WriteByte('_')
		}
	}
	out := sb.String()
	if out == "" || out == "." || out == ".." {
		return "module.zip"
	}
	return out
}

// ScanDir 扫描目录里的所有 zip 并解析模块信息。
func ScanDir(dir string) ([]Module, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []Module
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".zip") {
			continue
		}
		m, err := Inspect(filepath.Join(dir, e.Name()))
		if err != nil || m == nil {
			continue
		}
		out = append(out, *m)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].FileName < out[j].FileName })
	return out, nil
}
