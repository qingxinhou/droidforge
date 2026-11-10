package main

import (
	"fmt"
	"time"

	"droidforge/internal/store"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// ---------------------------------------------------------------------------
// 风险提示与免责声明
//
// 协议正文由前端 frontend/src/legal.ts 维护，后端只持久化「已同意的版本号」，
// 避免同一份文本在两个语言里各写一遍导致不一致。
// ---------------------------------------------------------------------------

// AcceptDisclaimer 记录用户已同意指定版本的免责声明。
func (a *App) AcceptDisclaimer(version int) error {
	if version <= 0 {
		return fmt.Errorf("无效的免责声明版本")
	}

	now := time.Now().Format(time.RFC3339)
	cfg := a.updateConfig(func(c *store.Config) {
		c.DisclaimerVersion = version
		c.DisclaimerAcceptedAt = now
	})
	if err := store.Save(cfg); err != nil {
		a.log("err", "保存免责声明记录失败："+err.Error())
		return err
	}

	a.log("info", fmt.Sprintf("已接受风险提示与免责声明 v%d", version))
	return nil
}

// DeclineDisclaimer 在用户拒绝协议时退出程序。
func (a *App) DeclineDisclaimer() {
	if ctx := a.appCtx(); ctx != nil {
		a.log("warn", "未接受免责声明，程序退出")
		wailsruntime.Quit(ctx)
	}
}

// ResetDisclaimer 撤销同意记录，使下次启动重新弹出协议。
func (a *App) ResetDisclaimer() error {
	cfg := a.updateConfig(func(c *store.Config) {
		c.DisclaimerVersion = 0
		c.DisclaimerAcceptedAt = ""
	})
	if err := store.Save(cfg); err != nil {
		return err
	}
	a.log("warn", "已撤销免责声明同意记录")
	return nil
}
