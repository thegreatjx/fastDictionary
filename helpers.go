package main

import (
	"strings"
	"time"
	"unsafe"

	"github.com/egoist/mygo"
)

func truncateFor(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

func sleepMS(n int) { time.Sleep(time.Duration(n) * time.Millisecond) }

func mygoRect(x, y, w, h int) mygo.Rectangle {
	return mygo.Rectangle{X: x, Y: y, Width: w, Height: h}
}

func nativePtr(addr uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&addr))
}

func CleanSelectionText(s string) string {
	if s == "" {
		return ""
	}
	s = removeZeroWidthChars(s)

	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	paragraphs := strings.Split(s, "\n\n")
	var cleanedParagraphs []string

	for _, p := range paragraphs {
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
	return (r >= 0x4E00 && r <= 0x9FFF) ||
		(r >= 0x3400 && r <= 0x4DBF) ||
		(r >= 0x3040 && r <= 0x309F) ||
		(r >= 0x30A0 && r <= 0x30FF) ||
		(r >= 0xAC00 && r <= 0xD7AF)
}

func isCJKPunctuation(r rune) bool {
	return (r >= 0x3000 && r <= 0x303F) ||
		(r >= 0xFF00 && r <= 0xFFEF) ||
		(r >= 0xFE30 && r <= 0xFE4F)
}

func isCJKOrPunctuation(r rune) bool {
	return isCJK(r) || isCJKPunctuation(r)
}

func CleanOcrText(s string) string {
	if s == "" {
		return ""
	}
	s = removeZeroWidthChars(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		ln = strings.Join(strings.Fields(ln), " ")
		lines[i] = removeCJKSeparation(ln)
	}
	return strings.Trim(strings.Join(lines, "\n"), "\n")
}

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

func flattenTranslation(s string) string {
	words := strings.Fields(removeZeroWidthChars(s))
	if len(words) == 0 {
		return ""
	}
	return removeCJKSeparation(strings.Join(words, " "))
}

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

const (
	layoutSmart    = "smart"
	layoutCompact  = "compact"
	layoutPreserve = "preserve"
)

func normalizeLayoutMode(mode string) string {
	switch mode {
	case layoutCompact, layoutPreserve:
		return mode
	default:
		return layoutSmart
	}
}

func cleanSourceForLayout(s, mode string, ocr bool) string {
	switch mode {
	case layoutCompact:
		return flattenTranslation(s)
	case layoutPreserve:
		if ocr {
			return CleanOcrText(s)
		}
		return preserveLineBreaks(s)
	default:
		if ocr {
			return CleanOcrText(s)
		}
		return CleanSelectionText(s)
	}
}

func preserveLineBreaks(s string) string {
	if s == "" {
		return ""
	}
	s = removeZeroWidthChars(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	return strings.Trim(strings.Join(lines, "\n"), "\n")
}

func formatTranslationForLayout(s, mode string, keepParagraphs bool) string {
	switch mode {
	case layoutCompact:
		return flattenTranslation(s)
	case layoutPreserve:
		return preserveLineBreaks(s)
	default:
		if keepParagraphs {
			return reflowTranslation(s)
		}
		return flattenTranslation(s)
	}
}

func TruncateEllipsis(s string, maxChars int) string {
	r := []rune(s)
	if len(r) <= maxChars {
		return s
	}
	return string(r[:maxChars]) + "…"
}
