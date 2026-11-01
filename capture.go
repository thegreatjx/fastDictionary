// Real selection capture on Windows.
//
// The primary strategy is UI Automation (see uia.go): the focused element's
// TextPattern answers with the selection without touching the clipboard and
// without synthesising any input. When UIA has nothing to say — terminal
// emulators, games, some Java and Qt applications — fastDictionary falls back to
// pot's technique: simulate Ctrl+C and read the clipboard.
//
// The fallback snapshots every representation the clipboard holds, not just
// its text, and puts the whole snapshot back afterwards, so a user who had
// a rich document or an image on the clipboard keeps it.
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

// winRect is a RECT.
type winRect struct{ Left, Top, Right, Bottom int32 }

const (
	// GWL_EXSTYLE is -20; GWL_STYLE is -16. Using the wrong one reads the
	// style bits and makes the diagnosis nonsense.
	gwlExStyle     = ^uintptr(19) // -20: GWL_EXSTYLE
	gwlStyle       = ^uintptr(15) // -16: GWL_STYLE
	wsExTopmost    = 0x00000008
	wsExToolWindow = 0x00000080
	wsExAppWindow  = 0x00040000
	wsExLayered    = 0x00080000
	wsExNoActivate = 0x08000000
	swpNoSize      = 0x0001
	swpNoMove      = 0x0002
)

// currentProcessID returns fastDictionary's own process id.
func currentProcessID() uint32 {
	r, _, _ := procGetCurrentPID.Call()
	return uint32(r)
}

// cursorPoint returns the mouse position in physical screen pixels, which
// is the coordinate space UI Automation and the hook use.
func cursorPoint() winPoint {
	var p winPoint
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	return p
}

// foregroundWindow returns the handle of the focused top-level window.
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

// modifierKeys are the keys a global shortcut is normally built from.
var modifierKeys = []uintptr{
	vkShift, vkControl, vkMenu, vkLWin, vkRWin,
	vkLShift, vkRShift, vkLControl, vkRControl, vkLMenu, vkRMenu,
}

// keyDown reports whether a virtual key is physically down right now.
func keyDown(vk uintptr) bool {
	s, _, _ := procGetAsyncKeyState.Call(vk)
	return int16(uint16(s)) < 0 // the high bit means "down"
}

// waitForModifiersReleased blocks until every modifier is up, or the
// timeout expires.
//
// This is what makes selection capture work from a shortcut at all. The
// shortcut that triggered us — Ctrl+Alt+F, say — is still physically held
// when this runs, so a synthesised Ctrl+C would reach the application as
// Ctrl+Alt+C and copy nothing. Every desktop selection tool waits here
// first for the same reason.
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
		time.Sleep(8 * time.Millisecond)
	}
}

// sendCtrlC presses and releases Ctrl+C in the foreground window. The
// caller must have waited for the modifiers to be released first.
func sendCtrlC() {
	keybd(vkControl, false)
	time.Sleep(8 * time.Millisecond)
	keybd(vkC, false)
	time.Sleep(12 * time.Millisecond)
	keybd(vkC, true)
	keybd(vkControl, true)
}

// privateClipboardFormat mirrors the representations mygo refuses to put
// back (process-local drag sessions and OLE bookkeeping).
func privateClipboardFormat(f transfer.Format) bool {
	s := string(f)
	return s == "DataObject" || s == "Ole Private Data" || strings.HasPrefix(s, "MyGo.Transfer.Length:")
}

// snapshotClipboard copies every representation the clipboard advertises.
func snapshotClipboard() transfer.Data {
	data, err := mygo.Clipboard.Read()
	if err != nil {
		return transfer.Data{}
	}
	return data
}

// restoreClipboard puts a snapshot back, dropping representations the
// clipboard cannot hold. It falls back to plain text when the rich write
// fails, so the user never ends up with an empty clipboard.
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

