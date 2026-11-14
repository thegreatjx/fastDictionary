package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
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

type settingsNavEntry struct {
	id    string
	label string
	icon  *ui.SVG
}

type settingsNavGroup struct {
	title string
	items []settingsNavEntry
}

// 导航按用户心智分组。分组只影响侧栏，页面路由仍由 id 决定。
var settingsNavGroups = []settingsNavGroup{
	// 工作台放最上面：主窗口默认打开的就是这一页，关掉浮窗后也能直接回来。
	{"工作台", []settingsNavEntry{
		{"home", "查词翻译", iconTranslate},
		{"vocabulary", "生词本", iconBookmark},
	}},
	{"常规", []settingsNavEntry{
		{"general", "通用", iconGear},
		{"hotkey", "快捷键", iconKeyboard},
	}},
	{"划词与翻译", []settingsNavEntry{
		{"selection", "划词", iconSearch},
		{"translate", "翻译", iconSwap},
		{"dict", "词典与发音", iconDict},
	}},
	{"AI 与指令", []settingsNavEntry{
		{"ai", "AI 精翻", iconPolish},
		{"actions", "AI 指令", iconSparkles},
	}},
	{"其他", []settingsNavEntry{
		{"about", "关于", iconInfo},
	}},
}

// settingsNavItem 是 32px 的导航行：图标占固定列宽并左对齐，选中态用
// 圆角淡蓝整行高亮，不再靠外置竖条区分。
func (a *app) settingsNavItem(c *ui.Context, t *ui.Theme, at AppTheme, item settingsNavEntry) {
	active := a.page == item.id
	b := ui.ButtonBase(c).Height(HeightControl).Radius(RadiusSM).
		Role(ui.RoleTreeItem).Focusable().FillWidth().
		Padding(0, Space2).Gap(Space2).Justify(ui.Start).AlignItems(ui.Center)
	if active {
		b.Background(at.AccentSoft)
	} else if b.Hovered() {
		b.Background(at.SurfaceHover)
	}
	iconCol := at.TextSec
	textCol := at.TextSec
	weight := 500
	if active {
		iconCol, textCol, weight = at.Accent, at.Text, 600
	}
	// 生词本后面直接带数量：侧栏是唯一能看到「存了多少」的地方。
	label := item.label
	if item.id == "vocabulary" {
		if n := GetVocabularyStore().Count(); n > 0 {
			label = fmt.Sprintf("生词本 (%d)", n)
		}
	}
	b.Children(func() {
		ui.Box(c).Width(IconSize).Height(IconSize).Shrink(0).Center().Children(func() {
			ui.Icon(c, item.icon).Size(IconSize, IconSize).TextColor(iconCol)
		})
		ui.Text(c, label).Font(fontUI).FontSize(FontControl).
			FontWeight(weight).TextColor(textCol).SingleLine()
	})
	if b.Clicked() && !active {
		a.page = item.id
		a.settingsRefresh()
	}
}

