package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

var langOptions = []string{"zh-Hans", "zh-Hant", "en", "ja", "ko", "fr", "de", "es", "ru"}
var langLabels = map[string]string{
	"zh-Hans": "简体中文", "zh-Hant": "繁体中文", "en": "英语", "ja": "日语",
	"ko": "韩语", "fr": "法语", "de": "德语", "es": "西班牙语", "ru": "俄语",
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
			ui.Text(c, "fast词典").FontSize(14).FontWeight(600).Padding(6, 8, 6, 8).TextColor(t.Accent)
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

func heading(c *ui.Context, t *ui.Theme, title, sub string) {
	ui.Text(c, title).FontSize(19).FontWeight(600).TextColor(t.Text)
	if sub != "" {
		ui.Text(c, sub).FontSize(12).TextColor(t.TextMuted).Wrap()
	}
}

func (a *app) pageGeneral(c *ui.Context, t *ui.Theme, cfg Config) {
	heading(c, t, "通用", "外观、透明度与窗口行为。改动即时保存。")
	ui.Form(c, func() {
		ui.Field(c, "外观主题", func() {
			idx := 0
			switch cfg.ThemeMode {
			case "light":
				idx = 1
			case "dark":
				idx = 2
			}
			ui.Segmented(c, &idx, "跟随系统", "浅色", "深色").MaxWidth(340).OnChange(func() {
				mode := []string{"system", "light", "dark"}[idx]
				a.store.Update(func(c *Config) { c.ThemeMode = mode })
			})
		})
		ui.Field(c, "窗口透明度", func() {
			op := cfg.WindowOpacity
			if op <= 0 {
				op = 0.95
			}
			ui.Slider(c, &op, 0.6, 1.0).Width(240).Step(0.05).OnChange(func() {
				a.store.Update(func(c *Config) { c.WindowOpacity = op })
				if a.win != nil {
					a.win.SetOpacity(op)
				}
			})
			ui.Textf(c, "%.0f%%", op*100).FontSize(12).TextColor(t.TextMuted)
		}).Description("调节划词弹窗透明度（60% - 100%）")
		ui.Field(c, "失焦自动关闭", func() {
			v := cfg.CloseOnBlur
			ui.Switch(c, &v).OnChange(func() {
				a.store.Update(func(c *Config) { c.CloseOnBlur = v })
			})
		}).Description("划词弹窗失去焦点时自动隐藏")
		ui.Field(c, "弹窗置顶", func() {
			v := cfg.AlwaysOnTop
			ui.Switch(c, &v).OnChange(func() {
				a.store.Update(func(c *Config) { c.AlwaysOnTop = v })
				if a.win != nil {
					a.win.SetAlwaysOnTop(v)
				}
			})
		})
		ui.Field(c, "弹窗宽度", func() {
			w := float64(cfg.PopupWidth)
			ui.Slider(c, &w, 360, 680).Width(240).Step(20).OnChange(func() {
				a.store.Update(func(cfg *Config) { cfg.PopupWidth = float32(w) })
			})
			ui.Textf(c, "%.0f px", w).FontSize(12).TextColor(t.TextMuted)
		}).Description("像素宽度，超长内容纵向平滑滚动")
	})
}

// pageSelection configures how a text selection triggers fastpot.
func (a *app) pageSelection(c *ui.Context, t *ui.Theme, cfg Config) {
	heading(c, t, "划词", "选中文字之后发生什么。")
	ui.Form(c, func() {
		ui.Field(c, "选中后", func() {
			idx := 0
			switch cfg.SelectionTrigger {
			case "auto":
				idx = 1
			case "hotkey":
				idx = 2
			}
			ui.Segmented(c, &idx, "显示图标", "直接翻译", "仅快捷键").MaxWidth(340).OnChange(func() {
				mode := []string{"icon", "auto", "hotkey"}[idx]
				a.store.Update(func(c *Config) { c.SelectionTrigger = mode })
				a.applySelectionMode()
			})
		}).Description("显示图标：选完在鼠标旁浮出一个小图标；直接翻译：选完立刻弹窗；仅快捷键：只响应全局快捷键")

		ui.Field(c, "取词方式", func() {
			idx := 0
			switch cfg.SelectionCapture {
			case "copy":
				idx = 1
			case "uia":
				idx = 2
			}
			ui.Segmented(c, &idx, "自动", "模拟复制", "仅 UIA").MaxWidth(340).OnChange(func() {
				mode := []string{"auto", "copy", "uia"}[idx]
				a.store.Update(func(c *Config) { c.SelectionCapture = mode })
			})
		}).Description("自动：优先 UI Automation，读不到再模拟 Ctrl+C（推荐）；模拟复制：始终模拟 Ctrl+C，兼容性最好；仅 UIA：绝不模拟按键，最安静但终端类程序取不到词")

		ui.Field(c, "这些程序里不划词", func() {
			// One card holds the list and the add row, so a remove button
			// sits next to its own name instead of being pushed to the far
			// edge of the form.
			// FillWidth, not a fixed 420: the form's label column already
			// takes space, so a fixed width overflowed the window as soon
			// as it was narrowed.
			ui.Box(c).FillWidth().Padding(10, 10).Radius(6).
				Background(t.Surface).Border(1, t.Border).Children(func() {
				ui.Column(c).FillWidth().Gap(6).Children(func() {
					for _, name := range cfg.SelectionBlocklist {
						n := name
						ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
							ui.Text(c, n).FontSize(12).TextColor(t.Text).Grow(1)
							btnDel := ui.Button(c, "移除").Height(22).Padding(1, 8).Radius(4).
								TextColor(t.TextMuted)
							if btnDel.Clicked() {
								a.store.Update(func(c *Config) {
									out := make([]string, 0, len(c.SelectionBlocklist))
									for _, item := range c.SelectionBlocklist {
										if !strings.EqualFold(item, n) {
											out = append(out, item)
										}
									}
									c.SelectionBlocklist = out
								})
								a.settingsRefresh()
							}
						})
					}
					ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
						ui.TextInput(c, &a.newBlockedApp).Placeholder("程序名，例如 pixpin.exe").Grow(1)
						btnAdd := ui.Button(c, "添加").Height(26).Padding(2, 10).Radius(5)
						if btnAdd.Clicked() {
							name := strings.ToLower(strings.TrimSpace(a.newBlockedApp))
							if name != "" {
								a.store.Update(func(c *Config) {
									for _, item := range c.SelectionBlocklist {
										if strings.EqualFold(item, name) {
											return
										}
									}
									c.SelectionBlocklist = append(c.SelectionBlocklist, name)
								})
								a.newBlockedApp = ""
								a.settingsRefresh()
							}
						}
					})
				})
			})
		}).Description("在这些程序里拖动鼠标不会被当成划词。截图工具必须在这里，否则框选区域会被误判成划词。")

		ui.Field(c, "关于剪贴板", func() {
			ui.Text(c, "模拟复制会先备份剪贴板再还原，图片与富文本一并保留。若某个程序取不到词，把「取词方式」改成「模拟复制」再试。").FontSize(12).TextColor(t.TextMuted).Wrap()
		})
	})
}

