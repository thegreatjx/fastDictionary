package main

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	ort "github.com/getcharzp/onnxruntime_purego"
)

// The Indra OCR engine.
//
// Why this exists: Windows.Media.Ocr misreads exactly the text a screenshot
// translator meets most — stylised headings, condensed type, mixed sizes. On the
// public "Plaid" OCR torture image it turned "CENTER FOR THE" into
// "C E N T £ R F 0 R T H £", "Fridays & Saturdays" into "Fridays U Saturdays"
// and "Music Theatre International, NY" into "Music Theatre № tern 0 al". Indra
// reads all three correctly, in 197ms instead of 580ms.
//
// It runs through onnxruntime, which has no cgo binding here: purego cannot
// dlopen on Windows, so the library is loaded with syscall.LoadLibrary by the
// ort package, and every OrtApi call is a Go function pointer taken from the
// library's own function table.
//
// Everything is optional. If the runtime or the model files are missing, OCR
// falls back to Windows.Media.Ocr rather than failing: a translator that cannot
// read a screenshot at all is worse than one that reads it imperfectly.

const (
	// indraModelDirEnv points at the model directory, for development.
	indraModelDirEnv = "FASTDICTIONARY_INDRA_DIR"
	// indraRuntimeEnv points at onnxruntime.dll, for development.
	indraRuntimeEnv = "FASTDICTIONARY_ORT_DLL"
	// indraDevDirEnv set to "0" turns off the development-only model lookup.
	indraDevDirEnv = "FASTDICTIONARY_INDRA_DEV"
	// indraMaxLines caps a page, so a screenshot of a whole article does not
	// turn into a hundred recognizer calls.
	indraMaxLines = 64
	// indraMinLineScore drops boxes the detector was unsure about.
	indraMinLineScore = 0.35
	// indraMinLineHeight and indraMinLineWidth drop boxes too small to be a
	// line of text. A single character is about 12px at screen resolution;
	// anything below that is a speck the detector found in a border or an icon.
	indraMinLineHeight = 8
	indraMinLineWidth  = 12
)

// indraEngine is the loaded models plus their runtime. One is shared by the
// whole process: creating a session costs 30-50ms each and they hold memory.
type indraEngine struct {
	eng      *ort.Engine
	opts     *ort.SessionOptions
	det      *ort.Session
	recCJK   *ort.Session
	recLatin *ort.Session
	script   *ort.Session
	window   int

	// vocabCJK and vocabLatin map recognizer output ids to characters.
	vocabCJK   []string
	vocabLatin []string

	// closeOnce guards Destroy: the engine outlives many OCR calls and is only
	// torn down when the app exits.
	closeOnce sync.Once
}

var (
	indraMu       sync.Mutex
	indraInst     *indraEngine
	indraErr      error
	indraLoaded   bool
	indraFailedAt time.Time
)

// indraRetryCooldown keeps a broken install from paying the full model-load
// cost on every OCR request while still letting a transient failure recover
// without restarting the app.
const indraRetryCooldown = 30 * time.Second

// indraLoadEngine is a variable so the retry policy can be tested without
// loading the real ONNX models.
var indraLoadEngine = indraLoad

// indraVocab is the shape of the vocabulary JSON in meta.json.
type indraVocab struct {
	Blank int      `json:"blank"`
	Chars []string `json:"chars"`
}

// indraMeta is the subset of meta.json this app needs.
type indraMeta struct {
	Vocab     json.RawMessage `json:"vocab"`
	VocabCJK  json.RawMessage `json:"vocab_cjk"`
	RecWindow int             `json:"rec_window"`
}

// indraLookupDirs lists where the model files may be, most specific first.
func indraLookupDirs() []string {
	var out []string
	if d := strings.TrimSpace(os.Getenv(indraModelDirEnv)); d != "" {
		out = append(out, d)
	}
	// Shipped beside the executable.
	if exe, err := os.Executable(); err == nil {
		base := filepath.Dir(exe)
		out = append(out, filepath.Join(base, "indra-models"),
			filepath.Join(base, "models", "indra"))
	}
	return out
}

