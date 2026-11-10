package main

import (
	"testing"
	"time"
)

// 批次 6d 源码契约：onChipPointer 的两个隐式时间窗口判断应收进具名谓词，
// 不再以内联布尔表达式散落。
func TestChipTipUsesNamedPredicates(t *testing.T) {
	src := readSourceFile(t, "selicon.go")
	if !containsText(src, "chipTipIsWarm(") {
		t.Fatal("warm 窗口判断仍未收进 chipTipIsWarm")
	}
	if !containsText(src, "chipTipIsDue(") {
		t.Fatal("delay 判断仍未收进 chipTipIsDue")
	}
	if countOccurrences(src, "now.Sub(t.hiddenAt) <= chipTipWarm") > 0 {
		t.Fatal("warm 窗口仍是内联裸表达式")
	}
	if countOccurrences(src, "now.Sub(t.since) >= chipTipDelay") > 0 {
		t.Fatal("delay 判断仍是内联裸表达式")
	}
}

// 零行为变化判据：遍历时间组合，两个具名谓词的结果必须与原内联表达式
// 逐项一致。这是把「收进谓词」钉成纯显式化的核心证据。
func TestChipTipTimeWindowsMatchLegacy(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	hiddenCases := []struct {
		name string
		at   time.Time
	}{
		{"zero", time.Time{}},
		{"just-now", base.Add(-100 * time.Millisecond)},
		{"inside-warm", base.Add(-chipTipWarm)},
		{"outside-warm", base.Add(-chipTipWarm - time.Millisecond)},
		{"far-past", base.Add(-time.Hour)},
		{"future", base.Add(time.Hour)},
	}
	sinceCases := []struct {
		name string
		at   time.Time
	}{
		{"zero", time.Time{}},
		{"just-now", base.Add(-10 * time.Millisecond)},
		{"at-delay", base.Add(-chipTipDelay)},
		{"past-delay", base.Add(-chipTipDelay - time.Millisecond)},
		{"far-past", base.Add(-time.Hour)},
	}
	for _, h := range hiddenCases {
		legacyWarm := !h.at.IsZero() && base.Sub(h.at) <= chipTipWarm
		if got := chipTipIsWarm(h.at, base); got != legacyWarm {
			t.Fatalf("chipTipIsWarm(%s) = %v, legacy = %v", h.name, got, legacyWarm)
		}
	}
	for _, sc := range sinceCases {
		for _, shown := range []bool{false, true} {
			label := sc.name
			if shown {
				label += "/shown"
			} else {
				label += "/!shown"
			}
			legacyDue := !shown && base.Sub(sc.at) >= chipTipDelay
			if got := chipTipIsDue(shown, sc.at, base); got != legacyDue {
				t.Fatalf("chipTipIsDue(%s) = %v, legacy = %v", label, got, legacyDue)
			}
		}
	}
}

func TestClassifyChipTipPhase(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	cases := []struct {
		name     string
		inside   bool
		shown    bool
		hiddenAt time.Time
		since    time.Time
		want     chipTipPhase
	}{
		{"outside", false, false, time.Time{}, base, tipHidden},
		{"shown", true, true, time.Time{}, base, tipShown},
		{"warm-beats-due", true, false, base.Add(-100 * time.Millisecond), base.Add(-time.Hour), tipWarm},
		{"ready", true, false, time.Time{}, base.Add(-chipTipDelay), tipReady},
		{"waiting", true, false, time.Time{}, base.Add(-10 * time.Millisecond), tipWaiting},
		{"shown-beats-warm", true, true, base.Add(-100 * time.Millisecond), base, tipShown},
	}
	for _, tc := range cases {
		if got := classifyChipTipPhase(tc.inside, tc.shown, tc.hiddenAt, tc.since, base); got != tc.want {
			t.Fatalf("classifyChipTipPhase(%s) = %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestChipTipPhaseString(t *testing.T) {
	cases := []struct {
		p    chipTipPhase
		want string
	}{
		{tipHidden, "hidden"},
		{tipWaiting, "waiting"},
		{tipWarm, "warm"},
		{tipReady, "ready"},
		{tipShown, "shown"},
		{chipTipPhase(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.p.String(); got != tc.want {
			t.Fatalf("chipTipPhase(%d).String() = %q, want %q", tc.p, got, tc.want)
		}
	}
}
