package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// homeState 保存主窗口「查词翻译工作台」首页的独立状态。
// 与划词浮窗完全隔离，避免用户在主窗口查词时被后台划词冲掉。
type homeState struct {
	input    string
	tab      string // "translate" | "grammar" | "writing"
	source   string
	target   string
	engine   string
	detected string
	errText  string
	busy     bool
	elapsed  time.Duration
	dict     *YoudaoDict
	gdict    *DictResult

	gen            requestToken
	aiGen          requestToken
	aiCancel       context.CancelFunc
	aiBusy         bool
	aiText         string
	aiRefining     bool
	aiRefineErr    string
	aiRefineGen    requestToken
	aiRefineCancel context.CancelFunc

	grammarResult         *SentenceAnalysisResult
	grammarResultPrefill  bool
	grammarSentIdx        int
	grammarSaved          bool
	onlyTrunk             bool
	showSlashes           bool
	filterRole            string
	grammarHoverKnowledge string

	writingResult *ImproveWritingResult
	writingSaved  bool
	wordSaved     bool
	transSaved    bool

	recent []string
}

func (a *app) pageHome(c *ui.Context, t *ui.Theme, cfg Config) {
	at := appTheme(t)

	// 顶部标题与快捷工具栏
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(Space3).Children(func() {
		ui.Column(c).Grow(1).Gap(Space1).Children(func() {
			PageTitle(c, t, "查词与翻译", "输入或粘贴英文/中文内容即查；划词与截图在后台随时可用。")
		})
		ui.Row(c).Gap(Space2).AlignItems(ui.Center).Shrink(0).Children(func() {
			targetLabel := langLabels[cfg.TargetLang]
			if targetLabel == "" {
				targetLabel = "简体中文"
			}
			ui.MenuButton(c, "→ "+targetLabel, func(m *ui.Menu) {
				for _, label := range langList() {
					if m.Item(label).Checked(label == targetLabel).Chosen() {
						code := langFromLabel(label)
						a.store.Update(func(c *Config) { c.TargetLang = code })
						a.mu.Lock()
						src := a.home.source
						a.mu.Unlock()
						if src != "" {
							a.homeTranslate(src)
						}
					}
				}
			}).Height(HeightControl).FontSize(FontHelper).Shrink(0)

			if SecondaryButton(c, t, "粘贴并翻译", iconCopy).Clicked() {
				if clip := strings.TrimSpace(mygo.Clipboard.ReadText()); clip != "" {
					a.home.input = clip
					a.homeSubmit(clip)
				}
			}
			if SecondaryButton(c, t, "截图翻译", iconScreenshot).
				Tooltip("框选屏幕区域并识别翻译 (" + cfg.HotkeyScreenshot + ")").Clicked() {
				a.TriggerScreenshotOCR()
			}
		})
	})

	// 输入卡片
	SurfaceCard(c, t).Children(func() {
		ui.Column(c).FillWidth().Gap(Space3).Children(func() {
			ui.TextArea(c, &a.home.input).Height(88).FillWidth().
				Placeholder("输入单词、短语或段落 · 回车翻译 · Shift+回车换行").
				OnTextKey(func(k ui.TextKey) bool {
					if k.Key == ui.KeyEnter && k.Mods&ui.Shift == 0 {
						if txt := CleanSelectionText(k.Text()); txt != "" {
							a.home.input = k.Text()
							a.homeSubmit(txt)
						}
						return true
					}
					return false
				})

			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(Space2).Children(func() {
				runeCount := len([]rune(strings.TrimSpace(a.home.input)))
				if runeCount > 0 {
					ui.Text(c, fmt.Sprintf("%d 字", runeCount)).Font(fontMono).
						FontFeatures("tnum").FontSize(FontMicro).TextColor(at.TextMuted)
				} else {
					ui.Text(c, "支持自动检测语种").Font(fontUI).FontSize(FontHelper).TextColor(at.TextMuted)
				}
				ui.Spacer(c).Grow(1)
				if strings.TrimSpace(a.home.input) != "" || a.home.source != "" {
					if GhostButton(c, t, "清空").Clicked() {
						a.homeClear()
					}
				}
				if PrimaryButton(c, t, "翻译").Disabled(strings.TrimSpace(a.home.input) == "").Clicked() {
					a.homeSubmit(a.home.input)
				}
			})
		})
	})

	a.mu.Lock()
	h := a.home
	a.mu.Unlock()

	if h.tab == "" {
		h.tab = "translate"
	}

	// 空态就绪面板：清晰告诉用户软件已启动、划词监听与快捷键就绪
	if strings.TrimSpace(h.source) == "" && !h.busy {
		a.renderHomeReadyState(c, t, at, cfg, h)
		return
	}

	// 结果切换标签
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(Space3).Children(func() {
		SegmentedControlMapped(c, false, h.tab, func(tab string) {
			a.homeSwitchTab(tab)
		},
			SegmentedOption[string]{Value: "translate", Label: "翻译与词典"},
			SegmentedOption[string]{Value: "grammar", Label: "长难句拆解"},
			SegmentedOption[string]{Value: "writing", Label: "写作润色"},
		)
		ui.Spacer(c).Grow(1)
		if h.source != "" {
			if GhostButton(c, t, "重新生成").Clicked() {
				a.homeRunTab(h.tab, h.source, true)
			}
		}
	})

	if h.errText != "" {
		SurfaceCard(c, t).Background(dangerSoft(t)).Border(1, dangerLine(t)).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(Space2).Children(func() {
				ui.Icon(c, iconInfo).Size(IconSize, IconSize).TextColor(at.Danger)
				ui.Text(c, h.errText).Font(fontUI).FontSize(FontHelper).TextColor(at.Danger).Wrap().Grow(1)
			})
		})
	}

	switch h.tab {
	case "grammar":
		a.renderHomeGrammarTab(c, t, at, cfg, h)
	case "writing":
		a.renderHomeWritingTab(c, t, at, cfg, h)
	default:
		a.renderHomeTranslateTab(c, t, at, cfg, h)
	}
}