// indraDevDirs is the development fallback, tried only after the real
// locations, and only when the binary is not a shipped build.
//
// A relative path resolves against wherever the process was started, so
// searching it first would make the model directory depend on how the app was
// launched: a shortcut in another folder would find a different set of models,
// or none. Keeping it last means a shipped app never depends on it.
func indraDevDirs() []string {
	return []string{filepath.Join("build", "indra-models"), "indra-models"}
}

// indraResolve finds the model directory and the runtime library.
//
// The environment variables come first so a developer can point at a checkout,
// and they are also what the tests use to simulate a machine with nothing
// installed. The executable's own directory is the shipping location. The
// working directory is deliberately *not* searched unless the variables say so:
// a relative path resolves against wherever the process was started, so a
// shortcut in another folder would silently find a different model directory —
// or none — and the failure would depend on how the app was launched.
func indraResolve() ([]string, string) {
	var dirs []string
	if d := strings.TrimSpace(os.Getenv(indraModelDirEnv)); d != "" {
		dirs = append(dirs, d)
	}
	if exe, err := os.Executable(); err == nil {
		base := filepath.Dir(exe)
		dirs = append(dirs, filepath.Join(base, "indra-models"),
			filepath.Join(base, "models", "indra"))
	}
	return dirs, ""
}

// indraRuntimeCandidates lists where onnxruntime.dll may be, most specific
// first.
//
// Order matters more than it looks. A bare "onnxruntime.dll" is resolved by
// Windows against its own search path, which reaches System32 first, and the
// copy Windows ships is old: this machine has 1.17.1, which does not expose the
// API version the ort package asks for (23) and fails outright. The absolute
// paths here are tried in order, and the system copy is last so it is only used
// when nothing else exists.
func indraRuntimeCandidates() []string {
	var out []string
	if d := strings.TrimSpace(os.Getenv(indraRuntimeEnv)); d != "" {
		out = append(out, d)
	}
	if exe, err := os.Executable(); err == nil {
		out = append(out, filepath.Join(filepath.Dir(exe), "onnxruntime.dll"))
	}
	// Development: the working directory, for a checkout that has not been
	// packaged yet.
	if wd, err := os.Getwd(); err == nil {
		out = append(out, filepath.Join(wd, "onnxruntime.dll"))
	}
	// A local Python onnxruntime keeps a current copy here.
	if home, err := os.UserHomeDir(); err == nil {
		out = append(out,
			filepath.Join(home, "AppData", "Local", "Programs", "Python", "Python312",
				"Lib", "site-packages", "onnxruntime", "capi", "onnxruntime.dll"))
	}
	if root := os.Getenv("SystemRoot"); root != "" {
		out = append(out, filepath.Join(root, "System32", "onnxruntime.dll"))
	}
	return out
}

// indraGet loads the engine once, or returns why it could not be loaded.
// A failed load is retried after indraRetryCooldown instead of being cached
// for the lifetime of the process.
func indraGet() (*indraEngine, error) {
	indraMu.Lock()
	defer indraMu.Unlock()
	if indraLoaded && indraErr == nil {
		return indraInst, nil
	}
	if indraLoaded && indraErr != nil && time.Since(indraFailedAt) < indraRetryCooldown {
		return indraInst, indraErr
	}
	indraInst, indraErr = indraLoadEngine()
	indraLoaded = true
	if indraErr != nil {
		indraFailedAt = time.Now()
		log.Printf("fastDictionary: Indra OCR unavailable, using Windows OCR: %v", indraErr)
	} else {
		indraFailedAt = time.Time{}
	}
	return indraInst, indraErr
}

