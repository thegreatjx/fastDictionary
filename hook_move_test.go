package main

import (
	"sync/atomic"
	"testing"
	"time"
)

// testMoveHook is what the coalescing worker calls in tests.
//
// The worker keeps one reference to OnMouseMove for the life of the process,
// so a test that reassigns OnMouseMove races with a running worker. Install the
// indirection once, at init, and let each test swap the pointer instead.
var testMoveHook atomic.Pointer[func(x, y int32)]

func init() {
	OnMouseMove = func(x, y int32) {
		if fn := testMoveHook.Load(); fn != nil {
			(*fn)(x, y)
		}
	}
}

// TestDispatchMouseMoveCoalescesAndSurvivesPanic covers the two properties the
// coalescing worker has to keep.
//
// Coalescing: pointer moves arrive far faster than a hover needs them, so the
// worker may skip positions — but the one it ends on must be the newest, or the
// hover follows a pointer that has already left.
//
// Surviving a panic: the recover used to wrap the whole loop, so one panic in
// the hover handler ended the worker and every later move was dropped for the
// rest of the session.
func TestDispatchMouseMoveCoalescesAndSurvivesPanic(t *testing.T) {
	got := make(chan [2]int32, 64)
	hook := func(x, y int32) {
		if x == 7 {
			panic("hover handler failed")
		}
		got <- [2]int32{x, y}
	}
	testMoveHook.Store(&hook)
	defer func() {
		testMoveHook.Store(nil)
		disarmChipHover()
	}()
	armChipHover()

	waitFor := func(want [2]int32) {
		t.Helper()
		deadline := time.After(3 * time.Second)
		for {
			select {
			case p := <-got:
				if p == want {
					return
				}
			case <-deadline:
				t.Fatalf("coordinate %v never arrived", want)
			}
		}
	}

	// A burst of positions ends on (12,13): that is the one the hover must see.
	for i := int32(0); i < 10; i++ {
		dispatchMouseMove(i, i)
	}
	dispatchMouseMove(12, 13)
	waitFor([2]int32{12, 13})

	// A panicking move must cost that move only.
	dispatchMouseMove(7, 9)
	dispatchMouseMove(5, 6)
	waitFor([2]int32{5, 6})
}

// TestDispatchMouseMoveIgnoresUnarmedChip pins the gate: with no chip on
// screen the worker must not call the handler at all.
func TestDispatchMouseMoveIgnoresUnarmedChip(t *testing.T) {
	got := make(chan struct{}, 4)
	hook := func(x, y int32) { got <- struct{}{} }
	testMoveHook.Store(&hook)
	defer testMoveHook.Store(nil)
	disarmChipHover()

	dispatchMouseMove(11, 12)
	select {
	case <-got:
		t.Fatal("pointer move reached the handler with no chip on screen")
	case <-time.After(150 * time.Millisecond):
	}
}
