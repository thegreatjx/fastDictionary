package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

// splitOcrParagraphs keeps blank lines as paragraph boundaries. OCR often
// reports one visual line per newline; when the page has no blank lines we keep
// the individual lines so each source line can be paired with its translation.
func splitOcrParagraphs(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	blocks := strings.Split(s, "\n\n")
	out := make([]string, 0, len(blocks))
	for _, block := range blocks {
		lines := strings.Split(strings.TrimSpace(block), "\n")
		if len(lines) <= 1 {
			if text := strings.TrimSpace(block); text != "" {
				out = append(out, text)
			}
			continue
		}
		for _, line := range lines {
			if text := strings.TrimSpace(line); text != "" {
				out = append(out, text)
			}
		}
	}
	return out
}

// renderOcrParagraphPairs is the screenshot reader's paragraph view. The
// translation is normally one block; for OCR, the user needs to see which
// translated paragraph belongs to which source paragraph. Long results fold at
// four paragraphs and can be expanded without leaving the card.
func (a *app) renderOcrParagraphPairs(c *ui.Context, t *ui.Theme, s snapshot) {
	src := splitOcrParagraphs(s.source)
	dst := splitOcrParagraphs(s.target)

	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
		ui.Icon(c, iconTranslate).Size(12, 12).TextColor(t.TextMuted)
		ui.Text(c, "段落对照").FontSize(11).TextColor(t.TextMuted)
		ui.Spacer(c).Grow(1)
		if renderIconButton(c, iconCopy, "复制译文", 26, 13, false).Clicked() {
			a.copyToClipboard(s.target)
		}
		if ui.Button(c, "合并显示").Height(24).Padding(1, 6).FontSize(10).Clicked() {
			a.mu.Lock()
			a.ocrCompare = false
			a.mu.Unlock()
			a.fitHeight()
		}
	})

	if len(src) == 0 || len(dst) == 0 || len(src) != len(dst) {
		if len(src) > 0 && len(dst) > 0 {
			ui.Text(c, fmt.Sprintf("识别到 %d 段原文、%d 段译文，段落数不一致，已按整段显示。", len(src), len(dst))).
				FontSize(10).TextColor(t.TextMuted).Wrap()
		}
		ui.Text(c, s.target).FontSize(15).TextColor(t.Text).Wrap().LineHeight(1.55)
		return
	}

	limit := len(src)
	if !s.ocrExpanded && limit > 8 {
		limit = 8
	}
	for i := 0; i < limit; i++ {
		ui.Box(c).FillWidth().Padding(8, 10).Radius(8).
			Background(t.Surface).Border(1, t.Border).Children(func() {
			ui.Column(c).FillWidth().Gap(4).Children(func() {
				ui.Text(c, src[i]).FontSize(12).TextColor(t.TextMuted).Wrap().LineHeight(1.5)
				ui.Text(c, dst[i]).FontSize(14).TextColor(t.Text).Wrap().LineHeight(1.55)
			})
		})
	}
	if len(src) > 8 {
		label := fmt.Sprintf("展开全文（共 %d 段）", len(src))
		if s.ocrExpanded {
			label = "收起段落"
		}
		if ui.Button(c, label).Height(26).FontSize(11).Clicked() {
			a.mu.Lock()
			a.ocrExpanded = !a.ocrExpanded
			a.mu.Unlock()
			a.fitHeight()
		}
	}
}

// renderScreenshotPairActions keeps the same action row as the merged view.
// Paragraph mode used to replace the whole result area, which hid copy, speech,
// AI refine and vocabulary controls that users expect from every translation
// card. The pair view now gets its own copy of that row.
func (a *app) renderScreenshotPairActions(c *ui.Context, t *ui.Theme, cfg Config, s snapshot) {
	ui.Row(c).FillWidth().Gap(2).AlignItems(ui.Center).Children(func() {
		copyTip := "复制译文"
		if s.copiedMsg != "" {
			copyTip = "译文已复制"
		}
		if renderIconButton(c, iconCopy, copyTip, 28, 14, false).Clicked() {
			a.copyToClipboard(s.target)
		}
		spText, spLang, spTip := primarySpeechTarget(s.source, s.target)
		if spText != "" {
			if renderIconButton(c, iconSpeak, spTip, 28, 14, false).Clicked() {
				PlaySpeechLang(spText, spLang, false)
			}
		}
		if s.source != "" && !s.isWord {
			tip := "AI 精翻（先读 OCR 文字，失败时用截图校正）"
			if s.aiRefining {
				tip = "AI 精翻中…"
			} else if s.engine == "AI 精翻" {
				tip = "已是 AI 精翻 · 点击重新生成"
			}
			btn := ui.Button(c, "").Tooltip(tip).Size(28, 28).Padding(0, 0).Radius(6).Center()
			btn.Children(func() {
				col := t.TextMuted
				if s.engine == "AI 精翻" {
					col = t.Accent
				}
				if s.aiRefining {
					ui.Spinner(c).Size(15, 15)
				} else {
					ui.Icon(c, iconSparkles).Size(15, 15).TextColor(col)
				}
			})
			if btn.Clicked() && !s.aiRefining {
				a.refineWithAI()
			}
			if s.aiRefineErr != "" {
				ui.Text(c, "精翻失败："+s.aiRefineErr).FontSize(10).TextColor(t.Danger).SingleLine()
			}
		}
		ui.Spacer(c).Grow(1)
		if cfg.ShowEngineTag && s.engine != "" {
			ui.Text(c, translationEngineCaption(s, cfg.TargetLang)).FontSize(10).
				TextColor(t.TextMuted).SingleLine().Opacity(0.85)
		}
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
}
