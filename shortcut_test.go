package main

import (
	"testing"
)

func TestShortcutRegistration(t *testing.T) {
	// In MyGo 0.5.1+, GlobalShortcut.Register dispatches via onMain requiring the App event loop.
	t.Skip("requires interactive app event loop on main thread")
}
