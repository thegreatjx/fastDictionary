package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSelectionChipStaysIconOnly(t *testing.T) {
	a := newTestApp()
	width := a.currentSelectionChipWidth()
	tt := ui.NewTester(a.selIconView, chipWindowWidth(width), chipWindowHeight())
	tt.Frame()

	if tt.HasText("悬停查看功能") {
		t.Errorf("chip still renders the removed name slot: %q", tt.Texts())
	}

	cfg := a.store.Get()
	want := 4 + len(cfg.GetEnabledActions())
	if got := len(tt.Texts()); got != want {
		t.Errorf("chip renders %d text nodes, want %d button labels: %q", got, want, tt.Texts())
	}
}

func TestSelectionChipButtonsAreComfortableHitTargets(t *testing.T) {
	a := newTestApp()
	width := a.currentSelectionChipWidth()
	tt := ui.NewTester(a.selIconView, chipWindowWidth(width), chipWindowHeight())
	tt.Frame()

	if chipButtonSize < 28 {
		t.Errorf("chip button is %vpx, below the 28px comfortable minimum", chipButtonSize)
	}

	cfg := a.store.Get()
	tips := []string{
		"划词翻译（" + cfg.HotkeySelection + "）",
		"朗读选中文字",
		"截图翻译（" + cfg.HotkeyScreenshot + "）",
	}
	for _, act := range cfg.GetEnabledActions() {
		tips = append(tips, act.Name)
	}
	tips = append(tips, "复制与更多功能")

	for _, tip := range tips {
		r, ok := tt.Find(tip)
		if !ok {
			t.Fatalf("chip button %q not found: %q", tip, tt.Texts())
		}
		if r.W != chipButtonSize || r.H != chipButtonSize {
			t.Errorf("button %q is %vx%v, want %vx%v", tip, r.W, r.H, chipButtonSize, chipButtonSize)
		}
		if r.X < 0 || r.X+r.W > float32(width) {
			t.Errorf("button %q is clipped: bounds %+v, pill width %d", tip, r, width)
		}
	}
}

func TestSelectionChipWidthIsProportional(t *testing.T) {
	base := selectionChipWidth(0)
	if got, want := selectionChipWidth(3), base+3*(chipButtonSize+chipRowGap); got != want {
		t.Errorf("chip width with 3 custom actions = %d, want %d", got, want)
	}
	if base >= 240 {
		t.Errorf("chip without custom actions is %dpx wide; it should stay compact", base)
	}
	if base <= 0 {
		t.Errorf("chip width = %d", base)
	}
}

func TestChipHeightFitsButtons(t *testing.T) {
	if need := chipButtonSize + 2*chipPadV + 2; selIconHeight < need {
		t.Errorf("chip window is %vpx tall, needs at least %v", selIconHeight, need)
	}
}