// indraLoad opens the runtime and the three models this app uses.
func indraLoad() (*indraEngine, error) {
	modelDir := ""
	candidates := indraLookupDirs()
	// Development fallback last, and only when nothing shipped was found.
	// A test, or a user with broken models, can switch the development lookup
	// off entirely by setting the variable to "0".
	if !isShippedBuild() && !strings.EqualFold(os.Getenv(indraDevDirEnv), "0") {
		candidates = append(candidates, indraDevDirs()...)
	}
	for _, d := range candidates {
		if _, err := os.Stat(filepath.Join(d, "det.onnx")); err == nil {
			modelDir = d
			break
		}
	}
	if modelDir == "" {
		return nil, fmt.Errorf("找不到模型文件（det.onnx），见 %s", indraModelDirEnv)
	}

	runtimePath := ""
	for _, p := range indraRuntimeCandidates() {
		if _, err := os.Stat(p); err == nil {
			runtimePath = p
			break
		}
	}
	if runtimePath == "" {
		return nil, fmt.Errorf("找不到 onnxruntime.dll，见 %s", indraRuntimeEnv)
	}

	eng, err := ort.NewEngine(runtimePath)
	if err != nil {
		return nil, fmt.Errorf("加载 onnxruntime 失败：%w", err)
	}
	opts, err := eng.NewSessionOptions()
	if err != nil {
		eng.Destroy()
		return nil, err
	}
	// The models are small (a 1M-parameter recognizer), so a thread pool per
	// session costs more in wakeups than it saves in arithmetic: on an 800x300
	// screenshot, four threads measured 107ms and twelve measured 500ms-1s for
	// the same work, because the detector finishes each tiny op before the
	// others in the pool wake up. Four is enough to keep the cores busy on the
	// one genuinely parallel step (the detector's convolutions).
	_ = opts.SetIntraOpNumThreads(4)

	e := &indraEngine{eng: eng, opts: opts}
	open := func(name string) (*ort.Session, error) {
		t := time.Now()
		s, err := eng.NewSession(filepath.Join(modelDir, name), opts)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		log.Printf("fastDictionary: indra %s ready in %v", name, time.Since(t).Round(time.Millisecond))
		return s, nil
	}

	// The CJK recognizer is required; the Latin one improves non-CJK text but
	// the app still works without it.
	if e.det, err = open("det.onnx"); err != nil {
		e.Destroy()
		return nil, err
	}
	if e.recCJK, err = open("rec_cjk.onnx"); err != nil {
		e.Destroy()
		return nil, err
	}
	if s, err := open("rec.onnx"); err == nil {
		e.recLatin = s
	}
	if s, err := open("script.onnx"); err == nil {
		e.script = s
	}

	if err := e.loadVocab(modelDir); err != nil {
		e.Destroy()
		return nil, err
	}
	return e, nil
}

// loadVocab reads the character tables out of meta.json.
func (e *indraEngine) loadVocab(modelDir string) error {
	raw, err := os.ReadFile(filepath.Join(modelDir, "meta.json"))
	if err != nil {
		return fmt.Errorf("读取 meta.json 失败：%w", err)
	}
	var m indraMeta
	if err := json.Unmarshal(raw, &m); err != nil {
		return fmt.Errorf("解析 meta.json 失败：%w", err)
	}
	// The vocab entries are JSON objects stored as strings.
	parse := func(field json.RawMessage) ([]string, error) {
		if len(field) == 0 {
			return nil, nil
		}
		var asString string
		if err := json.Unmarshal(field, &asString); err == nil {
			var v indraVocab
			if err := json.Unmarshal([]byte(asString), &v); err != nil {
				return nil, err
			}
			return v.Chars, nil
		}
		var v indraVocab
		if err := json.Unmarshal(field, &v); err != nil {
			return nil, err
		}
		return v.Chars, nil
	}
	if e.vocabCJK, err = parse(m.VocabCJK); err != nil {
		return fmt.Errorf("解析中文词表失败：%w", err)
	}
	if e.vocabLatin, err = parse(m.Vocab); err != nil {
		// A missing Latin table is not fatal: CJK recognition still works.
		e.vocabLatin = nil
	}
	if len(e.vocabCJK) == 0 {
		return fmt.Errorf("中文词表为空")
	}
	// The Latin recognizer reads a window of several line heights; without it
	// its input would be 32 pixels high where the model declares 96.
	e.window = m.RecWindow
	if e.window < 1 {
		e.window = 1
	}
	return nil
}

