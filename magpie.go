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
	"time"
)

const magpieBaseURL = "http://127.0.0.1:3425/v1/chat/completions"

// magpieClient carries a long timeout for streaming responses: the
// caller's context governs cancellation, and a model thinking for two
// minutes about a hard sentence is still streaming within it.
var magpieClient = &http.Client{Timeout: 120 * time.Second}

// Effort levels map to the user's reasoning settings.
const (
	EffortNone = "none"
	EffortLow  = "low"
	EffortHigh = "high"
)

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
func StreamMagpie(ctx context.Context, endpoint, model, system, user, effort string, onDelta func(chunk string)) (string, error) {
	if endpoint == "" {
		endpoint = magpieBaseURL
	}
	if model == "" {
		model = "group/auto-deepseek-v4-1-flash"
	}
	if effort == "" {
		effort = EffortNone
	}

	reqBody := magpieRequest{
		Model:  model,
		Stream: true,
		Messages: []magpieMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
	}

	if effort == EffortNone || effort == "off" {
		reqBody.ReasoningEffort = "none"
	} else {
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

const polishSystemPrompt = "You are a writing teacher. Judge the setting of the Selection first, then mark what a native writer would change for that setting, and give the Selection back corrected.\n" +
	"$1: Setting formality (\"Formal · email to professor\", \"Casual · tweet\")\n" +
	"$2: Annotations ([{\"text\": ..., \"fix\": ..., \"type\": \"grammar/word-choice/unnatural...\", \"note\": ...}])\n" +
	"$3: Corrected version as a native writer would write it\n" +
	"$4: One-sentence summary critique in target language\n" +
	"Output format: {\"setting\": \"...\", \"annotations\": [...], \"improved\": \"...\", \"summary\": \"...\"}\n"

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
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	var out []string
	for _, m := range data.Data {
		if m.ID != "" {
			out = append(out, m.ID)
		}
	}
	sort.Strings(out)
	return out, nil
}
