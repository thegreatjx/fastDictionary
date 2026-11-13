package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// uiTesterFor renders the settings window, which is the surface the review
// session lives on.
func uiTesterFor(a *app) *ui.Tester {
	return ui.NewTester(a.settingsView, 880, 640)
}

func TestNextReviewSchedule(t *testing.T) {
	now := time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
	cases := []struct {
		name              string
		interval, count   int
		grade             reviewGrade
		wantInterval      int
		wantCount         int
		wantDueAfterHours float64
	}{
		{"new word remembered", 0, 0, reviewGood, 1, 1, 24},
		{"second success", 1, 1, reviewGood, 3, 2, 72},
		{"third success", 3, 2, reviewGood, 7, 3, 7 * 24},
		{"long interval doubles", 10, 5, reviewGood, 23, 6, 23 * 24},
		{"interval is capped", 179, 20, reviewGood, reviewMaxIntervalDays, 21, reviewMaxIntervalDays * 24},
		{"hard from new", 0, 0, reviewHard, 1, 1, 24},
		{"hard extends slightly", 5, 3, reviewHard, 7, 4, 7 * 24},
		{"again resets and returns soon", 30, 9, reviewAgain, 0, 10, 10.0 / 60},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := nextReview(c.interval, c.count, c.grade, now)
			if got.IntervalDays != c.wantInterval {
				t.Fatalf("interval = %d, want %d", got.IntervalDays, c.wantInterval)
			}
			if got.Count != c.wantCount {
				t.Fatalf("count = %d, want %d", got.Count, c.wantCount)
			}
			hours := got.Due.Sub(now).Hours()
			if diff := hours - c.wantDueAfterHours; diff > 0.01 || diff < -0.01 {
				t.Fatalf("due in %.3f h, want %.3f h", hours, c.wantDueAfterHours)
			}
		})
	}
}

func TestDueEntriesIncludesOnlyDueWordsInOrder(t *testing.T) {
	now := time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
	s := &VocabularyStore{entries: []VocabularyEntry{
		{ID: "future", Term: "future", Source: "dict", ReviewDueAt: now.Add(48 * time.Hour)},
		{ID: "past", Term: "past", Source: "dict", ReviewDueAt: now.Add(-time.Hour)},
		{ID: "grammar", Term: "a long sentence from a book", Source: "grammar", ReviewDueAt: now.Add(-time.Hour)},
		{ID: "writing", Term: "polished sentence", Source: "writing", ReviewDueAt: now.Add(-time.Hour)},
		{ID: "new", Term: "new", Source: "dict"},
		{ID: "created", Term: "created", Source: "notes", CreatedAt: now.Add(-72 * time.Hour), ReviewDueAt: now.Add(-72 * time.Hour)},
	}}

	got := s.DueEntries(now)
	want := []string{"new", "created", "past"}
	if len(got) != len(want) {
		t.Fatalf("due = %v, want %v", idsOf(got), want)
	}
	for i := range want {
		if got[i].ID != want[i] {
			t.Fatalf("due order = %v, want %v", idsOf(got), want)
		}
	}
	if n := s.DueCount(now); n != len(want) {
		t.Fatalf("DueCount = %d, want %d", n, len(want))
	}
}

func idsOf(entries []VocabularyEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.ID
	}
	return out
}

func TestApplyReviewPersistsSchedule(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vocabulary.json")
	s := &VocabularyStore{path: path, entries: []VocabularyEntry{
		{ID: "a", Term: "alpha", Definition: "first", Source: "dict", ReviewDueAt: time.Now().Add(-time.Hour)},
	}}
	now := time.Now()
	got, ok := s.ApplyReview("a", reviewGood, now)
	if !ok {
		t.Fatal("ApplyReview reported a missing entry")
	}
	if got.ReviewCount != 1 || got.ReviewIntervalDays != 1 {
		t.Fatalf("schedule = %+v", got)
	}
	if d := got.ReviewDueAt.Sub(now); d < 23*time.Hour || d > 25*time.Hour {
		t.Fatalf("due in %v, want about a day", d)
	}

	reloaded := &VocabularyStore{path: path}
	reloaded.load()
	entries := reloaded.List()
	if len(entries) != 1 {
		t.Fatalf("reloaded %d entries, want 1", len(entries))
	}
	if entries[0].ReviewCount != 1 || entries[0].ReviewIntervalDays != 1 || entries[0].ReviewDueAt.IsZero() {
		t.Fatalf("schedule did not survive the write: %+v", entries[0])
	}
}

