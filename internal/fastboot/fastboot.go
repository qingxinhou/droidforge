// Package fastboot 封装 fastboot 命令行能力。
package fastboot

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"droidforge/internal/runner"
)

// Partition 是 bootloader 视角下的一个分区。
//
// 与 device.Partition 的区别：这里只描述 fastboot 能看到的物理分区，
// 没有挂载点、文件系统这类运行期信息（那要先启动系统才有）。
type Partition struct {
	// Name 是可刷写的完整名，带槽位后缀（boot_a）。
	Name string `json:"name"`
	// Base 是去掉槽位后的基础名（boot），用于分组展示。
	Base string `json:"base"`
	// SizeBytes 为 -1 表示该分区没报容量。
	SizeBytes int64  `json:"sizeBytes"`
	Slot      string `json:"slot"`
	FSType    string `json:"fsType"`
	HasSlot   bool   `json:"hasSlot"`
	// Kind 为 physical（bootloader 看到的物理分区）或
	// logical（fastbootd 里映射出来的 super 子分区）。
	Kind string `json:"kind"`
}

// 分区种类。
const (
	KindPhysical = "physical"
	KindLogical  = "logical"
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

// GetVar 读取单个 fastboot 变量。
//
// 真机上输出有两种形态：
//
//	getvar all              →  "(bootloader)\tpartition-size:boot_a: 4000000\r\n"
//	getvar <单个变量>        →  "partition-size:boot_a: 4000000\r\n"
//
// 「变量不存在」有两种表现，都得认（真机实测）：
//
//	① 键照常回显、值为空：`partition-size:foo: `（MIUI 的 bootloader 就是这样）
//	② 打印 FAILED：`getvar:foo FAILED (remote: 'GetVar Variable not found')`
//
// 两种情况下 fastboot 的**退出码都是 0**。所以：
//   - 情形 ① 返回 ("", nil)：变量确实回了，只是设备没给值，
//     对 partition-size:* 而言就等于「没有这个分区」；
//   - 情形 ② 返回 error：这是协议层拒绝，不是「值为空」。
//
// 调用方判断分区是否存在时，请同时检查取值是否为空。
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
	text := res.Combined()

	// `getvar all` 本身就是批量查询，语义上不该走单变量路径。
	if name == "all" {
		return text, nil
	}
	if msg, failed := failureText(text); failed {
		return "", fmt.Errorf("读取 %s 失败：%s", name, msg)
	}
	vars := ParseVars(text)
	if v, ok := vars[name]; ok {
		return v, nil
	}
	if v, ok := lookupFold(vars, name); ok {
		return v, nil
	}
	return "", fmt.Errorf("设备没有变量 %s", name)
}

// Vars 一次性读取所有 fastboot 变量。
//
// 比逐个 GetVar 快一个数量级（每次 GetVar 都要重新枚举一次 USB 设备），
// 而且能顺带拿到 partition-size:* / partition-type:* 这些批量信息。
func (c *Client) Vars(ctx context.Context, serial string) (map[string]string, error) {
	args := []string{}
	if serial != "" {
		args = append(args, "-s", serial)
	}
	args = append(args, "getvar", "all")
	res, err := runner.Run(ctx, "", c.Bin, args...)
	if err != nil {
		return nil, err
	}
	text := res.Combined()
	if msg, failed := failureText(text); failed {
		return nil, fmt.Errorf("读取变量表失败：%s", msg)
	}
	vars := ParseVars(text)
	if len(vars) == 0 {
		return nil, errors.New("未解析到任何 fastboot 变量")
	}
	return vars, nil
}

