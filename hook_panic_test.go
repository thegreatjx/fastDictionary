package main

import (
	"testing"
	"time"
)

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
