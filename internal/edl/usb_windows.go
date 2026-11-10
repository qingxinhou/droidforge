//go:build windows

package edl

import (
	"errors"
	"fmt"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

// GUID_DEVINTERFACE_USB_DEVICE，用于枚举所有 USB 设备接口。
var guidDevInterfaceUSB = GUID{
	Data1: 0xA5DCBF10,
	Data2: 0x6530,
	Data3: 0x11D2,
	Data4: [8]byte{0x90, 0x1F, 0x00, 0xC0, 0x4F, 0xB9, 0x51, 0xED},
}

// GUID 与 Windows 的 GUID 布局一致。
type GUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

type spDeviceInterfaceData struct {
	cbSize             uint32
	InterfaceClassGuid GUID
	Flags              uint32
	Reserved           uintptr
}

type usbInterfaceDescriptor struct {
	bLength            uint8
	bDescriptorType    uint8
	bInterfaceNumber   uint8
	bAlternateSetting  uint8
	bNumEndpoints      uint8
	bInterfaceClass    uint8
	bInterfaceSubClass uint8
	bInterfaceProtocol uint8
	iInterface         uint8
}

type winusbPipeInformation struct {
	PipeType          uint32
	PipeID            uint8
	MaximumPacketSize uint16
	Interval          uint8
}

const (
	digcfPresent         = 0x00000002
	digcfDeviceInterface = 0x00000010
	genericRead          = 0x80000000
	genericWrite         = 0x40000000
	fileShareRead        = 0x00000001
	fileShareWrite       = 0x00000002
	openExisting         = 3
	fileAttributeNormal  = 0x00000080
	errorNoMoreItems     = 259
	errorSemTimeout      = 121
	usbdPipeTypeBulk     = 2

	// WinUSB 的管道策略编号（WINUSB_PIPE_POLICY）。
	//
	// 注意 0x05 是 ALLOW_PARTIAL_READS 而不是 IGNORE_SHORT_PACKETS（后者是 0x04）：
	// 我们要的正是「有多少先返回多少」，所以用前者。写错过一次，别再改回去。
	pipeTransferTimeout     = 0x03
	pipeAllowPartialReads   = 0x05
	pipeMaximumTransferSize = 0x08

	// inPipeTimeout 要短：Firehose 的响应长度不定，靠短读累积，
	// 读超时返回 0 字节让上层继续等，而不是一直阻塞。
	inPipeTimeout = 500
	// outPipeTimeout 要够长：一整段 1 MB 的数据写下去可能要几十毫秒到几秒，
	// 卡太紧会在慢速 hub 上误判成写失败。
	outPipeTimeout = 5000
	// maxTransferSize 与 Firehose 的 MaxPayload 对齐。WinUSB 默认上限比这个小，
	// 不显式抬上去，一次性写 1 MB 会被驱动以参数错误直接拒绝。
	maxTransferSize = 1 << 20
)

var (
	modsetupapi = syscall.NewLazyDLL("setupapi.dll")
	modwinusb   = syscall.NewLazyDLL("winusb.dll")
	modkernel32 = syscall.NewLazyDLL("kernel32.dll")

	procSetupDiGetClassDevs             = modsetupapi.NewProc("SetupDiGetClassDevsW")
	procSetupDiEnumDeviceInterfaces     = modsetupapi.NewProc("SetupDiEnumDeviceInterfaces")
	procSetupDiGetDeviceInterfaceDetail = modsetupapi.NewProc("SetupDiGetDeviceInterfaceDetailW")
	procSetupDiDestroyDeviceInfoList    = modsetupapi.NewProc("SetupDiDestroyDeviceInfoList")

	procCreateFileW = modkernel32.NewProc("CreateFileW")
	procCloseHandle = modkernel32.NewProc("CloseHandle")

	procWinUsbInitialize     = modwinusb.NewProc("WinUsb_Initialize")
	procWinUsbFree           = modwinusb.NewProc("WinUsb_Free")
	procWinUsbQueryInterface = modwinusb.NewProc("WinUsb_QueryInterfaceSettings")
	procWinUsbQueryPipe      = modwinusb.NewProc("WinUsb_QueryPipe")
	procWinUsbSetPipePolicy  = modwinusb.NewProc("WinUsb_SetPipePolicy")
	procWinUsbWritePipe      = modwinusb.NewProc("WinUsb_WritePipe")
	procWinUsbReadPipe       = modwinusb.NewProc("WinUsb_ReadPipe")
)

// Device 是一个 9008 设备。
type Device struct {
	Path string `json:"path"`
	VID  uint16 `json:"vid"`
	PID  uint16 `json:"pid"`
	// Name 是给用户看的简短标识，比如 "VID_05C6&PID_9008"。
	Name string `json:"name"`
}

// IsEDL 判断是否是高通 EDL 设备。
func (d Device) IsEDL() bool {
	return d.VID == 0x05C6 && (d.PID == 0x9008 || d.PID == 0x900E)
}

// ListDevices 枚举当前系统的 USB 设备，并挑出 9008。
//
// 只返回能通过 WinUSB 打开的那部分。如果设备挂在「Qualcomm HS-USB QDLoader 9008」
// 串口驱动上，这里看不到它——需要先用 Zadig 换成 WinUSB。
func ListDevices() ([]Device, error) {
	h, _, err := procSetupDiGetClassDevs.Call(
		uintptr(unsafe.Pointer(&guidDevInterfaceUSB)), 0, 0,
		digcfPresent|digcfDeviceInterface)
	if h == uintptr(syscall.InvalidHandle) {
		return nil, fmt.Errorf("枚举设备失败：%v", err)
	}
	defer procSetupDiDestroyDeviceInfoList.Call(h)

	var out []Device
	for i := 0; ; i++ {
		var did spDeviceInterfaceData
		did.cbSize = uint32(unsafe.Sizeof(did))
		r, _, e := procSetupDiEnumDeviceInterfaces.Call(
			h, 0, uintptr(unsafe.Pointer(&guidDevInterfaceUSB)), uintptr(i),
			uintptr(unsafe.Pointer(&did)))
		if r == 0 {
			if errno, ok := e.(syscall.Errno); ok && errno == errorNoMoreItems {
				break
			}
			break
		}

		// 第一次调用只为了拿所需缓冲区大小。
		var required uint32
		procSetupDiGetDeviceInterfaceDetail.Call(
			h, uintptr(unsafe.Pointer(&did)), 0, 0,
			uintptr(unsafe.Pointer(&required)), 0)
		if required == 0 {
			continue
		}
		// 至少要装得下 cbSize（4 字节）与路径起始的 4 字节，否则下面取 &buf[4] 会越界。
		if required < 8 {
			continue
		}
		buf := make([]byte, required)
		// SP_DEVICE_INTERFACE_DETAIL_DATA_W.cbSize 在 64 位上是 8、32 位上是 6
		// （4 字节 DWORD + 2 字节 WCHAR，按指针宽度补齐），不是 12。填错会被
		// SetupDiGetDeviceInterfaceDetailW 判为参数无效，设备永远枚举不出来。
		*(*uint32)(unsafe.Pointer(&buf[0])) = uint32(4 + unsafe.Sizeof(uintptr(0))/2)
		r2, _, _ := procSetupDiGetDeviceInterfaceDetail.Call(
			h, uintptr(unsafe.Pointer(&did)), uintptr(unsafe.Pointer(&buf[0])),
			uintptr(required), uintptr(unsafe.Pointer(&required)), 0)
		if r2 == 0 {
			continue
		}
		path := utf16PtrToString((*uint16)(unsafe.Pointer(&buf[4])))
		if path == "" {
			continue
		}
		vid, pid := parseVIDPID(path)
		out = append(out, Device{
			Path: path,
			VID:  vid,
			PID:  pid,
			Name: fmt.Sprintf("VID_%04X&PID_%04X", vid, pid),
		})
	}
	return out, nil
}

// FindEDL 只返回 9008 设备。
func FindEDL() ([]Device, error) {
	all, err := ListDevices()
	if err != nil {
		return nil, err
	}
	var out []Device
	for _, d := range all {
		if d.IsEDL() {
			out = append(out, d)
		}
	}
	return out, nil
}

// parseVIDPID 从设备路径里取 VID/PID，形如 \\?\usb#vid_05c6&pid_9008#...
func parseVIDPID(p string) (uint16, uint16) {
	low := strings.ToLower(p)
	var vid, pid uint16
	if i := strings.Index(low, "vid_"); i >= 0 && i+8 <= len(low) {
		fmt.Sscanf(low[i+4:i+8], "%04x", &vid)
	}
	if i := strings.Index(low, "pid_"); i >= 0 && i+8 <= len(low) {
		fmt.Sscanf(low[i+4:i+8], "%04x", &pid)
	}
	return vid, pid
}

// UsbTransport 通过 WinUSB 的批量端点收发数据。
type UsbTransport struct {
	file    syscall.Handle
	usb     syscall.Handle
	inPipe  uint8
	outPipe uint8
}

// Open 打开设备并初始化 WinUSB。
//
// 句柄**不带 FILE_FLAG_OVERLAPPED**：这里全部走同步读写 + 管道超时策略。
// 带了这个标志却给 WinUsb_ReadPipe / WritePipe 传 NULL OVERLAPPED，
// 轻则报参数错误，重则「提前报告完成」——大块传输会静默截断，比直接失败更糟。
func Open(d Device) (*UsbTransport, error) {
	pathPtr, err := syscall.UTF16PtrFromString(d.Path)
	if err != nil {
		return nil, err
	}
	h, _, e := procCreateFileW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		genericRead|genericWrite,
		fileShareRead|fileShareWrite,
		0, openExisting, fileAttributeNormal, 0)
	if h == uintptr(syscall.InvalidHandle) {
		return nil, fmt.Errorf("打开设备失败：%v。若设备挂在 QDLoader 串口驱动上，请先用 Zadig 换成 WinUSB", e)
	}

	var usbHandle syscall.Handle
	r, _, e := procWinUsbInitialize.Call(h, uintptr(unsafe.Pointer(&usbHandle)))
	if r == 0 {
		procCloseHandle.Call(h)
		return nil, fmt.Errorf("WinUSB 初始化失败：%v（多为驱动不匹配）", e)
	}

	u := &UsbTransport{file: syscall.Handle(h), usb: usbHandle}
	if err := u.findPipes(); err != nil {
		u.Close()
		return nil, err
	}

	// 先抬上限，再设超时与短读策略。
	maxXfer := uint32(maxTransferSize)
	for _, pipe := range []uint8{u.inPipe, u.outPipe} {
		procWinUsbSetPipePolicy.Call(uintptr(u.usb), uintptr(pipe), pipeMaximumTransferSize,
			uintptr(unsafe.Sizeof(maxXfer)), uintptr(unsafe.Pointer(&maxXfer)))
	}

	inTimeout := uint32(inPipeTimeout)
	procWinUsbSetPipePolicy.Call(uintptr(u.usb), uintptr(u.inPipe), pipeTransferTimeout,
		uintptr(unsafe.Sizeof(inTimeout)), uintptr(unsafe.Pointer(&inTimeout)))
	allowPartial := uint8(1)
	procWinUsbSetPipePolicy.Call(uintptr(u.usb), uintptr(u.inPipe), pipeAllowPartialReads,
		uintptr(unsafe.Sizeof(allowPartial)), uintptr(unsafe.Pointer(&allowPartial)))

	outTimeout := uint32(outPipeTimeout)
	procWinUsbSetPipePolicy.Call(uintptr(u.usb), uintptr(u.outPipe), pipeTransferTimeout,
		uintptr(unsafe.Sizeof(outTimeout)), uintptr(unsafe.Pointer(&outTimeout)))
	return u, nil
}