func (a *app) pageTranslate(c *ui.Context, t *ui.Theme, cfg Config) {
	heading(c, t, "翻译", "免密钥的微软与谷歌接口，竞速取先到者。")
	ui.Form(c, func() {
		ui.Field(c, "引擎", func() {
			idx := 0
			switch cfg.EngineMode {
			case "microsoft":
				idx = 1
			case "google":
				idx = 2
			}
			ui.Segmented(c, &idx, "竞速", "仅微软", "仅谷歌").MaxWidth(340).OnChange(func() {
				mode := []string{"race", "microsoft", "google"}[idx]
				a.store.Update(func(c *Config) { c.EngineMode = mode })
			})
		}).Description("竞速：两个引擎并发，先回先用，慢的自动取消")
		ui.Field(c, "目标语言", func() {
			label := langLabels[cfg.TargetLang]
			ui.Select(c, &label, langList()).OnChange(func() {
				code := langFromLabel(label)
				a.store.Update(func(c *Config) { c.TargetLang = code })
			})
		})
		ui.Field(c, "源语言相同时译为", func() {
			label := langLabels[cfg.SecondLang]
			ui.Select(c, &label, langList()).OnChange(func() {
				code := langFromLabel(label)
				a.store.Update(func(c *Config) { c.SecondLang = code })
			})
		}).Description("例如中文内容翻译成英文时使用")
		ui.Field(c, "显示引擎与语种", func() {
			v := cfg.ShowEngineTag
			ui.Switch(c, &v).OnChange(func() {
				a.store.Update(func(c *Config) { c.ShowEngineTag = v })
			})
		})
	})
}

func (a *app) pageDict(c *ui.Context, t *ui.Theme, cfg Config) {
	heading(c, t, "词典与发音", "单个单词自动显示音标与释义，点音标即可朗读。")
	ui.Form(c, func() {
		ui.Field(c, "单词词典卡", func() {
			v := cfg.DictEnabled
			ui.Switch(c, &v).OnChange(func() {
				a.store.Update(func(c *Config) { c.DictEnabled = v })
			})
		}).Description("有道音标 + 谷歌词性释义，实测约 100ms")
		ui.Field(c, "自动朗读", func() {
			idx := 0
			switch cfg.AutoPronounce {
			case "uk":
				idx = 1
			case "us":
				idx = 2
			}
			ui.Segmented(c, &idx, "关闭", "英音", "美音").MaxWidth(340).OnChange(func() {
				v := []string{"off", "uk", "us"}[idx]
				a.store.Update(func(c *Config) { c.AutoPronounce = v })
			})
		}).Description("查询单词后自动发音")
		ui.Field(c, "音量", func() {
			v := float64(cfg.AudioVolume)
			if v <= 0 {
				v = 80
			}
			ui.Slider(c, &v, 0, 100).Width(240).Step(5).OnChange(func() {
				a.store.Update(func(c *Config) { c.AudioVolume = int(v) })
			})
			ui.Textf(c, "%.0f%%", v).FontSize(12).TextColor(t.TextMuted)
		}).Description("发音音量，改动后下一次朗读生效")
	})
}

