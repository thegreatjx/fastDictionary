package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestVocabularyDeleteDoesNotDeadlock guards the bug where Delete() held
// s.mu.Lock() and then called saveOrLog() -> save(), which itself takes
// s.mu.RLock(). sync.RWMutex is not reentrant, so deleting an entry from the
// notebook froze the whole process forever (the user saw it as a crash).
func TestVocabularyDeleteDoesNotDeadlock(t *testing.T) {
	dir := t.TempDir()
	s := &VocabularyStore{path: filepath.Join(dir, "vocabulary.json"), entries: []VocabularyEntry{}}

	s.Add(VocabularyEntry{ID: "a", Term: "alpha", Definition: "first"})
	s.Add(VocabularyEntry{ID: "b", Term: "beta", Definition: "second"})
	if got := s.Count(); got != 2 {
		t.Fatalf("setup: count = %d, want 2", got)
	}

	done := make(chan bool, 1)
	go func() { done <- s.Delete("a") }()

	select {
	case ok := <-done:
		if !ok {
			t.Fatal("Delete returned false for an existing id")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Delete deadlocked: it holds s.mu.Lock() and then calls save() which takes s.mu.RLock()")
	}

	if got := s.Count(); got != 1 {
		t.Fatalf("after delete: count = %d, want 1", got)
	}
	// The removal must also be persisted, not just dropped from memory.
	reloaded := &VocabularyStore{path: s.path, entries: []VocabularyEntry{}}
	reloaded.load()
	if got := reloaded.Count(); got != 1 {
		t.Fatalf("after reload: count = %d, want 1 (delete was not persisted)", got)
	}
	if _, err := os.Stat(s.path); err != nil {
		t.Fatalf("vocabulary file was not written: %v", err)
	}
}
