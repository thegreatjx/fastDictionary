package main

import (
	"context"
	"fmt"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"log"
	"os"
	"time"
)

// langOptions is the languages the translator offers, in the order the picker
// shows them: the ones a Chinese reader meets most, then the rest of the
// common set.
//
// The engines accept far more than this — both Microsoft and Google take any
// BCP-47 tag they support, and a test of fourteen of them (Thai, Vietnamese,
// Indonesian, Turkish, Arabic, Hindi, Portuguese, Italian, Dutch, Polish,
// Ukrainian, Hebrew, Persian, Swedish) came back translated in under a second
// each. The limit was this list, not the service: nine languages meant a user
// reading Thai had no target to pick and no way to translate into it.
var langOptions = []string{
	// Most used by a Chinese reader.
	"zh-Hans", "zh-Hant", "en", "ja", "ko",
	// East and Southeast Asia.
	"th", "vi", "id", "ms", "tl",
	// Europe.
	"fr", "de", "es", "pt", "it", "nl", "ru", "uk", "pl", "sv", "da",
	"no", "fi", "cs", "ro", "hu", "el", "tr",
	// Middle East and South Asia.
	"ar", "he", "fa", "hi", "bn", "ur",
}

var langLabels = map[string]string{
	"zh-Hans": "简体中文", "zh-Hant": "繁体中文", "en": "英语", "ja": "日语",
	"ko": "韩语", "th": "泰语", "vi": "越南语", "id": "印尼语", "ms": "马来语",
	"tl": "菲律宾语", "fr": "法语", "de": "德语", "es": "西班牙语",
	"pt": "葡萄牙语", "it": "意大利语", "nl": "荷兰语", "ru": "俄语",
	"uk": "乌克兰语", "pl": "波兰语", "sv": "瑞典语", "da": "丹麦语",
	"no": "挪威语", "fi": "芬兰语", "cs": "捷克语", "ro": "罗马尼亚语",
	"hu": "匈牙利语", "el": "希腊语", "tr": "土耳其语", "ar": "阿拉伯语",
	"he": "希伯来语", "fa": "波斯语", "hi": "印地语", "bn": "孟加拉语",
	"ur": "乌尔都语",
}

// langList returns the labels the language picker shows, in langOptions order.
func langList() []string {
	out := make([]string, 0, len(langOptions))
	for _, l := range langOptions {
		out = append(out, langLabels[l])
	}
	return out
}

func langFromLabel(label string) string {
	for code, name := range langLabels {
		if name == label {
			return code
		}
	}
	return "zh-Hans"
}

// settingsRefresh repaints the settings window after a change that alters
// the structure of a page (a list that grew or shrank).
func (a *app) settingsRefresh() {
	if a.cfgWin != nil && !a.cfgWin.IsDestroyed() {
		a.cfgWin.Update(func() {})
	}
}

