package main

import (
	"strings"
	"testing"
)

// TestRemoveHooksResetsClickTracking pins the session boundary. Uninstalling
// the hooks ends a gesture session; the double-click candidate (lastClickTime,
// lastClickPos, clickCount) must not survive into the next install. Those
// fields are read under hookMu in lowLevelMouseProc, so the reset has to run
// inside the same lock. This is a source contract: lowLevelMouseProc is a
// Win32 callback and cannot be driven headlessly.
func TestRemoveHooksResetsClickTracking(t *testing.T) {
	src := readSourceFile(t, "hook.go")
	if !containsText(src, "func resetClickTrackingLocked()") {
		t.Fatal("钩子卸载没有收口的双击候选复位方法 resetClickTrackingLocked")
	}
	start := strings.Index(src, "func removeHooks() {")
	if start < 0 {
		t.Fatal("removeHooks not found")
	}
	end := strings.Index(src[start:], "\n}\n")
	if end < 0 {
		t.Fatal("removeHooks body not delimited")
	}
	body := src[start : start+end]
	call := strings.Index(body, "resetClickTrackingLocked()")
	if call < 0 {
		t.Fatal("removeHooks 没有在卸载时复位双击候选")
	}
	if unlock := strings.Index(body, "hookMu.Unlock()"); unlock < 0 || call > unlock {
		t.Fatal("双击候选复位必须发生在 hookMu 锁内")
	}
	// 手势识别本身不改：判定表达式与 clickCount 累加保持原样，本批只在卸载边界复位会话。
	if !containsText(src, "clickCount++") || !containsText(src, "clickCount == 2") {
		t.Fatal("双击判定被改动了；本批只在卸载边界复位会话")
	}
}

// TestResetClickTrackingLocked checks the pure reset directly. The three
// fields are package globals shared with the Win32 callback, so save and
// restore them to keep other tests isolated.
func TestResetClickTrackingLocked(t *testing.T) {
	hookMu.Lock()
	oldTime, oldPos, oldCount := lastClickTime, lastClickPos, clickCount
	hookMu.Unlock()
	defer func() {
		hookMu.Lock()
		lastClickTime, lastClickPos, clickCount = oldTime, oldPos, oldCount
		hookMu.Unlock()
	}()

	cases := []struct {
		name  string
		time  uint32
		pos   winPoint
		count int
	}{
		{"双击后的残留", 4200, winPoint{X: 120, Y: 340}, 2},
		{"单击后的残留", 9000, winPoint{X: 7, Y: 9}, 1},
		{"已经是干净状态", 0, winPoint{}, 0},
	}
	for _, tc := range cases {
		hookMu.Lock()
		lastClickTime, lastClickPos, clickCount = tc.time, tc.pos, tc.count
		resetClickTrackingLocked()
		if lastClickTime != 0 || lastClickPos != (winPoint{}) || clickCount != 0 {
			t.Errorf("%s: 复位后仍有残留 time=%d pos=%+v count=%d", tc.name, lastClickTime, lastClickPos, clickCount)
		}
		// 幂等：会话边界被重复触发（applySelectionMode 每次设置变更都调）不应出问题。
		resetClickTrackingLocked()
		if lastClickTime != 0 || lastClickPos != (winPoint{}) || clickCount != 0 {
			t.Errorf("%s: 重复复位后不干净", tc.name)
		}
		hookMu.Unlock()
	}
}
