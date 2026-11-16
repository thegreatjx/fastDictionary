package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

// SegmentRole is the grammatical job of one contiguous slice of the sentence.
type SegmentRole string

const (
	RoleSubject   SegmentRole = "subject"
	RolePredicate SegmentRole = "predicate"
	RoleObject    SegmentRole = "object"
	RoleConnector SegmentRole = "connector"
	RoleModifier  SegmentRole = "modifier"
	RolePunct     SegmentRole = "punct"
)

// Segment is one contiguous, verbatim slice of the source sentence. The model
// never reports character offsets: joining every Text in order has to restore
// the sentence exactly, and the parser repairs or rejects everything else.
type Segment struct {
	Text        string      `json:"text"`
	Role        SegmentRole `json:"role"`
	IsBackbone  bool        `json:"is_backbone"`
	BreakBefore bool        `json:"break_before,omitempty"`
	ClauseID    string      `json:"clause_id,omitempty"`
	Note        string      `json:"note,omitempty"`
}

// ClauseInfo describes one clause. Clauses form a flat list with ParentID
// instead of a recursive tree, which keeps streaming JSON cheap and parseable.
type ClauseInfo struct {
	ID       string `json:"id"`
	ParentID string `json:"parent_id,omitempty"`
	Type     string `json:"type"`
	ShortTag string `json:"short_tag"`
	Target   string `json:"target,omitempty"`
}

// DifficultyPoint is one thing that makes the sentence hard to read.
type DifficultyPoint struct {
	Title       string `json:"title"`
	SpanText    string `json:"span_text"`
	Explanation string `json:"explanation"`
}

// SentenceAnalysisResult is the structured result returned by the grammar
// tutor LLM.
type SentenceAnalysisResult struct {
	BackboneSummary string            `json:"backbone_summary"`
	Segments        []Segment         `json:"segments"`
	Clauses         []ClauseInfo      `json:"clauses"`
	Difficulties    []DifficultyPoint `json:"difficulties,omitempty"`
	Translation     string            `json:"translation"`
	Raw             string            `json:"-"`
}

// ParseOutcome records which parse path produced the result.
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

// SentenceParseResult carries the result together with where it came from.
type SentenceParseResult struct {
	Outcome ParseOutcome
	Result  *SentenceAnalysisResult
	Cause   error
}

// grammarAbbrevs are abbreviations that do not end a sentence.
var grammarAbbrevs = map[string]bool{
	"mr.": true, "mrs.": true, "ms.": true, "dr.": true, "prof.": true,
	"sr.": true, "jr.": true, "st.": true, "vs.": true, "etc.": true,
	"e.g.": true, "i.e.": true, "u.s.": true, "u.k.": true, "fig.": true,
	"no.": true, "vol.": true, "inc.": true, "ltd.": true,
}

// splitGrammarSentences splits the selected text into sentences, merging hard
// line wraps inside a paragraph first. OCR and web pages break a single sentence
// across lines; treating every line as its own paragraph sent half-sentences to
// the model, which then reported fragments as complete clauses.
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
				continue // 3.5, 1.000
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

// grammarParagraphs joins lines separated by a single newline into one
// paragraph and keeps blank lines as paragraph boundaries.
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

// grammarAnalysisText normalizes a selection into the one piece of text the
// grammar pass analyses: paragraphs reflowed into single lines, sentences
// joined with one space. The card and the parser both see this same text, so
// the model's slices can be aligned against it exactly.
func grammarAnalysisText(text string) string {
	sents := splitGrammarSentences(text)
	if len(sents) == 0 {
		return strings.TrimSpace(text)
	}
	return strings.Join(sents, " ")
}

// ParseSentenceAnalysis parses the model output and validates it against the
// source sentence. The source is optional so a caller that only has the answer
// can still inspect it; without it, segment alignment cannot be checked.
func ParseSentenceAnalysis(raw string, original ...string) *SentenceAnalysisResult {
	return ParseSentenceAnalysisDetailed(raw, original...).Result
}

// ParseSentenceAnalysisDetailed keeps each parse outcome so callers can tell a
// translation-only answer from a malformed one.
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
		// The model rewrote, dropped or reordered text. Showing its slices would
		// put words on screen that are not in the sentence; drop them and let
		// the card show the original with whatever analysis survived.
		res.Segments = nil
		if res.Translation != "" || len(res.Difficulties) > 0 || len(res.Clauses) > 0 {
			return SentenceParseResult{Outcome: parseOutcomeFallbackDirect, Result: &res}
		}
		return SentenceParseResult{Outcome: parseOutcomeFailed, Cause: fmt.Errorf("语法标注与原文无法对齐")}
	}
	if res.Translation != "" {
		return SentenceParseResult{Outcome: parseOutcomeTranslationOnly, Result: &res}
	}
	return SentenceParseResult{Outcome: parseOutcomeFailed, Cause: fmt.Errorf("语法解析没有得到可用的标注")}
}

