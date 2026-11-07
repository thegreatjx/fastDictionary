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
