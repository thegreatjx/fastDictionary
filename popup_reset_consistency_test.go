package main

import "testing"

func TestResetPopupTransientLocked(t *testing.T) {
	a := &app{}
	a.onlyTrunk = true
	a.showSlashes = true
	a.activeActionID = "default-grammar"

	a.resetPopupTransientLocked()

	if a.onlyTrunk || a.showSlashes || a.activeActionID != "" {
		t.Fatalf("resetPopupTransientLocked left state behind: onlyTrunk=%v showSlashes=%v action=%q",
			a.onlyTrunk, a.showSlashes, a.activeActionID)
	}

	a.resetPopupTransientLocked()
	if a.onlyTrunk || a.showSlashes || a.activeActionID != "" {
		t.Fatal("resetPopupTransientLocked is not idempotent")
	}
}

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
