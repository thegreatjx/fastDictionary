package main

import (
	"strings"
	"testing"
)

func TestDetectLocalCoversNonCJKScripts(t *testing.T) {
	cases := []struct{ text, want, why string }{
		{"สวัสดีครับ วันนี้อากาศดีมาก", "th", "Thai"},
		{"مرحبا، الطقس جميل جدا اليوم", "ar", "Arabic"},
		{"שלום, מזג האוויר נחמד מאוד היום", "he", "Hebrew"},
		{"नमस्ते, आज मौसम बहुत अच्छा है", "hi", "Devanagari"},
		{"Привет, сегодня очень хорошая погода", "ru", "Cyrillic"},
		{"Γεια σου, ο καιρός είναι πολύ ωραίος", "el", "Greek"},
		{"你好世界", "zh-Hans", "Chinese"},
		{"こんにちは", "ja", "Japanese"},
		{"안녕하세요", "ko", "Korean"},
	}
	for _, c := range cases {
		if got := detectLocal(c.text); got != c.want {
			t.Errorf("detectLocal(%s text) = %q, want %q", c.why, got, c.want)
		}
	}
}

func TestDetectLocalLeavesAmbiguousLatinToTheEngine(t *testing.T) {
	for _, text := range []string{
		"hello world",
		"Hallo, het weer is vandaag erg mooi",
		"Olá, o tempo está muito bom hoje",
		"Cześć, dzisiaj jest bardzo ładna pogoda",
		"Hej, vädret är väldigt fint idag",
	} {
		if got := detectLocal(text); got != "" {
			t.Errorf("detectLocal(%q) = %q, want \"\" so the engine detects it", text, got)
		}
	}
}

func TestDetectLocalKeepsChineseWhenLatinTermsOutnumberIt(t *testing.T) {
	chinese := []struct{ text, why string }{
		{"Claude 这个 Building Always on Agents，实际是 Personal Agent 的最基本的设计模式。", "英文术语多于汉字"},
		{"之前我们在 Redis 上做过类似的 cache layer 设计，效果不错。", "同上"},
	}
	for _, c := range chinese {
		if got := detectLocal(c.text); got != "zh-Hans" {
			t.Errorf("detectLocal(%s) = %q, want \"zh-Hans\"：鉴定成空会让中文原文被当成翻译目标", c.why, got)
		}
	}

	for _, text := range []string{
		"The word 中文 appears here.",
		"This is a plain English sentence.",
	} {
		if got := detectLocal(text); got != "" {
			t.Errorf("detectLocal(%q) = %q, want \"\" so the engine detects it", text, got)
		}
	}
}

func TestLooksEnglishSeparatesEnglishFromOtherLatinScript(t *testing.T) {
	english := []string{
		"The report was never discussed in public.",
		"It is what it is.",
		"I have to go and see the doctor.",
	}
	for _, s := range english {
		if !looksEnglish(s) {
			t.Errorf("looksEnglish(%q) = false; an English sentence with stopwords should pass", s)
		}
	}
	notEnglish := []string{
		"Il fait beau aujourd'hui",
		"Das Wetter ist schön",
		"Hallo, het weer is vandaag erg mooi",
		"Bonjour",
		"日本語のテキスト",
	}
	for _, s := range notEnglish {
		if looksEnglish(s) {
			t.Errorf("looksEnglish(%q) = true; it is not English", s)
		}
	}
}

func TestLangOptionsAreLabelled(t *testing.T) {
	for _, code := range langOptions {
		if langLabels[code] == "" {
			t.Errorf("langOptions offers %q with no label", code)
		}
	}
	for code, label := range langLabels {
		if !strings.Contains(strings.Join(langOptions, " "), code) {
			t.Errorf("langLabels has %q (%s) but langOptions does not offer it", code, label)
		}
		if got := langFromLabel(label); got != code {
			t.Errorf("langFromLabel(%q) = %q, want %q", label, got, code)
		}
	}
}

func TestLangOptionsCoverTheWorld(t *testing.T) {
	want := []string{
		"zh-Hans", "zh-Hant", "en", "ja", "ko",
		"th", "vi", "id",
		"fr", "de", "es", "pt", "it", "ru", "pl", "tr",
		"ar", "hi",
	}
	have := map[string]bool{}
	for _, code := range langOptions {
		have[code] = true
	}
	for _, code := range want {
		if !have[code] {
			t.Errorf("langOptions does not offer %q (%s), which both engines support",
				code, langLabels[code])
		}
	}
	if len(langOptions) < 20 {
		t.Errorf("langOptions has %d languages; a reader of most of the world has no target to pick",
			len(langOptions))
	}
}
