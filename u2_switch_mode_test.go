package main

import "testing"

func TestSwitchModeInvalidatesTranslationGen(t *testing.T) {
	a := newTestApp()
	a.mu.Lock()
	a.source = "hello"
	a.mode = modeTranslate
	a.busy = true
	a.translateGen = 5
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