// settingsView renders the settings window: a sidebar of pages and a form.
func (a *app) settingsView(c *ui.Context) {
	cfg := a.store.Get()
	applyTheme(c, cfg.ThemeMode, c.Theme().Dark)
	t := c.Theme()

	if a.page == "" {
		a.page = "general"
	}
	if a.settingsPageSeen != a.page {
		a.settingsPageSeen = a.page
		a.settingsScroll.Y = 0
	}

	// The root row must fill the window in both directions. With only
	// FillWidth the row is as tall as its tallest child, the scroll view
	// then measures against an unbounded height, and the page can neither
	// scroll nor fit a small window: the content is simply cut off.
	ui.Row(c).FillWidth().FillHeight().Children(func() {
		ui.Column(c).Width(168).FillHeight().Padding(10, 8).Gap(2).
			Background(t.Surface).BorderWidth(0, 1, 0, 0).BorderColor(t.Border).Children(func() {
			ui.Text(c, appTitle).FontSize(14).FontWeight(600).Padding(6, 8, 6, 8).TextColor(t.Accent)
			ui.Sidebar(c, &a.page, func() {
				ui.SidebarItem(c, "general", nil, "通用")
				ui.SidebarItem(c, "selection", nil, "划词")
				ui.SidebarItem(c, "actions", nil, "AI 指令")
				ui.SidebarItem(c, "vocabulary", nil, "生词本")
				ui.SidebarItem(c, "translate", nil, "翻译")
				ui.SidebarItem(c, "dict", nil, "词典与发音")
				ui.SidebarItem(c, "ai", nil, "AI 网关")
				ui.SidebarItem(c, "hotkey", nil, "快捷键")
				ui.SidebarItem(c, "about", nil, "关于")
			})
		})

		ui.Scroll(c).TrackScroll(&a.settingsScroll).Grow(1).FillHeight().
			Padding(24, 28, 24, 28).ScrollbarInsets(0, 0, 8).Children(func() {
			// One measured column instead of controls stretched across the
			// whole window: a form reads as a form, not as a spreadsheet.
			ui.Column(c).FillWidth().Gap(18).Children(func() {
				// A failed save used to be only a log line: the switch moved,
				// the window looked saved, and the setting vanished on restart.
				// Keep the error visible until a retry actually succeeds.
				if saveErr := a.store.LastSaveError(); saveErr != nil {
					ui.Box(c).FillWidth().Padding(10, 12).Radius(8).
						Background(ui.RGBA(239, 68, 68, 0.08)).
						Border(1, ui.RGBA(239, 68, 68, 0.35)).Children(func() {
						ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(10).Children(func() {
							ui.Icon(c, iconInfo).Size(14, 14).TextColor(t.Danger)
							ui.Text(c, "设置未保存：磁盘写入失败。"+saveErr.Error()).FontSize(12).TextColor(t.Danger).Wrap().Grow(1)
							if ui.Button(c, "重试").Height(24).Padding(2, 10).FontSize(11).Clicked() {
								a.retryConfigSave()
							}
							if ui.Button(c, "查看日志").Height(24).Padding(2, 10).FontSize(11).Clicked() {
								a.copyToClipboard(settingsLogPath())
								a.mu.Lock()
								a.actionSavedMsg = "日志路径已复制到剪贴板"
								a.mu.Unlock()
							}
						})
					})
				}
				switch a.page {
				case "general":
					a.pageGeneral(c, t, cfg)
				case "selection":
					a.pageSelection(c, t, cfg)
				case "actions":
					a.pageCustomActions(c, t, cfg)
				case "vocabulary":
					a.pageVocabulary(c, t, cfg)
				case "translate":
					a.pageTranslate(c, t, cfg)
				case "dict":
					a.pageDict(c, t, cfg)
				case "ai":
					a.pageAI(c, t, cfg)
				case "hotkey":
					a.pageHotkey(c, t, cfg)
				default:
					a.pageAbout(c, t, cfg)
				}
			})
		})
	})

	// Outcomes of the page's own actions are shown as toasts (mygo v0.5.5)
	// instead of inline banners that pushed the form down and stayed on
	// screen. This runs after the page rendered, so the message the page
	// just set is the one shown.
	// Both fields are written from background goroutines (import, export,
	// enrich, delete) under a.mu; reading and clearing them here without the
	// lock was a data race that could also lose the message.
	a.mu.Lock()
	feedback, savedMsg := a.actionSaveFeedback, a.actionSavedMsg
	a.actionSaveFeedback, a.actionSavedMsg = "", ""
	a.mu.Unlock()
	if feedback != "" {
		c.Toast(feedback)
	}
	if savedMsg != "" {
		c.Toast(savedMsg)
	}
}

func (a *app) retryConfigSave() {
	a.mu.Lock()
	a.actionSaveFeedback = "正在重试保存设置…"
	a.mu.Unlock()
	a.settingsRefresh()
	go func() {
		err := a.store.RetrySave()
		a.mu.Lock()
		if err != nil {
			a.actionSaveFeedback = "设置仍未保存：" + err.Error()
		} else {
			a.actionSaveFeedback = "设置已保存"
		}
		a.mu.Unlock()
		a.settingsRefresh()
	}()
}

func settingsLogPath() string {
	dir, err := mygo.App.Path(mygo.PathLogs)
	if err != nil || dir == "" {
		dir = os.Getenv("APPDATA")
		if dir != "" {
			dir = dir + string(os.PathSeparator) + appName
		}
	}
	return dir + string(os.PathSeparator) + logFile
}

// testGateway pings the magpie gateway and reports the result.
func (a *app) testGateway() {
	cfg := a.store.Get()
	a.mu.Lock()
	a.aiTest = "测试中…"
	a.mu.Unlock()
	if a.cfgWin != nil {
		a.cfgWin.Update(func() {})
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		start := time.Now()
		out, err := StreamMagpie(ctx, cfg.AIURL, cfg.AIModel, "You are a test probe. Reply with 'pong' only.", "ping", ResolveEffort(cfg.AIEffort, cfg.AIModel), nil)
		msg := ""
		if err != nil {
			msg = "失败：" + err.Error()
		} else {
			msg = fmt.Sprintf("正常（耗时: %v，模型: %s，返回: %s）",
				time.Since(start).Round(time.Millisecond),
				cfg.AIModel,
				truncateFor(out, 20))
		}
		a.mu.Lock()
		a.aiTest = msg
		a.mu.Unlock()
		if a.cfgWin != nil {
			a.cfgWin.Update(func() {})
		}
		log.Printf("fastDictionary: gateway test: %s", msg)
	}()
}

