package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

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

func TestEngineCaptionStaysQuietWhenSourceIsTheTarget(t *testing.T) {
	got := translationEngineCaption(snapshot{engine: "微软", detected: "zh-Hans"}, "zh-Hans")
	if strings.Contains(got, "翻译自") {
		t.Errorf("caption = %q, names a source language for same-language text", got)
	}
	if got != "微软" {
		t.Errorf("caption = %q, want just the engine", got)
	}
}

func TestEngineCaptionKeepsEngineAndTiming(t *testing.T) {
	got := translationEngineCaption(snapshot{engine: "AI 精翻", detected: "es"}, "zh-Hans")
	if !strings.Contains(got, "AI 精翻") {
		t.Errorf("caption = %q, lost the engine", got)
	}
	if !strings.Contains(got, "翻译自 西班牙语") {
		t.Errorf("caption = %q, lost the source language", got)
	}
}

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
