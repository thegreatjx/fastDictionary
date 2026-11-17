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

const (
	indraModelDirEnv   = "FASTDICTIONARY_INDRA_DIR"
	indraRuntimeEnv    = "FASTDICTIONARY_ORT_DLL"
	indraDevDirEnv     = "FASTDICTIONARY_INDRA_DEV"
	indraMaxLines      = 64
	indraMinLineScore  = 0.35
	indraMinLineHeight = 8
	indraMinLineWidth  = 12
)

type indraEngine struct {
	eng      *ort.Engine
	opts     *ort.SessionOptions
	det      *ort.Session
	recCJK   *ort.Session
	recLatin *ort.Session
	script   *ort.Session
	window   int

	vocabCJK   []string
	vocabLatin []string

	closeOnce sync.Once
}

var (
	indraMu       sync.Mutex
	indraInst     *indraEngine
	indraErr      error
	indraLoaded   bool
	indraFailedAt time.Time
)

const indraRetryCooldown = 30 * time.Second

var indraLoadEngine = indraLoad

type indraVocab struct {
	Blank int      `json:"blank"`
	Chars []string `json:"chars"`
}

type indraMeta struct {
	Vocab     json.RawMessage `json:"vocab"`
	VocabCJK  json.RawMessage `json:"vocab_cjk"`
	RecWindow int             `json:"rec_window"`
}

func indraLookupDirs() []string {
	var out []string
	if d := strings.TrimSpace(os.Getenv(indraModelDirEnv)); d != "" {
		out = append(out, d)
	}
	if exe, err := os.Executable(); err == nil {
		base := filepath.Dir(exe)
		out = append(out, filepath.Join(base, "indra-models"),
			filepath.Join(base, "models", "indra"))
	}
	return out
}

func indraDevDirs() []string {
	return []string{filepath.Join("build", "indra-models"), "indra-models"}
}

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

func indraRuntimeCandidates() []string {
	var out []string
	if d := strings.TrimSpace(os.Getenv(indraRuntimeEnv)); d != "" {
		out = append(out, d)
	}
	if exe, err := os.Executable(); err == nil {
		out = append(out, filepath.Join(filepath.Dir(exe), "onnxruntime.dll"))
	}
	if wd, err := os.Getwd(); err == nil {
		out = append(out, filepath.Join(wd, "onnxruntime.dll"))
	}
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

// 模型目录与 exe 同级；找不到会静默回落到 Windows OCR。
func indraLoad() (*indraEngine, error) {
	modelDir := ""
	candidates := indraLookupDirs()
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

func (e *indraEngine) loadVocab(modelDir string) error {
	raw, err := os.ReadFile(filepath.Join(modelDir, "meta.json"))
	if err != nil {
		return fmt.Errorf("读取 meta.json 失败：%w", err)
	}
	var m indraMeta
	if err := json.Unmarshal(raw, &m); err != nil {
		return fmt.Errorf("解析 meta.json 失败：%w", err)
	}
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
		e.vocabLatin = nil
	}
	if len(e.vocabCJK) == 0 {
		return fmt.Errorf("中文词表为空")
	}
	e.window = m.RecWindow
	if e.window < 1 {
		e.window = 1
	}
	return nil
}

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
		if b.Y1-b.Y0 < indraMinLineHeight || b.X1-b.X0 < indraMinLineWidth {
			continue
		}
		kept = append(kept, b)
	}
	return dropNestedBoxes(kept), nil
}

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

const indraLatinMinHeight = 40

func (e *indraEngine) readLine(page grayImage, box lineBox) (string, error) {
	sess := e.recCJK
	height, window, vocab := indraLineHeight, 1, e.vocabCJK

	if e.script != nil && e.recLatin != nil && len(e.vocabLatin) > 0 {
		if !preferCJKRecognizer(e.classifyScript(page, box), box.Y1-box.Y0) {
			sess, height, vocab, window = e.recLatin, indraLatinHeight, e.vocabLatin, maxInt(1, e.window)
		}
	}

	ink, width := cropLine(page, box, indraLineHeight, window)
	return e.runRecognizer(sess, vocab, ink, width, height)
}

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

func decodeIndraIDs(ids []int64, vocab []string) string {
	var b strings.Builder
	for _, raw := range ids {
		id := int(raw)
		idx := id - 1
		if id <= 0 || idx >= len(vocab) {
			continue
		}
		b.WriteString(vocab[idx])
	}
	return b.String()
}

func indraAvailable() bool {
	_, err := indraGet()
	return err == nil
}

func indraStatus() string {
	e, err := indraGet()
	if err != nil {
		return "未启用：" + err.Error()
	}
	parts := []string{"已就绪"}
	if e.recLatin == nil || e.script == nil || len(e.vocabLatin) == 0 {
		parts = append(parts, "（仅中文识别器）")
	}
	return strings.Join(parts, "")
}

func isShippedBuild() bool {
	return buildStamp != "" && buildStamp != "dev"
}
