package main

import (
	"context"
	"encoding/json"
	"fmt"
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
	a.translateText(CleanOcrText(text))
}

// translateText is the body of translate, with the caller's cleaning already
// applied.
func (a *app) translateText(text string) {
	cfg := a.store.Get()
	win := a.win

	a.mu.Lock()
	if a.aiCancel != nil {
		a.aiCancel()
		a.aiCancel = nil
	}
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
			a.target = r.Text
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
		// If it's a sentence, trigger Read-Frog "猜你想存" extraction in background
		if cfg.ShowNotes && !word && len(text) > 20 {
			a.extractNotes(text)
		}
	}()
}

// extractNotes runs the opt-in "猜你想存" note extraction. Hiding the
// recommendation must also stop its otherwise invisible AI request.
func (a *app) extractNotes(text string) {
	cfg := a.store.Get()
	if !cfg.AIEnabled || !cfg.ShowNotes || strings.TrimSpace(text) == "" {
		return
	}

	a.mu.Lock()
	a.notesBusy = true
	a.notes = nil
	a.mu.Unlock()
	if a.win != nil {
		a.win.Update(func() {})
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		systemPrompt := "你是一个面向语言学习者的专业词典与背单词助手。\n" +
			"请从用户选中的英文文本或句子中，分析并挑选出 1 到 2 个最值得学习者记忆的核心生词。\n" +
			"必须为每个生词输出完整的词典数据（包含标准IPA音标、词性、精准释义、CEFR难度等级如A2/B1/B2/C1、简练词源记忆法）。\n" +
			"严格以紧凑的 JSON 数组格式输出，不要有任何 Markdown 标记或多余文字。\n" +
			"示例格式：[{\"word\":\"ambitious\",\"phonetic\":\"/æmˈbɪʃ.əs/\",\"pos\":\"adj.\",\"meaning\":\"有雄心的，抱负不凡的；宏大的\",\"difficulty\":\"B2\",\"memory_tips\":\"词源：ambi-（周围、环顾）+ -it-（走）+ -ious（形容词）→ 四处奔走谋求发展 → 雄心勃勃的\"}]"

		raw, err := StreamMagpie(ctx, cfg.AIURL, cfg.AIModel, systemPrompt, text, ResolveEffort(cfg.AIEffort, cfg.AIModel), nil)
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

			var items []NoteItem
			if err := json.Unmarshal([]byte(cleaned), &items); err == nil && len(items) > 0 {
				for i := range items {
					items[i].Checked = true
				}
				a.mu.Lock()
				a.notes = items
				a.mu.Unlock()
			}
		}

		a.mu.Lock()
		a.notesBusy = false
		a.mu.Unlock()
		if a.win != nil {
			a.win.Update(func() {})
		}
	}()
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
func (a *app) prefillActionTranslation(text, targetLang string) {
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
		// Only fill the slot if the model has not already answered it: a fast
		// model can finish before the engines do, and overwriting its
		// translation with the machine one would be a downgrade.
		if a.grammarResult == nil {
			a.grammarResult = &SentenceAnalysisResult{}
		}
		if a.grammarResult.Translation == "" {
			a.grammarResult.Translation = r.Text
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
func aiResultCacheKey(actionID, model, targetLang, text string) string {
	return actionID + "\x00" + model + "\x00" + targetLang + "\x00" + text
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
	cacheKey := aiResultCacheKey(actionID, model, cfg.TargetLang, text)

	// Looking at the same text twice must show the same card. Re-running the
	// pass is not just slow (8-13s for a sentence analysis): the model is
	// non-deterministic, so the clause split and the marks along the words
	// moved around between two looks at one sentence, and switching tabs
	// re-ran a pass the user had already waited for. The footer's 重新翻译
	// button sets skipAIResultCache, because there "try again" is the point.
	a.mu.Lock()
	useCache := !a.skipAIResultCache
	a.skipAIResultCache = false
	raw := ""
	if useCache {
		raw = a.cachedAIResult(cacheKey)
	}
	a.mu.Unlock()

	if raw != "" {
		a.mu.Lock()
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
	if a.aiCancel != nil {
		a.aiCancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	a.aiCancel = cancel
	a.activeActionID = actionID
	a.aiText = ""
	a.aiBusy = true
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
			a.prefillActionTranslation(text, cfg.TargetLang)
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
		a.aiBusy = false
		if err != nil {
			// Keep whatever arrived, but say it is incomplete: a truncated
			// answer used to render as an empty card that looked finished.
			a.errText = err.Error()
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
	if a.aiCancel != nil {
		a.aiCancel()
	}
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
		sys := "You are a translator. Translate the text into " + langPromptName(targetLang) +
			". Keep terminology accurate, keep the original's tone and structure, and output only the translation with no explanation."
		started := time.Now()
		out, err := StreamMagpie(ctx, cfg.AIURL, cfg.AIModel, sys, source, ResolveEffort(cfg.AIEffort, cfg.AIModel), nil)

		a.mu.Lock()
		a.aiRefining = false
		a.aiCancel = nil
		if err != nil {
			a.aiRefineErr = err.Error()
		} else if text := strings.TrimSpace(out); text != "" {
			a.target = text
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
