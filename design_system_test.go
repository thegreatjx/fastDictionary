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
