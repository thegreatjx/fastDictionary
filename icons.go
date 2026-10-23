package main

import (
	"github.com/egoist/mygo/ui"
)

// Clean vector icons extracted from modern icon sets (Tabler / Lucide / RemixIcon)
// matching read-frog's selection toolbar visual language.

const (
	// Icon: Language / Translation (中/A)
	svgTranslateData = `<svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
		<path d="m5 8 6 6"/>
		<path d="m4 14 6-6 2-3"/>
		<path d="M2 5h12"/>
		<path d="M7 2h1"/>
		<path d="m22 22-5-10-5 10"/>
		<path d="M14 18h6"/>
	</svg>`

	// Icon: Audio Speaker / Pronunciation (🔊)
	svgSpeakData = `<svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
		<polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5"/>
		<path d="M15.54 8.46a5 5 0 0 1 0 7.07"/>
		<path d="M19.07 4.93a10 10 0 0 1 0 14.14"/>
	</svg>`

	// Icon: Dictionary / Book (A-Z)
	svgDictData = `<svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
		<path d="M4 19.5v-15A2.5 2.5 0 0 1 6.5 2H20v20H6.5a2.5 2.5 0 0 1-2.5-2.5Z"/>
		<path d="M6 6h10"/>
		<path d="M6 10h10"/>
		<path d="M6 14h6"/>
	</svg>`

	// Icon: Search / Zoom / OCR / Focus (🔍)
	svgSearchData = `<svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
		<circle cx="11" cy="11" r="8"/>
		<path d="m21 21-4.3-4.3"/>
	</svg>`

	// Icon: Sparkles / AI (✨)
	svgSparklesData = `<svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
		<path d="m12 3-1.9 5.8a2 2 0 0 1-1.3 1.3L3 12l5.8 1.9a2 2 0 0 1 1.3 1.3L12 21l1.9-5.8a2 2 0 0 1 1.3-1.3L21 12l-5.8-1.9a2 2 0 0 1-1.3-1.3Z"/>
	</svg>`

	// Icon: Copy / Clipboard (📋)
	svgCopyData = `<svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
		<rect width="14" height="14" x="8" y="8" rx="2" ry="2"/>
		<path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/>
	</svg>`

	// Icon: Close / Cross (✕)
	svgCloseData = `<svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
		<path d="M18 6 6 18"/>
		<path d="m6 6 12 12"/>
	</svg>`

	// Icon: More / Three Dots Vertical (⋮)
	svgMoreData = `<svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
		<circle cx="12" cy="12" r="1"/>
		<circle cx="12" cy="5" r="1"/>
		<circle cx="12" cy="19" r="1"/>
	</svg>`

	// Icon: Swap languages (⇄)
	svgSwapData = `<svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
		<path d="m16 3 4 4-4 4"/>
		<path d="M20 7H4"/>
		<path d="m8 21-4-4 4-4"/>
		<path d="M4 17h16"/>
	</svg>`

	// Icon: Pin (📌)
	svgPinData = `<svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
		<line x1="12" y1="17" x2="12" y2="22"/>
		<path d="M5 17h14v-1.76a2 2 0 0 0-1.11-1.79l-1.78-.9A2 2 0 0 1 15 10.76V6h1a2 2 0 0 0 0-4H8a2 2 0 0 0 0 4h1v4.76a2 2 0 0 1-1.11 1.79l-1.78.9A2 2 0 0 0 5 15.24Z"/>
	</svg>`

	// Icon: Pinned filled
	svgPinnedData = `<svg viewBox="0 0 24 24" width="24" height="24" fill="currentColor" stroke="none">
		<path d="M12 17v5M5 17h14v-1.76a2 2 0 0 0-1.11-1.79l-1.78-.9A2 2 0 0 1 15 10.76V6h1a2 2 0 0 0 0-4H8a2 2 0 0 0 0 4h1v4.76a2 2 0 0 1-1.11 1.79l-1.78.9A2 2 0 0 0 5 15.24Z"/>
	</svg>`

	// Icon: Grammar / Sentence Analysis (Tree / Structure)
	svgGrammarData = `<svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
		<path d="M3 7V5a2 2 0 0 1 2-2h2"/>
		<path d="M17 3h2a2 2 0 0 1 2 2v2"/>
		<path d="M21 17v2a2 2 0 0 1-2 2h-2"/>
		<path d="M7 21H5a2 2 0 0 1-2-2v-2"/>
		<circle cx="11" cy="11" r="4"/>
		<path d="m14 14 3 3"/>
	</svg>`

	// Icon: Polish / Edit Pen
	svgPolishData = `<svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
		<path d="M17 3a2.85 2.83 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5Z"/>
		<path d="m15 5 4 4"/>
	</svg>`

	// Icon: Grip Dots (::: 拖拽把手)
	svgGripDotsData = `<svg viewBox="0 0 24 24" width="24" height="24" fill="currentColor">
		<circle cx="9" cy="6" r="1.5"/><circle cx="15" cy="6" r="1.5"/>
		<circle cx="9" cy="12" r="1.5"/><circle cx="15" cy="12" r="1.5"/>
		<circle cx="9" cy="18" r="1.5"/><circle cx="15" cy="18" r="1.5"/>
	</svg>`

	// Icon: Bookmark / Ribbon (🔖 猜你想存)
	svgBookmarkData = `<svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
		<path d="m19 21-7-4-7 4V5a2 2 0 0 1 2-2h10a2 2 0 0 1 2 2v16z"/>
	</svg>`

	// Icon: Checkbox Checked
	svgCheckSquareData = `<svg viewBox="0 0 24 24" width="24" height="24" fill="currentColor">
		<rect x="3" y="3" width="18" height="18" rx="4" fill="#0F172A"/>
		<path d="m9 12 2 2 4-4" stroke="#FFFFFF" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" fill="none"/>
	</svg>`

	// Icon: Microsoft Logo (4 squares)
	svgMicrosoftData = `<svg viewBox="0 0 24 24" width="24" height="24">
		<rect x="3" y="3" width="8" height="8" fill="#F25022"/>
		<rect x="13" y="3" width="8" height="8" fill="#7FBA00"/>
		<rect x="3" y="13" width="8" height="8" fill="#00A4EF"/>
		<rect x="13" y="13" width="8" height="8" fill="#FFB900"/>
	</svg>`

	// Icon: Refresh / Rerun (🔄)
	svgRefreshData = `<svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
		<path d="M21 12a9 9 0 0 0-9-9 9.75 9.75 0 0 0-6.74 2.74L3 8"/>
		<path d="M3 3v5h5"/>
		<path d="M3 12a9 9 0 0 0 9 9 9.75 9.75 0 0 0 6.74-2.74L21 16"/>
		<path d="M16 21h5v-5"/>
	</svg>`

	// Icon: Expand / Fullscreen
	svgExpandData = `<svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
		<path d="M15 3h6v6"/>
		<path d="M9 21H3v-6"/>
		<path d="M21 3l-7 7"/>
		<path d="M3 21l7-7"/>
	</svg>`

	// Icon: Screenshot OCR (⛶ 截图翻译)
	svgScreenshotData = `<svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
		<path d="M3 8V5a2 2 0 0 1 2-2h3"/>
		<path d="M16 3h3a2 2 0 0 1 2 2v3"/>
		<path d="M21 16v3a2 2 0 0 1-2 2h-3"/>
		<path d="M8 21H5a2 2 0 0 1-2-2v-3"/>
		<circle cx="12" cy="12" r="3.5"/>
	</svg>`

	// Icon: Chevron Down (⌄)
	svgChevronDownData = `<svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
		<polyline points="6 9 12 15 18 9"/>
	</svg>`

	// Icon: Chevron Up (⌃)
	svgChevronUpData = `<svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
		<polyline points="18 15 12 9 6 15"/>
	</svg>`

	// Icon: Info circle (i) - used by the popup's explanation banner.
	svgInfoData = `<svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
		<circle cx="12" cy="12" r="9"/>
		<path d="M12 11v5"/>
		<path d="M12 8h.01"/>
	</svg>`
)

