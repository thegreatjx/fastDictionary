package main

import "testing"

func TestSwitchBackDoesNotRestoreLoadingTab(t *testing.T) {
	a := newTestApp()
	a.mu.Lock()
	defer a.mu.Unlock()

	a.source = "hello"
	a.mode = modeGrammar
	a.aiBusy = true
	a.aiText = "partial"
	a.notesBusy = true

	a.saveTabLocked()
	st := a.tabStates[modeGrammar]
	if st == nil {
		t.Fatal("saveTabLocked 应当写下当前页签快照")
	}
	if st.hasContent() {
		t.Fatalf("进行中的页签不应被当作可恢复的完成态：aiBusy=%v notesBusy=%v aiText=%q",
			st.aiBusy, st.notesBusy, st.aiText)
	}

	a.aiBusy = false
	a.notesBusy = false
	a.aiText = "complete"
	a.saveTabLocked()
	if !a.tabStates[modeGrammar].hasContent() {
		t.Fatal("已完成的页签仍应报告有内容")
	}
}
