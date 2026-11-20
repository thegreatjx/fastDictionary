package main

import (
	"math"
	"strings"

	"github.com/egoist/mygo"
)

func (a *app) calcPopupPosition(pt mygo.Point, targetW, targetH int) (int, int) {
	x, y, _ := a.calcPopupPlacement(pt, targetW, targetH)
	return x, y
}

func (a *app) calcPopupPlacement(pt mygo.Point, targetW, targetH int) (int, int, bool) {
	d := mygo.Screen.DisplayNearestPoint(pt)
	return calcPopupPlacementIn(pt, targetW, targetH, d.WorkArea)
}

func calcPopupPositionIn(pt mygo.Point, targetW, targetH int, wa mygo.Rectangle) (int, int) {
	x, y, _ := calcPopupPlacementIn(pt, targetW, targetH, wa)
	return x, y
}

func calcPopupPlacementIn(pt mygo.Point, targetW, targetH int, wa mygo.Rectangle) (int, int, bool) {
	waX := wa.X
	waY := wa.Y
	waW := wa.Width
	waH := wa.Height
	waRight := waX + waW
	waBottom := waY + waH

	const margin = 14

	x := pt.X - 20
	if x+targetW > waRight-margin {
		x = waRight - targetW - margin
	}
	if x < waX+margin {
		x = waX + margin
	}

	spaceBelow := waBottom - (pt.Y + 14)
	isLowerHalf := pt.Y > waY+int(float32(waH)*0.50)

	var y int
	anchoredAbove := false
	if isLowerHalf || spaceBelow < targetH+margin {
		y = pt.Y - targetH - 14
		anchoredAbove = true
	} else {
		y = pt.Y + 14
	}

	if y+targetH > waBottom-margin {
		y = waBottom - targetH - margin
	}
	if y < waY+margin {
		y = waY + margin
	}

	return x, y, anchoredAbove
}

func popupWidth(width float32) float32 {
	if width < 360 {
		return 360
	}
	if width > 680 {
		return 680
	}
	return width
}

