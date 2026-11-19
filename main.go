package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

var buildStamp = "dev"

var registerHotkeysFunc = defaultRegisterHotkeys

func defaultRegisterHotkeys(sel, inp, ocr string, a *app) string {
	mygo.GlobalShortcut.UnregisterAll()

	msg := ""
	if sel != "" {
		if err := mygo.GlobalShortcut.Register(sel, func() {
			go func() {
				defer recoverLog("main.go:hotkey selection")
				a.openPopup(modeSelection)
			}()
		}); err != nil {
			msg += "划词快捷键 " + sel + " 注册失败：" + err.Error() + "；"
		}
	}
	if inp != "" {
		if err := mygo.GlobalShortcut.Register(inp, func() {
			go func() {
				defer recoverLog("main.go:hotkey input")
				a.openPopup(modeInput)
			}()
		}); err != nil {
			msg += "输入快捷键 " + inp + " 注册失败：" + err.Error() + "；"
		}
	}
	if ocr != "" {
		if err := mygo.GlobalShortcut.Register(ocr, func() {
			go func() {
				defer recoverLog("main.go:hotkey screenshot")
				a.TriggerScreenshotOCR()
			}()
		}); err != nil {
			msg += "截图快捷键 " + ocr + " 注册失败：" + err.Error() + "；"
		}
	}
	if msg == "" {
		log.Printf("fastDictionary: shortcuts ready (selection=%s input=%s ocr=%s)", sel, inp, ocr)
	} else {
		log.Printf("fastDictionary: shortcut problem: %s", msg)
	}
	return msg
}

func registerHotkeys(sel, inp, ocr string, a *app) string {
	return registerHotkeysFunc(sel, inp, ocr, a)
}

func (a *app) trayMenu() *mygo.Menu {
	cfg := a.store.Get()
	items := []*mygo.MenuItem{
		{Label: "打开主窗口（查词与翻译）", Click: func(*mygo.MenuItem, *mygo.Window) { a.showMainWindow("home") }},
		{Label: fmt.Sprintf("生词本 (%d)", GetVocabularyStore().Count()), Click: func(*mygo.MenuItem, *mygo.Window) { a.showMainWindow("vocabulary") }},
		mygo.Separator(),
		{Label: "划词翻译 (" + cfg.HotkeySelection + ")", Click: func(*mygo.MenuItem, *mygo.Window) { a.openPopup(modeSelection) }},
		{Label: "输入翻译 (" + cfg.HotkeyInput + ")", Click: func(*mygo.MenuItem, *mygo.Window) { a.openPopup(modeInput) }},
		{Label: "截图翻译 (" + cfg.HotkeyScreenshot + ")", Click: func(*mygo.MenuItem, *mygo.Window) { a.TriggerScreenshotOCR() }},
	}
	if due := GetVocabularyStore().DueCount(time.Now()); due > 0 {
		items = append(items, &mygo.MenuItem{
			Label: fmt.Sprintf("复习生词 (%d 个到期)", due),
			Click: func(*mygo.MenuItem, *mygo.Window) { a.openVocabReview() },
		})
	}
	items = append(items,
		mygo.Separator(),
		&mygo.MenuItem{Label: "划词模式", Submenu: []*mygo.MenuItem{
			selectionModeItem(a, "icon", "显示小图标", cfg),
			selectionModeItem(a, "auto", "直接翻译", cfg),
			selectionModeItem(a, "hotkey", "仅快捷键", cfg),
		}},
		mygo.Separator(),
		pauseSelectionItem(a),
		mygo.Separator(),
		elevationMenuItem(a),
		&mygo.MenuItem{Label: "设置", Click: func(*mygo.MenuItem, *mygo.Window) { a.showSettings() }},
		mygo.Separator(),
		&mygo.MenuItem{Label: "退出 " + appTitle, Click: func(*mygo.MenuItem, *mygo.Window) { a.quitApp() }},
	)
	return mygo.NewMenu(items)
}

