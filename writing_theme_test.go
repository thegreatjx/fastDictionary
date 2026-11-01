package main

import (
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
)

// writingContrast luminance follows WCAG's sRGB relative luminance formula.
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

func TestWritingThemeContrast(t *testing.T) {
	for _, tc := range []struct {
		name string
		base *ui.Theme
	}{
		{name: "light", base: ui.LightTheme()},
		{name: "dark", base: ui.DarkTheme()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := writingThemeFor(tc.base)
			base := tc.base
			checks := []struct {
				name       string
				foreground ui.Color
				background ui.Color
			}{
				{name: "setting label", foreground: w.settingLabel, background: w.setting.background},
				{name: "setting value", foreground: w.setting.text, background: w.setting.background},
				{name: "removed text", foreground: w.removed, background: base.SurfaceHover},
				{name: "added text", foreground: w.added, background: base.SurfaceHover},
				{name: "deleted text", foreground: w.deleted, background: base.SurfaceHover},
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
