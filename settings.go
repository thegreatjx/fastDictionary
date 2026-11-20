package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

var langOptions = func() []string {
	out := append([]string(nil), langPrimary...)
	for _, group := range langMoreGroups {
		out = append(out, group...)
	}
	return out
}()

var langPrimary = []string{"zh-Hans", "zh-Hant", "en", "ja", "ko"}

var langMoreGroups = [][]string{
	{"th", "vi", "id", "ms", "tl"},
	{"fr", "de", "es", "pt", "it", "nl", "ru", "uk", "pl", "sv", "da", "no", "fi", "cs", "ro", "hu", "el", "tr"},
	{"ar", "he", "fa", "hi", "bn", "ur"},
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

func buildLangMenu(m *ui.Menu, current string, onPick func(tag string)) {
	for _, tag := range langPrimary {
		if m.Item(langLabels[tag]).Checked(tag == current).Chosen() {
			onPick(tag)
		}
	}
	if inLangMore(current) {
		m.Separator()
		m.Submenu("更多语言（"+langLabels[current]+"）", func(sub *ui.Menu) {
			addMoreLangItems(sub, current, onPick)
		})
		return
	}
	m.Separator()
	m.Submenu("更多语言", func(sub *ui.Menu) {
		addMoreLangItems(sub, current, onPick)
	})
}

func addMoreLangItems(sub *ui.Menu, current string, onPick func(tag string)) {
	for i, group := range langMoreGroups {
		if i > 0 {
			sub.Separator()
		}
		for _, tag := range group {
			if sub.Item(langLabels[tag]).Checked(tag == current).Chosen() {
				onPick(tag)
			}
		}
	}
}

func inLangMore(tag string) bool {
	for _, group := range langMoreGroups {
		for _, t := range group {
			if t == tag {
				return true
			}
		}
	}
	return false
}

func (a *app) settingsRefresh() {
	if a.cfgWin != nil && !a.cfgWin.IsDestroyed() {
		a.cfgWin.Update(func() {})
	}
}

func (a *app) refreshThemeWindows() {
	for _, w := range []*mygo.Window{a.win, a.cfgWin} {
		if w != nil && !w.IsDestroyed() {
			w.Update(func() {})
		}
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

var settingsNavGroups = []settingsNavGroup{
	{"工作台", []settingsNavEntry{
		{"home", "查词翻译", iconTranslate},
		{"vocabulary", "生词本", iconBookmark},
		{"notebook", "句法笔记", iconGrammar},
	}},
	{"偏好设置", []settingsNavEntry{
		{"general", "通用", iconGear},
		{"hotkey", "快捷键", iconKeyboard},
		{"selection", "划词", iconSearch},
		{"translate", "翻译", iconSwap},
		{"dict", "词典与发音", iconDict},
	}},
	{"AI 能力", []settingsNavEntry{
		{"ai", "AI 精翻", iconPolish},
		{"actions", "AI 指令", iconSparkles},
	}},
}

var settingsAboutNav = settingsNavEntry{"about", "关于", iconInfo}

func (a *app) settingsNavItem(c *ui.Context, t *ui.Theme, at AppTheme, item settingsNavEntry) {
	active := a.page == item.id
	b := ui.ButtonBase(c).Height(32).Radius(RadiusSM).
		Role(ui.RoleTreeItem).Focusable().FillWidth().
		Padding(0, Space2+2).Gap(Space2 + 2).Justify(ui.Start).AlignItems(ui.Center)
	if active {
		b.Background(at.AccentSoft).Border(1, at.AccentRing)
	} else if b.Hovered() {
		b.Background(at.SurfaceHover).Border(1, ui.Transparent)
	} else {
		b.Background(ui.Transparent).Border(1, ui.Transparent)
	}
	iconCol := at.TextSec
	textCol := at.TextSec
	weight := 500
	if active {
		iconCol, textCol, weight = at.Accent, at.Accent, 600
	}
	count := 0
	switch item.id {
	case "vocabulary":
		count = GetVocabularyStore().Count()
	case "notebook":
		count = GetNotebookStore().Count()
	}
	b.Children(func() {
		ui.Box(c).Width(IconSize).Height(IconSize).Shrink(0).Center().Children(func() {
			ui.Icon(c, item.icon).Size(IconSize, IconSize).TextColor(iconCol)
		})
		ui.Text(c, item.label).Font(fontUI).FontSize(FontControl).
			FontWeight(weight).TextColor(textCol).SingleLine().Grow(1)
		if count > 0 {
			badgeBg := at.SurfaceHover
			badgeFg := at.TextMuted
			if active {
				badgeBg = at.Surface
				badgeFg = at.Accent
			}
			ui.Box(c).Padding(1, 6).Radius(10).Background(badgeBg).Shrink(0).Children(func() {
				ui.Text(c, fmt.Sprintf("%d", count)).Font(fontMono).FontFeatures("tnum").
					FontSize(FontMicro).FontWeight(600).TextColor(badgeFg).SingleLine()
			})
		}
	})
	if b.Clicked() && !active {
		a.page = item.id
		a.settingsRefresh()
	}
}

func (a *app) settingsView(c *ui.Context) {
	cfg := a.store.Get()
	applyTheme(c, "light", false, cfg.AccentTheme)
	t := c.Theme()
	at := appTheme(t)

	if a.page == "" {
		a.page = "general"
	}
	if a.settingsPageSeen != a.page {
		a.settingsPageSeen = a.page
		a.settingsScroll.Y = 0
	}

	ui.Row(c).FillWidth().FillHeight().Children(func() {
		ui.Column(c).Width(NavWidth).FillHeight().Padding(Space4, Space3, Space3, Space3).Gap(Space2).
			Background(at.Surface).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(Space2+2).Padding(0, Space2, Space1, Space2).Children(func() {
				ui.Box(c).Size(28, 28).Radius(RadiusSM).Background(at.Accent).Center().Shrink(0).Children(func() {
					ui.Icon(c, iconDict).Size(15, 15).TextColor(at.AccentText)
				})
				ui.Text(c, appTitle).Font(fontUI).FontSize(FontSection).FontWeight(700).
					TextColor(at.Text).SingleLine().Grow(1)
				ui.Box(c).Padding(1, 6).Radius(RadiusXS).Background(at.SurfaceHover).Shrink(0).Children(func() {
					ui.Text(c, "v"+appVersion).Font(fontMono).FontFeatures("tnum").FontSize(10).
						FontWeight(500).TextColor(at.TextMuted).SingleLine()
				})
			})
			ui.Scroll(c).TrackScroll(&a.navScroll).Grow(1).FillWidth().
				ScrollbarInsets(0, 0, Space2).Children(func() {
				ui.Column(c).FillWidth().Gap(2).Children(func() {
					for i, group := range settingsNavGroups {
						if i > 0 {
							ui.Box(c).FillWidth().Padding(Space2, Space2, 2, Space2).Children(func() {
								ui.Box(c).FillWidth().Height(1).Background(at.BorderSubtle)
							})
						}
						ui.Text(c, group.title).Font(fontUI).FontSize(FontMicro).FontWeight(600).
							Padding(Space1, Space2+2, 4, Space2+2).TextColor(at.TextMuted).SingleLine()
						for _, item := range group.items {
							a.settingsNavItem(c, t, at, item)
						}
					}
					ui.Box(c).FillWidth().Padding(Space2, Space2, 2, Space2).Children(func() {
						ui.Box(c).FillWidth().Height(1).Background(at.BorderSubtle)
					})
					a.settingsNavItem(c, t, at, settingsAboutNav)
				})
			})
			paused := !a.selectionPausedUntil().IsZero()
			dotCol := at.Accent
			statusText := "划词监听中"
			if paused {
				dotCol = at.Warning
				statusText = "划词已暂停"
			} else if cfg.SelectionTrigger == "hotkey" {
				dotCol = at.TextMuted
				statusText = "仅快捷键"
			}
			statusBtn := ui.ButtonBase(c).FillWidth().Radius(RadiusSM).Padding(Space2, Space2+2).
				Background(at.SurfaceHover).Border(1, at.BorderSubtle).Justify(ui.Start).AlignItems(ui.Center)
			if statusBtn.Hovered() {
				statusBtn.Background(at.SurfacePress).Border(1, at.BorderHover)
			}
			statusBtn.Children(func() {
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(Space2).Children(func() {
					ui.Box(c).Size(7, 7).Radius(4).Background(dotCol).Shrink(0)
					ui.Text(c, statusText).Font(fontUI).FontSize(FontHelper).FontWeight(500).
						TextColor(at.TextSec).SingleLine().Grow(1)
					if hk := strings.TrimSpace(cfg.HotkeySelection); hk != "" {
						KeycapBadge(c, t, hk)
					}
				})
			})
			if statusBtn.Clicked() {
				a.page = "selection"
				a.settingsRefresh()
			}
		})
		ui.Box(c).Width(1).FillHeight().Background(at.BorderSubtle)

		ui.Scroll(c).TrackScroll(&a.settingsScroll).Grow(1).FillHeight().
			Padding(Space6, Space8, Space8+Space4, Space8).ScrollbarInsets(0, 0, Space2).Children(func() {
			ui.Column(c).FillWidth().MaxWidth(ContentMaxWidth).Gap(Space6).Children(func() {
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
				case "notebook":
					a.pageNotebook(c, t, cfg)
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

	a.mu.Lock()
	feedback := a.actionSaveFeedback
	a.actionSaveFeedback = ""
	var savedMsg string
	if a.page != "home" {
		savedMsg = a.actionSavedMsg
		a.actionSavedMsg = ""
	}
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
		go a.rebuildTray()
		return
	}

	log.Printf("fastDictionary: shortcut update rejected: %s", msg)
	if rollback := registerHotkeys(prev.HotkeySelection, prev.HotkeyInput, prev.HotkeyScreenshot, a); rollback != "" {
		msg += "原快捷键也未能恢复：" + rollback
	}
	a.setHotkeyMsg(hotkeyMsgError, msg)
}

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

func (a *app) openAISettings() {
	a.showMainWindow("ai")
}

func (a *app) showMainWindow(page string) {
	if page != "" {
		a.page = page
	} else if a.page == "" {
		a.page = "home"
	}
	if a.trayFlyout != nil && !a.trayFlyout.IsDestroyed() {
		a.trayFlyout.Close()
		a.trayFlyout = nil
	}
	if a.cfgWin != nil && !a.cfgWin.IsDestroyed() {
		a.resetVocabularyConfirmations()
		a.resetNotebookConfirmations()
		if a.page == "ai" {
			a.loadAIModels()
		}
		a.mainWinVisible = true
		a.cfgWin.Show()
		a.cfgWin.Focus()
		a.cfgWin.Update(func() {})
		return
	}
	a.hotkeyDraftsInit = false
	a.initHotkeyDrafts()
	a.loadAIModels()
	a.resetVocabularyViewState()
	a.resetNotebookViewState()
	win := mygo.NewWindow(mygo.WindowOptions{
		Title:     appTitle,
		Width:     960,
		Height:    680,
		MinWidth:  740,
		MinHeight: 520,
		Content:   ui.View(safeView("settings", a.settingsView)),
	})
	win.OnClose(func(e *mygo.CloseEvent) {
		if a.isQuitting.Load() {
			return
		}
		e.PreventDefault()
		a.mainWinVisible = false
		win.Hide()
	})
	a.cfgWin = win
	a.mainWinVisible = true
	win.Show()
	win.Focus()
}

func (a *app) toggleMainWindow() {
	if a.cfgWin != nil && !a.cfgWin.IsDestroyed() && a.mainWinVisible {
		a.mainWinVisible = false
		a.cfgWin.Hide()
		return
	}
	a.showMainWindow("home")
}

func (a *app) showSettings() {
	if a.page == "" || a.page == "home" {
		a.page = "general"
	}
	a.showMainWindow(a.page)
}
