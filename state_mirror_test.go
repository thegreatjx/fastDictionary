package main

import (
	"testing"
	"time"
)

func TestStopAIPassLockedLegacyBehavior(t *testing.T) {
	a := newTestApp()
	a.mu.Lock()
	a.aiGen = 7
	a.aiBusy = true
	a.aiText = "partial"
	a.aiRefining = true
	a.aiRefineErr = "old error"
	a.stopAIPassLocked()
	got := struct {
		gen            requestToken
		busy, refining bool
		err, text      string
	}{a.aiGen, a.aiBusy, a.aiRefining, a.aiRefineErr, a.aiText}
	a.mu.Unlock()

	want := struct {
		gen            requestToken
		busy, refining bool
		err, text      string
	}{8, false, false, "", "partial"}
	if got != want {
		t.Fatalf("stopAIPassLocked 改变了既有行为: got %+v want %+v", got, want)
	}
}

func TestRestoreTabLockedLegacyBehavior(t *testing.T) {
	a := newTestApp()
	a.mu.Lock()
	a.mode = modeGrammar
	a.aiGen = 7
	a.notesGen = 3
	a.aiBusy = true
	a.notesBusy = true
	a.aiText = "partial"
	a.target = "machine"
	a.saveTabLocked()
	st := a.tabStates[modeGrammar]
	if st == nil || !st.aiBusy || !st.notesBusy {
		a.mu.Unlock()
		t.Fatalf("saveTabLocked 应当记下进行中的 AI / 笔记请求")
	}
	a.restoreTabLocked(st)
	got := struct {
		aiGen, notesGen   requestToken
		aiBusy, notesBusy bool
		aiText, target    string
	}{a.aiGen, a.notesGen, a.aiBusy, a.notesBusy, a.aiText, a.target}
	a.mu.Unlock()

	want := struct {
		aiGen, notesGen   requestToken
		aiBusy, notesBusy bool
		aiText, target    string
	}{7, 3, false, false, "partial", "machine"}
	if got != want {
		t.Fatalf("restoreTabLocked 改变了既有行为: got %+v want %+v", got, want)
	}
}

func TestRefineWithAIIndependentGeneration(t *testing.T) {
	a := newTestApp()
	a.store.mu.Lock()
	a.store.cfg.AIEnabled = true
	a.store.cfg.AIURL = "http://127.0.0.1:1"
	a.store.mu.Unlock()

	a.mu.Lock()
	a.aiGen = 7
	a.aiBusy = true
	a.aiText = "partial"
	a.mu.Unlock()

	a.refineWithAI()

	a.mu.Lock()
	got := struct {
		gen, refineGen requestToken
		busy           bool
		text           string
	}{a.aiGen, a.aiRefineGen, a.aiBusy, a.aiText}
	a.mu.Unlock()
	want := struct {
		gen, refineGen requestToken
		busy           bool
		text           string
	}{7, 1, true, "partial"}
	if got != want {
		t.Fatalf("refineWithAI 改变了既有行为: got %+v want %+v", got, want)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		a.mu.Lock()
		done := !a.aiRefining && a.aiRefineCancel == nil
		a.mu.Unlock()
		if done || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
}
