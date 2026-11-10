package main

import "testing"

// TestLoadingTabsAreNotCached is the tab-cache half of the card activity work:
// a tab saved while a translation or OCR pass is still running is not a
// finished tab, so switchMode must not restore it as if it were.
func TestLoadingTabsAreNotCached(t *testing.T) {
	a := newTestApp()
	a.mu.Lock()
	a.mode = modeTranslate
	a.source = "hello"
	a.target = "bonjour"
	a.busy = true
	a.saveTabLocked()
	st := a.tabStates[modeTranslate]
	a.mu.Unlock()
	if st == nil {
		t.Fatal("saveTabLocked 没有记下页签")
	}
	if st.hasContent() {
		t.Fatal("正在翻译的页签被当成完成态：切回来会展示半截译文且不再补发请求")
	}

	a2 := newTestApp()
	a2.mu.Lock()
	a2.mode = modeTranslate
	a2.source = "hello"
	a2.target = "bonjour"
	a2.ocrBusy = true
	a2.saveTabLocked()
	st2 := a2.tabStates[modeTranslate]
	a2.mu.Unlock()
	if st2.hasContent() {
		t.Fatal("正在识别截图的页签被当成完成态")
	}
}

// TestFreshFlagConsumedOnEarlyReturn pins the other half: a 重新翻译 request
// must not survive a run that never reaches the cache lookup.
func TestFreshFlagConsumedOnEarlyReturn(t *testing.T) {
	a := newTestApp()
	a.store.mu.Lock()
	a.store.cfg.AIEnabled = false
	a.store.mu.Unlock()

	a.mu.Lock()
	a.skipAIResultCache = true
	a.mu.Unlock()
	a.runAction("default-sentence-analysis", "hello")
	a.mu.Lock()
	left := a.skipAIResultCache
	a.mu.Unlock()
	if left {
		t.Fatal("AI 关闭时 runAction 提前返回，skipAIResultCache 残留到下一次请求")
	}
}

// TestFreshFlagNotArmedForTranslateMode: translate() has no AI cache, so the
// footer refresh must not arm a flag that a later AI pass would consume.
func TestFreshFlagNotArmedForTranslateMode(t *testing.T) {
	src := readSourceFile(t, "popup.go")
	body := sourceFuncBody(t, src, "func (a *app) rerunCurrentModeFresh(")
	if !containsText(body, "case modeDictionary, modeGrammar, modeWriting:") {
		t.Fatal("rerunCurrentModeFresh 应只为 AI 模式设置 skipAIResultCache，翻译模式没有 AI 缓存")
	}
}