// applyHotkeys re-registers both global shortcuts from the pending fields.
func (a *app) applyHotkeys() {
	cfg := a.store.Get()
	sel := a.pendingHotkeySelection
	inp := a.pendingHotkeyInput
	ocr := a.pendingHotkeyScreenshot
	if sel == "" {
		sel = cfg.HotkeySelection
	}
	if inp == "" {
		inp = cfg.HotkeyInput
	}
	if ocr == "" {
		ocr = cfg.HotkeyScreenshot
	}
	prev := cfg
	msg := registerHotkeys(sel, inp, ocr, a)
	a.mu.Lock()
	a.hotkeyMsg = msg
	a.mu.Unlock()
	if msg == "" {
		a.store.Update(func(c *Config) {
			c.HotkeySelection = sel
			c.HotkeyInput = inp
			c.HotkeyScreenshot = ocr
		})
		a.pendingHotkeySelection = ""
		a.pendingHotkeyInput = ""
		a.pendingHotkeyScreenshot = ""
	} else {
		// Registration failed for at least one shortcut: put the working
		// set back and keep the old values on disk. The old condition here
		// was always true, so a typo in a shortcut field was saved and the
		// app came back up with no working shortcuts at all.
		log.Printf("fastDictionary: shortcut update rejected: %s", msg)
		registerHotkeys(prev.HotkeySelection, prev.HotkeyInput, prev.HotkeyScreenshot, a)
	}
	if a.cfgWin != nil {
		a.cfgWin.Update(func() {})
	}
}

func (a *app) initHotkeyDrafts() {
	cfg := a.store.Get()
	if a.pendingHotkeySelection == "" {
		a.pendingHotkeySelection = cfg.HotkeySelection
	}
	if a.pendingHotkeyInput == "" {
		a.pendingHotkeyInput = cfg.HotkeyInput
	}
	if a.pendingHotkeyScreenshot == "" {
		a.pendingHotkeyScreenshot = cfg.HotkeyScreenshot
	}
}

func (a *app) loadMagpieModels() {
	cfg := a.store.Get()
	a.mu.Lock()
	a.loadingModels = true
	a.mu.Unlock()
	go func() {
		models, err := FetchAvailableModels(cfg.AIURL)
		a.mu.Lock()
		a.loadingModels = false
		if err == nil && len(models) > 0 {
			a.magpieModels = models
		}
		a.mu.Unlock()
		if a.cfgWin != nil {
			a.cfgWin.Update(func() {})
		}
	}()
}

func (a *app) getAvailableModels() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.magpieModels) > 0 {
		return a.magpieModels
	}
	return []string{
		"workbuddy-ai/deepseek-v4.1-flash",
		"group/auto-deepseek-v4-1-flash",
		"workbuddy/deepseek-v4.1-flash",
		"antigravity/gemini-3.8-flash",
		"workbuddy/glm-5.3-flash",
		"workbuddy/kimi-k3-1",
		"minimax-code/MiniMax-M2.7-highspeed",
		"codex/gpt-5.5",
	}
}

// openAISettings jumps straight to the AI gateway page. A failed AI refine
// should lead to the setting that can fix it, not to a generic settings page
// the user then has to search.
func (a *app) openAISettings() {
	a.page = "ai"
	a.showSettings()
	a.settingsRefresh()
}

// showSettings opens (or focuses) the settings window.
func (a *app) showSettings() {
	a.initHotkeyDrafts()
	a.loadMagpieModels()
	if a.cfgWin != nil && !a.cfgWin.IsDestroyed() {
		a.cfgWin.Show()
		a.cfgWin.Focus()
		return
	}
	win := mygo.NewWindow(mygo.WindowOptions{
		Title:     appTitle + " 设置",
		Width:     880,
		Height:    640,
		MinWidth:  680,
		MinHeight: 480,
		Content:   ui.View(safeView("settings", a.settingsView)),
	})
	win.OnClose(func(e *mygo.CloseEvent) {
		if a.isQuitting {
			return
		}
		e.PreventDefault()
		win.Hide()
	})
	a.cfgWin = win
	win.Show()
	win.Focus()
}
