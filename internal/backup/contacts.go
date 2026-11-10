package backup

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// content query 的输出形如：
//
//	Row: 0 _id=1, display_name=张三, data1=13800138000, mimetype=vnd.android.cursor.item/phone_v2
//
// 值里可能自带逗号，直接按 ", " 切会错位，所以先定位每个 key 的起点，
// 再把「当前 key 等号之后 到 下一个 key 之前」整段当作值。
var (
	rowPrefixRe = regexp.MustCompile(`^Row:\s*\d+\s+`)
	keyRe       = regexp.MustCompile(`([A-Za-z0-9_]+)=`)
)

func parseContentRows(out string) []map[string]string {
	var rows []map[string]string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if !strings.HasPrefix(line, "Row:") {
			continue
		}
		body := rowPrefixRe.ReplaceAllString(line, "")
		locs := keyRe.FindAllStringSubmatchIndex(body, -1)
		if len(locs) == 0 {
			continue
		}
		m := make(map[string]string, len(locs))
		for i, loc := range locs {
			key := body[loc[2]:loc[3]]
			start := loc[1]
			end := len(body)
			if i+1 < len(locs) {
				end = locs[i+1][0]
			}
			val := strings.TrimSpace(body[start:end])
			val = strings.TrimSuffix(val, ",")
			m[key] = strings.TrimSpace(val)
		}
		rows = append(rows, m)
	}
	return rows
}

// 通讯录字段的 mimetype 到中文名的映射。
var contactKinds = map[string]string{
	"vnd.android.cursor.item/phone_v2":          "电话",
	"vnd.android.cursor.item/email_v2":          "邮箱",
	"vnd.android.cursor.item/postal-address_v2": "地址",
	"vnd.android.cursor.item/organization":      "单位",
	"vnd.android.cursor.item/note":              "备注",
	"vnd.android.cursor.item/website":           "网站",
	"vnd.android.cursor.item/im":                "即时通讯",
	"vnd.android.cursor.item/nickname":          "昵称",
	"vnd.android.cursor.item/contact_event":     "纪念日",
	"vnd.android.cursor.item/relation":          "关系",
	"vnd.android.cursor.item/sip_address":       "SIP",
	"vnd.android.cursor.item/name":              "姓名",
	"vnd.android.cursor.item/photo":             "头像",
	"vnd.android.cursor.item/group_membership":  "群组",
}

// exportContacts 导出通讯录为 CSV。
func exportContacts(ctx context.Context, bin, serial, dst string) (string, error) {
	query := "content query --uri content://com.android.contacts/data " +
		"--projection display_name:data1:data2:data3:mimetype 2>/dev/null"
	out, err := shell(ctx, bin, serial, query)
	if err != nil {
		return "", err
	}
	rows := parseContentRows(out)
	if len(rows) == 0 {
		return "", fmt.Errorf("没有读到任何联系人（可能是系统限制了 provider 访问）")
	}

	// 按姓名归组，姓名相同的多条记录挨在一起。
	sort.SliceStable(rows, func(i, j int) bool {
		ni := strings.ToLower(rows[i]["display_name"])
		nj := strings.ToLower(rows[j]["display_name"])
		if ni != nj {
			return ni < nj
		}
		return rows[i]["mimetype"] < rows[j]["mimetype"]
	})

	if err := os.MkdirAll(dst, 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(dst, "contacts.csv")
	f, err := os.Create(p)
	if err != nil {
		return "", err
	}
	defer f.Close()

	w := csv.NewWriter(csvWriter(f))
	_ = w.Write([]string{"姓名", "类型", "值", "子类型", "标签"})
	for _, r := range rows {
		name := r["display_name"]
		kind := contactKinds[r["mimetype"]]
		if kind == "" {
			kind = r["mimetype"]
		}
		// 头像是一坨二进制，导出没有意义。
		if r["mimetype"] == "vnd.android.cursor.item/photo" {
			continue
		}
		_ = w.Write([]string{name, kind, r["data1"], r["data2"], r["data3"]})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return "", err
	}
	return p, nil
}

var smsDirections = map[string]string{
	"1": "接收",
	"2": "发送",
	"3": "草稿",
	"4": "发件箱",
	"5": "发送失败",
	"6": "待发送",
}

// exportSMS 导出短信为 CSV。
func exportSMS(ctx context.Context, bin, serial, dst string) (string, error) {
	query := "content query --uri content://sms --projection address:date:type:body 2>/dev/null"
	out, err := shell(ctx, bin, serial, query)
	if err != nil {
		return "", err
	}
	rows := parseContentRows(out)
	if len(rows) == 0 {
		return "", fmt.Errorf("没有读到任何短信（可能是纯数据机型，或系统限制了 provider 访问）")
	}

	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i]["date"] < rows[j]["date"]
	})

	if err := os.MkdirAll(dst, 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(dst, "sms.csv")
	f, err := os.Create(p)
	if err != nil {
		return "", err
	}
	defer f.Close()

	w := csv.NewWriter(csvWriter(f))
	_ = w.Write([]string{"号码", "时间", "方向", "正文"})
	for _, r := range rows {
		dir := smsDirections[r["type"]]
		if dir == "" {
			dir = r["type"]
		}
		_ = w.Write([]string{r["address"], formatMillis(r["date"]), dir, r["body"]})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return "", err
	}
	return p, nil
}

// formatMillis 把毫秒时间戳转成本地可读时间，解析失败时原样返回。
func formatMillis(s string) string {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || n <= 0 {
		return s
	}
	return time.UnixMilli(n).Format("2006-01-02 15:04:05")
}
