package main

import (
	"encoding/json"
	"fmt"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"reflect"
	"strings"
)

type DictionaryResult struct {
	Term               string `json:"term"`
	Phonetic           string `json:"phonetic"`
	PartOfSpeech       string `json:"partOfSpeech"`
	Definition         string `json:"definition"`
	Context            string `json:"context"`
	ContextTerm        string `json:"contextTerm"`
	ContextTranslation string `json:"contextTranslation"`
	MemoryTips         string `json:"memoryTips"`
	Difficulty         string `json:"difficulty"`
	Raw                string `json:"-"`
}

func CleanJSONText(raw string) string {
	cleaned := strings.TrimSpace(raw)
	if strings.HasPrefix(cleaned, "```json") {
		cleaned = strings.TrimPrefix(cleaned, "```json")
	} else if strings.HasPrefix(cleaned, "```") {
		cleaned = strings.TrimPrefix(cleaned, "```")
	}
	if idx := strings.LastIndex(cleaned, "```"); idx >= 0 {
		cleaned = cleaned[:idx]
	}
	return strings.TrimSpace(cleaned)
}

func ParseDictionaryResult(raw string) *DictionaryResult {
	cleaned := CleanJSONText(raw)
	if cleaned == "" {
		return nil
	}

	type rawDict struct {
		Term               string `json:"term"`
		Phonetic           string `json:"phonetic"`
		PartOfSpeech       string `json:"partOfSpeech"`
		PartOfSpeechAlt    string `json:"part_of_speech"`
		Definition         string `json:"definition"`
		Context            string `json:"context"`
		Sentence           string `json:"sentence"`
		ContextTerm        any    `json:"contextTerm"`
		ContextTermAlt     any    `json:"context_term"`
		ContextTranslation string `json:"contextTranslation"`
		ContextTransAlt    string `json:"context_translation"`
		MemoryTips         any    `json:"memoryTips"`
		MemoryTipsAlt      any    `json:"memory_tips"`
		Difficulty         string `json:"difficulty"`

		TermZH         string `json:"词条"`
		PhoneticZH     string `json:"音标"`
		PosZH          string `json:"词性"`
		DefZH          string `json:"释义"`
		ContextZH      string `json:"句子"`
		ContextTermZH  any    `json:"句中词形"`
		ContextTransZH string `json:"句子翻译"`
		MemTipsZH      any    `json:"记忆技巧"`
		DiffZH         string `json:"难度"`
	}

	var d rawDict
	if err := json.Unmarshal([]byte(cleaned), &d); err != nil {
		start := strings.Index(cleaned, "{")
		end := strings.LastIndex(cleaned, "}")
		if start >= 0 && end > start {
			sub := cleaned[start : end+1]
			if err2 := json.Unmarshal([]byte(sub), &d); err2 != nil {
				return nil
			}
		} else {
			return nil
		}
	}

	res := &DictionaryResult{
		Term:               firstNonEmpty(d.Term, d.TermZH),
		Phonetic:           firstNonEmpty(d.Phonetic, d.PhoneticZH),
		PartOfSpeech:       firstNonEmpty(d.PartOfSpeech, d.PartOfSpeechAlt, d.PosZH),
		Definition:         firstNonEmpty(d.Definition, d.DefZH),
		Context:            firstNonEmpty(d.Context, d.Sentence, d.ContextZH),
		ContextTranslation: firstNonEmpty(d.ContextTranslation, d.ContextTransAlt, d.ContextTransZH),
		Difficulty:         firstNonEmpty(d.Difficulty, d.DiffZH),
		Raw:                raw,
	}

	ctVal := firstNonNil(d.ContextTerm, d.ContextTermAlt, d.ContextTermZH)
	res.ContextTerm = parseContextTerm(ctVal)

	mtVal := firstNonNil(d.MemoryTips, d.MemoryTipsAlt, d.MemTipsZH)
	res.MemoryTips = parseMemoryTips(mtVal)

	if res.Term == "" && res.Definition == "" {
		return nil
	}
	return res
}

