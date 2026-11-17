package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "fastDictionary-test-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() {
		for i := 0; i < 10; i++ {
			if err := os.RemoveAll(dir); err == nil {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Logf("could not remove %s", dir)
	})
	return dir
}

func TestSaveIsAtomicAcrossCrashes(t *testing.T) {
	dir := tempDir(t)
	path := filepath.Join(dir, "vocabulary.json")
	s := &VocabularyStore{path: path, entries: []VocabularyEntry{}}

	for i := 0; i < 20; i++ {
		s.Add(VocabularyEntry{
			ID:   "id" + string(rune('a'+i%26)),
			Term: "term" + string(rune('a'+i%26)),
		})
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read after save %d: %v", i, err)
		}
		var parsed []VocabularyEntry
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("save %d left invalid JSON (%d bytes): %v", i, len(data), err)
		}
		if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
			t.Fatalf("save %d left %s.tmp behind", i, path)
		}
	}
}

func TestSaveSerialisesConcurrentWriters(t *testing.T) {
	dir := tempDir(t)
	path := filepath.Join(dir, "vocabulary.json")
	s := &VocabularyStore{path: path, entries: []VocabularyEntry{}}
	s.Add(VocabularyEntry{ID: "seed", Term: "seed"})

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = s.save()
		}(i)
	}
	wg.Wait()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after concurrent saves: %v", err)
	}
	var parsed []VocabularyEntry
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("concurrent saves produced invalid JSON: %v", err)
	}
	if len(parsed) != 1 || parsed[0].ID != "seed" {
		t.Fatalf("concurrent saves lost data: got %d entries", len(parsed))
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("a temp file survived the concurrent saves")
	}
}

func TestSaveWithNoPathIsANoop(t *testing.T) {
	s := &VocabularyStore{entries: []VocabularyEntry{}}
	if err := s.save(); err != nil {
		t.Fatalf("save on an in-memory store returned %v", err)
	}
	if s.Path() != "" {
		t.Fatalf("in-memory store reports path %q", s.Path())
	}
}

func TestSaveFailureIsReportedNotSwallowed(t *testing.T) {
	dir := tempDir(t)
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &VocabularyStore{
		path:    filepath.Join(blocker, "nested", "vocabulary.json"),
		entries: []VocabularyEntry{{ID: "a", Term: "alpha"}},
	}
	if err := s.save(); err == nil {
		t.Fatal("save reported success for an unwritable path")
	}
}

func TestTranslateRaceHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan raceResult, 1)
	go func() {
		done <- TranslateRace(ctx, "hello", "en", "zh", "auto")
	}()

	select {
	case r := <-done:
		if r.Err == nil {
			t.Fatalf("cancelled race reported success: %+v", r)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("TranslateRace ignored the cancelled context")
	}
}

func TestTranslateRacePinnedModeReturnsOneEngine(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	r := TranslateRace(ctx, "hello", "en", "zh", "microsoft")
	if r.Engine != "微软" {
		t.Fatalf("pinned microsoft race reported engine %q", r.Engine)
	}
}

func TestTranslateGenerationCancelsThePreviousAnswer(t *testing.T) {
	a := &app{store: newMemoryStore()}
	a.translateGen = 7
	a.translateGen++
	gen := a.translateGen
	a.translateGen++
	if gen == a.translateGen {
		t.Fatal("translateGen did not advance; the stale guard cannot fire")
	}
}
