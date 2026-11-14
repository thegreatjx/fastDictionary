// Low-level mouse and keyboard hooks, the trigger for the selection icon.
//
// A WH_MOUSE_LL hook sees every mouse event before the application it is
// meant for does, and a WH_KEYBOARD_LL hook sees the keys. Together they
// let fastDictionary recognise the gesture "the user finished selecting text":
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
	"time"
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
	hookDrags    uint64 // completed drags that passed the policy
	hookDragsOwn uint64 // drags rejected because the target was fastDictionary itself
	// hookRejectedDrags counts drags the policy dropped (too short, too
	// fast) and hookRejectedDragWhy keeps the last reason, so --hookstats
	// can explain a quiet trigger without a log line in the callback.
	hookRejectedDrags   uint64
	hookRejectedDragWhy atomic.Pointer[string]
	// what the hook rejected as "our own window", for diagnosis
	hookRejectedTitle atomic.Pointer[string]
	hookRejectedClass atomic.Pointer[string]
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
	// hookTarget is the window the gesture happened in: the window under the
	// pointer for a drag or a double click, the foreground window for a
	// keyboard selection.
	hookTarget uintptr
	// click tracking, for the double click that selects a word
	lastClickTime uint32
	lastClickPos  winPoint
	clickCount    int
	// dblClickPending remembers that the press now in progress was the second
	// (or third) of a double click, so the release can fire the gesture. The
	// click must not fire on the press: the applications select the word on
	// the release, and only then does the pointer say which window it is in.
	dblClickPending bool
	// pressTime is when the button went down, in the hook's own millisecond
	// clock (MSLLHOOKSTRUCT.Time). It is what the duration floor judges.
	pressTime uint32
	// pressAt is the same moment on the wall clock, and pressLastKey is when
	// the user last typed before it. The pair is what tells a caret drag
	// inside a text field apart from a selection drag: see typingSuppresses.
	// They are read at the release, so they are taken at the press.
	pressAt      time.Time
	pressLastKey time.Time
	// lastKeyAt is when a key that types (not a modifier) was last pressed.
	// It is the clock the typing guard reads; the zero value means the user
	// has not typed since the program started.
	lastKeyAt time.Time
	// OnSelectionGesture runs, on its own goroutine, when a selection
	// gesture has just finished. The gesture carries the executable name of
	// the application it happened in, which is what the trigger guard needs
	// to tell a real selection from a drag in a screenshot tool, and the
	// keystroke clock, which is what tells a selection from a caret drag
	// while the user is typing. main() installs it.
	OnSelectionGesture func(g selectionGesture)
	// OnMouseDown runs, on its own goroutine, for every left press. It lets
	// the selection chip notice a click somewhere else and go away.
	OnMouseDown func(x, y int32)
	// OnMouseMove runs, on its own goroutine, for every pointer move. The
	// chip needs it because a window that appears under a still pointer gets
	// no WM_MOUSEMOVE: without this the hovered button never learns it is
	// hovered, and the label it is supposed to show never appears.
	OnMouseMove func(x, y int32)
	// OnKeyDown runs, on its own goroutine, for every key press that is not
	// part of a selection. It is what takes the chip away when the user
	// starts typing: a chip left on screen over a text field is in the way
	// of the very thing the user is doing.
	OnKeyDown func(vk uint32)
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
	// 6 rather than 4: on a scaled display the two presses drift a pixel or
	// two, and a tighter slack dropped legitimate double clicks.
	doubleClickSlack = 6

	vkLeft  = 0x25
	vkUp    = 0x26
	vkRight = 0x27
	vkDown  = 0x28
	llkhfUp = 0x80
	// How far a drag must travel and how long it must take is not a magic
	// number here any more: see selection_policy.go, where the thresholds
	// are named, documented and tested.
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
	var fire, press, clickFire, moved bool
	var fx, fy, cx, cy, moveX, moveY int32
	var target uintptr
	var kind gestureKind
	var lastKey, pressedAt time.Time
	switch uint32(wParam) {
	case wmLButtonDown:
		atomic.AddUint64(&hookDowns, 1)
		dragging = true
		dragStart = pt
		dragEnd = pt
		pressTime = info.Time
		// The typing guard judges the moment the drag *began*, so both
		// clocks are taken here rather than at the release: a caret drag
		// that starts while the user is typing and lasts half a second is
		// still a caret drag.
		pressAt = time.Now()
		pressLastKey = lastKeyAt
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
		// The click becomes a gesture at the release, not here: a topmost
		// fastDictionary window (a pinned card, the settings page) is the
		// foreground window even while the user double-clicks a word
		// underneath it, and judging by it dropped those selections.
		dblClickPending = clickCount == 2 || clickCount == 3
	case wmMouseMove:
		if dragging {
			dragEnd = pt
		}
		moveX, moveY = pt.X, pt.Y
		moved = true
	case wmLButtonUp:
		atomic.AddUint64(&hookUps, 1)
		wasDbl := dblClickPending
		dblClickPending = false
		if dragging {
			dragging = false
			held := time.Duration(info.Time-pressTime) * time.Millisecond
			if wasDbl && absInt(int(pt.X-dragStart.X)) <= doubleClickSlack &&
				absInt(int(pt.Y-dragStart.Y)) <= doubleClickSlack {
				// The second press barely moved before coming up: a double
				// click, whose selection the application has just made. The
				// owner is the window under the release point.
				if hwnd := windowAt(pt.X, pt.Y); !isOwnProcess(hwnd) {
					clickFire = true
					kind = gestureDoubleClick
					lastKey = pressLastKey
					pressedAt = pressAt
					cx, cy = pt.X, pt.Y
					target = hwnd
				} else {
					atomic.AddUint64(&hookDragsOwn, 1)
				}
			} else if ok, why := dragIsSelection(dragStart, dragEnd, held); !ok {
				// Counted, not logged: a log line written from this callback
				// is slow enough to get the hook removed by Windows.
				atomic.AddUint64(&hookRejectedDrags, 1)
				hookRejectedDragWhy.Store(&why)
			} else {
				atomic.AddUint64(&hookDrags, 1)
				// Resolve the window under the pointer, not the foreground
				// window of the press: a topmost fastDictionary window (the
				// settings page, a pinned popup) is the foreground one even
				// while the user selects text underneath it, and judging by
				// it dropped every selection made with a fastDictionary window open.
				hwnd := windowAt(dragEnd.X, dragEnd.Y)
				if !isOwnProcess(hwnd) {
					fire = true
					kind = gestureDrag
					lastKey = pressLastKey
					pressedAt = pressAt
					fx, fy = dragEnd.X, dragEnd.Y
					target = hwnd
				} else {
					atomic.AddUint64(&hookDragsOwn, 1)
					title, class := windowTitleOf(hwnd), windowClassName(hwnd)
					hookRejectedTitle.Store(&title)
					hookRejectedClass.Store(&class)
				}
			}
		}
	case wmRButtonDown, wmMButtonDown:
		dragging = false
		dblClickPending = false
		resetClickTrackingLocked()
	}
	hookMu.Unlock()

	if press && OnMouseDown != nil {
		x, y := pt.X, pt.Y
		go OnMouseDown(x, y)
	}

	// Same rule as the press, with one more step: the move is handed to a
	// single coalescing worker rather than a goroutine per event. Pointer
	// moves arrive by the thousand per second, and starting one goroutine
	// per move for a bar that is on screen flooded the scheduler and made
	// the whole desktop stutter.
	if moved && OnMouseMove != nil && atomic.LoadUint32(&chipHoverArmed) != 0 {
		dispatchMouseMove(moveX, moveY)
	}

	// Everything slow happens in the goroutine, never here. A low-level hook
	// runs on the input thread and the whole desktop waits on it: OpenProcess
	// and QueryFullProcessImageName can take milliseconds, which is how a
	// "fast" translator ends up introducing input lag everywhere.
	if clickFire && OnSelectionGesture != nil {
		g := selectionGesture{X: cx, Y: cy, Kind: kind, LastKey: lastKey, PressedAt: pressedAt}
		hwnd := target
		go func() {
			defer recoverLog("hook.go:lowLevelMouseProc")
			settleAfterMouseUp()
			g.Owner = processNameOf(hwnd)
			OnSelectionGesture(g)
		}()
	}

	if fire && OnSelectionGesture != nil {
		// The button is still logically down here: the release reaches the
		// application after this hook returns. Let the selection settle
		// before asking for it.
		g := selectionGesture{X: fx, Y: fy, Kind: kind, LastKey: lastKey, PressedAt: pressedAt}
		hwnd := target
		go func() {
			defer recoverLog("hook.go:lowLevelMouseProc")
			settleAfterMouseUp()
			g.Owner = processNameOf(hwnd)
			OnSelectionGesture(g)
		}()
	}
	r, _, _ := procCallNextHookEx.Call(liveMouseHook, uintptr(nCode), wParam, lParam)
	return r
}

