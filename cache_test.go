package main

import (
	"fmt"
	"path/filepath"
	"testing"
)

func waitForCacheFlush(t *testing.T, qc *queryCache) {
	t.Helper()
	qc.flush()
}

func TestQueryCacheDiskPersistenceAndLRU(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "query_cache.json")
	qc := newQueryCache(path)

	tKey := translationCacheKey("race", "auto", "zh-Hans", "compact", false, "  Hello world  ")
	tKeyNorm := translationCacheKey("race", "auto", "zh-Hans", "compact", false, "Hello world")
	if tKey != tKeyNorm {
		t.Fatalf("translationCacheKey did not trim whitespace: %q vs %q", tKey, tKeyNorm)
	}

	qc.putTranslation(tKey, cachedTranslation{
		Target:   "你好，世界",
		Engine:   "微软",
		Detected: "en",
		Dict:     &YoudaoDict{UKPhone: "həˈləʊ", Explains: []string{"int. 你好"}},
	})
	nKey := notesCacheKey("model-x", "  Critical minerals matter. ")
	qc.putNotes(nKey, []NoteItem{
		{Word: "critical", Meaning: "关键的"},
		{Word: "mineral", Meaning: "矿物"},
	})
	aiKey := aiResultCacheKey("default-sentence-analysis", "model-x", "zh-Hans", "  Only then did she realize.  ")
	qc.putAI(aiKey, `{"translation":"直到那时她才意识到。"}`)

	waitForCacheFlush(t, qc)
	reloaded := newQueryCache(path)

	if tr, ok := reloaded.getTranslation(tKeyNorm); !ok || tr.Target != "你好，世界" || tr.Dict == nil {
		t.Fatalf("reloaded translation mismatch: ok=%v tr=%+v", ok, tr)
	}
	if notes, ok := reloaded.getNotes(notesCacheKey("model-x", "Critical minerals matter.")); !ok || len(notes) != 2 {
		t.Fatalf("reloaded notes mismatch: ok=%v notes=%+v", ok, notes)
	}
	if raw := reloaded.getAI(aiResultCacheKey("default-sentence-analysis", "model-x", "zh-Hans", "Only then did she realize.")); raw == "" {
		t.Fatal("reloaded AI cache missing entry")
	}

	for i := 0; i < queryCacheLimit+5; i++ {
		qc.putTranslation(fmt.Sprintf("k-%d", i), cachedTranslation{Target: fmt.Sprintf("v-%d", i), Engine: "微软"})
	}
	waitForCacheFlush(t, qc)
	if _, ok := qc.getTranslation(tKey); ok {
		t.Fatal("oldest translation survived LRU eviction")
	}
	if tr, ok := qc.getTranslation(fmt.Sprintf("k-%d", queryCacheLimit+4)); !ok || tr.Target != fmt.Sprintf("v-%d", queryCacheLimit+4) {
		t.Fatalf("newest LRU entry missing: ok=%v tr=%+v", ok, tr)
	}
}

func TestPopupTranslationNotesAndRefineCacheHits(t *testing.T) {
	a := newTestApp()
	a.store.Update(func(c *Config) {
		c.AIEnabled = true
		c.AIURL = "http://127.0.0.1:1"
		c.ShowNotes = true
		c.DismissedNoteWords = []string{"dismissed"}
	})
	cfg := a.store.Get()

	const sentence = "The committee reviewed critical mineral exports yesterday."
	from, to := translateLangs(cfg, sentence)
	mode := normalizeLayoutMode(cfg.LayoutMode)
	tKey := translationCacheKey(cfg.EngineMode, from, to, mode, false, sentence)
	nKey := notesCacheKey(cfg.AIModel, sentence)

	qc := a.ensureCache()
	qc.putTranslation(tKey, cachedTranslation{
		Target:   "委员会昨天审查了关键矿产出口。",
		Engine:   "谷歌",
		Detected: "en",
	})
	qc.putNotes(nKey, []NoteItem{
		{Word: "dismissed", Pos: "v.", Meaning: "驳回"},
		{Word: "mineral", Pos: "n.", Meaning: "矿产"},
		{Word: "committee", Pos: "n.", Meaning: "委员会"},
	})

	a.translateTextWithReflow(sentence, false)

	a.mu.Lock()
	target := a.target
	busy := a.busy
	notesBusy := a.notesBusy
	notes := append([]NoteItem(nil), a.notes...)
	a.mu.Unlock()

	if busy || notesBusy {
		t.Fatalf("cache hit left busy flags set: busy=%v notesBusy=%v", busy, notesBusy)
	}
	if target != "委员会昨天审查了关键矿产出口。" {
		t.Fatalf("cached translation mismatch: got %q", target)
	}
	if len(notes) != 2 || notes[0].Word != "mineral" || notes[1].Word != "committee" {
		t.Fatalf("dismissed-filtered cached notes mismatch: %+v", notes)
	}
	if rawNotes, ok := qc.getNotes(nKey); !ok || len(rawNotes) != 3 {
		t.Fatalf("raw notes cache should keep all 3 items before dismissal filter, got %+v", rawNotes)
	}

	_, refineTo := translateLangs(cfg, sentence)
	rKey := aiResultCacheKey("ai-refine", cfg.AIModel, refineTo, sentence,
		normalizeAIProvider(cfg.AIProvider), cfg.AIURL, cfg.AIReasoningEffortFor(cfg.AIModel))
	qc.putAI(rKey, "该委员会昨日审议了关键矿产出口事宜。")

	a.refineWithAI()
	a.mu.Lock()
	refined := a.target
	refining := a.aiRefining
	eng := a.engine
	a.mu.Unlock()
	if refining || eng != "AI 精翻" || refined != "该委员会昨日审议了关键矿产出口事宜。" {
		t.Fatalf("AI refine cache hit failed: refining=%v eng=%q target=%q", refining, eng, refined)
	}

	a.mode = modeTranslate
	a.rerunCurrentModeFresh(sentence)
	a.mu.Lock()
	freshBusy := a.busy
	a.mu.Unlock()
	if !freshBusy {
		t.Fatal("rerunCurrentModeFresh did not bypass translation cache")
	}
}
