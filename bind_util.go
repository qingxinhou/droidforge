package main

import (
	"os"
	"os/exec"
	"runtime"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// ---------------------------------------------------------------------------
// 文件选择
// ---------------------------------------------------------------------------

// PickFile 打开文件选择框。
func (a *App) PickFile(title, pattern string) (string, error) {
	filters := []wailsruntime.FileFilter{
		{DisplayName: "支持的文件", Pattern: pattern},
		{DisplayName: "所有文件", Pattern: "*.*"},
	}
	return wailsruntime.OpenFileDialog(a.appCtx(), wailsruntime.OpenDialogOptions{
		Title:   title,
		Filters: filters,
	})
}

// PickDirectory 打开目录选择框。
func (a *App) PickDirectory(title string) (string, error) {
	return wailsruntime.OpenDirectoryDialog(a.appCtx(), wailsruntime.OpenDialogOptions{Title: title})
}

// PickSaveFile 打开保存文件框。
func (a *App) PickSaveFile(title, defaultName string) (string, error) {
	return wailsruntime.SaveFileDialog(a.appCtx(), wailsruntime.SaveDialogOptions{
		Title:           title,
		DefaultFilename: defaultName,
	})
}

// OpenPath 用系统文件管理器打开路径。
func (a *App) OpenPath(path string) error {
	if _, err := os.Stat(path); err != nil {
		return err
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", path)
	case "darwin":
		cmd = exec.Command("open", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Start()
}

// OpenURL 用默认浏览器打开链接。
func (a *App) OpenURL(url string) {
	wailsruntime.BrowserOpenURL(a.appCtx(), url)
}
