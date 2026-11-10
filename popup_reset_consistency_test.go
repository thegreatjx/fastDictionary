package main

import "testing"

// TestResetPopupTransientLocked pins the shared reset for the per-look view
// switches and the active action id. Both popup entry points must apply this
// same list: onlyTrunk used to be cleared by one entry only, so a popup opened
// through the other entry kept the previous card's folded view.
func TestResetPopupTransientLocked(t *testing.T) {
	a := &app{}
	a.onlyTrunk = true
	a.showSlashes = true
	a.filterRole = "subject"
	a.grammarHoverKnowledge = "clause"
	a.activeActionID = "default-grammar"

	a.resetPopupTransientLocked()

	if a.onlyTrunk || a.showSlashes || a.filterRole != "" || a.grammarHoverKnowledge != "" || a.activeActionID != "" {
		t.Fatalf("resetPopupTransientLocked left state behind: onlyTrunk=%v showSlashes=%v filterRole=%q hover=%q action=%q",
			a.onlyTrunk, a.showSlashes, a.filterRole, a.grammarHoverKnowledge, a.activeActionID)
	}

	// Repeated resets must stay harmless.
	a.resetPopupTransientLocked()
	if a.onlyTrunk || a.showSlashes || a.filterRole != "" || a.activeActionID != "" {
		t.Fatal("resetPopupTransientLocked is not idempotent")
	}
}

// TestBothPopupEntriesResetTransients proves both entry points delegate to the
// shared reset, and that only that method spells out the field list.
func TestBothPopupEntriesResetTransients(t *testing.T) {
	src := readSourceFile(t, "popup.go")

	for _, sig := range []string{
		"func (a *app) openPopupAction(",
		"func (a *app) openPopupWithText(",
	} {
		body := sourceFuncBody(t, src, sig)
		if !containsText(body, "a.resetPopupTransientLocked()") {
			t.Fatalf("%s does not reset the transient popup state", sig)
		}
	}

	if got := countOccurrences(src, "a.onlyTrunk = false"); got != 1 {
		t.Fatalf("the transient field list is maintained in %d places, want exactly 1 shared method", got)
	}
}
