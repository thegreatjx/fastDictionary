package main

import (
	"log"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

// renderIconButton renders a square icon button where the icon is guaranteed
// to be strictly horizontally and vertically centered with crisp feedback.
func renderIconButton(c *ui.Context, icon *ui.SVG, tip string, size, iconSize float32, active bool) ui.Element {
	t := c.Theme()
	b := ui.Button(c, "").Size(size, size).Padding(0, 0).Radius(RadiusSM).Center().Tooltip(tip).Label(tip)
	bg := ui.Color{}
	fg := t.TextMuted
	if active {
		bg = t.SurfaceHover
		fg = t.Accent
	}
	if b.Hovered() {
		bg = t.SurfaceHover
		fg = t.Text
	}
	if b.Pressed() {
		bg = t.SurfacePressed
	}
	b.Background(bg).Border(0, ui.Transparent)
	b.Children(func() {
		ui.Icon(c, icon).Size(iconSize, iconSize).TextColor(fg)
	})
	return b
}

func (a *app) popupView(c *ui.Context) {
	cfg := a.store.Get()
	// MyGo resolves "system" in its base theme. Deriving darkness from the
	// config alone forced every system-themed popup into the light palette.
	applyTheme(c, cfg.ThemeMode, c.Theme().Dark)
	t := c.Theme()
	s := a.snap()

	if c.Shortcut(0, ui.KeyEscape) && a.win != nil {
		stopAudio()
		a.stopScreenshotOCR()
		a.win.Hide()
	}

	// Mode switching shortcuts: Ctrl+1 / Alt+1 (Translate), Ctrl+2 / Alt+2 (Grammar), Ctrl+3 / Alt+3 (Writing)
	if c.Shortcut(ui.Ctrl, ui.Key1) || c.Shortcut(ui.Alt, ui.Key1) {
		a.switchPopupMode(0)
	} else if c.Shortcut(ui.Ctrl, ui.Key2) || c.Shortcut(ui.Alt, ui.Key2) {
		a.switchPopupMode(1)
	} else if c.Shortcut(ui.Ctrl, ui.Key3) || c.Shortcut(ui.Alt, ui.Key3) {
		a.switchPopupMode(2)
	}

	// Quick refresh / re-translate: F5 or Ctrl+R
	if c.Shortcut(0, ui.KeyF5) || c.Shortcut(ui.Ctrl, ui.KeyR) {
		if s.source != "" {
			a.rerunCurrentModeFresh(s.source)
		}
	}

	// Quick save to notebook: Ctrl+S
	if c.Shortcut(ui.Ctrl, ui.KeyS) {
		a.handleQuickSave(s)
	}

	var contentEl, scrollEl ui.Element
	rootEl := ui.Column(c).FillWidth().FillHeight().Padding(8, 12, 8, 12).Gap(6)
	rootEl.Children(func() {
		a.renderPopupHeader(c, t, cfg, s)

		ui.Divider(c).FillWidth()

		// ===================================================================
		// 2. Main Content Scroll View (Independent Dedicated Views)
		// ===================================================================
		scrollEl = ui.Scroll(c).Grow(1).FillWidth()
		scrollEl.Children(func() {
			// Scroll paints its bar over the container's right edge: the
			// track starts 12px in and the thumb sits 9px in, both measure
			// from the right edge (ui/paint.go scrollBars, theme scrollbar
			// width 6). Text that ran to the edge had its last character
			// under the thumb. 14px of right padding clears the whole
			// track, so the bar never covers a glyph.
			// Padding is CSS order: top, right, bottom, left.
			contentEl = ui.Column(c).FillWidth().Gap(6).Padding(0, 14, 0, 0)
			contentEl.Children(func() {
				// A notice explains why the shortcut opened something other
				// than what the user expected (no selection, for instance).
				// It used to be stored and never rendered, so the user saw an
				// empty input box with no explanation at all.
				if s.notice != "" {
					ui.Box(c).FillWidth().Padding(8, 10).Radius(6).
						Background(noticeBg(t)).Border(1, noticeBorder(t)).Children(func() {
						ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(6).Children(func() {
							ui.Icon(c, iconInfo).Size(13, 13).TextColor(noticeText(t))
							ui.Text(c, s.notice).FontSize(12).TextColor(noticeText(t)).Wrap().Grow(1)
						})
					})
				}

				// The engine echoed the source back. It answered, so this is not
				// the red failure box: a 重试 would return the same text.
				if s.echoNote != "" {
					ui.Box(c).FillWidth().Padding(8, 10).Radius(6).
						Background(noticeBg(t)).Border(1, noticeBorder(t)).Children(func() {
						ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(6).Children(func() {
							ui.Icon(c, iconInfo).Size(13, 13).TextColor(noticeText(t))
							ui.Text(c, s.echoNote).FontSize(12).TextColor(noticeText(t)).Wrap().Grow(1)
						})
					})
				}

				// A quiet confirmation that a copy actually landed with positive success feedback.
				if s.copiedMsg != "" {
					ui.Box(c).FillWidth().Padding(6, 10).Radius(6).
						Background(appTheme(t).SuccessSoft).Border(1, appTheme(t).Success.Alpha(0.25)).Children(func() {
						ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
							ui.Icon(c, iconCheckSquare).Size(13, 13).TextColor(t.Success)
							ui.Text(c, s.copiedMsg).FontSize(12).TextColor(t.Success).Wrap().Grow(1)
						})
					})
				}

				// A save that landed shows a gentle success confirmation banner.
				if s.actionSavedMsg != "" {
					ui.Box(c).FillWidth().Padding(6, 10).Radius(6).
						Background(appTheme(t).SuccessSoft).Border(1, appTheme(t).Success.Alpha(0.25)).Children(func() {
						ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
							ui.Icon(c, iconCheckSquare).Size(13, 13).TextColor(t.Success)
							ui.Text(c, s.actionSavedMsg).FontSize(12).TextColor(t.Success).Wrap().Grow(1)
						})
					})
				}

				// Show general error if present. A failure that left the card
				// empty is not useful without a way to try again; keep the
				// reason and the retry together.
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

				// The AI busy state is drawn by renderAIResultState in the body,
				// where the card will appear. It used to be drawn twice: a
				// banner here and a second spinner plus progress bar in the
				// body, so one generation showed two spinners and a bar for a
				// task with no measurable progress. The body keeps the single
				// indicator, next to the message that says what is running.

				// RENDER CLEAN HERO CONTENT BY MODE:
				switch s.mode {
				case modeGrammar:
					// Dedicated Sentence Analysis Mode!
					switch classifyAIView(s) {
					case aiPassStructured:
						// One sentence at a time: the card shows the sentence the
						// analysis actually describes, not the whole paragraph the
						// user selected.
						sent, _, _ := grammarActiveSentence(s.source, s.grammarSentIdx)
						a.renderGrammarVisualCard(c, t, sent, s.grammarResult, s.grammarSaved, func() { a.saveGrammarToVocab(s.grammarResult) })
					case aiPassIdle:
						a.renderGrammarEmptyState(c, t, s)
					default:
						renderAIResultState(c, t, s)
					}

				case modeDictionary:
					// Dedicated Learner's Dictionary Mode!
					switch classifyAIView(s) {
					case aiPassStructured:
						renderDictionaryCard(c, t, s.source, s.dictResult, s.savedDictTerm, func() { a.saveDictToVocab(s.dictResult) })
					case aiPassIdle:
						a.renderDictionaryEmptyState(c, t, s)
					default:
						renderAIResultState(c, t, s)
					}

				case modeWriting:
					// Dedicated Improve Writing Mode!
					switch classifyAIView(s) {
					case aiPassStructured:
						renderImproveWritingCard(c, t, s.source, s.writingResult, s.writingSaved, func(txt string) { a.copyToClipboard(txt) }, func() { a.saveWritingToVocab(s.writingResult) })
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

		// ===================================================================
		// 3. Bottom Bar
		// ===================================================================
		a.renderPopupFooter(c, t, cfg, s)
	})

	// The window was opened at an estimated height. Once the first frame has
	// laid the content out, shrink or grow the window so it ends where the
	// text ends instead of leaving a screenful of empty space below it.
	a.scheduleAutofit(scrollEl, contentEl)
}

// scheduleAutofit resizes the popup from the previous frame's measurements.
// Bounds are one frame behind, so the resize is posted through Window.Update:
// it runs after the current frame, and the following frame renders at the new
// size.
//
// Both measurements come from the same layout pass, so the arithmetic stays in
// layout units (DIPs). Reading the window height instead mixed physical pixels
// with DIPs: at 125% scaling the window reported 1.25x the height the layout
// had used, so every shrink request looked like a growth and the popup stayed
// at its initial height.
func (a *app) scheduleAutofit(scrollEl, contentEl ui.Element) {
	viewportH := scrollEl.Bounds().H
	contentH := contentEl.LayoutBounds().H
	if a.win == nil {
		return
	}
	w, h := a.win.Size()

	a.mu.Lock()
	a.measuredViewportH = viewportH
	a.measuredContentH = contentH
	settled := h == a.lastAutofitWinH
	a.lastAutofitWinH = h
	a.mu.Unlock()

	// Bounds describe the previous frame's layout. Right after a resize this
	// frame lays out at the new height while the numbers still describe the
	// old one, so they are only trusted once the window height has stopped
	// changing. Without this the stale viewport fed a fresh delta back in
	// every frame and the popup oscillated instead of settling.
	if !settled {
		win := a.win
		win.Update(func() {})
		return
	}

	delta, ok := autofitHeight(viewportH, contentH)
	a.mu.Lock()
	delta, ok = a.autofitDecision(delta, ok)
	a.mu.Unlock()
	if !ok {
		return
	}

	log.Printf("fastDictionary: autofit viewport=%.1fx%.1f content=%.1f delta=%.1f winSize=%dx%d",
		scrollEl.Bounds().W, viewportH, contentH, delta, w, h)

	win := a.win
	win.Update(func() { a.applyPopupHeight(delta) })
}

// autofitDecision drops a correction that would undo the last one. Both
// heights come from a layout where the body Scroll fills the window, so
// shrinking the window reports taller content and asks to grow again: the two
// passes disagree forever and the window picks a new height on every repaint.
// Once they disagree, the correction is dropped and later ones are ignored
// until fitHeight reports the content itself changed. a.mu must be held.
func (a *app) autofitDecision(delta float32, ok bool) (float32, bool) {
	if !ok {
		// Already within a few DIPs: nothing to correct, and no reason to
		// remember a direction to compare against.
		return 0, false
	}
	if a.autofitLocked {
		// The lock only exists to stop the two-height bounce. A visible change
		// in content height means the content really moved (a streaming answer
		// grew), so the loop gets a fresh chance; otherwise it stays locked.
		if diff := a.measuredContentH - a.autofitLockContentH; diff > 8 || diff < -8 {
			a.autofitLocked = false
			a.autofitDir = 0
		} else {
			return 0, false
		}
	}
	if a.autofitDir != 0 && (delta > 0) != (a.autofitDir > 0) {
		a.autofitLocked = true
		a.autofitLockContentH = a.measuredContentH
		return 0, false
	}
	a.autofitDir = delta
	return delta, true
}

func renderNotesBlock(c *ui.Context, t *ui.Theme, s snapshot, a *app) {
	ui.Box(c).FillWidth().Padding(12, 14).Radius(8).
		Background(t.Surface).Border(1, t.Border).
		Shadow(0, 2, 6, 0, withAlpha(appTheme(t).ShadowColor, 0.5)).Children(func() {
		ui.Column(c).FillWidth().Gap(8).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, iconBookmark).Size(15, 15).TextColor(t.TextMuted)
					ui.Text(c, "猜你想存").FontSize(13).FontWeight(600).TextColor(t.Text)
				})
				ui.Spacer(c).Grow(1)
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "显示猜你想存").FontSize(11).TextColor(t.TextMuted)
					// Bound to the live config: the local variable this used
					// to read was recreated on every frame, so the switch
					// always rendered as on and clicking it did nothing.
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

			ui.Text(c, "选择要存入笔记本的词，将来随时复习").FontSize(11).TextColor(t.TextMuted)

			if s.notesBusy && len(s.notes) == 0 {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Padding(8, 0).Children(func() {
					ui.Spinner(c).Size(12, 12)
					ui.Text(c, "AI 正在提取生词...").FontSize(12).TextColor(t.TextMuted)
				})
			}

			for idx := range s.notes {
				item := &s.notes[idx]
				ui.Box(c).FillWidth().Padding(8, 10).Radius(6).
					Background(t.SurfaceHover).Children(func() {
					ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(8).Children(func() {
						ui.Checkbox(c, &item.Checked, "")
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
						ui.Box(c).Height(HeightIconSm).Padding(2, 12).Radius(RadiusSM).
							Background(t.Surface).Border(1, t.Border).Children(func() {
							ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
								ui.Icon(c, iconCheckSquare).Size(12, 12).TextColor(t.Success)
								ui.Text(c, "已保存 ✓").FontSize(11).FontWeight(600).TextColor(t.TextMuted)
							})
						})
					} else {
						btnSave := ui.Button(c, "").Height(HeightIconSm).Padding(2, 14).Radius(RadiusSM).Background(saveButtonColor(t)).TextColor(t.AccentText).Tooltip("保存所选生词到笔记本 (Ctrl+S)")
						btnSave.Children(func() {
							ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
								ui.Icon(c, iconBookmark).Size(12, 12).TextColor(t.AccentText)
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

// renderWordSaveButton keeps enrichment and file I/O off the UI thread.
func renderWordSaveButton(c *ui.Context, t *ui.Theme, s snapshot, a *app) {
	isSaved := s.savedDictTerm != "" && strings.EqualFold(strings.TrimSpace(s.savedDictTerm), strings.TrimSpace(s.source))
	if isSaved {
		ui.Box(c).Height(HeightIconSm).Padding(2, 8).Radius(RadiusSM).
			Background(t.Surface).Border(1, t.Border).Children(func() {
			ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconCheckSquare).Size(12, 12).TextColor(t.Success)
				ui.Text(c, "已保存 ✓").FontSize(11).FontWeight(600).TextColor(t.TextMuted)
			})
		})
		return
	}
	save := ui.Button(c, "").Height(HeightIconSm).Padding(2, 8).Radius(RadiusSM).
		Background(saveButtonColor(t)).TextColor(t.AccentText).Tooltip("将该单词保存到生词本 (Ctrl+S)")
	save.Children(func() {
		ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconBookmark).Size(12, 12).TextColor(t.AccentText)
			ui.Text(c, "保存生词").FontSize(11).FontWeight(600)
		})
	})
	if save.Clicked() {
		a.saveCurrentWordToVocab(s)
	}
}

// dictBlock renders pronunciation controls and meanings. The IPA is quiet
// text beside a small speaker chip, so the phonetics read as part of the
// word instead of as a button label.
// dictBlock renders the dictionary detail under the translation: phonetics
// with audible buttons, then part of speech and meaning.
//
// The type scale is deliberate — headword and phonetics in the caller, POS as
// a small tinted pill, meaning one step larger and darker than the pill, extra
// senses quieter still. Reading a word should not need squinting at one grey
// block of text.
func dictBlock(c *ui.Context, t *ui.Theme, s snapshot, a *app) {
	ui.Divider(c).FillWidth()
	hasPhones := (s.dict != nil && (s.dict.UKPhone != "" || s.dict.USPhone != "")) || (s.gdict != nil && s.gdict.Pron != "")
	if hasPhones || s.isWord {
		ui.Row(c).FillWidth().Gap(14).AlignItems(ui.Center).Children(func() {
			if s.dict != nil && s.dict.UKPhone != "" {
				ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
					btnUK := ui.Button(c, "").Label("英式发音").Tooltip("英式发音").
						Size(28, 28).Padding(0, 0).Radius(6).Center()
					if btnUK.Hovered() {
						btnUK.Background(t.SurfaceHover)
					}
					btnUK.Children(func() {
						ui.Icon(c, iconSpeak).Size(14, 14).TextColor(t.TextMuted)
					})
					if btnUK.Clicked() && a != nil {
						a.play(s.source, false)
					}
					ui.Text(c, "/"+s.dict.UKPhone+"/").FontSize(13).TextColor(t.TextMuted).SingleLine()
				})
			}
			if s.dict != nil && s.dict.USPhone != "" {
				ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
					btnUS := ui.Button(c, "").Label("美式发音").Tooltip("美式发音").
						Size(28, 28).Padding(0, 0).Radius(6).Center()
					if btnUS.Hovered() {
						btnUS.Background(t.SurfaceHover)
					}
					btnUS.Children(func() {
						ui.Icon(c, iconSpeak).Size(14, 14).TextColor(t.TextMuted)
					})
					if btnUS.Clicked() && a != nil {
						a.play(s.source, true)
					}
					ui.Text(c, "/"+s.dict.USPhone+"/").FontSize(13).TextColor(t.TextMuted).SingleLine()
				})
			} else if s.gdict != nil && s.gdict.Pron != "" {
				ui.Text(c, "/"+s.gdict.Pron+"/").FontSize(13).TextColor(t.Accent)
			}

			ui.Spacer(c).Grow(1)

			if a != nil {
				renderWordSaveButton(c, t, s, a)
			}
		})
	}
	if s.gdict != nil {
		for _, e := range s.gdict.Entries {
			ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Start).Children(func() {
				// A pill, not bare coloured text: it separates the part of
				// speech from the meaning without spending a whole column.
				ui.Box(c).Shrink(0).Padding(2, 6).Radius(4).
					Background(t.SurfaceHover).Border(1, t.Border).Children(func() {
					ui.Text(c, e.POS).FontSize(11).FontWeight(600).TextColor(t.Accent).SingleLine()
				})
				ui.Text(c, e.Explain).FontSize(14).TextColor(t.Text).Grow(1).Wrap().LineHeight(1.5)
			})
		}
	} else if s.dict != nil {
		for _, ex := range s.dict.Explains {
			ui.Text(c, ex).FontSize(14).TextColor(t.Text).Wrap().LineHeight(1.5)
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

// sameLang reports whether a detected tag and a target tag are the same
// language in practice (zh-CN vs zh-Hans and friends).
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

// renderAIResultState draws what the three AI cards show while their
// structured result does not exist yet.
//
// It used to render s.aiText — the raw, still-arriving model output — so the
// entire generation appeared as a wall of JSON that then vanished when the
// parser finally produced a card. While the model is still streaming the text
// is noise, not content, so a placeholder takes its place.
//
// After the stream ends the text is only shown when it is prose the reader can
// actually use. A failed parse of a JSON answer leaves braces and half-quoted
// keys, which is unreadable; that case gets a sentence saying so and points at
// the 重新翻译 button the footer already carries.
func renderAIResultState(c *ui.Context, t *ui.Theme, s snapshot) {
	switch classifyAIResult(s) {
	case aiPassIdle, aiPassStructured:
		// Structured cards are drawn by popupView's mode branches; this
		// function is only reached when no card is available.
		return
	case aiPassStreaming:
		// One spinner and one line: the task has no measurable progress, so a
		// progress bar only imitated work it could not report. The message
		// lives here rather than in a banner above so the wait appears where
		// the card will land.
		ui.Column(c).FillWidth().AlignItems(ui.Center).Gap(10).Padding(32, 0).Children(func() {
			ui.Spinner(c).Size(22, 22)
			ui.Text(c, aiBusyMessage(s.mode)).FontSize(13).TextColor(t.TextMuted)
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

// aiBusyMessage says what the running AI pass is doing, in the reader's terms.
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

// looksLikeStructuredPayload reports whether text is a JSON object, array or
// fenced block rather than prose. Only structured text is withheld: a model
// that answers in plain sentences has still produced something worth reading.
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

// renderAiMarkdown displays AI output using MyGo's markdown component:
// streaming with fade-in when busy, and complete rich view when finished.
func renderAiMarkdown(c *ui.Context, text string, busy bool) {
	if busy {
		markdown.Stream(c, text, nil)
	} else {
		markdown.View(c, text, nil)
	}
}

// renderPopupEmptyState 是浮窗空状态的统一组件：Lucide 图标 + 标题 + 说明，
// 不再用 Emoji 字符。
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
