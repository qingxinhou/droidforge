package lp

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func writeTemp(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatalf("写入 %s 失败：%v", name, err)
	}
	return p
}

func TestBuildParseSplit(t *testing.T) {
	dir := t.TempDir()

	system := bytes.Repeat([]byte{0xA1}, 12288)
	vendor := bytes.Repeat([]byte{0xB2}, 4096+777) // 故意非块对齐，验证向上取整
	sysPath := writeTemp(t, dir, "system_a.img", system)
	venPath := writeTemp(t, dir, "vendor_a.img", vendor)

	out := filepath.Join(dir, "super.img")
	built, err := Build(context.Background(), []BuildPartition{
		{Name: "system_a", Path: sysPath},
		{Name: "vendor_a", Path: venPath, Group: "qti_dynamic_partitions"},
	}, out, BuildOptions{}, nil)
	if err != nil {
		t.Fatalf("合成失败：%v", err)
	}

	if len(built.Partitions) != 2 {
		t.Fatalf("分区数应为 2，实际 %d", len(built.Partitions))
	}
	if !built.ChecksumOK {
		t.Fatal("刚生成的 metadata 校验和应当有效")
	}
	if built.Major != 10 || built.Minor != 2 {
		t.Fatalf("版本应为 10.2，实际 %d.%d", built.Major, built.Minor)
	}
	if got := built.Groups[0].Name; got != "default" {
		t.Fatalf("0 号组必须是 default，实际 %q", got)
	}
	if len(built.Groups) != 2 {
		t.Fatalf("应有 default 与自定义组共 2 个，实际 %d", len(built.Groups))
	}

	// 分区起点应落在 1MB 对齐处。
	p0, ok := built.Partition("system_a")
	if !ok {
		t.Fatal("找不到 system_a")
	}
	if got := p0.Extents[0].TargetData * sectorSize; got%DefaultAlignment != 0 {
		t.Fatalf("system_a 起点 %d 未按 1MB 对齐", got)
	}
	if p0.SizeBytes != int64(len(system)) {
		t.Fatalf("system_a 大小应为 %d，实际 %d", len(system), p0.SizeBytes)
	}

	p1, _ := built.Partition("vendor_a")
	if p1.Group != "qti_dynamic_partitions" {
		t.Fatalf("vendor_a 组名错误：%q", p1.Group)
	}
	if p1.SizeBytes%512 != 0 || p1.SizeBytes < int64(len(vendor)) {
		t.Fatalf("vendor_a 大小应按扇区向上取整，实际 %d", p1.SizeBytes)
	}

	// 拆分后逐字节比对。
	outDir := filepath.Join(dir, "out")
	res, err := Split(context.Background(), out, outDir, nil, nil)
	if err != nil {
		t.Fatalf("拆分失败：%v", err)
	}
	if len(res) != 2 {
		t.Fatalf("应拆出 2 个镜像，实际 %d", len(res))
	}
	got, err := os.ReadFile(filepath.Join(outDir, "system_a.img"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, system) {
		t.Fatal("system_a 拆分结果与原文件不一致")
	}

	gotVendor, err := os.ReadFile(filepath.Join(outDir, "vendor_a.img"))
	if err != nil {
		t.Fatal(err)
	}
	// 分区大小按扇区对齐，尾部补零。
	if len(gotVendor) != int(p1.SizeBytes) {
		t.Fatalf("vendor_a 输出长度应为 %d，实际 %d", p1.SizeBytes, len(gotVendor))
	}
	if !bytes.Equal(gotVendor[:len(vendor)], vendor) {
		t.Fatal("vendor_a 数据区不一致")
	}
	for _, b := range gotVendor[len(vendor):] {
		if b != 0 {
			t.Fatal("vendor_a 尾部补齐应为 0")
		}
	}
}

// makeSparse 把 raw 数据包成一个单块 raw 的 sparse 镜像，模拟线刷包里的 super.img。
func makeSparse(t *testing.T, raw []byte) []byte {
	t.Helper()
	const blk = 4096
	blocks := (len(raw) + blk - 1) / blk
	padded := make([]byte, blocks*blk)
	copy(padded, raw)

	head := make([]byte, 28)
	le := binary.LittleEndian
	le.PutUint32(head[0:4], sparseMagic)
	le.PutUint16(head[4:6], 1)
	le.PutUint16(head[6:8], 0)
	le.PutUint16(head[8:10], 28)
	le.PutUint16(head[10:12], 12)
	le.PutUint32(head[12:16], blk)
	le.PutUint32(head[16:20], uint32(blocks))
	le.PutUint32(head[20:24], 1)

	chunk := make([]byte, 12)
	le.PutUint16(chunk[0:2], chunkRaw)
	le.PutUint32(chunk[4:8], uint32(blocks))
	le.PutUint32(chunk[8:12], uint32(12+len(padded)))

	return append(append(head, chunk...), padded...)
}

func TestParseSparseSuper(t *testing.T) {
	dir := t.TempDir()
	boot := bytes.Repeat([]byte{0x5C}, 8192)
	bootPath := writeTemp(t, dir, "boot_a.img", boot)

	rawPath := filepath.Join(dir, "super.raw")
	_, err := Build(context.Background(), []BuildPartition{{Name: "boot_a", Path: bootPath}}, rawPath, BuildOptions{}, nil)
	if err != nil {
		t.Fatalf("合成失败：%v", err)
	}
	raw, err := os.ReadFile(rawPath)
	if err != nil {
		t.Fatal(err)
	}

	sparsePath := writeTemp(t, dir, "super.img", makeSparse(t, raw))
	meta, err := Parse(sparsePath)
	if err != nil {
		t.Fatalf("解析 sparse 失败：%v", err)
	}
	if !meta.Sparse {
		t.Fatal("应当识别为 sparse")
	}
	if len(meta.Partitions) != 1 || meta.Partitions[0].Name != "boot_a" {
		t.Fatalf("sparse 解析分区异常：%+v", meta.Partitions)
	}

	outDir := filepath.Join(dir, "sparse-out")
	if _, err := Split(context.Background(), sparsePath, outDir, []string{"boot_a"}, nil); err != nil {
		t.Fatalf("拆分 sparse 失败：%v", err)
	}
	got, err := os.ReadFile(filepath.Join(outDir, "boot_a.img"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, boot) {
		t.Fatal("sparse 拆分结果不一致")
	}
}

func TestParseRejectsNonSuper(t *testing.T) {
	dir := t.TempDir()
	p := writeTemp(t, dir, "random.img", bytes.Repeat([]byte{7}, 8192))
	if _, err := Parse(p); err == nil {
		t.Fatal("普通文件不应被当作 super 解析")
	}
	if IsSuper(p) {
		t.Fatal("IsSuper 对普通文件应返回 false")
	}
}

func TestChecksumMismatchStillParses(t *testing.T) {
	dir := t.TempDir()
	img := writeTemp(t, dir, "boot_a.img", bytes.Repeat([]byte{1}, 4096))
	out := filepath.Join(dir, "super.img")
	if _, err := Build(context.Background(), []BuildPartition{{Name: "boot_a", Path: img}}, out, BuildOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	buf, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	// 破坏表区一个字节但不动表头：校验和应当失败，解析仍应成功。
	buf[primaryMetadataOffset+headerSize+4] ^= 0xFF
	if err := os.WriteFile(out, buf, 0o644); err != nil {
		t.Fatal(err)
	}
	meta, err := Parse(out)
	if err != nil {
		t.Fatalf("校验和损坏时仍应能解析：%v", err)
	}
	if meta.ChecksumOK {
		t.Fatal("校验和应当被判为无效")
	}
}

func TestSplitMissingPartition(t *testing.T) {
	dir := t.TempDir()
	img := writeTemp(t, dir, "boot_a.img", bytes.Repeat([]byte{2}, 4096))
	out := filepath.Join(dir, "super.img")
	if _, err := Build(context.Background(), []BuildPartition{{Name: "boot_a", Path: img}}, out, BuildOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := Split(context.Background(), out, filepath.Join(dir, "o"), []string{"nope"}, nil); err == nil {
		t.Fatal("选择不存在的分区应当报错")
	}
}
