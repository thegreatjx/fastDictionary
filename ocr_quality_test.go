package main

import (
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPrepareOCRImageScalesForRecognition pins the scaling that makes small
// screen type legible.
//
// Windows.Media.Ocr reads 12px Chinese at roughly 40% character error and 14px
// at 15%; at 2x a real 900x200 crop of Windows-rendered text went from six
// wrong glyphs in fourteen to one, and English from a mangled word to exact.
// The scaling is the whole reason screenshot translation works, so a refactor
// that quietly drops it would be invisible in every other test.
func TestPrepareOCRImageScalesForRecognition(t *testing.T) {
	dir := t.TempDir()
	src := image.NewRGBA(image.Rect(0, 0, 40, 20))
	src.Set(0, 0, color.RGBA{R: 255, A: 255})
	in := filepath.Join(dir, "snip.png")
	writeTestPNG(t, in, src)

	out, cleanup, err := prepareOCRImage(in)
	if err != nil {
		t.Fatalf("prepareOCRImage: %v", err)
	}
	defer cleanup()

	if out == in {
		t.Fatal("prepareOCRImage returned the original, so nothing was scaled")
	}
	scaled, err := decodePNG(out)
	if err != nil {
		t.Fatalf("decode scaled image: %v", err)
	}
	if b := scaled.Bounds(); b.Dx() != 80 || b.Dy() != 40 {
		t.Fatalf("2x scale gave %dx%d, want 80x40", b.Dx(), b.Dy())
	}

	// The preview and the recorded crop bounds both need the original size,
	// so the scaled copy must be a separate file and the original untouched.
	if _, err := os.Stat(in); err != nil {
		t.Fatalf("original was removed: %v", err)
	}
	cleanup()
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("cleanup left the temp file behind: %v", err)
	}
}

// TestPrepareOCRImageRefusesToOverflowEngineLimit covers the crop so large that
// scaling would make the engine throw. A 1x read of a huge image still beats an
// error the user cannot act on.
func TestPrepareOCRImageRefusesToOverflowEngineLimit(t *testing.T) {
	dir := t.TempDir()
	huge := image.NewRGBA(image.Rect(0, 0, maxOCRSide, 10))
	in := filepath.Join(dir, "snip.png")
	writeTestPNG(t, in, huge)

	out, cleanup, err := prepareOCRImage(in)
	if err != nil {
		t.Fatalf("prepareOCRImage: %v", err)
	}
	defer cleanup()
	if out != in {
		t.Fatalf("a crop past MaxImageDimension was scaled anyway: %s", out)
	}
}

func writeTestPNG(t *testing.T, path string, img image.Image) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		t.Fatalf("encode %s: %v", path, err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close %s: %v", path, err)
	}
}

func decodePNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}