// lowLevelKeyProc is the WH_KEYBOARD_LL callback. ONLY Shift plus an arrow key
// extends a selection without the mouse; the gesture fires on key up.
//
// It also keeps the clock the typing guard reads (lastKeyAt) and tells the
// chip when the user starts typing, so a chip left over a text field goes
// away instead of sitting on top of what the user is writing.
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
			} else if !isModifierKey(info.VkCode) {
				// Any key that types restarts the typing guard's clock. A
				// modifier on its own does not: holding Shift or Ctrl before
				// dragging to select is not typing, and counting it would
				// swallow the very gesture the user is about to make.
				now := time.Now()
				hookMu.Lock()
				lastKeyAt = now
				// Typing ends any Shift+arrow selection: the up event of the
				// arrow would otherwise still report the text the user has
				// already navigated away from.
				keyboardSelecting = false
				hookMu.Unlock()
				if OnKeyDown != nil {
					vk := info.VkCode
					go OnKeyDown(vk)
				}
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
				g := selectionGesture{Kind: gestureKeyboard, PressedAt: time.Now()}
				hwnd := target
				go func() {
					defer recoverLog("hook.go:lowLevelKeyProc")
					settleAfterMouseUp()
					p := cursorPoint()
					g.X, g.Y = p.X, p.Y
					g.Owner = processNameOf(hwnd)
					OnSelectionGesture(g)
				}()
			}
		}
	}
	r, _, _ := procCallNextHookEx.Call(liveKeyHook, uintptr(nCode), wParam, lParam)
	return r
}