func (a *app) renderHomeReadyState(c *ui.Context, t *ui.Theme, at AppTheme, cfg Config, h homeState) {
	SurfaceCard(c, t).Children(func() {
		ui.Column(c).FillWidth().Gap(Space3).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(Space2).Children(func() {
				paused := !a.selectionPausedUntil().IsZero()
				dotCol := at.Success
				statusTitle := "后台划词监听已就绪"
				if cfg.SelectionTrigger == "hotkey" {
					statusTitle = "当前为仅快捷键划词模式"
				}
				if paused {
					dotCol = at.Warning
					statusTitle = "划词触发已暂停"
				}
				ui.Box(c).Size(8, 8).Radius(RadiusXS).Background(dotCol).Shrink(0)
				ui.Text(c, statusTitle).Font(fontUI).FontSize(FontControl).FontWeight(600).TextColor(at.Text)
				ui.Spacer(c).Grow(1)
				if GhostButton(c, t, "划词设置 →").Clicked() {
					a.page = "selection"
					a.settingsRefresh()
				}
			})

			ui.Row(c).FillWidth().Wrap().Gap(Space2).AlignItems(ui.Center).Children(func() {
				a.homeShortcutChip(c, t, at, "划词翻译", cfg.HotkeySelection)
				a.homeShortcutChip(c, t, at, "截图翻译", cfg.HotkeyScreenshot)
				a.homeShortcutChip(c, t, at, "快捷浮窗", cfg.HotkeyInput)
			})

			if prob := cfg.AISetupProblem(); prob != "" {
				ui.Box(c).FillWidth().Padding(Space2, Space3).Radius(RadiusSM).
					Background(at.TintSurface).Border(1, at.TintBorder).Children(func() {
					ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(Space2).Children(func() {
						ui.Icon(c, iconSparkles).Size(IconSize, IconSize).TextColor(at.Accent)
						ui.Text(c, "配置 AI 服务后可解锁 AI 精翻、长难句拆解与写作润色（当前："+prob+"）").
							Font(fontUI).FontSize(FontHelper).TextColor(at.TextSec).Wrap().Grow(1)
						if SecondaryButton(c, t, "去配置").Clicked() {
							a.page = "ai"
							a.settingsRefresh()
						}
					})
				})
			}
		})
	})

	// 最近查询快捷词条
	recent := h.recent
	if len(recent) == 0 {
		entries := GetVocabularyStore().Filter("all", "")
		for i := 0; i < len(entries) && i < 6; i++ {
			term := strings.TrimSpace(entries[i].Term)
			if term != "" && len([]rune(term)) <= 40 {
				recent = append(recent, term)
			}
		}
	}
	if len(recent) > 0 {
		SurfaceCard(c, t).Children(func() {
			ui.Column(c).FillWidth().Gap(Space2).Children(func() {
				ui.Text(c, "快捷复查").Font(fontUI).FontSize(FontHelper).FontWeight(600).TextColor(at.TextSec)
				ui.Row(c).FillWidth().Wrap().Gap(Space2).Children(func() {
					for _, item := range recent {
						term := item
						b := ui.ButtonBase(c).Height(HeightCompact).Radius(RadiusSM).Padding(0, Space3).
							Background(at.SurfaceSubtle).Border(1, at.BorderSubtle).Center()
						if b.Hovered() {
							b.Background(at.SurfaceHover).Border(1, at.BorderHover)
						}
						b.Children(func() {
							ui.Text(c, TruncateEllipsis(term, 24)).Font(fontUI).FontSize(FontHelper).TextColor(at.Text).SingleLine()
						})
						if b.Clicked() {
							a.home.input = term
							a.homeSubmit(term)
						}
					}
				})
			})
		})
	}
}

