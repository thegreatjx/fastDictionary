package main

import (
	"log"
	"strings"
	"time"
)

func DefaultSelectionBlocklist() []string {
	return []string{
		"snipaste.exe",
		"sharex.exe",
		"screensketch.exe",
		"snippingtool.exe",
		"screenclip.exe",
		"pixpin.exe",
		"greenshot.exe",
	}
}

func (a *app) selectionBlocklist() []string {
	list := a.store.Get().SelectionBlocklist
	if len(list) == 0 {
		return DefaultSelectionBlocklist()
	}
	return list
}

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

func (a *app) selectionPaused() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return !a.pausedUntil.IsZero() && time.Now().Before(a.pausedUntil)
}

func (a *app) pauseSelection(d time.Duration) {
	a.mu.Lock()
	a.pausedUntil = time.Now().Add(d)
	a.mu.Unlock()
	a.gestureGate.reset()
	log.Printf("fastDictionary: selection trigger paused for %s", d)
	go a.rebuildTray()
}

func (a *app) resumeSelection() {
	a.mu.Lock()
	a.pausedUntil = time.Time{}
	a.mu.Unlock()
	a.gestureGate.reset()
	log.Printf("fastDictionary: selection trigger resumed")
	go a.rebuildTray()
}

func (a *app) selectionPausedUntil() time.Time {
	a.mu.Lock()
	defer a.mu.Unlock()
	if time.Now().Before(a.pausedUntil) {
		return a.pausedUntil
	}
	return time.Time{}
}

func (a *app) shouldIgnoreSelection(owner string) (bool, string) {
	if a.selectionPaused() {
		return true, "paused"
	}
	if list := a.selectionBlocklist(); appIsBlocked(owner, list) {
		return true, "blocked:" + owner
	}
	if owner == "" {
		if fg := foregroundAppName(); appIsBlocked(fg, a.selectionBlocklist()) {
			return true, "blocked-foreground:" + fg
		}
	}
	return false, ""
}
