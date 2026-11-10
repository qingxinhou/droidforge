// Package avb 解析 Android Verified Boot 2.0 的 vbmeta 结构。
//
// 规范来源：AOSP external/avb/libavb 的 avb_vbmeta_image.h 与
// avb_descriptor.h，字段顺序与长度按公开头文件独立实现。
//
// 只做只读解析，不生成签名结构——签名需要私钥，属于厂商范畴。
package avb

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"sort"
	"strings"
)

// 镜像魔数。
var (
	magicVBMeta = []byte("AVB0")
	magicFooter = []byte("AVBf")
)

// HeaderSize 是 vbmeta 头的固定长度。
const HeaderSize = 256

// footerSize 是附加在镜像末尾的 AvbFooter 长度。
const footerSize = 64

// 算法类型编号。
var algorithmNames = map[uint32]string{
	0: "NONE（未签名）",
	1: "SHA256_RSA2048",
	2: "SHA256_RSA4096",
	3: "SHA256_RSA8192",
	4: "SHA512_RSA2048",
	5: "SHA512_RSA4096",
	6: "SHA512_RSA8192",
}

// 描述符类型标签。
const (
	DescProperty      = 0
	DescHashtree      = 1
	DescHash          = 2
	DescKernelCmdline = 3
	DescChain         = 4
)

var descNames = map[uint64]string{
	DescProperty:      "property",
	DescHashtree:      "hashtree",
	DescHash:          "hash",
	DescKernelCmdline: "kernel_cmdline",
	DescChain:         "chain_partition",
}

// Image 是解析后的 vbmeta。
type Image struct {
	Path string `json:"path"`

	// 位置信息。带 footer 的镜像里 vbmeta 通常追加在原始镜像之后。
	VBMetaOffset int64 `json:"vbmetaOffset"`
	VBMetaSize   int64 `json:"vbmetaSize"`
	// FooterOriginalSize 是 footer 记录的原始镜像长度（不含 vbmeta），
	// 直接为 0 时说明镜像本身就是裸 vbmeta。
	FooterOriginalSize int64 `json:"footerOriginalSize"`

	VersionMajor uint32 `json:"versionMajor"`
	VersionMinor uint32 `json:"versionMinor"`

	AlgorithmType uint32 `json:"algorithmType"`
	Algorithm     string `json:"algorithm"`

	// RollbackIndex 是防回滚版本号，ARB 分析的核心字段。
	RollbackIndex uint64 `json:"rollbackIndex"`
	// RollbackIndexLocation 指明该值存储在设备的哪个 rollback 槽。
	RollbackIndexLocation uint8 `json:"rollbackIndexLocation"`

	HashSize      uint64 `json:"hashSize"`
	SignatureSize uint64 `json:"signatureSize"`
	PublicKeySize uint64 `json:"publicKeySize"`

	Flags            uint32 `json:"flags"`
	HashtreeDisabled bool   `json:"hashtreeDisabled"`
	// VerificationDisabled 为 true 时该镜像不参与启动校验（常见于解锁后的设备）。
	VerificationDisabled bool `json:"verificationDisabled"`

	ReleaseString string `json:"releaseString"`

	Descriptors []Descriptor `json:"descriptors"`

	// Hash 是 header 里声明的 digest（不是对文件重算的），用于确认结构自洽。
	SelfHash string `json:"selfHash"`

	Notes []string `json:"notes"`
}

