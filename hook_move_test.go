package main

import (
	"sync/atomic"
	"testing"
	"time"
)

var testMoveHook atomic.Pointer[func(x, y int32)]

func init() {
	OnMouseMove = func(x, y int32) {
		if fn := testMoveHook.Load(); fn != nil {
			(*fn)(x, y)
		}
	}
}

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

	for i := int32(0); i < 10; i++ {
		dispatchMouseMove(i, i)
	}
	dispatchMouseMove(12, 13)
	waitFor([2]int32{12, 13})

	dispatchMouseMove(7, 9)
	dispatchMouseMove(5, 6)
	waitFor([2]int32{5, 6})
}

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
