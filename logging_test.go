package main

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The log used to be truncated on every start: restarting after a crash
// erased exactly the lines that explained it. Rotation keeps one previous
// file and appends to the current one.
func TestOpenLogFileRotatesOnceAtLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fastDictionary.log")
	if err := os.WriteFile(path, []byte("old crash\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	f, err := openLogFile(path, 4)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("new run\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()

	backup, err := os.ReadFile(path + logBackupSuffix)
	if err != nil {
		t.Fatalf("rotated file missing: %v", err)
	}
	if string(backup) != "old crash\n" {
		t.Fatalf("backup = %q, want the pre-rotation content", backup)
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != "new run\n" {
		t.Fatalf("current log = %q, want only the appended line", current)
	}

	// A second start below the limit must append, not rotate again.
	f2, err := openLogFile(path, 4096)
	if err != nil {
		t.Fatal(err)
	}
	f2.WriteString("next run\n")
	f2.Close()
	current, _ = os.ReadFile(path)
	if !strings.Contains(string(current), "new run") || !strings.Contains(string(current), "next run") {
		t.Fatalf("log was not appended to: %q", current)
	}
}

func TestLogTextHidesSelectionByDefault(t *testing.T) {
	a := newTestApp()
	got := a.logText("my private sentence", 40)
	if strings.Contains(got, "private") {
		t.Fatalf("selected text reached the log without opt-in: %q", got)
	}
	if !strings.Contains(got, "字符") {
		t.Fatalf("redacted log text should keep the length, got %q", got)
	}

	a.store.Update(func(c *Config) { c.LogSelectionText = true })
	got = a.logText(strings.Repeat("x", 50), 10)
	if !strings.Contains(got, strings.Repeat("x", 10)) {
		t.Fatalf("opt-in should record the text, got %q", got)
	}
	if strings.Contains(got, strings.Repeat("x", 11)) {
		t.Fatalf("opt-in text should still be truncated, got %q", got)
	}
}

func TestRecoverLogSwallowsBackgroundPanic(t *testing.T) {
	// The recovered stack is written through the standard logger; silence it
	// so a passing test does not print a panic that never happened.
	prev := log.Writer()
	log.SetOutput(io.Discard)
	t.Cleanup(func() { log.SetOutput(prev) })

	done := make(chan struct{})
	go func() {
		defer close(done)
		defer recoverLog("logging_test.go:panic")
		panic("boom")
	}()
	<-done
}
