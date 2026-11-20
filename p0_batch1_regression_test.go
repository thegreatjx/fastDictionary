package main

import (
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ort "github.com/getcharzp/onnxruntime_purego"
)

func TestDestroyOrtOutputsNilSafe(t *testing.T) {
	destroyOrtOutputs(nil)
	destroyOrtOutputs(map[string]*ort.Value{})
	destroyOrtOutputs(map[string]*ort.Value{"out": nil, "ids": nil})
}

func TestCloseIndraOCRIdempotentAndResetsState(t *testing.T) {
	resetIndraForTest(t)

	closeIndraOCR()
	closeIndraOCR()

	indraMu.Lock()
	indraInst = &indraEngine{}
	indraLoaded = true
	indraErr = errors.New("load failed")
	indraFailedAt = time.Now()
	indraMu.Unlock()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			closeIndraOCR()
		}()
	}
	wg.Wait()

	indraMu.Lock()
	inst, loaded, errVal, failedAt := indraInst, indraLoaded, indraErr, indraFailedAt
	indraMu.Unlock()

	if inst != nil || loaded || errVal != nil || !failedAt.IsZero() {
		t.Fatalf("closeIndraOCR did not reset globals: inst=%v loaded=%v err=%v failedAt=%v", inst, loaded, errVal, failedAt)
	}
}

