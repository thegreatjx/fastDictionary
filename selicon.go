// The selection icon: a small floating chip that appears where the user
// finished selecting text, offering to translate it.
//
// It is the answer to "selecting should not require pressing a shortcut".
// The chip never takes the keyboard focus when it appears (ShowInactive), so
// reading and typing continue undisturbed; only a click on one of its
// buttons acts. It goes away on the next click or wheel anywhere else, on
// Escape, or when a new selection replaces it.
package main

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// selIconWidth and selIconHeight are the chip's size in DIPs.
const (
	selIconWidth  = 246
	selIconHeight = 36
)

// selState is the chip's state, guarded by app.mu.
type selState struct {
	win   *mygo.Window
	text  string
	x, y  int // top-left in DIPs
	shown bool
}

var (
	gdi32Dll               = syscall.NewLazyDLL("gdi32.dll")
	procCreateRoundRectRgn = gdi32Dll.NewProc("CreateRoundRectRgn")
	procDeleteObject       = gdi32Dll.NewProc("DeleteObject")
	procSetWindowRgn       = user32.NewProc("SetWindowRgn")
)

// clipChipWindow clips the window to a clean rounded rectangle matching its actual dimensions.
// No extra cutouts or region hacks: tooltips are native overlays.
func clipChipWindow(hwnd uintptr, dpi int) {
	tbX1 := int32(0)
	tbY1 := int32(0)
	tbX2 := int32((selIconWidth*dpi + 48) / 96)
	tbY2 := int32((selIconHeight*dpi + 48) / 96)
	radius := int32((8*dpi + 48) / 96)

	tbRgn, _, _ := procCreateRoundRectRgn.Call(
		uintptr(tbX1), uintptr(tbY1), uintptr(tbX2+1), uintptr(tbY2+1),
		uintptr(radius), uintptr(radius),
	)
	if tbRgn == 0 {
		return
	}
	// SetWindowRgn takes ownership of the region on success: the system
	// deletes it with the window and the caller must not touch it again.
	// Deleting it here anyway was a double free that corrupted the GDI
	// handle table a little on every chip show.
	if ok, _, _ := procSetWindowRgn.Call(hwnd, tbRgn, 1); ok == 0 {
		procDeleteObject.Call(tbRgn)
	}
}

var selIconOnce sync.Once

// selIconWindow creates the chip's window on first use. It is hidden from
// the start and only ever shown with ShowInactive.
//
// The window must be created with a.mu NOT held. mygo.NewWindow dispatches
// the actual creation to the main thread and waits for it to finish, while
// the main thread's render pass takes a.mu: holding the lock across the
// call closed a cycle (gesture goroutine waits for the main thread, main
// thread waits for a.mu) that froze the whole app the first time a chip
// appeared.
func (a *app) selIconWindow() *mygo.Window {
	a.mu.Lock()
	existing := a.sel.win
	a.mu.Unlock()
	if existing != nil {
		return existing
	}

	selIconOnce.Do(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:         "fast词典 Selection",
			Width:         selIconWidth,
			Height:        selIconHeight,
			Hidden:        true,
			SkipTaskbar:   true,
			AlwaysOnTop:   true,
			Frameless:     true,
			Panel:         true,
			DisableShadow: false,
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

// showSelIcon places the chip next to the point the selection ended at and
// shows it without stealing the focus.
func (a *app) showSelIcon(px, py int32, text string) {
	win := a.selIconWindow()
	if win == nil || win.IsDestroyed() {
		return
	}
	x, y := chipPosition(px, py)

	a.mu.Lock()
	a.sel.text = text
	a.sel.x, a.sel.y = x, y
	a.sel.shown = true
	a.mu.Unlock()

	win.SetSize(selIconWidth, selIconHeight)
	win.SetAlwaysOnTop(true)
	win.ShowInactive() // the whole point: no focus change

	// Position and the tool-window bit are applied on the main thread after
	// the window is up: a window that was created hidden ignores a position
	// set before its first show, which is why the chip used to appear at
	// 0,0 — behind everything, with a taskbar button.
	mygo.RunOnMain(func() {
		hwnd := win.NativeHandle()
		if hwnd == 0 {
			return
		}
		// WS_EX_TOOLWINDOW keeps the chip out of the taskbar and out of
		// Alt+Tab. mygo has no option for it, so it is set here.
		ex, _, _ := procGetWindowLongPtr.Call(hwnd, gwlExStyle)
		if ex&wsExToolWindow == 0 {
			procSetWindowLongPtr.Call(hwnd, gwlExStyle, ex|wsExToolWindow|wsExNoActivate)
		}
		placeChipWindow(hwnd, x, y)
		// Re-assert after the frame change the style edit triggers.
		placeChipWindow(hwnd, x, y)
		var rr winRect
		procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rr)))
		log.Printf("fastpot: after placeChipWindow(%d,%d): hwnd=%#x rect=(%d,%d %dx%d)",
			x, y, hwnd, rr.Left, rr.Top, rr.Right-rr.Left, rr.Bottom-rr.Top)
	})
	win.Update(func() {})
}

