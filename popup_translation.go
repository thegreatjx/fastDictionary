package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

// renderPopupHeader keeps the four reading tools in the same place as the
// original popup. Only narrow windows fold them into a menu; moving them
// below the translation made the most common navigation needlessly indirect.
func (a *app) renderPopupHeader(c *ui.Context, t *ui.Theme, cfg Config, s snapshot) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).DragWindow().Children(func() {
		if a.popupW > 0 && a.popupW < 440 {
			a.renderCompactModeMenu(c, s)
		} else {
			a.renderPopupModeTabs(c, s)
		}
		ui.Spacer(c).Grow(1)
		ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
			if s.mode == modeTranslate || s.mode == modeInput || s.mode == modeSelectionPreview {
				targetLabel := langLabels[cfg.TargetLang]
				if targetLabel == "" {
					targetLabel = "简体中文"
				}
				ui.MenuButton(c, targetLabel, func(m *ui.Menu) {
					for _, label := range langList() {
						if m.Item(label).Checked(label == targetLabel).Chosen() {
							a.store.Update(func(c *Config) { c.TargetLang = langFromLabel(label) })
							if s.source != "" {
								a.translate(s.source)
							}
						}
					}
				}).Height(30).FontSize(12).Shrink(0)
			}
			pinIcon, pinTip := iconPin, "固定窗口（失焦不关闭）"
			if s.pinned {
				pinIcon, pinTip = iconPinned, "取消固定窗口"
			}
			if renderIconButton(c, pinIcon, pinTip, 30, 15, s.pinned).Clicked() {
				a.mu.Lock()
				a.pinned = !a.pinned
				pinned := a.pinned
				a.mu.Unlock()
				if a.win != nil {
					a.win.SetAlwaysOnTop(pinned || cfg.AlwaysOnTop)
					a.win.Update(func() {})
				}
			}
			if renderIconButton(c, iconClose, "关闭（Esc）", 30, 14, false).Clicked() && a.win != nil {
				stopAudio()
				a.stopScreenshotOCR()
				a.win.Hide()
			}
		})
	})
}

var popupModeLabels = [...]string{"翻译", "长难句", "写作"}

// popupModeIndex maps a mode onto its tab. 词典 is not a tab any more: a word's
// full card (phonetic, examples, memory tips) belongs next to the entry that
// keeps it, so it lives in 生词本, and the popup keeps the three views a reader
// actually reads in.
func popupModeIndex(mode, target popupMode) int {
	if mode == modeInput {
		mode = target
	}
	switch mode {
	case modeGrammar:
		return 1
	case modeWriting:
		return 2
	default:
		return 0
	}
}

func (a *app) switchPopupMode(index int) {
	modes := [...]popupMode{modeTranslate, modeGrammar, modeWriting}
	if index >= 0 && index < len(modes) {
		a.mu.Lock()
		a.inputTargetMode = modes[index]
		a.inputText = ""
		a.mu.Unlock()
		a.switchMode(modes[index])
	}
}

func (a *app) renderPopupModeTabs(c *ui.Context, s snapshot) {
	modeIdx := popupModeIndex(s.mode, s.inputTargetMode)
	ui.Segmented(c, &modeIdx, popupModeLabels[:]...).Height(30).FontSize(12).Shrink(0).
		Label("阅读工具").OnChange(func() { a.switchPopupMode(modeIdx) })
}

func (a *app) renderCompactModeMenu(c *ui.Context, s snapshot) {
	label := popupModeLabels[popupModeIndex(s.mode, s.inputTargetMode)]
	ui.MenuButton(c, label, func(m *ui.Menu) {
		for i, name := range popupModeLabels {
			if m.Item(name).Checked(i == popupModeIndex(s.mode, s.inputTargetMode)).Chosen() {
				a.switchPopupMode(i)
			}
		}
	}).Height(30).FontSize(12).Shrink(0).Label("切换阅读工具")
}

