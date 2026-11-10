package adb

import "testing"

func TestSplitPkgLine(t *testing.T) {
	cases := []struct {
		name     string
		line     string
		wantPath string
		wantPkg  string
		wantOK   bool
	}{
		{
			name:     "Android 13 及以前的路径",
			line:     "package:/data/app/MIUIHealth/base.apk=com.mi.health",
			wantPath: "/data/app/MIUIHealth/base.apk",
			wantPkg:  "com.mi.health",
			wantOK:   true,
		},
		{
			name:     "Android 14+ 路径含双等号",
			line:     "package:/data/app/~~AbCdEf==/com.example.app-XyZ123==/base.apk=com.example.app",
			wantPath: "/data/app/~~AbCdEf==/com.example.app-XyZ123==/base.apk",
			wantPkg:  "com.example.app",
			wantOK:   true,
		},
		{
			name:     "系统分区上的应用",
			line:     "package:/product/overlay/WallpaperOverlay.apk=com.miui.miwallpaper.wallpaperoverlay.config.overlay",
			wantPath: "/product/overlay/WallpaperOverlay.apk",
			wantPkg:  "com.miui.miwallpaper.wallpaperoverlay.config.overlay",
			wantOK:   true,
		},
		{
			name:     "带前后空白的行",
			line:     "  package:/system/app/Foo/Foo.apk=com.android.foo  ",
			wantPath: "/system/app/Foo/Foo.apk",
			wantPkg:  "com.android.foo",
			wantOK:   true,
		},
		{name: "没有等号", line: "package:/data/app/Foo/base.apk", wantOK: false},
		{name: "包名为空", line: "package:/data/app/Foo/base.apk=", wantOK: false},
		{name: "缺少 package 前缀", line: "/data/app/Foo/base.apk=com.foo", wantOK: false},
		{name: "空行", line: "", wantOK: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path, pkg, ok := splitPkgLine(c.line)
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v", ok, c.wantOK)
			}
			if !ok {
				return
			}
			if path != c.wantPath {
				t.Errorf("path = %q, want %q", path, c.wantPath)
			}
			if pkg != c.wantPkg {
				t.Errorf("pkg = %q, want %q", pkg, c.wantPkg)
			}
		})
	}
}
