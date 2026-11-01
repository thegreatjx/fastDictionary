package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestEngineCaptionNamesTheSourceLanguage: the caption used to print the raw
// detection tag (检测 en), which answers "what language was this?" in a form a
// reader has to decode. Grok's 翻译自 西班牙语 is the shape people are used to,
// and it is the same amount of space.
func TestEngineCaptionNamesTheSourceLanguage(t *testing.T) {
	s := snapshot{engine: "微软", detected: "en"}
	got := translationEngineCaption(s, "zh-Hans")
	if !strings.Contains(got, "翻译自 英语") {
		t.Errorf("caption = %q, want it to name the source language", got)
	}
	if strings.Contains(got, "检测") {
		t.Errorf("caption = %q, still shows the raw 检测 tag", got)
	}
	if strings.Contains(got, "en") {
		t.Errorf("caption = %q, still contains the bare language code", got)
	}
}

// TestEngineCaptionStaysQuietWhenSourceIsTheTarget keeps the caption from
// saying "翻译自 简体中文" above text that was already Chinese.
func TestEngineCaptionStaysQuietWhenSourceIsTheTarget(t *testing.T) {
	got := translationEngineCaption(snapshot{engine: "微软", detected: "zh-Hans"}, "zh-Hans")
	if strings.Contains(got, "翻译自") {
		t.Errorf("caption = %q, names a source language for same-language text", got)
	}
	if got != "微软" {
		t.Errorf("caption = %q, want just the engine", got)
	}
}

// TestEngineCaptionKeepsEngineAndTiming: the caption still has to say where the
// text came from and how long it took, or the 精翻 fix (which labelled the
// footer with the real source) would be undone for the in-card caption.
func TestEngineCaptionKeepsEngineAndTiming(t *testing.T) {
	got := translationEngineCaption(snapshot{engine: "AI 精翻", detected: "es"}, "zh-Hans")
	if !strings.Contains(got, "AI 精翻") {
		t.Errorf("caption = %q, lost the engine", got)
	}
	if !strings.Contains(got, "翻译自 西班牙语") {
		t.Errorf("caption = %q, lost the source language", got)
	}
}

// TestLangDisplayNameHandlesEngineTags: each engine reports its own shape of
// tag. A prefix match keeps "es-419" and "EN" readable; an unknown tag is
// echoed rather than blanked, so the caption never claims a language it does
// not know.
func TestLangDisplayNameHandlesEngineTags(t *testing.T) {
	cases := map[string]string{
		"en":      "英语",
		"EN":      "英语",
		"es":      "西班牙语",
		"es-419":  "西班牙语",
		"zh-Hans": "简体中文",
		"zh-TW":   "繁体中文",
		"ja":      "日语",
		"x-kling": "x-kling",
	}
	for tag, want := range cases {
		if got := langDisplayName(tag); got != want {
			t.Errorf("langDisplayName(%q) = %q, want %q", tag, got, want)
		}
	}
}

// TestTranslateCardShowsTheCaptionRendered is the wiring test: the function can
// be perfect and still not be on screen.
func TestTranslateCardShowsTheCaptionRendered(t *testing.T) {
	a := newTestApp()
	a.store.Update(func(c *Config) { c.ShowEngineTag = true; c.TargetLang = "zh-Hans" })
	a.mu.Lock()
	a.mode = modeTranslate
	a.source = "She cooked the books to hide the losses."
	a.target = "她做假账以掩盖亏损。"
	a.engine = "微软"
	a.detected = "en"
	a.mu.Unlock()

	tt := ui.NewTester(func(c *ui.Context) {
		a.popupView(c)
	}, 500, 700)
	if !tt.HasText("翻译自 英语") {
		t.Fatalf("the translate card does not show the source language: %v", tt.Texts())
	}
}
