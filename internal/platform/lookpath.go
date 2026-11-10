package platform

import "os/exec"

// execLookPath 单独抽出，便于测试替换。
func execLookPath(name string) (string, error) {
	return exec.LookPath(name)
}