func parseContextTerm(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		s = strings.TrimSpace(s)
		if strings.HasPrefix(s, "[") {
			var list []map[string]any
			if err := json.Unmarshal([]byte(s), &list); err == nil && len(list) > 0 {
				var words []string
				for _, item := range list {
					if t, ok := item["text"].(string); ok && t != "" {
						words = append(words, t)
					}
				}
				if len(words) > 0 {
					return strings.Join(words, ", ")
				}
			}
		}
		return s
	}
	if list, ok := v.([]any); ok {
		var words []string
		for _, item := range list {
			if m, ok := item.(map[string]any); ok {
				if t, ok := m["text"].(string); ok && t != "" {
					words = append(words, t)
				}
			} else if str, ok := item.(string); ok && str != "" {
				words = append(words, str)
			}
		}
		if len(words) > 0 {
			return strings.Join(words, ", ")
		}
	}
	return fmt.Sprint(v)
}

func parseMemoryTips(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	if list, ok := v.([]any); ok {
		var lines []string
		for _, item := range list {
			if str, ok := item.(string); ok && strings.TrimSpace(str) != "" {
				lines = append(lines, strings.TrimSpace(str))
			}
		}
		return strings.Join(lines, "\n")
	}
	return fmt.Sprint(v)
}

type WritingAnnotation struct {
	Text       string `json:"text"`
	Fix        string `json:"fix"`
	Type       string `json:"type"`
	Tag        string `json:"tag,omitempty"`
	Note       string `json:"note,omitempty"`
	Occurrence int    `json:"occurrence,omitempty"`
}

type ImproveWritingResult struct {
	Setting     string              `json:"setting"`
	Annotations []WritingAnnotation `json:"annotations"`
	Improved    string              `json:"improved"`
	Summary     string              `json:"summary"`
	Raw         string              `json:"-"`
}

func ParseImproveWritingResult(raw string) *ImproveWritingResult {
	cleaned := CleanJSONText(raw)
	if cleaned == "" {
		return nil
	}

	type rawWriting struct {
		Setting        string `json:"setting"`
		SettingZH      string `json:"场合"`
		SettingDollar  string `json:"$1"`
		Annotations    any    `json:"annotations"`
		AnnotationsZH  any    `json:"标注"`
		AnnotationsDol any    `json:"$2"`
		Improved       string `json:"improved"`
		ImprovedZH     string `json:"改后"`
		ImprovedDollar string `json:"$3"`
		Summary        string `json:"summary"`
		SummaryZH      string `json:"总评"`
		SummaryDollar  string `json:"$4"`
	}

	var w rawWriting
	if err := json.Unmarshal([]byte(cleaned), &w); err != nil {
		start := strings.Index(cleaned, "{")
		end := strings.LastIndex(cleaned, "}")
		if start >= 0 && end > start {
			sub := cleaned[start : end+1]
			if err2 := json.Unmarshal([]byte(sub), &w); err2 != nil {
				return nil
			}
		} else {
			return nil
		}
	}

	res := &ImproveWritingResult{
		Setting:  firstNonEmpty(w.Setting, w.SettingZH, w.SettingDollar),
		Improved: firstNonEmpty(w.Improved, w.ImprovedZH, w.ImprovedDollar),
		Summary:  firstNonEmpty(w.Summary, w.SummaryZH, w.SummaryDollar),
		Raw:      raw,
	}

	annVal := firstNonNil(w.Annotations, w.AnnotationsZH, w.AnnotationsDol)
	if annVal != nil {
		res.Annotations = parseWritingAnnotations(annVal)
	}

	if res.Improved == "" && len(res.Annotations) == 0 && res.Summary == "" {
		return nil
	}
	return res
}