func (a *app) homeShortcutChip(c *ui.Context, t *ui.Theme, at AppTheme, label, hk string) {
	if strings.TrimSpace(hk) == "" {
		hk = "未设置"
	}
	ui.Box(c).Padding(Space1, Space2).Radius(RadiusSM).
		Background(at.SurfaceSubtle).Border(1, at.BorderSubtle).Children(func() {
		ui.Row(c).Gap(Space2).AlignItems(ui.Center).Children(func() {
			ui.Text(c, label).Font(fontUI).FontSize(FontHelper).TextColor(at.TextSec).SingleLine()
			KeycapBadge(c, t, hk)
		})
	})
}

func (a *app) renderHomeTranslateTab(c *ui.Context, t *ui.Theme, at AppTheme, cfg Config, h homeState) {
	if h.busy {
		SurfaceCard(c, t).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(Space2).Padding(Space2, 0).Children(func() {
				ui.Spinner(c).Size(16, 16)
				ui.Text(c, "正在查询与翻译…").Font(fontUI).FontSize(FontControl).TextColor(at.TextMuted)
			})
		})
		return
	}

	if h.target != "" {
		SurfaceCard(c, t).Children(func() {
			ui.Column(c).FillWidth().Gap(Space3).Children(func() {
				ui.Text(c, h.target).Font(fontUI).FontSize(16).FontWeight(500).
					TextColor(at.Text).LineHeight(1.6).Wrap()

				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(Space2).Children(func() {
					spText, spLang, spTip := primarySpeechTarget(h.source, h.target)
					if spText != "" {
						if IconButton(c, t, iconSpeak, spTip).Clicked() {
							PlaySpeechLang(spText, spLang, false)
						}
					}
					if IconButton(c, t, iconCopy, "复制译文").Clicked() {
						mygo.Clipboard.WriteText(h.target)
						a.mu.Lock()
						a.actionSavedMsg = "译文已复制到剪贴板 ✓"
						a.mu.Unlock()
					}
					refineLabel := "AI 精翻"
					if h.aiRefining {
						refineLabel = "精翻中…"
					}
					if SecondaryButton(c, t, refineLabel, iconSparkles).Disabled(h.aiRefining).Clicked() {
						a.homeRefineWithAI()
					}
					if !h.transSaved && !isWord(h.source) {
						if SecondaryButton(c, t, "收藏", iconBookmark).Clicked() {
							a.homeSaveTranslation()
						}
					} else if h.transSaved {
						ui.Text(c, "已收藏 ✓").Font(fontUI).FontSize(FontHelper).TextColor(at.Success)
					}

					ui.Spacer(c).Grow(1)
					capText := h.engine
					if h.elapsed > 0 {
						if capText != "" {
							capText += " · "
						}
						capText += fmt.Sprintf("%.1fs", h.elapsed.Seconds())
					}
					if capText != "" {
						ui.Text(c, capText).Font(fontUI).FontSize(FontMicro).TextColor(at.TextMuted).SingleLine()
					}
				})

				if h.aiRefineErr != "" {
					ui.Text(c, h.aiRefineErr).Font(fontUI).FontSize(FontHelper).TextColor(at.Danger).Wrap()
				}
			})
		})
	}

	if h.dict != nil || h.gdict != nil {
		SurfaceCard(c, t).Children(func() {
			ui.Column(c).FillWidth().Gap(Space3).Children(func() {
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(Space3).Children(func() {
					ui.Text(c, h.source).Font(fontUI).FontSize(FontWordHead).FontWeight(600).TextColor(at.Text)
					if h.dict != nil && h.dict.UKPhone != "" {
						if GhostButton(c, t, "英 /"+h.dict.UKPhone+"/").Clicked() {
							PlaySpeechLang(h.source, h.detected, false)
						}
					}
					if h.dict != nil && h.dict.USPhone != "" {
						if GhostButton(c, t, "美 /"+h.dict.USPhone+"/").Clicked() {
							PlaySpeechLang(h.source, h.detected, true)
						}
					}
					ui.Spacer(c).Grow(1)
					if h.wordSaved {
						ui.Text(c, "已存入生词本 ✓").Font(fontUI).FontSize(FontHelper).TextColor(at.Success)
					} else if SecondaryButton(c, t, "存生词本", iconBookmark).Clicked() {
						a.homeSaveWord()
					}
				})

				if h.gdict != nil {
					for _, e := range h.gdict.Entries {
						ui.Row(c).FillWidth().Gap(Space2).AlignItems(ui.Start).Children(func() {
							ui.Box(c).Shrink(0).Padding(2, 6).Radius(RadiusXS).
								Background(at.SurfaceSubtle).Border(1, at.BorderSubtle).Children(func() {
								ui.Text(c, e.POS).FontSize(FontMicro).FontWeight(600).TextColor(at.Accent).SingleLine()
							})
							ui.Text(c, e.Explain).Font(fontUI).FontSize(FontControl).TextColor(at.Text).Wrap().Grow(1).LineHeight(1.5)
						})
					}
				} else if h.dict != nil {
					for _, ex := range h.dict.Explains {
						ui.Text(c, ex).Font(fontUI).FontSize(FontControl).TextColor(at.Text).Wrap().LineHeight(1.5)
					}
				}
			})
		})
	}
}

