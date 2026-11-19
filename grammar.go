package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

type SegmentRole string

const (
	RoleSubject   SegmentRole = "subject"
	RolePredicate SegmentRole = "predicate"
	RoleObject    SegmentRole = "object"
	RoleConnector SegmentRole = "connector"
	RoleModifier  SegmentRole = "modifier"
	RolePunct     SegmentRole = "punct"
)

type Segment struct {
	Text        string      `json:"text"`
	Role        SegmentRole `json:"role"`
	IsBackbone  bool        `json:"is_backbone"`
	BreakBefore bool        `json:"break_before,omitempty"`
	ClauseID    string      `json:"clause_id,omitempty"`
	PhraseID    string      `json:"phrase_id,omitempty"`
	Note        string      `json:"note,omitempty"`
}

type ClauseInfo struct {
	ID       string `json:"id"`
	ParentID string `json:"parent_id,omitempty"`
	Type     string `json:"type"`
	ShortTag string `json:"short_tag"`
	Target   string `json:"target,omitempty"`
}

type PhraseInfo struct {
	ID       string `json:"id"`
	ParentID string `json:"parent_id,omitempty"`
	Type     string `json:"type"`
	ShortTag string `json:"short_tag"`
	Role     string `json:"role,omitempty"`
	Target   string `json:"target,omitempty"`
}

type DifficultyPoint struct {
	Title       string `json:"title"`
	SpanText    string `json:"span_text"`
	Explanation string `json:"explanation"`
}

type SentenceAnalysisResult struct {
	BackboneSummary string            `json:"backbone_summary"`
	Segments        []Segment         `json:"segments"`
	Clauses         []ClauseInfo      `json:"clauses"`
	Phrases         []PhraseInfo      `json:"phrases,omitempty"`
	Difficulties    []DifficultyPoint `json:"difficulties,omitempty"`
	Translation     string            `json:"translation"`
	Raw             string            `json:"-"`
}

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

type SentenceParseResult struct {
	Outcome ParseOutcome
	Result  *SentenceAnalysisResult
	Cause   error
}

var grammarAbbrevs = map[string]bool{
	"mr.": true, "mrs.": true, "ms.": true, "dr.": true, "prof.": true,
	"sr.": true, "jr.": true, "st.": true, "vs.": true, "etc.": true,
	"e.g.": true, "i.e.": true, "u.s.": true, "u.k.": true, "fig.": true,
	"no.": true, "vol.": true, "inc.": true, "ltd.": true,
}

