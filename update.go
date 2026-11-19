package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var appVersion = "1.5.0"

const releasesAPI = "https://api.github.com/repos/thegreatjx/fastDictionary/releases/latest"

var errNoRelease = errors.New("GitHub 上还没有可用的发布版本")

type releaseInfo struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Name    string `json:"name"`
}

func fetchLatestRelease(ctx context.Context, client *http.Client, url string) (releaseInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return releaseInfo{}, err
	}
	req.Header.Set("User-Agent", "fastDictionary/"+appVersion)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return releaseInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return releaseInfo{}, errNoRelease
	}
	if resp.StatusCode != http.StatusOK {
		return releaseInfo{}, fmt.Errorf("GitHub 返回 HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return releaseInfo{}, err
	}
	var out releaseInfo
	if err := json.Unmarshal(body, &out); err != nil {
		return releaseInfo{}, fmt.Errorf("无法解析更新信息: %w", err)
	}
	if strings.TrimSpace(out.TagName) == "" {
		return releaseInfo{}, fmt.Errorf("更新信息缺少版本号")
	}
	return out, nil
}

func compareVersions(a, b string) int {
	as := numericVersion(a)
	bs := numericVersion(b)
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		av, bv := 0, 0
		if i < len(as) {
			av = as[i]
		}
		if i < len(bs) {
			bv = bs[i]
		}
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
	}
	return 0
}

func numericVersion(v string) []int {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, "-+ "); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			n = 0
		}
		out = append(out, n)
	}
	return out
}

func updateMessage(current string, rel releaseInfo) (msg string, newer bool) {
	if compareVersions(rel.TagName, current) > 0 {
		name := strings.TrimSpace(rel.Name)
		if name == "" || strings.EqualFold(name, rel.TagName) {
			name = rel.TagName
		}
		return "发现新版本 " + name, true
	}
	return "已是最新版本（" + current + "）", false
}

var checkForUpdate = func(ctx context.Context, client *http.Client) (releaseInfo, error) {
	return fetchLatestRelease(ctx, client, releasesAPI)
}

func updateHTTPClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Second}
}

func (a *app) updateSnapshot() (checking bool, msg, url string, newer bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.updateChecking, a.updateMsg, a.updateURL, a.updateNewer
}

func (a *app) setUpdateState(checking bool, msg, url string, newer bool) {
	a.mu.Lock()
	a.updateChecking, a.updateMsg, a.updateURL, a.updateNewer = checking, msg, url, newer
	a.mu.Unlock()
	a.settingsRefresh()
}

func (a *app) checkUpdatesAsync() {
	if checking, _, _, _ := a.updateSnapshot(); checking {
		return
	}
	a.setUpdateState(true, "正在检查…", "", false)
	go func() {
		defer recoverLog("update.go:checkUpdates")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		rel, err := checkForUpdate(ctx, updateHTTPClient())
		if err != nil {
			if errors.Is(err, errNoRelease) {
				a.setUpdateState(false, err.Error(), "", false)
				return
			}
			a.setUpdateState(false, "检查更新失败："+err.Error(), "", false)
			return
		}
		msg, newer := updateMessage(appVersion, rel)
		a.setUpdateState(false, msg, rel.HTMLURL, newer)
	}()
}
