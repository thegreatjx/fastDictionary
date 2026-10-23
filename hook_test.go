package main

import (
	"testing"
)

// TestCallNextHookExUsesTheLiveHandle guards the bug that made the selection
// chip disappear entirely: the hook callbacks passed 0 as hhk to
// CallNextHookEx. Windows documents that parameter as ignored, but passing 0
// makes the system drop the hook from the chain, so SetWindowsHookEx returns a
// valid handle and the callback is never called again. The symptom is
// indistinguishable from "no selection" unless something asserts the handle is
// forwarded.
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

// TestHooksRunOnAPumpingThread guards the other half of the same bug: a
// low-level hook is only delivered to a thread that pumps messages, so
// installing it from mygo's main thread (or any goroutine the scheduler can
// move) produces a hook that installs successfully and never fires.
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
	// And the install must happen on that thread, not via RunOnMain.
	if containsText(src, "mygo.RunOnMain(func() {\n\t\tif err := installHooks()") {
		t.Error("installHooks is still called from RunOnMain; it must run on the pumping thread")
	}
}

// TestGestureOwnerComesFromThePointer pins the fix for "a topmost fastpot
// window makes every selection look like our own window": the owner is the
// window under the drag end point, not the foreground window of the press.
func TestGestureOwnerComesFromThePointer(t *testing.T) {
	src := readSourceFile(t, "hook.go")
	if !containsText(src, "windowAt(dragEnd.X, dragEnd.Y)") {
		t.Error("the drag owner is not resolved from the pointer position")
	}
}
