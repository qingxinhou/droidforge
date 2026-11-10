package image

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// protobuf 编码辅助（仅测试用）
// ---------------------------------------------------------------------------

func pbVarint(v uint64) []byte {
	var buf [10]byte
	n := binary.PutUvarint(buf[:], v)
	return buf[:n]
}

func pbTag(field, wire int) []byte {
	return pbVarint(uint64(field<<3 | wire))
}

func pbUint(field int, v uint64) []byte {
	out := pbTag(field, 0)
	return append(out, pbVarint(v)...)
}

func pbBytes(field int, b []byte) []byte {
	out := pbTag(field, 2)
	out = append(out, pbVarint(uint64(len(b)))...)
	return append(out, b...)
}

func buildPayloadFixture(t *testing.T, partition string, blockSize int, blocks int, data []byte) string {
	t.Helper()

	extent := append(pbUint(1, 0), pbUint(2, uint64(blocks))...)
	op := append(pbUint(1, OpReplace), pbUint(2, 0)...)
	op = append(op, pbUint(3, uint64(len(data)))...)
	op = append(op, pbBytes(6, extent)...)

	part := pbBytes(1, []byte(partition))
	part = append(part, pbBytes(7, pbUint(1, uint64(blockSize*blocks)))...) // new_partition_info
	part = append(part, pbBytes(8, op)...)                                  // operations

	manifest := pbUint(3, uint64(blockSize))
	manifest = append(manifest, pbBytes(13, part)...)

	var buf bytes.Buffer
	buf.WriteString("CrAU")
	_ = binary.Write(&buf, binary.BigEndian, uint64(2))
	_ = binary.Write(&buf, binary.BigEndian, uint64(len(manifest)))
	_ = binary.Write(&buf, binary.BigEndian, uint32(0))
	buf.Write(manifest)
	buf.Write(data)

	path := filepath.Join(t.TempDir(), "payload.bin")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParsePayload(t *testing.T) {
	data := bytes.Repeat([]byte{0x5A}, 8192)
	path := buildPayloadFixture(t, "boot", 4096, 2, data)

	info, err := ParsePayload(path)
	if err != nil {
		t.Fatalf("ParsePayload: %v", err)
	}
	if info.Version != 2 {
		t.Errorf("version = %d, want 2", info.Version)
	}
	if info.BlockSize != 4096 {
		t.Errorf("blockSize = %d, want 4096", info.BlockSize)
	}
	if len(info.Partitions) != 1 || info.Partitions[0].Name != "boot" {
		t.Fatalf("partitions = %+v", info.Partitions)
	}
	if info.Partitions[0].Size != 8192 {
		t.Errorf("size = %d, want 8192", info.Partitions[0].Size)
	}
	if !info.Full {
		t.Error("expected full OTA")
	}
}

func TestExtractPayload(t *testing.T) {
	data := bytes.Repeat([]byte{0x5A}, 8192)
	path := buildPayloadFixture(t, "boot", 4096, 2, data)

	out := t.TempDir()
	files, err := ExtractPayload(path, []string{"boot"}, out, nil)
	if err != nil {
		t.Fatalf("ExtractPayload: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("files = %v", files)
	}
	got, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 8192 {
		t.Fatalf("extracted size = %d, want 8192", len(got))
	}
	if !bytes.Equal(got, data) {
		t.Error("extracted content mismatch")
	}
}

func TestPayloadStreamManifest(t *testing.T) {
	data := bytes.Repeat([]byte{0x11}, 4096)
	path := buildPayloadFixture(t, "vendor_boot", 4096, 1, data)

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	info, err := ParsePayloadStream(f)
	if err != nil {
		t.Fatalf("ParsePayloadStream: %v", err)
	}
	if len(info.Partitions) != 1 || info.Partitions[0].Name != "vendor_boot" {
		t.Fatalf("partitions = %+v", info.Partitions)
	}
}

// ---------------------------------------------------------------------------
// boot.img
// ---------------------------------------------------------------------------

func buildBootV0(t *testing.T, kernel, ramdisk []byte, pageSize int) string {
	t.Helper()

	hdr := make([]byte, pageSize)
	copy(hdr[0:8], bootMagic)
	le := binary.LittleEndian
	le.PutUint32(hdr[8:12], uint32(len(kernel)))
	le.PutUint32(hdr[12:16], 0x10008000)
	le.PutUint32(hdr[16:20], uint32(len(ramdisk)))
	le.PutUint32(hdr[20:24], 0x11000000)
	le.PutUint32(hdr[36:40], uint32(pageSize))
	le.PutUint32(hdr[40:44], 0) // header version
	le.PutUint32(hdr[44:48], 0)
	copy(hdr[48:64], []byte("test-board"))
	copy(hdr[64:64+64], []byte("console=ttyMSM0 androidboot.hardware=test"))

	var buf bytes.Buffer
	buf.Write(hdr)
	buf.Write(kernel)
	buf.Write(make([]byte, alignPad(len(kernel), pageSize)))
	buf.Write(ramdisk)
	buf.Write(make([]byte, alignPad(len(ramdisk), pageSize)))

	path := filepath.Join(t.TempDir(), "boot.img")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func alignPad(n, page int) int {
	if r := n % page; r != 0 {
		return page - r
	}
	return 0
}

func TestParseBootV0(t *testing.T) {
	k := bytes.Repeat([]byte{0x01}, 300)
	r := bytes.Repeat([]byte{0x02}, 120)
	path := buildBootV0(t, k, r, 2048)

	info, err := ParseBoot(path)
	if err != nil {
		t.Fatalf("ParseBoot: %v", err)
	}
	if info.HeaderVersion != 0 {
		t.Errorf("headerVersion = %d, want 0", info.HeaderVersion)
	}
	if info.PageSize != 2048 {
		t.Errorf("pageSize = %d, want 2048", info.PageSize)
	}
	if info.KernelSize != 300 {
		t.Errorf("kernelSize = %d, want 300", info.KernelSize)
	}
	if info.RamdiskSize != 120 {
		t.Errorf("ramdiskSize = %d, want 120", info.RamdiskSize)
	}
	if info.Name != "test-board" {
		t.Errorf("name = %q", info.Name)
	}
}

func TestUnpackBootV0(t *testing.T) {
	k := bytes.Repeat([]byte{0x01}, 300)
	r := bytes.Repeat([]byte{0x02}, 120)
	path := buildBootV0(t, k, r, 2048)

	outDir := t.TempDir()
	files, err := UnpackBoot(path, outDir)
	if err != nil {
		t.Fatalf("UnpackBoot: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("files = %v", files)
	}
	gotKernel, err := os.ReadFile(filepath.Join(outDir, "kernel.img"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotKernel, k) {
		t.Error("kernel content mismatch")
	}
	gotRamdisk, err := os.ReadFile(filepath.Join(outDir, "ramdisk.img"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotRamdisk, r) {
		t.Error("ramdisk content mismatch")
	}
}

// ---------------------------------------------------------------------------
// sparse
// ---------------------------------------------------------------------------

func TestUnsparse(t *testing.T) {
	const blk = 4096
	raw := bytes.Repeat([]byte{0xAB}, blk)

	var buf bytes.Buffer
	writeU32 := func(v uint32) { _ = binary.Write(&buf, binary.LittleEndian, v) }
	writeU16 := func(v uint16) { _ = binary.Write(&buf, binary.LittleEndian, v) }

	writeU32(sparseMagic)
	writeU16(1)   // major
	writeU16(0)   // minor
	writeU16(28)  // file header size
	writeU16(12)  // chunk header size
	writeU32(blk) // block size
	writeU32(2)   // total blocks
	writeU32(2)   // total chunks
	writeU32(0)   // checksum

	// RAW chunk
	writeU16(chunkRaw)
	writeU16(0)
	writeU32(1)                     // chunk blocks
	writeU32(uint32(12 + len(raw))) // total size
	buf.Write(raw)

	// DONT_CARE chunk
	writeU16(chunkDontCare)
	writeU16(0)
	writeU32(1)
	writeU32(12)

	src := filepath.Join(t.TempDir(), "super.img")
	if err := os.WriteFile(src, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "super_raw.img")
	if err := Unsparse(src, dst, nil); err != nil {
		t.Fatalf("Unsparse: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2*blk {
		t.Fatalf("size = %d, want %d", len(got), 2*blk)
	}
	if !bytes.Equal(got[:blk], raw) {
		t.Error("raw chunk mismatch")
	}
	for _, b := range got[blk:] {
		if b != 0 {
			t.Error("dont-care chunk should be zero-filled")
			break
		}
	}
}
