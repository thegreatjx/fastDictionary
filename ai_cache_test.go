package main

import (
	"strings"
	"testing"
)

// TestAnalysisCacheKeepsOneAnswerPerText is the regression test for the user's
// "长难句每次结果好像都不一样" report. Two looks at the same sentence must show
// the same card, and the 重新翻译 button must be able to ask for a new one.
func TestAnalysisCacheKeepsOneAnswerPerText(t *testing.T) {
	a := newTestApp()
	key := aiResultCacheKey("default-sentence-analysis", "group/model", "zh-Hans", "She cooked the books.")

	a.mu.Lock()
	if got := a.cachedAIResult(key); got != "" {
		a.mu.Unlock()
		t.Fatalf("empty cache returned %q", got)
	}
	a.storeAIResult(key, `{"annotations":[{"text":"She","type":"subject"}]}`)
	first := a.cachedAIResult(key)
	if first == "" {
		a.mu.Unlock()
		t.Fatal("stored answer was not retrievable")
	}
	// A second pass over the same text overwrites rather than appending, so the
	// cache cannot grow per look.
	a.storeAIResult(key, `{"annotations":[{"text":"She","type":"subject"},{"text":"cooked","type":"predicate"}]}`)
	second := a.cachedAIResult(key)
	a.mu.Unlock()

	if second == first {
		t.Fatal("the newest answer did not replace the older one")
	}
	if len(a.aiResultOrder) != 1 {
		t.Fatalf("one text produced %d cache slots, want 1", len(a.aiResultOrder))
	}
}

// TestAnalysisCacheEvictsOldest keeps the map from growing without bound.
func TestAnalysisCacheEvictsOldest(t *testing.T) {
	a := newTestApp()
	a.mu.Lock()
	for i := 0; i < aiResultCacheLimit+3; i++ {
		a.storeAIResult(aiResultCacheKey("act", "m", "zh-Hans", string(rune('a'+i))), "{}")
	}
	a.mu.Unlock()

	if len(a.aiResultOrder) != aiResultCacheLimit {
		t.Fatalf("cache holds %d entries, want %d", len(a.aiResultOrder), aiResultCacheLimit)
	}
	a.mu.Lock()
	oldest := a.cachedAIResult(aiResultCacheKey("act", "m", "zh-Hans", "a"))
	a.mu.Unlock()
	if oldest != "" {
		t.Fatalf("oldest entry %q survived eviction", oldest)
	}
}

// TestCacheHitSkipsTheModel proves the cached answer is used without a network
// call: runAction must return a card even though magpie is unreachable here.
func TestCacheHitSkipsTheModel(t *testing.T) {
	a := newTestApp()
	a.store.Update(func(c *Config) { c.AIEnabled = true })
	cfg := a.store.Get()
	act := cfg.GetAction("default-sentence-analysis")
	if act == nil {
		t.Fatal("default-sentence-analysis action missing from the default config")
	}
	model := act.Model
	if model == "" {
		model = cfg.AIModel
	}

	const sentence = "She cooked the books."
	key := aiResultCacheKey("default-sentence-analysis", model, cfg.TargetLang, sentence)
	a.mu.Lock()
	a.storeAIResult(key, `{"annotations":[{"text":"She","type":"subject"},{"text":"cooked","type":"predicate"}],"translation":"她做假账。"}`)
	a.mu.Unlock()

	a.runAction("default-sentence-analysis", sentence)

	a.mu.Lock()
	res := a.grammarResult
	busy := a.aiBusy
	raw := a.aiText
	a.mu.Unlock()

	if res == nil {
		t.Fatal("cache hit did not produce a result")
	}
	if res.Translation != "她做假账。" {
		t.Fatalf("translation %q, want the cached one", res.Translation)
	}
	if len(res.Annotations) != 2 {
		t.Fatalf("got %d annotations, want the 2 from the cache", len(res.Annotations))
	}
	if busy {
		t.Error("a cache hit still marked the card as loading")
	}
	if strings.Contains(raw, "{") {
		t.Errorf("cache hit left raw payload in aiText: %q", raw)
	}
}

// TestRefreshBypassesTheCache guards the other half: 重新翻译 has to ask the
// model again, or the button would silently do nothing on a cached sentence.
func TestRefreshBypassesTheCache(t *testing.T) {
	a := newTestApp()
	a.store.Update(func(c *Config) { c.AIEnabled = true })
	cfg := a.store.Get()
	act := cfg.GetAction("default-sentence-analysis")
	model := act.Model
	if model == "" {
		model = cfg.AIModel
	}
	const sentence = "She cooked the books."
	a.mu.Lock()
	a.storeAIResult(aiResultCacheKey("default-sentence-analysis", model, cfg.TargetLang, sentence), `{"translation":"cached"}`)
	a.mu.Unlock()

	a.mode = modeGrammar
	a.source = sentence
	a.rerunCurrentModeFresh(sentence)

	a.mu.Lock()
	busy := a.aiBusy
	a.mu.Unlock()
	if !busy {
		t.Fatal("重新翻译 was served from the cache instead of starting a new pass")
	}
}
