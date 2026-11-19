package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "fastDictionary-tests-")
	globalVocabOnce.Do(func() {})
	globalNotebookOnce.Do(func() {})
	if err != nil {
		globalVocabStore = newMemoryVocabularyStore()
		globalNotebookStore = newMemoryNotebookStore()
		os.Exit(m.Run())
	}
	globalVocabStore = &VocabularyStore{
		path:    filepath.Join(dir, "vocabulary.json"),
		entries: []VocabularyEntry{},
	}
	globalNotebookStore = &NotebookStore{
		path:    filepath.Join(dir, "notebook.json"),
		entries: []NotebookEntry{},
	}

	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
