package main

import "testing"

// TestSwitchBackDoesNotRestoreLoadingTab 复现 U6：切走时仍在加载的页签不该
// 被当作「已完成的缓存」恢复。修复前 saveTabLocked 原样存下 aiBusy/notesBusy
// 和半成品 aiText，hasContent() 只看内容不看 busy，switchMode 会直接走
// restoreTabLocked —— 半成品被当最终结果展示，且命中缓存后不会再补发请求。
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

	// 完成态仍应可恢复，防止把正常缓存一起关掉。
	a.aiBusy = false
	a.notesBusy = false
	a.aiText = "complete"
	a.saveTabLocked()
	if !a.tabStates[modeGrammar].hasContent() {
		t.Fatal("已完成的页签仍应报告有内容")
	}
}
