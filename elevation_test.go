package main

import (
	"os"
	"strings"
	"testing"
)

func TestElevationNoticeExplainsOnlyRealBlocking(t *testing.T) {
	cases := []struct {
		name       string
		app        string
		blocked    bool
		selfElevat bool
		wantEmpty  bool
		wantText   string
	}{
		{"plain app, not blocked", "chrome.exe", false, false, true, ""},
		{"elevated window blocks a normal process", "taskmgr.exe", true, false, false, "管理员权限"},
		{"elevated process is not blocked by itself", "taskmgr.exe", true, true, true, ""},
		{"unknown owner still reads naturally", "", true, false, false, "当前窗口"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := elevationNotice(c.app, c.blocked, c.selfElevat)
			if c.wantEmpty {
				if got != "" {
					t.Fatalf("notice = %q, want empty", got)
				}
				return
			}
			if !strings.Contains(got, c.wantText) {
				t.Fatalf("notice = %q, want it to mention %q", got, c.wantText)
			}
			if !strings.Contains(got, "以管理员身份重启") {
				t.Fatalf("notice must name the remedy: %q", got)
			}
		})
	}
}

// The Win32 wiring has to agree with itself: both entry points query the same
// token, so they must return the same answer for this process. This catches a
// wrong access mask or struct size that pure Go tests cannot see.
func TestProcessElevationQueriesAgreeForOwnProcess(t *testing.T) {
	self := currentProcessElevated()
	t.Logf("running elevated: %v", self)
	if got := processElevated(uint32(os.Getpid())); got != self {
		t.Fatalf("processElevated(self) = %v, currentProcessElevated = %v", got, self)
	}
	// Must not panic or block when there is no foreground window (CI).
	_ = foregroundProcessElevated()
}

func TestElevatedTakeoverArgIsExact(t *testing.T) {
	prev := os.Args
	t.Cleanup(func() { os.Args = prev })
	os.Args = []string{"fastDictionary.exe"}
	if elevatedTakeoverRequested() {
		t.Fatal("a normal launch must not skip the single-instance lock")
	}
	os.Args = []string{"fastDictionary.exe", elevatedTakeoverArg}
	if !elevatedTakeoverRequested() {
		t.Fatal("the elevated restart must be recognised")
	}
	os.Args = []string{"fastDictionary.exe", "--elevated-takeover-typo"}
	if elevatedTakeoverRequested() {
		t.Fatal("partial matches must not disable the lock")
	}
}
