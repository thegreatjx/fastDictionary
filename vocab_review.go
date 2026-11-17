package main

import (
	"math"
	"sort"
	"time"
)

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

func (e VocabularyEntry) reviewDue(now time.Time) bool {
	return e.ReviewDueAt.IsZero() || !e.ReviewDueAt.After(now)
}

func reviewable(e VocabularyEntry) bool {
	return classifyVocabEntry(e) == vocabCategoryWords
}

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
			return zi
		}
		if !out[i].ReviewDueAt.Equal(out[j].ReviewDueAt) {
			return out[i].ReviewDueAt.Before(out[j].ReviewDueAt)
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

func (s *VocabularyStore) DueCount(now time.Time) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, e := range s.entries {
		if reviewable(e) && e.reviewDue(now) {
			n++
		}
	}
	return n
}

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