func splitGrammarSentences(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	var out []string
	for _, para := range grammarParagraphs(text) {
		runes := []rune(para)
		start := 0
		for i := 0; i < len(runes); i++ {
			r := runes[i]
			if r != '.' && r != '?' && r != '!' && r != '。' && r != '？' && r != '！' {
				continue
			}
			if r == '.' && i > 0 && i+1 < len(runes) && unicode.IsDigit(runes[i-1]) && unicode.IsDigit(runes[i+1]) {
				continue
			}
			if r == '.' {
				wordStart := i
				for wordStart > start && !unicode.IsSpace(runes[wordStart-1]) {
					wordStart--
				}
				if grammarAbbrevs[strings.ToLower(string(runes[wordStart:i+1]))] {
					continue
				}
				if i-wordStart == 1 && unicode.IsLetter(runes[wordStart]) && i+1 < len(runes) && runes[i+1] != ' ' {
					continue
				}
			}
			end := i + 1
			for end < len(runes) && (runes[end] == '"' || runes[end] == '\'' || runes[end] == '”' || runes[end] == '’' || runes[end] == ')' || runes[end] == '）') {
				end++
			}
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

func grammarParagraphs(text string) []string {
	var out []string
	var paragraph []string
	flush := func() {
		if len(paragraph) == 0 {
			return
		}
		out = append(out, strings.Join(paragraph, " "))
		paragraph = nil
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			flush()
			continue
		}
		paragraph = append(paragraph, line)
	}
	flush()
	return out
}

func grammarAnalysisText(text string) string {
	sents := splitGrammarSentences(text)
	if len(sents) == 0 {
		return strings.TrimSpace(text)
	}
	return strings.Join(sents, " ")
}

func ParseSentenceAnalysis(raw string, original ...string) *SentenceAnalysisResult {
	return ParseSentenceAnalysisDetailed(raw, original...).Result
}

func ParseSentenceAnalysisDetailed(raw string, original ...string) SentenceParseResult {
	cleaned := CleanJSONText(raw)
	if cleaned == "" {
		return SentenceParseResult{Outcome: parseOutcomeEmpty}
	}
	if !strings.HasPrefix(cleaned, "{") {
		start := strings.Index(cleaned, "{")
		end := strings.LastIndex(cleaned, "}")
		if start >= 0 && end > start {
			cleaned = cleaned[start : end+1]
		}
	}

	var res SentenceAnalysisResult
	if err := json.Unmarshal([]byte(cleaned), &res); err != nil {
		return SentenceParseResult{Outcome: parseOutcomeFailed, Cause: fmt.Errorf("语法解析 JSON 无效: %w", err)}
	}
	res.Raw = raw
	res.Translation = flattenTranslation(res.Translation)
	normalizeClauses(&res)

	if len(res.Segments) > 0 {
		source := ""
		if len(original) > 0 {
			source = strings.TrimSpace(original[0])
		}
		aligned, ok := alignSegmentsToSource(res.Segments, source)
		if ok {
			res.Segments = aligned
			return SentenceParseResult{Outcome: parseOutcomeStructured, Result: &res}
		}
		res.Segments = nil
		if res.Translation != "" || len(res.Difficulties) > 0 || len(res.Clauses) > 0 || len(res.Phrases) > 0 {
			return SentenceParseResult{Outcome: parseOutcomeFallbackDirect, Result: &res}
		}
		return SentenceParseResult{Outcome: parseOutcomeFailed, Cause: fmt.Errorf("语法标注与原文无法对齐")}
	}
	if res.Translation != "" {
		return SentenceParseResult{Outcome: parseOutcomeTranslationOnly, Result: &res}
	}
	return SentenceParseResult{Outcome: parseOutcomeFailed, Cause: fmt.Errorf("语法解析没有得到可用的标注")}
}

func normalizeClauses(res *SentenceAnalysisResult) {
	for i := range res.Segments {
		res.Segments[i].Role = normalizeSegmentRole(res.Segments[i].Role)
		res.Segments[i].ClauseID = strings.TrimSpace(res.Segments[i].ClauseID)
		res.Segments[i].PhraseID = strings.TrimSpace(res.Segments[i].PhraseID)
		res.Segments[i].Note = strings.TrimSpace(res.Segments[i].Note)
	}
	clauses := make([]ClauseInfo, 0, len(res.Clauses))
	seen := make(map[string]bool)
	dropped := make(map[string]bool)
	for i, c := range res.Clauses {
		c.ID = strings.TrimSpace(c.ID)
		if c.ID == "" {
			c.ID = fmt.Sprintf("c%d", i+1)
		}
		if seen[c.ID] {
			continue
		}
		seen[c.ID] = true
		c.ParentID = strings.TrimSpace(c.ParentID)
		c.Type = strings.TrimSpace(c.Type)
		c.ShortTag = strings.TrimSpace(c.ShortTag)
		c.Target = strings.TrimSpace(c.Target)
		if strings.Contains(c.Type, "主句") && !strings.Contains(c.Type, "从句") {
			dropped[c.ID] = true
			continue
		}
		clauses = append(clauses, c)
	}
	for i := range clauses {
		if dropped[clauses[i].ParentID] {
			clauses[i].ParentID = ""
		}
	}
	for i := range res.Segments {
		if dropped[res.Segments[i].ClauseID] {
			res.Segments[i].ClauseID = ""
		}
	}
	active := ""
	for i := range res.Segments {
		seg := &res.Segments[i]
		if seg.ClauseID != "" {
			active = seg.ClauseID
		} else if active != "" && !seg.IsBackbone && seg.Role != RolePunct {
			seg.ClauseID = active
		}
		if seg.IsBackbone || grammarEndsSentence(seg.Text) {
			active = ""
		}
	}
	res.Clauses = clauses

	phrases := make([]PhraseInfo, 0, len(res.Phrases))
	seenPhrase := make(map[string]bool, len(res.Phrases))
	for i, p := range res.Phrases {
		p.ID = strings.TrimSpace(p.ID)
		if p.ID == "" {
			p.ID = fmt.Sprintf("p%d", i+1)
		}
		if seenPhrase[p.ID] {
			continue
		}
		seenPhrase[p.ID] = true
		p.ParentID = strings.TrimSpace(p.ParentID)
		p.Type = strings.TrimSpace(p.Type)
		p.ShortTag = strings.TrimSpace(p.ShortTag)
		p.Role = strings.TrimSpace(p.Role)
		p.Target = strings.TrimSpace(p.Target)
		phrases = append(phrases, p)
	}
	res.Phrases = phrases

	for i := range res.Difficulties {
		res.Difficulties[i].Title = strings.TrimSpace(res.Difficulties[i].Title)
		res.Difficulties[i].SpanText = strings.TrimSpace(res.Difficulties[i].SpanText)
		res.Difficulties[i].Explanation = strings.TrimSpace(res.Difficulties[i].Explanation)
	}
}

func grammarEndsSentence(text string) bool {
	text = strings.TrimRightFunc(text, unicode.IsSpace)
	if text == "" {
		return false
	}
	r, _ := utf8.DecodeLastRuneInString(text)
	return r == '.' || r == '?' || r == '!' || r == '。' || r == '？' || r == '！'
}

func normalizeSegmentRole(role SegmentRole) SegmentRole {
	switch SegmentRole(strings.ToLower(strings.TrimSpace(string(role)))) {
	case RoleSubject:
		return RoleSubject
	case RolePredicate:
		return RolePredicate
	case RoleObject:
		return RoleObject
	case RoleConnector:
		return RoleConnector
	case RolePunct:
		return RolePunct
	default:
		return RoleModifier
	}
}

func alignSegmentsToSource(segments []Segment, source string) ([]Segment, bool) {
	if len(segments) == 0 {
		return nil, false
	}
	if strings.TrimSpace(source) == "" {
		return segments, true
	}
	if concatSegments(segments) == source {
		return segments, true
	}
	if out, ok := alignSegmentsExact(segments, source); ok {
		return out, true
	}
	return alignSegmentsFuzzy(segments, source)
}

func alignSegmentsExact(segments []Segment, source string) ([]Segment, bool) {
	runes := []rune(source)
	positions := make([]int, 0, len(runes))
	for i, r := range runes {
		if !unicode.IsSpace(r) {
			positions = append(positions, i)
		}
	}
	out := make([]Segment, 0, len(segments))
	cursor, end := 0, 0
	for _, seg := range segments {
		nonSpace := nonSpaceRunes(seg.Text)
		if len(nonSpace) == 0 {
			continue
		}
		if cursor+len(nonSpace) > len(positions) {
			return nil, false
		}
		for i, r := range nonSpace {
			if runes[positions[cursor+i]] != r {
				return nil, false
			}
		}
		rebuilt := seg
		rebuilt.Text = string(runes[end : positions[cursor+len(nonSpace)-1]+1])
		out = append(out, rebuilt)
		end = positions[cursor+len(nonSpace)-1] + 1
		cursor += len(nonSpace)
	}
	if cursor != len(positions) || len(out) == 0 {
		return nil, false
	}
	if end < len(runes) {
		out[len(out)-1].Text += string(runes[end:])
	}
	if concatSegments(out) != source {
		return nil, false
	}
	return out, true
}

type grammarSourceToken struct {
	text       string
	start, end int
}

func grammarTokenSpans(text string) []grammarSourceToken {
	runes := []rune(text)
	out := make([]grammarSourceToken, 0, len(runes)/3)
	for i := 0; i < len(runes); {
		r := runes[i]
		if unicode.IsSpace(r) {
			i++
			continue
		}
		start := i
		switch {
		case unicode.Is(unicode.Han, r):
			i++
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			for i < len(runes) &&
				(unicode.IsLetter(runes[i]) || unicode.IsDigit(runes[i])) &&
				!unicode.Is(unicode.Han, runes[i]) {
				i++
			}
		default:
			i++
		}
		out = append(out, grammarSourceToken{text: string(runes[start:i]), start: start, end: i})
	}
	return out
}

func grammarFoldToken(text string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '‘', '’', '‚', '‛':
			return '\''
		case '“', '”', '„', '‟':
			return '"'
		case '–', '—', '−', '‐', '‑':
			return '-'
		}
		return unicode.ToLower(r)
	}, text)
}

