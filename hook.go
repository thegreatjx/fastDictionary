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

	hookEvents          uint64
	hookDowns           uint64
	hookUps             uint64
	hookDrags           uint64
	hookDragsOwn        uint64
	hookRejectedDrags   uint64
	hookRejectedDragWhy atomic.Pointer[string]
	hookRejectedTitle   atomic.Pointer[string]
	hookRejectedClass   atomic.Pointer[string]
	liveMouseHook       uintptr
	liveKeyHook         uintptr
	hookOnce            sync.Once
	mouseHook           uintptr
	keyHook             uintptr
	mouseProc           uintptr
	keyProc             uintptr
	hookMu              sync.Mutex
	dragging            bool
	dragStart           winPoint
	dragEnd             winPoint
	hookTarget          uintptr
	lastClickTime       uint32
	lastClickPos        winPoint
	clickCount          int
	dblClickPending     bool
	pressTime           uint32
	pressAt             time.Time
	pressLastKey        time.Time
	lastKeyAt           time.Time
	OnSelectionGesture  func(g selectionGesture)
	OnMouseDown         func(x, y int32)
	OnMouseWheel        func(x, y int32)
	OnMouseMove         func(x, y int32)
	// 钩子回调在系统线程上，不能阻塞；耗时工作丢到 goroutine。
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
	wmMouseWheel  = 0x020A
	wmMouseHWheel = 0x020E

	wmKeyDown    = 0x0100
	wmSysKeyDown = 0x0104

	doubleClickSlack = 6

	vkLeft  = 0x25
	vkUp    = 0x26
	vkRight = 0x27
	vkDown  = 0x28
	llkhfUp = 0x80
)

