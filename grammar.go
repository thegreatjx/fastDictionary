package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

// SyntaxAnnotation represents one grammatical unit from Read-Frog's sentence analysis
type SyntaxAnnotation struct {
	Text       string `json:"text"`
	Type       string `json:"type"` // subject, predicate, object, complement, attributive, adverbial, appositive, connector
	Form       string `json:"form,omitempty"`
	Sense      string `json:"sense,omitempty"`
	Head       string `json:"head,omitempty"`
	Obstacle   string `json:"obstacle,omitempty"`
	Restore    string `json:"restore,omitempty"`
	Note       string `json:"note,omitempty"`
	Occurrence int    `json:"occurrence,omitempty"`
}

// SentenceAnalysisResult is the structured result returned by the grammar tutor LLM
type SentenceAnalysisResult struct {
	Annotations []SyntaxAnnotation `json:"annotations"`
	Translation string             `json:"translation"`
	Raw         string             `json:"-"`
}

// ParseSentenceAnalysis parses the model output into a structured result
func ParseSentenceAnalysis(raw string) *SentenceAnalysisResult {
	cleaned := CleanJSONText(raw)
	if cleaned == "" {
		return nil
	}

	type rawGrammar struct {
		Annotations    any    `json:"annotations"`
		AnnotationsZH  any    `json:"标注"`
		AnnotationsDol any    `json:"$1"`
		Translation    string `json:"translation"`
		TranslationZH  string `json:"译文"`
		TranslationDol string `json:"$2"`
	}

	var g rawGrammar
	if err := json.Unmarshal([]byte(cleaned), &g); err == nil {
		annVal := firstNonNil(g.Annotations, g.AnnotationsZH, g.AnnotationsDol)
		trans := firstNonEmpty(g.Translation, g.TranslationZH, g.TranslationDol)
		var list []SyntaxAnnotation
		if annVal != nil {
			if s, ok := annVal.(string); ok {
				_ = json.Unmarshal([]byte(s), &list)
			} else {
				data, _ := json.Marshal(annVal)
				_ = json.Unmarshal(data, &list)
			}
		}
		if len(list) > 0 || trans != "" {
			return &SentenceAnalysisResult{
				Annotations: list,
				Translation: trans,
				Raw:         raw,
			}
		}
	}

	// Fallback 1: try direct unmarshal
	var res SentenceAnalysisResult
	if err := json.Unmarshal([]byte(cleaned), &res); err == nil && len(res.Annotations) > 0 {
		res.Raw = raw
		return &res
	}

	// Fallback 2: try parsing as raw annotations array
	var list []SyntaxAnnotation
	if err := json.Unmarshal([]byte(cleaned), &list); err == nil && len(list) > 0 {
		return &SentenceAnalysisResult{
			Annotations: list,
			Translation: "",
			Raw:         raw,
		}
	}

	return nil
}

// Translated badge labels matching Read-Frog zh-CN
func clauseBadgeLabel(item SyntaxAnnotation) string {
	if item.Sense != "" {
		switch item.Sense {
		case "time":
			return "时间状从"
		case "place":
			return "地点状从"
		case "cause":
			return "原因状从"
		case "condition":
			return "条件状从"
		case "concession":
			return "让步状从"
		case "purpose":
			return "目的状从"
		case "result":
			return "结果状从"
		case "manner":
			return "方式状从"
		case "comparison":
			return "比较状从"
		}
	}
	if item.Form == "clause" {
		switch item.Type {
		case "subject":
			return "主语从句"
		case "object":
			return "宾语从句"
		case "complement":
			return "表语从句"
		case "attributive":
			return "定语从句"
		case "adverbial":
			return "状语从句"
		case "appositive":
			return "同位语从句"
		default:
			return "从句"
		}
	}
	if item.Form != "" {
		switch item.Form {
		case "infinitive":
			return "不定式"
		case "gerund":
			return "动名词"
		case "present-participle":
			return "现在分词"
		case "past-participle":
			return "过去分词"
		}
	}
	return ""
}

// obstacleBadgeLabel maps obstacle keywords to Chinese tags
func obstacleBadgeLabel(item SyntaxAnnotation) string {
	switch item.Obstacle {
	case "passive":
		return "被动"
	case "inversion":
		return "倒装"
	case "fronting":
		return "前置"
	case "ellipsis":
		return "省略"
	case "subjunctive":
		return "虚拟"
	case "dummy-it":
		return "形式主语"
	case "postponed-subject":
		return "真主语"
	case "cleft":
		return "强调句"
	case "parenthesis":
		return "插入语"
	case "split":
		return "拆分动词"
	case "idiom":
		return "习语表达"
	case "comparison":
		return "比较结构"
	case "negation":
		return "否定结构"
	}
	return ""
}