func alignSegmentsFuzzy(segments []Segment, source string) ([]Segment, bool) {
	srcTokens := grammarTokenSpans(source)
	if len(srcTokens) == 0 {
		return nil, false
	}
	type modelToken struct {
		seg int
	}
	var modelTokens []modelToken
	byFold := make(map[string][]int)
	for si, seg := range segments {
		for _, tok := range grammarTokenSpans(seg.Text) {
			fold := grammarFoldToken(tok.text)
			byFold[fold] = append(byFold[fold], len(modelTokens))
			modelTokens = append(modelTokens, modelToken{seg: si})
		}
	}
	if len(modelTokens) == 0 {
		return nil, false
	}
	used := make([]bool, len(modelTokens))
	segFor := make([]int, len(srcTokens))
	for i := range segFor {
		segFor[i] = -1
	}
	last, matched := -1, 0
	for ti, tok := range srcTokens {
		best, bestCost := -1, 0
		for _, mi := range byFold[grammarFoldToken(tok.text)] {
			if used[mi] {
				continue
			}
			cost := mi - last
			if cost < 0 {
				cost = -cost + len(modelTokens)
			}
			if best < 0 || cost < bestCost {
				best, bestCost = mi, cost
			}
		}
		if best < 0 {
			continue
		}
		used[best] = true
		segFor[ti] = modelTokens[best].seg
		last = best
		matched++
	}
	if matched*2 < len(srcTokens) {
		return nil, false
	}
	for i := range segFor {
		if segFor[i] >= 0 {
			continue
		}
		prev, next := -1, -1
		for j := i - 1; j >= 0; j-- {
			if segFor[j] >= 0 {
				prev = segFor[j]
				break
			}
		}
		for j := i + 1; j < len(segFor); j++ {
			if segFor[j] >= 0 {
				next = segFor[j]
				break
			}
		}
		if prev >= 0 {
			segFor[i] = prev
		} else {
			segFor[i] = next
		}
	}
	runes := []rune(source)
	out := make([]Segment, 0, len(segments)+2)
	appendRun := func(segIdx, start, end int) {
		if start >= end || strings.TrimSpace(string(runes[start:end])) == "" {
			return
		}
		if segIdx < 0 || segIdx >= len(segments) {
			out = append(out, Segment{Role: RoleModifier, Text: string(runes[start:end])})
			return
		}
		seg := segments[segIdx]
		seg.Text = string(runes[start:end])
		out = append(out, seg)
	}
	start := 0
	runSeg := segFor[0]
	for i := 1; i < len(srcTokens); i++ {
		if segFor[i] == runSeg {
			continue
		}
		appendRun(runSeg, start, srcTokens[i].start)
		start = srcTokens[i].start
		runSeg = segFor[i]
	}
	appendRun(runSeg, start, len(runes))
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

func concatSegments(segments []Segment) string {
	var b strings.Builder
	for _, seg := range segments {
		b.WriteString(seg.Text)
	}
	return b.String()
}

func nonSpaceRunes(s string) []rune {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if !unicode.IsSpace(r) {
			out = append(out, r)
		}
	}
	return out
}

func grammarRoleLabel(role SegmentRole) string {
	switch role {
	case RoleSubject:
		return "主语"
	case RolePredicate:
		return "谓语"
	case RoleObject:
		return "宾语"
	case RoleConnector:
		return "连接词"
	case RolePunct:
		return "标点"
	default:
		return "修饰语"
	}
}

func grammarClauseLabel(info ClauseInfo) string {
	if tag := strings.TrimSpace(info.ShortTag); tag != "" {
		return tag
	}
	switch {
	case strings.Contains(info.Type, "宾语"):
		return "宾从"
	case strings.Contains(info.Type, "表语"):
		return "表从"
	case strings.Contains(info.Type, "主语"):
		return "主从"
	case strings.Contains(info.Type, "定语"):
		return "定从"
	case strings.Contains(info.Type, "同位"):
		return "同位"
	case strings.Contains(info.Type, "状语"):
		return "状从"
	default:
		return "从句"
	}
}

var grammarExplanations = map[string]string{
	"表语从句":  "【表语从句 (Predicative Clause)】：位于系动词后充当表语，详细解释主语的内容或性质。",
	"宾语从句":  "【宾语从句 (Object Clause)】：位于及物动词或介词后充当宾语，说明主句主语所想、所说或感知的事实。",
	"定语从句":  "【定语从句 (Attributive Clause)】：修饰先行词（名词或代词），起到形容词作用，限定或补充说明人或事物。",
	"主语从句":  "【主语从句 (Subject Clause)】：在复合句中充当主语，常用 it 作形式主语，而将真主语从句后置。",
	"同位语从句": "【同位语从句 (Appositive Clause)】：位于抽象名词后，对其具体内容作完整阐释。",
	"条件状从":  "【条件状语从句 (Condition)】：通常由 if, unless 引导，表示主句动作发生的条件或假设。",
	"时间状从":  "【时间状语从句 (Time)】：由 when, while, as, before, after 引导，说明动作发生的时间背景。",
	"原因状从":  "【原因状语从句 (Cause)】：由 because, since, as 引导，说明动作产生的根源或理由。",
	"让步状从":  "【让步状语从句 (Concession)】：由 although, though, even if 引导，表示转折让步。",
	"结果状从":  "【结果状语从句 (Result)】：由 so...that 引导，说明前述原因直接导致的结果。",
	"目的状从":  "【目的状语从句 (Purpose)】：由 so that, in order that 引导，表示主句动作要达到的目的。",
}

func grammarClauseTooltip(info ClauseInfo) string {
	if exp, ok := grammarExplanations[info.Type]; ok {
		return exp
	}
	if exp, ok := grammarExplanations[grammarClauseLabel(info)+"从句"]; ok {
		return exp
	}
	if info.Type != "" {
		return "【" + info.Type + "】" + info.Target
	}
	return "从句"
}

type grammarInk struct {
	body          ui.Color
	predicate     ui.Color
	predicateLine ui.Color
	muted         ui.Color
	connector     ui.Color
	clauseTint    ui.Color
	clauseText    ui.Color
}

type grammarCardState struct {
	OnlyTrunk     bool
	ShowSlashes   bool
	ShowTree      bool
	Prefill       bool
	SentenceCount int
	ClauseCount   int
	PhraseCount   int
	OnToggleTrunk func()
	OnToggleSlash func()
	OnToggleTree  func()
	OnSpeak       func()
	OnSave        func()
}

func (a *app) renderGrammarVisualCard(c *ui.Context, t *ui.Theme, origText string, res *SentenceAnalysisResult, saved bool, onSave func()) {
	st := grammarCardState{
		OnlyTrunk:   a.onlyTrunk,
		ShowSlashes: a.showSlashes,
		ShowTree:    a.showTree,
		Prefill:     a.grammarResultPrefill,
		OnToggleTrunk: func() {
			a.onlyTrunk = !a.onlyTrunk
			if a.onlyTrunk {
				a.showTree = false
			}
		},
		OnToggleSlash: func() { a.showSlashes = !a.showSlashes },
		OnToggleTree: func() {
			a.showTree = !a.showTree
			if a.showTree {
				a.onlyTrunk = false
			}
		},
		OnSpeak: func() { PlaySpeechLang(origText, a.speechLang(), false) },
		OnSave:  onSave,
	}
	renderGrammarVisualCardEx(c, t, origText, res, saved, st)
}