// renderTranslationContent keeps the source quiet and gives the result the
// strongest type on the surface. Copy, speech and saving sit beside the text
// they act on instead of competing with the engine selector in the footer.
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
				if text := CleanSelectionText(k.Text()); text != "" {
					targetMode := a.inputTargetMode
					if targetMode == 0 {
						targetMode = modeTranslate
					}
					a.mu.Lock()
					a.source = text
					a.inputText = ""
					a.mu.Unlock()
					a.switchMode(targetMode)
				}
				return true
			}
			return false
		})
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
		ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(8, 2).Children(func() {
			ui.Spinner(c).Size(15, 15)
			ui.Text(c, "正在翻译…").FontSize(13).TextColor(t.TextMuted)
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
			ui.Column(c).FillWidth().Gap(8).Children(func() {
				// The result carries the strongest type on the surface. A word
				// gets a larger, heavier face than a sentence: it is a headword,
				// not prose.
				fontSize := float32(15)
				fontWeight := 400
				lineHeight := float32(1.55)
				if s.isWord {
					fontSize = 17
					fontWeight = 600
					lineHeight = 1.4
				}
				ui.Text(c, s.target).FontSize(fontSize).FontWeight(fontWeight).
					TextColor(t.Text).Wrap().LineHeight(lineHeight)
				ui.Row(c).FillWidth().Gap(2).AlignItems(ui.Center).Children(func() {
					copyTip := "复制译文"
					if s.copiedMsg != "" {
						copyTip = "译文已复制"
					}
					if renderIconButton(c, iconCopy, copyTip, 28, 14, false).Clicked() {
						a.copyToClipboard(s.target)
					}
					// Pronunciation: for Chinese users, always pronounce the English/foreign language (never recite Chinese translation)
					spText, spLang, spTip := primarySpeechTarget(s.source, s.target)
					if spText != "" {
						if renderIconButton(c, iconSpeak, spTip, 28, 14, false).Clicked() {
							PlaySpeechLang(spText, spLang, false)
						}
					}
					renderAIRefineControls(c, t, a, s)
					// The spacer pushes engine caption and notes controls to the right.
					ui.Spacer(c).Grow(1)

					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						renderEngineCaptionWithAIState(c, t, s, cfg)

						// 猜你想存：靠右展示。AI提取时显示原生 Spinner 圈，提取就绪后显示带文字的卡片展开/收起按钮
						if cfg.ShowNotes && shouldExtractNotes(s.source) {
							if s.notesBusy && len(s.notes) == 0 {
								ui.Row(c).Gap(4).AlignItems(ui.Center).Padding(0, 4).Children(func() {
									ui.Spinner(c).Size(12, 12)
									ui.Text(c, "发现生词中…").FontSize(11).TextColor(t.TextMuted)
								})
							} else if len(s.notes) > 0 {
								btnText := fmt.Sprintf("猜你想存 · %d", len(s.notes))
								tip := "展开生词推荐"
								if s.notesExpanded {
									btnText = fmt.Sprintf("收起生词 · %d", len(s.notes))
									tip = "收起生词推荐"
								}
								btnNotes := ui.Button(c, "").Tooltip(tip).Height(24).Padding(0, 8).Radius(6).Center()
								bg := t.SurfaceHover
								borderCol := t.Border
								if s.notesExpanded {
									bg = withAlpha(t.Accent, 0.12)
									borderCol = withAlpha(t.Accent, 0.3)
								}
								if btnNotes.Hovered() {
									bg = withAlpha(t.Accent, 0.2)
								}
								btnNotes.Background(bg).Border(1, borderCol)
								btnNotes.Children(func() {
									ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
										ui.Icon(c, iconBookmark).Size(12, 12).TextColor(t.Accent)
										ui.Text(c, btnText).FontSize(11).FontWeight(600).TextColor(t.Accent)
									})
								})
								if btnNotes.Clicked() {
									a.mu.Lock()
									a.notesExpanded = !a.notesExpanded
									a.mu.Unlock()
									a.fitHeight()
								}
							}
						}
					})
				})
				if s.dict != nil || s.gdict != nil {
					dictBlock(c, t, s, a)
				}
			})
		}
	}
	a.renderNotesSummary(c, t, cfg, s)
}

// translationEngineCaption labels the result row with where the text came from.
//
// It used to print the raw detection tag — 检测 en — which reads as machine
// output rather than as an answer to "what language was this?". The source
// language is now spelled out (翻译自 英语), the way Grok's 翻译自 西班牙语
// does, and the engine and timing follow it because they answer the same
// question: where did the text on screen come from.
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

// langDisplayName names a detected language in the reader's language.
//
// Engines report tags in their own shape: 微软 gives "en" or "es", DeepL gives
// "EN", 腾讯 and 百度 give the bare code. A model may hand back "es-419" or
// "zh-TW", neither of which is a key in langLabels. Prefix matching keeps those
// readable, and anything still unknown is shown as-is so the caption never
// claims a language it does not know.
//
// Chinese is matched by its own rule rather than by prefix: zh-Hans and zh-Hant
// share the prefix "zh", so a prefix match would label Traditional text as
// 简体中文 — a wrong answer, not a missing one.
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

