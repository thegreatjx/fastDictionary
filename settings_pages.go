package main

import (
	"fmt"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *app) pageGeneral(c *ui.Context, t *ui.Theme, cfg Config) {
	PageTitle(c, t, "通用", "白昼纸墨外观、透明度与窗口行为。改动即时保存。")
	ui.Column(c).FillWidth().Gap(Space6).Children(func() {
		SettingCard(c, t, "外观", func(sc *settingCard) {
			sc.Row("主题色", "同步改变主色、胶囊高亮与焦点边框。", func() {
				at := appTheme(t)
				current := accentThemeChoice(cfg.AccentTheme).Value
				ui.Row(c).Gap(Space2).AlignItems(ui.Center).Children(func() {
					for _, choice := range accentChoices {
						selected := choice.Value == current
						b := ui.ButtonBase(c).Height(HeightCompact).Radius(14).
							Padding(0, 10).Gap(6).Center().Label(choice.Label)
						textCol := at.TextSec
						weight := 500
						if selected {
							b.Background(at.AccentSoft).Border(1, at.AccentRing)
							textCol = at.Accent
							weight = 600
						} else if b.Hovered() {
							b.Background(at.SurfaceHover).Border(1, at.BorderHover)
							textCol = at.Text
						} else {
							b.Background(at.Surface).Border(1, at.BorderCtrl)
						}
						dot := choice.Light
						b.Children(func() {
							ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
								ui.Box(c).Size(10, 10).Radius(5).Background(dot).Shrink(0)
								ui.Text(c, choice.Label).Font(fontUI).FontSize(FontHelper).
									FontWeight(weight).TextColor(textCol).SingleLine()
							})
						})
						value := choice.Value
						if b.Clicked() {
							a.store.Update(func(c *Config) { c.AccentTheme = value })
							a.refreshThemeWindows()
						}
					}
				})
			})
			op := cfg.WindowOpacity
			if op <= 0 {
				op = 0.95
			}
			sc.Slider("窗口透明度", "调整划词弹窗透明度（60%–100%）。",
				fmt.Sprintf("%.0f%%", op*100), func() {
					ui.Slider(c, &op, 0.6, 1.0).Width(160).Step(0.05).OnChange(func() {
						a.store.Update(func(c *Config) { c.WindowOpacity = op })
						if a.win != nil {
							a.win.SetOpacity(op)
						}
					})
				})
		})

		SettingCard(c, t, "弹窗行为", func(sc *settingCard) {
			sc.Row("失焦自动关闭", "划词弹窗失去焦点时自动隐藏。", func() {
				v := cfg.CloseOnBlur
				ui.Switch(c, &v).OnChange(func() {
					a.store.Update(func(c *Config) { c.CloseOnBlur = v })
				})
			})
			sc.Row("弹窗置顶", "使划词弹窗保持在其他窗口之上。", func() {
				v := cfg.AlwaysOnTop
				ui.Switch(c, &v).OnChange(func() {
					a.store.Update(func(c *Config) { c.AlwaysOnTop = v })
					if a.win != nil {
						a.win.SetAlwaysOnTop(v)
					}
				})
			})
			w := float64(cfg.PopupWidth)
			sc.Slider("弹窗宽度", "超长内容自动纵向平滑滚动。",
				fmt.Sprintf("%.0f px", w), func() {
					ui.Slider(c, &w, 360, 680).Width(160).Step(20).OnChange(func() {
						a.store.Update(func(cfg *Config) { cfg.PopupWidth = float32(w) })
					})
				})
		})
	})
}

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
		SettingCard(c, t, "触发方式", func(sc *settingCard) {
			sc.Row("划词模式", "显示图标：选完浮出小图标；直接翻译：选完立刻弹窗；仅快捷键：只响应全局快捷键。", func() {
				SegmentedControlMapped(c, false, cfg.SelectionTrigger, func(mode string) {
					a.store.Update(func(c *Config) { c.SelectionTrigger = mode })
					a.applySelectionMode()
				},
					SegmentedOption[string]{Value: "icon", Label: "显示图标"},
					SegmentedOption[string]{Value: "auto", Label: "直接翻译"},
					SegmentedOption[string]{Value: "hotkey", Label: "仅快捷键"},
				)
			})
			sc.Row("取词方式", "自动：优先 UI Automation，读不到再模拟 Ctrl+C（推荐）。", func() {
				SegmentedControlMapped(c, false, cfg.SelectionCapture, func(mode string) {
					a.store.Update(func(c *Config) { c.SelectionCapture = mode })
				},
					SegmentedOption[string]{Value: "auto", Label: "自动"},
					SegmentedOption[string]{Value: "copy", Label: "模拟复制"},
					SegmentedOption[string]{Value: "uia", Label: "仅 UIA"},
				)
			})
			sc.Row("关于剪贴板", "模拟复制会先备份剪贴板再还原，图片与富文本一并保留。若某个程序取不到词，把「取词方式」改成「模拟复制」再试。", func() {})
		})

		SettingCard(c, t, "应用黑名单", func(sc *settingCard) {
			sc.Long("这些程序里不划词", "截图工具必须在这里，否则框选区域会被误判成划词。", func() ui.Element {
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
				sc.Row(n, "", func() {
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
				sc.Custom(func() {
					ui.Box(c).FillWidth().Padding(Space3, Space4).Children(func() {
						ui.Text(c, "暂无屏蔽程序，列表为空。").Font(fontUI).
							FontSize(FontHelper).TextColor(at.TextMuted)
					})
				})
			}
		})
	})
}

