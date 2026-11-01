package main

import (
	"testing"
	"time"
)

// TestWaitForModifiersReleasedReturnsWhenClear checks the wait does not
// linger when no modifier is held, which is the case for the selection icon
// and for a shortcut the user has already let go of.
//
// The test measures a wait against the physical keyboard, so it cannot assert
// on a single reading: the user may press Ctrl or Alt while it runs, and that
// is not a bug in the wait. It retries a few times and only fails when the
// wait is slow on every attempt, which is what a real "it always sleeps"
// regression looks like.
func TestWaitForModifiersReleasedReturnsWhenClear(t *testing.T) {
	if !waitForModifiersReleased(200 * time.Millisecond) {
		t.Skip("a modifier is physically held right now; cannot test the clear case")
	}
	var worst time.Duration
	for attempt := 0; attempt < 5; attempt++ {
		start := time.Now()
		ok := waitForModifiersReleased(200 * time.Millisecond)
		elapsed := time.Since(start)
		if !ok {
			// A modifier went down between the two readings. That is the
			// user typing, not the wait misbehaving.
			t.Skip("a modifier was pressed during the measurement")
		}
		if elapsed > worst {
			worst = elapsed
		}
		if elapsed <= 100*time.Millisecond {
			return // returned promptly, which is the whole point
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("wait took %v with no modifier down on 5 attempts; it should return at once", worst)
}

// TestKeyDownReportsUnheldKeys is the guard for the bug that made selection
// capture fall through to input mode: the shortcut's own modifiers were
// still down when Ctrl+C was synthesised, so the application received
// Ctrl+Alt+C and copied nothing. The detection has to be trustworthy.
func TestKeyDownReportsUnheldKeys(t *testing.T) {
	// A key no shortcut and no test holds.
	const vkF24 = 0x87
	if keyDown(vkF24) {
		t.Error("keyDown says F24 is down; it is not a key anyone presses")
	}
}

// TestModifierListCoversTheShortcutModifiers keeps Ctrl, Alt, Shift and both
// Windows keys in the list: a shortcut built from any of them must be waited
// for, or the synthesised copy is corrupted.
func TestModifierListCoversTheShortcutModifiers(t *testing.T) {
	want := map[uintptr]string{
		0x10: "Shift", 0x11: "Ctrl", 0x12: "Alt",
		0x5B: "LWin", 0x5C: "RWin",
		0xA0: "LShift", 0xA1: "RShift",
		0xA2: "LCtrl", 0xA3: "RCtrl",
		0xA4: "LAlt", 0xA5: "RAlt",
	}
	have := map[uintptr]bool{}
	for _, vk := range modifierKeys {
		have[vk] = true
	}
	for vk, name := range want {
		if !have[vk] {
			t.Errorf("modifierKeys is missing %s (0x%02X)", name, vk)
		}
	}
}
