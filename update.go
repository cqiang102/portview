// PortView - 版本更新检查
// Copyright 2026 lacia.cq@qq.com
// License: Apache 2.0
//
// 本文件只做"查询 + 比较"，不下载、不替换任何文件：
// 发现新版本后由界面提示并打开下载页，安装仍由用户完成。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	releaseOwner = "cqiang102"
	releaseRepo  = "portview"

	// releasePage 是给用户打开的下载页（latest 会跳转到最新 Release）。
	releasePage = "https://github.com/cqiang102/portview/releases/latest"

	// updateTimeout 是单次版本查询的超时，避免界面长时间等待。
	updateTimeout = 10 * time.Second
	// maxReleaseBody 是更新说明的截断长度（按 rune 计）。
	maxReleaseBody = 800
	// maxReleaseBytes 限制响应体大小，防止异常响应占用内存。
	maxReleaseBytes = 1 << 20
)

// releaseAPIBase 允许测试替换为本地 httptest 服务。
var releaseAPIBase = "https://api.github.com"

// buildVersion 是当前程序版本，发布构建时由 CI 注入：
//
//	go build -ldflags "-X main.buildVersion=1.3.0"
//
// 本地开发未注入时使用此默认值。
var buildVersion = "1.3.0"

// Release 只保留版本检查用得到的字段。
type Release struct {
	TagName     string    `json:"tag_name"`
	HTMLURL     string    `json:"html_url"`
	Body        string    `json:"body"`
	PublishedAt time.Time `json:"published_at"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
}

// UpdateInfo 是一次版本检查的结果。
type UpdateInfo struct {
	Current string // 当前版本
	Latest  string // 最新 Release 的 tag，例如 v1.3.0
	URL     string // 下载页地址
	Notes   string // 更新说明（已截断）
	Newer   bool   // 是否存在更新
}

// checkForUpdate 查询最新 Release 并与当前版本比较。
// 网络/接口错误会原样返回；调用方决定是静默（自动检查）还是提示（手动检查）。
func checkForUpdate(ctx context.Context, current string) (UpdateInfo, error) {
	info := UpdateInfo{Current: current, URL: releasePage}
	rel, err := fetchLatestRelease(ctx)
	if err != nil {
		return info, err
	}
	// /releases/latest 不会返回草稿与预发布，这里做防御性处理。
	if rel.Draft || rel.Prerelease {
		return info, nil
	}
	info.Latest = rel.TagName
	if rel.HTMLURL != "" {
		info.URL = rel.HTMLURL
	}
	info.Notes = truncateNote(strings.TrimSpace(rel.Body), maxReleaseBody)
	info.Newer = isNewer(rel.TagName, current)
	return info, nil
}

// fetchLatestRelease 调用 GitHub API 获取最新 Release。
func fetchLatestRelease(ctx context.Context) (Release, error) {
	var rel Release
	ctx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()

	api := fmt.Sprintf("%s/repos/%s/%s/releases/latest", releaseAPIBase, releaseOwner, releaseRepo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	if err != nil {
		return rel, err
	}
	// GitHub API 要求提供 User-Agent，否则返回 403。
	req.Header.Set("User-Agent", "PortView/"+buildVersion)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return rel, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return rel, fmt.Errorf("GitHub API 返回 %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxReleaseBytes))
	if err != nil {
		return rel, err
	}
	if err := json.Unmarshal(body, &rel); err != nil {
		return rel, fmt.Errorf("解析 Release 数据失败: %w", err)
	}
	return rel, nil
}

// parseVersion 解析 "v1.2.3" / "1.2.3" 形式的三段版本号，忽略 -rc1 之类的后缀。
func parseVersion(v string) ([3]int, bool) {
	var out [3]int
	s := strings.TrimSpace(v)
	s = strings.TrimPrefix(s, "v")
	s = strings.TrimPrefix(s, "V")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// isNewer 判断 latest 是否比 current 更新。
// 任一版本号无法解析时返回 false：开发版本或异常 tag 不应触发更新提示。
func isNewer(latest, current string) bool {
	l, okLatest := parseVersion(latest)
	c, okCurrent := parseVersion(current)
	if !okLatest || !okCurrent {
		return false
	}
	for i := 0; i < 3; i++ {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}
