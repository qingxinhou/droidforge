package edl

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// 纯协议
// ---------------------------------------------------------------------------

func TestParseFHResponseACK(t *testing.T) {
	raw := "<?xml version=\"1.0\" ?><data><response value=\"ACK\" rawmode=\"false\" /></data>"
	r := ParseFHResponse(raw)
	if !r.OK() {
		t.Errorf("应当识别为 ACK：%+v", r)
	}
}

func TestParseFHResponseNAK(t *testing.T) {
	r := ParseFHResponse(`<response value="NAK" rawmode="false" /></data>`)
	if r.OK() {
		t.Error("NAK 不该被当成成功")
	}
	if r.Value != "NAK" {
		t.Errorf("value = %q", r.Value)
	}
}

func TestParseFHResponseWithLogs(t *testing.T) {
	raw := "<log value=\"INFO: Calling handler for configure\" />\n" +
		"<log value=\"INFO: Storage type set to UFS\" />\n" +
		"<response value=\"ACK\" rawmode=\"false\" />"
	r := ParseFHResponse(raw)
	if len(r.Logs) != 2 {
		t.Errorf("日志条数 = %d，期望 2", len(r.Logs))
	}
	if !r.OK() {
		t.Error("混合日志与响应时应判定为 ACK")
	}
}

func TestConfigureXML(t *testing.T) {
	x := Configure(ConfigureOptions{MemoryName: "emmc", MaxPayload: 512 << 10, ZlpAwareHost: true})
	for _, want := range []string{`MemoryName="emmc"`, `MaxPayloadSizeToTargetInBytes="524288"`, `ZlpAwareHost="1"`} {
		if !strings.Contains(x, want) {
			t.Errorf("configure 缺少 %s：%s", want, x)
		}
	}
}

func TestProgramXMLEscaping(t *testing.T) {
	x := Program(ProgramRequest{
		Label:       `sys"tem`,
		Filename:    "droidforge.bin",
		StartSector: 2048,
		Sectors:     100,
		SectorSize:  4096,
		LUN:         2,
	})
	if !strings.Contains(x, `label="sys&quot;tem"`) {
		t.Errorf("引号未转义：%s", x)
	}
	if !strings.Contains(x, `start_sector="2048"`) {
		t.Errorf("起始扇区不对：%s", x)
	}
	if !strings.Contains(x, `physical_partition_number="2"`) {
		t.Errorf("LUN 不对：%s", x)
	}
}

func TestReadAndEraseXML(t *testing.T) {
	r := Read(ReadRequest{Label: "boot_a", StartSector: 10, Sectors: 5, SectorSize: 4096})
	if !strings.Contains(r, "<read ") || !strings.Contains(r, `num_partition_sectors="5"`) {
		t.Errorf("read 请求格式不对：%s", r)
	}
	e := Erase(EraseRequest{Label: "userdata", StartSector: 1, Sectors: 9})
	if !strings.Contains(e, "<erase ") || !strings.Contains(e, `label="userdata"`) {
		t.Errorf("erase 请求格式不对：%s", e)
	}
	if !strings.Contains(Reset(), `value="reset"`) {
		t.Error("reset 请求格式不对")
	}
}

func TestEncodeDecodeHello(t *testing.T) {
	h := &HelloInfo{Version: 2, VersionMin: 1, MaxPacketSize: 16384, Mode: 0}
	raw := EncodeHelloResp(h, SaharaModeImageTxPending)
	if len(raw) != helloRespLen {
		t.Fatalf("Hello Response 长度 = %d，期望 %d", len(raw), helloRespLen)
	}
	cmd, length, err := ParsePacketHeader(raw)
	if err != nil || cmd != SaharaHelloResp || length != helloRespLen {
		t.Errorf("头解析错误：cmd=%d len=%d err=%v", cmd, length, err)
	}
}

func TestParseHelloRejectsWrongCommand(t *testing.T) {
	b := make([]byte, helloLen)
	binary.LittleEndian.PutUint32(b[0:], SaharaReadData)
	binary.LittleEndian.PutUint32(b[4:], helloLen)
	if _, err := ParseHello(b); err == nil {
		t.Error("命令字不对时应当报错")
	}
}

func TestInspectLoader(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name string
		want string
	}{
		{"prog_firehose_ddr.elf", "ufs"},
		{"prog_emmc_firehose_8953.mbn", "emmc"},
		{"prog_firehose_lite.elf", "ufs"},
	}
	for _, c := range cases {
		p := filepath.Join(dir, c.name)
		if err := os.WriteFile(p, make([]byte, 2048), 0o644); err != nil {
			t.Fatal(err)
		}
		info, err := InspectLoader(p)
		if err != nil {
			t.Fatalf("%s：%v", c.name, err)
		}
		if !info.Valid {
			t.Errorf("%s 应当有效：%s", c.name, info.Note)
		}
		if info.Memory != c.want {
			t.Errorf("%s 的存储类型 = %q，期望 %q", c.name, info.Memory, c.want)
		}
	}
}

