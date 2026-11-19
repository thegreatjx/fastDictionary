package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
)

func (a *app) copyToClipboard(text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	mygo.Clipboard.WriteText(text)
	a.mu.Lock()
	a.copiedMsg = "已复制 " + fmt.Sprintf("%d", len([]rune(text))) + " 字到剪贴板 ✓"
	a.mu.Unlock()
	if a.win != nil {
		a.win.Update(func() {})
	}
}

func (a *app) translate(text string) {
	a.translateText(cleanSourceForLayout(text, a.store.Get().LayoutMode, false))
}

func (a *app) translateKeepingLines(text string) {
	a.mu.Lock()
	a.rawSource = text
	a.mu.Unlock()
	a.translateTextWithReflow(cleanSourceForLayout(text, a.store.Get().LayoutMode, true), true)
}

// 每次新请求自增代次并取消上一个；旧 goroutine 的回包必须丢弃。
func (a *app) stopAIPassLocked() {
	a.aiGen++
	if a.aiCancel != nil {
		a.aiCancel()
		a.aiCancel = nil
	}
	a.aiRefineGen++
	if a.aiRefineCancel != nil {
		a.aiRefineCancel()
		a.aiRefineCancel = nil
	}
	a.aiBusy = false
	a.aiRefining = false
	a.aiRefineErr = ""
}

func (a *app) stopNotesLocked() {
	a.notesGen++
	if a.notesCancel != nil {
		a.notesCancel()
		a.notesCancel = nil
	}
	a.notesBusy = false
	a.notes = nil
}

func (a *app) translateText(text string) {
	a.translateTextWithReflow(text, false)
}

func (a *app) translateTextWithReflow(text string, keepParagraphs bool) {
	cfg := a.store.Get()
	mode := normalizeLayoutMode(cfg.LayoutMode)
	win := a.win
	from, to := translateLangs(cfg, text)
	transKey := translationCacheKey(cfg.EngineMode, from, to, mode, keepParagraphs, text)
	qc := a.ensureCache()
	nb := GetNotebookStore()

	a.mu.Lock()
	a.stopAIPassLocked()
	a.stopNotesLocked()
	freshTrans := a.skipTransCache
	a.skipTransCache = false
	a.source, a.target, a.errText = text, "", ""
	a.echoNote = ""
	a.sourceExpanded = false
	a.notesExpanded = false
	a.notesSaved = false
	a.grammarSaved = text != "" && nb.Contains("grammar", grammarAnalysisText(text))
	a.writingSaved = text != "" && nb.Contains("writing", text)
	a.engine, a.detected = "", ""
	a.dict, a.gdict, a.aiText, a.aiBusy = nil, nil, "", false
	a.copiedMsg = ""
	a.translateGen++
	gen := a.translateGen
	notesGen := a.notesGen
	if !freshTrans {
		if cached, hit := qc.getTranslation(transKey); hit && (!isWord(text) || !cfg.DictEnabled || cached.Dict != nil || cached.GDict != nil) {
			a.busy = false
			a.elapsed = 0
			a.target = cached.Target
			a.engine = cached.Engine
			a.detected = cached.Detected
			a.echoNote = cached.EchoNote
			a.dict = cached.Dict
			a.gdict = cached.GDict
			a.mu.Unlock()
			if win != nil {
				win.Update(func() {})
			}
			a.fitHeight()
			if isWord(text) && cfg.AutoPronounce != "off" {
				a.play(text, cfg.AutoPronounce == "us")
			}
			if cfg.ShowNotes && shouldExtractNotes(text) {
				a.extractNotesForGen(notesGen, text)
			}
			return
		}
	}
	a.busy = true
	a.mu.Unlock()
	if win != nil {
		win.Update(func() {})
	}

	go func() {
		defer recoverLog("popup_actions.go:translateTextWithReflow")
		start := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()

		r := TranslateRace(ctx, text, from, to, cfg.EngineMode)

		word := isWord(text)
		var yd *YoudaoDict
		var gd *DictResult
		if r.Err == nil && word && cfg.DictEnabled {
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer recoverLog("popup_actions.go:dict")
				defer wg.Done()
				yd, _ = LookupYoudao(ctx, text)
			}()
			go func() {
				defer recoverLog("popup_actions.go:gdict")
				defer wg.Done()
				gd, _ = GoogleDict(ctx, text, from, to)
			}()
			wg.Wait()
		}

		elapsed := time.Since(start)
		a.mu.Lock()
		if gen != a.translateGen {
			a.mu.Unlock()
			log.Printf("fastDictionary: dropped stale translation of %s", a.logText(text, 40))
			return
		}
		a.busy = false
		a.elapsed = elapsed
		if r.Err != nil {
			a.errText = r.Err.Error()
		} else if r.Untranslated {
			alt, usedAlt := raceResult{}, false
			if cfg.EngineMode != "race" {
				alt, usedAlt = pickEchoAnswer(r, TranslateRace(ctx, text, from, to, "race"))
			}
			if usedAlt {
				a.target = formatTranslationForLayout(alt.Text, mode, keepParagraphs)
				a.engine, a.detected = alt.Engine, alt.Detected
				if word {
					a.echoNote = r.Engine + "把「" + text + "」当作专有名词保留了原文，下面是" + alt.Engine + "的译文。"
				} else {
					a.echoNote = r.Engine + "没有给出译文，下面是" + alt.Engine + "的译文。"
				}
			} else {
				a.echoNote = r.Engine + "没有给出译文，返回的内容与原文相同：多半是专有名词或简称，也可能这组语言它不翻译。切到其它引擎可以拿到意译。"
				a.engine, a.detected = r.Engine, r.Detected
			}
			a.dict, a.gdict = yd, gd
		} else {
			a.target = formatTranslationForLayout(r.Text, mode, keepParagraphs)
			a.engine, a.detected = r.Engine, r.Detected
			a.dict, a.gdict = yd, gd
		}
		var toCache cachedTranslation
		shouldCache := r.Err == nil && a.target != ""
		if shouldCache {
			toCache = cachedTranslation{
				Target:   a.target,
				Engine:   a.engine,
				Detected: a.detected,
				EchoNote: a.echoNote,
				Dict:     a.dict,
				GDict:    a.gdict,
			}
		}
		a.mu.Unlock()
		if shouldCache {
			qc.putTranslation(transKey, toCache)
		}
		log.Printf("fastDictionary: %s -> %s engine=%s dict=%v in %v (err=%v)",
			a.logText(text, 40), a.logText(r.Text, 40), r.Engine, yd != nil, elapsed, r.Err)
		if win != nil {
			win.Update(func() {})
		}
		a.fitHeight()

		if r.Err == nil && word && cfg.AutoPronounce != "off" {
			a.play(text, cfg.AutoPronounce == "us")
		}
		if cfg.ShowNotes && shouldExtractNotes(text) {
			a.extractNotesForGen(notesGen, text)
		}
	}()
}

