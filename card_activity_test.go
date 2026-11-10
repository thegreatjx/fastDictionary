package main

import (
	"reflect"
	"testing"
)

func TestCardActivityPrecedence(t *testing.T) {
	cases := []struct {
		name string
		s    snapshot
		want cardActivity
	}{
		{"idle", snapshot{}, cardActivityIdle},
		{"translating", snapshot{busy: true}, cardActivityTranslating},
		{"streaming", snapshot{aiBusy: true}, cardActivityStreaming},
		{"refining", snapshot{aiRefining: true}, cardActivityRefining},
		{"notes", snapshot{notesBusy: true}, cardActivityLoadingNotes},
		{"ocr wins over everything", snapshot{ocrBusy: true, busy: true, aiBusy: true, aiRefining: true, notesBusy: true}, cardActivityRecognizing},
		{"stream wins over refine", snapshot{aiBusy: true, aiRefining: true}, cardActivityStreaming},
		{"translate wins over notes", snapshot{busy: true, notesBusy: true}, cardActivityTranslating},
	}
	for _, c := range cases {
		if got := cardActivityOf(c.s); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestCardPendingWorkMatchesLegacyFlags(t *testing.T) {
	// The old readers each spelled the OR out by hand. Exhaust all 32
	// combinations to prove the single predicate is exactly equivalent.
	for mask := 0; mask < 32; mask++ {
		s := snapshot{
			busy:       mask&1 != 0,
			aiBusy:     mask&2 != 0,
			aiRefining: mask&4 != 0,
			notesBusy:  mask&8 != 0,
			ocrBusy:    mask&16 != 0,
		}
		want := s.busy || s.aiBusy || s.aiRefining || s.notesBusy || s.ocrBusy
		if got := cardHasPendingWork(s); got != want {
			t.Fatalf("mask %05b: got %v want %v", mask, got, want)
		}
	}
}

func TestCardActivityString(t *testing.T) {
	want := map[cardActivity]string{
		cardActivityIdle:         "idle",
		cardActivityTranslating:  "translating",
		cardActivityStreaming:    "streaming",
		cardActivityRefining:     "refining",
		cardActivityLoadingNotes: "loading_notes",
		cardActivityRecognizing:  "recognizing",
		cardActivity(99):         "unknown",
	}
	for p, w := range want {
		if got := p.String(); got != w {
			t.Errorf("%d: got %q want %q", p, got, w)
		}
	}
}

// TestRequestTokenIsNamedType guards the compile-time half of C6: the four
// generation counters used to be plain uint64, so a token from one kind could
// be compared with another without the compiler noticing.
func TestRequestTokenIsNamedType(t *testing.T) {
	var tok requestToken = 1
	if tok != 1 {
		t.Fatal("requestToken 应可容纳普通计数值")
	}
	if name := reflect.TypeOf(tok).Name(); name != "requestToken" {
		t.Fatalf("代次应为具名类型，实际为 %q", name)
	}
	if kind := reflect.TypeOf(tok).Kind(); kind != reflect.Uint64 {
		t.Fatalf("requestToken 底层应为 uint64，实际为 %v", kind)
	}
}
