package main

import (
	"testing"
	"time"
)

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
			t.Skip("a modifier was pressed during the measurement")
		}
		if elapsed > worst {
			worst = elapsed
		}
		if elapsed <= 100*time.Millisecond {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("wait took %v with no modifier down on 5 attempts; it should return at once", worst)
}

func TestKeyDownReportsUnheldKeys(t *testing.T) {
	const vkF24 = 0x87
	if keyDown(vkF24) {
		t.Error("keyDown says F24 is down; it is not a key anyone presses")
	}
}

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