func (a *app) pageAI(c *ui.Context, t *ui.Theme, cfg Config) {
	heading(c, t, "AI 精翻", "长文与需要斟酌的句子交给本地网关，流式返回。")
	ui.Form(c, func() {
		ui.Field(c, "启用", func() {
			v := cfg.AIEnabled
			ui.Switch(c, &v).OnChange(func() {
				a.store.Update(func(c *Config) { c.AIEnabled = v })
			})
		})
		ui.Field(c, "网关地址", func() {
			v := cfg.AIURL
			ui.TextInput(c, &v).OnChange(func() {
				a.store.Update(func(c *Config) { c.AIURL = v })
			})
		}).Description("留空使用 magpie 默认地址")
		ui.Field(c, "默认模式", func() {
			idx := 0
			switch cfg.AIMode {
			case "grammar":
				idx = 0
			case "dict":
				idx = 1
			case "translate":
				idx = 2
			}
			ui.Segmented(c, &idx, "长难句分析", "词典精读", "润色翻译").MaxWidth(340).OnChange(func() {
				modes := []string{"grammar", "dict", "translate"}
				a.store.Update(func(c *Config) { c.AIMode = modes[idx] })
			})
		}).Description("长难句分析（语法树拆解）/ 词典精读（语境助记）/ 润色翻译（地道重构）")
		ui.Field(c, "模型", func() {
			currentModel := cfg.AIModel
			models := a.getAvailableModels()
			// FillWidth instead of a fixed 320: with the label column, a fixed
			// width pushed the refresh button past the right edge of a
			// narrow window.
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Select(c, &currentModel, models).Grow(1).OnChange(func() {
					a.store.Update(func(c *Config) { c.AIModel = currentModel })
				})
				if ui.Button(c, "🔄 刷新").Height(26).Padding(2, 8).Radius(4).Tooltip("从 Magpie 网关实时刷新完整模型列表").Clicked() {
					a.loadMagpieModels()
				}
			})
		}).Description(fmt.Sprintf("从本地 magpie 网关动态拉取（当前已加载 %d 个模型）", len(a.getAvailableModels())))
		ui.Field(c, "自定义模型ID", func() {
			v := cfg.AIModel
			ui.TextInput(c, &v).FillWidth().OnChange(func() {
				a.store.Update(func(c *Config) { c.AIModel = v })
			})
		}).Description("也可以直接手动粘贴 magpie 内任意自定义模型 ID")
		ui.Field(c, "推理强度", func() {
			idx := 0
			efforts := []string{"none", "low", "high"}
			for i, e := range efforts {
				if e == cfg.AIEffort {
					idx = i
				}
			}
			ui.Segmented(c, &idx, "免思考 (极速直出 · 推荐)", "轻度思考 (Low)", "深度思考 (High)").MaxWidth(340).OnChange(func() {
				v := efforts[idx]
				a.store.Update(func(c *Config) { c.AIEffort = v })
			})
		}).Description("免思考：0 reasoning tokens，极速秒级直出；轻度/深度：开启思维链深度推理")
		ui.Field(c, "连通性", func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				if ui.Button(c, "测试网关").Width(140).Clicked() {
					a.testGateway()
				}
				if a.aiTest != "" {
					ui.Text(c, a.aiTest).FontSize(12).TextColor(t.TextMuted)
				}
			})
		}).Description("测试本地 magpie 大模型网关连通性；日常划词查词走微软/谷歌双路直连通道（~200ms）")
	})
}

