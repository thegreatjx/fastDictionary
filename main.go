// fastDictionary — a fast desktop translator.
//
// Two global shortcuts: Ctrl+Alt+F reads the selection and shows the
// translation popup at the cursor; Ctrl+Alt+S opens the same popup in
// input mode. The popup is mygo native UI — no webview, so it appears at
// once. Engines: unauthenticated Microsoft translatetext racing Google
// gtx; a dictionary card with UK/US IPA and audio for single words; and
// an optional AI refinement streamed from Magpie or a direct
// OpenAI-compatible service.
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

// buildStamp is the time this binary was linked, injected by build.ps1 as
// `-X main.buildStamp=...`. A tray-resident app has no window to close and
// no visible version, so "am I running the build I just made?" had no
// answer: the shortcut kept launching an older binary while the fresh one
// sat on disk under a different name, and nothing on screen said which one
// was live. The stamp is logged at startup and shown in 关于.
var buildStamp = "dev"

var registerHotkeysFunc = defaultRegisterHotkeys

func defaultRegisterHotkeys(sel, inp, ocr string, a *app) string {
	mygo.GlobalShortcut.UnregisterAll()

	// The callbacks below are invoked by mygo on the window message-loop
	// thread, synchronously. Reading the selection takes hundreds of
	// milliseconds (UI Automation COM, a 600 ms modifier wait, a 500 ms
	// clipboard poll), and doing that on the message thread froze every
	// window of the app for the whole duration - the "the translator
	// stutters every time I press the shortcut" symptom. Each callback now
	// only hands the work to a goroutine; the UI calls inside are already
	// thread-safe because mygo marshals Show/Focus to the main thread itself.
	msg := ""
	if sel != "" {
		if err := mygo.GlobalShortcut.Register(sel, func() {
			// A hotkey is queued on its own goroutine: without the guard a
			// panic in one translation takes the tray process down.
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

// registerHotkeys (re)registers global shortcuts and returns a message
// describing what happened, empty when everything registered.
func registerHotkeys(sel, inp, ocr string, a *app) string {
	return registerHotkeysFunc(sel, inp, ocr, a)
}

// trayMenu builds the tray's menu from the current config. A radio entry's
// checked state is fixed when the menu is built, which is why the tray is
// rebuilt after the mode changes.
func (a *app) trayMenu() *mygo.Menu {
	cfg := a.store.Get()
	items := []*mygo.MenuItem{
		// 主窗口放在第一项：托盘右键是发现入口，也是主窗口关掉后唯一能把它叫回来的地方。
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
	// 原子写：OnBeforeQuit 回调与设置窗口 OnClose 也可能并发访问退出标志。
	a.isQuitting.Store(true)

	// Leave the desktop as we found it: stop any sound mid-playback and
	// take the low-level hooks down before the process goes away, so there
	// is no window where a hook fires into a dying process.
	stopAudio()
	if err := postHookCommand(removeHooks); err != nil {
		removeHooks() // hook thread never started: nothing to take down
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

// buildTray creates the tray icon with native Flyout quick panel support.
func (a *app) buildTray() {
	t, err := mygo.NewTray(mygo.TrayOptions{
		Icon:    trayIcon(),
		ToolTip: appTitle + " (左键打开主窗口 / 右键快捷菜单)",
	})
	if err != nil {
		log.Println("fastDictionary: tray:", err)
		return
	}
	// 左键直接打开主窗口：快捷面板也能从主窗口进去，而关掉主窗口后
	// 需要有一个地方能把它叫回来。
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

// rebuildTray replaces the tray menu so its radio state matches the config.
func (a *app) rebuildTray() {
	mygo.RunOnMain(func() {
		// A tray rebuild runs on the main thread: an unhandled panic here
		// takes the whole event loop down, not just this menu.
		defer recoverLog("main.go:rebuildTray")
		if a.tray == nil {
			a.buildTray()
			return
		}
		a.tray.SetMenu(a.trayMenu())
	})
}

// pauseSelectionItem is the tray's escape hatch for the selection trigger:
// the blocklist cannot know every application where a drag is not a
// selection, so the user can switch the trigger off without quitting.
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

// elevationMenuItem offers the only remedy for an elevated foreground
// window: restart this process at the same integrity level. Windows blocks
// hooks and UI Automation across that boundary, so no setting can fix it.
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
				// Give the elevated process time to load the config and
				// install its own hooks before this one lets go.
				time.Sleep(800 * time.Millisecond)
				mygo.RunOnMain(func() { a.quitApp() })
			}()
		},
	}
}

// selectionModeItem is one radio entry of the tray's 划词模式 submenu.
func selectionModeItem(a *app, mode, label string, cfg Config) *mygo.MenuItem {
	return &mygo.MenuItem{
		Label: label,
		Type:  mygo.MenuItemRadio,
		// The tray menu is rebuilt when the mode changes, so the checked
		// entry always reflects the stored mode.
		Checked: cfg.SelectionTrigger == mode,
		Click: func(*mygo.MenuItem, *mygo.Window) {
			a.store.Update(func(c *Config) { c.SelectionTrigger = mode })
			a.applySelectionMode()
			a.rebuildTray()
		},
	}
}

// setupLogging tees log output to a file next to the config.
//
// The file is appended to and rotated at logMaxBytes, so the lines written
// just before a crash survive the restart that follows it. The runtime's own
// fatal output (panics that no recover caught) goes to the same file through
// SetCrashOutput; without it a windowsgui process died without a trace.
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

// safeView wraps a view function so a panic while rendering leaves a
// readable placeholder instead of taking the whole process down. fastDictionary is
// a tray-resident tool: one bad snapshot crashing every window at once is
// worse than one window showing an error box.
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
	// Before the config store reads anything: a build renamed after the last
	// one has to find the settings the previous name wrote, not start over.
	// The message is held until setupLogging opens the file; a GUI build has
	// no stderr, and this is the one run whose record the user may need.
	migration := migrateUserData()

	a := &app{store: newStore()}
	cfg := a.store.Get()

	// One instance only. A second copy installs a second pair of low-level
	// hooks and runs a second "clear clipboard / Ctrl+C / restore" cycle on
	// every selection, so the two fight over the clipboard and the user sees
	// selections that occasionally do not arrive and a clipboard that is
	// sometimes empty. The second instance hands its command line to the
	// first one and exits.
	//
	// Logging is set up after this check on purpose: setupLogging truncates
	// the log, so a second launch would wipe the running instance's log on
	// its way out, exactly when the user is trying to find out why the
	// shortcut "did nothing".
	// A takeover launch is the elevated restart the tray menu asks for. The
	// new process cannot take the lock while this one is still shutting
	// down, and waiting for the old process to exit before UAC would lose
	// the app when the prompt is cancelled. The new process skips the lock;
	// the old one quits as soon as the new one started.
	if !elevatedTakeoverRequested() && !mygo.App.RequestSingleInstanceLock() {
		return
	}
	setupLogging()
	log.Printf("fastDictionary: build %s", buildStamp)
	if migration != "" {
		log.Print(migration)
	}
	mygo.App.OnSecondInstance(func(args []string, workingDir string) {
		// A second launch is the user asking for the app: bring the main
		// window forward instead of opening yet another popup.
		mygo.RunOnMain(func() {
			a.showMainWindow("home")
		})
	})
	// The audio path reads the playback volume through this handle.
	activeStore = a.store

	mygo.App.WhenReady(func() {
		mygo.App.OnBeforeQuit(func(e *mygo.QuitEvent) {
			a.isQuitting.Store(true)
		})

		win := mygo.NewWindow(mygo.WindowOptions{
			Title:  appTitle,
			Width:  int(cfg.PopupWidth),
			Height: 340,
			// 下限跟 popupMinHeight 一致：正文有自己的 Scroll，把窗口拖到
			// 只剩标题栏就什么都看不到了，也不留一条无法再拖开的缝。
			MinWidth:    360,
			MinHeight:   int(popupMinHeight),
			Hidden:      true,
			SkipTaskbar: true,
			AlwaysOnTop: cfg.AlwaysOnTop,
			Frameless:   true,
			Panel:       true,
			Opacity:     cfg.WindowOpacity, // 0 means unset: mygo treats it as fully opaque, which reads as the default
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

		// The selection chip's triggers. Hooks must be installed from the
		// main thread; the callbacks they fire arrive on goroutines of
		// their own, so the work they start never blocks the hook.
		OnSelectionGesture = a.onSelectionGesture
		OnMouseDown = a.onGlobalMouseDown
		OnMouseMove = a.onChipPointer
		OnKeyDown = a.onGlobalKeyDown
		if cfg.SelectionTrigger != "hotkey" {
			runOnMainThread()
		}

		a.buildTray()

		// Fetch the Edge TTS token now rather than on the first speaker
		// press: the handshake is ~650ms and the press is when the user is
		// waiting for sound.
		warmEdgeTTSAuth()

		// Learn which reasoning levels each model declares before the first
		// request, not when the settings page happens to open. 快速 asks for a
		// model's own lowest level, and without this list it has nothing to
		// ask for and falls back to sending no field at all — which is exactly
		// the slow path the option exists to avoid on models that reason by
		// default (measured: gemini 8.93s with no field against 2.58s on low).
		a.loadAIModels()

		log.Printf("fastDictionary: ready (selection=%s input=%s)", cfg.HotkeySelection, cfg.HotkeyInput)

		// Development flags: --settings opens the settings window, --input the
		// input popup, --shot <file> captures a window to PNG and exits.
		var shotPath, shotWindow, demoText, demoSelection, demoAction, demoOCR string
		// --shot-size WxH resizes the captured window first, which is how the
		// minimum-size layout and its scrolling are checked without a human
		// dragging the frame.
		shotSizeW, shotSizeH := 0, 0
		openSettingsFlag := false
		// minimizedFlag is set by --minimized and by --autostart: a login
		// start must not throw a window in the user's face.
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
				// Preview the spaced-repetition card without saving words
				// through the popup first.
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
				// Development aid: print how many events the mouse hook has
				// seen. "Installed but never delivered" and "delivered but
				// rejected by the policy" look identical from the outside;
				// this counter is the only way to tell them apart.
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
				// Show the selection chip at a fixed point for diagnosis,
				// with a sample word, and log what the window really is.
				chipDemo = true
			case "--chip-tip":
				// Raise the hover label at a fixed point, so its window can be
				// measured without a pointer: a label window that fails to
				// place itself is invisible to every headless test.
				chipDemo, chipTipDemo = true, true
			case "--ui-dark":
				// Visual previews use an in-memory theme only; they never change
				// the user's saved appearance.
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
				// Exercise the real crop preview, local OCR and editable result.
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
			a.mu.Lock()
			a.sel.text = "serendipity"
			a.mu.Unlock()
			go func() {
				defer recoverLog("main.go:main")
				sleepMS(600)
				a.showSelIcon(760, 520, "serendipity")
				sleepMS(700)
				a.logChipState()
				if chipTipDemo {
					// The second button, so the plate is centred on something that
					// is not the first or the last: both of those are clamped.
					a.raiseChipTip("朗读选中文字", ui.Rect{X: chipPadH + chipButtonSize + chipRowGap, Y: 4, W: chipButtonSize, H: chipButtonSize})
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
				a.source = "This is a slide from Terence Tao's 'Math 2.0' public lecture yesterday evening at Caltech. I am highly confident that, if the cure actually worked, almost any Stage 4 cancer patient would tell you that human comprehension of its mechanism is completely irrelevant to them."
				a.activeActionID = "default-sentence-analysis"
				a.grammarResult = &SentenceAnalysisResult{
					Annotations: []SyntaxAnnotation{
						{Text: "This", Type: "subject"},
						{Text: "is", Type: "predicate"},
						{Text: "I", Type: "subject"},
						{Text: "am", Type: "predicate"},
						{Text: "that, if the cure actually worked, almost any Stage 4 cancer patient would tell you that human comprehension of its mechanism is completely irrelevant to them", Type: "complement", Form: "clause"},
						{Text: "that", Type: "connector"},
						{Text: "if the cure actually worked", Type: "adverbial", Form: "clause", Sense: "condition"},
						{Text: "if", Type: "connector"},
						{Text: "the cure", Type: "subject"},
						{Text: "worked", Type: "predicate"},
						{Text: "almost any Stage 4 cancer patient", Type: "subject"},
						{Text: "would tell", Type: "predicate"},
						{Text: "you", Type: "object"},
						{Text: "that human comprehension of its mechanism is completely irrelevant to them", Type: "object", Form: "clause"},
						{Text: "human comprehension of its mechanism", Type: "subject"},
						{Text: "is", Type: "predicate"},
					},
					Translation: "这是陶哲轩昨晚在加州理工学院举办的“数学 2.0”公开讲座中的一张幻灯片。我十分确信，如果这种疗法真的有效，几乎任何一位四期癌症患者都会告诉你：人类是否理解其背后的机制，对他们而言根本无关紧要。",
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
		// 普通启动直接打开主窗口的「查词与翻译」工作台：首页是一张输入框加
		// 三个页签，不是过去那个 900px 的设置表单，打开也没有压迫感。自动启动
		// （--autostart）和明确要求最小化时仍静默进托盘，无头测试模式不弹窗。
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
			// --page 点名了某一页就用它，没指定时打开工作台。
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

	mygo.App.OnWindowAllClosed(func() {}) // stay in the tray
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
