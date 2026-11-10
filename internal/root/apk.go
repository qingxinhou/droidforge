package root

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func osMkdirAll(dir string) error { return os.MkdirAll(dir, 0o755) }

// prepareToolFiles 从官方 APK 中解出补丁所需的 ARM 二进制与脚本。
func prepareToolFiles(apkPath, family, outDir string) ([]string, error) {
	zr, err := zip.OpenReader(apkPath)
	if err != nil {
		return nil, fmt.Errorf("打开 APK 失败：%w", err)
	}
	defer zr.Close()

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}

	// 索引所有条目（统一小写用于匹配，保留原始名称用于读取）
	index := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		index[strings.ToLower(f.Name)] = f
	}

	var out []string
	write := func(f *zip.File, name string) error {
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()
		dst := filepath.Join(outDir, name)
		outFile, err := os.Create(dst)
		if err != nil {
			return err
		}
		defer outFile.Close()
		if _, err := io.Copy(outFile, rc); err != nil {
			return err
		}
		out = append(out, dst)
		return nil
	}

	switch family {
	case "magisk":
		// 所有 arm64-v8a 下的 lib*.so 去掉前缀/后缀即为可执行名
		for lower, f := range index {
			if !strings.HasPrefix(lower, "lib/arm64-v8a/lib") || !strings.HasSuffix(lower, ".so") {
				continue
			}
			base := filepath.Base(lower)
			name := strings.TrimSuffix(strings.TrimPrefix(base, "lib"), ".so")
			if name == "" {
				continue
			}
			if err := write(f, name); err != nil {
				return nil, err
			}
		}
		// 补丁脚本与 stub
		for _, want := range []string{"assets/boot_patch.sh", "assets/util_functions.sh", "assets/stub.apk"} {
			if f, ok := index[want]; ok {
				if err := write(f, filepath.Base(want)); err != nil {
					return nil, err
				}
			}
		}
		if len(out) < 2 {
			return nil, fmt.Errorf("APK 内未找到 magiskboot / boot_patch.sh，可能不是有效的 Magisk 安装包")
		}

	case "kernelsu":
		if f, ok := index["lib/arm64-v8a/libksud.so"]; ok {
			if err := write(f, "ksud"); err != nil {
				return nil, err
			}
		} else {
			// 兜底：模糊匹配
			for lower, f := range index {
				if strings.Contains(lower, "ksud") && strings.HasSuffix(lower, ".so") {
					if err := write(f, "ksud"); err != nil {
						return nil, err
					}
					break
				}
			}
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("APK 内未找到 ksud，可能不是有效的 KernelSU 安装包")
		}

	case "apatch":
		if f, ok := index["lib/arm64-v8a/libapd.so"]; ok {
			if err := write(f, "apd"); err != nil {
				return nil, err
			}
		} else {
			for lower, f := range index {
				if strings.Contains(lower, "apd") && strings.HasSuffix(lower, ".so") {
					if err := write(f, "apd"); err != nil {
						return nil, err
					}
					break
				}
			}
		}
		// KernelPatch 相关
		for lower, f := range index {
			if strings.Contains(lower, "kpatch") && strings.HasSuffix(lower, ".so") {
				if err := write(f, "kpatch"); err != nil {
					return nil, err
				}
				break
			}
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("APK 内未找到 apd，可能不是有效的 APatch 安装包")
		}

	default:
		return nil, fmt.Errorf("未知方案族：%s", family)
	}

	return out, nil
}