var grammarExplanations = map[string]string{
	"subject":    "【主语 (Subject)】：句子的陈述主体，动作的发起者或状态的承担者。主干核心成分之一。",
	"predicate":  "【谓语 (Predicate)】：说明主语“做什么”或“处于什么状态”，由动词构成。深蓝波浪线标注核心谓语动词。",
	"object":     "【宾语 (Object)】：动作的承受者或及物动词支配的对象。主干核心成分之一。",
	"connector":  "【连接词 (Connector)】：引导从句并起语法衔接纽带作用（如 that, which, if, because 等）。",
	"complement": "【表语/补语 (Complement)】：紧跟系动词，补充说明主语或宾语的性质、特征或状态。",
	"表语从句":       "【表语从句 (Predicative Clause)】：位于系动词后充当表语，详细解释主语的内容或性质。",
	"宾语从句":       "【宾语从句 (Object Clause)】：位于及物动词或介词后充当宾语，说明主句主语所想、所说或感知的事实。",
	"定语从句":       "【定语从句 (Attributive Clause)】：修饰先行词（名词或代词），起到形容词作用，限定或补充说明人或事物。",
	"主语从句":       "【主语从句 (Subject Clause)】：在复合句中充当主语，常用 it 作形式主语，而将真主语从句后置。",
	"同位语从句":      "【同位语从句 (Appositive Clause)】：位于抽象名词后，对其具体内容作完整阐释。",
	"条件状从":       "【条件状语从句 (Condition)】：通常由 if, unless 引导，表示主句动作发生的条件或假设。",
	"时间状从":       "【时间状语从句 (Time)】：由 when, while, as, before, after 引导，说明动作发生的时间背景。",
	"原因状从":       "【原因状语从句 (Cause)】：由 because, since, as 引导，说明动作产生的根源或理由。",
	"让步状从":       "【让步状语从句 (Concession)】：由 although, though, even if 引导，表示转折让步。",
	"结果状从":       "【结果状语从句 (Result)】：由 so...that 引导，说明前述原因直接导致的结果。",
	"目的状从":       "【目的状语从句 (Purpose)】：由 so that, in order that 引导，表示主句动作要达到的目的。",
	"被动":         "【被动语态 (Passive Voice)】：结构为 be + 过去分词，强调动作的承受者而非发出者。",
	"倒装":         "【倒装句 (Inversion)】：谓语动词或助动词置于主语之前，用于加强语气或承接上下文。",
	"前置":         "【成分前置 (Fronting)】：将宾语、状语或表语提到句首，起到突出强调或话题承接作用。",
	"省略":         "【省略句 (Ellipsis)】：省略句中重复或显而易见的成分，使表达更加精炼紧凑。",
	"虚拟":         "【虚拟语气 (Subjunctive)】：动词用过去时或 should+动词原形，表示非真实假设、愿望或建议。",
	"形式主语":       "【形式主语 (Dummy-it)】：用 it 占据句首主语位置，避免头重脚轻，真主语（从句或不定式）后置。",
}

// trunkUnit is one annotation of the sentence's trunk, kept together with where
// it sits in the sentence. Order alone is not enough: the model lists
// annotations in the order it noticed them, so pairing "the first subject" with
// "the first predicate" produced lines like "主语: This ➔ 谓语: is ➔ 宾语: you"
// for a sentence whose real object was "a slide".
type trunkUnit struct {
	text string
	pos  int
}

// trunkLine builds the one-line summary of the main clause: the first verb in
// the sentence, the subject closest in front of it, and the object closest
// after it.
func trunkLine(subjects, predicates, objects []trunkUnit) string {
	if len(predicates) == 0 {
		return ""
	}
	main := predicates[0]
	for _, p := range predicates {
		if p.pos >= 0 && (main.pos < 0 || p.pos < main.pos) {
			main = p
		}
	}
	if main.pos < 0 {
		// The verb could not be located in the sentence, so nothing can be
		// paired by position; fall back to the order the model listed.
		line := "谓语: " + main.text
		if len(subjects) > 0 {
			line = "主语: " + subjects[0].text + "  ➔  " + line
		}
		if len(objects) > 0 {
			line += "  ➔  宾语: " + objects[0].text
		}
		return line
	}

	// A subject or object belongs to this verb only if no other verb stands
	// between them: without that bound the search reaches across clauses and
	// "This is a slide. I am confident..." picked up "you" from two clauses
	// later as the object of "is".
	lower, upper := -1, -1
	for _, p := range predicates {
		if p.pos < 0 || p.pos == main.pos {
			continue
		}
		if p.pos < main.pos && p.pos > lower {
			lower = p.pos
		}
		if p.pos > main.pos && (upper < 0 || p.pos < upper) {
			upper = p.pos
		}
	}

	line := ""
	if subj, ok := nearestIn(subjects, lower, main.pos, false); ok {
		line += "主语: " + subj + "  ➔  "
	}
	line += "谓语: " + main.text
	if obj, ok := nearestIn(objects, main.pos, upper, true); ok {
		line += "  ➔  宾语: " + obj
	}
	return line
}

