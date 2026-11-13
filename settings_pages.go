package main

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *app) pageGeneral(c *ui.Context, t *ui.Theme, cfg Config) {
	PageTitle(c, t, "通用", "外观、透明度与窗口行为。改动即时保存。")
	ui.Column(c).FillWidth().Gap(Space6).Children(func() {
		SettingCard(c, t, "外观", func() {
			SettingRow(c, t, "外观主题", "跟随 Windows 深浅色或手动固定。", func() {
				SegmentedControlMapped(c, false, cfg.ThemeMode, func(mode string) {
					a.store.Update(func(c *Config) { c.ThemeMode = mode })
				},
					SegmentedOption[string]{Value: "system", Label: "跟随系统"},
					SegmentedOption[string]{Value: "light", Label: "浅色"},
					SegmentedOption[string]{Value: "dark", Label: "深色"},
				)
			})
			op := cfg.WindowOpacity
			if op <= 0 {
				op = 0.95
			}
			SettingSliderRow(c, t, "窗口透明度", "调节划词弹窗透明度（60% - 100%）。",
				fmt.Sprintf("%.0f%%", op*100), func() {
					ui.Slider(c, &op, 0.6, 1.0).Width(180).Step(0.05).OnChange(func() {
						a.store.Update(func(c *Config) { c.WindowOpacity = op })
						if a.win != nil {
							a.win.SetOpacity(op)
						}
					})
				})
		})

		SettingCard(c, t, "弹窗行为", func() {
			SettingRow(c, t, "失焦自动关闭", "划词弹窗失去焦点时自动隐藏。", func() {
				v := cfg.CloseOnBlur
				ui.Switch(c, &v).OnChange(func() {
					a.store.Update(func(c *Config) { c.CloseOnBlur = v })
				})
			})
			SettingRow(c, t, "弹窗置顶", "使划词弹窗保持在其他窗口之上。", func() {
				v := cfg.AlwaysOnTop
				ui.Switch(c, &v).OnChange(func() {
					a.store.Update(func(c *Config) { c.AlwaysOnTop = v })
					if a.win != nil {
						a.win.SetAlwaysOnTop(v)
					}
				})
			})
			w := float64(cfg.PopupWidth)
			SettingSliderRow(c, t, "弹窗宽度", "超长内容会自动纵向平滑滚动。",
				fmt.Sprintf("%.0f px", w), func() {
					ui.Slider(c, &w, 360, 680).Width(180).Step(20).OnChange(func() {
						a.store.Update(func(cfg *Config) { cfg.PopupWidth = float32(w) })
					})
				})
		})
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
	PageTitle(c, t, "划词", "选中文字之后发生什么。")
	at := appTheme(t)
	addApp := func() {
		name := cleanBlockedAppName(a.newBlockedApp)
		if name == "" {
			return
		}
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
	ui.Column(c).FillWidth().Gap(Space6).Children(func() {
		SettingCard(c, t, "触发方式", func() {
			SettingRow(c, t, "划词模式", "显示图标：选完浮出小图标；直接翻译：选完立刻弹窗；仅快捷键：只响应全局快捷键。", func() {
				SegmentedControlMapped(c, false, cfg.SelectionTrigger, func(mode string) {
					a.store.Update(func(c *Config) { c.SelectionTrigger = mode })
					a.applySelectionMode()
				},
					SegmentedOption[string]{Value: "icon", Label: "显示图标"},
					SegmentedOption[string]{Value: "auto", Label: "直接翻译"},
					SegmentedOption[string]{Value: "hotkey", Label: "仅快捷键"},
				)
			})
			SettingRow(c, t, "取词方式", "自动：优先 UI Automation，读不到再模拟 Ctrl+C（推荐）。", func() {
				SegmentedControlMapped(c, false, cfg.SelectionCapture, func(mode string) {
					a.store.Update(func(c *Config) { c.SelectionCapture = mode })
				},
					SegmentedOption[string]{Value: "auto", Label: "自动"},
					SegmentedOption[string]{Value: "copy", Label: "模拟复制"},
					SegmentedOption[string]{Value: "uia", Label: "仅 UIA"},
				)
			})
			SettingRow(c, t, "关于剪贴板", "模拟复制会先备份剪贴板再还原，图片与富文本一并保留。若某个程序取不到词，把「取词方式」改成「模拟复制」再试。", func() {})
		})

		SettingCard(c, t, "应用黑名单", func() {
			LongField(c, t, "这些程序里不划词", "截图工具必须在这里，否则框选区域会被误判成划词。", func() ui.Element {
				return ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(Space2).Children(func() {
					ui.TextInput(c, &a.newBlockedApp).Placeholder("程序名，例如 pixpin.exe").
						Grow(1).Height(HeightControl).Radius(RadiusSM).OnSubmit(addApp)
					if SecondaryButton(c, t, "添加").Clicked() {
						addApp()
					}
				})
			})
			for _, name := range cfg.SelectionBlocklist {
				n := name
				SettingRow(c, t, n, "", func() {
					if DangerGhostButton(c, t, "移除").Clicked() {
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
			if len(cfg.SelectionBlocklist) == 0 {
				ui.Box(c).FillWidth().Padding(Space3, Space4).Children(func() {
					ui.Text(c, "暂无屏蔽程序，列表为空。").Font(fontUI).
						FontSize(FontHelper).TextColor(at.TextMuted)
				})
			}
		})
	})
}

func (a *app) pageTranslate(c *ui.Context, t *ui.Theme, cfg Config) {
	PageTitle(c, t, "翻译", "微软、谷歌、腾讯三家都免密钥，装上就能用；竞速是三家并发，微软有 350ms 宽限，慢一点但更准。")
	ui.Column(c).FillWidth().Gap(Space6).Children(func() {
		SettingCard(c, t, "翻译引擎", func() {
			SettingRow(c, t, "引擎模式", "竞速：三家并发，先回来先用；腾讯只参与它支持的目标语言。", func() {
				SegmentedControlMapped(c, false, cfg.EngineMode, func(mode string) {
					a.store.Update(func(c *Config) { c.EngineMode = mode })
				},
					SegmentedOption[string]{Value: "race", Label: "竞速"},
					SegmentedOption[string]{Value: "microsoft", Label: "仅微软"},
					SegmentedOption[string]{Value: "google", Label: "仅谷歌"},
					SegmentedOption[string]{Value: "tencent", Label: "仅腾讯"},
				)
			})
			SettingRow(c, t, "引擎状态", "某个引擎连续失败两次会暂时跳过，不影响其他引擎继续翻译。", func() {
				ui.Column(c).Gap(Space1).Children(func() {
					for _, h := range engineHealths() {
						ui.Row(c).AlignItems(ui.Center).Gap(Space2).Children(func() {
							ui.Text(c, h.Engine).FontSize(FontHelper).TextColor(t.Text).Width(48)
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
							}
							ui.Text(c, status).FontSize(FontHelper).TextColor(col).SingleLine()
						})
					}
				})
			})
		})

		SettingCard(c, t, "语言偏好", func() {
			SettingRow(c, t, "首选目标语言", "翻译结果使用的语言。", func() {
				label := langLabels[cfg.TargetLang]
				ui.Select(c, &label, langList()).Height(HeightControl).
					Radius(RadiusSM).Width(InputWidth).OnChange(func() {
					code := langFromLabel(label)
					a.store.Update(func(c *Config) { c.TargetLang = code })
				})
			})
			SettingRow(c, t, "第二目标语言", "例如中文内容翻译成英文时使用。", func() {
				label := langLabels[cfg.SecondLang]
				ui.Select(c, &label, langList()).Height(HeightControl).
					Radius(RadiusSM).Width(InputWidth).OnChange(func() {
					code := langFromLabel(label)
					a.store.Update(func(c *Config) { c.SecondLang = code })
				})
			})
		})

		SettingCard(c, t, "显示选项", func() {
			SettingRow(c, t, "显示引擎与语种", "在浮窗底部标注引擎与目标语言。", func() {
				v := cfg.ShowEngineTag
				ui.Switch(c, &v).OnChange(func() {
					a.store.Update(func(c *Config) { c.ShowEngineTag = v })
				})
			})
			SettingRow(c, t, "智能生词推荐", "开启后，长句翻译会调用 AI 提取可保存的生词；关闭时不会发起这项后台请求。", func() {
				v := cfg.ShowNotes
				ui.Switch(c, &v).OnChange(func() {
					a.store.Update(func(c *Config) { c.ShowNotes = v })
				})
			})
		})
	})
}

func (a *app) pageDict(c *ui.Context, t *ui.Theme, cfg Config) {
	PageTitle(c, t, "词典与发音", "单个单词自动显示音标与释义，点音标即可朗读。")
	ui.Column(c).FillWidth().Gap(Space6).Children(func() {
		SettingCard(c, t, "本地词典", func() {
			SettingRow(c, t, "启用离线词典", "有道音标 + 谷歌词性释义，实测约 100ms。", func() {
				v := cfg.DictEnabled
				ui.Switch(c, &v).OnChange(func() {
					a.store.Update(func(c *Config) { c.DictEnabled = v })
				})
			})
		})
		SettingCard(c, t, "朗读发音", func() {
			SettingRow(c, t, "查词自动发音", "查询单词后自动发音。", func() {
				SegmentedControlMapped(c, false, cfg.AutoPronounce, func(v string) {
					a.store.Update(func(c *Config) { c.AutoPronounce = v })
				},
					SegmentedOption[string]{Value: "off", Label: "关闭"},
					SegmentedOption[string]{Value: "uk", Label: "英音"},
					SegmentedOption[string]{Value: "us", Label: "美音"},
				)
			})
			v := float64(cfg.AudioVolume)
			if v <= 0 {
				v = 80
			}
			SettingSliderRow(c, t, "朗读音量", "发音音量，改动后下一次朗读生效。",
				fmt.Sprintf("%.0f%%", v), func() {
					ui.Slider(c, &v, 0, 100).Width(180).Step(5).OnChange(func() {
						a.store.Update(func(c *Config) { c.AudioVolume = int(v) })
					})
				})
		})
	})
}

func (a *app) pageAI(c *ui.Context, t *ui.Theme, cfg Config) {
	PageTitle(c, t, "AI 精翻", "可选。用你自己的大模型 API，或本机 Magpie；不配置也能正常查词和机翻。")
	ui.Column(c).FillWidth().Gap(Space6).Children(func() {
		SettingCard(c, t, "服务", func() {
			a.aiEnabledRow(c, t, cfg)
			a.aiServiceField(c, t, cfg)
			if cfg.IsMagpie() {
				a.aiMagpieFields(c, t, cfg)
			} else {
				a.aiDirectFields(c, t, cfg)
			}
		})
		SettingCard(c, t, "模型", func() {
			a.aiModelField(c, t, cfg)
			a.aiTestField(c, t, cfg)
		})
		SettingCard(c, t, "偏好", func() {
			a.aiDefaultModeRow(c, t, cfg)
			a.aiEffortRow(c, t, cfg)
		})
		SettingCard(c, t, "维护", func() {
			a.aiCacheRow(c, t)
		})
		a.aiHelpField(c, t, cfg)
	})
}

func (a *app) aiEnabledRow(c *ui.Context, t *ui.Theme, cfg Config) {
	idx := 0
	if cfg.AIEnabled {
		idx = 1
	}
	SettingRow(c, t, "状态", aiStatusDescription(cfg), func() {
		SegmentedControl(c, &idx, "关闭", "启用").OnChange(func() {
			a.store.Update(func(c *Config) { c.AIEnabled = idx == 1 })
		})
	})
}

func aiStatusDescription(cfg Config) string {
	if problem := cfg.AISetupProblem(); cfg.AIEnabled && problem != "" {
		return "还差一步：" + problem + "。补上之前不会发起大模型请求。"
	}
	return "关闭后不显示 AI 卡片，也不发起请求。"
}

func (a *app) aiServiceField(c *ui.Context, t *ui.Theme, cfg Config) {
	preset := aiPresetForConfig(cfg)
	current := preset.Name
	LongField(c, t, "模型服务", "本机网关或任意 OpenAI 兼容服务；没有 Magpie 就选一个服务商。", func() ui.Element {
		return ui.Select(c, &current, aiPresetNames()).FillWidth().
			Height(HeightControl).Radius(RadiusSM).OnChange(func() {
			next, ok := aiPresetByName(current)
			if !ok || next.ID == preset.ID {
				return
			}
			needKey := false
			a.store.Update(func(c *Config) {
				needKey = applyAIPreset(c, next)
			})
			a.resetAIStatus()
			a.loadAIModels()
			a.aiShowKey = false
			a.aiHelpOpen = false
			if needKey {
				a.mu.Lock()
				a.actionSaveFeedback = "已切换服务，请重新填写 API Key"
				a.mu.Unlock()
			}
			a.settingsRefresh()
		})
	})
}

func (a *app) aiMagpieFields(c *ui.Context, t *ui.Theme, cfg Config) {
	LongField(c, t, "网关地址", "只在本机使用，不需要 API Key。", func() ui.Element {
		v := cfg.AIURL
		return ui.TextInput(c, &v).FillWidth().Height(HeightControl).Radius(RadiusSM).
			Placeholder(magpieDefaultEndpoint).OnChange(func() {
			a.store.Update(func(c *Config) { c.AIURL = v })
		})
	})
}

func (a *app) aiDirectFields(c *ui.Context, t *ui.Theme, cfg Config) {
	preset := aiPresetForConfig(cfg)
	LongField(c, t, "API 地址", "通常以 /v1 结尾，程序会自动补 /chat/completions。远程地址请用 https://。", func() ui.Element {
		v := cfg.AIURL
		return ui.TextInput(c, &v).FillWidth().Height(HeightControl).Radius(RadiusSM).
			Placeholder("https://api.deepseek.com/v1").OnChange(func() {
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
	})

	if preset.NeedsKey {
		a.aiKeyField(c, t, cfg, preset)
	}
	SettingRow(c, t, "图片输入", "是否发送截图由「截图识别模式」决定。", func() {
		v := cfg.AIVision
		ui.Switch(c, &v).OnChange(func() {
			a.store.Update(func(c *Config) { c.AIVision = v })
		})
	})
	ui.Box(c).FillWidth().Padding(Space3, Space4).Children(func() {
		HelperText(c, t, "划词内容会发送到你选择的服务商；本机 Ollama 或本机网关不会出本机。")
	})
}

func (a *app) aiKeyField(c *ui.Context, t *ui.Theme, cfg Config, preset aiPreset) {
	desc := "只保存在本机，用当前 Windows 账户加密；不要带 Bearer 前缀。"
	if preset.ID == "custom" {
		desc += "本机无认证服务可留空。"
	}
	LongField(c, t, "API Key", desc, func() ui.Element {
		return ui.Row(c).FillWidth().Gap(Space2).AlignItems(ui.Center).Children(func() {
			v := cfg.AIKey
			placeholder := "sk-..."
			if preset.ID == "gemini" {
				placeholder = "AIza..."
			}
			input := ui.TextInput(c, &v).Grow(1).Height(HeightControl).Radius(RadiusSM).
				Placeholder(placeholder)
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
			if SecondaryButton(c, t, label).Clicked() {
				a.aiShowKey = !a.aiShowKey
				a.settingsRefresh()
			}
			if preset.KeyURL != "" {
				if SecondaryButton(c, t, "去获取 Key").Clicked() {
					c.OpenURL(preset.KeyURL)
				}
			}
		})
	})
}

func (a *app) aiModelField(c *ui.Context, t *ui.Theme, cfg Config) {
	state := a.aiModelsStatus(cfg)
	preset := aiPresetForConfig(cfg)
	models := aiModelsForState(cfg, state)
	showSelect := state.phase == aiModelsReady || cfg.IsMagpie() || len(preset.ModelOptions) > 0
	desc := "选择或手填模型 ID；点「获取模型」可拉取实时列表。"
	if len(preset.ModelOptions) > 0 {
		desc = "从清单选择模型 ID；点「获取模型」可拉取实时列表。"
	}
	if preset.ID == "gemini" {
		desc += " 默认 gemini-3.5-flash-lite 支持图片输入和文字输出；截图识别模式可选「AI 直接看图」或「本地优先」。"
	}
	current := strings.TrimSpace(cfg.AIModel)
	notInFetched := state.phase == aiModelsReady && current != "" && !slices.Contains(state.models, current)
	if current != "" && !slices.Contains(models, current) {
		// Keep the active model visible even when the service did not list it.
		// Hiding it behind "自定义模型…" made the page look like it had no
		// model while a valid one was still configured.
		models = append([]string{current}, models...)
	}

	LongField(c, t, "模型", desc, func() ui.Element {
		main := ui.Column(c).FillWidth().Gap(Space2).Children(func() {
			ui.Row(c).FillWidth().Gap(Space2).AlignItems(ui.Center).Children(func() {
				if showSelect {
					options := append(append([]string(nil), models...), "自定义模型…")
					chosen := cfg.AIModel
					if !slices.Contains(models, chosen) {
						chosen = "自定义模型…"
					}
					ui.Select(c, &chosen, options).Grow(1).
						Height(HeightControl).Radius(RadiusSM).OnChange(func() {
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
					ui.TextInput(c, &v).Grow(1).Height(HeightControl).Radius(RadiusSM).
						Placeholder("输入模型 ID").OnChange(func() {
						a.store.Update(func(c *Config) { c.AIModel = v })
					})
				}
				label := "获取模型"
				if state.phase == aiModelsLoading {
					label = "获取中…"
				}
				if SecondaryButton(c, t, label).Disabled(state.phase == aiModelsLoading).Clicked() {
					a.refreshAIModels()
				}
			})
			if showSelect && !slices.Contains(models, cfg.AIModel) {
				v := cfg.AIModel
				ui.TextInput(c, &v).FillWidth().Height(HeightControl).Radius(RadiusSM).
					Placeholder("输入模型 ID").OnChange(func() {
					a.store.Update(func(c *Config) { c.AIModel = v })
				})
			}
			switch state.phase {
			case aiModelsUnsupported:
				HelperText(c, t, "该服务不提供实时模型列表；仍可从上方清单选择，或手动填写模型 ID。")
			case aiModelsFailed:
				HelperText(c, t, "获取失败："+state.err).TextColor(t.Danger)
			}
			if notInFetched {
				HelperText(c, t, "当前模型不在服务返回的列表里，可保留或重选。").
					TextColor(appTheme(t).Warning).SingleLine()
			}
		})
		return main
	})
}

func (a *app) aiEffortRow(c *ui.Context, t *ui.Theme, cfg Config) {
	idx := 0
	efforts := []string{EffortFast, EffortDeep}
	for i, e := range efforts {
		if e == cfg.AIEffort {
			idx = i
		}
	}
	SettingRow(c, t, "推理强度", "快速按实测档位请求；深度思考强制思维链，慢 2-10 倍但更适合难句。", func() {
		SegmentedControl(c, &idx, "快速 (默认 · 推荐)", "深度思考").OnChange(func() {
			v := efforts[idx]
			a.store.Update(func(c *Config) { c.AIEffort = v })
		})
	})
}

func (a *app) aiTestField(c *ui.Context, t *ui.Theme, cfg Config) {
	state := a.aiProbeStatus(cfg)
	at := appTheme(t)
	SettingRow(c, t, "连通性", "发送一条最小测试请求，会消耗极少额度。", func() {
		ui.Row(c).Gap(Space2).AlignItems(ui.Center).Shrink(0).Children(func() {
			switch state.phase {
			case aiProbeSucceeded:
				ui.Text(c, "● "+state.msg).Font(fontMono).FontFeatures("tnum").
					FontSize(FontHelper).TextColor(at.Success).
					MaxWidth(220).SingleLine().Ellipsis("…")
			case aiProbeFailed:
				ui.Text(c, "● "+state.msg).Font(fontMono).FontFeatures("tnum").
					FontSize(FontHelper).TextColor(at.Danger).
					MaxWidth(220).SingleLine().Ellipsis("…")
			}
			label := "测试连接"
			if state.phase == aiProbeRunning {
				label = "测试中…"
			}
			if SecondaryButton(c, t, label).Disabled(state.phase == aiProbeRunning).Clicked() {
				a.testAIConnection()
			}
		})
	})
}

func (a *app) aiDefaultModeRow(c *ui.Context, t *ui.Theme, cfg Config) {
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
	SettingRow(c, t, "默认模式", "划词后打开哪张 AI 卡片；「跟随翻译」只做机翻。", func() {
		SegmentedControl(c, &idx, "跟随翻译", "长难句分析", "润色翻译").OnChange(func() {
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
	})
}

func (a *app) aiHelpField(c *ui.Context, t *ui.Theme, cfg Config) {
	preset := aiPresetForConfig(cfg)
	label := "不会填？看填写说明"
	if a.aiHelpOpen {
		label = "收起填写说明"
	}
	if GhostButton(c, t, label).Clicked() {
		a.aiHelpOpen = !a.aiHelpOpen
		a.settingsRefresh()
	}
	if !a.aiHelpOpen {
		return
	}
	SurfaceCard(c, t).Children(func() {
		ui.Column(c).FillWidth().Gap(Space2).Children(func() {
			if preset.Magpie {
				ui.Text(c, "本机 Magpie 需要先启动 magpie 网关（默认 127.0.0.1:3425）。没装的话，把「模型服务」换成 DeepSeek 等服务商，填上 API Key 即可。").
					Font(fontUI).FontSize(FontHelper).TextColor(t.Text).Wrap()
			} else {
				var steps []string
				switch preset.ID {
				case "ollama":
					ui.Text(c, "以本机 Ollama 为例：").Font(fontUI).FontSize(FontSection).FontWeight(600).TextColor(t.Text)
					steps = []string{
						"1. 先启动 Ollama，默认地址是 127.0.0.1:11434。",
						"2. 用 ollama list 确认模型已经下载。",
						"3. 把模型名填到「模型」，点「测试连接」。",
						"Ollama 不需要 API Key。",
					}
				case "gemini":
					ui.Text(c, "以 Google AI Studio 的 Gemini 为例：").Font(fontUI).FontSize(FontSection).FontWeight(600).TextColor(t.Text)
					steps = []string{
						"1. 在 Google AI Studio 创建 API Key 并复制。",
						"2. 模型服务选 Gemini；这里已填好官方 OpenAI 兼容地址，把 Key 粘进「API Key」。",
						"3. 在「模型」清单选 gemini-3.5-flash-lite（默认），点「测试连接」。",
						"该模型支持图片输入和文字输出；截图识别模式在「快捷键」页，可选「AI 直接看图」或「本地优先」。",
						"免费额度与可用模型以 Google AI Studio 当前价格页为准，每次请求都会计入配额。",
					}
				case "custom":
					ui.Text(c, "以任意 OpenAI 兼容服务为例：").Font(fontUI).FontSize(FontSection).FontWeight(600).TextColor(t.Text)
					steps = []string{
						"1. 在服务商文档里找到 API 地址，通常以 /v1 结尾。",
						"2. 把地址填到「API 地址」，把 Key 粘到「API Key」；本机无认证服务可留空。",
						"3. 填服务商给的模型 ID，点「测试连接」。",
						"看到「正常」就能用了。每次精翻会消耗账号额度。",
					}
				default:
					ui.Text(c, "以 "+preset.Name+" 为例：").Font(fontUI).FontSize(FontSection).FontWeight(600).TextColor(t.Text)
					steps = []string{
						"1. 打开服务商控制台，创建 API Key 并复制。Key 通常只显示一次。",
						"2. 回到这里，模型服务选 " + preset.Name + "，地址已经填好；把 Key 粘进「API Key」。",
						"3. 点「获取模型」选一个模型，再点「测试连接」。",
						"看到「正常」就能用了。每次精翻会消耗账号额度。",
					}
				}
				for _, line := range steps {
					HelperText(c, t, line)
				}
			}
			if preset.KeyURL != "" {
				if SecondaryButton(c, t, "打开 "+preset.Name+" 控制台").Clicked() {
					c.OpenURL(preset.KeyURL)
				}
			}
		})
	})
}

func (a *app) aiCacheRow(c *ui.Context, t *ui.Theme) {
	SettingRow(c, t, "临时缓存", "AI 结果按提示词、模型、地址和推理强度自动失效。", func() {
		ui.Text(c, fmt.Sprintf("当前 %d 条", a.aiCacheCount())).
			Font(fontMono).FontFeatures("tnum").FontSize(FontHelper).
			TextColor(t.TextMuted)
		if DangerGhostButton(c, t, "清空临时缓存").Clicked() {
			a.clearAICache()
			a.mu.Lock()
			a.actionSaveFeedback = "临时 AI 缓存已清空"
			a.mu.Unlock()
			a.settingsRefresh()
		}
	})
}

func (a *app) pageHotkey(c *ui.Context, t *ui.Theme, cfg Config) {
	PageTitle(c, t, "快捷键", "全局生效，任何程序中均可秒级唤起。")
	a.initHotkeyDrafts()

	conflictMsg := checkHotkeyConflict(a.pendingHotkeySelection, a.pendingHotkeyInput, a.pendingHotkeyScreenshot)
	ui.Column(c).FillWidth().Gap(Space6).Children(func() {
		SettingCard(c, t, "全局快捷键", func() {
			ui.Box(c).FillWidth().Padding(Space4).BorderWidth(0, 0, 1, 0).
				BorderColor(appTheme(t).BorderSubtle).Children(func() {
				ui.Column(c).FillWidth().Gap(Space2).Children(func() {
					btnP1 := PrimaryButton(c, t, "极速单手：Alt+D (划词) / Alt+C (截图) / Alt+Q (输入)").
						Tooltip("【最推荐】左手单手盲操极速触发。注意：Alt+D 在 Chrome/Edge 中会聚焦地址栏，浏览器内建议用方案二")
					if btnP1.Clicked() {
						a.pendingHotkeySelection = "Alt+D"
						a.pendingHotkeyScreenshot = "Alt+C"
						a.pendingHotkeyInput = "Alt+Q"
						a.applyHotkeys()
					}
					btnP2 := SecondaryButton(c, t, "稳健三键：Ctrl+Alt+D (划词) / Ctrl+Alt+S (截图) / Ctrl+Alt+I (输入)").
						Tooltip("标准三键组合，防误触")
					if btnP2.Clicked() {
						a.pendingHotkeySelection = "Ctrl+Alt+D"
						a.pendingHotkeyScreenshot = "Ctrl+Alt+S"
						a.pendingHotkeyInput = "Ctrl+Alt+I"
						a.applyHotkeys()
					}
					btnDefault := SecondaryButton(c, t, "恢复默认：Alt+D (划词) / Alt+C (截图) / Alt+Q (输入)").
						Tooltip("恢复出厂默认快捷键设置")
					if btnDefault.Clicked() {
						a.resetHotkeysToDefault()
					}
				})
			})

			if conflictMsg != "" {
				ui.Box(c).FillWidth().Padding(Space3, Space4).BorderWidth(0, 0, 1, 0).
					BorderColor(appTheme(t).BorderSubtle).Children(func() {
					SurfaceCard(c, t).Background(dangerSoft(t)).Border(1, dangerLine(t)).Children(func() {
						ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(Space2).Children(func() {
							ui.Text(c, "冲突提示").Font(fontUI).FontSize(FontHelper).
								FontWeight(600).TextColor(t.Danger).SingleLine()
							ui.Icon(c, iconInfo).Size(IconSize, IconSize).TextColor(t.Danger)
							HelperText(c, t, conflictMsg).TextColor(t.Danger).Grow(1)
						})
					})
				})
			}

			SettingRow(c, t, "划词翻译快捷键", "选中文本后按下触发（推荐 Alt+D 或 Ctrl+Alt+D）。", func() {
				ui.TextInput(c, &a.pendingHotkeySelection).Width(InputWidth).
					Height(HeightControl).Radius(RadiusSM)
			})
			SettingRow(c, t, "截图翻译快捷键", "唤起屏幕框选并自动离线 OCR 翻译（推荐 Alt+C）。", func() {
				ui.TextInput(c, &a.pendingHotkeyScreenshot).Width(InputWidth).
					Height(HeightControl).Radius(RadiusSM)
			})
			SettingRow(c, t, "输入翻译快捷键", "弹出独立输入翻译框（推荐 Alt+Q）。", func() {
				ui.TextInput(c, &a.pendingHotkeyInput).Width(InputWidth).
					Height(HeightControl).Radius(RadiusSM)
			})
			SettingRow(c, t, "生效保存", "支持 Alt+D、Ctrl+Shift+D 等组合；留空表示停用该项。", func() {
				ui.Row(c).Gap(Space2).AlignItems(ui.Center).Children(func() {
					if PrimaryButton(c, t, "应用快捷键").Clicked() {
						a.applyHotkeys()
					}
					if SecondaryButton(c, t, "恢复默认").Clicked() {
						a.resetHotkeysToDefault()
					}
				})
			})
			if a.hotkeyMsg != "" {
				msgCol := t.Accent
				if a.hotkeyMsgKind.isError() {
					msgCol = t.Danger
				}
				ui.Box(c).FillWidth().Padding(Space3, Space4).Children(func() {
					ui.Text(c, a.hotkeyMsg).FontSize(FontHelper).TextColor(msgCol).Wrap()
				})
			}
		})

		SettingCard(c, t, "截图 OCR", func() {
			SettingRow(c, t, "OCR 识别语言", "「自动」用 Windows 语言列表里的第一个 OCR 语言；所选语言未安装时会明确报错。", func() {
				opts := []SegmentedOption[string]{
					{Value: "auto", Label: "自动"},
					{Value: "zh-Hans-CN", Label: "中文"},
					{Value: "en-US", Label: "英文"},
					{Value: "ja-JP", Label: "日文"},
					{Value: "ko-KR", Label: "韩文"},
				}
				SegmentedControlMapped(c, false, cfg.OCRLang, func(v string) {
					a.store.Update(func(c *Config) { c.OCRLang = v })
				}, opts...)
			})
			SettingRow(c, t, "OCR 识别模式", "本地只跑离线 OCR；AI 直接看图会立即发图给当前模型；本地优先在识别可疑时再发图。", func() {
				modes := ocrModeValues()
				labels := ocrModeLabels()
				opts := make([]SegmentedOption[string], 0, len(modes))
				for i, m := range modes {
					label := m
					if i < len(labels) {
						label = labels[i]
					}
					opts = append(opts, SegmentedOption[string]{Value: m, Label: label})
				}
				SegmentedControlMapped(c, false, normalizeOCRMode(cfg.OCRMode), func(v string) {
					a.store.Update(func(c *Config) { c.OCRMode = v })
				}, opts...)
			})
		})
	})
}

func (a *app) pageAbout(c *ui.Context, t *ui.Theme, cfg Config) {
	PageTitle(c, t, "关于", appTitle+" "+appVersion)
	at := appTheme(t)
	ui.Column(c).FillWidth().Gap(Space6).Children(func() {
		SettingCard(c, t, "版本信息", func() {
			rows := [][2]string{
				{"构建", buildStamp},
				{"翻译引擎", "微软 translatetext、谷歌 gtx、腾讯翻译君，免鉴权竞速"},
				{"词典与音标", "有道 jsonapi（英/美 IPA）"},
				{"发音", "Edge TTS 流式语音，失败时回落系统 SAPI"},
				{"OCR 引擎", "Indra OCR 本地推理，缺模型时回落 Windows.Media.Ocr"},
				{"AI 精翻", "Magpie 或任意 OpenAI 兼容服务"},
				{"外壳", "mygo v0.9.0 原生 UI（Go，无 cgo）"},
			}
			for _, r := range rows {
				label, value := r[0], r[1]
				SettingRow(c, t, label, "", func() {
					ui.Text(c, value).Font(fontUI).FontSize(FontHelper).
						TextColor(at.TextSec).Wrap().MaxWidth(320)
				})
			}
			checking, msg, url, newer := a.updateSnapshot()
			SettingRow(c, t, "版本更新", "读取 GitHub 上的最新发布版本，只提示，不自动下载或安装。", func() {
				ui.Row(c).Gap(Space2).AlignItems(ui.Center).Children(func() {
					label := "检查更新"
					if checking {
						label = "检查中…"
					}
					if SecondaryButton(c, t, label).Disabled(checking).Clicked() {
						a.checkUpdatesAsync()
					}
					if newer && url != "" {
						if SecondaryButton(c, t, "打开下载页").Clicked() {
							if err := mygo.Shell.OpenExternal(url); err != nil {
								a.actionSavedMsg = "打开下载页失败: " + err.Error()
							}
						}
					}
				})
			})
			if msg != "" {
				ui.Box(c).FillWidth().Padding(Space3, Space4).Children(func() {
					ui.Text(c, msg).Font(fontUI).FontSize(FontHelper).TextColor(at.TextMuted).Wrap()
				})
			}
		})

		SettingCard(c, t, "诊断与日志", func() {
			SettingRow(c, t, "记录划词日志", "默认只记录字符数；开启后会写入选中的原文与 OCR 结果。", func() {
				v := cfg.LogSelectionText
				ui.Switch(c, &v).OnChange(func() {
					a.store.Update(func(c *Config) { c.LogSelectionText = v })
				})
			})
			SettingRow(c, t, "日志目录", "启动时若日志超过 2 MB，会轮转为 .1 并保留一份。", func() {
				if SecondaryButton(c, t, "打开目录").Clicked() {
					if err := mygo.Shell.OpenExternal(filepath.Dir(settingsLogPath())); err != nil {
						a.actionSavedMsg = "打开日志目录失败: " + err.Error()
					}
				}
			})
		})

		ui.Box(c).FillWidth().Padding(Space3, Space4).Children(func() {
			ui.Text(c, "配置文件：用户数据目录下的 config.json；生词本与设置互不影响，可单独备份。").
				Font(fontUI).FontSize(FontHelper).TextColor(at.TextMuted).Wrap()
		})
	})
}
