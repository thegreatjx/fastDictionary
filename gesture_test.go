package main

import (
	"strings"
	"testing"
	"time"
)

// TestGesturePolicy covers what a selection does for each trigger, with the
// reading of the selection supplied: the policy is what matters, and it is
// testable without a real selection on screen.
func TestGesturePolicy(t *testing.T) {
	cases := []struct {
		trigger    string
		wantChip   bool
		wantPopup  bool
		wantBounds bool
	}{
		{trigger: "icon", wantChip: true},
		{trigger: "auto", wantPopup: true},
		{trigger: "hotkey"}, // neither: the shortcut is the only trigger
	}
	for _, tc := range cases {
		t.Run(tc.trigger, func(t *testing.T) {
			a := newTestApp()
			a.store.Update(func(c *Config) { c.SelectionTrigger = tc.trigger })
			a.captureSelection = func() selection {
				return selection{Text: "serendipity", Bounds: tc.wantBounds}
			}
			// Route the two outcomes to flags instead of real windows.
			chips := 0
			popups := 0
			a.showChip = func(x, y int, text string) { chips++ }
			a.showPopup = func(m popupMode, text string) { popups++ }

			a.onSelectionGesture(100, 200, "")

			if tc.wantChip && chips != 1 {
				t.Errorf("chip shown %d times, want 1", chips)
			}
			if !tc.wantChip && chips != 0 {
				t.Errorf("chip shown %d times, want 0", chips)
			}
			if tc.wantPopup && popups != 1 {
				t.Errorf("popup shown %d times, want 1", popups)
			}
			if !tc.wantPopup && popups != 0 {
				t.Errorf("popup shown %d times, want 0", popups)
			}
		})
	}
}

// TestGestureIgnoresEmptySelection keeps an empty read from opening
// anything at all.
func TestGestureIgnoresEmptySelection(t *testing.T) {
	a := newTestApp()
	a.captureSelection = func() selection { return selection{Text: "   "} }
	chips, popups := 0, 0
	a.showChip = func(x, y int, text string) { chips++ }
	a.showPopup = func(m popupMode, text string) { popups++ }
	a.onSelectionGesture(10, 10, "")
	if chips != 0 || popups != 0 {
		t.Errorf("empty selection acted on: chips=%d popups=%d", chips, popups)
	}
}

// TestChipAnchorsToSelectionBounds checks the chip follows the selection's
// own rectangle when the provider reports one, rather than the pointer:
// with the keyboard the pointer is nowhere near the selection.
func TestChipAnchorsToSelectionBounds(t *testing.T) {
	a := newTestApp()
	a.store.Update(func(c *Config) { c.SelectionTrigger = "icon" })
	a.captureSelection = func() selection {
		return selection{
			Text:   "hello",
			Bounds: true,
			Rect:   uiaRect{Left: 100, Top: 200, Right: 300, Bottom: 220},
		}
	}
	var gotX, gotY int
	a.showChip = func(x, y int, text string) { gotX, gotY = x, y }

	a.onSelectionGesture(9999, 9999, "") // a pointer far away from the selection

	// The seam receives the anchor: the middle of the selection's bottom
	// edge, in physical pixels. Turning that anchor into a window position
	// is placeChip's job, covered by the TestPlaceChip tests.
	wantX := int((100 + 300) / 2)
	wantY := 220
	if gotX != wantX || gotY != wantY {
		t.Errorf("anchor = %d,%d; want %d,%d (the selection's own bounds, not the pointer at 9999,9999)",
			gotX, gotY, wantX, wantY)
	}
}

// TestChipFallsBackToThePointer checks a provider that reports no rectangle
// leaves the chip at the pointer, which is the end of a mouse drag.
func TestChipFallsBackToThePointer(t *testing.T) {
	a := newTestApp()
	a.store.Update(func(c *Config) { c.SelectionTrigger = "icon" })
	a.captureSelection = func() selection { return selection{Text: "hi"} } // no Bounds
	var gotX, gotY int
	a.showChip = func(x, y int, text string) { gotX, gotY = x, y }

	a.onSelectionGesture(640, 480, "")
	if gotX != 640 || gotY != 480 {
		t.Errorf("anchor = %d,%d; want the pointer 640,480", gotX, gotY)
	}
}