// isModifierKey reports whether vk is a modifier, which never types a
// character on its own.
func isModifierKey(vk uint32) bool {
	for _, m := range modifierKeys {
		if uint32(m) == vk {
			return true
		}
	}
	return false
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

	// Installing twice would leak the first pair of handles and deliver every
	// event twice. The guard above (mouseHook != 0) is what makes this
	// idempotent; applySelectionMode calls it on every settings change.

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
	log.Printf("fastDictionary: selection hooks installed on thread %d (mouse=%d key=%d)",
		currentThreadID(), h, k)
	return nil
}

// chipHoverArmed gates the pointer-move callback. It is set while the
// selection chip is on screen and clear otherwise, so the hook can skip the
// dispatch without taking a lock or starting a goroutine.
var chipHoverArmed uint32

// armChipHover lets pointer moves reach OnMouseMove.
func armChipHover() { atomic.StoreUint32(&chipHoverArmed, 1) }

// moveWake carries at most one pending wake-up for the pointer-move worker;
// latestMove keeps the coordinates of the newest event. Together they turn a
// stream of moves into "the pointer is here now", which is all a hover needs.
var (
	moveWake   = make(chan struct{}, 1)
	moveWorker sync.Once
	latestMove atomic.Uint64 // high 32 bits: X, low 32 bits: Y
)

// dispatchMouseMove hands the newest pointer position to one long-lived
// worker goroutine. The hook callback must stay cheap, and one goroutine per
// move made the scheduler the bottleneck whenever a selection chip was on
// screen.
func dispatchMouseMove(x, y int32) {
	moveWorker.Do(func() {
		go func() {
			for range moveWake {
				// Recover per move, not around the whole loop: a panic in the
				// hover handler must cost one event, not every later one.
				func() {
					defer recoverLog("hook.go:moveWorker")
					v := latestMove.Load()
					if fn := OnMouseMove; fn != nil && atomic.LoadUint32(&chipHoverArmed) != 0 {
						fn(int32(uint32(v>>32)), int32(uint32(v)))
					}
				}()
			}
		}()
	})
	latestMove.Store(uint64(uint32(x))<<32 | uint64(uint32(y)))
	select {
	case moveWake <- struct{}{}:
	default: // a wake-up is already pending; it will read the newest position
	}
}

// disarmChipHover stops pointer moves from reaching OnMouseMove.
func disarmChipHover() {
	atomic.StoreUint32(&chipHoverArmed, 0)
}

// resetClickTrackingLocked 在手势会话结束时清空双击候选。调用方必须持有 hookMu：
// lowLevelMouseProc 在同一把锁下读写这三个字段。卸载钩子表示会话结束，旧的
// lastClickTime / lastClickPos / clickCount 不能带进下一次安装，否则重装后短时间
// 内的首次点击可能沿用旧候选被误判为双击。
func resetClickTrackingLocked() {
	lastClickTime = 0
	lastClickPos = winPoint{}
	clickCount = 0
	dblClickPending = false
}

// removeHooks uninstalls both hooks. Unhooking is thread-agnostic in win32,
// but it is still done from the hook thread so the loop can exit cleanly.
//
// It is idempotent: removing hooks that were never installed is a no-op, which
// is what lets applySelectionMode be called freely on every settings change.
func removeHooks() {
	hookMu.Lock()
	mh, kh := mouseHook, keyHook
	mouseHook, keyHook = 0, 0
	liveMouseHook, liveKeyHook = 0, 0
	resetClickTrackingLocked()
	hookMu.Unlock()
	if mh == 0 && kh == 0 {
		return
	}
	if mh != 0 {
		procUnhookWindowsHook.Call(mh)
	}
	if kh != 0 {
		procUnhookWindowsHook.Call(kh)
	}
	log.Printf("fastDictionary: selection hooks removed")
}