func (a *app) pageHotkey(c *ui.Context, t *ui.Theme, cfg Config) {
	heading(c, t, "快捷键", "全局生效，任何程序中均可秒级唤起。")
	a.initHotkeyDrafts()

	ui.Form(c, func() {
		ui.Field(c, "推荐预设方案", func() {
			ui.Column(c).Gap(6).Children(func() {
				btnP1 := ui.Button(c, "⚡ 极速单手：Alt+D (划词) / Alt+C (截图) / Alt+Q (输入)").Height(28).Padding(2, 10).Radius(5).
					Background(ui.Hex("#EFF6FF")).TextColor(ui.Hex("#1D4ED8")).
					Tooltip("【最推荐】左手单手盲操极速触发。注意：Alt+D 在 Chrome/Edge 中会聚焦地址栏，浏览器内建议用方案二")
				if btnP1.Clicked() {
					a.pendingHotkeySelection = "Alt+D"
					a.pendingHotkeyScreenshot = "Alt+C"
					a.pendingHotkeyInput = "Alt+Q"
					a.applyHotkeys()
				}

				btnP2 := ui.Button(c, "🛡️ 稳健三键：Ctrl+Alt+D (划词) / Ctrl+Alt+S (截图) / Ctrl+Alt+I (输入)").Height(28).Padding(2, 10).Radius(5).
					Background(t.SurfaceHover).
					Tooltip("标准三键组合，防误触")
				if btnP2.Clicked() {
					a.pendingHotkeySelection = "Ctrl+Alt+D"
					a.pendingHotkeyScreenshot = "Ctrl+Alt+S"
					a.pendingHotkeyInput = "Ctrl+Alt+I"
					a.applyHotkeys()
				}
			})
		}).Description("点击即可一键应用，无键位冲突且符合单手人体工学习惯")

		ui.Field(c, "划词翻译快捷键", func() {
			ui.TextInput(c, &a.pendingHotkeySelection).Width(240)
		}).Description("选中文本后按下触发（推荐 Alt+D 或 Ctrl+Alt+D）")

		ui.Field(c, "截图翻译快捷键", func() {
			ui.TextInput(c, &a.pendingHotkeyScreenshot).Width(240)
		}).Description("唤起屏幕框选并自动离线 OCR 翻译（推荐 Alt+C 或 Ctrl+Alt+C，0热键冲突）")

		ui.Field(c, "输入翻译快捷键", func() {
			ui.TextInput(c, &a.pendingHotkeyInput).Width(240)
		}).Description("弹出独立输入翻译框（推荐 Alt+Q 或 Ctrl+Alt+I）")

		ui.Field(c, "生效保存", func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				if ui.PrimaryButton(c, "应用修改").Width(120).Height(30).Clicked() {
					a.applyHotkeys()
				}
				if a.hotkeyMsg != "" {
					ui.Text(c, a.hotkeyMsg).FontSize(12).TextColor(t.Accent).Wrap()
				}
			})
		})

		ui.Field(c, "格式说明", func() {
			ui.Text(c, "支持输入任意自定义组合键（如 Alt+D、Ctrl+Shift+D 等），点击【应用修改】立即注册生效。").FontSize(12).TextColor(t.TextMuted).Wrap()
		})
	})
}

func (a *app) pageAbout(c *ui.Context, t *ui.Theme, cfg Config) {
	heading(c, t, "关于", "fast词典 1.0.0")
	ui.Text(c, "轻巧极速的桌面划词翻译与词典工具：原生 UI 渲染、零卡顿、低内存常驻。").FontSize(13).TextColor(t.Text).Wrap()
	ui.Divider(c).Fill()
	rows := [][2]string{
		{"翻译引擎", "微软 translatetext（免鉴权）⇄ 谷歌 gtx，竞速取先到"},
		{"词典与音标", "有道 jsonapi（英/美 IPA）"},
		{"发音", "有道 dictvoice，mp3 缓存 + 系统 MCI 播放"},
		{"OCR 引擎", "Windows.Media.Ocr 原生离线神经文字识别"},
		{"AI 精翻", "magpie 网关，闪速模型 + 低推理档"},
		{"外壳", "mygo v0.4.6 原生 UI（Go，无 cgo）"},
	}
	for _, r := range rows {
		ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Start).Children(func() {
			ui.Text(c, r[0]).FontSize(12).TextColor(t.TextMuted).Width(80)
			ui.Text(c, r[1]).FontSize(12).TextColor(t.Text).Grow(1).Wrap()
		})
	}
	ui.Divider(c).Fill()
	ui.Text(c, "配置文件：用户数据目录下的 config.json").FontSize(12).TextColor(t.TextMuted)
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
		out, err := StreamMagpie(ctx, cfg.AIURL, cfg.AIModel, "You are a test probe. Reply with 'pong' only.", "ping", cfg.AIEffort, nil)
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
		log.Printf("fastpot: gateway test: %s", msg)
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
		log.Printf("fastpot: shortcut update rejected: %s", msg)
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
		"group/auto-deepseek-v4-1-flash",
		"antigravity/gemini-3.8-flash",
		"workbuddy/deepseek-v4.1-flash",
		"workbuddy/glm-5.3-flash",
		"workbuddy/kimi-k3-1",
		"minimax-code/MiniMax-M2.7-highspeed",
		"codex/gpt-5.5",
	}
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
		Title:     "fast词典 设置",
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

