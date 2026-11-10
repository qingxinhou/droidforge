package download

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// rangeServer 返回一个支持 Range 的测试服务，并记录收到的区间请求。
func rangeServer(t *testing.T, payload []byte) (*httptest.Server, *[]string) {
	t.Helper()
	var mu sync.Mutex
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Header.Get("Range"))
		mu.Unlock()

		rh := r.Header.Get("Range")
		if rh == "" {
			w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
			w.Write(payload)
			return
		}
		var start, end int64
		if _, err := fmt.Sscanf(rh, "bytes=%d-%d", &start, &end); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if start >= int64(len(payload)) {
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		if end >= int64(len(payload)) {
			end = int64(len(payload)) - 1
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload)))
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
		w.WriteHeader(http.StatusPartialContent)
		w.Write(payload[start : end+1])
	}))
	return srv, &seen
}

// TestFetchRangeRetryDoesNotDoubleCount 验证重试不会把同一段字节重复计入进度。
//
// 旧实现每次重试都从原始 start 重新请求，回调却继续累加，
// seg.Done 会涨到超过分段长度——续传时判定「已完成」，文件中间留空洞。
func TestFetchRangeRetryDoesNotDoubleCount(t *testing.T) {
	payload := bytes.Repeat([]byte("Z"), 128<<10)
	var mu sync.Mutex
	attempts := map[string]int{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rh := r.Header.Get("Range")
		mu.Lock()
		n := attempts[rh]
		attempts[rh] = n + 1
		mu.Unlock()

		var start, end int64
		if _, err := fmt.Sscanf(rh, "bytes=%d-%d", &start, &end); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload)))
		w.WriteHeader(http.StatusPartialContent)
		if n == 0 {
			// 第一次只发一小段就断开，逼出重试。
			w.Write(payload[start : start+1024])
			panic(http.ErrAbortHandler)
		}
		w.Write(payload[start : end+1])
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "part.bin")
	f, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var total int64
	got, err := fetchRange(context.Background(), srv.URL, f, 0, int64(len(payload))-1, Options{}, func(n int64) {
		total += n
	})
	if err != nil {
		t.Fatalf("重试后应当成功：%v", err)
	}
	if got != int64(len(payload)) {
		t.Errorf("返回值 = %d，期望 %d", got, len(payload))
	}
	if total != int64(len(payload)) {
		t.Errorf("进度累计 = %d，期望 %d（重试时重复计数了）", total, len(payload))
	}
	onDisk, _ := os.ReadFile(dst)
	if !bytes.Equal(onDisk, payload) {
		t.Error("落盘内容与源不一致")
	}
}

