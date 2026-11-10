package root

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// releaseAsset 是 GitHub Release 中的一个资源。
type releaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

type releaseResp struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

// DownloadSolutionAPK 下载指定方案的最新官方 APK，返回本地路径。
func DownloadSolutionAPK(ctx context.Context, sol Solution, dir, mirror string, onProgress func(int)) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	rel, err := fetchLatestRelease(ctx, sol.Repo)
	if err != nil {
		// 网络不可用时给出可操作提示
		return "", fmt.Errorf("获取 %s 版本信息失败：%w\n可尝试在设置中配置 GitHub 加速前缀，或手动下载 APK 后选择本地文件", sol.Name, err)
	}

	re, err := regexp.Compile("(?i)" + sol.AssetGlob)
	if err != nil {
		return "", err
	}

	var candidates []releaseAsset
	for _, a := range rel.Assets {
		if re.MatchString(a.Name) {
			candidates = append(candidates, a)
		}
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("%s 的最新版本 %s 中没有匹配 %s 的安装包", sol.Name, rel.TagName, sol.AssetGlob)
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].Size > candidates[j].Size })

	asset := candidates[0]
	dst := filepath.Join(dir, asset.Name)
	if st, err := os.Stat(dst); err == nil && st.Size() == asset.Size {
		return dst, nil // 已缓存
	}

	url := asset.URL
	if mirror != "" && strings.HasPrefix(url, "http") {
		url = strings.TrimRight(mirror, "/") + "/" + url
	}
	if err := downloadFile(ctx, url, dst, onProgress); err != nil {
		return "", fmt.Errorf("下载 %s 失败：%w", asset.Name, err)
	}
	return dst, nil
}

func fetchLatestRelease(ctx context.Context, repo string) (*releaseResp, error) {
	api := "https://api.github.com/repos/" + repo + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "DroidForge/1.0")
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("GitHub API 限流（HTTP %d），请稍后重试或配置 GITHUB_TOKEN", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API 返回 HTTP %d", resp.StatusCode)
	}

	var out releaseResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

// downloadFile 带进度的文件下载。
func downloadFile(ctx context.Context, url, dst string, onProgress func(int)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "DroidForge/1.0")

	client := &http.Client{Timeout: 0}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	tmp := dst + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer func() {
		f.Close()
		os.Remove(tmp)
	}()

	total := resp.ContentLength
	var written int64
	buf := make([]byte, 128*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return werr
			}
			written += int64(n)
			if total > 0 && onProgress != nil {
				onProgress(int(written * 100 / total))
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}