func (a *app) renderHomeGrammarTab(c *ui.Context, t *ui.Theme, at AppTheme, cfg Config, h homeState) {
	if prob := cfg.AISetupProblem(); prob != "" {
		SurfaceCard(c, t).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(Space2).Children(func() {
				ui.Text(c, "长难句拆解需要配置 AI 服务（"+prob+"）").Font(fontUI).FontSize(FontControl).TextColor(at.TextSec).Grow(1)
				if PrimaryButton(c, t, "前往配置").Clicked() {
					a.page = "ai"
					a.settingsRefresh()
				}
			})
		})
		return
	}
	if h.grammarResult != nil {
		sent, sentIdx, sentCount := grammarActiveSentence(h.source, h.grammarSentIdx)
		st := grammarCardState{
			OnlyTrunk:      h.onlyTrunk,
			ShowSlashes:    h.showSlashes,
			FilterRole:     h.filterRole,
			HoverKnowledge: h.grammarHoverKnowledge,
			Prefill:        h.grammarResultPrefill,
			SentIdx:        sentIdx,
			SentCount:      sentCount,
			OnPrevSent: func() {
				if sentIdx > 0 {
					a.mu.Lock()
					a.home.grammarSentIdx = sentIdx - 1
					src := a.home.source
					a.mu.Unlock()
					a.homeRunGrammar(src, false)
				}
			},
			OnNextSent: func() {
				if sentIdx+1 < sentCount {
					a.mu.Lock()
					a.home.grammarSentIdx = sentIdx + 1
					src := a.home.source
					a.mu.Unlock()
					a.homeRunGrammar(src, false)
				}
			},
			OnToggleTrunk: func() {
				a.mu.Lock()
				a.home.onlyTrunk = !a.home.onlyTrunk
				a.mu.Unlock()
				a.settingsRefresh()
			},
			OnToggleSlash: func() {
				a.mu.Lock()
				a.home.showSlashes = !a.home.showSlashes
				a.mu.Unlock()
				a.settingsRefresh()
			},
			OnToggleRole: func(role string) {
				a.mu.Lock()
				if a.home.filterRole == role {
					a.home.filterRole = ""
					a.home.grammarHoverKnowledge = ""
				} else {
					a.home.filterRole = role
					a.home.grammarHoverKnowledge = grammarExplanations[role]
				}
				a.mu.Unlock()
				a.settingsRefresh()
			},
			OnSpeak: func() {
				PlaySpeechLang(sent, h.detected, false)
			},
			OnSave: func() {
				a.homeSaveGrammar()
			},
		}
		renderGrammarVisualCardEx(c, t, sent, h.grammarResult, h.grammarSaved, st)
		return
	}
	if h.aiBusy {
		SurfaceCard(c, t).Children(func() {
			ui.Column(c).FillWidth().AlignItems(ui.Center).Gap(Space2).Padding(Space6, 0).Children(func() {
				ui.Spinner(c).Size(22, 22)
				ui.Text(c, "正在拆解长难句…").Font(fontUI).FontSize(FontControl).TextColor(at.TextMuted)
			})
		})
		return
	}
	if h.aiText != "" && !looksLikeStructuredPayload(h.aiText) {
		SurfaceCard(c, t).Children(func() {
			renderAiMarkdown(c, h.aiText, false)
		})
		return
	}
	// 没有结果也不在拆解时不能留白：空卡片比一句说明更像坏了。
	SurfaceCard(c, t).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(Space2).Children(func() {
			ui.Icon(c, iconSearch).Size(IconSize, IconSize).TextColor(at.TextMuted)
			ui.Text(c, "还没有拆解结果，点「重新生成」再跑一次。").
				Font(fontUI).FontSize(FontHelper).TextColor(at.TextMuted).Wrap().Grow(1)
		})
	})
}

