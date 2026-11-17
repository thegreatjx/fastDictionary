package main

import (
	"log"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
)

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	shcore               = syscall.NewLazyDLL("shcore.dll")
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	dwmapi               = syscall.NewLazyDLL("dwmapi.dll")
	procKeybdEvent       = user32.NewProc("keybd_event")
	procGetForegroundWnd = user32.NewProc("GetForegroundWindow")
	procSetForegroundWnd = user32.NewProc("SetForegroundWindow")
	procGetCursorPos     = user32.NewProc("GetCursorPos")
	procGetWindowThread  = user32.NewProc("GetWindowThreadProcessId")
	procGetClassNameW    = user32.NewProc("GetClassNameW")
	procOpenProcess      = kernel32.NewProc("OpenProcess")
	procQueryFullProcess = kernel32.NewProc("QueryFullProcessImageNameW")
	procCloseHandle      = kernel32.NewProc("CloseHandle")
	procGetAsyncKeyState = user32.NewProc("GetAsyncKeyState")
	procGetWindowRect    = user32.NewProc("GetWindowRect")
	procIsWindowVisible  = user32.NewProc("IsWindowVisible")
	procIsIconic         = user32.NewProc("IsIconic")
	procGetWindowLongPtr = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtr = user32.NewProc("SetWindowLongPtrW")
	procSetWindowPos     = user32.NewProc("SetWindowPos")
	procGetWindow        = user32.NewProc("GetWindow")
	procMoveWindow       = user32.NewProc("MoveWindow")
	procMonitorFromPoint = user32.NewProc("MonitorFromPoint")
	procClientToScreen   = user32.NewProc("ClientToScreen")
	procGetDpiForMonitor = shcore.NewProc("GetDpiForMonitor")
	procDwmSetWindowAttr = dwmapi.NewProc("DwmSetWindowAttribute")
	procGetCurrentPID    = kernel32.NewProc("GetCurrentProcessId")
	procGetModuleHandle  = kernel32.NewProc("GetModuleHandleW")
	procGetWindowTextW   = user32.NewProc("GetWindowTextW")
	procEnumWindows      = user32.NewProc("EnumWindows")
	procGetParent        = user32.NewProc("GetParent")
)

const (
	vkShift        = 0x10
	vkControl      = 0x11
	vkMenu         = 0x12 // Alt
	vkLWin         = 0x5B
	vkRWin         = 0x5C
	vkLShift       = 0xA0
	vkRShift       = 0xA1
	vkLControl     = 0xA2
	vkRControl     = 0xA3
	vkLMenu        = 0xA4
	vkRMenu        = 0xA5
	vkC            = 0x43
	keyeventfKeyUp = 0x0002
)

type winPoint struct{ X, Y int32 }

type winRect struct{ Left, Top, Right, Bottom int32 }

const (
	// GetWindowLongPtr 的 GWL_EXSTYLE / GWL_STYLE 是负偏移。
	gwlExStyle     = ^uintptr(19)
	gwlStyle       = ^uintptr(15)
	wsExTopmost    = 0x00000008
	wsExToolWindow = 0x00000080
	wsExAppWindow  = 0x00040000
	wsExLayered    = 0x00080000
	wsExNoActivate = 0x08000000
	swpNoSize      = 0x0001
	swpNoMove      = 0x0002
)

func currentProcessID() uint32 {
	r, _, _ := procGetCurrentPID.Call()
	return uint32(r)
}

func cursorPoint() winPoint {
	var p winPoint
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	return p
}

func foregroundWindow() uintptr {
	h, _, _ := procGetForegroundWnd.Call()
	return h
}

func keybd(vk uintptr, up bool) {
	flags := uintptr(0)
	if up {
		flags = keyeventfKeyUp
	}
	procKeybdEvent.Call(vk, 0, flags, 0)
}

var modifierKeys = []uintptr{
	vkShift, vkControl, vkMenu, vkLWin, vkRWin,
	vkLShift, vkRShift, vkLControl, vkRControl, vkLMenu, vkRMenu,
}

func keyDown(vk uintptr) bool {
	s, _, _ := procGetAsyncKeyState.Call(vk)
	return int16(uint16(s)) < 0
}

func waitForModifiersReleased(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		busy := false
		for _, vk := range modifierKeys {
			if keyDown(vk) {
				busy = true
				break
			}
		}
		if !busy {
			return true
		}
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(4 * time.Millisecond)
	}
}

func sendCtrlC() {
	keybd(vkControl, false)
	time.Sleep(8 * time.Millisecond)
	keybd(vkC, false)
	time.Sleep(12 * time.Millisecond)
	keybd(vkC, true)
	keybd(vkControl, true)
}

func privateClipboardFormat(f transfer.Format) bool {
	s := string(f)
	return s == "DataObject" || s == "Ole Private Data" || strings.HasPrefix(s, "MyGo.Transfer.Length:")
}

func snapshotClipboard() transfer.Data {
	data, err := mygo.Clipboard.Read()
	if err != nil {
		return transfer.Data{}
	}
	return data
}

func restoreClipboard(snap transfer.Data, text string) {
	var items []transfer.Item
	for _, item := range snap.Items() {
		var reps []transfer.Representation
		for _, f := range item.Formats() {
			if privateClipboardFormat(f) {
				continue
			}
			b, err := item.Read(f)
			if err != nil {
				continue
			}
			reps = append(reps, transfer.Bytes(f, b))
		}
		if len(reps) > 0 {
			items = append(items, transfer.NewItem(reps...))
		}
	}
	if len(items) > 0 {
		if err := mygo.Clipboard.Write(transfer.New(items...)); err == nil {
			return
		}
	}
	if text != "" {
		mygo.Clipboard.WriteText(text)
	}
}

