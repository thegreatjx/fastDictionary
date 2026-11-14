package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
)

// httpClient serves the race. Eight seconds is generous on purpose: the
// race cancels the loser, so a slow engine costs nothing while the other one
// is answering.
var httpClient = &http.Client{Timeout: 8 * time.Second}

// httpClientPinned serves the "only Google" / "only Microsoft" modes. There
// is no second engine to cover a hung request, and Google regularly needs
// more than eight seconds on a throttled route, so waiting that long only
// stalls the popup; fail fast and let the user switch back to the race.
var httpClientPinned = &http.Client{Timeout: 4 * time.Second}

// userAgent identifies the translator to the engines' edge endpoints. Both
// advertise themselves as a browser to something that expects a page request.
const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

// httpClientPinned serves the "only Google" / "only Microsoft" modes. There
// is no second engine to cover a hung request, and Google regularly needs
// ---------------------------------------------------------------------------
// Local language detection: zero network cost.
// ---------------------------------------------------------------------------

// detectLocal guesses the source language from its script.
//
// It answers two questions: is this already the user's target language (so
// translate out rather than in), and which source language to tell the engine.
// It used to answer "en" for everything that was not Chinese, Japanese or
// Korean, which sent Russian, Arabic and Thai to the engine labelled as English
// — the engine then translated from the wrong language and returned nonsense or
// a refusal. The ranges below cover the languages people actually read;
// anything else returns "" and the engine detects it itself, which is what
// "auto" means.
func detectLocal(text string) string {
	var han, kana, hangul, cyrillic, arabic, hebrew, thai, devanagari, greek int
	var latin int
	for _, r := range text {
		switch {
		case unicode.Is(unicode.Han, r):
			han++
		case unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r):
			kana++
		case unicode.Is(unicode.Hangul, r):
			hangul++
		case unicode.Is(unicode.Cyrillic, r):
			cyrillic++
		case unicode.Is(unicode.Arabic, r):
			arabic++
		case unicode.Is(unicode.Hebrew, r):
			hebrew++
		case unicode.Is(unicode.Thai, r):
			thai++
		case unicode.Is(unicode.Devanagari, r):
			devanagari++
		case unicode.Is(unicode.Greek, r):
			greek++
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			latin++
		case unicode.Is(unicode.Latin, r):
			// A Latin letter beyond ASCII: é, ñ, ü, ł, ø, ß. Its presence is
			// what tells English apart from the rest of the Latin-script
			// languages.
			latin++
		}
	}
	switch {
	case kana > 0 && kana >= hangul:
		return "ja"
	case hangul > 0:
		return "ko"
	case han > latin:
		return "zh-Hans"
	case thai > 0:
		return "th"
	case hebrew > 0:
		return "he"
	case arabic > 0:
		return "ar"
	case devanagari > 0:
		return "hi"
	case greek > 0:
		return "el"
	case cyrillic > 0:
		// Cyrillic covers Russian, Ukrainian and Bulgarian. The letters cannot
		// tell them apart, and the engines detect the difference themselves,
		// so the answer is the umbrella the API understands.
		return "ru"
	default:
		// Latin script that is not plain ASCII-pure (French, German, Polish),
		// or a script not listed above. The engine detects these itself, and
		// guessing "en" here is what made a French sentence get read by an
		// English voice and translated from the wrong source language.
		return ""
	}
}

