package avb

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// buildDescriptor 拼一个描述符：tag + following 长度 + 正文。
// 正文补齐到 8 字节对齐，与 avbtool 的输出一致。
func buildDescriptor(tag uint64, body []byte) []byte {
	pad := (8 - len(body)%8) % 8
	padded := make([]byte, len(body)+pad)
	copy(padded, body)

	out := make([]byte, 16+len(padded))
	binary.BigEndian.PutUint64(out[0:], tag)
	binary.BigEndian.PutUint64(out[8:], uint64(len(padded)))
	copy(out[16:], padded)
	return out
}

// hashDescBody 按 AvbHashDescriptor 的字段顺序拼正文。
func hashDescBody(imageSize uint64, alg, name string, salt, digest []byte, flags uint32) []byte {
	body := make([]byte, 116, 116+len(name)+len(salt)+len(digest))
	binary.BigEndian.PutUint64(body[0:], imageSize)
	putPadded(body[8:40], alg)
	binary.BigEndian.PutUint32(body[40:], uint32(len(name)))
	binary.BigEndian.PutUint32(body[44:], uint32(len(salt)))
	binary.BigEndian.PutUint32(body[48:], uint32(len(digest)))
	binary.BigEndian.PutUint32(body[52:], flags)
	body = append(body, name...)
	body = append(body, salt...)
	body = append(body, digest...)
	return body
}

func putPadded(dst []byte, s string) {
	for i := range dst {
		dst[i] = 0
	}
	copy(dst, s)
}

// buildVBMeta 生成一个结构自洽的 vbmeta。
func buildVBMeta(rollback uint64, flags uint32, alg uint32, descs []byte) []byte {
	hash := make([]byte, 32)
	sig := make([]byte, 0)
	if alg != 0 {
		sig = make([]byte, 256)
		for i := range sig {
			sig[i] = byte(i)
		}
	}
	pk := make([]byte, 0)
	if alg != 0 {
		pk = make([]byte, 520)
	}

	hashOff := uint64(HeaderSize)
	sigOff := hashOff + uint64(len(hash))
	pkOff := sigOff + uint64(len(sig))
	descOff := pkOff + uint64(len(pk))

	total := HeaderSize + len(hash) + len(sig) + len(pk) + len(descs)
	out := make([]byte, total)
	copy(out, "AVB0")
	be := binary.BigEndian
	be.PutUint32(out[4:], 2) // major
	be.PutUint32(out[8:], 0) // minor
	be.PutUint64(out[12:], uint64(HeaderSize)+uint64(len(hash))+uint64(len(sig))+uint64(len(pk)))
	be.PutUint64(out[20:], uint64(len(descs)))
	be.PutUint32(out[28:], alg)
	be.PutUint64(out[32:], hashOff)
	be.PutUint64(out[40:], uint64(len(hash)))
	be.PutUint64(out[48:], sigOff)
	be.PutUint64(out[56:], uint64(len(sig)))
	be.PutUint64(out[64:], pkOff)
	be.PutUint64(out[72:], uint64(len(pk)))
	be.PutUint64(out[96:], descOff)
	be.PutUint64(out[104:], uint64(len(descs)))
	be.PutUint64(out[112:], rollback)
	be.PutUint32(out[120:], flags)
	out[124] = 0
	copy(out[128:175], "avbtool 1.2.0")

	copy(out[hashOff:], hash)
	copy(out[sigOff:], sig)
	copy(out[pkOff:], pk)
	copy(out[descOff:], descs)
	return out
}