// placeChipWindow puts the chip at a DIP position with Win32 directly. It
// runs on the main thread. The DPI of the monitor the point is on converts
// the DIP position the layout uses into the physical pixels SetWindowPos
// wants.
func placeChipWindow(hwnd uintptr, x, y int) {
	dpi := monitorDPIAt(x, y)
	px := int32((x*dpi + 48) / 96)
	py := int32((y*dpi + 48) / 96)
	w := int32((selIconWidth*dpi + 48) / 96)
	h := int32((selIconHeight*dpi + 48) / 96)
	const (
		swpNoZOrder   = 0x0004
		swpNoActivate = 0x0010
		hwndTopmost   = ^uintptr(0)
	)
	// SWP_SHOWWINDOW keeps it visible while moving.
	const swpShowWindow = 0x0040
	procSetWindowPos.Call(hwnd, hwndTopmost, uintptr(px), uintptr(py), uintptr(w), uintptr(h),
		swpNoActivate|swpShowWindow)
	clipChipWindow(hwnd, dpi)
}

// monitorDPIAt returns the DPI of the monitor holding a DIP point, falling
// back to the primary monitor's.
func monitorDPIAt(x, y int) int {
	pt := uintptr(uint32(int32(x))) | uintptr(uint32(int32(y)))<<32
	mon, _, _ := procMonitorFromPoint.Call(pt, 2 /* MONITOR_DEFAULTTONEAREST */)
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

func (a *app) logChipState() {
	a.mu.Lock()
	win := a.sel.win
	a.mu.Unlock()
	if win == nil || win.IsDestroyed() {
		log.Printf("fastpot: chip window is gone")
		return
	}
	hwnd := win.NativeHandle()
	var r winRect
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	ex, _, _ := procGetWindowLongPtr.Call(hwnd, gwlExStyle)
	st, _, _ := procGetWindowLongPtr.Call(hwnd, gwlStyle)
	vis, _, _ := procIsWindowVisible.Call(hwnd)
	iconic, _, _ := procIsIconic.Call(hwnd)
	owner, _, _ := procGetWindow.Call(hwnd, 4) // GW_OWNER
	log.Printf("fastpot: chip hwnd=%#x rect=(%d,%d %dx%d) visible=%v iconic=%v",
		hwnd, r.Left, r.Top, r.Right-r.Left, r.Bottom-r.Top, vis != 0, iconic != 0)
	log.Printf("fastpot:   exstyle=%#x toolwindow=%v layered=%v topmost=%v noactivate=%v appwindow=%v",
		uint32(ex), ex&wsExToolWindow != 0, ex&wsExLayered != 0, ex&wsExTopmost != 0,
		ex&wsExNoActivate != 0, ex&wsExAppWindow != 0)
	log.Printf("fastpot:   style=%#x child=%v popup=%v visibleBit=%v owner=%#x",
		uint32(st), st&0x40000000 != 0, st&0x80000000 != 0, st&0x10000000 != 0, owner)
	b := win.Bounds()
	log.Printf("fastpot: chip reports bounds %+v", b)

	// Can Win32 itself move it? The return value and error say whether the
	// call was refused, and MoveWindow is the other API that moves a window.
	const swpNoZOrder = 0x0004
	const swpNoActivate = 0x0010
	r0 := r
	ret, _, callErr := procSetWindowPos.Call(hwnd, 0, 900, 700, 0, 0, swpNoSize|swpNoZOrder|swpNoActivate)
	var r1 winRect
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r1)))
	log.Printf("fastpot: raw SetWindowPos ret=%d err=%v -> (%d,%d), moved=%v",
		ret, callErr, r1.Left, r1.Top, r1.Left != r0.Left || r1.Top != r0.Top)

	mret, _, merr := procMoveWindow.Call(hwnd, 950, 750, 263, 67, 1)
	var r2 winRect
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r2)))
	log.Printf("fastpot: MoveWindow ret=%d err=%v -> (%d,%d), moved=%v",
		mret, merr, r2.Left, r2.Top, r2.Left != r1.Left || r2.Top != r1.Top)

	// Every top-level window this process owns, with its real rectangle.
	log.Printf("fastpot: windows of pid %d:", currentProcessID())
	for _, w := range listProcessWindows(currentProcessID()) {
		log.Printf("fastpot:   %s", w)
	}
}