func (a *app) pageTranslate(c *ui.Context, t *ui.Theme, cfg Config) {
	PageTitle(c, t, "翻译", "微软、谷歌、腾讯三家都免密钥，装上就能用；竞速是三家并发，微软有 350ms 宽限，慢一点但更准。")
	ui.Column(c).FillWidth().Gap(Space6).Children(func() {
		SettingCard(c, t, "翻译引擎", func(sc *settingCard) {
			sc.Row("引擎模式", "竞速：三家并发，先回来先用；腾讯只参与它支持的目标语言。", func() {
				SegmentedControlMapped(c, false, cfg.EngineMode, func(mode string) {
					a.store.Update(func(c *Config) { c.EngineMode = mode })
				},
					SegmentedOption[string]{Value: "race", Label: "竞速"},
					SegmentedOption[string]{Value: "microsoft", Label: "仅微软"},
					SegmentedOption[string]{Value: "google", Label: "仅谷歌"},
					SegmentedOption[string]{Value: "tencent", Label: "仅腾讯"},
				)
			})
			sc.Row("引擎状态", "某个引擎连续失败两次会暂时跳过，不影响其他引擎继续翻译。", func() {
				at := appTheme(t)
				ui.Row(c).Gap(Space2).AlignItems(ui.Center).Wrap().Children(func() {
					for _, h := range engineHealths() {
						status := "正常"
						col := at.TextSec
						dotCol := at.Success
						if h.Open {
							remain := time.Until(h.Until).Round(time.Second)
							if remain < 0 {
								remain = 0
							}
							status = fmt.Sprintf("熔断中 · 约 %s 后重试", remain)
							col = at.Danger
							dotCol = at.Danger
						} else if h.Fails > 0 {
							status = fmt.Sprintf("连续失败 %d 次", h.Fails)
							col = at.Warning
							dotCol = at.Warning
						}
						ui.Box(c).Padding(6, 10).Radius(RadiusSM).
							Background(at.SurfaceHover).Border(1, at.BorderSubtle).Children(func() {
							ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
								ui.Box(c).Size(6, 6).Radius(3).Background(dotCol)
								ui.Text(c, h.Engine).FontSize(FontHelper).FontWeight(600).TextColor(at.Text)
								ui.Text(c, status).FontSize(FontMicro).TextColor(col).SingleLine()
							})
						})
					}
				})
			})
		})

		SettingCard(c, t, "语言偏好", func(sc *settingCard) {
			sc.Row("首选目标语言", "翻译结果使用的语言。", func() {
				label := langLabels[cfg.TargetLang]
				ui.Select(c, &label, langList()).Height(HeightControl).
					Radius(RadiusSM).Width(InputWidth).OnChange(func() {
					code := langFromLabel(label)
					a.store.Update(func(c *Config) { c.TargetLang = code })
				})
			})
			sc.Row("第二目标语言", "例如中文内容翻译成英文时使用。", func() {
				label := langLabels[cfg.SecondLang]
				ui.Select(c, &label, langList()).Height(HeightControl).
					Radius(RadiusSM).Width(InputWidth).OnChange(func() {
					code := langFromLabel(label)
					a.store.Update(func(c *Config) { c.SecondLang = code })
				})
			})
		})

		SettingCard(c, t, "显示选项", func(sc *settingCard) {
			sc.Row("原文与译文排版", "紧凑：全部并成一段，浮窗最矮；智能：段内折行合并、保留段落间隔；保留换行：原文或截图怎么换行就怎么显示，适合代码和逐行对照。", func() {
				SegmentedControlMapped(c, false, cfg.LayoutMode, func(mode string) {
					a.store.Update(func(c *Config) { c.LayoutMode = mode })
				},
					SegmentedOption[string]{Value: layoutCompact, Label: "紧凑"},
					SegmentedOption[string]{Value: layoutSmart, Label: "智能"},
					SegmentedOption[string]{Value: layoutPreserve, Label: "保留换行"},
				)
			})
			sc.Row("显示引擎与语种", "在浮窗底部标注引擎与目标语言。", func() {
				v := cfg.ShowEngineTag
				ui.Switch(c, &v).OnChange(func() {
					a.store.Update(func(c *Config) { c.ShowEngineTag = v })
				})
			})
			sc.Row("智能生词推荐", "开启后，长句翻译会调用 AI 提取可保存的生词；关闭时不会发起这项后台请求。", func() {
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
		SettingCard(c, t, "本地词典", func(sc *settingCard) {
			sc.Row("启用离线词典", "有道音标 + 谷歌词性释义，实测约 100ms。", func() {
				v := cfg.DictEnabled
				ui.Switch(c, &v).OnChange(func() {
					a.store.Update(func(c *Config) { c.DictEnabled = v })
				})
			})
		})
		SettingCard(c, t, "朗读发音", func(sc *settingCard) {
			sc.Row("查词自动发音", "查询单词后自动发音。", func() {
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
			sc.Slider("朗读音量", "发音音量，改动后下一次朗读生效。",
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
		SettingCard(c, t, "服务", func(sc *settingCard) {
			a.aiEnabledRow(sc, t, cfg)
			a.aiServiceField(sc, t, cfg)
			if cfg.IsMagpie() {
				a.aiMagpieFields(sc, t, cfg)
			} else {
				a.aiDirectFields(sc, t, cfg)
			}
		})
		SettingCard(c, t, "模型", func(sc *settingCard) {
			a.aiModelField(sc, t, cfg)
			a.aiTestField(sc, t, cfg)
		})
		SettingCard(c, t, "偏好", func(sc *settingCard) {
			a.aiDefaultModeRow(sc, t, cfg)
			a.aiEffortRow(sc, t, cfg)
		})
		SettingCard(c, t, "维护", func(sc *settingCard) {
			a.aiCacheRow(sc, t)
		})
		a.aiHelpField(c, t, cfg)
	})
}

func (a *app) aiEnabledRow(sc *settingCard, t *ui.Theme, cfg Config) {
	c := sc.c
	idx := 0
	if cfg.AIEnabled {
		idx = 1
	}
	sc.Row("状态", aiStatusDescription(cfg), func() {
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

func (a *app) aiServiceField(sc *settingCard, t *ui.Theme, cfg Config) {
	c := sc.c
	preset := aiPresetForConfig(cfg)
	current := preset.Name
	sc.Long("模型服务", "本机网关或任意 OpenAI 兼容服务；没有 Magpie 就选一个服务商。", func() ui.Element {
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

func (a *app) aiMagpieFields(sc *settingCard, t *ui.Theme, cfg Config) {
	c := sc.c
	sc.Long("网关地址", "只在本机使用，不需要 API Key。", func() ui.Element {
		v := cfg.AIURL
		return ui.TextInput(c, &v).FillWidth().Height(HeightControl).Radius(RadiusSM).
			Placeholder(magpieDefaultEndpoint).OnChange(func() {
			a.store.Update(func(c *Config) { c.AIURL = v })
		})
	})
}

func (a *app) aiDirectFields(sc *settingCard, t *ui.Theme, cfg Config) {
	c := sc.c
	preset := aiPresetForConfig(cfg)
	sc.Long("API 地址", "通常以 /v1 结尾，程序会自动补 /chat/completions。远程地址请用 https://。", func() ui.Element {
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
		a.aiKeyField(sc, t, cfg, preset)
	}
	sc.Row("图片输入", "是否发送截图由「截图识别模式」决定。", func() {
		v := cfg.AIVision
		ui.Switch(c, &v).OnChange(func() {
			a.store.Update(func(c *Config) { c.AIVision = v })
		})
	})
	sc.Custom(func() {
		ui.Box(c).FillWidth().Padding(Space3, Space4).Children(func() {
			HelperText(c, t, "划词内容会发送到你选择的服务商；本机 Ollama 或本机网关不会出本机。")
		})
	})
}

func (a *app) aiKeyField(sc *settingCard, t *ui.Theme, cfg Config, preset aiPreset) {
	c := sc.c
	desc := "只保存在本机，用当前 Windows 账户加密；不要带 Bearer 前缀。"
	if preset.ID == "custom" {
		desc += "本机无认证服务可留空。"
	}
	sc.Long("API Key", desc, func() ui.Element {
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

func (a *app) aiModelField(sc *settingCard, t *ui.Theme, cfg Config) {
	c := sc.c
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
		models = append([]string{current}, models...)
	}

	sc.Long("模型", desc, func() ui.Element {
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
				if LoadingSecondaryButton(c, t, "获取模型", "获取中…", state.phase == aiModelsLoading).Clicked() {
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

func (a *app) aiEffortRow(sc *settingCard, t *ui.Theme, cfg Config) {
	c := sc.c
	idx := 0
	efforts := []string{EffortFast, EffortDeep}
	for i, e := range efforts {
		if e == cfg.AIEffort {
			idx = i
		}
	}
	sc.Row("推理强度", "快速按实测档位请求；深度思考强制思维链，慢 2-10 倍但更适合难句。", func() {
		SegmentedControl(c, &idx, "快速 (默认 · 推荐)", "深度思考").OnChange(func() {
			v := efforts[idx]
			a.store.Update(func(c *Config) { c.AIEffort = v })
		})
	})
}

func (a *app) aiTestField(sc *settingCard, t *ui.Theme, cfg Config) {
	c := sc.c
	state := a.aiProbeStatus(cfg)
	at := appTheme(t)
	sc.Row("连通性", "发送一条最小测试请求，会消耗极少额度。", func() {
		if LoadingSecondaryButton(c, t, "测试连接", "测试中…", state.phase == aiProbeRunning).Clicked() {
			a.testAIConnection()
		}
	})
	if state.phase != aiProbeSucceeded && state.phase != aiProbeFailed {
		return
	}
	ink, bg := at.Success, at.SuccessSoft
	if state.phase == aiProbeFailed {
		ink, bg = at.Danger, at.DangerHoverBg
	}
	ui.Box(c).FillWidth().Padding(0, Space4, Space4, Space4).Children(func() {
		ui.Box(c).FillWidth().Padding(Space2, Space3).Radius(RadiusSM).Background(bg).Children(func() {
			ui.Text(c, state.msg).Font(fontMono).FontFeatures("tnum").
				FontSize(FontHelper).LineHeight(1.5).TextColor(ink).Wrap()
		})
	})
}

func (a *app) aiDefaultModeRow(sc *settingCard, t *ui.Theme, cfg Config) {
	c := sc.c
	idx := 0
	if cfg.AIModeIsChoice {
		switch cfg.AIMode {
		case "grammar":
			idx = 1
		case "translate", "polish":
			idx = 2
		}
	}
	sc.Row("默认模式", "划词后打开哪张 AI 卡片；「跟随翻译」只做机翻。", func() {
		SegmentedControl(c, &idx, "跟随翻译", "长难句分析", "润色翻译").OnChange(func() {
			if idx > 0 && !a.store.Get().AIEnabled {
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

func (a *app) aiCacheRow(sc *settingCard, t *ui.Theme) {
	c := sc.c
	sc.Row("查询缓存", "翻译、生词提取与 AI 分析结果本地持久化缓存，相同文本复用零等待；改配置或点重新翻译自动刷新。", func() {
		ui.Text(c, fmt.Sprintf("当前 %d 条", a.aiCacheCount())).
			Font(fontMono).FontFeatures("tnum").FontSize(FontHelper).
			TextColor(t.TextMuted)
		if DangerGhostButton(c, t, "清空查询缓存").Clicked() {
			a.clearAICache()
			a.mu.Lock()
			a.actionSaveFeedback = "查询缓存已清空"
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
		SettingCard(c, t, "全局快捷键", func(sc *settingCard) {
			sc.Row("快捷方案", "一键套用常用组合，或直接在下方微调。", func() {
				ui.Row(c).Gap(Space2).AlignItems(ui.Center).Children(func() {
					btnP1 := AccentSoftButton(c, t, "极速单手 (Alt+D)").Height(HeightCompact).
						Tooltip("Alt+D 划词 / Alt+C 截图 / Alt+Q 输入（浏览器内 Alt+D 会聚焦地址栏，可用稳健三键）")
					if btnP1.Clicked() {
						a.pendingHotkeySelection = "Alt+D"
						a.pendingHotkeyScreenshot = "Alt+C"
						a.pendingHotkeyInput = "Alt+Q"
						a.applyHotkeys()
					}
					btnP2 := SecondaryButton(c, t, "稳健三键 (Ctrl+Alt+D)").Height(HeightCompact).
						Tooltip("Ctrl+Alt+D 划词 / Ctrl+Alt+S 截图 / Ctrl+Alt+I 输入（防误触）")
					if btnP2.Clicked() {
						a.pendingHotkeySelection = "Ctrl+Alt+D"
						a.pendingHotkeyScreenshot = "Ctrl+Alt+S"
						a.pendingHotkeyInput = "Ctrl+Alt+I"
						a.applyHotkeys()
					}
				})
			})

			if conflictMsg != "" {
				sc.Custom(func() {
					ui.Box(c).FillWidth().Padding(Space3, Space4).Children(func() {
						SurfaceCard(c, t).Background(dangerSoft(t)).Border(1, dangerLine(t)).Children(func() {
							ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(Space2).Children(func() {
								ui.Text(c, "冲突提示").Font(fontUI).FontSize(FontHelper).
									FontWeight(600).TextColor(t.Danger).SingleLine()
								ui.Icon(c, iconInfo).Size(IconSize, IconSize).TextColor(t.Danger)
								HelperText(c, t, conflictMsg).TextColor(t.Danger).Grow(1)
							})
						})
					})
				})
			}

			sc.Row("划词翻译快捷键", "选中文本后按下触发（推荐 Alt+D 或 Ctrl+Alt+D）。", func() {
				ui.TextInput(c, &a.pendingHotkeySelection).Width(InputWidth).
					Height(HeightControl).Radius(RadiusSM)
			})
			sc.Row("截图翻译快捷键", "唤起屏幕框选并自动离线 OCR 翻译（推荐 Alt+C）。", func() {
				ui.TextInput(c, &a.pendingHotkeyScreenshot).Width(InputWidth).
					Height(HeightControl).Radius(RadiusSM)
			})
			sc.Row("输入翻译快捷键", "弹出独立输入翻译框（推荐 Alt+Q）。", func() {
				ui.TextInput(c, &a.pendingHotkeyInput).Width(InputWidth).
					Height(HeightControl).Radius(RadiusSM)
			})
			sc.Row("生效保存", "支持 Alt+D、Ctrl+Shift+D 等组合；留空表示停用该项。", func() {
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
				sc.Custom(func() {
					ui.Box(c).FillWidth().Padding(Space3, Space4).Children(func() {
						ui.Text(c, a.hotkeyMsg).FontSize(FontHelper).TextColor(msgCol).Wrap()
					})
				})
			}
		})

		SettingCard(c, t, "截图 OCR", func(sc *settingCard) {
			sc.Row("OCR 识别语言", "「自动」用 Windows 语言列表里的第一个 OCR 语言；所选语言未安装时会明确报错。", func() {
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
			sc.Row("OCR 识别模式", "本地只跑离线 OCR；AI 直接看图会立即发图给当前模型；本地优先在识别可疑时再发图。", func() {
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

func mygoModuleVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range info.Deps {
			if dep.Path == "github.com/egoist/mygo" && dep.Version != "" {
				return dep.Version
			}
		}
	}
	return "v0.10.0"
}

func (a *app) pageAbout(c *ui.Context, t *ui.Theme, cfg Config) {
	PageTitle(c, t, "关于", appTitle+" "+appVersion)
	at := appTheme(t)
	ui.Column(c).FillWidth().Gap(Space6).Children(func() {
		SettingCard(c, t, "版本信息", func(sc *settingCard) {
			rows := [][2]string{
				{"构建", buildStamp},
				{"翻译引擎", "微软 translatetext、谷歌 gtx、腾讯翻译君，免鉴权竞速"},
				{"词典与音标", "有道 jsonapi（英/美 IPA）"},
				{"发音", "Edge TTS 流式语音，失败时回落系统 SAPI"},
				{"OCR 引擎", "Indra OCR 本地推理，缺模型时回落 Windows.Media.Ocr"},
				{"AI 精翻", "Magpie 或任意 OpenAI 兼容服务"},
				{"外壳", "mygo " + mygoModuleVersion() + " 原生 UI（Go，无 cgo）"},
			}
			for _, r := range rows {
				label, value := r[0], r[1]
				sc.Row(label, "", func() {
					ui.Text(c, value).Font(fontUI).FontSize(FontHelper).
						TextColor(at.TextSec).Wrap().MaxWidth(320)
				})
			}
			checking, msg, url, newer := a.updateSnapshot()
			sc.Row("版本更新", "读取 GitHub 上的最新发布版本，只提示，不自动下载或安装。", func() {
				ui.Row(c).Gap(Space2).AlignItems(ui.Center).Children(func() {
					if LoadingSecondaryButton(c, t, "检查更新", "检查中…", checking).Clicked() {
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
				sc.Custom(func() {
					ui.Box(c).FillWidth().Padding(Space3, Space4).Children(func() {
						ui.Text(c, msg).Font(fontUI).FontSize(FontHelper).TextColor(at.TextMuted).Wrap()
					})
				})
			}
		})

		SettingCard(c, t, "诊断与日志", func(sc *settingCard) {
			sc.Row("记录划词日志", "默认只记录字符数；开启后会写入选中的原文与 OCR 结果。", func() {
				v := cfg.LogSelectionText
				ui.Switch(c, &v).OnChange(func() {
					a.store.Update(func(c *Config) { c.LogSelectionText = v })
				})
			})
			sc.Row("日志目录", "启动时若日志超过 2 MB，会轮转为 .1 并保留一份。", func() {
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
