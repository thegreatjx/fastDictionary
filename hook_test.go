package main

import (
	"testing"
)

func TestCallNextHookExUsesTheLiveHandle(t *testing.T) {
	src := readSourceFile(t, "hook.go")
	if count := countOccurrences(src, "procCallNextHookEx.Call(0,"); count != 0 {
		t.Fatalf("hook.go still passes 0 as hhk to CallNextHookEx in %d place(s); "+
			"the system drops such a hook from the chain and the callback never runs", count)
	}
	for _, want := range []string{
		"procCallNextHookEx.Call(liveMouseHook,",
		"procCallNextHookEx.Call(liveKeyHook,",
	} {
		if !containsText(src, want) {
			t.Errorf("hook.go does not forward the live hook handle: missing %q", want)
		}
	}
}

func TestHooksRunOnAPumpingThread(t *testing.T) {
	src := readSourceFile(t, "hook.go")
	for _, want := range []string{
		"runtime.LockOSThread()",
		"procGetMessageW.Call",
		"procDispatchMessageW.Call",
	} {
		if !containsText(src, want) {
			t.Errorf("hook.go is missing %q: the hook would be installed on a thread that never pumps", want)
		}
	}
	if containsText(src, "mygo.RunOnMain(func() {\n\t\tif err := installHooks()") {
		t.Error("installHooks is still called from RunOnMain; it must run on the pumping thread")
	}
}

func TestGestureOwnerComesFromThePointer(t *testing.T) {
	src := readSourceFile(t, "hook.go")
	if !containsText(src, "windowAt(dragEnd.X, dragEnd.Y)") {
		t.Error("the drag owner is not resolved from the pointer position")
	}
}

func TestGestureCarriesTheTypingClocks(t *testing.T) {
	src := readSourceFile(t, "hook.go")

	for _, want := range []string{
		"pressAt = time.Now()",
		"pressLastKey = lastKeyAt",
	} {
		if !containsText(src, want) {
			t.Errorf("hook.go is missing %q: the typing guard would have no clock to read", want)
		}
	}
	if got, want := countOccurrences(src, "PressedAt: pressedAt"), 2; got != want {
		t.Errorf("hook.go sets PressedAt in %d gesture(s), want %d (double click and drag)", got, want)
	}
	if got, want := countOccurrences(src, "LastKey: lastKey"), 2; got != want {
		t.Errorf("hook.go sets LastKey in %d gesture(s), want %d (double click and drag)", got, want)
	}

	if !containsText(src, "Kind: gestureKeyboard") {
		t.Error("the keyboard selection does not report its kind; it would be judged as a drag")
	}
}

func TestKeyHookFeedsTheTypingClock(t *testing.T) {
	src := readSourceFile(t, "hook.go")
	if !containsText(src, "lastKeyAt = now") {
		t.Error("the key hook never records a keystroke time")
	}
	if !containsText(src, "dispatchKeyDown(vk)") {
		t.Error("the key hook never tells the chip that typing started")
	}
	if !containsText(src, "else if !isModifierKey(info.VkCode)") {
		t.Error("the key hook counts modifiers as typing")
	}
}