func renderGrammarVisualCardEx(c *ui.Context, t *ui.Theme, origText string, res *SentenceAnalysisResult, saved bool, st grammarCardState) {
	at := appTheme(t)
	ink := grammarInkFor(t)
	st.SentenceCount = len(splitGrammarSentences(origText))
	st.ClauseCount = len(res.Clauses)
	st.PhraseCount = len(res.Phrases)

	ui.Box(c).FillWidth().Padding(Space3, Space4).Radius(RadiusTrack).
		Background(at.Surface).Border(1, at.BorderSubtle).Children(func() {
		ui.Column(c).FillWidth().Gap(Space2).Children(func() {
			renderGrammarCardHeader(c, t, saved, st)

			ui.Box(c).FillWidth().Padding(12, 14).Radius(RadiusSM).
				Background(at.SurfaceHover).Border(1, at.BorderSubtle).Children(func() {
				ui.Column(c).FillWidth().Gap(6).Children(func() {
					switch {
					case len(res.Segments) == 0:
						renderGrammarFallbackSentence(c, t, origText, res, st)
					case st.ShowTree:
						renderGrammarTreeView(c, t, at, ink, res, st)
					default:
						renderGrammarSentence(c, t, res, st)
					}
				})
			})

			if len(res.Segments) > 0 {
				renderGrammarControls(c, t, ink, st)
			}

			if res.Translation != "" {
				renderGrammarTranslation(c, t, at, res.Translation, st.Prefill)
			}

			if len(res.Difficulties) > 0 {
				renderGrammarDifficulties(c, t, at, res.Difficulties)
			}
		})
	})
}

func renderGrammarCardHeader(c *ui.Context, t *ui.Theme, saved bool, st grammarCardState) {
	at := appTheme(t)
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(Space2).Children(func() {
		scope := grammarScopeLabel(st)
		if scope == "" {
			scope = "句法拆解"
		}
		ui.Box(c).Padding(2, Space2).Radius(RadiusXS).
			Background(at.SurfaceHover).Border(1, at.BorderSubtle).Children(func() {
			ui.Text(c, scope).Font(fontMono).FontFeatures("tnum").FontSize(FontMicro).
				FontWeight(600).TextColor(at.TextSec).SingleLine()
		})

		ui.Spacer(c).Grow(1)

		btnSpeak := renderIconButton(c, iconSpeak, "朗读整段", 28, 14, false)
		if btnSpeak.Clicked() && st.OnSpeak != nil {
			st.OnSpeak()
		}
		if st.Prefill {
			ui.Row(c).Height(HeightCompact).Gap(Space1).AlignItems(ui.Center).Children(func() {
				ui.Spinner(c).Size(12, 12).TextColor(at.Accent)
				ui.Text(c, "句法分析中…").FontSize(FontMicro).FontWeight(500).TextColor(at.TextMuted).SingleLine()
			})
			return
		}
		renderGrammarHeaderSave(c, t, at, saved, st.OnSave)
	})
}

func renderGrammarHeaderSave(c *ui.Context, t *ui.Theme, at AppTheme, saved bool, onSave func()) {
	if saved {
		ui.Row(c).Height(HeightCompact).Gap(Space1).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconCheck).Size(12, 12).TextColor(at.TextMuted)
			ui.Text(c, "已存入句法笔记").FontSize(FontMicro).FontWeight(500).TextColor(at.TextMuted).SingleLine()
		})
		return
	}
	if onSave == nil {
		return
	}
	btnSave := ui.Button(c, "").Height(HeightCompact).Padding(0, Space3).Radius(RadiusSM).
		Tooltip("将这条长难句拆解存入句法笔记")
	saveFG := saveButtonStyle(btnSave, t)
	btnSave.Children(func() {
		ui.Row(c).Gap(Space1).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconBookmark).Size(12, 12).TextColor(saveFG)
			ui.Text(c, "存入句法笔记").FontSize(FontMicro).FontWeight(600).SingleLine()
		})
	})
	if btnSave.Clicked() {
		onSave()
	}
}

func grammarScopeLabel(st grammarCardState) string {
	parts := make([]string, 0, 3)
	if st.SentenceCount > 0 {
		parts = append(parts, fmt.Sprintf("%d 句", st.SentenceCount))
	}
	if st.ClauseCount > 0 {
		parts = append(parts, fmt.Sprintf("%d 处从句", st.ClauseCount))
	}
	if st.PhraseCount > 0 {
		parts = append(parts, fmt.Sprintf("%d 处短语", st.PhraseCount))
	}
	return strings.Join(parts, " · ")
}

