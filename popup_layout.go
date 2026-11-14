package main

import (
	"strings"

	"github.com/egoist/mygo"
)

// calcPopupPosition calculates the ideal position for the popup window,
// proactively lifting it upwards (翻转置于选区上方或向上抬高) when the cursor
// is in the lower half of the screen or near the bottom, matching Read-Frog's behaviour.
func (a *app) calcPopupPosition(pt mygo.Point, targetW, targetH int) (int, int) {
	d := mygo.Screen.DisplayNearestPoint(pt)
	return calcPopupPositionIn(pt, targetW, targetH, d.WorkArea)
}

// calcPopupPositionIn calculates the ideal position for the popup window inside a given work area.
// It is a pure function of its inputs so its behaviour is verifiable without a screen.
func calcPopupPositionIn(pt mygo.Point, targetW, targetH int, wa mygo.Rectangle) (int, int) {
	waX := wa.X
	waY := wa.Y
	waW := wa.Width
	waH := wa.Height
	waRight := waX + waW
	waBottom := waY + waH

	const margin = 14

	// Horizontal: offset slightly to the left of the cursor, clamped to work area
	x := pt.X - 20
	if x+targetW > waRight-margin {
		x = waRight - targetW - margin
	}
	if x < waX+margin {
		x = waX + margin
	}

	// Vertical (主动抬高):
	// Check if cursor is in the lower half of the screen or if downward space is tight
	spaceBelow := waBottom - (pt.Y + 14)
	isLowerHalf := pt.Y > waY+int(float32(waH)*0.50)

	var y int
	if isLowerHalf || spaceBelow < targetH+margin {
		// Flip ABOVE the cursor/selection!
		y = pt.Y - targetH - 14
	} else {
		// Place BELOW the cursor/selection
		y = pt.Y + 14
	}

	// Clamp vertically within screen boundaries
	if y+targetH > waBottom-margin {
		y = waBottom - targetH - margin
	}
	if y < waY+margin {
		y = waY + margin
	}

	return x, y
}

// popupWidth keeps every popup entry point inside the range offered by the
// settings page. A second, higher minimum used to silently undo 360–430px.
func popupWidth(width float32) float32 {
	if width < 360 {
		return 360
	}
	if width > 680 {
		return 680
	}
	return width
}

