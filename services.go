package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
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

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

// ---------------------------------------------------------------------------
// Local language detection: zero network cost. CJK/Hangul/Kana ranges cover
// the direction choice (zh -> en, anything else -> zh). Engines also return
// their own detection, which the UI shows for reference.
// ---------------------------------------------------------------------------

func detectLocal(text string) string {
	var han, kana, hangul, latin int
	for _, r := range text {
		switch {
		case unicode.Is(unicode.Han, r):
			han++
		case unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r):
			kana++
		case unicode.Is(unicode.Hangul, r):
			hangul++
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			latin++
		}
	}
	switch {
	case kana > 0 && kana >= hangul:
		return "ja"
	case hangul > 0:
		return "ko"
	case han > latin:
		return "zh-CN"
	default:
		return "en"
	}
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
// Google: free gtx web endpoint on translate.googleapis.com (the API host is
// steadier than translate.google.com, which intermittently 429s on shared
// proxy nodes). dt=bd adds dictionary data for single words.
// ---------------------------------------------------------------------------

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

// googleTranslate does the work; pinned selects the fail-fast client: a
// pinned engine has no second engine to cover a hung request, so it uses
// the shorter timeout rather than making the popup wait eight seconds on a
// route Google is throttling. It used to be a global atomic set by
// TranslateRace, which made concurrent translations interfere.
func googleTranslate(ctx context.Context, text, from, to string, pinned bool) (string, string, error) {
	q := url.Values{}
	q.Set("client", "gtx")
	q.Set("sl", from)
	q.Set("tl", to)
	q.Set("hl", to)
	q.Set("ie", "UTF-8")
	q.Set("oe", "UTF-8")
	q.Set("dt", "t")
	q.Set("q", text)

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", "", ctx.Err()
			case <-time.After(800 * time.Millisecond):
			}
		}
		req, err := http.NewRequestWithContext(ctx, "GET", "https://translate.googleapis.com/translate_a/single?"+q.Encode(), nil)
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
	q.Set("client", "gtx")
	q.Set("sl", from)
	q.Set("tl", to)
	q.Set("ie", "UTF-8")
	q.Set("oe", "UTF-8")
	q.Set("dt", "bd")
	q.Set("dt", "rm")
	q.Set("q", word)
	req, err := http.NewRequestWithContext(ctx, "GET", "https://translate.googleapis.com/translate_a/single?"+q.Encode(), nil)
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
}

func TranslateRace(ctx context.Context, text, from, to, mode string) raceResult {
	ctx2, cancel := context.WithCancel(ctx)
	defer cancel()

	if mode == "microsoft" || mode == "google" {
		// single-engine modes: no race, the user pinned one provider
		if mode == "microsoft" {
			r, err := MicrosoftTranslate(ctx2, text, from, to)
			return raceResult{Text: r.Text, Engine: "微软", Detected: r.Detected, Err: err}
		}
		t, d, err := googleTranslate(ctx2, text, from, to, true)
		return raceResult{Text: t, Engine: "谷歌", Detected: d, Err: err}
	}

	ch := make(chan raceResult, 2)

	go func() {
		r, err := MicrosoftTranslate(ctx2, text, from, to)
		ch <- raceResult{Text: r.Text, Engine: "微软", Detected: r.Detected, Err: err}
	}()
	go func() {
		t, d, err := googleTranslate(ctx2, text, from, to, false)
		ch <- raceResult{Text: t, Engine: "谷歌", Detected: d, Err: err}
	}()

	var errs []string
	for i := 0; i < 2; i++ {
		r := <-ch
		if r.Err == nil && r.Text != "" {
			cancel() // stop the loser
			return r
		}
		if r.Err != nil {
			errs = append(errs, r.Err.Error())
		} else {
			errs = append(errs, r.Engine+": empty result")
		}
	}
	return raceResult{Err: fmt.Errorf("all engines failed: %s", strings.Join(errs, " | "))}
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
