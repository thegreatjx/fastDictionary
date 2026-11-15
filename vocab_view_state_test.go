package main

import "testing"

func TestVocabularyViewStateResetsConfirmations(t *testing.T) {
	a := &app{}
	a.vocabConfirmDeleteID = "word-1"
	a.confirmClearVocab = true
	a.vocabExpandedID = "word-2"

	a.resetVocabularyViewState()

	if a.vocabConfirmDeleteID != "" || a.confirmClearVocab || a.vocabExpandedID != "" {
		t.Fatalf("词库页状态没有复位：deleteID=%q clear=%v expanded=%q",
			a.vocabConfirmDeleteID, a.confirmClearVocab, a.vocabExpandedID)
	}
}

func TestVocabularyViewStateIsResetOnOpenAndFilterChange(t *testing.T) {
	settings := readSourceFile(t, "settings.go")
	if !containsText(settings, "a.resetVocabularyConfirmations()") {
		t.Fatal("showMainWindow 重聚焦时没有清掉生词本的两步删除确认")
	}
	page := readSourceFile(t, "settings_vocab.go")
	if countOccurrences(page, "a.resetVocabularyViewState()") < 2 {
		t.Fatal("分类与搜索切换没有复位确认状态")
	}
}

// TestConfirmResetKeepsExpandedEntry 钉住重聚焦时的折中：破坏性确认要清，
// 用户正在展开的词条不能跟着被收起来。
func TestConfirmResetKeepsExpandedEntry(t *testing.T) {
	a := &app{}
	a.vocabConfirmDeleteID = "word-1"
	a.confirmClearVocab = true
	a.vocabExpandedID = "word-2"

	a.resetVocabularyConfirmations()

	if a.vocabConfirmDeleteID != "" || a.confirmClearVocab {
		t.Fatalf("破坏性确认没有复位：deleteID=%q clear=%v", a.vocabConfirmDeleteID, a.confirmClearVocab)
	}
	if a.vocabExpandedID != "word-2" {
		t.Fatalf("展开中的词条被收起了：%q", a.vocabExpandedID)
	}
}