// captureByCopy is the fallback: clear the clipboard, press Ctrl+C, wait for
// text to arrive, then restore what was there before.
//
// It must run while the application the user selected in still has the
// keyboard focus: fastDictionary's own window has to stay hidden until it returns.
func captureByCopy() string {
	// The shortcut's own modifiers are usually still held: releasing them
	// first is what keeps the synthesised Ctrl+C from arriving as
	// Ctrl+Alt+C, which copies nothing.
	waitForModifiersReleased(modifierReleaseTimeout)

	snap := snapshotClipboard()
	previous := mygo.Clipboard.ReadText()

	// Clearing lets us tell a real copy from stale text: an app that ignores
	// Ctrl+C leaves the clipboard empty instead of making us re-read it.
	mygo.Clipboard.Clear()
	sendCtrlC()

	deadline := time.Now().Add(clipboardPollTimeout)
	var got string
	for time.Now().Before(deadline) {
		time.Sleep(clipboardPollInterval)
		if t := mygo.Clipboard.ReadText(); t != "" {
			got = t
			break
		}
	}

	restoreClipboard(snap, previous)
	return strings.TrimSpace(got)
}

// CaptureSelection returns the text the user has selected in the focused
// application. By default it asks UI Automation first and falls back to a
// simulated copy; the "copy" setting always simulates the copy, for the
// applications whose accessibility tree says nothing useful. Either way the
// clipboard is left exactly as it was.
//
// Call it before showing any fastDictionary window: showing one takes the focus
// away from the application holding the selection.
func (a *app) CaptureSelection() selection {
	if a.store.Get().SelectionCapture != "copy" {
		if s := captureUIA(); s.Text != "" {
			return s
		}
	}
	return selection{Text: captureByCopy()}
}

// Timing knobs of the simulated-copy capture, named rather than magic:
// every value was measured against real applications (see HANDOFF.md).
const (
	// modifierReleaseTimeout is how long to wait for the shortcut's own
	// Ctrl/Alt/Shift to come back up before synthesising Ctrl+C.
	modifierReleaseTimeout = 600 * time.Millisecond
	// clipboardPollTimeout is how long to wait for the target app to put
	// the selection on the clipboard after Ctrl+C.
	clipboardPollTimeout = 500 * time.Millisecond
	// clipboardPollInterval is the polling cadence inside that window.
	clipboardPollInterval = 15 * time.Millisecond
)

// uiaTimeout bounds a UIA round trip. Most providers answer in tens of
// milliseconds, but a misbehaving one (some Electron and Java apps) blocks
// inside GetSelection for seconds. The caller would rather fall back to a
// synthesised copy than freeze the capture for that long, so the slow call
// is abandoned and its goroutine is left to finish on its own.
const uiaTimeout = 300 * time.Millisecond

// captureUIA asks UI Automation only and never falls back to a simulated
// copy.
//
// It is the first half of the automatic trigger: cheap, silent, and it
// disturbs nothing. When it comes back empty the caller decides whether a
// synthesised copy is acceptable (see captureForGesture).
func captureUIA() selection {
	// The COM call runs on its own goroutine so a stuck provider cannot
	// take the capture path down with it; the abandoned goroutine leaks one
	// pinned thread, which is cheaper than a frozen app.
	ch := make(chan selection, 1)
	go func() {
		s, err := SelectionUIA()
		if err != nil {
			s = selection{}
		}
		ch <- s
	}()
	select {
	case s := <-ch:
		s.Text = strings.TrimSpace(s.Text)
		return s
	case <-time.After(uiaTimeout):
		log.Printf("fastDictionary: uia capture timed out after %v", uiaTimeout)
		return selection{}
	}
}

