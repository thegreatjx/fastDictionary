package main

import (
	"strings"
	"testing"
)

func sourceFuncBody(t *testing.T, src, signature string) string {
	t.Helper()
	start := strings.Index(src, signature)
	if start < 0 {
		t.Fatalf("找不到函数 %s", signature)
	}
	rel := strings.Index(src[start:], "\n}\n")
	if rel < 0 {
		t.Fatalf("函数 %s 没有结束", signature)
	}
	return src[start : start+rel]
}

// TestPauseAndModeChangeResetGestureGate is the regression: only resumeSelection
// reset the gate, so pausing or switching the trigger mode left the 300ms
// cooldown behind and the first gesture afterwards was swallowed.
func TestPauseAndModeChangeResetGestureGate(t *testing.T) {
	guard := readSourceFile(t, "selection_guard.go")
	pause := sourceFuncBody(t, guard, "func (a *app) pauseSelection(")
	if !containsText(pause, "a.gestureGate.reset()") {
		t.Fatal("pauseSelection 没有清掉手势冷却：暂停后第一个手势会被吞掉")
	}
	if strings.Index(pause, "a.gestureGate.reset()") > strings.Index(pause, "go a.rebuildTray()") {
		t.Fatal("pauseSelection 应先清手势冷却，再刷新托盘")
	}

	selicon := readSourceFile(t, "selicon.go")
	apply := sourceFuncBody(t, selicon, "func (a *app) applySelectionMode()")
	if !containsText(apply, "a.gestureGate.reset()") {
		t.Fatal("applySelectionMode 没有清掉手势冷却：切换触发模式后第一个手势会被吞掉")
	}
}

// TestTrayFlyoutRebuildsTrayOnModeChange is the other half: the flyout writes
// the mode but never rebuilt the menu, so the tray radio stayed on the old
// mode until something else rebuilt it.
func TestTrayFlyoutRebuildsTrayOnModeChange(t *testing.T) {
	src := readSourceFile(t, "tray.go")
	// 模式切换改用显式值映射 SegmentedControlMapped，定位锚点随之更新；
	// 断言的语义不变：切换后必须重建托盘菜单，否则单选状态会停在旧模式。
	const anchor = "SegmentedControlMapped(c, true, cfg.SelectionTrigger"
	idx := strings.Index(src, anchor)
	if idx < 0 {
		t.Fatal("找不到托盘浮层的划词模式切换")
	}
	tail := src[idx:]
	end := strings.Index(tail, "\n\t\t\t})")
	if end < 0 {
		t.Fatal("托盘浮层的划词模式切换没有结束")
	}
	body := tail[:end]
	if !containsText(body, "a.rebuildTray()") {
		t.Fatal("浮层切换划词模式后没有重建托盘菜单，托盘单选状态会停留在旧模式")
	}
}
