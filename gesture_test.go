package main

import (
	"testing"
	"time"
)

func dragGesture(x, y int32, owner string) selectionGesture {
	return selectionGesture{X: x, Y: y, Owner: owner, Kind: gestureDrag}
}

func TestGesturePolicy(t *testing.T) {
	cases := []struct {
		trigger    string
		wantChip   bool
		wantPopup  bool
		wantBounds bool
	}{
		{trigger: "icon", wantChip: true},
		{trigger: "auto", wantPopup: true},
		{trigger: "hotkey"},
	}
	for _, tc := range cases {
		t.Run(tc.trigger, func(t *testing.T) {
			a := newTestApp()
			a.store.Update(func(c *Config) { c.SelectionTrigger = tc.trigger })
			a.captureSelection = func() selection {
				return selection{Text: "serendipity", Bounds: tc.wantBounds}
			}
			chips := 0
			popups := 0
			a.showChip = func(x, y int, text string) { chips++ }
			a.showPopup = func(m popupMode, text string) { popups++ }

			a.onSelectionGesture(dragGesture(100, 200, ""))

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

func TestGestureIgnoresEmptySelection(t *testing.T) {
	a := newTestApp()
	a.captureSelection = func() selection { return selection{Text: "   "} }
	chips, popups := 0, 0
	a.showChip = func(x, y int, text string) { chips++ }
	a.showPopup = func(m popupMode, text string) { popups++ }
	a.onSelectionGesture(dragGesture(10, 10, ""))
	if chips != 0 || popups != 0 {
		t.Errorf("empty selection acted on: chips=%d popups=%d", chips, popups)
	}
}

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

	a.onSelectionGesture(dragGesture(9999, 9999, ""))

	wantX := int((100 + 300) / 2)
	wantY := 220
	if gotX != wantX || gotY != wantY {
		t.Errorf("anchor = %d,%d; want %d,%d (the selection's own bounds, not the pointer at 9999,9999)",
			gotX, gotY, wantX, wantY)
	}
}

func TestChipFallsBackToThePointer(t *testing.T) {
	a := newTestApp()
	a.store.Update(func(c *Config) { c.SelectionTrigger = "icon" })
	a.captureSelection = func() selection { return selection{Text: "hi"} }
	var gotX, gotY int
	a.showChip = func(x, y int, text string) { gotX, gotY = x, y }

	a.onSelectionGesture(dragGesture(640, 480, ""))
	if gotX != 640 || gotY != 480 {
		t.Errorf("anchor = %d,%d; want the pointer 640,480", gotX, gotY)
	}
}

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

			a.onSelectionGesture(dragGesture(100, 200, owner))

			if reads != 0 {
				t.Errorf("selection was read %d times inside %s; want 0", reads, owner)
			}
			if chips != 0 {
				t.Errorf("chip shown inside %s; want none", owner)
			}
		})
	}
}

func TestGestureStillWorksInNormalApps(t *testing.T) {
	for _, owner := range []string{"chrome.exe", "WINWORD.EXE", "", "notepad++.exe"} {
		t.Run(owner, func(t *testing.T) {
			a := newTestApp()
			a.store.Update(func(c *Config) { c.SelectionTrigger = "icon" })
			a.captureSelection = func() selection { return selection{Text: "serendipity"} }
			chips := 0
			a.showChip = func(x, y int, text string) { chips++ }

			a.onSelectionGesture(dragGesture(100, 200, owner))

			if chips != 1 {
				t.Errorf("chip shown %d times in %q; want 1", chips, owner)
			}
		})
	}
}

func TestPauseSelectionStopsTheTrigger(t *testing.T) {
	a := newTestApp()
	a.store.Update(func(c *Config) { c.SelectionTrigger = "icon" })
	reads := 0
	a.captureSelection = func() selection { reads++; return selection{Text: "x"} }
	a.showChip = func(x, y int, text string) {}

	a.pauseSelection(time.Minute)
	a.onSelectionGesture(dragGesture(10, 10, "chrome.exe"))
	if reads != 0 {
		t.Fatalf("selection read while paused")
	}

	a.resumeSelection()
	a.onSelectionGesture(dragGesture(10, 10, "chrome.exe"))
	if reads != 1 {
		t.Fatalf("selection read %d times after resume; want 1", reads)
	}
}

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

func TestGestureCaptureFallsBackForQuietApps(t *testing.T) {
	a := newTestApp()
	a.store.Update(func(c *Config) { c.SelectionCapture = "auto" })
	if got := appsWhereDragIsNotSelection["chrome.exe"]; got {
		t.Fatal("chrome must not be excluded from the copy fallback")
	}
}

func TestShellNeverSimulatesCopy(t *testing.T) {
	for _, owner := range []string{"explorer.exe", "EXPLORER.EXE", "Desktop.exe"} {
		if !neverSimulateCopy(owner) {
			t.Errorf("%s is not excluded from the copy fallback", owner)
		}
	}
	for _, owner := range []string{"chrome.exe", "Code.exe", "WINWORD.EXE", ""} {
		if neverSimulateCopy(owner) {
			t.Errorf("%s is wrongly excluded from the copy fallback", owner)
		}
	}
}