// nearestIn returns the annotation sitting closest to the verb inside the gap
// between the neighbouring verbs: backwards for the subject, forwards for the
// object. Either bound may be -1, meaning unbounded.
func nearestIn(units []trunkUnit, from, to int, forward bool) (string, bool) {
	best, bestPos := "", -1
	for _, u := range units {
		if u.pos < 0 || u.pos <= from {
			continue
		}
		if to >= 0 && u.pos >= to {
			continue
		}
		if bestPos < 0 || (forward && u.pos < bestPos) || (!forward && u.pos > bestPos) {
			best, bestPos = u.text, u.pos
		}
	}
	return best, bestPos >= 0
}

// renderGrammarVisualCard renders the interactive visual grammar tree card matching user screenshot
func (a *app) renderGrammarVisualCard(c *ui.Context, t *ui.Theme, origText string, res *SentenceAnalysisResult, onSave func()) {
	// 1. Collect obstacles and structural components
	var obstacles []SyntaxAnnotation
	var subjects []trunkUnit
	var predicates []trunkUnit
	var objects []trunkUnit
	var clauses []SyntaxAnnotation

	for _, ann := range res.Annotations {
		if ann.Obstacle != "" || ann.Restore != "" {
			obstacles = append(obstacles, ann)
		}
		if ann.Form == "clause" {
			clauses = append(clauses, ann)
			continue
		}
		unit := trunkUnit{text: ann.Text, pos: indexOfOccurrence(origText, ann.Text, ann.Occurrence)}
		switch strings.ToLower(ann.Type) {
		case "subject":
			subjects = append(subjects, unit)
		case "predicate":
			predicates = append(predicates, unit)
		case "object", "complement":
			objects = append(objects, unit)
		}
	}

	ui.Box(c).FillWidth().Padding(14, 16).Radius(8).
		Background(t.Surface).Border(1, t.Border).
		Shadow(0, 2, 8, 0, ui.RGBA(0, 0, 0, 0.08)).Children(func() {
		ui.Column(c).FillWidth().Gap(10).Children(func() {
			// 1. Header: mode label and the speak button
			ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
				ui.Box(c).Padding(3, 10).Radius(12).
					Background(ui.Hex("#F1F5F9")).Border(1, ui.Hex("#E2E8F0")).Children(func() {
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Text(c, "长难句拆解").FontSize(11).FontWeight(600).TextColor(ui.Hex("#475569"))
					})
				})

				ui.Spacer(c).Grow(1)

				btnSpeak := renderIconButton(c, iconSpeak, "朗读整句", 28, 14, false)
				if btnSpeak.Clicked() {
					PlaySpeechLang(origText, a.speechLang(), false)
				}
			})

			// 2. Main Sentence with Annotated Highlights (Spans & Badges)
			ui.Box(c).FillWidth().Padding(12, 14).Radius(6).Background(t.SurfaceHover).Border(1, t.Border).Children(func() {
				ui.Column(c).FillWidth().Gap(6).Children(func() {
					a.renderGrammarSpans(c, t, origText, res)
				})
			})

			// 3. Filter controls & role legend
			//
			// The toggles read as quiet outlined buttons that fill in when on,
			// and the legend is drawn in the same ink the sentence uses, so
			// what the legend promises is what the sentence shows.
			ink := grammarInkFor(t)
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Padding(2, 0, 2, 0).Children(func() {
				if grammarToggle(c, t, "只看主干", a.onlyTrunk, "只留主句骨架，从句和修饰短语折叠成省略号").Clicked() {
					a.onlyTrunk = !a.onlyTrunk
				}
				if grammarToggle(c, t, "断句", a.showSlashes, "在从句和意群的边界显示断句斜线").Clicked() {
					a.showSlashes = !a.showSlashes
				}

				ui.Spacer(c).Grow(1)

				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					grammarLegend(c, t, ink, "subject", "主语", a.filterRole, func(role string) {
						a.toggleGrammarRole(role)
					})
					grammarLegend(c, t, ink, "predicate", "谓语", a.filterRole, func(role string) {
						a.toggleGrammarRole(role)
					})
					grammarLegend(c, t, ink, "object", "宾语", a.filterRole, func(role string) {
						a.toggleGrammarRole(role)
					})
				})
			})

			// 3.5 Live grammar knowledge card
			if a.grammarHoverKnowledge != "" {
				ui.Box(c).FillWidth().Padding(8, 12).Radius(6).
					Background(ink.clauseTint).Border(1, t.Border).Children(func() {
					ui.Row(c).Gap(6).AlignItems(ui.Start).Children(func() {
						ui.Text(c, "·").FontSize(12).FontWeight(700).TextColor(ink.connector)
						ui.Text(c, a.grammarHoverKnowledge).FontSize(12).FontWeight(500).
							TextColor(ink.clauseText).Wrap().Grow(1)
					})
				})
			}

			// 4. Hard structures: what makes the sentence difficult, and how to
			// put it back in plain order.
			if len(obstacles) > 0 {
				ui.Box(c).FillWidth().Padding(10, 12).Radius(6).
					Background(ui.Hex("#FAF6F4")).Border(1, ui.Hex("#E8DDD8")).Children(func() {
					ui.Column(c).FillWidth().Gap(8).Children(func() {
						ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
							ui.Text(c, "句式难点").FontSize(11).FontWeight(700).TextColor(ui.Hex("#8A5044"))
						})
						for _, obs := range obstacles {
							obsItem := obs
							label := obstacleBadgeLabel(obsItem)
							if label == "" {
								label = obsItem.Obstacle
							}
							if label == "" {
								// The item qualified through "restore" alone: the
								// model reordered a span without naming an
								// obstacle. An empty badge rendered as a small
								// blank box, which reads as a rendering fault.
								label = "语序"
							}
							ui.Column(c).FillWidth().Gap(3).Padding(2, 4).Children(func() {
								ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
									ui.Box(c).Padding(1, 6).Radius(4).
										Background(ui.Hex("#F0E4DF")).Border(1, ui.Hex("#E0CFC8")).Children(func() {
										ui.Text(c, label).FontSize(10).FontWeight(600).TextColor(ui.Hex("#8A5044"))
									})
									ui.Text(c, obsItem.Text).FontSize(13).FontWeight(600).TextColor(ui.Hex("#3F3A38")).Wrap().Grow(1)
								})
								if obsItem.Restore != "" {
									ui.Row(c).Gap(6).AlignItems(ui.Start).Padding(1, 0, 1, 14).Children(func() {
										ui.Text(c, "还原语序：").FontSize(11).FontWeight(600).TextColor(ui.Hex("#A26B5C"))
										ui.Text(c, obsItem.Restore).FontSize(12).FontWeight(600).TextColor(ui.Hex("#55708C")).Wrap().Grow(1)
									})
								}
								if obsItem.Note != "" {
									ui.Row(c).Gap(6).AlignItems(ui.Start).Padding(0, 0, 0, 14).Children(func() {
										ui.Text(c, "解析说明：").FontSize(11).TextColor(ui.Hex("#9A8478"))
										ui.Text(c, obsItem.Note).FontSize(12).TextColor(ui.Hex("#5C524C")).Wrap().Grow(1)
									})
								}
							})
						}
					})
				})
			}
			// 5. Trunk summary: the core of the sentence on one line, and every
			// clause listed so it can be read without hunting through the markup.
			if len(predicates) > 0 || len(clauses) > 0 {
				ui.Box(c).FillWidth().Padding(8, 12).Radius(6).
					Background(t.SurfaceHover).Children(func() {
					ui.Column(c).FillWidth().Gap(4).Children(func() {
						if len(predicates) > 0 {
							// The trunk line shows the main clause only. It used
							// to join every subject and predicate in the whole
							// sentence with " / ", which on a compound sentence
							// read as "主语: she / she / She / her arms / You"
							// — six subjects for one trunk, and no way to tell
							// which verb belonged to which. The rest are already
							// listed under 从句拆解.
							trunkText := trunkLine(subjects, predicates, objects)
							ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
								ui.Text(c, "主干核心:").FontSize(11).FontWeight(600).TextColor(t.TextMuted)
								ui.Text(c, trunkText).FontSize(12).FontWeight(600).TextColor(t.Text).Wrap().Grow(1)
							})
							// Say how much was left out, so the line is not read
							// as the whole sentence's structure.
							if extra := len(predicates) + len(clauses) - 1; extra > 0 {
								ui.Text(c, fmt.Sprintf("另有 %d 处谓语或从句，见下方从句拆解。", extra)).
									FontSize(11).TextColor(t.TextMuted)
							}
						}
						if len(clauses) > 0 {
							var clauseStrs []string
							for _, cl := range clauses {
								b := clauseBadgeLabel(cl)
								if b == "" {
									b = "从句"
								}
								s := b + ": " + cl.Text
								if cl.Head != "" {
									s += " (修饰: " + cl.Head + ")"
								}
								clauseStrs = append(clauseStrs, s)
							}
							ui.Row(c).Gap(6).AlignItems(ui.Start).Children(func() {
								ui.Text(c, "从句拆解:").FontSize(11).FontWeight(600).TextColor(t.TextMuted)
								ui.Text(c, strings.Join(clauseStrs, " · ")).FontSize(12).TextColor(t.TextMuted).Wrap().Grow(1)
							})
						}
					})
				})
			}

			ui.Divider(c).FillWidth()

			// 6. Natural Chinese Translation
			if res.Translation != "" {
				ui.Column(c).FillWidth().Gap(4).Children(func() {
					// While the annotations are still streaming, this line is the
					// machine engines' translation, not the model's. Saying so
					// stops it reading as the finished article when the rest of
					// the card is still empty.
					head := "地道精翻:"
					if a.grammarResultPrefill {
						head = "先看译文（机翻，语法分析生成中）:"
					}
					ui.Text(c, head).FontSize(11).FontWeight(600).TextColor(t.TextMuted)
					ui.Text(c, res.Translation).FontSize(14).FontWeight(500).TextColor(t.Text).Wrap()
				})
			}

			ui.Divider(c).FillWidth()

			// 7. Bottom action bar: [ 保存到长难句本 ]
			ui.Row(c).FillWidth().Justify(ui.End).AlignItems(ui.Center).Children(func() {
				saveLabel := "保存到长难句本"
				saveBg := ui.Hex("#55708C")
				if a.grammarSaved {
					saveLabel = "✓ 已存入长难句本"
					saveBg = ui.Hex("#3F6E5A")
				}
				btnSave := ui.Button(c, "").Height(26).Padding(2, 14).Radius(5).
					Background(saveBg).TextColor(ui.Hex("#FFFFFF")).Tooltip("将这条长难句分析保存到生词本的长难句分类")
				btnSave.Children(func() {
					ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
						ui.Icon(c, iconBookmark).Size(13, 13).TextColor(ui.Hex("#FFFFFF"))
						ui.Text(c, saveLabel).FontSize(11).FontWeight(600)
					})
				})
				if btnSave.Clicked() {
					if onSave != nil {
						onSave()
					}
					a.mu.Lock()
					a.grammarSaved = true
					a.mu.Unlock()
					if a.win != nil {
						a.win.Update(func() {})
					}
				}
			})
		})
	})
}