// appsWhereDragIsNotSelection are the windows where a drag or a double
// click does something other than select text, so reading the selection
// there returns something the user never asked for, and the fallback itself
// has a side effect.
//
// Two families live here:
//
//   - the shell (Explorer, the desktop): a double click selects a file, and
//     a synthesised Ctrl+C puts its path on the clipboard;
//   - terminals: Ctrl+C is not copy there, it is the interrupt. A drag in a
//     terminal is a selection the terminal handles itself, and when UI
//     Automation cannot read it the fallback used to send a real Ctrl+C into
//     whatever command was running. Measured on this machine: 0 of 3
//     terminal gestures produced text, and each one pressed Ctrl+C.
//
// The map is keyed by lowercased executable name.
var appsWhereDragIsNotSelection = map[string]bool{
	"explorer.exe": true, // double click selects a file, Ctrl+C copies it
	"desktop.exe":  true,

	// Terminals and shells: never synthesise Ctrl+C here.
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

// neverSimulateCopy reports whether the copy fallback must stay off in the
// application named owner, whatever the 取词方式 setting says. A drag in these
// applications either means something else entirely or makes Ctrl+C mean
// something else; in both cases pressing it is worse than reading nothing.
func neverSimulateCopy(owner string) bool {
	name := strings.ToLower(strings.TrimSpace(owner))
	if name == "" {
		return false
	}
	return appsWhereDragIsNotSelection[name]
}

// captureForGesture reads the selection for the automatic trigger.
//
// UI Automation first, because it is silent and covers browsers, editors and
// Office. A synthesised Ctrl+C is the fallback for everything UIA cannot
// see — terminals, Electron windows, some Qt and Java applications — which
// is where the chip used to work and, after the first version of the
// Snipaste fix, silently stopped appearing.
//
// The fallback is still gated: the trigger guard has already dropped
// screenshot tools and the user's own pause, and neverSimulateCopy drops the
// shell and the terminals here, because in those the synthesised Ctrl+C does
// something other than copy.
func (a *app) captureForGesture(owner string) selection {
	// The test seam wins: the trigger policy is verified without a real
	// selection, and it must not reach for the clipboard to do it.
	if a.captureSelection != nil {
		return a.captureSelection()
	}
	mode := a.store.Get().SelectionCapture
	if s := captureUIA(); s.Text != "" {
		log.Printf("fastDictionary: capture path=uia owner=%q bounds=%v", owner, s.Bounds)
		return s
	}
	if mode == "uia" {
		log.Printf("fastDictionary: capture path=uia-empty owner=%q mode=uia", owner)
		return selection{}
	}
	if neverSimulateCopy(owner) {
		// The distinction matters when reading the log later: this is not
		// "the app had no selection", it is "the app is one where pressing
		// Ctrl+C would do something else".
		log.Printf("fastDictionary: capture path=uia-empty owner=%q fallback=refused", owner)
		return selection{}
	}
	txt := captureByCopy()
	log.Printf("fastDictionary: capture path=copy owner=%q got=%d chars", owner, len(txt))
	return selection{Text: txt}
}

// foregroundAppName returns the executable name of the focused window, used
// to avoid capturing from our own popup.
func foregroundAppName() string {
	return processNameOf(foregroundWindow())
}

// processNameOf returns the lowercased executable name owning hwnd.
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

// ---------------------------------------------------------------------------
// Focus control
// ---------------------------------------------------------------------------

var (
	procGetCurrentThread = kernel32.NewProc("GetCurrentThreadId")
)

// windowTitleOf returns a window's title, for logs.
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

// windowClassName returns the window class of hwnd, used to recognise the
// shell's own windows (desktop, taskbar) during a drag.
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

// isOwnProcess reports whether hwnd belongs to fastDictionary itself.
// procWindowFromPoint resolves the window under a screen point.
var procWindowFromPoint = user32.NewProc("WindowFromPoint")

// windowAt returns the window under a physical screen point, walking up to the
// top-level window that owns it.
func windowAt(x, y int32) uintptr {
	pt := uintptr(uint32(x)) | uintptr(uint32(y))<<32
	h, _, _ := procWindowFromPoint.Call(pt)
	if h == 0 {
		return 0
	}
	// The child window under the pointer may belong to a different process
	// than the frame, so walk to the root of this window's tree.
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
