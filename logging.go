package main

import (
	"fmt"
	"log"
	"os"
	"runtime/debug"
)

// The log is the only diagnostic channel a windowsgui build has: there is no
// console, and the settings page points the user at this file when something
// goes wrong. It used to be truncated on every start, so restarting after a
// crash erased the evidence before anyone could read it.
const (
	logMaxBytes     int64 = 2 << 20 // rotate at 2 MiB
	logBackupSuffix       = ".1"
)

// openLogFile opens the log for appending and rotates it once it passes
// maxBytes. One previous file is kept, which is enough to cover "the crash
// happened just before I restarted" without turning the log directory into a
// second data store. A failed rotation is not fatal: appending to a large
// file still beats losing the line that explains the failure.
func openLogFile(path string, maxBytes int64) (*os.File, error) {
	if info, err := os.Stat(path); err == nil && info.Size() >= maxBytes {
		backup := path + logBackupSuffix
		_ = os.Remove(backup)
		_ = os.Rename(path, backup)
	}
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
}

// recoverLog is deferred at the top of a background goroutine. A panic there
// used to take the whole tray process down without writing a single line,
// because a windowsgui binary has no stderr. One bad response should cost one
// job, not the session; the stack in the log is what makes the next fix
// possible.
func recoverLog(where string) {
	if r := recover(); r != nil {
		log.Printf("fastDictionary: panic in %s: %v\n%s", where, r, debug.Stack())
	}
}

// logText renders user content for the log. Selected text and OCR output are
// private — a translator sees passwords, medical results and half-written
// mail — so only the length is recorded unless the user opts in from 关于.
func (a *app) logText(s string, n int) string {
	if a != nil && a.store != nil && a.store.Get().LogSelectionText {
		return fmt.Sprintf("%q", truncateFor(s, n))
	}
	return fmt.Sprintf("<%d 字符，未记录>", len([]rune(s)))
}
