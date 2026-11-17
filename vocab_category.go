package main

import "strings"

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
