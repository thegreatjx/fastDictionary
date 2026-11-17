package main

import (
	"errors"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

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
		comCall(this, 2)
	}
}

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

func comApt(fn func() error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hr, _, _ := procCoInitializeEx.Call(0, coinitMultithreaded)
	owned := uint32(hr) == 0
	defer func() {
		if owned {
			procCoUninitialize.Call()
		}
	}()
	return fn()
}

const (
	uiaTextPatternID        = 10014
	uiaIsPasswordPropertyID = 30019

	treeScopeSubtree = 7
)

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

var ErrNoUIA = errors.New("uia: no selection")

func pointArg(x, y int32) uintptr {
	return uintptr(uint64(uint32(x)) | uint64(uint32(y))<<32)
}

type uiaRect struct{ Left, Top, Right, Bottom float64 }

type selection struct {
	Text   string
	Rect   uiaRect
	Bounds bool
}

func selectionFromElement(el uintptr) (selection, error) {
	var out selection
	if el == 0 {
		return out, ErrNoUIA
	}

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
		if hr := comCall(rng, slotTextRangeGetText, ^uintptr(0), uintptr(unsafe.Pointer(&bstr))); hr == 0 && bstr != 0 {
			text += bstrToString(bstr)
			procSysFreeString.Call(bstr)
		}
		if r, ok := rangeBounds(rng); ok {
			out.Rect, out.Bounds = r, true
		}
		comRelease(rng)
		if text != "" {
			break
		}
	}
	if text == "" {
		return out, ErrNoUIA
	}
	out.Text = text
	return out, nil
}

func rangeBounds(rng uintptr) (uiaRect, bool) {
	var sa uintptr
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

type uiaWorker struct {
	jobs    chan uiaJob
	stopCh  chan struct{}
	doneCh  chan struct{}
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
	uiaWorkerCooldown = 8 * time.Second
	uiaMaxHungWorkers = 4
)

var (
	uiaMu          sync.Mutex
	uiaActive      *uiaWorker
	uiaBusy        bool
	uiaLastTimeout time.Time
	uiaHungWorkers int
)

func newUIAWorker() *uiaWorker {
	w := &uiaWorker{
		jobs:   make(chan uiaJob),
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
	go func() {
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
	var el uintptr
	if hr := comCall(uia, slotUiaGetFocusedElement, uintptr(unsafe.Pointer(&el))); hr != 0 {
		el = 0
	}
	if el != 0 && int32(comCall(el, slotElementCurrentProcessID)) == int32(currentProcessID()) {
		comRelease(el)
		el = 0
	}
	if el != 0 {
		s, err := selectionFromElement(el)
		comRelease(el)
		if err == nil {
			return s, nil
		}
	}

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

func SelectionUIA() (selection, error) {
	now := time.Now()
	uiaMu.Lock()
	if uiaBusy {
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
		select {
		case res := <-job.done:
			return res.sel, res.err
		default:
			return selection{}, ErrNoUIA
		}
	case <-waitTimer.C:
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