// GrammarToken represents a single word with mapped syntax information
type GrammarToken struct {
	Word       string
	CleanWord  string
	Start      int
	End        int
	OpenClause string
	Role       string
	RoleLabel  string
	Obstacle   string
	HoverTip   string
	IsCore     bool
	// SpanStart and SpanEnd mark where the annotation covering this word
	// begins and ends. A multi-word annotation is one span, and the mark under
	// it has to be drawn as one continuous line: drawing each word separately
	// leaves visible gaps and a row of mismatched stroke ends, which is what
	// made "Stage 4 cancer patient" look like three broken underlines.
	SpanStart int
	SpanEnd   int
	// SpanIndex distinguishes two annotations that cover identical text.
	SpanIndex int
}

type clauseSpan struct {
	start int
	end   int
	label string
}

type roleSpan struct {
	start    int
	end      int
	role     string
	label    string
	obstacle string
	restore  string
}

// indexOfOccurrence finds where a span sits in the sentence.
//
// strings.Index alone returns the first match, so in "She hands you a bottle.
// She smiles." both "She" annotations landed on the first word: the second
// subject was drawn under the first one, and the tokens between them lost their
// marks. The model already reports "occurrence" for exactly this case; this is
// the reader for it.
func indexOfOccurrence(text, span string, occurrence int) int {
	if span == "" {
		return -1
	}
	if occurrence <= 1 {
		return strings.Index(text, span)
	}
	from := 0
	for n := 1; n <= occurrence; n++ {
		idx := strings.Index(text[from:], span)
		if idx < 0 {
			// The model counted more appearances than the sentence has.
			// Falling back to the first keeps the annotation visible instead
			// of dropping it.
			return strings.Index(text, span)
		}
		abs := from + idx
		if n == occurrence {
			return abs
		}
		from = abs + len(span)
	}
	return -1
}

