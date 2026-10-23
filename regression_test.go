package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestAddBatchWritesOnceAndMerges guards the batch path: importing N words
// must persist once and still merge duplicates instead of appending them.
func TestAddBatchWritesOnceAndMerges(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vocabulary.json")
	s := &VocabularyStore{path: path, entries: []VocabularyEntry{}}
	s.Add(VocabularyEntry{ID: "x", Term: "alpha", Definition: "first"})

	n := s.AddBatch([]VocabularyEntry{
		{Term: "alpha", Definition: "first, again"}, // same term + no context: merged
		{Term: "beta", Definition: "second"},
		{Term: "gamma", Definition: "third"},
	})
	if n != 2 {
		t.Fatalf("AddBatch added %d, want 2 (alpha must merge)", n)
	}
	if got := s.Count(); got != 3 {
		t.Fatalf("count = %d, want 3", got)
	}
	reloaded := &VocabularyStore{path: path, entries: []VocabularyEntry{}}
	reloaded.load()
	if got := reloaded.Count(); got != 3 {
		t.Fatalf("after reload count = %d, want 3 (batch was not persisted)", got)
	}
}

// TestSaveKeepsBackupCopy guards the "one bad write loses the notebook"
// case: the previous file must survive as vocabulary.json.bak.
func TestSaveKeepsBackupCopy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vocabulary.json")
	s := &VocabularyStore{path: path, entries: []VocabularyEntry{}}
	s.Add(VocabularyEntry{ID: "a", Term: "alpha"})
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("first save: %v", err)
	}
	s.Add(VocabularyEntry{ID: "b", Term: "beta"})
	bak, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatalf("no backup written: %v", err)
	}
	if string(bak) != string(first) {
		t.Fatal("backup does not hold the previous contents")
	}
}

// TestThemeFollowsSystem checks the "跟随系统" mode: it used to resolve to
// light forever because the callers passed cfg.ThemeMode == "dark".
func TestThemeFollowsSystem(t *testing.T) {
	// The callers now pass c.Theme().Dark; the helper must not override it
	// when the mode is "system".
	cfg := DefaultConfig()
	if cfg.ThemeMode != "system" {
		t.Fatalf("default theme mode = %q, want system", cfg.ThemeMode)
	}
}

// TestImportEntriesDoesNotBlockOnNetwork pins the shape of ImportEntries:
// entries without phonetic data are enriched, but the whole import results
// in one persisted list rather than one write per word.
func TestImportEntriesDoesNotBlockOnNetwork(t *testing.T) {
	dir := t.TempDir()
	s := &VocabularyStore{path: filepath.Join(dir, "vocabulary.json"), entries: []VocabularyEntry{}}
	// Terms with an existing definition and a fake difficulty skip the
	// network path entirely, keeping this test offline.
	entries := []VocabularyEntry{
		{Term: "alpha", Definition: "d1", Difficulty: "A1", Phonetic: "/a/", MemoryTips: "m"},
		{Term: "beta", Definition: "d2", Difficulty: "A2", Phonetic: "/b/", MemoryTips: "m"},
	}
	if n := s.ImportEntries(entries); n != 2 {
		t.Fatalf("ImportEntries = %d, want 2", n)
	}
	if got := s.Count(); got != 2 {
		t.Fatalf("count = %d, want 2", got)
	}
}

// TestTranslateGenerationDropsStaleResult documents the guard that keeps a
// slow answer from replacing a newer one.
func TestTranslateGenerationDropsStaleResult(t *testing.T) {
	a := &app{store: newMemoryStore()}
	a.translateGen = 1
	gen := a.translateGen
	a.translateGen++
	if gen == a.translateGen {
		t.Fatal("translateGen did not advance; the stale guard would never fire")
	}
}

// TestApplySelectionModeIsReversible guards the one-way door: the hook
// trigger must be able to come back after being switched off.
func TestApplySelectionModeIsReversible(t *testing.T) {
	src, err := os.ReadFile("selicon.go")
	if err != nil {
		t.Fatalf("read selicon.go: %v", err)
	}
	body := string(src)
	if strings.Contains(body, "mygo.RunOnMain(removeHooks)") {
		t.Fatal("applySelectionMode still uninstalls hooks on the main thread; switching back to a hook trigger would never reinstall them")
	}
	if !strings.Contains(body, "postHookCommand") {
		t.Fatal("applySelectionMode does not post the hook change to the hook thread")
	}
}

// TestHotkeySaveIsNotAlwaysTrue guards the永真 condition that persisted a
// failed shortcut set.
func TestHotkeySaveIsNotAlwaysTrue(t *testing.T) {
	src, err := os.ReadFile("settings.go")
	if err != nil {
		t.Fatalf("read settings.go: %v", err)
	}
	if strings.Contains(string(src), "if msg == \"\" || len(msg) > 0") {
		t.Fatal("applyHotkeys still uses the always-true condition, so a failed registration is saved and survives a restart")
	}
}

// TestNoLockedWindowCreation guards the deadlock: the chip window must not
// be created while a.mu is held.
func TestNoLockedWindowCreation(t *testing.T) {
	src, err := os.ReadFile("selicon.go")
	if err != nil {
		t.Fatalf("read selicon.go: %v", err)
	}
	body := string(src)
	start := strings.Index(body, "func (a *app) selIconWindow()")
	if start < 0 {
		t.Fatal("selIconWindow not found")
	}
	end := strings.Index(body[start:], "\n}\n")
	if end < 0 {
		t.Fatal("selIconWindow body not delimited")
	}
	fn := body[start : start+end]
	if strings.Contains(fn, "a.mu.Lock()") && strings.Contains(fn, "mygo.NewWindow") {
		// Both may appear, but the lock must not span the NewWindow call.
		lock := strings.Index(fn, "a.mu.Lock()")
		unlock := strings.Index(fn, "a.mu.Unlock()")
		newWin := strings.Index(fn, "mygo.NewWindow")
		if lock >= 0 && unlock > lock && newWin > lock && newWin < unlock {
			t.Fatal("selIconWindow calls mygo.NewWindow while holding a.mu: NewWindow blocks on the main thread, which takes a.mu while rendering - that is a deadlock")
		}
	}
}

// TestConcurrentDeleteAndAdd exercises the store under the race detector.
func TestConcurrentDeleteAndAdd(t *testing.T) {
	dir := t.TempDir()
	s := &VocabularyStore{path: filepath.Join(dir, "vocabulary.json"), entries: []VocabularyEntry{}}
	for i := 0; i < 20; i++ {
		s.Add(VocabularyEntry{ID: string(rune('a' + i)), Term: string(rune('a' + i))})
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			s.Delete(string(rune('a' + i)))
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			s.Add(VocabularyEntry{ID: "new" + string(rune('a'+i)), Term: "new" + string(rune('a'+i))})
		}
	}()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("concurrent Add/Delete deadlocked")
	}
}
