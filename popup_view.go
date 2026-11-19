package main

import (
	"log"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

func renderIconButton(c *ui.Context, icon *ui.SVG, tip string, size, iconSize float32, active bool) ui.Element {
	t := c.Theme()
	at := appTheme(t)
	b := ui.Button(c, "").Size(size, size).Padding(0, 0).Radius(RadiusSM).Center().Tooltip(tip).Label(tip)
	bg := ui.Color{}
	fg := at.TextSec
	if icon == iconCheck {
		fg = at.Success
	}
	if active {
		fg = at.Accent
	}
	if b.Hovered() {
		bg = at.SurfaceHover
		if icon != iconCheck && !active {
			fg = at.Text
		}
	}
	if b.Pressed() {
		bg = at.SurfacePress
	}
	b.Background(bg).Border(0, ui.Transparent).AccessSelected(active)
	b.Children(func() {
		ui.Icon(c, icon).Size(iconSize, iconSize).TextColor(fg)
	})
	return b
}

func (a *app) popupView(c *ui.Context) {
	cfg := a.store.Get()
	applyTheme(c, cfg.ThemeMode, c.Theme().Dark, cfg.AccentTheme)
	t := c.Theme()
	s := a.snap()

	if c.Shortcut(0, ui.KeyEscape) && a.win != nil {
		stopAudio()
		a.stopScreenshotOCR()
		a.win.Hide()
	}

	if c.Shortcut(ui.Ctrl, ui.Key1) || c.Shortcut(ui.Alt, ui.Key1) {
		a.switchPopupMode(0)
	} else if c.Shortcut(ui.Ctrl, ui.Key2) || c.Shortcut(ui.Alt, ui.Key2) {
		a.switchPopupMode(1)
	} else if c.Shortcut(ui.Ctrl, ui.Key3) || c.Shortcut(ui.Alt, ui.Key3) {
		a.switchPopupMode(2)
	}

	if c.Shortcut(0, ui.KeyF5) || c.Shortcut(ui.Ctrl, ui.KeyR) {
		if s.source != "" {
			a.rerunCurrentModeFresh(s.source)
		}
	}

	if c.Shortcut(ui.Ctrl, ui.KeyS) {
		a.handleQuickSave(s)
	}
	if c.Shortcut(ui.Ctrl|ui.Shift, ui.KeyC) {
		if text := primaryCopyText(s); text != "" {
			a.copyToClipboard(text)
		}
	}
	if c.Shortcut(ui.Ctrl, ui.KeyL) {
		if s.mode == modeGrammar {
			if text := grammarAnalysisText(s.source); text != "" {
				a.play(text, false)
			}
		} else if text, lang, _ := primarySpeechTarget(s.source, s.target); text != "" {
			PlaySpeechLang(text, lang, false)
		}
	}
	if c.Shortcut(ui.Ctrl, ui.KeyP) {
		a.togglePopupPin(cfg)
	}

	var contentEl, scrollEl ui.Element
	rootEl := ui.Column(c).FillWidth().FillHeight().Padding(8, 12, 8, 12).Gap(6)
	rootEl.Children(func() {
		a.renderPopupHeader(c, t, cfg, s)

		ui.Divider(c).FillWidth()

		scrollEl = ui.Scroll(c).Grow(1).FillWidth()
		scrollEl.Children(func() {
			contentEl = ui.Column(c).FillWidth().Gap(6).Padding(0, 14, 0, 0)
			contentEl.Children(func() {
				if s.notice != "" {
					ui.Box(c).FillWidth().Padding(8, 10).Radius(6).
						Background(noticeBg(t)).Border(1, noticeBorder(t)).Children(func() {
						ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(6).Children(func() {
							ui.Icon(c, iconInfo).Size(13, 13).TextColor(noticeText(t))
							ui.Text(c, s.notice).FontSize(12).TextColor(noticeText(t)).Wrap().Grow(1)
						})
					})
				}

				if s.echoNote != "" {
					ui.Box(c).FillWidth().Padding(8, 10).Radius(6).
						Background(noticeBg(t)).Border(1, noticeBorder(t)).Children(func() {
						ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(6).Children(func() {
							ui.Icon(c, iconInfo).Size(13, 13).TextColor(noticeText(t))
							ui.Text(c, s.echoNote).FontSize(12).TextColor(noticeText(t)).Wrap().Grow(1)
						})
					})
				}

				if s.saveErr != "" {
					ui.Box(c).FillWidth().Padding(6, 10).Radius(6).
						Background(dangerSoft(t)).Border(1, dangerLine(t)).Children(func() {
						ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
							ui.Icon(c, iconInfo).Size(13, 13).TextColor(t.Danger)
							ui.Text(c, s.saveErr).FontSize(12).TextColor(t.Danger).Wrap().Grow(1)
						})
					})
				}

				if s.errText != "" {
					ui.Box(c).FillWidth().Padding(Space2, Space3).Radius(RadiusTrack).
						Background(dangerSoft(t)).
						Border(1, dangerLine(t)).Children(func() {
						ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
							ui.Icon(c, iconInfo).Size(13, 13).TextColor(t.Danger)
							ui.Text(c, s.errText).FontSize(12).TextColor(t.Danger).Wrap().Grow(1)
							if s.source != "" && s.mode == modeTranslate {
								if ui.Button(c, "重试").Height(22).Padding(0, 8).FontSize(10).Radius(4).Clicked() {
									a.rerunCurrentModeFresh(s.source)
								}
							}
						})
					})
				}

				switch s.mode {
				case modeGrammar:
					switch classifyAIView(s) {
					case aiPassStructured:
						text := grammarAnalysisText(s.source)
						a.renderGrammarVisualCard(c, t, text, s.grammarResult, s.grammarSaved, func() { a.saveGrammarToNotebook(s.grammarResult) })
					case aiPassIdle:
						a.renderGrammarEmptyState(c, t, s)
					default:
						renderAIResultState(c, t, s)
					}

				case modeDictionary:
					switch classifyAIView(s) {
					case aiPassStructured:
						renderDictionaryCard(c, t, s.source, s.dictResult, s.savedDictTerm, func() { a.saveDictToVocab(s.dictResult) })
					case aiPassIdle:
						a.renderDictionaryEmptyState(c, t, s)
					default:
						renderAIResultState(c, t, s)
					}

				case modeWriting:
					switch classifyAIView(s) {
					case aiPassStructured:
						renderImproveWritingCard(c, t, s.source, s.writingResult, s.writingSaved, func(txt string) { a.copyToClipboard(txt) }, func() { a.saveWritingToNotebook(s.writingResult) })
					case aiPassIdle:
						a.renderWritingEmptyState(c, t, s)
					default:
						renderAIResultState(c, t, s)
					}

				default:
					a.renderTranslationContent(c, t, cfg, s)
				}
			})
		})

		ui.Divider(c).FillWidth()

		a.renderPopupFooter(c, t, cfg, s)
	})

	a.scheduleAutofit(scrollEl, contentEl)
}