// settingsView renders the settings window: a sidebar of pages and a form.
func (a *app) settingsView(c *ui.Context) {
	cfg := a.store.Get()
	applyTheme(c, cfg.ThemeMode, c.Theme().Dark)
	t := c.Theme()
	at := appTheme(t)

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
		// 自定义导航：框架 ui.Sidebar 的选中态是实心 accent 块，无法改成
		// 「浅底 + 左侧竖条」。用 ButtonBase + RoleTreeItem 保留焦点与辅助技术。
		ui.Column(c).Width(NavWidth).FillHeight().Padding(Space3).Gap(Space1).
			Background(at.Canvas).Children(func() {
			// 主窗口标识：右上角的状态胶囊是常驻的，标题行只是把身份说清楚。
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(Space2).Padding(Space1, Space2).Children(func() {
				ui.Box(c).Size(24, 24).Radius(RadiusSM).Background(at.AccentSoft).Center().Shrink(0).Children(func() {
					ui.Icon(c, iconDict).Size(14, 14).TextColor(at.Accent)
				})
				ui.Text(c, appTitle).Font(fontUI).FontSize(FontControl).FontWeight(600).
					TextColor(at.Text).SingleLine()
			})
			// 导航组单独滚动：窗口压到最小高度时，五组入口加底部胶囊装不下，
			// 没有这个滚动区最后几项就会被直接裁掉。标题行和胶囊保持常驻。
			ui.Scroll(c).TrackScroll(&a.navScroll).Grow(1).FillWidth().
				ScrollbarInsets(0, 0, Space2).Children(func() {
				ui.Column(c).FillWidth().Gap(Space1).Children(func() {
					for _, group := range settingsNavGroups {
						ui.Text(c, group.title).Font(fontUI).FontSize(FontMicro).FontWeight(500).
							Padding(Space2, Space2, Space1, Space2).TextColor(at.TextMuted).SingleLine()
						for _, item := range group.items {
							a.settingsNavItem(c, t, at, item)
						}
					}
				})
			})
			// 常驻状态胶囊：不管用户停在哪一页，都能看到划词监听是否真的在跑。
			paused := !a.selectionPausedUntil().IsZero()
			dotCol := at.Success
			statusText := "划词监听中"
			if paused {
				dotCol = at.Warning
				statusText = "划词已暂停"
			} else if cfg.SelectionTrigger == "hotkey" {
				statusText = "仅快捷键划词"
			}
			statusBtn := ui.ButtonBase(c).FillWidth().Radius(RadiusSM).Padding(Space2, Space2).
				Background(at.Surface).Border(1, at.BorderSubtle).Justify(ui.Start).AlignItems(ui.Center)
			if statusBtn.Hovered() {
				statusBtn.Background(at.SurfaceHover)
			}
			statusBtn.Children(func() {
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(Space2).Children(func() {
					ui.Box(c).Size(6, 6).Radius(RadiusXS).Background(dotCol).Shrink(0)
					ui.Column(c).Grow(1).Children(func() {
						ui.Text(c, statusText).Font(fontUI).FontSize(FontMicro).FontWeight(600).TextColor(at.Text).SingleLine()
						if hk := strings.TrimSpace(cfg.HotkeySelection); hk != "" {
							ui.Text(c, "快捷键 "+hk).Font(fontMono).FontSize(10).TextColor(at.TextMuted).SingleLine()
						}
					})
				})
			})
			if statusBtn.Clicked() {
				a.page = "selection"
				a.settingsRefresh()
			}
		})

		ui.Scroll(c).TrackScroll(&a.settingsScroll).Grow(1).FillHeight().
			Padding(Space6, Space8, Space8+Space4, Space8).ScrollbarInsets(0, 0, Space2).Children(func() {
			// One measured column instead of controls stretched across the
			// whole window: a form reads as a form, not as a spreadsheet.
			ui.Column(c).FillWidth().MaxWidth(ContentMaxWidth).Gap(Space6).Children(func() {
				// A failed save used to be only a log line: the switch moved,
				// the window looked saved, and the setting vanished on restart.
				// Keep the error visible until a retry actually succeeds.
				if saveErr := a.store.LastSaveError(); saveErr != nil {
					SurfaceCard(c, t).Background(dangerSoft(t)).Border(1, dangerLine(t)).Children(func() {
						ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(Space2).Children(func() {
							ui.Icon(c, iconInfo).Size(IconSize, IconSize).TextColor(t.Danger)
							HelperText(c, t, "设置未保存：磁盘写入失败。"+saveErr.Error()).
								TextColor(t.Danger).Grow(1)
							if SecondaryButton(c, t, "重试").Clicked() {
								a.retryConfigSave()
							}
							if SecondaryButton(c, t, "查看日志").Clicked() {
								a.copyToClipboard(settingsLogPath())
								a.mu.Lock()
								a.actionSavedMsg = "日志路径已复制到剪贴板"
								a.mu.Unlock()
							}
						})
					})
				}
				switch a.page {
				case "home":
					a.pageHome(c, t, cfg)
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
		defer recoverLog("settings.go:retryConfigSave")
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

func canonicalHotkey(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	parts := strings.Split(s, "+")
	var mods []string
	var key string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		switch p {
		case "CTRL", "CONTROL":
			mods = append(mods, "CTRL")
		case "ALT", "MENU":
			mods = append(mods, "ALT")
		case "SHIFT":
			mods = append(mods, "SHIFT")
		case "WIN", "WINDOWS", "SUPER":
			mods = append(mods, "WIN")
		default:
			key = p
		}
	}
	uniqueMods := make(map[string]bool)
	var orderedMods []string
	for _, m := range []string{"ALT", "CTRL", "SHIFT", "WIN"} {
		for _, mod := range mods {
			if mod == m && !uniqueMods[m] {
				uniqueMods[m] = true
				orderedMods = append(orderedMods, m)
			}
		}
	}
	if key != "" {
		orderedMods = append(orderedMods, key)
	}
	return strings.Join(orderedMods, "+")
}

func normalizeHotkey(s string) string {
	return canonicalHotkey(s)
}

// checkHotkeyConflict reports whether any two non-empty shortcuts are identical.
func checkHotkeyConflict(sel, inp, ocr string) string {
	nSel := canonicalHotkey(sel)
	nInp := canonicalHotkey(inp)
	nOcr := canonicalHotkey(ocr)

	if nSel != "" && nInp != "" && nSel == nInp {
		return fmt.Sprintf("快捷键冲突：划词快捷键与输入快捷键相同 (%s)，请分别指定不同按键", strings.TrimSpace(sel))
	}
	if nSel != "" && nOcr != "" && nSel == nOcr {
		return fmt.Sprintf("快捷键冲突：划词快捷键与截图快捷键相同 (%s)，请分别指定不同按键", strings.TrimSpace(sel))
	}
	if nInp != "" && nOcr != "" && nInp == nOcr {
		return fmt.Sprintf("快捷键冲突：输入快捷键与截图快捷键相同 (%s)，请分别指定不同按键", strings.TrimSpace(inp))
	}
	return ""
}

// applyHotkeys re-registers global shortcuts from the pending fields.
func (a *app) applyHotkeys() {
	if !a.hotkeyDraftsInit {
		a.initHotkeyDrafts()
	}
	sel := strings.TrimSpace(a.pendingHotkeySelection)
	inp := strings.TrimSpace(a.pendingHotkeyInput)
	ocr := strings.TrimSpace(a.pendingHotkeyScreenshot)
	if conflict := checkHotkeyConflict(sel, inp, ocr); conflict != "" {
		a.setHotkeyMsg(hotkeyMsgError, conflict)
		return
	}
	prev := a.store.Get()
	msg := registerHotkeys(sel, inp, ocr, a)
	if msg == "" {
		a.store.Update(func(c *Config) {
			c.HotkeySelection = sel
			c.HotkeyInput = inp
			c.HotkeyScreenshot = ocr
		})
		a.pendingHotkeySelection = sel
		a.pendingHotkeyInput = inp
		a.pendingHotkeyScreenshot = ocr
		a.setHotkeyMsg(hotkeyMsgSuccess, "快捷键已成功生效 ✓")
		// 托盘菜单标签里写着快捷键，菜单只在重建时读取配置。
		go a.rebuildTray()
		return
	}

	// 新的一组注册失败：重新注册旧的一组，磁盘上保留旧值。
	// 旧键可能已被别的程序占走，回滚失败时必须告诉用户快捷键已失效。
	log.Printf("fastDictionary: shortcut update rejected: %s", msg)
	if rollback := registerHotkeys(prev.HotkeySelection, prev.HotkeyInput, prev.HotkeyScreenshot, a); rollback != "" {
		msg += "原快捷键也未能恢复：" + rollback
	}
	a.setHotkeyMsg(hotkeyMsgError, msg)
}

// setHotkeyMsg records the 快捷键 page message and repaints the settings window.
func (a *app) setHotkeyMsg(kind hotkeyMsgKind, msg string) {
	a.mu.Lock()
	a.setHotkeyMsgLocked(kind, msg)
	a.mu.Unlock()
	if a.cfgWin != nil {
		a.cfgWin.Update(func() {})
	}
}

func (a *app) initHotkeyDrafts() {
	if a.hotkeyDraftsInit {
		return
	}
	cfg := a.store.Get()
	a.pendingHotkeySelection = cfg.HotkeySelection
	a.pendingHotkeyInput = cfg.HotkeyInput
	a.pendingHotkeyScreenshot = cfg.HotkeyScreenshot
	a.hotkeyDraftsInit = true
}

func (a *app) resetHotkeysToDefault() {
	def := DefaultConfig()
	a.pendingHotkeySelection = def.HotkeySelection
	a.pendingHotkeyScreenshot = def.HotkeyScreenshot
	a.pendingHotkeyInput = def.HotkeyInput
	a.applyHotkeys()
}

// openAISettings jumps straight to the AI page. A failed AI refine should lead
// to the setting that can fix it, not to a generic settings page the user then
// has to search.
func (a *app) openAISettings() {
	a.showMainWindow("ai")
}

// showMainWindow opens or focuses the one main window, on the given page.
// The main window now carries the workbench home page as well as the
// settings, so every entry point (tray, chip menu, startup) comes through
// here instead of building its own page state.
func (a *app) showMainWindow(page string) {
	if page != "" {
		a.page = page
	} else if a.page == "" {
		a.page = "home"
	}
	a.hotkeyDraftsInit = false
	a.initHotkeyDrafts()
	a.loadAIModels()
	a.resetVocabularyViewState()
	// The tray flyout duplicates the main window's home page; leaving it open
	// behind the window reads as a second, stale copy.
	if a.trayFlyout != nil && !a.trayFlyout.IsDestroyed() {
		a.trayFlyout.Close()
		a.trayFlyout = nil
	}
	if a.cfgWin != nil && !a.cfgWin.IsDestroyed() {
		a.mainWinVisible = true
		a.cfgWin.Show()
		a.cfgWin.Focus()
		a.cfgWin.Update(func() {})
		return
	}
	win := mygo.NewWindow(mygo.WindowOptions{
		Title:     appTitle,
		Width:     920,
		Height:    660,
		MinWidth:  700,
		MinHeight: 500,
		Content:   ui.View(safeView("settings", a.settingsView)),
	})
	win.OnClose(func(e *mygo.CloseEvent) {
		if a.isQuitting.Load() {
			return
		}
		// Closing the main window is not quitting: the selection trigger keeps
		// working in the tray, which is what makes the window safe to close.
		e.PreventDefault()
		a.mainWinVisible = false
		win.Hide()
	})
	a.cfgWin = win
	a.mainWinVisible = true
	win.Show()
	win.Focus()
}

// toggleMainWindow is the tray's left click: show the workbench, or hide the
// window when it is already in front.
func (a *app) toggleMainWindow() {
	if a.cfgWin != nil && !a.cfgWin.IsDestroyed() && a.mainWinVisible {
		a.mainWinVisible = false
		a.cfgWin.Hide()
		return
	}
	a.showMainWindow("home")
}

// showSettings opens the main window on a settings page.
//
// It is kept for the callers that mean "the settings", not "the workbench":
// the home page is what the tray and a plain start open, and landing there
// when the user asked for settings would be wrong.
func (a *app) showSettings() {
	if a.page == "" || a.page == "home" {
		a.page = "general"
	}
	a.showMainWindow(a.page)
}
