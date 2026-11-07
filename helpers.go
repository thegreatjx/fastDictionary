package main

import (
	"strings"
	"time"
	"unsafe"

	"github.com/egoist/mygo"
)

// truncateFor shortens s to n runes for logs and status lines.
func truncateFor(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// sleepMS pauses for n milliseconds.
func sleepMS(n int) { time.Sleep(time.Duration(n) * time.Millisecond) }

// mygoRect builds a mygo.Rectangle, for the geometry tables in tests and in
// the chip's placement.
func mygoRect(x, y, w, h int) mygo.Rectangle {
	return mygo.Rectangle{X: x, Y: y, Width: w, Height: h}
}

// nativePtr turns an address of memory Go does not manage — a COM pointer,
// a hook's parameter block — into a pointer. Going through a local variable
// keeps the conversion inside the one pattern go vet accepts, the same way
// mygo's own Windows backend does it.
func nativePtr(addr uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&addr))
}

// CleanSelectionText normalizes whitespace, removes zero-width characters,
// and tidies up line breaks inspired by Read-Frog's cleanText.
func CleanSelectionText(s string) string {
	if s == "" {
		return ""
	}
	// 1. Remove zero-width characters
	s = removeZeroWidthChars(s)

	// 2. Normalize Windows newlines
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	// 3. Split by paragraph boundaries (2 or more newlines)
	paragraphs := strings.Split(s, "\n\n")
	var cleanedParagraphs []string

	for _, p := range paragraphs {
		// Inside each paragraph, collapse runs of whitespace to single space
		var runes []rune
		inSpace := false
		for _, r := range p {
			if r == ' ' || r == '\t' || r == '\n' || r == '\f' || r == '\v' {
				if !inSpace {
					runes = append(runes, ' ')
					inSpace = true
				}
			} else {
				runes = append(runes, r)
				inSpace = false
			}
		}

		// Remove space if it sits between two CJK characters
		var b strings.Builder
		for i := 0; i < len(runes); i++ {
			if runes[i] == ' ' && i > 0 && i < len(runes)-1 {
				if isCJKOrPunctuation(runes[i-1]) && isCJKOrPunctuation(runes[i+1]) {
					continue
				}
			}
			b.WriteRune(runes[i])
		}

		trimmed := strings.TrimSpace(b.String())
		if trimmed != "" {
			cleanedParagraphs = append(cleanedParagraphs, trimmed)
		}
	}

	return strings.Join(cleanedParagraphs, "\n\n")
}

func isCJK(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
		(r >= 0x3400 && r <= 0x4DBF) || // CJK Extension A
		(r >= 0x3040 && r <= 0x309F) || // Hiragana
		(r >= 0x30A0 && r <= 0x30FF) || // Katakana
		(r >= 0xAC00 && r <= 0xD7AF) // Hangul
}

// isCJKPunctuation reports full-width punctuation, which OCR separates from
// the glyph beside it with the same spurious space it puts between glyphs.
// It is deliberately not part of isCJK: selection policy uses isCJK to answer
// "is this text at all", and punctuation is not.
func isCJKPunctuation(r rune) bool {
	return (r >= 0x3000 && r <= 0x303F) || // CJK symbols and punctuation
		(r >= 0xFF00 && r <= 0xFFEF) || // Halfwidth and fullwidth forms
		(r >= 0xFE30 && r <= 0xFE4F) // CJK compatibility forms
}

// isCJKOrPunctuation is the neighbour test for removing OCR's spurious glyph
// spaces: a space between “好” and “，” is the same artifact as one between
// “好” and “世”, so punctuation counts as a neighbour here but nowhere else.
func isCJKOrPunctuation(r rune) bool {
	return isCJK(r) || isCJKPunctuation(r)
}

// CleanOcrText tidies OCR output the way CleanSelectionText tidies a
// selection, except it keeps line structure: OCR reports the page's own
// line breaks, and folding them into spaces (as the selection path does)
// turns a paragraph of an image into one unwieldy line.
func CleanOcrText(s string) string {
	if s == "" {
		return ""
	}
	s = removeZeroWidthChars(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		// Collapse runs of spaces/tabs inside a line, but never across lines.
		ln = strings.Join(strings.Fields(ln), " ")
		// Windows.Media.Ocr reports each CJK glyph as its own word, so its
		// text comes back as "微 软 翻 译". Left alone, that reaches the
		// translator as six tokens instead of two words, and the result
		// reads as broken Chinese. Only spaces between two CJK glyphs go:
		// a space beside Latin text is a real word break.
		lines[i] = removeCJKSeparation(ln)
	}
	// Drop empty lines at the edges; keep the interior ones — they separate
	// paragraphs of the image.
	return strings.Trim(strings.Join(lines, "\n"), "\n")
}

// removeCJKSeparation deletes the single spaces OCR puts between CJK glyphs,
// which it reports as one word each.
func removeCJKSeparation(s string) string {
	runes := []rune(s)
	out := make([]rune, 0, len(runes))
	for i, r := range runes {
		if r == ' ' && i > 0 && i+1 < len(runes) &&
			isCJKOrPunctuation(runes[i-1]) && isCJKOrPunctuation(runes[i+1]) {
			continue
		}
		out = append(out, r)
	}
	return string(out)
}

func removeZeroWidthChars(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\u200B', '\u200C', '\u200D', '\uFEFF':
			continue
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// flattenTranslation folds the line breaks a translation engine copies from
// the source into one continuous paragraph. The popup wraps text to its own
// width, so keeping those empty lines only leaves blank bands between
// sentences and makes the card far taller than the text needs.
func flattenTranslation(s string) string {
	words := strings.Fields(removeZeroWidthChars(s))
	if len(words) == 0 {
		return ""
	}
	return removeCJKSeparation(strings.Join(words, " "))
}

// reflowTranslation is flattenTranslation for OCR results: blank lines still
// separate paragraphs, but ordinary line breaks inside a paragraph become
// spaces so the card can wrap to the popup width. Indented blocks (code,
// lists) keep their own line breaks because folding them would destroy their
// meaning.
func reflowTranslation(s string) string {
	if s == "" {
		return ""
	}
	s = removeZeroWidthChars(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	var blocks []string
	var paragraph []string
	flush := func() {
		if len(paragraph) == 0 {
			return
		}
		preformatted := false
		for _, line := range paragraph {
			if strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "    ") {
				preformatted = true
				break
			}
		}
		if preformatted {
			blocks = append(blocks, strings.Join(paragraph, "\n"))
		} else {
			joined := strings.Join(paragraph, " ")
			blocks = append(blocks, removeCJKSeparation(strings.Join(strings.Fields(joined), " ")))
		}
		paragraph = nil
	}

	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimRight(line, " \t")
		if strings.TrimSpace(line) == "" {
			flush()
			if len(blocks) > 0 && blocks[len(blocks)-1] != "" {
				blocks = append(blocks, "")
			}
			continue
		}
		paragraph = append(paragraph, line)
	}
	flush()
	return strings.Trim(strings.Join(blocks, "\n"), "\n")
}

// TruncateEllipsis limits text to maxChars runes, appending "…" if truncated.
func TruncateEllipsis(s string, maxChars int) string {
	r := []rune(s)
	if len(r) <= maxChars {
		return s
	}
	return string(r[:maxChars]) + "…"
}
