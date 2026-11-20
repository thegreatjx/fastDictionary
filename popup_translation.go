package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

func (a *app) renderPopupHeader(c *ui.Context, t *ui.Theme, cfg Config, s snapshot) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).DragWindow().Children(func() {
		if a.popupW > 0 && a.popupW < 440 {
			a.renderCompactModeMenu(c, cfg, s)
		} else {
			a.renderPopupModeTabs(c, cfg, s)
		}
		ui.Spacer(c).Grow(1)
		ui.Row(c).Gap(Space1).AlignItems(ui.Center).Children(func() {
			if s.mode == modeTranslate || s.mode == modeInput || s.mode == modeSelectionPreview {
				targetLabel := langLabels[cfg.TargetLang]
				if targetLabel == "" {
					targetLabel = "简体中文"
				}
				ui.MenuButton(c, targetLabel, func(m *ui.Menu) {
					buildLangMenu(m, cfg.TargetLang, func(code string) {
						a.store.Update(func(c *Config) { c.TargetLang = code })
						if s.source != "" {
							a.translate(s.source)
						}
					})
				}).Height(30).FontSize(12).Shrink(0)
			}
			pinIcon, pinTip := iconPin, "固定窗口（失焦不关闭）"
			if s.pinned {
				pinIcon, pinTip = iconPinned, "取消固定窗口"
			}
			if renderIconButton(c, pinIcon, pinTip, 30, 15, s.pinned).Clicked() {
				a.togglePopupPin(cfg)
			}
			if renderIconButton(c, iconClose, "关闭（Esc）", 30, 14, false).Clicked() && a.win != nil {
				stopAudio()
				a.stopScreenshotOCR()
				a.win.Hide()
			}
		})
	})
}

func (a *app) togglePopupPin(cfg Config) {
	a.mu.Lock()
	a.pinned = !a.pinned
	pinned := a.pinned
	a.mu.Unlock()
	if a.win != nil {
		a.win.SetAlwaysOnTop(pinned || cfg.AlwaysOnTop)
		a.win.Update(func() {})
	}
}

type popupModeOption struct {
	mode  popupMode
	label string
}

var popupModeLabels = [...]string{"翻译", "长难句", "写作"}

func isPopupModeEnabled(cfg Config, m popupMode) bool {
	switch m {
	case modeGrammar:
		if act := cfg.GetAction("default-sentence-analysis"); act != nil {
			return act.Enabled
		}
		return true
	case modeWriting:
		if act := cfg.GetAction("default-improve-writing"); act != nil {
			return act.Enabled
		}
		return true
	default:
		return true
	}
}

func enabledPopupModes(cfg Config) []popupModeOption {
	opts := []popupModeOption{{mode: modeTranslate, label: "翻译"}}
	if isPopupModeEnabled(cfg, modeGrammar) {
		opts = append(opts, popupModeOption{mode: modeGrammar, label: "长难句"})
	}
	if isPopupModeEnabled(cfg, modeWriting) {
		opts = append(opts, popupModeOption{mode: modeWriting, label: "写作"})
	}
	return opts
}

func popupModeIndex(mode, target popupMode) int {
	return popupModeIndexIn(enabledPopupModes(DefaultConfig()), mode, target)
}

func popupModeIndexIn(opts []popupModeOption, mode, target popupMode) int {
	if mode == modeInput {
		mode = target
	}
	for i, opt := range opts {
		if opt.mode == mode {
			return i
		}
	}
	return 0
}

func (a *app) switchPopupModeByOpts(opts []popupModeOption, index int) {
	if index < 0 || index >= len(opts) {
		return
	}
	target := opts[index].mode
	a.mu.Lock()
	a.inputTargetMode = target
	a.inputText = ""
	a.mu.Unlock()
	a.switchMode(target)
}

func (a *app) switchPopupMode(index int) {
	a.switchPopupModeByOpts(enabledPopupModes(a.store.Get()), index)
}

