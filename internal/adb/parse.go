package adb

import "strings"

// splitPkgLine 解析 `pm list packages -f` 的一行：package:<apk path>=<package name>。
//
// Android 14 起的应用安装路径本身带等号：
//
//	package:/data/app/~~AbCdEf==/com.example.app-XyZ123==/base.apk=com.example.app
//
// 从第一个等号切会把路径截断成 "/data/app/~~AbCdEf"，包名切成
// "=/com.example.app-XyZ123==/base.apk=com.example.app"，后续所有按包名的操作都会失败。
// 因此必须以**最后一个**等号作为分隔点。
func splitPkgLine(line string) (path, pkg string, ok bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "package:") {
		return "", "", false
	}
	body := strings.TrimPrefix(line, "package:")
	idx := strings.LastIndex(body, "=")
	if idx <= 0 || idx == len(body)-1 {
		return "", "", false
	}
	return body[:idx], body[idx+1:], true
}
