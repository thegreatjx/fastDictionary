// fastpot — a fast desktop translator.
//
// Two global shortcuts: Ctrl+Alt+F reads the selection and shows the
// translation popup at the cursor; Ctrl+Alt+S opens the same popup in
// input mode. The popup is mygo native UI — no webview, so it appears at
// once. Engines: unauthenticated Microsoft translatetext racing Google
// gtx; a dictionary card with UK/US IPA and audio for single words; and
// an optional AI refinement streamed from the local magpie gateway.
package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// registerHotkeys (re)registers global shortcuts and returns a message
// describing what happened, empty when everything registered.
func registerHotkeys(sel, inp, ocr string, a *app) string {
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
		if err := mygo.GlobalShortcut.Register(sel, func() { go a.openPopup(modeSelection) }); err != nil {
			msg += "划词快捷键 " + sel + " 注册失败：" + err.Error() + "；"
		}
	}
	if inp != "" {
		if err := mygo.GlobalShortcut.Register(inp, func() { go a.openPopup(modeInput) }); err != nil {
			msg += "输入快捷键 " + inp + " 注册失败：" + err.Error() + "；"
		}
	}
	if ocr != "" {
		if err := mygo.GlobalShortcut.Register(ocr, func() { go a.TriggerScreenshotOCR() }); err != nil {
			msg += "截图快捷键 " + ocr + " 注册失败：" + err.Error() + "；"
		}
	}
	if msg == "" {
		log.Printf("fastpot: shortcuts ready (selection=%s input=%s ocr=%s)", sel, inp, ocr)
	} else {
		log.Printf("fastpot: shortcut problem: %s", msg)
	}
	return msg
}

// trayMenu builds the tray's menu from the current config. A radio entry's
// checked state is fixed when the menu is built, which is why the tray is
// rebuilt after the mode changes.
func (a *app) trayMenu() *mygo.Menu {
	cfg := a.store.Get()
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: "划词翻译 (" + cfg.HotkeySelection + ")", Click: func(*mygo.MenuItem, *mygo.Window) { a.openPopup(modeSelection) }},
		{Label: "输入翻译 (" + cfg.HotkeyInput + ")", Click: func(*mygo.MenuItem, *mygo.Window) { a.openPopup(modeInput) }},
		{Label: "截图翻译 (" + cfg.HotkeyScreenshot + ")", Click: func(*mygo.MenuItem, *mygo.Window) { a.TriggerScreenshotOCR() }},
		mygo.Separator(),
		{Label: "划词模式", Submenu: []*mygo.MenuItem{
			selectionModeItem(a, "icon", "显示小图标", cfg),
			selectionModeItem(a, "auto", "直接翻译", cfg),
			selectionModeItem(a, "hotkey", "仅快捷键", cfg),
		}},
		mygo.Separator(),
		pauseSelectionItem(a),
		mygo.Separator(),
		{Label: "设置", Click: func(*mygo.MenuItem, *mygo.Window) { a.showSettings() }},
		mygo.Separator(),
		{Label: "退出 fast词典", Click: func(*mygo.MenuItem, *mygo.Window) { a.quitApp() }},
	})
}

