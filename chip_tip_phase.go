package main

import "time"

type chipTipPhase uint8

const (
	tipHidden chipTipPhase = iota
	tipWaiting
	tipWarm
	tipReady
	tipShown
)

func (p chipTipPhase) String() string {
	switch p {
	case tipHidden:
		return "hidden"
	case tipWaiting:
		return "waiting"
	case tipWarm:
		return "warm"
	case tipReady:
		return "ready"
	case tipShown:
		return "shown"
	default:
		return "unknown"
	}
}

func chipTipIsWarm(hiddenAt, now time.Time) bool {
	return !hiddenAt.IsZero() && now.Sub(hiddenAt) <= chipTipWarm
}

func chipTipIsDue(shown bool, since, now time.Time) bool {
	return !shown && now.Sub(since) >= chipTipDelay
}

func classifyChipTipPhase(inside, shown bool, hiddenAt, since, now time.Time) chipTipPhase {
	switch {
	case !inside:
		return tipHidden
	case shown:
		return tipShown
	case chipTipIsWarm(hiddenAt, now):
		return tipWarm
	case chipTipIsDue(shown, since, now):
		return tipReady
	default:
		return tipWaiting
	}
}
