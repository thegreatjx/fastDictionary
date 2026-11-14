package main

import (
	"strings"
	"testing"
)

// TestHideChipTakesTheWindowDown 是残留窗口的守卫：sel.shown 与窗口真实
// 可见性可能不一致（早先的 Hide 之后又落下一个 ShowInactive），隐藏必须以
// 窗口为准，不能以标志为准，否则 chip 会永远留在屏幕上。
func TestHideChipTakesTheWindowDown(t *testing.T) {
	src := readSourceFile(t, "selicon.go")
	start := strings.Index(src, "func (a *app) hideSelIcon()")
	if start < 0 {
		t.Fatal("找不到 hideSelIcon")
	}
	body := src[start:]
	body = body[:strings.Index(body, "\n}\n")]
	if containsText(body, "was :=") || containsText(body, "&& was") {
		t.Fatal("hideSelIcon 仍以 sel.shown 作为隐藏前提，状态不一致时窗口会留在屏幕上")
	}
	if !containsText(body, "win.Hide()") {
		t.Fatal("hideSelIcon 没有隐藏窗口")
	}
}

// TestChipSampleIsGenerationScoped 钉住 chip 的 pointer 采样只对当次显示有效：
// 采样启动后 chip 被收起、或收起后很快又显示，旧采样都不能再碰新会话。
func TestChipSampleIsGenerationScoped(t *testing.T) {
	a := &app{}
	a.mu.Lock()
	a.sel.shown = true
	a.sel.gen = 1
	a.mu.Unlock()

	if !a.chipSampleStillCurrent(1) {
		t.Fatal("当前会话的采样被判为过期")
	}
	a.mu.Lock()
	a.sel.gen = 2 // 收起后又显示
	a.mu.Unlock()
	if a.chipSampleStillCurrent(1) {
		t.Fatal("旧会话的采样仍被采纳，可能把提示抬到新 chip 上")
	}
	if !a.chipSampleStillCurrent(2) {
		t.Fatal("新会话的采样被判为过期")
	}
	a.mu.Lock()
	a.sel.shown = false
	a.mu.Unlock()
	if a.chipSampleStillCurrent(2) {
		t.Fatal("chip 已隐藏，采样仍被采纳")
	}
}

// TestChipShowHideBumpGeneration 是源码契约：显示与隐藏都必须自增代次，
// 采样必须拿到显示时的代次。
func TestChipShowHideBumpGeneration(t *testing.T) {
	src := readSourceFile(t, "selicon.go")
	if got := countOccurrences(src, "a.sel.gen++"); got != 2 {
		t.Fatalf("selState.gen 自增次数 = %d，期望显示与隐藏各一次", got)
	}
	if !containsText(src, "go a.sampleChipPointer(gen)") {
		t.Fatal("sampleChipPointer 没有收到当次显示的代次")
	}
	if !containsText(src, "func (a *app) sampleChipPointer(gen uint64)") {
		t.Fatal("sampleChipPointer 签名缺少代次参数")
	}
}