// popupContentHeight estimates how tall the popup needs to be for the current
// snapshot. It is a pure function of the snapshot so its boundaries (the 260
// floor, the per-mode estimates, the wrap maths) are verifiable without a
// window or a screen.
func popupContentHeight(s snapshot, w float32) float32 {
	h := float32(160)
	switch s.mode {
	case modeDictionary:
		// Estimate from the card that is actually rendered. A fixed 460 was
		// shorter than a full card — wrapped definition, context block,
		// memory tips — so the 保存到笔记本 row ended up below the visible
		// area: the button worked, but it could not be seen or pressed.
		h = 150 // window chrome, card padding and the gaps between rows
		if !cardReadyForMode(s) {
			// Loading placeholder or the parse-failure notice.
			h += 160
			break
		}
		dr := s.dictResult
		h += 34 // headword row: term, phonetics, speaker button
		if dr.Definition != "" {
			lines := wrappedLines(dr.Definition, w-80, 14)
			if lines < 1 {
				lines = 1
			}
			h += 10 + float32(lines)*20
		}
		if dr.Context != "" {
			h += 20 // box padding
			lines := wrappedLines(dr.Context, w-110, 13)
			if lines < 1 {
				lines = 1
			}
			h += float32(lines)*20 + 6
			if dr.ContextTranslation != "" {
				n := wrappedLines(dr.ContextTranslation, w-110, 12)
				if n < 1 {
					n = 1
				}
				h += float32(n) * 18
			}
		}
		if dr.MemoryTips != "" {
			// The card splits the tips on the escaped \n it gets from the
			// model, so count those breaks too.
			tips := strings.ReplaceAll(dr.MemoryTips, `\n`, "\n")
			n := wrappedLines(tips, w-80, 12)
			if n < 1 {
				n = 1
			}
			h += 36 + float32(n)*18 // box padding and the 记忆技巧 title row
		}
		h += 48 // save button row
	case modeGrammar:
		h = 420
		if s.source != "" {
			// 只算当前这一句：卡片只渲染它，按整段估高会留下一整块空白。
			// 行高从 16 调到 18，与渲染时 26 DIP 等高行内盒的实际行距对齐。
			sent, _, _ := grammarActiveSentence(s.source, s.grammarSentIdx)
			h += float32(wrappedLines(sent, w-48, 14)) * 18
		}
		if cardReadyForMode(s) && s.grammarResult.Translation != "" {
			h += float32(wrappedLines(s.grammarResult.Translation, w-48, 14)) * 16
		}
	case modeWriting:
		// Estimate from the text that is actually rendered: a fixed 40px
		// per annotation was too small for any annotation that wraps, which
		// left the save button below the visible area.
		h = 130
		if cardReadyForMode(s) {
			wr := s.writingResult
			if wr.Setting != "" {
				h += 30
			}
			for _, ann := range wr.Annotations {
				lines := wrappedLines(ann.Text+" "+ann.Fix, w-120, 13)
				if lines < 1 {
					lines = 1
				}
				h += 18 + float32(lines)*20
				if ann.Note != "" {
					n := wrappedLines(ann.Note, w-72, 12)
					if n < 1 {
						n = 1
					}
					h += float32(n) * 18
				}
				h += 8 // card padding and gap
			}
			if wr.Improved != "" {
				h += 44 + float32(wrappedLines(wr.Improved, w-72, 14))*22
			}
			if wr.Summary != "" {
				h += 40 + float32(wrappedLines(wr.Summary, w-72, 12))*18
			}
			h += 48 // save button row
		}
	case modeInput:
		h += 60
		if s.target != "" {
			h += 120
		}
	default:
		if s.isScreenshotMode {
			// Collapsed image, editable OCR text and correction actions.
			h = 430
			if s.snipExpanded {
				h += 124
			}
		} else {
			// Translation mode:
			// Base window chrome: header (36), footer (30), dividers (14), padding (16) = 96
			h = 96
			if s.source != "" {
				srcText := s.source
				// If source is not expanded, renderTranslationSource truncates it to 110 chars.
				// Accounting for full text when truncated creates massive ghost whitespace.
				if !s.sourceExpanded && (len([]rune(srcText)) > 110 || strings.Contains(srcText, "\n")) {
					srcText = TruncateEllipsis(srcText, 110)
				}
				h += 24 + float32(wrappedLines(srcText, w-48, 13))*18
			}
			if s.target != "" {
				sz := float32(14)
				if s.isWord {
					sz = 16
				}
				// 40 covers target card padding and action bar row with gaps
				h += 40 + float32(wrappedLines(s.target, w-48, sz))*20
			}
			if s.dict != nil || s.gdict != nil {
				h += 60
			}
			if s.notesExpanded && len(s.notes) > 0 {
				h += float32(len(s.notes))*52 + 104
			}
		}
	}

	if h < popupMinHeight {
		h = popupMinHeight
	}
	return h
}

// 浮窗高度只有两个边界，入口和模式共用，避免各页面再各自加减常量。
const (
	// popupMinHeight 是收起后的下限：原文、译文与底部操作栏要同时可读。
	// 更长的内容交给正文 Scroll，不再把窗口压缩到只剩一条缝。
	popupMinHeight float32 = 260
	// popupMaxAutoHeight 是自动增高的上限。长回答在正文内滚动，
	// 不把浮窗撑成整屏面板。
	popupMaxAutoHeight float32 = 580
)

