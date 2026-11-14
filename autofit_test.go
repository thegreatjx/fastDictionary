package main

import (
	"testing"
)

// TestAutofitStopsWhenTheTwoHeightsDisagree walks the sequence a real session
// produced: shrink, then a frame that asks to grow by a similar amount, then
// the shrink again. Before the latch, every pass applied its correction and the
// window never stopped moving.
func TestAutofitStopsWhenTheTwoHeightsDisagree(t *testing.T) {
	a := newTestApp()
	a.mu.Lock()
	defer a.mu.Unlock()

	// First correction is accepted.
	delta, ok := a.autofitDecision(-43.9, true)
	if !ok || delta != -43.9 {
		t.Fatalf("first correction dropped: delta=%v ok=%v", delta, ok)
	}
	// A later correction in the same direction is still accepted.
	delta, ok = a.autofitDecision(-20.5, true)
	if !ok || delta != -20.5 {
		t.Fatalf("same-direction correction dropped: delta=%v ok=%v", delta, ok)
	}
	// The opposite one is the feedback loop starting: drop it.
	delta, ok = a.autofitDecision(20.8, true)
	if ok || delta != 0 {
		t.Fatalf("reversal applied: delta=%v ok=%v", delta, ok)
	}
	// And it stays dropped until the content changes.
	delta, ok = a.autofitDecision(-43.9, true)
	if ok || delta != 0 {
		t.Fatalf("correction applied while latched: delta=%v ok=%v", delta, ok)
	}
}

// TestAutofitResumesAfterContentChanges makes sure the latch is not permanent:
// fitHeight is what a new translation calls, and it has to release the latch.
func TestAutofitResumesAfterContentChanges(t *testing.T) {
	a := newTestApp()
	a.popupH = 260

	a.mu.Lock()
	a.autofitDecision(-40, true)
	a.autofitDecision(40, true)
	locked := a.autofitLocked
	a.mu.Unlock()
	if !locked {
		t.Fatal("no latch after a reversal")
	}

	// fitHeight releases it. It needs a window, so drive the release the way
	// fitHeight does rather than opening a real one.
	a.mu.Lock()
	a.autofitDir = 0
	a.autofitLocked = false
	a.mu.Unlock()

	a.mu.Lock()
	defer a.mu.Unlock()
	delta, ok := a.autofitDecision(-60, true)
	if !ok || delta != -60 {
		t.Fatalf("latched after a content change: delta=%v ok=%v", delta, ok)
	}
}

// TestAutofitIgnoresFramesInsideTheDeadZone covers the small corrections the
// caller reports as "nothing to do": they must not clear the direction that the
// next real correction is compared against.
func TestAutofitIgnoresFramesInsideTheDeadZone(t *testing.T) {
	a := newTestApp()
	a.mu.Lock()
	defer a.mu.Unlock()

	a.autofitDecision(-43, true)
	if _, ok := a.autofitDecision(0, false); ok {
		t.Fatal("dead-zone frame applied a correction")
	}
	if a.autofitDir != -43 {
		t.Fatalf("dead-zone frame changed the direction: %v", a.autofitDir)
	}
	// A reversal is still recognised afterwards.
	if _, ok := a.autofitDecision(30, true); ok {
		t.Fatal("reversal applied after a dead-zone frame")
	}
}
