package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
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

