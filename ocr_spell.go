package main

import (
	_ "embed"
	"strings"
	"sync"
)

// The recognizer sometimes doubles a letter inside a word.
//
// It is a small error with an outsized effect. "Sttocks" is still readable to a
// person, but the translator reads it as an unknown token and guesses from
// context: "Stocks in bold rose or fell 5%" came back as "加粗的数字涨跌5%",
// where the correct word gives "加粗的股票涨跌5%". The user sees a translation
// that is confidently about the wrong thing, with no sign that a single letter
// went wrong on the way in.
//
// The doubling happens at the recognizer, not in the crop: sweeping the input
// height, both recognizers and every upscale factor, a given line always keeps
// at least one doubled letter. So it is repaired after decoding, where the
// evidence — the word list — is cheap to consult.
//
// The repair is deliberately one-directional. A word is only rewritten when it
// is not a word itself and collapsing its runs of doubled letters gives a word
// that is. That keeps "letter", "success", "really", "all" and the rest of the
// genuinely doubled English words untouched, because they are already words.

//go:embed resources/spell_en.txt
var spellData string

var (
	spellOnce sync.Once
	spellSet  map[string]struct{}
)

func spellWords() map[string]struct{} {
	spellOnce.Do(func() {
		lines := strings.Split(spellData, "\n")
		spellSet = make(map[string]struct{}, len(lines))
		for _, w := range lines {
			w = strings.TrimSpace(w)
			if w != "" {
				spellSet[w] = struct{}{}
			}
		}
	})
	return spellSet
}

// collapseDoubles folds every run of two or more identical letters down to one:
// "sttocks" becomes "stocks", "inveestmentts" becomes "investments".
func collapseDoubles(w string) string {
	var b strings.Builder
	b.Grow(len(w))
	var prev rune
	for i, r := range w {
		if i > 0 && r == prev {
			continue
		}
		b.WriteRune(r)
		prev = r
	}
	return b.String()
}

// fixDoubledLetter repairs one word, returning it unchanged when the evidence
// does not support a rewrite.
func fixDoubledLetter(word string) string {
	// Only words made of letters can be looked up; "5%" and "723-8698" are left
	// alone, and so is anything with an apostrophe, which the list does not
	// carry.
	lower := strings.ToLower(word)
	for _, r := range lower {
		if r < 'a' || r > 'z' {
			return word
		}
	}
	if len(lower) < 3 {
		return word
	}
	collapsed := collapseDoubles(lower)
	if collapsed == lower {
		return word // no doubled letters to repair
	}

	words := spellWords()
	if _, ok := words[lower]; ok {
		return word // already a word: "letter", "success", "all"
	}
	if _, ok := words[collapsed]; !ok {
		return word // collapsing does not produce a word: leave it alone
	}
	return matchCase(word, collapsed)
}

// matchCase gives the repaired word the capitalisation the original had, so a
// sentence-initial word and an all-caps heading both survive the repair.
func matchCase(original, fixed string) string {
	switch {
	case original == strings.ToUpper(original):
		return strings.ToUpper(fixed)
	case original != "" && original[0] >= 'A' && original[0] <= 'Z':
		return strings.ToUpper(fixed[:1]) + fixed[1:]
	default:
		return fixed
	}
}

// fixDoubledLetters repairs a whole line of recognised text.
//
// Words are split on spaces only, so punctuation stays attached: "sttocks," is
// looked up as "stocks," — which is not a word — and therefore left alone. The
// alternative, stripping punctuation before lookup, would need the punctuation
// put back exactly, and the repair is worth less than the risk of mangling a
// number or a URL.
func fixDoubledLetters(line string) string {
	if line == "" {
		return line
	}
	fields := strings.Split(line, " ")
	changed := false
	for i, f := range fields {
		if f == "" {
			continue
		}
		// Separate trailing punctuation, repair the word, put it back. OCR
		// commonly ends a line with a comma or a period, and without this the
		// last word of every line would go unrepaired.
		core, tail := splitTrailingPunct(f)
		if core == "" {
			continue
		}
		if fixed := fixDoubledLetter(core); fixed != core {
			fields[i] = fixed + tail
			changed = true
		}
	}
	if !changed {
		return line
	}
	return strings.Join(fields, " ")
}

// splitTrailingPunct separates the word from the punctuation that follows it.
func splitTrailingPunct(s string) (core, tail string) {
	i := len(s)
	for i > 0 {
		c := s[i-1]
		if c == '.' || c == ',' || c == ';' || c == ':' || c == '!' || c == '?' ||
			c == ')' || c == ']' || c == '"' || c == '\'' || c == '%' {
			i--
			continue
		}
		break
	}
	return s[:i], s[i:]
}

// fixDoubledLettersAll applies the repair to every line of a multi-line block.
func fixDoubledLettersAll(text string) string {
	if !strings.Contains(text, "\n") {
		return fixDoubledLetters(text)
	}
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = fixDoubledLetters(l)
	}
	return strings.Join(lines, "\n")
}
