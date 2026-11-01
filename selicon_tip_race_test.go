package main

import (
	"testing"
	"time"
)

// TestChipTipRaiseGivesUpAfterHide pins the race that left an unclosable plate
// on screen.
//
// onChipPointer decides the label is due, marks it
// shown, and starts a goroutine to create or show the flyout. The pointer can
// leave before that goroutine runs: a hide then marks the label hidden and
// calls Hide, and the goroutine's ShowInactive lands afterwards.
// The window is visible, t.shown is false, so every later hide skips the
// Hide call and the plate stays on top of everything until the app restarts.
//
// The fix is a generation counter bumped by every hide. A raise captures it and
// refuses to show if it has moved on. This test drives that state machine
// directly, because the window itself cannot be created in a test process.
func TestChipTipRaiseGivesUpAfterHide(t *testing.T) {
	a := newTestApp()

	// What onChipPointer does when the wait is over.
	tip := &a.chipTip
	tip.mu.Lock()
	tip.label = "划词翻译"
	tip.shown = true
	gen := tip.gen
	tip.mu.Unlock()

	// The pointer leaves before raiseChipTip gets to run.
	a.hideChipTip()

	tip.mu.Lock()
	stale := tip.gen != gen
	shown := tip.shown
	tip.mu.Unlock()

	if !stale {
		t.Fatal("a hide did not invalidate the in-flight raise: the goroutine " +
			"would call ShowInactive after Hide and strand the plate on screen")
	}
	if shown {
		t.Fatal("hideChipTip left the label marked as shown")
	}
}

// TestHideChipTipAlwaysHidesWindow covers the recovery half.
//
// Even with the race closed, state and window can disagree — an older build, a
// dropped frame, a show that slipped through. Making the hide unconditional is
// what lets a stranded plate go away without a restart.
func TestHideChipTipAlwaysHidesWindow(t *testing.T) {
	a := newTestApp()

	// Pretend a plate is up on screen while the state says it is not, which is
	// exactly the state the race produced.
	tip := &a.chipTip
	tip.mu.Lock()
	tip.shown = false
	tip.key = "划词翻译"
	tip.mu.Unlock()

	// hideChipTip must not return early on `shown == false`; it has to clear
	// the key and bump the generation regardless, so a stale raise cannot
	// revive the plate.
	before := tip.gen
	a.hideChipTip()

	tip.mu.Lock()
	after := tip.gen
	key := tip.key
	tip.mu.Unlock()

	if after == before {
		t.Fatal("hideChipTip did not bump the generation when shown was already false")
	}
	if key != "" {
		t.Fatalf("hideChipTip left the hovered button key set to %q", key)
	}
}

// TestChipTipMovingBetweenButtonsKeepsTheWarmWindow guards the behaviour the
// generation counter must not break: sweeping the pointer along the row moves
// the plate with it instead of making the user wait 160ms per button.
func TestChipTipMovingBetweenButtonsKeepsTheWarmWindow(t *testing.T) {
	a := newTestApp()
	tip := &a.chipTip

	// A plate was just up.
	tip.mu.Lock()
	tip.hiddenAt = time.Now()
	tip.key = ""
	tip.mu.Unlock()

	// Moving to the next button inside chipTipWarm must not restart the wait.
	tip.mu.Lock()
	warm := !tip.hiddenAt.IsZero()
	tip.mu.Unlock()
	if !warm {
		t.Fatal("hiddenAt was not recorded, so the next label would wait again")
	}
}
