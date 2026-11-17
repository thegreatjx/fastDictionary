package main

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	selIconHeight  = 36
	chipButtonSize = 28
	chipIconSize   = 17
	chipPadV       = 3
	chipPadH       = 4
	chipRowGap     = 2
)

const (
	chipTipHeight   = 22
	chipTipGap      = 7
	chipTipFontSize = 12
	chipTipPadH     = 9
	chipTipPadV     = 5
	chipTipDelay    = 160 * time.Millisecond

	selIconSettle = 120 * time.Millisecond
	chipTipWarm   = 400 * time.Millisecond
)

const chipMoreLabel = "复制与更多功能"

// selState 由 app.mu 保护。
type selState struct {
	win   *mygo.Window
	text  string
	x, y  int
	shown bool
	gen   uint64
}

type chipTipState struct {
	mu         sync.Mutex
	win        *mygo.Window
	key, label string
	since      time.Time
	shown      bool
	hiddenAt   time.Time
	anchor     ui.Rect
	gen        uint64
	hits       []chipHit
	timer      *time.Timer
	geom       chipGeom
}

type chipGeom struct {
	displays []geomDisplay
	primary  geomDisplay
}

type chipHit struct {
	label string
	rect  ui.Rect
}

var (
	gdi32Dll         = syscall.NewLazyDLL("gdi32.dll")
	procDeleteObject = gdi32Dll.NewProc("DeleteObject")
)

func selectionChipWidth(enabledActions int) int {
	if enabledActions < 0 {
		enabledActions = 0
	}
	items := 4 + enabledActions
	gaps := items
	return 2*chipPadH + items*chipButtonSize + 1 + gaps*chipRowGap
}

func (a *app) currentSelectionChipWidth() int {
	return selectionChipWidth(len(a.store.Get().GetEnabledActions()))
}

// 窗口矩形必须等于胶囊矩形；多出的透明边会被 DWM 描边或合成路径暴露。
func chipWindowWidth(pillWidth int) int { return pillWidth }
func chipWindowHeight() int             { return selIconHeight }

var selIconOnce sync.Once

// NewWindow 会切到主线程并等锁，调用时不能持有 a.mu。
func (a *app) selIconWindow() *mygo.Window {
	a.mu.Lock()
	existing := a.sel.win
	a.mu.Unlock()
	if existing != nil {
		return existing
	}

	selIconOnce.Do(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:         appTitle + " Selection",
			Width:         chipWindowWidth(a.currentSelectionChipWidth()),
			Height:        chipWindowHeight(),
			Hidden:        true,
			SkipTaskbar:   true,
			AlwaysOnTop:   true,
			Frameless:     true,
			Transparent:   true,
			Panel:         true,
			DisableShadow: true,
			DisableResize: true,
			DisableMove:   false,
			Content:       ui.View(safeView("chip", a.selIconView)),
		})
		a.mu.Lock()
		a.sel.win = win
		a.mu.Unlock()
	})
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sel.win
}

func (a *app) showSelIcon(px, py int32, text string) {
	win := a.selIconWindow()
	if win == nil || win.IsDestroyed() {
		return
	}
	a.cacheChipGeom()

	x, y := a.chipPosition(px, py)
	width := a.currentSelectionChipWidth()

	a.mu.Lock()
	a.sel.text = text
	a.sel.x, a.sel.y = x, y
	a.sel.shown = true
	a.sel.gen++
	gen := a.sel.gen
	a.mu.Unlock()

	win.SetSize(chipWindowWidth(width), chipWindowHeight())
	win.SetAlwaysOnTop(true)
	win.ShowInactive()

	mygo.RunOnMain(func() {
		hwnd := win.NativeHandle()
		if hwnd == 0 {
			return
		}
		ex, _, _ := procGetWindowLongPtr.Call(hwnd, gwlExStyle)
		// Panel 已设 WS_EX_TOOLWINDOW；mygo 把 Focusable 写死为 true，NOACTIVATE 只能手动补。
		want := ex | wsExNoActivate
		if want != ex {
			procSetWindowLongPtr.Call(hwnd, gwlExStyle, want)
		}
		clearChipFrame(hwnd)
		a.placeChipWindow(hwnd, x, y)
		if want != ex {
			a.placeChipWindow(hwnd, x, y)
		}
		var rr winRect
		procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rr)))
		log.Printf("fastDictionary: after placeChipWindow(%d,%d): hwnd=%#x rect=(%d,%d %dx%d)",
			x, y, hwnd, rr.Left, rr.Top, rr.Right-rr.Left, rr.Bottom-rr.Top)
	})
	win.Update(func() {})

	armChipHover()

	go a.sampleChipPointer(gen)
}

