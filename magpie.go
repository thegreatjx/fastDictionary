// AI refinement via the local magpie gateway (http://127.0.0.1:3425).
// Verified 2026-10-10: group/auto-deepseek-v4-1-flash + reasoning.effort=low
// answers a translation with reasoning_tokens=0 — fast and cheap.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

const magpieBaseURL = "http://127.0.0.1:3425/v1/chat/completions"

// magpieClient carries a long timeout for streaming responses: the
// caller's context governs cancellation, and a model thinking for two
// minutes about a hard sentence is still streaming within it.
var magpieClient = &http.Client{Timeout: 120 * time.Second}

// Effort is a preference, not a wire value: the wire value depends on the
// model. There is no "no reasoning" level to ask for — the gateway validates
// reasoning_effort against each model's own reasoning_levels and silently drops
// anything it does not list, after which the model applies its default.
//
// Asking for each model's lowest declared level is not the same as making it
// fast, because a declared level switches reasoning ON for a model that would
// otherwise not think at all. Measured here on the same sentence, with the
// top-level reasoning_effort field being the one the gateway honours (the
// nested reasoning.effort is ignored by this gateway):
//
//	deepseek-v4.1-flash, no field 1.2s / 0 reasoning tokens / 24 annotations
//	deepseek-v4.1-flash, low      38s  / 5000-7000 tokens  / 55 annotations
//	gemini-3.8-flash,    no field 10.5s / 743 reasoning tokens
//	gemini-3.8-flash,    low      2.9s / 0 reasoning tokens
//
// The two families point opposite ways, so no single choice serves both: what
// makes deepseek fast makes gemini slow, and the reverse. EffortFast therefore
// sends the value measured to be fast for the model's family (empty = send
// nothing), and EffortDeep asks for high everywhere.
const (
	EffortFast = "fast"
	EffortDeep = "deep"
)

// fastEfforts records, per model family, the wire value that actually makes
// that family answer quickly. An empty level means "send no reasoning field".
//
// This table is measured, not derived: it cannot be computed from
// reasoning_levels, because the levels say what a model can be asked for, not
// whether asking switches thinking on. A family missing from this table falls
// back to its lowest declared level, which is right for models that think by
// default — the only kind where the fallback has an effect that is safe.
var fastEfforts = []struct {
	match string
	level string
}{
	{"gemini", "low"}, // thinks by default: no field 10.5s, low 2.9s
	{"deepseek", ""},  // idle by default: no field 1.2s, low 38s
}

// fastEffortFor returns the measured fast value for a model. The second result
// reports whether the family was known; an unknown family must not be guessed
// at, because sending a level can only ever turn reasoning on.
func fastEffortFor(model string) (string, bool) {
	name := strings.ToLower(model)
	for _, f := range fastEfforts {
		if strings.Contains(name, f.match) {
			return f.level, true
		}
	}
	return "", false
}

// effortRank orders the levels the gateway reports so the lowest one can be
// picked without trusting the order the API happens to return.
var effortRank = map[string]int{
	"minimal": 0, "low": 1, "medium": 2, "high": 3, "xhigh": 4, "max": 5,
}

// modelEfforts caches each model's declared reasoning_levels, filled by
// FetchAvailableModels. ResolveEffort needs it to answer "what is this model's
// lowest level", which differs per model: deepseek lists low/high/max, gemini
// lists low/medium/high, and one gateway model lists only high/max.
var modelEfforts = struct {
	sync.RWMutex
	m map[string][]string
}{m: map[string][]string{}}

// lowestReasoningLevel returns the cheapest level the model declares.
func lowestReasoningLevel(model string) (string, bool) {
	modelEfforts.RLock()
	levels := modelEfforts.m[model]
	modelEfforts.RUnlock()
	best, found := "", false
	for _, l := range levels {
		rank, known := effortRank[strings.ToLower(l)]
		if !known {
			continue
		}
		if !found || rank < effortRank[best] {
			best, found = strings.ToLower(l), true
		}
	}
	return best, found
}

// ResolveEffort turns the user's preference into the value to send for this
// model. An empty result means "send no reasoning field", which is not a
// neutral value: it is what lets a model that is idle by default stay idle.
//
// 快速 uses the measured table first and only falls back to the lowest declared
// level for a family nobody has measured, where a level is likely to help
// rather than hurt. 深度思考 asks for high regardless — that is what the user
// picked it for, and it is the setting under which a slow answer is expected.
func ResolveEffort(pref, model string) string {
	if pref == EffortDeep {
		return "high"
	}
	if level, ok := fastEffortFor(model); ok {
		return level
	}
	if level, ok := lowestReasoningLevel(model); ok {
		return level
	}
	return ""
}

type magpieMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type magpieRequest struct {
	Model           string          `json:"model"`
	Messages        []magpieMessage `json:"messages"`
	Stream          bool            `json:"stream"`
	MaxTokens       int             `json:"max_tokens,omitempty"`
	ReasoningEffort string          `json:"reasoning_effort,omitempty"`
	Reasoning       *struct {
		Effort string `json:"effort"`
	} `json:"reasoning,omitempty"`
}

// StreamMagpie sends a chat request and calls onDelta for each streamed
// content chunk. It returns the full accumulated text or an error.
//
// effort is a wire value, not a preference: callers turn Config.AIEffort into
// one with ResolveEffort, which knows what this model actually accepts. An
// empty effort sends no reasoning field at all.
func StreamMagpie(ctx context.Context, endpoint, model, system, user, effort string, onDelta func(chunk string)) (string, error) {
	if endpoint == "" {
		endpoint = magpieBaseURL
	}
	if model == "" {
		model = "group/auto-deepseek-v4-1-flash"
	}

	reqBody := magpieRequest{
		Model:  model,
		Stream: true,
		Messages: []magpieMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
	}

	// Sending a level the model does not list is worse than sending nothing:
	// the gateway drops it and the model applies its own default, which may be
	// far more expensive than the level that was asked for.
	if effort != "" && effort != "off" && effort != "none" {
		reqBody.ReasoningEffort = effort
		reqBody.Reasoning = &struct {
			Effort string `json:"effort"`
		}{Effort: effort}
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := magpieClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("magpie: %w (网关没开？需要 magpie 在 127.0.0.1:3425)", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("magpie: http %d: %s", resp.StatusCode, truncateFor(string(b), 120))
	}

	var sb strings.Builder
	sawDone := false
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			sawDone = true
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			c := chunk.Choices[0].Delta.Content
			sb.WriteString(c)
			if onDelta != nil {
				onDelta(c)
			}
		}
	}
	if err := sc.Err(); err != nil {
		if ctx.Err() != nil {
			return sb.String(), ctx.Err()
		}
		return sb.String(), err
	}
	// The stream is only complete when the gateway sent the terminator. A
	// connection that drops half way leaves a truncated JSON object that
	// parses into an empty card, which the UI then showed as "done"; say so
	// instead, and keep the partial text so the caller can still show it.
	if !sawDone && ctx.Err() == nil {
		partial := strings.TrimSpace(sb.String())
		if partial == "" {
			return "", fmt.Errorf("magpie: stream ended before any content")
		}
		return partial, fmt.Errorf("magpie: stream ended early (incomplete answer)")
	}
	out := strings.TrimSpace(sb.String())
	if out == "" {
		return "", fmt.Errorf("magpie: empty stream")
	}
	return out, nil
}

// refineSystemPrompt is the translation-refinement persona (mirrors pot's
// gemini prompt: translate only, never interpret).
const refineSystemPrompt = "You are a professional translation engine. Translate the user's text into the other language (Chinese source -> elegant English, otherwise -> natural colloquial Chinese). Output ONLY the translation, never explanations."

