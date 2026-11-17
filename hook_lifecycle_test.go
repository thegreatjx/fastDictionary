package main

import (
	"strings"
	"testing"
)

func TestInstallHooksIsIdempotent(t *testing.T) {
	src := readSourceFile(t, "hook.go")
	start := strings.Index(src, "func installHooks() error {")
	if start < 0 {
		t.Fatal("installHooks not found")
	}
	body := src[start:]

	guard := strings.Index(body, "already := mouseHook != 0")
	if guard < 0 {
		t.Fatal("installHooks has no already-installed guard")
	}
	firstInstall := strings.Index(body, "procSetWindowsHookEx.Call(whMouseLL")
	if firstInstall < 0 {
		t.Fatal("installHooks does not install the mouse hook")
	}
	if guard > firstInstall {
		t.Fatal("the already-installed guard comes after the install, so it guards nothing")
	}
	if !strings.Contains(body[:firstInstall], "return nil") {
		t.Fatal("the guard does not return early on a second install")
	}
}

func TestRemoveHooksIsIdempotent(t *testing.T) {
	src := readSourceFile(t, "hook.go")
	start := strings.Index(src, "func removeHooks() {")
	if start < 0 {
		t.Fatal("removeHooks not found")
	}
	end := strings.Index(src[start:], "\n}\n")
	if end < 0 {
		t.Fatal("removeHooks body not delimited")
	}
	body := src[start : start+end]

	if !strings.Contains(body, "if mh == 0 && kh == 0") {
		t.Fatal("removeHooks calls UnhookWindowsHookEx with zero handles when nothing was installed")
	}
	early := strings.Index(body, "if mh == 0 && kh == 0")
	unhook := strings.Index(body, "procUnhookWindowsHook.Call(mh)")
	if early > unhook {
		t.Fatal("the zero-handle guard comes after the unhook")
	}
}

func TestApplySelectionModeTogglesBothWays(t *testing.T) {
	src := readSourceFile(t, "selicon.go")
	start := strings.Index(src, "func (a *app) applySelectionMode() {")
	if start < 0 {
		t.Fatal("applySelectionMode not found")
	}
	end := strings.Index(src[start:], "\n}\n")
	if end < 0 {
		t.Fatal("applySelectionMode body not delimited")
	}
	body := src[start : start+end]

	if !strings.Contains(body, "installHooks()") {
		t.Fatal("applySelectionMode never installs the hooks")
	}
	if !strings.Contains(body, "removeHooks()") {
		t.Fatal("applySelectionMode never removes the hooks")
	}
	if !strings.Contains(body, `wantHooks := a.store.Get().SelectionTrigger != "hotkey"`) {
		t.Fatal("applySelectionMode does not derive wantHooks from the trigger setting")
	}
}