func (a *app) extractNotes(text string) {
	a.mu.Lock()
	gen := a.notesGen
	a.mu.Unlock()
	a.extractNotesForGen(gen, text)
}

func shouldExtractNotes(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" || isWord(text) {
		return false
	}
	latinWords, cjk := 0, 0
	for _, r := range text {
		if isCJK(r) {
			cjk++
		}
	}
	for _, field := range strings.Fields(text) {
		for _, r := range field {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				latinWords++
				break
			}
		}
	}
	return latinWords >= 4 || cjk >= 8
}

func filterDismissedNotes(raw []NoteItem, dismissed []string) []NoteItem {
	kept := make([]NoteItem, 0, len(raw))
	for _, item := range raw {
		if strings.TrimSpace(item.Word) == "" {
			continue
		}
		skip := false
		for _, word := range dismissed {
			if strings.EqualFold(strings.TrimSpace(word), strings.TrimSpace(item.Word)) {
				skip = true
				break
			}
		}
		if !skip {
			item.Checked = true
			kept = append(kept, item)
		}
		if len(kept) >= 5 {
			break
		}
	}
	return kept
}

func (a *app) extractNotesForGen(gen requestToken, text string) {
	cfg := a.store.Get()
	if cfg.AISetupProblem() != "" || !cfg.ShowNotes || !shouldExtractNotes(text) {
		a.mu.Lock()
		if gen == a.notesGen {
			a.stopNotesLocked()
		}
		a.mu.Unlock()
		return
	}

	notesKey := notesCacheKey(cfg.AIModel, text)
	a.mu.Lock()
	if gen != a.notesGen {
		a.mu.Unlock()
		return
	}
	freshNotes := a.skipNotesCache
	a.skipNotesCache = false
	if !freshNotes {
		if cachedRaw, ok := a.ensureCacheLocked().getNotes(notesKey); ok {
			filtered := filterDismissedNotes(cachedRaw, cfg.DismissedNoteWords)
			a.notes = filtered
			a.notesBusy = false
			a.notesCancel = nil
			win := a.win
			a.mu.Unlock()
			if win != nil {
				win.Update(func() {})
			}
			a.fitHeight()
			return
		}
	}
	a.stopNotesLocked()
	gen = a.notesGen
	a.notesBusy = true
	a.notes = nil
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	a.notesCancel = cancel
	a.mu.Unlock()
	if a.win != nil {
		a.win.Update(func() {})
	}

	go func() {
		defer recoverLog("popup_actions.go:extractNotesForGen")
		defer cancel()

		systemPrompt := "你是一个面向语言学习者的专业词典与背单词助手。\n" +
			"请从用户选中的文本中，分析并挑选出 3 到 5 个最值得学习者记忆的核心生词。\n" +
			"必须为每个生词输出完整的词典数据（包含标准IPA音标、词性、精准释义、CEFR难度等级如A2/B1/B2/C1、简练词源记忆法）。\n" +
			"严格以紧凑的 JSON 数组格式输出，不要有任何 Markdown 标记或多余文字。\n" +
			"示例格式：[{\"word\":\"ambitious\",\"phonetic\":\"/æmˈbɪʃ.əs/\",\"pos\":\"adj.\",\"meaning\":\"有雄心的，抱负不凡的；宏大的\",\"difficulty\":\"B2\",\"memory_tips\":\"词源：ambi-（周围、环顾）+ -it-（走）+ -ious（形容词）→ 四处奔走谋求发展 → 雄心勃勃的\"}]"

		raw, err := StreamAI(ctx, cfg.AIConn(cfg.AIModel), systemPrompt, text, cfg.AIReasoningEffortFor(cfg.AIModel), nil)
		var items []NoteItem
		if err == nil && raw != "" {
			cleaned := strings.TrimSpace(raw)
			if strings.HasPrefix(cleaned, "```json") {
				cleaned = strings.TrimPrefix(cleaned, "```json")
			}
			if strings.HasPrefix(cleaned, "```") {
				cleaned = strings.TrimPrefix(cleaned, "```")
			}
			cleaned = strings.TrimSuffix(cleaned, "```")
			cleaned = strings.TrimSpace(cleaned)
			if err := json.Unmarshal([]byte(cleaned), &items); err != nil {
				items = nil
			}
		}

		validItems := make([]NoteItem, 0, len(items))
		for _, item := range items {
			if strings.TrimSpace(item.Word) != "" {
				validItems = append(validItems, item)
			}
		}
		if len(validItems) > 0 {
			a.ensureCache().putNotes(notesKey, validItems)
		}
		items = filterDismissedNotes(validItems, cfg.DismissedNoteWords)

		a.mu.Lock()
		if gen != a.notesGen {
			a.mu.Unlock()
			return
		}
		if len(items) > 0 {
			a.notes = items
		}
		a.notesBusy = false
		a.notesCancel = nil
		a.mu.Unlock()
		if a.win != nil {
			a.win.Update(func() {})
		}
	}()
}