func TestTranslateUntranslatedFallbackDoesNotHoldMutex(t *testing.T) {
	oldRace := translateRaceFunc
	t.Cleanup(func() { translateRaceFunc = oldRace })

	fallbackEntered := make(chan struct{})
	releaseFallback := make(chan struct{})
	var calls atomic.Int32
	translateRaceFunc = func(ctx context.Context, text, from, to, mode string) raceResult {
		n := calls.Add(1)
		if n == 1 {
			if mode != "microsoft" {
				t.Errorf("first call mode = %q, want microsoft", mode)
			}
			return raceResult{Text: text, Engine: "微软", Detected: "en", Untranslated: true}
		}
		if mode != "race" {
			t.Errorf("fallback call mode = %q, want race", mode)
		}
		close(fallbackEntered)
		<-releaseFallback
		return raceResult{Text: "开放人工智能平台", Engine: "谷歌", Detected: "en"}
	}

	a := newTestApp()
	a.cache = newQueryCache("")
	a.store.Update(func(c *Config) {
		c.EngineMode = "microsoft"
		c.DictEnabled = false
		c.ShowNotes = false
		c.AutoPronounce = "off"
	})

	a.translateTextWithReflow("OpenAI platform", false)

	select {
	case <-fallbackEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("untranslated fallback race was not triggered")
	}

	lockAcquired := make(chan struct{})
	go func() {
		a.mu.Lock()
		_ = a.busy
		a.mu.Unlock()
		close(lockAcquired)
	}()

	select {
	case <-lockAcquired:
	case <-time.After(200 * time.Millisecond):
		close(releaseFallback)
		t.Fatal("a.mu is held while untranslated fallback network call is blocked")
	}

	close(releaseFallback)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		a.mu.Lock()
		busy := a.busy
		target := a.target
		engine := a.engine
		echoNote := a.echoNote
		a.mu.Unlock()
		if !busy {
			if target != "开放人工智能平台" || engine != "谷歌" || echoNote == "" {
				t.Fatalf("unexpected fallback result: target=%q engine=%q echoNote=%q", target, engine, echoNote)
			}
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("translation did not finish after releasing fallback")
}

func TestTranslateUntranslatedSkipsFallbackWhenStale(t *testing.T) {
	oldRace := translateRaceFunc
	t.Cleanup(func() { translateRaceFunc = oldRace })

	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	var fallbackCalled atomic.Bool
	translateRaceFunc = func(ctx context.Context, text, from, to, mode string) raceResult {
		if mode == "race" {
			fallbackCalled.Store(true)
			return raceResult{Text: "不应执行", Engine: "谷歌", Detected: "en"}
		}
		close(firstEntered)
		<-releaseFirst
		return raceResult{Text: text, Engine: "微软", Detected: "en", Untranslated: true}
	}

	a := newTestApp()
	a.cache = newQueryCache("")
	a.store.Update(func(c *Config) {
		c.EngineMode = "microsoft"
		c.DictEnabled = false
		c.ShowNotes = false
		c.AutoPronounce = "off"
	})

	a.translateTextWithReflow("OpenAI platform", false)

	select {
	case <-firstEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("initial translate call did not start")
	}

	a.mu.Lock()
	a.translateGen++
	a.mu.Unlock()
	close(releaseFirst)

	time.Sleep(50 * time.Millisecond)
	if fallbackCalled.Load() {
		t.Fatal("stale translation still executed fallback race network call")
	}
}

func writeTinyTestPNG(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "shot.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestVisionRefineStreamThrottlesAndCompletes(t *testing.T) {
	oldStream := streamAIWithImageFunc
	t.Cleanup(func() { streamAIWithImageFunc = oldStream })

	shotPath := writeTinyTestPNG(t)
	a := newTestApp()
	a.store.Update(func(c *Config) {
		c.AIProvider = "custom"
		c.AIURL = "http://127.0.0.1:1"
		c.AIModel = "vision-test"
		c.AIKey = "test-key"
		c.TargetLang = "zh-Hans"
	})
	a.mu.Lock()
	a.isScreenshotMode = true
	a.ocrPath = shotPath
	a.ocrGeneration = 1
	a.source = "old ocr"
	a.mu.Unlock()

	streamErr := make(chan string, 1)
	streamAIWithImageFunc = func(ctx context.Context, conn aiConn, system, user, imageDataURL, effort string, onDelta func(string)) (string, error) {
		full := "[SOURCE]\nHello world\n[TARGET]\n你好世界再见"
		onDelta("[SOURCE]\nHello world\n[TARGET]\n你")
		a.mu.Lock()
		firstTarget := a.target
		a.mu.Unlock()
		if firstTarget != "你" {
			streamErr <- "first delta did not paint immediately: got " + firstTarget
			return full, nil
		}

		for _, ch := range []string{"好", "世", "界"} {
			onDelta(ch)
			a.mu.Lock()
			got := a.target
			a.mu.Unlock()
			if got != "你" {
				streamErr <- "rapid delta bypassed 40ms throttle: got " + got
				return full, nil
			}
		}

		time.Sleep(55 * time.Millisecond)
		onDelta("再")
		a.mu.Lock()
		afterInterval := a.target
		a.mu.Unlock()
		if afterInterval != "你好世界再" {
			streamErr <- "delta after 40ms interval did not paint accumulated text: got " + afterInterval
			return full, nil
		}

		onDelta("见")
		a.mu.Lock()
		trailing := a.target
		a.mu.Unlock()
		if trailing != "你好世界再" {
			streamErr <- "trailing rapid delta bypassed 40ms throttle: got " + trailing
			return full, nil
		}
		return full, nil
	}

	a.visionRefine()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		a.mu.Lock()
		refining := a.aiRefining
		src, dst, errMsg := a.source, a.target, a.aiRefineErr
		a.mu.Unlock()
		if !refining {
			select {
			case msg := <-streamErr:
				t.Fatal(msg)
			default:
			}
			if errMsg != "" {
				t.Fatalf("visionRefine failed: %s", errMsg)
			}
			if src != "Hello world" || dst != "你好世界再见" {
				t.Fatalf("final visionRefine state incomplete: source=%q target=%q", src, dst)
			}
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("visionRefine did not complete")
}

func TestRecognizeScreenshotVisionStreamThrottlesAndCompletes(t *testing.T) {
	oldStream := streamAIWithImageFunc
	t.Cleanup(func() { streamAIWithImageFunc = oldStream })

	shotPath := writeTinyTestPNG(t)
	a := newTestApp()
	a.store.Update(func(c *Config) {
		c.AIProvider = "custom"
		c.AIURL = "http://127.0.0.1:1"
		c.AIModel = "vision-test"
		c.AIKey = "test-key"
		c.TargetLang = "zh-Hans"
	})
	a.mu.Lock()
	a.isScreenshotMode = true
	a.ocrBusy = true
	a.ocrGeneration = 3
	a.mu.Unlock()

	streamAIWithImageFunc = func(ctx context.Context, conn aiConn, system, user, imageDataURL, effort string, onDelta func(string)) (string, error) {
		full := "[SOURCE]\nFast dict\n[TARGET]\n快速词典完成"
		onDelta("[SOURCE]\nFast dict\n[TARGET]\n快")
		a.mu.Lock()
		firstTarget := a.target
		a.mu.Unlock()
		if firstTarget != "快" {
			t.Errorf("first delta did not paint immediately: got %q", firstTarget)
		}

		for _, ch := range []string{"速", "词", "典"} {
			onDelta(ch)
			a.mu.Lock()
			got := a.target
			a.mu.Unlock()
			if got != "快" {
				t.Errorf("rapid delta bypassed 40ms throttle: got %q", got)
			}
		}

		time.Sleep(55 * time.Millisecond)
		onDelta("完")
		a.mu.Lock()
		afterInterval := a.target
		a.mu.Unlock()
		if afterInterval != "快速词典完" {
			t.Errorf("delta after 40ms interval did not paint accumulated text: got %q", afterInterval)
		}

		onDelta("成")
		a.mu.Lock()
		trailing := a.target
		a.mu.Unlock()
		if trailing != "快速词典完" {
			t.Errorf("trailing rapid delta bypassed 40ms throttle: got %q", trailing)
		}
		return full, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	a.recognizeScreenshotVision(ctx, cancel, 3, shotPath, "fallback")

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ocrBusy {
		t.Fatal("ocrBusy still true after recognizeScreenshotVision finished")
	}
	if a.ocrError != "" {
		t.Fatalf("unexpected ocrError: %q", a.ocrError)
	}
	if a.source != "Fast dict" || a.target != "快速词典完成" {
		t.Fatalf("final screenshot vision state incomplete: source=%q target=%q", a.source, a.target)
	}
}
