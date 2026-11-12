package main

import (
	"fmt"
	"log"
	"os"
	"syscall"
	"unsafe"
)

// Windows refuses cross-integrity input: a normal process cannot install a
// hook into, or read the selection of, a window running as administrator.
// The failure is silent on both sides — the hotkey appears to do nothing —
// so the app has to recognise the situation and say what is wrong.
var (
	advapi32                = syscall.NewLazyDLL("advapi32.dll")
	shell32                 = syscall.NewLazyDLL("shell32.dll")
	procOpenProcessToken    = advapi32.NewProc("OpenProcessToken")
	procGetTokenInformation = advapi32.NewProc("GetTokenInformation")
	procShellExecuteW       = shell32.NewProc("ShellExecuteW")
)

const (
	processQueryLimitedInfo = 0x1000
	tokenQuery              = 0x0008
	tokenElevationClass     = 20 // TokenElevation
	swShowNormal            = 1
)

// tokenElevation mirrors TOKEN_ELEVATION. GetTokenInformation writes a
// DWORD; the struct keeps the call site readable and the size explicit.
type tokenElevation struct {
	IsElevated uint32
}

// processElevated asks the kernel whether pid belongs to an elevated token.
// Query-limited access is enough for this question and is granted across
// integrity levels, which is exactly why it is used instead of querying the
// process image or its window title.
func processElevated(pid uint32) bool {
	h, _, _ := procOpenProcess.Call(processQueryLimitedInfo, 0, uintptr(pid))
	if h == 0 {
		return false
	}
	defer procCloseHandle.Call(h)

	var token uintptr
	if r, _, _ := procOpenProcessToken.Call(h, tokenQuery, uintptr(unsafe.Pointer(&token))); r == 0 {
		return false
	}
	defer procCloseHandle.Call(token)

	var info tokenElevation
	var size uint32
	if r, _, _ := procGetTokenInformation.Call(token, tokenElevationClass,
		uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info),
		uintptr(unsafe.Pointer(&size))); r == 0 {
		return false
	}
	return info.IsElevated != 0
}

// currentProcessElevated reports whether fastDictionary itself is elevated.
// Elevation solves the problem for every window, so the detection is skipped.
func currentProcessElevated() bool {
	return processElevated(uint32(os.Getpid()))
}

// foregroundProcessElevated reports whether the window that owns the
// selection runs with a higher integrity level than this process.
func foregroundProcessElevated() bool {
	hwnd, _, _ := procGetForegroundWnd.Call()
	if hwnd == 0 {
		return false
	}
	var pid uint32
	procGetWindowThread.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return false
	}
	return processElevated(pid)
}

// elevationNotice is the sentence shown instead of the generic "nothing was
// selected" when Windows blocked the read. Empty means elevation is not the
// explanation and the caller should keep its normal message.
func elevationNotice(app string, blocked, selfElevated bool) string {
	if !blocked || selfElevated {
		return ""
	}
	if app == "" {
		app = "当前窗口"
	}
	return "没取到选中的文字：" + app + " 以管理员权限运行，Windows 不允许普通程序读取它的选区。" +
		"可以在下面直接输入，或从托盘菜单选「以管理员身份重启」。"
}

// relaunchElevated starts a second copy through the UAC prompt and leaves
// this one running: the user may cancel the prompt. The old process quits
// after the new one reports success, so a cancelled UAC never leaves the user
// without an app.
func relaunchElevated() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	verb, _ := syscall.UTF16PtrFromString("runas")
	file, _ := syscall.UTF16PtrFromString(exe)
	args, _ := syscall.UTF16PtrFromString(elevatedTakeoverArg)
	r, _, callErr := procShellExecuteW.Call(0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(args)),
		0, swShowNormal)
	if r <= 32 {
		if callErr != nil && callErr != syscall.Errno(0) {
			return fmt.Errorf("以管理员身份重启失败: %v", callErr)
		}
		return fmt.Errorf("以管理员身份重启失败（ShellExecute 返回 %d）", r)
	}
	log.Printf("fastDictionary: elevated restart requested, pid=%d", os.Getpid())
	return nil
}

const elevatedTakeoverArg = "--elevated-takeover"

// elevatedTakeoverRequested reports whether this process was started by the
// tray's restart action. Such an instance deliberately skips the single
// instance lock: the instance it replaces is still exiting at that moment.
func elevatedTakeoverRequested() bool {
	for _, arg := range os.Args[1:] {
		if arg == elevatedTakeoverArg {
			return true
		}
	}
	return false
}
