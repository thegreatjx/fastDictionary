package main

import (
	"context"
	"strings"
	"testing"
)

func TestFixDoubledLetterRepairsOcrNoise(t *testing.T) {
	cases := map[string]string{
		"Sttocks":       "Stocks",
		"witth":         "with",
		"miless":        "miles",
		"Stockks":       "Stocks",
		"Trackk":        "Track",
		"Saturddays":    "Saturdays",
		"Sundday":       "Sunday",
		"inveestmentts": "investments",
		"Nasddaq":       "Nasdaq",
	}
	for in, want := range cases {
		if got := fixDoubledLetter(in); got != want {
			t.Errorf("fixDoubledLetter(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFixDoubledLetterKeepsRealWords(t *testing.T) {
	keep := []string{
		"letter", "better", "coffee", "address", "success", "really",
		"all", "will", "well", "need", "book", "look", "see", "tree",
		"three", "week", "class", "process", "office", "committee",
	}
	for _, w := range keep {
		if got := fixDoubledLetter(w); got != w {
			t.Errorf("fixDoubledLetter(%q) = %q; a real word must not be rewritten", w, got)
		}
	}
}

func TestFixDoubledLetterLeavesUnprovableAlone(t *testing.T) {
	for _, w := range []string{
		"felll",
		"Maatinees",
		"usaatoday",
		"continuousll",
		"5%",
		"723-8698",
		"don't",
		"Stocks",
		"ab",
	} {
		if got := fixDoubledLetter(w); got != w {
			t.Errorf("fixDoubledLetter(%q) = %q, want it unchanged", w, got)
		}
	}
}

func TestFixDoubledLettersHandlesPunctuation(t *testing.T) {
	in := "Sttocks in bold, rose or fell 5%."
	got := fixDoubledLetters(in)
	if !strings.Contains(got, "Stocks") {
		t.Errorf("fixDoubledLetters(%q) = %q, want the leading word repaired", in, got)
	}
	if !strings.HasSuffix(got, "5%.") {
		t.Errorf("fixDoubledLetters(%q) = %q, punctuation must survive", in, got)
	}
}

func TestFixDoubledLettersMultiLine(t *testing.T) {
	in := "Nasddaq & AMEX\nSttocks in bold rose\nUSA"
	got := fixDoubledLettersAll(in)
	lines := strings.Split(got, "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3: %q", len(lines), got)
	}
	if lines[0] != "Nasdaq & AMEX" {
		t.Errorf("line 0 = %q, want %q", lines[0], "Nasdaq & AMEX")
	}
	if lines[2] != "USA" {
		t.Errorf("line 2 = %q, want %q", lines[2], "USA")
	}
}

func TestSpellListLoaded(t *testing.T) {
	words := spellWordList()
	if len(words) < 10000 {
		t.Fatalf("word list has only %d entries; the repair would be useless", len(words))
	}
	for _, w := range []string{"stocks", "with", "miles", "nasdaq", "saturday"} {
		if !hasSpellWord(w) {
			t.Errorf("word list is missing %q, so that repair cannot fire", w)
		}
	}
}

func TestOCRRepairEndToEnd(t *testing.T) {
	txt := liveIndraText(t, "docs/ocr-bench/stock_gs200.jpg")
	if txt == "" {
		t.Skip("no text recognised")
	}
	if strings.Contains(txt, "Nasddaq") {
		t.Errorf("doubled letter survived the repair:\n%s", txt)
	}
	if !strings.Contains(txt, "Nasdaq") {
		t.Logf("heading not recognised as Nasdaq; full text:\n%s", txt)
	}
}

func TestRepairKeepsTranslationMeaning(t *testing.T) {
	broken := "Sttocks in bold rose or fell 5% or more"
	fixed := fixDoubledLetters(broken)
	if !strings.Contains(fixed, "Stocks") {
		t.Fatalf("repair did not fire: %q", fixed)
	}
	if testing.Short() {
		t.Skip("network")
	}
	r := TranslateRace(context.Background(), fixed, "auto", "zh-Hans", "race")
	if r.Err != nil {
		t.Skipf("translate unavailable: %v", r.Err)
	}
	if !strings.Contains(r.Text, "股票") {
		t.Errorf("repaired text translated to %q; expected it to mention 股票 (stocks)", r.Text)
	}
}
