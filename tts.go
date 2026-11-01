// Edge TTS: the neural voices read-frog uses, ported to Go.
//
// The old fallback was SAPI5 (Huihui/Zira/David), which is concatenative and
// sounds like a machine. Edge TTS is the same service the browser's Read
// Aloud uses: it returns an ordinary mp3, so the existing MCI player handles
// it with no new dependency.
//
// Two calls:
//
//  1. POST dev.microsofttranslator.com/apps/endpoint with an HMAC-SHA256
//     X-MT-Signature header. The response carries a bearer token and the
//     region the synthesis endpoint lives in.
//  2. POST {region}.tts.speech.microsoft.com/cognitiveservices/v1 with that
//     token and an SSML body. The response body is the mp3.
//
// The signing material is public (it ships inside every first-party Edge
// client); the algorithm is read-frog's utils/server/edge-tts/signature.ts.
package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	edgeTTSEndpointURL = "https://dev.microsofttranslator.com/apps/endpoint?api-version=1.0"
	edgeTTSAppID       = "MSTranslatorAndroidApp"
	edgeTTSClientVer   = "4.0.530a 5fe1dc6c"
	edgeTTSUserID      = "0f04d16a175c411e"
	edgeTTSHomeRegion  = "zh-Hans-CN"
	edgeTTSUserAgent   = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Safari/537.36 Edg/127.0.0.0"
	// edgeTTSFormat is the audio the service returns.
	//
	// It used to be audio-24khz-48kbitrate-mono-mp3: a 24kHz band-limited
	// stream. Whatever a better DAC could reveal is not in that signal, which
	// is why a motherboard jack and a USB dongle sound identical on it — the
	// information was lost at the source, not in the output stage.
	//
	// Measured on this machine, same sentence:
	//
	//	24kHz/48kbps   1.04s   100 KB   (old)
	//	24kHz/96kbps   0.78s   201 KB
	//	48kHz/96kbps   1.02s   201 KB   (chosen)
	//	48kHz/192kbps  1.83s   401 KB
	//
	// 48kHz/96kbps carries the full band the neural voice produces. 192kbps
	// buys nothing audible for speech and costs most of a second, which the
	// speaker button wears as sluggishness.
	edgeTTSFormat = "audio-48khz-96kbitrate-mono-mp3"
	// edgeTTSSecretBase64 is the signing key every Edge/Translator client
	// carries. It is not a user secret: it identifies the client, nothing
	// more.
	edgeTTSSecretBase64 = "oik6PdDdMnOXemTbwvMn9de/h9lFnfBaCWbGMMZqqoSaQaqUOqjVGm5NqsmjcBI1x+sS9ugjB55HEJWRiFXYFw=="
	// Refresh a little before the token expires, so a long read never dies
	// mid-sentence.
	edgeTTSTokenSlack = 3 * time.Minute
)

// edgeTTSAuth is the cached endpoint token.
type edgeTTSAuth struct {
	token     string
	region    string
	expiresAt time.Time
}

var (
	edgeTTSMu    sync.Mutex
	edgeTTSAuth_ edgeTTSAuth
)

// edgeTTSHTTP is the client for the two TTS calls. It is separate from
// httpClient (the 8s translation client) because a long sentence takes
// longer to synthesise than a translation request.
var edgeTTSHTTP = &http.Client{Timeout: 15 * time.Second}

// edgeTTSSignature builds the X-MT-Signature header: appid, base64 HMAC of
// "appid + url + date + requestID", the date and the request id, joined by
// "::". Everything is lowercased before signing, which is what the service
// expects.
func edgeTTSSignature(now time.Time, requestID string) (string, error) {
	key, err := base64.StdEncoding.DecodeString(edgeTTSSecretBase64)
	if err != nil {
		return "", err
	}
	encodedURL := strings.ReplaceAll(edgeTTSEndpointURL, "https://", "")
	encodedURL = strings.ReplaceAll(encodedURL, ":", "%3A")
	encodedURL = strings.ReplaceAll(encodedURL, "/", "%2F")
	encodedURL = strings.ReplaceAll(encodedURL, "?", "%3F")
	encodedURL = strings.ReplaceAll(encodedURL, "=", "%3D")
	encodedURL = strings.ReplaceAll(encodedURL, "&", "%26")

	date := strings.ToLower(now.UTC().Format("Mon, 02 Jan 2006 15:04:05")) + " GMT"
	payload := strings.ToLower(edgeTTSAppID + encodedURL + date + requestID)

	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(payload))
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return edgeTTSAppID + "::" + sig + "::" + date + "::" + requestID, nil
}

