//go:build !windows

package edl

import (
	"errors"
	"runtime"
)

// Device 在非 Windows 平台只是一个占位，避免调用方 import 时报错。
type Device struct {
	Path string `json:"path"`
	VID  uint16 `json:"vid"`
	PID  uint16 `json:"pid"`
	Name string `json:"name"`
}

// IsEDL 判断是否是高通 EDL 设备。
func (d Device) IsEDL() bool {
	return d.VID == 0x05C6 && (d.PID == 0x9008 || d.PID == 0x900E)
}

var errUnsupported = errors.New("EDL 直连目前只实现了 Windows 的 WinUSB 后端（" + runtime.GOOS + " 暂不支持）")

// ListDevices 在非 Windows 平台直接返回不支持。
func ListDevices() ([]Device, error) { return nil, errUnsupported }

// FindEDL 同上。
func FindEDL() ([]Device, error) { return nil, errUnsupported }

// UsbTransport 在非 Windows 平台不可用。
type UsbTransport struct{}

// Open 在非 Windows 平台直接返回不支持。
func Open(d Device) (*UsbTransport, error) { return nil, errUnsupported }

// Read 未实现。
func (u *UsbTransport) Read(p []byte) (int, error) { return 0, errUnsupported }

// Write 未实现。
func (u *UsbTransport) Write(p []byte) (int, error) { return 0, errUnsupported }

// Close 未实现。
func (u *UsbTransport) Close() error { return nil }