// Read-Frog 1:1 System Prompts for Built-in Actions
const grammarSystemPrompt = `You are a grammar tutor. Mark up the Selection (a sentence in any language) so a learner sees its structure at a glance: the core of the main clause, every clause and longer phrase, and whatever makes the sentence hard to read.

## Output Format
Output a valid JSON object with:
- "annotations": a JSON array of annotation items
- "translation": a natural translation of the whole Selection in {{targetLanguage}}
Do not wrap in markdown fences or add explanatory text.

## Annotations Specification
Each annotation quotes one span of the Selection and says what it is, with these keys only:
- "text": the span, copied character for character from the Selection: the same words in the same order, with nothing left out, added or replaced (no "…"), and no spaces at either end.
- "type": the span's role in its clause, exactly one of: subject, predicate, object, complement, attributive, adverbial, appositive, connector.
- "form": only for a clause or a non-finite phrase, exactly one of: clause, infinitive, gerund, present-participle, past-participle.
- "sense": only for an adverbial clause, or an adverbial participle or infinitive phrase, exactly one of: time, place, cause, condition, concession, purpose, result, manner, comparison.
- "head": only for an attributive, an appositive, or a clause that is an object or complement: the one word it modifies or completes, copied from the Selection.
- "obstacle": only where a learner is likely to stumble, exactly one of: inversion, fronting, ellipsis, passive, subjunctive, parenthesis, dummy-it, postponed-subject, cleft, comparison, negation, split, idiom.
- "restore": only with fronting, inversion or ellipsis: the clause in plain word order, with any missing words put back.
- "note": at most 10 words in {{targetLanguage}}, only when it adds what the keys above cannot say.
- "occurrence": only when the same text appears more than once in the Selection: 1 for the first appearance, 2 for the second, and so on.
Leave out every optional key that does not apply; never add a key of your own.

## What to annotate
1. The main clause: its subject, predicate, object or complement, and adverbials.
2. Every subordinate clause: one annotation for the whole clause, with "form": "clause" and the type of the role it plays (e.g. relative clause is attributive; clause after verb is object). Then its connector, its predicate, and internal elements.
3. Participle, infinitive and gerund phrases.
4. Hard structures:
   - Fronting & inversion: fronted part has "obstacle": "fronting" and "restore"; verb has "obstacle": "inversion".
   - Ellipsis: missing words restored in "restore".
   - Dummy "it": "It" is subject with "obstacle": "dummy-it"; real clause has "obstacle": "postponed-subject".
   - Parenthesis: "obstacle": "parenthesis".
   - Passive: "obstacle": "passive".

## Examples
Selection: The report that the committee published last week, which surprised many readers, was never discussed in public.
{"annotations":[{"text":"The report that the committee published last week, which surprised many readers","type":"subject"},{"text":"that the committee published last week","type":"attributive","form":"clause","head":"report"},{"text":"that","type":"connector"},{"text":"the committee","type":"subject"},{"text":"published","type":"predicate"},{"text":"last week","type":"adverbial"},{"text":"which surprised many readers","type":"attributive","form":"clause","head":"report","obstacle":"parenthesis"},{"text":"which","type":"connector"},{"text":"surprised","type":"predicate"},{"text":"many readers","type":"object"},{"text":"was never discussed","type":"predicate","obstacle":"passive"},{"text":"in public","type":"adverbial"}],"translation":"委员会上周发布的报告令许多读者感到惊讶，但该报告从未被公开讨论过。"}

Selection: Exhausted by the long journey, the travelers decided to rest as soon as they arrived.
{"annotations":[{"text":"Exhausted by the long journey","type":"adverbial","form":"past-participle","sense":"cause"},{"text":"the travelers","type":"subject"},{"text":"decided","type":"predicate"},{"text":"to rest","type":"object","form":"infinitive"},{"text":"as soon as they arrived","type":"adverbial","form":"clause","sense":"time"},{"text":"as soon as","type":"connector"},{"text":"arrived","type":"predicate"}],"translation":"由于长途跋涉疲惫不堪，旅客们决定一到达就休息。"}

Selection: Only then did she turn the offer down, fearing it was a trap.
{"annotations":[{"text":"Only then","type":"adverbial","obstacle":"fronting","restore":"she turned the offer down only then"},{"text":"did","type":"predicate","obstacle":"inversion"},{"text":"she","type":"subject"},{"text":"turn","type":"predicate","obstacle":"split"},{"text":"the offer","type":"object"},{"text":"down","type":"predicate","obstacle":"split"},{"text":"fearing it was a trap","type":"adverbial","form":"present-participle","sense":"cause"},{"text":"it was a trap","type":"object","form":"clause","head":"fearing","obstacle":"ellipsis","restore":"that it was a trap"},{"text":"was","type":"predicate"},{"text":"a trap","type":"complement"}],"translation":"直到那时她才拒绝了这个提议，担心这是一个陷阱。"}`