func popupContentHeight(s snapshot, w float32) float32 {
	h := float32(160)
	switch s.mode {
	case modeDictionary:
		h = 150
		if !cardReadyForMode(s) {
			h += 160
			break
		}
		dr := s.dictResult
		h += 34
		if dr.Definition != "" {
			lines := wrappedLines(dr.Definition, w-80, 14)
			if lines < 1 {
				lines = 1
			}
			h += 10 + float32(lines)*20
		}
		if dr.Context != "" {
			h += 20
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
			tips := strings.ReplaceAll(dr.MemoryTips, `\n`, "\n")
			n := wrappedLines(tips, w-80, 12)
			if n < 1 {
				n = 1
			}
			h += 36 + float32(n)*18
		}
		h += 48
	case modeGrammar:
		h = 300
		if s.source != "" {
			text := grammarAnalysisText(s.source)
			h += float32(wrappedLines(text, w-56, 14)) * 22
		}
		if cardReadyForMode(s) {
			gr := s.grammarResult
			if gr.Translation != "" {
				h += 22 + float32(wrappedLines(gr.Translation, w-56, 14))*20
			}
			if n := len(gr.Difficulties); n > 0 {
				if n > 3 {
					n = 3
				}
				h += 24
				for i := 0; i < n; i++ {
					h += 22
					if gr.Difficulties[i].Explanation != "" {
						h += float32(wrappedLines(gr.Difficulties[i].Explanation, w-64, 11)) * 16
					}
				}
			}
		}
	case modeWriting:
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
				h += 8
			}
			if wr.Improved != "" {
				h += 44 + float32(wrappedLines(wr.Improved, w-72, 14))*22
			}
			if wr.Summary != "" {
				h += 40 + float32(wrappedLines(wr.Summary, w-72, 12))*18
			}
			h += 48
		}
	case modeInput:
		h += 60
		if s.target != "" {
			h += 120
		}
	default:
		if s.isScreenshotMode {
			h = 430
			if s.snipExpanded {
				h += 124
			}
		} else {
			h = 96
			if s.source != "" {
				srcText := s.source
				if !s.sourceExpanded && (len([]rune(srcText)) > sourceCollapseThreshold || strings.Count(srcText, "\n") >= 4) {
					srcText = TruncateEllipsis(flattenTranslation(srcText), sourceCollapseThreshold)
				}
				srcParas := splitDisplayParagraphs(srcText)
				srcLines := wrappedLines(strings.Join(srcParas, "\n"), w-48, 13)
				h += 24 + float32(srcLines)*18 + float32(len(srcParas)-1)*Space2
			}
			if s.target != "" {
				sz := float32(15)
				if s.isWord {
					sz = 14
				}
				paras := splitDisplayParagraphs(s.target)
				lines := wrappedLines(strings.Join(paras, "\n"), w-64, sz)
				h += 40 + float32(lines)*20 + float32(len(paras)-1)*Space2
			}
			if s.dict != nil || s.gdict != nil {
				h += 72
				if s.dict != nil {
					for _, ex := range s.dict.Explains {
						lines := wrappedLines(ex, w-110, 14)
						if lines < 1 {
							lines = 1
						}
						h += float32(lines)*22 + 6
					}
				} else if s.gdict != nil {
					for _, e := range s.gdict.Entries {
						lines := wrappedLines(e.Explain, w-110, 14)
						if lines < 1 {
							lines = 1
						}
						h += float32(lines)*22 + 6
					}
				}
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

const (
	popupMinHeight     float32 = 260
	popupMaxAutoHeight float32 = 680
)

// viewportH/contentH 都是 DIP 布局值；内容高出 1 DIP 也必须撑开，否则描边被 Scroll 裁掉。
func autofitHeight(viewportH, contentH float32) (float32, bool) {
	if viewportH <= 0 || contentH <= 0 {
		return 0, false
	}
	delta := contentH - viewportH
	if delta > 0 {
		return float32(math.Ceil(float64(delta))) + 2, true
	}
	if delta > -4 {
		return 0, false
	}
	return delta, true
}

func (a *app) applyPopupHeight(delta float32) {
	win := a.win
	if win == nil {
		return
	}
	a.mu.Lock()
	resized := a.userResized
	a.mu.Unlock()
	if resized {
		return
	}
	curW, curH := win.Size()
	curX, curY := win.Position()
	actualY := curY
	a.mu.Lock()
	anchoredAbove := a.popupAnchoredAbove
	anchorBottom := a.popupAnchorBottom
	if curY != a.popupY {
		anchoredAbove = false
		a.popupAnchoredAbove = false
	}
	a.popupX, a.popupY = curX, curY
	a.mu.Unlock()
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
	y := curY
	if anchoredAbove {
		y = anchorBottom - h
	}
	newY, h := liftPopupY(y, h, waTop, waBottom)
	if h == curH && newY == actualY {
		return
	}

	a.mu.Lock()
	a.popupY = newY
	a.popupH = h
	a.popupW = curW
	a.mu.Unlock()
	win.SetBounds(mygo.Rectangle{X: curX, Y: newY, Width: curW, Height: h})
}

func clampPopupHeight(h float32, waTop, waBottom float32) float32 {
	avail := waBottom - waTop
	if avail <= 0 {
		return h
	}
	cap := avail * 0.72
	if cap > popupMaxAutoHeight {
		cap = popupMaxAutoHeight
	}
	if h > cap {
		return cap
	}
	return h
}

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

func heightHeldWhileBusy(s snapshot) bool {
	return s.busy || s.aiBusy || (s.notesBusy && s.notesExpanded)
}

func (a *app) fitHeight() {
	win := a.win
	if win == nil {
		return
	}
	a.mu.Lock()
	resized := a.userResized
	a.mu.Unlock()
	if resized {
		win.Update(func() {})
		return
	}
	cfg := a.store.Get()
	s := a.snap()

	w := popupWidth(cfg.PopupWidth)

	curX, curY := win.Position()
	a.mu.Lock()
	anchoredAbove := a.popupAnchoredAbove
	anchorBottom := a.popupAnchorBottom
	if curY != a.popupY {
		anchoredAbove = false
		a.popupAnchoredAbove = false
	}
	a.popupX, a.popupY = curX, curY
	a.mu.Unlock()
	h := popupContentHeight(s, w)

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
	if _, curH := win.Size(); targetH < curH {
		if heightHeldWhileBusy(s) {
			targetH = curH
		}
	}
	y := curY
	if anchoredAbove {
		y = anchorBottom - targetH
	}
	newY, targetH := liftPopupY(y, targetH, waTop, waBottom)

	a.mu.Lock()
	a.popupY = newY
	a.popupH = targetH
	a.popupW = int(w)
	a.autofit.reset()
	a.mu.Unlock()

	win.SetBounds(mygo.Rectangle{X: curX, Y: newY, Width: int(w), Height: targetH})
	win.Update(func() {})
}

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