func (a *app) sampleChipPointer(gen uint64) {
	defer recoverLog("selicon.go:sampleChipPointer")
	for i := 0; i < 10; i++ {
		time.Sleep(selIconSettle)
		if a.chipHitBoxesReady() {
			break
		}
	}
	if !a.chipSampleStillCurrent(gen) {
		return
	}
	var p winPoint
	if ok, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&p))); ok == 0 {
		return
	}
	a.onChipPointer(p.X, p.Y)
}

func (a *app) chipSampleStillCurrent(gen uint64) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sel.shown && a.sel.gen == gen
}

func (a *app) chipHitBoxesReady() bool {
	a.chipTip.mu.Lock()
	ready := len(a.chipTip.hits) > 0
	a.chipTip.mu.Unlock()
	return ready
}

func (a *app) placeChipWindow(hwnd uintptr, x, y int) {
	dpi := monitorDPIAt(x, y)
	px := int32((x*dpi + 48) / 96)
	py := int32((y*dpi + 48) / 96)
	w := int32((chipWindowWidth(a.currentSelectionChipWidth())*dpi + 48) / 96)
	h := int32((chipWindowHeight()*dpi + 48) / 96)
	const (
		swpNoZOrder   = 0x0004
		swpNoActivate = 0x0010
		hwndTopmost   = ^uintptr(0)
	)
	const swpShowWindow = 0x0040
	procSetWindowPos.Call(hwnd, hwndTopmost, uintptr(px), uintptr(py), uintptr(w), uintptr(h),
		swpNoActivate|swpShowWindow)
}

// Win11 给顶层窗口画 1px DWM 边框，会压住胶囊描边；Win10 调用失败可忽略。
func clearChipFrame(hwnd uintptr) {
	const (
		dwmwaBorderColor = 34
		dwmColorNone     = 0xFFFFFFFE
	)
	none := uint32(dwmColorNone)
	procDwmSetWindowAttr.Call(hwnd, dwmwaBorderColor, uintptr(unsafe.Pointer(&none)), 4)
}

func monitorDPIAt(x, y int) int {
	pt := uintptr(uint32(int32(x))) | uintptr(uint32(int32(y)))<<32
	mon, _, _ := procMonitorFromPoint.Call(pt, 2)
	if mon != 0 {
		var dx, dy uint32
		if r, _, _ := procGetDpiForMonitor.Call(mon, 0, uintptr(unsafe.Pointer(&dx)), uintptr(unsafe.Pointer(&dy))); r == 0 && dx != 0 {
			return int(dx)
		}
	}
	if d := mygo.Screen.PrimaryDisplay().ScaleFactor; d > 0 {
		return int(d*96 + 0.5)
	}
	return 96
}

func (a *app) logChipTipState() {
	t := &a.chipTip
	t.mu.Lock()
	win := t.win
	label := t.label
	shown := t.shown
	t.mu.Unlock()
	log.Printf("fastDictionary: chip tip label=%q shown=%v", label, shown)
	if win == nil || win.IsDestroyed() {
		log.Printf("fastDictionary: chip tip window is gone")
		return
	}
	hwnd := win.NativeHandle()
	var r winRect
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	var origin winPoint
	procClientToScreen.Call(hwnd, uintptr(unsafe.Pointer(&origin)))
	ex, _, _ := procGetWindowLongPtr.Call(hwnd, gwlExStyle)
	vis, _, _ := procIsWindowVisible.Call(hwnd)
	log.Printf("fastDictionary: chip tip hwnd=%#x rect=(%d,%d %dx%d) clientOrigin=(%d,%d) visible=%v topmost=%v noactivate=%v tool=%v",
		hwnd, r.Left, r.Top, r.Right-r.Left, r.Bottom-r.Top, origin.X, origin.Y, vis != 0,
		ex&wsExTopmost != 0, ex&wsExNoActivate != 0, ex&wsExToolWindow != 0)

	a.mu.Lock()
	chip := a.sel.win
	a.mu.Unlock()
	if chip != nil && !chip.IsDestroyed() {
		ch := chip.NativeHandle()
		var cr winRect
		procGetWindowRect.Call(ch, uintptr(unsafe.Pointer(&cr)))
		var co winPoint
		procClientToScreen.Call(ch, uintptr(unsafe.Pointer(&co)))
		log.Printf("fastDictionary: chip hwnd=%#x rect=(%d,%d %dx%d) clientOrigin=(%d,%d) same=%v",
			ch, cr.Left, cr.Top, cr.Right-cr.Left, cr.Bottom-cr.Top, co.X, co.Y, ch == hwnd)
	}
}

