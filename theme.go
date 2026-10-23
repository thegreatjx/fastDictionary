package main

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// Design direction: "Clean, Flat, Modern Utility" — styled after Google Translate Web,
// Read-Frog popover and Linear. Crisp typography, subtle 1px gray borders,
// no heavy dark frames, high readability.
func applyTheme(c *ui.Context, mode string, dark bool) {
	curr := c.Theme()
	t := *curr
	if mode == "light" {
		dark = false
	} else if mode == "dark" {
		dark = true
	}

	// Capture desktop/system accent color if available
	sysAccent := curr.Accent
	hasSysAccent := sysAccent.A > 0 && !(sysAccent.R == 0 && sysAccent.G == 0 && sysAccent.B == 0)

	if dark {
		t.Dark = true
		t.Background = ui.Hex("#18181B") // Zinc 900
		t.Surface = ui.Hex("#27272A")    // Zinc 800
		t.SurfaceHover = ui.Hex("#3F3F46")
		t.SurfacePressed = ui.Hex("#52525B")
		t.Border = ui.Hex("#3F3F46")
		t.Text = ui.Hex("#F4F4F5")
		t.TextMuted = ui.Hex("#A1A1AA")
		if hasSysAccent {
			t.Accent = sysAccent
			t.AccentHover = sysAccent
			t.AccentPressed = sysAccent
			t.Selection = ui.RGBA(sysAccent.R, sysAccent.G, sysAccent.B, 0.28)
			t.Focus = sysAccent
		} else {
			t.Accent = ui.Hex("#38BDF8") // Sky blue
			t.AccentHover = ui.Hex("#7DD3FC")
			t.AccentPressed = ui.Hex("#0284C7")
			t.Selection = ui.RGBA(56, 189, 248, 0.28)
			t.Focus = ui.Hex("#38BDF8")
		}
		t.AccentText = ui.Hex("#09090B")
		t.Danger = ui.Hex("#F87171")
		t.Warning = ui.Hex("#FBBF24")
		t.Success = ui.Hex("#4ADE80")
		t.Scrollbar = ui.RGBA(161, 161, 170, 0.35)
	} else {
		t.Dark = false
		t.Background = ui.Hex("#F8FAFC") // Slate 50 clean flat paper
		t.Surface = ui.Hex("#FFFFFF")    // Crisp white card
		t.SurfaceHover = ui.Hex("#F1F5F9")
		t.SurfacePressed = ui.Hex("#E2E8F0")
		t.Border = ui.Hex("#E2E8F0")    // Crisp subtle border
		t.Text = ui.Hex("#0F172A")      // Slate 900 high contrast readable text
		t.TextMuted = ui.Hex("#64748B") // Slate 500 quiet text
		if hasSysAccent {
			t.Accent = sysAccent
			t.AccentHover = sysAccent
			t.AccentPressed = sysAccent
			t.Selection = ui.RGBA(sysAccent.R, sysAccent.G, sysAccent.B, 0.20)
			t.Focus = sysAccent
		} else {
			t.Accent = ui.Hex("#2563EB") // Google / Material Royal Blue
			t.AccentHover = ui.Hex("#1D4ED8")
			t.AccentPressed = ui.Hex("#1E40AF")
			t.Selection = ui.RGBA(37, 99, 235, 0.18)
			t.Focus = ui.Hex("#2563EB")
		}
		t.AccentText = ui.Hex("#FFFFFF")
		t.Danger = ui.Hex("#DC2626")
		t.Warning = ui.Hex("#D97706")
		t.Success = ui.Hex("#16A34A")
		t.Scrollbar = ui.RGBA(100, 116, 139, 0.28)
	}
	t.Radius = 7
	t.Spacing = 4
	c.SetTheme(&t)
}

// inkRuleWidth is the 2px accent rule marking the hero translation.
const inkRuleWidth float32 = 3

// Notice colours: the "why did the shortcut open this" banner. Amber in
// both themes, because it explains rather than reports an error.
func noticeBg(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.RGBA(251, 191, 36, 0.14)
	}
	return ui.RGBA(251, 191, 36, 0.12)
}

func noticeBorder(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.RGBA(251, 191, 36, 0.45)
	}
	return ui.RGBA(217, 119, 6, 0.35)
}

func noticeText(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.Hex("#FCD34D")
	}
	return ui.Hex("#92400E")
}

// cefrColors tints the CEFR level badge by difficulty band. Both colours come
// from the theme so the badge stays readable in dark mode, where the fixed
// pastels it used to use turned into unreadable pale blocks.
func cefrColors(t *ui.Theme, level string) (bg, fg ui.Color) {
	switch {
	case strings.HasPrefix(level, "C"):
		// Hardest: the danger-ish end of the scale.
		if t.Dark {
			return ui.RGBA(168, 85, 247, 0.22), ui.Hex("#D8B4FE")
		}
		return ui.RGBA(168, 85, 247, 0.12), ui.Hex("#7E22CE")
	case strings.HasPrefix(level, "A"):
		// Easiest: the calm end.
		if t.Dark {
			return ui.RGBA(74, 222, 128, 0.18), ui.Hex("#86EFAC")
		}
		return ui.RGBA(22, 163, 74, 0.10), ui.Hex("#15803D")
	}
	if t.Dark {
		return ui.RGBA(56, 189, 248, 0.18), ui.Hex("#7DD3FC")
	}
	return ui.RGBA(37, 99, 235, 0.10), ui.Hex("#1D4ED8")
}

// memoryTipColors tints the "memory tips" card: amber in light mode, a dim
// amber wash in dark mode, where the light version was a glaring block.
func memoryTipColors(t *ui.Theme) (bg, border, title, body ui.Color) {
	if t.Dark {
		return ui.RGBA(251, 191, 36, 0.12), ui.RGBA(251, 191, 36, 0.35), ui.Hex("#FCD34D"), ui.Hex("#FDE68A")
	}
	return ui.Hex("#FEFCE8"), ui.Hex("#FEF08A"), ui.Hex("#854D0E"), ui.Hex("#713F12")
}

// saveButtonColor is the amber of the "save to notebook" action, brightened in
// dark mode so it keeps its contrast against the dark surface.
func saveButtonColor(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.Hex("#CA8A04")
	}
	return ui.Hex("#EAB308")
}