// Partitions 从 getvar all 的变量表里还原分区列表。
//
// 数据来自三族键：partition-size:NAME、partition-type:NAME、has-slot:NAME。
// 注意 capacity 的坑：bootloader 报的是**裸十六进制**（`73ff7f8000`），
// 而 max-download-size 这类变量带 `0x` 前缀，两种都得认。
func (c *Client) Partitions(ctx context.Context, serial string) ([]Partition, error) {
	vars, err := c.Vars(ctx, serial)
	if err != nil {
		return nil, err
	}
	ps := PartitionsFromVars(vars)
	// bootloader 没有 is-logical 键，按当前模式兜底。
	// `is-userspace: yes` 表示当前是 fastbootd：它看到的是 super 里映射出来的
	// 逻辑分区（system_a 之类），而不是 bootloader 的物理分区表。
	fallback := KindPhysical
	if strings.EqualFold(strings.TrimSpace(vars["is-userspace"]), "yes") {
		fallback = KindLogical
	}
	for i := range ps {
		if ps[i].Kind == "" {
			ps[i].Kind = fallback
		}
	}
	return ps, nil
}

// PartitionsFromVars 把变量表转成分区列表，纯函数便于测试。
//
// 分区种类优先看 `is-logical:NAME`：fastbootd 会逐分区标注（真机上
// 106 个分区里 16 个是 yes），bootloader 没有这个键，由调用方兜底。
func PartitionsFromVars(vars map[string]string) []Partition {
	hasSlot := map[string]bool{}
	isLogical := map[string]bool{}
	for k, v := range vars {
		if base, ok := strings.CutPrefix(k, "has-slot:"); ok {
			hasSlot[base] = strings.EqualFold(strings.TrimSpace(v), "yes")
		}
		if name, ok := strings.CutPrefix(k, "is-logical:"); ok {
			isLogical[strings.TrimSpace(name)] = strings.EqualFold(strings.TrimSpace(v), "yes")
		}
	}

	type builder struct {
		Partition
		hasSignal bool
	}

	byName := map[string]*builder{}
	order := []string{}
	touch := func(full, base, slot string) *builder {
		if p, ok := byName[full]; ok {
			return p
		}
		p := &builder{Partition: Partition{Name: full, Base: base, Slot: slot, SizeBytes: -1}}
		byName[full] = p
		order = append(order, full)
		return p
	}

	for k, v := range vars {
		if rest, ok := strings.CutPrefix(k, "partition-size:"); ok {
			// 值为空表示设备没有这个分区（bootloader 对未知变量正是这么回的），
			// 不能凭空造一条 -1 容量的假记录。
			n, parsed := parseHexSize(v)
			if !parsed {
				continue
			}
			full := strings.TrimSpace(rest)
			base, slot := splitSlot(full, hasSlot)
			p := touch(full, base, slot)
			p.SizeBytes = n
			p.hasSignal = true
			continue
		}
		if rest, ok := strings.CutPrefix(k, "partition-type:"); ok {
			fs := strings.TrimSpace(v)
			if fs == "" {
				continue
			}
			full := strings.TrimSpace(rest)
			base, slot := splitSlot(full, hasSlot)
			p := touch(full, base, slot)
			p.hasSignal = true
			if fs != "raw" {
				p.FSType = fs
			}
		}
	}

	out := make([]Partition, 0, len(order))
	for _, full := range order {
		p := byName[full]
		if !p.hasSignal {
			continue
		}
		if hs, ok := hasSlot[p.Base]; ok {
			p.HasSlot = hs
		}
		if logical, ok := isLogical[p.Name]; ok {
			if logical {
				p.Kind = KindLogical
			} else {
				p.Kind = KindPhysical
			}
		}
		out = append(out, p.Partition)
	}
	sortPartitions(out)
	return out
}

// sortPartitions 按「基础名 + 槽位」排序，让同名分区的 a/b 相邻。
func sortPartitions(ps []Partition) {
	sort.SliceStable(ps, func(i, j int) bool {
		if ps[i].Base != ps[j].Base {
			return ps[i].Base < ps[j].Base
		}
		return ps[i].Slot < ps[j].Slot
	})
}

// splitSlot 把 `boot_a` 拆成 (`boot`, "a")。
// 只有基础名在 has-slot 表里且为 yes 时才拆，避免把 `foo_b` 这种
// 真名里带下划线的分区误判。
func splitSlot(name string, hasSlot map[string]bool) (string, string) {
	if len(name) < 3 || name[len(name)-2] != '_' {
		return name, ""
	}
	slot := name[len(name)-1:]
	if slot != "a" && slot != "b" {
		return name, ""
	}
	base := name[:len(name)-2]
	if yes, ok := hasSlot[base]; ok && yes {
		return base, slot
	}
	return name, ""
}

