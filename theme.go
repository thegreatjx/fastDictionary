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

// chipTone names what one selection-chip button does. The tone picks one of
// two inks, not one of seven hues.
//
// The chip used to colour every action differently: blue, teal, violet,
// amber, green, magenta in one 36px row. Six saturated hues side by side read
// as a toy, and they fought the quiet grey the rest of the app is built from.
// Colour is now spent once — on the primary action — and the other buttons
// share a cool slate ink that differs from its neighbours by glyph and by
// position, which is how a real toolbar separates its tools.
type chipTone int

const (
	// chipTonePrimary marks the chip's main action (translate). It is the one
	// button allowed an accent colour, so the row has a single place to look
	// first.
	chipTonePrimary chipTone = iota
	// chipToneInk is every other action: speech, screenshot, the AI actions
	// and the utility menu.
	chipToneInk
)

// The chip's palette. One cool, low-saturation family: a blue-grey slate for
// the ink and a single steel blue for the primary action. The surface is not
// pure white or pure black — a trace of blue in it keeps the bar from
// looking pasted on top of the page.
const (
	chipSurfaceLight = "#FBFCFD"
	chipSurfaceDark  = "#1B2027"
	chipBorderLight  = "#E3E8EE"
	chipBorderDark   = "#2E3641"
	chipInkLight     = "#3F4A5A"
	chipInkDark      = "#AEBAC8"
	chipInkHoverL    = "#1E2A38"
	chipInkHoverD    = "#E4EAF1"
	chipAccentLight  = "#4A6E8A"
	chipAccentDark   = "#7FA6C4"
	chipTipBgLight   = "#26303B"
	chipTipBgDark    = "#E8EDF2"
	chipTipFgLight   = "#EAF0F6"
	chipTipFgDark    = "#1A2028"
)

// chipSurface is the pill the buttons sit on.
func chipSurface(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.Hex(chipSurfaceDark)
	}
	return ui.Hex(chipSurfaceLight)
}

// chipBorderColor is the hairline around the pill and the rule that separates
// the tools from the menu.
func chipBorderColor(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.Hex(chipBorderDark)
	}
	return ui.Hex(chipBorderLight)
}

// chipToneColors returns the icon colour of a chip action and the wash its
// button takes on hover.
//
// Both inks sit at or above the 3:1 non-text contrast floor against
// chipSurface in both themes, which is what keeps a glyph legible when colour
// is no longer carrying the meaning. The wash is the same slate for every
// button: a hover that changes hue per button was what made the row look like
// six unrelated tools.
func chipToneColors(t *ui.Theme, tone chipTone) (fg, wash ui.Color) {
	if t.Dark {
		if tone == chipTonePrimary {
			return ui.Hex(chipAccentDark), ui.RGBA(127, 166, 196, 0.16)
		}
		return ui.Hex(chipInkDark), ui.RGBA(174, 186, 200, 0.12)
	}
	if tone == chipTonePrimary {
		return ui.Hex(chipAccentLight), ui.RGBA(74, 110, 138, 0.10)
	}
	return ui.Hex(chipInkLight), ui.RGBA(63, 74, 90, 0.07)
}

// chipIconHover is the icon colour while the pointer is on the button: the ink
// deepens instead of turning into another hue, so hover reads as emphasis and
// not as a change of identity.
func chipIconHover(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.Hex(chipInkHoverD)
	}
	return ui.Hex(chipInkHoverL)
}

// chipTipColors returns the fill and text of the hover label.
//
// It is a dark slate plate in the light theme and a pale one in the dark theme
// — inverted from the chip, as a tooltip should be, but drawn from the same
// cool family rather than from the theme's neutral Inverse, which is nearly
// black and reads as a different product.
func chipTipColors(t *ui.Theme) (bg, fg ui.Color) {
	if t.Dark {
		return ui.Hex(chipTipBgDark), ui.Hex(chipTipFgDark)
	}
	return ui.Hex(chipTipBgLight), ui.Hex(chipTipFgLight)
}