func (a *app) scheduleAutofit(scrollEl, contentEl ui.Element) {
	viewportH := scrollEl.Bounds().H
	contentH := contentEl.LayoutBounds().H
	if a.win == nil {
		return
	}
	w, h := a.win.Size()

	a.mu.Lock()
	settled := h == a.lastAutofitWinH
	a.lastAutofitWinH = h
	a.mu.Unlock()

	if !settled {
		win := a.win
		win.Update(func() {})
		return
	}

	delta, ok := autofitHeight(viewportH, contentH)
	a.mu.Lock()
	a.autofit.confirm(h)
	delta, ok = a.autofit.decide(delta, ok, h, contentH)
	a.mu.Unlock()
	if !ok {
		return
	}

	log.Printf("fastDictionary: autofit viewport=%.1fx%.1f content=%.1f delta=%.1f winSize=%dx%d",
		scrollEl.Bounds().W, viewportH, contentH, delta, w, h)

	win := a.win
	win.Update(func() { a.applyPopupHeight(delta) })
}

func primaryCopyText(s snapshot) string {
	switch s.mode {
	case modeWriting:
		if s.writingResult != nil && s.writingResult.Improved != "" {
			return s.writingResult.Improved
		}
	case modeGrammar:
		if s.grammarResult != nil && s.grammarResult.Translation != "" {
			return s.grammarResult.Translation
		}
	}
	return s.target
}

