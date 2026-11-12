package main

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
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

// cleanBlockedAppName strips surrounding quotes and directory paths and
// ensures the .exe extension is lowercased.
func cleanBlockedAppName(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, `"'`)
	raw = strings.TrimSpace(raw)
	if idx := strings.LastIndexAny(raw, `/\`); idx != -1 {
		raw = raw[idx+1:]
	}
	raw = strings.TrimSpace(raw)
	raw = strings.ToLower(raw)
	if raw != "" && !strings.HasSuffix(raw, ".exe") {
		raw += ".exe"
	}
	return raw
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
						addApp := func() {
							name := cleanBlockedAppName(a.newBlockedApp)
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
						ui.TextInput(c, &a.newBlockedApp).Placeholder("程序名，例如 pixpin.exe").Grow(1).OnSubmit(addApp)
						btnAdd := ui.Button(c, "添加").Height(26).Padding(2, 10).Radius(5)
						if btnAdd.Clicked() {
							addApp()
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
		ui.Field(c, "引擎状态", func() {
			ui.Column(c).FillWidth().Gap(5).Children(func() {
				for _, h := range engineHealths() {
					ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(10).Children(func() {
						ui.Text(c, h.Engine).FontSize(12).TextColor(t.Text).Width(48)
						status := "正常"
						col := t.TextMuted
						if h.Open {
							remain := time.Until(h.Until).Round(time.Second)
							if remain < 0 {
								remain = 0
							}
							status = fmt.Sprintf("熔断中 · 约 %s 后重试", remain)
							col = t.Danger
						} else if h.Fails > 0 {
							status = fmt.Sprintf("连续失败 %d 次", h.Fails)
							col = t.TextMuted
						}
						ui.Text(c, status).FontSize(11).TextColor(col).Width(160)
						if h.LastError != "" {
							ui.Text(c, truncateFor(h.LastError, 88)).FontSize(10).TextColor(t.TextMuted).Grow(1).SingleLine()
						}
					})
				}
			})
		}).Description("某个引擎连续失败两次会暂时跳过，不影响其他引擎继续翻译")
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
	heading(c, t, "AI 精翻", "可选。用你自己的大模型 API，或本机 Magpie；不配置也能正常查词和机翻。")
	ui.Form(c, func() {
		a.aiEnabledField(c, cfg)
		a.aiServiceField(c, cfg)
		if cfg.IsMagpie() {
			a.aiMagpieFields(c, t, cfg)
		} else {
			a.aiDirectFields(c, t, cfg)
		}
		a.aiDefaultModeField(c, cfg)
		a.aiHelpField(c, t, cfg)
		a.aiCacheField(c, t)
	})
}

func (a *app) aiEnabledField(c *ui.Context, cfg Config) {
	idx := 0
	if cfg.AIEnabled {
		idx = 1
	}
	ui.Field(c, "状态", func() {
		ui.Segmented(c, &idx, "关闭", "启用").MaxWidth(200).OnChange(func() {
			a.store.Update(func(c *Config) { c.AIEnabled = idx == 1 })
		})
	}).Description("关闭后不显示 AI 卡片，也不会发起大模型请求。")
}

func (a *app) aiServiceField(c *ui.Context, cfg Config) {
	preset := aiPresetForConfig(cfg)
	current := preset.Name
	ui.Field(c, "模型服务", func() {
		ui.Select(c, &current, aiPresetNames()).Grow(1).OnChange(func() {
			next, ok := aiPresetByName(current)
			if !ok || next.ID == preset.ID {
				return
			}
			cleared := false
			a.store.Update(func(c *Config) {
				cleared = applyAIPreset(c, next)
			})
			a.resetAIStatus()
			a.aiShowKey = false
			a.aiHelpOpen = false
			if cleared && next.NeedsKey {
				a.mu.Lock()
				a.actionSaveFeedback = "已切换服务，请重新填写 API Key"
				a.mu.Unlock()
			}
			a.settingsRefresh()
		})
	}).Description("Magpie 是本机网关；没有装就选一个服务商，或选「自定义」填任意 OpenAI 兼容地址。")
}

func (a *app) aiMagpieFields(c *ui.Context, t *ui.Theme, cfg Config) {
	ui.Field(c, "网关地址", func() {
		v := cfg.AIURL
		ui.TextInput(c, &v).FillWidth().Placeholder(magpieDefaultEndpoint).OnChange(func() {
			a.store.Update(func(c *Config) { c.AIURL = v })
		})
	}).Description("只在本机使用，不需要 API Key。")
	a.aiModelField(c, t, cfg)
	a.aiEffortField(c, cfg)
	a.aiTestField(c, t, cfg)
}

func (a *app) aiDirectFields(c *ui.Context, t *ui.Theme, cfg Config) {
	preset := aiPresetForConfig(cfg)
	ui.Field(c, "API 地址", func() {
		v := cfg.AIURL
		ui.TextInput(c, &v).FillWidth().Placeholder("https://api.deepseek.com/v1").OnChange(func() {
			cleared := false
			a.store.Update(func(c *Config) {
				oldHost := endpointHost(c.AIURL)
				c.AIURL = v
				if endpointHost(v) != oldHost && c.AIKey != "" {
					c.AIKey = ""
					c.AIKeyEnc = ""
					cleared = true
				}
			})
			if cleared {
				a.mu.Lock()
				a.actionSaveFeedback = "地址已更换，请重新填写 API Key"
				a.mu.Unlock()
				a.settingsRefresh()
			}
		})
	}).Description("填服务商文档里的地址，通常以 /v1 结尾；程序会自动补 /chat/completions。也可以粘贴完整接口地址。远程地址请用 https://，http:// 只适合本机服务。")

	if preset.NeedsKey {
		a.aiKeyField(c, cfg, preset)
	}
	a.aiModelField(c, t, cfg)
	ui.Field(c, "图片输入", func() {
		v := cfg.AIVision
		ui.Switch(c, &v).OnChange(func() {
			a.store.Update(func(c *Config) { c.AIVision = v })
		})
	}).Description("截图翻译时把图片一起发给模型；只有支持看图的模型才需要打开。")
	a.aiTestField(c, t, cfg)
	ui.Text(c, "划词内容会发送到你选择的服务商；本机 Ollama 或本机网关不会出本机。").
		FontSize(12).TextColor(t.TextMuted).Wrap()
}

func (a *app) aiKeyField(c *ui.Context, cfg Config, preset aiPreset) {
	desc := "只保存在本机，用当前 Windows 账户加密；不要带 Bearer 前缀。"
	if preset.ID == "custom" {
		desc += "本机无认证服务可留空。"
	}
	ui.Field(c, "API Key", func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			v := cfg.AIKey
			input := ui.TextInput(c, &v).Grow(1).Placeholder("sk-...")
			if !a.aiShowKey {
				input.Password()
			}
			input.OnChange(func() {
				a.store.Update(func(c *Config) { c.AIKey = normalizeAPIKey(v) })
			})
			label := "显示"
			if a.aiShowKey {
				label = "隐藏"
			}
			if ui.Button(c, label).Height(26).Padding(2, 8).Clicked() {
				a.aiShowKey = !a.aiShowKey
				a.settingsRefresh()
			}
			if preset.KeyURL != "" {
				if ui.Button(c, "去获取 Key").Height(26).Padding(2, 8).Clicked() {
					c.OpenURL(preset.KeyURL)
				}
			}
		})
	}).Description(desc)
}

func (a *app) aiModelField(c *ui.Context, t *ui.Theme, cfg Config) {
	state := a.aiModelsStatus(cfg)
	models := aiModelsForState(cfg, state)
	showSelect := state.phase == aiModelsReady || cfg.IsMagpie()
	current := strings.TrimSpace(cfg.AIModel)
	notInFetched := state.phase == aiModelsReady && current != "" && !slices.Contains(state.models, current)
	if current != "" && !slices.Contains(models, current) {
		// Keep the active model visible even when the service did not list it.
		// Hiding it behind "自定义模型…" made the page look like it had no
		// model while a valid one was still configured.
		models = append([]string{current}, models...)
	}

	ui.Field(c, "模型", func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			if showSelect {
				options := append(append([]string(nil), models...), "自定义模型…")
				chosen := cfg.AIModel
				if !slices.Contains(models, chosen) {
					chosen = "自定义模型…"
				}
				ui.Select(c, &chosen, options).Grow(1).OnChange(func() {
					if chosen == "自定义模型…" {
						// Clearing the stored model is what makes the next frame
						// show the text input. A local "custom" flag would be a
						// second source of truth for the same fact.
						a.store.Update(func(c *Config) { c.AIModel = "" })
						a.settingsRefresh()
						return
					}
					a.store.Update(func(c *Config) { c.AIModel = chosen })
					a.settingsRefresh()
				})
			} else {
				v := cfg.AIModel
				ui.TextInput(c, &v).Grow(1).Placeholder("输入模型 ID").OnChange(func() {
					a.store.Update(func(c *Config) { c.AIModel = v })
				})
			}
			label := "获取模型"
			if state.phase == aiModelsLoading {
				label = "获取中…"
			}
			if ui.Button(c, label).Height(26).Padding(2, 8).
				Disabled(state.phase == aiModelsLoading).Clicked() {
				a.refreshAIModels()
			}
		})
		if showSelect && !slices.Contains(models, cfg.AIModel) {
			v := cfg.AIModel
			ui.TextInput(c, &v).FillWidth().Placeholder("输入模型 ID").OnChange(func() {
				a.store.Update(func(c *Config) { c.AIModel = v })
			})
		}
		switch state.phase {
		case aiModelsUnsupported:
			ui.Text(c, "该服务不提供模型列表，请直接手动填写模型 ID。").
				FontSize(12).TextColor(t.TextMuted).Wrap()
		case aiModelsFailed:
			ui.Text(c, "获取失败："+state.err).FontSize(12).TextColor(t.Danger).Wrap()
		}
		if notInFetched {
			ui.Text(c, "当前模型不在服务返回的列表里，可以保留，也可以重新选择。").
				FontSize(12).TextColor(t.TextMuted).Wrap()
		}
	}).Description("填模型 ID，不是显示名称。点「获取模型」从服务读取；有些服务不提供列表，直接手填即可。")
}

func (a *app) aiEffortField(c *ui.Context, cfg Config) {
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
	}).Description("快速：按模型实测的档位发请求，1-3 秒出结果；深度思考：强制开启思维链，慢 2-10 倍但更适合难句。")
}

func (a *app) aiTestField(c *ui.Context, t *ui.Theme, cfg Config) {
	state := a.aiProbeStatus(cfg)
	ui.Field(c, "连通性", func() {
		ui.Column(c).Gap(6).Children(func() {
			label := "测试连接"
			if state.phase == aiProbeRunning {
				label = "测试中…"
			}
			if ui.Button(c, label).Width(140).
				Disabled(state.phase == aiProbeRunning).
				Clicked() {
				a.testAIConnection()
			}
			switch state.phase {
			case aiProbeSucceeded:
				ui.Text(c, "✓ "+state.msg).FontSize(12).TextColor(t.Success).Wrap()
			case aiProbeFailed:
				ui.Text(c, "✗ "+state.msg).FontSize(12).TextColor(t.Danger).Wrap()
			}
		})
	}).Description("用当前地址、Key 和模型发一条最小请求；会消耗少量额度。")
}

func (a *app) aiDefaultModeField(c *ui.Context, cfg Config) {
	ui.Field(c, "默认模式", func() {
		// 跟随翻译 is the "no preference" slot: it keeps the popup on the
		// translate engine, which is what every existing install does.
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
			if idx > 0 && !a.store.Get().AIEnabled {
				// The preference would be ignored at popup time anyway;
				// keeping it selected would claim a mode the app cannot run.
				a.mu.Lock()
				a.actionSaveFeedback = "AI 精翻未启用，默认模式保持「跟随翻译」。请先启用 AI 精翻"
				a.mu.Unlock()
				a.settingsRefresh()
				return
			}
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
}

func (a *app) aiHelpField(c *ui.Context, t *ui.Theme, cfg Config) {
	preset := aiPresetForConfig(cfg)
	label := "不会填？看填写说明"
	if a.aiHelpOpen {
		label = "收起填写说明"
	}
	if ui.Button(c, label).Height(26).Padding(2, 10).FontSize(12).Clicked() {
		a.aiHelpOpen = !a.aiHelpOpen
		a.settingsRefresh()
	}
	if !a.aiHelpOpen {
		return
	}
	ui.Box(c).FillWidth().Padding(12, 14).Radius(8).
		Background(t.Surface).Border(1, t.Border).Children(func() {
		ui.Column(c).FillWidth().Gap(8).Children(func() {
			if preset.Magpie {
				ui.Text(c, "本机 Magpie 需要先启动 magpie 网关（默认 127.0.0.1:3425）。没装的话，把「模型服务」换成 DeepSeek 等服务商，填上 API Key 即可。").
					FontSize(12).TextColor(t.Text).Wrap()
			} else {
				var steps []string
				switch preset.ID {
				case "ollama":
					ui.Text(c, "以本机 Ollama 为例：").FontSize(12).FontWeight(600).TextColor(t.Text)
					steps = []string{
						"1. 先启动 Ollama，默认地址是 127.0.0.1:11434。",
						"2. 用 ollama list 确认模型已经下载。",
						"3. 把模型名填到「模型」，点「测试连接」。",
						"Ollama 不需要 API Key。",
					}
				case "custom":
					ui.Text(c, "以任意 OpenAI 兼容服务为例：").FontSize(12).FontWeight(600).TextColor(t.Text)
					steps = []string{
						"1. 在服务商文档里找到 API 地址，通常以 /v1 结尾。",
						"2. 把地址填到「API 地址」，把 Key 粘到「API Key」；本机无认证服务可留空。",
						"3. 填服务商给的模型 ID，点「测试连接」。",
						"看到「正常」就能用了。每次精翻会消耗账号额度。",
					}
				default:
					ui.Text(c, "以 "+preset.Name+" 为例：").FontSize(12).FontWeight(600).TextColor(t.Text)
					steps = []string{
						"1. 打开服务商控制台，创建 API Key 并复制。Key 通常只显示一次。",
						"2. 回到这里，模型服务选 " + preset.Name + "，地址已经填好；把 Key 粘进「API Key」。",
						"3. 点「获取模型」选一个模型，再点「测试连接」。",
						"看到「正常」就能用了。每次精翻会消耗账号额度。",
					}
				}
				for _, line := range steps {
					ui.Text(c, line).FontSize(12).TextColor(t.TextMuted).Wrap()
				}
			}
			if preset.KeyURL != "" {
				if ui.Button(c, "打开 "+preset.Name+" 控制台").Height(26).Padding(2, 10).FontSize(11).Clicked() {
					c.OpenURL(preset.KeyURL)
				}
			}
		})
	})
}

func (a *app) aiCacheField(c *ui.Context, t *ui.Theme) {
	ui.Field(c, "临时缓存", func() {
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, fmt.Sprintf("当前 %d 条 · 仅在内存中，重启后清空", a.aiCacheCount())).
				FontSize(12).TextColor(t.TextMuted)
			if ui.Button(c, "清空临时缓存").Height(26).Padding(2, 10).FontSize(11).Clicked() {
				a.clearAICache()
				a.mu.Lock()
				a.actionSaveFeedback = "临时 AI 缓存已清空"
				a.mu.Unlock()
				a.settingsRefresh()
			}
		})
	}).Description("AI 结果会按提示词、模型、服务地址和推理强度自动失效；这里可手动清空")
}

func (a *app) pageHotkey(c *ui.Context, t *ui.Theme, cfg Config) {
	heading(c, t, "快捷键", "全局生效，任何程序中均可秒级唤起。")
	a.initHotkeyDrafts()

	conflictMsg := checkHotkeyConflict(a.pendingHotkeySelection, a.pendingHotkeyInput, a.pendingHotkeyScreenshot)

	ui.Form(c, func() {
		ui.Field(c, "推荐预设方案", func() {
			ui.Column(c).Gap(6).Children(func() {
				p1Bg := ui.Hex("#EFF6FF")
				p1Fg := ui.Hex("#1D4ED8")
				if t.Dark {
					p1Bg = withAlpha(t.Accent, 0.16)
					p1Fg = t.Accent
				}
				btnP1 := ui.Button(c, "⚡ 极速单手：Alt+D (划词) / Alt+C (截图) / Alt+Q (输入)").Height(28).Padding(2, 10).Radius(5).
					Background(p1Bg).TextColor(p1Fg).
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

				btnDefault := ui.Button(c, "🔄 恢复默认：Alt+D (划词) / Alt+C (截图) / Alt+Q (输入)").Height(28).Padding(2, 10).Radius(5).
					Background(t.SurfaceHover).
					Tooltip("恢复出厂默认快捷键设置")
				if btnDefault.Clicked() {
					a.resetHotkeysToDefault()
				}
			})
		}).Description("点击即可一键应用，无键位冲突且符合单手人体工学习惯")

		if conflictMsg != "" {
			ui.Field(c, "冲突提示", func() {
				ui.Box(c).FillWidth().Padding(8, 12).Radius(6).
					Background(ui.RGBA(239, 68, 68, 0.08)).
					Border(1, ui.RGBA(239, 68, 68, 0.35)).Children(func() {
					ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
						ui.Icon(c, iconInfo).Size(14, 14).TextColor(t.Danger)
						ui.Text(c, conflictMsg).FontSize(12).TextColor(t.Danger).Wrap().Grow(1)
					})
				})
			})
		}

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
					msgCol := t.Accent
					if a.hotkeyMsgKind.isError() {
						msgCol = t.Danger
					}
					ui.Text(c, a.hotkeyMsg).FontSize(12).TextColor(msgCol).Wrap()
				}
			})
		})

		ui.Field(c, "格式说明", func() {
			ui.Text(c, "支持输入任意自定义组合键（如 Alt+D、Ctrl+Shift+D 等），留空表示停用该项快捷键，点击【应用修改】立即注册生效。").FontSize(12).TextColor(t.TextMuted).Wrap()
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
		{"AI 精翻", "Magpie 或任意 OpenAI 兼容服务"},
		{"外壳", "mygo v0.8.3 原生 UI（Go，无 cgo）"},
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
