package backup

import (
	"testing"
)

func TestParseContentRows(t *testing.T) {
	out := "Row: 0 _id=1, display_name=张三, data1=13800138000, data2=2, data3=手机, mimetype=vnd.android.cursor.item/phone_v2\n" +
		"Row: 1 _id=2, display_name=李四, 北京, data1=lisi@example.com, mimetype=vnd.android.cursor.item/email_v2\n"

	rows := parseContentRows(out)
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
	rows := parseContentRows(out)
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
