package main

import (
	"strings"
	"testing"
)

func TestAnalysisCacheKeepsOneAnswerPerText(t *testing.T) {
	a := newTestApp()
	key := aiResultCacheKey("default-sentence-analysis", "group/model", "zh-Hans", "She cooked the books.")

	a.mu.Lock()
	if got := a.cachedAIResult(key); got != "" {
		a.mu.Unlock()
		t.Fatalf("empty cache returned %q", got)
	}
	a.storeAIResult(key, `{"segments":[{"text":"She","role":"subject","is_backbone":true}]}`)
	first := a.cachedAIResult(key)
	if first == "" {
		a.mu.Unlock()
		t.Fatal("stored answer was not retrievable")
	}
	a.storeAIResult(key, `{"segments":[{"text":"She","role":"subject","is_backbone":true},{"text":" cooked","role":"predicate","is_backbone":true}]}`)
	second := a.cachedAIResult(key)
	a.mu.Unlock()

	if second == first {
		t.Fatal("the newest answer did not replace the older one")
	}
	if got := a.aiCacheCount(); got != 1 {
		t.Fatalf("one text produced %d cache slots, want 1", got)
	}
}

func TestAnalysisCacheEvictsOldest(t *testing.T) {
	a := newTestApp()
	a.mu.Lock()
	for i := 0; i < queryCacheLimit+3; i++ {
		a.storeAIResult(aiResultCacheKey("act", "m", "zh-Hans", string(rune('a'+i))), "{}")
	}
	a.mu.Unlock()

	if got := a.aiCacheCount(); got != queryCacheLimit {
		t.Fatalf("cache holds %d entries, want %d", got, queryCacheLimit)
	}
	a.mu.Lock()
	oldest := a.cachedAIResult(aiResultCacheKey("act", "m", "zh-Hans", "a"))
	a.mu.Unlock()
	if oldest != "" {
		t.Fatalf("oldest entry %q survived eviction", oldest)
	}
}

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
	a.storeAIResult(key, `{"segments":[{"text":"She","role":"subject","is_backbone":true},{"text":" cooked","role":"predicate","is_backbone":true},{"text":" the books","role":"object","is_backbone":true},{"text":".","role":"punct"}],"translation":"她做假账。"}`)
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
	if len(res.Segments) != 4 {
		t.Fatalf("got %d segments, want the 4 from the cache", len(res.Segments))
	}
	if busy {
		t.Error("a cache hit still marked the card as loading")
	}
	if strings.Contains(raw, "{") {
		t.Errorf("cache hit left raw payload in aiText: %q", raw)
	}
}

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