// edgeTTSFetchAuth exchanges a signature for a bearer token and the region
// its endpoint lives in.
func edgeTTSFetchAuth(ctx context.Context) (edgeTTSAuth, error) {
	reqID := randomHexID(16)
	sig, err := edgeTTSSignature(time.Now(), reqID)
	if err != nil {
		return edgeTTSAuth{}, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", edgeTTSEndpointURL, strings.NewReader(""))
	if err != nil {
		return edgeTTSAuth{}, err
	}
	req.Header.Set("Accept-Language", "zh-Hans")
	req.Header.Set("X-ClientVersion", edgeTTSClientVer)
	req.Header.Set("X-UserId", edgeTTSUserID)
	req.Header.Set("X-HomeGeographicRegion", edgeTTSHomeRegion)
	req.Header.Set("X-ClientTraceId", reqID)
	req.Header.Set("X-MT-Signature", sig)
	req.Header.Set("User-Agent", edgeTTSUserAgent)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := edgeTTSHTTP.Do(req)
	if err != nil {
		return edgeTTSAuth{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return edgeTTSAuth{}, err
	}
	if resp.StatusCode != 200 {
		return edgeTTSAuth{}, fmt.Errorf("edge tts auth: http %d", resp.StatusCode)
	}
	var out struct {
		Token  string `json:"t"`
		Region string `json:"r"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return edgeTTSAuth{}, fmt.Errorf("edge tts auth: %w", err)
	}
	if out.Token == "" || out.Region == "" {
		return edgeTTSAuth{}, fmt.Errorf("edge tts auth: empty token or region")
	}
	// The token is a JWT; its exp claim is the real expiry. Falling back to
	// a ten minute lifetime matches the service's default.
	ttl := 10 * time.Minute
	if exp, ok := jwtExpiry(out.Token); ok {
		ttl = time.Until(exp)
	}
	return edgeTTSAuth{token: out.Token, region: out.Region, expiresAt: time.Now().Add(ttl)}, nil
}

// jwtExpiry reads the exp claim of a JWT without verifying it. The token is
// used immediately and the service validates it; this only schedules the
// refresh.
func jwtExpiry(token string) (time.Time, bool) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return time.Time{}, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, false
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(raw, &claims); err != nil || claims.Exp == 0 {
		return time.Time{}, false
	}
	return time.Unix(claims.Exp, 0), true
}

// edgeTTSGetAuth returns a cached token, fetching a new one when it is
// missing or about to expire.
func edgeTTSGetAuth(ctx context.Context) (edgeTTSAuth, error) {
	edgeTTSMu.Lock()
	cached := edgeTTSAuth_
	edgeTTSMu.Unlock()
	if cached.token != "" && time.Now().Before(cached.expiresAt.Add(-edgeTTSTokenSlack)) {
		return cached, nil
	}
	fresh, err := edgeTTSFetchAuth(ctx)
	if err != nil {
		return edgeTTSAuth{}, err
	}
	edgeTTSMu.Lock()
	edgeTTSAuth_ = fresh
	edgeTTSMu.Unlock()
	return fresh, nil
}

// edgeTTSVoice picks a neural voice for a language tag, honouring a user
// override when one is configured.
//
// Every language the translator offers needs a voice here. The list used to
// stop at the nine the picker had, so when the picker grew to thirty-four the
// other twenty-five fell through to en-US-AriaNeural: Japanese, Arabic, Thai
// and Hindi were all read aloud by an English speaker. That is the "robotic"
// sound — an English voice pronouncing foreign text.
func edgeTTSVoice(langTag string, us bool, override string) string {
	if strings.TrimSpace(override) != "" {
		return strings.TrimSpace(override)
	}
	base := strings.ToLower(strings.TrimSpace(langTag))
	switch {
	case strings.HasPrefix(base, "en"):
		if us {
			return "en-US-AriaNeural"
		}
		return "en-GB-SoniaNeural"
	case strings.HasPrefix(base, "zh-hant") || base == "zh-tw" || base == "zh-hk":
		return "zh-TW-HsiaoChenNeural"
	case strings.HasPrefix(base, "zh"):
		return "zh-CN-XiaoxiaoNeural"
	case strings.HasPrefix(base, "ja"):
		return "ja-JP-NanamiNeural"
	case strings.HasPrefix(base, "ko"):
		return "ko-KR-SunHiNeural"
	case strings.HasPrefix(base, "th"):
		return "th-TH-PremwadeeNeural"
	case strings.HasPrefix(base, "vi"):
		return "vi-VN-HoaiMyNeural"
	case strings.HasPrefix(base, "id"):
		return "id-ID-GadisNeural"
	case strings.HasPrefix(base, "ms"):
		return "ms-MY-YasminNeural"
	case strings.HasPrefix(base, "tl") || strings.HasPrefix(base, "fil"):
		return "fil-PH-BlessicaNeural"
	case strings.HasPrefix(base, "fr"):
		return "fr-FR-DeniseNeural"
	case strings.HasPrefix(base, "de"):
		return "de-DE-KatjaNeural"
	case strings.HasPrefix(base, "es"):
		return "es-ES-ElviraNeural"
	case strings.HasPrefix(base, "pt"):
		return "pt-BR-FranciscaNeural"
	case strings.HasPrefix(base, "it"):
		return "it-IT-ElsaNeural"
	case strings.HasPrefix(base, "nl"):
		return "nl-NL-ColetteNeural"
	case strings.HasPrefix(base, "ru"):
		return "ru-RU-SvetlanaNeural"
	case strings.HasPrefix(base, "uk"):
		return "uk-UA-PolinaNeural"
	case strings.HasPrefix(base, "pl"):
		return "pl-PL-ZofiaNeural"
	case strings.HasPrefix(base, "sv"):
		return "sv-SE-SofieNeural"
	case strings.HasPrefix(base, "da"):
		return "da-DK-ChristelNeural"
	case strings.HasPrefix(base, "nb") || strings.HasPrefix(base, "no"):
		return "nb-NO-PernilleNeural"
	case strings.HasPrefix(base, "fi"):
		return "fi-FI-NooraNeural"
	case strings.HasPrefix(base, "cs"):
		return "cs-CZ-VlastaNeural"
	case strings.HasPrefix(base, "ro"):
		return "ro-RO-AlinaNeural"
	case strings.HasPrefix(base, "hu"):
		return "hu-HU-NoemiNeural"
	case strings.HasPrefix(base, "el"):
		return "el-GR-AthinaNeural"
	case strings.HasPrefix(base, "tr"):
		return "tr-TR-EmelNeural"
	case strings.HasPrefix(base, "ar"):
		return "ar-SA-ZariyahNeural"
	case strings.HasPrefix(base, "he"):
		return "he-IL-HilaNeural"
	case strings.HasPrefix(base, "fa"):
		return "fa-IR-DilaraNeural"
	case strings.HasPrefix(base, "hi"):
		return "hi-IN-SwaraNeural"
	case strings.HasPrefix(base, "bn"):
		return "bn-IN-TanishaaNeural"
	case strings.HasPrefix(base, "ur"):
		return "ur-PK-UzmaNeural"
	}
	// An unknown tag is better read by an English voice than not at all, but
	// it should be rare: every offered language is listed above.
	return "en-US-AriaNeural"
}

// escapeSSML escapes the five XML entities and drops control characters the
// service rejects.
func escapeSSML(text string) string {
	var b strings.Builder
	for _, r := range text {
		switch {
		case r == '&':
			b.WriteString("&amp;")
		case r == '<':
			b.WriteString("&lt;")
		case r == '>':
			b.WriteString("&gt;")
		case r == '"':
			b.WriteString("&quot;")
		case r == '\'':
			b.WriteString("&apos;")
		case r < 0x20 && r != '\t' && r != '\n':
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// edgeTTSSSML wraps the text in the SSML body the service expects.
func edgeTTSSSML(text, voice string) string {
	locale := "en-US"
	parts := strings.Split(voice, "-")
	if len(parts) >= 2 {
		locale = parts[0] + "-" + parts[1]
	}
	return fmt.Sprintf(
		"<speak version=\"1.0\" xmlns=\"http://www.w3.org/2001/10/synthesis\" xml:lang=\"%s\"><voice name=\"%s\"><prosody rate=\"+0%%\" pitch=\"+0Hz\" volume=\"+0%%\">%s</prosody></voice></speak>",
		escapeSSML(locale), escapeSSML(voice), escapeSSML(text))
}

// FetchEdgeTTS synthesises text to a cached mp3 file and returns its path.
// Repeated playback of the same text is served from the cache, which matters
// for the chip's speak button.
func FetchEdgeTTS(ctx context.Context, text, voice string) (string, error) {
	return fetchEdgeTTSFormat(ctx, text, voice, edgeTTSFormat)
}

// fetchEdgeTTSWithFormat is the measurement seam: it returns the byte count
// too, so a format can be compared instead of guessed.
func fetchEdgeTTSWithFormat(ctx context.Context, text, voice, format string) (string, int, error) {
	path, err := fetchEdgeTTSFormat(ctx, text, voice, format)
	if err != nil {
		return "", 0, err
	}
	fi, statErr := os.Stat(path)
	if statErr != nil {
		return path, 0, nil
	}
	return path, int(fi.Size()), nil
}

// fetchEdgeTTSFormat synthesises with an explicit output format and caches by
// (voice, format, text).
func fetchEdgeTTSFormat(ctx context.Context, text, voice, format string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("edge tts: empty text")
	}
	dir := filepath.Join(os.TempDir(), "fastDictionary_tts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(voice + "\x00" + format + "\x00" + text))
	path := filepath.Join(dir, hex.EncodeToString(sum[:16])+".mp3")
	if fi, err := os.Stat(path); err == nil && fi.Size() > 1024 {
		return path, nil
	}

	auth, err := edgeTTSGetAuth(ctx)
	if err != nil {
		return "", err
	}
	endpoint := "https://" + auth.region + ".tts.speech.microsoft.com/cognitiveservices/v1"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader([]byte(edgeTTSSSML(text, voice))))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", auth.token)
	req.Header.Set("Content-Type", "application/ssml+xml")
	req.Header.Set("X-Microsoft-OutputFormat", format)
	req.Header.Set("User-Agent", edgeTTSUserAgent)

	resp, err := edgeTTSHTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		// The token went stale between the cache check and the call.
		edgeTTSMu.Lock()
		edgeTTSAuth_ = edgeTTSAuth{}
		edgeTTSMu.Unlock()
		return "", fmt.Errorf("edge tts: token rejected (http %d)", resp.StatusCode)
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("edge tts: http %d", resp.StatusCode)
	}
	tmp := path + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	n, copyErr := io.Copy(f, resp.Body)
	closeErr := f.Close()
	if copyErr != nil || closeErr != nil || n < 1024 {
		os.Remove(tmp)
		if copyErr != nil {
			return "", copyErr
		}
		return "", fmt.Errorf("edge tts: short audio (%d bytes)", n)
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	return path, nil
}

// randomHexID returns n random bytes as lowercase hex, used for the request
// id the signature covers.
func randomHexID(n int) string {
	buf := make([]byte, n)
	seed := uint64(time.Now().UnixNano())
	for i := range buf {
		// A cheap xorshift: the id only has to be unique per request.
		seed ^= seed << 13
		seed ^= seed >> 7
		seed ^= seed << 17
		buf[i] = byte(seed)
	}
	return hex.EncodeToString(buf)
}

// warmEdgeTTSAuth fetches the endpoint token in the background at startup.
//
// The token handshake is about 650ms and used to be paid by the first
// utterance of the session, which is exactly the press where the user is
// waiting for sound. Warming it at launch moves that cost off the hot path.
func warmEdgeTTSAuth() {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if _, err := edgeTTSGetAuth(ctx); err != nil {
			log.Printf("fastDictionary: edge tts warmup failed (will retry on demand): %v", err)
			return
		}
		log.Printf("fastDictionary: edge tts token ready")
	}()
}