func (a *app) setNoteChecked(idx int, word string, checked bool) {
	a.mu.Lock()
	if idx >= 0 && idx < len(a.notes) && a.notes[idx].Word == word {
		a.notes[idx].Checked = checked
	}
	a.mu.Unlock()
	if a.win != nil {
		a.win.Update(func() {})
	}
}

func (a *app) dismissNoteWord(word string) {
	word = strings.TrimSpace(word)
	if word == "" {
		return
	}
	a.store.Update(func(c *Config) {
		for _, item := range c.DismissedNoteWords {
			if strings.EqualFold(strings.TrimSpace(item), word) {
				return
			}
		}
		c.DismissedNoteWords = append(c.DismissedNoteWords, word)
	})
	a.mu.Lock()
	kept := a.notes[:0]
	for _, item := range a.notes {
		if !strings.EqualFold(strings.TrimSpace(item.Word), word) {
			kept = append(kept, item)
		}
	}
	a.notes = kept
	a.mu.Unlock()
	if a.win != nil {
		a.win.Update(func() {})
	}
	a.fitHeight()
}

func (a *app) saveDictToVocab(res *DictionaryResult) {
	if res == nil {
		return
	}
	term := res.Term
	if term == "" {
		term = a.source
	}
	entry := VocabularyEntry{
		Term:               term,
		Phonetic:           res.Phonetic,
		PartOfSpeech:       res.PartOfSpeech,
		Definition:         res.Definition,
		Context:            res.Context,
		ContextTerm:        res.ContextTerm,
		ContextTranslation: res.ContextTranslation,
		MemoryTips:         res.MemoryTips,
		Difficulty:         res.Difficulty,
		Source:             "dict",
		CreatedAt:          time.Now(),
	}
	vocabStore := GetVocabularyStore()
	vocabStore.Add(entry)
	a.mu.Lock()
	a.savedDictTerm = term
	if err := vocabStore.LastSaveError(); err != nil {
		a.savedDictTerm = ""
		a.saveErr = "保存失败: " + err.Error()
	} else {
		a.saveErr = ""
	}
	a.mu.Unlock()
	if a.win != nil {
		a.win.Update(func() {})
	}
}