func parseWritingAnnotations(v any) []WritingAnnotation {
	if v == nil {
		return nil
	}
	if s, ok := v.(string); ok {
		s = strings.TrimSpace(s)
		var list []WritingAnnotation
		if err := json.Unmarshal([]byte(s), &list); err == nil {
			return normalizeWritingAnnotations(list)
		}
		return nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var list []WritingAnnotation
	if err := json.Unmarshal(data, &list); err == nil {
		return normalizeWritingAnnotations(list)
	}
	return nil
}

var writingTypes = []string{
	"spelling", "grammar", "punctuation", "word-choice", "logic",
	"unnatural", "register", "clarity", "good",
}

func normalizeWritingAnnotations(list []WritingAnnotation) []WritingAnnotation {
	out := list[:0]
	for _, an := range list {
		an.Type = normalizeWritingType(an.Type)
		out = append(out, an)
	}
	return out
}

func normalizeWritingType(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return ""
	}
	for _, known := range writingTypes {
		if s == known {
			return known
		}
	}
	switch {
	case strings.Contains(s, "good") || strings.Contains(s, "worth keeping"):
		return "good"
	case strings.Contains(s, "literal translation") || strings.Contains(s, "stiff") ||
		strings.Contains(s, "wordy") || strings.Contains(s, "unnatural"):
		return "unnatural"
	case strings.Contains(s, "too casual") || strings.Contains(s, "too formal") ||
		strings.Contains(s, "impolite") || strings.Contains(s, "honorific") ||
		strings.Contains(s, "register"):
		return "register"
	case strings.Contains(s, "ambiguous") || strings.Contains(s, "confusing") ||
		strings.Contains(s, "clarity"):
		return "clarity"
	case strings.Contains(s, "spelling") || strings.Contains(s, "typo"):
		return "spelling"
	case strings.Contains(s, "grammar"):
		return "grammar"
	case strings.Contains(s, "punctuation"):
		return "punctuation"
	case strings.Contains(s, "word-choice") || strings.Contains(s, "word choice"):
		return "word-choice"
	case strings.Contains(s, "logic"):
		return "logic"
	}
	return ""
}