// buildGrammarTokens calculates exact, non-duplicating syntax spans
func buildGrammarTokens(origText string, res *SentenceAnalysisResult) []GrammarToken {
	if res == nil {
		return nil
	}

	// 1. Gather all clause boundaries
	var clauses []clauseSpan
	for _, ann := range res.Annotations {
		badge := clauseBadgeLabel(ann)
		if badge != "" || ann.Form == "clause" {
			if badge == "" {
				badge = "从句"
			}
			idx := indexOfOccurrence(origText, ann.Text, ann.Occurrence)
			if idx >= 0 {
				clauses = append(clauses, clauseSpan{
					start: idx,
					end:   idx + len(ann.Text),
					label: badge,
				})
			}
		}
	}

	// 2. Gather constituent role boundaries (word or short phrase)
	var roles []roleSpan
	for _, ann := range res.Annotations {
		if ann.Form == "clause" {
			continue // clauses handled above
		}
		rType := strings.ToLower(ann.Type)
		if rType == "subject" || rType == "predicate" || rType == "object" || rType == "connector" || rType == "complement" {
			idx := indexOfOccurrence(origText, ann.Text, ann.Occurrence)
			if idx >= 0 {
				rLabel := "成分"
				switch rType {
				case "subject":
					rLabel = "主语"
				case "predicate":
					rLabel = "谓语"
				case "object":
					rLabel = "宾语"
				case "connector":
					rLabel = "连接词"
				case "complement":
					rLabel = "表语/补语"
				}
				roles = append(roles, roleSpan{
					start:    idx,
					end:      idx + len(ann.Text),
					role:     rType,
					label:    rLabel,
					obstacle: obstacleBadgeLabel(ann),
					restore:  ann.Restore,
				})
			}
		}
	}

	// 3. Tokenize words with precise character offsets
	var tokens []GrammarToken
	words := strings.Fields(origText)
	searchPos := 0
	clauseEmitted := make(map[int]bool)

	for _, w := range words {
		pos := strings.Index(origText[searchPos:], w)
		if pos < 0 {
			continue
		}
		absStart := searchPos + pos
		absEnd := absStart + len(w)
		searchPos = absEnd

		clean := strings.Trim(w, ",.?!;:()\"'")

		tok := GrammarToken{
			Word:      w,
			CleanWord: clean,
			Start:     absStart,
			End:       absEnd,
		}

		// Emit clause badge ONCE at the start of that clause
		for cIdx, c := range clauses {
			if absStart <= c.start && absEnd > c.start && !clauseEmitted[cIdx] {
				tok.OpenClause = c.label
				clauseEmitted[cIdx] = true
			}
		}

		// Check role overlap. The token records the whole span it belongs to,
		// not just its own bounds, so the renderer can tell a span's first
		// word from its middle and draw one continuous mark underneath.
		for ri, r := range roles {
			if absStart < r.end && absEnd > r.start {
				tok.Role = r.role
				tok.RoleLabel = r.label
				tok.Obstacle = r.obstacle
				tok.IsCore = (r.role == "subject" || r.role == "predicate" || r.role == "object")
				tok.SpanStart = r.start
				tok.SpanEnd = r.end
				tok.SpanIndex = ri

				tip := r.label
				if r.obstacle != "" {
					tip += " ★" + r.obstacle
				}
				if r.restore != "" {
					tip += " (还原语序: " + r.restore + ")"
				}
				tok.HoverTip = tip
				break
			}
		}

		tokens = append(tokens, tok)
	}

	return tokens
}

