// Low-level mouse and keyboard hooks, the trigger for the selection icon.
//
// A WH_MOUSE_LL hook sees every mouse event before the application it is
// meant for does, and a WH_KEYBOARD_LL hook sees the keys. Together they
// let fastpot recognise the gesture "the user finished selecting text":
//
//   - the left button went down and came back up, and
//   - the pointer moved more than a few pixels while it was down (a drag,
//     not a click), or Shift plus an arrow key selected without the mouse.
//
// The hook runs on the thread that installed it, which is the main thread;
// it must return quickly, so it only records the gesture and hands the work
// to a goroutine through a channel.
package main

import (
	"fmt"
	"log"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"
)

var (
	procSetWindowsHookEx   = user32.NewProc("SetWindowsHookExW")
	procUnhookWindowsHook  = user32.NewProc("UnhookWindowsHookEx")
	procCallNextHookEx     = user32.NewProc("CallNextHookEx")
	procGetDoubleClickTime = user32.NewProc("GetDoubleClickTime")

	hookEvents   uint64 // events seen by the mouse hook, for diagnosis
	hookDowns    uint64 // left button downs
	hookUps      uint64 // left button ups
	hookDrags    uint64 // completed drags past the slack
	hookDragsOwn uint64 // drags rejected because the target was fastpot itself
	// what the hook rejected as "our own window", for diagnosis
	hookRejectedTitle string
	hookRejectedClass string
	// liveMouseHook/liveKeyHook are the handles the callbacks pass back to
	// CallNextHookEx. Passing 0 there is not harmless: the system drops the
	// hook from the chain, and the symptom is a hook that installs
	// successfully and then is never called again.
	liveMouseHook uintptr
	liveKeyHook   uintptr
	hookOnce      sync.Once
	mouseHook     uintptr
	keyHook       uintptr
	mouseProc     uintptr
	keyProc       uintptr
	hookMu        sync.Mutex
	dragging      bool
	dragStart     winPoint
	dragEnd       winPoint
	hookTarget    uintptr // window that was foreground when the drag started
	// click tracking, for the double click that selects a word
	lastClickTime uint32
	lastClickPos  winPoint
	clickCount    int
	// OnSelectionGesture runs, on its own goroutine, when a selection
	// gesture has just finished. owner is the executable name of the
	// application the gesture happened in, which is what the trigger guard
	// needs to tell a real selection from a drag in a screenshot tool.
	// main() installs it.
	OnSelectionGesture func(x, y int32, owner string)
	// OnMouseDown runs, on its own goroutine, for every left press. It lets
	// the selection chip notice a click somewhere else and go away.
	OnMouseDown func(x, y int32)
)

const (
	whMouseLL  = 14
	whKeyboard = 13

	wmMouseMove   = 0x0200
	wmLButtonDown = 0x0201
	wmLButtonUp   = 0x0202
	wmRButtonDown = 0x0204
	wmMButtonDown = 0x0207

	wmKeyDown    = 0x0100
	wmSysKeyDown = 0x0104

	// doubleClickSlack recognises the second press near the first; the
	// time window itself comes from the system (doubleClickThreshold).
	doubleClickSlack = 4

	vkLeft    = 0x25
	vkUp      = 0x26
	vkRight   = 0x27
	vkDown    = 0x28
	llkhfUp   = 0x80
	dragSlack = 10 // pixels: below this a press-and-release is a click
)

