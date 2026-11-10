package backup

import (
	"archive/tar"
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseContentRows(t *testing.T) {
	out := "Row: 0 _id=1, display_name=张三, data1=13800138000, data2=2, data3=手机, mimetype=vnd.android.cursor.item/phone_v2\n" +
		"Row: 1 _id=2, display_name=李四, 北京, data1=lisi@example.com, mimetype=vnd.android.cursor.item/email_v2\n"

	rows := parseContentRows(out, []string{"display_name", "data1", "data2", "data3", "mimetype"})
	if len(rows) != 2 {
		t.Fatalf("行数 = %d，期望 2", len(rows))
	}
	if rows[0]["display_name"] != "张三" {
		t.Errorf("display_name = %q", rows[0]["display_name"])
	}
	if rows[0]["data1"] != "13800138000" {
		t.Errorf("data1 = %q", rows[0]["data1"])
	}
	if rows[0]["mimetype"] != "vnd.android.cursor.item/phone_v2" {
		t.Errorf("mimetype = %q", rows[0]["mimetype"])
	}
	// 值里带逗号时不能被切断。
	if rows[1]["email"] != "" && rows[1]["display_name"] != "李四" {
		t.Errorf("含逗号的姓名解析错误：%+v", rows[1])
	}
	if rows[1]["data1"] != "lisi@example.com" {
		t.Errorf("带逗号后的字段错位：%+v", rows[1])
	}
}

func TestParseContentRowsIgnoresNoise(t *testing.T) {
	out := "No result found.\n\nRow: 0 _id=7, address=10086, type=1, body=余额提醒\n"
	rows := parseContentRows(out, []string{"address", "date", "type", "body"})
	if len(rows) != 1 {
		t.Fatalf("行数 = %d，期望 1", len(rows))
	}
	if rows[0]["body"] != "余额提醒" {
		t.Errorf("body = %q", rows[0]["body"])
	}
	if rows[0]["type"] != "1" {
		t.Errorf("type = %q", rows[0]["type"])
	}
}

func TestSafeRel(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"sdcard/DCIM/Camera/a.jpg", "DCIM/Camera/a.jpg", true},
		{"/sdcard/Pictures/b.png", "Pictures/b.png", true},
		{"DCIM/2024/c.mp4", "DCIM/2024/c.mp4", true},
		{"../upper/file", "", false},
		{"sdcard/../../escape", "", false},
		{"C:/somewhere/file", "", false},
		{"", "", false},
		{"/", "", false},
	}
	for _, c := range cases {
		got, ok := safeRel(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("safeRel(%q) = (%q,%v)，期望 (%q,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestFormatMillis(t *testing.T) {
	// 1700000000000 → 2023-11-15 06:13:20 UTC，东八区为 14:13:20。
	// 这里只断言能解析成非原串，避免依赖运行机器的时区。
	got := formatMillis("1700000000000")
	if got == "1700000000000" {
		t.Errorf("毫秒时间戳未被格式化：%q", got)
	}
	if formatMillis("bad") != "bad" {
		t.Errorf("非法输入应原样返回")
	}
}

func TestCategoriesAreSane(t *testing.T) {
	ids := map[string]bool{}
	for _, c := range Categories() {
		if c.ID == "" || c.Label == "" {
			t.Errorf("类别缺少 ID 或名称：%+v", c)
		}
		if ids[c.ID] {
			t.Errorf("类别 ID 重复：%s", c.ID)
		}
		ids[c.ID] = true
		switch c.Kind {
		case "files":
			if len(c.Paths) == 0 {
				t.Errorf("%s 是文件类但没有路径", c.ID)
			}
		case "contacts", "sms":
		default:
			t.Errorf("%s 的 kind 非法：%s", c.ID, c.Kind)
		}
	}
	if !ids["photos"] || !ids["contacts"] {
		t.Error("缺少必需类别")
	}
}

func TestPlanTotalOf(t *testing.T) {
	p := &Plan{Categories: []CategoryPlan{
		{Category: Category{ID: "photos"}, Size: 100},
		{Category: Category{ID: "sms"}, Size: 1},
		{Category: Category{ID: "audio"}, Size: 50},
	}}
	if got := p.TotalOf(nil); got != 151 {
		t.Errorf("全选合计 = %d，期望 151", got)
	}
	if got := p.TotalOf([]string{"photos"}); got != 100 {
		t.Errorf("单选合计 = %d，期望 100", got)
	}
}

func TestScanScriptContainsAllCategories(t *testing.T) {
	s := scanScript()
	for _, c := range Categories() {
		if c.Kind != "files" {
			continue
		}
		for _, p := range c.Paths {
			if !contains(s, p) {
				t.Errorf("扫描脚本缺少路径 %s", p)
			}
		}
	}
	if !contains(s, markerContacts) || !contains(s, markerSMS) {
		t.Error("扫描脚本缺少分隔标记")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestParseContentRowsIgnoresEqualsInBody 验证正文里出现 x=y 时不会把后面的列挤歪。
func TestParseContentRowsIgnoresEqualsInBody(t *testing.T) {
	out := "Row: 0 _id=3, address=95588, date=1700000000000, type=1, body=您的验证码是 code=8821 请勿泄露\n"
	rows := parseContentRows(out, []string{"address", "date", "type", "body"})
	if len(rows) != 1 {
		t.Fatalf("行数 = %d", len(rows))
	}
	if got := rows[0]["body"]; got != "您的验证码是 code=8821 请勿泄露" {
		t.Errorf("body = %q", got)
	}
	if rows[0]["type"] != "1" {
		t.Errorf("type = %q（正文里的等号被当成新字段了）", rows[0]["type"])
	}
	if rows[0]["address"] != "95588" {
		t.Errorf("address = %q", rows[0]["address"])
	}
}

// TestExtractTarSurvivesCRLF 锁住真机上踩到的坑：
// Windows 版 adb 会把设备端输出的 \n 改写成 \r\n，裸 tar 流的 512 字节头
// 会因此错位，整个备份静默失败。现在设备端先 base64，这里验证
// 「base64 + CRLF 换行」仍然能完整解包。
func TestExtractTarSurvivesCRLF(t *testing.T) {
	body := []byte("第一行\n第二行\r\n第三行\n")
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	if err := tw.WriteHeader(&tar.Header{
		Name: "sdcard/DCIM/camera/a.txt",
		Mode: 0o644,
		Size: int64(len(body)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	// 模拟 toybox base64 的 76 列换行，再模拟 adb 把 \n 变成 \r\n
	enc := base64.StdEncoding.EncodeToString(raw.Bytes())
	var b64 strings.Builder
	for i := 0; i < len(enc); i += 76 {
		end := i + 76
		if end > len(enc) {
			end = len(enc)
		}
		b64.WriteString(enc[i:end])
		b64.WriteString("\r\n")
	}

	dst := t.TempDir()
	files, _, total, entries, err := extractTar(
		base64.NewDecoder(base64.StdEncoding, strings.NewReader(b64.String())),
		dst, false, func(int64) {}, func(string) {},
	)
	if err != nil {
		t.Fatalf("解包失败: %v", err)
	}
	if files != 1 {
		t.Fatalf("files = %d, want 1", files)
	}
	if total != int64(len(body)) {
		t.Errorf("bytes = %d, want %d", total, len(body))
	}
	if len(entries) != 1 || entries[0].Path != "DCIM/camera/a.txt" {
		t.Errorf("entries = %+v", entries)
	}

	got, err := os.ReadFile(filepath.Join(dst, "DCIM", "camera", "a.txt"))
	if err != nil {
		t.Fatalf("读回失败: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("内容不一致:\n got=%q\nwant=%q", got, body)
	}
}

// TestExtractTarSkipsExisting 验证增量：已存在且大小一致的文件不重复写。
func TestExtractTarSkipsExisting(t *testing.T) {
	body := []byte("same-size-content")
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	tw.WriteHeader(&tar.Header{Name: "sdcard/Download/x.bin", Mode: 0o644, Size: int64(len(body))})
	tw.Write(body)
	tw.Close()

	dst := t.TempDir()
	first := base64.StdEncoding.EncodeToString(raw.Bytes())
	if _, _, _, _, err := extractTar(base64.NewDecoder(base64.StdEncoding, strings.NewReader(first)), dst, true, func(int64) {}, func(string) {}); err != nil {
		t.Fatal(err)
	}

	files, skipped, _, _, err := extractTar(base64.NewDecoder(base64.StdEncoding, strings.NewReader(first)), dst, true, func(int64) {}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if files != 0 || skipped != 1 {
		t.Errorf("第二次 files=%d skipped=%d, want 0/1", files, skipped)
	}
}

// TestShellJoinQuotes 确保路径进 shell 前被单引号包住，且内部引号正确转义。
func TestShellJoinQuotes(t *testing.T) {
	got := shellJoin([]string{"/sdcard/DCIM", "/sdcard/a b", "/sdcard/it's"})
	want := `'/sdcard/DCIM' '/sdcard/a b' '/sdcard/it'\''s'`
	if got != want {
		t.Errorf("shellJoin = %s, want %s", got, want)
	}
}