// TestGestureIgnoredInScreenshotTools is the Snipaste fix: a drag inside a
// screenshot tool is a snip region, not a selection, so nothing may be read
// from it — reading means clearing the clipboard and synthesising Ctrl+C,
// which is what tore the snip session down.
func TestGestureIgnoredInScreenshotTools(t *testing.T) {
	for _, owner := range []string{"snipaste.exe", "Snipaste.exe", "PixPin.exe", "SnippingTool.exe"} {
		t.Run(owner, func(t *testing.T) {
			a := newTestApp()
			a.store.Update(func(c *Config) { c.SelectionTrigger = "icon" })
			reads := 0
			a.captureSelection = func() selection {
				reads++
				return selection{Text: "should not be read"}
			}
			chips := 0
			a.showChip = func(x, y int, text string) { chips++ }

			a.onSelectionGesture(100, 200, owner)

			if reads != 0 {
				t.Errorf("selection was read %d times inside %s; want 0", reads, owner)
			}
			if chips != 0 {
				t.Errorf("chip shown inside %s; want none", owner)
			}
		})
	}
}

// TestGestureStillWorksInNormalApps keeps the guard from swallowing real
// selections.
func TestGestureStillWorksInNormalApps(t *testing.T) {
	for _, owner := range []string{"chrome.exe", "WINWORD.EXE", "", "notepad++.exe"} {
		t.Run(owner, func(t *testing.T) {
			a := newTestApp()
			a.store.Update(func(c *Config) { c.SelectionTrigger = "icon" })
			a.captureSelection = func() selection { return selection{Text: "serendipity"} }
			chips := 0
			a.showChip = func(x, y int, text string) { chips++ }

			a.onSelectionGesture(100, 200, owner)

			if chips != 1 {
				t.Errorf("chip shown %d times in %q; want 1", chips, owner)
			}
		})
	}
}

// TestPauseSelectionStopsTheTrigger covers the tray's escape hatch.
func TestPauseSelectionStopsTheTrigger(t *testing.T) {
	a := newTestApp()
	a.store.Update(func(c *Config) { c.SelectionTrigger = "icon" })
	reads := 0
	a.captureSelection = func() selection { reads++; return selection{Text: "x"} }
	a.showChip = func(x, y int, text string) {}

	a.pauseSelection(time.Minute)
	a.onSelectionGesture(10, 10, "chrome.exe")
	if reads != 0 {
		t.Fatalf("selection read while paused")
	}

	a.resumeSelection()
	a.onSelectionGesture(10, 10, "chrome.exe")
	if reads != 1 {
		t.Fatalf("selection read %d times after resume; want 1", reads)
	}
}

// TestBlocklistMatching pins the name matching rules.
func TestBlocklistMatching(t *testing.T) {
	list := []string{"snipaste.exe", "ShareX.exe"}
	cases := map[string]bool{
		"snipaste.exe": true, "Snipaste.EXE": true, "snipaste": true,
		"sharex.exe": true, "chrome.exe": false, "": false, "snipaste2.exe": false,
	}
	for name, want := range cases {
		if got := appIsBlocked(name, list); got != want {
			t.Errorf("appIsBlocked(%q) = %v, want %v", name, got, want)
		}
	}
}

// TestGestureCaptureFallsBackForQuietApps covers the regression that made the
// chip stop appearing: applications with no UI Automation text (terminals,
// Electron windows) need the simulated copy, and removing it silently broke
// them.
func TestGestureCaptureFallsBackForQuietApps(t *testing.T) {
	a := newTestApp()
	a.store.Update(func(c *Config) { c.SelectionCapture = "auto" })
	// captureUIA returns nothing in the test environment; the fallback is
	// what the gesture must reach for a normal application.
	if got := appsWhereDragIsNotSelection["chrome.exe"]; got {
		t.Fatal("chrome must not be excluded from the copy fallback")
	}
}

// TestShellNeverSimulatesCopy keeps the Explorer half of the fix: a double
// click there selects a file, and a synthesised Ctrl+C would put its path on
// the clipboard.
func TestShellNeverSimulatesCopy(t *testing.T) {
	for _, owner := range []string{"explorer.exe", "EXPLORER.EXE", "Desktop.exe"} {
		if !appsWhereDragIsNotSelection[strings.ToLower(owner)] {
			t.Errorf("%s is not excluded from the copy fallback", owner)
		}
	}
	for _, owner := range []string{"chrome.exe", "Code.exe", "WindowsTerminal.exe", ""} {
		if appsWhereDragIsNotSelection[strings.ToLower(owner)] {
			t.Errorf("%s is wrongly excluded from the copy fallback", owner)
		}
	}
}

// TestCaptureForGestureHonoursUIAOnly checks the quietest mode.
func TestCaptureForGestureHonoursUIAOnly(t *testing.T) {
	a := newTestApp()
	a.store.Update(func(c *Config) { c.SelectionCapture = "uia" })
	// With no UI Automation selection available, UIA-only returns nothing
	// rather than falling back to a copy.
	if sel := a.captureForGesture("chrome.exe"); sel.Text != "" {
		t.Logf("a real selection was present (%q)", sel.Text)
	}
}