// listProcessWindows describes every top-level window this process owns.
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

// hideSelIcon takes the chip away.
func (a *app) hideSelIcon() {
	a.mu.Lock()
	win := a.sel.win
	was := a.sel.shown
	a.sel.shown = false
	a.mu.Unlock()
	if win != nil && was && !win.IsDestroyed() {
		win.Hide()
	}
}

// selIconContains reports whether a physical screen point is over the chip,
// so a click on it is not mistaken for a click elsewhere.
func (a *app) selIconContains(px, py int32) bool {
	a.mu.Lock()
	shown, x, y := a.sel.shown, a.sel.x, a.sel.y
	a.mu.Unlock()
	if !shown {
		return false
	}
	dx, dy := physicalToDIP(px, py)
	// A little slack: the click that lands on the chip's very edge should
	// still count as being on it.
	const slack = 4
	return dx >= x-slack && dx <= x+selIconWidth+slack &&
		dy >= y-slack && dy <= y+selIconHeight+slack
}

// screenPrimary returns the primary display, for callers that only need a
// scale factor and a work area.
func screenPrimary() mygo.Display { return mygo.Screen.PrimaryDisplay() }

// geomDisplay is the slice of a display's geometry the chip's placement
// needs. Keeping it separate from mygo.Display lets the placement be tested
// without a running application.
type geomDisplay struct {
	Bounds      mygo.Rectangle
	WorkArea    mygo.Rectangle
	ScaleFactor float64
}

// chipGap is the distance between the selection and the chip.
const chipGap = 8

// placeChip computes where the chip goes for a physical-pixel anchor. It
// sits below and left-aligned with the selection, flipping above when there
// is no room below, and never leaves the work area.
//
// It is a pure function of its inputs so its behaviour is verifiable
// without a screen.
func placeChip(displays []geomDisplay, primary geomDisplay, px, py int32) (int, int) {
	x, y := physicalToDIPIn(displays, primary, px, py)
	wa := workAreaAt(displays, primary, x, y)
	return clampChip(x, y, wa)
}

// clampChip keeps the chip inside wa, flipping it above the anchor when the
// space below is too small.
func clampChip(x, y int, wa mygo.Rectangle) (int, int) {
	if x+selIconWidth+chipGap > wa.X+wa.Width {
		x = wa.X + wa.Width - selIconWidth - chipGap
	}
	if y+selIconHeight+chipGap > wa.Y+wa.Height {
		// Below the selection there is no room: sit above it instead.
		y = y - selIconHeight - chipGap*2
	} else {
		y += chipGap
	}
	if y < wa.Y {
		y = wa.Y
	}
	if x < wa.X {
		x = wa.X
	}
	return x, y
}

