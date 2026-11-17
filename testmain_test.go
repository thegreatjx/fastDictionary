package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "fastDictionary-tests-")
	globalVocabOnce.Do(func() {})
	if err != nil {
		globalVocabStore = newMemoryVocabularyStore()
		os.Exit(m.Run())
	}
	globalVocabStore = &VocabularyStore{
		path:    filepath.Join(dir, "vocabulary.json"),
		entries: []VocabularyEntry{},
	}

	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