func TestAddPutsNewWordInReviewQueue(t *testing.T) {
	s := &VocabularyStore{path: filepath.Join(t.TempDir(), "vocabulary.json")}
	// Phonetic + difficulty keep Add from making a network enrichment call.
	s.Add(VocabularyEntry{ID: "a", Term: "alpha", Phonetic: "/x/", Difficulty: "B1"})

	entry, ok := s.Get("a")
	if !ok {
		t.Fatal("entry was not added")
	}
	if entry.ReviewDueAt.IsZero() {
		t.Fatal("new entry has no review date, so it would never be scheduled")
	}
	if entry.ReviewDueAt.After(time.Now().Add(time.Second)) {
		t.Fatalf("new entry due at %v, want immediately", entry.ReviewDueAt)
	}
}

func TestReviewPageShowsTermThenAnswer(t *testing.T) {
	s := swapVocabStore(t, VocabularyEntry{
		ID: "a", Term: "serendipity", Phonetic: "/ˌserənˈdɪpəti/", PartOfSpeech: "n.", Definition: "机缘巧合", Source: "dict", ReviewDueAt: time.Now().Add(-time.Minute),
	})

	a := newTestApp()
	a.page = "vocabulary"
	a.startVocabReview()
	if !a.vocabReviewing || len(a.vocabReviewIDs) != 1 {
		t.Fatalf("review did not start: %+v", a.vocabReviewIDs)
	}

	tt := uiTesterFor(a)
	if !tt.HasText("serendipity") || !tt.HasText("显示答案") {
		t.Fatalf("card is missing the term or the reveal button: %v", tt.Texts())
	}
	if _, ok := tt.Find("朗读发音"); !ok {
		t.Errorf("review card is missing the pronunciation button: %v", tt.Texts())
	}
	if tt.HasText("机缘巧合") {
		t.Fatalf("answer leaked before reveal: %v", tt.Texts())
	}

	a.vocabReviewRevealed = true
	tt = uiTesterFor(a)
	for _, want := range []string{"机缘巧合", "忘记", "模糊", "记得"} {
		if !tt.HasText(want) {
			t.Errorf("revealed card is missing %q: %v", want, tt.Texts())
		}
	}

	a.gradeVocabReview(reviewGood)
	if a.vocabReviewing && a.vocabReviewIdx != 1 {
		t.Fatalf("grade did not advance the queue: idx=%d", a.vocabReviewIdx)
	}
	if got := s.List()[0]; got.ReviewCount != 1 || got.ReviewIntervalDays != 1 {
		t.Fatalf("grade was not persisted: %+v", got)
	}
}

func TestReviewGradeLabelsStayHuman(t *testing.T) {
	for _, g := range []reviewGrade{reviewAgain, reviewHard, reviewGood} {
		if strings.TrimSpace(g.String()) == "" || g.String() == "unknown" {
			t.Fatalf("grade %d has no stable label", g)
		}
	}
}

// EnrichAll works on a snapshot and writes the result back later. Replacing
// the whole entry reverted anything that happened meanwhile — including a
// review grade — so only the fields enrichment owns may be copied back.
func TestEnrichmentMergeKeepsReviewSchedule(t *testing.T) {
	s := &VocabularyStore{entries: []VocabularyEntry{{
		ID: "a", Term: "alpha", Definition: "first", Source: "dict",
		ReviewCount: 4, ReviewIntervalDays: 9, ReviewDueAt: time.Now().Add(9 * 24 * time.Hour),
	}}}
	stale := s.List()
	stale[0].Phonetic = "/ˈælfə/"
	stale[0].Difficulty = "B1"

	s.mergeEnriched(stale)

	got, ok := s.Get("a")
	if !ok {
		t.Fatal("entry vanished")
	}
	if got.ReviewCount != 4 || got.ReviewIntervalDays != 9 || got.ReviewDueAt.IsZero() {
		t.Fatalf("enrichment reverted the review schedule: %+v", got)
	}
	if got.Phonetic != "/ˈælfə/" || got.Difficulty != "B1" {
		t.Fatalf("enrichment fields were not merged: %+v", got)
	}
}
