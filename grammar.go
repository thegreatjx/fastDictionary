package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

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

// ParseOutcome 记录解析停在哪一级。
//
// 以前只用 nil / 非 nil 表示结果，空输入、不是 JSON、只有译文没有标注这三种
// 情况在调用方看来完全一样，卡片只能静默降级，用户不知道发生了什么。
type ParseOutcome uint8

const (
	parseOutcomeEmpty ParseOutcome = iota
	parseOutcomeStructured
	parseOutcomeTranslationOnly
	parseOutcomeFallbackDirect
	parseOutcomeFallbackArray
	parseOutcomeFailed
)

func (o ParseOutcome) String() string {
	switch o {
	case parseOutcomeEmpty:
		return "empty"
	case parseOutcomeStructured:
		return "structured"
	case parseOutcomeTranslationOnly:
		return "translation_only"
	case parseOutcomeFallbackDirect:
		return "fallback_direct"
	case parseOutcomeFallbackArray:
		return "fallback_array"
	case parseOutcomeFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// SentenceParseResult 把解析结果和它的来路一起交出去。
type SentenceParseResult struct {
	Outcome ParseOutcome
	Result  *SentenceAnalysisResult
	Cause   error
}

// grammarAbbrevs 是不该结束句子的缩写。分句靠标点切分，没有这张表时
// "Mr. Smith" 会被切成两句。
var grammarAbbrevs = map[string]bool{
	"mr.": true, "mrs.": true, "ms.": true, "dr.": true, "prof.": true,
	"sr.": true, "jr.": true, "st.": true, "vs.": true, "etc.": true,
	"e.g.": true, "i.e.": true, "u.s.": true, "u.k.": true, "fig.": true,
	"no.": true, "vol.": true, "inc.": true, "ltd.": true,
}

// splitGrammarSentences 把一段选中的文字拆成句子，按段落就地处理。
//
// 长难句卡片一次只分析一句：整段发给模型时，模型会把不同句子的成分混在一张
// 卡片里，回答也慢得多（一句话 3-5 秒，一段话十几秒）。拆句用纯函数实现，
// 这样边界情况（小数、缩写、引号、中文句号）可以直接测。
func splitGrammarSentences(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	var out []string
	for _, para := range strings.Split(text, "\n") {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		runes := []rune(para)
		start := 0
		for i := 0; i < len(runes); i++ {
			r := runes[i]
			if r != '.' && r != '?' && r != '!' && r != '。' && r != '？' && r != '！' {
				continue
			}
			if r == '.' && i > 0 && i+1 < len(runes) && unicode.IsDigit(runes[i-1]) && unicode.IsDigit(runes[i+1]) {
				continue // 3.5、1.000 里的小数点不是句末
			}
			if r == '.' {
				wordStart := i
				for wordStart > start && !unicode.IsSpace(runes[wordStart-1]) {
					wordStart--
				}
				if grammarAbbrevs[strings.ToLower(string(runes[wordStart:i+1]))] {
					continue
				}
				// 单字母缩写（"J. K. Rowling"）后面跟空格时也算缩写。
				if i-wordStart == 1 && unicode.IsLetter(runes[wordStart]) && i+1 < len(runes) && runes[i+1] != ' ' {
					continue
				}
			}
			end := i + 1
			for end < len(runes) && (runes[end] == '"' || runes[end] == '\'' || runes[end] == '”' || runes[end] == '’' || runes[end] == ')' || runes[end] == '）') {
				end++
			}
			// 句号后面紧跟字母，说明这不是句末（"e.g.this"、文件路径）。
			if end < len(runes) && !unicode.IsSpace(runes[end]) && r != '。' && r != '？' && r != '！' {
				continue
			}
			if seg := strings.TrimSpace(string(runes[start:end])); seg != "" {
				out = append(out, seg)
			}
			start = end
			i = end - 1
		}
		if rest := strings.TrimSpace(string(runes[start:])); rest != "" {
			out = append(out, rest)
		}
	}
	if len(out) == 0 {
		return []string{text}
	}
	return out
}

// grammarActiveSentence 返回第 idx 句以及夹逼后的下标和总句数。
// 下标越界时夹到首尾，调用方不必自己再检查一遍。
func grammarActiveSentence(text string, idx int) (string, int, int) {
	sents := splitGrammarSentences(text)
	if len(sents) == 0 {
		return strings.TrimSpace(text), 0, 0
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sents) {
		idx = len(sents) - 1
	}
	return sents[idx], idx, len(sents)
}

// ParseSentenceAnalysis parses the model output into a structured result
func ParseSentenceAnalysis(raw string) *SentenceAnalysisResult {
	return ParseSentenceAnalysisDetailed(raw).Result
}

// ParseSentenceAnalysisDetailed 在 ParseSentenceAnalysis 的基础上保留每一级的去向。
func ParseSentenceAnalysisDetailed(raw string) SentenceParseResult {
	cleaned := CleanJSONText(raw)
	if cleaned == "" {
		return SentenceParseResult{Outcome: parseOutcomeEmpty}
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
		if len(list) > 0 {
			return SentenceParseResult{Outcome: parseOutcomeStructured, Result: &SentenceAnalysisResult{
				Annotations: list,
				Translation: trans,
				Raw:         raw,
			}}
		}
		if trans != "" {
			// 只有译文没有标注算不上完整结果，也不能当失败丢弃。
			return SentenceParseResult{Outcome: parseOutcomeTranslationOnly, Result: &SentenceAnalysisResult{
				Annotations: nil,
				Translation: trans,
				Raw:         raw,
			}}
		}
	}

	// Fallback 1: try direct unmarshal
	var res SentenceAnalysisResult
	if err := json.Unmarshal([]byte(cleaned), &res); err == nil && len(res.Annotations) > 0 {
		res.Raw = raw
		return SentenceParseResult{Outcome: parseOutcomeFallbackDirect, Result: &res}
	}

	// Fallback 2: try parsing as raw annotations array
	var list []SyntaxAnnotation
	if err := json.Unmarshal([]byte(cleaned), &list); err == nil && len(list) > 0 {
		return SentenceParseResult{Outcome: parseOutcomeFallbackArray, Result: &SentenceAnalysisResult{
			Annotations: list,
			Translation: "",
			Raw:         raw,
		}}
	}

	return SentenceParseResult{Outcome: parseOutcomeFailed, Cause: fmt.Errorf("语法解析没有得到可用的标注")}
}

// Translated badge labels matching Read-Frog zh-CN
func clauseBadgeLabel(item SyntaxAnnotation) string {
	form := strings.ToLower(strings.TrimSpace(item.Form))
	// 只有真正的从句（form=clause）才允许出现「时间状从 / 定语从句」这类徽标。
	// 以前先看 sense，模型给 Tonight、suddenly 这类副词填了 sense=time，就被
	// 标成了时间状语从句，整句看上去满是错标。
	if form == "clause" {
		switch strings.ToLower(strings.TrimSpace(item.Sense)) {
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
		switch strings.ToLower(strings.TrimSpace(item.Type)) {
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
	// 非谓语短语至少三个词才贴标签："to go"、"running" 这类两词以内的小
	// 片段贴上去只会把句子挤满徽标，反而看不出结构。
	if len(strings.Fields(item.Text)) >= 3 {
		switch form {
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

// grammarCardState is everything the grammar card reads from the app plus the
// callbacks it fires. The card used to reach into *app for each of these,
// which made it impossible to render a card in a test without a live app and
// impossible to reuse the same layout for a read-only preview.
type grammarCardState struct {
	OnlyTrunk      bool
	ShowSlashes    bool
	FilterRole     string
	HoverKnowledge string
	Prefill        bool
	SentIdx        int
	SentCount      int
	OnPrevSent     func()
	OnNextSent     func()
	OnToggleTrunk  func()
	OnToggleSlash  func()
	OnToggleRole   func(role string)
	OnSpeak        func()
	OnSave         func()
}

// renderGrammarVisualCard renders the interactive visual grammar tree card matching user screenshot
func (a *app) renderGrammarVisualCard(c *ui.Context, t *ui.Theme, origText string, res *SentenceAnalysisResult, saved bool, onSave func()) {
	// 句子切换只换下标后重跑一次当前动作：缓存按单句隔离，切回来不用再等模型。
	a.mu.Lock()
	src, curIdx := a.source, a.grammarSentIdx
	a.mu.Unlock()
	_, sentIdx, sentCount := grammarActiveSentence(src, curIdx)
	st := grammarCardState{
		OnlyTrunk:      a.onlyTrunk,
		ShowSlashes:    a.showSlashes,
		FilterRole:     a.filterRole,
		HoverKnowledge: a.grammarHoverKnowledge,
		Prefill:        a.grammarResultPrefill,
		SentIdx:        sentIdx,
		SentCount:      sentCount,
		OnPrevSent: func() {
			if sentIdx <= 0 {
				return
			}
			a.mu.Lock()
			a.grammarSentIdx = sentIdx - 1
			src := a.source
			a.mu.Unlock()
			a.runAction("default-sentence-analysis", src)
		},
		OnNextSent: func() {
			if sentIdx+1 >= sentCount {
				return
			}
			a.mu.Lock()
			a.grammarSentIdx = sentIdx + 1
			src := a.source
			a.mu.Unlock()
			a.runAction("default-sentence-analysis", src)
		},
		OnToggleTrunk: func() { a.onlyTrunk = !a.onlyTrunk },
		OnToggleSlash: func() { a.showSlashes = !a.showSlashes },
		OnToggleRole:  func(role string) { a.toggleGrammarRole(role) },
		OnSpeak:       func() { PlaySpeechLang(origText, a.speechLang(), false) },
		OnSave:        onSave,
	}
	renderGrammarVisualCardEx(c, t, origText, res, saved, st)
}

// renderGrammarVisualCardEx draws the card from an explicit state, so the same
// layout serves any caller that can supply one.
func renderGrammarVisualCardEx(c *ui.Context, t *ui.Theme, origText string, res *SentenceAnalysisResult, saved bool, st grammarCardState) {
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
		if strings.EqualFold(ann.Form, "clause") {
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
		Shadow(0, 2, 8, 0, withAlpha(appTheme(t).ShadowColor, 0.67)).Children(func() {
		ui.Column(c).FillWidth().Gap(10).Children(func() {
			// 1. Header: mode label, sentence pager and the speak button
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(Space2).Children(func() {
				topBg, topBorder, topText := grammarHeaderColors(t)
				ui.Box(c).Padding(3, 10).Radius(12).
					Background(topBg).Border(1, topBorder).Children(func() {
					ui.Text(c, "长难句拆解").FontSize(11).FontWeight(600).TextColor(topText).SingleLine()
				})

				// 多句时顶部出现分页器：每次只把当前这一句发给模型，避免多句混标。
				if st.SentCount > 1 {
					ui.Row(c).AlignItems(ui.Center).Gap(Space1).Children(func() {
						prevBtn := ui.Button(c, "‹").Height(22).Width(22).Padding(0, 0).Radius(RadiusXS).
							Center().Disabled(st.SentIdx <= 0).Tooltip("上一句")
						if prevBtn.Clicked() && st.OnPrevSent != nil {
							st.OnPrevSent()
						}
						ui.Text(c, fmt.Sprintf("第 %d / %d 句", st.SentIdx+1, st.SentCount)).
							Font(fontMono).FontFeatures("tnum").FontSize(FontMicro).
							TextColor(t.TextMuted).SingleLine()
						nextBtn := ui.Button(c, "›").Height(22).Width(22).Padding(0, 0).Radius(RadiusXS).
							Center().Disabled(st.SentIdx+1 >= st.SentCount).Tooltip("下一句")
						if nextBtn.Clicked() && st.OnNextSent != nil {
							st.OnNextSent()
						}
					})
				}

				ui.Spacer(c).Grow(1)

				btnSpeak := renderIconButton(c, iconSpeak, "朗读本句", 28, 14, false)
				if btnSpeak.Clicked() && st.OnSpeak != nil {
					st.OnSpeak()
				}
			})

			// 2. Main Sentence with Annotated Highlights (Spans & Badges)
			ui.Box(c).FillWidth().Padding(12, 14).Radius(6).Background(t.SurfaceHover).Border(1, t.Border).Children(func() {
				ui.Column(c).FillWidth().Gap(6).Children(func() {
					renderGrammarSpansWithState(c, t, origText, res, st.OnlyTrunk, st.ShowSlashes, st.FilterRole)
				})
			})

			// 2.5 只有译文没有标注时要说清楚。否则读者看到的是一张没有
			// 任何语法信息的卡片，只会以为功能坏了。
			if len(res.Annotations) == 0 {
				ui.Box(c).FillWidth().Padding(8, 12).Radius(6).
					Background(t.SurfaceHover).Border(1, t.Border).Children(func() {
					ui.Text(c, "这次回答只返回了译文，没有语法标注。可以点「AI 精翻」重新生成。").
						FontSize(12).FontWeight(500).TextColor(t.TextMuted).Wrap()
				})
			}

			// 3. Filter controls & role legend
			//
			// The toggles read as quiet outlined buttons that fill in when on,
			// and the legend is drawn in the same ink the sentence uses, so
			// what the legend promises is what the sentence shows.
			ink := grammarInkFor(t)
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Padding(2, 0, 2, 0).Children(func() {
				if grammarToggle(c, t, "只看主干", st.OnlyTrunk, "只留主句骨架，从句和修饰短语折叠成省略号").Clicked() && st.OnToggleTrunk != nil {
					st.OnToggleTrunk()
				}
				if grammarToggle(c, t, "断句", st.ShowSlashes, "在从句和意群的边界显示断句斜线").Clicked() && st.OnToggleSlash != nil {
					st.OnToggleSlash()
				}

				ui.Spacer(c).Grow(1)

				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					grammarLegend(c, t, ink, "subject", "主语", st.FilterRole, func(role string) {
						if st.OnToggleRole != nil {
							st.OnToggleRole(role)
						}
					})
					grammarLegend(c, t, ink, "predicate", "谓语", st.FilterRole, func(role string) {
						if st.OnToggleRole != nil {
							st.OnToggleRole(role)
						}
					})
					grammarLegend(c, t, ink, "object", "宾语", st.FilterRole, func(role string) {
						if st.OnToggleRole != nil {
							st.OnToggleRole(role)
						}
					})
				})
			})

			// 3.5 Live grammar knowledge card
			if st.HoverKnowledge != "" {
				ui.Box(c).FillWidth().Padding(8, 12).Radius(6).
					Background(ink.clauseTint).Border(1, t.Border).Children(func() {
					ui.Row(c).Gap(6).AlignItems(ui.Start).Children(func() {
						ui.Text(c, "·").FontSize(12).FontWeight(700).TextColor(ink.connector)
						ui.Text(c, st.HoverKnowledge).FontSize(12).FontWeight(500).
							TextColor(ink.clauseText).Wrap().Grow(1)
					})
				})
			}

			// 4. Hard structures: what makes the sentence difficult, and how to
			// put it back in plain order.
			if len(obstacles) > 0 {
				cardBg, cardBorder, cardTitle, badgeBg, badgeBorder, badgeText, obsText, restoreLabel, restoreText, noteLabel, noteText := grammarObstacleColors(t, ink)
				ui.Box(c).FillWidth().Padding(10, 12).Radius(6).
					Background(cardBg).Border(1, cardBorder).Children(func() {
					ui.Column(c).FillWidth().Gap(8).Children(func() {
						ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
							ui.Text(c, "句式难点").FontSize(11).FontWeight(700).TextColor(cardTitle)
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
										Background(badgeBg).Border(1, badgeBorder).Children(func() {
										ui.Text(c, label).FontSize(10).FontWeight(600).TextColor(badgeText)
									})
									ui.Text(c, obsItem.Text).FontSize(13).FontWeight(600).TextColor(obsText).Wrap().Grow(1)
								})
								if obsItem.Restore != "" {
									ui.Row(c).Gap(6).AlignItems(ui.Start).Padding(1, 0, 1, 14).Children(func() {
										ui.Text(c, "还原语序：").FontSize(11).FontWeight(600).TextColor(restoreLabel)
										ui.Text(c, obsItem.Restore).FontSize(12).FontWeight(600).TextColor(restoreText).Wrap().Grow(1)
									})
								}
								if obsItem.Note != "" {
									ui.Row(c).Gap(6).AlignItems(ui.Start).Padding(0, 0, 0, 14).Children(func() {
										ui.Text(c, "解析说明：").FontSize(11).TextColor(noteLabel)
										ui.Text(c, obsItem.Note).FontSize(12).TextColor(noteText).Wrap().Grow(1)
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
					if st.Prefill {
						head = "先看译文（机翻，语法分析生成中）:"
					}
					ui.Text(c, head).FontSize(11).FontWeight(600).TextColor(t.TextMuted)
					ui.Text(c, res.Translation).FontSize(14).FontWeight(500).TextColor(t.Text).Wrap()
				})
			}

			ui.Divider(c).FillWidth()

			// 7. Bottom action bar: [ 保存到长难句本 ]
			ui.Row(c).FillWidth().Justify(ui.End).AlignItems(ui.Center).Children(func() {
				if saved {
					ui.Box(c).Height(26).Padding(2, 12).Radius(5).
						Background(t.Surface).Border(1, t.Border).Children(func() {
						ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
							ui.Icon(c, iconCheckSquare).Size(12, 12).TextColor(t.Success)
							ui.Text(c, "✓ 已存入长难句本").FontSize(11).FontWeight(600).TextColor(t.TextMuted)
						})
					})
					return
				}
				if st.OnSave == nil {
					return
				}
				btnSave := ui.Button(c, "").Height(26).Padding(2, 14).Radius(5).
					Background(saveButtonColor(t)).TextColor(t.AccentText).Tooltip("将这条长难句分析保存到生词本的长难句分类")
				btnSave.Children(func() {
					ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
						ui.Icon(c, iconBookmark).Size(13, 13).TextColor(t.AccentText)
						ui.Text(c, "保存到长难句本").FontSize(11).FontWeight(600)
					})
				})
				if btnSave.Clicked() {
					st.OnSave()
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

// renderGrammarSpans draws the marked-up sentence.
//
// This is the app's own entry point; the state it reads is passed explicitly
// to renderGrammarSpansWithState, which is what the card itself calls.
func (a *app) renderGrammarSpans(c *ui.Context, t *ui.Theme, origText string, res *SentenceAnalysisResult) {
	renderGrammarSpansWithState(c, t, origText, res, a.onlyTrunk, a.showSlashes, a.filterRole)
}

// renderGrammarSpansWithState draws the marked-up sentence.
//
// Every word is rendered once, in order, inside a wrapping row of equal-height
// boxes. The equal height is what makes the row wrap predictably: a badge (a
// clause label, a star, a slash) used to make its own word taller than its
// neighbours, the row aligned those taller words somewhere else, and the line
// ends came out ragged — which is how "Chinese Exclusive" lost its right edge
// and "open-" floated above its own line.
//
// A word carries at most one mark, taken from the most specific annotation
// covering it, so the sentence reads as text with underlines rather than as a
// pile of chips. The words of one span are joined by leaving no gap between
// them, so the underlines of a phrase meet instead of showing gaps and
// mismatched stroke ends.
func renderGrammarSpansWithState(c *ui.Context, t *ui.Theme, origText string, res *SentenceAnalysisResult, onlyTrunk, showSlashes bool, filterRole string) {
	tokens := buildGrammarTokens(origText, res)
	ink := grammarInkFor(t)
	const lineBoxH float32 = 26

	ui.Row(c).FillWidth().Wrap().Gap(0).AlignItems(ui.Center).Children(func() {
		for i := 0; i < len(tokens); i++ {
			item := tokens[i]

			if onlyTrunk && !item.IsCore {
				continue
			}

			if showSlashes && item.OpenClause != "" && item.IsCore {
				ui.Box(c).Height(lineBoxH).Padding(0, 4, 0, 2).Center().Children(func() {
					ui.Text(c, "/").FontSize(14).FontWeight(700).TextColor(ink.connector).SingleLine()
				})
			}

			// The clause badge sits before the word that opens the clause: a small
			// label in the clause's tint, so nesting reads as depth.
			if item.OpenClause != "" {
				ui.Box(c).Height(lineBoxH).Padding(0, 4, 0, 0).Center().Children(func() {
					ui.Box(c).Padding(1, 5).Radius(3).Background(ink.clauseTint).
						Tooltip(grammarClauseTooltip(item.OpenClause)).Children(func() {
						ui.Text(c, item.OpenClause).FontSize(9).FontWeight(700).TextColor(ink.connector).SingleLine()
					})
				})
			}

			joinedNext := false
			if item.Role != "" && item.SpanEnd > item.Start && i+1 < len(tokens) {
				next := tokens[i+1]
				if next.SpanIndex == item.SpanIndex && next.Role == item.Role && (!onlyTrunk || next.IsCore) {
					joinedNext = true
				}
			}
			renderGrammarWordAligned(c, t, ink, item, lineBoxH, joinedNext, filterRole)
		}
		if onlyTrunk && len(tokens) > 0 {
			for _, tok := range tokens {
				if !tok.IsCore {
					ui.Box(c).Height(lineBoxH).Center().Children(func() {
						ui.Text(c, "…").FontSize(14).TextColor(ink.muted).SingleLine()
					})
					break
				}
			}
		}
	})
}

// renderGrammarWordAligned draws one word, its role's mark and its obstacle
// star inside a box of lineBoxH, so every word occupies the same vertical slot
// and the row wraps at predictable places.
//
// The mark is drawn, not styled: mygo has no dashed or double border, and the
// hand-drawn wobble that makes read-frog's card legible is a polyline, not a
// text decoration. The mark sits at the bottom of the equal-height box rather
// than below the glyphs, so the marks of a whole line stay level with each
// other.
//
// filterRole dims every word that is not the role chosen in the legend. The
// legend used to look interactive and change nothing.
func renderGrammarWordAligned(c *ui.Context, t *ui.Theme, ink grammarInk, item GrammarToken, lineBoxH float32, joinedNext bool, filterRole string) {
	kind, colour, ok := markForRole(item.Role, ink)
	filteredOut := filterRole != "" && item.Role != filterRole
	if filteredOut {
		ok = false
	}
	if !ok {
		col := ink.body
		if filteredOut {
			col = ink.muted
		} else if !item.IsCore {
			col = t.Text
		}
		weight := 400
		// The connector is drawn as coloured text rather than marked; it has no
		// underline of its own, so it must not read as plain body text.
		if item.Role == "connector" && !filteredOut {
			col = ink.connector
			weight = 600
		}
		ui.Box(c).Height(lineBoxH).Center().Children(func() {
			ui.Text(c, item.Word+" ").FontSize(14).FontWeight(weight).TextColor(col).SingleLine()
		})
		return
	}

	seed := int64(item.Start*7919 + item.End*104729 + 1)
	weight := 600
	if item.Role == "predicate" {
		weight = 700
	}

	displayWord := item.Word
	if joinedNext {
		displayWord = item.Word + " "
	}
	box := ui.Box(c).Height(lineBoxH).Padding(0, 1).Center().Tooltip(tooltipFor(item))
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		mh := markHeight(kind)
		markRect := ui.Rect{X: r.X, Y: r.Y + r.H - mh, W: r.W, H: mh}
		drawMark(p, markRect, kind, colour, seed)
	})
	box.Children(func() {
		ui.Text(c, displayWord).FontSize(14).FontWeight(weight).TextColor(ink.body).SingleLine()
	})
	if item.Obstacle != "" {
		ui.Box(c).Height(lineBoxH).Center().Children(func() {
			ui.Text(c, "★").FontSize(9).TextColor(ink.obstacle).SingleLine()
		})
	}
	if !joinedNext {
		ui.Box(c).Height(lineBoxH).Center().Children(func() {
			ui.Text(c, " ").FontSize(14).SingleLine()
		})
	}
}

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
		bg, text, border = grammarToggleOnColors(t)
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