// autofitHeight corrects the estimated height with what the last frame
// actually laid out.
//
// popupContentHeight guesses; the guess is often far too tall (a long
// paragraph with three blank lines was reserving a screenful of empty space
// below the text). The Scroll always fills its parent, so
//
//	contentHeight - viewportHeight
//
// is how much the window has to grow (negative: shrink) for the viewport to
// wrap the content exactly. It needs no knowledge of the header, footer or
// padding sizes, which is what made a hand-written chrome constant drift out
// of date.
//
// A difference is returned rather than an absolute height because the window
// is a few DIPs taller than the layout root; the caller adds it to the real
// window height. The value is the previous frame's, so the caller applies it
// one frame later; the tolerance keeps that from turning into a resize every
// frame.
func autofitHeight(viewportH, contentH float32) (float32, bool) {
	if viewportH <= 0 || contentH <= 0 {
		return 0, false
	}
	delta := contentH - viewportH
	if diff := delta; diff < 4 && diff > -4 {
		return 0, false
	}
	return delta, true
}

// applyPopupHeight grows (or shrinks) the window by delta and keeps it inside
// the work area. It must run outside a render pass, so callers post it through
// Window.Update.
func (a *app) applyPopupHeight(delta float32) {
	win := a.win
	if win == nil {
		return
	}
	// A window the user sized keeps that size: the autofit corrects an
	// estimate, and there is no estimate to correct any more.
	a.mu.Lock()
	resized := a.userResized
	a.mu.Unlock()
	if resized {
		return
	}
	curW, curH := win.Size()
	// The user can drag the window, so the cached position goes stale: resizing
	// from it would snap the window back to where it was opened. Read where it
	// actually is.
	curX, curY := win.Position()
	a.popupX, a.popupY = curX, curY
	// delta is measured in layout units, but the window is a few DIPs taller
	// than the layout root (frameless panels still carry a thin border), so
	// the target is applied as a difference. Using it as an absolute height
	// left the window exactly that border shorter than the content and clipped
	// the last row.
	target := float32(curH) + delta
	if target < popupMinHeight {
		target = popupMinHeight
	}

	pt := mygo.Point{X: curX, Y: curY}
	if curX == 0 && curY == 0 {
		pt = mygo.Screen.CursorScreenPoint()
	}
	d := mygo.Screen.DisplayNearestPoint(pt)
	const margin = 14
	waTop := d.WorkArea.Y + margin
	waBottom := d.WorkArea.Y + d.WorkArea.Height - margin
	target = clampPopupHeight(target, float32(waTop), float32(waBottom))

	h := int(target)
	newY, h := liftPopupY(curY, h, waTop, waBottom)
	if h == curH && newY == curY {
		return
	}

	a.mu.Lock()
	a.popupY = newY
	a.popupH = h
	// Record the width too: scheduleAutofit compares the window against these
	// fields to tell its own resizes from the user's.
	a.popupW = curW
	a.mu.Unlock()
	// One call for both: SetSize and SetPosition each move the window on their
	// own, so pairing them makes the window appear at the new size before it
	// has moved — two repaints where the user sees one.
	win.SetBounds(mygo.Rectangle{X: curX, Y: newY, Width: curW, Height: h})
}

// clampPopupHeight caps an automatic height to 62% of the work area and to
// popupMaxAutoHeight, whichever is smaller. The two margins are already
// subtracted by the caller, so the ratio stays conservative on small screens.
func clampPopupHeight(h float32, waTop, waBottom float32) float32 {
	avail := waBottom - waTop
	if avail <= 0 {
		return h
	}
	cap := avail * 0.62
	if cap > popupMaxAutoHeight {
		cap = popupMaxAutoHeight
	}
	if h > cap {
		return cap
	}
	return h
}

