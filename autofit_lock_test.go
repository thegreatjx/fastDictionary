package main

import "testing"

// TestAutofitLockReleasesWhenContentGrows is the regression: once the loop
// detected an oscillation it stayed locked until fitHeight ran at the end of a
// stream, so a growing answer never grew the window while it streamed.
func TestAutofitLockReleasesWhenContentGrows(t *testing.T) {
	a := &app{}
	a.autofitLocked = true
	a.autofitLockContentH = 100
	a.autofitDir = -43
	a.measuredContentH = 140

	delta, ok := a.autofitDecision(40, true)
	if !ok || delta != 40 {
		t.Fatalf("内容增高 40 DIP 后应解除锁定并采纳修正，得到 delta=%v ok=%v", delta, ok)
	}
	if a.autofitLocked {
		t.Fatal("内容已经变化，振荡锁定必须解除，否则流式回答期间窗口不再长高")
	}
	if a.autofitDir != 40 {
		t.Fatalf("解除锁定后应采纳新方向，autofitDir=%v", a.autofitDir)
	}
}

func TestAutofitLockHoldsWhileContentStable(t *testing.T) {
	a := &app{}
	a.autofitLocked = true
	a.autofitLockContentH = 100
	a.autofitDir = -43
	a.measuredContentH = 102

	if _, ok := a.autofitDecision(40, true); ok {
		t.Fatal("内容没变时振荡锁定必须继续生效")
	}
	if !a.autofitLocked {
		t.Fatal("内容没变却解除了振荡锁定")
	}
}

func TestAutofitLockSurvivesTinyMeasurementNoise(t *testing.T) {
	a := &app{}
	a.autofitLocked = true
	a.autofitLockContentH = 100
	a.autofitDir = 43
	a.measuredContentH = 93

	if _, ok := a.autofitDecision(-40, true); ok {
		t.Fatal("7 DIP 的测量抖动不应解除振荡锁定")
	}
}