func renderNotesBlock(c *ui.Context, t *ui.Theme, s snapshot, a *app) {
	at := appTheme(t)
	ui.Box(c).FillWidth().Padding(12, 14).Radius(8).
		Background(at.Surface).Border(1, at.BorderSubtle).Children(func() {
		ui.Column(c).FillWidth().Gap(8).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, iconBookmark).Size(15, 15).TextColor(t.TextMuted)
					ui.Text(c, "猜你想存").FontSize(13).FontWeight(600).TextColor(t.Text)
				})
				ui.Spacer(c).Grow(1)
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "显示猜你想存").FontSize(11).TextColor(t.TextMuted)
					enabled := a.store.Get().ShowNotes
					ui.Switch(c, &enabled).Height(18).OnChange(func() {
						a.store.Update(func(c *Config) { c.ShowNotes = enabled })
						if a.win != nil {
							a.win.Update(func() {})
						}
					})
					btnCollapse := ui.Button(c, "收起").Height(20).Padding(0, 6).FontSize(11).Radius(4)
					btnCollapse.Background(ui.Transparent).Border(0, ui.Transparent).TextColor(t.TextMuted)
					if btnCollapse.Clicked() {
						a.mu.Lock()
						a.notesExpanded = false
						a.mu.Unlock()
						a.fitHeight()
					}
				})
			})

			ui.Text(c, "选择要存入生词本的词，将来随时复习").FontSize(11).TextColor(t.TextMuted)

			if s.notesBusy && len(s.notes) == 0 {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Padding(8, 0).Children(func() {
					ui.Spinner(c).Size(12, 12)
					ui.Text(c, "AI 正在提取生词...").FontSize(12).TextColor(t.TextMuted)
				})
			}

			for idx := range s.notes {
				i := idx
				item := s.notes[idx]
				checked := item.Checked
				ui.Box(c).FillWidth().Padding(8, 10).Radius(6).
					Background(t.SurfaceHover).Children(func() {
					ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(8).Children(func() {
						ui.Checkbox(c, &checked, "").OnChange(func() {
							a.setNoteChecked(i, item.Word, checked)
						})
						ui.Column(c).FillWidth().Gap(2).Children(func() {
							ui.Text(c, item.Word).FontSize(13).FontWeight(600).TextColor(t.Text)
							desc := item.Pos + " " + item.Meaning
							ui.Text(c, desc).FontSize(12).TextColor(t.TextMuted).Wrap()
						})
						btnDismiss := ui.Button(c, "不再推荐").Height(20).Padding(0, 6).FontSize(10).Radius(4)
						btnDismiss.Background(ui.Transparent).Border(0, ui.Transparent).TextColor(t.TextMuted)
						if btnDismiss.Clicked() {
							a.dismissNoteWord(item.Word)
						}
					})
				})
			}

			if len(s.notes) > 0 {
				ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
					if s.notesSaved {
						ui.Row(c).Height(HeightCompact).Gap(Space1).AlignItems(ui.Center).Children(func() {
							ui.Icon(c, iconCheck).Size(12, 12).TextColor(appTheme(t).TextMuted)
							ui.Text(c, "已保存").FontSize(11).FontWeight(500).TextColor(appTheme(t).TextMuted)
						})
					} else {
						btnSave := ui.Button(c, "").Height(HeightCompact).Padding(2, Space3).Radius(RadiusSM).Tooltip("保存所选生词到生词本 (Ctrl+S)")
						saveFG := saveButtonStyle(btnSave, t)
						btnSave.Children(func() {
							ui.Row(c).Gap(Space1).AlignItems(ui.Center).Children(func() {
								ui.Icon(c, iconBookmark).Size(12, 12).TextColor(saveFG)
								ui.Text(c, "保存所选生词").FontSize(11).FontWeight(600)
							})
						})
						if btnSave.Clicked() {
							a.saveNotesToVocab(s)
						}
					}
				})
			}
		})
	})
}

