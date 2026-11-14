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
	"time"
	"unsafe"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// The chip is a single row of icon buttons on one quiet surface. Sizes are
// chosen so the hit target stays comfortable (28px, above the 24px a mouse
// pointer needs) while the bar still reads as a small floating accessory.
const (
	selIconHeight  = 36
	chipButtonSize = 28
	chipIconSize   = 17
	chipPadV       = 3
	chipPadH       = 4
	chipRowGap     = 2
)

// The hover label: a small plate above the chip naming the button under the
// pointer, as read-frog's selection toolbar does.
//
// It is its own window rather than something drawn inside the chip, because
// the chip window is clipped to the pill's rounded rectangle (clipChipWindow)
// and has no room above the row: anything drawn there would be cut away.
const (
	chipTipHeight   = 22
	chipTipGap      = 7
	chipTipFontSize = 12
	chipTipPadH     = 9
	chipTipPadV     = 5
	// chipTipDelay is how long the pointer rests on a button before its label
	// shows. read-frog's tooltips feel immediate; mygo's built-in Tooltip waits
	// 600ms, which on a five-button row reads as "nothing happened".
	chipTipDelay = 160 * time.Millisecond

	// selIconSettle is how long the chip is given to lay out before the
	// pointer under it is sampled. The frame that positions the buttons runs
	// asynchronously, and the boxes recorded by recordChipHit are only there
	// once it has.
	selIconSettle = 120 * time.Millisecond
	// chipTipWarm is how soon after a label goes the next one shows at once,
	// so sweeping the pointer along the row moves the plate with it.
	chipTipWarm = 400 * time.Millisecond
)

// selState is the chip's state, guarded by app.mu.
type selState struct {
	win   *mygo.Window
	text  string
	x, y  int // top-left in DIPs
	shown bool
	// gen 每次显示/隐藏都自增。显示时启动的 pointer 采样只对当次会话有效，
	// 否则 chip 被收起（或收起后很快又重新显示）时，旧采样会把提示抬到
	// 已经不该显示提示的 chip 上。
	gen uint64
}

// chipTipState is the hover label's state. It has a lock of its own so the
// render pass (main thread) and hideSelIcon (any goroutine) never contend for
// app.mu, whose ordering with the window calls is delicate.
type chipTipState struct {
	mu sync.Mutex
	// win is created on first use and then kept for the life of the app.
	win *mygo.Window
	// key identifies the hovered button, label is what it says, since is
	// when the pointer arrived on it, and shown reports that the label is up.
	key, label string
	since      time.Time
	shown      bool
	// hiddenAt is when the label last went, so the next button's label can
	// appear at once as the pointer moves along the row.
	hiddenAt time.Time
	// anchor is the button's box in the chip window, in DIPs.
	anchor ui.Rect
	// gen is bumped every time the label is asked to go away. A raise that
	// was already in flight captures it, and skips the show when it has
	// moved on — otherwise the goroutine's ShowInactive lands after the
	// Hide and the plate stays on screen with nothing left to hide it.
	gen uint64
	// hits are the button boxes of the last frame, in window-local DIPs,
	// each with the label it shows. The pointer is matched against these
	// rather than against a frame's Hovered(): Windows sends no WM_MOUSEMOVE
	// when a window appears under a still pointer, so the render pass that
	// lays these boxes out can be the last one that ever runs.
	hits []chipHit
	// timer finishes the hover wait when the pointer stops moving. One timer
	// is kept and reset rather than one per move: a fast sweep over the row
	// would otherwise leave a hundred of them to fire at once.
	timer *time.Timer
	// geom is the display geometry the chip was placed with, sampled once per
	// appearance. Every pointer move converts a physical point to DIPs, and
	// asking mygo.Screen for the displays there would put a main-thread round
	// trip on the pointer path hundreds of times a second. Displays do not
	// move while the chip is on screen.
	geom chipGeom
}

