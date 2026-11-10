package remote

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ulikunitz/xz"

	"droidforge/internal/image"
)

// ---------------------------------------------------------------------------
// payload 测试夹具
// ---------------------------------------------------------------------------

func pbVarint(v uint64) []byte {
	var buf [10]byte
	n := binary.PutUvarint(buf[:], v)
	return buf[:n]
}

func pbTag(field, wire int) []byte { return pbVarint(uint64(field<<3 | wire)) }

func pbUint(field int, v uint64) []byte {
	return append(pbTag(field, 0), pbVarint(v)...)
}

func pbBytes(field int, b []byte) []byte {
	out := pbTag(field, 2)
	out = append(out, pbVarint(uint64(len(b)))...)
	return append(out, b...)
}

type testExtent struct {
	start  int64
	blocks int64
}

type testOp struct {
	typ     int
	data    []byte
	extents []testExtent
}

type testPart struct {
	name string
	size int64
	ops  []testOp
}

// buildPayloadBytes 生成一个最小的 full payload（version 2）。
func buildPayloadBytes(t *testing.T, parts []testPart) []byte {
	t.Helper()
	const blockSize = 4096

	var data []byte
	var manifestParts [][]byte
	for _, p := range parts {
		var ops [][]byte
		for _, op := range p.ops {
			b := pbUint(1, uint64(op.typ))
			b = append(b, pbUint(2, uint64(len(data)))...) // dataOffset
			b = append(b, pbUint(3, uint64(len(op.data)))...)
			for _, e := range op.extents {
				ext := append(pbUint(1, uint64(e.start)), pbUint(2, uint64(e.blocks))...)
				b = append(b, pbBytes(6, ext)...)
			}
			ops = append(ops, b)
			data = append(data, op.data...)
		}
		size := p.size
		if size == 0 {
			var maxEnd int64
			for _, op := range p.ops {
				for _, e := range op.extents {
					if end := (e.start + e.blocks) * blockSize; end > maxEnd {
						maxEnd = end
					}
				}
			}
			size = maxEnd
		}
		part := pbBytes(1, []byte(p.name))
		part = append(part, pbBytes(7, pbUint(1, uint64(size)))...) // new_partition_info
		for _, op := range ops {
			part = append(part, pbBytes(8, op)...) // operations
		}
		manifestParts = append(manifestParts, part)
	}

	manifest := pbUint(3, blockSize)
	for _, p := range manifestParts {
		manifest = append(manifest, pbBytes(13, p)...)
	}

	var buf bytes.Buffer
	buf.WriteString("CrAU")
	_ = binary.Write(&buf, binary.BigEndian, uint64(2))
	_ = binary.Write(&buf, binary.BigEndian, uint64(len(manifest)))
	_ = binary.Write(&buf, binary.BigEndian, uint32(0))
	buf.Write(manifest)
	buf.Write(data)
	return buf.Bytes()
}

