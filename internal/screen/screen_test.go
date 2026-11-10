package screen

import "testing"

func TestBuildArgsDefaults(t *testing.T) {
	args := BuildArgs(Options{Serial: "ABC123"})
	want := []string{"--serial=ABC123"}
	if len(args) != len(want) || args[0] != want[0] {
		t.Errorf("默认参数 = %v，期望 %v", args, want)
	}
}

func TestBuildArgsFull(t *testing.T) {
	args := BuildArgs(Options{
		Serial:        "S1",
		AdbPath:       `C:\platform-tools\adb.exe`,
		BitRate:       "16M",
		MaxSize:       1024,
		MaxFPS:        60,
		TurnScreenOff: true,
		StayAwake:     true,
		NoAudio:       true,
		AlwaysOnTop:   true,
		Record:        `C:\out\rec.mp4`,
	})
	joined := ""
	for _, a := range args {
		joined += a + "\n"
	}
	for _, want := range []string{
		"--serial=S1",
		`--adb=C:\platform-tools\adb.exe`,
		"--video-bit-rate=16M",
		"--max-size=1024",
		"--max-fps=60",
		"--turn-screen-off",
		"--stay-awake",
		"--no-audio",
		"--always-on-top",
		`--record=C:\out\rec.mp4`,
	} {
		if !containsLine(joined, want) {
			t.Errorf("缺少参数 %q，实际：%v", want, args)
		}
	}
}

func TestBuildArgsSkipsZero(t *testing.T) {
	args := BuildArgs(Options{MaxSize: 0, MaxFPS: 0})
	for _, a := range args {
		if len(a) > 10 && a[:10] == "--max-size" {
			t.Errorf("maxSize 为 0 时不该传参：%v", args)
		}
	}
}

func containsLine(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
