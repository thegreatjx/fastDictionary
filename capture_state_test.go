package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/transfer"
)

func TestParseCaptureStrategy(t *testing.T) {
	cases := []struct {
		raw  string
		want captureStrategy
		name string
	}{
		{"", captureAuto, "auto"},
		{"auto", captureAuto, "auto"},
		{"copy", captureCopyOnly, "copy"},
		{"uia", captureUIAOnly, "uia"},
		{"something-else", captureAuto, "auto"},
	}
	for _, c := range cases {
		got := parseCaptureStrategy(c.raw)
		if got != c.want {
			t.Errorf("parseCaptureStrategy(%q) = %v want %v", c.raw, got, c.want)
		}
		if got.String() != c.name {
			t.Errorf("parseCaptureStrategy(%q).String() = %q want %q", c.raw, got.String(), c.name)
		}
	}
	if got := captureStrategy(99).String(); got != "unknown" {
		t.Errorf("unknown strategy String() = %q", got)
	}
}

func TestCapturePlan(t *testing.T) {
	cases := []struct {
		strategy  captureStrategy
		tryUIA    bool
		allowCopy bool
	}{
		{captureAuto, true, true},
		{captureCopyOnly, false, true},
		{captureUIAOnly, true, false},
	}
	for _, c := range cases {
		tryUIA, allowCopy := c.strategy.capturePlan()
		if tryUIA != c.tryUIA || allowCopy != c.allowCopy {
			t.Errorf("%v: plan = (%v,%v) want (%v,%v)", c.strategy, tryUIA, allowCopy, c.tryUIA, c.allowCopy)
		}
	}
}

func TestClipboardTransactionRestoresOnce(t *testing.T) {
	calls := 0
	tx := &clipboardTransaction{
		snapshot: transfer.Data{},
		previous: "before",
		restoreFn: func(snap transfer.Data, text string) {
			calls++
			if text != "before" {
				t.Errorf("restore text = %q want before", text)
			}
		},
	}
	tx.restore()
	tx.restore()
	if calls != 1 {
		t.Fatalf("剪贴板应只还原一次，实际 %d 次", calls)
	}
}

// TestCaptureByCopyDefersRestore is the source contract for the clipboard
// transaction: the restore must be deferred, not a trailing call that an early
// return can skip.
func TestCaptureByCopyDefersRestore(t *testing.T) {
	src := readSourceFile(t, "capture.go")
	start := strings.Index(src, "func captureByCopy() string {")
	if start < 0 {
		t.Fatal("找不到 captureByCopy")
	}
	body := src[start:]
	if end := strings.Index(body, "\n}\n"); end >= 0 {
		body = body[:end]
	}
	if !strings.Contains(body, "defer tx.restore()") {
		t.Fatal("captureByCopy 必须用 defer 还原剪贴板")
	}
	if strings.Index(body, "defer tx.restore()") > strings.Index(body, "mygo.Clipboard.Clear()") {
		t.Fatal("defer 必须在清空剪贴板之前登记")
	}
}

func TestCaptureEntryPointsShareStrategy(t *testing.T) {
	src := readSourceFile(t, "capture.go")
	if got := strings.Count(src, "parseCaptureStrategy("); got < 2 {
		t.Fatalf("两个取词入口都应使用同一个策略解析，实际出现 %d 次", got)
	}
	for _, legacy := range []string{`SelectionCapture != "copy"`, `mode == "uia"`} {
		if strings.Contains(src, legacy) {
			t.Fatalf("仍有旧的字符串判定 %q", legacy)
		}
	}
}
