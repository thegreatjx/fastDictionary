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
		{"magpie not running", &aiFailure{kind: aiFailureMagpieDown}, false},
		{"untyped text", errors.New("503 service unavailable"), false},
	}
	for _, tc := range cases {
		if got := isTransientAIError(tc.err); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

// A refused connection to Magpie is its own kind: the popup tells the user to
// start Magpie or switch service, and retrying a stopped process is pointless.
func TestMagpieRefusedIsTyped(t *testing.T) {
	conn := aiConn{Provider: "magpie", Endpoint: "http://127.0.0.1:1/v1", Model: "m"}
	_, err := StreamAI(context.Background(), conn, "s", "u", "", nil)
	var failure *aiFailure
	if !errors.As(err, &failure) || failure.kind != aiFailureMagpieDown {
		t.Fatalf("got %v", err)
	}
	direct := aiConn{Provider: "custom", Endpoint: "http://127.0.0.1:1/v1", Model: "m", Key: "k"}
	_, err = StreamAI(context.Background(), direct, "s", "u", "", nil)
	if !errors.As(err, &failure) || failure.kind != aiFailureNetwork {
		t.Fatalf("direct refused should stay a network failure, got %v", err)
	}
}
