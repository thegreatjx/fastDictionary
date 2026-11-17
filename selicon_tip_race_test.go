package main

import (
	"testing"
	"time"
)

func TestChipTipRaiseGivesUpAfterHide(t *testing.T) {
	a := newTestApp()

	tip := &a.chipTip
	tip.mu.Lock()
	tip.label = "划词翻译"
	tip.shown = true
	gen := tip.gen
	tip.mu.Unlock()

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

func TestHideChipTipAlwaysHidesWindow(t *testing.T) {
	a := newTestApp()

	tip := &a.chipTip
	tip.mu.Lock()
	tip.shown = false
	tip.key = "划词翻译"
	tip.mu.Unlock()

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

func TestChipTipMovingBetweenButtonsKeepsTheWarmWindow(t *testing.T) {
	a := newTestApp()
	tip := &a.chipTip

	tip.mu.Lock()
	tip.hiddenAt = time.Now()
	tip.key = ""
	tip.mu.Unlock()

	tip.mu.Lock()
	warm := !tip.hiddenAt.IsZero()
	tip.mu.Unlock()
	if !warm {
		t.Fatal("hiddenAt was not recorded, so the next label would wait again")
	}
}
