// Selection trigger guard.
//
// A global mouse hook cannot tell "the user selected text" from "the user
// dragged something in a screenshot tool": both are a press, a long drag and
// a release. Without a guard, fastDictionary reacts to every drag in every
// application — which is what made it fight Snipaste: a screenshot region is
// exactly that gesture, and the capture fallback (clear clipboard, send
// Ctrl+C) tears the snip session down.
//
// The guard has two parts:
//
//   - a blocklist of applications where a drag is never a selection;
//   - a pause switch, for the cases the blocklist cannot know about.
package main

import (
	"log"
	"strings"
	"time"
)

// DefaultSelectionBlocklist is the shipped list of applications where a drag
// is a gesture of the application itself, not a text selection. Names are
// matched against the lowercased executable name, without the path.
func DefaultSelectionBlocklist() []string {
	return []string{
		"snipaste.exe",     // screenshot tool: drag = snip region
		"sharex.exe",       // screenshot tool: drag = region
		"screensketch.exe", // Windows 11 snipping tool
		"snippingtool.exe", // Windows 10 snipping tool
		"screenclip.exe",   // Windows 11 snipping overlay
		"pixpin.exe",       // screenshot tool (Snipaste successor)
		"greenshot.exe",    // screenshot tool
	}
}

// selectionBlocklist returns the configured blocklist, falling back to the
// shipped one when the config predates the field or was emptied by hand.
func (a *app) selectionBlocklist() []string {
	list := a.store.Get().SelectionBlocklist
	if len(list) == 0 {
		return DefaultSelectionBlocklist()
	}
	return list
}

// appIsBlocked reports whether exeName is in the blocklist. The comparison is
// on the executable name only: the same tool can be installed anywhere.
func appIsBlocked(exeName string, list []string) bool {
	name := strings.ToLower(strings.TrimSpace(exeName))
	if name == "" {
		return false
	}
	for _, entry := range list {
		e := strings.ToLower(strings.TrimSpace(entry))
		if e == "" {
			continue
		}
		if e == name || strings.TrimSuffix(e, ".exe") == strings.TrimSuffix(name, ".exe") {
			return true
		}
	}
	return false
}

// selectionPaused reports whether the user paused the selection trigger.
func (a *app) selectionPaused() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return !a.pausedUntil.IsZero() && time.Now().Before(a.pausedUntil)
}

// pauseSelection stops the trigger for d. It is the escape hatch for an
// application the blocklist does not know yet.
func (a *app) pauseSelection(d time.Duration) {
	a.mu.Lock()
	a.pausedUntil = time.Now().Add(d)
	a.mu.Unlock()
	log.Printf("fastDictionary: selection trigger paused for %s", d)
	// The tray rebuild hops onto the main thread and blocks until it has
	// run; doing it in the background keeps this callable from anywhere,
	// including tests that have no message loop.
	go a.rebuildTray()
}

// resumeSelection turns the trigger back on.
func (a *app) resumeSelection() {
	a.mu.Lock()
	a.pausedUntil = time.Time{}
	a.mu.Unlock()
	// A pause is a deliberate break: the gesture after it must not be judged
	// against the one from before it.
	a.gestureGate.reset()
	log.Printf("fastDictionary: selection trigger resumed")
	go a.rebuildTray()
}

// selectionPausedUntil returns the pause deadline, for the tray label.
func (a *app) selectionPausedUntil() time.Time {
	a.mu.Lock()
	defer a.mu.Unlock()
	if time.Now().Before(a.pausedUntil) {
		return a.pausedUntil
	}
	return time.Time{}
}

// shouldIgnoreSelection reports whether a gesture that just finished in the
// application named owner must be ignored. It is the single place where the
// trigger policy is decided, so the hooks stay dumb.
func (a *app) shouldIgnoreSelection(owner string) (bool, string) {
	if a.selectionPaused() {
		return true, "paused"
	}
	if list := a.selectionBlocklist(); appIsBlocked(owner, list) {
		return true, "blocked:" + owner
	}
	// A screenshot tool that keeps its overlay in a different process (or
	// hides its own window name) is caught here: while the foreground
	// application is one the user marked as a snip tool, the trigger stays
	// quiet no matter which window the drag ended in.
	if owner == "" {
		if fg := foregroundAppName(); appIsBlocked(fg, a.selectionBlocklist()) {
			return true, "blocked-foreground:" + fg
		}
	}
	return false, ""
}
