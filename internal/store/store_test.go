package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolate 把配置目录重定向到临时目录，避免测试污染用户真实配置。
func isolate(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)         // Windows
	t.Setenv("XDG_CONFIG_HOME", dir) // Linux
	t.Setenv("HOME", dir)            // macOS / 兜底
	return dir
}

func TestDefault(t *testing.T) {
	cfg := Default()

	if cfg.CacheDir == "" || cfg.OutputDir == "" {
		t.Fatal("默认配置必须带缓存与输出目录")
	}
	if !cfg.AutoRefresh || !cfg.ConfirmDanger || !cfg.KeepAwake {
		t.Fatalf("默认开关不符合预期：%+v", cfg)
	}
	if cfg.Theme != "system" || cfg.ThemeExplicit {
		t.Fatalf("默认应跟随系统（system 且未手动选择），实际 %q / %v", cfg.Theme, cfg.ThemeExplicit)
	}
	if cfg.CpuIntervalMS != 1500 {
		t.Fatalf("默认 CPU 间隔应为 1500ms，实际 %d", cfg.CpuIntervalMS)
	}
	if cfg.UIScale != 1 {
		t.Fatalf("默认界面缩放应为 1，实际 %v", cfg.UIScale)
	}
	if cfg.PushDir != cfg.OutputDir {
		t.Fatalf("PushDir 应默认等于 OutputDir：%q vs %q", cfg.PushDir, cfg.OutputDir)
	}
}

func TestLoadMissingFileReturnsDefault(t *testing.T) {
	isolate(t)

	cfg := Load()
	if cfg.CacheDir == "" || cfg.OutputDir == "" {
		t.Fatal("配置文件不存在时应返回完整默认值")
	}
	if cfg.Theme != "system" {
		t.Fatalf("默认主题错误：%q", cfg.Theme)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	isolate(t)

	in := Default()
	in.CustomAdb = "/opt/adb"
	in.CustomFastboot = "/opt/fastboot"
	in.Theme = "light"
	in.ThemeExplicit = true
	in.UIScale = 1.25
	in.CpuIntervalMS = 800
	in.HideIds = true
	in.OtaUserAgent = "Mozilla/5.0 test"

	if err := Save(in); err != nil {
		t.Fatalf("保存失败：%v", err)
	}

	got := Load()
	if got.CustomAdb != in.CustomAdb || got.CustomFastboot != in.CustomFastboot {
		t.Fatalf("工具路径丢失：%+v", got)
	}
	if got.Theme != "light" || got.UIScale != 1.25 || got.CpuIntervalMS != 800 || !got.HideIds {
		t.Fatalf("界面配置丢失：%+v", got)
	}
	if got.OtaUserAgent != in.OtaUserAgent {
		t.Fatalf("OTA UA 丢失：%q", got.OtaUserAgent)
	}
}

// TestLoadCorruptFile 损坏的配置文件不能让应用起不来，应回退默认值。
func TestLoadCorruptFile(t *testing.T) {
	isolate(t)

	dir := configDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{ this is not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := Load()
	if cfg.CacheDir == "" || cfg.OutputDir == "" || cfg.Theme == "" {
		t.Fatalf("损坏配置应回退默认值，实际 %+v", cfg)
	}
}

// TestLoadFillsEmptyFields 老版本配置缺少新字段时要有合理兜底。
func TestLoadFillsEmptyFields(t *testing.T) {
	isolate(t)

	dir := configDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// 只写少数字段，模拟旧版本配置
	raw := `{"customAdb":"/adb","theme":""}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := Load()
	if cfg.CustomAdb != "/adb" {
		t.Fatalf("已有字段不应被覆盖：%q", cfg.CustomAdb)
	}
	if cfg.Theme != "system" || cfg.UIScale != 1 || cfg.CpuIntervalMS != 1500 || cfg.StartPage != "/" {
		t.Fatalf("缺失字段未回填默认值：%+v", cfg)
	}
	if cfg.CacheDir == "" || cfg.OutputDir == "" {
		t.Fatal("目录字段未回填")
	}
	if cfg.PushDir != cfg.OutputDir {
		t.Fatalf("PushDir 未跟随 OutputDir：%q", cfg.PushDir)
	}
}

// TestLoadMigratesImplicitTheme 1.5.x 把默认主题写死成 dark，且没有 themeExplicit 标记。
// 升级后这类「从没主动选过」的配置要回到跟随系统，手动选过的一律保留。
func TestLoadMigratesImplicitTheme(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"旧版默认值视为未选择", `{"theme":"dark"}`, "system"},
		{"旧版浅色同样视为未选择", `{"theme":"light"}`, "system"},
		{"手动选过则保留", `{"theme":"dark","themeExplicit":true}`, "dark"},
		{"手动选过浅色则保留", `{"theme":"light","themeExplicit":true}`, "light"},
		{"全新配置", `{}`, "system"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			isolate(t)

			dir := configDir()
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(c.raw), 0o644); err != nil {
				t.Fatal(err)
			}

			if got := Load().Theme; got != c.want {
				t.Fatalf("主题迁移错误：want %q，got %q", c.want, got)
			}
		})
	}
}

func TestSaveCreatesDir(t *testing.T) {
	isolate(t)

	if err := Save(Default()); err != nil {
		t.Fatalf("保存失败：%v", err)
	}
	data, err := os.ReadFile(filepath.Join(configDir(), "config.json"))
	if err != nil {
		t.Fatalf("配置文件未生成：%v", err)
	}
	if !strings.Contains(string(data), "\"theme\"") {
		t.Fatalf("配置内容异常：%s", data)
	}
}

func TestConfigPathUnderConfigDir(t *testing.T) {
	isolate(t)

	if !strings.HasSuffix(configDir(), "DroidForge") {
		t.Fatalf("配置目录命名异常：%s", configDir())
	}
	if !strings.HasSuffix(configPath(), "config.json") {
		t.Fatalf("配置文件名异常：%s", configPath())
	}
}