func (a *app) pageCustomActions(c *ui.Context, t *ui.Theme, cfg Config) {
	if a.editingActionID == "" {
		// List View
		heading(c, t, "AI 指令", "管理与定制划词悬浮窗中的独立 AI 指令。支持独立开关、模型配置与提示词深度定制。")

		ui.Column(c).FillWidth().Gap(10).Children(func() {
			for _, action := range cfg.CustomActions {
				act := action
				ui.Box(c).FillWidth().Padding(14, 16).Radius(8).
					Background(t.Surface).Border(1, t.Border).
					Shadow(0, 1, 4, 0, ui.RGBA(0, 0, 0, 0.04)).Children(func() {
					ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(14).Children(func() {
						// Action Icon
						var actIcon *ui.SVG
						switch act.ID {
						case "default-dictionary":
							actIcon = iconDict
						case "default-sentence-analysis":
							actIcon = iconGrammar
						case "default-improve-writing":
							actIcon = iconPolish
						default:
							actIcon = iconSparkles
						}
						ui.Box(c).Padding(8, 8).Radius(6).Background(t.SurfaceHover).Children(func() {
							ui.Icon(c, actIcon).Size(22, 22).TextColor(t.Accent)
						})

						// Title, Model badge & description
						ui.Column(c).Grow(1).Gap(3).Children(func() {
							ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
								ui.Text(c, act.Name).FontSize(15).FontWeight(600).TextColor(t.Text)
								if act.Model != "" {
									ui.Box(c).Padding(1, 6).Radius(10).Background(ui.Hex("#EFF6FF")).Border(1, ui.Hex("#BFDBFE")).Children(func() {
										ui.Text(c, act.Model).FontSize(11).FontWeight(600).TextColor(ui.Hex("#1D4ED8"))
									})
								} else {
									ui.Box(c).Padding(1, 6).Radius(10).Background(t.SurfaceHover).Children(func() {
										ui.Text(c, "默认模型").FontSize(11).TextColor(t.TextMuted)
									})
								}
							})

							desc := "自定义 AI 指令"
							switch act.ID {
							case "default-dictionary":
								desc = "面向语言学习者的精细词典：音标、词性、原句高亮与记忆技巧"
							case "default-sentence-analysis":
								desc = "长难句语法树拆解：主干、从句微标、连词与波浪线谓语标注"
							case "default-improve-writing":
								desc = "写作评改与润色：场景得体性、逐项错误修改建议与一键复制"
							}
							ui.Text(c, desc).FontSize(12).TextColor(t.TextMuted).Wrap()
						})

						// [定制化] button
						btnCustom := ui.Button(c, "定制化").Height(28).Padding(2, 14).Radius(5)
						if btnCustom.Clicked() {
							a.editingActionID = act.ID
							a.customizingAction = act
							a.actionSaveFeedback = ""
						}

						// Toggle Switch
						enabled := act.Enabled
						ui.Switch(c, &enabled).OnChange(func() {
							a.store.Update(func(c *Config) {
								for i := range c.CustomActions {
									if c.CustomActions[i].ID == act.ID {
										c.CustomActions[i].Enabled = enabled
									}
								}
							})
							if a.cfgWin != nil {
								a.cfgWin.Update(func() {})
							}
						})
					})
				})
			}
		})
	} else {
		// Customization View: Fully responsive cards with multi-line TextArea
		act := a.customizingAction
		heading(c, t, "定制化指令："+act.Name, "自由配置此指令调用的独立模型、系统提示词与输入模板。")

		// Top nav bar: [← 返回指令列表]  [恢复默认提示词]
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			btnBack := ui.Button(c, "← 返回指令列表").Height(28).Padding(2, 12).Radius(5)
			if btnBack.Clicked() {
				a.editingActionID = ""
			}

			ui.Spacer(c).Grow(1)

			if strings.HasPrefix(a.editingActionID, "default-") {
				btnReset := ui.Button(c, "恢复默认提示词").Height(28).Padding(2, 12).Radius(5)
				if btnReset.Clicked() {
					a.store.Update(func(c *Config) {
						c.ResetAction(a.editingActionID)
					})
					if def := cfg.GetAction(a.editingActionID); def != nil {
						a.customizingAction = *def
					}
					a.actionSaveFeedback = "已恢复为默认提示词设置"
				}
			}
		})

		ui.Column(c).FillWidth().Gap(14).Children(func() {
			// 1. Basic configuration card (名称与模型)
			ui.Box(c).FillWidth().Padding(14, 16).Radius(8).
				Background(t.Surface).Border(1, t.Border).Children(func() {
				ui.Column(c).FillWidth().Gap(12).Children(func() {
					// 指令名称
					ui.Column(c).FillWidth().Gap(4).Children(func() {
						ui.Text(c, "指令名称").FontSize(13).FontWeight(600).TextColor(t.Text)
						ui.TextInput(c, &a.customizingAction.Name).FillWidth()
					})

					// 模型选择
					ui.Column(c).FillWidth().Gap(4).Children(func() {
						ui.Text(c, "调用模型").FontSize(13).FontWeight(600).TextColor(t.Text)
						models := []string{
							"跟随全局 (默认)",
							"group/auto-deepseek-v4-1-flash",
							"workbuddy/deepseek-v4.1-flash",
							"antigravity/gemini-3.8-flash",
							"codex/gpt-5.5",
							"workbuddy/glm-5.3-flash",
							"workbuddy/kimi-k3-1",
							"qoder-plugin/qfmodel",
							"minimax-code/MiniMax-M2.7-highspeed",
						}
						curModel := a.customizingAction.Model
						if curModel == "" {
							curModel = "跟随全局 (默认)"
						}
						ui.Select(c, &curModel, models).FillWidth().OnChange(func() {
							if curModel == "跟随全局 (默认)" {
								a.customizingAction.Model = ""
							} else {
								a.customizingAction.Model = curModel
							}
						})
						ui.Text(c, "从本地 magpie 网关支持的模型列表中选择；也可在下方手动输入任意自定义模型 ID。").FontSize(11).TextColor(t.TextMuted)
					})

					// 自定义模型 ID
					ui.Column(c).FillWidth().Gap(4).Children(func() {
						ui.Text(c, "自定义模型 ID (可选)").FontSize(13).FontWeight(600).TextColor(t.Text)
						ui.TextInput(c, &a.customizingAction.Model).FillWidth().Placeholder("留空则跟随全局默认模型")
					})
				})
			})

			// 2. System Prompt Card (多行 TextArea，不再挤在单行)
			ui.Box(c).FillWidth().Padding(14, 16).Radius(8).
				Background(t.Surface).Border(1, t.Border).Children(func() {
				ui.Column(c).FillWidth().Gap(6).Children(func() {
					ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
						ui.Text(c, "系统提示词 (System Prompt)").FontSize(13).FontWeight(600).TextColor(t.Text)
						ui.Spacer(c).Grow(1)
						ui.Text(c, "指导 AI 的角色设定、专业规则与输出规范约定").FontSize(11).TextColor(t.TextMuted)
					})
					ui.TextArea(c, &a.customizingAction.SystemPrompt).Height(160).FillWidth()
				})
			})

			// 3. User Prompt Card (带占位符插入按钮与多行输入)
			ui.Box(c).FillWidth().Padding(14, 16).Radius(8).
				Background(t.Surface).Border(1, t.Border).Children(func() {
				ui.Column(c).FillWidth().Gap(6).Children(func() {
					ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
						ui.Text(c, "用户提示词 (User Prompt)").FontSize(13).FontWeight(600).TextColor(t.Text)
						ui.Spacer(c).Grow(1)
						ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
							ui.Text(c, "快捷插入变量：").FontSize(11).TextColor(t.TextMuted)
							btn1 := ui.Button(c, "{{selection}}").Height(22).Padding(1, 6).Radius(4)
							if btn1.Clicked() {
								a.customizingAction.Prompt += "{{selection}}"
							}
							btn2 := ui.Button(c, "{{paragraphs}}").Height(22).Padding(1, 6).Radius(4)
							if btn2.Clicked() {
								a.customizingAction.Prompt += "{{paragraphs}}"
							}
							btn3 := ui.Button(c, "{{targetLanguage}}").Height(22).Padding(1, 6).Radius(4)
							if btn3.Clicked() {
								a.customizingAction.Prompt += "{{targetLanguage}}"
							}
						})
					})
					ui.TextArea(c, &a.customizingAction.Prompt).Height(90).FillWidth()
					ui.Text(c, "用户请求模板，支持 {{selection}} (选中文本)、{{paragraphs}} (周围段落)、{{targetLanguage}} (目标语言) 占位符。").FontSize(11).TextColor(t.TextMuted)
				})
			})

			// 4. Output Schema Card
			ui.Box(c).FillWidth().Padding(10, 14).Radius(8).
				Background(t.SurfaceHover).Children(func() {
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
					ui.Text(c, "输出字段约定 (Schema)：").FontSize(12).FontWeight(600).TextColor(t.Text)
					ui.Text(c, strings.Join(a.customizingAction.OutputSchema, " · ")).FontSize(12).TextColor(t.Accent).Grow(1).Wrap()
				})
			})

			// 5. Actions row
			ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Padding(4, 0).Children(func() {
				if ui.PrimaryButton(c, "保存修改").Width(140).Height(32).Clicked() {
					a.store.Update(func(c *Config) {
						for i := range c.CustomActions {
							if c.CustomActions[i].ID == a.editingActionID {
								c.CustomActions[i] = a.customizingAction
							}
						}
					})
					a.actionSaveFeedback = "已成功保存指令「" + a.customizingAction.Name + "」的定制修改"
					a.editingActionID = ""
				}
				if ui.Button(c, "取消").Width(90).Height(32).Clicked() {
					a.editingActionID = ""
				}
			})
		})
	}
}