func (a *app) quitApp() {
	a.isQuitting.Store(true)

	stopAudio()
	if err := postHookCommand(removeHooks); err != nil {
		removeHooks()
	}

	if a.trayFlyout != nil && !a.trayFlyout.IsDestroyed() {
		a.trayFlyout.Close()
	}
	if a.moreFlyout != nil && !a.moreFlyout.IsDestroyed() {
		a.moreFlyout.Close()
	}
	if a.sel.win != nil && !a.sel.win.IsDestroyed() {
		a.sel.win.Close()
	}
	if a.win != nil && !a.win.IsDestroyed() {
		a.win.Close()
	}
	if a.cfgWin != nil && !a.cfgWin.IsDestroyed() {
		a.cfgWin.Destroy()
	}
	if a.tray != nil {
		a.tray.Destroy()
	}
	mygo.App.Exit(0)
}

func (a *app) buildTray() {
	t, err := mygo.NewTray(mygo.TrayOptions{
		Icon:    trayIcon(),
		ToolTip: appTitle + " (左键打开主窗口 / 右键快捷菜单)",
	})
	if err != nil {
		log.Println("fastDictionary: tray:", err)
		return
	}
	t.OnClick(a.toggleMainWindow)
	t.OnRightClick(func() {
		if a.trayFlyout != nil && !a.trayFlyout.IsDestroyed() {
			a.trayFlyout.Close()
		}
		t.SetMenu(a.trayMenu())
		defer t.SetMenu(nil)
		t.PopUpMenu()
	})
	a.tray = t
}

func (a *app) rebuildTray() {
	mygo.RunOnMain(func() {
		defer recoverLog("main.go:rebuildTray")
		if a.tray == nil {
			a.buildTray()
			return
		}
		a.tray.SetMenu(nil)
	})
}

func pauseSelectionItem(a *app) *mygo.MenuItem {
	if until := a.selectionPausedUntil(); !until.IsZero() {
		return &mygo.MenuItem{
			Label: "恢复划词（已暂停）",
			Click: func(*mygo.MenuItem, *mygo.Window) { a.resumeSelection() },
		}
	}
	return &mygo.MenuItem{
		Label: "暂停划词 30 分钟",
		Click: func(*mygo.MenuItem, *mygo.Window) { a.pauseSelection(30 * time.Minute) },
	}
}

func elevationMenuItem(a *app) *mygo.MenuItem {
	if currentProcessElevated() {
		return &mygo.MenuItem{Label: "已以管理员身份运行", Disabled: true}
	}
	return &mygo.MenuItem{
		Label: "以管理员身份重启…",
		Click: func(*mygo.MenuItem, *mygo.Window) {
			go func() {
				defer recoverLog("main.go:elevationRestart")
				if err := relaunchElevated(); err != nil {
					log.Printf("fastDictionary: %v", err)
					return
				}
				time.Sleep(800 * time.Millisecond)
				mygo.RunOnMain(func() { a.quitApp() })
			}()
		},
	}
}

func selectionModeItem(a *app, mode, label string, cfg Config) *mygo.MenuItem {
	return &mygo.MenuItem{
		Label:   label,
		Type:    mygo.MenuItemRadio,
		Checked: cfg.SelectionTrigger == mode,
		Click: func(*mygo.MenuItem, *mygo.Window) {
			a.store.Update(func(c *Config) { c.SelectionTrigger = mode })
			a.applySelectionMode()
			a.rebuildTray()
		},
	}
}