var (
	iconTranslate   = ui.MustParseSVG([]byte(svgTranslateData))
	iconSpeak       = ui.MustParseSVG([]byte(svgSpeakData))
	iconDict        = ui.MustParseSVG([]byte(svgDictData))
	iconSearch      = ui.MustParseSVG([]byte(svgSearchData))
	iconSparkles    = ui.MustParseSVG([]byte(svgSparklesData))
	iconCopy        = ui.MustParseSVG([]byte(svgCopyData))
	iconClose       = ui.MustParseSVG([]byte(svgCloseData))
	iconMore        = ui.MustParseSVG([]byte(svgMoreData))
	iconSwap        = ui.MustParseSVG([]byte(svgSwapData))
	iconPin         = ui.MustParseSVG([]byte(svgPinData))
	iconPinned      = ui.MustParseSVG([]byte(svgPinnedData))
	iconGrammar     = ui.MustParseSVG([]byte(svgGrammarData))
	iconPolish      = ui.MustParseSVG([]byte(svgPolishData))
	iconGripDots    = ui.MustParseSVG([]byte(svgGripDotsData))
	iconBookmark    = ui.MustParseSVG([]byte(svgBookmarkData))
	iconCheckSquare = ui.MustParseSVG([]byte(svgCheckSquareData))
	iconMicrosoft   = ui.MustParseSVG([]byte(svgMicrosoftData))
	iconRefresh     = ui.MustParseSVG([]byte(svgRefreshData))
	iconExpand      = ui.MustParseSVG([]byte(svgExpandData))
	iconScreenshot  = ui.MustParseSVG([]byte(svgScreenshotData))
	iconChevronDown = ui.MustParseSVG([]byte(svgChevronDownData))
	iconChevronUp   = ui.MustParseSVG([]byte(svgChevronUpData))
	iconInfo        = ui.MustParseSVG([]byte(svgInfoData))
)
