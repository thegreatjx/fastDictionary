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
	if err != nil {
		os.Exit(m.Run())
	}
	globalVocabOnce.Do(func() {}) // consume the Once; it must not overwrite the test store
	globalVocabStore = &VocabularyStore{
		path:    filepath.Join(dir, "vocabulary.json"),
		entries: []VocabularyEntry{},
	}

	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
