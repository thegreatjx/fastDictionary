package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestSelectionChipStaysIconOnly guards the redesign: the chip is a row of
// icons on one quiet surface. It must not render a persistent text label —
// the old 64px name box ("悬停查看功能") was the part the user called ugly and
// unprofessional. Action names survive as button labels, which is what the
// accessibility tree and the tooltip read.
func TestSelectionChipStaysIconOnly(t *testing.T) {
	a := newTestApp()
	width := a.currentSelectionChipWidth()
	tt := ui.NewTester(a.selIconView, width, selIconHeight)
	tt.Frame()

	if tt.HasText("悬停查看功能") {
		t.Errorf("chip still renders the removed name slot: %q", tt.Texts())
	}

	// Every text left in the tree must be a button label (one per button).
	cfg := a.store.Get()
	want := 4 + len(cfg.GetEnabledActions())
	if got := len(tt.Texts()); got != want {
		t.Errorf("chip renders %d text nodes, want %d button labels: %q", got, want, tt.Texts())
	}
}

// TestSelectionChipButtonsAreComfortableHitTargets enforces the hit-zone rule
// the user asked for: every icon button is at least 28px and lands inside the
// chip window, so nothing is clipped and nothing needs a pixel hunt.
func TestSelectionChipButtonsAreComfortableHitTargets(t *testing.T) {
	a := newTestApp()
	width := a.currentSelectionChipWidth()
	tt := ui.NewTester(a.selIconView, width, selIconHeight)
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
			t.Errorf("button %q is clipped: bounds %+v, width %d", tip, r, width)
		}
	}
}

// TestChipCloseBadgeSitsOnThePillCorner pins the close affordance: it is not a
// permanent sixth button and not an overlay on the ellipsis. The badge is a
// flyout whose centre lands on the pill's top-right corner, so the dots stay
// visible and the resting row keeps its old width.
func TestChipCloseBadgeSitsOnThePillCorner(t *testing.T) {
	width := selectionChipWidth(2)
	more := ui.Rect{
		X: float32(width - chipPadH - chipButtonSize),
		Y: chipPadV,
		W: chipButtonSize,
		H: chipButtonSize,
	}
	r := chipCloseRect(more)
	if r.W != chipCloseSize || r.H != chipCloseSize {
		t.Fatalf("badge is %vx%v, want %dx%d", r.W, r.H, chipCloseSize, chipCloseSize)
	}
	if got, want := r.X+r.W/2, more.X+more.W-1; got != want {
		t.Errorf("badge centre x = %v, want the more button's right edge %v", got, want)
	}
	if got := r.Y + r.H/2; got != 0 {
		t.Errorf("badge centre y = %v, want the pill's top edge 0", got)
	}
	if r.Y >= more.Y {
		t.Errorf("badge top %v is not above the more button %v: it would cover the dots", r.Y, more.Y)
	}
}

// TestChipCloseHangsOutsideTheChipWindow keeps the pointer handoff working:
// the badge's centre is above the pill, so the chip's own hit boxes do not
// cover it. The badge state must answer for that overhanging part.
func TestChipCloseHangsOutsideTheChipWindow(t *testing.T) {
	a := newTestApp()
	a.chipClose.mu.Lock()
	a.chipClose.shown = true
	a.chipClose.rect = ui.Rect{X: 100, Y: -chipCloseSize / 2, W: chipCloseSize, H: chipCloseSize}
	a.chipClose.mu.Unlock()

	if !a.chipCloseContains(100, -8) {
		t.Error("badge does not answer for its overhanging part above the chip")
	}
	if a.chipCloseContains(100, 20) {
		t.Error("badge answers for a point on the chip, not on the badge")
	}
	a.hideChipClose()
	if a.chipCloseContains(100, -8) {
		t.Error("hidden badge still claims the pointer")
	}
}

// TestSelectionChipWidthIsProportional keeps the chip compact: each extra
// action adds exactly one button plus one gap, and no space is reserved for a
// name label that no longer exists.
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

// TestChipHeightFitsButtons keeps the window tall enough for the 28px buttons
// and their padding, so the row is never vertically clipped.
func TestChipHeightFitsButtons(t *testing.T) {
	if need := chipButtonSize + 2*chipPadV + 2; selIconHeight < need {
		t.Errorf("chip window is %vpx tall, needs at least %v", selIconHeight, need)
	}
}
