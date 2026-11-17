package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

func swapOCRRecognizer(t *testing.T, fake func(context.Context, string, string) (string, error)) func() {
	t.Helper()
	old := recognizeImageFileIn
	recognizeImageFileIn = fake
	return func() { recognizeImageFileIn = old }
}

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

	a.stopScreenshotOCR()
	a.mu.Lock()
	a.ocrText = "哨兵：本轮不应被旧结果覆盖"
	a.source = "哨兵原文"
	a.ocrError = cancelMessage
	a.mu.Unlock()

	close(release)
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