// findPipes 找出第一个批量输入与批量输出端点。
func (u *UsbTransport) findPipes() error {
	var iface usbInterfaceDescriptor
	iface.bLength = uint8(unsafe.Sizeof(iface))
	r, _, e := procWinUsbQueryInterface.Call(uintptr(u.usb), 0, uintptr(unsafe.Pointer(&iface)))
	if r == 0 {
		return fmt.Errorf("查询接口描述符失败：%v", e)
	}
	for i := uint8(0); i < iface.bNumEndpoints; i++ {
		var pipe winusbPipeInformation
		r, _, _ := procWinUsbQueryPipe.Call(uintptr(u.usb), 0, uintptr(i), uintptr(unsafe.Pointer(&pipe)))
		if r == 0 {
			continue
		}
		if pipe.PipeType != usbdPipeTypeBulk {
			continue
		}
		// 端点地址最高位为 1 表示 IN。
		if pipe.PipeID&0x80 != 0 {
			if u.inPipe == 0 {
				u.inPipe = pipe.PipeID
			}
		} else if u.outPipe == 0 {
			u.outPipe = pipe.PipeID
		}
	}
	if u.inPipe == 0 || u.outPipe == 0 {
		return errors.New("设备没有可用的批量端点，多半是驱动装错了")
	}
	return nil
}