func (a *app) quitApp() {
	a.mu.Lock()
	a.isQuitting = true
	a.mu.Unlock()

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
		ToolTip: "fast词典 (左键快速面板 / 右键菜单)",
	})
	if err != nil {
		log.Println("fastpot: tray:", err)
		return
	}
	t.OnClick(a.toggleTrayFlyout)
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
// The shipped executable is built with -H=windowsgui, so it has no stderr:
// every diagnostic line the selection path writes (hook installs, gesture
// rejections, chip placement, AI failures) went nowhere, which is why a
// user reporting "the chip never appears" could not attach anything. The
// file is truncated on each start and capped, so it never grows.
func setupLogging() {
	dir, err := mygo.App.Path(mygo.PathLogs)
	if err != nil || dir == "" {
		if appData := os.Getenv("APPDATA"); appData != "" {
			dir = filepath.Join(appData, "fast词典")
		} else {
			return
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "fastpot.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return
	}
	log.SetOutput(io.MultiWriter(f))
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
}

// safeView wraps a view function so a panic while rendering leaves a
// readable placeholder instead of taking the whole process down. fastpot is
// a tray-resident tool: one bad snapshot crashing every window at once is
// worse than one window showing an error box.
func safeView(name string, view func(*ui.Context)) func(*ui.Context) {
	return func(c *ui.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("fastpot: panic rendering %s view: %v\n%s", name, r, debug.Stack())
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
	a := &app{store: newStore()}
	setupLogging()
	cfg := a.store.Get()

	// One instance only. A second copy installs a second pair of low-level
	// hooks and runs a second "clear clipboard / Ctrl+C / restore" cycle on
	// every selection, so the two fight over the clipboard and the user sees
	// selections that occasionally do not arrive and a clipboard that is
	// sometimes empty. The second instance hands its command line to the
	// first one and exits.
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	mygo.App.OnSecondInstance(func(args []string, workingDir string) {
		// A second launch is the user asking for the app: show the popup.
		go a.openPopup(modeInput)
	})
	// The audio path reads the playback volume through this handle.
	activeStore = a.store

	mygo.App.WhenReady(func() {
		mygo.App.OnBeforeQuit(func(e *mygo.QuitEvent) {
			a.isQuitting = true
		})

		win := mygo.NewWindow(mygo.WindowOptions{
			Title:       "fast词典",
			Width:       int(cfg.PopupWidth),
			Height:      340,
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
				win.Hide()
			}
		})

		if msg := registerHotkeys(cfg.HotkeySelection, cfg.HotkeyInput, cfg.HotkeyScreenshot, a); msg != "" {
			a.hotkeyMsg = msg
		}

		// The selection chip's triggers. Hooks must be installed from the
		// main thread; the callbacks they fire arrive on goroutines of
		// their own, so the work they start never blocks the hook.
		OnSelectionGesture = a.onSelectionGesture
		OnMouseDown = a.onGlobalMouseDown
		if cfg.SelectionTrigger != "hotkey" {
			runOnMainThread()
		}

		a.buildTray()

		// Fetch the Edge TTS token now rather than on the first speaker
		// press: the handshake is ~650ms and the press is when the user is
		// waiting for sound.
		warmEdgeTTSAuth()

		log.Printf("fastpot: ready (selection=%s input=%s)", cfg.HotkeySelection, cfg.HotkeyInput)

		// Development flags: --settings opens the settings window, --input the
		// input popup, --shot <file> captures a window to PNG and exits.
		var shotPath, shotWindow, demoText, demoSelection, demoAction string
		// --shot-size WxH resizes the captured window first, which is how the
		// minimum-size layout and its scrolling are checked without a human
		// dragging the frame.
		shotSizeW, shotSizeH := 0, 0
		openSettingsFlag := false
		chipDemo := false
		shotDelay := 1500 * time.Millisecond
		args := os.Args[1:]
		for i, arg := range args {
			switch arg {
			case "--settings", "-settings":
				openSettingsFlag = true
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
					last := uint64(0)
					for i := 0; i < 120; i++ {
						sleepMS(1000)
						e, d, u, g, own := hookStats()
						if e != last {
							log.Printf("fastpot: hook events=%d down=%d up=%d drag=%d skippedOwn=%d rejected=%q/%q",
								e, d, u, g, own, hookRejectedTitle, hookRejectedClass)
							last = e
						}
					}
				}()
			case "--chip":
				// Show the selection chip at a fixed point for diagnosis,
				// with a sample word, and log what the window really is.
				chipDemo = true
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
		if chipDemo {
			a.mu.Lock()
			a.sel.text = "serendipity"
			a.mu.Unlock()
			go func() {
				sleepMS(600)
				a.showSelIcon(760, 520, "serendipity")
				sleepMS(700)
				a.logChipState()
			}()
		}
		if demoText != "" {
			a.openPopup(modeInput)
			a.translate(demoText)
		}
		if demoSelection != "" {
			a.mu.Lock()
			a.mode = modeSelection
			a.mu.Unlock()
			a.openPopup(modeSelectionPreview)
			a.translate(demoSelection)
		}
		if demoAction != "" {
			a.openPopup(modeSelectionPreview)
			a.mu.Lock()
			a.mode = modeSelection
			switch demoAction {
			case "dict":
				a.mode = modeDictionary
				a.source = "blossom"
				a.target = "花朵（尤指果树的花）"
				a.activeActionID = "default-dictionary"
				a.dictResult = &DictionaryResult{
					Term:               "blossom",
					Phonetic:           "/ˈblɒs.əm/",
					PartOfSpeech:       "noun",
					Definition:         "花；花朵（尤指果树的花）",
					Context:            "The ephemeral beauty of cherry blossoms reminds us to cherish each moment.",
					ContextTerm:        "blossoms",
					ContextTranslation: "樱花短暂的美丽提醒我们珍惜每一刻。",
					MemoryTips:         "词源：和 bloom（开花）同源，bl- 开头的词多与花或植物生长有关\n近义词：bloom 多指花开的状态，blossom 常指果树的花",
					Difficulty:         "B2",
				}
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
		// A translator belongs in the tray. Opening the settings window on
		// every launch put a 900 px form in front of whatever the user was
		// reading before they had pressed a single shortcut. It still opens
		// on the first run (so the app is not invisible after installing)
		// and whenever it is asked for explicitly.
		firstRun := !cfg.Onboarded
		if firstRun {
			a.store.Update(func(c *Config) { c.Onboarded = true })
		}
		headless := shotPath != "" || chipDemo || demoText != "" || demoSelection != "" || demoAction != ""
		if openSettingsFlag || (firstRun && !headless) {
			a.showSettings()
		}

		if shotPath != "" {
			go func() {
				time.Sleep(shotDelay)
				target := win
				if shotWindow == "settings" && a.cfgWin != nil {
					target = a.cfgWin
				} else if shotWindow == "chip" && a.sel.win != nil {
					target = a.sel.win
				}
				if shotSizeW > 0 && shotSizeH > 0 {
					target.SetSize(shotSizeW, shotSizeH)
					time.Sleep(400 * time.Millisecond)
				}
				png, err := target.CapturePage()
				if err != nil {
					log.Printf("fastpot: capture failed: %v", err)
				} else if err := os.WriteFile(shotPath, png, 0o644); err != nil {
					log.Printf("fastpot: capture write failed: %v", err)
				} else {
					log.Printf("fastpot: captured %d bytes to %s", len(png), shotPath)
				}
				time.Sleep(200 * time.Millisecond)
				mygo.RunOnMain(func() {
					mygo.App.Quit()
				})
				go func() {
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
