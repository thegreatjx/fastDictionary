package main

import "testing"

// TestAutofitLockReleasesWhenContentGrows is the regression: once the loop
// detected an oscillation it stayed locked until fitHeight ran at the end of a
// stream, so a growing answer never grew the window while it streamed.
func TestAutofitLockReleasesWhenContentGrows(t *testing.T) {
	l := autofitLoop{locked: true, lockContent: 100}

	delta, ok := l.decide(40, true, 500, 140)
	if !ok || delta != 40 {
		t.Fatalf("内容增高 40 DIP 后应解除锁定并采纳修正，得到 delta=%v ok=%v", delta, ok)
	}
	if l.locked {
		t.Fatal("内容已经变化，锁定必须解除，否则流式回答期间窗口不再长高")
	}
}

func TestAutofitLockHoldsWhileContentStable(t *testing.T) {
	l := autofitLoop{locked: true, lockContent: 100}

	if _, ok := l.decide(40, true, 500, 102); ok {
		t.Fatal("内容没变时锁定必须继续生效")
	}
	if !l.locked {
		t.Fatal("内容没变却解除了锁定")
	}
}

func TestAutofitLockSurvivesTinyMeasurementNoise(t *testing.T) {
	l := autofitLoop{locked: true, lockContent: 100}

	if _, ok := l.decide(-40, true, 500, 93); ok {
		t.Fatal("7 DIP 的测量抖动不应解除锁定")
	}
}
