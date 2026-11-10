package main

import (
	"testing"
	"time"
)

// TestRefineWithAIDoesNotCancelCardStream 复现 U1 的核心缺陷：
// 「AI 精翻」与 AI 卡片流共用 a.aiGen。卡片流以 gen=7 判定，
// 点精翻时 refineWithAI 调 stopAIPassLocked() 把 aiGen 增到 8，
// 卡片的流式回调与收尾因此被丢弃，输出停在中途且没有提示。
//
// 修复后精翻使用独立代次，卡片流的 aiGen / aiBusy / aiText 保持不变。
func TestRefineWithAIDoesNotCancelCardStream(t *testing.T) {
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

	// 等后台精翻 goroutine 收尾，避免测试之间残留写 app 的 goroutine。
	// 用 defer，复现阶段的 Fatalf 也不会把 goroutine 留给下一个测试。
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
