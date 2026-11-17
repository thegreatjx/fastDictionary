package main

import "testing"

func TestSourceLangForLoneWord(t *testing.T) {
	cases := map[string]string{
		"fast":                            "en",
		"Fast":                            "en",
		"fastDictionary":                  "en",
		"war":                             "en",
		"fast dictionary":                 "",
		"I use fastDictionary every day.": "",
		"你好":                              "zh-Hans",
		"こんにちは":                           "ja",
		"don't":                           "",
		"fast2":                           "",
		"café":                            "",
		"":                                "",
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