func (a *app) logChipState() {
	a.mu.Lock()
	win := a.sel.win
	a.mu.Unlock()
	if win == nil || win.IsDestroyed() {
		log.Printf("fastDictionary: chip window is gone")
		return
	}
	hwnd := win.NativeHandle()
	var r winRect
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	ex, _, _ := procGetWindowLongPtr.Call(hwnd, gwlExStyle)
	st, _, _ := procGetWindowLongPtr.Call(hwnd, gwlStyle)
	vis, _, _ := procIsWindowVisible.Call(hwnd)
	iconic, _, _ := procIsIconic.Call(hwnd)
	owner, _, _ := procGetWindow.Call(hwnd, 4)
	log.Printf("fastDictionary: chip hwnd=%#x rect=(%d,%d %dx%d) visible=%v iconic=%v",
		hwnd, r.Left, r.Top, r.Right-r.Left, r.Bottom-r.Top, vis != 0, iconic != 0)
	log.Printf("fastDictionary:   exstyle=%#x toolwindow=%v layered=%v topmost=%v noactivate=%v appwindow=%v",
		uint32(ex), ex&wsExToolWindow != 0, ex&wsExLayered != 0, ex&wsExTopmost != 0,
		ex&wsExNoActivate != 0, ex&wsExAppWindow != 0)
	log.Printf("fastDictionary:   style=%#x child=%v popup=%v visibleBit=%v owner=%#x",
		uint32(st), st&0x40000000 != 0, st&0x80000000 != 0, st&0x10000000 != 0, owner)
	b := win.Bounds()
	log.Printf("fastDictionary: chip reports bounds %+v", b)

	log.Printf("fastDictionary: windows of pid %d:", currentProcessID())
	for _, w := range listProcessWindows(currentProcessID()) {
		log.Printf("fastDictionary:   %s", w)
	}
}

func listProcessWindows(pid uint32) []string {
	var out []string
	cb := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		var wpid uint32
		procGetWindowThread.Call(hwnd, uintptr(unsafe.Pointer(&wpid)))
		if wpid != pid {
			return 1
		}
		var r winRect
		procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
		ex, _, _ := procGetWindowLongPtr.Call(hwnd, gwlExStyle)
		vis, _, _ := procIsWindowVisible.Call(hwnd)
		out = append(out, fmt.Sprintf("hwnd=%#x rect=(%d,%d %dx%d) vis=%v tool=%v topmost=%v title=%q",
			hwnd, r.Left, r.Top, r.Right-r.Left, r.Bottom-r.Top,
			vis != 0, ex&wsExToolWindow != 0, ex&wsExTopmost != 0, windowTitleOf(hwnd)))
		return 1
	})
	procEnumWindows.Call(cb, 0)
	return out
}

func (a *app) hideSelIcon() {
	a.mu.Lock()
	win := a.sel.win
	a.sel.shown = false
	a.sel.gen++
	more := a.moreFlyout
	a.moreFlyout = nil
	a.moreFlyoutShown = false
	a.moreFlyoutRect = ui.Rect{}
	a.mu.Unlock()
	disarmChipHover()
	a.hideChipTip()
	if more != nil && !more.IsDestroyed() {
		more.Close()
	}
	if win != nil && !win.IsDestroyed() {
		win.Hide()
	}
}

func (a *app) recordChipHit(btn ui.Element, label string) {
	t := &a.chipTip
	t.mu.Lock()
	replaced := false
	for i := range t.hits {
		if t.hits[i].label == label {
			t.hits[i].rect = btn.Bounds()
			replaced = true
			break
		}
	}
	if !replaced {
		t.hits = append(t.hits, chipHit{label: label, rect: btn.Bounds()})
	}
	t.mu.Unlock()
}

