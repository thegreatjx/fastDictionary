package main

import (
	"testing"
	"time"
)

func TestRefineWithAIDoesNotCancelCardStream(t *testing.T) {
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

	defer func() {
		deadline := time.Now().Add(5 * time.Second)
		for {
			a.mu.Lock()
			done := !a.aiRefining
			a.mu.Unlock()
			if done || time.Now().After(deadline) {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()

	a.refineWithAI()

	a.mu.Lock()
	got := struct {
		gen  requestToken
		busy bool
		text string
	}{a.aiGen, a.aiBusy, a.aiText}
	a.mu.Unlock()
	want := struct {
		gen  requestToken
		busy bool
		text string
	}{7, true, "partial"}
	if got != want {
		t.Fatalf("点「AI 精翻」不应取消正在进行的卡片流: got %+v want %+v", got, want)
	}
}
