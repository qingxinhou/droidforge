// Package adb 封装 adb 命令行能力。
package adb

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"droidforge/internal/runner"
)

// Device 表示一个 adb 设备。
type Device struct {
	Serial      string `json:"serial"`
	State       string `json:"state"` // device / offline / unauthorized / bootloader
	Model       string `json:"model"`
	Product     string `json:"product"`
	Device      string `json:"device"`
	TransportID string `json:"transportId"`
}

// Connected 判断设备是否可执行命令。
func (d Device) Connected() bool { return d.State == "device" }

// Client 是一次会话内的 adb 调用器。
type Client struct {
	Bin string
}

func New(bin string) *Client { return &Client{Bin: bin} }

// Devices 列出所有 adb 设备。
func (c *Client) Devices(ctx context.Context) ([]Device, error) {
	res, err := runner.Run(ctx, "", c.Bin, "devices", "-l")
	if err != nil {
		return nil, err
	}
	devices := make([]Device, 0, 4)
	for _, line := range strings.Split(res.Stdout, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(line, "List of devices") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		d := Device{Serial: fields[0], State: fields[1]}
		for _, f := range fields[2:] {
			k, v, ok := strings.Cut(f, ":")
			if !ok {
				continue
			}
			switch k {
			case "model":
				d.Model = v
			case "product":
				d.Product = v
			case "device":
				d.Device = v
			case "transport_id":
				d.TransportID = v
			}
		}
		devices = append(devices, d)
	}
	sort.SliceStable(devices, func(i, j int) bool { return devices[i].Serial < devices[j].Serial })
	return devices, nil
}

// Connect 连接无线 adb 设备，addr 形如 192.168.1.10:5555。
func (c *Client) Connect(ctx context.Context, addr string) (string, error) {
	res, err := runner.Run(ctx, "", c.Bin, "connect", addr)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(res.Combined()), nil
}

// Disconnect 断开无线 adb 设备。
func (c *Client) Disconnect(ctx context.Context, addr string) (string, error) {
	res, err := runner.Run(ctx, "", c.Bin, "disconnect", addr)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(res.Combined()), nil
}

// EnableTcpip 将设备切换为无线调试模式（需要设备先 USB 连接）。
func (c *Client) EnableTcpip(ctx context.Context, serial string, port int) (string, error) {
	res, err := runner.Run(ctx, "", c.Bin, "-s", serial, "tcpip", strconv.Itoa(port))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(res.Combined()), nil
}

// Shell 执行一条 shell 命令，返回 stdout+stderr。
func (c *Client) Shell(ctx context.Context, serial, command string) (string, error) {
	args := []string{}
	if serial != "" {
		args = append(args, "-s", serial)
	}
	args = append(args, "shell", command)
	res, err := runner.Run(ctx, "", c.Bin, args...)
	if err != nil {
		return "", err
	}
	out := res.Combined()
	// adb shell 的退出码经常不可靠，这里只在明显失败且无输出时提示
	if res.ExitCode != 0 && strings.TrimSpace(out) == "" {
		return out, fmt.Errorf("命令执行失败（退出码 %d）", res.ExitCode)
	}
	return out, nil
}

// ShellStream 执行 shell 命令并流式回调每一行。
func (c *Client) ShellStream(ctx context.Context, serial, command string, onLine runner.LineHandler) (*runner.Result, error) {
	args := []string{}
	if serial != "" {
		args = append(args, "-s", serial)
	}
	args = append(args, "shell", command)
	return runner.RunStream(ctx, "", onLine, c.Bin, args...)
}

// Reboot 重启设备到指定目标："" / recovery / bootloader / fastboot / edl / download / sideload / userspace。
func (c *Client) Reboot(ctx context.Context, serial, target string) error {
	args := []string{}
	if serial != "" {
		args = append(args, "-s", serial)
	}
	args = append(args, "reboot")
	if target != "" {
		args = append(args, target)
	}
	_, err := runner.MustRun(ctx, "", c.Bin, args...)
	return err
}