func (a *app) renderPopupModeTabs(c *ui.Context, cfg Config, s snapshot) {
	opts := enabledPopupModes(cfg)
	if len(opts) <= 1 {
		at := appTheme(c.Theme())
		ui.Text(c, opts[0].label).Font(fontUI).FontSize(FontControl).FontWeight(600).
			TextColor(at.Text).Padding(0, Space1).SingleLine()
		return
	}
	labels := make([]string, len(opts))
	for i, opt := range opts {
		labels[i] = opt.label
	}
	modeIdx := popupModeIndexIn(opts, s.mode, s.inputTargetMode)
	SegmentedControlCompact(c, &modeIdx, labels...).
		Label("阅读工具").OnChange(func() { a.switchPopupModeByOpts(opts, modeIdx) })
}

func (a *app) renderCompactModeMenu(c *ui.Context, cfg Config, s snapshot) {
	opts := enabledPopupModes(cfg)
	if len(opts) <= 1 {
		at := appTheme(c.Theme())
		ui.Text(c, opts[0].label).Font(fontUI).FontSize(FontControl).FontWeight(600).
			TextColor(at.Text).Padding(0, Space1).SingleLine()
		return
	}
	idx := popupModeIndexIn(opts, s.mode, s.inputTargetMode)
	label := opts[idx].label
	ui.MenuButton(c, label, func(m *ui.Menu) {
		for i, opt := range opts {
			if m.Item(opt.label).Checked(i == idx).Chosen() {
				a.switchPopupModeByOpts(opts, i)
			}
		}
	}).Height(30).FontSize(12).Shrink(0).Label("切换阅读工具")
}

