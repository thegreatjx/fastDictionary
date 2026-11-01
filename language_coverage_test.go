package main

import (
	"strings"
	"testing"
)

// TestDetectLocalCoversNonCJKScripts is the regression test for a bug that made
// a large part of the world untranslatable.
//
// detectLocal answered "en" for anything that was not Chinese, Japanese or
// Korean. A Russian or Arabic or Thai selection was therefore sent to the
// engine labelled as English, and the engine either translated from the wrong
// language or refused. The scripts have to be recognised for what they are.
func TestDetectLocalCoversNonCJKScripts(t *testing.T) {
	cases := []struct{ text, want, why string }{
		{"สวัสดีครับ วันนี้อากาศดีมาก", "th", "Thai"},
		{"مرحبا، الطقس جميل جدا اليوم", "ar", "Arabic"},
		{"שלום, מזג האוויר נחמד מאוד היום", "he", "Hebrew"},
		{"नमस्ते, आज मौसम बहुत अच्छा है", "hi", "Devanagari"},
		{"Привет, сегодня очень хорошая погода", "ru", "Cyrillic"},
		{"Γεια σου, ο καιρός είναι πολύ ωραίος", "el", "Greek"},
		// CJK still wins over the Latin counter, as it did before.
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

// TestDetectLocalLeavesAmbiguousLatinToTheEngine pins the other half: Latin
// script alone cannot say which language it is, so detection stays out of it
// and the engine decides. Guessing "en" here is what sent French text to the
// translator as English and read it aloud with an English voice.
func TestDetectLocalLeavesAmbiguousLatinToTheEngine(t *testing.T) {
	for _, text := range []string{
		"hello world",                             // English, but ASCII-only
		"Hallo, het weer is vandaag erg mooi",     // Dutch
		"Olá, o tempo está muito bom hoje",        // Portuguese
		"Cześć, dzisiaj jest bardzo ładna pogoda", // Polish
		"Hej, vädret är väldigt fint idag",        // Swedish
	} {
		if got := detectLocal(text); got != "" {
			t.Errorf("detectLocal(%q) = %q, want \"\" so the engine detects it", text, got)
		}
	}
}

// TestLooksEnglishSeparatesEnglishFromOtherLatinScript covers the speech
// fallback: a sentence of plain ASCII still has to be read by some voice, and
// the stopwords are what tell an English one from a French or Dutch one.
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
		"Il fait beau aujourd'hui",            // French
		"Das Wetter ist schön",                // German
		"Hallo, het weer is vandaag erg mooi", // Dutch
		"Bonjour",                             // one word, no stopwords
		"日本語のテキスト",                            // not Latin at all
	}
	for _, s := range notEnglish {
		if looksEnglish(s) {
			t.Errorf("looksEnglish(%q) = true; it is not English", s)
		}
	}
}

// TestLangOptionsAreLabelled guards the picker: every offered language needs a
// name to show, and every name needs a language.
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
		// The label must map back, or picking it changes the language.
		if got := langFromLabel(label); got != code {
			t.Errorf("langFromLabel(%q) = %q, want %q", label, got, code)
		}
	}
}

// TestLangOptionsCoverTheWorld guards the breadth the engines already had.
//
// Microsoft and Google both accepted all of these when tested; the limit was
// this list. A user reading Thai had no target to pick at all.
func TestLangOptionsCoverTheWorld(t *testing.T) {
	want := []string{
		"zh-Hans", "zh-Hant", "en", "ja", "ko", // the core set
		"th", "vi", "id", // Southeast Asia
		"fr", "de", "es", "pt", "it", "ru", "pl", "tr", // Europe
		"ar", "hi", // Middle East and South Asia
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