// loneLatinWord reports text that is one word of ASCII letters: no spaces,
// digits, punctuation or accented letters. The engines get no second word to
// detect a language from.
func loneLatinWord(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	for _, r := range text {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return true
}

// sourceLangFor returns the source language to hand the engines.
//
// detectLocal settles the script and leaves plain ASCII undecided, because a
// French or Dutch sentence is ASCII too. A lone word has no other word to
// settle it with, and the engines get it wrong often: measured 2026-11-13
// against 微软, 8 of 25 common English words came back translated as another
// language — "fast" (de) 几乎, "war" (de) 是, "sin" (es) 然而, "hat" (de) 具有,
// "tag" (de) 一天. Looking such a word up is what this app is for, so a single
// ASCII word is sent as English.
//
// The cost is the reverse case: a lone German or French word is now read as
// English ("nicht" comes back untranslated). Sentences keep the old behaviour
// — they are long enough for the engines to detect, and guessing "en" for
// ASCII is what once had a French sentence read by an English voice.
func sourceLangFor(text string) string {
	if tag := detectLocal(text); tag != "" {
		return tag
	}
	if loneLatinWord(text) {
		return "en"
	}
	return ""
}

// translateLangs picks the source and target language for a translation.
//
// 目标语言等于原文语言时（中文用户把目标设成简体中文，又输入中文），机器翻译会
// 改译到第二语言，否则卡片上“译文”就是原文本身。这条规则有三个消费者——机器
// 翻译、长难句的机翻预填、AI 精翻——以前各写一份，精翻那一份漏了回退，于是点
// 「AI 精翻」会把中文再译回中文，看着像“把我的译文又翻了回去”。
func translateLangs(cfg Config, text string) (from, to string) {
	from = sourceLangFor(text)
	to = cfg.TargetLang
	if to == "" {
		to = "zh-Hans"
	}
	if from == to || sameLang(from, to) {
		to = cfg.SecondLang
		// 第二语言为空或与原文语言相同时，至少给一个能出结果的方向。
		if to == "" || sameLang(from, to) {
			if langPrefix(from) == "zh" {
				to = "en"
			} else {
				to = "zh-Hans"
			}
		}
	}
	return from, to
}

// ---------------------------------------------------------------------------
// Microsoft: unauthenticated translatetext endpoint (successor of the
// api-edge.cognitive.microsofttranslator.com flow whose token endpoint
// edge.microsoft.com/translate/auth was removed upstream in 2026-07).
// Body is a bare JSON string array; from="" means auto-detect and the
// response carries detectedLanguage. Verified 2026-10-10: 3/3 ok, ~520ms.
// ---------------------------------------------------------------------------

type msResult struct {
	Text     string
	Detected string
}

func MicrosoftTranslate(ctx context.Context, text, from, to string) (msResult, error) {
	if from == "auto" {
		from = ""
	}
	if strings.HasPrefix(from, "zh") {
		from = "zh-Hans"
	}
	body, _ := json.Marshal([]string{text})
	endpoint := fmt.Sprintf("https://edge.microsoft.com/translate/translatetext?from=%s&to=%s&isEnterpriseClient=false",
		url.QueryEscape(from), url.QueryEscape(to))
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return msResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return msResult{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return msResult{}, err
	}
	if resp.StatusCode != 200 {
		return msResult{}, fmt.Errorf("microsoft: http %d", resp.StatusCode)
	}
	var out []struct {
		DetectedLanguage struct {
			Language string  `json:"language"`
			Score    float64 `json:"score"`
		} `json:"detectedLanguage"`
		Translations []struct {
			Text string `json:"text"`
		} `json:"translations"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return msResult{}, fmt.Errorf("microsoft: bad json: %w", err)
	}
	if len(out) == 0 || len(out[0].Translations) == 0 {
		return msResult{}, fmt.Errorf("microsoft: empty result")
	}
	return msResult{Text: out[0].Translations[0].Text, Detected: out[0].DetectedLanguage.Language}, nil
}

// ---------------------------------------------------------------------------
// Google: free web endpoint on translate.googleapis.com (the API host is
// steadier than translate.google.com, which intermittently 429s on shared
// proxy nodes). dt=bd adds dictionary data for single words.
// ---------------------------------------------------------------------------

// googleClients are tried in order, one per attempt. "gtx" is what most
// scrapers send and it is the first thing Google throttles: measured on this
// machine 2026-10-26, gtx answered 429 on every request for hours while
// dict-chrome-ex — the client id the Chrome dictionary extension sends — kept
// answering 200 with the same JSON shape. Both are the same endpoint and the
// same payload, so the only difference is which rate-limit bucket the request
// lands in.
var googleClients = []string{"dict-chrome-ex", "gtx"}

// DictEntry is one part-of-speech group of a dictionary-style result.
type DictEntry struct {
	POS     string
	Explain string
}

// DictResult is a word-level result: pronunciation and grouped meanings.
type DictResult struct {
	Pron    string
	Entries []DictEntry
}

func GoogleTranslate(ctx context.Context, text, from, to string) (string, string, error) {
	return googleTranslate(ctx, text, from, to, false)
}

// googleSingleURL is the free web endpoint. It is a variable so tests can
// serve a canned reply and assert on the query string without a network call.
const googleSingleDefaultURL = "https://translate.googleapis.com/translate_a/single"

var googleSingleURL = googleSingleDefaultURL

// googleTranslate does the work; pinned selects the fail-fast client: a
// pinned engine has no second engine to cover a hung request, so it uses
// the shorter timeout rather than making the popup wait eight seconds on a
// route Google is throttling. It used to be a global atomic set by
// TranslateRace, which made concurrent translations interfere.
func googleTranslate(ctx context.Context, text, from, to string, pinned bool) (string, string, error) {
	q := url.Values{}
	// Google rejects an empty sl with HTTP 400 and an HTML error page: the
	// parameter has to say "auto" explicitly. detectLocal returns "" for text
	// whose script it cannot pin down, which is every plain-ASCII English
	// sentence, so sending it verbatim broke Google for the most common input
	// there is. Verified 2026-10-26: sl= → 400, sl=auto → 200.
	if from == "" {
		from = "auto"
	}
	q.Set("sl", from)
	q.Set("tl", to)
	q.Set("hl", to)
	q.Set("ie", "UTF-8")
	q.Set("oe", "UTF-8")
	q.Set("dt", "t")
	q.Set("q", text)

	var lastErr error
	for attempt := 0; attempt < len(googleClients); attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", "", ctx.Err()
			case <-time.After(800 * time.Millisecond):
			}
		}
		// Retry with the next client id rather than the same one: a 429 means
		// this bucket is exhausted, and repeating the request only spends
		// another second on a route already known to be closed.
		q.Set("client", googleClients[attempt])
		req, err := http.NewRequestWithContext(ctx, "GET", googleSingleURL+"?"+q.Encode(), nil)
		if err != nil {
			return "", "", err
		}
		req.Header.Set("User-Agent", userAgent)
		client := httpClient
		if pinned {
			client = httpClientPinned
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if resp.StatusCode == 429 || resp.StatusCode == 503 {
			lastErr = fmt.Errorf("google: http %d", resp.StatusCode)
			continue
		}
		if resp.StatusCode != 200 {
			return "", "", fmt.Errorf("google: http %d", resp.StatusCode)
		}
		var raw any
		if err := json.Unmarshal(body, &raw); err != nil {
			return "", "", fmt.Errorf("google: bad json: %w", err)
		}
		top, ok := raw.([]any)
		if !ok || len(top) == 0 {
			return "", "", fmt.Errorf("google: unexpected shape")
		}
		var sb strings.Builder
		if sents, ok := top[0].([]any); ok {
			for _, s := range sents {
				if arr, ok := s.([]any); ok && len(arr) > 0 {
					if str, ok := arr[0].(string); ok {
						sb.WriteString(str)
					}
				}
			}
		}
		detected := ""
		if len(top) > 2 {
			if d, ok := top[2].(string); ok {
				detected = d
			}
		}
		out := strings.TrimSpace(sb.String())
		if out == "" {
			lastErr = fmt.Errorf("google: empty result")
			continue
		}
		return out, detected, nil
	}
	return "", "", lastErr
}

// GoogleDict fetches dictionary data (pronunciation + POS groups) for a word.
func GoogleDict(ctx context.Context, word, from, to string) (*DictResult, error) {
	q := url.Values{}
	q.Set("client", googleClients[0])
	// Same 400 as googleTranslate: the endpoint rejects an empty sl.
	if from == "" {
		from = "auto"
	}
	q.Set("sl", from)
	q.Set("tl", to)
	q.Set("ie", "UTF-8")
	q.Set("oe", "UTF-8")
	q.Set("dt", "bd")
	q.Set("dt", "rm")
	q.Set("q", word)
	req, err := http.NewRequestWithContext(ctx, "GET", googleSingleURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("google dict: http %d", resp.StatusCode)
	}
	var raw any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	top, ok := raw.([]any)
	if !ok || len(top) < 2 {
		return nil, fmt.Errorf("google dict: unexpected shape")
	}
	dict, ok := top[1].([]any)
	if !ok || len(dict) == 0 {
		return nil, fmt.Errorf("google dict: no dictionary data")
	}
	res := &DictResult{}
	for _, d := range dict {
		arr, ok := d.([]any)
		if !ok || len(arr) < 3 {
			continue
		}
		pos, _ := arr[0].(string)
		var terms []string
		if expl, ok := arr[2].([]any); ok {
			for _, e := range expl {
				if t, ok := e.(string); ok {
					terms = append(terms, t)
				}
			}
		}
		if len(terms) > 0 {
			res.Entries = append(res.Entries, DictEntry{POS: pos, Explain: strings.Join(terms, "；")})
		}
	}
	// romanization lives in top[0][i][3] when present
	if sents, ok := top[0].([]any); ok && len(sents) > 0 {
		if arr, ok := sents[0].([]any); ok && len(arr) > 3 {
			if rm, ok := arr[3].(string); ok {
				res.Pron = rm
			}
		}
	}
	if len(res.Entries) == 0 {
		return nil, fmt.Errorf("google dict: empty entries")
	}
	return res, nil
}

// ---------------------------------------------------------------------------
// Race: fire Microsoft and Google together, first valid result wins, the
// loser is cancelled. Hides either engine's rate limits and slow moments.
// ---------------------------------------------------------------------------

// raceResult is what the race reports: the winner's text, which engine it
// came from, its own language detection, or the combined error when every
// route failed.

type raceResult struct {
	Text     string
	Engine   string
	Detected string
	Err      error
	// Untranslated marks an answer that came back as the source text: the
	// engine replied, it just had nothing to change. That is not a failure
	// (a retry returns the same text), so the card says so instead of showing
	// a red error the retry button cannot clear.
	Untranslated bool
}

// racePreferred is the engine the race waits a moment for. 微软 reads complex
// English better than 腾讯 and 谷歌 in side-by-side tests; the other two still
// cover the cases where it is rate limited or slow.
const racePreferred = "微软"

// raceGrace bounds how long a faster answer waits for the preferred engine.
const raceGrace = 350 * time.Millisecond

// shouldRecordBreaker says whether one engine attempt should update its
// breaker. A success must always be recorded: without it, an engine that
// recovers keeps its old failure count, and a single later failure can open
// the breaker. A cancellation caused by the race ending is not the engine's
// failure, but a cancellation/timeout from the engine's own request is.
func shouldRecordBreaker(ctx context.Context, err error) bool {
	if err == nil {
		return true
	}
	// A canceled request is the race stopping the loser, not an engine
	// failure, even if the request reports context.Canceled directly.
	if errors.Is(err, context.Canceled) {
		return false
	}
	// DeadlineExceeded can be a real engine timeout; only skip it when the
	// race context itself was canceled or hit its deadline.
	if errors.Is(err, context.DeadlineExceeded) {
		return ctx.Err() == nil
	}
	return true
}

// TranslateRace asks every usable engine at once and returns the best answer
// that arrives inside the grace window. Every engine here is keyless: the app
// asks for nothing but a selection, and there is no credential to configure.
func TranslateRace(ctx context.Context, text, from, to, mode string) raceResult {
	ctx2, cancel := context.WithCancel(ctx)
	defer cancel()

	// A pinned engine is tried even when its breaker is open: the user asked
	// for that engine, and silently answering with another one would be a lie.
	if mode == "microsoft" || mode == "google" || mode == "tencent" {
		var res raceResult
		switch mode {
		case "microsoft":
			r, err := MicrosoftTranslate(ctx2, text, from, to)
			currentBreaker().record("microsoft", err)
			res = raceResult{Text: r.Text, Engine: "微软", Detected: r.Detected, Err: err}
		case "tencent":
			r, err := tencentTranslate(ctx2, text, from, to)
			currentBreaker().record("tencent", err)
			res = raceResult{Text: r.Text, Engine: "腾讯", Detected: r.Detected, Err: err}
		default:
			t, d, err := googleTranslate(ctx2, text, from, to, true)
			currentBreaker().record("google", err)
			res = raceResult{Text: t, Engine: "谷歌", Detected: d, Err: err}
		}
		// Pinning an engine asks for that engine, not for an unchecked answer
		// from it: the same repairs and checks the race applies.
		return finalizePinned(text, from, to, res)
	}

	// The race: whichever engine answers first wins.
	type contender struct {
		name string
		run  func() (string, string, error)
	}
	cands := make([]contender, 0, 3)

	if currentBreaker().allow("microsoft") {
		cands = append(cands, contender{"微软", func() (string, string, error) {
			r, err := MicrosoftTranslate(ctx2, text, from, to)
			return r.Text, r.Detected, err
		}})
	}
	if currentBreaker().allow("google") {
		cands = append(cands, contender{"谷歌", func() (string, string, error) {
			return googleTranslate(ctx2, text, from, to, false)
		}})
	}
	// 腾讯 only joins pairs it can actually serve: it answers
	// "Unsupported-Language" for about half of this app's target list, and a
	// contender that always fails would open its breaker for no reason.
	if tencentSupportsTarget(to) && currentBreaker().allow("tencent") {
		cands = append(cands, contender{"腾讯", func() (string, string, error) {
			r, err := tencentTranslate(ctx2, text, from, to)
			return r.Text, r.Detected, err
		}})
	}
	// Every engine has its breaker open: try them all anyway rather than
	// failing. A blocked engine may have recovered, and a wrong-but-present
	// translation beats an error.
	if len(cands) == 0 {
		cands = append(cands,
			contender{"微软", func() (string, string, error) {
				r, err := MicrosoftTranslate(ctx2, text, from, to)
				return r.Text, r.Detected, err
			}},
			contender{"谷歌", func() (string, string, error) {
				return googleTranslate(ctx2, text, from, to, false)
			}},
		)
		if tencentSupportsTarget(to) {
			cands = append(cands, contender{"腾讯", func() (string, string, error) {
				r, err := tencentTranslate(ctx2, text, from, to)
				return r.Text, r.Detected, err
			}})
		}
	}

	// A race returns the first valid answer, but first is not the same as
	// best. Measured here, 腾讯 answers in 200-300ms and 微软 in 300-500ms, so
	// 腾讯 wins most races — and loses on exactly the sentences a reader
	// notices: "her arms are up" became "她的手臂向上", "scale law" became
	// "比例律". The reader sees one engine's weakest output at random and
	// concludes the translation is broken.
	//
	// The preferred engine therefore gets a short grace window: when another
	// engine answers first, its text is held for up to raceGrace so 微软 can
	// still land. A healthy 微软 usually does; when it is slow or down the
	// cost is bounded at 350ms, below the point where the popup feels stalled.
	preferredPending := false
	for _, c := range cands {
		if c.name == racePreferred {
			preferredPending = true
		}
	}

	ch := make(chan raceResult, len(cands))
	for _, c := range cands {
		c := c
		go func() {
			defer recoverLog("services.go:TranslateRace")
			txt, det, err := c.run()
			// Record here, not after the race is decided. The winner returns
			// at once and stops reading the channel, so a loser's failure was
			// never seen and its breaker never opened — the engine stayed in
			// the race to fail again on the next selection.
			//
			// But a loser whose context we cancelled is not a failed engine:
			// cancel() at the winner makes its in-flight request return
			// context.Canceled. Counting that as a failure opened the breaker
			// for a healthy slow engine on every single race. Successes are
			// still recorded, so a recovered engine can reset its count.
			if shouldRecordBreaker(ctx2, err) {
				currentBreaker().record(engineKey(c.name), err)
			}
			ch <- raceResult{Text: txt, Engine: c.name, Detected: det, Err: err}
		}()
	}

	var errs []string
	var held *raceResult
	var echoed *raceResult
	var grace <-chan time.Time
	for i := 0; i < len(cands); i++ {
		var r raceResult
		select {
		case r = <-ch:
		case <-grace:
			cancel()
			return *held
		}
		if r.Err == nil {
			r.Text = repairSplitLatin(text, r.Text)
		}
		if r.Engine == racePreferred {
			preferredPending = false
		}
		if r.Err == nil && r.Text != "" && looksUntranslated(text, r.Text, from, to) {
			// 腾讯 hands back the source untouched when it cannot handle the
			// pair, and a race that accepts it wins with the sentence the
			// reader already had. Measured on a Portuguese sentence: 腾讯
			// returned it verbatim in 365ms while 微软 translated it.
			//
			// The echo still may not win, but it is an answer rather than a
			// failure: keep it in case every engine echoes, so the card can say
			// what happened instead of listing three "failures" that a retry
			// reproduces.
			r.Untranslated = true
			echoed = &r
			continue
		}
		if r.Err == nil && r.Text != "" {
			if r.Engine == racePreferred || !preferredPending {
				cancel() // stop the losers
				return r
			}
			if held == nil {
				held = &r
				grace = time.After(raceGrace)
			}
			continue
		}
		// The contender goroutine already recorded this attempt's outcome.
		// Recording it again here doubled every failure: with a threshold of
		// two, one bad answer was enough to open the breaker for a minute.
		if r.Err != nil {
			errs = append(errs, r.Err.Error())
		} else {
			errs = append(errs, r.Engine+": empty result")
		}
		// The preferred engine failed: the held answer is now the best there
		// is, and there is nothing left to wait for.
		if held != nil && !preferredPending {
			cancel()
			return *held
		}
	}
	if held != nil {
		cancel()
		return *held
	}
	if echoed != nil {
		cancel()
		return *echoed
	}
	return raceResult{Err: fmt.Errorf("all engines failed: %s", strings.Join(errs, " | "))}
}

// finalizePinned applies the repairs and the echo classification a pinned
// engine's answer gets before it reaches the card.
//
// An echoed answer used to become an error. That was wrong: 微软 returns a name
// it does not translate ("fastDictionary", "GitHub", "kubernetes" -> "Kubernetes")
// exactly as it received it, and pressing 重试 produces the same text again. A
// failure needs a cause a retry could change; an echo only needs saying out
// loud.
func finalizePinned(source, from, to string, res raceResult) raceResult {
	if res.Err != nil {
		return res
	}
	res.Text = repairSplitLatin(source, res.Text)
	res.Untranslated = looksUntranslated(source, res.Text, from, to)
	return res
}

// pickEchoAnswer decides what the card shows when a pinned engine echoed the
// source: another engine's answer when the race produced one, otherwise the
// echo itself.
//
// The race already drops echoed answers, so a usable fallback there is a real
// translation. Pinning is about which engine the reader asked for, not about
// showing them nothing when that engine has nothing to say.
func pickEchoAnswer(pinned, fallback raceResult) (raceResult, bool) {
	if fallback.Err == nil && !fallback.Untranslated && strings.TrimSpace(fallback.Text) != "" {
		return fallback, true
	}
	return pinned, false
}

// looksUntranslated reports whether an engine echoed the sentence back instead
// of translating it. Only judged when the two languages differ, and only on a
// whole-sentence match: a translation that keeps a proper noun or a code
// snippet is normal, a translation identical to its input is not.
func looksUntranslated(source, result, from, to string) bool {
	if from != "" && to != "" && strings.EqualFold(from, to) {
		return false
	}
	norm := func(s string) string {
		s = strings.ToLower(strings.TrimSpace(s))
		return strings.Join(strings.Fields(s), " ")
	}
	return norm(source) != "" && norm(source) == norm(result)
}

// repairSplitLatin puts back a Latin word that an engine cut in two.
//
// Measured 2026-11-14 against the endpoint this app calls,
// edge.microsoft.com/translate/translatetext:
//
//	in:  I can't believe now I am counting on Huawei and Deepseek for the very
//	     survival of the OSS community. What a loss...
//	out: 我简直不敢相信现在我竟然指望华为和Deepsee克来拯救OSS社区的生存。真是损失......
//
// The engine keeps the word up to its last letter and renders that letter as a
// Han character ("k" → 克). The same sentence with the brand's own spelling
// "DeepSeek" survives intact, so this is the tokenizer meeting an unknown
// all-lowercase name, not a missing glossary entry. 腾讯 answers the exact
// sentence correctly, which is why the race made the defect look random: the
// reader sees it only on the runs 微软 wins.
//
// The repair is narrow on purpose: a run of Latin letters in the output must
// spell the start of a Latin word from the source, that word must be missing
// whole from the output, the run must be at least four letters, and exactly one
// letter must be missing — one letter is what one Han character can stand in
// for. Outside that shape the output is left alone: a wrong repair costs more
// than the letter it saves.
func repairSplitLatin(source, translated string) string {
	if source == "" || translated == "" {
		return translated
	}
	var words []string
	for _, w := range strings.FieldsFunc(source, func(r rune) bool { return !isLatinLetter(r) }) {
		// Shorter words cannot be split and still leave a four-letter run.
		if len(w) >= 5 {
			words = append(words, w)
		}
	}
	if len(words) == 0 {
		return translated
	}
	lower := strings.ToLower(translated)
	rs := []rune(translated)
	var b strings.Builder
	b.Grow(len(translated))
	for i := 0; i < len(rs); {
		if !isLatinLetter(rs[i]) {
			b.WriteRune(rs[i])
			i++
			continue
		}
		j := i
		for j < len(rs) && isLatinLetter(rs[j]) {
			j++
		}
		run := string(rs[i:j])
		if j < len(rs) && isCJK(rs[j]) && len(run) >= 4 {
			if word, ok := wholeWordBehind(words, run, lower); ok {
				b.WriteString(word)
				i = j + 1 // the Han character stood in for the lost letters
				continue
			}
		}
		b.WriteString(run)
		i = j
	}
	return b.String()
}

// wholeWordBehind answers "which source word starts with run and is missing
// whole from the output", which is the signature of the split above.
func wholeWordBehind(words []string, run, translatedLower string) (string, bool) {
	for _, w := range words {
		if len(w)-len(run) != 1 || !strings.EqualFold(w[:len(run)], run) {
			continue
		}
		if strings.Contains(translatedLower, strings.ToLower(w)) {
			// The word is already there somewhere; the run is something else.
			continue
		}
		return w, true
	}
	return "", false
}

func isLatinLetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

// engineKey maps a display name onto the breaker's key.
func engineKey(display string) string {
	switch display {
	case "微软":
		return "microsoft"
	case "谷歌":
		return "google"
	case "腾讯":
		return "tencent"
	}
	return display
}

// record updates a breaker from one attempt's outcome.
func (b *engineBreaker) record(name string, err error) {
	if err != nil {
		b.fail(name)
		b.mu.Lock()
		if b.lastErr == nil {
			// Tests replace the package-level breaker with a minimal literal;
			// keep the diagnostic map optional rather than requiring every
			// constructor to remember it.
			b.lastErr = map[string]string{}
		}
		b.lastErr[name] = err.Error()
		b.mu.Unlock()
		return
	}
	b.ok(name)
}

// engineHealth is the read-only view of one engine's breaker that the
// settings page can render. It deliberately has no dependency on the UI.
type engineHealth struct {
	Engine    string
	Fails     int
	Open      bool
	Until     time.Time
	LastError string
}

// health reports the current breaker state without changing it.
func (b *engineBreaker) health(name string) engineHealth {
	b.mu.Lock()
	defer b.mu.Unlock()
	h := engineHealth{Engine: name, Fails: b.fails[name], LastError: b.lastErr[name]}
	if until, ok := b.until[name]; ok {
		if time.Now().Before(until) {
			h.Open, h.Until = true, until
		} else {
			// Expired breakers are logically closed even before allow() cleans
			// the map; report the honest state instead of a stale countdown.
			h.Fails = 0
			h.LastError = ""
		}
	}
	return h
}

// engineHealths returns the three built-in engines in display order.
func engineHealths() []engineHealth {
	names := []struct{ key, label string }{
		{"microsoft", "微软"},
		{"google", "谷歌"},
		{"tencent", "腾讯"},
	}
	out := make([]engineHealth, 0, len(names))
	for _, item := range names {
		h := currentBreaker().health(item.key)
		h.Engine = item.label
		out = append(out, h)
	}
	return out
}

// isWord reports whether text is a single ASCII dictionary-worthy word.
func isWord(text string) bool {
	t := strings.TrimSpace(text)
	if len(t) == 0 || len(t) > 32 {
		return false
	}
	for i := 0; i < len(t); i++ {
		r := t[i]
		isAlpha := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		if !isAlpha && r != '-' && r != '\'' {
			return false
		}
	}
	return true
}

// engineBreaker remembers an engine that has just failed repeatedly, so the
// race stops paying for it.
//
// Google's keyless endpoint answers a burst of requests with an HTML "Sorry"
// page and HTTP 429. Each of those costs the failed engine's own retry loop —
// two attempts, 1.6 seconds — while the healthy engine has already answered.
// The winner was never in doubt, so the only thing the loser added was load,
// which extends its own blockout.
//
// A breaker is keyed by engine name and holds an expiry. While it is open the
// engine is skipped entirely; the first request after it expires tries again,
// so recovery needs no timer.
type engineBreaker struct {
	mu      sync.Mutex
	until   map[string]time.Time
	fails   map[string]int
	lastErr map[string]string
}

// breakers is swapped atomically so a test can start from a clean breaker even
// when a request started by an earlier test is still finishing. Production
// never replaces it after init; the indirection exists only to make that
// isolation race-free.
var breakers atomic.Pointer[engineBreaker]

func init() {
	breakers.Store(&engineBreaker{
		until:   map[string]time.Time{},
		fails:   map[string]int{},
		lastErr: map[string]string{},
	})
}

// currentBreaker is the single read point for the package breaker.
func currentBreaker() *engineBreaker { return breakers.Load() }

// breakerThreshold is how many consecutive failures open the breaker.
//
// One is not enough: a single 429 can be a transient blip, and skipping the
// engine for a full minute on that basis would cost quality on every request
// that follows. Two is the smallest count that distinguishes a burst from a
// coincidence.
const breakerThreshold = 2

// breakerCooldown is how long an open breaker lasts.
//
// Google's blockouts last from a few seconds to a few minutes; a minute lets
// the common case recover before the next selection, and costs nothing when it
// does not, because the other engines carry the translation.
const breakerCooldown = 60 * time.Second

// allow reports whether an engine may be tried.
func (b *engineBreaker) allow(name string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	until, open := b.until[name]
	if !open {
		return true
	}
	if time.Now().After(until) {
		// Expired: forget the failures so the next burst starts clean.
		delete(b.until, name)
		delete(b.fails, name)
		return true
	}
	return false
}

// ok clears an engine's failure count.
func (b *engineBreaker) ok(name string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.fails, name)
	delete(b.until, name)
	delete(b.lastErr, name)
}

// fail records a failure and opens the breaker once the threshold is reached.
func (b *engineBreaker) fail(name string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.fails == nil {
		b.fails = map[string]int{}
	}
	if b.until == nil {
		b.until = map[string]time.Time{}
	}
	b.fails[name]++
	if b.fails[name] >= breakerThreshold {
		b.until[name] = time.Now().Add(breakerCooldown)
	}
}

// open reports whether an engine is currently being skipped, for the UI.
func (b *engineBreaker) open(name string) bool { return !b.allow(name) }