func renderGrammarSentence(c *ui.Context, t *ui.Theme, res *SentenceAnalysisResult, st grammarCardState) {
	at := appTheme(t)
	ink := grammarInkFor(t)
	clauseByID := make(map[string]ClauseInfo, len(res.Clauses))
	for _, clause := range res.Clauses {
		clauseByID[clause.ID] = clause
	}
	phraseByID := make(map[string]PhraseInfo, len(res.Phrases))
	for _, phrase := range res.Phrases {
		phraseByID[phrase.ID] = phrase
	}

	items := grammarSentenceItems(res.Segments, st.OnlyTrunk)
	var segmentEls []ui.Element
	var segmentData []Segment
	var decorEls []ui.Element
	var decorClause []string
	var decorPhrase []string
	var foldEls []ui.Element
	var foldClause []string
	var foldPhrase []string
	hoveredIndex := -1
	hoveredFoldIndex := -1
	hoveredClause := ""
	hoveredPhrase := ""
	hoverText := ""

	ui.RichText(c).FillWidth().LineHeight(1.80).Selectable().Children(func() {
		seenClause := make(map[string]bool)
		activePhrase := ""
		activePhraseClause := ""
		var activePhraseInfo *PhraseInfo
		activePhraseOwnParens := false
		lastEndsSpace := true

		closePhrase := func(trail string) {
			if activePhrase == "" {
				return
			}
			if !activePhraseOwnParens {
				bracketCol := at.TextMuted
				if activePhraseInfo != nil {
					bracketCol = grammarPhrasePalette(t, grammarPhraseShortTag(*activePhraseInfo)).bracket
				}
				el := ui.Text(c, ")").FontSize(13).FontWeight(700).TextColor(bracketCol).Unselectable()
				decorEls = append(decorEls, el)
				decorClause = append(decorClause, activePhraseClause)
				decorPhrase = append(decorPhrase, activePhrase)
			}
			activePhrase = ""
			activePhraseClause = ""
			activePhraseInfo = nil
			activePhraseOwnParens = false
			if trail != "" {
				ui.Text(c, trail).Unselectable()
				lastEndsSpace = true
			} else {
				lastEndsSpace = false
			}
		}

		for itemIndex, item := range items {
			first := res.Segments[item.segments[0]]
			if !item.fold && activePhrase != "" && first.PhraseID != activePhrase {
				closePhrase("")
			}
			if item.fold && activePhrase != "" {
				closePhrase("")
			}

			slashed := false
			if st.ShowSlashes && itemIndex > 0 && first.BreakBefore {
				decorEls = append(decorEls, renderGrammarSlash(c, at))
				decorClause = append(decorClause, "")
				decorPhrase = append(decorPhrase, "")
				lastEndsSpace = true
				slashed = true
			}

			if item.fold {
				runText := segmentsText(res.Segments, item.segments)
				var clause *ClauseInfo
				var phrase *PhraseInfo
				for _, idx := range item.segments {
					if id := res.Segments[idx].ClauseID; id != "" && clause == nil {
						if info, ok := clauseByID[id]; ok {
							clause = &info
						} else {
							clause = &ClauseInfo{ID: id, Type: "从句", ShortTag: "从句"}
						}
					}
					if id := res.Segments[idx].PhraseID; id != "" && phrase == nil {
						if info, ok := phraseByID[id]; ok {
							phrase = &info
						}
					}
				}
				if strings.TrimSpace(runText) == "" {
					ui.Text(c, runText).Unselectable()
					lastEndsSpace = strings.HasSuffix(runText, " ")
					continue
				}
				prefix := ""
				if !lastEndsSpace && leadingSpace(runText) != "" {
					prefix = leadingSpace(runText)
				}
				el := renderGrammarFoldMarker(c, t, clause, phrase, runText, prefix)
				foldEls = append(foldEls, el)
				foldID, pID := "", ""
				if clause != nil {
					foldID = clause.ID
				}
				if phrase != nil {
					pID = phrase.ID
				}
				foldClause = append(foldClause, foldID)
				foldPhrase = append(foldPhrase, pID)
				if tail := trailingSpace(runText); tail != "" {
					ui.Text(c, tail).Unselectable()
					lastEndsSpace = true
				} else {
					lastEndsSpace = false
				}
				if el.Hovered() {
					hoverText = grammarFoldNote(clause, phrase, runText)
					hoveredClause = "fold"
					if clause != nil {
						hoveredClause = clause.ID
					} else if phrase != nil {
						hoveredPhrase = phrase.ID
					}
					hoveredFoldIndex = len(foldEls) - 1
				}
				continue
			}

			seg := res.Segments[item.segments[0]]
			var clause *ClauseInfo
			if seg.ClauseID != "" {
				if info, ok := clauseByID[seg.ClauseID]; ok {
					clause = &info
				} else {
					clause = &ClauseInfo{ID: seg.ClauseID, Type: "从句", ShortTag: "从句"}
				}
				if !seenClause[seg.ClauseID] {
					decorEls = append(decorEls, renderGrammarClauseBadge(c, t, *clause))
					decorClause = append(decorClause, seg.ClauseID)
					decorPhrase = append(decorPhrase, "")
					seenClause[seg.ClauseID] = true
				}
			}

			var phrase *PhraseInfo
			if seg.PhraseID != "" {
				if info, ok := phraseByID[seg.PhraseID]; ok {
					phrase = &info
				} else {
					phrase = &PhraseInfo{ID: seg.PhraseID, Type: "修饰短语", ShortTag: "短语"}
				}
			}

			segText := seg.Text
			if phrase != nil && seg.PhraseID != activePhrase {
				lead := leadingSpace(segText)
				segText = strings.TrimPrefix(segText, lead)
				if lead != "" && !slashed && itemIndex > 0 && !lastEndsSpace {
					ui.Text(c, lead).Unselectable()
				}
				pc := grammarPhrasePalette(t, grammarPhraseShortTag(*phrase))
				ownParens := phraseSpanHasOuterParens(res.Segments, items, itemIndex, seg.PhraseID)
				if !ownParens {
					openEl := ui.Text(c, "(").FontSize(13).FontWeight(700).TextColor(pc.bracket).Unselectable()
					decorEls = append(decorEls, openEl)
					decorClause = append(decorClause, seg.ClauseID)
					decorPhrase = append(decorPhrase, seg.PhraseID)
				}

				badgeEl := renderGrammarPhraseBadge(c, t, *phrase)
				decorEls = append(decorEls, badgeEl)
				decorClause = append(decorClause, seg.ClauseID)
				decorPhrase = append(decorPhrase, seg.PhraseID)
				if strings.TrimSpace(segText) != "" {
					gapEl := ui.Text(c, " ").FontSize(13).Unselectable()
					if seg.ClauseID != "" && clause != nil {
						gapEl.Background(grammarClausePalette(t, grammarClauseLabel(*clause)).wash)
					} else {
						gapEl.Background(pc.wash)
					}
					decorEls = append(decorEls, gapEl)
					decorClause = append(decorClause, seg.ClauseID)
					decorPhrase = append(decorPhrase, seg.PhraseID)
				}

				activePhrase = seg.PhraseID
				activePhraseClause = seg.ClauseID
				activePhraseInfo = phrase
				activePhraseOwnParens = ownParens
			}

			isLastInPhrase := phrase != nil && (itemIndex+1 >= len(items) || items[itemIndex+1].fold || res.Segments[items[itemIndex+1].segments[0]].PhraseID != seg.PhraseID)
			phraseTrail := ""
			if isLastInPhrase {
				phraseTrail = trailingSpace(segText)
				segText = strings.TrimSuffix(segText, phraseTrail)
			}

			el := renderGrammarSegmentText(c, t, at, ink, seg, segText, clause, phrase)
			lastEndsSpace = strings.HasSuffix(segText, " ")
			if el.Hovered() {
				hoveredIndex = len(segmentEls)
				hoveredClause = seg.ClauseID
				hoveredPhrase = seg.PhraseID
				hoverText = grammarSegmentNote(seg, clause, phrase)
			}
			segmentEls = append(segmentEls, el)
			segmentData = append(segmentData, seg)

			if isLastInPhrase {
				closePhrase(phraseTrail)
			}
		}
		closePhrase("")

		if hoveredIndex >= 0 || hoveredClause != "" || hoveredPhrase != "" || hoveredFoldIndex >= 0 {
			for i, el := range segmentEls {
				keep := hoveredIndex == i
				if hoveredClause != "" && segmentData[i].ClauseID == hoveredClause {
					keep = true
				}
				if hoveredClause == "" && hoveredPhrase != "" && segmentData[i].PhraseID == hoveredPhrase {
					keep = true
				}
				if !keep {
					el.Opacity(0.35)
				}
			}
			for i, el := range decorEls {
				if hoveredClause != "" && decorClause[i] == hoveredClause {
					continue
				}
				if hoveredClause == "" && hoveredPhrase != "" && decorPhrase[i] == hoveredPhrase {
					continue
				}
				el.Opacity(0.35)
			}
			for i, el := range foldEls {
				if i == hoveredFoldIndex {
					continue
				}
				if hoveredClause != "" && foldClause[i] == hoveredClause {
					continue
				}
				if hoveredClause == "" && hoveredPhrase != "" && foldPhrase[i] == hoveredPhrase {
					continue
				}
				el.Opacity(0.35)
			}
		}
	})

	if hoverText == "" {
		hoverText = grammarDefaultHint(res, st.Prefill)
	}
	renderGrammarHintBar(c, t, at, hoverText)
}

