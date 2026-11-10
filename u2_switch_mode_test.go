package main

import "testing"

// TestSwitchModeInvalidatesTranslationGen 复现 U2：切换 mode 必须让在飞的
// 翻译代次失效。修复前 switchMode 只把 busy 置 false，translateGen 不变，
// translateTextWithReflow 的 goroutine 回来时 `if gen != a.translateGen`
// 会通过，把旧译文 / 引擎 / 词典写进新模式的视图。
func TestSwitchModeInvalidatesTranslationGen(t *testing.T) {
	a := newTestApp()
	a.mu.Lock()
	a.source = "hello"
	a.mode = modeTranslate
	a.busy = true
	a.translateGen = 5
	// 预先给目标 mode 一个有内容的快照，让 switchMode 命中缓存、
	// 不走 rerunCurrentMode（那条路径会发真实网络请求）。
	a.tabStates = map[popupMode]*tabViewState{
		modeDictionary: &tabViewState{source: "hello", target: "cached"},
	}
	a.mu.Unlock()

	a.switchMode(modeDictionary)

	a.mu.Lock()
	got := a.translateGen
	a.mu.Unlock()
	if got == 5 {
		t.Fatal("switchMode 没有让在飞的翻译代次失效：旧 goroutine 回来仍会把旧译文写进新视图")
	}
}
