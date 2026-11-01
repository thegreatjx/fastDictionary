// Audio playback via winmm.dll MCI (zero external dependencies).
// Verified on this machine: open/play youdao mp3 err=0, no mpv/ffmpeg needed.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	winmm       = syscall.NewLazyDLL("winmm.dll")
	procMCISend = winmm.NewProc("mciSendStringW")
	audioMu     sync.Mutex
	curAlias    string
)

func mciCall(cmd string) error {
	cmdPtr, err := syscall.UTF16PtrFromString(cmd)
	if err != nil {
		return err
	}
	ret, _, _ := procMCISend.Call(uintptr(unsafe.Pointer(cmdPtr)), 0, 0, 0)
	if ret != 0 {
		return fmt.Errorf("mci %q: code %d", cmd, ret)
	}
	return nil
}

// playbackVolume is the volume MCI applies, on its 0-1000 scale.
//
// The config's audio_volume was written and persisted but never read, so the
// slider it was meant for did nothing. It is read here on every play, so a
// change in the settings window takes effect on the next utterance without a
// restart.
func playbackVolume() int {
	v := GetConfigVolume()
	if v <= 0 {
		v = 80
	}
	if v > 100 {
		v = 100
	}
	return v * 10
}

// playMP3 plays an mp3 file from disk, stopping whatever plays now.
func playMP3(path string) error {
	audioMu.Lock()
	defer audioMu.Unlock()
	if curAlias != "" {
		mciCall("stop " + curAlias)
		mciCall("close " + curAlias)
		curAlias = ""
	}
	const alias = "fastDictionary_snd"
	if err := mciCall(fmt.Sprintf("open \"%s\" alias %s type mpegvideo", path, alias)); err != nil {
		return err
	}
	curAlias = alias
	// setaudio is a device property; it has to be applied before play.
	mciCall(fmt.Sprintf("setaudio %s volume to %d", alias, playbackVolume()))
	return mciCall("play " + alias)
}

// stopAudio halts current playback (called on window hide).
//
// It stops both players: MCI for the mp3s and the SAPI child process for the
// fallback voice. Bumping the generation makes a synthesis that is still in
// flight drop its result instead of starting to speak after the window is
// gone.
func stopAudio() {
	speakMu.Lock()
	speakGeneration++
	cmd := speakCmd
	speakCmd = nil
	speakMu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}

	audioMu.Lock()
	defer audioMu.Unlock()
	if curAlias != "" {
		mciCall("stop " + curAlias)
		mciCall("close " + curAlias)
		curAlias = ""
	}
}

// speakGeneration guards against a slow synthesis finishing after the user
// already asked for something else: every PlaySpeech bumps the generation,
// and a result from an older one is dropped instead of played over the new
// one. It also lets stopAudio silence a SAPI utterance, which runs in a
// child process and cannot be stopped through MCI.
var (
	speakMu         sync.Mutex
	speakGeneration int
	speakCmd        *exec.Cmd
)

// PlaySpeech speaks text: a single English word uses Youdao's studio
// recording, everything else goes through Edge TTS (neural voices), and
// Windows SAPI is kept only as the last resort when the network fails.
// PlaySpeech speaks text with the language guessed from its script.
//
// Prefer PlaySpeechLang: a guess is what made French read aloud by an English
// voice. This stays for the call sites that only have the text.
func PlaySpeech(text string, us bool) {
	PlaySpeechLang(text, "", us)
}

// PlaySpeechLang speaks text in a named language.
//
// lang is a BCP-47 tag. When it is empty the language is settled here, before
// synthesis: the script, then a two-word English test, then the engine's own
// detection. That order matters — an empty tag used to fall straight through to
// en-US-AriaNeural, so French and German were read by an English voice, which
// is the robotic sound the user hears.
func PlaySpeechLang(text, lang string, us bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	speakMu.Lock()
	speakGeneration++
	gen := speakGeneration
	prev := speakCmd
	speakCmd = nil
	speakMu.Unlock()
	if prev != nil && prev.Process != nil {
		_ = prev.Process.Kill()
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		if isWord(text) {
			shortCtx, shortCancel := context.WithTimeout(ctx, 5*time.Second)
			path, err := FetchYoudaoAudio(shortCtx, text, us)
			shortCancel()
			if err == nil && path != "" {
				if err := playMP3(path); err == nil {
					return
				}
			}
		}

		speechLang := resolveSpeechLang(ctx, text, lang)
		voice := edgeTTSVoice(speechLang, us, "")
		path, err := FetchEdgeTTS(ctx, text, voice)
		if err == nil && path != "" {
			speakMu.Lock()
			current := gen == speakGeneration
			speakMu.Unlock()
			if !current {
				return
			}
			if err := playMP3(path); err == nil {
				return
			}
		}

		// Network failed: fall back to the offline voice rather than silence,
		// in the same language. Handing the text to SAPI without a language
		// let it use the default voice, which on a Chinese Windows is a Chinese
		// voice reading Japanese.
		speakMu.Lock()
		current := gen == speakGeneration
		speakMu.Unlock()
		if current {
			speakWithSAPI(text, speechLang, gen)
		}
	}()
}