func (a *app) onChipPointer(px, py int32) {
	a.mu.Lock()
	shown, originX, originY := a.sel.shown, a.sel.x, a.sel.y
	a.mu.Unlock()
	if !shown {
		return
	}
	dx, dy := a.chipTip.chipToDIP(px, py)
	t := &a.chipTip
	t.mu.Lock()
	label, inside := t.hitAt(dx, dy, originX, originY)
	if !inside {
		was := t.shown
		t.key = ""
		t.shown = false
		if was {
			t.hiddenAt = time.Now()
		}
		win := t.win
		t.mu.Unlock()
		if was && win != nil && !win.IsDestroyed() {
			win.Hide()
		}
		return
	}
	if t.key == label && t.shown {
		t.mu.Unlock()
		return
	}
	now := time.Now()
	if t.key != label {
		warm := chipTipIsWarm(t.hiddenAt, now)
		t.key, t.label = label, label
		if !warm {
			t.since = now
		}
		t.shown = false
	}
	due := chipTipIsDue(t.shown, t.since, now)
	if !due {
		remaining := chipTipDelay - now.Sub(t.since)
		if t.timer != nil {
			t.timer.Stop()
		}
		t.timer = time.AfterFunc(remaining, func() { a.onChipPointer(px, py) })
		t.mu.Unlock()
		return
	}
	if t.timer != nil {
		t.timer.Stop()
		t.timer = nil
	}
	t.shown = true
	anchor := t.anchorFor(label)
	t.mu.Unlock()
	a.raiseChipTip(label, anchor)
}

func (t *chipTipState) hitAt(x, y int, originX, originY int) (string, bool) {
	x, y = x-originX, y-originY
	for _, h := range t.hits {
		r := h.rect
		if float32(x) >= r.X && float32(x) <= r.X+r.W && float32(y) >= r.Y && float32(y) <= r.Y+r.H {
			return h.label, true
		}
	}
	const gapBridge = float32(chipRowGap) / 2
	for _, h := range t.hits {
		r := h.rect
		if float32(x) >= r.X-gapBridge && float32(x) <= r.X+r.W+gapBridge &&
			float32(y) >= r.Y && float32(y) <= r.Y+r.H {
			return h.label, true
		}
	}
	return "", false
}

func (t *chipTipState) anchorFor(label string) ui.Rect {
	for _, h := range t.hits {
		if h.label == label {
			return h.rect
		}
	}
	return ui.Rect{}
}

func (a *app) raiseChipTip(label string, anchor ui.Rect) {
	a.mu.Lock()
	chipShown := a.sel.shown
	chipWin := a.sel.win
	a.mu.Unlock()
	if !chipShown || chipWin == nil || chipWin.IsDestroyed() {
		return
	}

	t := &a.chipTip
	t.mu.Lock()
	t.label = label
	t.shown = true
	gen := t.gen
	win := t.win
	t.mu.Unlock()

	width := chipTipWidth(label)
	anchorRect := mygo.Rectangle{
		X: int(anchor.X+anchor.W/2) - width/2, Y: int(anchor.Y), Width: width, Height: 1,
	}

	if win == nil || win.IsDestroyed() {
		win = mygo.NewFlyout(mygo.FlyoutOptions{
			Parent:    chipWin,
			Anchor:    anchorRect,
			Placement: mygo.PlacementTop,
			Gap:       chipTipGap,
			Width:     width,
			Height:    chipTipHeight,
			Focusable: false,
			Shadow:    false,
			Content:   ui.View(safeView("chipTip", a.chipTipView)),
		})
		t.mu.Lock()
		t.win = win
		t.mu.Unlock()
	} else {
		win.SetAnchor(anchorRect)
		win.SetSize(width, chipTipHeight)
		win.SetAnchor(anchorRect)
	}
	t.mu.Lock()
	stale := t.gen != gen
	t.mu.Unlock()
	if stale {
		return
	}
	win.ShowInactive()
	mygo.RunOnMain(func() {
		t.mu.Lock()
		stale := t.gen != gen || !t.shown
		t.mu.Unlock()
		if stale {
			win.Hide()
			return
		}
		win.SetAnchor(anchorRect)
		win.SetSize(width, chipTipHeight)
		win.SetAnchor(anchorRect)
	})
	win.Update(func() {})
}

func (a *app) hideChipTip() {
	t := &a.chipTip
	t.mu.Lock()
	t.shown = false
	t.key = ""
	t.gen++
	t.hiddenAt = time.Now()
	if t.timer != nil {
		t.timer.Stop()
		t.timer = nil
	}
	win := t.win
	t.mu.Unlock()
	if win != nil && !win.IsDestroyed() {
		win.Hide()
	}
}

func chipTipWidth(label string) int {
	w := chipTipPadH * 2
	for _, r := range label {
		if r > 0x2E80 {
			w += chipTipFontSize
			continue
		}
		w += chipTipFontSize * 55 / 100
	}
	return max(w, 48)
}

