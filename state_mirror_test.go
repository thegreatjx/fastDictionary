package main

import (
	"testing"
	"time"
)

// TestStopAIPassLockedLegacyBehavior 录下改造前的行为，作为状态机显式化的护栏。
// 阶段 A（镜像 → 切读 → 清理）不得改变其中任何一项，包括
// stopAIPassLocked 不清 aiText 这一已知缺陷（见 docs/STATE_MACHINES.md §4.2 U1）。
// 缺陷本身留到阶段 B 单独修，修的时候先写复现测试。
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

// TestRestoreTabLockedLegacyBehavior 录下改造前 U6 的行为，作为阶段 B 的护栏。
// 现状：saveTabLocked 记下 aiBusy / notesBusy，restoreTabLocked 却强制置 false，
// 且不碰 aiGen / notesGen —— 切走再切回时旧异步请求的代次仍然匹配。
// 缺陷本身留到阶段 B 单独修，修的时候先写复现测试。
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

// TestRefineWithAIIndependentGeneration 录下批次 4b 修复后 refineWithAI 的入口行为。
// 修复前精翻复用 a.aiGen 并调 stopAIPassLocked()，aiGen 从 7 增到 8、aiBusy 被清空，
// 正在输出的卡片流静默中断（批次 4a 里这条断言原为 {8, false, "partial"}）。
// 修复后精翻只自增 aiRefineGen，卡片流的 aiGen / aiBusy / aiText 原样保留。
func TestRefineWithAIIndependentGeneration(t *testing.T) {
	a := newTestApp()
	a.store.mu.Lock()
	a.store.cfg.AIEnabled = true
	a.store.cfg.AIURL = "http://127.0.0.1:1" // 无监听端口，请求立即失败
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

	// 等后台精翻 goroutine 收尾，避免测试之间残留写 app 的 goroutine。
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
