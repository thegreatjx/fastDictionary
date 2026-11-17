package main

import "testing"

func TestAutofitLocksWhenWindowDoesNotMove(t *testing.T) {
	var l autofitLoop

	if d, ok := l.decide(-43.9, true, 580, 700); !ok || d != -43.9 {
		t.Fatalf("首次修正应被采纳：delta=%v ok=%v", d, ok)
	}
	if _, ok := l.decide(-43.9, true, 580, 700); ok {
		t.Fatal("窗口没动时应锁定")
	}
	if d, ok := l.decide(20.8, true, 580, 400); !ok || d != 20.8 {
		t.Fatalf("内容变化后应解锁并采纳新方向：delta=%v ok=%v", d, ok)
	}
}

func TestAutofitResumesAfterReset(t *testing.T) {
	var l autofitLoop
	l.decide(-40, true, 500, 600)
	if _, ok := l.decide(-40, true, 500, 600); ok {
		t.Fatal("应锁定")
	}

	l.reset()

	if d, ok := l.decide(40, true, 500, 400); !ok || d != 40 {
		t.Fatalf("reset 后应采纳任何方向的修正：delta=%v ok=%v", d, ok)
	}
}

func TestAutofitIgnoresFramesInsideTheDeadZone(t *testing.T) {
	var l autofitLoop
	l.decide(-43, true, 500, 600)

	if _, ok := l.decide(0, false, 500, 600); ok {
		t.Fatal("死区内的帧不该应用修正")
	}
}