// Descriptor 是一个 vbmeta 描述符。
type Descriptor struct {
	Tag   uint64 `json:"tag"`
	Kind  string `json:"kind"`
	Class uint64 `json:"class"`

	// 通用
	PartitionName string `json:"partitionName,omitempty"`
	ImageSize     uint64 `json:"imageSize,omitempty"`
	Flags         uint32 `json:"flags,omitempty"`

	// hash
	HashAlgorithm string `json:"hashAlgorithm,omitempty"`
	Digest        string `json:"digest,omitempty"`
	Salt          string `json:"salt,omitempty"`

	// hashtree
	TreeOffset    uint64 `json:"treeOffset,omitempty"`
	TreeSize      uint64 `json:"treeSize,omitempty"`
	DataBlockSize uint32 `json:"dataBlockSize,omitempty"`
	HashBlockSize uint32 `json:"hashBlockSize,omitempty"`
	FecNumRoots   uint32 `json:"fecNumRoots,omitempty"`
	FecOffset     uint64 `json:"fecOffset,omitempty"`
	FecSize       uint64 `json:"fecSize,omitempty"`
	RootDigest    string `json:"rootDigest,omitempty"`
	DMVerityVer   uint32 `json:"dmVerityVersion,omitempty"`

	// kernel cmdline
	Cmdline string `json:"cmdline,omitempty"`

	// chain partition
	RollbackIndexLocation uint32 `json:"rollbackIndexLocation,omitempty"`
	PublicKeyLen          uint32 `json:"publicKeyLen,omitempty"`

	// property
	Key   string `json:"key,omitempty"`
	Value string `json:"value,omitempty"`

	Size uint64 `json:"size"`
}

// Parse 解析指定镜像。若镜像本身不是 vbmeta，会尝试在末尾寻找 AvbFooter。
func Parse(path string) (*Image, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	img, err := ParseBytes(data)
	if err != nil {
		return nil, err
	}
	img.Path = path
	return img, nil
}