func (a *app) renderTranslationContent(c *ui.Context, t *ui.Theme, cfg Config, s snapshot) {
	if s.mode == modeInput {
		placeholder := "输入要翻译的内容 · 回车翻译 · Shift+回车换行"
		switch s.inputTargetMode {
		case modeGrammar:
			placeholder = "输入要拆解的英文长句 · 回车拆解 · Shift+回车换行"
		case modeWriting:
			placeholder = "输入要批改的英文句子或段落 · 回车批改 · Shift+回车换行"
		case modeDictionary:
			placeholder = "输入要查询的单词 · 回车查词"
		}
		ui.TextArea(c, &a.inputText).Height(64).FillWidth().
			Placeholder(placeholder).
			AutoFocus().OnTextKey(func(k ui.TextKey) bool {
			if k.Key == ui.KeyEnter && k.Mods&ui.Shift == 0 {
				raw := k.Text()
				if text := cleanSourceForLayout(raw, a.store.Get().LayoutMode, false); text != "" {
					targetMode := a.inputTargetMode
					if targetMode == 0 {
						targetMode = modeTranslate
					}
					a.mu.Lock()
					a.source = text
					a.rawSource = raw
					a.inputText = ""
					a.mu.Unlock()
					a.switchMode(targetMode)
				}
				return true
			}
			return false
		})
	}
	hasDict := s.dict != nil || s.gdict != nil
	if !s.isScreenshotMode && isWordSource(s) && (hasDict || s.target != "") {
		a.renderWordTranslationCard(c, t, cfg, s)
		a.renderNotesSummary(c, t, cfg, s)
		return
	}
	if s.isScreenshotMode {
		a.renderScreenshotSource(c, t, s)
	} else if s.source != "" {
		a.renderTranslationSource(c, t, s)
	}
	if s.mode == modeInput {
		ui.Divider(c).FillWidth()
	}
	if s.busy {
		at := appTheme(t)
		ui.Box(c).FillWidth().Padding(8, 2).Children(func() {
			ui.Text(c, "正在翻译…").FontSize(13).TextColor(at.TextMuted)
		})
	}
	if s.target == "" && !cardHasPendingWork(s) && s.mode != modeInput && s.source == "" && s.errText == "" {
		a.renderTranslateEmptyState(c, t)
	}
	if s.target != "" {
		hasMultiPara := strings.Contains(strings.TrimSpace(s.source), "\n") && strings.Contains(strings.TrimSpace(s.target), "\n")
		if (s.isScreenshotMode || hasMultiPara) && s.ocrCompare {
			a.renderOcrParagraphPairs(c, t, s)
			a.renderScreenshotPairActions(c, t, cfg, s)
		} else {
			if (s.isScreenshotMode || hasMultiPara) && strings.Contains(s.target, "\n") {
				if ui.Button(c, "段落对照").Height(24).Padding(1, 6).FontSize(10).Clicked() {
					a.mu.Lock()
					a.ocrCompare = true
					a.mu.Unlock()
					a.fitHeight()
				}
			}
			at := appTheme(t)
			ui.Box(c).FillWidth().Padding(Space3, Space4).Radius(RadiusTrack).
				Background(at.Surface).Border(1, at.BorderSubtle).Children(func() {
				ui.Column(c).FillWidth().Gap(Space3).Children(func() {
					ui.Column(c).FillWidth().Gap(Space2).Children(func() {
						for _, para := range splitDisplayParagraphs(s.target) {
							ui.Text(c, para).FontSize(15).FontWeight(400).
								TextColor(at.Text).Wrap().LineHeight(1.6)
						}
					})
					ui.Row(c).FillWidth().Gap(Space1).AlignItems(ui.Center).Children(func() {
						copyIcon := iconCopy
						copyTip := "复制译文"
						if s.copiedMsg != "" {
							copyIcon = iconCheck
							copyTip = "译文已复制"
						}
						if renderIconButton(c, copyIcon, copyTip, 28, 14, false).Clicked() {
							a.copyToClipboard(s.target)
						}
						spText, spLang, spTip := primarySpeechTarget(s.source, s.target)
						if spText != "" {
							if renderIconButton(c, iconSpeak, spTip, 28, 14, false).Clicked() {
								PlaySpeechLang(spText, spLang, false)
							}
						}
						a.renderLayoutMenu(c, cfg)
						renderAIRefineControls(c, t, a, s)
						ui.Spacer(c).Grow(1)

						if cfg.ShowNotes && shouldExtractNotes(s.source) {
							ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
								switch notesViewPhase(s) {
								case notesLoading:
									ui.Box(c).Padding(0, Space1).Children(func() {
										ui.Text(c, "发现生词中…").Font(fontUI).FontSize(FontHelper).TextColor(t.TextMuted)
									})
								case notesReady:
									btnText := fmt.Sprintf("猜你想存 · %d", len(s.notes))
									tip := "展开生词推荐"
									if s.notesExpanded {
										btnText = fmt.Sprintf("收起生词 · %d", len(s.notes))
										tip = "收起生词推荐"
									}
									btnNotes := ui.Button(c, "").Tooltip(tip).Height(HeightCompact).Padding(0, Space2).Radius(RadiusSM).Center()
									bg := at.TintSurface
									borderCol := at.TintBorder
									if s.notesExpanded {
										bg = at.AccentSoft
										borderCol = at.AccentRing
									}
									if btnNotes.Hovered() {
										bg = at.SurfaceHover
									}
									btnNotes.Background(bg).Border(1, borderCol)
									btnNotes.Children(func() {
										ui.Row(c).Gap(Space1).AlignItems(ui.Center).Children(func() {
											ui.Icon(c, iconBookmark).Size(14, 14).TextColor(at.Accent)
											ui.Text(c, btnText).Font(fontUI).FontSize(FontHelper).FontWeight(600).TextColor(at.Accent)
										})
									})
									if btnNotes.Clicked() {
										a.mu.Lock()
										a.notesExpanded = !a.notesExpanded
										a.mu.Unlock()
										a.fitHeight()
									}
								}
							})
						}
					})
				})
			})
		}
	}
	a.renderNotesSummary(c, t, cfg, s)
}

func isWordSource(s snapshot) bool {
	if s.isWord {
		return true
	}
	trimmed := strings.TrimSpace(s.source)
	return trimmed != "" && !strings.Contains(trimmed, " ") && len([]rune(trimmed)) <= 28 && !strings.Contains(trimmed, "\n")
}

