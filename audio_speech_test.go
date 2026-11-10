package main

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func audioFuncBody(t *testing.T, src, signature string) string {
	t.Helper()
	start := strings.Index(src, signature)
	if start < 0 {
		t.Fatalf("找不到函数 %s", signature)
	}
	rel := strings.Index(src[start:], "\n}\n")
	if rel < 0 {
		t.Fatalf("函数 %s 没有结束", signature)
	}
	return src[start : start+rel]
}

// TestAudioPlaybackIsGenerationGuarded is the regression: the word fast path
// used to call playMP3 without checking speakGeneration, so a slow Youdao
// fetch could start playing over the word the user clicked next.
func TestAudioPlaybackIsGenerationGuarded(t *testing.T) {
	src := readSourceFile(t, "audio.go")
	body := audioFuncBody(t, src, "func PlaySpeechLang(")
	if countOccurrences(body, "playMP3(path)") != 0 {
		t.Fatal("PlaySpeechLang 仍直接播放未经代次校验的音频")
	}
	if countOccurrences(body, "playSpeechMP3(gen, path)") != 2 {
		t.Fatalf("PlaySpeechLang 的代次校验播放点 = %d，want 2（有道单词与 Edge TTS 两条路径）",
			countOccurrences(body, "playSpeechMP3(gen, path)"))
	}
	if !containsText(src, "func playSpeechMP3(gen int, path string) error") {
		t.Fatal("缺少 playSpeechMP3：检查代次与播放必须在同一把锁内完成")
	}
}

func TestPlaySpeechMP3DropsSuperseded(t *testing.T) {
	oldPlay := playMP3Func
	defer func() { playMP3Func = oldPlay }()
	var played []string
	playMP3Func = func(path string) error {
		played = append(played, path)
		return nil
	}

	speakMu.Lock()
	oldGen := speakGeneration
	speakGeneration = 7
	speakMu.Unlock()
	t.Cleanup(func() {
		speakMu.Lock()
		speakGeneration = oldGen
		speakMu.Unlock()
	})

	if err := playSpeechMP3(7, "newest.mp3"); err != nil {
		t.Fatalf("最新请求应播放: %v", err)
	}
	if err := playSpeechMP3(6, "stale.mp3"); !errors.Is(err, errSpeechSuperseded) {
		t.Fatalf("过期请求应返回 errSpeechSuperseded，得到 %v", err)
	}
	if len(played) != 1 || played[0] != "newest.mp3" {
		t.Fatalf("播放序列 = %v，want [newest.mp3]", played)
	}
}

// TestSpeechPlaybackSerializesWithNewerRequest proves the newer request cannot
// jump ahead of a playback that already holds speakMu: it bumps the generation
// only after the old clip returned, so the old clip finishes first and the new
// one is the last thing heard.
func TestSpeechPlaybackSerializesWithNewerRequest(t *testing.T) {
	oldPlay := playMP3Func
	defer func() { playMP3Func = oldPlay }()

	entered := make(chan struct{}, 1)
	var mu sync.Mutex
	var order []string
	playMP3Func = func(path string) error {
		if path == "old.mp3" {
			entered <- struct{}{}
		}
		time.Sleep(5 * time.Millisecond)
		mu.Lock()
		order = append(order, path)
		mu.Unlock()
		return nil
	}

	speakMu.Lock()
	oldGen := speakGeneration
	speakGeneration = 1
	speakMu.Unlock()
	t.Cleanup(func() {
		speakMu.Lock()
		speakGeneration = oldGen
		speakMu.Unlock()
	})

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = playSpeechMP3(1, "old.mp3")
	}()
	<-entered
	go func() {
		defer wg.Done()
		speakMu.Lock()
		speakGeneration = 2
		speakMu.Unlock()
		_ = playSpeechMP3(2, "new.mp3")
	}()
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if len(order) != 2 || order[0] != "old.mp3" || order[1] != "new.mp3" {
		t.Fatalf("播放顺序 = %v，want [old.mp3 new.mp3]：新请求必须排在已在播的旧请求之后", order)
	}
}