func renderWordSaveButton(c *ui.Context, t *ui.Theme, s snapshot, a *app) {
	isSaved := s.savedDictTerm != "" && strings.EqualFold(strings.TrimSpace(s.savedDictTerm), strings.TrimSpace(s.source))
	if isSaved {
		ui.Row(c).Height(HeightCompact).Gap(Space1).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconCheck).Size(12, 12).TextColor(appTheme(t).TextMuted)
			ui.Text(c, "已保存").FontSize(11).FontWeight(500).TextColor(appTheme(t).TextMuted)
		})
		return
	}
	save := ui.Button(c, "").Height(HeightCompact).Padding(2, Space3).Radius(RadiusSM).
		Tooltip("将该单词保存到生词本 (Ctrl+S)")
	saveFG := saveButtonStyle(save, t)
	save.Children(func() {
		ui.Row(c).Gap(Space1).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconBookmark).Size(12, 12).TextColor(saveFG)
			ui.Text(c, "保存生词").FontSize(11).FontWeight(600)
		})
	})
	if save.Clicked() {
		a.saveCurrentWordToVocab(s)
	}
}

func dictBlock(c *ui.Context, t *ui.Theme, s snapshot, a *app) {
	ui.Divider(c).FillWidth()
	hasPhones := (s.dict != nil && (s.dict.UKPhone != "" || s.dict.USPhone != "")) || (s.gdict != nil && s.gdict.Pron != "")
	if hasPhones || s.isWord {
		ui.Row(c).FillWidth().Gap(14).AlignItems(ui.Center).Children(func() {
			if s.dict != nil && s.dict.UKPhone != "" {
				ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
					btnUK := renderIconButton(c, iconSpeak, "英式发音", 28, 14, false)
					if btnUK.Clicked() && a != nil {
						a.play(s.source, false)
					}
					ui.Text(c, "/"+s.dict.UKPhone+"/").Font(fontIPA).FontSize(13).TextColor(appTheme(t).TextMuted).SingleLine()
				})
			}
			if s.dict != nil && s.dict.USPhone != "" {
				ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
					btnUS := renderIconButton(c, iconSpeak, "美式发音", 28, 14, false)
					if btnUS.Clicked() && a != nil {
						a.play(s.source, true)
					}
					ui.Text(c, "/"+s.dict.USPhone+"/").Font(fontIPA).FontSize(13).TextColor(appTheme(t).TextMuted).SingleLine()
				})
			} else if s.gdict != nil && s.gdict.Pron != "" {
				ui.Text(c, "/"+s.gdict.Pron+"/").Font(fontIPA).FontSize(13).TextColor(t.Accent)
			}

			ui.Spacer(c).Grow(1)

			if a != nil {
				renderWordSaveButton(c, t, s, a)
			}
		})
	}
	if s.dict != nil && len(s.dict.Explains) > 0 {
		for _, ex := range s.dict.Explains {
			ui.Text(c, ex).FontSize(14).TextColor(t.Text).Wrap().LineHeight(1.5)
		}
	} else if s.gdict != nil {
		for _, e := range s.gdict.Entries {
			ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Start).Children(func() {
				ui.Box(c).Shrink(0).Padding(2, 6).Radius(4).
					Background(t.SurfaceHover).Border(1, t.Border).Children(func() {
					ui.Text(c, e.POS).FontSize(11).FontWeight(600).TextColor(t.Accent).SingleLine()
				})
				ui.Text(c, e.Explain).FontSize(14).TextColor(t.Text).Grow(1).Wrap().LineHeight(1.5)
			})
		}
	}
}

func (a *app) renderEmptyActionStart(c *ui.Context, t *ui.Theme, s snapshot, actionID, tip string) {
	ui.Row(c).FillWidth().Padding(6, 4).AlignItems(ui.Center).Children(func() {
		ui.Text(c, tip).FontSize(12).TextColor(t.TextMuted)
		ui.Spacer(c).Grow(1)
		btnStart := ui.Button(c, "").Height(HeightIconSm).Padding(2, 10).Radius(RadiusSM).Background(t.Accent).TextColor(t.AccentText)
		btnStart.Children(func() {
			ui.Text(c, "立即开始").FontSize(11).FontWeight(600)
		})
		if btnStart.Clicked() {
			txt := s.source
			if txt == "" {
				txt = s.target
			}
			if txt != "" {
				a.runAction(actionID, txt)
			}
		}
	})
}

func sameLang(a, b string) bool {
	if a == b {
		return true
	}
	pa, pb := langPrefix(a), langPrefix(b)
	return pa == pb
}