func captureByCopy() string {
	// 快捷键触发时修饰键仍按住，必须等释放后再取选区。
	waitForModifiersReleased(modifierReleaseTimeout)

	tx := beginClipboardTransaction()
	defer tx.restore()

	mygo.Clipboard.Clear()
	sendCtrlC()

	deadline := time.Now().Add(clipboardPollTimeout)
	var got string
	interval := 3 * time.Millisecond
	for time.Now().Before(deadline) {
		time.Sleep(interval)
		if t := mygo.Clipboard.ReadText(); t != "" {
			got = t
			break
		}
		if interval < clipboardPollInterval {
			interval += 4 * time.Millisecond
			if interval > clipboardPollInterval {
				interval = clipboardPollInterval
			}
		}
	}

	return strings.TrimSpace(got)
}

func (a *app) CaptureSelection() selection {
	tryUIA, allowCopy := parseCaptureStrategy(a.store.Get().SelectionCapture).capturePlan()
	if tryUIA {
		if s := captureUIA(); s.Text != "" {
			return s
		}
	}
	if !allowCopy {
		return selection{}
	}
	return selection{Text: captureByCopy()}
}

const (
	modifierReleaseTimeout = 600 * time.Millisecond
	clipboardPollTimeout   = 500 * time.Millisecond
	clipboardPollInterval  = 15 * time.Millisecond
)

const uiaTimeout = 300 * time.Millisecond

func captureUIA() selection {
	s, err := SelectionUIA()
	if err != nil {
		return selection{}
	}
	s.Text = strings.TrimSpace(s.Text)
	return s
}

var appsWhereDragIsNotSelection = map[string]bool{
	// 资源管理器里拖拽是选中文件，不是划词。
	"explorer.exe": true,
	"desktop.exe":  true,

	"pwsh.exe":                   true,
	"powershell.exe":             true,
	"windowsterminal.exe":        true,
	"wt.exe":                     true,
	"cmd.exe":                    true,
	"conhost.exe":                true,
	"openconsole.exe":            true,
	"bash.exe":                   true,
	"wsl.exe":                    true,
	"wslhost.exe":                true,
	"alacritty.exe":              true,
	"wezterm-gui.exe":            true,
	"wezterm.exe":                true,
	"mintty.exe":                 true,
	"conemu.exe":                 true,
	"conemu64.exe":               true,
	"tabby.exe":                  true,
	"hyper.exe":                  true,
	"kitty.exe":                  true,
	"fluentterminal.exe":         true,
	"windowsterminalpreview.exe": true,
}

func neverSimulateCopy(owner string) bool {
	name := strings.ToLower(strings.TrimSpace(owner))
	if name == "" {
		return false
	}
	return appsWhereDragIsNotSelection[name]
}

func (a *app) captureForGesture(owner string) selection {
	if a.captureSelection != nil {
		return a.captureSelection()
	}
	strategy := parseCaptureStrategy(a.store.Get().SelectionCapture)
	tryUIA, allowCopy := strategy.capturePlan()
	if tryUIA {
		if s := captureUIA(); s.Text != "" {
			log.Printf("fastDictionary: capture path=uia owner=%q bounds=%v", owner, s.Bounds)
			return s
		}
	}
	if !allowCopy {
		log.Printf("fastDictionary: capture path=uia-empty owner=%q mode=%s", owner, strategy)
		return selection{}
	}
	if neverSimulateCopy(owner) {
		log.Printf("fastDictionary: capture path=uia-empty owner=%q fallback=refused", owner)
		return selection{}
	}
	txt := captureByCopy()
	log.Printf("fastDictionary: capture path=copy owner=%q got=%d chars", owner, len(txt))
	return selection{Text: txt}
}

func foregroundAppName() string {
	return processNameOf(foregroundWindow())
}

func processNameOf(hwnd uintptr) string {
	if hwnd == 0 {
		return ""
	}
	var pid uint32
	procGetWindowThread.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return ""
	}
	const processQueryLimitedInformation = 0x1000
	h, _, _ := procOpenProcess.Call(processQueryLimitedInformation, 0, uintptr(pid))
	if h == 0 {
		return ""
	}
	defer procCloseHandle.Call(h)
	buf := make([]uint16, 512)
	size := uint32(len(buf))
	if ok, _, _ := procQueryFullProcess.Call(h, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size))); ok == 0 {
		return ""
	}
	full := syscall.UTF16ToString(buf)
	if i := strings.LastIndexAny(full, "\\/"); i >= 0 {
		full = full[i+1:]
	}
	return strings.ToLower(full)
}

var (
	procGetCurrentThread = kernel32.NewProc("GetCurrentThreadId")
)

func windowTitleOf(hwnd uintptr) string {
	if hwnd == 0 {
		return ""
	}
	buf := make([]uint16, 512)
	n, _, _ := procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:n])
}

func windowClassName(hwnd uintptr) string {
	if hwnd == 0 {
		return ""
	}
	buf := make([]uint16, 256)
	n, _, _ := procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:n])
}

var procWindowFromPoint = user32.NewProc("WindowFromPoint")

func windowAt(x, y int32) uintptr {
	pt := uintptr(uint32(x)) | uintptr(uint32(y))<<32
	h, _, _ := procWindowFromPoint.Call(pt)
	if h == 0 {
		return 0
	}
	for {
		parent, _, _ := procGetParent.Call(h)
		if parent == 0 {
			break
		}
		h = parent
	}
	return h
}

func isOwnProcess(hwnd uintptr) bool {
	if hwnd == 0 {
		return false
	}
	var pid uint32
	procGetWindowThread.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	return pid == currentProcessID()
}
