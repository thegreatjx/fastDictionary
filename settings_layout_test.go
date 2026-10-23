package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// newSettingsTester renders the settings view headlessly at a given size.
func newSettingsTester(a *app, w, h int) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) { a.settingsView(c) }, w, h)
}

// TestSettingsScrollsWhenContentOverflows is the regression test for "the
// settings window cannot scroll and the page is cut off".
//
// The root row of the settings view must fill the window in both directions.
// With only FillWidth the row grows to its content, the scroll view measures
// against an unbounded height, and a long page is simply clipped: MaxY stays 0
// and the bottom of the form can never be reached. The invariant checked here
// is the one that was broken: when the content is taller than the viewport, the
// page must report overflow and scrolling to the end must bring the last item
// into view.
func TestSettingsScrollsWhenContentOverflows(t *testing.T) {
	const w, h = 680, 360 // a short window: the form is certainly taller

	a := newTestApp()
	a.page = "selection"
	a.cfgWin = nil
	a.settingsPageSeen = ""

	v := newSettingsTester(a, w, h)
	v.Frame()
	v.Frame()

	if a.settingsScroll.MaxY <= 0 {
		t.Fatalf("MaxY=%v at %dx%d: the layout is unbounded, so a long page is clipped with no way to scroll", a.settingsScroll.MaxY, w, h)
	}

	bottom := "关于剪贴板"
	before, ok := v.Find(bottom)
	if !ok {
		t.Fatalf("the last field %q is missing from the layout", bottom)
	}
	if before.Y+before.H <= float32(h) {
		t.Skipf("%q is already visible at %dx%d; the page fits, nothing to prove", bottom, w, h)
	}

	// Scrolling to the end must bring it into the viewport.
	a.settingsScroll.Y = a.settingsScroll.MaxY
	v.Frame()
	after, ok := v.Find(bottom)
	if !ok {
		t.Fatalf("the last field %q disappeared after scrolling", bottom)
	}
	if after.Y < 0 || after.Y+after.H > float32(h) {
		t.Fatalf("after scrolling to the end %q is at y=%.0f h=%.0f, outside the %dpx viewport",
			bottom, after.Y, after.H, h)
	}
}

// TestSettingsScrollKeepsPosition checks the other half of the fix: flipping a
// switch near the bottom of a page must not throw the user back to the top,
// which is what happens when the offset lives only in the widget.
func TestSettingsScrollKeepsPosition(t *testing.T) {
	a := newTestApp()
	a.page = "selection"
	a.cfgWin = nil
	a.settingsPageSeen = ""

	v := newSettingsTester(a, 680, 360)
	v.Frame()
	v.Frame()

	deep := a.settingsScroll.MaxY * 0.75
	a.settingsScroll.Y = deep
	v.Frame()
	if a.settingsScroll.Y != deep {
		t.Fatalf("scroll offset reset from %v to %v by a re-render", deep, a.settingsScroll.Y)
	}

	// Switching page starts the new page at the top.
	a.page = "general"
	v.Frame()
	if a.settingsScroll.Y != 0 {
		t.Fatalf("scroll offset %v kept across a page switch, want 0", a.settingsScroll.Y)
	}
}
