package main

import (
	"math"
	"sort"
	"time"
)

// Vocabulary review uses a small spaced-repetition schedule instead of a
// separate service: one interval and one due date per entry is enough to make
// the notebook a study loop, and it survives export/import as plain JSON.
//
// The grades are the three answers a learner can actually give after seeing
// the card: 忘记 (start over tonight), 模糊 (a little longer), 记得 (double).
// The numbers are intentionally mild; a missed word returning in ten minutes
// is more useful than a week-long penalty.
type reviewGrade uint8

const (
	reviewAgain reviewGrade = iota
	reviewHard
	reviewGood
)

func (g reviewGrade) String() string {
	switch g {
	case reviewAgain:
		return "again"
	case reviewHard:
		return "hard"
	case reviewGood:
		return "good"
	}
	return "unknown"
}

const (
	reviewAgainDelay      = 10 * time.Minute
	reviewMaxIntervalDays = 180
	reviewHardFactor      = 1.3
	reviewGoodFactor      = 2.3
)

// reviewSchedule is the result of grading one card. It is a value type so the
// arithmetic can be tested without a store, a lock or a clock.
type reviewSchedule struct {
	Due          time.Time
	IntervalDays int
	Count        int
}

func nextReview(intervalDays, count int, grade reviewGrade, now time.Time) reviewSchedule {
	count++
	switch grade {
	case reviewAgain:
		return reviewSchedule{Due: now.Add(reviewAgainDelay), IntervalDays: 0, Count: count}
	case reviewHard:
		next := int(math.Round(float64(intervalDays) * reviewHardFactor))
		if next < 1 {
			next = 1
		}
		if next > reviewMaxIntervalDays {
			next = reviewMaxIntervalDays
		}
		return reviewSchedule{Due: now.AddDate(0, 0, next), IntervalDays: next, Count: count}
	default:
		var next int
		switch {
		case intervalDays < 1:
			next = 1
		case intervalDays < 3:
			next = 3
		default:
			next = int(math.Round(float64(intervalDays) * reviewGoodFactor))
		}
		if next > reviewMaxIntervalDays {
			next = reviewMaxIntervalDays
		}
		return reviewSchedule{Due: now.AddDate(0, 0, next), IntervalDays: next, Count: count}
	}
}

// reviewDue reports whether an entry belongs in today's queue. A zero due date
// means the file predates the schedule (or came from CSV/TXT import): those
// entries are treated as due rather than silently excluded from review.
func (e VocabularyEntry) reviewDue(now time.Time) bool {
	return e.ReviewDueAt.IsZero() || !e.ReviewDueAt.After(now)
}

// reviewable keeps grammar and writing notes out of the word queue: their
// "card" is a sentence analysis, not a term-and-definition pair, so grading it
// as a vocabulary item would count the same work twice.
func reviewable(e VocabularyEntry) bool {
	return classifyVocabEntry(e) == vocabCategoryWords
}

// DueEntries returns the reviewable entries that are due, oldest first. The
// store is snapshotted under the read lock so the settings page can paint
// while a background import runs.
func (s *VocabularyStore) DueEntries(now time.Time) []VocabularyEntry {
	s.mu.RLock()
	out := make([]VocabularyEntry, 0, len(s.entries))
	for _, e := range s.entries {
		if reviewable(e) && e.reviewDue(now) {
			out = append(out, e)
		}
	}
	s.mu.RUnlock()

	sort.SliceStable(out, func(i, j int) bool {
		zi, zj := out[i].ReviewDueAt.IsZero(), out[j].ReviewDueAt.IsZero()
		if zi != zj {
			return zi // unscheduled entries come first
		}
		if !out[i].ReviewDueAt.Equal(out[j].ReviewDueAt) {
			return out[i].ReviewDueAt.Before(out[j].ReviewDueAt)
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

// DueCount is the badge number; it takes the same path as DueEntries so the
// button and the queue can never disagree.
func (s *VocabularyStore) DueCount(now time.Time) int {
	return len(s.DueEntries(now))
}

// Get returns one entry by ID. The review queue holds IDs rather than copies,
// so an entry edited or deleted from another surface is seen as it is now.
func (s *VocabularyStore) Get(id string) (VocabularyEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, e := range s.entries {
		if e.ID == id {
			return e, true
		}
	}
	return VocabularyEntry{}, false
}

// ApplyReview schedules the entry after a grade and persists the result. The
// write happens outside the lock for the same reason Delete does: save()
// takes the read lock itself.
func (s *VocabularyStore) ApplyReview(id string, grade reviewGrade, now time.Time) (VocabularyEntry, bool) {
	s.mu.Lock()
	idx := -1
	for i := range s.entries {
		if s.entries[i].ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		s.mu.Unlock()
		return VocabularyEntry{}, false
	}
	e := &s.entries[idx]
	sched := nextReview(e.ReviewIntervalDays, e.ReviewCount, grade, now)
	e.ReviewDueAt = sched.Due
	e.ReviewIntervalDays = sched.IntervalDays
	e.ReviewCount = sched.Count
	out := *e
	s.mu.Unlock()

	s.saveOrLog()
	return out, true
}
