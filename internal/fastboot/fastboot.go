// Package fastboot 封装 fastboot 命令行能力。
package fastboot

import (
	"context"
	"fmt"
	"strings"

	"droidforge/internal/runner"
)

// Client 是一次会话内的 fastboot 调用器。
type Client struct {
	Bin string
}

func New(bin string) *Client { return &Client{Bin: bin} }

// Devices 返回处于 fastboot / bootloader 模式的设备序列号。
func (c *Client) Devices(ctx context.Context) ([]string, error) {
	res, err := runner.Run(ctx, "", c.Bin, "devices")
	if err != nil {
		return nil, err
	}
	var out []string
	for _, line := range strings.Split(res.Stdout, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) >= 2 && fields[1] == "fastboot" {
			out = append(out, fields[0])
		}
	}
	return out, nil
}

// GetVar 读取 fastboot 变量。
func (c *Client) GetVar(ctx context.Context, serial, name string) (string, error) {
	args := []string{}
	if serial != "" {
		args = append(args, "-s", serial)
	}
	args = append(args, "getvar", name)
	res, err := runner.Run(ctx, "", c.Bin, args...)
	if err != nil {
		return "", err
	}
	// fastboot 把 getvar 结果写到 stderr，格式可能是 "name: value" 或 "(bootloader) name: value"
	text := res.Combined()
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if idx := strings.Index(line, name+":"); idx >= 0 {
			return strings.TrimSpace(line[idx+len(name)+1:]), nil
		}
	}
	return strings.TrimSpace(text), nil
}

// Reboot 重启设备："" / bootloader / recovery / fastboot / edl。
func (c *Client) Reboot(ctx context.Context, serial, target string) error {
	args := []string{}
	if serial != "" {
		args = append(args, "-s", serial)
	}
	switch target {
	case "bootloader", "recovery", "fastboot", "edl":
		args = append(args, "reboot-"+target)
	default:
		args = append(args, "reboot")
	}
	_, err := runner.MustRun(ctx, "", c.Bin, args...)
	return err
}

// Flash 刷入单个分区镜像。
func (c *Client) Flash(ctx context.Context, serial, partition, image string, onLine runner.LineHandler) error {
	args := []string{}
	if serial != "" {
		args = append(args, "-s", serial)
	}
	args = append(args, "flash", partition, image)
	res, err := runner.RunStream(ctx, "", onLine, c.Bin, args...)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 || strings.Contains(strings.ToLower(res.Combined()), "failed") {
		return fmt.Errorf("%s", strings.TrimSpace(res.Combined()))
	}
	return nil
}

// Erase 擦除分区。
func (c *Client) Erase(ctx context.Context, serial, partition string, onLine runner.LineHandler) error {
	args := []string{}
	if serial != "" {
		args = append(args, "-s", serial)
	}
	args = append(args, "erase", partition)
	res, err := runner.RunStream(ctx, "", onLine, c.Bin, args...)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 || strings.Contains(strings.ToLower(res.Combined()), "failed") {
		return fmt.Errorf("%s", strings.TrimSpace(res.Combined()))
	}
	return nil
}

// SetActive 切换 A/B 槽位。
func (c *Client) SetActive(ctx context.Context, serial, slot string) error {
	args := []string{}
	if serial != "" {
		args = append(args, "-s", serial)
	}
	args = append(args, "--set-active="+slot)
	_, err := runner.MustRun(ctx, "", c.Bin, args...)
	return err
}

// FlashPartition 是 Flash 的别名，语义更明确。
func (c *Client) FlashPartition(ctx context.Context, serial, partition, image string, onLine runner.LineHandler) error {
	return c.Flash(ctx, serial, partition, image, onLine)
}
