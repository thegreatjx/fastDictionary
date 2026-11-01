package main

import (
	"log"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

// renderIconButton renders a square icon button where the icon is guaranteed
// to be strictly horizontally and vertically centered with crisp feedback.
func renderIconButton(c *ui.Context, icon *ui.SVG, tip string, size, iconSize float32, active bool) ui.Element {
	t := c.Theme()
	b := ui.Button(c, "").Size(size, size).Padding(0, 0).Radius(5).Center().Tooltip(tip).Label(tip)
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

				// A quiet confirmation that a copy actually landed. The
				// clipboard gives no visible feedback on its own, so the
				// button used to look dead.
				if s.copiedMsg != "" {
					ui.Box(c).FillWidth().Padding(8, 10).Radius(6).
						Background(noticeBg(t)).Border(1, noticeBorder(t)).Children(func() {
						ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(6).Children(func() {
							ui.Icon(c, iconInfo).Size(13, 13).TextColor(noticeText(t))
							ui.Text(c, s.copiedMsg).FontSize(12).TextColor(noticeText(t)).Wrap().Grow(1)
						})
					})
				}

				// A save that landed said nothing in the dictionary, grammar
				// and writing cards: the confirmation was rendered inside the
				// translation tab only, so those three looked like the button
				// had done nothing at all. The translate tab keeps its own
				// copy, hence the mode guard — rendering here for it too would
				// print the line twice.
				if s.actionSavedMsg != "" && s.mode != modeTranslate && s.mode != modeInput && s.mode != modeSelectionPreview {
					ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
						ui.Icon(c, iconCheckSquare).Size(13, 13).TextColor(t.Accent)
						ui.Text(c, s.actionSavedMsg).FontSize(12).TextColor(t.Accent).Wrap().Grow(1)
					})
				}

				// Show general error if present
				if s.errText != "" {
					ui.Text(c, s.errText).FontSize(13).TextColor(t.Danger).Wrap()
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
					if s.grammarResult != nil {
						a.renderGrammarVisualCard(c, t, s.source, s.grammarResult, func() { a.saveGrammarToVocab(s.grammarResult) })
					} else {
						renderAIResultState(c, t, s)
					}

				case modeDictionary:
					// Dedicated Learner's Dictionary Mode!
					if s.dictResult != nil {
						renderDictionaryCard(c, t, s.source, s.dictResult, s.savedDictTerm, func() { a.saveDictToVocab(s.dictResult) })
					} else {
						renderAIResultState(c, t, s)
					}

				case modeWriting:
					// Dedicated Improve Writing Mode!
					if s.writingResult != nil {
						renderImproveWritingCard(c, t, s.source, s.writingResult, func() { a.saveWritingToVocab(s.writingResult) })
					} else {
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
	a.autofitTarget = 0
	if !ok {
		// Already within a few DIPs: nothing to correct, and no reason to
		// remember a direction to compare against.
		return 0, false
	}
	if a.autofitLocked {
		return 0, false
	}
	if a.autofitDir != 0 && (delta > 0) != (a.autofitDir > 0) {
		a.autofitLocked = true
		return 0, false
	}
	a.autofitTarget = delta
	a.autofitDir = delta
	a.autofitSteps++
	return delta, true
}

func renderNotesBlock(c *ui.Context, t *ui.Theme, s snapshot, a *app) {
	ui.Box(c).FillWidth().Padding(12, 14).Radius(8).
		Background(t.Surface).Border(1, t.Border).
		Shadow(0, 2, 6, 0, ui.RGBA(0, 0, 0, 0.06)).Children(func() {
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
					})
				})
			}

			if len(s.notes) > 0 {
				ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
					if s.notesSaved {
						ui.Box(c).Height(24).Padding(2, 12).Radius(5).Background(ui.Hex("#10B981")).Children(func() {
							ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
								ui.Text(c, "✓ 已存入生词本").FontSize(11).FontWeight(600).TextColor(ui.Hex("#FFFFFF"))
							})
						})
					} else {
						btnSave := ui.Button(c, "").Height(24).Padding(2, 14).Radius(5).Background(ui.Hex("#EAB308")).TextColor(ui.Hex("#FFFFFF"))
						btnSave.Children(func() {
							ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
								ui.Icon(c, iconBookmark).Size(12, 12).TextColor(ui.Hex("#FFFFFF"))
								ui.Text(c, "保存所选生词").FontSize(11).FontWeight(600)
							})
						})
						if btnSave.Clicked() {
							// Collect under the lock, do the slow part
							// outside it. EnrichVocabularyEntry makes a
							// network call with a 3 s timeout and Add
							// rewrites the whole notebook file, so the old
							// code held a.mu - the render lock - for up to
							// three seconds per word and the window went
							// "not responding".
							a.mu.Lock()
							a.notesSaved = true
							a.actionSavedMsg = "已将所选生词存入生词库 ✓"
							pending := make([]VocabularyEntry, 0, len(s.notes))
							for _, n := range s.notes {
								if n.Checked {
									pending = append(pending, VocabularyEntry{
										Term:               n.Word,
										Phonetic:           n.Phonetic,
										PartOfSpeech:       n.Pos,
										Definition:         n.Meaning,
										Difficulty:         n.Difficulty,
										MemoryTips:         n.MemoryTips,
										Context:            s.source,
										ContextTerm:        n.Word,
										ContextTranslation: s.target,
										Source:             "notes",
										CreatedAt:          time.Now(),
									})
								}
							}
							a.mu.Unlock()
							if a.win != nil {
								a.win.Update(func() {})
							}
							go func() {
								for i := range pending {
									EnrichVocabularyEntry(&pending[i])
								}
								GetVocabularyStore().AddBatch(pending)
							}()
						}
					}
				})
			}
		})
	})
}