func (a *app) saveGrammarToNotebook(res *SentenceAnalysisResult) {
	if res == nil {
		return
	}
	a.mu.Lock()
	src := a.source
	a.mu.Unlock()
	text := grammarAnalysisText(src)
	if text == "" {
		text = src
	}
	entry := NotebookEntry{
		Kind:        "grammar",
		Source:      text,
		Translation: res.Translation,
		Grammar:     res,
		CreatedAt:   time.Now(),
	}
	notebookStore := GetNotebookStore()
	notebookStore.Add(entry)
	a.mu.Lock()
	a.grammarSaved = true
	if err := notebookStore.LastSaveError(); err != nil {
		a.grammarSaved = false
		a.saveErr = "保存失败: " + err.Error()
	} else {
		a.saveErr = ""
	}
	a.mu.Unlock()
	if a.win != nil {
		a.win.Update(func() {})
	}
}

func (a *app) saveWritingToNotebook(res *ImproveWritingResult) {
	if res == nil {
		return
	}
	a.mu.Lock()
	src := a.source
	a.mu.Unlock()
	entry := NotebookEntry{
		Kind:        "writing",
		Source:      src,
		Translation: res.Improved,
		Writing:     res,
		CreatedAt:   time.Now(),
	}
	notebookStore := GetNotebookStore()
	notebookStore.Add(entry)
	a.mu.Lock()
	a.writingSaved = true
	if err := notebookStore.LastSaveError(); err != nil {
		a.writingSaved = false
		a.saveErr = "保存失败: " + err.Error()
	} else {
		a.saveErr = ""
	}
	a.mu.Unlock()
	if a.win != nil {
		a.win.Update(func() {})
	}
}

func (a *app) prefillActionTranslation(gen requestToken, text, targetLang string) {
	cfg := a.store.Get()
	prefillCfg := cfg
	if targetLang != "" {
		prefillCfg.TargetLang = targetLang
	}
	from, to := translateLangs(prefillCfg, text)
	go func() {
		defer recoverLog("popup_actions.go:prefillActionTranslation")
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		r := TranslateRace(ctx, text, from, to, cfg.EngineMode)
		if r.Err != nil || r.Untranslated || strings.TrimSpace(r.Text) == "" {
			return
		}
		a.mu.Lock()
		if gen != a.aiGen {
			a.mu.Unlock()
			return
		}
		if a.grammarResult == nil {
			a.grammarResult = &SentenceAnalysisResult{}
		}
		if a.grammarResult.Translation == "" {
			a.grammarResult.Translation = flattenTranslation(r.Text)
			a.grammarResultPrefill = true
		}
		win := a.win
		a.mu.Unlock()
		if win != nil {
			win.Update(func() {})
		}
	}()
}

func aiResultCacheKey(actionID, model, targetLang, text string, variant ...string) string {
	key := actionID + "\x00" + model + "\x00" + targetLang + "\x00" + normalizeCacheText(text)
	for _, v := range variant {
		if v != "" {
			key += "\x00" + v
		}
	}
	return key
}