func FormatActionPrompt(template, selection, paragraphs, targetLang string) string {
	res := template
	res = strings.ReplaceAll(res, "{{selection}}", selection)
	res = strings.ReplaceAll(res, "{{paragraphs}}", paragraphs)
	res = strings.ReplaceAll(res, "{{targetLanguage}}", targetLang)
	return res
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func firstNonNil(vals ...any) any {
	for _, v := range vals {
		if v == nil {
			continue
		}
		rv := reflect.ValueOf(v)
		switch rv.Kind() {
		case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan, reflect.Interface:
			if rv.IsNil() {
				continue
			}
		}
		return v
	}
	return nil
}

func renderDictionaryCard(c *ui.Context, t *ui.Theme, origText string, res *DictionaryResult, savedTerm string, onSave func()) {
	termText := res.Term
	if termText == "" {
		termText = origText
	}
	saved := savedTerm != "" && savedTerm == termText
	ui.Box(c).FillWidth().Padding(Space3, Space4).Radius(8).
		Background(t.Surface).Border(1, t.Border).
		Shadow(0, 2, 8, 0, ui.RGBA(0, 0, 0, 0.08)).Children(func() {
		ui.Column(c).FillWidth().Gap(Space2).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Text(c, termText).FontSize(18).FontWeight(700).TextColor(t.Text)

				if res.Phonetic != "" {
					ph := res.Phonetic
					if !strings.HasPrefix(ph, "/") {
						ph = "/" + ph + "/"
					}
					ui.Text(c, ph).FontSize(13).TextColor(t.TextMuted)
				}

				btnSpeak := renderIconButton(c, iconSpeak, "朗读发音", 28, 15, false)
				if btnSpeak.Clicked() {
					PlaySpeechLang(termText, "", false)
				}

				ui.Spacer(c).Grow(1)

				if res.Difficulty != "" {
					d := strings.ToUpper(strings.TrimSpace(res.Difficulty))
					bg, fg := cefrColors(t, d)
					ui.Box(c).Padding(2, 8).Radius(10).Background(bg).Children(func() {
						ui.Text(c, d).FontSize(11).FontWeight(700).TextColor(fg)
					})
				}
			})

			ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(8).Children(func() {
				if res.PartOfSpeech != "" {
					ui.Box(c).Padding(2, 6).Radius(4).Background(t.SurfaceHover).Border(1, t.Border).Children(func() {
						ui.Text(c, res.PartOfSpeech).FontSize(11).FontWeight(600).TextColor(t.Accent)
					})
				}
				if res.Definition != "" {
					ui.Text(c, res.Definition).FontSize(14).FontWeight(500).TextColor(t.Text).Wrap().Grow(1)
				}
			})

			if res.Context != "" {
				ui.Box(c).FillWidth().Padding(10, 12).Radius(6).
					Background(t.SurfaceHover).Border(1, t.Border).Children(func() {
					ui.Column(c).FillWidth().Gap(6).Children(func() {
						ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Start).Children(func() {
							btnSpeakCtx := renderIconButton(c, iconSpeak, "朗读例句", 24, 14, false)
							if btnSpeakCtx.Clicked() {
								PlaySpeechLang(res.Context, "", false)
							}
							renderHighlightedContext(c, t, res.Context, res.ContextTerm)
						})
						if res.ContextTranslation != "" {
							ui.Text(c, res.ContextTranslation).FontSize(12).TextColor(t.TextMuted).Wrap().Padding(0, 0, 0, 20)
						}
					})
				})
			}

			if res.MemoryTips != "" {
				tipBg, tipBorder, tipTitle, tipBody := memoryTipColors(t)
				ui.Box(c).FillWidth().Padding(10, 12).Radius(6).
					Background(tipBg).Border(1, tipBorder).Children(func() {
					ui.Column(c).FillWidth().Gap(4).Children(func() {
						ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
							ui.Icon(c, iconInfo).Size(12, 12).TextColor(tipTitle)
							ui.Text(c, "记忆技巧").FontSize(11).FontWeight(700).TextColor(tipTitle)
						})
						tipLines := strings.Split(res.MemoryTips, "\n")
						for _, line := range tipLines {
							trimmed := strings.TrimSpace(line)
							if trimmed != "" {
								ui.Text(c, "• "+trimmed).FontSize(12).TextColor(tipBody).Wrap()
							}
						}
					})
				})
			}

			ui.Divider(c).FillWidth()

			ui.Row(c).FillWidth().Justify(ui.End).AlignItems(ui.Center).Children(func() {
				if saved || onSave == nil {
					ui.Row(c).Height(HeightCompact).Gap(Space1).AlignItems(ui.Center).Children(func() {
						ui.Icon(c, iconCheck).Size(12, 12).TextColor(appTheme(t).TextMuted)
						ui.Text(c, "已保存").FontSize(11).FontWeight(500).TextColor(appTheme(t).TextMuted)
					})
					return
				}
				btnSave := ui.Button(c, "").Height(HeightCompact).Padding(2, Space3).Radius(RadiusSM).
					Tooltip("把这个词条存入本地生词本")
				saveFG := saveButtonStyle(btnSave, t)
				btnSave.Children(func() {
					ui.Row(c).Gap(Space1).AlignItems(ui.Center).Children(func() {
						ui.Icon(c, iconBookmark).Size(12, 12).TextColor(saveFG)
						ui.Text(c, "保存到生词本").FontSize(11).FontWeight(600)
					})
				})
				if btnSave.Clicked() {
					onSave()
				}
			})
		})
	})
}

type writingTone struct {
	background ui.Color
	border     ui.Color
	text       ui.Color
}

type writingTheme struct {
	setting       writingTone
	tag           writingTone
	success       writingTone
	improved      writingTone
	copy          writingTone
	save          writingTone
	settingLabel  ui.Color
	removed       ui.Color
	added         ui.Color
	deleted       ui.Color
	improvedTitle ui.Color
}