func (a *app) renderHomeWritingTab(c *ui.Context, t *ui.Theme, at AppTheme, cfg Config, h homeState) {
	if prob := cfg.AISetupProblem(); prob != "" {
		SurfaceCard(c, t).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(Space2).Children(func() {
				ui.Text(c, "写作批改需要配置 AI 服务（"+prob+"）").Font(fontUI).FontSize(FontControl).TextColor(at.TextSec).Grow(1)
				if PrimaryButton(c, t, "前往配置").Clicked() {
					a.page = "ai"
					a.settingsRefresh()
				}
			})
		})
		return
	}
	if h.writingResult != nil {
		renderImproveWritingCard(c, t, h.source, h.writingResult, h.writingSaved, func(txt string) {
			mygo.Clipboard.WriteText(txt)
			a.mu.Lock()
			a.actionSavedMsg = "改后版本已复制 ✓"
			a.mu.Unlock()
			a.settingsRefresh()
		}, func() {
			a.homeSaveWriting()
		})
		return
	}
	if h.aiBusy {
		SurfaceCard(c, t).Children(func() {
			ui.Column(c).FillWidth().AlignItems(ui.Center).Gap(Space2).Padding(Space6, 0).Children(func() {
				ui.Spinner(c).Size(22, 22)
				ui.Text(c, "正在批改与润色…").Font(fontUI).FontSize(FontControl).TextColor(at.TextMuted)
			})
		})
		return
	}
	if h.aiText != "" && !looksLikeStructuredPayload(h.aiText) {
		SurfaceCard(c, t).Children(func() {
			renderAiMarkdown(c, h.aiText, false)
		})
		return
	}
	// 与长难句页签同一处理：没有结果也不在批改时，留一句说明而不是一张空卡片。
	SurfaceCard(c, t).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(Space2).Children(func() {
			ui.Icon(c, iconSearch).Size(IconSize, IconSize).TextColor(at.TextMuted)
			ui.Text(c, "还没有批改结果，点「重新生成」再跑一次。").
				Font(fontUI).FontSize(FontHelper).TextColor(at.TextMuted).Wrap().Grow(1)
		})
	})
}

func (a *app) homeClear() {
	a.mu.Lock()
	if a.home.aiCancel != nil {
		a.home.aiCancel()
	}
	if a.home.aiRefineCancel != nil {
		a.home.aiRefineCancel()
	}
	a.home.gen++
	a.home.aiGen++
	a.home.aiRefineGen++
	recent := a.home.recent
	tab := a.home.tab
	if tab == "" {
		tab = "translate"
	}
	a.home = homeState{tab: tab, recent: recent}
	a.mu.Unlock()
	a.settingsRefresh()
}