// renderWordSaveButton keeps enrichment and file I/O off the UI thread.
func renderWordSaveButton(c *ui.Context, t *ui.Theme, s snapshot, a *app) {
	label := "保存生词"
	background := saveButtonColor(t)
	if a.actionSavedMsg != "" {
		label = "已保存 ✓"
		background = t.Success
	}
	save := ui.Button(c, "").Height(24).Padding(2, 8).Radius(5).
		Background(background).TextColor(ui.Hex("#FFFFFF")).Tooltip("将该单词保存到生词本")
	save.Children(func() {
		ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconBookmark).Size(12, 12).TextColor(ui.Hex("#FFFFFF"))
			ui.Text(c, label).FontSize(11).FontWeight(600)
		})
	})
	if save.Clicked() {
		phone := ""
		if s.dict != nil {
			if s.dict.UKPhone != "" {
				phone = "/" + s.dict.UKPhone + "/"
			} else if s.dict.USPhone != "" {
				phone = "/" + s.dict.USPhone + "/"
			}
		}
		explanation, pos := s.target, ""
		if s.gdict != nil && len(s.gdict.Entries) > 0 {
			pos = s.gdict.Entries[0].POS
		} else if s.dict != nil && len(s.dict.Explains) > 0 {
			explanation = strings.Join(s.dict.Explains, "；")
			parts := strings.SplitN(s.dict.Explains[0], " ", 2)
			if len(parts) == 2 && strings.HasSuffix(parts[0], ".") {
				pos = parts[0]
			}
		}
		a.mu.Lock()
		a.actionSavedMsg = "已保存「" + s.source + "」到生词本 ✓"
		a.mu.Unlock()
		if a.win != nil {
			a.win.Update(func() {})
		}
		entry := VocabularyEntry{
			Term: s.source, Phonetic: phone, PartOfSpeech: pos,
			Definition: explanation, Source: "dict", CreatedAt: time.Now(),
		}
		go func() {
			EnrichVocabularyEntry(&entry)
			GetVocabularyStore().Add(entry)
		}()
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
	if s.dict != nil && (s.dict.UKPhone != "" || s.dict.USPhone != "") {
		ui.Row(c).FillWidth().Gap(14).AlignItems(ui.Center).Children(func() {
			if s.dict.UKPhone != "" {
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
			if s.dict.USPhone != "" {
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
			}

			ui.Spacer(c).Grow(1)

			if a != nil {
				renderWordSaveButton(c, t, s, a)
			}
		})
	} else if s.gdict != nil && s.gdict.Pron != "" {
		ui.Text(c, "/"+s.gdict.Pron+"/").FontSize(13).TextColor(t.Accent)
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
		btnStart := ui.Button(c, "").Height(24).Padding(2, 10).Radius(5).Background(ui.Hex("#2563EB")).TextColor(ui.Hex("#FFFFFF"))
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
	if s.aiBusy {
		// One spinner and one line: the task has no measurable progress, so a
		// progress bar only imitated work it could not report. The message
		// lives here rather than in a banner above so the wait appears where
		// the card will land.
		ui.Column(c).FillWidth().AlignItems(ui.Center).Gap(10).Padding(32, 0).Children(func() {
			ui.Spinner(c).Size(22, 22)
			ui.Text(c, aiBusyMessage(s.mode)).FontSize(13).TextColor(t.TextMuted)
		})
		return
	}
	if s.aiText == "" {
		return
	}
	if !looksLikeStructuredPayload(s.aiText) {
		renderAiMarkdown(c, s.aiText, false)
		return
	}
	ui.Box(c).FillWidth().Padding(14, 16).Radius(8).
		Background(t.Surface).Border(1, t.Border).Children(func() {
		ui.Column(c).FillWidth().Gap(6).Children(func() {
			ui.Text(c, "这次没能解析成结构化卡片").FontSize(13).FontWeight(600).TextColor(t.Text)
			ui.Text(c, "模型返回的内容不完整或格式不对，原始输出不适合直接阅读。点右下角的「重新翻译」可以再生成一次。").
				FontSize(12).TextColor(t.TextMuted).Wrap()
		})
	})
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
