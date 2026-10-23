package main

import (
	"testing"
	"time"
)

// TestWaitForModifiersReleasedReturnsWhenClear checks the wait does not
// linger when no modifier is held, which is the case for the selection icon
// and for a shortcut the user has already let go of.
func TestWaitForModifiersReleasedReturnsWhenClear(t *testing.T) {
	if !waitForModifiersReleased(200 * time.Millisecond) {
		t.Skip("a modifier is physically held right now; cannot test the clear case")
	}
	start := time.Now()
	if !waitForModifiersReleased(200 * time.Millisecond) {
		t.Fatal("wait reported a held modifier when none is down")
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Errorf("wait took %v with no modifier down; it should return at once", elapsed)
	}
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