func setupLogging() {
	dir, err := mygo.App.Path(mygo.PathLogs)
	if err != nil || dir == "" {
		if appData := os.Getenv("APPDATA"); appData != "" {
			dir = filepath.Join(appData, appName)
		} else {
			return
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	f, err := openLogFile(filepath.Join(dir, logFile), logMaxBytes)
	if err != nil {
		return
	}
	log.SetOutput(f)
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Printf("fastDictionary: log %s (pid %d, rotates at %d KiB)", filepath.Join(dir, logFile), os.Getpid(), logMaxBytes/1024)
	if err := debug.SetCrashOutput(f, debug.CrashOptions{}); err != nil {
		log.Printf("fastDictionary: crash output unavailable: %v", err)
	}
}

func safeView(name string, view func(*ui.Context)) func(*ui.Context) {
	return func(c *ui.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("fastDictionary: panic rendering %s view: %v\n%s", name, r, debug.Stack())
				t := c.Theme()
				ui.Box(c).FillWidth().Padding(16, 12).Radius(8).
					Background(t.Surface).Border(1, t.Danger).Children(func() {
					ui.Column(c).Gap(6).Children(func() {
						ui.Text(c, "渲染出错："+name).FontSize(13).FontWeight(600).TextColor(t.Danger)
						ui.Text(c, fmt.Sprintf("%v", r)).FontSize(12).TextColor(t.TextMuted).Wrap()
						ui.Text(c, "已记录到日志，其余功能不受影响").FontSize(11).TextColor(t.TextMuted)
					})
				})
			}
		}()
		view(c)
	}
}

