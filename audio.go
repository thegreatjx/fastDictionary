package main

import (
	"context"
	"encoding/json"
	"errors"
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
	mciCall(fmt.Sprintf("setaudio %s volume to %d", alias, playbackVolume()))
	return mciCall("play " + alias)
}

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

var errSpeechSuperseded = errors.New("speech request superseded")

var playMP3Func = playMP3

func playSpeechMP3(gen int, path string) error {
	speakMu.Lock()
	defer speakMu.Unlock()
	if gen != speakGeneration {
		return errSpeechSuperseded
	}
	return playMP3Func(path)
}

var (
	speakMu         sync.Mutex
	speakGeneration int
	speakCmd        *exec.Cmd
)

func PlaySpeech(text string, us bool) {
	PlaySpeechLang(text, "", us)
}

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
		defer recoverLog("audio.go:PlaySpeechLang")
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		if isWord(text) {
			shortCtx, shortCancel := context.WithTimeout(ctx, 5*time.Second)
			path, err := FetchYoudaoAudio(shortCtx, text, us)
			shortCancel()
			if err == nil && path != "" {
				playErr := playSpeechMP3(gen, path)
				if playErr == nil || errors.Is(playErr, errSpeechSuperseded) {
					return
				}
			}
		}

		speechLang := resolveSpeechLang(ctx, text, lang)
		voice := edgeTTSVoice(speechLang, us, "")
		path, err := FetchEdgeTTS(ctx, text, voice)
		if err == nil && path != "" {
			playErr := playSpeechMP3(gen, path)
			if playErr == nil || errors.Is(playErr, errSpeechSuperseded) {
				return
			}
		}

		speakMu.Lock()
		current := gen == speakGeneration
		speakMu.Unlock()
		if current {
			speakWithSAPI(text, speechLang, gen)
		}
	}()
}

func resolveSpeechLang(ctx context.Context, text, lang string) string {
	if tag := strings.TrimSpace(lang); tag != "" {
		return tag
	}
	if tag := sourceLangFor(text); tag != "" {
		return tag
	}
	if looksEnglish(text) {
		return "en"
	}
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if r, err := MicrosoftTranslate(probeCtx, text, "auto", "zh-Hans"); err == nil && r.Detected != "" {
		return r.Detected
	}
	return "en"
}

func speakWithSAPI(text, lang string, gen int) {
	defer recoverLog("audio.go:speakWithSAPI")
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
		go func() {
			defer recoverLog("audio.go:speakWithSAPI.wait")
			_ = c.Wait()
		}()
		return
	}
	speakCmd = c
	speakMu.Unlock()
	go func() {
		defer recoverLog("audio.go:speakWithSAPI.wait")
		_ = c.Wait()
	}()
}

func escapePowerShell(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

type YoudaoDict struct {
	UKPhone  string
	USPhone  string
	Explains []string
}

var youdaoDictsParam = url.QueryEscape(`{"count":99,"dicts":[["ec","simple"]]}`)

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
		return path, nil
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
