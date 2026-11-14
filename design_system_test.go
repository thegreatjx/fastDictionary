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
		for _, want := range []string{"服务", "模型", "偏好", "维护", "状态", "模型服务", "推理强度", "测试连接", "默认模式"} {
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
	chipSrc, err := os.ReadFile("selicon.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(chipSrc), "ui.Hex(") || strings.Contains(string(chipSrc), "ui.RGBA(") {
		t.Error("selicon.go 仍含硬编码色值，应移到 theme.go 的 chip 色板")
	}
}

// TestSegmentedControlGeometry 钉住第 1 批的两个结论：轨道不随父列横拉，
// 白滑块严格居中不压底边。
func TestSegmentedControlGeometry(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(map[bool]string{false: "light", true: "dark"}[dark], func(t *testing.T) {
			tt := ui.NewTester(func(c *ui.Context) {
				applyTheme(c, themeModeFor(dark), dark)
				idx := 1
				ui.Column(c).FillWidth().Padding(Space4).Gap(Space3).Children(func() {
					SegmentedControl(c, &idx, "显示图标", "直接翻译", "仅快捷键").Label("std-track")
					SegmentedControlCompact(c, &idx, "显示图标", "直接翻译", "仅快捷键").Label("cmp-track")
				})
			}, 880, 240)
			tt.Frame()

			std, ok := tt.Find("std-track")
			if !ok {
				t.Fatal("找不到标准分段轨道")
			}
			compact, ok := tt.Find("cmp-track")
			if !ok {
				t.Fatal("找不到紧凑分段轨道")
			}
			if std.H != HeightControl {
				t.Fatalf("标准分段高度 = %v，want %v", std.H, HeightControl)
			}
			if compact.H != HeightCompact {
				t.Fatalf("紧凑分段高度 = %v，want %v", compact.H, HeightCompact)
			}
			if std.W >= 500 || compact.W >= 500 {
				t.Fatalf("分段控件被 880px 父列拉宽：std=%v cmp=%v", std.W, compact.W)
			}

			label, ok := tt.Find("直接翻译")
			if !ok {
				t.Fatal("找不到选中分段文本")
			}
			topGap := label.Y - std.Y
			bottomGap := (std.Y + std.H) - (label.Y + label.H)
			if diff := topGap - bottomGap; diff < -1.5 || diff > 1.5 {
				t.Fatalf("分段文本未垂直居中：topGap=%v bottomGap=%v", topGap, bottomGap)
			}
		})
	}
}

// TestChipPaletteCoolBlue 防止悬浮条退回中性灰：主图标必须偏冷蓝，浅色底
// 必须带蓝调，hover 微底必须可见。
func TestChipPaletteCoolBlue(t *testing.T) {
	for _, dark := range []bool{false, true} {
		th := ui.Theme{Dark: dark, Accent: defaultAccent(dark)}
		surf := chipSurface(&th)
		border := chipBorderColor(&th)
		primFg, primWash := chipToneColors(&th, chipTonePrimary)
		inkFg, _ := chipToneColors(&th, chipToneInk)
		if surf == border || primFg == inkFg {
			t.Fatalf("dark=%v: 悬浮条颜色塌缩", dark)
		}
		if primFg.B <= primFg.R {
			t.Fatalf("dark=%v: 主图标应偏冷蓝，得到 %+v", dark, primFg)
		}
		if primWash.A <= 0 {
			t.Fatalf("dark=%v: 主图标 hover 微底不可见", dark)
		}
		if !dark && surf.B <= surf.R {
			t.Fatalf("浅色悬浮条底色应保留淡蓝调，得到 %+v", surf)
		}
	}
}

// TestSettingsNavIsGroupedAndAligned 确认侧栏按 5 组呈现，且所有图标后的
// 文本左缘在同一列。工作台放在最前：主窗口默认打开的就是这一页。
func TestSettingsNavIsGroupedAndAligned(t *testing.T) {
	a := newTestApp()
	a.page = "general"
	tt := ui.NewTester(a.settingsView, 920, 660)
	tt.Frame()

	for _, group := range []string{"工作台", "常规", "划词与翻译", "AI 与指令", "其他"} {
		if !tt.HasText(group) {
			t.Errorf("侧栏缺少分组 %q", group)
		}
	}

	// 默认窗口（920×660）一屏放得下：不需要滚动，最末入口完整可见。
	tall, ok := tt.Find("关于")
	if !ok || tall.H == 0 || tall.Y+tall.H > 660 {
		t.Errorf("默认窗口下侧栏最后一项被裁切：ok=%v rect=%+v", ok, tall)
	}
	if a.navScroll.MaxY > 0 {
		t.Errorf("默认窗口下侧栏仍需要滚动（MaxY=%v）", a.navScroll.MaxY)
	}

	// 最小尺寸（700×500）时五组入口加底部胶囊装不下：多出的部分交给侧栏
	// 滚动承载，不能像以前那样被直接裁掉。
	small := ui.NewTester(a.settingsView, 700, 500)
	small.Frame()
	if !small.HasText("关于") {
		t.Error("最小高度下侧栏丢了最后一个入口")
	}
	if a.navScroll.MaxY <= 0 {
		t.Errorf("最小高度下侧栏没有可滚动余量（MaxY=%v），超出的入口无法到达", a.navScroll.MaxY)
	}

	labels := []string{"查词翻译", "生词本", "通用", "快捷键", "划词", "翻译", "词典与发音", "AI 精翻", "AI 指令", "关于"}
	var wantX float32
	for i, label := range labels {
		r, ok := tt.Find(label)
		if !ok {
			t.Fatalf("侧栏缺少 %q", label)
		}
		if i == 0 {
			wantX = r.X
			continue
		}
		if diff := r.X - wantX; diff < -0.5 || diff > 0.5 {
			t.Errorf("导航文本 %q 左缘 x=%v，应为 %v", label, r.X, wantX)
		}
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
