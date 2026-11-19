package main

import (
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
)

func writingContrastLuminance(c ui.Color) float64 {
	channel := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.04045 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*channel(c.R) + 0.7152*channel(c.G) + 0.0722*channel(c.B)
}

func writingContrastRatio(foreground, background ui.Color) float64 {
	fg := writingContrastLuminance(foreground.Over(background))
	bg := writingContrastLuminance(background)
	if fg < bg {
		fg, bg = bg, fg
	}
	return (fg + 0.05) / (bg + 0.05)
}

func writingThemeForTest(dark bool, accent string) *ui.Theme {
	var got *ui.Theme
	tt := ui.NewTester(func(c *ui.Context) {
		applyTheme(c, themeModeFor(dark), dark, accent)
		got = c.Theme()
	}, 40, 20)
	tt.Frame()
	return got
}

func TestWritingThemeContrast(t *testing.T) {
	for _, choice := range accentChoices {
		for _, dark := range []bool{false, true} {
			name := choice.Value + "/" + themeModeFor(dark)
			base := writingThemeForTest(dark, choice.Value)
			t.Run(name, func(t *testing.T) {
				w := writingThemeFor(base)
				if w.removed == w.added {
					t.Fatalf("removed and added colors should be distinct, both %+v", w.removed)
				}
				hoverBg := base.SurfaceHover.Over(base.Surface)
				checks := []struct {
					name       string
					foreground ui.Color
					background ui.Color
				}{
					{name: "setting label", foreground: w.settingLabel, background: w.setting.background},
					{name: "setting value", foreground: w.setting.text, background: w.setting.background},
					{name: "removed text", foreground: w.removed, background: hoverBg},
					{name: "added text", foreground: w.added, background: hoverBg},
					{name: "deleted text", foreground: w.deleted, background: hoverBg},
					{name: "note text", foreground: w.note, background: hoverBg},
					{name: "annotation tag", foreground: w.tag.text, background: w.tag.background},
					{name: "success message", foreground: w.success.text, background: w.success.background},
					{name: "improved heading", foreground: w.improvedTitle, background: w.improved.background},
					{name: "improved text", foreground: w.improved.text, background: w.improved.background},
					{name: "copy text", foreground: w.copy.text, background: w.copy.background},
					{name: "save text", foreground: w.save.text, background: w.save.background},
				}
				for _, check := range checks {
					t.Run(check.name, func(t *testing.T) {
						if got := writingContrastRatio(check.foreground, check.background); got < 4.5 {
							t.Errorf("contrast ratio = %.2f, want at least 4.5", got)
						}
					})
				}
			})
		}
	}
}
