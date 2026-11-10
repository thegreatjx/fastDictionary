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
	if !containsText(settings, "a.resetVocabularyViewState()") {
		t.Fatal("showSettings 没有复位词库页状态")
	}
	page := readSourceFile(t, "settings_vocab.go")
	if countOccurrences(page, "a.resetVocabularyViewState()") < 2 {
		t.Fatal("分类与搜索切换没有复位确认状态")
	}
}
