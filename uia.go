// Windows UI Automation selection capture.
//
// This is the primary way fastDictionary reads what the user selected: the
// IUIAutomationTextPattern of the focused element answers GetSelection()
// with the selected text, without touching the clipboard and without
// synthesising any input. It works in every app that exposes UI Automation
// text (browsers, editors, Office, WinUI, WPF, and legacy edit controls
// through the MSAA bridge), and it leaves the user's clipboard alone.
//
// The interfaces are called directly through their COM vtables with
// syscall.SyscallN, so no cgo and no extra dependency is needed. Vtable
// slots come from the Windows SDK's uiautomationclient.h, mirrored by
// Wine's include/uiautomationclient.idl (a copy of which is kept in
// .research/uiautomationclient.idl). Every slot below is listed with the
// methods that precede it so the arithmetic stays checkable: slots 0, 1
// and 2 of every interface are IUnknown's QueryInterface, AddRef and
// Release, and the interface's own first method is slot 3.
package main

import (
	"errors"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// ---------------------------------------------------------------------------
// COM plumbing
// ---------------------------------------------------------------------------

type comGUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

var (
	clsidCUIAutomation = comGUID{0xff48dba4, 0x60ef, 0x4201, [8]byte{0xaa, 0x87, 0x54, 0x10, 0x3e, 0xef, 0x59, 0x4e}}
	iidIUIAutomation   = comGUID{0x30cbe57d, 0xd9d0, 0x452a, [8]byte{0xab, 0x13, 0x7a, 0xc5, 0xac, 0x48, 0x25, 0xee}}

	ole32                = syscall.NewLazyDLL("ole32.dll")
	oleaut32             = syscall.NewLazyDLL("oleaut32.dll")
	procCoInitializeEx   = ole32.NewProc("CoInitializeEx")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")
	procCoUninitialize   = ole32.NewProc("CoUninitialize")
	procSysFreeString    = oleaut32.NewProc("SysFreeString")
	procVariantClear     = oleaut32.NewProc("VariantClear")

	procSafeArrayGetLBound  = oleaut32.NewProc("SafeArrayGetLBound")
	procSafeArrayGetUBound  = oleaut32.NewProc("SafeArrayGetUBound")
	procSafeArrayGetElement = oleaut32.NewProc("SafeArrayGetElement")
	procSafeArrayDestroy    = oleaut32.NewProc("SafeArrayDestroy")
)

const (
	coinitMultithreaded = 0x0
	clsctxInprocServer  = 0x1
	rpcEChangedMode     = 0x80010106
)

// comCall invokes the method in vtable slot of the COM interface at this.
// slot counts from the start of the vtable, so 0 is QueryInterface, 1
// AddRef, 2 Release, and 3 the interface's own first method.
func comCall(this uintptr, slot int, args ...uintptr) uintptr {
	vtbl := *(*uintptr)(nativePtr(this))
	fn := *(*uintptr)(nativePtr(vtbl + uintptr(slot)*unsafe.Sizeof(uintptr(0))))
	callArgs := make([]uintptr, 0, len(args)+1)
	callArgs = append(callArgs, this)
	callArgs = append(callArgs, args...)
	ret, _, _ := syscall.SyscallN(fn, callArgs...)
	return ret
}

func comRelease(this uintptr) {
	if this != 0 {
		comCall(this, 2) // IUnknown::Release
	}
}

// bstrToString copies a BSTR into a Go string. A BSTR stores its byte
// length in the four bytes before the pointer and is NUL terminated.
func bstrToString(b uintptr) string {
	if b == 0 {
		return ""
	}
	n := *(*uint32)(nativePtr(b - 4))
	if n == 0 {
		return ""
	}
	return syscall.UTF16ToString(unsafe.Slice((*uint16)(nativePtr(b)), n/2))
}

// variant is the 24-byte VARIANT of the Windows ABI: an 8-byte header
// (VARTYPE and three reserved words) and a 16-byte union.
type variant struct {
	VT  uint16
	_   [6]byte
	Val uint64
	_   uint64
}

const (
	vtI4   = 3
	vtBstr = 8
	vtBool = 11
)

func (v *variant) clear() {
	procVariantClear.Call(uintptr(unsafe.Pointer(v)))
}

// comApt runs fn on a thread initialized for COM multi-threaded apartment
// use. UI Automation clients are apartment-bound, so the goroutine is
// pinned for the duration.
func comApt(fn func() error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hr, _, _ := procCoInitializeEx.Call(0, coinitMultithreaded)
	// RPC_E_CHANGED_MODE means COM is already up with another apartment
	// model on this thread: usable, but not ours to uninitialize.
	owned := uint32(hr) == 0
	defer func() {
		if owned {
			procCoUninitialize.Call()
		}
	}()
	return fn()
}

// ---------------------------------------------------------------------------
// UI Automation constants (uiautomationclient.h)
// ---------------------------------------------------------------------------

const (
	uiaTextPatternID        = 10014 // UIA_TextPatternId
	uiaIsPasswordPropertyID = 30019 // UIA_IsPasswordPropertyId

	// TreeScope_Subtree: the element and everything under it.
	treeScopeSubtree = 7
)

// Vtable slots, counted from the start of the vtable.
//
// IUIAutomation (after IUnknown): CompareElements 3, CompareRuntimeIds 4,
// GetRootElement 5, ElementFromHandle 6, ElementFromPoint 7,
// GetFocusedElement 8.
//
// IUIAutomationElement (after IUnknown): SetFocus 3, GetRuntimeId 4,
// FindFirst 5, FindAll 6, FindFirstBuildCache 7, FindAllBuildCache 8,
// BuildUpdatedCache 9, GetCurrentPropertyValue 10,
// GetCurrentPropertyValueEx 11, GetCachedPropertyValue 12,
// GetCachedPropertyValueEx 13, GetCurrentPatternAs 14,
// GetCachedPatternAs 15, GetCurrentPattern 16, GetCachedPattern 17,
// GetCachedParent 18, GetCachedChildren 19, CurrentProcessId 20.
//
// IUIAutomationTextPattern (after IUnknown): RangeFromPoint 3,
// RangeFromChild 4, GetSelection 5, GetVisibleRanges 6, DocumentRange 7,
// SupportedTextSelection 8.
//
// IUIAutomationTextRange (after IUnknown): Clone 3, Compare 4,
// CompareEndpoints 5, ExpandToEnclosingUnit 6, FindAttribute 7, FindText 8,
// GetAttributeValue 9, GetBoundingRectangles 10, GetEnclosingElement 11,
// GetText 12.
//
// IUIAutomationTextRangeArray (after IUnknown): Length 3, GetElement 4.
const (
	slotUiaElementFromHandle   = 6
	slotUiaElementFromPoint    = 7
	slotUiaGetFocusedElement   = 8
	slotUiaCreateTrueCondition = 21

	slotElementFindAll                 = 6
	slotElementGetCurrentPropertyValue = 10
	slotElementGetCurrentPattern       = 16
	slotElementGetCachedParent         = 18
	slotElementGetCachedChildren       = 19
	slotElementCurrentProcessID        = 20

	slotTextPatternGetSelection = 5

	slotTextRangeGetBoundingRects = 10
	slotTextRangeGetText          = 12

	slotRangeArrayLength     = 3
	slotRangeArrayGetElement = 4
)

// ErrNoUIA means UI Automation had nothing to say: no focused element, no
// text pattern on it, or no selection in it.
var ErrNoUIA = errors.New("uia: no selection")

// ---------------------------------------------------------------------------
// Selection capture
// ---------------------------------------------------------------------------

// pointArg packs a POINT for a call taking it by value. On the x64 ABI an
// eight-byte struct of two LONGs travels in one integer register.
func pointArg(x, y int32) uintptr {
	return uintptr(uint64(uint32(x)) | uint64(uint32(y))<<32)
}

// uiaRect is the UIA Rect of a text range, in physical screen pixels.
type uiaRect struct{ Left, Top, Right, Bottom float64 }

// selection is what UI Automation reports about the current selection: its
// text and, when the provider gives one, where it is on screen.
type selection struct {
	Text string
	// Rect is the bounding rectangle of the selected text in physical
	// screen pixels. Bounds is false when the provider gave none, which
	// some controls do.
	Rect   uiaRect
	Bounds bool
}

// selectionFromElement reads the selection of a UIA element, or ErrNoUIA
// when the element has no text pattern or nothing is selected.
func selectionFromElement(el uintptr) (selection, error) {
	var out selection
	if el == 0 {
		return out, ErrNoUIA
	}

	// Never capture passwords, whatever pattern the control exposes.
	var pw variant
	if hr := comCall(el, slotElementGetCurrentPropertyValue,
		uiaIsPasswordPropertyID, uintptr(unsafe.Pointer(&pw))); hr == 0 {
		isPassword := pw.VT == vtBool && int16(pw.Val) != 0
		pw.clear()
		if isPassword {
			return out, ErrNoUIA
		}
	}

	var pattern uintptr
	if hr := comCall(el, slotElementGetCurrentPattern,
		uiaTextPatternID, uintptr(unsafe.Pointer(&pattern))); hr != 0 || pattern == 0 {
		return out, ErrNoUIA
	}
	defer comRelease(pattern)

	var ranges uintptr
	if hr := comCall(pattern, slotTextPatternGetSelection,
		uintptr(unsafe.Pointer(&ranges))); hr != 0 || ranges == 0 {
		return out, ErrNoUIA
	}
	defer comRelease(ranges)

	length := int32(comCall(ranges, slotRangeArrayLength))
	if length <= 0 {
		return out, ErrNoUIA
	}

	var text string
	for i := int32(0); i < length; i++ {
		var rng uintptr
		if hr := comCall(ranges, slotRangeArrayGetElement,
			uintptr(i), uintptr(unsafe.Pointer(&rng))); hr != 0 || rng == 0 {
			continue
		}
		var bstr uintptr
		// -1 asks for the whole range.
		if hr := comCall(rng, slotTextRangeGetText, ^uintptr(0), uintptr(unsafe.Pointer(&bstr))); hr == 0 && bstr != 0 {
			text += bstrToString(bstr)
			procSysFreeString.Call(bstr)
		}
		// Where the selection is: the provider may give several rectangles
		// for a selection spanning lines, and the last one is where it
		// ended, which is the natural place to put the chip.
		if r, ok := rangeBounds(rng); ok {
			out.Rect, out.Bounds = r, true
		}
		comRelease(rng)
		if text != "" {
			break // the first non-empty range is the user's selection
		}
	}
	if text == "" {
		return out, ErrNoUIA
	}
	out.Text = text
	return out, nil
}

// rangeBounds reads a text range's bounding rectangles. A selection across
// several lines answers with one rectangle per line; the last is the end of
// the selection. It returns the union of the last line's rectangles, or
// false when the provider gave none.
func rangeBounds(rng uintptr) (uiaRect, bool) {
	var sa uintptr
	// GetBoundingRectangles returns a SAFEARRAY of doubles: left, top,
	// width, height per rectangle, in physical screen pixels.
	if hr := comCall(rng, slotTextRangeGetBoundingRects, uintptr(unsafe.Pointer(&sa))); hr != 0 || sa == 0 {
		return uiaRect{}, false
	}
	defer procSafeArrayDestroy.Call(sa)

	var lower, upper int32
	procSafeArrayGetLBound.Call(sa, 1, uintptr(unsafe.Pointer(&lower)))
	procSafeArrayGetUBound.Call(sa, 1, uintptr(unsafe.Pointer(&upper)))
	n := int(upper-lower) + 1
	if n < 4 {
		return uiaRect{}, false
	}

	val := func(i int) (float64, bool) {
		var v float64
		idx := int32(i + int(lower))
		if hr, _, _ := procSafeArrayGetElement.Call(sa, uintptr(unsafe.Pointer(&idx)), uintptr(unsafe.Pointer(&v))); hr != 0 {
			return 0, false
		}
		return v, true
	}

	// The last rectangle is the end of the selection; grow it to cover the
	// rectangles on the same line so a wide selection anchors correctly.
	last := n/4 - 1
	l, ok1 := val(last * 4)
	tp, ok2 := val(last*4 + 1)
	w, ok3 := val(last*4 + 2)
	h, ok4 := val(last*4 + 3)
	if !ok1 || !ok2 || !ok3 || !ok4 {
		return uiaRect{}, false
	}
	r := uiaRect{Left: l, Top: tp, Right: l + w, Bottom: tp + h}
	for i := 0; i < last; i++ {
		rl, ok1 := val(i * 4)
		rt, ok2 := val(i*4 + 1)
		rw, ok3 := val(i*4 + 2)
		rh, ok4 := val(i*4 + 3)
		if !ok1 || !ok2 || !ok3 || !ok4 {
			continue
		}
		// Same visual line (vertical overlap): extend to the right.
		if rt < r.Bottom && rt+rh > r.Top {
			if rl < r.Left {
				r.Left = rl
			}
			if rl+rw > r.Right {
				r.Right = rl + rw
			}
			if rt < r.Top {
				r.Top = rt
			}
			if rt+rh > r.Bottom {
				r.Bottom = rt + rh
			}
		}
	}
	if r.Right <= r.Left || r.Bottom <= r.Top {
		return uiaRect{}, false
	}
	return r, true
}

// uiaWorker is one thread with its own COM apartment and its own
// IUIAutomation instance, plus the private channel that feeds it.
//
// One worker serves every gesture: creating a COM instance costs tens of
// milliseconds, and doing it per gesture also leaked a locked OS thread
// whenever a provider blocked inside GetSelection and the caller gave up.
type uiaWorker struct {
	jobs   chan uiaJob
	stopCh chan struct{}
	// doneCh is closed by the worker on the way out: an exited worker can no
	// longer answer, and the caller would otherwise sit out the whole timeout
	// waiting for a reply that cannot come.
	doneCh chan struct{}
	// retired is set under uiaMu when this worker timed out and was taken out
	// of service while still blocked inside COM.
	retired bool
}

type uiaResult struct {
	sel selection
	err error
}

type uiaJob struct {
	done chan uiaResult
}

const (
	// uiaWorkerCooldown is how long UIA is left alone after a provider
	// blocked past uiaTimeout. Long enough that a page which hangs every
	// lookup does not buy one leaked thread per gesture, short enough that
	// switching back to a normal app heals on its own.
	uiaWorkerCooldown = 8 * time.Second
	// uiaMaxHungWorkers caps the threads a misbehaving provider can pin.
	// Only a provider that never returns keeps its thread; a slow one frees
	// it as soon as the call comes back.
	uiaMaxHungWorkers = 4
)

var (
	uiaMu          sync.Mutex
	uiaActive      *uiaWorker
	uiaBusy        bool
	uiaLastTimeout time.Time
	uiaHungWorkers int
)

// newUIAWorker starts one worker thread and returns its handle.
func newUIAWorker() *uiaWorker {
	w := &uiaWorker{
		jobs:   make(chan uiaJob),
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
	go func() {
		// LIFO: put the shared state back, then tell the waiters this worker
		// is gone, and only then swallow a panic.
		defer recoverLog("uia.go:worker")
		defer close(w.doneCh)
		defer func() {
			uiaMu.Lock()
			if uiaActive == w {
				uiaActive = nil
				uiaBusy = false
			}
			if w.retired && uiaHungWorkers > 0 {
				uiaHungWorkers--
			}
			uiaMu.Unlock()
		}()
		_ = comApt(func() error {
			var uia uintptr
			ensureUIA := func() uintptr {
				if uia != 0 {
					return uia
				}
				hr, _, _ := procCoCreateInstance.Call(
					uintptr(unsafe.Pointer(&clsidCUIAutomation)), 0, clsctxInprocServer,
					uintptr(unsafe.Pointer(&iidIUIAutomation)), uintptr(unsafe.Pointer(&uia)))
				if hr != 0 || uia == 0 {
					uia = 0
					return 0
				}
				return uia
			}
			for {
				select {
				case <-w.stopCh:
					if uia != 0 {
						comRelease(uia)
					}
					return nil
				case job := <-w.jobs:
					inst := ensureUIA()
					if inst == 0 {
						job.done <- uiaResult{err: ErrNoUIA}
						continue
					}
					sel, err := queryUIAOnce(inst)
					job.done <- uiaResult{sel: sel, err: err}
				}
			}
		})
	}()
	return w
}

func queryUIAOnce(uia uintptr) (selection, error) {
	// The element with the keyboard focus owns the selection.
	var el uintptr
	if hr := comCall(uia, slotUiaGetFocusedElement, uintptr(unsafe.Pointer(&el))); hr != 0 {
		el = 0
	}
	if el != 0 && int32(comCall(el, slotElementCurrentProcessID)) == int32(currentProcessID()) {
		comRelease(el) // our own popup is a UIA provider too
		el = 0
	}
	if el != 0 {
		s, err := selectionFromElement(el)
		comRelease(el)
		if err == nil {
			return s, nil
		}
	}

	// Some controls answer only about the element under the pointer.
	// During a selection drag that is the text the user just selected.
	pt := cursorPoint()
	var under uintptr
	if hr := comCall(uia, slotUiaElementFromPoint, pointArg(pt.X, pt.Y), uintptr(unsafe.Pointer(&under))); hr == 0 && under != 0 {
		if int32(comCall(under, slotElementCurrentProcessID)) != int32(currentProcessID()) {
			s, err := selectionFromElement(under)
			comRelease(under)
			if err == nil {
				return s, nil
			}
		} else {
			comRelease(under)
		}
	}
	return selection{}, ErrNoUIA
}

// retireUIAWorker takes a worker out of service after it failed to answer in
// time. It is closed through stopCh rather than by closing jobs: the caller
// may still be the sender, and a send on a closed channel panics.
func retireUIAWorker(w *uiaWorker) {
	uiaMu.Lock()
	if uiaActive == w {
		uiaActive = nil
		uiaBusy = false
		uiaLastTimeout = time.Now()
		uiaHungWorkers++
		w.retired = true
		close(w.stopCh)
	}
	uiaMu.Unlock()
}

// SelectionUIA returns what UI Automation knows about the selection in the
// foreground application: its text, and where it is on screen when the
// provider reports a bounding rectangle.
func SelectionUIA() (selection, error) {
	now := time.Now()
	uiaMu.Lock()
	if uiaBusy {
		// One call at a time: a second gesture while the first is still in
		// flight would only queue behind it.
		uiaMu.Unlock()
		return selection{}, ErrNoUIA
	}
	w := uiaActive
	if w == nil {
		cooling := !uiaLastTimeout.IsZero() && now.Sub(uiaLastTimeout) < uiaWorkerCooldown
		if cooling || uiaHungWorkers >= uiaMaxHungWorkers {
			uiaMu.Unlock()
			return selection{}, ErrNoUIA
		}
		w = newUIAWorker()
		uiaActive = w
	}
	uiaBusy = true
	uiaMu.Unlock()

	job := uiaJob{done: make(chan uiaResult, 1)}

	// Two budgets, not one: the worker may still be initialising COM on its
	// first job, and that cost does not belong to the provider's 300ms.
	sendTimer := time.NewTimer(uiaTimeout)
	select {
	case w.jobs <- job:
		sendTimer.Stop()
	case <-w.doneCh:
		sendTimer.Stop()
		return selection{}, ErrNoUIA
	case <-sendTimer.C:
		retireUIAWorker(w)
		return selection{}, ErrNoUIA
	}

	waitTimer := time.NewTimer(uiaTimeout)
	select {
	case res := <-job.done:
		waitTimer.Stop()
		uiaMu.Lock()
		if uiaActive == w {
			uiaBusy = false
		}
		uiaMu.Unlock()
		return res.sel, res.err
	case <-w.doneCh:
		waitTimer.Stop()
		// The worker can exit after writing its answer.
		select {
		case res := <-job.done:
			return res.sel, res.err
		default:
			return selection{}, ErrNoUIA
		}
	case <-waitTimer.C:
		// An answer that landed at the same tick as the timeout is still an
		// answer: retiring a healthy worker would cost a whole cooldown.
		select {
		case res := <-job.done:
			uiaMu.Lock()
			if uiaActive == w {
				uiaBusy = false
			}
			uiaMu.Unlock()
			return res.sel, res.err
		default:
		}
		retireUIAWorker(w)
		return selection{}, ErrNoUIA
	}
}
