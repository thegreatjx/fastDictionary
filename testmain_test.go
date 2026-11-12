package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain points the vocabulary singleton at a temporary file before any
// test can touch it. Tests used to call GetVocabularyStore() directly, which
// loaded %APPDATA%\fastDictionary\vocabulary.json and let Clear()/Add() rewrite
// the developer's real notebook. A test run must never be able to erase user
// data, even when the developer is the user.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "fastDictionary-tests-")
	globalVocabOnce.Do(func() {}) // consume the Once; it must not overwrite the test store
	if err != nil {
		// Better an empty in-memory notebook than letting a test touch the
		// developer's real vocabulary.json.
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
