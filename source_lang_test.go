package main

import "testing"

// TestSourceLangForLoneWord pins the fix for "fast" coming back as 几乎: a
// single ASCII word carries no language evidence, the engines detect it
// themselves, and 微软 read 8 of 25 common English words as German, Spanish or
// Dutch. A lone word is what this app is opened for, so it goes out as English.
func TestSourceLangForLoneWord(t *testing.T) {
	cases := map[string]string{
		"fast":           "en",
		"Fast":           "en",
		"fastDictionary": "en",
		"war":            "en",
		// Sentences keep the old behaviour: the engines detect these, and
		// guessing "en" for ASCII was the bug that read French with an
		// English voice.
		"fast dictionary":                 "",
		"I use fastDictionary every day.": "",
		// Anything the script settles is left alone.
		"你好":    "zh-Hans",
		"こんにちは": "ja",
		// Not one plain word: punctuation, digits, accented letters, empty.
		"don't": "",
		"fast2": "",
		"café":  "",
		"":      "",
	}
	for in, want := range cases {
		if got := sourceLangFor(in); got != want {
			t.Errorf("sourceLangFor(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLoneLatinWord(t *testing.T) {
	for in, want := range map[string]bool{
		"fast": true, "FastDictionary": true, "F": true,
		"fast dictionary": false, "fast-dictionary": false, "fast_dictionary": false,
		"fast2": false, "café": false, "": false, "  ": false,
	} {
		if got := loneLatinWord(in); got != want {
			t.Errorf("loneLatinWord(%q) = %v, want %v", in, got, want)
		}
	}
}