// hookThread runs the selection hooks on a thread of its own that pumps
// messages, which is what a low-level hook needs in order to be delivered.
//
// It replaces the old "install from mygo's main thread" approach: a hook
// installed on a thread that is not pumping is never called.
var (
	hookThreadOnce sync.Once
	hookLoopID     uintptr // thread id, for PostThreadMessage

	// hookReadyCh is closed by hookLoop as soon as the thread has recorded its
	// id and is about to pump messages. Posting a command before that point
	// addresses a thread that does not exist yet, so the work is lost.
	hookReadyCh   = make(chan struct{})
	hookReadyOnce sync.Once

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

// waitHookThreadReady starts the hook thread if needed and waits until its
// message loop is up. It returns false when the thread did not get there in
// time, which is the caller's cue to skip the command rather than to install
// the hooks itself: installing from a goroutine with no message loop gives a
// hook that is installed and never fires.
func waitHookThreadReady(timeout time.Duration) bool {
	startHookThread()
	select {
	case <-hookReadyCh:
		return true
	case <-time.After(timeout):
		return false
	}
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

// hookPolicyRejects reports how many drags the selection policy dropped and
// why the last one was dropped, for the --hookstats reporter.
func hookPolicyRejects() (uint64, string) {
	why := ""
	if p := hookRejectedDragWhy.Load(); p != nil {
		why = *p
	}
	return atomic.LoadUint64(&hookRejectedDrags), why
}

// hookOwnRejects reports what the hook last mistook for a fastDictionary window.
func hookOwnRejects() (title, class string) {
	if p := hookRejectedTitle.Load(); p != nil {
		title = *p
	}
	if p := hookRejectedClass.Load(); p != nil {
		class = *p
	}
	return title, class
}

// wmHookCommand is the private window message used to run hook work on the
// hook thread. WM_APP is the first message id applications may use for
// themselves; anything above it is free.
const wmHookCommand = 0x8000 + 1

// hookCommand carries the work posted to the hook thread.
var (
	// hookCommands is a bounded queue: the hook thread may be busy in a
	// callback when another mode switch asks for install/remove, and a single
	// slot would silently drop the earlier request.
	hookCommands           = make(chan func(), 16)
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
	select {
	case hookCommands <- fn:
	default:
		log.Printf("fastDictionary: hook command queue full, dropping request")
		return fmt.Errorf("hook command queue full")
	}
	if r, _, err := procPostThreadMessageW.Call(tid, wmHookCommand, 0, 0); r == 0 {
		return fmt.Errorf("PostThreadMessage: %w", err)
	}
	return nil
}

// runHookCommand executes every queued command. It runs on the hook thread.
// Draining the whole queue keeps the order the requests were made in.
func runHookCommand() {
	for {
		select {
		case fn := <-hookCommands:
			if fn != nil {
				// Recover per command: the hook thread is the only path a
				// selection takes into the app, so a panic here used to end
				// the process with nothing in the log.
				func() {
					defer recoverLog("hook.go:runHookCommand")
					fn()
				}()
			}
		default:
			return
		}
	}
}

func hookLoop() {
	// Fallback only: without it a panic that escapes the loop ends the process
	// with nothing in the log. The loop itself must not die on one message,
	// hence dispatchMessage's own guard below.
	defer recoverLog("hook.go:hookLoop")

	// A low-level hook callback is only delivered to a thread running a
	// message loop, and Go moves goroutines between threads, so pin this one.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hookMu.Lock()
	hookLoopID = currentThreadID()
	hookMu.Unlock()
	hookReadyOnce.Do(func() { close(hookReadyCh) })

	if err := installHooks(); err != nil {
		log.Printf("fastDictionary: selection hooks unavailable: %v", err)
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
		dispatchMessage(&m)
	}
}

// dispatchMessage hands one queued message to the window procedures that read it.
//
// Recovered per message, not around the loop: the hook thread is the only path a
// selection takes into the app, so one bad message must cost one message. Letting
// it reach the loop would end the thread and the trigger would stop working until
// the app is restarted.
func dispatchMessage(m *msgW) {
	defer recoverLog("hook.go:dispatchMessage")
	procTranslateMessage.Call(uintptr(unsafe.Pointer(m)))
	procDispatchMessageW.Call(uintptr(unsafe.Pointer(m)))
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
