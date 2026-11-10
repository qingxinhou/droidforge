// Package store 负责应用配置的读写（JSON 持久化到用户配置目录）。
package store

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config 是应用配置。
type Config struct {
	CustomAdb      string `json:"customAdb"`
	CustomFastboot string `json:"customFastboot"`
	Mirror         string `json:"mirror"`    // GitHub 加速前缀
	CacheDir       string `json:"cacheDir"`  // 下载与解包缓存
	OutputDir      string `json:"outputDir"` // 提取镜像输出目录
	LastSerial     string `json:"lastSerial"`
	AutoRefresh    bool   `json:"autoRefresh"`
	ConfirmDanger  bool   `json:"confirmDanger"`
	Theme          string `json:"theme"`         // dark / light / system
	ThemeExplicit  bool   `json:"themeExplicit"` // 用户是否手动选过主题；false 表示跟随系统
	KeepAwake      bool   `json:"keepAwake"`
	OtaUserAgent   string `json:"otaUserAgent"` // 远程 OTA 请求的自定义 UA
	OtaReferer     string `json:"otaReferer"`   // 远程 OTA 请求的 Referer

	// 法律与合规
	DisclaimerVersion    int    `json:"disclaimerVersion"`    // 已同意的免责声明版本，0 表示未同意
	DisclaimerAcceptedAt string `json:"disclaimerAcceptedAt"` // 同意时间（RFC3339）

	// 界面与工具行为
	StartPage        string  `json:"startPage"`        // 启动页路由
	UIScale          float64 `json:"uiScale"`          // 界面缩放 0.8–1.4
	CpuIntervalMS    int     `json:"cpuIntervalMs"`    // CPU 频率刷新间隔
	PushDir          string  `json:"pushDir"`          // 拉取 / 截图默认保存目录
	HideIds          bool    `json:"hideIds"`          // 隐藏序列号与设备标识
	InstallReinstall bool    `json:"installReinstall"` // 安装时默认覆盖
}

// Default 返回默认配置。
func Default() Config {
	return Config{
		CacheDir:         defaultCacheDir(),
		OutputDir:        defaultOutputDir(),
		AutoRefresh:      true,
		ConfirmDanger:    true,
		Theme:            "system",
		KeepAwake:        true,
		StartPage:        "/",
		UIScale:          1,
		CpuIntervalMS:    1500,
		PushDir:          defaultOutputDir(),
		InstallReinstall: true,
	}
}

func configDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = "."
	}
	return filepath.Join(base, "DroidForge")
}

func defaultCacheDir() string {
	return filepath.Join(configDir(), "cache")
}

func defaultOutputDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(configDir(), "output")
	}
	return filepath.Join(home, "DroidForge")
}

func configPath() string {
	return filepath.Join(configDir(), "config.json")
}

// Load 读取配置；文件不存在时返回默认值。
func Load() Config {
	cfg := Default()
	data, err := os.ReadFile(configPath())
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(data, &cfg)
	if cfg.CacheDir == "" {
		cfg.CacheDir = defaultCacheDir()
	}
	if cfg.OutputDir == "" {
		cfg.OutputDir = defaultOutputDir()
	}
	// 1.5.x 的默认值是深色，且没有「手动选过」的标记。升级到 1.6 时把这类配置
	// 收回「跟随系统」，但用户自己点过主题的（themeExplicit=true）一律保留原值。
	if !cfg.ThemeExplicit {
		cfg.Theme = "system"
	}
	if cfg.Theme == "" {
		cfg.Theme = "system"
	}
	if cfg.UIScale <= 0 {
		cfg.UIScale = 1
	}
	if cfg.CpuIntervalMS <= 0 {
		cfg.CpuIntervalMS = 1500
	}
	if cfg.StartPage == "" {
		cfg.StartPage = "/"
	}
	if cfg.PushDir == "" {
		cfg.PushDir = cfg.OutputDir
	}
	return cfg
}

// Save 写入配置。
func Save(cfg Config) error {
	if err := os.MkdirAll(configDir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(), data, 0o644)
}

// CachePath 返回缓存目录下的子路径，并确保目录存在。
func CachePath(sub string) (string, error) {
	dir := filepath.Join(Load().CacheDir, sub)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}