func (a *app) renderTranslationSource(c *ui.Context, t *ui.Theme, s snapshot) {
	text := s.source
	isLong := len([]rune(text)) > 110 || strings.Contains(text, "\n")
	if isLong && !s.sourceExpanded {
		text = TruncateEllipsis(text, 110)
	}
	ui.Box(c).FillWidth().Padding(8, 10).Radius(6).Background(t.SurfaceHover).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(4).Children(func() {
			ui.Text(c, text).FontSize(13).TextColor(t.TextMuted).Wrap().Grow(1).LineHeight(1.5)
			ui.Row(c).Gap(0).AlignItems(ui.Center).Children(func() {
				if isLong {
					icon, tip := iconChevronDown, "展开完整原文"
					if s.sourceExpanded {
						icon, tip = iconChevronUp, "收起原文"
					}
					if renderIconButton(c, icon, tip, 28, 14, false).Clicked() {
						a.mu.Lock()
						a.sourceExpanded = !a.sourceExpanded
						a.mu.Unlock()
						a.fitHeight()
					}
				}
				if renderIconButton(c, iconCopy, "复制原文", 28, 14, false).Clicked() {
					a.copyToClipboard(s.source)
				}
				if renderIconButton(c, iconSpeak, "朗读原文", 28, 14, false).Clicked() {
					PlaySpeechLang(s.source, a.speechLang(), false)
				}
			})
		})
	})
}

// primarySpeechTarget picks what the speaker button should read, and in which
// primarySpeechTarget picks what the speaker button should read, and in which
// language.
//
// For a Chinese reader it never recites the Chinese translation of a foreign
// sentence: hearing your own language read back is not pronunciation help. The
// language goes out with the text because the caller cannot tell which of the
// two strings it got, and passing the wrong tag is what made foreign text read
// with an English voice.
func primarySpeechTarget(source, target string) (text, lang, tip string) {
	srcLang := detectLocal(source)
	if srcLang == "" && looksEnglish(source) {
		// Plain ASCII with no accents. English is the best guess for speech,
		// but only for speech: the translator leaves the source to the engine,
		// which knows better.
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

// englishHints are the words that make plain ASCII text look English. A French
// or German sentence of the same length almost never contains them, so a short
// stopword test separates the cases that accented letters cannot.
// englishWords and their common counterparts in the Latin-script languages a
// reader is most likely to meet. Plain ASCII cannot be told apart by letters
// alone, but the words that carry a sentence can: "the/and/of/to" against
// "het/en/van/te", "le/et/de/à", "der/und/des/zu".
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
	// Words that mark some other Latin-script language. Their presence vetoes
	// an English reading even when an English-looking word also appears:
	// Dutch shares "is" with English but not "het".
	foreignWords = map[string]bool{
		// Dutch
		"het": true, "een": true, "van": true, "niet": true, "voor": true,
		"zijn": true, "maar": true, "ook": true, "nog": true, "wel": true,
		// French
		"le": true, "les": true, "des": true, "une": true, "est": true,
		"pas": true, "pour": true, "dans": true, "sur": true, "avec": true,
		"mais": true, "vous": true, "nous": true, "bonjour": true,
		// German
		"der": true, "die": true, "das": true, "und": true, "ist": true,
		"nicht": true, "mit": true, "für": true, "auf": true, "sich": true,
		"ein": true, "eine": true, "dass": true, "wie": true,
		// Spanish / Portuguese / Italian
		"el": true, "los": true, "las": true, "una": true, "es": true,
		"por": true, "para": true, "com": true, "não": true, "que": true,
		"il": true, "lo": true, "gli": true, "che": true, "per": true,
		// Scandinavian
		"och": true, "att": true, "som": true, "med": true, "är": true,
		"og": true, "ikke": true, "på": true, "av": true,
		// Polish / Czech / Turkish
		"jest": true, "nie": true, "się": true, "które": true, "ve": true,
		"bir": true, "için": true, "ile": true,
	}
)

// looksEnglish reports whether plain ASCII text is probably English.
//
// It is used only for the speech voice, where some answer beats none. The test
// is deliberately two-sided: English function words must appear, and words that
// belong to another Latin-script language must not. Counting English words
// alone was not enough — Dutch shares "is" and "we" with English, so a Dutch
// sentence scored as English and was read by an English voice.
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

// isPlainLatin reports text made only of ASCII letters, digits, spaces and
// common punctuation — English and Dutch and German-without-umlauts all look
// like this, so it is a hint and never a fact.
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

// renderPopupFooter keeps engine selection and retry visible. It describes
// what the user can do, rather than advertising the app's implementation.
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

// modelShortName trims a model id down to the part worth reading in a 28px
// tall footer button. The ids this app is configured with carry a provider
// prefix ("workbuddy-ai/deepseek-v4.1-flash"), and the footer only has room
// for the model itself.
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
	// An AI refinement replaces the translation without changing the engine
	// setting, so reading EngineMode alone left the footer saying 微软 / 谷歌 ·
	// 竞速 after the text on screen had come from the model. The menu is
	// labelled with where the current text actually came from, and still opens
	// the engine list so switching back is one click.
	// The race label lists the engines that are actually in the race. All three
	// are keyless, so the list is fixed — it is spelled out because "竞速"
	// alone does not tell the reader what is being raced.
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

// saveTranslationToVocab runs enrichment off the UI thread; Add may make a
// dictionary request before it writes the local notebook.
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
