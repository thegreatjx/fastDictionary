package main

import (
	"context"
	"errors"
	"testing"
)

// TestIsTransientAIError: only failures a short retry can fix are retried.
// A cancelled request was stopped on purpose; resending it is wasted work.
func TestIsTransientAIError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"cancelled", context.Canceled, false},
		{"wrapped cancel", errors.Join(errors.New("stream"), context.Canceled), false},
		{"rate limit", &aiFailure{kind: aiFailureRateLimit}, true},
		{"server", &aiFailure{kind: aiFailureServer}, true},
		{"timeout", &aiFailure{kind: aiFailureTimeout}, true},
		{"auth", &aiFailure{kind: aiFailureAuth}, false},
		{"bad request", &aiFailure{kind: aiFailureBadRequest}, false},
	}
	for _, tc := range cases {
		if got := isTransientAIError(tc.err); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