// ParseBytes 从内存解析。
func ParseBytes(data []byte) (*Image, error) {
	if len(data) < HeaderSize {
		return nil, fmt.Errorf("文件只有 %d 字节，不足一个 vbmeta 头（%d 字节）", len(data), HeaderSize)
	}

	img := &Image{}

	// 裸 vbmeta 直接在开头；boot/system 这类镜像要读末尾的 footer。
	if bytes.Equal(data[:4], magicVBMeta) {
		img.VBMetaOffset = 0
		img.VBMetaSize = int64(len(data))
	} else {
		off, size, orig, err := findFooter(data)
		if err != nil {
			return nil, err
		}
		img.VBMetaOffset = off
		img.VBMetaSize = size
		img.FooterOriginalSize = orig
		img.Notes = append(img.Notes, fmt.Sprintf(
			"vbmeta 未在文件开头，通过末尾 AVBf footer 定位到偏移 %d（原始镜像 %d 字节）", off, orig))
		if off < 0 || off+size > int64(len(data)) || size < HeaderSize {
			return nil, fmt.Errorf("footer 指向的 vbmeta 区域越界：offset=%d size=%d", off, size)
		}
	}

	h := data[img.VBMetaOffset : img.VBMetaOffset+HeaderSize]
	if !bytes.Equal(h[:4], magicVBMeta) {
		return nil, fmt.Errorf("偏移 %d 处没有 AVB0 魔数，不是 vbmeta 镜像", img.VBMetaOffset)
	}

	be := binary.BigEndian
	img.VersionMajor = be.Uint32(h[4:])
	img.VersionMinor = be.Uint32(h[8:])
	authBlock := be.Uint64(h[12:])
	auxBlock := be.Uint64(h[20:])
	img.AlgorithmType = be.Uint32(h[28:])
	hashOff := be.Uint64(h[32:])
	img.HashSize = be.Uint64(h[40:])
	sigOff := be.Uint64(h[48:])
	img.SignatureSize = be.Uint64(h[56:])
	pkOff := be.Uint64(h[64:])
	img.PublicKeySize = be.Uint64(h[72:])
	pkmOff := be.Uint64(h[80:])
	pkmSize := be.Uint64(h[88:])
	descOff := be.Uint64(h[96:])
	descSize := be.Uint64(h[104:])
	img.RollbackIndex = be.Uint64(h[112:])
	img.Flags = be.Uint32(h[120:])
	img.RollbackIndexLocation = h[124]
	img.ReleaseString = trimNul(h[128:175])

	img.HashtreeDisabled = img.Flags&1 != 0
	img.VerificationDisabled = img.Flags&2 != 0
	if name, ok := algorithmNames[img.AlgorithmType]; ok {
		img.Algorithm = name
	} else {
		img.Algorithm = fmt.Sprintf("未知算法（%d）", img.AlgorithmType)
		img.Notes = append(img.Notes, "算法编号不在 libavb 已定义的 0–6 范围内，可能是定制签名方案")
	}
	if img.AlgorithmType == 0 {
		img.Notes = append(img.Notes, "算法为 NONE：该 vbmeta 没有签名，任何修改都不会被校验拦下")
	}
	if img.VerificationDisabled {
		img.Notes = append(img.Notes, "verification 已被禁用：开机不会校验分区完整性，常见于解锁或刷入第三方 vbmeta 后")
	}
	if img.HashtreeDisabled {
		img.Notes = append(img.Notes, "hashtree 已被禁用：dm-verity 不生效，system/vendor 被改动不会被发现")
	}

	// 公钥元数据（avbtool 的 --public_key_metadata）通常是 avb_pkmd.bin。
	if pkmSize > 0 && pkmOff+pkmSize <= uint64(len(h)) {
		img.Notes = append(img.Notes, fmt.Sprintf("public key metadata 存在（%d 字节）", pkmSize))
	}
	_ = pkOff
	_ = pkmOff
	_ = sigOff

	// hash 与签名都相对 header 起点。
	if img.HashSize > 0 && hashOff+img.HashSize <= uint64(HeaderSize) {
		img.SelfHash = hexOrEmpty(h[hashOff : hashOff+img.HashSize])
	}

	// 描述符区总长必须装得下 auth block 之外的部分。
	if descSize > 0 {
		start := int64(img.VBMetaOffset) + int64(descOff)
		end := start + int64(descSize)
		if end > int64(len(data)) {
			return nil, fmt.Errorf("描述符区越界：offset=%d size=%d，文件只有 %d 字节", descOff, descSize, len(data))
		}
		descs, err := parseDescriptors(data[start:end])
		if err != nil {
			img.Notes = append(img.Notes, "描述符解析中断："+err.Error())
		}
		img.Descriptors = descs
	} else {
		img.Notes = append(img.Notes, "vbmeta 里没有任何描述符，通常是只用于下发 flags 的空壳（super 的 vbmeta 常见这种）")
	}

	// auth block 与 aux block 的结构一致性检查，纯提示，不拒绝解析。
	expectAuth := uint64(HeaderSize) + img.HashSize + img.SignatureSize + img.PublicKeySize
	if authBlock > 0 && authBlock != expectAuth {
		img.Notes = append(img.Notes, fmt.Sprintf(
			"authentication block 声明 %d 字节，按 header 里的偏移推算应为 %d，结构可能被第三方工具改写", authBlock, expectAuth))
	}
	_ = auxBlock

	sort.SliceStable(img.Descriptors, func(i, j int) bool {
		return img.Descriptors[i].Kind < img.Descriptors[j].Kind
	})
	return img, nil
}

// findFooter 在文件末尾寻找 AvbFooter。
func findFooter(data []byte) (off, size, orig int64, err error) {
	if len(data) < footerSize {
		return 0, 0, 0, fmt.Errorf("文件不足 %d 字节，既不是 vbmeta 也没有 footer", footerSize)
	}
	f := data[len(data)-footerSize:]
	if !bytes.Equal(f[:4], magicFooter) {
		return 0, 0, 0, fmt.Errorf("文件开头没有 AVB0、末尾没有 AVBf，不是 AVB 镜像")
	}
	be := binary.BigEndian
	orig = int64(be.Uint64(f[12:]))
	off = int64(be.Uint64(f[20:]))
	size = int64(be.Uint64(f[28:]))
	return off, size, orig, nil
}

