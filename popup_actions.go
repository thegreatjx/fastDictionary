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

// Both copy buttons used to write to the clipboard silently. The clipboard is
// invisible by design, so a click with no reaction is indistinguishable from a
// click that missed — and the "复制" buttons sit next to 朗读, which does make
// noise, so silence read as a broken button. The message follows the same
// pattern as actionSavedMsg on the 存入生词库 button.
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

// translate runs the fast path: engine race, then dictionary data for words.
func (a *app) translate(text string) {
	a.translateText(CleanSelectionText(text))
}

// translateKeepingLines is translate for text whose line breaks carry meaning,
// which is OCR output: the engine reports the page's own lines, and a screenshot
// of a paragraph translated as one run-on line is harder to read than the image
// was. CleanSelectionText folds newlines into spaces, so it cannot be used here.
func (a *app) translateKeepingLines(text string) {
	a.translateTextWithReflow(CleanOcrText(text), true)
}

// stopAIPassLocked invalidates the AI pass in flight. The generation and the
// cancel have to move together: cancelling alone leaves the old goroutine
// running until its HTTP request returns, and its teardown would then write a
// stale result over the newer request's card. The caller must hold a.mu.
func (a *app) stopAIPassLocked() {
	a.aiGen++
	if a.aiCancel != nil {
		a.aiCancel()
		a.aiCancel = nil
	}
	// The old goroutine drops its result at the generation check, so it will
	// not clear these itself. Leaving them set would pin the loading indicator
	// (or the 精翻中 button) on the newer card forever.
	a.aiBusy = false
	a.aiRefining = false
	a.aiRefineErr = ""
}

// stopNotesLocked invalidates and cancels the "猜你想存" request. Like an AI
// pass, it may still be in its 20-second HTTP call when the user closes the
// popup or starts another translation. Caller must hold a.mu.
func (a *app) stopNotesLocked() {
	a.notesGen++
	if a.notesCancel != nil {
		a.notesCancel()
		a.notesCancel = nil
	}
	a.notesBusy = false
	a.notes = nil
}

// translateText is the body of translate, with the caller's cleaning already
// applied.
func (a *app) translateText(text string) {
	a.translateTextWithReflow(text, false)
}