// grammarInk is the grammar card's palette.
//
// Unlike the selection chip, which is a passive affordance and stays quiet,
// this card is a teaching aid: its whole job is to make eight grammatical roles
// tell themselves apart at a glance, and muted greys cannot do that. The hues
// follow read-frog's grammar card (layout-engine presets.js: --rf-sa-blue
// #2a57c4, --rf-sa-green #177a53, --rf-sa-red #c43d27, --rf-sa-faded #8c94a3),
// which is what a reader of that tool already recognises.
//
// Each role is distinguished by KIND as well as hue, so the card still works
// for a reader who cannot separate the colours:
//
//	subject     solid underline, slate
//	predicate   wavy underline, blue        (the verb is what the sentence turns on)
//	object      double underline, ink       (heavier than subject: the receiver)
//	complement  dotted underline, grey      (a predicate about the subject)
//	connector   no line, green bold         (a joint, not a part)
//	clause      green tint block, deeper per nesting level
type grammarInk struct {
	subjectLine    ui.Color
	predicateLine  ui.Color
	objectLine     ui.Color
	complementLine ui.Color
	connector      ui.Color
	clauseTint     ui.Color
	clauseTint2    ui.Color
	clauseText     ui.Color
	obstacle       ui.Color
	obstacleBg     ui.Color
	body           ui.Color
	muted          ui.Color
}

func grammarInkFor(t *ui.Theme) grammarInk {
	if t != nil && t.Dark {
		return grammarInk{
			subjectLine:    ui.Hex("#5EEAD4"),
			predicateLine:  ui.Hex("#86A6FF"),
			objectLine:     ui.Hex("#FDBA74"),
			complementLine: ui.Hex("#C4B5FD"),
			connector:      ui.Hex("#5CCB99"),
			clauseTint:     ui.RGBA(92, 203, 153, 0.13),
			clauseTint2:    ui.RGBA(92, 203, 153, 0.20),
			clauseText:     ui.Hex("#A7B4C4"),
			obstacle:       ui.Hex("#FF8A70"),
			obstacleBg:     ui.RGBA(255, 138, 112, 0.12),
			body:           ui.Hex("#E4EAF1"),
			muted:          ui.Hex("#7D8595"),
		}
	}
	return grammarInk{
		subjectLine:    ui.Hex("#0E7C86"),
		predicateLine:  ui.Hex("#2A57C4"),
		objectLine:     ui.Hex("#A21CAF"),
		complementLine: ui.Hex("#6D28D9"),
		connector:      ui.Hex("#15803D"),
		clauseTint:     ui.RGBA(23, 122, 83, 0.08),
		clauseTint2:    ui.RGBA(23, 122, 83, 0.13),
		clauseText:     ui.Hex("#4B5563"),
		obstacle:       ui.Hex("#C43D27"),
		obstacleBg:     ui.RGBA(196, 61, 39, 0.08),
		body:           ui.Hex("#1E293B"),
		muted:          ui.Hex("#8C94A3"),
	}
}