// Destroy releases the sessions and the runtime.
func (e *indraEngine) Destroy() {
	e.closeOnce.Do(func() {
		for _, s := range []*ort.Session{e.det, e.recCJK, e.recLatin, e.script} {
			if s != nil {
				s.Destroy()
			}
		}
		if e.opts != nil {
			e.opts.Destroy()
		}
		if e.eng != nil {
			e.eng.Destroy()
		}
	})
}

// indraRecognize reads text from an entire image.
func indraRecognize(ctx context.Context, img image.Image) (string, error) {
	e, err := indraGet()
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	page := toGray(img)
	if page.W < 2 || page.H < 2 {
		return "", fmt.Errorf("图片太小")
	}

	boxes, err := e.detect(ctx, page)
	if err != nil {
		return "", err
	}
	if len(boxes) == 0 {
		return "", nil
	}
	sortBoxesByY(boxes)
	if len(boxes) > indraMaxLines {
		boxes = boxes[:indraMaxLines]
	}

	var lines []string
	for _, b := range boxes {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		txt, err := e.readLine(page, b)
		if err != nil || strings.TrimSpace(txt) == "" {
			continue
		}
		lines = append(lines, txt)
	}
	return strings.Join(lines, "\n"), nil
}

// detect runs the line detector and returns boxes in page pixels.
func (e *indraEngine) detect(ctx context.Context, page grayImage) ([]lineBox, error) {
	ink, ph, pw, scale := detPageToInput(page)

	in, err := ort.NewTensor([]int64{1, 1, int64(ph), int64(pw)}, ink)
	if err != nil {
		return nil, err
	}
	defer in.Destroy()

	out, err := e.det.Run(map[string]*ort.Value{"x": in})
	if err != nil {
		return nil, fmt.Errorf("检测失败：%w", err)
	}
	v, ok := out["out"]
	if !ok {
		return nil, fmt.Errorf("检测输出缺失")
	}
	shape, err := v.GetShape()
	if err != nil || len(shape) != 4 {
		return nil, fmt.Errorf("检测输出形状异常")
	}
	h, w := int(shape[2]), int(shape[3])
	data, err := ort.GetTensorData[float32](v)
	if err != nil {
		return nil, err
	}
	boxes := detDecode(data, h, w, scale)
	kept := boxes[:0]
	for _, b := range boxes {
		if b.Score < indraMinLineScore {
			continue
		}
		// Drop specks. The detector sometimes isolates a single digit or a
		// scrap of a larger heading, and one stray "2" ahead of the real text
		// is worse than nothing: it reaches the translator as its own line and
		// comes back as a line of its own.
		if b.Y1-b.Y0 < indraMinLineHeight || b.X1-b.X0 < indraMinLineWidth {
			continue
		}
		kept = append(kept, b)
	}
	return dropNestedBoxes(kept), nil
}

