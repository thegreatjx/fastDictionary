package main

import (
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestDesignSystemScale 钉住唯一几何标尺，避免圆角、高度、间距再漂回
// 4/5/6/8/10 混用。
func TestDesignSystemScale(t *testing.T) {
	for _, v := range []float32{Space1, Space2, Space3, Space4, Space6, Space8} {
		if v != float32(int(v)) || int(v)%4 != 0 {
			t.Errorf("间距 %v 不在 4px 栅格上", v)
		}
	}
	if RadiusSM != 6 || RadiusTrack != 8 || RadiusWindow != 10 {
		t.Errorf("圆角标尺被改动：SM=%v Track=%v Window=%v", RadiusSM, RadiusTrack, RadiusWindow)
	}
	if HeightCompact != 28 || HeightControl != 32 || HeightSearch != 34 {
		t.Errorf("控件高度标尺被改动：%v/%v/%v", HeightCompact, HeightControl, HeightSearch)
	}
	if FontPageTitle != 20 || FontControl != 13 || FontHelper != 12 {
		t.Errorf("字阶被改动：%v/%v/%v", FontPageTitle, FontControl, FontHelper)
	}
}

// TestAppThemeDerivesBothPalettes 确认明暗两套色板都给出显式灰阶与半透明
// 状态色，禁用态和 hover 不用容器 Opacity 顶替。
func TestAppThemeDerivesBothPalettes(t *testing.T) {
	for _, dark := range []bool{false, true} {
		at := newAppTheme(dark, defaultAccent(dark))
		if at.Dark != dark {
			t.Errorf("dark=%v 得到 Dark=%v", dark, at.Dark)
		}
		if at.AccentSoft.A >= at.Accent.A {
			t.Errorf("dark=%v AccentSoft 没有变浅", dark)
		}
		if at.DangerHoverBg.A >= at.Danger.A {
			t.Errorf("dark=%v DangerHoverBg 没有变浅", dark)
		}
		if at.TextDisabled.A >= at.Text.A {
			t.Errorf("dark=%v TextDisabled 没有给出灰阶", dark)
		}
		if at.BorderSubtle.A >= at.BorderCtrl.A {
			t.Errorf("dark=%v 细边框没有比控件边框更淡", dark)
		}
	}
}

func themeModeFor(dark bool) string {
	if dark {
		return "dark"
	}
	return "light"
}

// TestTrayPanelRendersInBothThemes 用 headless 渲染确认两套主题下托盘结构
// 完整，且结构图标不再用 emoji。
func TestTrayPanelRendersInBothThemes(t *testing.T) {
	for _, dark := range []bool{false, true} {
		a := newTestApp()
		a.store.Update(func(c *Config) { c.ThemeMode = themeModeFor(dark) })
		tt := ui.NewTester(func(c *ui.Context) { a.trayFlyoutView(c) }, 300, 248)
		tt.Frame()
		for _, want := range []string{"fast词典", "生词本", "截图翻译", "退出应用", "Enter"} {
			if !tt.HasText(want) {
				t.Errorf("dark=%v 托盘缺少 %q：%v", dark, want, tt.Texts())
			}
		}
		for _, bad := range []string{"⚙️", "⚡", "🛡️", "🔄", "🔖", "📘"} {
			if tt.HasText(bad) {
				t.Errorf("dark=%v 托盘仍用 emoji %q", dark, bad)
			}
		}
	}
}

// TestAIPageRendersGroupedCardsInBothThemes 确认 AI 页脱离 ui.Form 后，
// 分组卡片与短/长字段在两套主题下都能渲染。
func TestAIPageRendersGroupedCardsInBothThemes(t *testing.T) {
	for _, dark := range []bool{false, true} {
		a := newTestApp()
		a.page = "ai"
		a.store.Update(func(c *Config) { c.ThemeMode = themeModeFor(dark) })
		tt := ui.NewTester(a.settingsView, 880, 640)
		tt.Frame()
		for _, want := range []string{"服务", "模型", "偏好", "状态", "模型服务", "推理强度", "测试连接", "默认模式"} {
			if !tt.HasText(want) {
				t.Errorf("dark=%v AI 页缺少 %q：%v", dark, want, tt.Texts())
			}
		}
	}
}

// TestTraySourceHasNoHardcodedColor 是红线的静态护栏：退出按钮常态不能
// 出现红底填充，托盘里也不能再写散落色值。
func TestTraySourceHasNoHardcodedColor(t *testing.T) {
	src, err := os.ReadFile("tray.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	for _, bad := range []string{"ui.Hex(", "ui.RGBA(", "Background(at.Danger", "#FEE2E2", "⚙️"} {
		if strings.Contains(s, bad) {
			t.Errorf("tray.go 仍含 %q", bad)
		}
	}
	if !strings.Contains(s, "DangerGhostButton") {
		t.Error("tray.go 的退出按钮没有用 DangerGhostButton")
	}
}

func TestNoLegacyFormInSettingsPages(t *testing.T) {
	src, err := os.ReadFile("settings_pages.go")
	if err != nil {
		t.Fatalf("read settings_pages.go: %v", err)
	}
	if strings.Contains(string(src), "ui.Form(") {
		t.Fatal("settings_pages.go still contains legacy ui.Form(; migrate all pages to SettingCard/SettingRow/LongField")
	}
}

func TestUIFilesHaveNoEmojiOrHardcodedColors(t *testing.T) {
	files := []string{
		"tray.go", "settings.go", "settings_pages.go",
		"popup_view.go", "popup_translation.go", "ai_refine_button.go", "grammar.go",
	}
	banned := []string{"ui.Hex(", "ui.RGBA(", "⚙️", "🔍", "✏️", "📘"}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		text := string(raw)
		for _, tok := range banned {
			if strings.Contains(text, tok) {
				t.Errorf("%s contains banned token %q; move colors to theme.go or icons to icons.go", f, tok)
			}
		}
	}
}

func TestNotesViewPhaseIsExplicit(t *testing.T) {
	cases := []struct {
		name string
		s    snapshot
		want notesPhase
	}{
		{"idle", snapshot{}, notesEmpty},
		{"loading", snapshot{notesBusy: true}, notesLoading},
		{"ready", snapshot{notes: []NoteItem{{}}}, notesReady},
		{"ready wins over busy", snapshot{notesBusy: true, notes: []NoteItem{{}}}, notesReady},
	}
	for _, tc := range cases {
		if got := notesViewPhase(tc.s); got != tc.want {
			t.Errorf("%s: notesViewPhase = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestAllSettingsPagesRenderInBothThemes 用 headless 渲染覆盖 9 个子页、
// 托盘与浮窗的明暗两套主题：不 panic、不残留 Emoji，且每个页面都有文本。
func TestAllSettingsPagesRenderInBothThemes(t *testing.T) {
	pages := []string{"general", "selection", "actions", "vocabulary", "translate", "dict", "ai", "hotkey", "about"}
	for _, dark := range []bool{false, true} {
		for _, page := range pages {
			a := newTestApp()
			a.page = page
			a.store.Update(func(c *Config) { c.ThemeMode = themeModeFor(dark) })
			tt := ui.NewTester(a.settingsView, 880, 640)
			tt.Frame()
			if len(tt.Texts()) == 0 {
				t.Errorf("dark=%v page=%s rendered no text", dark, page)
			}
			for _, bad := range []string{"⚙️", "🔍", "✏️", "📘"} {
				if tt.HasText(bad) {
					t.Errorf("dark=%v page=%s still shows emoji %q", dark, page, bad)
				}
			}
		}
		a := newTestApp()
		a.store.Update(func(c *Config) { c.ThemeMode = themeModeFor(dark) })
		ui.NewTester(func(c *ui.Context) { a.trayFlyoutView(c) }, 300, 248).Frame()
		ui.NewTester(a.popupView, 420, 300).Frame()
	}
}
