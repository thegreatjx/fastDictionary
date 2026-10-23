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
				if isCJK(runes[i-1]) && isCJK(runes[i+1]) {
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
		lines[i] = strings.Join(strings.Fields(ln), " ")
	}
	// Drop empty lines at the edges; keep the interior ones — they separate
	// paragraphs of the image.
	return strings.Trim(strings.Join(lines, "\n"), "\n")
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

// TruncateEllipsis limits text to maxChars runes, appending "…" if truncated.
func TruncateEllipsis(s string, maxChars int) string {
	r := []rune(s)
	if len(r) <= maxChars {
		return s
	}
	return string(r[:maxChars]) + "…"
}