// resolveSpeechLang settles the language to read text in.
func resolveSpeechLang(ctx context.Context, text, lang string) string {
	if tag := strings.TrimSpace(lang); tag != "" {
		return tag
	}
	if tag := detectLocal(text); tag != "" {
		return tag
	}
	if looksEnglish(text) {
		return "en"
	}
	// Latin script that the local tests cannot settle: French, German, Dutch,
	// Polish. Ask the engine, which recognises a language from one short
	// request; without this the text would be read by an English voice.
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if r, err := MicrosoftTranslate(probeCtx, text, "auto", "zh-Hans"); err == nil && r.Detected != "" {
		return r.Detected
	}
	return "en"
}

// speakWithSAPI reads text with the Windows voice when Edge TTS is unreachable.
//
// Three things matter here, all learned from a real machine:
//
//   - SAPI's Voice.Language is empty on Windows 11; the language only appears
//     in the voice id ("TTS_MS_ZH-CN_HUIHUI_11.0"), so that is what is matched.
//   - The installed set is tiny (Chinese and two English voices here), so a
//     language usually has no voice at all. Falling back to the default would
//     read Japanese with a Chinese voice, which is worse than useless.
//   - SAPI reads non-Latin scripts badly or not at all. When no matching voice
//     exists, silence plus a log line is more honest than wrong-sounding
//     gibberish.
func speakWithSAPI(text, lang string, gen int) {
	escaped := strings.ReplaceAll(text, "'", "''")
	escaped = strings.ReplaceAll(escaped, "\n", " ")
	escaped = strings.ReplaceAll(escaped, "\r", " ")
	script := fmt.Sprintf(`$s = New-Object -ComObject SAPI.SpVoice
$want = '%s'
$hit = $null
if ($want) {
  $hit = $s.GetVoices() | Where-Object { $_.Id -match "(?i)(^|[_\-])$want([_\-]|$)" } | Select-Object -First 1
} else {
  $hit = $s.GetVoices() | Select-Object -First 1
}
if (-not $hit) { exit 3 }
$s.Voice = $hit
$s.Speak('%s')`, escapePowerShell(langPrefix(lang)), escaped)
	c := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := c.Start(); err != nil {
		return
	}
	speakMu.Lock()
	if gen != speakGeneration {
		speakMu.Unlock()
		_ = c.Process.Kill()
		return
	}
	speakCmd = c
	speakMu.Unlock()
	go func() { _ = c.Wait() }()
}

// escapePowerShell escapes a value for the single-quoted string above.
func escapePowerShell(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// ---------------------------------------------------------------------------
// Youdao: dictionary JSON (phonetics UK/US) + dictvoice mp3 (word audio).
// Both free, no key, fast from CN (103ms / 108ms measured 2026-10-10).
// ---------------------------------------------------------------------------

// YoudaoDict is the parsed dictionary data for a word.
type YoudaoDict struct {
	UKPhone  string
	USPhone  string
	Explains []string // e.g. "n. 意外发现美好事物的运气"
}

var youdaoDictsParam = url.QueryEscape(`{"count":99,"dicts":[["ec","simple"]]}`)

// LookupYoudao fetches phonetics and brief explanations for an English word.
func LookupYoudao(ctx context.Context, word string) (*YoudaoDict, error) {
	endpoint := "https://dict.youdao.com/jsonapi?q=" + url.QueryEscape(word) + "&dicts=" + youdaoDictsParam
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
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
		return nil, fmt.Errorf("youdao dict: http %d", resp.StatusCode)
	}
	var raw struct {
		Ec struct {
			Word []struct {
				UKPhone string `json:"ukphone"`
				USPhone string `json:"usphone"`
				Trs     []struct {
					Tr []struct {
						L struct {
							I any `json:"i"`
						} `json:"l"`
					} `json:"tr"`
				} `json:"trs"`
			} `json:"word"`
		} `json:"ec"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	if len(raw.Ec.Word) == 0 {
		return nil, fmt.Errorf("youdao dict: no entry")
	}
	w := raw.Ec.Word[0]
	d := &YoudaoDict{UKPhone: w.UKPhone, USPhone: w.USPhone}
	for _, tr := range w.Trs {
		for _, t := range tr.Tr {
			switch v := t.L.I.(type) {
			case string:
				d.Explains = append(d.Explains, v)
			case []any:
				var sb []string
				for _, item := range v {
					if s, ok := item.(string); ok {
						sb = append(sb, s)
					}
				}
				d.Explains = append(d.Explains, strings.Join(sb, ""))
			}
		}
	}
	return d, nil
}

// sanitizeFileName cleans a word for safe filesystem storage.
func sanitizeFileName(s string) string {
	s = strings.TrimSpace(s)
	var sb strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('_')
		}
	}
	return sb.String()
}

// FetchYoudaoAudio downloads the pronunciation mp3 for a word (type: 1=UK, 2=US)
// into the cache dir and returns the local path.
func FetchYoudaoAudio(ctx context.Context, word string, us bool) (string, error) {
	dir := filepath.Join(os.TempDir(), "fastDictionary_audio")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	t, suffix := "1", "uk"
	if us {
		t, suffix = "2", "us"
	}
	path := filepath.Join(dir, sanitizeFileName(word)+"_"+suffix+".mp3")
	if fi, err := os.Stat(path); err == nil && fi.Size() > 1024 {
		return path, nil // cached
	}
	endpoint := "https://dict.youdao.com/dictvoice?audio=" + url.QueryEscape(word) + "&type=" + t
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("youdao audio: http %d", resp.StatusCode)
	}
	// Write through a .part file: a crash mid-download used to leave a
	// truncated mp3 behind that the size check above then treated as a
	// valid cache hit, playing garbage forever after.
	tmp := path + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return "", err
	}
	return path, nil
}
