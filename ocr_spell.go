package main

import (
	_ "embed"
	"sort"
	"strings"
	"sync"
)

//go:embed resources/spell_en.txt
var spellData string

var (
	spellOnce sync.Once
	spellList []string
)

func spellWordList() []string {
	spellOnce.Do(func() {
		lines := strings.Split(spellData, "\n")
		out := make([]string, 0, len(lines))
		for _, w := range lines {
			w = strings.TrimSpace(w)
			if w != "" {
				out = append(out, w)
			}
		}
		if !sort.StringsAreSorted(out) {
			sort.Strings(out)
		}
		spellList = out
	})
	return spellList
}

func hasSpellWord(word string) bool {
	if word == "" {
		return false
	}
	list := spellWordList()
	i := sort.SearchStrings(list, word)
	return i < len(list) && list[i] == word
}

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

func fixDoubledLetter(word string) string {
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
		return word
	}

	if hasSpellWord(lower) {
		return word
	}
	if !hasSpellWord(collapsed) {
		return word
	}
	return matchCase(word, collapsed)
}

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