// renderGrammarSpans draws the marked-up sentence.
//
// Every word is rendered once, in order. A word carries at most one mark, taken
// from the most specific annotation covering it, so the sentence reads as text
// with underlines rather than as a pile of chips. Clause openings add a small
// label above the word that starts them.
func (a *app) renderGrammarSpans(c *ui.Context, t *ui.Theme, origText string, res *SentenceAnalysisResult) {
	tokens := buildGrammarTokens(origText, res)
	ink := grammarInkFor(t)

	ui.Row(c).FillWidth().Wrap().Gap(4).AlignItems(ui.End).Children(func() {
		for i := 0; i < len(tokens); i++ {
			item := tokens[i]

			if a.onlyTrunk && !item.IsCore {
				continue
			}

			if a.showSlashes && item.OpenClause != "" && item.IsCore {
				ui.Text(c, "/").FontSize(14).FontWeight(700).TextColor(ink.connector)
			}

			// A run of words covered by the same annotation is one span, and
			// its mark is one line. Drawing per word left gaps and mismatched
			// stroke ends, so "Stage 4 cancer patient" read as three separate
			// underlines instead of one.
			if item.Role != "" && item.SpanEnd > item.Start {
				run := make([]GrammarToken, 0, 4)
				j := i
				for ; j < len(tokens); j++ {
					next := tokens[j]
					if next.SpanIndex != item.SpanIndex || next.Role != item.Role {
						break
					}
					if a.onlyTrunk && !next.IsCore {
						break
					}
					run = append(run, next)
				}
				if len(run) > 1 {
					renderGrammarSpan(c, t, ink, run)
					i = j - 1
					continue
				}
			}

			renderGrammarWord(c, t, ink, item)
		}
		if a.onlyTrunk && len(tokens) > 0 {
			for _, tok := range tokens {
				if !tok.IsCore {
					ui.Text(c, "…").FontSize(14).TextColor(ink.muted)
					break
				}
			}
		}
	})
}

// renderGrammarSpan draws several consecutive words as one marked span.
//
// The words keep their own spacing, but the mark is a single continuous line
// under the whole run, which is what makes a phrase read as one unit of the
// sentence rather than as a string of separately annotated words.
func renderGrammarSpan(c *ui.Context, t *ui.Theme, ink grammarInk, run []GrammarToken) {
	kind, colour, ok := markForRole(run[0].Role, ink)
	if !ok {
		for _, item := range run {
			renderGrammarWord(c, t, ink, item)
		}
		return
	}
	seed := int64(run[0].SpanStart*7919 + run[0].SpanEnd*104729 + 1)

	box := ui.Box(c).Padding(0, 1).Tooltip(tooltipFor(run[0]))
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		markRect := ui.Rect{X: r.X, Y: r.Y + r.H + 1, W: r.W, H: markHeight(kind)}
		drawMark(p, markRect, kind, colour, seed)
	})
	box.Children(func() {
		ui.Row(c).Gap(4).AlignItems(ui.End).Children(func() {
			for _, item := range run {
				weight := 600
				if item.Role == "predicate" {
					weight = 700
				}
				ui.Text(c, item.Word+" ").FontSize(14).FontWeight(weight).TextColor(ink.body)
			}
		})
	})
}