// buildOtaZip 生成包含 payload.bin（stored，不压缩）的 OTA zip。
func buildOtaZip(t *testing.T, payload []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	_ = zw.SetComment(strings.Repeat("droidforge-ota-comment", 40)) // 覆盖 EOCD 扫描逻辑

	w, err := zw.CreateHeader(&zip.FileHeader{
		Name:   "payload.bin",
		Method: zip.Store,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(payload); err != nil {
		t.Fatal(err)
	}
	props, err := zw.Create("payload_properties.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := props.Write([]byte("FILE_HASH=deadbeef\n")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func serveRange(t *testing.T, name string, data []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// ---------------------------------------------------------------------------
// 用例
// ---------------------------------------------------------------------------

func TestRemoteInspectAndExtract(t *testing.T) {
	raw1 := bytes.Repeat([]byte{0xA1}, 8192)
	xz1 := mustXZ(t, bytes.Repeat([]byte{0xB2}, 8192))
	cached := bytes.Repeat([]byte{0xC3}, 4096)

	payload := buildPayloadBytes(t, []testPart{
		{
			name: "boot",
			ops: []testOp{{
				typ:     image.OpReplace,
				data:    raw1,
				extents: []testExtent{{0, 2}},
			}},
		},
		{
			name: "system",
			ops: []testOp{
				{typ: image.OpReplaceXz, data: xz1, extents: []testExtent{{0, 1}, {1, 1}}},
				{typ: image.OpZero, data: nil, extents: []testExtent{{2, 1}}},
				{typ: image.OpReplace, data: cached, extents: []testExtent{{3, 1}}},
			},
		},
	})
	zipData := buildOtaZip(t, payload)
	srv := serveRange(t, "ota-update.zip", zipData)

	ctx := context.Background()
	info, err := Inspect(ctx, srv.URL+"/download?token=abc", Options{})
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if !info.SupportsRange {
		t.Fatal("expected range support")
	}
	if info.Container != "zip" {
		t.Errorf("container = %q, want zip", info.Container)
	}
	if info.FileName != "ota-update.zip" {
		t.Errorf("fileName = %q", info.FileName)
	}
	if info.TotalSize != int64(len(zipData)) {
		t.Errorf("totalSize = %d, want %d", info.TotalSize, len(zipData))
	}
	if len(info.Partitions) != 2 {
		t.Fatalf("partitions = %+v", info.Partitions)
	}

	var sys *Partition
	for i := range info.Partitions {
		if info.Partitions[i].Name == "system" {
			sys = &info.Partitions[i]
		}
	}
	if sys == nil {
		t.Fatal("system partition not found")
	}
	if sys.Size != 4*4096 {
		t.Errorf("system size = %d, want %d", sys.Size, 4*4096)
	}
	if sys.Compressed != int64(len(xz1)+len(cached)) {
		t.Errorf("system compressed = %d, want %d", sys.Compressed, len(xz1)+len(cached))
	}

	out := t.TempDir()
	files, err := Extract(ctx, srv.URL+"/download?token=abc", []string{"boot", "system"}, out, Options{}, nil)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("files = %v", files)
	}

	boot, err := os.ReadFile(filepath.Join(out, "boot.img"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(boot, raw1) {
		t.Error("boot content mismatch")
	}

	system, err := os.ReadFile(filepath.Join(out, "system.img"))
	if err != nil {
		t.Fatal(err)
	}
	if len(system) != 4*4096 {
		t.Fatalf("system size = %d", len(system))
	}
	want := append(bytes.Repeat([]byte{0xB2}, 8192), make([]byte, 4096)...)
	want = append(want, cached...)
	if !bytes.Equal(system, want) {
		t.Error("system content mismatch（xz 解压 / 多 extent 写入 / ZERO 区填充有误）")
	}
}

func TestRemoteDirectPayload(t *testing.T) {
	payload := buildPayloadBytes(t, []testPart{{
		name: "init_boot",
		ops:  []testOp{{typ: image.OpReplace, data: bytes.Repeat([]byte{0x77}, 4096), extents: []testExtent{{0, 1}}}},
	}})
	srv := serveRange(t, "payload.bin", payload)

	info, err := Inspect(context.Background(), srv.URL+"/payload.bin", Options{})
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if info.Container != "payload" {
		t.Errorf("container = %q, want payload", info.Container)
	}
	if len(info.Partitions) != 1 || info.Partitions[0].Name != "init_boot" {
		t.Fatalf("partitions = %+v", info.Partitions)
	}
}

func TestReadAtOffsets(t *testing.T) {
	data := make([]byte, 300000)
	for i := range data {
		data[i] = byte(i % 251)
	}
	srv := serveRange(t, "blob.bin", data)

	f, err := Open(context.Background(), srv.URL, Options{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if f.Size != int64(len(data)) {
		t.Fatalf("size = %d", f.Size)
	}
	for _, tc := range []struct{ off, n int }{
		{0, 16}, {1000, 4096}, {299000, 1000}, {131072, 70000},
	} {
		buf := make([]byte, tc.n)
		if _, err := f.ReadAt(buf, int64(tc.off)); err != nil {
			t.Fatalf("ReadAt(%d,%d): %v", tc.off, tc.n, err)
		}
		if !bytes.Equal(buf, data[tc.off:tc.off+tc.n]) {
			t.Fatalf("ReadAt(%d,%d) content mismatch", tc.off, tc.n)
		}
	}

	// 顺序读取器要能跨越多个分块
	rc := f.Reader(0, int64(len(data)))
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("Reader: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Error("Reader content mismatch")
	}
}

func TestDownloadWholePackage(t *testing.T) {
	payload := buildPayloadBytes(t, []testPart{{
		name: "dtbo",
		ops:  []testOp{{typ: image.OpReplace, data: bytes.Repeat([]byte{0x11}, 4096), extents: []testExtent{{0, 1}}}},
	}})
	zipData := buildOtaZip(t, payload)
	srv := serveRange(t, "full-ota.zip", zipData)

	outDir := t.TempDir()
	path, err := Download(context.Background(), srv.URL+"/x/full-ota.zip", outDir, Options{}, nil)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if filepath.Base(path) != "full-ota.zip" {
		t.Errorf("path = %s", path)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, zipData) {
		t.Error("downloaded content mismatch")
	}
}

func TestNoRangeServer(t *testing.T) {
	data := buildOtaZip(t, buildPayloadBytes(t, []testPart{{
		name: "boot",
		ops:  []testOp{{typ: image.OpReplace, data: bytes.Repeat([]byte{1}, 4096), extents: []testExtent{{0, 1}}}},
	}}))
	// 忽略 Range 的服务器：始终返回 200 全量
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", itoa(len(data)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	info, err := Inspect(context.Background(), srv.URL, Options{})
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if info.SupportsRange {
		t.Error("expected SupportsRange=false")
	}
	if info.Note == "" {
		t.Error("expected a note explaining the limitation")
	}

	if _, err := Extract(context.Background(), srv.URL, nil, t.TempDir(), Options{}, nil); err == nil {
		t.Error("Extract should fail when server ignores Range")
	}
}

func mustXZ(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := xz.NewWriter(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func itoa(v int) string {
	var buf [20]byte
	i := len(buf)
	if v == 0 {
		return "0"
	}
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