// translateTextWithReflow is translateText with control over whether the
// engine's line breaks are kept. OCR output carries the page's paragraphs, so
// the screenshot path asks for reflow; ordinary selections stay on one line.
func (a *app) translateTextWithReflow(text string, keepParagraphs bool) {
	cfg := a.store.Get()
	win := a.win

	a.mu.Lock()
	a.stopAIPassLocked()
	a.stopNotesLocked()
	a.source, a.target, a.errText = text, "", ""
	a.sourceExpanded = false
	a.notesExpanded = false
	a.notesSaved = false
	a.grammarSaved = false
	a.engine, a.detected = "", ""
	a.dict, a.gdict, a.aiText, a.aiBusy = nil, nil, "", false
	// A fresh translation invalidates the "已复制" confirmation: the text it
	// referred to is no longer on screen.
	a.copiedMsg = ""
	a.busy = true
	// A new translation invalidates the one in flight. Without this, pressing
	// translate again while the first request was still running let the older
	// answer land last and replace the newer text on screen.
	a.translateGen++
	gen := a.translateGen
	notesGen := a.notesGen
	a.mu.Unlock()
	if win != nil {
		win.Update(func() {})
	}

	go func() {
		start := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()

		from := detectLocal(text)
		to := cfg.TargetLang
		if from == cfg.TargetLang || sameLang(from, to) {
			to = cfg.SecondLang // reading your own language: translate out
		}

		r := TranslateRace(ctx, text, from, to, cfg.EngineMode)

		word := isWord(text)
		var yd *YoudaoDict
		var gd *DictResult
		if r.Err == nil && word && cfg.DictEnabled {
			var wg sync.WaitGroup
			wg.Add(2)
			go func() { defer wg.Done(); yd, _ = LookupYoudao(ctx, text) }()
			go func() { defer wg.Done(); gd, _ = GoogleDict(ctx, text, from, to) }()
			wg.Wait()
		}

		elapsed := time.Since(start)
		a.mu.Lock()
		if gen != a.translateGen {
			// A newer translation started while this one was in flight: its
			// answer is the one the user is waiting for, so drop this one
			// instead of flashing a stale result.
			a.mu.Unlock()
			log.Printf("fastDictionary: dropped stale translation of %q", truncateFor(text, 40))
			return
		}
		a.busy = false
		a.elapsed = elapsed
		if r.Err != nil {
			a.errText = r.Err.Error()
		} else {
			if keepParagraphs {
				a.target = reflowTranslation(r.Text)
			} else {
				a.target = flattenTranslation(r.Text)
			}
			a.engine, a.detected = r.Engine, r.Detected
			a.dict, a.gdict = yd, gd
		}
		a.mu.Unlock()
		log.Printf("fastDictionary: %q -> %q engine=%s dict=%v in %v (err=%v)",
			truncateFor(text, 40), truncateFor(r.Text, 40), r.Engine, yd != nil, elapsed, r.Err)
		if win != nil {
			win.Update(func() {})
		}
		a.fitHeight()

		// Auto pronunciation for single words, when configured.
		if r.Err == nil && word && cfg.AutoPronounce != "off" {
			a.play(text, cfg.AutoPronounce == "us")
		}
		// If it's a long sentence, trigger Read-Frog "猜你想存" extraction in
		// background. A single word or a short phrase is already the job of
		// the dictionary card; recommending words from it only adds noise.
		if cfg.ShowNotes && shouldExtractNotes(text) {
			a.extractNotesForGen(notesGen, text)
		}
	}()
}

// extractNotes runs the opt-in "猜你想存" note extraction. Hiding the
// recommendation must also stop its otherwise invisible AI request.
//
// This one-argument form is kept for callers that are already looking at the
// current card; it captures the current generation before starting.
func (a *app) extractNotes(text string) {
	a.mu.Lock()
	gen := a.notesGen
	a.mu.Unlock()
	a.extractNotesForGen(gen, text)
}

// extractNotesForGen starts a note extraction only if gen still identifies the
// current card. The request can take 20 seconds, so both its start and its
// teardown must be checked: a new translation or popup may have replaced the
// card long before the old answer arrives.
// shouldExtractNotes decides whether a text is long enough to deserve word
// recommendations. Counting characters alone treated a single long English
// word as a sentence; a Chinese paragraph of eight characters is meaningful
// while four English words are the equivalent minimum.
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