// Read 实现 io.Reader。超时返回 (0, nil)，让上层自行决定是否继续等。
func (u *UsbTransport) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	var read uint32
	r, _, e := procWinUsbReadPipe.Call(uintptr(u.usb), uintptr(u.inPipe),
		uintptr(unsafe.Pointer(&p[0])), uintptr(len(p)),
		uintptr(unsafe.Pointer(&read)), 0)
	if r == 0 {
		if errno, ok := e.(syscall.Errno); ok && errno == errorSemTimeout {
			time.Sleep(5 * time.Millisecond)
			return 0, nil
		}
		return 0, fmt.Errorf("读取失败：%v", e)
	}
	return int(read), nil
}

// Write 实现 io.Writer，保证把整块写完。
func (u *UsbTransport) Write(p []byte) (int, error) {
	total := 0
	for total < len(p) {
		var written uint32
		chunk := p[total:]
		r, _, e := procWinUsbWritePipe.Call(uintptr(u.usb), uintptr(u.outPipe),
			uintptr(unsafe.Pointer(&chunk[0])), uintptr(len(chunk)),
			uintptr(unsafe.Pointer(&written)), 0)
		if r == 0 {
			return total, fmt.Errorf("写入失败（已写 %d/%d 字节）：%v", total, len(p), e)
		}
		if written == 0 {
			return total, errors.New("写入返回 0 字节")
		}
		total += int(written)
	}
	return total, nil
}

// Close 释放句柄。
func (u *UsbTransport) Close() error {
	if u.usb != 0 {
		procWinUsbFree.Call(uintptr(u.usb))
		u.usb = 0
	}
	if u.file != 0 {
		procCloseHandle.Call(uintptr(u.file))
		u.file = 0
	}
	return nil
}

// utf16PtrToString 读一个以 0 结尾的 UTF-16 字符串。
//
// 不用 syscall.UTF16PtrToString：它在部分 Go 版本的 windows 构建里不可用。
func utf16PtrToString(p *uint16) string {
	if p == nil {
		return ""
	}
	var buf []uint16
	ptr := unsafe.Pointer(p)
	for i := 0; i < 32768; i++ {
		v := *(*uint16)(ptr)
		if v == 0 {
			break
		}
		buf = append(buf, v)
		ptr = unsafe.Pointer(uintptr(ptr) + 2)
	}
	return string(utf16.Decode(buf))
}