// parseHexSize 解析 fastboot 报的容量。
//
// bootloader 给的是不带前缀的十六进制（`4000000` = 64 MiB），
// 少数实现会带 `0x`；还有 `-` / 空值表示未知。
func parseHexSize(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return 0, false
	}
	s = strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X")
	n, err := strconv.ParseInt(s, 16, 64)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// ParseVars 把 `getvar all`（或单变量查询）的输出解析成键值表。
//
// 四条真机约束（两台形态实测）：
//  1. Windows 版 fastboot 输出 CRLF，行尾 \r 必须去掉；
//  2. `getvar all` 每行以 `(bootloader)\t` 开头；单变量查询没有前缀；
//  3. 键本身可能含冒号（`slot-retry-count:b: 7`、
//     `partition-size:boot_a: 4000000`），必须用**最后一个**冒号切分；
//  4. 冒号后**可能没有空格**：bootloader 给 `partition-size:boot_a: 4000000`，
//     而 fastbootd 给 `partition-size:boot_a:0x4000000`。两种都得认，
//     否则 fastbootd 下会解析出 106 个空值分区（真机踩过）。
func ParseVars(out string) map[string]string {
	vars := make(map[string]string)
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" {
			continue
		}
		if idx := strings.Index(line, "(bootloader)"); idx >= 0 {
			line = strings.TrimSpace(line[idx+len("(bootloader)"):])
		} else if isJunkLine(line) {
			continue
		}
		if line == "" {
			continue
		}

		// 用最后一个冒号切：键里带冒号是常态，值里带冒号才是异常
		idx := strings.LastIndex(line, ":")
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		if key == "" {
			continue
		}
		vars[key] = val
	}
	return vars
}

// isJunkLine 识别 `getvar all` 尾部混进来的非变量行。
func isJunkLine(line string) bool {
	switch {
	case strings.HasPrefix(line, "Finished."):
		return true
	case strings.HasPrefix(line, "all:"):
		return true
	case strings.Contains(line, "FAILED"):
		return true
	}
	return false
}

// failureText 提取 fastboot 的失败原因。
func failureText(out string) (string, bool) {
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if !strings.Contains(line, "FAILED") {
			continue
		}
		if i := strings.Index(line, "FAILED"); i >= 0 {
			msg := strings.TrimSpace(line[i+len("FAILED"):])
			msg = strings.Trim(msg, "()")
			if msg == "" {
				msg = line
			}
			return msg, true
		}
	}
	return "", false
}

func lookupFold(m map[string]string, key string) (string, bool) {
	for k, v := range m {
		if strings.EqualFold(k, key) {
			return v, true
		}
	}
	return "", false
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
	return flashError(res, "刷入 "+partition)
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
	return flashError(res, "擦除 "+partition)
}

// flashError 把一次 fastboot 写操作的结果收敛成人类可读的错误。
//
// 真机上失败长这样（stdout 全空，全部走 stderr）：
//
//	Warning: skip copying foo image avb footer (...)
//	Sending 'foo' (1024 KB)  OKAY [  0.032s]
//	Writing 'foo'            FAILED (remote: 'partition does not exist')
//	fastboot: error: Command failed
//
// 旧实现把整段输出塞进 error，UI 上会弹出一大坨进度噪声，
// 真正的失败原因被埋在中间。这里只取 FAILED 那一句。
func flashError(res *runner.Result, action string) error {
	if res.ExitCode == 0 && !strings.Contains(strings.ToLower(res.Combined()), "failed") {
		return nil
	}
	if msg, failed := failureText(res.Combined()); failed {
		return fmt.Errorf("%s失败：%s", action, msg)
	}
	text := strings.TrimSpace(res.Combined())
	if text == "" {
		return fmt.Errorf("%s失败（fastboot 退出码 %d）", action, res.ExitCode)
	}
	return fmt.Errorf("%s失败：%s", action, text)
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