func (a *app) renderWordTranslationCard(c *ui.Context, t *ui.Theme, cfg Config, s snapshot) {
	at := appTheme(t)
	hasPhones := (s.dict != nil && (s.dict.UKPhone != "" || s.dict.USPhone != "")) || (s.gdict != nil && s.gdict.Pron != "")
	hasDict := s.dict != nil || s.gdict != nil

	ui.Box(c).FillWidth().Padding(Space3, Space4).Radius(RadiusTrack).
		Background(at.Surface).Border(1, at.BorderSubtle).Children(func() {
		ui.Column(c).FillWidth().Gap(Space2 + 2).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(Space2).Children(func() {
				ui.Text(c, strings.TrimSpace(s.source)).Font(fontHeadword).FontSize(22).
					FontWeight(700).TextColor(at.Text).Grow(1).Wrap()
				copyIcon, copyTip := iconCopy, "复制原文"
				if s.copiedMsg != "" {
					copyIcon, copyTip = iconCheck, "已复制"
				}
				if renderIconButton(c, copyIcon, copyTip, HeightIconMd, 14, false).Clicked() {
					a.copyToClipboard(s.source)
				}
				if !hasPhones {
					if renderIconButton(c, iconSpeak, "朗读原文", HeightIconMd, 14, false).Clicked() {
						PlaySpeechLang(s.source, a.speechLang(), false)
					}
				}
				if s.isWord {
					renderWordSaveButton(c, t, s, a)
				}
			})

			if hasPhones {
				ui.Row(c).FillWidth().Gap(Space2).AlignItems(ui.Center).Wrap().Children(func() {
					if s.dict != nil && s.dict.UKPhone != "" {
						renderHomePhonePill(c, at, "UK", s.dict.UKPhone, func() {
							a.play(s.source, false)
						})
					}
					if s.dict != nil && s.dict.USPhone != "" {
						renderHomePhonePill(c, at, "US", s.dict.USPhone, func() {
							a.play(s.source, true)
						})
					} else if s.gdict != nil && s.gdict.Pron != "" {
						renderHomePhonePill(c, at, "", s.gdict.Pron, func() {
							a.play(s.source, false)
						})
					}
				})
			}

			if s.busy {
				ui.Box(c).FillWidth().Padding(Space1, 0).Children(func() {
					ui.Text(c, "正在翻译…").FontSize(13).TextColor(at.TextMuted)
				})
			}

			if hasDict {
				dictBlock(c, t, s, a)
				if s.target != "" {
					ui.Row(c).FillWidth().AlignItems(ui.Stretch).Radius(RadiusSM).
						Background(at.SurfaceHover).Clip().Children(func() {
						ui.Box(c).Width(inkRuleWidth).Background(at.Accent)
						ui.Box(c).Grow(1).Padding(6, 10).Children(func() {
							ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(Space2).Children(func() {
								ui.Text(c, "速译").Font(fontUI).FontSize(FontMicro).FontWeight(600).TextColor(at.TextMuted).SingleLine()
								ui.Text(c, flattenTranslation(s.target)).Font(fontUI).FontSize(13.5).FontWeight(500).TextColor(at.TextSec).Wrap().Grow(1)
								if renderIconButton(c, iconCopy, "复制译文", 22, 12, false).Clicked() {
									a.copyToClipboard(s.target)
								}
							})
						})
					})
				}
			} else if s.target != "" {
				ui.Divider(c).FillWidth()
				ui.Column(c).FillWidth().Gap(Space2).Children(func() {
					for _, para := range splitDisplayParagraphs(s.target) {
						ui.Text(c, para).FontSize(17).FontWeight(600).TextColor(at.Text).Wrap().LineHeight(1.45)
					}
				})
				ui.Row(c).FillWidth().Gap(Space1).AlignItems(ui.Center).Children(func() {
					copyIcon := iconCopy
					copyTip := "复制译文"
					if s.copiedMsg != "" {
						copyIcon = iconCheck
						copyTip = "译文已复制"
					}
					if renderIconButton(c, copyIcon, copyTip, 28, 14, false).Clicked() {
						a.copyToClipboard(s.target)
					}
					spText, spLang, spTip := primarySpeechTarget(s.source, s.target)
					if spText != "" {
						if renderIconButton(c, iconSpeak, spTip, 28, 14, false).Clicked() {
							PlaySpeechLang(spText, spLang, false)
						}
					}
				})
			}
		})
	})
}

func splitDisplayParagraphs(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.Trim(s, "\n")
	if strings.TrimSpace(s) == "" {
		return nil
	}
	blocks := strings.Split(s, "\n\n")
	out := make([]string, 0, len(blocks))
	for _, block := range blocks {
		block = strings.Trim(block, "\n")
		if strings.TrimSpace(block) != "" {
			out = append(out, block)
		}
	}
	return out
}