func TestParseBareVBMetaWithHashDescriptor(t *testing.T) {
	digest := make([]byte, 32)
	for i := range digest {
		digest[i] = 0xAB
	}
	body := hashDescBody(8<<20, "sha256", "boot", []byte{1, 2, 3, 4}, digest, 0)
	descs := buildDescriptor(DescHash<<16, body)

	raw := buildVBMeta(7, 0, 2, descs)
	img, err := ParseBytes(raw)
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if img.RollbackIndex != 7 {
		t.Errorf("rollback_index = %d，期望 7", img.RollbackIndex)
	}
	if img.Algorithm != "SHA256_RSA4096" {
		t.Errorf("算法 = %q", img.Algorithm)
	}
	if img.VBMetaOffset != 0 || img.VBMetaSize != int64(len(raw)) {
		t.Errorf("裸 vbmeta 的位置信息不对：off=%d size=%d", img.VBMetaOffset, img.VBMetaSize)
	}
	if len(img.Descriptors) != 1 {
		t.Fatalf("描述符数量 = %d，期望 1", len(img.Descriptors))
	}
	d := img.Descriptors[0]
	if d.Kind != "hash" {
		t.Errorf("类型 = %q，期望 hash", d.Kind)
	}
	if d.PartitionName != "boot" {
		t.Errorf("分区名 = %q", d.PartitionName)
	}
	if d.ImageSize != 8<<20 {
		t.Errorf("image_size = %d", d.ImageSize)
	}
	if d.HashAlgorithm != "sha256" {
		t.Errorf("hash 算法 = %q", d.HashAlgorithm)
	}
	if d.Salt != "01020304" {
		t.Errorf("salt = %q", d.Salt)
	}
	if len(d.Digest) != 64 || d.Digest[:4] != "abab" {
		t.Errorf("digest = %q", d.Digest)
	}
}

func TestParseFlagsAndHashtree(t *testing.T) {
	// flags: HASHTREE_DISABLED | VERIFICATION_DISABLED
	body := make([]byte, 164)
	be := binary.BigEndian
	be.PutUint32(body[0:], 1) // dm-verity version
	be.PutUint64(body[4:], 64<<20)
	be.PutUint64(body[12:], 64<<20) // tree_offset
	be.PutUint64(body[20:], 1024)   // tree_size
	be.PutUint32(body[28:], 4096)   // data_block_size
	be.PutUint32(body[32:], 4096)   // hash_block_size
	putPadded(body[56:88], "sha256")
	name := "system"
	root := make([]byte, 32)
	for i := range root {
		root[i] = 0xCD
	}
	be.PutUint32(body[88:], uint32(len(name)))
	be.PutUint32(body[92:], 0)
	be.PutUint32(body[96:], uint32(len(root)))
	body = append(body, name...)
	body = append(body, root...)

	descs := buildDescriptor(DescHashtree<<16, body)
	raw := buildVBMeta(3, 3, 5, descs)

	img, err := ParseBytes(raw)
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if !img.HashtreeDisabled || !img.VerificationDisabled {
		t.Errorf("flags 解析错误：%+v", img.Flags)
	}
	if len(img.Descriptors) != 1 {
		t.Fatalf("描述符数量 = %d", len(img.Descriptors))
	}
	d := img.Descriptors[0]
	if d.Kind != "hashtree" || d.PartitionName != "system" {
		t.Fatalf("hashtree 描述符解析错误：%+v", d)
	}
	if d.DataBlockSize != 4096 || d.TreeOffset != 64<<20 {
		t.Errorf("块大小/树偏移错误：%+v", d)
	}
	if len(d.RootDigest) != 64 {
		t.Errorf("root digest 长度 = %d", len(d.RootDigest))
	}
}

