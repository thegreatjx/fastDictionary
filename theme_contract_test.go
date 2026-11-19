package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func themeForContract(t *testing.T, dark bool, accent string) *ui.Theme {
	t.Helper()
	var got *ui.Theme
	tt := ui.NewTester(func(c *ui.Context) {
		mode := "light"
		if dark {
			mode = "dark"
		}
		applyTheme(c, mode, dark, accent)
		got = c.Theme()
	}, 40, 20)
	tt.Frame()
	if got == nil {
		t.Fatalf("applyTheme(%v, %q) produced no theme", dark, accent)
	}
	return got
}

func contrastOn(fg, bg ui.Color) float64 {
	flat := fg.Over(bg)
	a, b := relativeLuminance(flat), relativeLuminance(bg)
	if a < b {
		a, b = b, a
	}
	return (a + 0.05) / (b + 0.05)
}

func requireContrast(t *testing.T, label string, fg, bg ui.Color, min float64) {
	t.Helper()
	if got := contrastOn(fg, bg); got < min {
		t.Errorf("%s contrast = %.2f:1, want >= %.1f:1 (fg=%+v bg=%+v)", label, got, min, fg, bg)
	}
}

func TestGrammarPaletteContrastAcrossThemes(t *testing.T) {
	tags := []string{"宾从", "主从", "表从", "定从", "同位", "状从"}
	for _, choice := range accentChoices {
		for _, dark := range []bool{false, true} {
			name := choice.Value + "/" + themeModeFor(dark)
			th := themeForContract(t, dark, choice.Value)
			at := appTheme(th)

			bg, _, text := grammarHeaderColors(th)
			requireContrast(t, name+" header", text, bg, 4.5)

			toggleBg, toggleText, _ := grammarToggleOnColors(th)
			requireContrast(t, name+" toggle", toggleText, toggleBg, 4.5)

			for _, tag := range tags {
				colors := grammarClausePalette(th, tag)
				wash := colors.wash.Over(at.SurfaceHover)
				badge := colors.badgeBg.Over(wash)
				requireContrast(t, name+" badge "+tag, colors.badgeText, badge, 4.5)
			}

			d := grammarDifficultyPalette(at)
			requireContrast(t, name+" difficulty heading", d.heading, d.surface, 4.5)
			requireContrast(t, name+" difficulty chip", d.chipText, d.chipBg, 4.5)
			requireContrast(t, name+" difficulty note", d.note, d.surface, 4.5)

			w := writingThemeFor(th)
			requireContrast(t, name+" writing removed", w.removed, at.SurfaceHover, 4.5)
			requireContrast(t, name+" writing added", w.added, at.SurfaceHover, 4.5)
		}
	}
}

func TestGrammarClauseWashHierarchy(t *testing.T) {
	for _, choice := range accentChoices {
		for _, dark := range []bool{false, true} {
			name := choice.Value + "/" + themeModeFor(dark)
			th := themeForContract(t, dark, choice.Value)
			shallow := grammarClausePalette(th, "状从").wash.A
			medium := grammarClausePalette(th, "定从").wash.A
			deep := grammarClausePalette(th, "宾从").wash.A
			badge := grammarClausePalette(th, "宾从").badgeBg.A
			if !(shallow < medium && medium < deep && deep < badge && badge < 255) {
				t.Errorf("%s wash alphas must rise 状<定<宾 and stay below badge, got %d/%d/%d/%d", name, shallow, medium, deep, badge)
			}
		}
	}
}

func TestGrammarDifficultyStaysNeutral(t *testing.T) {
	for _, choice := range accentChoices {
		for _, dark := range []bool{false, true} {
			at := appTheme(themeForContract(t, dark, choice.Value))
			d := grammarDifficultyPalette(at)
			if d.surface != at.SurfaceHover || d.border != at.BorderSubtle ||
				d.heading != at.TextSec || d.chipBg != at.Surface ||
				d.chipBorder != at.BorderSubtle || d.chipText != at.TextSec ||
				d.dot != at.Accent || d.note != at.TextSec {
				t.Errorf("%s/%s difficulty palette left the neutral role map: %+v", choice.Value, themeModeFor(dark), d)
			}
			for field, color := range map[string]ui.Color{
				"surface": d.surface, "border": d.border, "heading": d.heading,
				"chip": d.chipBg, "chipText": d.chipText, "dot": d.dot, "note": d.note,
			} {
				if color == at.Warning || color == at.Danger {
					t.Errorf("%s/%s difficulty %s uses %+v; difficulties are not warnings", choice.Value, themeModeFor(dark), field, color)
				}
			}
		}
	}
}

func TestBaseThemeContrastAcrossThemes(t *testing.T) {
	for _, choice := range accentChoices {
		for _, dark := range []bool{false, true} {
			name := choice.Value + "/" + themeModeFor(dark)
			th := themeForContract(t, dark, choice.Value)
			at := appTheme(th)
			surface := at.Surface
			hover := at.SurfaceHover.Over(surface)

			for bgName, bg := range map[string]ui.Color{
				"canvas":  at.Canvas,
				"surface": surface,
				"hover":   hover,
			} {
				requireContrast(t, name+" Text on "+bgName, at.Text, bg, 4.5)
				requireContrast(t, name+" TextSec on "+bgName, at.TextSec, bg, 4.5)
				requireContrast(t, name+" TextMuted on "+bgName, at.TextMuted, bg, 4.5)
			}

			requireContrast(t, name+" AccentText on Accent", at.AccentText, at.Accent, 4.5)
			accentSoftBg := at.AccentSoft.Over(surface)
			requireContrast(t, name+" Accent on AccentSoft", at.Accent, accentSoftBg, 4.5)

			w := writingThemeFor(th)
			requireContrast(t, name+" writing note on hover", w.note, hover, 4.5)
		}
	}
}
