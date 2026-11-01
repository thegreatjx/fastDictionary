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

// TestGestureOwnerComesFromThePointer pins the fix for "a topmost fastDictionary
// window makes every selection look like our own window": the owner is the
// window under the drag end point, not the foreground window of the press.
func TestGestureOwnerComesFromThePointer(t *testing.T) {
	src := readSourceFile(t, "hook.go")
	if !containsText(src, "windowAt(dragEnd.X, dragEnd.Y)") {
		t.Error("the drag owner is not resolved from the pointer position")
	}
}

// TestGestureCarriesTheTypingClocks is the wiring check for the typing guard.
//
// The guard itself is a pure function with its own tests, and those tests stay
// green even when the hook forgets to fill the gesture in: an unset PressedAt
// or LastKey makes typingSuppresses answer false, which is exactly the
// behaviour the guard exists to prevent. The failure would be invisible in
// every other test and show up only as "the chip still jumps out while I
// type", so the wiring is pinned here.
func TestGestureCarriesTheTypingClocks(t *testing.T) {
	src := readSourceFile(t, "hook.go")

	// The clocks are taken when the button goes down...
	for _, want := range []string{
		"pressAt = time.Now()",
		"pressLastKey = lastKeyAt",
	} {
		if !containsText(src, want) {
			t.Errorf("hook.go is missing %q: the typing guard would have no clock to read", want)
		}
	}
	// ...and copied into every gesture the mouse path reports.
	if got, want := countOccurrences(src, "PressedAt: pressedAt"), 2; got != want {
		t.Errorf("hook.go sets PressedAt in %d gesture(s), want %d (double click and drag)", got, want)
	}
	if got, want := countOccurrences(src, "LastKey: lastKey"), 2; got != want {
		t.Errorf("hook.go sets LastKey in %d gesture(s), want %d (double click and drag)", got, want)
	}

	// The keyboard path reports a selection that is deliberate by definition,
	// so it must say so rather than defaulting to the drag kind.
	if !containsText(src, "Kind: gestureKeyboard") {
		t.Error("the keyboard selection does not report its kind; it would be judged as a drag")
	}
}

// TestKeyHookFeedsTheTypingClock pins the other half of the wiring: without a
// key clock the guard never fires, and without OnKeyDown the chip stays on
// screen over the field the user has moved on to.
func TestKeyHookFeedsTheTypingClock(t *testing.T) {
	src := readSourceFile(t, "hook.go")
	if !containsText(src, "lastKeyAt = now") {
		t.Error("the key hook never records a keystroke time")
	}
	if !containsText(src, "go OnKeyDown(vk)") {
		t.Error("the key hook never tells the chip that typing started")
	}
	// A modifier on its own must not count as typing: Ctrl or Shift held
	// before a selection drag would otherwise swallow that very drag.
	if !containsText(src, "else if !isModifierKey(info.VkCode)") {
		t.Error("the key hook counts modifiers as typing")
	}
}