func TestParseSector(t *testing.T) {
	if v, err := ParseSector("0x10"); err != nil || v != 16 {
		t.Errorf("十六进制解析失败：%d %v", v, err)
	}
	if v, err := ParseSector(" 2048 "); err != nil || v != 2048 {
		t.Errorf("十进制解析失败：%d %v", v, err)
	}
	if _, err := ParseSector(""); err == nil {
		t.Error("空串应当报错")
	}
}

// ---------------------------------------------------------------------------
// Sahara 会话（net.Pipe 模拟设备）
// ---------------------------------------------------------------------------

// fakeSahara 模拟设备侧的 Sahara 状态机，收到的镜像数据写进返回的 buffer。
func fakeSahara(conn net.Conn, maxPacket uint32) *bytes.Buffer {
	received := &bytes.Buffer{}
	go func() {
		defer conn.Close()

		hello := make([]byte, helloLen)
		binary.LittleEndian.PutUint32(hello[0:], SaharaHello)
		binary.LittleEndian.PutUint32(hello[4:], helloLen)
		binary.LittleEndian.PutUint32(hello[8:], 2)
		binary.LittleEndian.PutUint32(hello[12:], 1)
		binary.LittleEndian.PutUint32(hello[16:], maxPacket)
		if _, err := conn.Write(hello); err != nil {
			return
		}

		buf := make([]byte, helloRespLen)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return
		}

		for {
			hdr := make([]byte, packetHdrLen)
			if _, err := io.ReadFull(conn, hdr); err != nil {
				return
			}
			cmd, length, err := ParsePacketHeader(hdr)
			if err != nil {
				return
			}
			body := make([]byte, 0)
			if length > packetHdrLen {
				body = make([]byte, length-packetHdrLen)
				if _, err := io.ReadFull(conn, body); err != nil {
					return
				}
			}
			switch cmd {
			case SaharaReadData:
				imageID := binary.LittleEndian.Uint32(body[0:])
				offset := binary.LittleEndian.Uint32(body[4:])
				n := binary.LittleEndian.Uint32(body[8:])
				resp := make([]byte, readDataLen)
				binary.LittleEndian.PutUint32(resp[0:], SaharaReadData)
				binary.LittleEndian.PutUint32(resp[4:], readDataLen)
				binary.LittleEndian.PutUint32(resp[8:], imageID)
				binary.LittleEndian.PutUint32(resp[12:], offset)
				binary.LittleEndian.PutUint32(resp[16:], n)
				if _, err := conn.Write(resp); err != nil {
					return
				}
				chunk := make([]byte, n)
				if _, err := io.ReadFull(conn, chunk); err != nil {
					return
				}
				received.Write(chunk)
			case SaharaEndOfImage:
				done := make([]byte, doneRespLen)
				binary.LittleEndian.PutUint32(done[0:], SaharaDone)
				binary.LittleEndian.PutUint32(done[4:], doneRespLen)
				binary.LittleEndian.PutUint32(done[8:], 0) // status
				if _, err := conn.Write(done); err != nil {
					return
				}
				// 等 host 回 DoneResp（完整 12 字节）
				resp := make([]byte, doneRespLen)
				if _, err := io.ReadFull(conn, resp); err != nil {
					return
				}
				return
			default:
				return
			}
		}
	}()
	return received
}

func TestUploadLoaderTransfersAllBytes(t *testing.T) {
	host, dev := net.Pipe()
	loader := bytes.Repeat([]byte{0xAB}, 10000) // 跨越多个包
	received := fakeSahara(dev, 4096)

	done := make(chan error, 1)
	go func() {
		_, err := UploadLoader(host, loader, ImageIDFlashProgrammer, nil)
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("加载失败：%v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("超时，可能死锁")
	}

	if received.Len() != len(loader) {
		t.Fatalf("设备收到 %d 字节，期望 %d", received.Len(), len(loader))
	}
	if !bytes.Equal(received.Bytes(), loader) {
		t.Error("收到的内容与原始镜像不一致")
	}
}

func TestUploadLoaderRejectsEmpty(t *testing.T) {
	host, dev := net.Pipe()
	go func() { dev.Close() }()
	if _, err := UploadLoader(host, nil, 13, nil); err == nil {
		t.Error("空 loader 应当报错")
	}
}

func TestUploadLoaderWrongCommand(t *testing.T) {
	host, dev := net.Pipe()
	go func() {
		defer dev.Close()
		pkt := make([]byte, helloLen)
		binary.LittleEndian.PutUint32(pkt[0:], SaharaReadData) // 故意发错
		binary.LittleEndian.PutUint32(pkt[4:], helloLen)
		dev.Write(pkt)
		io.Copy(io.Discard, dev)
	}()
	if _, err := UploadLoader(host, []byte{1, 2, 3}, 13, nil); err == nil {
		t.Error("命令字不对时应当报错")
	}
}

// ---------------------------------------------------------------------------
// Firehose 会话
// ---------------------------------------------------------------------------

// readUntil 逐字节读到 marker 结束，避免 bufio 预读吃掉后续的二进制数据。
func readUntil(r io.Reader, marker string) (string, error) {
	var sb strings.Builder
	buf := make([]byte, 1)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			sb.Write(buf[:n])
			if strings.HasSuffix(sb.String(), marker) {
				return sb.String(), nil
			}
		}
		if err != nil {
			return sb.String(), err
		}
		if sb.Len() > 1<<20 {
			return sb.String(), io.ErrUnexpectedEOF
		}
	}
}