func TestParseVBMetaBehindFooter(t *testing.T) {
	raw := buildVBMeta(1, 0, 0, nil)
	orig := make([]byte, 4096)
	for i := range orig {
		orig[i] = byte(i % 251)
	}
	combined := append(append([]byte{}, orig...), raw...)

	footer := make([]byte, footerSize)
	copy(footer, "AVBf")
	be := binary.BigEndian
	be.PutUint32(footer[4:], 1)
	be.PutUint32(footer[8:], 0)
	be.PutUint64(footer[12:], uint64(len(orig)))
	be.PutUint64(footer[20:], uint64(len(orig)))
	be.PutUint64(footer[28:], uint64(len(raw)))
	combined = append(combined, footer...)

	img, err := ParseBytes(combined)
	if err != nil {
		t.Fatalf("带 footer 的镜像解析失败：%v", err)
	}
	if img.VBMetaOffset != int64(len(orig)) {
		t.Errorf("vbmeta 偏移 = %d，期望 %d", img.VBMetaOffset, len(orig))
	}
	if img.FooterOriginalSize != int64(len(orig)) {
		t.Errorf("原始镜像长度 = %d", img.FooterOriginalSize)
	}
	if img.AlgorithmType != 0 {
		t.Errorf("算法应为 NONE，实际 %d", img.AlgorithmType)
	}
	if len(img.Descriptors) != 0 {
		t.Errorf("不该有描述符，实际 %d 个", len(img.Descriptors))
	}
	// 空描述符应给出提示。
	found := false
	for _, n := range img.Notes {
		if n != "" && img.Algorithm == "NONE（未签名）" {
			found = true
		}
	}
	if !found {
		t.Errorf("缺少未签名提示，notes=%v", img.Notes)
	}
}

func TestParseChainAndCmdline(t *testing.T) {
	// chain partition
	cb := make([]byte, 76)
	be := binary.BigEndian
	be.PutUint32(cb[0:], 2) // rollback_index_location
	name := "vbmeta_system"
	be.PutUint32(cb[4:], uint32(len(name)))
	be.PutUint32(cb[8:], 1024) // public key 长度
	cb = append(cb, name...)

	// kernel cmdline
	cmd := "androidboot.vbmeta.device_state=unlocked"
	kb := make([]byte, 8, 8+len(cmd))
	be.PutUint32(kb[0:], 1)
	be.PutUint32(kb[4:], uint32(len(cmd)))
	kb = append(kb, cmd...)

	descs := append(buildDescriptor(DescChain<<16, cb), buildDescriptor(DescKernelCmdline<<16, kb)...)
	img, err := ParseBytes(buildVBMeta(2, 0, 2, descs))
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if len(img.Descriptors) != 2 {
		t.Fatalf("描述符数量 = %d，期望 2", len(img.Descriptors))
	}
	var chain, cmdline *Descriptor
	for i := range img.Descriptors {
		switch img.Descriptors[i].Kind {
		case "chain_partition":
			chain = &img.Descriptors[i]
		case "kernel_cmdline":
			cmdline = &img.Descriptors[i]
		}
	}
	if chain == nil || chain.PartitionName != "vbmeta_system" || chain.RollbackIndexLocation != 2 {
		t.Errorf("chain 描述符解析错误：%+v", chain)
	}
	if cmdline == nil || cmdline.Cmdline != cmd {
		t.Errorf("cmdline 描述符解析错误：%+v", cmdline)
	}
}

func TestParseRejectsNonAVB(t *testing.T) {
	raw := make([]byte, 512)
	for i := range raw {
		raw[i] = 'x'
	}
	if _, err := ParseBytes(raw); err == nil {
		t.Fatal("非 AVB 文件应当报错")
	}
	if _, err := ParseBytes([]byte("short")); err == nil {
		t.Fatal("过短的文件应当报错")
	}
}

func TestParseTruncatedDescriptors(t *testing.T) {
	body := hashDescBody(1024, "sha256", "boot", nil, make([]byte, 32), 0)
	full := buildDescriptor(DescHash<<16, body)
	// 砍掉尾部 10 字节，模拟下载不完整的镜像。
	raw := buildVBMeta(1, 0, 2, full)
	raw = raw[:len(raw)-10]

	img, err := ParseBytes(raw)
	if err != nil {
		// 描述符越界应该在 header 校验阶段就拦下，这是可接受的结果。
		return
	}
	if len(img.Notes) == 0 {
		t.Errorf("截断的镜像应当至少给出一条提示")
	}
}

func TestParseFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "vbmeta.img")
	raw := buildVBMeta(9, 0, 2, nil)
	if err := os.WriteFile(p, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	img, err := Parse(p)
	if err != nil {
		t.Fatalf("解析文件失败：%v", err)
	}
	if img.Path != p || img.RollbackIndex != 9 {
		t.Errorf("路径或 rollback 不对：%+v", img)
	}
}
