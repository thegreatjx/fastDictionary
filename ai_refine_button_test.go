package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestAIRefineViewForStates(t *testing.T) {
	idle := aiRefineViewFor(snapshot{engine: "微软"})
	if idle.label != "AI 精翻" || idle.active || idle.busy {
		t.Fatalf("idle state = %+v", idle)
	}
	shotIdle := aiRefineViewFor(snapshot{engine: "微软", isScreenshotMode: true})
	if shotIdle.label != "AI 精翻" || !strings.Contains(shotIdle.tip, "截图") {
		t.Fatalf("screenshot idle state = %+v", shotIdle)
	}
	busy := aiRefineViewFor(snapshot{engine: "微软", aiRefining: true})
	if busy.label != "AI 处理中…" || !busy.active || !busy.busy {
		t.Fatalf("busy state = %+v", busy)
	}
	refined := aiRefineViewFor(snapshot{engine: "AI 精翻"})
	if refined.label != "已 AI 精翻" || !refined.active || refined.busy {
		t.Fatalf("refined state = %+v", refined)
	}
	vision := aiRefineViewFor(snapshot{engine: "AI 精翻", isScreenshotMode: true})
	if vision.label != "已 AI 精翻" || !vision.active || vision.busy || !strings.Contains(vision.tip, "看图") {
		t.Fatalf("screenshot refined state = %+v", vision)
	}
}

func TestScreenshotSingleWordShowsOneAIRefineAction(t *testing.T) {
	a := newTestApp()
	a.mode = modeTranslate
	a.isScreenshotMode = true
	a.ocrText = "untolc"
	a.source = a.ocrText
	a.target = "联合国国家运输公司"
	a.engine = "谷歌"
	a.detected = "en"
	tt := ui.NewTester(a.popupView, 520, 520)
	if !tt.HasText("AI 精翻") {
		t.Fatalf("single-word screenshot misses AI 精翻: %q", tt.Texts())
	}
	for _, unwanted := range []string{"AI 看图", "看图", "AI 精翻已生效"} {
		if tt.HasText(unwanted) {
			t.Errorf("single-word screenshot still shows duplicate %q: %q", unwanted, tt.Texts())
		}
	}
}

func TestAIRefineVisualStateHasNoDuplicateBadge(t *testing.T) {
	selection := newTestApp()
	selection.mode = modeTranslate
	selection.source = "Fast and efficient translation"
	selection.target = "快速且高效的翻译"
	selection.engine = "AI 精翻"
	selection.detected = "en"
	tt := ui.NewTester(selection.popupView, 520, 480)
	if !tt.HasText("已 AI 精翻") {
		t.Fatalf("selection AI state misses button: %q", tt.Texts())
	}
	if tt.HasText("AI 精翻已生效") {
		t.Errorf("selection AI state has duplicate badge: %q", tt.Texts())
	}

	screenshot := newTestApp()
	screenshot.mode = modeTranslate
	screenshot.isScreenshotMode = true
	screenshot.ocrCompare = true
	screenshot.ocrText = "Fast and efficient"
	screenshot.source = screenshot.ocrText
	screenshot.target = "快速且高效"
	screenshot.engine = "AI 精翻"
	ttShot := ui.NewTester(screenshot.popupView, 520, 520)
	if !ttShot.HasText("已 AI 精翻") {
		t.Fatalf("screenshot AI state misses button: %q", ttShot.Texts())
	}
	for _, unwanted := range []string{"AI 看图已生效", "已 AI 看图", "AI 精翻已生效"} {
		if ttShot.HasText(unwanted) {
			t.Errorf("screenshot AI state has duplicate %q: %q", unwanted, ttShot.Texts())
		}
	}
}