// liftPopupY keeps the popup inside [waTop, waBottom]: it first pushes the
// window up so it fits above the bottom edge, then pins it to the top and
// shrinks the height when the work area itself is too small.
func liftPopupY(curY, targetH, waTop, waBottom int) (int, int) {
	if curY+targetH > waBottom {
		curY = waBottom - targetH
	}
	if curY < waTop {
		curY = waTop
		if targetH > waBottom-waTop {
			targetH = waBottom - waTop
		}
	}
	return curY, targetH
}

// fitHeight resizes the popup according to its current active mode,
// and proactively lifts the window upwards if expanding downward would exceed the screen.
// heightHeldWhileBusy 报告当前是否有可见的进行中内容，使窗口不该缩小。
//
// 流式回答还在长，高度估算会随增量抖动，先不缩；但生词推荐在面板收起后
// 不可见，它的进行中状态不该把展开时的尺寸留在窗口上 —— 那样用户折叠
// 面板会看到一整块空白。
func heightHeldWhileBusy(s snapshot) bool {
	return s.busy || s.aiBusy || (s.notesBusy && s.notesExpanded)
}

func (a *app) fitHeight() {
	win := a.win
	if win == nil {
		return
	}
	// The user sized this window by hand: refresh the content and leave the
	// bounds alone, or the next page change undoes the drag.
	a.mu.Lock()
	resized := a.userResized
	a.mu.Unlock()
	if resized {
		win.Update(func() {})
		return
	}
	cfg := a.store.Get()
	s := a.snap()

	// The config clamps PopupWidth at load; honour the user's choice down
	// to that floor instead of forcing the old 440 minimum back on them.
	w := popupWidth(cfg.PopupWidth)

	// The user can drag the window, so the cached position goes stale. Read
	// where it actually is; resizing from the cached one would snap it back.
	curX, curY := win.Position()
	a.popupX, a.popupY = curX, curY
	h := popupContentHeight(s, w)

	// Query monitor work area for adaptive lifting
	pt := mygo.Point{X: curX, Y: curY}
	if curX == 0 && curY == 0 {
		pt = mygo.Screen.CursorScreenPoint()
	}
	d := mygo.Screen.DisplayNearestPoint(pt)
	const margin = 14
	waTop := d.WorkArea.Y + margin
	waBottom := d.WorkArea.Y + d.WorkArea.Height - margin
	h = clampPopupHeight(h, float32(waTop), float32(waBottom))

	targetH := int(h)
	// fitHeight 在换页、折叠和结果落地时调用。流式回答尚未结束时，
	// 估算会随增量抖动，先不缩；一旦结果落地或用户折叠面板，就按
	// 新的内容高度回落，而不是把展开时的尺寸永久留在窗口上。
	if _, curH := win.Size(); targetH < curH {
		if heightHeldWhileBusy(s) {
			targetH = curH
		}
	}
	newY, targetH := liftPopupY(curY, targetH, waTop, waBottom)

	a.mu.Lock()
	a.popupY = newY
	a.popupH = targetH
	a.popupW = int(w)
	// fitHeight is called when the content changes, not by the autofit
	// feedback loop, so it is the one place that knows the measured content
	// is about to be different: let scheduleAutofit take a fresh decision.
	a.autofit.reset()
	a.mu.Unlock()

	win.SetBounds(mygo.Rectangle{X: curX, Y: newY, Width: int(w), Height: targetH})
	win.Update(func() {})
}

// wrappedLines estimates how many lines text needs: CJK glyphs are about a
// font size wide, latin about 0.55 of it.
func wrappedLines(text string, width, fontSize float32) int {
	if text == "" {
		return 0
	}
	usable := width - 46
	if usable < 80 {
		usable = 80
	}
	lines := 0
	for _, para := range strings.Split(text, "\n") {
		w := float32(0)
		n := 1
		for _, r := range para {
			adv := fontSize * 0.55
			if r > 0x2E80 {
				adv = fontSize
			}
			w += adv
			if w > usable {
				n++
				w = adv
			}
		}
		lines += n
	}
	return lines
}