var layoutModeOptions = [...]struct{ label, mode string }{
	{"紧凑", layoutCompact},
	{"智能", layoutSmart},
	{"保留换行", layoutPreserve},
}

func layoutModeLabel(mode string) string {
	for _, option := range layoutModeOptions {
		if option.mode == normalizeLayoutMode(mode) {
			return option.label
		}
	}
	return layoutModeOptions[1].label
}

func (a *app) renderLayoutMenu(c *ui.Context, cfg Config) {
	mode := normalizeLayoutMode(cfg.LayoutMode)
	btn := renderIconButton(c, iconLayout, "排版："+layoutModeLabel(mode), 28, 14, mode != layoutSmart)
	btn.Menu(func(m *ui.Menu) {
		for _, option := range layoutModeOptions {
			if m.Item(option.label).Checked(option.mode == mode).Chosen() {
				a.applyLayoutMode(option.mode)
			}
		}
	})
}

func (a *app) applyLayoutMode(mode string) {
	mode = normalizeLayoutMode(mode)
	if a.store.Get().LayoutMode == mode {
		return
	}
	a.store.Update(func(c *Config) { c.LayoutMode = mode })

	a.mu.Lock()
	raw := a.rawSource
	source := a.source
	screenshot := a.isScreenshotMode
	aiAnswer := isAIResultEngine(a.engine)
	busy := a.busy
	hasTarget := a.target != ""
	if mode == layoutCompact {
		a.ocrCompare = false
	}
	a.mu.Unlock()
	if raw == "" {
		raw = source
	}

	if !busy && hasTarget && !aiAnswer && raw != "" {
		if screenshot {
			a.translateKeepingLines(raw)
		} else {
			a.translate(raw)
		}
		return
	}

	a.mu.Lock()
	if raw != "" {
		a.source = cleanSourceForLayout(raw, mode, screenshot)
	}
	a.target = formatTranslationForLayout(a.target, mode, screenshot)
	a.mu.Unlock()
	a.fitHeight()
}

func translationEngineCaption(s snapshot, targetLang string) string {
	parts := make([]string, 0, 3)
	if s.detected != "" && !sameLang(s.detected, targetLang) {
		parts = append(parts, "翻译自 "+langDisplayName(s.detected))
	}
	if s.engine != "" {
		parts = append(parts, s.engine)
	}
	if s.elapsed > 0 {
		parts = append(parts, fmt.Sprintf("%.1fs", s.elapsed.Seconds()))
	}
	return strings.Join(parts, " · ")
}

func langDisplayName(tag string) string {
	if name := langLabels[tag]; name != "" {
		return name
	}
	lower := strings.ToLower(strings.ReplaceAll(tag, "_", "-"))
	switch {
	case strings.HasPrefix(lower, "zh-hant"), strings.HasPrefix(lower, "zh-tw"),
		strings.HasPrefix(lower, "zh-hk"), strings.HasPrefix(lower, "zh-mo"):
		return langLabels["zh-Hant"]
	case strings.HasPrefix(lower, "zh"):
		return langLabels["zh-Hans"]
	}
	for _, full := range langOptions {
		if strings.HasPrefix(lower, langPrefix(full)) {
			if name := langLabels[full]; name != "" {
				return name
			}
		}
	}
	return tag
}

const sourceCollapseThreshold = 180