const polishSystemPrompt = `You are a writing teacher. Judge the setting of the Selection first, then mark what a native writer would change for that setting, and give the Selection back corrected.

Write the setting, every "tag" and "note", and the summary in {{targetLanguage}}, even when the Selection is in another language; write every "fix" and the "improved" text in the Selection's own language.

## setting
How formal the setting is and what the text is, in {{targetLanguage}}: "<formality> · <what it is>", at most 16 characters, where the formality is formal, neutral or casual (e.g. "Formal · email to a professor", "Casual · tweet"). Judge it from the Page title, the Page URL, the Paragraphs and the Selection itself. Everything below is judged against this setting: what is natural in a group chat is not natural in a weekly report, and the other way round.

## annotations
Each annotation quotes one span of the Selection, with these keys only:
- "text": the span, copied character for character from the Selection: the same words in the same order, with nothing left out, added or replaced (no "…"), and no spaces at either end. When the change touches one short word, or adds a missing one, quote a neighboring word too (quote "to beach" to add "the"), so the span can be found.
- "fix": the span as it should read, in the Selection's language; "" deletes it. Leave it out only for "good".
- "type": exactly one bare word from this list, never a phrase and never a sentence:
  spelling, grammar, punctuation, word-choice, logic, unnatural, register, clarity, good.
  The first five are wrong in any setting. "unnatural" (a literal translation, a stiff or
  wordy phrase), "register" (too casual or too formal, impolite, wrong honorifics) and
  "clarity" (ambiguous or confusing) mean it is not what a native writer would write in
  this setting. "good" marks a choice worth keeping.
- "tag": optional, at most 6 characters in {{targetLanguage}}: the short name of the rule (a tense, an article, a particle, honorifics, too formal).
- "note": at most 20 words in {{targetLanguage}}. For an error, the rule; otherwise, what a native writer would write here and why.
- "occurrence": only when the same text appears more than once in the Selection: 1 for the first appearance, 2 for the second, and so on.
Leave out every optional key that does not apply; never add a key of your own.

## What to mark
1. Everything wrong, then what a native writer would not write in this setting. Nothing else: never an upgrade to fancier words or more complex sentences, and never a change of formality the setting does not call for.
2. At most 8 annotations. When several problems touch the same words, merge them into one annotation, typed by the most serious. Annotations never overlap; list them in the order they appear.
3. At most one "good", for a choice genuinely worth keeping.
4. Never mark names, brands, code, URLs, @mentions, #tags or emoji; slang, abbreviations or mixed languages the setting allows; accepted expressions ("Long time no see"); the language's own conventions (a space before "!" in French).
5. When nothing needs changing, "annotations" is "[]".

## improved
The whole Selection as a native writer would write it in this setting, in the Selection's language, keeping the writer's meaning and voice. Apply every fix, and change nothing the setting does not call for. Identical to the Selection when nothing needs changing. Keep the writer's capitalisation habits: do not lower a sentence's first letter just because the Selection started mid-sentence, and do not add a capital the Selection did not have.

## summary
One sentence in {{targetLanguage}}: what works, and the one thing to work on.

## Format
The "annotations" value is a STRING holding a compact JSON array; escape its inner double quotes as \". Do not wrap it in markdown.

## Examples
Selection: I very like this movie, it make me feel very touching.
Page title: Official Trailer - YouTube
Target language: Simplified Chinese
setting: 随意 · 视频评论
annotations: "[{\"text\":\"I very like\",\"fix\":\"I really like\",\"type\":\"grammar\",\"tag\":\"副词位置\",\"note\":\"very 不能直接修饰动词，用 really like\"},{\"text\":\"movie, it\",\"fix\":\"movie. It\",\"type\":\"punctuation\",\"tag\":\"逗号连句\",\"note\":\"两个完整的句子不能只用逗号连接\"},{\"text\":\"make me feel very touching\",\"fix\":\"was so moving\",\"type\":\"grammar\",\"tag\":\"-ing/-ed\",\"note\":\"touching 指令人感动的事物，人被感动用 moved；it 后要用 makes\"}]"
improved: I really like this movie. It was so moving.
summary: 意思清楚，问题在中文句式直译：very 修饰动词、逗号连句。

Selection: pls check the attached file and give me ur feedback asap. thx!
Paragraphs: Dear Professor Smith, pls check the attached file and give me ur feedback asap. thx!
Page title: Draft - Gmail
Target language: English
setting: Formal · email to a professor
annotations: "[{\"text\":\"pls check\",\"fix\":\"Could you please take a look at\",\"type\":\"register\",\"tag\":\"tone\",\"note\":\"Sounds like an order to a professor; pls is chat shorthand\"},{\"text\":\"ur\",\"fix\":\"your\",\"type\":\"spelling\",\"tag\":\"abbrev\",\"note\":\"Chat shorthand has no place in an email\"},{\"text\":\"asap\",\"fix\":\"when you have a chance\",\"type\":\"register\",\"tag\":\"tone\",\"note\":\"asap pressures the reader\"},{\"text\":\"thx!\",\"fix\":\"Thank you!\",\"type\":\"spelling\",\"tag\":\"abbrev\",\"note\":\"Spell it out in an email\"}]"
improved: Could you please take a look at the attached file and share your feedback when you have a chance? Thank you!
summary: The request is clear, but the shorthand and the orders sound rude to a professor.
Selection: pls check the attached file and give me ur feedback asap. thx!
Paragraphs: hey, report's done. pls check the attached file and give me ur feedback asap. thx!
Page title: Tom (DM) - Slack
Target language: English
setting: Casual · message to a teammate
annotations: "[]"
improved: pls check the attached file and give me ur feedback asap. thx!
summary: Natural for a quick message to a teammate: nothing to change.

Selection: 部長がおっしゃられたように、会議は延期するになりました。
Page title: 受信トレイ - Outlook
Target language: Simplified Chinese
setting: 正式 · 写给上级的邮件
annotations: "[{\"text\":\"おっしゃられた\",\"fix\":\"おっしゃった\",\"type\":\"register\",\"tag\":\"二重敬语\",\"note\":\"おっしゃる已是尊敬语，再加られる成了二重敬语\"},{\"text\":\"延期するになりました\",\"fix\":\"延期することになりました\",\"type\":\"grammar\",\"tag\":\"ことになる\",\"note\":\"动词接になる要先名词化：〜することになる\"}]"
improved: 部長がおっしゃったように、会議は延期することになりました。
summary: 语法只错一处，另有一处二重敬语。

Selection: 这个功能很好用，我觉得没问题，可以上线了。
Paragraphs: 这个功能很好用，我觉得没问题，可以上线了。
Page title: 产品评审 - 飞书
Target language: 简体中文
setting: 中性 · 工作群消息
annotations: "[{\"text\":\"很好用\",\"fix\":\"用起来很顺\",\"type\":\"unnatural\",\"tag\":\"口语化\",\"note\":\"很好用略显笼统，用起来很顺更具体\"}]"
improved: 这个功能用起来很顺，我觉得没问题，可以上线了。
summary: 意思明确，把笼统的评价换成具体感受会更可信。

## Output
Return exactly one JSON object and nothing else, with these keys in this order: "setting", "annotations", "improved", "summary". All four are strings. Do not add any other key. Do not wrap the JSON in markdown.`