func langPrefix(tag string) string {
	if i := strings.IndexAny(tag, "-_"); i > 0 {
		return strings.ToLower(tag[:i])
	}
	return strings.ToLower(tag)
}

func renderAIResultState(c *ui.Context, t *ui.Theme, s snapshot) {
	switch classifyAIResult(s) {
	case aiPassIdle, aiPassStructured:
		return
	case aiPassStreaming:
		at := appTheme(t)
		msg := aiBusyMessage(s.mode)
		ui.Column(c).FillWidth().AlignItems(ui.Center).Gap(10).Padding(32, 0).Children(func() {
			ui.Spinner(c).Size(20, 20).TextColor(at.Accent)
			ui.Text(c, msg).FontSize(13).TextColor(at.TextSec)
			ui.Progress(c, -1).Width(160).Height(3).TextColor(at.Accent)
		})
		return
	case aiPassProse:
		renderAiMarkdown(c, s.aiText, false)
		return
	case aiPassParseFailed:
		ui.Box(c).FillWidth().Padding(14, 16).Radius(8).
			Background(t.Surface).Border(1, t.Border).Children(func() {
			ui.Column(c).FillWidth().Gap(6).Children(func() {
				ui.Text(c, "这次没能解析成结构化卡片").FontSize(13).FontWeight(600).TextColor(t.Text)
				ui.Text(c, "模型返回的内容不完整或格式不对，原始输出不适合直接阅读。点右下角的「重新翻译」可以再生成一次。").
					FontSize(12).TextColor(t.TextMuted).Wrap()
			})
		})
	}
}

func aiBusyMessage(m popupMode) string {
	switch m {
	case modeDictionary:
		return "正在生成词条与例句…"
	case modeGrammar:
		return "正在拆解长难句…"
	case modeWriting:
		return "正在批改句子…"
	}
	return "AI 正在分析…"
}

func looksLikeStructuredPayload(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	switch trimmed[0] {
	case '{', '[', '`':
		return true
	}
	head := trimmed
	if len(head) > 200 {
		head = head[:200]
	}
	return strings.Contains(head, "{\"")
}

func renderAiMarkdown(c *ui.Context, text string, busy bool) {
	if busy {
		markdown.Stream(c, text, nil)
	} else {
		markdown.View(c, text, nil)
	}
}

func renderPopupEmptyState(c *ui.Context, t *ui.Theme, icon *ui.SVG, title, desc string) {
	at := appTheme(t)
	ui.Column(c).FillWidth().Padding(Space6, Space4).Gap(Space2).Center().Children(func() {
		ui.Box(c).Size(40, 40).Radius(RadiusTrack).Center().
			Background(at.TintSurface).Border(1, at.TintBorder).Children(func() {
			ui.Icon(c, icon).Size(20, 20).TextColor(at.Accent)
		})
		ui.Text(c, title).Font(fontUI).FontSize(FontSection).
			FontWeight(600).TextColor(at.Text).Balance().Center()
		if desc != "" {
			ui.Text(c, desc).Font(fontUI).FontSize(FontHelper).
				TextColor(at.TextMuted).MaxWidth(280).LineHeight(1.5).Balance().Center()
		}
	})
}

func (a *app) renderGrammarEmptyState(c *ui.Context, t *ui.Theme, s snapshot) {
	if s.source != "" {
		ui.Box(c).FillWidth().Padding(14, 16).Radius(8).
			Background(t.Surface).Border(1, t.Border).Children(func() {
			ui.Column(c).FillWidth().Gap(8).Children(func() {
				ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
					ui.Text(c, "长难句拆解待就绪").FontSize(13).FontWeight(600).TextColor(t.Text)
					ui.Spacer(c).Grow(1)
					btnStart := ui.Button(c, "开始拆解长难句").Height(HeightCompact).Padding(2, 12).Radius(RadiusSM).
						Background(t.Accent).TextColor(t.AccentText)
					if btnStart.Clicked() {
						a.runAction("default-sentence-analysis", s.source)
					}
				})
				ui.Text(c, "将自动识别句子主干（主谓宾）、从句类型微标与倒装修饰关系。").FontSize(12).TextColor(t.TextMuted)
			})
		})
		return
	}
	ui.Box(c).FillWidth().Padding(28, 16).Radius(8).
		Background(t.Surface).Border(1, t.Border).Children(func() {
		ui.Column(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
			renderPopupEmptyState(c, t, iconSearch, "长难句拆解", "在任意界面选中英文长句后按快捷键，AI 将自动分析句子主干与从属修饰关系。")
			btnInput := ui.Button(c, "键盘输入长句").Height(HeightCompact).Padding(2, 12).Radius(RadiusSM).Background(t.SurfaceHover)
			if btnInput.Clicked() {
				a.mu.Lock()
				a.inputTargetMode = modeGrammar
				a.mu.Unlock()
				a.switchMode(modeInput)
			}
		})
	})
}