func (a *app) renderTranslationSource(c *ui.Context, t *ui.Theme, s snapshot) {
	at := appTheme(t)
	text := s.source
	if s.isWord || (!strings.Contains(strings.TrimSpace(text), " ") && len([]rune(strings.TrimSpace(text))) <= 28 && !strings.Contains(text, "\n")) {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(Space2).Padding(2, 2).Children(func() {
			ui.Text(c, strings.TrimSpace(text)).Font(fontHeadword).FontSize(FontWordHead).
				FontWeight(700).TextColor(at.Text).Grow(1).Wrap()
			if renderIconButton(c, iconCopy, "复制原文", HeightIconMd, 14, false).Clicked() {
				a.copyToClipboard(s.source)
			}
			if renderIconButton(c, iconSpeak, "朗读原文", HeightIconMd, 14, false).Clicked() {
				PlaySpeechLang(s.source, a.speechLang(), false)
			}
		})
		return
	}
	isLong := len([]rune(text)) > sourceCollapseThreshold || strings.Count(text, "\n") >= 4
	if isLong && !s.sourceExpanded {
		text = TruncateEllipsis(flattenTranslation(text), sourceCollapseThreshold)
	}
	ui.Row(c).FillWidth().AlignItems(ui.Stretch).Radius(RadiusSM).
		Background(at.SurfaceHover).Clip().Children(func() {
		ui.Box(c).Width(inkRuleWidth).Background(at.Accent)
		ui.Box(c).Grow(1).Padding(Space2, Space3).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(Space1).Children(func() {
				ui.Column(c).Grow(1).Gap(Space2).Children(func() {
					for _, para := range splitDisplayParagraphs(text) {
						ui.Text(c, para).Font(fontUI).FontSize(FontControl).
							TextColor(at.TextSec).Wrap().LineHeight(1.5)
					}
				})
				ui.Row(c).Gap(0).AlignItems(ui.Center).Shrink(0).Children(func() {
					if isLong {
						icon, tip := iconChevronDown, "展开完整原文"
						if s.sourceExpanded {
							icon, tip = iconChevronUp, "收起原文"
						}
						if renderIconButton(c, icon, tip, HeightIconMd, 14, false).Clicked() {
							a.mu.Lock()
							a.sourceExpanded = !a.sourceExpanded
							a.mu.Unlock()
							a.fitHeight()
						}
					}
					if renderIconButton(c, iconCopy, "复制原文", HeightIconMd, 14, false).Clicked() {
						a.copyToClipboard(s.source)
					}
					if renderIconButton(c, iconSpeak, "朗读原文", HeightIconMd, 14, false).Clicked() {
						PlaySpeechLang(s.source, a.speechLang(), false)
					}
				})
			})
		})
	})
}

func primarySpeechTarget(source, target string) (text, lang, tip string) {
	srcLang := detectLocal(source)
	if srcLang == "" && looksEnglish(source) {
		srcLang = "en"
	}
	if !strings.HasPrefix(srcLang, "zh") && strings.TrimSpace(source) != "" {
		return source, srcLang, "朗读原文"
	}
	tgtLang := detectLocal(target)
	if tgtLang == "" && looksEnglish(target) {
		tgtLang = "en"
	}
	if strings.HasPrefix(tgtLang, "en") {
		return target, tgtLang, "朗读英文译文"
	}
	if strings.TrimSpace(target) != "" {
		return target, tgtLang, "朗读译文"
	}
	return source, srcLang, "朗读原文"
}

var (
	englishWords = map[string]bool{
		"the": true, "and": true, "of": true, "to": true, "in": true, "that": true,
		"this": true, "with": true, "for": true, "on": true, "as": true, "at": true,
		"by": true, "not": true, "but": true, "or": true, "from": true, "they": true,
		"you": true, "was": true, "were": true, "are": true, "have": true,
		"has": true, "had": true, "will": true, "would": true, "should": true,
		"could": true, "what": true, "when": true, "where": true, "who": true,
		"why": true, "how": true, "hello": true, "world": true, "yes": true,
		"there": true, "their": true, "which": true, "been": true, "than": true,
	}
	foreignWords = map[string]bool{
		"het": true, "een": true, "van": true, "niet": true, "voor": true,
		"zijn": true, "maar": true, "ook": true, "nog": true, "wel": true,
		"le": true, "les": true, "des": true, "une": true, "est": true,
		"pas": true, "pour": true, "dans": true, "sur": true, "avec": true,
		"mais": true, "vous": true, "nous": true, "bonjour": true,
		"der": true, "die": true, "das": true, "und": true, "ist": true,
		"nicht": true, "mit": true, "für": true, "auf": true, "sich": true,
		"ein": true, "eine": true, "dass": true, "wie": true,
		"el": true, "los": true, "las": true, "una": true, "es": true,
		"por": true, "para": true, "com": true, "não": true, "que": true,
		"il": true, "lo": true, "gli": true, "che": true, "per": true,
		"och": true, "att": true, "som": true, "med": true, "är": true,
		"og": true, "ikke": true, "på": true, "av": true,
		"jest": true, "nie": true, "się": true, "które": true, "ve": true,
		"bir": true, "için": true, "ile": true,
	}
)

