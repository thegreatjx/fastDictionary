package main

import (
	"context"
	"strings"
	"testing"
)

// TestFixDoubledLetterRepairsOcrNoise covers the errors the recognizer actually
// produces on printed English.
//
// Every input here was taken from a real run over a newspaper page. The repairs
// matter because a single doubled letter changes what the translator thinks the
// subject is: "Sttocks in bold rose or fell 5%" came back as "加粗的数字涨跌5%"
// while the correct "Stocks" gives "加粗的股票涨跌5%".
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

// TestFixDoubledLetterKeepsRealWords is the guard that makes the repair safe.
//
// English has plenty of words with genuinely doubled letters. Rewriting any of
// them would corrupt text that was already correct, which is worse than the bug
// being fixed: a doubled letter can still be read, a wrong word cannot.
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

// TestFixDoubledLetterLeavesUnprovableAlone covers the cases where collapsing
// does not produce a word. Guessing here would invent text.
func TestFixDoubledLetterLeavesUnprovableAlone(t *testing.T) {
	for _, w := range []string{
		"felll",     // collapses to "fel", not a word
		"Maatinees", // collapses to "matines", not a word
		"usaatoday", // a URL fragment, not in any word list
		"continuousll",
		"5%",       // not letters at all
		"723-8698", // a phone number
		"don't",    // an apostrophe: the list does not carry contractions
		"Stocks",   // already correct
		"ab",       // too short to judge
	} {
		if got := fixDoubledLetter(w); got != w {
			t.Errorf("fixDoubledLetter(%q) = %q, want it unchanged", w, got)
		}
	}
}

// TestFixDoubledLettersHandlesPunctuation pins that the last word of a line is
// still repaired. OCR ends lines with commas and periods, and a repair that
// skipped them would miss most of the words at line ends.
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

// TestFixDoubledLettersMultiLine keeps a table from being joined into one line.
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

// TestSpellListLoaded guards the embedded word list against being emptied by an
// over-eager clean-up. Without it the repair silently does nothing.
func TestSpellListLoaded(t *testing.T) {
	words := spellWords()
	if len(words) < 10000 {
		t.Fatalf("word list has only %d entries; the repair would be useless", len(words))
	}
	for _, w := range []string{"stocks", "with", "miles", "nasdaq", "saturday"} {
		if _, ok := words[w]; !ok {
			t.Errorf("word list is missing %q, so that repair cannot fire", w)
		}
	}
}

// TestOCRRepairEndToEnd runs the real engine on the page whose doubled letters
// started this. It skips when the models are not installed.
func TestOCRRepairEndToEnd(t *testing.T) {
	txt := liveIndraText(t, "docs/ocr-bench/stock_gs200.jpg")
	if txt == "" {
		t.Skip("no text recognised")
	}
	// The heading used to come back as "Nasddaq && AMEX".
	if strings.Contains(txt, "Nasddaq") {
		t.Errorf("doubled letter survived the repair:\n%s", txt)
	}
	if !strings.Contains(txt, "Nasdaq") {
		t.Logf("heading not recognised as Nasdaq; full text:\n%s", txt)
	}
}

// TestRepairKeepsTranslationMeaning is the reason the repair exists: the
// doubled letter changed what the sentence was about.
func TestRepairKeepsTranslationMeaning(t *testing.T) {
	broken := "Sttocks in bold rose or fell 5% or more"
	fixed := fixDoubledLetters(broken)
	if !strings.Contains(fixed, "Stocks") {
		t.Fatalf("repair did not fire: %q", fixed)
	}
	// And the repaired text still translates, which is the whole point.
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