func (a *app) homeSubmit(raw string) {
	text := CleanSelectionText(raw)
	if text == "" {
		return
	}
	a.mu.Lock()
	if a.home.tab == "" {
		a.home.tab = "translate"
	}
	if a.home.source != text {
		a.home.grammarSentIdx = 0
		a.home.grammarResult = nil
		a.home.writingResult = nil
		a.home.grammarSaved = false
		a.home.writingSaved = false
		a.home.wordSaved = false
		a.home.transSaved = false
	}
	a.home.source = text
	var nextRecent []string
	nextRecent = append(nextRecent, text)
	for _, item := range a.home.recent {
		if item != text && len(nextRecent) < 6 {
			nextRecent = append(nextRecent, item)
		}
	}
	a.home.recent = nextRecent
	tab := a.home.tab
	a.mu.Unlock()

	a.homeRunTab(tab, text, false)
}

func (a *app) homeSwitchTab(tab string) {
	a.mu.Lock()
	a.home.tab = tab
	src := a.home.source
	needGrammar := tab == "grammar" && a.home.grammarResult == nil && !a.home.aiBusy
	needWriting := tab == "writing" && a.home.writingResult == nil && !a.home.aiBusy
	needTrans := tab == "translate" && a.home.target == "" && !a.home.busy
	a.mu.Unlock()
	a.settingsRefresh()
	if src == "" {
		return
	}
	if needGrammar || needWriting || needTrans {
		a.homeRunTab(tab, src, false)
	}
}

func (a *app) homeRunTab(tab, text string, fresh bool) {
	switch tab {
	case "grammar":
		a.homeRunGrammar(text, fresh)
	case "writing":
		a.homeRunWriting(text, fresh)
	default:
		a.homeTranslate(text)
	}
}

func (a *app) homeTranslate(text string) {
	cfg := a.store.Get()
	a.mu.Lock()
	a.home.gen++
	gen := a.home.gen
	a.home.busy = true
	a.home.errText = ""
	a.home.target = ""
	a.home.dict = nil
	a.home.gdict = nil
	a.mu.Unlock()
	a.settingsRefresh()

	go func() {
		defer recoverLog("home.go:homeTranslate")
		start := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()

		from := sourceLangFor(text)
		to := cfg.TargetLang
		if from == to || sameLang(from, to) {
			to = cfg.SecondLang
		}
		r := TranslateRace(ctx, text, from, to, cfg.EngineMode)

		var yd *YoudaoDict
		var gd *DictResult
		if r.Err == nil && isWord(text) && cfg.DictEnabled {
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer recoverLog("home.go:dict")
				defer wg.Done()
				yd, _ = LookupYoudao(ctx, text)
			}()
			go func() {
				defer recoverLog("home.go:gdict")
				defer wg.Done()
				gd, _ = GoogleDict(ctx, text, from, to)
			}()
			wg.Wait()
		}

		a.mu.Lock()
		if gen != a.home.gen {
			a.mu.Unlock()
			return
		}
		a.home.busy = false
		a.home.elapsed = time.Since(start)
		if r.Err != nil {
			a.home.errText = r.Err.Error()
		} else {
			a.home.target = flattenTranslation(r.Text)
			a.home.engine = r.Engine
			a.home.detected = r.Detected
			a.home.dict = yd
			a.home.gdict = gd
		}
		a.mu.Unlock()
		a.settingsRefresh()
	}()
}

func (a *app) homeRefineWithAI() {
	cfg := a.store.Get()
	if prob := cfg.AISetupProblem(); prob != "" {
		a.mu.Lock()
		a.home.aiRefineErr = prob
		a.mu.Unlock()
		a.settingsRefresh()
		return
	}
	a.mu.Lock()
	source := a.home.source
	if source == "" {
		a.mu.Unlock()
		return
	}
	if a.home.aiRefineCancel != nil {
		a.home.aiRefineCancel()
	}
	a.home.aiRefineGen++
	gen := a.home.aiRefineGen
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	a.home.aiRefineCancel = cancel
	a.home.aiRefining = true
	a.home.aiRefineErr = ""
	a.mu.Unlock()
	a.settingsRefresh()

	go func() {
		defer recoverLog("home.go:homeRefineWithAI")
		defer cancel()
		started := time.Now()
		sys := "You are a translator. Translate the text into " + langPromptName(cfg.TargetLang) +
			". Keep terminology accurate, keep the original's tone and structure, and output only the translation with no explanation."
		out, err := StreamAI(ctx, cfg.AIConn(cfg.AIModel), sys, source, cfg.AIReasoningEffortFor(cfg.AIModel), nil)
		a.mu.Lock()
		if gen != a.home.aiRefineGen {
			a.mu.Unlock()
			return
		}
		a.home.aiRefining = false
		a.home.aiRefineCancel = nil
		if err != nil {
			a.home.aiRefineErr = friendlyAIError(err)
		} else if txt := strings.TrimSpace(out); txt != "" {
			a.home.target = flattenTranslation(txt)
			a.home.engine = "AI 精翻"
			a.home.elapsed = time.Since(started)
		}
		a.mu.Unlock()
		a.settingsRefresh()
	}()
}