// msllHookStruct is MSLLHOOKSTRUCT.
type msllHookStruct struct {
	Pt          winPoint
	MouseData   uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

// kbdllHookStruct is KBDLLHOOKSTRUCT.
type kbdllHookStruct struct {
	VkCode      uint32
	ScanCode    uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

// lowLevelMouseProc is the WH_MOUSE_LL callback. It tracks press, drag and
// release, and reports a completed drag as a selection gesture.
func lowLevelMouseProc(nCode int, wParam uintptr, lParam uintptr) uintptr {
	if nCode < 0 {
		r, _, _ := procCallNextHookEx.Call(liveMouseHook, uintptr(nCode), wParam, lParam)
		return r
	}
	info := (*msllHookStruct)(nativePtr(lParam))
	pt := info.Pt

	// Nothing slow may run here. A WH_MOUSE_LL callback that takes longer
	// than LowLevelHooksTimeout makes Windows silently remove the hook, and
	// the counter below is the only thing cheap enough to keep: a log line
	// written from this callback is enough to kill it.
	atomic.AddUint64(&hookEvents, 1)

	hookMu.Lock()
	var fire, press, clickFire bool
	var fx, fy, cx, cy int32
	var target uintptr
	switch uint32(wParam) {
	case wmLButtonDown:
		atomic.AddUint64(&hookDowns, 1)
		dragging = true
		dragStart = pt
		dragEnd = pt
		hookTarget = foregroundWindow()
		press = true
		// A second press near the previous one, soon after it, is a double
		// click: the applications select the word under the pointer for it,
		// which is how a dictionary is normally used.
		if info.Time-lastClickTime <= doubleClickThreshold() &&
			absInt(int(pt.X-lastClickPos.X)) <= doubleClickSlack &&
			absInt(int(pt.Y-lastClickPos.Y)) <= doubleClickSlack {
			clickCount++
		} else {
			clickCount = 1
		}
		lastClickTime = info.Time
		lastClickPos = pt
		if clickCount == 2 && !isOwnProcess(hookTarget) {
			clickFire = true
			cx, cy = pt.X, pt.Y
			target = hookTarget
		}
	case wmMouseMove:
		if dragging {
			dragEnd = pt
		}
	case wmLButtonUp:
		atomic.AddUint64(&hookUps, 1)
		if dragging {
			dragging = false
			dx := int(dragEnd.X - dragStart.X)
			dy := int(dragEnd.Y - dragStart.Y)
			if dx*dx+dy*dy >= dragSlack*dragSlack {
				atomic.AddUint64(&hookDrags, 1)
				// Resolve the window under the pointer, not the foreground
				// window of the press: a topmost fastpot window (the
				// settings page, a pinned popup) is the foreground one even
				// while the user selects text underneath it, and judging by
				// it dropped every selection made with a fastpot window open.
				hwnd := windowAt(dragEnd.X, dragEnd.Y)
				if !isOwnProcess(hwnd) {
					fire = true
					fx, fy = dragEnd.X, dragEnd.Y
					target = hwnd
				} else {
					atomic.AddUint64(&hookDragsOwn, 1)
					hookRejectedTitle = windowTitleOf(hwnd)
					hookRejectedClass = windowClassName(hwnd)
				}
			}
		}
	case wmRButtonDown, wmMButtonDown:
		dragging = false
	}
	hookMu.Unlock()

	if press && OnMouseDown != nil {
		x, y := pt.X, pt.Y
		go OnMouseDown(x, y)
	}

	// Everything slow happens in the goroutine, never here. A low-level hook
	// runs on the input thread and the whole desktop waits on it: OpenProcess
	// and QueryFullProcessImageName can take milliseconds, which is how a
	// "fast" translator ends up introducing input lag everywhere.
	if clickFire && OnSelectionGesture != nil {
		x, y, hwnd := cx, cy, target
		go func() {
			settleAfterMouseUp()
			OnSelectionGesture(x, y, processNameOf(hwnd))
		}()
	}

	if fire && OnSelectionGesture != nil {
		// The button is still logically down here: the release reaches the
		// application after this hook returns. Let the selection settle
		// before asking for it.
		x, y, hwnd := fx, fy, target
		go func() {
			settleAfterMouseUp()
			OnSelectionGesture(x, y, processNameOf(hwnd))
		}()
	}
	r, _, _ := procCallNextHookEx.Call(liveMouseHook, uintptr(nCode), wParam, lParam)
	return r
}

// lowLevelKeyProc is the WH_KEYBOARD_LL callback. ONLY Shift plus an arrow key
// extends a selection without the mouse; the gesture fires on key up.
func lowLevelKeyProc(nCode int, wParam uintptr, lParam uintptr) uintptr {
	if nCode >= 0 {
		info := (*kbdllHookStruct)(nativePtr(lParam))
		msg := uint32(wParam)
		if msg == wmKeyDown || msg == wmSysKeyDown {
			isArrow := info.VkCode == vkLeft || info.VkCode == vkUp ||
				info.VkCode == vkRight || info.VkCode == vkDown
			shiftHeld := keyDown(vkShift) || keyDown(vkLShift) || keyDown(vkRShift)
			// Crucial: Only treat as selection if Shift is actively held down.
			// Pure arrow keys during normal typing must never trigger selection!
			if isArrow && shiftHeld {
				hookMu.Lock()
				keyboardSelecting = true
				hookTarget = foregroundWindow()
				hookMu.Unlock()
			}
		} else if info.Flags&llkhfUp != 0 {
			hookMu.Lock()
			wasSelecting := keyboardSelecting
			target := hookTarget
			keyboardSelecting = false
			hookMu.Unlock()
			if wasSelecting && !isOwnProcess(target) && OnSelectionGesture != nil {
				// Same rule as the mouse hook: no OpenProcess on the input
				// thread, only inside the goroutine.
				hwnd := target
				go func() {
					settleAfterMouseUp()
					p := cursorPoint()
					OnSelectionGesture(p.X, p.Y, processNameOf(hwnd))
				}()
			}
		}
	}
	r, _, _ := procCallNextHookEx.Call(liveKeyHook, uintptr(nCode), wParam, lParam)
	return r
}

var keyboardSelecting bool

// absInt is |v|, for distance comparisons.
func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// settleAfterMouseUp waits for the application to apply the selection the
// drag just made. Chromium and Word both update it on the mouse-up message,
// which the hook sees before the application does.
func settleAfterMouseUp() {
	// 60ms measured enough on this machine for Chrome, Word and Notepad++;
	// the capture retries anyway, so a slow app costs one more attempt
	// rather than a missed icon.
	sleepMS(60)
}

// installHooks installs both hooks **on the calling thread**.
//
// This detail is the whole reason the selection chip silently stopped
// working: SetWindowsHookEx returns a valid handle even when the installing
// thread has no message loop, and a WH_MOUSE_LL hook is delivered by
// dispatching messages to that thread. Installing from a thread that never
// pumps (mygo's main thread while it is busy, or any worker goroutine) gives
// you a hook that is "installed" and never fires — which is exactly the
// symptom: hooks installed, no events, no chip, no error.
//
// So the caller must be a thread that runs GetMessage/PeekMessage in a loop.
// hookThread below is that thread.
func installHooks() error {
	hookMu.Lock()
	already := mouseHook != 0
	hookMu.Unlock()
	if already {
		return nil
	}

	// Judge the second press by the system's own double-click threshold:
	// a user who tuned Windows' double-click speed expects the same
	// sensitivity here. Cached on first read; 0 means not read yet.
	if atomic.LoadUint32(&doubleClickMSLive) == 0 {
		if ms, _, _ := procGetDoubleClickTime.Call(); ms > 0 {
			atomic.StoreUint32(&doubleClickMSLive, uint32(ms))
		} else {
			atomic.StoreUint32(&doubleClickMSLive, 500)
		}
	}

	// The callbacks are created once: syscall.NewCallback leaks its trampoline
	// otherwise, and the same function may be reused for a new hook.
	hookOnce.Do(func() {
		mouseProc = syscall.NewCallback(lowLevelMouseProc)
		keyProc = syscall.NewCallback(lowLevelKeyProc)
	})

	h, _, err := procSetWindowsHookEx.Call(whMouseLL, mouseProc, 0, 0)
	if h == 0 {
		return fmt.Errorf("mouse hook: %w", err)
	}
	k, _, err := procSetWindowsHookEx.Call(whKeyboard, keyProc, 0, 0)
	if k == 0 {
		procUnhookWindowsHook.Call(h)
		return fmt.Errorf("keyboard hook: %w", err)
	}
	hookMu.Lock()
	mouseHook, keyHook = h, k
	liveMouseHook, liveKeyHook = h, k
	hookMu.Unlock()
	log.Printf("fastpot: selection hooks installed on thread %d (mouse=%d key=%d)",
		currentThreadID(), h, k)
	return nil
}

// removeHooks uninstalls both hooks. Unhooking is thread-agnostic in win32,
// but it is still done from the hook thread so the loop can exit cleanly.
func removeHooks() {
	hookMu.Lock()
	mh, kh := mouseHook, keyHook
	mouseHook, keyHook = 0, 0
	liveMouseHook, liveKeyHook = 0, 0
	hookMu.Unlock()
	if mh != 0 {
		procUnhookWindowsHook.Call(mh)
	}
	if kh != 0 {
		procUnhookWindowsHook.Call(kh)
	}
	log.Printf("fastpot: selection hooks removed")
}

// hookThread runs the selection hooks on a thread of its own that pumps
// messages, which is what a low-level hook needs in order to be delivered.
//
// It replaces the old "install from mygo's main thread" approach: a hook
// installed on a thread that is not pumping is never called.
var (
	hookThreadOnce sync.Once
	hookLoopID     uintptr // thread id, for PostThreadMessage

	// doubleClickMSLive caches GetDoubleClickTime, the system's own
	// double-click threshold, read once from installHooks.
	doubleClickMSLive uint32
)

// doubleClickThreshold returns the window in which a second press counts
// as a double click. The system value honours the user's own double-click
// speed setting; 500ms is the fallback.
func doubleClickThreshold() uint32 {
	if v := atomic.LoadUint32(&doubleClickMSLive); v != 0 {
		return v
	}
	return 500
}

func startHookThread() {
	hookThreadOnce.Do(func() {
		go hookLoop()
	})
}

// hookEventCount is read by diagnostics from a normal goroutine; the hook
// itself only increments a counter.
func hookEventCount() uint64 { return atomic.LoadUint64(&hookEvents) }

// hookStats is a snapshot for the --hookstats reporter.
func hookStats() (events, downs, ups, drags, own uint64) {
	return atomic.LoadUint64(&hookEvents), atomic.LoadUint64(&hookDowns),
		atomic.LoadUint64(&hookUps), atomic.LoadUint64(&hookDrags),
		atomic.LoadUint64(&hookDragsOwn)
}

// wmHookCommand is the private window message used to run hook work on the
// hook thread. WM_APP is the first message id applications may use for
// themselves; anything above it is free.
const wmHookCommand = 0x8000 + 1

// hookCommand carries the work posted to the hook thread.
var (
	hookCommandMu          sync.Mutex
	hookCommandFn          func()
	procPostThreadMessageW = user32.NewProc("PostThreadMessageW")
)

// postHookCommand runs fn on the hook thread, which is the only thread that
// may install or remove a low-level hook. It returns an error when the hook
// thread has not started yet.
func postHookCommand(fn func()) error {
	hookMu.Lock()
	tid := hookLoopID
	hookMu.Unlock()
	if tid == 0 {
		return fmt.Errorf("hook thread not running")
	}
	hookCommandMu.Lock()
	hookCommandFn = fn
	hookCommandMu.Unlock()
	if r, _, err := procPostThreadMessageW.Call(tid, wmHookCommand, 0, 0); r == 0 {
		return fmt.Errorf("PostThreadMessage: %w", err)
	}
	return nil
}

// runHookCommand executes the posted command. It runs on the hook thread.
func runHookCommand() {
	hookCommandMu.Lock()
	fn := hookCommandFn
	hookCommandFn = nil
	hookCommandMu.Unlock()
	if fn != nil {
		fn()
	}
}

func hookLoop() {
	// A low-level hook callback is only delivered to a thread running a
	// message loop, and Go moves goroutines between threads, so pin this one.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hookMu.Lock()
	hookLoopID = currentThreadID()
	hookMu.Unlock()

	if err := installHooks(); err != nil {
		log.Printf("fastpot: selection hooks unavailable: %v", err)
		return
	}

	var m msgW
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 { // WM_QUIT (0) or an error (-1)
			return
		}
		if m.Message == wmHookCommand {
			runHookCommand()
			continue
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// runOnMainThread starts the hook thread. The name is historical; nothing
// here needs the main thread any more, and using it was the bug.
func runOnMainThread() {
	startHookThread()
}

// currentThreadID returns the id of the calling OS thread.
func currentThreadID() uintptr {
	id, _, _ := procGetCurrentThread.Call()
	return id
}
