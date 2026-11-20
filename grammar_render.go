package main

import (
	"fmt"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

type grammarInk struct {
	body           ui.Color
	predicate      ui.Color
	predicateLine  ui.Color
	muted          ui.Color
	connector      ui.Color
	clauseTint     ui.Color
	clauseText     ui.Color
	subjectLine    ui.Color
	objectLine     ui.Color
	complementLine ui.Color
	adverbialLine  ui.Color
	connectorText  ui.Color
	connectorRing  ui.Color
	obstacleStar   ui.Color
	highlightFill  ui.Color
	targetPillBg   ui.Color
	targetPillRing ui.Color
	arrowStroke    ui.Color
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

			ui.Box(c).FillWidth().Padding(15, 14, 14, 14).Radius(RadiusSM).
				Background(at.SurfaceHover).Border(1, at.BorderSubtle).Children(func() {
				ui.Column(c).FillWidth().Gap(10).Children(func() {
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

type grammarTextRun struct {
	text  string
	space bool
}

func splitGrammarTextRuns(s string) []grammarTextRun {
	if s == "" {
		return nil
	}
	var out []grammarTextRun
	start := 0
	var curSpace bool
	for i, r := range s {
		sp := unicode.IsSpace(r)
		if i == 0 {
			curSpace = sp
			continue
		}
		if sp != curSpace {
			out = append(out, grammarTextRun{text: s[start:i], space: curSpace})
			start = i
			curSpace = sp
		}
	}
	out = append(out, grammarTextRun{text: s[start:], space: curSpace})
	return out
}

type grammarWordDrawNode struct {
	el            ui.Element
	rawWord       string
	cleanWord     string
	segIndex      int
	seg           Segment
	clauseID      string
	phraseID      string
	firstInSeg    bool
	firstInPhrase bool
	lastInDiff    bool
	subTag        string
	subTagColor   ui.Color
}

type grammarBadgeDrawNode struct {
	el       ui.Element
	isClause bool
	clauseID string
	phraseID string
	bg       ui.Color
	border   ui.Color
	textCol  ui.Color
	target   string
	wordIdx  int
}

type grammarBracketDrawNode struct {
	el       ui.Element
	isLeft   bool
	clauseID string
	phraseID string
	color    ui.Color
	seed     int64
}

type grammarSlashDrawNode struct {
	el       ui.Element
	isDouble bool
	color    ui.Color
	seed     int64
}

func cleanHeadMatchWord(w string) string {
	return strings.ToLower(strings.Trim(w, " \t\r\n,.;:!?()[]\"'“”‘’—-"))
}

func extractTargetEnglishTokens(target string) []string {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil
	}
	var tokens []string
	var cur strings.Builder
	for _, r := range target {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '\'' || r == '-' {
			cur.WriteRune(unicode.ToLower(r))
		} else if cur.Len() > 0 {
			tok := strings.Trim(cur.String(), "'-")
			if tok != "" {
				tokens = append(tokens, tok)
			}
			cur.Reset()
		}
	}
	if cur.Len() > 0 {
		if tok := strings.Trim(cur.String(), "'-"); tok != "" {
			tokens = append(tokens, tok)
		}
	}
	return tokens
}

func findTargetWordIndices(words []grammarWordDrawNode, anchorWordIdx int, targetSpec string) []int {
	want := extractTargetEnglishTokens(targetSpec)
	if len(want) == 0 || len(words) == 0 {
		return nil
	}
	matchAt := func(start int) []int {
		if start < 0 || start+len(want) > len(words) {
			return nil
		}
		idxs := make([]int, 0, len(want))
		for k, w := range want {
			cw := words[start+k].cleanWord
			if cw != w && strings.TrimSuffix(cw, "'s") != strings.TrimSuffix(w, "'s") {
				return nil
			}
			idxs = append(idxs, start+k)
		}
		return idxs
	}
	maxBefore := min(anchorWordIdx, len(words))
	for i := maxBefore - len(want); i >= 0; i-- {
		if got := matchAt(i); len(got) > 0 {
			return got
		}
	}
	for i := max(0, anchorWordIdx); i <= len(words)-len(want); i++ {
		if got := matchAt(i); len(got) > 0 {
			return got
		}
	}
	lastWant := want[len(want)-1]
	for i := maxBefore - 1; i >= 0; i-- {
		if words[i].cleanWord == lastWant || strings.TrimSuffix(words[i].cleanWord, "'s") == lastWant {
			return []int{i}
		}
	}
	return nil
}

func grammarSubTagForPhrase(p PhraseInfo) string {
	role := strings.TrimSpace(p.Role)
	typ := strings.TrimSpace(p.Type)
	tag := grammarPhraseShortTag(p)
	switch {
	case strings.Contains(typ, "过去分词"):
		return "过去分词"
	case strings.Contains(typ, "现在分词"):
		return "现在分词"
	case strings.Contains(typ, "不定式") || tag == "不定式":
		if role != "" && !strings.Contains(role, "不定式") {
			return "不定式·" + role
		}
		return "不定式"
	case strings.Contains(typ, "动名") || tag == "动名":
		return "动名词"
	case strings.Contains(typ, "分词") || tag == "分词":
		if role != "" {
			return "分词·" + role
		}
		return "分词短语"
	case role != "":
		return role
	case typ != "":
		return typ
	default:
		return tag
	}
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

	diffTitleBySeg := make(map[int]string)
	for _, d := range res.Difficulties {
		span := strings.TrimSpace(d.SpanText)
		if span == "" {
			continue
		}
		title := strings.TrimSpace(d.Title)
		if title == "" {
			title = "难点"
		}
		firstMatch := -1
		for idx, seg := range res.Segments {
			if seg.Role == RolePunct {
				continue
			}
			stxt := strings.Trim(strings.TrimSpace(seg.Text), " ,.;:!?()[]{}\"'")
			if len(stxt) < 2 {
				continue
			}
			if strings.Contains(span, stxt) || strings.Contains(stxt, span) {
				firstMatch = idx
				break
			}
		}
		if firstMatch >= 0 {
			if _, exists := diffTitleBySeg[firstMatch]; !exists {
				diffTitleBySeg[firstMatch] = title
			}
		}
	}

	items := grammarSentenceItems(res.Segments, st.OnlyTrunk)
	var words []grammarWordDrawNode
	var badges []grammarBadgeDrawNode
	var brackets []grammarBracketDrawNode
	var slashes []grammarSlashDrawNode
	var decorEls []ui.Element
	var decorClause []string
	var decorPhrase []string
	var foldEls []ui.Element
	var foldClause []string
	var foldPhrase []string
	hoveredSegIndex := -1
	hoveredFoldIndex := -1
	hoveredClause := ""
	hoveredPhrase := ""
	hoverText := ""

	para := ui.RichText(c).FillWidth().LineHeight(2.68).Selectable()
	para.Children(func() {
		seenClause := make(map[string]bool)
		activePhrase := ""
		activePhraseClause := ""
		var activePhraseInfo *PhraseInfo
		activePhraseOwnParens := false
		lastEndsSpace := true
		prevClauseID := ""

		closePhrase := func(trail string) {
			if activePhrase == "" {
				return
			}
			if !activePhraseOwnParens {
				bracketCol := at.TextMuted
				if activePhraseInfo != nil {
					bracketCol = grammarPhrasePalette(t, grammarPhraseShortTag(*activePhraseInfo)).bracket
				}
				el := ui.Text(c, ")").FontSize(13).FontWeight(700).TextColor(ui.Transparent).Unselectable()
				brackets = append(brackets, grammarBracketDrawNode{
					el: el, isLeft: false, clauseID: activePhraseClause, phraseID: activePhrase,
					color: bracketCol, seed: int64(len(brackets)+1) * 19,
				})
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
			isClauseBoundary := first.ClauseID != "" && first.ClauseID != prevClauseID
			if st.ShowSlashes && itemIndex > 0 && (first.BreakBefore || isClauseBoundary) {
				slashEl := renderGrammarSlash(c)
				slashes = append(slashes, grammarSlashDrawNode{
					el: slashEl, isDouble: isClauseBoundary, color: ink.predicate.Alpha(0.70),
					seed: int64(itemIndex+1) * 29,
				})
				decorEls = append(decorEls, slashEl)
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

			segIdx := item.segments[0]
			seg := res.Segments[segIdx]
			prevClauseID = seg.ClauseID
			var clause *ClauseInfo
			if seg.ClauseID != "" {
				if info, ok := clauseByID[seg.ClauseID]; ok {
					clause = &info
				} else {
					clause = &ClauseInfo{ID: seg.ClauseID, Type: "从句", ShortTag: "从句"}
				}
				if !seenClause[seg.ClauseID] {
					cc := grammarClausePalette(t, grammarClauseLabel(*clause))
					bEl := renderGrammarClauseBadge(c, t, *clause)
					badges = append(badges, grammarBadgeDrawNode{
						el: bEl, isClause: true, clauseID: seg.ClauseID,
						bg: at.Surface, border: cc.badgeText.Alpha(0.55), textCol: cc.badgeText,
						target: clause.Target, wordIdx: len(words),
					})
					decorEls = append(decorEls, bEl)
					decorClause = append(decorClause, seg.ClauseID)
					decorPhrase = append(decorPhrase, "")
					seenClause[seg.ClauseID] = true
					if bEl.Hovered() {
						hoveredClause = seg.ClauseID
						hoverText = grammarClauseTooltip(*clause)
					}
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
			justOpenedPhrase := false
			if phrase != nil && seg.PhraseID != activePhrase {
				justOpenedPhrase = true
				lead := leadingSpace(segText)
				segText = strings.TrimPrefix(segText, lead)
				if lead != "" && !slashed && itemIndex > 0 && !lastEndsSpace {
					ui.Text(c, lead).Unselectable()
				}
				pc := grammarPhrasePalette(t, grammarPhraseShortTag(*phrase))
				ownParens := phraseSpanHasOuterParens(res.Segments, items, itemIndex, seg.PhraseID)
				if !ownParens {
					openEl := ui.Text(c, "(").FontSize(13).FontWeight(700).TextColor(ui.Transparent).Unselectable()
					brackets = append(brackets, grammarBracketDrawNode{
						el: openEl, isLeft: true, clauseID: seg.ClauseID, phraseID: seg.PhraseID,
						color: pc.bracket, seed: int64(len(brackets)+1) * 17,
					})
					decorEls = append(decorEls, openEl)
					decorClause = append(decorClause, seg.ClauseID)
					decorPhrase = append(decorPhrase, seg.PhraseID)
				}

				badgeEl := renderGrammarPhraseBadge(c, t, *phrase)
				badges = append(badges, grammarBadgeDrawNode{
					el: badgeEl, isClause: false, clauseID: seg.ClauseID, phraseID: seg.PhraseID,
					bg: at.Surface, border: pc.border, textCol: pc.badgeText,
					target: phrase.Target, wordIdx: len(words),
				})
				decorEls = append(decorEls, badgeEl)
				decorClause = append(decorClause, seg.ClauseID)
				decorPhrase = append(decorPhrase, seg.PhraseID)
				if badgeEl.Hovered() {
					hoveredPhrase = seg.PhraseID
					hoveredClause = seg.ClauseID
					hoverText = phraseDetail(*phrase)
				}
				if strings.TrimSpace(segText) != "" {
					gapEl := ui.Text(c, " ").FontSize(13).Unselectable()
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

			subTag, subTagCol := grammarSubTagForSegment(t, ink, seg, segIdx, clause, phrase, justOpenedPhrase, diffTitleBySeg)
			runs := splitGrammarTextRuns(segText)
			wordInSeg := 0

			for _, r := range runs {
				if r.space {
					ui.Text(c, r.text).FontSize(14).Unselectable()
					continue
				}
				isFirstWord := wordInSeg == 0
				wordInSeg++

				wEl := renderGrammarWordElement(c, t, at, ink, seg, r.text, clause, phrase)
				if wEl.Hovered() {
					hoveredSegIndex = segIdx
					hoveredClause = seg.ClauseID
					hoveredPhrase = seg.PhraseID
					hoverText = grammarSegmentNote(seg, clause, phrase)
				}
				tag := ""
				if isFirstWord {
					tag = subTag
				}
				words = append(words, grammarWordDrawNode{
					el:            wEl,
					rawWord:       r.text,
					cleanWord:     cleanHeadMatchWord(r.text),
					segIndex:      segIdx,
					seg:           seg,
					clauseID:      seg.ClauseID,
					phraseID:      seg.PhraseID,
					firstInSeg:    isFirstWord,
					firstInPhrase: isFirstWord && justOpenedPhrase,
					lastInDiff:    isFirstWord && diffTitleBySeg[segIdx] != "",
					subTag:        tag,
					subTagColor:   subTagCol,
				})
			}
			lastEndsSpace = strings.HasSuffix(segText, " ")

			if isLastInPhrase {
				closePhrase(phraseTrail)
			}
		}
		closePhrase("")

		if hoveredSegIndex >= 0 || hoveredClause != "" || hoveredPhrase != "" || hoveredFoldIndex >= 0 {
			for _, w := range words {
				keep := w.segIndex == hoveredSegIndex
				if hoveredClause != "" && w.clauseID == hoveredClause {
					keep = true
				}
				if hoveredClause == "" && hoveredPhrase != "" && w.phraseID == hoveredPhrase {
					keep = true
				}
				if !keep {
					w.el.Opacity(0.34)
				}
			}
			for i, el := range decorEls {
				if hoveredClause != "" && decorClause[i] == hoveredClause {
					continue
				}
				if hoveredClause == "" && hoveredPhrase != "" && decorPhrase[i] == hoveredPhrase {
					continue
				}
				el.Opacity(0.34)
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
				el.Opacity(0.34)
			}
		}
	})

	type resolvedArrow struct {
		badge      grammarBadgeDrawNode
		targetIdxs []int
		active     bool
	}
	var candidateArrows []resolvedArrow
	for _, b := range badges {
		if strings.TrimSpace(b.target) == "" {
			continue
		}
		tIdxs := findTargetWordIndices(words, b.wordIdx, b.target)
		if len(tIdxs) == 0 {
			continue
		}
		isActive := (b.isClause && hoveredClause != "" && b.clauseID == hoveredClause) ||
			(!b.isClause && hoveredPhrase != "" && b.phraseID == hoveredPhrase)
		candidateArrows = append(candidateArrows, resolvedArrow{badge: b, targetIdxs: tIdxs, active: isActive})
	}
	selectVisibleArrows := func() []resolvedArrow {
		var out []resolvedArrow
		usedTarget := make(map[int]bool)
		anyHover := hoveredSegIndex >= 0 || hoveredClause != "" || hoveredPhrase != "" || hoveredFoldIndex >= 0
		for _, ar := range candidateArrows {
			if ar.active {
				out = append(out, ar)
				continue
			}
			if anyHover {
				continue
			}
			bb := ar.badge.el.Bounds()
			tb := words[ar.targetIdxs[0]].el.Bounds()
			if bb.W <= 0 || tb.W <= 0 {
				continue
			}
			bMid := bb.Y + bb.H*0.5
			tMid := tb.Y + tb.H*0.5
			if math.Abs(float64(tMid-bMid)) > 6 {
				continue
			}
			var pInfo PhraseInfo
			if !ar.badge.isClause {
				pInfo = phraseByID[ar.badge.phraseID]
			}
			if ar.badge.isClause || isNonFinitePhrase(pInfo) || len(out) == 0 {
				if !usedTarget[ar.targetIdxs[0]] && len(out) < 2 {
					usedTarget[ar.targetIdxs[0]] = true
					out = append(out, ar)
				}
			}
		}
		return out
	}

	para.Draw(func(p *ui.Painter, r ui.Rect) {
		for _, b := range badges {
			bb := b.el.Bounds()
			if bb.W <= 0 || bb.H <= 0 {
				continue
			}
			midY := bb.Y + bb.H*0.5
			chip := ui.Rect{X: bb.X + 1, Y: midY - 8, W: max(bb.W-2, 14), H: 16}
			p.Fill(chip, b.bg, 4)
			p.Stroke(chip, b.border, 4, 1)
		}
	})

	para.DrawOver(func(p *ui.Painter, r ui.Rect) {
		drawGrammarRoleMarksAndTags(p, t, ink, words, phraseByID)

		for _, br := range brackets {
			bb := br.el.Bounds()
			if bb.W <= 0 || bb.H <= 0 {
				continue
			}
			midY := bb.Y + bb.H*0.5
			rect := ui.Rect{X: bb.X, Y: midY - 9.5, W: max(bb.W, 5.5), H: 19}
			drawCurvedBracket(p, rect, br.isLeft, br.color, br.seed)
		}

		for _, sl := range slashes {
			sb := sl.el.Bounds()
			if sb.W <= 0 || sb.H <= 0 {
				continue
			}
			midY := sb.Y + sb.H*0.5
			w := float32(7)
			if sl.isDouble {
				w = 11
			}
			rect := ui.Rect{X: sb.X + (sb.W-w)*0.5, Y: midY - 8.5, W: w, H: 17}
			drawHandSlash(p, rect, sl.isDouble, sl.color, sl.seed)
		}

		for _, ar := range selectVisibleArrows() {
			bb := ar.badge.el.Bounds()
			if bb.W <= 0 || len(ar.targetIdxs) == 0 {
				continue
			}
			tb := words[ar.targetIdxs[0]].el.Bounds()
			if len(ar.targetIdxs) > 1 {
				lastB := words[ar.targetIdxs[len(ar.targetIdxs)-1]].el.Bounds()
				if math.Abs(float64(lastB.Y-tb.Y)) < 4 {
					tb.W = (lastB.X + lastB.W) - tb.X
				}
			}
			if tb.W <= 0 {
				continue
			}
			bMid := bb.Y + bb.H*0.5
			tMid := tb.Y + tb.H*0.5
			if math.Abs(float64(tMid-bMid)) > 6 {
				continue
			}
			fromRect := ui.Rect{X: bb.X, Y: bMid - 8.5, W: bb.W, H: 16}
			toRect := ui.Rect{X: tb.X, Y: tMid - 9.5, W: tb.W, H: 18}
			col := ar.badge.textCol
			width := float32(1.6)
			if !ar.active {
				col = withAlpha(col, 0.58)
				width = 1.35
			}
			drawModifierArrow(p, fromRect, toRect, r.Y-11, col, width)
		}
	})

	if hoverText == "" {
		hoverText = grammarDefaultHint(res, st.Prefill)
	}
	renderGrammarHintBar(c, t, at, hoverText)
}

func grammarSubTagForSegment(
	t *ui.Theme,
	ink grammarInk,
	seg Segment,
	segIdx int,
	clause *ClauseInfo,
	phrase *PhraseInfo,
	justOpenedPhrase bool,
	diffTitleBySeg map[int]string,
) (string, ui.Color) {
	if diffTitle, ok := diffTitleBySeg[segIdx]; ok {
		return diffTitle, ink.obstacleStar
	}
	if justOpenedPhrase && phrase != nil {
		pc := grammarPhrasePalette(t, grammarPhraseShortTag(*phrase))
		return grammarSubTagForPhrase(*phrase), pc.badgeText
	}
	switch seg.Role {
	case RoleSubject:
		if seg.IsBackbone && seg.ClauseID == "" {
			return "主语", ink.subjectLine
		}
		if seg.ClauseID != "" {
			return "从句主语", withAlpha(ink.subjectLine, 0.85)
		}
	case RolePredicate:
		if seg.IsBackbone && seg.ClauseID == "" {
			return "谓语", ink.predicate
		}
		return "从句谓语", withAlpha(ink.predicate, 0.85)
	case RoleObject:
		if seg.IsBackbone && seg.ClauseID == "" {
			return "宾语", ink.objectLine
		}
		if seg.ClauseID != "" {
			return "从句宾语", withAlpha(ink.objectLine, 0.85)
		}
	case RoleConnector:
		return "引导词", ink.connectorText
	case RoleModifier:
		if seg.PhraseID == "" && seg.Note != "" {
			switch {
			case strings.Contains(seg.Note, "倒装") || strings.Contains(seg.Note, "前置"):
				return "★ 前置/倒装", ink.obstacleStar
			case strings.Contains(seg.Note, "定语"):
				return "后置定语", ink.muted
			case strings.Contains(seg.Note, "状语"):
				return "状语", ink.muted
			}
		}
	}
	return "", ink.muted
}

func drawGrammarRoleMarksAndTags(
	p *ui.Painter,
	t *ui.Theme,
	ink grammarInk,
	words []grammarWordDrawNode,
	phraseByID map[string]PhraseInfo,
) {
	if len(words) == 0 {
		return
	}

	lastTagRightByLine := make(map[int]float32)
	i := 0
	for i < len(words) {
		first := words[i]
		b0 := first.el.Bounds()
		if b0.W <= 0 || b0.H <= 0 {
			i++
			continue
		}
		spanRect := b0
		j := i + 1
		for j < len(words) && words[j].segIndex == first.segIndex {
			bj := words[j].el.Bounds()
			if bj.W <= 0 || bj.H <= 0 {
				j++
				continue
			}
			if math.Abs(float64(bj.Y-spanRect.Y)) >= 4 {
				break
			}
			spanRect.W = max(spanRect.X+spanRect.W, bj.X+bj.W) - spanRect.X
			j++
		}

		seg := first.seg
		midY := spanRect.Y + spanRect.H*0.5
		markY := midY + 5.8
		seed := int64(first.segIndex*31 + i + 1)

		switch seg.Role {
		case RoleSubject:
			col := ink.subjectLine
			if !seg.IsBackbone || seg.ClauseID != "" {
				col = withAlpha(col, 0.55)
			}
			drawMark(p, ui.Rect{X: spanRect.X, Y: markY, W: spanRect.W, H: markHeight(markUnderline)}, markUnderline, col, seed)
		case RolePredicate:
			col := ink.predicateLine
			if !seg.IsBackbone && seg.ClauseID != "" {
				col = withAlpha(col, 0.72)
			}
			drawMark(p, ui.Rect{X: spanRect.X, Y: markY - 0.5, W: spanRect.W, H: markHeight(markWavy)}, markWavy, col, seed)
		case RoleObject:
			col := ink.objectLine
			if !seg.IsBackbone || seg.ClauseID != "" {
				col = withAlpha(col, 0.58)
			}
			drawMark(p, ui.Rect{X: spanRect.X, Y: markY - 0.3, W: spanRect.W, H: markHeight(markDouble)}, markDouble, col, seed)
		case RoleConnector:
			if len(strings.TrimSpace(seg.Text)) <= 18 {
				oval := ui.Rect{X: spanRect.X - 3.5, Y: midY - 9.2, W: spanRect.W + 7, H: 18.4}
				drawConnectorOval(p, oval, ink.connectorRing, seed)
			} else {
				drawMark(p, ui.Rect{X: spanRect.X, Y: markY, W: spanRect.W, H: 5}, markUnderline, ink.connectorRing, seed)
			}
		case RoleModifier:
			if seg.PhraseID != "" {
				pInfo := phraseByID[seg.PhraseID]
				pc := grammarPhrasePalette(t, grammarPhraseShortTag(pInfo))
				kind := markDashed
				if isNonFinitePhrase(pInfo) {
					kind = markDotted
				}
				drawMark(p, ui.Rect{X: spanRect.X, Y: markY, W: spanRect.W, H: markHeight(kind)}, kind, pc.underline, seed)
			} else if j-i >= 2 || seg.Note != "" {
				drawMark(p, ui.Rect{X: spanRect.X, Y: markY, W: spanRect.W, H: markHeight(markDashed)}, markDashed, ink.adverbialLine, seed)
			}
		}

		if first.subTag != "" {
			lineKey := int(math.Round(float64(midY / 14.0)))
			span := ui.Span{Text: first.subTag, Font: fontUI, Size: 9.2, Weight: 600, Color: first.subTagColor}
			tw, _ := p.MeasureText(0, span)
			starOffset := float32(0)
			if first.lastInDiff {
				starOffset = 10.5
			}
			totalW := tw + starOffset
			tx := spanRect.X
			maxShift := max(spanRect.W-totalW+10, float32(18))
			canDraw := true
			if prevRight, exists := lastTagRightByLine[lineKey]; exists && tx < prevRight+4 {
				if tx+maxShift >= prevRight+4 {
					tx = prevRight + 4
				} else {
					canDraw = false
				}
			}
			if canDraw {
				tagY := midY + 9.0
				if first.lastInDiff {
					drawHandStar(p, ui.Rect{X: tx, Y: tagY + 1.2, W: 8.5, H: 8.5}, ink.obstacleStar, seed)
				}
				p.RichText(tx+starOffset, tagY, 0, span)
				lastTagRightByLine[lineKey] = tx + totalW
			}
		}

		i = j
	}
}

func renderGrammarWordElement(c *ui.Context, t *ui.Theme, at AppTheme, ink grammarInk, seg Segment, text string, clause *ClauseInfo, phrase *PhraseInfo) ui.Element {
	el := ui.Text(c, text).FontSize(14)

	switch {
	case seg.Role == RolePredicate:
		weight := 700
		if !seg.IsBackbone && seg.ClauseID != "" {
			weight = 600
		}
		el.TextColor(ink.predicate).FontWeight(weight)
	case seg.Role == RoleConnector:
		el.TextColor(ink.connectorText).FontWeight(700)
	case seg.IsBackbone && (seg.Role == RoleSubject || seg.Role == RoleObject):
		el.TextColor(ink.body).FontWeight(700)
	case seg.IsBackbone:
		el.TextColor(ink.body).FontWeight(600)
	case seg.Role == RolePunct:
		el.TextColor(ink.muted)
	case seg.PhraseID != "" && phrase != nil:
		pc := grammarPhrasePalette(t, grammarPhraseShortTag(*phrase))
		el.TextColor(pc.text).FontWeight(500)
	case seg.ClauseID != "" && clause != nil:
		if seg.Role == RoleSubject || seg.Role == RoleObject {
			el.TextColor(ink.body).FontWeight(500)
		} else {
			el.TextColor(ink.clauseText).FontWeight(500)
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
	return ui.Text(c, "\u00A0"+label+"\u00A0").FontSize(10).FontWeight(700).
		TextColor(pc.badgeText).Unselectable().
		Tooltip(phraseDetail(phrase))
}

func renderGrammarClauseBadge(c *ui.Context, t *ui.Theme, clause ClauseInfo) ui.Element {
	raw := grammarClauseLabel(clause)
	colors := grammarClausePalette(t, raw)
	label := strings.Join(strings.Split(raw, ""), "\u2060")
	return ui.Text(c, "\u00A0"+label+"\u00A0").FontSize(10).FontWeight(700).
		TextColor(colors.badgeText).Unselectable().
		Tooltip(grammarClauseTooltip(clause))
}

func renderGrammarSlash(c *ui.Context) ui.Element {
	return ui.Text(c, " / ").FontSize(13).FontWeight(500).
		TextColor(ui.Transparent).Unselectable()
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
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Spinner(c).Size(14, 14).TextColor(at.Accent)
			ui.Text(c, text).FontSize(12).FontWeight(500).TextColor(at.TextSec).
				LineHeight(1.45).Wrap().Grow(1)
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
			Tooltip("在阅读蛙式手绘句法标注与缩进句法树之间切换").
			OnChange(func() {
				if (viewIdx == 1) != st.ShowTree && st.OnToggleTree != nil {
					st.OnToggleTree()
				}
			})

		if grammarToggle(c, t, "只看主干", st.OnlyTrunk, "只留主句骨架，从句和修饰短语折叠成省略号").Clicked() && st.OnToggleTrunk != nil {
			st.OnToggleTrunk()
		}
		if grammarToggle(c, t, "断句", st.ShowSlashes, "在自然意群与从句边界显示手绘单/双断句斜线").Clicked() && st.OnToggleSlash != nil {
			st.OnToggleSlash()
		}

		ui.Spacer(c).Grow(1)

		ui.Row(c).Gap(6).AlignItems(ui.Center).Shrink(0).Children(func() {
			grammarLegendStatic(c, t, at, ink, "主语", "subject")
			grammarLegendStatic(c, t, at, ink, "谓语", "predicate")
			grammarLegendStatic(c, t, at, ink, "宾语", "object")
			grammarLegendStatic(c, t, at, ink, "从句", "clause")
			grammarLegendStatic(c, t, at, ink, "介短", "prep")
			grammarLegendStatic(c, t, at, ink, "非谓语", "nonfinite")
			grammarLegendStatic(c, t, at, ink, "同位/插", "appositive")
		})
	})
}

func grammarLegendStatic(c *ui.Context, t *ui.Theme, at AppTheme, ink grammarInk, label, kind string) {
	box := ui.Box(c).Padding(1, 3, 5, 3).Center()
	switch kind {
	case "subject":
		box.Tooltip("手绘单横线：主语（动作发起者）")
		box.Children(func() {
			ui.Text(c, label).FontSize(11).FontWeight(700).TextColor(ink.body).SingleLine()
		})
		box.DrawOver(func(p *ui.Painter, r ui.Rect) {
			drawMark(p, ui.Rect{X: r.X + 2, Y: r.Y + r.H - 5, W: max(r.W-4, 8), H: 4.5}, markUnderline, ink.subjectLine, 101)
		})
	case "predicate":
		box.Tooltip("手绘蓝波浪线：核心谓语动词")
		box.Children(func() {
			ui.Text(c, label).FontSize(11).FontWeight(700).TextColor(ink.predicate).SingleLine()
		})
		box.DrawOver(func(p *ui.Painter, r ui.Rect) {
			drawMark(p, ui.Rect{X: r.X + 2, Y: r.Y + r.H - 5.5, W: max(r.W-4, 8), H: 5.5}, markWavy, ink.predicateLine, 102)
		})
	case "object":
		box.Tooltip("手绘双横线：宾语 / 表语")
		box.Children(func() {
			ui.Text(c, label).FontSize(11).FontWeight(700).TextColor(ink.body).SingleLine()
		})
		box.DrawOver(func(p *ui.Painter, r ui.Rect) {
			drawMark(p, ui.Rect{X: r.X + 2, Y: r.Y + r.H - 5.5, W: max(r.W-4, 8), H: 5.5}, markDouble, ink.objectLine, 103)
		})
	case "clause":
		cc := grammarClausePalette(t, "宾从")
		box.Padding(1, 5, 2, 5).Radius(4).Background(at.Surface).Border(1, cc.badgeText.Alpha(0.50)).
			Tooltip("描边微标 + 绿圈引导词：各类从句")
		box.Children(func() {
			ui.Text(c, label).FontSize(10.5).FontWeight(700).TextColor(cc.badgeText).SingleLine()
		})
	case "prep":
		pc := grammarPhrasePalette(t, "介短")
		box.Tooltip("青靛色手绘弧括号 + 虚线：介词短语（定语/状语）")
		box.Children(func() {
			ui.Text(c, label).FontSize(11).FontWeight(600).TextColor(pc.badgeText).SingleLine()
		})
		box.DrawOver(func(p *ui.Painter, r ui.Rect) {
			drawMark(p, ui.Rect{X: r.X + 2, Y: r.Y + r.H - 5, W: max(r.W-4, 8), H: 4.5}, markDashed, pc.underline, 105)
		})
	case "nonfinite":
		pc := grammarPhrasePalette(t, "分词")
		box.Tooltip("暖琥珀手绘弧括号 + 圆点线：非谓语短语（分词 / 不定式 / 动名词）")
		box.Children(func() {
			ui.Text(c, label).FontSize(11).FontWeight(600).TextColor(pc.badgeText).SingleLine()
		})
		box.DrawOver(func(p *ui.Painter, r ui.Rect) {
			drawMark(p, ui.Rect{X: r.X + 2, Y: r.Y + r.H - 5, W: max(r.W-4, 8), H: 4.5}, markDotted, pc.underline, 106)
		})
	case "appositive":
		pc := grammarPhrasePalette(t, "同位")
		box.Padding(1, 6, 3, 6).Tooltip("梅紫手绘弧括号：同位语 / 插入语")
		box.Children(func() {
			ui.Text(c, label).FontSize(11).FontWeight(600).TextColor(pc.badgeText).SingleLine()
		})
		box.DrawOver(func(p *ui.Painter, r ui.Rect) {
			drawCurvedBracket(p, ui.Rect{X: r.X, Y: r.Y + 2, W: 4.5, H: r.H - 4}, true, pc.bracket, 107)
			drawCurvedBracket(p, ui.Rect{X: r.X + r.W - 4.5, Y: r.Y + 2, W: 4.5, H: r.H - 4}, false, pc.bracket, 108)
		})
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

