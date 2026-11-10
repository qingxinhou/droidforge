package remote

import (
	"os"
	"path/filepath"
)

// createFile 创建（覆盖）目标文件，必要时创建父目录。
func createFile(dst string) (*os.File, error) {
	if dir := filepath.Dir(dst); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	return os.Create(dst)
}
