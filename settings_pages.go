package main

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"strings"
)

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

// pageSelection configures how a text selection triggers fastDictionary.
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
	heading(c, t, "翻译", "微软、谷歌、腾讯三家都免密钥，装上就能用；竞速是三家并发，微软有 350ms 宽限，慢一点但更准。")
	ui.Form(c, func() {
		ui.Field(c, "引擎", func() {
			idx := 0
			switch cfg.EngineMode {
			case "microsoft":
				idx = 1
			case "google":
				idx = 2
			case "tencent":
				idx = 3
			}
			ui.Segmented(c, &idx, "竞速", "仅微软", "仅谷歌", "仅腾讯").MaxWidth(460).OnChange(func() {
				mode := []string{"race", "microsoft", "google", "tencent"}[idx]
				a.store.Update(func(c *Config) { c.EngineMode = mode })
			})
		}).Description("竞速：三家并发，先回来先用，微软有 350ms 宽限。腾讯只参与它支持的目标语言，不支持时会自动退出")
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
		ui.Field(c, "智能生词推荐", func() {
			v := cfg.ShowNotes
			ui.Switch(c, &v).OnChange(func() {
				a.store.Update(func(c *Config) { c.ShowNotes = v })
			})
		}).Description("开启后，长句翻译会调用 AI 提取可保存的生词；关闭时不会发起这项后台请求。")
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
			// 跟随翻译 is the "no preference" slot: it keeps the popup on the
			// translate engine, which is what every existing install does.
			// Without it, the setting has no way back to the old behaviour
			// once a mode has been picked.
			idx := 0
			if cfg.AIModeIsChoice {
				switch cfg.AIMode {
				case "grammar":
					idx = 1
				case "translate", "polish":
					idx = 2
				}
			}
			ui.Segmented(c, &idx, "跟随翻译", "长难句分析", "润色翻译").MaxWidth(400).OnChange(func() {
				modes := []string{"", "grammar", "translate"}
				a.store.Update(func(c *Config) {
					if idx <= 0 {
						c.AIModeIsChoice = false
						return
					}
					c.AIModeIsChoice = true
					c.AIMode = modes[idx]
				})
			})
		}).Description("划词后直接打开哪张 AI 卡片；选「跟随翻译」则只做机翻，需要时再点标签页。词条卡片在生词本里查看")
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
			efforts := []string{EffortFast, EffortDeep}
			for i, e := range efforts {
				if e == cfg.AIEffort {
					idx = i
				}
			}
			ui.Segmented(c, &idx, "快速 (默认 · 推荐)", "深度思考").MaxWidth(340).OnChange(func() {
				v := efforts[idx]
				a.store.Update(func(c *Config) { c.AIEffort = v })
			})
		}).Description("快速：按模型实测的快捷键档位发请求，1-3 秒出结果；深度思考：强制开启思维链，慢 2-10 倍但更适合难句")
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

		ui.Field(c, "截图识别语言", func() {
			langs := []string{"auto", "zh-Hans-CN", "en-US", "ja-JP", "ko-KR"}
			labels := []string{"自动", "中文", "英文", "日文", "韩文"}
			idx := 0
			for i, l := range langs {
				if l == cfg.OCRLang {
					idx = i
				}
			}
			ui.Segmented(c, &idx, labels...).MaxWidth(340).OnChange(func() {
				v := langs[idx]
				a.store.Update(func(c *Config) { c.OCRLang = v })
			})
		}).Description("「自动」用 Windows 语言列表里的第一个 OCR 语言。截英文用「英文」更准；所选语言未安装时会自动退回自动")

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
	heading(c, t, "关于", appTitle+" 1.0.0")
	ui.Text(c, "轻巧极速的桌面划词翻译与词典工具：原生 UI 渲染、零卡顿、低内存常驻。").FontSize(13).TextColor(t.Text).Wrap()
	// FillWidth, never Fill: Fill sets both axes to 100%, so in this column the
	// divider grew to the full height of the scroll viewport and the page
	// showed two blank screens between the rows.
	ui.Divider(c).FillWidth()
	rows := [][2]string{
		{"构建", buildStamp},
		{"翻译引擎", "微软 translatetext（免鉴权）⇄ 谷歌 gtx，竞速取先到"},
		{"词典与音标", "有道 jsonapi（英/美 IPA）"},
		{"发音", "有道 dictvoice，mp3 缓存 + 系统 MCI 播放"},
		{"OCR 引擎", "Windows.Media.Ocr 原生离线神经文字识别"},
		{"AI 精翻", "magpie 网关，闪速模型 + 低推理档"},
		{"外壳", "mygo v0.6.3 原生 UI（Go，无 cgo）"},
	}
	for _, r := range rows {
		ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Start).Children(func() {
			ui.Text(c, r[0]).FontSize(12).TextColor(t.TextMuted).Width(80)
			ui.Text(c, r[1]).FontSize(12).TextColor(t.Text).Grow(1).Wrap()
		})
	}
	ui.Divider(c).FillWidth()
	ui.Text(c, "配置文件：用户数据目录下的 config.json").FontSize(12).TextColor(t.TextMuted)
}