func (a *app) extractNotesForGen(gen uint64, text string) {
	cfg := a.store.Get()
	if !cfg.AIEnabled || !cfg.ShowNotes || !shouldExtractNotes(text) {
		a.mu.Lock()
		if gen == a.notesGen {
			a.stopNotesLocked()
		}
		a.mu.Unlock()
		return
	}

	a.mu.Lock()
	if gen != a.notesGen {
		a.mu.Unlock()
		return
	}
	// Invalidate the previous extraction before this one becomes current.
	// stopNotesLocked increments the generation again, so capture the new one
	// for the goroutine's teardown check.
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
		defer cancel()

		systemPrompt := "你是一个面向语言学习者的专业词典与背单词助手。\n" +
			"请从用户选中的文本中，分析并挑选出 3 到 5 个最值得学习者记忆的核心生词。\n" +
			"必须为每个生词输出完整的词典数据（包含标准IPA音标、词性、精准释义、CEFR难度等级如A2/B1/B2/C1、简练词源记忆法）。\n" +
			"严格以紧凑的 JSON 数组格式输出，不要有任何 Markdown 标记或多余文字。\n" +
			"示例格式：[{\"word\":\"ambitious\",\"phonetic\":\"/æmˈbɪʃ.əs/\",\"pos\":\"adj.\",\"meaning\":\"有雄心的，抱负不凡的；宏大的\",\"difficulty\":\"B2\",\"memory_tips\":\"词源：ambi-（周围、环顾）+ -it-（走）+ -ious（形容词）→ 四处奔走谋求发展 → 雄心勃勃的\"}]"

		raw, err := StreamMagpie(ctx, cfg.AIURL, cfg.AIModel, systemPrompt, text, ResolveEffort(cfg.AIEffort, cfg.AIModel), nil)
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

		dismissed := cfg.DismissedNoteWords
		kept := items[:0]
		for _, item := range items {
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
		}
		items = kept
		if len(items) > 5 {
			items = items[:5]
		}

		a.mu.Lock()
		if gen != a.notesGen {
			// A newer popup or translation owns the note fields now.
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

// dismissNoteWord remembers that the user never wants this recommendation
// again, both for the current card and for future popups. It is a preference,
// so it goes through the config store rather than a transient app field.
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
	GetVocabularyStore().Add(entry)
	a.mu.Lock()
	a.actionSavedMsg = "已保存「" + term + "」到生词本 ✓"
	// Remember which entry was just saved so the card's own button can read
	// 已保存 instead of inviting a second click that would add a duplicate.
	a.savedDictTerm = term
	a.mu.Unlock()
	if a.win != nil {
		a.win.Update(func() {})
	}
}

func (a *app) saveGrammarToVocab(res *SentenceAnalysisResult) {
	if res == nil {
		return
	}
	term := truncateFor(a.source, 24)
	entry := VocabularyEntry{
		Term:               term,
		Context:            a.source,
		ContextTranslation: res.Translation,
		Source:             "grammar",
		CreatedAt:          time.Now(),
	}
	GetVocabularyStore().Add(entry)
	a.mu.Lock()
	a.actionSavedMsg = "长难句已保存到生词本 ✓"
	a.mu.Unlock()
	if a.win != nil {
		a.win.Update(func() {})
	}
}

func (a *app) saveWritingToVocab(res *ImproveWritingResult) {
	if res == nil {
		return
	}
	term := truncateFor(a.source, 24)
	entry := VocabularyEntry{
		Term:               term,
		Definition:         res.Improved,
		Context:            a.source,
		ContextTranslation: res.Summary,
		Source:             "writing",
		CreatedAt:          time.Now(),
	}
	GetVocabularyStore().Add(entry)
	a.mu.Lock()
	a.actionSavedMsg = "写作改进已保存到生词本 ✓"
	a.mu.Unlock()
	if a.win != nil {
		a.win.Update(func() {})
	}
}

// prefillActionTranslation fills the sentence card's translation with the
// machine engines' answer, before the model has produced anything.
//
// The grammar action asks the model for a full per-word annotation pass, which
// is thousands of output tokens and 8-13 seconds on a local gateway. Until it
// finishes, the card has nothing to show but raw streaming JSON. The machine
// translation arrives in well under a second and answers the question the user
// actually asked, so it is fetched in parallel and dropped into the same field
// the model will later overwrite with its own translation.
func (a *app) prefillActionTranslation(gen uint64, text, targetLang string) {
	cfg := a.store.Get()
	from := detectLocal(text)
	to := targetLang
	if from == to || sameLang(from, to) {
		to = cfg.SecondLang
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		r := TranslateRace(ctx, text, from, to, cfg.EngineMode)
		if r.Err != nil || strings.TrimSpace(r.Text) == "" {
			return
		}
		a.mu.Lock()
		// A superseded pass must not write at all: its machine prefill would
		// land in the middle of the newer request's card.
		if gen != a.aiGen {
			a.mu.Unlock()
			return
		}
		// Only fill the slot if the model has not already answered it: a fast
		// model can finish before the engines do, and overwriting its
		// translation with the machine one would be a downgrade.
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

// aiResultCacheLimit is how many AI answers stay resident. The popup only ever
// shows one, but a reader flips back to the previous sentence often enough that
// a single slot would miss; a dozen entries is a few hundred KB of text.
const aiResultCacheLimit = 12

// aiResultCacheKey identifies one AI pass: same action, same model, same text,
// same target language. The target language is part of the action's prompt, so
// leaving it out would serve a Chinese card after the user switched to 日本語.
//
// variant is optional and appended only when non-empty. It keeps the four-
// argument form used by older tests and callers working, while runAction passes
// the action/URL/effort fingerprint that changes the answer.
func aiResultCacheKey(actionID, model, targetLang, text string, variant ...string) string {
	key := actionID + "\x00" + model + "\x00" + targetLang + "\x00" + text
	for _, v := range variant {
		if v != "" {
			key += "\x00" + v
		}
	}
	return key
}

// defaultActionByID returns a copy of the shipped action with this ID.
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

// sameActionContent reports whether an action still produces the same kind of
// answer as its shipped default. Name, icon and enabled state do not affect the
// model output, so they are deliberately ignored here.
func sameActionContent(act, def CustomActionConfig) bool {
	return act.ID == def.ID &&
		act.ProviderID == def.ProviderID &&
		act.Model == def.Model &&
		act.SystemPrompt == def.SystemPrompt &&
		act.Prompt == def.Prompt &&
		sameStrings(act.OutputSchema, def.OutputSchema)
}

// actionFingerprint identifies every setting that can change an AI answer but
// is not already part of the cache key: the action's prompt and model, plus the
// gateway URL and reasoning effort. Without it, editing a prompt or switching
// AIURL still served the old cached card.
func actionFingerprint(act *CustomActionConfig, cfg Config) string {
	if act == nil {
		return ""
	}
	h := fnv.New64a()
	fmt.Fprintf(h, "%q\x00%q\x00%q\x00%q\x00%q\x00%q\x00%q",
		act.ID, act.ProviderID, act.Model, act.SystemPrompt, act.Prompt, cfg.AIURL, cfg.AIEffort)
	for _, field := range act.OutputSchema {
		fmt.Fprintf(h, "\x00%q", field)
	}
	return fmt.Sprintf("%x", h.Sum64())
}

// actionCacheVariant returns the extra cache-key part for an action. The
// shipped default action with the default gateway and effort returns "", which
// keeps the old cache keys (and its tests) valid. Any change that can alter the
// model answer returns a fingerprint instead.
func actionCacheVariant(act *CustomActionConfig, cfg Config) string {
	if act == nil {
		return ""
	}
	if def := defaultActionByID(act.ID); def != nil &&
		sameActionContent(*act, *def) &&
		cfg.AIURL == DefaultConfig().AIURL &&
		cfg.AIEffort == DefaultConfig().AIEffort {
		return ""
	}
	return actionFingerprint(act, cfg)
}

// cachedAIResult returns the stored raw answer for key, or "". Caller holds a.mu.
func (a *app) cachedAIResult(key string) string {
	if a.aiResultCache == nil {
		return ""
	}
	return a.aiResultCache[key]
}

// storeAIResult remembers one raw answer, dropping the oldest entry when full.
// Caller holds a.mu.
func (a *app) storeAIResult(key, raw string) {
	if strings.TrimSpace(raw) == "" {
		return
	}
	if a.aiResultCache == nil {
		a.aiResultCache = make(map[string]string, aiResultCacheLimit)
	}
	if _, seen := a.aiResultCache[key]; !seen {
		a.aiResultOrder = append(a.aiResultOrder, key)
	}
	a.aiResultCache[key] = raw
	for len(a.aiResultOrder) > aiResultCacheLimit {
		oldest := a.aiResultOrder[0]
		a.aiResultOrder = a.aiResultOrder[1:]
		delete(a.aiResultCache, oldest)
	}
}

// aiCacheCount reports how many temporary AI answers are resident. The cache is
// deliberately in memory only; a restart starts clean.
func (a *app) aiCacheCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.aiResultCache)
}

// clearAICache drops every temporary AI answer. It is exposed in settings so a
// user who edited a prompt or model can force a fresh request without
// restarting the whole application.
func (a *app) clearAICache() {
	a.mu.Lock()
	a.aiResultCache = nil
	a.aiResultOrder = nil
	a.skipAIResultCache = false
	a.mu.Unlock()
}

// applyParsedAction fills the result field that belongs to actionID and reports
// whether a card was produced. It is the one place that maps a raw answer to a
// card, so the streaming path and the cache-hit path cannot drift apart.
func (a *app) applyParsedAction(actionID, raw string) bool {
	switch actionID {
	case "default-dictionary":
		res := ParseDictionaryResult(raw)
		a.dictResult = res
		return res != nil
	case "default-sentence-analysis":
		parsed := ParseSentenceAnalysis(raw)
		if parsed == nil {
			return false
		}
		// Keep the machine translation when the model's answer has none: a
		// truncated response that stops after the annotations array would
		// otherwise erase a perfectly good translation.
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
	cfg := a.store.Get()
	if !cfg.AIEnabled || strings.TrimSpace(text) == "" {
		return
	}
	act := cfg.GetAction(actionID)
	if act == nil {
		return
	}

	model := act.Model
	if model == "" {
		model = cfg.AIModel
	}
	cacheKey := aiResultCacheKey(actionID, model, cfg.TargetLang, text, actionCacheVariant(act, cfg))

	// Looking at the same text twice must show the same card. Re-running the
	// pass is not just slow (8-13s for a sentence analysis): the model is
	// non-deterministic, so the clause split and the marks along the words
	// moved around between two looks at one sentence, and switching tabs
	// re-ran a pass the user had already waited for. The footer's 重新翻译
	// button sets skipAIResultCache, because there "try again" is the point.
	a.mu.Lock()
	// Starting a pass invalidates the one in flight, cached or not. Cancel
	// alone is not enough: the old goroutine still runs until its request
	// returns, and would otherwise write its stale card over this one.
	a.stopAIPassLocked()
	gen := a.aiGen
	useCache := !a.skipAIResultCache
	a.skipAIResultCache = false
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
		// The cached answer carries the translation it was produced with, so
		// there is no machine prefill to fall back to.
		a.grammarResultPrefill = false
		a.applyParsedAction(actionID, raw)
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
		// Another pass started while we were reading the cache. It owns the
		// shared fields now; starting ours would reset its card.
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
	a.copiedMsg = ""
	// A new pass produces a new entry, so the button stops claiming the
	// previous one is already in the notebook.
	a.savedDictTerm = ""
	// clear previous results for this action
	switch actionID {
	case "default-dictionary":
		a.dictResult = nil
	case "default-sentence-analysis":
		a.grammarResult = nil
	case "default-improve-writing":
		a.writingResult = nil
	}
	a.mu.Unlock()
	if win != nil {
		win.Update(func() {})
	}

	go func() {
		defer cancel()
		targetLangName := langLabels[cfg.TargetLang]
		if targetLangName == "" {
			targetLangName = "简体中文"
		}

		// The translation comes first, and it comes from the machine engines,
		// not the model: 500ms against 8-13s for a full annotation pass. The
		// question a person asks when they select a hard sentence is "what does
		// this mean"; the structure is the second question. Answering them in
		// that order is what makes the popup feel immediate instead of dead
		// while the model works.
		if actionID == "default-sentence-analysis" {
			a.prefillActionTranslation(gen, text, cfg.TargetLang)
		}

		// The selection fills {{selection}}. {{paragraphs}} is left empty on
		// purpose: it used to be given the selection a second time, which
		// doubled the input tokens of every AI action for no information — the
		// model saw the same sentence twice under two different headings, and
		// could not tell which one the surrounding text was. The default
		// templates still name it, so a user who wants to paste real context
		// keeps a placeholder to paste into.
		userPrompt := FormatActionPrompt(act.Prompt, text, "", targetLangName)
		systemPrompt := act.SystemPrompt

		// Streaming used to repaint the entire popup on every chunk, which
		// at a few hundred chunks a second spent more time laying out than
		// generating. Repaint at most every 40 ms; the final Update below
		// still shows the last of the text.
		var lastPaint time.Time
		_, err := StreamMagpie(ctx, cfg.AIURL, model, systemPrompt, userPrompt, ResolveEffort(cfg.AIEffort, model), func(chunk string) {
			a.mu.Lock()
			if gen != a.aiGen {
				// A newer pass reset a.aiText; appending here would splice two
				// answers into one card.
				a.mu.Unlock()
				return
			}
			a.aiText += chunk
			due := time.Since(lastPaint) >= 40*time.Millisecond
			if due {
				lastPaint = time.Now()
			}
			a.mu.Unlock()
			if due && win != nil {
				win.Update(func() {})
			}
		})

		a.mu.Lock()
		if gen != a.aiGen {
			// A newer pass superseded this one while it was streaming. Its
			// context was cancelled, so the error below belongs to us, not to
			// the new card; writing any of it would corrupt that card.
			a.mu.Unlock()
			log.Printf("fastDictionary: dropped stale AI result for %s", actionID)
			return
		}
		a.aiBusy = false
		if err != nil {
			// Keep whatever arrived, but say it is incomplete: a truncated
			// answer used to render as an empty card that looked finished.
			a.errText = friendlyAIError(err)
		}
		if a.aiText != "" {
			// Only remember answers that parsed into a card: caching a
			// truncated or malformed stream would replay the same broken
			// answer on the next look, when a fresh pass might succeed.
			if a.applyParsedAction(actionID, a.aiText) {
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

// refineWithAI replaces the machine translation with an LLM one.
//
// The two are not competitors, they answer different questions. A dedicated
// engine returns in 100-700ms and is right most of the time; an LLM takes
// 1-2s because it generates token by token, but it handles the things machine
// translation still gets wrong — terminology ("mutating" is not 变异), clause
// attachment, register. Measured on this machine, same sentence: Google 780ms,
// Microsoft 718ms, the configured model 1376ms.
//
// So the fast engine stays on the default path, and this runs only when the
// user asks for it. The result replaces the translation in place rather than
// opening a second view: they are two answers to one question, and showing
// both invites comparing them character by character.
func (a *app) refineWithAI() {
	cfg := a.store.Get()
	if !cfg.AIEnabled {
		a.mu.Lock()
		a.errText = "AI 精翻需要先在设置里启用 AI"
		a.mu.Unlock()
		if a.win != nil {
			a.win.Update(func() {})
		}
		return
	}

	a.mu.Lock()
	source, targetLang := a.source, cfg.TargetLang
	isScreenshotMode, ocrPath := a.isScreenshotMode, a.ocrPath
	a.stopAIPassLocked()
	gen := a.aiGen
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	a.aiCancel = cancel
	a.aiRefining = true
	a.aiRefineErr = ""
	win := a.win
	a.mu.Unlock()
	if win != nil {
		win.Update(func() {})
	}

	go func() {
		defer cancel()
		sys := "You are a translator. Translate the text into " + langPromptName(targetLang) +
			". Keep terminology accurate, keep the original's tone and structure, and output only the translation with no explanation."
		started := time.Now()
		effort := ResolveEffort(cfg.AIEffort, cfg.AIModel)
		out, err := StreamMagpie(ctx, cfg.AIURL, cfg.AIModel, sys, source, effort, nil)
		if err != nil && isTransientAIError(err) {
			// A short network hiccup or a 429 should not cost the user the
			// machine translation they already have. One short retry is enough;
			// repeating it would make a genuinely broken gateway feel frozen.
			time.Sleep(350 * time.Millisecond)
			out, err = StreamMagpie(ctx, cfg.AIURL, cfg.AIModel, sys, source, effort, nil)
		}
		if err != nil && isScreenshotMode && ocrPath != "" && ctx.Err() == nil {
			// OCR text is the cheap first attempt. If the model cannot answer
			// from it, give it the original pixels as well: the model can read
			// the screenshot and correct the OCR mistakes that caused the
			// failure. This costs more tokens, so it is deliberately last.
			if dataURL, imageErr := imageFileDataURL(ocrPath); imageErr == nil {
				log.Printf("fastDictionary: text AI refine failed, retrying with screenshot: %v", err)
				out, err = StreamMagpieWithImage(ctx, cfg.AIURL, cfg.AIModel,
					screenshotRefineSystemPrompt(targetLang), screenshotRefineUserPrompt(source),
					dataURL, effort, nil)
			} else {
				log.Printf("fastDictionary: screenshot AI fallback unavailable: %v", imageErr)
			}
		}

		a.mu.Lock()
		if gen != a.aiGen {
			// A newer pass owns the translation now. Writing our (cancelled)
			// answer here would replace its target and label it "AI 精翻".
			a.mu.Unlock()
			log.Printf("fastDictionary: dropped stale AI refinement")
			return
		}
		a.aiRefining = false
		a.aiCancel = nil
		if err != nil {
			a.aiRefineErr = friendlyAIError(err)
		} else if text := strings.TrimSpace(out); text != "" {
			a.target = flattenTranslation(text)
			a.engine = "AI 精翻"
			a.aiRefineErr = ""
			// The caption under the translation pairs the engine with the
			// duration. That duration belonged to the machine pass, so a fresh
			// AI result read "AI 精翻 · 检测 en · 0.3s" — a 0.3s figure measured
			// before the model was called. `detected` is left alone: it
			// describes the source text, which did not change.
			a.elapsed = time.Since(started)
		}
		a.mu.Unlock()
		if win != nil {
			win.Update(func() {})
			a.fitHeight()
		}
	}()
}

// isTransientAIError lists failures where one short retry is worth trying.
// Permanent configuration errors (bad key, unknown model, bad URL) are left
// alone so the user gets the right instruction immediately.
func isTransientAIError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	text := strings.ToLower(err.Error())
	for _, needle := range []string{"timeout", "timed out", "temporary", "connection reset", "connection refused", "eof", "429", "too many requests", "502", "503", "504", "bad gateway", "service unavailable"} {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

// friendlyAIError turns transport/API status strings into the action the user
// can take. The raw error is intentionally not shown as the primary line: it
// is useful in the log, not in a small popup.
func friendlyAIError(err error) string {
	if err == nil {
		return ""
	}
	text := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, context.DeadlineExceeded) || strings.Contains(text, "timeout") || strings.Contains(text, "timed out"):
		return "网络超时，请稍后重试。"
	case strings.Contains(text, "401") || strings.Contains(text, "403") || strings.Contains(text, "api key") || strings.Contains(text, "unauthorized"):
		return "API Key 无效或未配置，请检查 AI 网关设置。"
	case strings.Contains(text, "429") || strings.Contains(text, "too many requests") || strings.Contains(text, "rate limit"):
		return "请求过于频繁（429），请稍后再试。"
	case strings.Contains(text, "404") || strings.Contains(text, "model") && strings.Contains(text, "not found"):
		return "模型不存在或不可用，请检查模型设置。"
	case strings.Contains(text, "connection refused") || strings.Contains(text, "no such host") || strings.Contains(text, "dial tcp"):
		return "无法连接 AI 网关，请检查网关地址是否已启动。"
	default:
		return err.Error()
	}
}

// langPromptName gives the model a language name it will recognise, rather
// than the BCP-47 tag the rest of the app passes around.
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
