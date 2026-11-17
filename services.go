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

var httpClient = &http.Client{Timeout: 8 * time.Second}

var httpClientPinned = &http.Client{Timeout: 4 * time.Second}

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

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
			latin++
		}
	}
	switch {
	case kana > 0 && kana >= hangul:
		return "ja"
	case hangul > 0:
		return "ko"
	case han > 0 && han*3 >= latin:
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
		return "ru"
	default:
		return ""
	}
}

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

func sourceLangFor(text string) string {
	if tag := detectLocal(text); tag != "" {
		return tag
	}
	if loneLatinWord(text) {
		return "en"
	}
	return ""
}

func translateLangs(cfg Config, text string) (from, to string) {
	from = sourceLangFor(text)
	to = cfg.TargetLang
	if to == "" {
		to = "zh-Hans"
	}
	if from == to || sameLang(from, to) {
		to = cfg.SecondLang
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

var googleClients = []string{"dict-chrome-ex", "gtx"}

type DictEntry struct {
	POS     string
	Explain string
}

type DictResult struct {
	Pron    string
	Entries []DictEntry
}

func GoogleTranslate(ctx context.Context, text, from, to string) (string, string, error) {
	return googleTranslate(ctx, text, from, to, false)
}

const googleSingleDefaultURL = "https://translate.googleapis.com/translate_a/single"

var googleSingleURL = googleSingleDefaultURL

func googleTranslate(ctx context.Context, text, from, to string, pinned bool) (string, string, error) {
	q := url.Values{}
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

func GoogleDict(ctx context.Context, word, from, to string) (*DictResult, error) {
	q := url.Values{}
	q.Set("client", googleClients[0])
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

type raceResult struct {
	Text         string
	Engine       string
	Detected     string
	Err          error
	Untranslated bool
}

const racePreferred = "微软"

const raceGrace = 350 * time.Millisecond

func shouldRecordBreaker(ctx context.Context, err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ctx.Err() == nil
	}
	return true
}

func TranslateRace(ctx context.Context, text, from, to, mode string) raceResult {
	ctx2, cancel := context.WithCancel(ctx)
	defer cancel()

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
		return finalizePinned(text, from, to, res)
	}

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
	if tencentSupportsTarget(to) && currentBreaker().allow("tencent") {
		cands = append(cands, contender{"腾讯", func() (string, string, error) {
			r, err := tencentTranslate(ctx2, text, from, to)
			return r.Text, r.Detected, err
		}})
	}
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
			r.Untranslated = true
			echoed = &r
			continue
		}
		if r.Err == nil && r.Text != "" {
			if r.Engine == racePreferred || !preferredPending {
				cancel()
				return r
			}
			if held == nil {
				held = &r
				grace = time.After(raceGrace)
			}
			continue
		}
		if r.Err != nil {
			errs = append(errs, r.Err.Error())
		} else {
			errs = append(errs, r.Engine+": empty result")
		}
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

func finalizePinned(source, from, to string, res raceResult) raceResult {
	if res.Err != nil {
		return res
	}
	res.Text = repairSplitLatin(source, res.Text)
	res.Untranslated = looksUntranslated(source, res.Text, from, to)
	return res
}

func pickEchoAnswer(pinned, fallback raceResult) (raceResult, bool) {
	if fallback.Err == nil && !fallback.Untranslated && strings.TrimSpace(fallback.Text) != "" {
		return fallback, true
	}
	return pinned, false
}

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

func repairSplitLatin(source, translated string) string {
	if source == "" || translated == "" {
		return translated
	}
	var words []string
	for _, w := range strings.FieldsFunc(source, func(r rune) bool { return !isLatinLetter(r) }) {
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
				i = j + 1
				continue
			}
		}
		b.WriteString(run)
		i = j
	}
	return b.String()
}

func wholeWordBehind(words []string, run, translatedLower string) (string, bool) {
	for _, w := range words {
		if len(w)-len(run) != 1 || !strings.EqualFold(w[:len(run)], run) {
			continue
		}
		if strings.Contains(translatedLower, strings.ToLower(w)) {
			continue
		}
		return w, true
	}
	return "", false
}

func isLatinLetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

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

func (b *engineBreaker) record(name string, err error) {
	if err != nil {
		b.fail(name)
		b.mu.Lock()
		if b.lastErr == nil {
			b.lastErr = map[string]string{}
		}
		b.lastErr[name] = err.Error()
		b.mu.Unlock()
		return
	}
	b.ok(name)
}

type engineHealth struct {
	Engine    string
	Fails     int
	Open      bool
	Until     time.Time
	LastError string
}

func (b *engineBreaker) health(name string) engineHealth {
	b.mu.Lock()
	defer b.mu.Unlock()
	h := engineHealth{Engine: name, Fails: b.fails[name], LastError: b.lastErr[name]}
	if until, ok := b.until[name]; ok {
		if time.Now().Before(until) {
			h.Open, h.Until = true, until
		} else {
			h.Fails = 0
			h.LastError = ""
		}
	}
	return h
}

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

type engineBreaker struct {
	mu      sync.Mutex
	until   map[string]time.Time
	fails   map[string]int
	lastErr map[string]string
}

var breakers atomic.Pointer[engineBreaker]

func init() {
	breakers.Store(&engineBreaker{
		until:   map[string]time.Time{},
		fails:   map[string]int{},
		lastErr: map[string]string{},
	})
}

func currentBreaker() *engineBreaker { return breakers.Load() }

const breakerThreshold = 2

const breakerCooldown = 60 * time.Second

func (b *engineBreaker) allow(name string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	until, open := b.until[name]
	if !open {
		return true
	}
	if time.Now().After(until) {
		delete(b.until, name)
		delete(b.fails, name)
		return true
	}
	return false
}

func (b *engineBreaker) ok(name string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.fails, name)
	delete(b.until, name)
	delete(b.lastErr, name)
}

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

func (b *engineBreaker) open(name string) bool { return !b.allow(name) }