func defaultActionByID(id string) *CustomActionConfig {
	for _, a := range DefaultCustomActions() {
		if a.ID == id {
			copy := a
			return &copy
		}
	}
	return nil
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sameActionContent(act, def CustomActionConfig) bool {
	return act.ID == def.ID &&
		act.ProviderID == def.ProviderID &&
		act.Model == def.Model &&
		act.SystemPrompt == def.SystemPrompt &&
		act.Prompt == def.Prompt &&
		sameStrings(act.OutputSchema, def.OutputSchema)
}

func actionFingerprint(act *CustomActionConfig, cfg Config) string {
	if act == nil {
		return ""
	}
	h := fnv.New64a()
	fmt.Fprintf(h, "%q\x00%q\x00%q\x00%q\x00%q\x00%q\x00%q\x00%q",
		act.ID, act.ProviderID, act.Model, act.SystemPrompt, act.Prompt,
		normalizeAIProvider(cfg.AIProvider), cfg.AIURL, cfg.AIEffort)
	for _, field := range act.OutputSchema {
		fmt.Fprintf(h, "\x00%q", field)
	}
	return fmt.Sprintf("%x", h.Sum64())
}

func actionCacheVariant(act *CustomActionConfig, cfg Config) string {
	if act == nil {
		return ""
	}
	if def := defaultActionByID(act.ID); def != nil &&
		sameActionContent(*act, *def) &&
		cfg.IsMagpie() &&
		cfg.AIURL == DefaultConfig().AIURL &&
		cfg.AIEffort == DefaultConfig().AIEffort {
		return ""
	}
	return actionFingerprint(act, cfg)
}

func (a *app) cachedAIResult(key string) string {
	return a.ensureCacheLocked().getAI(key)
}

func (a *app) storeAIResult(key, raw string) {
	a.ensureCacheLocked().putAI(key, raw)
}

func (a *app) aiCacheCount() int {
	a.mu.Lock()
	qc := a.ensureCacheLocked()
	a.mu.Unlock()
	return qc.aiCount()
}

func (a *app) clearAICache() {
	a.mu.Lock()
	qc := a.ensureCacheLocked()
	a.skipAIResultCache = false
	a.skipTransCache = false
	a.skipNotesCache = false
	a.mu.Unlock()
	qc.clear()
}

func (a *app) applyParsedAction(actionID, raw, source string) bool {
	switch actionID {
	case "default-dictionary":
		res := ParseDictionaryResult(raw)
		a.dictResult = res
		return res != nil
	case "default-sentence-analysis":
		parse := ParseSentenceAnalysisDetailed(raw, source)
		parsed := parse.Result
		if parsed == nil {
			if parse.Outcome == parseOutcomeFailed {
				log.Printf("fastDictionary: grammar answer could not be parsed: %v", parse.Cause)
			}
			return false
		}
		if parse.Outcome == parseOutcomeTranslationOnly {
			log.Printf("fastDictionary: grammar answer carried a translation but no segments")
		}
		if parse.Outcome == parseOutcomeFallbackDirect {
			log.Printf("fastDictionary: grammar answer could not be aligned with the source sentence")
		}
		if parsed.Translation == "" && a.grammarResultPrefill && a.grammarResult != nil {
			parsed.Translation = a.grammarResult.Translation
		} else if parsed.Translation != "" {
			parsed.Translation = flattenTranslation(parsed.Translation)
		}
		a.grammarResult = parsed
		a.grammarResultPrefill = false
		return true
	case "default-improve-writing":
		res := ParseImproveWritingResult(raw)
		a.writingResult = res
		return res != nil
	}
	return false
}

func (a *app) runAction(actionID string, text string) {
	a.mu.Lock()
	fresh := a.skipAIResultCache
	a.skipAIResultCache = false
	a.mu.Unlock()
	cfg := a.store.Get()
	if cfg.AISetupProblem() != "" || strings.TrimSpace(text) == "" {
		return
	}
	act := cfg.GetAction(actionID)
	if act == nil {
		return
	}

	targetText := text
	if actionID == "default-sentence-analysis" {
		targetText = grammarAnalysisText(text)
	}

	model := act.Model
	if model == "" {
		model = cfg.AIModel
	}
	cacheKey := aiResultCacheKey(actionID, model, cfg.TargetLang, targetText, actionCacheVariant(act, cfg))
	nb := GetNotebookStore()

	a.mu.Lock()
	a.stopAIPassLocked()
	gen := a.aiGen
	useCache := !fresh
	raw := ""
	if useCache {
		raw = a.cachedAIResult(cacheKey)
	}
	a.mu.Unlock()

	if raw != "" {
		a.mu.Lock()
		if gen != a.aiGen {
			a.mu.Unlock()
			return
		}
		a.aiBusy = false
		a.aiText = ""
		a.errText = ""
		a.activeActionID = actionID
		a.grammarResultPrefill = false
		a.applyParsedAction(actionID, raw, targetText)
		if actionID == "default-sentence-analysis" {
			a.grammarSaved = nb.Contains("grammar", targetText)
		} else if actionID == "default-improve-writing" {
			a.writingSaved = nb.Contains("writing", targetText)
		}
		a.mu.Unlock()
		if a.win != nil {
			a.win.Update(func() {})
			a.fitHeight()
		}
		return
	}

	win := a.win
	a.mu.Lock()
	if gen != a.aiGen {
		a.mu.Unlock()
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	a.aiCancel = cancel
	a.activeActionID = actionID
	a.aiText = ""
	a.aiBusy = true
	a.errText = ""
	a.actionSavedMsg = ""
	a.saveErr = ""
	a.copiedMsg = ""
	a.savedDictTerm = ""
	switch actionID {
	case "default-dictionary":
		a.dictResult = nil
	case "default-sentence-analysis":
		a.grammarResult = nil
		a.grammarSaved = nb.Contains("grammar", targetText)
	case "default-improve-writing":
		a.writingResult = nil
		a.writingSaved = nb.Contains("writing", targetText)
	}
	a.mu.Unlock()
	if win != nil {
		win.Update(func() {})
	}

	go func() {
		defer recoverLog("popup_actions.go:runAction")
		defer cancel()
		targetLangName := langLabels[cfg.TargetLang]
		if targetLangName == "" {
			targetLangName = "简体中文"
		}

		if actionID == "default-sentence-analysis" {
			a.prefillActionTranslation(gen, targetText, cfg.TargetLang)
		}

		userPrompt := FormatActionPrompt(act.Prompt, targetText, "", targetLangName)
		systemPrompt := act.SystemPrompt

		var lastPaint time.Time
		var buf strings.Builder
		_, err := StreamAI(ctx, cfg.AIConn(model), systemPrompt, userPrompt, cfg.AIReasoningEffortFor(model), func(chunk string) {
			buf.WriteString(chunk)
			due := time.Since(lastPaint) >= 40*time.Millisecond
			if due {
				lastPaint = time.Now()
			}
			if !due {
				return
			}
			text := buf.String()
			a.mu.Lock()
			if gen != a.aiGen {
				a.mu.Unlock()
				return
			}
			a.aiText = text
			a.mu.Unlock()
			if win != nil {
				win.Update(func() {})
			}
		})

		a.mu.Lock()
		if gen != a.aiGen {
			a.mu.Unlock()
			log.Printf("fastDictionary: dropped stale AI result for %s", actionID)
			return
		}
		a.aiBusy = false
		if err != nil {
			a.errText = friendlyAIError(err)
		}
		if buf.Len() > 0 {
			a.aiText = buf.String()
		}
		if a.aiText != "" {
			if a.applyParsedAction(actionID, a.aiText, targetText) {
				a.storeAIResult(cacheKey, a.aiText)
			}
		}
		a.aiCancel = nil
		a.mu.Unlock()
		if win != nil {
			win.Update(func() {})
			a.fitHeight()
		}
	}()
}

func (a *app) refineWithAI() {
	cfg := a.store.Get()
	if problem := cfg.AISetupProblem(); problem != "" {
		a.mu.Lock()
		a.aiRefineErr = problem
		a.mu.Unlock()
		if a.win != nil {
			a.win.Update(func() {})
		}
		return
	}

	a.mu.Lock()
	source := a.source
	isScreenshotMode, ocrPath := a.isScreenshotMode, a.ocrPath
	_, refineTo := translateLangs(cfg, source)
	a.aiRefineGen++
	if a.aiRefineCancel != nil {
		a.aiRefineCancel()
		a.aiRefineCancel = nil
	}
	gen := a.aiRefineGen
	refineKey := aiResultCacheKey("ai-refine", cfg.AIModel, refineTo, source,
		normalizeAIProvider(cfg.AIProvider), cfg.AIURL, cfg.AIReasoningEffortFor(cfg.AIModel))
	if strings.TrimSpace(source) != "" && a.engine != "AI 精翻" {
		if cached := a.cachedAIResult(refineKey); cached != "" {
			a.target = cached
			a.engine = "AI 精翻"
			a.aiRefining = false
			a.aiRefineErr = ""
			a.elapsed = 0
			win := a.win
			a.mu.Unlock()
			if win != nil {
				win.Update(func() {})
				a.fitHeight()
			}
			return
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	a.aiRefineCancel = cancel
	a.aiRefining = true
	a.aiRefineErr = ""
	win := a.win
	a.mu.Unlock()
	if win != nil {
		win.Update(func() {})
	}

	go func() {
		defer recoverLog("popup_actions.go:refineWithAI")
		defer cancel()
		sys := "You are a translator. Translate the text into " + langPromptName(refineTo) +
			". Keep terminology accurate, keep the original's tone and structure, and output only the translation with no explanation."
		started := time.Now()
		effort := cfg.AIReasoningEffortFor(cfg.AIModel)
		out, err := StreamAI(ctx, cfg.AIConn(cfg.AIModel), sys, source, effort, nil)
		if err != nil && ctx.Err() == nil && isTransientAIError(err) {
			time.Sleep(350 * time.Millisecond)
			out, err = StreamAI(ctx, cfg.AIConn(cfg.AIModel), sys, source, effort, nil)
		}
		if err != nil && isScreenshotMode && ocrPath != "" && ctx.Err() == nil {
			if dataURL, imageErr := compressedImageFileDataURL(ocrPath); imageErr == nil {
				log.Printf("fastDictionary: text AI refine failed, retrying with compressed screenshot: %v", err)
				out, err = StreamAIWithImage(ctx, cfg.AIConn(cfg.AIModel),
					screenshotRefineSystemPrompt(refineTo), screenshotRefineUserPrompt(source),
					dataURL, effort, nil)
			} else {
				log.Printf("fastDictionary: screenshot AI fallback unavailable: %v", imageErr)
			}
		}

		a.mu.Lock()
		if gen != a.aiRefineGen {
			a.mu.Unlock()
			log.Printf("fastDictionary: dropped stale AI refinement")
			return
		}
		a.aiRefining = false
		a.aiRefineCancel = nil
		if err != nil {
			a.aiRefineErr = friendlyAIError(err)
		} else if text := strings.TrimSpace(out); text != "" {
			if refineEchoedInput(source, text) {
				a.aiRefineErr = "模型把原文原样返回了，换个目标语言再试"
				a.mu.Unlock()
				if win != nil {
					win.Update(func() {})
				}
				return
			}
			refined := flattenTranslation(text)
			a.target = refined
			a.engine = "AI 精翻"
			a.aiRefineErr = ""
			a.elapsed = time.Since(started)
			a.storeAIResult(refineKey, refined)
		}
		a.mu.Unlock()
		if win != nil {
			win.Update(func() {})
			a.fitHeight()
		}
	}()
}

func isTransientAIError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var failure *aiFailure
	if !errors.As(err, &failure) {
		return false
	}
	switch failure.kind {
	case aiFailureRateLimit, aiFailureServer, aiFailureNetwork, aiFailureTimeout:
		return true
	}
	return false
}

func friendlyAIError(err error) string {
	if err == nil {
		return ""
	}
	var failure *aiFailure
	if errors.As(err, &failure) {
		switch failure.kind {
		case aiFailureAuth:
			return "API Key 无效或已过期，请在 设置 → AI 精翻 里重新填写。"
		case aiFailurePermission:
			return "服务拒绝访问：检查模型是否开通、余额是否充足、地区是否支持。"
		case aiFailureNotFound:
			return "地址或模型不存在，请检查 API 地址和模型 ID。"
		case aiFailureRateLimit:
			return "请求过于频繁或额度用完，请稍后再试。"
		case aiFailureBadRequest:
			return "服务拒绝了请求：当前模型可能不支持图片输入，或请求格式不兼容；请检查模型 ID 和服务商文档。"
		case aiFailureServer:
			return "服务商暂时故障，请稍后重试。"
		case aiFailureTimeout:
			return "网络超时，请检查网络或代理。"
		case aiFailureNetwork:
			return "无法连接 AI 服务，请检查地址、网络或代理。"
		case aiFailureMagpieDown:
			return "连不上本机 Magpie，请在设置里换一个服务商，或先启动 Magpie。"
		default:
			return failure.msg
		}
	}
	switch {
	case errors.Is(err, context.Canceled):
		return "请求已取消。"
	case errors.Is(err, context.DeadlineExceeded):
		return "网络超时，请稍后重试。"
	}
	return err.Error()
}

func refineEchoedInput(source, out string) bool {
	norm := func(s string) string {
		return strings.ToLower(strings.Join(strings.Fields(s), " "))
	}
	source, out = norm(source), norm(out)
	return source != "" && source == out
}

func langPromptName(tag string) string {
	if name := langLabels[tag]; name != "" {
		switch {
		case strings.HasPrefix(tag, "zh-Hans"):
			return "Simplified Chinese"
		case strings.HasPrefix(tag, "zh-Hant"):
			return "Traditional Chinese"
		}
		return name
	}
	return "the target language"
}

func (a *app) saveCurrentWordToVocab(s snapshot) {
	if s.source == "" {
		return
	}
	phone := ""
	if s.dict != nil {
		if s.dict.UKPhone != "" {
			phone = "/" + s.dict.UKPhone + "/"
		} else if s.dict.USPhone != "" {
			phone = "/" + s.dict.USPhone + "/"
		}
	}
	explanation, pos := s.target, ""
	if s.gdict != nil && len(s.gdict.Entries) > 0 {
		pos = s.gdict.Entries[0].POS
	} else if s.dict != nil && len(s.dict.Explains) > 0 {
		explanation = strings.Join(s.dict.Explains, "；")
		parts := strings.SplitN(s.dict.Explains[0], " ", 2)
		if len(parts) == 2 && strings.HasSuffix(parts[0], ".") {
			pos = parts[0]
		}
	}
	a.mu.Lock()
	a.savedDictTerm = s.source
	a.saveErr = ""
	a.mu.Unlock()
	if a.win != nil {
		a.win.Update(func() {})
	}
	entry := VocabularyEntry{
		Term: s.source, Phonetic: phone, PartOfSpeech: pos,
		Definition: explanation, Source: "dict", CreatedAt: time.Now(),
	}
	go func() {
		defer recoverLog("popup_actions.go:saveCurrentWordToVocab")
		EnrichVocabularyEntry(&entry)
		GetVocabularyStore().Add(entry)
	}()
}

func (a *app) saveNotesToVocab(s snapshot) {
	if len(s.notes) == 0 {
		return
	}
	a.mu.Lock()
	a.notesSaved = true
	a.saveErr = ""
	pending := make([]VocabularyEntry, 0, len(s.notes))
	for _, n := range s.notes {
		if n.Checked {
			pending = append(pending, VocabularyEntry{
				Term:               n.Word,
				Phonetic:           n.Phonetic,
				PartOfSpeech:       n.Pos,
				Definition:         n.Meaning,
				Difficulty:         n.Difficulty,
				MemoryTips:         n.MemoryTips,
				Context:            s.source,
				ContextTerm:        n.Word,
				ContextTranslation: s.target,
				Source:             "notes",
				CreatedAt:          time.Now(),
			})
		}
	}
	a.mu.Unlock()
	if a.win != nil {
		a.win.Update(func() {})
	}
	go func() {
		defer recoverLog("popup_actions.go:saveNotesToVocab")
		for i := range pending {
			EnrichVocabularyEntry(&pending[i])
		}
		GetVocabularyStore().AddBatch(pending)
	}()
}

func (a *app) handleQuickSave(s snapshot) {
	switch s.mode {
	case modeGrammar:
		if cardReadyForMode(s) && !s.grammarSaved {
			a.saveGrammarToNotebook(s.grammarResult)
		}
	case modeWriting:
		if cardReadyForMode(s) && !s.writingSaved {
			a.saveWritingToNotebook(s.writingResult)
		}
	case modeDictionary:
		if cardReadyForMode(s) && (s.savedDictTerm == "" || !strings.EqualFold(s.savedDictTerm, s.dictResult.Term)) {
			a.saveDictToVocab(s.dictResult)
		}
	case modeTranslate:
		if s.isWord && s.source != "" {
			isSaved := s.savedDictTerm != "" && strings.EqualFold(strings.TrimSpace(s.savedDictTerm), strings.TrimSpace(s.source))
			if !isSaved {
				a.saveCurrentWordToVocab(s)
			}
		} else if len(s.notes) > 0 && !s.notesSaved {
			a.saveNotesToVocab(s)
		} else if s.source != "" && s.target != "" {
			a.saveTranslationToVocab(s.source, s.target)
		}
	}
}
