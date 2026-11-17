package main

import (
	"os"
	"strings"
	"testing"
)

func TestModelLevelsAreLearnedAtStartup(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	body := string(src)
	if !strings.Contains(body, "a.loadAIModels()") {
		t.Fatal("main.go never loads the model list, so 快速 cannot resolve a level before the settings page opens")
	}
	ready := strings.Index(body, "fastDictionary: ready")
	call := strings.Index(body, "a.loadAIModels()")
	if ready < 0 || call < 0 || call > ready {
		t.Errorf("loadAIModels() is not called before the app reports ready (call at %d, ready at %d)", call, ready)
	}
}