// TestFetchRangeClampsOverlongResponse 验证服务端多给数据时不会写到相邻分段区域。
func TestFetchRangeClampsOverlongResponse(t *testing.T) {
	payload := bytes.Repeat([]byte("Q"), 8192)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 无视请求区间，每次都把整段吐出来。
		w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-%d/%d", len(payload)-1, len(payload)))
		w.WriteHeader(http.StatusPartialContent)
		w.Write(payload)
	}))
	defer srv.Close()

	dir := t.TempDir()
	dst := filepath.Join(dir, "clamp.bin")
	// 用 0xEE 铺底，检查区间之外有没有被覆盖。
	if err := os.WriteFile(dst, bytes.Repeat([]byte{0xEE}, len(payload)), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(dst, os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	const want = 4096
	if _, err := fetchRange(context.Background(), srv.URL, f, 0, want-1, Options{}, func(int64) {}); err != nil {
		t.Fatalf("读取应当成功：%v", err)
	}
	got, _ := os.ReadFile(dst)
	if !bytes.Equal(got[:want], payload[:want]) {
		t.Error("区间内数据不正确")
	}
	for i := want; i < len(got); i++ {
		if got[i] != 0xEE {
			t.Fatalf("区间外第 %d 字节被覆盖成 0x%02x", i, got[i])
		}
	}
}

func TestFetchMultiThread(t *testing.T) {
	payload := bytes.Repeat([]byte("0123456789abcdef"), 128*1024) // 2 MB
	srv, _ := rangeServer(t, payload)
	defer srv.Close()

	dir := t.TempDir()
	dst := filepath.Join(dir, "ota.zip")

	var lastDone, lastTotal int64
	res, err := Fetch(context.Background(), srv.URL+"/ota.zip", dst, Options{
		Threads: 4,
		Resume:  true,
		OnProgress: func(done, total int64) {
			lastDone, lastTotal = done, total
		},
	})
	if err != nil {
		t.Fatalf("下载失败：%v", err)
	}
	if !res.SupportsRange || res.Threads != 4 {
		t.Fatalf("应走 4 线程分段，实际 %+v", res)
	}
	if res.Resumed {
		t.Fatal("首次下载不应标记为续传")
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("下载内容与源不一致")
	}
	if lastDone != int64(len(payload)) || lastTotal != int64(len(payload)) {
		t.Fatalf("进度回调最终值异常：%d/%d", lastDone, lastTotal)
	}
	if _, err := os.Stat(dst + ".dpart"); !os.IsNotExist(err) {
		t.Fatal("完成后应清理断点状态文件")
	}
}

func TestFetchResumeFromState(t *testing.T) {
	payload := bytes.Repeat([]byte("R"), 2<<20) // 2 MB
	srv, seen := rangeServer(t, payload)
	defer srv.Close()

	dir := t.TempDir()
	dst := filepath.Join(dir, "ota.zip")

	// 伪造「前半段已下完」的现场：文件预分配，第一段 Done 满，状态文件落盘。
	half := int64(len(payload) / 2)
	if err := os.WriteFile(dst, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	st := state{
		URL:  srv.URL + "/ota.zip",
		Size: int64(len(payload)),
		Segments: []segment{
			{Start: 0, End: half - 1, Done: half},
			{Start: half, End: int64(len(payload)) - 1, Done: 0},
		},
	}
	b, _ := json.Marshal(&st)
	if err := os.WriteFile(dst+".dpart", b, 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Fetch(context.Background(), srv.URL+"/ota.zip", dst, Options{Threads: 2, Resume: true})
	if err != nil {
		t.Fatalf("续传失败：%v", err)
	}
	if !res.Resumed {
		t.Fatal("应识别为断点续传")
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("续传后内容不一致")
	}

	// 第一个分段已完成，正常情况下不应再对 0- 发起请求（探测请求除外）。
	for _, r := range *seen {
		if strings.HasPrefix(r, "bytes=0-") && !strings.HasPrefix(r, "bytes=0-0") {
			t.Fatalf("已完成的分段被重复下载：%s", r)
		}
	}
}

func TestFetchFallsBackWhenNoRange(t *testing.T) {
	payload := bytes.Repeat([]byte("N"), 300<<10)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 无视 Range，直接整包返回。
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		w.Write(payload)
	}))
	defer srv.Close()

	dir := t.TempDir()
	dst := filepath.Join(dir, "rom.zip")
	res, err := Fetch(context.Background(), srv.URL+"/rom.zip", dst, Options{Threads: 8})
	if err != nil {
		t.Fatalf("降级下载失败：%v", err)
	}
	if res.SupportsRange {
		t.Fatal("该服务不应被判定为支持分段")
	}
	got, _ := os.ReadFile(dst)
	if !bytes.Equal(got, payload) {
		t.Fatal("降级下载内容不一致")
	}
}

func TestFetchExpiredLink(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "x.bin")
	_, err := Fetch(context.Background(), srv.URL+"/x.bin", dst, Options{})
	if err == nil {
		t.Fatal("403 应当返回错误")
	}
	if !strings.Contains(err.Error(), "过期") && !strings.Contains(err.Error(), "拒绝") {
		t.Fatalf("错误信息应提示链接失效：%v", err)
	}
}

func TestDetectFileName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="mi14-ota.zip"`)
		w.Header().Set("Content-Length", "1024")
		w.WriteHeader(http.StatusOK)
		w.Write(make([]byte, 1024))
	}))
	defer srv.Close()

	p, err := Detect(context.Background(), srv.URL, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if p.FileName != "mi14-ota.zip" {
		t.Fatalf("文件名解析错误：%q", p.FileName)
	}
	if p.Size != 1024 {
		t.Fatalf("大小解析错误：%d", p.Size)
	}
}