func (a *app) chipTipView(c *ui.Context) {
	cfg := a.store.Get()
	applyTheme(c, cfg.ThemeMode, c.Theme().Dark, cfg.AccentTheme)
	t := c.Theme()
	c.Root().Background(ui.Transparent)

	tip := &a.chipTip
	tip.mu.Lock()
	label := tip.label
	tip.mu.Unlock()
	if label == "" {
		return
	}
	bg, fg := chipTipColors(t)
	ui.Row(c).Fill().AlignItems(ui.Center).Justify(ui.Center).
		Padding(chipTipPadV, chipTipPadH).Radius(6).
		Background(bg).
		Children(func() {
			ui.Text(c, label).FontSize(chipTipFontSize).FontWeight(500).
				TextColor(fg).SingleLine().MaxLines(1)
		})
}

func (a *app) selIconContains(px, py int32) bool {
	a.mu.Lock()
	shown, x, y := a.sel.shown, a.sel.x, a.sel.y
	a.mu.Unlock()
	if !shown {
		return false
	}
	dx, dy := a.chipTip.chipToDIP(px, py)
	const slack = 4
	return dx >= x-slack && dx <= x+a.currentSelectionChipWidth()+slack &&
		dy >= y-slack && dy <= y+selIconHeight+slack
}

type geomDisplay struct {
	Bounds      mygo.Rectangle
	WorkArea    mygo.Rectangle
	ScaleFactor float64
	Primary     bool
}

const chipGap = 8

func placeChip(displays []geomDisplay, primary geomDisplay, px, py int32) (int, int) {
	return placeChipWithWidth(displays, primary, px, py, selectionChipWidth(len(DefaultCustomActions())))
}

func placeChipWithWidth(displays []geomDisplay, primary geomDisplay, px, py int32, width int) (int, int) {
	x, y := physicalToDIPIn(displays, primary, px, py)
	wa := workAreaAt(displays, primary, x, y)
	return clampChip(x, y, wa, width)
}

func clampChip(x, y int, wa mygo.Rectangle, width int) (int, int) {
	if x+width+chipGap > wa.X+wa.Width {
		x = wa.X + wa.Width - width - chipGap
	}
	if x < wa.X+chipGap {
		x = wa.X + chipGap
	}

	if y+selIconHeight+chipGap > wa.Y+wa.Height {
		y = y - selIconHeight - chipGap*2
	} else {
		y += chipGap
	}
	if y+selIconHeight+chipGap > wa.Y+wa.Height {
		y = wa.Y + wa.Height - selIconHeight - chipGap
	}
	if y < wa.Y+chipGap {
		y = wa.Y + chipGap
	}
	return x, y
}

func physicalToDIPIn(displays []geomDisplay, primary geomDisplay, px, py int32) (int, int) {
	for _, d := range displays {
		sf := scaleOf(d)
		left := float64(d.Bounds.X) * sf
		top := float64(d.Bounds.Y) * sf
		if float64(px) >= left && float64(px) < left+float64(d.Bounds.Width)*sf &&
			float64(py) >= top && float64(py) < top+float64(d.Bounds.Height)*sf {
			return int(float64(px) / sf), int(float64(py) / sf)
		}
	}
	sf := scaleOf(primary)
	return int(float64(px) / sf), int(float64(py) / sf)
}

func workAreaAt(displays []geomDisplay, primary geomDisplay, x, y int) mygo.Rectangle {
	for _, d := range displays {
		b := d.Bounds
		if x >= b.X && x < b.X+b.Width && y >= b.Y && y < b.Y+b.Height {
			return d.WorkArea
		}
	}
	return primary.WorkArea
}

func scaleOf(d geomDisplay) float64 {
	if d.ScaleFactor <= 0 {
		return 1
	}
	return d.ScaleFactor
}

func (a *app) chipPosition(px, py int32) (int, int) {
	geom := a.chipTip.geomSnapshot()
	return placeChipWithWidth(geom.displays, geom.primary, px, py, a.currentSelectionChipWidth())
}

func (a *app) cacheChipGeom() {
	displays := displayGeom()
	t := &a.chipTip
	t.mu.Lock()
	t.geom = chipGeom{displays: displays, primary: primaryGeom(displays)}
	t.mu.Unlock()
}

func (t *chipTipState) geomSnapshot() chipGeom {
	t.mu.Lock()
	geom := t.geom
	t.mu.Unlock()
	return geom
}

func (t *chipTipState) chipToDIP(px, py int32) (int, int) {
	geom := t.geomSnapshot()
	return physicalToDIPIn(geom.displays, geom.primary, px, py)
}

func displayGeom() []geomDisplay {
	ds := mygo.Screen.Displays()
	out := make([]geomDisplay, 0, len(ds))
	for _, d := range ds {
		out = append(out, geomDisplay{Bounds: d.Bounds, WorkArea: d.WorkArea, ScaleFactor: d.ScaleFactor, Primary: d.Primary})
	}
	return out
}