func writingThemeFor(t *ui.Theme) writingTheme {
	at := newAppTheme(false, defaultAccent(false))
	if t != nil {
		at = appTheme(t)
	}
	saveBackground := saveButtonColor(t)
	saveText := saveButtonTextColor(t)
	removed := at.Danger.Mix(at.TextSec, 0.28)
	added := at.AccentPress
	if at.Dark {
		removed = at.Danger.Mix(at.TextSec, 0.18)
		added = at.Accent
	}
	return writingTheme{
		setting:       writingTone{background: at.SurfaceHover, border: at.BorderSubtle, text: at.Text},
		tag:           writingTone{background: at.SurfaceHover, border: at.BorderSubtle, text: at.TextSec},
		success:       writingTone{background: at.TintSurface, border: at.TintBorder, text: at.TextSec},
		improved:      writingTone{background: at.TintSurface, border: at.TintBorder, text: at.Text},
		copy:          writingTone{background: at.Surface, border: at.BorderCtrl, text: at.TextSec},
		save:          writingTone{background: saveBackground, border: saveBackground, text: saveText},
		settingLabel:  at.TextSec,
		removed:       removed,
		added:         added,
		deleted:       at.TextSec,
		improvedTitle: at.TextSec,
	}
}

func renderImproveWritingCard(c *ui.Context, t *ui.Theme, origText string, res *ImproveWritingResult, saved bool, onCopy func(string), onSave func()) {
	w := writingThemeFor(t)
	ui.Box(c).FillWidth().Padding(Space3, Space4).Radius(8).
		Background(t.Surface).Border(1, t.Border).
		Shadow(0, 2, 8, 0, ui.RGBA(0, 0, 0, 0.08)).Children(func() {
		ui.Column(c).FillWidth().Gap(Space2).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
				settingText := res.Setting
				if settingText == "" {
					settingText = "通用场合"
				}
				ui.Box(c).Padding(2, 10).Radius(12).Background(w.setting.background).Border(1, w.setting.border).Children(func() {
					ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
						ui.Text(c, "场合：").FontSize(11).TextColor(w.settingLabel)
						ui.Text(c, settingText).FontSize(11).FontWeight(600).TextColor(w.setting.text)
					})
				})

				ui.Spacer(c).Grow(1)
				ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, iconPolish).Size(13, 13).TextColor(t.TextMuted)
					ui.Text(c, "批改完成").FontSize(11).TextColor(t.TextMuted)
				})
			})

			if len(res.Annotations) > 0 {
				ui.Column(c).FillWidth().Gap(6).Children(func() {
					for _, ann := range res.Annotations {
						ui.Box(c).FillWidth().Padding(8, 10).Radius(6).
							Background(t.SurfaceHover).Border(1, t.Border).Children(func() {
							ui.Column(c).FillWidth().Gap(3).Children(func() {
								ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
									ui.Text(c, ann.Text).FontSize(13).FontWeight(600).Strikethrough().TextColor(w.removed)
									ui.Text(c, "→").FontSize(12).TextColor(t.TextMuted)
									if ann.Fix != "" {
										ui.Text(c, ann.Fix).FontSize(13).FontWeight(600).TextColor(w.added)
									} else {
										ui.Text(c, "(删除)").FontSize(11).TextColor(w.deleted)
									}
									ui.Spacer(c).Grow(1)
									tagLabel := ann.Tag
									if tagLabel == "" {
										tagLabel = ann.Type
									}
									if tagLabel != "" {
										ui.Box(c).Padding(1, 6).Radius(4).Shrink(0).Background(w.tag.background).Border(1, w.tag.border).Children(func() {
											ui.Text(c, tagLabel).FontSize(10).FontWeight(600).TextColor(w.tag.text).SingleLine()
										})
									}
								})
								if ann.Note != "" {
									ui.Text(c, ann.Note).FontSize(12).TextColor(t.TextMuted).Wrap()
								}
							})
						})
					}
				})
			} else {
				ui.Box(c).FillWidth().Padding(8, 10).Radius(RadiusSM).
					Background(w.success.background).Border(1, w.success.border).Children(func() {
					ui.Row(c).Gap(Space1).AlignItems(ui.Center).Children(func() {
						ui.Icon(c, iconCheck).Size(12, 12).TextColor(w.success.text)
						ui.Text(c, "原文表达地道得体，无需修改。").FontSize(12).TextColor(w.success.text)
					})
				})
			}

			if res.Improved != "" {
				ui.Box(c).FillWidth().Padding(10, 12).Radius(6).
					Background(w.improved.background).Border(1, w.improved.border).Children(func() {
					ui.Column(c).FillWidth().Gap(6).Children(func() {
						ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
							ui.Text(c, "改后版本").FontSize(11).FontWeight(700).TextColor(w.improvedTitle)
							ui.Spacer(c).Grow(1)
							btnCopy := ui.Button(c, "").Height(24).Padding(2, 8).Radius(5).
								Background(w.copy.background).Border(1, w.copy.border).TextColor(w.copy.text).Tooltip("复制改后版本")
							btnCopy.Children(func() {
								ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
									ui.Icon(c, iconCopy).Size(12, 12).TextColor(w.copy.text)
									ui.Text(c, "复制").FontSize(11).TextColor(w.copy.text)
								})
							})
							if btnCopy.Clicked() {
								if onCopy != nil {
									onCopy(res.Improved)
								} else {
									mygo.Clipboard.WriteText(res.Improved)
								}
							}
						})
						ui.Text(c, res.Improved).FontSize(14).FontWeight(600).TextColor(w.improved.text).Wrap()
					})
				})
			}

			if res.Summary != "" {
				ui.Box(c).FillWidth().Padding(8, 10).Radius(6).
					Background(t.SurfaceHover).Children(func() {
					ui.Row(c).Gap(6).AlignItems(ui.Start).Children(func() {
						ui.Icon(c, iconInfo).Size(13, 13).TextColor(t.TextMuted)
						ui.Column(c).Grow(1).Gap(2).Children(func() {
							ui.Text(c, "导师总评").FontSize(11).FontWeight(600).TextColor(t.TextMuted)
							ui.Text(c, res.Summary).FontSize(12).TextColor(t.Text).Wrap()
						})
					})
				})
			}

			ui.Divider(c).FillWidth()

			ui.Row(c).FillWidth().Justify(ui.End).AlignItems(ui.Center).Children(func() {
				if saved {
					ui.Row(c).Height(HeightCompact).Gap(Space1).AlignItems(ui.Center).Children(func() {
						ui.Icon(c, iconCheck).Size(12, 12).TextColor(appTheme(t).TextMuted)
						ui.Text(c, "已存入笔记本").FontSize(11).FontWeight(500).TextColor(appTheme(t).TextMuted)
					})
					return
				}
				if onSave == nil {
					return
				}
				btnSave := ui.Button(c, "").Height(HeightCompact).Padding(2, Space3).Radius(RadiusSM).
					Tooltip("保存改后版本与批注到笔记本")
				saveFG := saveButtonStyle(btnSave, t)
				btnSave.Children(func() {
					ui.Row(c).Gap(Space1).AlignItems(ui.Center).Children(func() {
						ui.Icon(c, iconBookmark).Size(12, 12).TextColor(saveFG)
						ui.Text(c, "保存到笔记本").FontSize(11).FontWeight(600)
					})
				})
				if btnSave.Clicked() {
					onSave()
				}
			})
		})
	})
}

func renderHighlightedContext(c *ui.Context, t *ui.Theme, sentence, targetWord string) {
	if targetWord == "" {
		ui.Text(c, sentence).FontSize(13).TextColor(t.Text).Wrap().Grow(1)
		return
	}
	words := strings.Fields(sentence)
	targets := strings.Fields(strings.ToLower(targetWord))
	ui.Row(c).FillWidth().Wrap().Gap(3).AlignItems(ui.Center).Grow(1).Children(func() {
		for _, w := range words {
			clean := strings.ToLower(strings.Trim(w, ",.?!;:()\"'"))
			matched := false
			for _, tw := range targets {
				twClean := strings.Trim(tw, ",.?!;:()\"'")
				if clean == twClean || strings.Contains(clean, twClean) {
					matched = true
					break
				}
			}
			if matched {
				ui.Text(c, w).FontSize(13).FontWeight(700).Underline().
					DecorationColor(t.Accent).TextColor(t.Accent)
			} else {
				ui.Text(c, w).FontSize(13).TextColor(t.Text)
			}
		}
	})
}
