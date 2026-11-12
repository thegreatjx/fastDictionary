package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOneNameEverywhere pins the rule this rename established: the app has
// exactly two names, fastDictionary on disk and fast词典 on screen, and no
// third one survives anywhere.
//
// The name is not cosmetic. mygo derives the user data directory, the log
// directory and the single instance lock from the executable name, so a
// stale name in the source is a path that points somewhere the app never
// writes. That is how the settings page ended up showing a directory the
// store had stopped using.
func TestOneNameEverywhere(t *testing.T) {
	if appName != "fastDictionary" {
		t.Fatalf("appName = %q; the exe name and the module name have to agree", appName)
	}
	if appTitle != "fast词典" {
		t.Fatalf("appTitle = %q; the interface name is fast词典", appTitle)
	}

	// The module name decides what `go build .` produces, and the produced
	// file name decides the data directory. A drift here is the bug that
	// made the shortcut launch a binary reading a different config.
	gomod, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	// Git on Windows may check the file out with CRLF; the declaration is
	// still the same line.
	gomodText := strings.ReplaceAll(string(gomod), "\r\n", "\n")
	if !strings.Contains(gomodText, "module "+appName+"\n") {
		t.Fatalf("go.mod does not declare `module %s`", appName)
	}

	// No source file may still carry the old name. The migration list no
	// longer includes it: there is no old data to rescue.
	oldName := "fast" + "pot"
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || name == "naming_test.go" {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for i, line := range strings.Split(string(src), "\n") {
			if !strings.Contains(strings.ToLower(line), oldName) {
				continue
			}
			t.Errorf("%s:%d still mentions %s; the old name has to go", name, i+1, oldName)
		}
	}
}

// TestLegacyDataDirsCoverEveryOldName pins the migration list against the
// names this app actually used on disk.
func TestLegacyDataDirsCoverEveryOldName(t *testing.T) {
	for _, want := range []string{"Dictionary", "fast词典"} {
		found := false
		for _, got := range legacyDataDirs {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("legacyDataDirs = %v, missing %q: settings written under that name would be orphaned", legacyDataDirs, want)
		}
	}
	for _, got := range legacyDataDirs {
		if got == appName {
			t.Fatalf("legacyDataDirs contains the current name %q; it would copy onto itself", appName)
		}
	}
}

// TestCopyUserDataBringsTheNotebook is the behaviour the user actually
// notices: after a rename the words they saved are still there.
func TestCopyUserDataBringsTheNotebook(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	for name, body := range map[string]string{
		"config.json":     `{"hotkey_selection":"Ctrl+Alt+F"}`,
		"vocabulary.json": `[{"term":"serendipity"}]`,
		"notebook.html":   "<html></html>",
	} {
		if err := os.WriteFile(filepath.Join(src, name), []byte(body), 0o644); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	// Disposable directories must not travel: a stale cache copied forward
	// is indistinguishable from a fresh one.
	if err := os.MkdirAll(filepath.Join(src, "cache"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "cache", "shot.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	n, err := copyUserData(src, dst)
	if err != nil {
		t.Fatalf("copyUserData: %v", err)
	}
	if n != 3 {
		t.Fatalf("copied %d files, want 3", n)
	}
	got, err := os.ReadFile(filepath.Join(dst, "vocabulary.json"))
	if err != nil {
		t.Fatalf("the notebook did not arrive: %v", err)
	}
	if !strings.Contains(string(got), "serendipity") {
		t.Fatalf("notebook content = %q", got)
	}
	if _, err := os.Stat(filepath.Join(dst, "cache")); err == nil {
		t.Fatal("copyUserData copied the cache directory; it must be left behind")
	}
}

// TestCopyUserDataToleratesPartialLegacyDirs covers the real directory on
// this machine: it has a config and a notebook but no vocabulary file.
func TestCopyUserDataToleratesPartialLegacyDirs(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "config.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err := copyUserData(src, dst)
	if err != nil {
		t.Fatalf("a missing vocabulary.json must not abort the migration: %v", err)
	}
	if n != 1 {
		t.Fatalf("copied %d files, want 1", n)
	}
}

// TestVocabPathFollowsTheAppName pins that the notebook does not point at a
// directory named after some other build.
func TestVocabPathFollowsTheAppName(t *testing.T) {
	p := defaultVocabPath()
	if !strings.HasSuffix(p, "vocabulary.json") {
		t.Fatalf("defaultVocabPath() = %q, not a vocabulary.json file", p)
	}
}