func primaryGeom(displays []geomDisplay) geomDisplay {
	for _, d := range displays {
		if d.Primary {
			return d
		}
	}
	if len(displays) > 0 {
		return displays[0]
	}
	return geomDisplay{ScaleFactor: 1}
}

func (a *app) applySelectionMode() {
	a.gestureGate.reset()
	wantHooks := a.store.Get().SelectionTrigger != "hotkey"
	if wantHooks {
		if !waitHookThreadReady(800 * time.Millisecond) {
			log.Printf("fastDictionary: hook thread not ready in time; selection hooks not installed")
			return
		}
	} else {
		hookMu.Lock()
		tid := hookLoopID
		hookMu.Unlock()
		if tid == 0 {
			return
		}
	}
	apply := func() {
		if wantHooks {
			if err := installHooks(); err != nil {
				log.Printf("fastDictionary: could not install selection hooks: %v", err)
			}
			return
		}
		removeHooks()
	}
	if err := postHookCommand(apply); err != nil {
		log.Printf("fastDictionary: could not post selection hook command: %v", err)
	}
}

func (a *app) onSelectionGesture(g selectionGesture) {
	px, py, owner := g.X, g.Y, g.Owner
	cfg := a.store.Get()
	log.Printf("fastDictionary: gesture at (%d,%d) owner=%q kind=%s trigger=%q", px, py, owner, g.Kind, cfg.SelectionTrigger)

	if cfg.SelectionTrigger == "hotkey" {
		log.Printf("fastDictionary: gesture skipped: trigger is hotkey-only")
		return
	}
	if typingSuppresses(g) {
		log.Printf("fastDictionary: gesture dropped (typing: %v after last key, kind=%s)",
			g.PressedAt.Sub(g.LastKey).Round(time.Millisecond), g.Kind)
		return
	}
	if ignore, why := a.shouldIgnoreSelection(owner); ignore {
		log.Printf("fastDictionary: selection ignored (%s)", why)
		return
	}
	now := time.Now()
	if g.Kind != gestureTripleClick {
		if ok, why := a.gestureGate.canAllow(now, px, py); !ok {
			log.Printf("fastDictionary: gesture dropped (%s)", why)
			return
		}
	}

	sel := a.captureForGesture(owner)
	if why := selectionTextProblem(sel.Text); why != "" {
		log.Printf("fastDictionary: gesture produced no chip (%s): %s from %q (capture=%s)",
			why, a.logText(sel.Text, 30), owner, cfg.SelectionCapture)
		return
	}
	log.Printf("fastDictionary: selection %s (%d chars) bounds=%v",
		a.logText(sel.Text, 30), len(sel.Text), sel.Bounds)
	a.gestureGate.record(now, px, py)
	a.deliverSelection(sel, px, py)
}

func (a *app) capture() selection {
	if a.captureSelection != nil {
		return a.captureSelection()
	}
	return captureUIA()
}

func (a *app) deliverSelection(sel selection, px, py int32) {
	sel.Text = removeZeroWidthChars(sel.Text)
	if why := selectionTextProblem(sel.Text); why != "" {
		log.Printf("fastDictionary: selection unusable (%s)", why)
		return
	}
	if a.store.Get().SelectionTrigger == "auto" {
		a.showPopupText(modeSelection, sel.Text)
		return
	}

	if sel.Bounds {
		px = int32((sel.Rect.Left + sel.Rect.Right) / 2)
		py = int32(sel.Rect.Bottom)
	}
	a.showChipAt(px, py, sel.Text)
}

func (a *app) showChipAt(px, py int32, text string) {
	if a.showChip != nil {
		a.showChip(int(px), int(py), text)
		return
	}
	a.showSelIcon(px, py, text)
}

func (a *app) showPopupText(m popupMode, text string) {
	if a.showPopup != nil {
		a.showPopup(m, text)
		return
	}
	a.openPopupWithText(m, text)
}

func (a *app) onGlobalMouseDown(px, py int32) {
	defer recoverLog("selicon.go:onGlobalMouseDown")
	if a.selIconContains(px, py) {
		return
	}
	if a.moreFlyoutContains(px, py) {
		return
	}
	a.hideSelIcon()
}

func (a *app) onGlobalMouseWheel(px, py int32) {
	defer recoverLog("selicon.go:onGlobalMouseWheel")
	if a.moreFlyoutContains(px, py) {
		return
	}
	a.hideSelIcon()
}