// dropNestedBoxes removes boxes that another box already covers.
//
// The detector works on a half-resolution core band, so a character inside a
// dark banner can come out as its own component: on the "January 6-29" heading
// the "2" of "-29" was boxed separately from the words around it, and the
// recognizer read it as a line of its own — a stray "2" above the real text,
// which the translator then turned into a stray line of its own.
//
// Containment is measured by overlap rather than by strict nesting, because the
// two boxes are rarely nested exactly: the stray "2" started four pixels above
// the line's own top, so a strict "inside" test missed it. A box that is mostly
// covered by a larger one adds nothing the larger one does not already say.
func dropNestedBoxes(boxes []lineBox) []lineBox {
	if len(boxes) < 2 {
		return boxes
	}
	keep := boxes[:0]
	for i, b := range boxes {
		covered := false
		for j, other := range boxes {
			if i == j {
				continue
			}
			bigger := (other.X1-other.X0)*(other.Y1-other.Y0) > (b.X1-b.X0)*(b.Y1-b.Y0)
			if !bigger {
				continue
			}
			ox := math.Min(b.X1, other.X1) - math.Max(b.X0, other.X0)
			oy := math.Min(b.Y1, other.Y1) - math.Max(b.Y0, other.Y0)
			if ox <= 0 || oy <= 0 {
				continue
			}
			area := (b.X1 - b.X0) * (b.Y1 - b.Y0)
			if area > 0 && (ox*oy)/area > 0.6 {
				covered = true
				break
			}
		}
		if !covered {
			keep = append(keep, b)
		}
	}
	return keep
}

// readLine recognises one line box.
//
// Which recognizer reads the line is decided by script.onnx, not by guessing
// from ink density: the CJK model reads Latin text as approximate shapes
// ("Fridays & Saturdays" came back as "FTiuavs O Satirdavs"), so the choice
// matters as much as the crop does.
//
// The two recognizers take different crop heights — rec_cjk.onnx declares
// [B,1,32,W] and rec.onnx [B,1,96,W] — so the line is cropped a second time at
// the chosen height. Feeding a 32-high crop to the Latin model fails outright
// with an invalid-dimensions error, which is how this was found.
// indraLatinMinHeight is the line height, in source pixels, above which the CJK
// recognizer reads a Latin line better than the Latin one.
//
// Both models are trained on 32-pixel line bands, so a taller line has to be
// scaled down harder, and the two handle that differently. Measured on printed
// English:
//
//	line height 70px  cjk "Nasdaq & AMEX"      latin "Nasddaq && AMEX"
//	line height 26px  cjk "Sundday Maatinees"  latin "Sunday Matinees"
//	line height 28px  cjk "Fridays O Saturddays" latin "Fridays & Saturdays"
//
// The Latin model wants a 96-pixel window (rec_window 3 x 32) and its upscaling
// turns a small, clean line into doubled strokes, while on a large heading it
// loses the ampersand entirely. 40px sits between the two behaviours with room
// on both sides.
const indraLatinMinHeight = 40

func (e *indraEngine) readLine(page grayImage, box lineBox) (string, error) {
	sess := e.recCJK
	// The CJK recognizer reads the line band only: [B,1,32,W].
	height, window, vocab := indraLineHeight, 1, e.vocabCJK

	if e.script != nil && e.recLatin != nil && len(e.vocabLatin) > 0 {
		if !preferCJKRecognizer(e.classifyScript(page, box), box.Y1-box.Y0) {
			sess, height, vocab, window = e.recLatin, indraLatinHeight, e.vocabLatin, maxInt(1, e.window)
		}
	}

	ink, width := cropLine(page, box, indraLineHeight, window)
	return e.runRecognizer(sess, vocab, ink, width, height)
}

// classifyScript asks script.onnx which writing system a line uses.
//
// When the classifier is not loaded, or its answer is not confident, the line is
// treated as CJK: the CJK recognizer also carries the ASCII characters, so it is
// the safer default, while the Latin recognizer cannot read Chinese at all.
func (e *indraEngine) classifyScript(page grayImage, box lineBox) scriptKind {
	ink, width := cropLine(page, box, indraLineHeight, 1)
	in, err := ort.NewTensor([]int64{1, 1, indraLineHeight, int64(width)}, ink)
	if err != nil {
		return scriptCJK
	}
	defer in.Destroy()
	widths, err := ort.NewTensor([]int64{1}, []int64{int64(width)})
	if err != nil {
		return scriptCJK
	}
	defer widths.Destroy()

	out, err := e.script.Run(map[string]*ort.Value{"x": in, "widths": widths})
	if err != nil {
		return scriptCJK
	}
	p, ok := out["p"]
	if !ok {
		return scriptCJK
	}
	probs, err := ort.GetTensorData[float32](p)
	if err != nil || len(probs) < 4 {
		return scriptCJK
	}
	best, bestP := scriptCJK, float32(0)
	for i := 0; i < 4; i++ {
		if probs[i] > bestP {
			best, bestP = scriptKind(i), probs[i]
		}
	}
	return best
}

