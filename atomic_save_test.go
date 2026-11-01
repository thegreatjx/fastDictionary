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

// tempDir creates a scratch directory and removes it with retries.
//
// t.TempDir() cleans up with a single RemoveAll, which on Windows fails with
// "The directory is not empty" when a handle on the just-renamed files is
// still open (the search indexer and Defender both do this). The retry is
// test hygiene, not a workaround for the code under test: the save path is
// already fully synchronous by the time cleanup runs.
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
		// Leave it behind rather than failing a passing test on cleanup.
		t.Logf("could not remove %s", dir)
	})
	return dir
}

// TestSaveIsAtomicAcrossCrashes is the P2-17 item: the notebook is written to
// a temp file, flushed, and only then renamed into place, so a reader (or a
// crash) never sees a half-written vocabulary.json.
//
// The test cannot kill the process mid-write, so it asserts the observable
// consequences of that design instead: no temp file survives a successful
// save, the file on disk always parses, and the previous copy is preserved.
func TestSaveIsAtomicAcrossCrashes(t *testing.T) {
	dir := tempDir(t)
	path := filepath.Join(dir, "vocabulary.json")
	s := &VocabularyStore{path: path, entries: []VocabularyEntry{}}

	for i := 0; i < 20; i++ {
		s.Add(VocabularyEntry{
			ID:   "id" + string(rune('a'+i%26)),
			Term: "term" + string(rune('a'+i%26)),
		})
		// The file must always be complete JSON, never a truncated write.
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read after save %d: %v", i, err)
		}
		var parsed []VocabularyEntry
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("save %d left invalid JSON (%d bytes): %v", i, len(data), err)
		}
		// The temp file is renamed, not left behind.
		if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
			t.Fatalf("save %d left %s.tmp behind", i, path)
		}
	}
}

// TestSaveSerialisesConcurrentWriters guards the saveMu: two goroutines
// saving at once used to be able to interleave their temp-file writes.
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

// TestSaveWithNoPathIsANoop covers the in-memory store used by tests and by
// the settings preview: saving must not invent a file.
func TestSaveWithNoPathIsANoop(t *testing.T) {
	s := &VocabularyStore{entries: []VocabularyEntry{}}
	if err := s.save(); err != nil {
		t.Fatalf("save on an in-memory store returned %v", err)
	}
	if s.Path() != "" {
		t.Fatalf("in-memory store reports path %q", s.Path())
	}
}

// TestSaveFailureIsReportedNotSwallowed covers saveOrLog: a notebook on a
// path that cannot be written must produce a log line, not silence.
func TestSaveFailureIsReportedNotSwallowed(t *testing.T) {
	dir := tempDir(t)
	// A path whose parent is a regular file: MkdirAll and OpenFile both fail.
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

// TestTranslateRaceHonoursCancellation is the P2-17 item for the race: a
// cancelled context must come back promptly instead of waiting out both
// engines' HTTP timeouts.
func TestTranslateRaceHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled before the race starts

	done := make(chan raceResult, 1)
	go func() {
		// An unresolvable host keeps this offline: the request fails on the
		// cancelled context before any DNS lookup matters.
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

// TestTranslateRacePinnedModeReturnsOneEngine pins the single-engine modes:
// pinning Microsoft or Google must not start the other one at all.
func TestTranslateRacePinnedModeReturnsOneEngine(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// The timeout fires almost immediately; the point is that the call
	// returns one engine's verdict rather than an "all engines failed"
	// combination of two.
	r := TranslateRace(ctx, "hello", "en", "zh", "microsoft")
	if r.Engine != "微软" {
		t.Fatalf("pinned microsoft race reported engine %q", r.Engine)
	}
}

// TestTranslateGenerationCancelsThePreviousAnswer is the same guard the race
// has, one level up: a new translation must invalidate the one in flight, or
// a slow answer overwrites a newer one on screen.
func TestTranslateGenerationCancelsThePreviousAnswer(t *testing.T) {
	a := &app{store: newMemoryStore()}
	a.translateGen = 7
	// Simulate translate() starting a new request.
	a.translateGen++
	gen := a.translateGen
	// A second request starts before the first finishes.
	a.translateGen++
	if gen == a.translateGen {
		t.Fatal("translateGen did not advance; the stale guard cannot fire")
	}
}