func renderGrammarSegmentText(c *ui.Context, t *ui.Theme, at AppTheme, ink grammarInk, seg Segment, text string, clause *ClauseInfo, phrase *PhraseInfo) ui.Element {
	el := ui.Text(c, text).FontSize(14)
	if seg.ClauseID != "" && clause != nil {
		el.Background(grammarClausePalette(t, grammarClauseLabel(*clause)).wash)
	}

	switch {
	case seg.IsBackbone && seg.Role == RolePredicate:
		el.TextColor(ink.predicate).FontWeight(700).
			Underline().DecorationColor(ink.predicateLine).
			DecorationThickness(1.2).UnderlineOffset(3)
	case seg.IsBackbone && seg.Role == RoleConnector:
		el.TextColor(ink.connector).FontWeight(600)
	case seg.IsBackbone:
		el.TextColor(ink.body).FontWeight(600)
	case seg.Role == RolePunct:
		el.TextColor(ink.muted)
	case seg.Role == RoleConnector:
		el.TextColor(ink.connector).FontWeight(500)
	case seg.ClauseID != "" && clause != nil:
		if seg.Role == RoleModifier {
			el.TextColor(ink.clauseText).FontWeight(500)
		} else {
			el.TextColor(ink.body).FontWeight(500)
		}
	case seg.PhraseID != "" && phrase != nil:
		pc := grammarPhrasePalette(t, grammarPhraseShortTag(*phrase))
		el.TextColor(pc.text).FontWeight(500)
		if seg.ClauseID == "" {
			el.Background(pc.wash)
		}
		if isNonFinitePhrase(*phrase) {
			el.DottedUnderline().DecorationColor(pc.underline).
				DecorationThickness(1.2).UnderlineOffset(3)
		}
	default:
		el.TextColor(ink.muted).FontWeight(500)
	}

	if note := grammarSegmentTooltip(seg, clause, phrase); note != "" {
		el.Tooltip(note)
	}
	return el
}

func phraseSpanHasOuterParens(segments []Segment, items []grammarSentenceItem, startIdx int, phraseID string) bool {
	var b strings.Builder
	for i := startIdx; i < len(items); i++ {
		if items[i].fold {
			break
		}
		first := segments[items[i].segments[0]]
		if first.PhraseID != phraseID {
			break
		}
		for _, idx := range items[i].segments {
			b.WriteString(segments[idx].Text)
		}
	}
	trimmed := strings.TrimSpace(b.String())
	if len(trimmed) < 2 {
		return false
	}
	startsParen := strings.HasPrefix(trimmed, "(") || strings.HasPrefix(trimmed, "（")
	endsParen := strings.HasSuffix(trimmed, ")") || strings.HasSuffix(trimmed, "）")
	return startsParen && endsParen
}

func grammarPhraseShortTag(p PhraseInfo) string {
	if s := strings.TrimSpace(p.ShortTag); s != "" {
		return s
	}
	switch {
	case strings.Contains(p.Type, "分词"):
		return "分词"
	case strings.Contains(p.Type, "不定式"):
		return "不定式"
	case strings.Contains(p.Type, "动名"):
		return "动名"
	case strings.Contains(p.Type, "同位"):
		return "同位"
	case strings.Contains(p.Type, "插入"):
		return "插入"
	default:
		return "介短"
	}
}

func isNonFinitePhrase(p PhraseInfo) bool {
	tag := grammarPhraseShortTag(p)
	return strings.Contains(tag, "分词") || strings.Contains(tag, "不定式") || strings.Contains(tag, "动名") ||
		strings.Contains(p.Type, "分词") || strings.Contains(p.Type, "不定式")
}

func phraseDetail(p PhraseInfo) string {
	parts := make([]string, 0, 3)
	if p.Type != "" {
		parts = append(parts, p.Type)
	}
	if p.Role != "" {
		parts = append(parts, "作"+p.Role)
	}
	if p.Target != "" {
		parts = append(parts, p.Target)
	}
	return strings.Join(parts, " · ")
}

func renderGrammarPhraseBadge(c *ui.Context, t *ui.Theme, phrase PhraseInfo) ui.Element {
	raw := grammarPhraseShortTag(phrase)
	pc := grammarPhrasePalette(t, raw)
	label := strings.Join(strings.Split(raw, ""), "\u2060")
	return ui.Text(c, "\u00A0"+label+"\u00A0").FontSize(10).FontWeight(600).
		TextColor(pc.badgeText).Background(pc.badgeBg).Unselectable().
		Tooltip(phraseDetail(phrase))
}

func renderGrammarClauseBadge(c *ui.Context, t *ui.Theme, clause ClauseInfo) ui.Element {
	raw := grammarClauseLabel(clause)
	colors := grammarClausePalette(t, raw)
	label := strings.Join(strings.Split(raw, ""), "\u2060")
	el := ui.Text(c, "\u00A0"+label+"\u00A0").FontSize(10).FontWeight(600).
		TextColor(colors.badgeText).Background(colors.badgeBg).Unselectable().
		Tooltip(grammarClauseTooltip(clause))
	return el
}

func renderGrammarSlash(c *ui.Context, at AppTheme) ui.Element {
	return ui.Text(c, " / ").FontSize(13).FontWeight(500).
		TextColor(at.Accent.Alpha(0.55)).Unselectable()
}

func renderGrammarFoldMarker(c *ui.Context, t *ui.Theme, clause *ClauseInfo, phrase *PhraseInfo, runText, prefix string) ui.Element {
	at := appTheme(t)
	tip := strings.TrimSpace(runText)
	if clause != nil {
		label := "[" + grammarClauseLabel(*clause) + " …]"
		tip = grammarFoldNote(clause, phrase, runText)
		colors := grammarClausePalette(t, grammarClauseLabel(*clause))
		return ui.Text(c, prefix+label).FontSize(12).FontWeight(600).
			TextColor(colors.badgeText).Background(colors.badgeBg).
			Unselectable().Tooltip(tip)
	}
	if phrase != nil {
		tag := grammarPhraseShortTag(*phrase)
		pc := grammarPhrasePalette(t, tag)
		label := "(" + tag + " …)"
		tip = grammarFoldNote(nil, phrase, runText)
		return ui.Text(c, prefix+label).FontSize(11).FontWeight(600).
			TextColor(pc.badgeText).Background(pc.badgeBg).
			Unselectable().Tooltip(tip)
	}
	return ui.Text(c, prefix+"…").FontSize(13).FontWeight(600).
		TextColor(at.TextSec).Unselectable().Tooltip(tip)
}

