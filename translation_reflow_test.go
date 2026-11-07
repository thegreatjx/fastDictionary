package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestTranslationReflowsEngineLineBreaks verifies that multi-line translation
// answers that copy paragraph/line breaks from the source are flattened so the
// card wraps naturally without leaving blank bands between sentences.
func TestTranslationReflowsEngineLineBreaks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[[["等等，OpenAI居然真的在做这个？！\n\n他们把Codex的线束直接嵌入到Agents SDK里\n\n你无需自行构建和维护执行工具，可以直接在SDK中使用\nCodex"]],null,"en"]`)
	}))
	defer srv.Close()

	oldURL := googleSingleURL
	googleSingleURL = srv.URL
	defer func() { googleSingleURL = oldURL }()

	a := newTestApp()
	a.store.Update(func(c *Config) {
		c.EngineMode = "google"
		c.ShowNotes = false
		c.DictEnabled = false
		c.AutoPronounce = "off"
	})

	a.translate("Wait, OpenAI is actually doing this?!")
	waitForStaleState(t, "reflowed translation", func() bool {
		a.mu.Lock()
		defer a.mu.Unlock()
		return !a.busy
	})

	a.mu.Lock()
	target := a.target
	a.mu.Unlock()

	if strings.ContainsAny(target, "\n\r") {
		t.Fatalf("target still has line breaks: %q", target)
	}
	want := "等等，OpenAI居然真的在做这个？！他们把Codex的线束直接嵌入到Agents SDK里你无需自行构建和维护执行工具，可以直接在SDK中使用 Codex"
	if target != want {
		t.Fatalf("target = %q, want %q", target, want)
	}
}

func TestGrammarPrefillReflowsEngineLineBreaks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[[["第一行翻译。\n\n第二行翻译。"]],null,"en"]`)
	}))
	defer srv.Close()

	oldURL := googleSingleURL
	googleSingleURL = srv.URL
	defer func() { googleSingleURL = oldURL }()

	a := newTestApp()
	a.store.Update(func(c *Config) {
		c.EngineMode = "google"
		c.ShowNotes = false
		c.DictEnabled = false
		c.AutoPronounce = "off"
	})

	a.mu.Lock()
	gen := a.aiGen
	a.mu.Unlock()

	a.prefillActionTranslation(gen, "First line.\n\nSecond line.", "zh-Hans")
	waitForStaleState(t, "grammar prefill translation", func() bool {
		a.mu.Lock()
		defer a.mu.Unlock()
		return a.grammarResult != nil && a.grammarResult.Translation != ""
	})

	a.mu.Lock()
	trans := a.grammarResult.Translation
	a.mu.Unlock()

	if strings.ContainsAny(trans, "\n\r") {
		t.Fatalf("grammar translation still has line breaks: %q", trans)
	}
	want := "第一行翻译。第二行翻译。"
	if trans != want {
		t.Fatalf("grammar translation = %q, want %q", trans, want)
	}
}
