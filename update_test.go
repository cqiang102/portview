package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"
)

// ============================================================
// 版本号解析与比较
// ============================================================

func TestParseVersion(t *testing.T) {
	cases := []struct {
		in   string
		want [3]int
		ok   bool
	}{
		{"v1.2.3", [3]int{1, 2, 3}, true},
		{"1.2.3", [3]int{1, 2, 3}, true},
		{"V2.0.0", [3]int{2, 0, 0}, true},
		{" v1.10.7 ", [3]int{1, 10, 7}, true},
		{"v1.3.0-rc1", [3]int{1, 3, 0}, true},
		{"v1.3.0+build5", [3]int{1, 3, 0}, true},
		{"", [3]int{}, false},
		{"dev", [3]int{}, false},
		{"v", [3]int{}, false},
		{"v1.2", [3]int{}, false},
		{"v1.2.3.4", [3]int{}, false},
		{"v1.2.x", [3]int{}, false},
		{"v1.-2.3", [3]int{}, false},
	}
	for _, c := range cases {
		got, ok := parseVersion(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("parseVersion(%q) = %v,%v；期望 %v,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestIsNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"v1.2.2", "v1.2.1", true},
		{"v1.3.0", "1.2.1", true},
		{"v2.0.0", "v1.99.99", true},
		{"v1.10.0", "v1.9.9", true}, // 按数值比较，不能按字符串比较
		{"v1.2.1", "v1.2.1", false},
		{"v1.2.0", "v1.2.1", false},
		{"v1.0.0", "v2.0.0", false},
		{"v9.9.9", "dev", false}, // 当前版本不可解析时不提示更新
		{"not-a-version", "v1.0.0", false},
		{"v1.2.2", "", false},
	}
	for _, c := range cases {
		if got := isNewer(c.latest, c.current); got != c.want {
			t.Errorf("isNewer(%q, %q) = %v；期望 %v", c.latest, c.current, got, c.want)
		}
	}
}

func TestVersionText(t *testing.T) {
	restore := buildVersion
	t.Cleanup(func() { buildVersion = restore })

	buildVersion = "1.3.0"
	if got := versionText(); got != "v1.3.0" {
		t.Errorf("versionText() = %q，期望 v1.3.0", got)
	}
	buildVersion = "dev"
	if got := versionText(); got != "开发版本" {
		t.Errorf("开发版本应显示 开发版本，得到 %q", got)
	}
}

// ============================================================
// GitHub Release 查询（用 httptest 模拟，不访问真实网络）
// ============================================================

// withReleaseAPI 把接口地址指向测试服务器，并在用例结束后还原。
func withReleaseAPI(t *testing.T, base string) {
	t.Helper()
	old := releaseAPIBase
	releaseAPIBase = base
	t.Cleanup(func() { releaseAPIBase = old })
}

func releaseServer(t *testing.T, status int, body string, check func(*http.Request)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if check != nil {
			check(r)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCheckForUpdate_NewerAvailable(t *testing.T) {
	var gotPath, gotUA string
	srv := releaseServer(t, http.StatusOK,
		`{"tag_name":"v1.3.0","html_url":"https://example.com/rel","body":"修复若干问题","draft":false,"prerelease":false}`,
		func(r *http.Request) { gotPath, gotUA = r.URL.Path, r.Header.Get("User-Agent") })
	withReleaseAPI(t, srv.URL)

	info, err := checkForUpdate(context.Background(), "1.2.1")
	if err != nil {
		t.Fatalf("检查更新失败: %v", err)
	}
	if !info.Newer || info.Latest != "v1.3.0" || info.URL != "https://example.com/rel" {
		t.Fatalf("结果错误: %+v", info)
	}
	if !strings.Contains(info.Notes, "修复若干问题") {
		t.Fatalf("更新说明缺失: %q", info.Notes)
	}
	if gotPath != "/repos/cqiang102/portview/releases/latest" {
		t.Fatalf("请求路径错误: %q", gotPath)
	}
	// GitHub API 缺少 User-Agent 会返回 403
	if gotUA == "" {
		t.Fatal("请求必须带 User-Agent")
	}
}

func TestCheckForUpdate_UpToDate(t *testing.T) {
	srv := releaseServer(t, http.StatusOK, `{"tag_name":"v1.2.1","html_url":"https://example.com/v1.2.1"}`, nil)
	withReleaseAPI(t, srv.URL)

	info, err := checkForUpdate(context.Background(), "v1.2.1")
	if err != nil {
		t.Fatal(err)
	}
	if info.Newer || info.Latest != "v1.2.1" {
		t.Fatalf("已是最新版本时不应提示更新: %+v", info)
	}
}

func TestCheckForUpdate_IgnoresDraftAndPrerelease(t *testing.T) {
	for _, body := range []string{
		`{"tag_name":"v9.9.9","draft":true}`,
		`{"tag_name":"v9.9.9","prerelease":true}`,
	} {
		srv := releaseServer(t, http.StatusOK, body, nil)
		withReleaseAPI(t, srv.URL)
		info, err := checkForUpdate(context.Background(), "1.2.1")
		if err != nil {
			t.Fatal(err)
		}
		if info.Newer {
			t.Fatalf("草稿/预发布不应提示更新: %s", body)
		}
	}
}

func TestCheckForUpdate_EmptyHTMLURLFallsBackToReleasePage(t *testing.T) {
	srv := releaseServer(t, http.StatusOK, `{"tag_name":"v1.3.0"}`, nil)
	withReleaseAPI(t, srv.URL)

	info, err := checkForUpdate(context.Background(), "1.2.1")
	if err != nil {
		t.Fatal(err)
	}
	if info.URL != releasePage {
		t.Fatalf("html_url 缺失时应回退下载页，得到 %q", info.URL)
	}
}

func TestCheckForUpdate_Errors(t *testing.T) {
	srv := releaseServer(t, http.StatusForbidden, `{"message":"rate limited"}`, nil)
	withReleaseAPI(t, srv.URL)
	if _, err := checkForUpdate(context.Background(), "1.2.1"); err == nil {
		t.Fatal("非 200 响应应返回错误")
	}

	bad := releaseServer(t, http.StatusOK, "not json", nil)
	withReleaseAPI(t, bad.URL)
	if _, err := checkForUpdate(context.Background(), "1.2.1"); err == nil {
		t.Fatal("非法 JSON 应返回错误")
	}
}

func TestUpdateDialogText(t *testing.T) {
	restore := buildVersion
	t.Cleanup(func() { buildVersion = restore })
	buildVersion = "1.2.1"

	newer := updateDialogText(UpdateInfo{Latest: "v1.3.0", Newer: true, Notes: "新功能"})
	if !strings.Contains(newer, "发现新版本 v1.3.0") || !strings.Contains(newer, "新功能") {
		t.Fatalf("更新提示文案错误: %q", newer)
	}
	latest := updateDialogText(UpdateInfo{Latest: "v1.2.1"})
	if !strings.Contains(latest, "已是最新版本") {
		t.Fatalf("已是最新版本文案错误: %q", latest)
	}
	empty := updateDialogText(UpdateInfo{})
	if !strings.Contains(empty, "没有查询到") {
		t.Fatalf("无发布版本文案错误: %q", empty)
	}
	buildVersion = "dev"
	dev := updateDialogText(UpdateInfo{Latest: "v1.2.1"})
	if !strings.Contains(dev, "开发版本") {
		t.Fatalf("开发版本文案错误: %q", dev)
	}
}

// ============================================================
// 界面接线：版本标签与状态栏提示
// ============================================================

func TestApplyUpdateInfoUpdatesLabels(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	pv := &PortViewer{win: a.NewWindow("test"), meta: testStore(t), selRow: -1}
	pv.buildUI()

	if pv.version == nil || pv.updateBtn == nil {
		t.Fatal("主界面应包含版本标签与「检查更新」按钮")
	}
	if pv.version.Text != versionText() {
		t.Fatalf("初始版本标签错误: %q", pv.version.Text)
	}

	pv.applyUpdateInfo(UpdateInfo{Latest: "v9.9.9", Newer: true})
	if !strings.Contains(pv.version.Text, "v9.9.9") {
		t.Fatalf("发现新版本后版本标签未更新: %q", pv.version.Text)
	}
	if !strings.Contains(pv.status.Text, "发现新版本") {
		t.Fatalf("发现新版本后状态栏未提示: %q", pv.status.Text)
	}

	pv.applyUpdateInfo(UpdateInfo{Latest: buildVersion, Newer: false})
	if pv.version.Text != versionText() {
		t.Fatalf("无更新时应恢复当前版本显示: %q", pv.version.Text)
	}
}

// ============================================================
// 更新检查设置持久化
// ============================================================

func TestStoreAutoCheckUpdateDefaultsOn(t *testing.T) {
	s := testStore(t)
	if !s.AutoCheckUpdate() {
		t.Fatal("默认应开启自动检查更新")
	}
	if s.NotifiedVersion() != "" {
		t.Fatalf("默认不应有已提示版本: %q", s.NotifiedVersion())
	}
}

func TestStoreUpdateSettingsRoundTrip(t *testing.T) {
	s := testStore(t)
	if err := s.SetAutoCheckUpdate(false); err != nil {
		t.Fatal(err)
	}
	if err := s.SetNotifiedVersion("v1.3.0"); err != nil {
		t.Fatal(err)
	}
	// 再次写入其它设置，确保 cloneData 不会把已有设置清空
	if err := s.SaveGroup("", CustomGroup{Name: "临时", Ports: []int{1}}); err != nil {
		t.Fatal(err)
	}
	if s.AutoCheckUpdate() {
		t.Fatal("关闭自动检查后不应变回开启")
	}
	if s.NotifiedVersion() != "v1.3.0" {
		t.Fatalf("已提示版本丢失: %q", s.NotifiedVersion())
	}
	if err := s.load(); err != nil {
		t.Fatal(err)
	}
	if s.AutoCheckUpdate() || s.NotifiedVersion() != "v1.3.0" {
		t.Fatalf("重新加载后设置丢失: auto=%v notified=%q", s.AutoCheckUpdate(), s.NotifiedVersion())
	}
	// 显式改回开启
	if err := s.SetAutoCheckUpdate(true); err != nil {
		t.Fatal(err)
	}
	if err := s.load(); err != nil {
		t.Fatal(err)
	}
	if !s.AutoCheckUpdate() {
		t.Fatal("重新开启后未持久化")
	}
}

func TestStoreResetKeepsUpdateSettings(t *testing.T) {
	s := testStore(t)
	if err := s.SetAutoCheckUpdate(false); err != nil {
		t.Fatal(err)
	}
	if err := s.SetNotifiedVersion("v1.3.0"); err != nil {
		t.Fatal(err)
	}
	if err := s.ResetAll(); err != nil {
		t.Fatal(err)
	}
	if s.AutoCheckUpdate() {
		t.Fatal("重置分组不应重新打开自动检查")
	}
	if s.NotifiedVersion() != "v1.3.0" {
		t.Fatalf("重置分组不应清除已提示版本: %q", s.NotifiedVersion())
	}
}

func TestStoreLegacyFileDefaultsAutoCheckOn(t *testing.T) {
	s := testStore(t)
	if err := os.WriteFile(s.path, []byte(`{"18080":{"group":"old","note":"保留"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.load(); err != nil {
		t.Fatal(err)
	}
	if !s.AutoCheckUpdate() {
		t.Fatal("旧版配置应默认开启自动检查更新")
	}
}