func (a *app) pageVocabulary(c *ui.Context, t *ui.Theme, cfg Config) {
	vocabStore := GetVocabularyStore()
	heading(c, t, "生词本与长难句笔记", "本地持久化存储（%AppData%\\Dictionary\\vocabulary.json）。支持分类归档、格式导出导入与智能补全。")

	// Filter & Action Bar
	ui.Box(c).FillWidth().Padding(10, 12).Radius(8).Background(t.Surface).Border(1, t.Border).Children(func() {
		ui.Column(c).FillWidth().Gap(8).Children(func() {
			// Row 1: Full-width Category Tabs — one pass over the store,
			// not four counters each scanning every entry per frame.
			cAll, cWords, cSentences, cWriting := vocabStore.CategoryCounts()
			catNames := []string{
				fmt.Sprintf("全部 (%d)", cAll),
				fmt.Sprintf("📘 生词 (%d)", cWords),
				fmt.Sprintf("🔍 长难句 (%d)", cSentences),
				fmt.Sprintf("✏️ 写作 (%d)", cWriting),
			}
			ui.Row(c).FillWidth().Children(func() {
				ui.Segmented(c, &a.vocabCategoryIdx, catNames...).FillWidth().Height(28).FontSize(12)
			})

			// Row 2: search on its own line. Sharing the row with six
			// buttons overflowed the form as soon as the window was narrow.
			ui.SearchField(c, &a.vocabSearchQuery).FillWidth()

			// Row 3: the actions, which wrap freely now.
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
				// [📥 导入]
				btnImport := ui.Button(c, "📥 导入").Height(28).Padding(2, 8).Radius(5).
					Tooltip("从外部 JSON/CSV/TXT 导入生词")
				if btnImport.Clicked() {
					go func() {
						paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
							Title:   "导入生词文件",
							Filters: []mygo.FileFilter{{Name: "生词文件 (*.json, *.csv, *.txt)", Extensions: []string{"json", "csv", "txt"}}},
						})
						if err == nil && len(paths) > 0 {
							n, err := vocabStore.ImportFile(paths[0])
							a.mu.Lock()
							if err == nil {
								a.actionSavedMsg = fmt.Sprintf("成功导入 %d 条生词 ✓", n)
							} else {
								a.actionSavedMsg = "导入失败: " + err.Error()
							}
							a.mu.Unlock()
							if a.cfgWin != nil {
								a.cfgWin.Update(func() {})
							}
						}
					}()
				}

				// [📊 导出]
				btnExpCSV := ui.Button(c, "📊 导出").Height(28).Padding(2, 8).Radius(5).
					Tooltip("导出生词库为 CSV 表格（支持 Excel / Anki 直接打开）")
				if btnExpCSV.Clicked() {
					go func() {
						path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
							Title:       "导出生词库 CSV",
							DefaultPath: "fast词典_生词库.csv",
							Filters:     []mygo.FileFilter{{Name: "CSV 表格", Extensions: []string{"csv"}}},
						})
						if err == nil && path != "" {
							if err := vocabStore.ExportCSV(path); err == nil {
								a.mu.Lock()
								a.actionSavedMsg = "已成功导出 CSV 表格 ✓"
								a.mu.Unlock()
								if a.cfgWin != nil {
									a.cfgWin.Update(func() {})
								}
							}
						}
					}()
				}

				// [✨ 补全]
				btnEnrich := ui.Button(c, "✨ 补全").Height(28).Padding(2, 8).Radius(5).
					Tooltip("为生词自动补全音标、CEFR 难度、例句与记忆技巧")
				if btnEnrich.Clicked() {
					go func() {
						n := vocabStore.EnrichAll()
						a.mu.Lock()
						a.actionSavedMsg = fmt.Sprintf("已智能补全 %d 条生词数据 ✓", n)
						a.mu.Unlock()
						if a.cfgWin != nil {
							a.cfgWin.Update(func() {})
						}
					}()
				}

				// [🌐 网页版]
				btnWeb := ui.Button(c, "🌐 网页版").Height(28).Padding(2, 8).Radius(5).
					Tooltip("在默认浏览器中全屏查阅生词本")
				if btnWeb.Clicked() {
					if path, err := vocabStore.ExportHTML(); err == nil {
						go mygo.Shell.OpenExternal(path)
					}
				}

				// 清空 is destructive and irreversible, so it asks once.
				// It used to sit next to 导入/导出 with the same weight and
				// wiped the whole notebook on a single click.
				if vocabStore.Count() > 0 {
					if a.confirmClearVocab {
						btnYes := ui.Button(c, "确认清空").Height(28).Padding(2, 8).Radius(5).
							Background(t.Danger).TextColor(ui.Hex("#FFFFFF")).Tooltip("删除全部生词，无法恢复")
						if btnYes.Clicked() {
							if err := vocabStore.Clear(); err != nil {
								a.actionSavedMsg = "清空失败: " + err.Error()
							} else {
								a.actionSavedMsg = "生词本已清空"
							}
							a.confirmClearVocab = false
							a.settingsRefresh()
						}
						btnNo := ui.Button(c, "取消").Height(28).Padding(2, 8).Radius(5)
						if btnNo.Clicked() {
							a.confirmClearVocab = false
							a.settingsRefresh()
						}
					} else {
						btnClear := ui.Button(c, "清空").Height(28).Padding(2, 8).Radius(5).
							TextColor(t.Danger).Tooltip("清空当前所有笔记（会先确认）")
						if btnClear.Clicked() {
							a.confirmClearVocab = true
							a.settingsRefresh()
						}
					}
				}
			})
		})
	})

	catKey := "all"
	switch a.vocabCategoryIdx {
	case 1:
		catKey = "words"
	case 2:
		catKey = "sentences"
	case 3:
		catKey = "writing"
	}

	entries := vocabStore.Filter(catKey, a.vocabSearchQuery)
	if len(entries) == 0 {
		ui.Box(c).FillWidth().Padding(36, 20).Radius(8).Background(t.Surface).Border(1, t.Border).Children(func() {
			ui.Column(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Text(c, "📘").FontSize(24)
				ui.Text(c, "当前分类暂无内容").FontSize(14).FontWeight(600).TextColor(t.Text)
				ui.Text(c, "在划词弹窗的词典、长难句分析或写作改进中点击【保存到笔记本】即可自动收录。").FontSize(12).TextColor(t.TextMuted)
			})
		})
		return
	}

	// A virtual list: every entry used to be built into a full card tree
	// each frame (~25-40 nodes each), so a thousand entries meant tens of
	// thousands of nodes per keystroke in the search box. ui.List builds
	// only the rows in view; the row height adapts to the entry's content.
	list := ui.List(c, &a.vocabList, len(entries), func(i int) {
		entry := entries[i]
		ui.Box(c).FillWidth().Padding(12, 14).Radius(8).
			Background(t.Surface).Border(1, t.Border).
			Shadow(0, 1, 4, 0, ui.RGBA(0, 0, 0, 0.04)).Children(func() {
			ui.Column(c).FillWidth().Gap(8).Children(func() {
				// 1. Meta & Actions Header: Tags on left, Time & Delete on right (SpaceBetween, never overflows)
				ui.Row(c).FillWidth().AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
					// Left: badges
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						srcLabel := "📘 生词"
						if entry.Source == "grammar" || len(strings.Fields(entry.Term)) > 2 {
							srcLabel = "🔍 长难句"
						} else if entry.Source == "writing" {
							srcLabel = "✏️ 写作"
						}
						ui.Box(c).Padding(1, 6).Radius(4).Background(t.SurfaceHover).Children(func() {
							ui.Text(c, srcLabel).FontSize(11).FontWeight(600).TextColor(t.Accent)
						})

						if entry.Phonetic != "" {
							ui.Text(c, entry.Phonetic).FontSize(12).TextColor(t.TextMuted)
						}

						if entry.PartOfSpeech != "" {
							ui.Box(c).Padding(1, 6).Radius(4).Background(t.SurfaceHover).Children(func() {
								ui.Text(c, entry.PartOfSpeech).FontSize(10).FontWeight(600).TextColor(t.Accent)
							})
						}

						if entry.Difficulty != "" {
							ui.Box(c).Padding(1, 6).Radius(10).Background(ui.Hex("#EFF6FF")).Border(1, ui.Hex("#BFDBFE")).Children(func() {
								ui.Text(c, entry.Difficulty).FontSize(10).FontWeight(700).TextColor(ui.Hex("#1D4ED8"))
							})
						}
					})

					// Right: timestamp + delete button
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Text(c, entry.CreatedAt.Format("2006-01-02 15:04")).FontSize(11).TextColor(t.TextMuted)

						btnDel := ui.Button(c, "删除").Height(22).Padding(1, 8).Radius(4).Tooltip("从生词本删除此条目")
						if btnDel.Clicked() {
							// Repaint after the removal: the row we are
							// rendering into no longer exists, and without a
							// refresh the list kept showing the deleted word.
							if vocabStore.Delete(entry.ID) {
								a.actionSavedMsg = "已删除「" + entry.Term + "」"
							}
							a.settingsRefresh()
						}
					})
				})

				// 2. Term / Sentence text (Full width, wraps gracefully)
				ui.Text(c, entry.Term).FontSize(15).FontWeight(700).TextColor(t.Text).Wrap()

				// 2. Definition
				if entry.Definition != "" {
					ui.Text(c, entry.Definition).FontSize(13).TextColor(t.Text).Wrap()
				}

				// 3. Context & Translation
				if entry.Context != "" {
					ui.Box(c).FillWidth().Padding(8, 10).Radius(6).Background(t.SurfaceHover).Children(func() {
						ui.Column(c).FillWidth().Gap(4).Children(func() {
							ui.Text(c, entry.Context).FontSize(12).FontWeight(500).TextColor(t.Text).Wrap()
							if entry.ContextTranslation != "" {
								ui.Text(c, entry.ContextTranslation).FontSize(12).TextColor(t.TextMuted).Wrap()
							}
						})
					})
				}

				// 4. Memory Tips
				if entry.MemoryTips != "" {
					ui.Box(c).FillWidth().Padding(6, 10).Radius(4).Background(ui.Hex("#FEFCE8")).Border(1, ui.Hex("#FEF08A")).Children(func() {
						ui.Text(c, "💡 "+entry.MemoryTips).FontSize(12).TextColor(ui.Hex("#713F12")).Wrap()
					})
				}
			})
		})
	})
	list.FillWidth()
}