// parseDescriptors 遍历描述符链。
func parseDescriptors(b []byte) ([]Descriptor, error) {
	var out []Descriptor
	be := binary.BigEndian
	for pos := 0; pos+16 <= len(b); {
		tag := be.Uint64(b[pos:])
		following := be.Uint64(b[pos+8:])
		total := 16 + following
		if following > uint64(len(b)-pos-16) {
			return out, fmt.Errorf("偏移 %d 的描述符声明长度 %d，超出所在区域", pos, following)
		}
		rec := b[pos : pos+int(total)]
		d := Descriptor{Tag: tag, Size: total}
		cls, field := splitTag(tag)
		d.Class = cls
		if name, ok := descNames[field]; ok {
			d.Kind = name
		} else {
			d.Kind = fmt.Sprintf("unknown(%d)", field)
		}
		parseDescriptorBody(&d, field, rec[16:])
		out = append(out, d)
		if total == 0 {
			break
		}
		pos += int(total)
		// 描述符按 8 字节对齐。
		if pos%8 != 0 {
			pos += 8 - pos%8
		}
	}
	return out, nil
}

// splitTag 拆出 (class, field)。
//
// libavb 的编码是 field<<16 | class，但不同年代的工具链出现过把 class 放到
// 高 16 位的变体，这里两种都认，避免因为编码差异整段解析失败。
func splitTag(tag uint64) (class, field uint64) {
	lowClass := tag & 0xffff
	lowField := (tag >> 16) & 0xffff
	if lowField <= DescChain && lowClass == 0 {
		return lowClass, lowField
	}
	highClass := (tag >> 48) & 0xffff
	highField := (tag >> 32) & 0xffff
	if highField <= DescChain {
		return highClass, highField
	}
	return lowClass, lowField
}