// Root 尝试以 root 身份重启 adbd（需要已 root）。
func (c *Client) Root(ctx context.Context, serial string) (string, error) {
	args := []string{}
	if serial != "" {
		args = append(args, "-s", serial)
	}
	args = append(args, "root")
	res, err := runner.Run(ctx, "", c.Bin, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(res.Combined()), nil
}

// Push 推送文件到设备。
func (c *Client) Push(ctx context.Context, serial, local, remote string, onLine runner.LineHandler) error {
	args := []string{}
	if serial != "" {
		args = append(args, "-s", serial)
	}
	args = append(args, "push", local, remote)
	_, err := runner.RunStream(ctx, "", onLine, c.Bin, args...)
	return err
}

// Pull 从设备拉取文件。
func (c *Client) Pull(ctx context.Context, serial, remote, local string, onLine runner.LineHandler) error {
	args := []string{}
	if serial != "" {
		args = append(args, "-s", serial)
	}
	args = append(args, "pull", remote, local)
	_, err := runner.RunStream(ctx, "", onLine, c.Bin, args...)
	return err
}

// Install 安装 APK。
func (c *Client) Install(ctx context.Context, serial, apk string, reinstall bool, onLine runner.LineHandler) error {
	args := []string{}
	if serial != "" {
		args = append(args, "-s", serial)
	}
	args = append(args, "install")
	if reinstall {
		args = append(args, "-r")
	}
	args = append(args, apk)
	res, err := runner.RunStream(ctx, "", onLine, c.Bin, args...)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 || strings.Contains(res.Combined(), "Failure") {
		return fmt.Errorf("%s", strings.TrimSpace(res.Combined()))
	}
	return nil
}

// Uninstall 卸载应用。
func (c *Client) Uninstall(ctx context.Context, serial, pkg string, keepData bool) error {
	args := []string{}
	if serial != "" {
		args = append(args, "-s", serial)
	}
	args = append(args, "uninstall")
	if keepData {
		args = append(args, "-k")
	}
	args = append(args, pkg)
	res, err := runner.Run(ctx, "", c.Bin, args...)
	if err != nil {
		return err
	}
	if strings.Contains(res.Combined(), "Failure") {
		return fmt.Errorf("%s", strings.TrimSpace(res.Combined()))
	}
	return nil
}

// GetProp 读取单个系统属性。
func (c *Client) GetProp(ctx context.Context, serial, key string) string {
	out, err := c.Shell(ctx, serial, "getprop "+key)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// BatchGetProp 一次性读取多个属性，减少 adb 往返。
func (c *Client) BatchGetProp(ctx context.Context, serial string, keys []string) map[string]string {
	result := make(map[string]string, len(keys))
	if len(keys) == 0 {
		return result
	}
	// 用 `getprop` 全量输出再过滤，比逐条调用快得多
	out, err := c.Shell(ctx, serial, "getprop")
	if err != nil {
		return result
	}
	want := make(map[string]bool, len(keys))
	for _, k := range keys {
		want[k] = true
	}
	re := regexp.MustCompile(`^\[(.+?)\]: \[(.*)\]$`)
	for _, line := range strings.Split(out, "\n") {
		m := re.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		if want[m[1]] {
			result[m[1]] = m[2]
		}
	}
	return result
}

// Screenshot 截屏并保存到 localPath。
func (c *Client) Screenshot(ctx context.Context, serial, localPath string) error {
	remote := "/sdcard/droidforge_screenshot.png"
	if _, err := c.Shell(ctx, serial, "screencap -p "+remote); err != nil {
		return err
	}
	defer c.Shell(context.Background(), serial, "rm -f "+remote)
	return c.Pull(ctx, serial, remote, localPath, nil)
}

// AppInfo 描述一个已安装应用。
type AppInfo struct {
	Package   string `json:"package"`
	Label     string `json:"label"`
	Version   string `json:"version"`
	System    bool   `json:"system"`
	Enabled   bool   `json:"enabled"`
	APKPath   string `json:"apkPath"`
	SizeBytes int64  `json:"sizeBytes"`
}

// ListPackages 列出第三方（或全部）应用。
func (c *Client) ListPackages(ctx context.Context, serial string, includeSystem bool) ([]AppInfo, error) {
	flag := "-3"
	if includeSystem {
		flag = ""
	}
	cmd := "pm list packages -f"
	if flag != "" {
		cmd += " " + flag
	}
	out, err := c.Shell(ctx, serial, cmd)
	if err != nil {
		return nil, err
	}
	apps := make([]AppInfo, 0, 64)
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "package:") {
			continue
		}
		path, pkg, ok := splitPkgLine(line)
		if !ok {
			continue
		}
		apps = append(apps, AppInfo{
			Package: pkg,
			APKPath: path,
			System:  strings.HasPrefix(path, "/system") || strings.HasPrefix(path, "/product") || strings.HasPrefix(path, "/vendor"),
			Enabled: true,
		})
	}
	sort.SliceStable(apps, func(i, j int) bool { return apps[i].Package < apps[j].Package })
	return apps, nil
}

// FileBase 返回路径的文件名，供前端展示。
func FileBase(p string) string { return filepath.Base(p) }
