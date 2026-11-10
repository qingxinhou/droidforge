package arb

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// buildImage 生成一段带 ARB 标记的假引导镜像。
func buildImage(t *testing.T, markers []string, value uint32) string {
	t.Helper()
	buf := make([]byte, 8192)
	for i := range buf {
		buf[i] = byte(i % 97)
	}
	pos := 1024
	for _, m := range markers {
		if pos+len(m)+8 > len(buf) {
			t.Fatal("测试镜像太小")
		}
		copy(buf[pos:], m)
		pos += len(m)
		// 标记后 8 字节写值（跳过 4 字节，让扫描落在对齐位置）。
		binary.LittleEndian.PutUint32(buf[pos+4:], value)
		pos += 512
	}
	p := filepath.Join(t.TempDir(), "xbl.elf")
	if err := os.WriteFile(p, buf, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestScanFindsValue(t *testing.T) {
	p := buildImage(t, []string{"anti-rollback"}, 5)
	rep, err := ScanFile(p)
	if err != nil {
		t.Fatalf("扫描失败：%v", err)
	}
	if len(rep.Hits) != 1 {
		t.Fatalf("命中数 = %d，期望 1（%+v）", len(rep.Hits), rep.Hits)
	}
	if rep.Hits[0].Value != 5 {
		t.Errorf("值 = %d，期望 5", rep.Hits[0].Value)
	}
	if rep.Confident != "medium" {
		t.Errorf("单点命中的置信度 = %q，期望 medium", rep.Confident)
	}
	if len(rep.Values) != 1 || rep.Values[0] != 5 {
		t.Errorf("候选值集合 = %v", rep.Values)
	}
}

func TestScanMultipleMarkersSameValue(t *testing.T) {
	p := buildImage(t, []string{"anti-rollback", "rollback_index", "AntiRollback"}, 3)
	rep, err := ScanFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Hits) != 3 {
		t.Fatalf("命中数 = %d，期望 3", len(rep.Hits))
	}
	if rep.Confident != "high" {
		t.Errorf("置信度 = %q，期望 high", rep.Confident)
	}
	if rep.Values[0] != 3 {
		t.Errorf("候选值 = %v", rep.Values)
	}
}

func TestScanNoMarker(t *testing.T) {
	buf := make([]byte, 4096)
	for i := range buf {
		buf[i] = 0x90
	}
	p := filepath.Join(t.TempDir(), "blank.bin")
	if err := os.WriteFile(p, buf, 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := ScanFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Hits) != 0 {
		t.Errorf("不该有命中：%+v", rep.Hits)
	}
	if rep.Confident != "none" {
		t.Errorf("置信度 = %q，期望 none", rep.Confident)
	}
	if rep.Note == "" {
		t.Error("无命中时应当给出解释")
	}
}

func TestScanUTF16Marker(t *testing.T) {
	buf := make([]byte, 4096)
	pos := 512
	for _, r := range "anti-rollback" {
		buf[pos] = byte(r)
		buf[pos+1] = 0
		pos += 2
	}
	binary.LittleEndian.PutUint32(buf[pos+4:], 12)
	p := filepath.Join(t.TempDir(), "uefi.bin")
	if err := os.WriteFile(p, buf, 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := ScanFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Hits) != 1 {
		t.Fatalf("UTF-16 标记未命中：%+v", rep.Hits)
	}
	if rep.Hits[0].Encoding != "utf16le" || rep.Hits[0].Value != 12 {
		t.Errorf("命中内容不对：%+v", rep.Hits[0])
	}
}

func TestScanIgnoresHugeValues(t *testing.T) {
	// 标记后面写的不是小整数（比如指针或长度），不应被当成 ARB 版本。
	buf := make([]byte, 4096)
	copy(buf[256:], "rollback_index")
	binary.LittleEndian.PutUint32(buf[256+14+4:], 0xdeadbeef)
	p := filepath.Join(t.TempDir(), "noise.bin")
	if err := os.WriteFile(p, buf, 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := ScanFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Hits) != 0 {
		t.Errorf("0xdeadbeef 不该算作 ARB 版本：%+v", rep.Hits)
	}
}

func TestCompare(t *testing.T) {
	cases := []struct {
		current, target int64
		want            string
	}{
		{-1, 5, "unknown"},
		{5, -1, "unknown"},
		{5, 5, "ok"},
		{5, 4, "ok"},
		{5, 6, "warn"},
	}
	for _, c := range cases {
		v := Compare(c.current, c.target)
		if v.Level != c.want {
			t.Errorf("Compare(%d,%d).Level = %q，期望 %q", c.current, c.target, v.Level, c.want)
		}
		if v.Title == "" || v.Detail == "" {
			t.Errorf("Compare(%d,%d) 文案不完整：%+v", c.current, c.target, v)
		}
	}
}