// chipGeom is the display snapshot a chip appearance is placed with.
type chipGeom struct {
	displays []geomDisplay
	primary  geomDisplay
}

// chipHit is one button of the chip: where it is and what it is called.
type chipHit struct {
	label string
	rect  ui.Rect
}

var (
	gdi32Dll               = syscall.NewLazyDLL("gdi32.dll")
	procCreateRoundRectRgn = gdi32Dll.NewProc("CreateRoundRectRgn")
	procDeleteObject       = gdi32Dll.NewProc("DeleteObject")
	procSetWindowRgn       = user32.NewProc("SetWindowRgn")
)

// selectionChipWidth keeps the toolbar compact and proportional.
// Base buttons: translate, speak, screenshot, more (4 buttons). Every item is
// chipButtonSize wide, one 1px divider separates the tools from "more", and
// the row carries a gap between each pair of neighbours.
func selectionChipWidth(enabledActions int) int {
	if enabledActions < 0 {
		enabledActions = 0
	}
	items := 4 + enabledActions
	// One gap between each neighbouring item, the divider included.
	gaps := items
	return 2*chipPadH + items*chipButtonSize + 1 + gaps*chipRowGap
}

func (a *app) currentSelectionChipWidth() int {
	return selectionChipWidth(len(a.store.Get().GetEnabledActions()))
}

// clipChipWindow clips the window to a clean rounded rectangle matching its
// actual dimensions. The radius matches the pill drawn inside it, so the
// window's square corners never show over the page.
func (a *app) clipChipWindow(hwnd uintptr, dpi int) {
	w := int32((a.currentSelectionChipWidth()*dpi + 48) / 96)
	h := int32((selIconHeight*dpi + 48) / 96)
	// 11 DIP is the pill's own radius; anything smaller leaves a sliver of
	// window outside the drawn surface.
	clipRoundedWindow(hwnd, w, h, (11*dpi+48)/96)
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
			Title:         appTitle + " Selection",
			Width:         a.currentSelectionChipWidth(),
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
	// The display list is read once here, on the showing path, and reused by
	// every pointer move that follows. Reading it per move would be a
	// main-thread round trip on the hook's goroutine thousands of times a
	// second while the pointer travels.
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

	win.SetSize(width, selIconHeight)
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
		// Alt+Tab, and WS_EX_NOACTIVATE keeps a click on it from pulling
		// the keyboard away from the page. mygo has no option for either, so
		// they are set here — as bits to add, not as a branch that runs only
		// when toolwindow is missing, which Panel may already have set.
		ex, _, _ := procGetWindowLongPtr.Call(hwnd, gwlExStyle)
		want := ex | wsExToolWindow | wsExNoActivate
		if want != ex {
			procSetWindowLongPtr.Call(hwnd, gwlExStyle, want)
		}
		a.placeChipWindow(hwnd, x, y)
		// Re-assert after the frame change the style edit triggers.
		a.placeChipWindow(hwnd, x, y)
		var rr winRect
		procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rr)))
		log.Printf("fastDictionary: after placeChipWindow(%d,%d): hwnd=%#x rect=(%d,%d %dx%d)",
			x, y, hwnd, rr.Left, rr.Top, rr.Right-rr.Left, rr.Bottom-rr.Top)
	})
	win.Update(func() {})

	// Pointer moves are dropped by the hook until the chip is actually up, so
	// the very first move after appearance still finds it armed.
	armChipHover()

	// The chip appears under a pointer that is already still: the bar is put
	// where the selection ended, which is where the mouse is. Windows sends no
	// move for that, so the pointer is sampled here once the frame that gives
	// the buttons their boxes has run. The wait is retried because that frame
	// is posted to the main thread and can land after the window is up.
	go a.sampleChipPointer(gen)
}

