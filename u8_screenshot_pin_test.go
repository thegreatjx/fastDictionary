package main

import (
	"strings"
	"testing"
)

// TestBothPopupEntriesResetScreenshotPin pins the invariant behind U8:
// opening a fresh popup must release the pin a screenshot translation set
// for its own window, on BOTH entry points. openPopupAction and
// openPopupWithText need a real window, so their behaviour cannot be replayed
// headless; the migration is factored into one pure method that is asserted
// structurally here and exercised in TestResetScreenshotPinLocked.
func TestBothPopupEntriesResetScreenshotPin(t *testing.T) {
	src := readSourceFile(t, "popup.go")
	if !containsText(src, "func (a *app) resetScreenshotPinLocked()") {
		t.Fatal("缺少统一的截图置顶复位方法 resetScreenshotPinLocked，两条入口仍各自解释 screenshotPinned")
	}
	actionAt := strings.Index(src, "func (a *app) openPopupAction(")
	withTextAt := strings.Index(src, "func (a *app) openPopupWithText(")
	if actionAt < 0 || withTextAt < 0 || withTextAt <= actionAt {
		t.Fatalf("找不到两条 popup 入口的函数边界：openPopupAction@%d openPopupWithText@%d", actionAt, withTextAt)
	}
	actionBody := src[actionAt:withTextAt]
	withTextBody := src[withTextAt:]
	if !containsText(actionBody, "a.resetScreenshotPinLocked()") {
		t.Fatal("openPopupAction 没有复位截图置顶：截图翻译后从划词 chip / 托盘打开的新卡片会永久置顶且失焦不关闭")
	}
	if !containsText(withTextBody, "a.resetScreenshotPinLocked()") {
		t.Fatal("openPopupWithText 没有走统一的复位方法")
	}
	if got := countOccurrences(src, "if a.screenshotPinned {"); got != 1 {
		t.Fatalf("截图置顶迁移应只在 resetScreenshotPinLocked 里出现一次，实际 %d 处", got)
	}
}

// TestResetScreenshotPinLocked exercises the pure state migration directly.
// This is the behaviour-level counterpart of the source-contract test above:
// only a pin requested by a screenshot translation is released; a pin the user
// set by hand (screenshotPinned == false) must survive a fresh popup.
func TestResetScreenshotPinLocked(t *testing.T) {
	cases := []struct {
		name                     string
		screenshotPinned, pinned bool
		wantScreenshot, wantPin  bool
	}{
		{"截图翻译的临时置顶被释放", true, true, false, false},
		{"用户手动置顶保留", false, true, false, true},
		{"本来就没置顶不变", false, false, false, false},
		{"只置顶标记未置窗口也归零", true, false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := newTestApp()
			a.screenshotPinned, a.pinned = tc.screenshotPinned, tc.pinned
			a.resetScreenshotPinLocked()
			if a.screenshotPinned != tc.wantScreenshot || a.pinned != tc.wantPin {
				t.Fatalf("复位后 screenshotPinned=%v pinned=%v，期望 %v %v",
					a.screenshotPinned, a.pinned, tc.wantScreenshot, tc.wantPin)
			}
			// 重复调用必须幂等，不能把用户手动置顶也清掉。
			a.resetScreenshotPinLocked()
			if a.screenshotPinned != tc.wantScreenshot || a.pinned != tc.wantPin {
				t.Fatalf("重复复位后状态漂移：screenshotPinned=%v pinned=%v",
					a.screenshotPinned, a.pinned)
			}
		})
	}
}
