package main

import (
	"os"
	"strings"
	"testing"
)

// TestModelLevelsAreLearnedAtStartup guards a silent failure: 快速 resolves to
// the model's own lowest reasoning level, which only exists after
// FetchAvailableModels has run. When the only caller was showSettings, the
// first translation after launch used whichever model the user had configured
// without knowing its levels, so it sent no reasoning field at all — the slow
// path for a model that reasons by default.
func TestModelLevelsAreLearnedAtStartup(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	body := string(src)
	if !strings.Contains(body, "a.loadMagpieModels()") {
		t.Fatal("main.go never loads the model list, so 快速 cannot resolve a level before the settings page opens")
	}
	// The startup call must sit in the startup path, before the app reports
	// itself ready, not inside a settings-only branch.
	ready := strings.Index(body, "fastDictionary: ready")
	call := strings.Index(body, "a.loadMagpieModels()")
	if ready < 0 || call < 0 || call > ready {
		t.Errorf("loadMagpieModels() is not called before the app reports ready (call at %d, ready at %d)", call, ready)
	}
}