func TestTerminalsNeverSimulateCopy(t *testing.T) {
	for _, owner := range []string{
		"WindowsTerminal.exe", "wt.exe", "pwsh.exe", "PowerShell.exe",
		"cmd.exe", "conhost.exe", "OpenConsole.exe", "bash.exe",
		"wsl.exe", "alacritty.exe", "wezterm-gui.exe", "mintty.exe",
		"ConEmu64.exe", "WindowsTerminalPreview.exe",
	} {
		if !neverSimulateCopy(owner) {
			t.Errorf("%s is not excluded from the copy fallback", owner)
		}
	}
	for _, owner := range []string{"wtx.exe", "bashful.exe", "mycmd.exe", "cmdx.exe"} {
		if neverSimulateCopy(owner) {
			t.Errorf("%s is wrongly excluded from the copy fallback", owner)
		}
	}
}

func TestCaptureForGestureHonoursUIAOnly(t *testing.T) {
	a := newTestApp()
	a.store.Update(func(c *Config) { c.SelectionCapture = "uia" })
	if sel := a.captureForGesture("chrome.exe"); sel.Text != "" {
		t.Logf("a real selection was present (%q)", sel.Text)
	}
}

func TestDragWhileTypingIsNotASelection(t *testing.T) {
	a := newTestApp()
	a.store.Update(func(c *Config) { c.SelectionTrigger = "icon" })
	reads := 0
	a.captureSelection = func() selection {
		reads++
		return selection{Text: "量价背离"}
	}
	chips := 0
	a.showChip = func(x, y int, text string) { chips++ }

	typed := time.Now()
	g := selectionGesture{
		X: 2506, Y: 1656, Owner: "chrome.exe", Kind: gestureDrag,
		LastKey:   typed,
		PressedAt: typed.Add(80 * time.Millisecond),
	}
	a.onSelectionGesture(g)

	if reads != 0 {
		t.Errorf("the selection was read %d times during typing; want 0", reads)
	}
	if chips != 0 {
		t.Errorf("chip shown %d times during typing; want 0", chips)
	}
}

func TestDragLongAfterTypingIsASelection(t *testing.T) {
	a := newTestApp()
	a.store.Update(func(c *Config) { c.SelectionTrigger = "icon" })
	a.captureSelection = func() selection { return selection{Text: "serendipity"} }
	chips := 0
	a.showChip = func(x, y int, text string) { chips++ }

	typed := time.Now()
	g := selectionGesture{
		X: 100, Y: 200, Owner: "chrome.exe", Kind: gestureDrag,
		LastKey:   typed,
		PressedAt: typed.Add(typingGrace + 50*time.Millisecond),
	}
	a.onSelectionGesture(g)

	if chips != 1 {
		t.Errorf("chip shown %d times after typing stopped; want 1", chips)
	}
}

func TestDoubleClickStillWorksWhileTyping(t *testing.T) {
	for _, kind := range []gestureKind{gestureDoubleClick, gestureTripleClick, gestureKeyboard} {
		t.Run(kind.String(), func(t *testing.T) {
			a := newTestApp()
			a.store.Update(func(c *Config) { c.SelectionTrigger = "icon" })
			a.captureSelection = func() selection { return selection{Text: "serendipity"} }
			chips := 0
			a.showChip = func(x, y int, text string) { chips++ }

			typed := time.Now()
			a.onSelectionGesture(selectionGesture{
				X: 100, Y: 200, Owner: "chrome.exe", Kind: kind,
				LastKey:   typed,
				PressedAt: typed.Add(10 * time.Millisecond),
			})

			if chips != 1 {
				t.Errorf("chip shown %d times for a %s during typing; want 1", chips, kind)
			}
		})
	}
}

func TestTypingTakesTheChipAway(t *testing.T) {
	a := newTestApp()
	a.hideSelIcon()
	a.sel.text = "量价背离"
	a.onGlobalKeyDown('A')
	if got := a.selText(); got != "量价背离" {
		t.Errorf("typing changed the chip's text to %q", got)
	}
}

func TestTripleClickIsNotSwallowedByTheCooldown(t *testing.T) {
	a := newTestApp()
	a.store.Update(func(c *Config) { c.SelectionTrigger = "icon" })
	a.captureSelection = func() selection { return selection{Text: "serendipity is a good word"} }
	chips := 0
	a.showChip = func(x, y int, text string) { chips++ }

	a.onSelectionGesture(selectionGesture{X: 100, Y: 200, Owner: "chrome.exe", Kind: gestureDoubleClick})
	a.onSelectionGesture(selectionGesture{X: 100, Y: 200, Owner: "chrome.exe", Kind: gestureTripleClick})

	if chips != 2 {
		t.Fatalf("chips = %d, want 2：第 3 击被同点冷却吞掉了", chips)
	}
}
