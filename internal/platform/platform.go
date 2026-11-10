// Package platform 负责定位 adb / fastboot 可执行文件。
//
// 查找顺序（优先级从高到低）：
//  1. 用户自定义路径
//  2. 程序目录下的 platform-tools/（绿色版随身携带）
//  3. ANDROID_HOME / ANDROID_SDK_ROOT
//  4. 常见 Android SDK 安装位置
//  5. 系统 PATH
package platform

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"droidforge/internal/runner"
)

// Tools 描述当前可用的平台工具状态。
type Tools struct {
	AdbPath         string `json:"adbPath"`
	FastbootPath    string `json:"fastbootPath"`
	AdbVersion      string `json:"adbVersion"`
	FastbootVersion string `json:"fastbootVersion"`
	Source          string `json:"source"`
	Ready           bool   `json:"ready"`
	Hint            string `json:"hint"`
}

func exeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Dir(exe)
}

// candidateRoots 返回可能包含 platform-tools 的候选目录（不含 PATH）。
func candidateRoots() []string {
	var roots []string
	if v := os.Getenv("ANDROID_HOME"); v != "" {
		roots = append(roots, v)
	}
	if v := os.Getenv("ANDROID_SDK_ROOT"); v != "" {
		roots = append(roots, v)
	}
	if runtime.GOOS == "windows" {
		if v := os.Getenv("LOCALAPPDATA"); v != "" {
			roots = append(roots, filepath.Join(v, "Android", "Sdk"))
		}
		roots = append(roots,
			`C:\Android\Sdk`,
			`C:\Android`,
			`C:\platform-tools`,
			`D:\Android\Sdk`,
		)
	} else if runtime.GOOS == "darwin" {
		home, _ := os.UserHomeDir()
		roots = append(roots,
			filepath.Join(home, "Library", "Android", "sdk"),
			"/usr/local/share/android-sdk",
			"/opt/homebrew/share/android-commandlinetools",
		)
	} else {
		home, _ := os.UserHomeDir()
		roots = append(roots,
			filepath.Join(home, "Android", "Sdk"),
			"/usr/lib/android-sdk",
		)
	}
	return roots
}

// Detect 按优先级探测 adb / fastboot。
func Detect(customAdb, customFastboot string) Tools {
	var t Tools

	tryAdb := func(p string) bool {
		if p == "" {
			return false
		}
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			t.AdbPath = p
			return true
		}
		return false
	}
	tryFastboot := func(p string) bool {
		if p == "" {
			return false
		}
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			t.FastbootPath = p
			return true
		}
		return false
	}

	// 1. 自定义
	if customAdb != "" {
		tryAdb(customAdb)
	}
	if customFastboot != "" {
		tryFastboot(customFastboot)
	}
	if t.AdbPath != "" && t.FastbootPath != "" {
		t.Source = "custom"
	}

	// 2. 程序目录 / platform-tools
	if t.AdbPath == "" || t.FastbootPath == "" {
		if dir := exeDir(); dir != "" {
			base := filepath.Join(dir, "platform-tools")
			if t.AdbPath == "" {
				tryAdb(filepath.Join(base, exeName("adb")))
			}
			if t.FastbootPath == "" {
				tryFastboot(filepath.Join(base, exeName("fastboot")))
			}
			if t.AdbPath != "" {
				t.Source = "bundled"
			}
		}
	}

	// 3/4. SDK 目录
	if t.AdbPath == "" || t.FastbootPath == "" {
		for _, root := range candidateRoots() {
			base := filepath.Join(root, "platform-tools")
			if t.AdbPath == "" {
				tryAdb(filepath.Join(base, exeName("adb")))
			}
			if t.FastbootPath == "" {
				tryFastboot(filepath.Join(base, exeName("fastboot")))
			}
			if t.AdbPath != "" && t.FastbootPath != "" {
				t.Source = "sdk"
				break
			}
		}
	}

	// 5. PATH
	if t.AdbPath == "" {
		if p, err := resolveFromPath("adb"); err == nil {
			t.AdbPath = p
			t.Source = "path"
		}
	}
	if t.FastbootPath == "" {
		if p, err := resolveFromPath("fastboot"); err == nil {
			t.FastbootPath = p
			if t.Source == "" {
				t.Source = "path"
			}
		}
	}

	if t.AdbPath != "" {
		if out, err := runner.Run(context.Background(), "", t.AdbPath, "version"); err == nil {
			t.AdbVersion = firstLine(out.Stdout)
		}
	}
	if t.FastbootPath != "" {
		if out, err := runner.Run(context.Background(), "", t.FastbootPath, "--version"); err == nil {
			t.FastbootVersion = firstLine(out.Combined())
		}
	}

	t.Ready = t.AdbPath != "" && t.FastbootPath != ""
	switch {
	case t.Ready:
		t.Hint = "平台工具就绪"
	case t.AdbPath != "":
		t.Hint = "已找到 adb，但缺少 fastboot（线刷/解锁功能不可用）"
	case t.FastbootPath != "":
		t.Hint = "已找到 fastboot，但缺少 adb（设备信息/终端功能不可用）"
	default:
		t.Hint = "未找到 adb / fastboot，可在「设置」中下载官方 platform-tools"
	}
	return t
}

func resolveFromPath(name string) (string, error) {
	return execLookPath(exeName(name))
}

func firstLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			return strings.TrimSpace(l)
		}
	}
	return ""
}

// PlatformToolsURL 返回当前系统对应的官方 platform-tools 下载地址。
func PlatformToolsURL() string {
	switch runtime.GOOS {
	case "windows":
		return "https://dl.google.com/android/repository/platform-tools-latest-windows.zip"
	case "darwin":
		return "https://dl.google.com/android/repository/platform-tools-latest-darwin.zip"
	default:
		return "https://dl.google.com/android/repository/platform-tools-latest-linux.zip"
	}
}

// DownloadPlatformTools 下载官方 platform-tools 并解压到 destDir。
// onProgress 回调接收 0-100 的进度，可为 nil。
func DownloadPlatformTools(ctx context.Context, destDir string, onLine func(string), onProgress func(int)) error {
	if onLine == nil {
		onLine = func(string) {}
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	url := PlatformToolsURL()
	onLine("开始下载：" + url)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 0}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("下载失败：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败：HTTP %d", resp.StatusCode)
	}

	tmp := filepath.Join(os.TempDir(), fmt.Sprintf("platform-tools-%d.zip", time.Now().Unix()))
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	total := resp.ContentLength
	var written int64
	buf := make([]byte, 128*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				f.Close()
				return werr
			}
			written += int64(n)
			if total > 0 && onProgress != nil {
				onProgress(int(written * 100 / total))
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			return rerr
		}
	}
	f.Close()
	defer os.Remove(tmp)
	onLine("下载完成，正在解压…")

	zr, err := zip.OpenReader(tmp)
	if err != nil {
		return err
	}
	defer zr.Close()

	for _, file := range zr.File {
		target := filepath.Join(destDir, file.Name)
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)+string(os.PathSeparator)) {
			continue // 防目录穿越
		}
		if file.FileInfo().IsDir() {
			_ = os.MkdirAll(target, 0o755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := file.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, file.Mode())
		if err != nil {
			rc.Close()
			return err
		}
		_, cpErr := io.Copy(out, rc)
		rc.Close()
		out.Close()
		if cpErr != nil {
			return cpErr
		}
		// 类 Unix 下恢复可执行权限
		if runtime.GOOS != "windows" {
			_ = os.Chmod(target, 0o755)
		}
	}
	onLine("platform-tools 已解压到：" + destDir)
	return nil
}
