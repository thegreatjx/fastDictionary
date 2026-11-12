package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"v1.0.0", "1.0.0", 0},
		{"1.0.1", "1.0.0", 1},
		{"1.9.9", "1.10.0", -1},
		{"v2.0", "1.99.99", 1},
		{"1.2.0-beta.1", "1.2.0", 0},
		{"1.2.3.4", "1.2.3", 1},
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestUpdateMessage(t *testing.T) {
	msg, newer := updateMessage("1.0.0", releaseInfo{TagName: "v1.2.0", Name: "v1.2.0 — 复习与诊断"})
	if !newer || msg == "" || !containsText(msg, "新版本") {
		t.Fatalf("newer release: msg=%q newer=%v", msg, newer)
	}
	msg, newer = updateMessage("1.0.0", releaseInfo{TagName: "v1.0.0"})
	if newer || !containsText(msg, "最新") {
		t.Fatalf("same version: msg=%q newer=%v", msg, newer)
	}
	msg, newer = updateMessage("1.0.0", releaseInfo{TagName: "v0.9.0"})
	if newer || !containsText(msg, "最新") {
		t.Fatalf("older release: msg=%q newer=%v", msg, newer)
	}
}

func TestFetchLatestRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request had no User-Agent; GitHub rejects those")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"tag_name":"v1.3.0","html_url":"https://github.com/thegreatjx/fastDictionary/releases/tag/v1.3.0","name":"v1.3.0"}`))
	}))
	defer srv.Close()

	rel, err := fetchLatestRelease(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if rel.TagName != "v1.3.0" || rel.HTMLURL == "" {
		t.Fatalf("release = %+v", rel)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer bad.Close()
	if _, err := fetchLatestRelease(context.Background(), bad.Client(), bad.URL); err == nil {
		t.Fatal("403 must be reported as an error")
	}
}

func TestUpdateCheckIsUserVisible(t *testing.T) {
	src := readSourceFile(t, "settings_pages.go")
	for _, want := range []string{"检查更新", "checkUpdatesAsync", "updateSnapshot"} {
		if !containsText(src, want) {
			t.Errorf("关于 page is missing %q", want)
		}
	}
	if !containsText(readSourceFile(t, "update.go"), "checkForUpdate") {
		t.Error("the update check has no network seam")
	}
}

func TestFetchLatestReleaseExplainsMissingRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	_, err := fetchLatestRelease(context.Background(), srv.Client(), srv.URL)
	if err == nil {
		t.Fatal("404 must be reported")
	}
	if !containsText(err.Error(), "发布版本") {
		t.Fatalf("404 message should explain the empty release feed, got %q", err)
	}
}

func TestAboutPageShowsUpdateResult(t *testing.T) {
	a := newTestApp()
	a.page = "about"
	a.setUpdateState(false, "发现新版本 v1.2.0", "https://example.com/download", true)
	tt := uiTesterFor(a)
	for _, want := range []string{"检查更新", "发现新版本 v1.2.0", "打开下载页"} {
		if !tt.HasText(want) {
			t.Errorf("about page is missing %q: %v", want, tt.Texts())
		}
	}
}
