package main

import "strings"

// vocabCategory is the notebook tab an entry belongs to. The source is
// authoritative; the word-count heuristic only decides for entries whose
// source cannot be mapped (imports and older files).
type vocabCategory uint8

const (
	vocabCategoryWords vocabCategory = iota
	vocabCategorySentences
	vocabCategoryWriting
)

func (c vocabCategory) String() string {
	switch c {
	case vocabCategoryWords:
		return "words"
	case vocabCategorySentences:
		return "sentences"
	case vocabCategoryWriting:
		return "writing"
	}
	return "words"
}

// vocabCategoryFromKey maps the tab keys used by the settings page and the
// exported page back to the enum.
func vocabCategoryFromKey(key string) (vocabCategory, bool) {
	switch key {
	case "words":
		return vocabCategoryWords, true
	case "sentences":
		return vocabCategorySentences, true
	case "writing":
		return vocabCategoryWriting, true
	}
	return vocabCategoryWords, false
}

// classifyVocabEntry is the single classification rule. Every surface that
// groups entries — the tab counters, the filter, the row badges, the exported
// notebook page and the Anki export — calls it, so one entry can no longer
// land in different tabs on different surfaces.
func classifyVocabEntry(e VocabularyEntry) vocabCategory {
	switch e.Source {
	case "writing":
		return vocabCategoryWriting
	case "grammar":
		return vocabCategorySentences
	case "dict", "notes":
		return vocabCategoryWords
	}
	if len(strings.Fields(e.Term)) > 2 {
		return vocabCategorySentences
	}
	return vocabCategoryWords
}
