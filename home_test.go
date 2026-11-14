package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// renderHome draws the 查词与翻译 workbench at the main window's default size
// and returns every string it rendered.
func renderHome(t *testing.T, a *app) []string {
	t.Helper()
	tt := ui.NewTester(func(c *ui.Context) {
		applyTheme(c, "light", false)
		a.pageHome(c, c.Theme(), a.store.Get())
	}, 920, 660)
	tt.Frame()
	return tt.Texts()
}

// TestHomeReadyStateShowsTheBackgroundState 是首页空态的验收：用户打开主窗口
// 时，第一眼要知道"软件在跑、划词可用、快捷键是什么"，而不是看到一张空白。
func TestHomeReadyStateShowsTheBackgroundState(t *testing.T) {
	a := newTestApp()
	texts := strings.Join(renderHome(t, a), "")
	for _, want := range []string{
		"查词与翻译",
		"后台划词监听已就绪",
		"划词翻译",
		"截图翻译",
		"快捷浮窗",
		"支持自动检测语种",
	} {
		if !strings.Contains(texts, want) {
			t.Errorf("首页空态缺少 %q，实际渲染 %q", want, texts)
		}
	}
	// 默认配置没有配 AI 服务时，引导必须写出来，否则长难句和润色点了没反应。
	a.store.Update(func(c *Config) { c.AIEnabled = false })
	texts = strings.Join(renderHome(t, a), "")
	if !strings.Contains(texts, "配置 AI 服务后可解锁") {
		t.Errorf("首页没有 AI 配置引导：%q", texts)
	}
}

// TestHomeGrammarTabExplainsMissingAI 保证页签在没有 AI 配置时给出原因，
// 而不是留一片空白让人以为坏了。
func TestHomeGrammarTabExplainsMissingAI(t *testing.T) {
	a := newTestApp()
	a.store.Update(func(c *Config) { c.AIEnabled = false })
	a.home.source = "Only then did she realize the truth."
	a.home.tab = "grammar"

	texts := strings.Join(renderHome(t, a), "")
	if !strings.Contains(texts, "长难句拆解需要配置 AI 服务") {
		t.Errorf("未配置 AI 时页签没有说明：%q", texts)
	}
}

// TestHomeGrammarTabIsNeverBlank 覆盖另一个洞：AI 已启用、但这一句尚未生成
// 结果时，页签不能什么都不画。
func TestHomeGrammarTabIsNeverBlank(t *testing.T) {
	a := newTestApp()
	a.home.source = "Only then did she realize the truth."
	a.home.tab = "grammar"

	texts := strings.Join(renderHome(t, a), "")
	if !strings.Contains(texts, "还没有拆解结果") {
		t.Errorf("空结果时页签是空白的：%q", texts)
	}
}

// TestHomeSwitchTabWithoutSourceStaysLocal pins the state machine: switching a
// tab with nothing looked up yet must not start a request, and the tab survives
// a clear so the user keeps the view they chose.
func TestHomeSwitchTabWithoutSourceStaysLocal(t *testing.T) {
	a := newTestApp()
	a.homeSwitchTab("writing")
	if a.home.tab != "writing" {
		t.Fatalf("tab = %q, want writing", a.home.tab)
	}
	if a.home.gen != 0 || a.home.busy {
		t.Error("切换页签在没有查询内容时启动了请求")
	}

	a.home.input = "hello"
	a.home.source = "hello"
	a.home.target = "你好"
	a.home.recent = []string{"hello"}
	a.homeClear()
	if a.home.input != "" || a.home.source != "" || a.home.target != "" {
		t.Errorf("清空后仍有残留：%+v", a.home)
	}
	if a.home.tab != "writing" {
		t.Errorf("清空把页签重置成了 %q", a.home.tab)
	}
	if len(a.home.recent) != 1 || a.home.recent[0] != "hello" {
		t.Errorf("清空把快捷复查列表弄丢了：%q", a.home.recent)
	}
}

// TestMainWindowIsTheDefaultEntry pins the delivery shape: a plain launch opens
// the workbench, an autostart stays in the tray, and the tray's left click is
// the way back to the window.
func TestMainWindowIsTheDefaultEntry(t *testing.T) {
	src := readSourceFile(t, "main.go")
	for _, want := range []string{
		`a.showMainWindow("home")`,
		`case "--minimized", "-minimized", "--autostart":`,
		"t.OnClick(a.toggleMainWindow)",
		"打开主窗口（查词与翻译）",
	} {
		if !containsText(src, want) {
			t.Errorf("main.go 缺少 %q", want)
		}
	}
	if containsText(src, "t.OnClick(a.toggleTrayFlyout)") {
		t.Error("托盘左键仍是快捷面板，没有直达主窗口")
	}
	// 无头测试与自动启动不能弹窗，否则截图和开机启动都变了行为。
	if !containsText(src, "else if !headless && !minimizedFlag") {
		t.Error("启动弹窗没有排除无头模式与最小化启动")
	}
}