func TestFirehoseConfigureACK(t *testing.T) {
	host, dev := net.Pipe()
	go func() {
		defer dev.Close()
		if _, err := readUntil(dev, "</data>"); err != nil {
			return
		}
		dev.Write([]byte(`<response value="ACK" rawmode="false" />`))
	}()

	f := NewFirehose(host, nil)
	f.Timeout = 5 * time.Second
	if err := f.Configure(context.Background(), ConfigureOptions{MemoryName: "ufs"}); err != nil {
		t.Fatalf("configure 应当成功：%v", err)
	}
}

func TestFirehoseConfigureNAK(t *testing.T) {
	host, dev := net.Pipe()
	go func() {
		defer dev.Close()
		if _, err := readUntil(dev, "</data>"); err != nil {
			return
		}
		dev.Write([]byte(`<response value="NAK" rawmode="false" />`))
	}()

	f := NewFirehose(host, nil)
	f.Timeout = 5 * time.Second
	err := f.Configure(context.Background(), ConfigureOptions{})
	if err == nil {
		t.Fatal("NAK 应当返回错误")
	}
	if !strings.Contains(err.Error(), "NAK") {
		t.Errorf("错误信息里应包含 NAK：%v", err)
	}
}

func TestFirehoseWritePartition(t *testing.T) {
	host, dev := net.Pipe()
	const sectorSize = 4096
	payload := bytes.Repeat([]byte{0x5A}, 2*sectorSize)
	got := &bytes.Buffer{}

	go func() {
		defer dev.Close()
		for i := 0; i < 2; i++ { // 两次 program（每轮 1 个扇区，因为 MaxPayload 被设成 4096）
			cmd, err := readUntil(dev, "</data>")
			if err != nil {
				return
			}
			if !strings.Contains(cmd, "<program") {
				return
			}
			dev.Write([]byte(`<response value="ACK" rawmode="false" />`))
			chunk := make([]byte, sectorSize)
			if _, err := io.ReadFull(dev, chunk); err != nil {
				return
			}
			got.Write(chunk)
			dev.Write([]byte(`<response value="ACK" rawmode="false" />`))
		}
	}()

	f := NewFirehose(host, nil)
	f.MaxPayload = sectorSize
	f.Timeout = 5 * time.Second

	var written int64
	err := f.WritePartition(context.Background(), "boot_a", bytes.NewReader(payload), WriteOptions{
		SectorSize:  sectorSize,
		LUN:         0,
		StartSector: 100,
		Sectors:     2,
	}, func(n int64) { written = n })
	if err != nil {
		t.Fatalf("写入失败：%v", err)
	}
	if written != int64(len(payload)) {
		t.Errorf("进度 = %d，期望 %d", written, len(payload))
	}
	if !bytes.Equal(got.Bytes(), payload) {
		t.Errorf("设备收到 %d 字节，内容不一致", got.Len())
	}
}

func TestFirehoseReadPartition(t *testing.T) {
	host, dev := net.Pipe()
	const sectorSize = 4096
	content := bytes.Repeat([]byte{0x3C}, sectorSize)

	go func() {
		defer dev.Close()
		if _, err := readUntil(dev, "</data>"); err != nil {
			return
		}
		dev.Write([]byte(`<response value="ACK" rawmode="false" />`))
		dev.Write(content)
		dev.Write([]byte(`<response value="ACK" rawmode="false" />`))
	}()

	f := NewFirehose(host, nil)
	f.MaxPayload = sectorSize
	f.Timeout = 5 * time.Second

	out := &bytes.Buffer{}
	err := f.ReadPartition(context.Background(), "boot_a", out, WriteOptions{
		SectorSize: sectorSize,
		Sectors:    1,
	}, nil)
	if err != nil {
		t.Fatalf("读取失败：%v", err)
	}
	if !bytes.Equal(out.Bytes(), content) {
		t.Errorf("读到的内容不一致：%d 字节", out.Len())
	}
}