func main() {
	migration := migrateUserData()

	a := &app{
		store: newStore(),
		cache: newQueryCache(defaultQueryCachePath()),
	}
	cfg := a.store.Get()

	if !elevatedTakeoverRequested() && !mygo.App.RequestSingleInstanceLock() {
		return
	}
	setupLogging()
	log.Printf("fastDictionary: build %s", buildStamp)
	if migration != "" {
		log.Print(migration)
	}
	mygo.App.OnSecondInstance(func(args []string, workingDir string) {
		mygo.RunOnMain(func() {
			a.showMainWindow("home")
		})
	})
	activeStore = a.store

	mygo.App.WhenReady(func() {
		mygo.App.OnBeforeQuit(func(e *mygo.QuitEvent) {
			a.isQuitting.Store(true)
		})

		win := mygo.NewWindow(mygo.WindowOptions{
			Title:       appTitle,
			Width:       int(cfg.PopupWidth),
			Height:      340,
			MinWidth:    360,
			MinHeight:   int(popupMinHeight),
			Hidden:      true,
			SkipTaskbar: true,
			AlwaysOnTop: cfg.AlwaysOnTop,
			Frameless:   true,
			Panel:       true,
			Opacity:     cfg.WindowOpacity,
			Content:     ui.View(safeView("popup", a.popupView)),
		})
		a.win = win
		win.OnBlur(func() {
			a.mu.Lock()
			isPinned := a.pinned
			a.mu.Unlock()
			if !isPinned && a.store.Get().CloseOnBlur {
				stopAudio()
				a.stopScreenshotOCR()
				win.Hide()
			}
		})

		if msg := registerHotkeys(cfg.HotkeySelection, cfg.HotkeyInput, cfg.HotkeyScreenshot, a); msg != "" {
			a.setHotkeyMsgLocked(hotkeyMsgError, msg)
		}

		OnSelectionGesture = a.onSelectionGesture
		OnMouseDown = a.onGlobalMouseDown
		OnMouseWheel = a.onGlobalMouseWheel
		OnMouseMove = a.onChipPointer
		OnKeyDown = a.onGlobalKeyDown
		if cfg.SelectionTrigger != "hotkey" {
			runOnMainThread()
		}

		a.buildTray()

		warmEdgeTTSAuth()

		a.loadAIModels()

		log.Printf("fastDictionary: ready (selection=%s input=%s)", cfg.HotkeySelection, cfg.HotkeyInput)

		var shotPath, shotWindow, demoText, demoSelection, demoAction, demoOCR string
		shotSizeW, shotSizeH := 0, 0
		openSettingsFlag := false
		minimizedFlag := false
		reviewDemo := false
		chipDemo := false
		chipTipDemo := false
		previewDark := false
		shotDelay := 1500 * time.Millisecond
		args := os.Args[1:]
		for i, arg := range args {
			switch arg {
			case "--settings", "-settings":
				openSettingsFlag = true
			case "--minimized", "-minimized", "--autostart":
				minimizedFlag = true
			case "--input", "-input":
				a.openPopup(modeInput)
			case "--shot":
				if i+1 < len(args) {
					shotPath = args[i+1]
				}
			case "--shot-window":
				if i+1 < len(args) {
					shotWindow = args[i+1]
				}
			case "--page":
				if i+1 < len(args) {
					a.page = args[i+1]
				}
			case "--review":
				reviewDemo = true
				openSettingsFlag = true
			case "--edit-action":
				if i+1 < len(args) {
					a.editingActionID = args[i+1]
					if def := a.store.Get().GetAction(args[i+1]); def != nil {
						a.customizingAction = *def
					}
				}
			case "--hookstats":
				go func() {
					defer recoverLog("main.go:main")
					last := uint64(0)
					for i := 0; i < 120; i++ {
						sleepMS(1000)
						e, d, u, g, own := hookStats()
						if e != last {
							rejected, why := hookPolicyRejects()
							title, class := hookOwnRejects()
							log.Printf("fastDictionary: hook events=%d down=%d up=%d drag=%d skippedOwn=%d dragRejected=%d(%q) rejected=%q/%q",
								e, d, u, g, own, rejected, why, title, class)
							last = e
						}
					}
				}()
			case "--chip":
				chipDemo = true
			case "--chip-tip":
				chipDemo, chipTipDemo = true, true
			case "--ui-dark":
				previewDark = true
				a.store = &store{cfg: cfg}
			case "--demo":
				if i+1 < len(args) {
					demoText = args[i+1]
				}
			case "--demo-selection":
				if i+1 < len(args) {
					demoSelection = args[i+1]
				}
			case "--demo-action":
				if i+1 < len(args) {
					demoAction = args[i+1]
				}
			case "--demo-ocr":
				if i+1 < len(args) {
					demoOCR = args[i+1]
				}
			case "--shot-size":
				if i+1 < len(args) {
					var w, h int
					if n, _ := fmt.Sscanf(args[i+1], "%dx%d", &w, &h); n == 2 && w > 0 && h > 0 {
						shotSizeW, shotSizeH = w, h
					}
				}
			case "--shot-delay":
				if i+1 < len(args) {
					var ms int
					if _, err := fmt.Sscanf(args[i+1], "%d", &ms); err == nil && ms > 0 {
						shotDelay = time.Duration(ms) * time.Millisecond
					}
				}
			}
		}
		if previewDark {
			a.store.Update(func(c *Config) { c.ThemeMode = "dark" })
		}
		if chipDemo {
			removeHooks()
			a.mu.Lock()
			a.sel.text = "serendipity"
			a.mu.Unlock()
			go func() {
				defer recoverLog("main.go:main")
				sleepMS(600)
				a.showSelIcon(760, 520, "serendipity")
				sleepMS(500)
				a.logChipState()
				if chipTipDemo {
					anchorX := float32(chipPadH + chipButtonSize + chipRowGap)
					a.raiseChipTip("朗读选中文字", ui.Rect{X: anchorX, Y: chipPadV, W: chipButtonSize, H: chipButtonSize})
					sleepMS(500)
					a.logChipTipState()
				}
				if shotWindow == "more" {
					mygo.RunOnMain(func() { a.toggleMoreFlyout() })
					sleepMS(600)
					log.Printf("fastDictionary: demo more flyout open=%v", a.moreFlyout != nil)
				}
			}()
		}
		if demoText != "" {
			a.openPopup(modeInput)
			a.translate(demoText)
		}
		if demoSelection != "" {
			a.openPopupWithText(modeSelectionPreview, demoSelection)
		}
		if demoAction != "" {
			a.openPopup(modeSelectionPreview)
			a.mu.Lock()
			a.mode = modeSelection
			switch demoAction {
			case "grammar":
				a.mode = modeGrammar
				a.source = "I am highly confident that, if the cure actually worked, almost any Stage 4 cancer patient would tell you that human comprehension of its mechanism is completely irrelevant to them."
				a.activeActionID = "default-sentence-analysis"
				a.grammarResult = &SentenceAnalysisResult{
					BackboneSummary: "我十分确信：几乎任何一位四期癌症患者都会告诉你，人类是否理解其机制对他们无关紧要。",
					Segments: []Segment{
						{Text: "I", Role: RoleSubject, IsBackbone: true},
						{Text: " am", Role: RolePredicate, IsBackbone: true},
						{Text: " highly confident", Role: RoleObject, IsBackbone: true, BreakBefore: true, Note: "形容词短语作表语"},
						{Text: " that,", Role: RoleConnector, ClauseID: "c1", Note: "引导宾语从句"},
						{Text: " if", Role: RoleConnector, BreakBefore: true, ClauseID: "c2", Note: "条件状从，修饰 would tell"},
						{Text: " the cure", Role: RoleSubject, ClauseID: "c2"},
						{Text: " actually worked", Role: RolePredicate, ClauseID: "c2"},
						{Text: ",", Role: RolePunct, ClauseID: "c1"},
						{Text: " almost any Stage 4 cancer patient", Role: RoleSubject, BreakBefore: true, ClauseID: "c1"},
						{Text: " would tell", Role: RolePredicate, ClauseID: "c1"},
						{Text: " you", Role: RoleObject, ClauseID: "c1"},
						{Text: " that", Role: RoleConnector, BreakBefore: true, ClauseID: "c3", Note: "引导第二层宾语从句"},
						{Text: " human comprehension", Role: RoleSubject, ClauseID: "c3"},
						{Text: " of its mechanism", Role: RoleModifier, ClauseID: "c3", PhraseID: "p1", Note: "介词短语作后置定语，修饰 comprehension"},
						{Text: " is", Role: RolePredicate, ClauseID: "c3"},
						{Text: " completely irrelevant", Role: RoleObject, ClauseID: "c3", Note: "表语，说明 comprehension"},
						{Text: " to them", Role: RoleModifier, ClauseID: "c3", PhraseID: "p2", Note: "介词短语作形容词补足语，说明 irrelevant 的对象"},
						{Text: ".", Role: RolePunct},
					},
					Clauses: []ClauseInfo{
						{ID: "c1", Type: "宾语从句", ShortTag: "宾从", Target: "作 confident 的宾语"},
						{ID: "c2", ParentID: "c1", Type: "条件状语从句", ShortTag: "状从", Target: "修饰 would tell"},
						{ID: "c3", ParentID: "c1", Type: "宾语从句", ShortTag: "宾从", Target: "作 tell 的宾语"},
					},
					Phrases: []PhraseInfo{
						{ID: "p1", ParentID: "c3", Type: "介词短语", ShortTag: "介短", Role: "后置定语", Target: "修饰 comprehension"},
						{ID: "p2", ParentID: "c3", Type: "介词短语", ShortTag: "介短", Role: "补足语", Target: "修饰 irrelevant"},
					},
					Difficulties: []DifficultyPoint{
						{Title: "三层嵌套", SpanText: "that, if ..., almost any ... would tell you that ...", Explanation: "宾语从句里套条件状语从句，再套一层宾语从句；先抓每层的主谓，再补修饰。"},
						{Title: "分隔", SpanText: "that, if the cure actually worked,", Explanation: "that 与真正的主干被条件从句隔开，阅读时要先跳过插入的条件。"},
					},
					Translation: "我十分确信，如果这种疗法真的有效，几乎任何一位四期癌症患者都会告诉你：人类是否理解其背后的机制，对他们而言根本无关紧要。",
				}
			case "grammar-news", "grammar-tree":
				a.mode = modeGrammar
				a.source = "Japan's dispute with China over Prime Minister Sanae Takaichi's remarks on Taiwan has spread from diplomatic protests to export restrictions by Beijing on critical minerals used in everything from cars and semiconductors to military equipment."
				a.activeActionID = "default-sentence-analysis"
				a.showTree = demoAction == "grammar-tree"
				a.grammarResult = &SentenceAnalysisResult{
					BackboneSummary: "日本（与中国）的争端已（从外交抗议）蔓延到（北京对关键矿产的）出口限制。",
					Segments: []Segment{
						{Text: "Japan's dispute", Role: RoleSubject, IsBackbone: true, Note: "全句主语中心词"},
						{Text: " with China", Role: RoleModifier, PhraseID: "p1", Note: "介词短语定语，说明争端双方"},
						{Text: " over Prime Minister Sanae Takaichi's remarks", Role: RoleModifier, BreakBefore: true, PhraseID: "p2", Note: "介词短语定语，说明争端事由"},
						{Text: " on Taiwan", Role: RoleModifier, PhraseID: "p3", Note: "嵌套定语，限定 remarks 的主题"},
						{Text: " has spread", Role: RolePredicate, BreakBefore: true, IsBackbone: true, Note: "现在完成时核心谓语"},
						{Text: " from diplomatic protests", Role: RoleModifier, BreakBefore: true, PhraseID: "p4", Note: "起点状语，对应后面的 to ..."},
						{Text: " to export restrictions", Role: RoleModifier, BreakBefore: true, PhraseID: "p5", Note: "终点补足/结果状语（核心落点）"},
						{Text: " by Beijing", Role: RoleModifier, PhraseID: "p6", Note: "施事定语，修饰 export restrictions"},
						{Text: " on critical minerals", Role: RoleModifier, BreakBefore: true, PhraseID: "p7", Note: "介短定语，说明限制对象"},
						{Text: " used in everything", Role: RoleModifier, BreakBefore: true, PhraseID: "p8", Note: "过去分词短语作后置定语，相当于 which are used ..."},
						{Text: " from cars and semiconductors to military equipment", Role: RoleModifier, PhraseID: "p9", Note: "介短范围定语，举例说明 everything"},
						{Text: ".", Role: RolePunct},
					},
					Phrases: []PhraseInfo{
						{ID: "p1", Type: "介词短语", ShortTag: "介短", Role: "后置定语", Target: "修饰 dispute"},
						{ID: "p2", Type: "介词短语", ShortTag: "介短", Role: "后置定语", Target: "修饰 dispute"},
						{ID: "p3", ParentID: "p2", Type: "介词短语", ShortTag: "介短", Role: "后置定语", Target: "修饰 remarks"},
						{ID: "p4", Type: "介词短语", ShortTag: "介短", Role: "起点状语", Target: "修饰 has spread"},
						{ID: "p5", Type: "介词短语", ShortTag: "介短", Role: "终点状语", Target: "修饰 has spread"},
						{ID: "p6", ParentID: "p5", Type: "介词短语", ShortTag: "介短", Role: "后置定语", Target: "修饰 restrictions"},
						{ID: "p7", ParentID: "p5", Type: "介词短语", ShortTag: "介短", Role: "后置定语", Target: "修饰 restrictions"},
						{ID: "p8", ParentID: "p7", Type: "分词短语", ShortTag: "分词", Role: "后置定语", Target: "修饰 minerals"},
						{ID: "p9", ParentID: "p8", Type: "介词短语", ShortTag: "介短", Role: "范围定语", Target: "修饰 everything"},
					},
					Difficulties: []DifficultyPoint{
						{Title: "九层零从句链式后置修饰", SpanText: "with China over ... on Taiwan ... by Beijing on ... used in ... from ... to ...", Explanation: "全句没有任何从句，却靠 8 个介词短语和 1 个过去分词短语把 37 词连成一句；先抓 Japan's dispute has spread from ... to ... 骨架，再层层挂修饰。"},
						{Title: "过去分词省略关系代词", SpanText: "used in everything from cars and semiconductors to military equipment", Explanation: "used in ... 等价于定语从句 that are used in ...，后置修饰 critical minerals。"},
					},
					Translation: "日本与中国围绕高市早苗涉台言论产生的争端，已从外交抗议升级为北京对关键矿产的出口限制——这些矿产广泛应用于从汽车、半导体到军事装备的各类领域。",
				}
			case "writing":
				a.mode = modeWriting
				a.source = "pls check the attached file and give me ur feedback asap. thx!"
				a.target = "请查收附件并尽快给我反馈。谢谢！"
				a.activeActionID = "default-improve-writing"
				a.writingResult = &ImproveWritingResult{
					Setting: "正式 · 邮件给导师",
					Annotations: []WritingAnnotation{
						{Text: "pls check", Fix: "Could you please take a look at", Type: "register", Tag: "语气", Note: "对导师语气过于生硬随意；pls 是网络聊天缩写"},
						{Text: "give me ur feedback asap.", Fix: "share your feedback when you have a chance?", Type: "register", Tag: "得体", Note: "asap 显得过于催促，ur 是聊天缩写"},
						{Text: "thx!", Fix: "Thank you!", Type: "register", Tag: "礼貌", Note: "正式信函请写完整的 Thank you!"},
					},
					Improved: "Could you please take a look at the attached file and share your feedback when you have a chance? Thank you!",
					Summary:  "意图清晰，但对长辈或导师使用网络缩写（pls/ur/thx）和祈使句会显得失礼，建议改用委婉礼貌的问句。",
				}
			}
			a.mu.Unlock()
			a.fitHeight()
			if a.win != nil {
				a.win.Update(func() {})
			}
		}
		if demoOCR != "" {
			a.beginScreenshotOCR(winRect{Left: 120, Top: 120, Right: 420, Bottom: 360}, demoOCR)
		}
		if !cfg.Onboarded {
			a.store.Update(func(c *Config) { c.Onboarded = true })
		}
		headless := shotPath != "" || chipDemo || chipTipDemo || demoText != "" || demoSelection != "" || demoAction != "" || demoOCR != ""
		if reviewDemo {
			a.page = "vocabulary"
			a.showMainWindow("vocabulary")
			a.startVocabReview()
		} else if openSettingsFlag {
			a.showSettings()
		} else if !headless && !minimizedFlag {
			startPage := a.page
			if startPage == "" {
				startPage = "home"
			}
			a.showMainWindow(startPage)
		}

		if shotPath != "" {
			go func() {
				defer recoverLog("main.go:main")
				time.Sleep(shotDelay)
				target := win
				if shotWindow == "settings" && a.cfgWin != nil {
					target = a.cfgWin
				} else if shotWindow == "chip" && a.sel.win != nil {
					target = a.sel.win
				} else if shotWindow == "more" && a.moreFlyout != nil {
					target = a.moreFlyout
				} else if shotWindow == "chip-tip" && a.chipTip.win != nil {
					target = a.chipTip.win
				}
				if shotSizeW > 0 && shotSizeH > 0 {
					target.SetSize(shotSizeW, shotSizeH)
					time.Sleep(400 * time.Millisecond)
				}
				png, err := target.CapturePage()
				if err != nil {
					log.Printf("fastDictionary: capture failed: %v", err)
				} else if err := os.WriteFile(shotPath, png, 0o644); err != nil {
					log.Printf("fastDictionary: capture write failed: %v", err)
				} else {
					log.Printf("fastDictionary: captured %d bytes to %s", len(png), shotPath)
				}
				time.Sleep(200 * time.Millisecond)
				mygo.RunOnMain(func() {
					mygo.App.Quit()
				})
				go func() {
					defer recoverLog("main.go:main")
					time.Sleep(500 * time.Millisecond)
					os.Exit(0)
				}()
			}()
		}
	})

	mygo.App.OnWindowAllClosed(func() {})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
