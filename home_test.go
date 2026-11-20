package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func renderHome(t *testing.T, a *app) []string {
	t.Helper()
	tt := ui.NewTester(func(c *ui.Context) {
		applyTheme(c, "light", false, "")
		a.pageHome(c, c.Theme(), a.store.Get())
	}, 920, 660)
	tt.Frame()
	return tt.Texts()
}

func TestHomeReadyStateKeepsTheInputAndAIHint(t *testing.T) {
	a := newTestApp()
	texts := strings.Join(renderHome(t, a), "")
	for _, want := range []string{
		"查词与翻译",
		"支持自动检测语种",
	} {
		if !strings.Contains(texts, want) {
			t.Errorf("首页空态缺少 %q，实际渲染 %q", want, texts)
		}
	}
	if strings.Contains(texts, "后台划词监听已就绪") {
		t.Errorf("首页仍在重复侧栏的监听状态：%q", texts)
	}
	if strings.Contains(texts, "左侧输入单词或长句") || strings.Contains(texts, "条待复习") {
		t.Errorf("首页仍包含冗余空态框或待复习提醒：%q", texts)
	}
	a.store.Update(func(c *Config) { c.AIEnabled = false })
	texts = strings.Join(renderHome(t, a), "")
	if !strings.Contains(texts, "配置 AI 服务后可解锁") {
		t.Errorf("首页没有 AI 配置引导：%q", texts)
	}
}

func TestHomeAndPopupRespectDisabledWritingAction(t *testing.T) {
	a := newTestApp()
	a.store.Update(func(c *Config) {
		for i := range c.CustomActions {
			if c.CustomActions[i].ID == "default-improve-writing" {
				c.CustomActions[i].Enabled = false
			}
		}
	})

	a.home.source = "hello"
	a.home.target = "你好"
	homeTexts := strings.Join(renderHome(t, a), "")
	if strings.Contains(homeTexts, "写作润色") {
		t.Errorf("关闭写作指令后工作台仍显示写作页签：%q", homeTexts)
	}
	if !strings.Contains(homeTexts, "长难句拆解") {
		t.Errorf("未关闭长难句时工作台缺少长难句页签：%q", homeTexts)
	}

	a.source = "hello"
	a.target = "你好"
	a.popupW = 500
	tt := ui.NewTester(a.popupView, 500, 340)
	tt.Frame()
	if tt.HasText("写作") {
		t.Errorf("关闭写作指令后弹窗仍显示写作页签：%v", tt.Texts())
	}
	if !tt.HasText("长难句") {
		t.Errorf("未关闭长难句时弹窗缺少长难句页签：%v", tt.Texts())
	}
}

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

func TestHomeGrammarTabIsNeverBlank(t *testing.T) {
	a := newTestApp()
	a.home.source = "Only then did she realize the truth."
	a.home.tab = "grammar"

	texts := strings.Join(renderHome(t, a), "")
	if !strings.Contains(texts, "还没有拆解结果") {
		t.Errorf("空结果时页签是空白的：%q", texts)
	}
}

func TestHomeTranslateTabIsNeverBlank(t *testing.T) {
	a := newTestApp()
	a.home.source = "hello"
	a.home.tab = "translate"

	texts := strings.Join(renderHome(t, a), "")
	if !strings.Contains(texts, "这次没有取到译文") {
		t.Errorf("空译文时页签是空白的：%q", texts)
	}
}

func TestHomeShowsSaveAndCopyFeedback(t *testing.T) {
	a := newTestApp()
	a.actionSavedMsg = "已保存到生词本 ✓"

	texts := strings.Join(renderHome(t, a), "")
	if !strings.Contains(texts, "已保存到生词本") {
		t.Errorf("主窗口没有渲染保存反馈：%q", texts)
	}
}

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

func TestMainWindowIsTheDefaultEntry(t *testing.T) {
	src := readSourceFile(t, "main.go")
	for _, want := range []string{
		"a.showMainWindow(startPage)",
		`startPage = "home"`,
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
	if !containsText(src, "else if !headless && !minimizedFlag") {
		t.Error("启动弹窗没有排除无头模式与最小化启动")
	}
}
