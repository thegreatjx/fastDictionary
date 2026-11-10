package main

import (
	"testing"
	"time"
)

func TestSnapHidesExpiredFeedback(t *testing.T) {
	a := &app{store: newMemoryStore()}
	a.actionSavedMsg = "已保存「alpha」✓"
	a.copiedMsg = "已复制 3 字到剪贴板 ✓"
	a.feedbackText = a.copiedMsg + "\x00" + a.actionSavedMsg
	a.feedbackAt = time.Now().Add(-feedbackTTL - time.Second)

	s := a.snap()
	if s.actionSavedMsg != "" || s.copiedMsg != "" {
		t.Fatalf("过期横幅仍然显示：actionSavedMsg=%q copiedMsg=%q", s.actionSavedMsg, s.copiedMsg)
	}
}

func TestSnapKeepsFreshFeedback(t *testing.T) {
	a := &app{store: newMemoryStore()}
	a.actionSavedMsg = "已保存「alpha」✓"
	a.feedbackText = "\x00" + a.actionSavedMsg
	a.feedbackAt = time.Now()

	s := a.snap()
	if s.actionSavedMsg != a.actionSavedMsg {
		t.Fatalf("新鲜横幅被提前隐藏：%q", s.actionSavedMsg)
	}
}

func TestSnapResetsFeedbackClockOnNewMessage(t *testing.T) {
	a := &app{store: newMemoryStore()}
	a.actionSavedMsg = "已保存「alpha」✓"
	a.feedbackText = "\x00" + a.actionSavedMsg
	a.feedbackAt = time.Now().Add(-feedbackTTL - time.Second)

	a.actionSavedMsg = "已保存「beta」✓"
	s := a.snap()
	if s.actionSavedMsg != "已保存「beta」✓" {
		t.Fatalf("新消息沿用了旧消息的过期时间：%q", s.actionSavedMsg)
	}
	if time.Since(a.feedbackAt) > time.Second {
		t.Fatalf("新消息没有重置计时：%v", time.Since(a.feedbackAt))
	}
}

func TestFeedbackExpiryIsWiredIntoSnapshot(t *testing.T) {
	src := readSourceFile(t, "popup.go")
	if !containsText(src, "a.noteFeedbackLocked()") || !containsText(src, "feedbackTTL") {
		t.Fatal("snap 没有接入反馈横幅过期逻辑")
	}
}