func TestFirehoseRespectsContext(t *testing.T) {
	host, _ := net.Pipe()
	f := NewFirehose(host, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := f.Send(ctx, Reset()); err == nil {
		t.Error("已取消的上下文应当直接返回错误")
	}
}

// ---------------------------------------------------------------------------
// GPT
// ---------------------------------------------------------------------------

func buildGPT(t *testing.T, sectorSize int, names []string) []byte {
	t.Helper()
	const entryCount = 8
	const entrySize = 128
	data := make([]byte, 3*sectorSize+entryCount*entrySize)

	// 分区表项
	entries := data[2*sectorSize : 2*sectorSize+entryCount*entrySize]
	for i, name := range names {
		e := entries[i*entrySize : (i+1)*entrySize]
		// type GUID：随便造一个非零值
		e[0] = 0x28
		e[1] = 0x73
		e[2] = 0x2A
		e[3] = 0xC1
		binary.LittleEndian.PutUint64(e[32:], uint64(2048*(i+1)))
		binary.LittleEndian.PutUint64(e[40:], uint64(2048*(i+1)+2047))
		binary.LittleEndian.PutUint64(e[48:], 0)
		for j, r := range name {
			if j >= 36 {
				break
			}
			binary.LittleEndian.PutUint16(e[56+j*2:], uint16(r))
		}
	}

	// header
	hdr := data[sectorSize : sectorSize+512]
	copy(hdr, "EFI PART")
	binary.LittleEndian.PutUint32(hdr[8:], 0x00010000)
	binary.LittleEndian.PutUint32(hdr[12:], 92)
	binary.LittleEndian.PutUint64(hdr[24:], 1)   // current LBA
	binary.LittleEndian.PutUint64(hdr[32:], 100) // backup LBA
	binary.LittleEndian.PutUint64(hdr[40:], 34)  // first usable
	binary.LittleEndian.PutUint64(hdr[48:], 999) // last usable
	binary.LittleEndian.PutUint64(hdr[72:], 2)   // entries LBA
	binary.LittleEndian.PutUint32(hdr[80:], entryCount)
	binary.LittleEndian.PutUint32(hdr[84:], entrySize)
	binary.LittleEndian.PutUint32(hdr[88:], crc32IEEE(entries))

	tmp := make([]byte, 92)
	copy(tmp, hdr[:92])
	binary.LittleEndian.PutUint32(tmp[16:], 0)
	binary.LittleEndian.PutUint32(hdr[16:], crc32IEEE(tmp))
	return data
}

func TestParseGPT(t *testing.T) {
	const sectorSize = 512
	data := buildGPT(t, sectorSize, []string{"boot_a", "system_a", "userdata"})

	g, err := ParseGPT(data, sectorSize, 0)
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if len(g.Partitions) != 3 {
		t.Fatalf("分区数 = %d，期望 3", len(g.Partitions))
	}
	if !g.ChecksumOK {
		t.Errorf("CRC 应当通过：%s", g.Note)
	}
	p, ok := g.Find("boot_a")
	if !ok {
		t.Fatal("没找到 boot_a")
	}
	if p.Sectors != 2048 {
		t.Errorf("扇区数 = %d，期望 2048", p.Sectors)
	}
	if p.ByteSize(sectorSize) != 2048*512 {
		t.Errorf("字节数计算错误")
	}
	if _, ok := g.Find("BOOT_A"); !ok {
		t.Error("查找应当忽略大小写")
	}
}

func TestParseGPTBadCRC(t *testing.T) {
	const sectorSize = 512
	data := buildGPT(t, sectorSize, []string{"boot"})
	// 篡改分区名，让表项 CRC 对不上
	data[2*sectorSize+56] = 'X'
	g, err := ParseGPT(data, sectorSize, 0)
	if err != nil {
		t.Fatalf("CRC 不符时仍应解析出结构：%v", err)
	}
	if g.Note == "" {
		t.Error("CRC 不符应当写进备注")
	}
}

func TestParseGPTNoSignature(t *testing.T) {
	data := make([]byte, 4096)
	if _, err := ParseGPT(data, 512, 0); err == nil {
		t.Error("没有 GPT 签名时应当报错")
	}
}

func TestParseGPTTooSmall(t *testing.T) {
	if _, err := ParseGPT(make([]byte, 100), 512, 0); err == nil {
		t.Error("数据太短时应当报错")
	}
}