func parseDescriptorBody(d *Descriptor, kind uint64, b []byte) {
	be := binary.BigEndian
	switch kind {
	case DescProperty:
		if len(b) < 16 {
			return
		}
		klen := be.Uint64(b[0:])
		vlen := be.Uint64(b[8:])
		if 16+klen+vlen > uint64(len(b)) {
			return
		}
		d.Key = string(b[16 : 16+klen])
		d.Value = string(b[16+klen : 16+klen+vlen])

	case DescHash:
		// 定长部分 116 字节，后面紧跟 partition_name + salt + digest。
		//
		// 字段顺序在不同年代的实现里出现过两种：AOSP libavb 现行布局是
		// image_size, flags, name_len, salt_len, digest_len, hash_algorithm；
		// 早期 avbtool 变体则是 image_size, hash_algorithm, name_len…, flags。
		// 读错的话摘要和算法会整段变成乱码，所以两种都算一遍、取自洽的那个。
		const fixed = 116
		if len(b) < fixed {
			return
		}
		d.ImageSize = be.Uint64(b[0:])
		tail := b[fixed:]

		type hashLayout struct {
			flags     uint32
			nameLen   uint32
			saltLen   uint32
			digestLen uint32
			algorithm string
			score     int
		}
		cands := [2]hashLayout{
			{
				// image_size(0:8) flags(8) name_len(12) salt_len(16) digest_len(20)
				// hash_algorithm(24:56) reserved(56:116)
				flags: be.Uint32(b[8:]), nameLen: be.Uint32(b[12:]), saltLen: be.Uint32(b[16:]),
				digestLen: be.Uint32(b[20:]), algorithm: trimNul(b[24:56]),
			},
			{
				// image_size(0:8) hash_algorithm(8:40) name_len(40) salt_len(44)
				// digest_len(48) flags(52) reserved(56:116)
				flags: be.Uint32(b[52:]), nameLen: be.Uint32(b[40:]), saltLen: be.Uint32(b[44:]),
				digestLen: be.Uint32(b[48:]), algorithm: trimNul(b[8:40]),
			},
		}
		for i := range cands {
			c := &cands[i]
			sum := uint64(c.nameLen) + uint64(c.saltLen) + uint64(c.digestLen)
			if sum == uint64(len(tail)) {
				c.score += 3
			} else if sum < uint64(len(tail)) {
				c.score-- // 说明尾部有没解释的字节，多半是字段读歪了
			} else {
				c.score -= 3
			}
			if c.nameLen == 0 || c.nameLen > 64 {
				c.score -= 2
			}
			if isHashAlgorithmName(c.algorithm) {
				c.score += 2
			}
		}
		best := cands[0]
		if cands[1].score > best.score {
			best = cands[1]
		}
		if best.score <= 0 {
			// 两种布局都对不上，宁可留空也不输出一段看起来像摘要的垃圾。
			return
		}
		d.Flags = best.flags
		d.HashAlgorithm = best.algorithm
		sum := uint64(best.nameLen) + uint64(best.saltLen) + uint64(best.digestLen)
		if sum > uint64(len(tail)) {
			return
		}
		d.PartitionName = string(tail[:best.nameLen])
		rest := tail[best.nameLen:]
		d.Salt = hexOrEmpty(rest[:best.saltLen])
		d.Digest = hexOrEmpty(rest[best.saltLen : best.saltLen+best.digestLen])

	case DescHashtree:
		// 4 + 8*3 + 4*3 + 8*2 + 32 + 4*4 + 60
		if len(b) < 4+24+12+16+32+16+60 {
			return
		}
		d.DMVerityVer = be.Uint32(b[0:])
		d.ImageSize = be.Uint64(b[4:])
		d.TreeOffset = be.Uint64(b[12:])
		d.TreeSize = be.Uint64(b[20:])
		d.DataBlockSize = be.Uint32(b[28:])
		d.HashBlockSize = be.Uint32(b[32:])
		d.FecNumRoots = be.Uint32(b[36:])
		d.FecOffset = be.Uint64(b[40:])
		d.FecSize = be.Uint64(b[48:])
		d.HashAlgorithm = trimNul(b[56:88])
		nameLen := be.Uint32(b[88:])
		saltLen := be.Uint32(b[92:])
		rootLen := be.Uint32(b[96:])
		d.Flags = be.Uint32(b[100:])
		tail := b[164:]
		if uint64(nameLen+saltLen+rootLen) > uint64(len(tail)) {
			return
		}
		d.PartitionName = string(tail[:nameLen])
		rest := tail[nameLen:]
		d.Salt = hexOrEmpty(rest[:saltLen])
		d.RootDigest = hexOrEmpty(rest[saltLen : saltLen+rootLen])

	case DescKernelCmdline:
		if len(b) < 8 {
			return
		}
		d.Flags = be.Uint32(b[0:])
		l := be.Uint32(b[4:])
		if 8+uint64(l) <= uint64(len(b)) {
			d.Cmdline = trimNul(b[8 : 8+l])
		}

	case DescChain:
		// 4 rollback_index_location + 4 name_len + 4 public_key_len + 4 flags + 64 reserved
		const fixed = 16 + 64
		if len(b) < fixed {
			return
		}
		d.RollbackIndexLocation = be.Uint32(b[0:])
		nameLen := be.Uint32(b[4:])
		pkLen := be.Uint32(b[8:])
		d.Flags = be.Uint32(b[12:])
		tail := b[fixed:]
		if uint64(nameLen) > uint64(len(tail)) {
			return
		}
		d.PartitionName = string(tail[:nameLen])
		d.PublicKeyLen = pkLen
	}
}

func trimNul(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(bytes.TrimSpace(b))
}

// isHashAlgorithmName 判断一个字段像不像摘要算法名，用来给布局打分。
func isHashAlgorithmName(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "sha256", "sha512", "blake2b", "sha1", "md5":
		return true
	}
	return false
}

func hexOrEmpty(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	const hexdigits = "0123456789abcdef"
	var sb strings.Builder
	sb.Grow(len(b) * 2)
	for _, c := range b {
		sb.WriteByte(hexdigits[c>>4])
		sb.WriteByte(hexdigits[c&0xf])
	}
	return sb.String()
}