// TestCleanOcrTextRemovesCJKSeparation covers the shape Windows.Media.Ocr
// actually produces for Chinese: one word per glyph.
func TestCleanOcrTextRemovesCJKSeparation(t *testing.T) {
	cases := []struct{ in, want string }{
		// The reported bug: 微软翻译 came back as separate glyphs.
		{"微 软 翻 译", "微软翻译"},
		{"点 击 这 里 开 始", "点击这里开始"},
		// A space beside Latin text is a real word break and must survive.
		{"打开 Chrome 浏览器", "打开 Chrome 浏览器"},
		{"Hello world 你好 世界", "Hello world 你好世界"},
		// Full-width punctuation stays put, and the space next to it goes
		// only when both neighbours are CJK.
		{"你 好 ， 世 界 。", "你好，世界。"},
		// Mixed line: cleaning one line must not touch the next.
		{"微 软 翻 译\nHello world", "微软翻译\nHello world"},
	}
	for _, c := range cases {
		if got := CleanOcrText(c.in); got != c.want {
			t.Errorf("CleanOcrText(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestCleanOcrTextKeepsLineStructure guards the reason CleanOcrText exists
// separately from CleanSelectionText: a screenshot of a paragraph has to stay
// a paragraph.
func TestCleanOcrTextKeepsLineStructure(t *testing.T) {
	in := "第一行\n第二行\n\n第四段"
	if got := CleanOcrText(in); got != in {
		t.Fatalf("CleanOcrText collapsed the lines: %q, want %q", got, in)
	}
	// The contrast: the selection cleaner is allowed to fold them.
	if got := CleanSelectionText(in); !strings.Contains(got, "\n") {
		t.Logf("CleanSelectionText kept newlines too: %q", got)
	}
}

// TestStreamMagpieOmitsReasoningWhenEffortNone is the regression test for the
// 2.3x slowdown.
//
// The gateway validates reasoning_effort against each model's own
// reasoning_levels and silently discards a value the model does not list,
// after which the model applies its default reasoning. Sending a guessed level
// is therefore worse than sending nothing: on gemini-3.8-flash, omitting the
// field costs 8.93s and 743 reasoning tokens, while asking for its real "low"
// level costs 2.58s and none. An empty effort must carry no reasoning key.
func TestStreamMagpieOmitsReasoningWhenEffortNone(t *testing.T) {
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"))
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	run := func(effort string) map[string]any {
		bodies = nil
		if _, err := StreamMagpie(context.Background(), srv.URL, "m", "s", "u", effort, nil); err != nil {
			t.Fatalf("StreamMagpie(%q): %v", effort, err)
		}
		if len(bodies) != 1 {
			t.Fatalf("StreamMagpie(%q) sent %d requests, want 1", effort, len(bodies))
		}
		return bodies[0]
	}

	// "" is what ResolveEffort returns when the model's levels are unknown.
	// The legacy spellings are still accepted so an old config or a saved
	// custom action cannot reintroduce the guessed-value bug.
	for _, effort := range []string{"", "off", "none"} {
		body := run(effort)
		if _, ok := body["reasoning_effort"]; ok {
			t.Errorf("effort %q sent reasoning_effort=%v; it must be omitted, "+
				"because a value the model does not list makes the gateway apply "+
				"the model's default reasoning instead of none",
				effort, body["reasoning_effort"])
		}
		if _, ok := body["reasoning"]; ok {
			t.Errorf("effort %q sent reasoning=%v; it must be omitted", effort, body["reasoning"])
		}
	}

	// A real level must still be sent, both ways: the gateway reads the flat
	// field and the nested object, and older paths depend on each.
	for _, effort := range []string{EffortDeep, "low", "minimal"} {
		body := run(effort)
		if body["reasoning_effort"] != effort {
			t.Errorf("effort %q sent reasoning_effort=%v", effort, body["reasoning_effort"])
		}
		reasoning, ok := body["reasoning"].(map[string]any)
		if !ok || reasoning["effort"] != effort {
			t.Errorf("effort %q sent reasoning=%v, want effort=%q", effort, body["reasoning"], effort)
		}
	}
}

// TestRunActionDoesNotDuplicateSelection guards the doubled input tokens.
//
// The action prompt used to be formatted with the selection in both
// {{selection}} and {{paragraphs}}, so the model received the same sentence
// twice under two headings. It could not tell which one was context.
func TestRunActionDoesNotDuplicateSelection(t *testing.T) {
	tpl := "## Input\nSelection: {{selection}}\nParagraphs: {{paragraphs}}\nTarget: {{targetLanguage}}"
	got := FormatActionPrompt(tpl, "量价背离是技术分析术语", "", "简体中文")
	if strings.Count(got, "量价背离是技术分析术语") != 1 {
		t.Fatalf("selection appears %d times, want 1:\n%s",
			strings.Count(got, "量价背离是技术分析术语"), got)
	}
	if !strings.Contains(got, "Paragraphs: \n") {
		t.Fatalf("{{paragraphs}} should expand to an empty field, got:\n%s", got)
	}
}