// sampleChipPointer asks the chip which button the pointer is on right now.
//
// It exists for the moment the chip appears: the pointer is already where it
// will be, and a still pointer generates no moves, so nothing else would ever
// start the hover wait.
func (a *app) sampleChipPointer(gen uint64) {
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

// chipSampleStillCurrent 判断一次 pointer 采样是否仍属于当前 chip 会话。
func (a *app) chipSampleStillCurrent(gen uint64) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sel.shown && a.sel.gen == gen
}

// chipHitBoxesReady reports whether the last frame recorded the button boxes.
func (a *app) chipHitBoxesReady() bool {
	a.chipTip.mu.Lock()
	ready := len(a.chipTip.hits) > 0
	a.chipTip.mu.Unlock()
	return ready
}

// placeChipWindow puts the chip at a DIP position with Win32 directly. It
// runs on the main thread. The DPI of the monitor the point is on converts
// the DIP position the layout uses into the physical pixels SetWindowPos
// wants.
func (a *app) placeChipWindow(hwnd uintptr, x, y int) {
	dpi := monitorDPIAt(x, y)
	px := int32((x*dpi + 48) / 96)
	py := int32((y*dpi + 48) / 96)
	w := int32((a.currentSelectionChipWidth()*dpi + 48) / 96)
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
	a.clipChipWindow(hwnd, dpi)
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

// logChipTipState prints what the hover label window really is, as
// logChipState does for the chip. It exists because a label window that fails
// to place itself is invisible to every headless test: the plate would simply
// never appear on screen with no failing assertion anywhere.
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

	// What the flyout sees as its parent's client origin: this is the value
	// its placement is built from, and if it disagrees with the chip's real
	// rectangle the label lands somewhere else entirely.
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
	owner, _, _ := procGetWindow.Call(hwnd, 4) // GW_OWNER
	log.Printf("fastDictionary: chip hwnd=%#x rect=(%d,%d %dx%d) visible=%v iconic=%v",
		hwnd, r.Left, r.Top, r.Right-r.Left, r.Bottom-r.Top, vis != 0, iconic != 0)
	log.Printf("fastDictionary:   exstyle=%#x toolwindow=%v layered=%v topmost=%v noactivate=%v appwindow=%v",
		uint32(ex), ex&wsExToolWindow != 0, ex&wsExLayered != 0, ex&wsExTopmost != 0,
		ex&wsExNoActivate != 0, ex&wsExAppWindow != 0)
	log.Printf("fastDictionary:   style=%#x child=%v popup=%v visibleBit=%v owner=%#x",
		uint32(st), st&0x40000000 != 0, st&0x80000000 != 0, st&0x10000000 != 0, owner)
	b := win.Bounds()
	log.Printf("fastDictionary: chip reports bounds %+v", b)

	// Every top-level window this process owns, with its real rectangle.
	log.Printf("fastDictionary: windows of pid %d:", currentProcessID())
	for _, w := range listProcessWindows(currentProcessID()) {
		log.Printf("fastDictionary:   %s", w)
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
	a.sel.shown = false
	a.sel.gen++
	a.mu.Unlock()
	// Before the tip: from here on no pointer move is the chip's business.
	disarmChipHover()
	a.hideChipTip()
	// Hide whenever the window exists, not only when shown was true: the flag
	// and the window can disagree (a ShowInactive that landed after an earlier
	// Hide), and the old guard then left a chip on screen that no later hide
	// would ever take down. Hide on a hidden window is a no-op.
	if win != nil && !win.IsDestroyed() {
		win.Hide()
	}
}

// ---------------------------------------------------------------------------
// The hover label
// ---------------------------------------------------------------------------

// recordChipHit notes the box this button occupied last frame, so the hover
// label can follow the pointer even when no frame runs.
//
// Windows sends WM_MOUSEMOVE to the window under the pointer, and the chip is
// shown *under* a pointer that is already still. It therefore receives no
// move at all, and a chain driven from the render pass fires exactly once —
// before the buttons have a box — and never again. The pointer hook calls
// onChipPointer, which matches the cursor against these boxes.
func (a *app) recordChipHit(btn ui.Element, label string) {
	t := &a.chipTip
	t.mu.Lock()
	// Hit boxes are rebuilt every frame: a theme change or a different hotkey
	// label can move or relabel them.
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

// onChipPointer follows the pointer over the chip. The mouse hook calls it on
// its own goroutine for every move; it is also called once after the chip is
// shown, because the pointer that selected the text is already inside the bar
// by the time the bar appears.
func (a *app) onChipPointer(px, py int32) {
	a.mu.Lock()
	shown, originX, originY := a.sel.shown, a.sel.x, a.sel.y
	a.mu.Unlock()
	if !shown {
		return
	}
	// The display list is the snapshot taken when the chip appeared, not a
	// fresh read: mygo.Screen answers on the main thread, and a pointer move
	// must not wait for it.
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
		// A new button under the pointer restarts the wait — unless a label
		// was showing a moment ago. Moving along the row then moves the plate
		// with the pointer, which is how a toolbar should feel.
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
		// The pointer may stop moving, so the wait is finished on a timer.
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

// hitAt reports which button, if any, holds the given screen point.
//
// The point arrives in DIPs, already converted: converting it needs
// mygo.Screen, which answers on the main thread, and this is called while
// chipTip.mu is held — the same lock the render thread takes at the top of
// selIconView. Waiting for the main thread there deadlocks the two.
func (t *chipTipState) hitAt(x, y int, originX, originY int) (string, bool) {
	x, y = x-originX, y-originY
	for _, h := range t.hits {
		r := h.rect
		if float32(x) >= r.X && float32(x) <= r.X+r.W && float32(y) >= r.Y && float32(y) <= r.Y+r.H {
			return h.label, true
		}
	}
	return "", false
}

// anchorFor returns the box of a recorded button.
func (t *chipTipState) anchorFor(label string) ui.Rect {
	for _, h := range t.hits {
		if h.label == label {
			return h.rect
		}
	}
	return ui.Rect{}
}

// raiseChipTip shows the label above the hovered button.
//
// It is a flyout of the chip rather than a plain window. A window created by
// NewWindow is an overlapped window, and the desktop floors an overlapped
// window's height at 64 physical pixels — a 22 DIP plate silently became a
// 36 DIP slab with the text floating in it. A flyout is a WS_POPUP owned by
// the chip, with no such floor, and it already knows how to sit beside a
// rectangle of its parent and to flip when the work area has no room.
func (a *app) raiseChipTip(label string, anchor ui.Rect) {
	a.mu.Lock()
	chipShown := a.sel.shown
	chipWin := a.sel.win
	a.mu.Unlock()
	if !chipShown || chipWin == nil || chipWin.IsDestroyed() {
		return
	}

	// Record the label before the flyout draws: the view reads it from here.
	t := &a.chipTip
	t.mu.Lock()
	t.label = label
	t.shown = true
	gen := t.gen
	win := t.win
	t.mu.Unlock()

	width := chipTipWidth(label)
	// Centre the plate on the button it names. The anchor is the button's box
	// in the chip's content area, which is exactly what a flyout's Anchor is
	// in.
	anchorRect := mygo.Rectangle{
		X: int(anchor.X+anchor.W/2) - width/2, Y: 0, Width: width, Height: 1,
	}

	if win == nil || win.IsDestroyed() {
		// Created at the anchor it will keep: a flyout places itself from its
		// options as it is created, and calling SetAnchor afterwards races
		// the placement that ShowInactive triggers.
		win = mygo.NewFlyout(mygo.FlyoutOptions{
			Parent:    chipWin,
			Anchor:    anchorRect,
			Placement: mygo.PlacementTop,
			Gap:       chipTipGap,
			Width:     width,
			Height:    chipTipHeight,
			// Never focusable: the label must not take the keyboard from the
			// page the user is reading.
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
		// SetAnchor and SetSize each re-place the flyout; one more explicit
		// placement after both settles it at the size and anchor just set.
		win.SetAnchor(anchorRect)
	}
	// Showing must not outrun a hide that happened while this goroutine was
	// starting: onChipPointer claims the raise before starting the goroutine and
	// then leaves, so the pointer can be off the chip, and the plate hidden,
	// before the show below ever runs. Re-checking the generation under the
	// same lock closes that window.
	t.mu.Lock()
	stale := t.gen != gen
	t.mu.Unlock()
	if stale {
		return
	}
	win.ShowInactive()
	// Placing is re-asserted after the show, exactly as the chip's own
	// placement is: a flyout computes its position from the parent's client
	// origin, and that origin is only valid once the parent is really on
	// screen. Without this the label landed at a fixed offset from the
	// desktop's top-left corner instead of above its button.
	mygo.RunOnMain(func() {
		// The same race one queue-hop later: this callback runs on the main
		// thread after ShowInactive, and a hide may have arrived in between.
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

// hideChipTip takes the label away.
func (a *app) hideChipTip() {
	t := &a.chipTip
	t.mu.Lock()
	t.shown = false
	t.key = ""
	// Bumping the generation is what makes a raise already in flight give
	// up. Without it the goroutine's ShowInactive lands after this Hide and
	// the plate stays on screen with t.shown false, so no later hide would
	// ever take it down — the window becomes unclosable until restart.
	t.gen++
	t.hiddenAt = time.Now()
	if t.timer != nil {
		t.timer.Stop()
		t.timer = nil
	}
	win := t.win
	t.mu.Unlock()
	// Hide unconditionally rather than only when the state said the plate was
	// up. State and window can disagree after a race, and this is the last
	// line of defence against a plate the user cannot dismiss: hiding a
	// window that is already hidden costs nothing.
	if win != nil && !win.IsDestroyed() {
		win.Hide()
	}
}

// chipTipWidth sizes the label plate to its text, with a minimum so a short
// name still reads as a plate and not as a smudge.
func chipTipWidth(label string) int {
	// Latin glyphs average about 55% of the font size, CJK ones a full em.
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

// clipRoundedWindow cuts a window to a rounded rectangle. The chip needs it: a
// frameless window has square corners, and the pill is drawn with a radius, so
// without the region the corners of the window show as opaque squares over the
// page. (A flyout is already transparent there, so the label does not need it.)
func clipRoundedWindow(hwnd uintptr, w, h int32, radius int) {
	rgn, _, _ := procCreateRoundRectRgn.Call(0, 0, uintptr(w+1), uintptr(h+1),
		uintptr(radius), uintptr(radius))
	if rgn == 0 {
		return
	}
	// SetWindowRgn takes ownership of the region on success.
	if ok, _, _ := procSetWindowRgn.Call(hwnd, rgn, 1); ok == 0 {
		procDeleteObject.Call(rgn)
	}
}

// chipTipView draws the label: one line of text on a slate plate.
func (a *app) chipTipView(c *ui.Context) {
	cfg := a.store.Get()
	applyTheme(c, cfg.ThemeMode, c.Theme().Dark)
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
	// The flyout is transparent where the content draws nothing, so the plate
	// supplies its own rounded corners; no window region is needed.
	ui.Row(c).Fill().AlignItems(ui.Center).Justify(ui.Center).
		Padding(chipTipPadV, chipTipPadH).Radius(6).
		Background(bg).
		Children(func() {
			ui.Text(c, label).FontSize(chipTipFontSize).FontWeight(500).
				TextColor(fg).SingleLine().MaxLines(1)
		})
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
	// Cached snapshot: this runs on the hook's goroutine for every press, and
	// a synchronous mygo.Screen read there waits on the main thread.
	dx, dy := a.chipTip.chipToDIP(px, py)
	// A little slack: the click that lands on the chip's very edge should
	// still count as being on it.
	const slack = 4
	return dx >= x-slack && dx <= x+a.currentSelectionChipWidth()+slack &&
		dy >= y-slack && dy <= y+selIconHeight+slack
}

// geomDisplay is the slice of a display's geometry the chip's placement
// needs. Keeping it separate from mygo.Display lets the placement be tested
// without a running application.
type geomDisplay struct {
	Bounds      mygo.Rectangle
	WorkArea    mygo.Rectangle
	ScaleFactor float64
	Primary     bool
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
	return placeChipWithWidth(displays, primary, px, py, selectionChipWidth(len(DefaultCustomActions())))
}

func placeChipWithWidth(displays []geomDisplay, primary geomDisplay, px, py int32, width int) (int, int) {
	x, y := physicalToDIPIn(displays, primary, px, py)
	wa := workAreaAt(displays, primary, x, y)
	return clampChip(x, y, wa, width)
}

// clampChip keeps the chip inside wa, flipping it above the anchor when the
// space below is too small, and ensuring symmetrical safety margins on all edges.
func clampChip(x, y int, wa mygo.Rectangle, width int) (int, int) {
	if x+width+chipGap > wa.X+wa.Width {
		x = wa.X + wa.Width - width - chipGap
	}
	if x < wa.X+chipGap {
		x = wa.X + chipGap
	}

	if y+selIconHeight+chipGap > wa.Y+wa.Height {
		// Below the selection there is no room: sit above it instead.
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

// chipPosition places the chip using the display snapshot taken for this
// appearance, so placement and pointer matching agree on one geometry.
func (a *app) chipPosition(px, py int32) (int, int) {
	geom := a.chipTip.geomSnapshot()
	return placeChipWithWidth(geom.displays, geom.primary, px, py, a.currentSelectionChipWidth())
}

// cacheChipGeom takes the display snapshot the pointer path converts with.
//
// It reads mygo.Screen, which answers on the main thread, so it runs on the
// showing path where one round trip costs nothing, not on the move path where
// there would be one per pointer event.
func (a *app) cacheChipGeom() {
	displays := displayGeom()
	t := &a.chipTip
	t.mu.Lock()
	t.geom = chipGeom{displays: displays, primary: primaryGeom(displays)}
	t.mu.Unlock()
}

// geomSnapshot copies the cached display geometry.
func (t *chipTipState) geomSnapshot() chipGeom {
	t.mu.Lock()
	geom := t.geom
	t.mu.Unlock()
	return geom
}

// chipToDIP converts a physical screen pixel to DIPs with the cached snapshot.
//
// It never touches mygo: the caller may run on the pointer hook's goroutine,
// and mygo answers on the main thread.
func (t *chipTipState) chipToDIP(px, py int32) (int, int) {
	geom := t.geomSnapshot()
	return physicalToDIPIn(geom.displays, geom.primary, px, py)
}

// displayGeom reads the connected displays.
func displayGeom() []geomDisplay {
	ds := mygo.Screen.Displays()
	out := make([]geomDisplay, 0, len(ds))
	for _, d := range ds {
		out = append(out, geomDisplay{Bounds: d.Bounds, WorkArea: d.WorkArea, ScaleFactor: d.ScaleFactor, Primary: d.Primary})
	}
	return out
}

// primaryGeom picks the primary display, or the first one.
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
	// The trigger changed: a cooldown from the previous mode would be charged
	// against the first gesture under the new one.
	a.gestureGate.reset()
	wantHooks := a.store.Get().SelectionTrigger != "hotkey"
	// Both install and remove must run on the hook thread, which is the only
	// thread with a message loop. It has to be up before the command is
	// posted: posting to a thread that is still starting loses the command,
	// and installing from this goroutine instead gives a hook that is
	// installed and never fires (see installHooks).
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
			return // no hooks were ever installed, so there is nothing to remove
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

// onSelectionGesture runs when a selection gesture has finished. It is
// called from a goroutine of its own, never from the hook itself.
//
// The gesture carries the application it happened in, which is checked before
// anything is read: in a screenshot tool a drag is a snip region, not a text
// selection, and reading the selection there (clear clipboard, synthesise
// Ctrl+C) breaks the tool's own capture.
func (a *app) onSelectionGesture(g selectionGesture) {
	px, py, owner := g.X, g.Y, g.Owner
	cfg := a.store.Get()
	// Every early return below is logged. The trigger used to fail silently,
	// which made "the chip never appears" impossible to diagnose from the log:
	// the only way to tell the causes apart was to read the source.
	log.Printf("fastDictionary: gesture at (%d,%d) owner=%q kind=%s trigger=%q", px, py, owner, g.Kind, cfg.SelectionTrigger)

	if cfg.SelectionTrigger == "hotkey" {
		log.Printf("fastDictionary: gesture skipped: trigger is hotkey-only")
		return
	}
	// A drag that begins while the user is typing is the user placing the
	// caret, not asking for a translation. This is the "chip jumps out while
	// I am typing in an input box" report: inside a text field every attempt
	// to move the caret, or to sweep a few characters in order to rewrite
	// them, has the exact shape of a selection.
	if typingSuppresses(g) {
		log.Printf("fastDictionary: gesture dropped (typing: %v after last key, kind=%s)",
			g.PressedAt.Sub(g.LastKey).Round(time.Millisecond), g.Kind)
		return
	}
	if ignore, why := a.shouldIgnoreSelection(owner); ignore {
		log.Printf("fastDictionary: selection ignored (%s)", why)
		return
	}
	// The cooldown runs after the guard so a paused or blocklisted app does
	// not consume the window, and before the read so a refused gesture never
	// touches the clipboard. It is only booked once the read produced text:
	// a failed read used to burn the same-spot cooldown, so retrying it a
	// second later was refused as a repeat of a gesture that never worked.
	now := time.Now()
	if ok, why := a.gestureGate.canAllow(now, px, py); !ok {
		log.Printf("fastDictionary: gesture dropped (%s)", why)
		return
	}

	// The user's application still has the focus here: this is the moment
	// the selection can be read without disturbing anything.
	sel := a.captureForGesture(owner)
	if why := selectionTextProblem(sel.Text); why != "" {
		// The read itself is reported, not just the refusal: "read one
		// character" and "read nothing" are different bugs, and telling
		// them apart from the log used to be impossible.
		log.Printf("fastDictionary: gesture produced no chip (%s): %s from %q (capture=%s)",
			why, a.logText(sel.Text, 30), owner, cfg.SelectionCapture)
		return
	}
	log.Printf("fastDictionary: selection %s (%d chars) bounds=%v",
		a.logText(sel.Text, 30), len(sel.Text), sel.Bounds)
	a.gestureGate.record(now, px, py)
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
	if why := selectionTextProblem(sel.Text); why != "" {
		// Cleaning can empty a selection that was only whitespace, so the
		// check runs again on the text the chip would actually show.
		log.Printf("fastDictionary: selection unusable after cleaning (%s)", why)
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

// onGlobalKeyDown hides the chip when the user starts typing. It runs on a
// goroutine of its own.
//
// This is the other half of the "chip jumps out while I am typing" report:
// the trigger no longer fires there, but a chip raised a moment earlier would
// otherwise stay on screen, on top of the text field the user is typing into,
// until they happened to click somewhere. Typing is unambiguous evidence that
// the user has moved on to something else, so the chip goes away at the first
// character.
func (a *app) onGlobalKeyDown(vk uint32) {
	// The hotkeys fastDictionary itself owns are handled elsewhere; taking the chip
	// away for them too is harmless, and keeping the rule simple is worth
	// more than a special case here.
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

	// One cool surface, one accent. The primary action (translate) carries the
	// only saturated colour on the bar; every other glyph is the same slate
	// ink, so the row reads as one instrument instead of six stickers. The
	// buttons are told apart by glyph and position, and — the part this chip
	// was missing — by a label that names the hovered button within 160ms.
	// Boxes are rebuilt from scratch on every frame, so a button that the
	// current config no longer draws cannot keep answering for a stale box.
	a.chipTip.mu.Lock()
	a.chipTip.hits = a.chipTip.hits[:0]
	a.chipTip.mu.Unlock()

	enabledActions := cfg.GetEnabledActions()
	contact, lift := chipShadows(t)
	ui.Row(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Gap(chipRowGap).
		Padding(chipPadV, chipPadH).Radius(11).
		Background(chipSurface(t)).Border(1, chipBorderColor(t)).
		// Two shadows instead of one: a tight contact shadow that seats the
		// pill on the page, and a wide soft one that lifts it. A single
		// 12px blur at 12% looked like a grey halo.
		Shadow(0, 1, 2, 0, contact).
		Shadow(0, 6, 20, 0, lift).
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
			a.chipIconButton(c, t, iconMore, chipToneInk, "复制与更多功能", func() { a.toggleMoreFlyout() })
		})
}

// chipHoverMotion is the curve every chip button shares. Short and
// decelerating: the chip is a small accessory, so feedback has to arrive
// within a glance (140ms) and settle, never bounce. Sharing one token keeps
// the seven buttons in step instead of each with a duration of its own.
var chipHoverMotion = ui.Animation{Duration: 140 * time.Millisecond, Ease: ui.EaseOut}

// chipIconButton renders one square command button. The label stays on the
// button for the accessibility tree; the visible name is the hover plate
// above the bar, raised by onChipPointer.
//
// Hover does two things at once and both are deliberate: the glyph deepens
// from slate to near-black, and the button takes a faint wash. The wash is
// the same slate for every button — a per-button hue was what made the row
// look like six unrelated tools — and the primary action keeps its accent so
// it stays findable while the pointer is elsewhere.
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
	// The label is raised from the box this button had last frame, so it sits
	// exactly over the button that is hovered.
	a.recordChipHit(btn, tip)
	btn.Children(func() {
		// Scale is a transform: it never moves the neighbours, so the row
		// cannot jitter as the pointer crosses it.
		iconEl := ui.Icon(c, icon).Size(chipIconSize, chipIconSize)
		if btn.Pressed() {
			iconEl.Scale(0.88, 0.88)
		}
		iconEl.TextColor(iconEl.AnimateColor("fg", fg, chipHoverMotion))
	})
}

// speakSelection pronounces text using Youdao dictionary audio or Windows speech.
func (a *app) speakSelection(text string, us bool) {
	PlaySpeechLang(text, a.speechLang(), us)
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
		Width:     232,
		// Four rows and a title. The old 272 was sized for the duplicated
		// action list that no longer exists, so it left a third of the panel
		// empty below 设置.
		Height:    196,
		Focusable: true,
		Shadow:    true,
		Content:   ui.View(safeView("more", a.moreFlyoutView)),
	})
	a.moreFlyout.OnClosed(func() {
		a.moreFlyout = nil
	})
}

// moreFlyoutView holds what the chip itself does not offer. The AI actions
// used to be listed here a second time, one full-width button each, which
// duplicated the row of icons the user had just clicked from and made the
// menu tall for no gain.
func (a *app) moreFlyoutView(c *ui.Context) {
	cfg := a.store.Get()
	applyTheme(c, cfg.ThemeMode, c.Theme().Dark)
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
			a.showMainWindow("actions")
		}) {
			return
		}
		if a.chipMenuItem(c, t, iconBookmark, "生词本", func() {
			a.showMainWindow("vocabulary")
		}) {
			return
		}
		if a.chipMenuItem(c, t, iconInfo, "设置", func() { a.showSettings() }) {
			return
		}
	})
}

// chipMenuItem is one flat row in the "more" flyout: icon, label, full-width
// hit area, hover fill. It reports whether it was clicked so the caller can
// stop the rest of the frame, mirroring the old inline pattern.
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
	if a.moreFlyout != nil {
		a.moreFlyout.Close()
	}
	fn()
	return true
}
