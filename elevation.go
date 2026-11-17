package main

import (
	"fmt"
	"log"
	"os"
	"syscall"
	"unsafe"
)

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

type tokenElevation struct {
	IsElevated uint32
}

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

func currentProcessElevated() bool {
	return processElevated(uint32(os.Getpid()))
}

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

func elevatedTakeoverRequested() bool {
	for _, arg := range os.Args[1:] {
		if arg == elevatedTakeoverArg {
			return true
		}
	}
	return false
}
