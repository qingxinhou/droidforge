package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"droidforge/internal/platform"
	"droidforge/internal/store"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// ---------------------------------------------------------------------------
// 配置
// ---------------------------------------------------------------------------

// GetConfig 返回当前配置。
func (a *App) GetConfig() store.Config { return a.config() }

// SaveConfig 保存配置。
func (a *App) SaveConfig(cfg store.Config) error {
	cfg.CacheDir = strings.TrimSpace(cfg.CacheDir)
	cfg.OutputDir = strings.TrimSpace(cfg.OutputDir)

	// 免责声明同意记录只允许前进：前端的配置快照可能早于用户刚完成的同意动作，
	// 直接覆盖会让用户下次启动又被拦一次。
	if cur := a.config(); cfg.DisclaimerVersion < cur.DisclaimerVersion {
		cfg.DisclaimerVersion = cur.DisclaimerVersion
		cfg.DisclaimerAcceptedAt = cur.DisclaimerAcceptedAt
	}

	saved := a.updateConfig(func(c *store.Config) { *c = cfg })
	a.setTools(platform.Detect(cfg.CustomAdb, cfg.CustomFastboot))

	if err := store.Save(saved); err != nil {
		a.log("err", "保存配置失败："+err.Error())
		return err
	}
	_ = os.MkdirAll(cfg.CacheDir, 0o755)
	_ = os.MkdirAll(cfg.OutputDir, 0o755)
	a.log("ok", "配置已保存")
	return nil
}

// RevealInFolder 在文件管理器中定位文件。
func (a *App) RevealInFolder(path string) {
	if runtime.GOOS == "windows" {
		_ = exec.Command("explorer", "/select,", path).Start()
		return
	}
	_ = a.OpenPath(filepath.Dir(path))
}

// CopyText 复制文本到剪贴板。
func (a *App) CopyText(text string) {
	wailsruntime.ClipboardSetText(a.appCtx(), text)
}

// SaveTextFile 弹出保存对话框并写入文本（用于导出日志）。
func (a *App) SaveTextFile(defaultName, content string) (string, error) {
	path, err := wailsruntime.SaveFileDialog(a.appCtx(), wailsruntime.SaveDialogOptions{
		Title:           "保存文件",
		DefaultFilename: defaultName,
	})
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", nil
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		a.log("err", "保存失败："+err.Error())
		return "", err
	}
	a.log("ok", "已保存："+path)
	return path, nil
}

func fileSize(path string) int64 {
	st, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return st.Size()
}
