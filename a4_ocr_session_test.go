package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

// swapOCRRecognizer 把 OCR 引擎换成测试可控的实现，返回恢复函数。
// 截图识别会启动 PowerShell，测试里不能真跑；这个接缝让代次与取消路径
// 可以在不依赖 Windows OCR 的前提下被确定性地验证。
func swapOCRRecognizer(t *testing.T, fake func(context.Context, string, string) (string, error)) func() {
	t.Helper()
	old := recognizeImageFileIn
	recognizeImageFileIn = fake
	return func() { recognizeImageFileIn = old }
}

// waitForOCRGoroutine 等一条 OCR goroutine 落到终态。
// 错误分支会把 ocrError 写成 err.Error()，所以它是可观察的完成信号。
func waitForOCRGoroutine(t *testing.T, a *app, wantErr string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		a.mu.Lock()
		got := a.ocrError
		a.mu.Unlock()
		if got == wantErr {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("OCR goroutine 没有在 5s 内收尾（ocrError=%q，期望 %q）", func() string {
		a.mu.Lock()
		defer a.mu.Unlock()
		return a.ocrError
	}(), wantErr)
}

// TestScreenshotOCRInvalidatesInFlightTranslation 复现 A4 的「旧结果写回」：
// 新截图是一张新卡片，openPinnedScreenshot / recognizeScreenshot 会清空
// source 与 target，但修复前都不让 translateGen 失效。上一轮 OCR 成功后
// translateKeepingLines 启动的翻译 goroutine 回来时 `gen == a.translateGen`
// 仍成立，就把旧译文 / 引擎 / 词典写进这张新卡片。4d 已为 switchMode 与
// 两个 openPopup Reset 段补过同样的自增，这里是第四处入口。
func TestScreenshotOCRInvalidatesInFlightTranslation(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	const failText = "识别已停止"
	restore := swapOCRRecognizer(t, func(ctx context.Context, path, lang string) (string, error) {
		close(entered)
		<-release
		return "", errors.New(failText)
	})
	defer restore()

	a := newTestApp()
	a.mu.Lock()
	a.isScreenshotMode = true
	a.mode = modeTranslate
	a.busy = true
	a.translateGen = 5
	a.source = "上一轮原文"
	a.target = "上一轮译文"
	a.mu.Unlock()

	a.recognizeScreenshot("fake.png")

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("OCR goroutine 没有启动")
	}

	a.mu.Lock()
	got := a.translateGen
	a.mu.Unlock()
	if got == 5 {
		t.Fatal("新截图没有让在飞的机器翻译代次失效：旧译文仍会写进这张新卡片")
	}

	close(release)
	waitForOCRGoroutine(t, a, failText)
}

// TestCanceledScreenshotOCRDropsLateResult 是 A4 的 characterization：
// stopScreenshotOCR 自增 ocrGeneration 并 cancel ctx，goroutine 的
// `generation != a.ocrGeneration` 会丢弃取消后才到达的旧识别结果。
// 这条测试把「取消不写回」钉成规格，避免后续重构把代次校验去掉。
func TestCanceledScreenshotOCRDropsLateResult(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	const lateText = "过期识别结果"
	const cancelMessage = "已取消识别。可以重试或重新截图。"
	restore := swapOCRRecognizer(t, func(ctx context.Context, path, lang string) (string, error) {
		close(entered)
		<-release
		return lateText, nil
	})
	defer restore()

	a := newTestApp()
	a.mu.Lock()
	a.isScreenshotMode = true
	a.mode = modeTranslate
	a.ocrText = "哨兵：本轮不应被旧结果覆盖"
	a.source = "哨兵原文"
	a.ocrError = cancelMessage
	a.mu.Unlock()

	a.recognizeScreenshot("fake.png")

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("OCR goroutine 没有启动")
	}

	// 模拟用户点「取消」：stopScreenshotOCR 失效代次，按钮再写取消文案。
	a.stopScreenshotOCR()
	a.mu.Lock()
	a.ocrText = "哨兵：本轮不应被旧结果覆盖"
	a.source = "哨兵原文"
	a.ocrError = cancelMessage
	a.mu.Unlock()

	close(release)
	// 给 goroutine 足够时间跑完写回逻辑；若代次校验被去掉，哨兵会被改写。
	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		a.mu.Lock()
		text, source, errText := a.ocrText, a.source, a.ocrError
		a.mu.Unlock()
		if text == lateText || source == lateText || errText != cancelMessage {
			t.Fatalf("取消后旧识别结果写回了卡片：ocrText=%q source=%q ocrError=%q", text, source, errText)
		}
		time.Sleep(2 * time.Millisecond)
	}
}
