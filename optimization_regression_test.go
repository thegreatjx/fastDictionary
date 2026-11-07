package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func waitForOptimizationState(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func noteSSE(content string) string {
	b, _ := json.Marshal(map[string]any{
		"choices": []any{map[string]any{"delta": map[string]string{"content": content}}},
	})
	return "data: " + string(b) + "\n\n"
}

func TestStoreGetReturnsIndependentSlices(t *testing.T) {
	s := newMemoryStore()
	first := s.Get()
	if len(first.CustomActions) == 0 || len(first.SelectionBlocklist) == 0 {
		t.Fatal("default config has no actions or blocklist entries")
	}
	first.CustomActions[0].Prompt = "changed by caller"
	first.CustomActions[0].OutputSchema[0] = "changed schema"
	first.SelectionBlocklist[0] = "changed.exe"

	second := s.Get()
	if second.CustomActions[0].Prompt == "changed by caller" {
		t.Error("Get returned a config that shares CustomActions backing storage")
	}
	if second.CustomActions[0].OutputSchema[0] == "changed schema" {
		t.Error("Get returned a config that shares OutputSchema backing storage")
	}
	if second.SelectionBlocklist[0] == "changed.exe" {
		t.Error("Get returned a config that shares SelectionBlocklist backing storage")
	}
}

func TestStoreUpdateSerialisesSaves(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	s := &store{cfg: DefaultConfig(), path: path}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(volume int) {
			defer wg.Done()
			s.Update(func(c *Config) { c.AudioVolume = volume })
		}(i)
	}
	wg.Wait()

	inMemory := s.Get().AudioVolume
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var onDisk Config
	if err := json.Unmarshal(data, &onDisk); err != nil {
		t.Fatalf("saved config is not valid JSON: %v", err)
	}
	if onDisk.AudioVolume != inMemory {
		t.Fatalf("disk AudioVolume = %d, memory = %d", onDisk.AudioVolume, inMemory)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "config.json" {
		t.Fatalf("save left temporary files: %v", entries)
	}
}

func TestActionCacheVariantInvalidatesPromptAndURL(t *testing.T) {
	cfg := DefaultConfig()
	act := cfg.GetAction("default-sentence-analysis")
	if act == nil {
		t.Fatal("default-sentence-analysis action missing")
	}
	base := actionCacheVariant(act, cfg)
	if base != "" {
		t.Fatalf("default action should use the old cache key, got %q", base)
	}

	act.Prompt += "\nchanged"
	if got := actionCacheVariant(act, cfg); got == base {
		t.Fatal("editing the action prompt did not change the cache variant")
	}

	cfg2 := DefaultConfig()
	cfg2.AIURL = "http://127.0.0.1:9999/v1/chat/completions"
	act2 := cfg2.GetAction("default-sentence-analysis")
	if got := actionCacheVariant(act2, cfg2); got == "" {
		t.Fatal("changing AIURL did not change the cache variant")
	}
}

func TestReflowTranslationKeepsParagraphsAndIndentedBlocks(t *testing.T) {
	got := reflowTranslation("First line.\nsecond line.\n\nSecond paragraph.")
	want := "First line. second line.\n\nSecond paragraph."
	if got != want {
		t.Fatalf("reflowTranslation = %q, want %q", got, want)
	}

	code := "func main() {\n    println(\"x\")\n}"
	if got := reflowTranslation(code); got != code {
		t.Fatalf("indented block was folded: %q", got)
	}
}

func TestStopAIPassClearsRefineError(t *testing.T) {
	a := newTestApp()
	a.aiRefineErr = "old error"
	cancelled := false
	a.aiCancel = func() { cancelled = true }

	a.mu.Lock()
	a.stopAIPassLocked()
	a.mu.Unlock()

	if !cancelled {
		t.Fatal("stopAIPassLocked did not cancel the in-flight pass")
	}
	if a.aiRefineErr != "" {
		t.Fatalf("aiRefineErr = %q, want empty", a.aiRefineErr)
	}
}

func TestExtractNotesRejectsStaleGeneration(t *testing.T) {
	a := newTestApp()
	a.notesGen = 7
	a.notesBusy = false

	a.extractNotesForGen(6, "This is a long sentence that would otherwise ask the model.")

	if a.notesBusy {
		t.Fatal("stale note extraction started against the current card")
	}
	if a.notesGen != 7 {
		t.Fatalf("stale note extraction changed notesGen to %d", a.notesGen)
	}
}

func TestExtractNotesDropsLateResultAfterCardChange(t *testing.T) {
	release := make(chan struct{})
	finished := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(noteSSE(`[{"word":"ambitious","phonetic":"/æmˈbɪʃ.əs/","pos":"adj.","meaning":"有雄心的","difficulty":"B2","memory_tips":"词源"}]`)))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		close(finished)
	}))
	defer srv.Close()

	a := newTestApp()
	a.store.Update(func(c *Config) {
		c.AIEnabled = true
		c.AIURL = srv.URL
		c.ShowNotes = true
		c.AIModel = "test"
	})
	a.mu.Lock()
	a.notesGen = 0
	a.mu.Unlock()

	a.extractNotesForGen(0, "This is a long sentence that would otherwise ask the model.")
	waitForOptimizationState(t, "the old note request to start", func() bool {
		a.mu.Lock()
		defer a.mu.Unlock()
		return a.notesBusy
	})

	// Simulate a newer card taking over without cancelling the old request.
	a.mu.Lock()
	a.notesGen++
	a.notesBusy = false
	a.notes = nil
	a.notesCancel = nil
	a.mu.Unlock()

	close(release)
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the old note request")
	}

	// The old goroutine must not write its answer into the new card.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		a.mu.Lock()
		got := a.notes
		a.mu.Unlock()
		if got != nil {
			t.Fatalf("stale note request wrote into the new card: %+v", got)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestApplyParsedActionKeepsPrefilledTranslation(t *testing.T) {
	a := newTestApp()
	a.grammarResult = &SentenceAnalysisResult{Translation: "机器译文"}
	a.grammarResultPrefill = true

	ok := a.applyParsedAction("default-sentence-analysis", `{"annotations":[{"text":"The report","type":"subject"}]}`)
	if !ok {
		t.Fatal("sentence analysis did not parse")
	}
	if a.grammarResult == nil || a.grammarResult.Translation != "机器译文" {
		t.Fatalf("prefilled translation was lost: %+v", a.grammarResult)
	}
	if a.grammarResultPrefill {
		t.Fatal("grammarResultPrefill stayed true after the model result was applied")
	}
}