func leadingSpace(s string) string {
	if s == "" {
		return ""
	}
	for i, r := range s {
		if !unicode.IsSpace(r) {
			return s[:i]
		}
	}
	return s
}

func trailingSpace(s string) string {
	if s == "" {
		return ""
	}
	i := len(s)
	for i > 0 {
		r, size := utf8.DecodeLastRuneInString(s[:i])
		if !unicode.IsSpace(r) {
			break
		}
		i -= size
	}
	return s[i:]
}

func renderGrammarHintBar(c *ui.Context, t *ui.Theme, at AppTheme, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	ui.Box(c).FillWidth().Padding(5, 8).Radius(RadiusXS).
		Background(at.Surface).Border(1, at.BorderSubtle).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(6).Children(func() {
			ui.Text(c, "·").FontSize(12).FontWeight(700).TextColor(at.Accent)
			ui.Text(c, text).FontSize(11).FontWeight(500).TextColor(at.TextSec).
				LineHeight(1.45).Wrap().MaxLines(2).Tooltip(text).Grow(1)
		})
	})
}

func renderGrammarLoadingBanner(c *ui.Context, t *ui.Theme, at AppTheme, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	ui.Box(c).FillWidth().Padding(8, 10).Radius(RadiusXS).
		Background(at.Surface).Border(1, at.BorderSubtle).Children(func() {
		ui.Column(c).FillWidth().Gap(6).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Spinner(c).Size(14, 14).TextColor(at.Accent)
				ui.Text(c, text).FontSize(12).FontWeight(500).TextColor(at.TextSec).
					LineHeight(1.45).Wrap().Grow(1)
			})
			IndeterminateBar(c, t, 3, "")
		})
	})
}

func renderGrammarFallbackSentence(c *ui.Context, t *ui.Theme, origText string, res *SentenceAnalysisResult, st grammarCardState) {
	at := appTheme(t)
	ui.Text(c, origText).FontSize(14).TextColor(at.Text).LineHeight(1.7).Wrap().Selectable()
	if st.Prefill {
		renderGrammarLoadingBanner(c, t, at, "正在拆解句法结构（主干 / 从句 / 短语），先看机器译文…")
		return
	}
	ui.Text(c, "这次回答没有按原文切分，已先显示原句与译文。可以点刷新按钮再试一次。").
		FontSize(11).FontWeight(500).TextColor(at.TextMuted).Wrap()
	renderGrammarHintBar(c, t, at, grammarDefaultHint(res, st.Prefill))
}

func renderGrammarControls(c *ui.Context, t *ui.Theme, ink grammarInk, st grammarCardState) {
	at := appTheme(t)
	ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(6).Padding(2, 0).Children(func() {
		viewIdx := 0
		if st.ShowTree {
			viewIdx = 1
		}
		SegmentedControlCompact(c, &viewIdx, "括号标注", "结构树").
			Tooltip("在行内括号标注与缩进句法树之间切换").
			OnChange(func() {
				if (viewIdx == 1) != st.ShowTree && st.OnToggleTree != nil {
					st.OnToggleTree()
				}
			})

		if grammarToggle(c, t, "只看主干", st.OnlyTrunk, "只留主句骨架，从句和修饰短语折叠成省略号").Clicked() && st.OnToggleTrunk != nil {
			st.OnToggleTrunk()
		}
		if grammarToggle(c, t, "断句", st.ShowSlashes, "在自然意群边界显示断句斜线").Clicked() && st.OnToggleSlash != nil {
			st.OnToggleSlash()
		}

		ui.Spacer(c).Grow(1)

		ui.Row(c).Gap(6).AlignItems(ui.Center).Shrink(0).Children(func() {
			grammarLegendStatic(c, t, at, ink, "主干", "backbone")
			grammarLegendStatic(c, t, at, ink, "谓语", "predicate")
			grammarLegendStatic(c, t, at, ink, "从句", "clause")
			grammarLegendStatic(c, t, at, ink, "介短", "prep")
			grammarLegendStatic(c, t, at, ink, "非谓语", "nonfinite")
			grammarLegendStatic(c, t, at, ink, "同位/插", "appositive")
		})
	})
}

func grammarLegendStatic(c *ui.Context, t *ui.Theme, at AppTheme, ink grammarInk, label, kind string) {
	el := ui.Text(c, label).FontSize(11).FontWeight(600).TextColor(at.TextMuted)
	switch kind {
	case "backbone":
		el.TextColor(ink.body).FontWeight(600).Tooltip("加粗：主句主干（主语、宾语/表语）")
	case "predicate":
		el.TextColor(ink.predicate).FontWeight(700).
			Underline().DecorationColor(ink.predicateLine).
			DecorationThickness(1.1).UnderlineOffset(3).
			Tooltip("主题色下划线：核心谓语动词")
	case "clause":
		cc := grammarClausePalette(t, "宾从")
		el.TextColor(cc.badgeText).Background(cc.wash).Tooltip("浅主题底：从句，微标标明类型")
	case "prep":
		pc := grammarPhrasePalette(t, "介短")
		el.TextColor(pc.badgeText).Background(pc.wash).Tooltip("青靛色括号标注：介词短语（定语/状语）")
	case "nonfinite":
		pc := grammarPhrasePalette(t, "分词")
		el.TextColor(pc.badgeText).Background(pc.wash).
			DottedUnderline().DecorationColor(pc.underline).
			DecorationThickness(1.1).UnderlineOffset(3).
			Tooltip("琥珀虚线底纹：非谓语短语（分词 / 不定式 / 动名词）")
	case "appositive":
		pc := grammarPhrasePalette(t, "同位")
		el.TextColor(pc.badgeText).Background(pc.wash).Tooltip("紫玫瑰色括号标注：同位语 / 插入语")
	}
}

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

func renderGrammarTranslation(c *ui.Context, t *ui.Theme, at AppTheme, translation string, prefill bool) {
	head := "地道精翻"
	if prefill {
		head = "先看译文（机翻，语法分析生成中）"
	}
	ui.Column(c).FillWidth().Gap(4).Children(func() {
		ui.Text(c, head).FontSize(11).FontWeight(600).TextColor(at.TextMuted)
		ui.Row(c).FillWidth().AlignItems(ui.Stretch).Gap(10).Children(func() {
			ui.Box(c).Width(2).Background(at.Accent.Alpha(0.55))
			ui.Text(c, translation).FontSize(14).FontWeight(500).TextColor(at.Text).
				Wrap().LineHeight(1.6).Grow(1)
		})
	})
}