// physicalToDIPIn converts a physical screen pixel to the DIPs mygo windows
// are positioned in, using the scale of the display holding the point.
func physicalToDIPIn(displays []geomDisplay, primary geomDisplay, px, py int32) (int, int) {
	for _, d := range displays {
		sf := scaleOf(d)
		// Display bounds are DIPs; scale them back to physical pixels to
		// find which display holds the point.
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

// workAreaAt returns the work area of the display holding a DIP point.
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

// chipPosition places the chip using the displays mygo reports.
func chipPosition(px, py int32) (int, int) {
	displays := displayGeom()
	return placeChip(displays, primaryGeom(displays), px, py)
}

// physicalToDIP converts a physical screen pixel to DIPs using the displays
// mygo reports.
func physicalToDIP(px, py int32) (int, int) {
	displays := displayGeom()
	return physicalToDIPIn(displays, primaryGeom(displays), px, py)
}

// displayGeom reads the connected displays.
func displayGeom() []geomDisplay {
	ds := mygo.Screen.Displays()
	out := make([]geomDisplay, 0, len(ds))
	for _, d := range ds {
		out = append(out, geomDisplay{Bounds: d.Bounds, WorkArea: d.WorkArea, ScaleFactor: d.ScaleFactor})
	}
	return out
}

// primaryGeom picks the primary display, or the first one.
func primaryGeom(displays []geomDisplay) geomDisplay {
	ds := mygo.Screen.Displays()
	for i, d := range ds {
		if d.Primary && i < len(displays) {
			return displays[i]
		}
	}
	if len(displays) > 0 {
		return displays[0]
	}
	return geomDisplay{ScaleFactor: 1}
}

// ---------------------------------------------------------------------------
// Gestures
// ---------------------------------------------------------------------------

// applySelectionMode (re)installs or removes the selection hooks after the
// user changes the trigger in the settings window.
//
// Both directions are executed by the hook thread itself. removeHooks used
// to run on the main thread, which left the hook thread alive but hookless;
// switching back then hit hookThreadOnce (already done) and never installed
// anything again, so "仅快捷键" was a one-way door that killed selection
// translation until the app was restarted.
func (a *app) applySelectionMode() {
	wantHooks := a.store.Get().SelectionTrigger != "hotkey"
	runOnMainThread()
	apply := func() {
		if wantHooks {
			if err := installHooks(); err != nil {
				log.Printf("fastpot: could not install selection hooks: %v", err)
			}
			return
		}
		removeHooks()
	}
	if err := postHookCommand(apply); err != nil {
		log.Printf("fastpot: selection hook thread not ready (%v); applying on this thread", err)
		apply()
	}
}

// onSelectionGesture runs when a selection gesture has finished. It is
// called from a goroutine of its own, never from the hook itself.
//
// owner is the application the gesture happened in. It is checked before
// anything is read: in a screenshot tool a drag is a snip region, not a text
// selection, and reading the selection there (clear clipboard, synthesise
// Ctrl+C) breaks the tool's own capture.
func (a *app) onSelectionGesture(px, py int32, owner string) {
	cfg := a.store.Get()
	// Every early return below is logged. The trigger used to fail silently,
	// which made "the chip never appears" impossible to diagnose from the log:
	// the only way to tell the causes apart was to read the source.
	log.Printf("fastpot: gesture at (%d,%d) owner=%q trigger=%q", px, py, owner, cfg.SelectionTrigger)

	if cfg.SelectionTrigger == "hotkey" {
		log.Printf("fastpot: gesture skipped: trigger is hotkey-only")
		return
	}
	if ignore, why := a.shouldIgnoreSelection(owner); ignore {
		log.Printf("fastpot: selection ignored (%s)", why)
		return
	}

	// The user's application still has the focus here: this is the moment
	// the selection can be read without disturbing anything.
	sel := a.captureForGesture(owner)
	if strings.TrimSpace(sel.Text) == "" {
		log.Printf("fastpot: gesture read no text (owner=%q capture=%q)", owner, a.store.Get().SelectionCapture)
		return
	}
	log.Printf("fastpot: selection %q (%d chars)", truncateFor(sel.Text, 30), len(sel.Text))
	a.deliverSelection(sel, px, py)
}

// capture reads the current selection. It is a field so tests can supply
// one without a real selection, and so the reading strategy can change.
func (a *app) capture() selection {
	if a.captureSelection != nil {
		return a.captureSelection()
	}
	return captureUIA()
}

// deliverSelection decides what a freshly read selection does: translate at
// once, or offer the chip. It is the whole of the trigger policy, which is
// why it is separate from reading the selection.
func (a *app) deliverSelection(sel selection, px, py int32) {
	sel.Text = CleanSelectionText(sel.Text)
	if strings.TrimSpace(sel.Text) == "" {
		return
	}
	if a.store.Get().SelectionTrigger == "auto" {
		a.showPopupText(modeSelection, sel.Text)
		return
	}

	// Anchor the chip to the selection itself when the provider says where
	// it is, and to the pointer otherwise: the pointer is at the end of a
	// mouse drag, but nowhere near a selection made with the keyboard.
	if sel.Bounds {
		px = int32((sel.Rect.Left + sel.Rect.Right) / 2)
		py = int32(sel.Rect.Bottom)
	}
	a.showChipAt(px, py, sel.Text)
}

// showChipAt shows the selection chip, through the test seam when one is
// installed.
func (a *app) showChipAt(px, py int32, text string) {
	if a.showChip != nil {
		a.showChip(int(px), int(py), text)
		return
	}
	a.showSelIcon(px, py, text)
}

// showPopupText opens the popup, through the test seam when one is
// installed.
func (a *app) showPopupText(m popupMode, text string) {
	if a.showPopup != nil {
		a.showPopup(m, text)
		return
	}
	a.openPopupWithText(m, text)
}

// onGlobalMouseDown hides the chip when the user clicks anywhere else. It
// runs on a goroutine of its own.
func (a *app) onGlobalMouseDown(px, py int32) {
	if a.selIconContains(px, py) {
		return // a click on the chip itself: let it through
	}
	a.hideSelIcon()
}

// ---------------------------------------------------------------------------
// The chip's actions
// ---------------------------------------------------------------------------

// selText returns the text the chip is offering.
func (a *app) selText() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sel.text
}

// actOnSelection runs fn with the chip's text and takes the chip away.
func (a *app) actOnSelection(fn func(text string)) {
	text := a.selText()
	a.hideSelIcon()
	if strings.TrimSpace(text) != "" {
		fn(text)
	}
}

// ---------------------------------------------------------------------------
// The chip's view
// ---------------------------------------------------------------------------

func (a *app) selIconView(c *ui.Context) {
	cfg := a.store.Get()
	applyTheme(c, cfg.ThemeMode, c.Theme().Dark)
	t := c.Theme()
	c.Root().Background(ui.Transparent)

	enabledActions := cfg.GetEnabledActions()

	// Fill the entire window with the clean capsule pill
	ui.Row(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Gap(3).
		Padding(2, 6, 2, 6).Radius(7).
		Background(t.Surface).Border(1, t.Border).
		Shadow(0, 2, 8, 0, ui.RGBA(0, 0, 0, 0.12)).
		Children(func() {
			// 1. [中/A] Translate action
			a.chipIconOnly(c, t, iconTranslate, "划词翻译 (Alt+D)", func() {
				a.actOnSelection(func(text string) { a.openPopupAction(modeTranslate, text) })
			})

			// 2. [🔊] Speak action
			a.chipIconOnly(c, t, iconSpeak, "发音朗读", func() {
				text := a.selText()
				if strings.TrimSpace(text) != "" {
					a.speakSelection(text, false)
				}
			})

			// 3. [⛶] Screenshot OCR action
			a.chipIconOnly(c, t, iconScreenshot, "截图翻译 (Alt+C)", func() {
				a.TriggerScreenshotOCR()
			})

			// 4. Dynamic Enabled AI Actions (词典 / 长难句 / 写作改进 / 自定义)
			for _, action := range enabledActions {
				act := action
				var actIcon *ui.SVG
				var actMode popupMode
				switch act.ID {
				case "default-dictionary":
					actIcon = iconDict
					actMode = modeDictionary
				case "default-sentence-analysis":
					actIcon = iconGrammar
					actMode = modeGrammar
				case "default-improve-writing":
					actIcon = iconPolish
					actMode = modeWriting
				default:
					actIcon = iconSparkles
					actMode = modeTranslate
				}
				a.chipIconOnly(c, t, actIcon, act.Name, func() {
					a.actOnSelection(func(text string) {
						a.openPopupAction(actMode, text)
					})
				})
			}

			// Thin divider
			ui.Box(c).Width(1).Height(16).Background(t.Border)

			// 5. [⋮] More
			a.chipIconOnly(c, t, iconMore, "更多功能", func() { a.toggleMoreFlyout() })
		})
}

// chipIconOnly renders a compact, square, icon-only button with native tooltip
func (a *app) chipIconOnly(c *ui.Context, t *ui.Theme, icon *ui.SVG, tip string, fn func()) {
	btn := ui.Button(c, "").Tooltip(tip).Width(28).Height(26).Padding(2, 2).Radius(5)
	if btn.Hovered() {
		btn.Background(ui.RGBA(0, 0, 0, 0.08))
	}
	btn.OnClick(fn)
	btn.Children(func() {
		ui.Icon(c, icon).Size(16, 16).TextColor(t.Text)
	})
}

// speakSelection pronounces text using Youdao dictionary audio or Windows speech.
func (a *app) speakSelection(text string, us bool) {
	PlaySpeech(text, us)
}

func (a *app) toggleMoreFlyout() {
	if a.moreFlyout != nil && !a.moreFlyout.IsDestroyed() {
		a.moreFlyout.Close()
		a.moreFlyout = nil
		return
	}
	if a.sel.win == nil || a.sel.win.IsDestroyed() {
		return
	}
	a.moreFlyout = mygo.NewFlyout(mygo.FlyoutOptions{
		Parent:    a.sel.win,
		Placement: mygo.PlacementBottomEnd,
		Width:     240,
		Height:    260,
		Focusable: true,
		Shadow:    true,
		Content:   ui.View(safeView("more", a.moreFlyoutView)),
	})
	a.moreFlyout.OnClosed(func() {
		a.moreFlyout = nil
	})
}

func (a *app) moreFlyoutView(c *ui.Context) {
	cfg := a.store.Get()
	applyTheme(c, cfg.ThemeMode, c.Theme().Dark)
	t := c.Theme()

	c.Root().Background(ui.Transparent)
	box := ui.Column(c).Fill().Background(t.Surface).Radius(8).Border(1, t.Border).Padding(12).Gap(8)
	box.Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			ui.Text(c, "悬浮条功能设置").FontSize(13).FontWeight(700).TextColor(t.Text)
			ui.Spacer(c).Grow(1)
			btnClose := ui.Button(c, "✕").Height(18).Padding(0, 4)
			if btnClose.Clicked() {
				if a.moreFlyout != nil {
					a.moreFlyout.Close()
				}
			}
		})

		ui.Divider(c).FillWidth()

		// Custom actions switches
		for _, act := range cfg.CustomActions {
			action := act
			ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
				ui.Text(c, action.Name).FontSize(12).TextColor(t.Text).Grow(1)
				enabled := action.Enabled
				ui.Switch(c, &enabled).Height(18).OnChange(func() {
					a.store.Update(func(c *Config) {
						for i := range c.CustomActions {
							if c.CustomActions[i].ID == action.ID {
								c.CustomActions[i].Enabled = enabled
							}
						}
					})
					if a.sel.win != nil {
						a.sel.win.Update(func() {})
					}
				})
			})
		}

		ui.Divider(c).FillWidth()

		// Quick links
		btnSettings := ui.Button(c, "⚙️ 详细设置中心").FillWidth().Height(26)
		if btnSettings.Clicked() {
			if a.moreFlyout != nil {
				a.moreFlyout.Close()
			}
			a.showSettings()
		}

		btnVocab := ui.Button(c, "🔖 生词本与长难句").FillWidth().Height(26)
		if btnVocab.Clicked() {
			if a.moreFlyout != nil {
				a.moreFlyout.Close()
			}
			a.page = "vocabulary"
			a.showSettings()
		}

		btnHide := ui.Button(c, "关闭悬浮条").FillWidth().Height(26)
		if btnHide.Clicked() {
			if a.moreFlyout != nil {
				a.moreFlyout.Close()
			}
			a.hideSelIcon()
		}
	})
}