func (a *app) homeRunGrammar(text string, fresh bool) {
	cfg := a.store.Get()
	if cfg.AISetupProblem() != "" || strings.TrimSpace(text) == "" {
		return
	}
	act := cfg.GetAction("default-sentence-analysis")
	if act == nil {
		return
	}
	model := act.Model
	if model == "" {
		model = cfg.AIModel
	}
	a.mu.Lock()
	sent, clamped, _ := grammarActiveSentence(text, a.home.grammarSentIdx)
	a.home.grammarSentIdx = clamped
	if a.home.aiCancel != nil {
		a.home.aiCancel()
	}
	a.home.aiGen++
	gen := a.home.aiGen
	cacheKey := aiResultCacheKey(act.ID, model, cfg.TargetLang, sent, actionCacheVariant(act, cfg))
	raw := ""
	if !fresh {
		raw = a.cachedAIResult(cacheKey)
	}
	if raw != "" {
		parsed := ParseSentenceAnalysisDetailed(raw)
		if parsed.Result != nil {
			a.home.grammarResult = parsed.Result
			a.home.grammarResultPrefill = false
			a.home.grammarSaved = false
			a.home.aiBusy = false
			a.home.aiText = ""
			a.home.errText = ""
			a.mu.Unlock()
			a.settingsRefresh()
			return
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	a.home.aiCancel = cancel
	a.home.aiBusy = true
	a.home.aiText = ""
	a.home.errText = ""
	a.home.grammarResult = nil
	a.home.grammarSaved = false
	a.mu.Unlock()
	a.settingsRefresh()

	go func() {
		defer recoverLog("home.go:homeRunGrammar")
		defer cancel()
		targetLangName := langLabels[cfg.TargetLang]
		if targetLangName == "" {
			targetLangName = "简体中文"
		}
		userPrompt := FormatActionPrompt(act.Prompt, sent, "", targetLangName)
		out, err := StreamAI(ctx, cfg.AIConn(model), act.SystemPrompt, userPrompt, cfg.AIReasoningEffortFor(model), nil)
		a.mu.Lock()
		if gen != a.home.aiGen {
			a.mu.Unlock()
			return
		}
		a.home.aiBusy = false
		a.home.aiCancel = nil
		if err != nil {
			a.home.errText = friendlyAIError(err)
		} else {
			a.home.aiText = out
			if parsed := ParseSentenceAnalysisDetailed(out); parsed.Result != nil {
				a.home.grammarResult = parsed.Result
				a.home.grammarResultPrefill = false
				a.storeAIResult(cacheKey, out)
			}
		}
		a.mu.Unlock()
		a.settingsRefresh()
	}()
}

func (a *app) homeRunWriting(text string, fresh bool) {
	cfg := a.store.Get()
	if cfg.AISetupProblem() != "" || strings.TrimSpace(text) == "" {
		return
	}
	act := cfg.GetAction("default-improve-writing")
	if act == nil {
		return
	}
	model := act.Model
	if model == "" {
		model = cfg.AIModel
	}
	a.mu.Lock()
	if a.home.aiCancel != nil {
		a.home.aiCancel()
	}
	a.home.aiGen++
	gen := a.home.aiGen
	cacheKey := aiResultCacheKey(act.ID, model, cfg.TargetLang, text, actionCacheVariant(act, cfg))
	raw := ""
	if !fresh {
		raw = a.cachedAIResult(cacheKey)
	}
	if raw != "" {
		if res := ParseImproveWritingResult(raw); res != nil {
			a.home.writingResult = res
			a.home.writingSaved = false
			a.home.aiBusy = false
			a.home.aiText = ""
			a.home.errText = ""
			a.mu.Unlock()
			a.settingsRefresh()
			return
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	a.home.aiCancel = cancel
	a.home.aiBusy = true
	a.home.aiText = ""
	a.home.errText = ""
	a.home.writingResult = nil
	a.home.writingSaved = false
	a.mu.Unlock()
	a.settingsRefresh()

	go func() {
		defer recoverLog("home.go:homeRunWriting")
		defer cancel()
		targetLangName := langLabels[cfg.TargetLang]
		if targetLangName == "" {
			targetLangName = "简体中文"
		}
		userPrompt := FormatActionPrompt(act.Prompt, text, "", targetLangName)
		out, err := StreamAI(ctx, cfg.AIConn(model), act.SystemPrompt, userPrompt, cfg.AIReasoningEffortFor(model), nil)
		a.mu.Lock()
		if gen != a.home.aiGen {
			a.mu.Unlock()
			return
		}
		a.home.aiBusy = false
		a.home.aiCancel = nil
		if err != nil {
			a.home.errText = friendlyAIError(err)
		} else {
			a.home.aiText = out
			if res := ParseImproveWritingResult(out); res != nil {
				a.home.writingResult = res
				a.storeAIResult(cacheKey, out)
			}
		}
		a.mu.Unlock()
		a.settingsRefresh()
	}()
}

func (a *app) homeSaveWord() {
	a.mu.Lock()
	h := a.home
	a.mu.Unlock()
	if h.source == "" {
		return
	}
	phone := ""
	if h.dict != nil {
		if h.dict.UKPhone != "" {
			phone = "/" + h.dict.UKPhone + "/"
		} else if h.dict.USPhone != "" {
			phone = "/" + h.dict.USPhone + "/"
		}
	}
	explanation, pos := h.target, ""
	if h.gdict != nil && len(h.gdict.Entries) > 0 {
		pos = h.gdict.Entries[0].POS
	} else if h.dict != nil && len(h.dict.Explains) > 0 {
		explanation = strings.Join(h.dict.Explains, "；")
	}
	a.mu.Lock()
	a.home.wordSaved = true
	a.actionSavedMsg = "已保存「" + h.source + "」到生词本 ✓"
	a.mu.Unlock()
	a.settingsRefresh()

	entry := VocabularyEntry{
		Term: h.source, Phonetic: phone, PartOfSpeech: pos,
		Definition: explanation, Source: "dict", CreatedAt: time.Now(),
	}
	go func() {
		defer recoverLog("home.go:homeSaveWord")
		EnrichVocabularyEntry(&entry)
		GetVocabularyStore().Add(entry)
	}()
}

func (a *app) homeSaveTranslation() {
	a.mu.Lock()
	src, dst := a.home.source, a.home.target
	if src == "" || dst == "" {
		a.mu.Unlock()
		return
	}
	a.home.transSaved = true
	a.actionSavedMsg = "已保存到生词本 ✓"
	a.mu.Unlock()
	a.settingsRefresh()
	go func() {
		defer recoverLog("home.go:homeSaveTranslation")
		GetVocabularyStore().Add(VocabularyEntry{
			Term: TruncateEllipsis(src, 80), Context: src, ContextTranslation: dst, Source: "translate", CreatedAt: time.Now(),
		})
	}()
}

func (a *app) homeSaveGrammar() {
	a.mu.Lock()
	res := a.home.grammarResult
	src := a.home.source
	idx := a.home.grammarSentIdx
	if res == nil {
		a.mu.Unlock()
		return
	}
	sent, _, _ := grammarActiveSentence(src, idx)
	a.home.grammarSaved = true
	a.actionSavedMsg = "长难句已保存到生词本 ✓"
	a.mu.Unlock()
	a.settingsRefresh()

	GetVocabularyStore().Add(VocabularyEntry{
		Term: truncateFor(sent, 24), Context: sent, ContextTranslation: res.Translation, Source: "grammar", CreatedAt: time.Now(),
	})
}

func (a *app) homeSaveWriting() {
	a.mu.Lock()
	res := a.home.writingResult
	src := a.home.source
	if res == nil {
		a.mu.Unlock()
		return
	}
	a.home.writingSaved = true
	a.actionSavedMsg = "写作改进已保存到生词本 ✓"
	a.mu.Unlock()
	a.settingsRefresh()

	GetVocabularyStore().Add(VocabularyEntry{
		Term: truncateFor(src, 24), Definition: res.Improved, Context: src, ContextTranslation: res.Summary, Source: "writing", CreatedAt: time.Now(),
	})
}
