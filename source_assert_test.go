package main

import (
	"os"
	"strings"
	"testing"
)

// readSourceFile reads a source file next to the test, for the structural
// assertions the runtime behaviour cannot express on a headless machine.
func readSourceFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

func countOccurrences(s, sub string) int { return strings.Count(s, sub) }

func containsText(s, sub string) bool { return strings.Contains(s, sub) }