const learnerDictSystemPrompt = "你是一个面向语言学习者的词典助手。\n" +
	"根据给定的词语及其周围段落，生成简洁的词典条目，匹配所需的输出对象。\n" +
	"规则：\n" +
	"1. 聚焦于最匹配所提供段落内容的含义。\n" +
	"2. 将词条规范化为其基本/标准形式。\n" +
	"3. 保持释义精确且适合学习者。\n" +
	"4. “句子”字段只提取所提供段落中包含选中词语的完整句子，不要包含前后句。\n" +
	"5. “句子翻译”字段只翻译所提取的句子。\n" +
	"6. 音标必须使用该语言的标准标注（如英语用 IPA，中文用拼音，日语用罗马字）。\n" +
	"7. 词性使用英文（noun, verb, adjective 等）。\n" +
	"8. 难度必须是 CEFR 等级（A1, A2, B1, B2, C1, 或 C2）。\n" +
	"9. 如果某个字段未知，返回空字符串而非猜测。\n" +
	"10. 除非需要保留原文形式，所有文本字段均使用 {{targetLanguage}} 作答。\n" +
	"11. “句中词形”字段标出选中词语在“句子”中的位置：紧凑的 JSON 数组 [{\"text\": ...}]。\n" +
	"12. “记忆技巧”针对词条特点教学习者记住它（最多 3 条，每条一行）：词根词缀、拆字、词源、近反义词、易混词、搭配。严禁谐音或硬凑。\n" +
	"严格以紧凑的 JSON 对象格式输出，包含以下字段：\n" +
	"{\"term\": \"...\", \"phonetic\": \"...\", \"partOfSpeech\": \"...\", \"definition\": \"...\", \"context\": \"...\", \"contextTerm\": [{\"text\": \"...\"}], \"contextTranslation\": \"...\", \"memoryTips\": \"...\", \"difficulty\": \"...\"}\n"

// FetchAvailableModels queries the local magpie gateway for the list of available model IDs.
func FetchAvailableModels(endpoint string) ([]string, error) {
	if endpoint == "" {
		endpoint = "http://127.0.0.1:3425/v1/models"
	} else if strings.Contains(endpoint, "/chat/completions") {
		endpoint = strings.Replace(endpoint, "/chat/completions", "/models", 1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := magpieClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data struct {
		Data []struct {
			ID             string   `json:"id"`
			ReasoningLevel []string `json:"reasoning_levels"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	var out []string
	levels := make(map[string][]string, len(data.Data))
	for _, m := range data.Data {
		if m.ID == "" {
			continue
		}
		out = append(out, m.ID)
		if len(m.ReasoningLevel) > 0 {
			levels[m.ID] = m.ReasoningLevel
		}
	}
	// Remember what each model accepts: ResolveEffort turns 快速 into the
	// model's own cheapest level, and that list only exists here.
	modelEfforts.Lock()
	modelEfforts.m = levels
	modelEfforts.Unlock()
	sort.Strings(out)
	return out, nil
}