func renderGrammarDifficulties(c *ui.Context, t *ui.Theme, at AppTheme, points []DifficultyPoint) {
	if len(points) > 3 {
		points = points[:3]
	}
	d := grammarDifficultyPalette(at)
	ui.Box(c).FillWidth().Padding(Space3, Space4).Radius(RadiusSM).
		Background(d.surface).Border(1, d.border).Children(func() {
		ui.Column(c).FillWidth().Gap(Space2).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
				ui.Icon(c, iconInfo).Size(12, 12).TextColor(d.icon)
				ui.Text(c, "语法难点").FontSize(11).FontWeight(600).TextColor(d.heading)
				ui.Spacer(c).Grow(1)
				ui.Text(c, fmt.Sprintf("%d", len(points))).Font(fontMono).
					FontFeatures("tnum").FontSize(10).TextColor(d.count)
			})
			for _, point := range points {
				ui.Column(c).FillWidth().Gap(2).Children(func() {
					ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
						ui.Box(c).Size(4, 4).Radius(2).Background(d.dot).Shrink(0)
						title := point.Title
						if title == "" {
							title = "难点"
						}
						ui.Box(c).Padding(1, 6).Radius(RadiusXS).
							Background(d.chipBg).Border(1, d.chipBorder).Children(func() {
							ui.Text(c, title).FontSize(10).FontWeight(600).
								TextColor(d.chipText).SingleLine()
						})
						if point.SpanText != "" {
							ui.Text(c, point.SpanText).FontSize(12).FontWeight(600).
								TextColor(d.span).SingleLine().Tooltip(point.SpanText).Grow(1)
						}
					})
					if point.Explanation != "" {
						ui.Text(c, point.Explanation).FontSize(11).TextColor(d.note).
							Wrap().MarginStart(10)
					}
				})
			}
		})
	})
}

type grammarSentenceItem struct {
	segments []int
	fold     bool
}

func grammarSentenceItems(segments []Segment, onlyTrunk bool) []grammarSentenceItem {
	if !onlyTrunk {
		items := make([]grammarSentenceItem, 0, len(segments))
		for i := range segments {
			items = append(items, grammarSentenceItem{segments: []int{i}})
		}
		return items
	}

	var items []grammarSentenceItem
	for i := 0; i < len(segments); {
		if grammarTrunkKeeps(segments[i]) {
			items = append(items, grammarSentenceItem{segments: []int{i}})
			i++
			continue
		}
		start := i
		for i < len(segments) && !grammarTrunkKeeps(segments[i]) {
			i++
		}
		items = append(items, grammarSentenceItem{segments: indexRange(start, i), fold: true})
	}
	return items
}

func grammarTrunkKeeps(seg Segment) bool {
	if seg.IsBackbone {
		return true
	}
	if strings.TrimSpace(seg.Text) == "" {
		return false
	}
	return seg.Role == RolePunct && seg.ClauseID == "" && seg.PhraseID == ""
}

func indexRange(start, end int) []int {
	out := make([]int, 0, end-start)
	for i := start; i < end; i++ {
		out = append(out, i)
	}
	return out
}

func segmentsText(segments []Segment, indexes []int) string {
	var b strings.Builder
	for _, idx := range indexes {
		b.WriteString(segments[idx].Text)
	}
	return b.String()
}

func grammarSegmentTooltip(seg Segment, clause *ClauseInfo, phrase *PhraseInfo) string {
	parts := make([]string, 0, 3)
	if role := grammarRoleLabel(seg.Role); role != "" && seg.Role != RolePunct {
		parts = append(parts, role)
	}
	if clause != nil {
		parts = append(parts, grammarClauseLabel(*clause))
	}
	if phrase != nil {
		parts = append(parts, grammarPhraseShortTag(*phrase))
	}
	if seg.Note != "" {
		parts = append(parts, seg.Note)
	}
	return strings.Join(parts, " · ")
}

func grammarSegmentNote(seg Segment, clause *ClauseInfo, phrase *PhraseInfo) string {
	parts := make([]string, 0, 4)
	if text := strings.TrimSpace(seg.Text); text != "" {
		parts = append(parts, text)
	}
	if clause != nil {
		parts = append(parts, grammarClauseLabel(*clause))
	}
	if phrase != nil {
		parts = append(parts, grammarPhraseShortTag(*phrase))
	}
	switch {
	case phrase != nil:
		if detail := phraseDetail(*phrase); detail != "" {
			parts = append(parts, detail)
		} else if seg.Note != "" {
			parts = append(parts, seg.Note)
		}
	case seg.Note != "":
		parts = append(parts, seg.Note)
	case clause != nil:
		if detail := clauseDetail(*clause); detail != "" {
			parts = append(parts, detail)
		}
	default:
		if role := grammarRoleLabel(seg.Role); role != "" && seg.Role != RolePunct {
			parts = append(parts, role)
		}
	}
	return strings.Join(parts, " · ")
}

func grammarFoldNote(clause *ClauseInfo, phrase *PhraseInfo, runText string) string {
	parts := make([]string, 0, 3)
	if text := strings.TrimSpace(runText); text != "" {
		parts = append(parts, text)
	}
	if clause != nil {
		if label := grammarClauseLabel(*clause); label != "" {
			parts = append(parts, label)
		}
		if detail := clauseDetail(*clause); detail != "" {
			parts = append(parts, detail)
		}
	} else if phrase != nil {
		parts = append(parts, grammarPhraseShortTag(*phrase))
		if detail := phraseDetail(*phrase); detail != "" {
			parts = append(parts, detail)
		}
	}
	return strings.Join(parts, " · ")
}

func clauseDetail(clause ClauseInfo) string {
	parts := make([]string, 0, 2)
	if clause.Type != "" {
		parts = append(parts, clause.Type)
	}
	if clause.Target != "" {
		parts = append(parts, clause.Target)
	}
	return strings.Join(parts, "，")
}

func grammarDefaultHint(res *SentenceAnalysisResult, prefill bool) string {
	if prefill {
		return "语法分析生成中，先看机器译文…"
	}
	if summary := strings.TrimSpace(res.BackboneSummary); summary != "" {
		return "主干：" + summary
	}
	if len(res.Clauses) > 0 && len(res.Phrases) > 0 {
		return fmt.Sprintf("共 %d 处从句、%d 处修饰短语，悬停任意片段查看作用", len(res.Clauses), len(res.Phrases))
	}
	if len(res.Phrases) > 0 {
		return fmt.Sprintf("共 %d 处修饰短语，悬停任意片段查看修饰目标", len(res.Phrases))
	}
	if len(res.Clauses) > 0 {
		return fmt.Sprintf("共 %d 处从句，悬停句子成分查看语法作用", len(res.Clauses))
	}
	return "悬停句子成分，查看它的语法作用"
}

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