// normalizeClauses lowercases roles, drops empty clauses and gives every clause
// a stable ID and short badge.
func normalizeClauses(res *SentenceAnalysisResult) {
	for i := range res.Segments {
		res.Segments[i].Role = normalizeSegmentRole(res.Segments[i].Role)
		res.Segments[i].ClauseID = strings.TrimSpace(res.Segments[i].ClauseID)
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
		// Some models file the main clause as a clause too, with type 主句 and
		// a 主从 badge. It is already marked by is_backbone; washing the whole
		// sentence and calling it a subordinate clause is worse than dropping it.
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
	// Models often tag only the first two words of a clause. Carry the open
	// clause across the following non-backbone words until the next clause or
	// the end of the sentence, so the wash covers one continuous block instead
	// of stopping in the middle of the clause.
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
	for i := range res.Difficulties {
		res.Difficulties[i].Title = strings.TrimSpace(res.Difficulties[i].Title)
		res.Difficulties[i].SpanText = strings.TrimSpace(res.Difficulties[i].SpanText)
		res.Difficulties[i].Explanation = strings.TrimSpace(res.Difficulties[i].Explanation)
	}
}

// grammarEndsSentence reports whether text closes a sentence, so an open
// clause does not run into the next one.
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

// alignSegmentsToSource compares the model's slices with the real sentence.
//
// A perfect match is rebuilt from the source's own spacing. Models also
// reorder a word or two ("will visit" before "first") or change a dash or
// quote; in that case the exact walk fails, and the fuzzy fallback assigns
// each source token to the segment that named it, then emits source-order
// runs. Every returned Text is still cut from the source, so the card never
// shows words the model invented.
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

// alignSegmentsExact walks the model's slices over the source's non-space
// runes. It repairs whitespace-only differences and nothing else.
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

// grammarSourceToken is one word or punctuation mark with its rune span in
// the source.
type grammarSourceToken struct {
	text       string
	start, end int
}

// grammarTokenSpans splits text into comparable tokens: Latin/digit runs stay
// whole, Han characters count one by one, and punctuation is its own token.
// Hyphens and apostrophes are punctuation, so "Asia-Pacific" and "Asia–Pacific"
// still line up token by token.
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

// alignSegmentsFuzzy maps source tokens back to the model's segments by token
// text, so a locally reordered phrase or a changed punctuation mark keeps its
// role instead of dropping the whole analysis.
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

// grammarExplanations are the clause explanations used by tooltips.
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

// grammarInk is the grammar card's palette: one ink ladder rather than one
// saturated colour per role.
type grammarInk struct {
	body          ui.Color
	predicate     ui.Color
	predicateLine ui.Color
	muted         ui.Color
	connector     ui.Color
	clauseTint    ui.Color
	clauseText    ui.Color
}

// grammarCardState is everything the grammar card reads plus the callbacks it
// fires.
type grammarCardState struct {
	OnlyTrunk     bool
	ShowSlashes   bool
	Prefill       bool
	SentenceCount int
	ClauseCount   int
	OnToggleTrunk func()
	OnToggleSlash func()
	OnSpeak       func()
	OnSave        func()
}

// renderGrammarVisualCard renders the read-only card from the app's state.
func (a *app) renderGrammarVisualCard(c *ui.Context, t *ui.Theme, origText string, res *SentenceAnalysisResult, saved bool, onSave func()) {
	st := grammarCardState{
		OnlyTrunk:     a.onlyTrunk,
		ShowSlashes:   a.showSlashes,
		Prefill:       a.grammarResultPrefill,
		OnToggleTrunk: func() { a.onlyTrunk = !a.onlyTrunk },
		OnToggleSlash: func() { a.showSlashes = !a.showSlashes },
		OnSpeak:       func() { PlaySpeechLang(origText, a.speechLang(), false) },
		OnSave:        onSave,
	}
	renderGrammarVisualCardEx(c, t, origText, res, saved, st)
}

// renderGrammarVisualCardEx draws the card from an explicit state.
func renderGrammarVisualCardEx(c *ui.Context, t *ui.Theme, origText string, res *SentenceAnalysisResult, saved bool, st grammarCardState) {
	at := appTheme(t)
	ink := grammarInkFor(t)
	st.SentenceCount = len(splitGrammarSentences(origText))
	st.ClauseCount = len(res.Clauses)

	ui.Box(c).FillWidth().Padding(Space3, Space4).Radius(8).
		Background(t.Surface).Border(1, t.Border).
		Shadow(0, 2, 8, 0, withAlpha(at.ShadowColor, 0.67)).Children(func() {
		ui.Column(c).FillWidth().Gap(Space2).Children(func() {
			renderGrammarCardHeader(c, t, st)

			ui.Box(c).FillWidth().Padding(12, 14).Radius(6).
				Background(at.SurfaceHover).Border(1, at.BorderSubtle).Children(func() {
				ui.Column(c).FillWidth().Gap(6).Children(func() {
					if len(res.Segments) == 0 {
						renderGrammarFallbackSentence(c, t, origText, res, st)
					} else {
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

			ui.Divider(c).FillWidth()
			renderGrammarSaveRow(c, t, at, saved, st.OnSave)
		})
	})
}

func renderGrammarCardHeader(c *ui.Context, t *ui.Theme, st grammarCardState) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(Space2).Children(func() {
		topBg, topBorder, topText := grammarHeaderColors(t)
		ui.Box(c).Padding(3, 10).Radius(12).
			Background(topBg).Border(1, topBorder).Children(func() {
			ui.Text(c, "长难句拆解").FontSize(11).FontWeight(600).TextColor(topText).SingleLine()
		})

		if scope := grammarScopeLabel(st); scope != "" {
			ui.Text(c, scope).Font(fontMono).FontFeatures("tnum").FontSize(FontMicro).
				TextColor(t.TextMuted).SingleLine()
		}

		ui.Spacer(c).Grow(1)

		btnSpeak := renderIconButton(c, iconSpeak, "朗读整段", 28, 14, false)
		if btnSpeak.Clicked() && st.OnSpeak != nil {
			st.OnSpeak()
		}
	})
}

func grammarScopeLabel(st grammarCardState) string {
	parts := make([]string, 0, 2)
	if st.SentenceCount > 0 {
		parts = append(parts, fmt.Sprintf("%d 句", st.SentenceCount))
	}
	if st.ClauseCount > 0 {
		parts = append(parts, fmt.Sprintf("%d 处从句", st.ClauseCount))
	}
	return strings.Join(parts, " · ")
}

// renderGrammarSentence draws the sentence as one wrapping rich text. Each
// segment is an inline text element, so it keeps the line breaking of the
// paragraph while still reporting hover and carrying a tooltip. The four
// visual dimensions are: backbone ink and weight, one accent underline on the
// main predicate, muted modifiers, and a faint clause wash with a micro badge.
func renderGrammarSentence(c *ui.Context, t *ui.Theme, res *SentenceAnalysisResult, st grammarCardState) {
	at := appTheme(t)
	ink := grammarInkFor(t)
	clauseByID := make(map[string]ClauseInfo, len(res.Clauses))
	for _, clause := range res.Clauses {
		clauseByID[clause.ID] = clause
	}

	items := grammarSentenceItems(res.Segments, st.OnlyTrunk)
	var segmentEls []ui.Element
	var segmentData []Segment
	var decorEls []ui.Element
	var decorClause []string
	var foldEls []ui.Element
	var foldClause []string
	hoveredIndex := -1
	hoveredFoldIndex := -1
	hoveredClause := ""
	hoverText := ""

	ui.RichText(c).FillWidth().LineHeight(1.75).Selectable().Children(func() {
		seenClause := make(map[string]bool)
		lastEndsSpace := true
		for itemIndex, item := range items {
			first := res.Segments[item.segments[0]]
			if st.ShowSlashes && itemIndex > 0 && first.BreakBefore {
				decorEls = append(decorEls, renderGrammarSlash(c, at))
				decorClause = append(decorClause, "")
				lastEndsSpace = true
			}

			if item.fold {
				runText := segmentsText(res.Segments, item.segments)
				var clause *ClauseInfo
				for _, idx := range item.segments {
					if id := res.Segments[idx].ClauseID; id != "" {
						if info, ok := clauseByID[id]; ok {
							clause = &info
						} else {
							clause = &ClauseInfo{ID: id, Type: "从句", ShortTag: "从句"}
						}
						break
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
				el := renderGrammarFoldMarker(c, t, clause, runText, prefix)
				foldEls = append(foldEls, el)
				foldID := ""
				if clause != nil {
					foldID = clause.ID
				}
				foldClause = append(foldClause, foldID)
				if tail := trailingSpace(runText); tail != "" {
					ui.Text(c, tail).Unselectable()
					lastEndsSpace = true
				} else {
					lastEndsSpace = false
				}
				if el.Hovered() {
					hoverText = grammarFoldNote(clause, runText)
					hoveredClause = "fold"
					if clause != nil {
						hoveredClause = clause.ID
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
					seenClause[seg.ClauseID] = true
				}
			}

			el := renderGrammarSegment(c, t, at, ink, seg, clause)
			lastEndsSpace = strings.HasSuffix(seg.Text, " ")
			if el.Hovered() {
				hoveredIndex = len(segmentEls)
				hoveredClause = seg.ClauseID
				hoverText = grammarSegmentNote(seg, clause)
			}
			segmentEls = append(segmentEls, el)
			segmentData = append(segmentData, seg)
		}

		if hoveredIndex >= 0 || hoveredClause != "" || hoveredFoldIndex >= 0 {
			for i, el := range segmentEls {
				keep := hoveredIndex == i
				if hoveredClause != "" && segmentData[i].ClauseID == hoveredClause {
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
				el.Opacity(0.35)
			}
			for i, el := range foldEls {
				if i == hoveredFoldIndex {
					continue
				}
				if hoveredClause != "" && foldClause[i] == hoveredClause {
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

func renderGrammarSegment(c *ui.Context, t *ui.Theme, at AppTheme, ink grammarInk, seg Segment, clause *ClauseInfo) ui.Element {
	text := seg.Text
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
		// The wash already groups the clause; its own content stays body ink so
		// the reading flow does not drop to grey. Only modifiers inside it step
		// back, which keeps the clause backbone legible at a glance.
		if seg.Role == RoleModifier {
			el.TextColor(ink.clauseText).FontWeight(500)
		} else {
			el.TextColor(ink.body).FontWeight(500)
		}
	default:
		el.TextColor(ink.muted).FontWeight(500)
	}

	if note := grammarSegmentTooltip(seg, clause); note != "" {
		el.Tooltip(note)
	}
	return el
}

func renderGrammarClauseBadge(c *ui.Context, t *ui.Theme, clause ClauseInfo) ui.Element {
	raw := grammarClauseLabel(clause)
	colors := grammarClausePalette(t, raw)
	// A RichText span breaks Chinese between any two characters. Two-character
	// clause tags then split across lines ("宾" at the end of one, "从" at the
	// start of the next). U+2060 WORD JOINER tells the line breaker not to
	// break here, so the tag stays one chip.
	label := strings.Join(strings.Split(raw, ""), "\u2060")
	el := ui.Text(c, "  "+label+"  ").FontSize(10).FontWeight(600).
		TextColor(colors.badgeText).Background(colors.badgeBg).Unselectable().
		Tooltip(grammarClauseTooltip(clause))
	return el
}

func renderGrammarSlash(c *ui.Context, at AppTheme) ui.Element {
	return ui.Text(c, " / ").FontSize(13).FontWeight(500).
		TextColor(at.Accent.Alpha(0.55)).Unselectable()
}

func renderGrammarFoldMarker(c *ui.Context, t *ui.Theme, clause *ClauseInfo, runText, prefix string) ui.Element {
	label := "…"
	tip := strings.TrimSpace(runText)
	if clause != nil {
		label = "[" + grammarClauseLabel(*clause) + " …]"
		tip = grammarFoldNote(clause, runText)
		colors := grammarClausePalette(t, grammarClauseLabel(*clause))
		return ui.Text(c, prefix+label).FontSize(12).FontWeight(600).
			TextColor(colors.badgeText).Background(colors.badgeBg).
			Unselectable().Tooltip(tip)
	}
	return ui.Text(c, prefix+label).FontSize(13).FontWeight(600).
		TextColor(appTheme(t).TextSec).Unselectable().Tooltip(tip)
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
	ui.Box(c).FillWidth().Height(24).Padding(3, 8).Radius(4).
		Background(at.Surface).Border(1, at.BorderSubtle).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Text(c, "·").FontSize(12).FontWeight(700).TextColor(at.Accent)
			ui.Text(c, text).FontSize(11).FontWeight(500).TextColor(at.TextSec).
				SingleLine().Tooltip(text).Grow(1)
		})
	})
}

func renderGrammarFallbackSentence(c *ui.Context, t *ui.Theme, origText string, res *SentenceAnalysisResult, st grammarCardState) {
	at := appTheme(t)
	ui.Text(c, origText).FontSize(14).TextColor(at.Text).LineHeight(1.7).Wrap().Selectable()
	if st.Prefill {
		renderGrammarHintBar(c, t, at, "语法分析生成中，先看机器译文…")
		return
	}
	ui.Text(c, "这次回答没有按原文切分，已先显示原句与译文。可以点刷新按钮再试一次。").
		FontSize(11).FontWeight(500).TextColor(at.TextMuted).Wrap()
	renderGrammarHintBar(c, t, at, grammarDefaultHint(res, st.Prefill))
}

func renderGrammarControls(c *ui.Context, t *ui.Theme, ink grammarInk, st grammarCardState) {
	at := appTheme(t)
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Padding(2, 0).Children(func() {
		if grammarToggle(c, t, "只看主干", st.OnlyTrunk, "只留主句骨架，从句和修饰短语折叠成省略号").Clicked() && st.OnToggleTrunk != nil {
			st.OnToggleTrunk()
		}
		if grammarToggle(c, t, "断句", st.ShowSlashes, "在自然意群边界显示断句斜线").Clicked() && st.OnToggleSlash != nil {
			st.OnToggleSlash()
		}

		ui.Spacer(c).Grow(1)

		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			grammarLegendStatic(c, t, at, ink, "主干", "backbone")
			grammarLegendStatic(c, t, at, ink, "谓语", "predicate")
			grammarLegendStatic(c, t, at, ink, "从句", "clause")
		})
	})
}

func grammarLegendStatic(c *ui.Context, t *ui.Theme, at AppTheme, ink grammarInk, label, kind string) {
	color := at.TextMuted
	el := ui.Text(c, label).FontSize(11).FontWeight(600).TextColor(color)
	switch kind {
	case "backbone":
		el.TextColor(ink.body).FontWeight(600).Tooltip("加粗：主句主干（主语、宾语/表语）")
	case "predicate":
		el.TextColor(ink.predicate).FontWeight(700).
			Underline().DecorationColor(ink.predicateLine).
			DecorationThickness(1.1).UnderlineOffset(3).
			Tooltip("蓝色下划线：核心谓语动词")
	case "clause":
		el.TextColor(ink.clauseText).Background(ink.clauseTint).Tooltip("浅色底：从句，徽章标明类型")
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

func renderGrammarSaveRow(c *ui.Context, t *ui.Theme, at AppTheme, saved bool, onSave func()) {
	ui.Row(c).FillWidth().Justify(ui.End).AlignItems(ui.Center).Children(func() {
		if saved {
			ui.Row(c).Height(HeightCompact).Gap(Space1).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconCheck).Size(12, 12).TextColor(at.TextMuted)
				ui.Text(c, "已存入长难句本").FontSize(11).FontWeight(500).TextColor(at.TextMuted)
			})
			return
		}
		if onSave == nil {
			return
		}
		btnSave := ui.Button(c, "").Height(HeightCompact).Padding(2, Space3).Radius(RadiusSM).
			Tooltip("将这条长难句分析保存到生词本的长难句分类")
		saveFG := saveButtonStyle(btnSave, t)
		btnSave.Children(func() {
			ui.Row(c).Gap(Space1).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconBookmark).Size(13, 13).TextColor(saveFG)
				ui.Text(c, "保存到长难句本").FontSize(11).FontWeight(600)
			})
		})
		if btnSave.Clicked() && onSave != nil {
			onSave()
		}
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
	// A sentence's own punctuation stays; punctuation inside a folded clause
	// belongs to that clause and disappears with it.
	return seg.Role == RolePunct && seg.ClauseID == ""
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

func grammarSegmentTooltip(seg Segment, clause *ClauseInfo) string {
	parts := make([]string, 0, 3)
	if role := grammarRoleLabel(seg.Role); role != "" && seg.Role != RolePunct {
		parts = append(parts, role)
	}
	if clause != nil {
		parts = append(parts, grammarClauseLabel(*clause))
	}
	if seg.Note != "" {
		parts = append(parts, seg.Note)
	}
	return strings.Join(parts, " · ")
}

func grammarSegmentNote(seg Segment, clause *ClauseInfo) string {
	parts := make([]string, 0, 4)
	if text := strings.TrimSpace(seg.Text); text != "" {
		parts = append(parts, text)
	}
	if clause != nil {
		parts = append(parts, grammarClauseLabel(*clause))
	}
	switch {
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

func grammarFoldNote(clause *ClauseInfo, runText string) string {
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
	if len(res.Clauses) > 0 {
		return fmt.Sprintf("共 %d 处从句，悬停句子成分查看语法作用", len(res.Clauses))
	}
	return "悬停句子成分，查看它的语法作用"
}
