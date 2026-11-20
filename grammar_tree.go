package main

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

type grammarTreeRow struct {
	Depth     int
	Tag       string
	Text      string
	Target    string
	Kind      string // "predicate", "backbone", "clause", "phrase", "mod"
	ClauseTag string
	PhraseTag string
}

func grammarNodeDepth(clauseID, phraseID string, clauseByID map[string]ClauseInfo, phraseByID map[string]PhraseInfo) int {
	depth := 0
	cur := phraseID
	if cur == "" {
		cur = clauseID
	}
	seen := make(map[string]bool, 8)
	for cur != "" && !seen[cur] && depth < 4 {
		seen[cur] = true
		depth++
		if p, ok := phraseByID[cur]; ok {
			cur = p.ParentID
			continue
		}
		if c, ok := clauseByID[cur]; ok {
			cur = c.ParentID
			continue
		}
		break
	}
	return depth
}

func grammarTreeRows(res *SentenceAnalysisResult) []grammarTreeRow {
	clauseByID := make(map[string]ClauseInfo, len(res.Clauses))
	for _, cl := range res.Clauses {
		clauseByID[cl.ID] = cl
	}
	phraseByID := make(map[string]PhraseInfo, len(res.Phrases))
	for _, ph := range res.Phrases {
		phraseByID[ph.ID] = ph
	}

	var rows []grammarTreeRow
	for i := 0; i < len(res.Segments); {
		seg := res.Segments[i]
		if seg.Role == RolePunct && strings.TrimSpace(seg.Text) != "" && len(rows) > 0 {
			rows[len(rows)-1].Text += strings.TrimSpace(seg.Text)
			i++
			continue
		}
		if strings.TrimSpace(seg.Text) == "" {
			i++
			continue
		}

		isPred := seg.IsBackbone && seg.Role == RolePredicate
		j := i + 1
		for j < len(res.Segments) {
			next := res.Segments[j]
			if next.Role == RolePunct {
				break
			}
			nextPred := next.IsBackbone && next.Role == RolePredicate
			if next.IsBackbone != seg.IsBackbone || nextPred != isPred ||
				(seg.IsBackbone && next.Role != seg.Role) ||
				next.ClauseID != seg.ClauseID || next.PhraseID != seg.PhraseID {
				break
			}
			j++
		}

		var textBuf strings.Builder
		var notes []string
		for k := i; k < j; k++ {
			textBuf.WriteString(res.Segments[k].Text)
			if n := strings.TrimSpace(res.Segments[k].Note); n != "" {
				notes = append(notes, n)
			}
		}
		chunkText := strings.TrimSpace(textBuf.String())
		for j < len(res.Segments) && res.Segments[j].Role == RolePunct {
			p := strings.TrimSpace(res.Segments[j].Text)
			if p != "" {
				chunkText += p
			}
			j++
		}
		i = j
		if chunkText == "" {
			continue
		}

		depth := grammarNodeDepth(seg.ClauseID, seg.PhraseID, clauseByID, phraseByID)
		row := grammarTreeRow{
			Depth:  depth,
			Text:   chunkText,
			Target: strings.Join(notes, " · "),
		}

		switch {
		case seg.PhraseID != "":
			row.Kind = "phrase"
			if p, ok := phraseByID[seg.PhraseID]; ok {
				tag := grammarPhraseShortTag(p)
				row.PhraseTag = tag
				if p.Role != "" {
					tag += "·" + p.Role
				}
				row.Tag = tag
				if p.Target != "" {
					row.Target = p.Target
				}
			} else {
				row.PhraseTag = "短语"
				row.Tag = "短语"
			}
		case seg.ClauseID != "":
			row.Kind = "clause"
			if cl, ok := clauseByID[seg.ClauseID]; ok {
				row.ClauseTag = grammarClauseLabel(cl)
				row.Tag = row.ClauseTag
				if cl.Target != "" && row.Target == "" {
					row.Target = cl.Target
				}
			} else {
				row.ClauseTag = "从句"
				row.Tag = "从句"
			}
		case seg.IsBackbone && seg.Role == RolePredicate:
			row.Kind = "predicate"
			row.Tag = "谓语"
			row.Depth = 0
		case seg.IsBackbone:
			row.Kind = "backbone"
			row.Tag = grammarRoleLabel(seg.Role)
			if row.Tag == "修饰语" {
				row.Tag = "主干"
			}
			row.Depth = 0
		default:
			row.Kind = "mod"
			row.Tag = "修饰"
			if depth == 0 {
				row.Depth = 1
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func renderGrammarTreeView(c *ui.Context, t *ui.Theme, at AppTheme, ink grammarInk, res *SentenceAnalysisResult, st grammarCardState) {
	rows := grammarTreeRows(res)
	ui.Column(c).FillWidth().Gap(5).Children(func() {
		for _, row := range rows {
			indent := float32(row.Depth) * 14
			ui.Row(c).FillWidth().AlignItems(ui.Start).Padding(0, 0, 0, indent).Gap(6).Children(func() {
				if row.Depth > 0 {
					barCol := at.BorderCtrl
					switch row.Kind {
					case "clause":
						barCol = at.Accent.Alpha(0.55)
					case "phrase":
						barCol = grammarPhrasePalette(t, row.PhraseTag).bracket
					}
					ui.Box(c).Width(2).Height(18).Radius(1).Background(barCol).Shrink(0)
				}

				tagBg, tagBorder, tagText := at.Surface, at.BorderSubtle, at.TextSec
				switch row.Kind {
				case "predicate":
					tagBg, tagBorder, tagText = at.Accent, at.Accent, at.AccentText
				case "backbone":
					tagBg, tagBorder, tagText = at.AccentSoft, at.AccentRing, at.Accent
				case "clause":
					cc := grammarClausePalette(t, row.ClauseTag)
					tagBg, tagBorder, tagText = cc.badgeBg, cc.badgeBg, cc.badgeText
				case "phrase":
					pc := grammarPhrasePalette(t, row.PhraseTag)
					tagBg, tagBorder, tagText = pc.badgeBg, pc.border, pc.badgeText
				}
				ui.Box(c).Padding(1, 6).Radius(RadiusXS).Shrink(0).Background(tagBg).Border(1, tagBorder).Children(func() {
					ui.Text(c, row.Tag).FontSize(10).FontWeight(600).TextColor(tagText).SingleLine()
				})

				ui.Column(c).Grow(1).Gap(2).Children(func() {
					el := ui.Text(c, row.Text).FontSize(13).LineHeight(1.45).Wrap().Selectable()
					switch row.Kind {
					case "predicate":
						el.TextColor(ink.predicate).FontWeight(700).Underline().DecorationColor(ink.predicateLine).DecorationThickness(1.1).UnderlineOffset(2)
					case "backbone":
						el.TextColor(ink.body).FontWeight(600)
					case "clause":
						el.TextColor(ink.body).FontWeight(500)
					case "phrase":
						pc := grammarPhrasePalette(t, row.PhraseTag)
						el.TextColor(pc.text).FontWeight(500)
						if grammarPhraseFamily(row.PhraseTag) == "nonfinite" {
							el.DottedUnderline().DecorationColor(pc.underline).DecorationThickness(1.2).UnderlineOffset(2)
						}
					default:
						el.TextColor(at.TextSec).FontWeight(500)
					}
					if row.Target != "" {
						ui.Text(c, "↳ "+row.Target).FontSize(11).TextColor(at.TextMuted).Wrap()
					}
				})
			})
		}
	})
	renderGrammarHintBar(c, t, at, grammarDefaultHint(res, st.Prefill))
}