// runRecognizer feeds one line crop through a recognizer and decodes the ids.
//
// The models emit already-decoded ids with their confidences; there is no CTC
// loop here because the graph does it.
func (e *indraEngine) runRecognizer(s *ort.Session, vocab []string, ink []float32, width, height int) (string, error) {
	in, err := ort.NewTensor([]int64{1, 1, int64(height), int64(width)}, ink)
	if err != nil {
		return "", err
	}
	defer in.Destroy()
	widths, err := ort.NewTensor([]int64{1}, []int64{int64(width)})
	if err != nil {
		return "", err
	}
	defer widths.Destroy()

	out, err := s.Run(map[string]*ort.Value{"x": in, "widths": widths})
	if err != nil {
		return "", fmt.Errorf("识别失败：%w", err)
	}
	ids, ok := out["ids"]
	if !ok {
		return "", fmt.Errorf("识别输出缺失")
	}
	idList, err := ort.GetTensorData[int64](ids)
	if err != nil {
		return "", err
	}
	length := len(idList)
	if lens, ok := out["lengths"]; ok {
		if ln, err := ort.GetTensorData[int64](lens); err == nil && len(ln) > 0 {
			if n := int(ln[0]); n > 0 && n <= len(idList) {
				length = n
			}
		}
	}

	return decodeIndraIDs(idList[:length], vocab), nil
}

// decodeIndraIDs turns recognizer ids into text.
//
// The vocabulary JSON declares "blank": 0, so id 1 is chars[0]: the table is
// one-based once the blank is counted. Reading chars[id] instead shifted every
// character by one, and "January 6-29" came out as "Kbovbsz!7.3:" — still text,
// still readable as gibberish, easy to miss in a screenshot nobody knows the
// answer to.
func decodeIndraIDs(ids []int64, vocab []string) string {
	var b strings.Builder
	for _, raw := range ids {
		id := int(raw)
		idx := id - 1
		if id <= 0 || idx >= len(vocab) {
			continue // 0 is the blank, and an id past the table is dropped
		}
		b.WriteString(vocab[idx])
	}
	return b.String()
}

// indraAvailable reports whether the engine can be used, for the settings page.
func indraAvailable() bool {
	_, err := indraGet()
	return err == nil
}

// indraStatus describes the engine for the settings page.
func indraStatus() string {
	e, err := indraGet()
	if err != nil {
		return "未启用：" + err.Error()
	}
	parts := []string{"已就绪"}
	// readLine only considers the Latin recognizer when all three pieces are
	// present; reporting ready with just recLatin would promise a mode the
	// engine cannot actually use.
	if e.recLatin == nil || e.script == nil || len(e.vocabLatin) == 0 {
		parts = append(parts, "（仅中文识别器）")
	}
	return strings.Join(parts, "")
}

// isShippedBuild reports whether the running binary is a release build rather
// than a development one.
//
// build.ps1 bakes a stamp into release binaries with -X main.buildStamp; a dev
// build built straight from go build has the zero value. The dev fallback for
// model locations hangs off this, so a release never resolves a model path
// relative to the working directory.
func isShippedBuild() bool {
	// build.ps1 injects a timestamp; a plain go build leaves "dev".
	return buildStamp != "" && buildStamp != "dev"
}