func (a *app) moreFlyoutContains(px, py int32) bool {
	a.mu.Lock()
	shown, rect := a.moreFlyoutShown, a.moreFlyoutRect
	a.mu.Unlock()
	if !shown {
		return false
	}
	dx, dy := a.chipTip.chipToDIP(px, py)
	const slack = 2
	return float32(dx) >= rect.X-slack && float32(dx) <= rect.X+rect.W+slack &&
		float32(dy) >= rect.Y-slack && float32(dy) <= rect.Y+rect.H+slack
}

func (a *app) onGlobalKeyDown(vk uint32) {
	defer recoverLog("selicon.go:onGlobalKeyDown")
	a.hideSelIcon()
}

func (a *app) selText() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sel.text
}

func (a *app) actOnSelection(fn func(text string)) {
	text := a.selText()
	a.hideSelIcon()
	if strings.TrimSpace(text) != "" {
		fn(text)
	}
}

func (a *app) selIconView(c *ui.Context) {
	cfg := a.store.Get()
	applyTheme(c, cfg.ThemeMode, c.Theme().Dark, cfg.AccentTheme)
	t := c.Theme()
	c.Root().Background(ui.Transparent)

	a.chipTip.mu.Lock()
	a.chipTip.hits = a.chipTip.hits[:0]
	a.chipTip.mu.Unlock()

	enabledActions := cfg.GetEnabledActions()
	pillWidth := a.currentSelectionChipWidth()
	ui.Row(c).Absolute().Left(0).Top(0).
		Width(float32(pillWidth)).Height(selIconHeight).
		AlignItems(ui.Center).Justify(ui.Center).Gap(chipRowGap).
		Padding(chipPadV, chipPadH).Radius(11).
		Background(chipSurface(t)).Border(1, chipBorderColor(t)).
		Children(func() {
			a.chipIconButton(c, t, iconTranslate, chipTonePrimary, "划词翻译（"+cfg.HotkeySelection+"）", func() {
				a.actOnSelection(func(text string) { a.openPopupAction(modeTranslate, text) })
			})
			a.chipIconButton(c, t, iconSpeak, chipToneInk, "朗读选中文字", func() {
				text := a.selText()
				if strings.TrimSpace(text) != "" {
					a.speakSelection(text, false)
				}
			})
			a.chipIconButton(c, t, iconScreenshot, chipToneInk, "截图翻译（"+cfg.HotkeyScreenshot+"）", func() {
				a.hideSelIcon()
				a.TriggerScreenshotOCR()
			})
			for _, action := range enabledActions {
				act := action
				icon, mode := iconSparkles, modeTranslate
				switch act.ID {
				case "default-sentence-analysis":
					icon, mode = iconGrammar, modeGrammar
				case "default-improve-writing":
					icon, mode = iconPolish, modeWriting
				}
				a.chipIconButton(c, t, icon, chipToneInk, act.Name, func() {
					a.actOnSelection(func(text string) {
						a.openPopupAction(mode, text)
						if mode == modeTranslate {
							a.runAction(act.ID, text)
						}
					})
				})
			}
			ui.Box(c).Width(1).Height(14).Background(chipBorderColor(t))
			a.chipIconButton(c, t, iconMore, chipToneInk, chipMoreLabel, func() { a.toggleMoreFlyout() })
		})
}

var chipHoverMotion = ui.Animation{Duration: 140 * time.Millisecond, Ease: ui.EaseOut}

func (a *app) chipIconButton(c *ui.Context, t *ui.Theme, icon *ui.SVG, tone chipTone, tip string, fn func()) {
	btn := ui.Button(c, "").Label(tip).
		Size(chipButtonSize, chipButtonSize).Padding(0, 0).Radius(7).Center().
		Cursor(ui.CursorPointer)
	restFg, restWash := chipToneColors(t, tone)
	wash := ui.Transparent
	fg := restFg
	if btn.Hovered() {
		wash = restWash
		fg = chipIconHover(t)
	}
	if btn.Pressed() {
		wash = restWash.Mix(restFg, 0.22)
	}
	btn.Background(btn.AnimateColor("bg", wash, chipHoverMotion)).
		Border(0, ui.Transparent).OnClick(fn)
	a.recordChipHit(btn, tip)
	btn.Children(func() {
		iconEl := ui.Icon(c, icon).Size(chipIconSize, chipIconSize)
		if btn.Pressed() {
			iconEl.Scale(0.88, 0.88)
		}
		iconEl.TextColor(iconEl.AnimateColor("fg", fg, chipHoverMotion))
	})
}