// renderGrammarWord draws one word with its role's mark.
//
// The mark is drawn, not styled: mygo has no dashed or double border, and the
// hand-drawn wobble that makes read-frog's card legible is a polyline, not a
// text decoration. Each word gets a Draw callback that strokes its role's mark
// along the bottom of the word's own box.
func renderGrammarWord(c *ui.Context, t *ui.Theme, ink grammarInk, item GrammarToken) {
	word := item.Word

	// The clause badge sits before the word that opens the clause: a small
	// label in the clause's tint, so nesting reads as depth.
	if item.OpenClause != "" {
		ui.Box(c).Padding(1, 5).Radius(3).Background(ink.clauseTint).
			Tooltip(grammarClauseTooltip(item.OpenClause)).Children(func() {
			ui.Text(c, item.OpenClause).FontSize(9).FontWeight(700).TextColor(ink.connector)
		})
	}

	kind, colour, ok := markForRole(item.Role, ink)
	if !ok {
		if item.IsCore {
			ui.Text(c, word+" ").FontSize(14).TextColor(ink.body)
		} else {
			ui.Text(c, word+" ").FontSize(14).TextColor(t.Text)
		}
		return
	}

	seed := int64(item.Start*7919 + item.End*104729 + 1)
	weight := 600
	if item.Role == "predicate" {
		weight = 700
	}

	ui.Row(c).AlignItems(ui.Center).Children(func() {
		box := ui.Box(c).Padding(0, 1).Tooltip(tooltipFor(item))
		// The mark sits just below the glyphs, not on them. The Draw box is the
		// element's content box, whose bottom edge is roughly the baseline, so
		// the mark is offset down by a couple of DIPs and given enough height
		// for the wave to swing without touching the letters.
		box.Draw(func(p *ui.Painter, r ui.Rect) {
			markRect := ui.Rect{X: r.X, Y: r.Y + r.H + 1, W: r.W, H: markHeight(kind)}
			drawMark(p, markRect, kind, colour, seed)
		})
		box.Children(func() {
			txt := ui.Text(c, word).FontSize(14).FontWeight(weight).TextColor(ink.body)
			_ = txt
		})
		if item.Obstacle != "" {
			ui.Text(c, "★").FontSize(9).TextColor(ink.obstacle)
		}
		ui.Text(c, " ").FontSize(14)
	})
}

// markForRole maps a grammar role onto its line style and colour.
func markForRole(role string, ink grammarInk) (markKind, ui.Color, bool) {
	switch role {
	case "subject":
		return markUnderline, ink.subjectLine, true
	case "predicate":
		return markWavy, ink.predicateLine, true
	case "object":
		return markDouble, ink.objectLine, true
	case "complement":
		return markDotted, ink.complementLine, true
	case "connector":
		return markUnderline, ink.connector, false // drawn as coloured text
	}
	return markUnderline, ink.muted, false
}

// tooltipFor builds a word's hover text: what it is, and anything that made it
// hard to read.
func tooltipFor(item GrammarToken) string {
	if item.HoverTip != "" {
		return item.HoverTip
	}
	if item.RoleLabel != "" {
		return item.RoleLabel
	}
	return item.Word
}

// grammarClauseTooltip returns the explanation for a clause kind, falling back
// to the label itself when the map has no entry.
func grammarClauseTooltip(kind string) string {
	if exp, ok := grammarExplanations[kind]; ok {
		return exp
	}
	return "从句: " + kind
}

// grammarToggle draws one of the card's on/off switches. On fills with the
// quiet accent, off stays an outline: the two states differ by more than tint,
// which is what makes them readable at a glance on a light card.
func grammarToggle(c *ui.Context, t *ui.Theme, label string, on bool, tip string) ui.Element {
	bg := ui.Transparent
	text := t.Text
	border := t.Border
	if on {
		bg = ui.Hex("#55708C")
		text = ui.Hex("#FFFFFF")
		border = ui.Hex("#4A6E8A")
	}
	btn := ui.Button(c, "").Height(24).Padding(2, 10).Radius(5).
		Background(bg).TextColor(text).Border(1, border).Tooltip(tip)
	btn.Children(func() {
		ui.Text(c, label).FontSize(11).FontWeight(600).TextColor(text)
	})
	return btn
}

// grammarLegend draws one role as a legend chip, in the same mark the sentence
// uses: a line of the role's colour under the role's name.
func grammarLegend(c *ui.Context, t *ui.Theme, ink grammarInk, role, label, active string, onClick func(role string)) {
	bg := ui.Transparent
	border := t.Border
	if active == role {
		bg = ink.clauseTint
		border = ink.subjectLine
	}
	btn := ui.Button(c, "").Height(22).Padding(1, 6).Radius(4).
		Background(bg).Border(1, border).Tooltip("点击在句中高亮所有" + label)
	btn.Children(func() {
		switch role {
		case "predicate":
			ui.Text(c, label).FontSize(11).FontWeight(600).
				WavyUnderline().DecorationColor(ink.predicateLine).TextColor(ink.body)
		case "object":
			ui.Text(c, label).FontSize(11).FontWeight(600).
				Underline().DecorationColor(ink.objectLine).TextColor(ink.body)
		default:
			ui.Text(c, label).FontSize(11).FontWeight(600).
				Underline().DecorationColor(ink.subjectLine).TextColor(ink.body)
		}
	})
	if btn.Clicked() {
		onClick(role)
	}
}

// toggleGrammarRole turns the role filter on, or off when it is already on, and
// keeps the knowledge card in step with it.
func (a *app) toggleGrammarRole(role string) {
	if a.filterRole == role {
		a.filterRole = ""
		a.grammarHoverKnowledge = ""
		return
	}
	a.filterRole = role
	a.grammarHoverKnowledge = grammarExplanations[role]
}