func (a *app) renderWritingEmptyState(c *ui.Context, t *ui.Theme, s snapshot) {
	if s.source != "" {
		ui.Box(c).FillWidth().Padding(14, 16).Radius(8).
			Background(t.Surface).Border(1, t.Border).Children(func() {
			ui.Column(c).FillWidth().Gap(8).Children(func() {
				ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
					ui.Text(c, "写作批改待就绪").FontSize(13).FontWeight(600).TextColor(t.Text)
					ui.Spacer(c).Grow(1)
					btnStart := ui.Button(c, "开始批改润色").Height(HeightCompact).Padding(2, 12).Radius(RadiusSM).
						Background(t.Accent).TextColor(t.AccentText)
					if btnStart.Clicked() {
						a.runAction("default-improve-writing", s.source)
					}
				})
				ui.Text(c, "将进行场景得体性润色、标出逐项语法错误并给出修改建议。").FontSize(12).TextColor(t.TextMuted)
			})
		})
		return
	}
	ui.Box(c).FillWidth().Padding(28, 16).Radius(8).
		Background(t.Surface).Border(1, t.Border).Children(func() {
		ui.Column(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
			renderPopupEmptyState(c, t, iconPolish, "AI 写作批改与润色", "选中草拟的英文句子或段落，AI 将进行语法纠错与场景得体性润色。")
			btnInput := ui.Button(c, "键盘输入句子").Height(HeightCompact).Padding(2, 12).Radius(RadiusSM).Background(t.SurfaceHover)
			if btnInput.Clicked() {
				a.mu.Lock()
				a.inputTargetMode = modeWriting
				a.mu.Unlock()
				a.switchMode(modeInput)
			}
		})
	})
}

func (a *app) renderDictionaryEmptyState(c *ui.Context, t *ui.Theme, s snapshot) {
	ui.Box(c).FillWidth().Padding(28, 16).Radius(8).
		Background(t.Surface).Border(1, t.Border).Children(func() {
		ui.Column(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
			renderPopupEmptyState(c, t, iconDict, "词典精读", "选中文中陌生单词，即可查看权威释义、音标发音与生词卡。")
			btnInput := ui.Button(c, "键盘输入查词").Height(HeightCompact).Padding(2, 12).Radius(RadiusSM).Background(t.SurfaceHover)
			if btnInput.Clicked() {
				a.mu.Lock()
				a.inputTargetMode = modeDictionary
				a.mu.Unlock()
				a.switchMode(modeInput)
			}
		})
	})
}

func (a *app) renderTranslateEmptyState(c *ui.Context, t *ui.Theme) {
	ui.Box(c).FillWidth().Padding(28, 16).Radius(8).
		Background(t.Surface).Border(1, t.Border).Children(func() {
		ui.Column(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
			renderPopupEmptyState(c, t, iconTranslate, "划词翻译就绪", "选中文本后按划词快捷键（默认 Alt+D）即可自动翻译；也可点击下方按钮直接输入。")
			btnInput := ui.Button(c, "键盘输入翻译").Height(HeightCompact).Padding(2, 12).Radius(RadiusSM).
				Background(t.Accent).TextColor(t.AccentText)
			if btnInput.Clicked() {
				a.mu.Lock()
				a.inputTargetMode = modeTranslate
				a.mu.Unlock()
				a.switchMode(modeInput)
			}
			ui.Text(c, "常用快捷键：Ctrl+1 翻译 · Ctrl+2 长难句 · Ctrl+3 写作 · Ctrl+S 保存 · Esc 退出").
				FontSize(11).TextColor(t.TextMuted)
		})
	})
}