func (a *app) speakSelection(text string, us bool) {
	PlaySpeechLang(text, a.speechLang(), us)
}

func (a *app) toggleMoreFlyout() {
	a.mu.Lock()
	existing := a.moreFlyout
	if existing != nil && !existing.IsDestroyed() {
		a.moreFlyout = nil
		a.moreFlyoutShown = false
		a.moreFlyoutRect = ui.Rect{}
	}
	selWin := a.sel.win
	a.mu.Unlock()
	if existing != nil && !existing.IsDestroyed() {
		existing.Close()
		return
	}
	if selWin == nil || selWin.IsDestroyed() {
		return
	}
	flyout := mygo.NewFlyout(mygo.FlyoutOptions{
		Parent:    selWin,
		Placement: mygo.PlacementBottomEnd,
		Width:     232,
		Height:    196,
		Focusable: true,
		Shadow:    true,
		Content:   ui.View(safeView("more", a.moreFlyoutView)),
	})
	flyout.OnShow(func() {
		x, y := flyout.Position()
		w, h := flyout.Size()
		a.mu.Lock()
		if a.moreFlyout == flyout {
			a.moreFlyoutRect = ui.Rect{X: float32(x), Y: float32(y), W: float32(w), H: float32(h)}
			a.moreFlyoutShown = true
		}
		a.mu.Unlock()
	})
	flyout.OnClosed(func() {
		a.mu.Lock()
		if a.moreFlyout == flyout {
			a.moreFlyout = nil
			a.moreFlyoutShown = false
			a.moreFlyoutRect = ui.Rect{}
		}
		a.mu.Unlock()
	})
	a.mu.Lock()
	a.moreFlyout = flyout
	a.moreFlyoutShown = false
	a.moreFlyoutRect = ui.Rect{}
	a.mu.Unlock()
}

func (a *app) moreFlyoutView(c *ui.Context) {
	cfg := a.store.Get()
	applyTheme(c, cfg.ThemeMode, c.Theme().Dark, cfg.AccentTheme)
	t := c.Theme()

	c.Root().Background(ui.Transparent)
	box := ui.Column(c).Fill().Background(t.Surface).Radius(10).Border(1, t.Border).Padding(10).Gap(2)
	box.Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Padding(0, 4).Children(func() {
			ui.Text(c, "更多工具").FontSize(12).FontWeight(600).TextColor(t.TextMuted)
			ui.Spacer(c).Grow(1)
		})
		ui.Text(c, TruncateEllipsis(a.selText(), 22)).FontSize(11).TextColor(t.TextMuted).
			Wrap().Padding(0, 4).MaxLines(2)
		ui.Divider(c).FillWidth()
		if a.chipMenuItem(c, t, iconCopy, "复制选中文字", func() {
			if text := a.selText(); strings.TrimSpace(text) != "" {
				a.copyToClipboard(text)
			}
			a.hideSelIcon()
		}) {
			return
		}
		ui.Divider(c).FillWidth()
		if a.chipMenuItem(c, t, iconSparkles, "管理 AI 指令", func() {
			a.hideSelIcon()
			a.showMainWindow("actions")
		}) {
			return
		}
		if a.chipMenuItem(c, t, iconBookmark, "生词本", func() {
			a.hideSelIcon()
			a.showMainWindow("vocabulary")
		}) {
			return
		}
		if a.chipMenuItem(c, t, iconInfo, "设置", func() {
			a.hideSelIcon()
			a.showSettings()
		}) {
			return
		}
	})
}

func (a *app) chipMenuItem(c *ui.Context, t *ui.Theme, icon *ui.SVG, label string, fn func()) bool {
	item := ui.Button(c, "").FillWidth().Height(30).Padding(2, 6).Radius(6).
		Tooltip(label).Label(label).Justify(ui.Start)
	bg := ui.Color{}
	fg := t.Text
	if item.Hovered() {
		bg = t.SurfaceHover
	}
	if item.Pressed() {
		bg = t.SurfacePressed
	}
	item.Background(bg).Border(0, ui.Transparent)
	item.Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icon).Size(14, 14).TextColor(t.TextMuted)
			ui.Text(c, label).FontSize(12).TextColor(fg)
		})
	})
	if !item.Clicked() {
		return false
	}
	a.mu.Lock()
	more := a.moreFlyout
	a.moreFlyout = nil
	a.moreFlyoutShown = false
	a.moreFlyoutRect = ui.Rect{}
	a.mu.Unlock()
	if more != nil && !more.IsDestroyed() {
		more.Close()
	}
	fn()
	return true
}