type msllHookStruct struct {
	Pt          winPoint
	MouseData   uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

type kbdllHookStruct struct {
	VkCode      uint32
	ScanCode    uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

func lowLevelMouseProc(nCode int, wParam uintptr, lParam uintptr) uintptr {
	if nCode < 0 {
		r, _, _ := procCallNextHookEx.Call(liveMouseHook, uintptr(nCode), wParam, lParam)
		return r
	}
	info := (*msllHookStruct)(nativePtr(lParam))
	pt := info.Pt

	atomic.AddUint64(&hookEvents, 1)

	hookMu.Lock()
	var fire, press, clickFire, moved, wheel bool
	var fx, fy, cx, cy, moveX, moveY, wx, wy int32
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
		pressAt = time.Now()
		pressLastKey = lastKeyAt
		press = true
		if info.Time-lastClickTime <= doubleClickThreshold() &&
			absInt(int(pt.X-lastClickPos.X)) <= doubleClickSlack &&
			absInt(int(pt.Y-lastClickPos.Y)) <= doubleClickSlack {
			clickCount++
		} else {
			clickCount = 1
		}
		lastClickTime = info.Time
		lastClickPos = pt
		dblClickPending = clickCount == 2 || clickCount == 3
	case wmMouseMove:
		if dragging {
			dragEnd = pt
		}
		moveX, moveY = pt.X, pt.Y
		moved = true
	case wmMouseWheel, wmMouseHWheel:
		if atomic.LoadUint32(&chipHoverArmed) != 0 {
			wx, wy = pt.X, pt.Y
			wheel = true
		}
	case wmLButtonUp:
		atomic.AddUint64(&hookUps, 1)
		wasDbl := dblClickPending
		dblClickPending = false
		if dragging {
			dragging = false
			held := time.Duration(info.Time-pressTime) * time.Millisecond
			if wasDbl && absInt(int(pt.X-dragStart.X)) <= doubleClickSlack &&
				absInt(int(pt.Y-dragStart.Y)) <= doubleClickSlack {
				if hwnd := windowAt(pt.X, pt.Y); !isOwnProcess(hwnd) {
					clickFire = true
					kind = gestureDoubleClick
					if clickCount >= 3 {
						kind = gestureTripleClick
					}
					lastKey = pressLastKey
					pressedAt = pressAt
					cx, cy = pt.X, pt.Y
					target = hwnd
				} else {
					atomic.AddUint64(&hookDragsOwn, 1)
				}
			} else if ok, why := dragIsSelection(dragStart, dragEnd, held); !ok {
				atomic.AddUint64(&hookRejectedDrags, 1)
				hookRejectedDragWhy.Store(&why)
			} else {
				atomic.AddUint64(&hookDrags, 1)
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
		dispatchMouseDown(x, y)
	}

	if wheel && OnMouseWheel != nil {
		x, y := wx, wy
		dispatchMouseWheel(x, y)
	}

	if moved && OnMouseMove != nil && atomic.LoadUint32(&chipHoverArmed) != 0 {
		dispatchMouseMove(moveX, moveY)
	}

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

func lowLevelKeyProc(nCode int, wParam uintptr, lParam uintptr) uintptr {
	if nCode >= 0 {
		info := (*kbdllHookStruct)(nativePtr(lParam))
		msg := uint32(wParam)
		if msg == wmKeyDown || msg == wmSysKeyDown {
			isArrow := info.VkCode == vkLeft || info.VkCode == vkUp ||
				info.VkCode == vkRight || info.VkCode == vkDown
			shiftHeld := keyDown(vkShift) || keyDown(vkLShift) || keyDown(vkRShift)
			if isArrow && shiftHeld {
				hookMu.Lock()
				keyboardSelecting = true
				hookTarget = foregroundWindow()
				hookMu.Unlock()
			} else if !isModifierKey(info.VkCode) {
				now := time.Now()
				hookMu.Lock()
				lastKeyAt = now
				keyboardSelecting = false
				hookMu.Unlock()
				if OnKeyDown != nil {
					vk := info.VkCode
					dispatchKeyDown(vk)
				}
			}
		} else if info.Flags&llkhfUp != 0 {
			hookMu.Lock()
			wasSelecting := keyboardSelecting
			target := hookTarget
			keyboardSelecting = false
			hookMu.Unlock()
			if wasSelecting && !isOwnProcess(target) && OnSelectionGesture != nil {
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

func isModifierKey(vk uint32) bool {
	for _, m := range modifierKeys {
		if uint32(m) == vk {
			return true
		}
	}
	return false
}

var keyboardSelecting bool

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func settleAfterMouseUp() {
	sleepMS(60)
}

func installHooks() error {
	hookMu.Lock()
	already := mouseHook != 0
	hookMu.Unlock()
	if already {
		return nil
	}

	if atomic.LoadUint32(&doubleClickMSLive) == 0 {
		if ms, _, _ := procGetDoubleClickTime.Call(); ms > 0 {
			atomic.StoreUint32(&doubleClickMSLive, uint32(ms))
		} else {
			atomic.StoreUint32(&doubleClickMSLive, 500)
		}
	}

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
	log.Printf("fastDictionary: selection hooks installed on thread %d (mouse=%d key=%d)",
		currentThreadID(), h, k)
	return nil
}

var chipHoverArmed uint32

func armChipHover() { atomic.StoreUint32(&chipHoverArmed, 1) }

var (
	moveWake   = make(chan struct{}, 1)
	moveWorker sync.Once
	latestMove atomic.Uint64
)

func dispatchMouseMove(x, y int32) {
	moveWorker.Do(func() {
		go func() {
			for range moveWake {
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
	default:
	}
}

var (
	wheelWake   = make(chan struct{}, 1)
	wheelWorker sync.Once
	latestWheel atomic.Uint64

	keyWake   = make(chan struct{}, 1)
	keyWorker sync.Once
	latestKey atomic.Uint32
)

func dispatchMouseDown(x, y int32) {
	fn := OnMouseDown
	if fn == nil {
		return
	}
	go func() {
		defer recoverLog("hook.go:mouseDown")
		fn(x, y)
	}()
}

func dispatchMouseWheel(x, y int32) {
	wheelWorker.Do(func() {
		go func() {
			for range wheelWake {
				func() {
					defer recoverLog("hook.go:wheelWorker")
					v := latestWheel.Load()
					if fn := OnMouseWheel; fn != nil {
						fn(int32(uint32(v>>32)), int32(uint32(v)))
					}
				}()
			}
		}()
	})
	latestWheel.Store(uint64(uint32(x))<<32 | uint64(uint32(y)))
	select {
	case wheelWake <- struct{}{}:
	default:
	}
}

func dispatchKeyDown(vk uint32) {
	keyWorker.Do(func() {
		go func() {
			for range keyWake {
				func() {
					defer recoverLog("hook.go:keyWorker")
					k := latestKey.Load()
					if fn := OnKeyDown; fn != nil {
						fn(k)
					}
				}()
			}
		}()
	})
	latestKey.Store(vk)
	select {
	case keyWake <- struct{}{}:
	default:
	}
}

func disarmChipHover() {
	atomic.StoreUint32(&chipHoverArmed, 0)
}

func resetClickTrackingLocked() {
	lastClickTime = 0
	lastClickPos = winPoint{}
	clickCount = 0
	dblClickPending = false
}

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

var (
	hookThreadOnce sync.Once
	hookLoopID     uintptr

	hookReadyCh   = make(chan struct{})
	hookReadyOnce sync.Once

	doubleClickMSLive uint32
)

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

func waitHookThreadReady(timeout time.Duration) bool {
	startHookThread()
	select {
	case <-hookReadyCh:
		return true
	case <-time.After(timeout):
		return false
	}
}

func hookEventCount() uint64 { return atomic.LoadUint64(&hookEvents) }

func hookStats() (events, downs, ups, drags, own uint64) {
	return atomic.LoadUint64(&hookEvents), atomic.LoadUint64(&hookDowns),
		atomic.LoadUint64(&hookUps), atomic.LoadUint64(&hookDrags),
		atomic.LoadUint64(&hookDragsOwn)
}

func hookPolicyRejects() (uint64, string) {
	why := ""
	if p := hookRejectedDragWhy.Load(); p != nil {
		why = *p
	}
	return atomic.LoadUint64(&hookRejectedDrags), why
}

func hookOwnRejects() (title, class string) {
	if p := hookRejectedTitle.Load(); p != nil {
		title = *p
	}
	if p := hookRejectedClass.Load(); p != nil {
		class = *p
	}
	return title, class
}

const wmHookCommand = 0x8000 + 1

var (
	hookCommands           = make(chan func(), 16)
	procPostThreadMessageW = user32.NewProc("PostThreadMessageW")
)

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

func runHookCommand() {
	for {
		select {
		case fn := <-hookCommands:
			if fn != nil {
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
	defer recoverLog("hook.go:hookLoop")

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
		if int32(r) <= 0 {
			return
		}
		if m.Message == wmHookCommand {
			runHookCommand()
			continue
		}
		dispatchMessage(&m)
	}
}

func dispatchMessage(m *msgW) {
	defer recoverLog("hook.go:dispatchMessage")
	procTranslateMessage.Call(uintptr(unsafe.Pointer(m)))
	procDispatchMessageW.Call(uintptr(unsafe.Pointer(m)))
}

func runOnMainThread() {
	startHookThread()
}

func currentThreadID() uintptr {
	id, _, _ := procGetCurrentThread.Call()
	return id
}