func looksEnglish(s string) bool {
	if !isPlainLatin(s) {
		return false
	}
	english, foreign := 0, 0
	for _, w := range strings.Fields(strings.ToLower(s)) {
		w = strings.Trim(w, ".,;:!?\"'()[]")
		if englishWords[w] {
			english++
		}
		if foreignWords[w] {
			foreign++
		}
	}
	return english > foreign && english > 0
}

func isPlainLatin(s string) bool {
	letters := 0
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			letters++
		case r > 0x7F:
			return false
		}
	}
	return letters > 0
}

func (a *app) renderNotesSummary(c *ui.Context, t *ui.Theme, cfg Config, s snapshot) {
	if !cfg.ShowNotes || len(s.notes) == 0 || !s.notesExpanded {
		return
	}
	renderNotesBlock(c, t, s, a)
}

func (a *app) renderPopupFooter(c *ui.Context, t *ui.Theme, cfg Config, s snapshot) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
		if s.mode != modeTranslate && s.mode != modeInput && s.mode != modeSelectionPreview {
			ui.Icon(c, iconSparkles).Size(12, 12).TextColor(t.Accent)
			model := cfg.AIModel
			if act := cfg.GetAction(s.activeActionID); act != nil && act.Model != "" {
				model = act.Model
			}
			ui.Text(c, model).FontSize(10).TextColor(t.TextMuted).Grow(1).SingleLine().Ellipsis("…").Tooltip(model)
		} else {
			a.renderEngineMenu(c, cfg, s)
			renderEngineCaptionWithAIState(c, t, s, cfg)
		}
		ui.Spacer(c).Grow(1)
		if renderIconButton(c, iconExpand, "全屏 / 恢复窗口", 28, 13, false).Clicked() && a.win != nil {
			a.win.ToggleFullScreen()
		}
		if renderIconButton(c, iconRefresh, "重新翻译", 28, 13, false).Clicked() && s.source != "" {
			a.rerunCurrentModeFresh(s.source)
		}
		if renderIconButton(c, iconMore, "翻译设置", 28, 15, false).Clicked() {
			a.page = "translate"
			a.showSettings()
		}
	})
}

func modelShortName(id string) string {
	name := strings.TrimSpace(id)
	if i := strings.LastIndex(name, "/"); i >= 0 && i < len(name)-1 {
		name = name[i+1:]
	}
	if name == "" {
		return "AI"
	}
	return TruncateEllipsis(name, 24)
}

func (a *app) renderEngineMenu(c *ui.Context, cfg Config, s snapshot) {
	raceLabel := "微软 / 谷歌 / 腾讯 · 竞速"

	label := "Microsoft Translator"
	switch cfg.EngineMode {
	case "google":
		label = "Google Translate"
	case "tencent":
		label = "腾讯翻译"
	case "race":
		label = raceLabel
	}
	if isAIResultEngine(s.engine) {
		label = s.engine + " · " + modelShortName(cfg.AIModel)
	}
	options := []struct{ label, mode string }{
		{"Microsoft Translator", "microsoft"},
		{"Google Translate", "google"},
		{"腾讯翻译", "tencent"},
		{raceLabel, "race"},
	}
	ui.MenuButton(c, label, func(m *ui.Menu) {
		for _, option := range options {
			if m.Item(option.label).Checked(cfg.EngineMode == option.mode).Chosen() {
				a.store.Update(func(c *Config) { c.EngineMode = option.mode })
				if s.source != "" && !s.busy {
					a.translate(s.source)
				}
			}
		}
	}).Height(28).FontSize(11).Shrink(0).Label("翻译引擎")
}

func (a *app) saveTranslationToVocab(source, target string) {
	if strings.TrimSpace(source) == "" || strings.TrimSpace(target) == "" {
		return
	}
	go func() {
		defer recoverLog("popup_translation.go:saveTranslationToVocab")
		GetVocabularyStore().Add(VocabularyEntry{Term: TruncateEllipsis(source, 80), Context: source, ContextTranslation: target, Source: "translate"})
		a.mu.Lock()
		a.actionSavedMsg = "原文与译文已保存到本地生词本 ✓"
		a.mu.Unlock()
		if a.win != nil {
			a.win.Update(func() {})
		}
	}()
}
