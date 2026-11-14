package main

import (
	"testing"
	"time"
)

// TestRunHookCommandSurvivesPanic checks that one bad hook command costs one
// command, not the hook thread. The hook thread is the only path a selection
// takes into the app, and a windowsgui build has no console: a panic that ends
// the process writes nothing, and the user only sees the shortcut stop working.
func TestRunHookCommandSurvivesPanic(t *testing.T) {
	ran := make(chan struct{})
	hookCommands <- func() { panic("boom") }
	hookCommands <- func() { close(ran) }

	runHookCommand()

	select {
	case <-ran:
	case <-time.After(time.Second):
		t.Fatal("a panicking hook command stopped the commands queued after it")
	}
}
