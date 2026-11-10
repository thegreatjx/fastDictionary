package main

import "testing"

// 批次 6c 源码契约：renderScreenshotSource 的 OCR 状态判断应收进
// classifyOCRView 与两个具名谓词，不再散落成三个裸布尔 if。
func TestOCRRenderUsesExplicitPhase(t *testing.T) {
	src := readSourceFile(t, "ocr.go")
	if !containsText(src, "classifyOCRView(s) == ocrViewRecognizing") {
		t.Fatal("识别中的判断仍未收进 classifyOCRView")
	}
	if !containsText(src, "ocrShowErrorBox(s)") {
		t.Fatal("错误框判断仍未收进 ocrShowErrorBox")
	}
	if !containsText(src, "ocrShowEditor(s)") {
		t.Fatal("编辑区判断仍未收进 ocrShowEditor")
	}
	if countOccurrences(src, "if s.ocrBusy {") > 0 {
		t.Fatal("renderScreenshotSource 仍直接读 s.ocrBusy")
	}
}

// TestClassifyOCRView 钉住主阶段枚举的判定顺序：busy 优先，其次 error，
// 再次有文本，最后 idle。
func TestClassifyOCRView(t *testing.T) {
	cases := []struct {
		name              string
		busy              bool
		err, source, text string
		want              ocrViewPhase
	}{
		{"idle", false, "", "", "", ocrViewIdle},
		{"recognizing", true, "", "", "", ocrViewRecognizing},
		{"busy beats error", true, "E", "S", "T", ocrViewRecognizing},
		{"error beats text", false, "E", "S", "T", ocrViewError},
		{"error alone", false, "E", "", "", ocrViewError},
		{"editing by source", false, "", "S", "", ocrViewEditing},
		{"editing by text", false, "", "", "T", ocrViewEditing},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := snapshot{ocrBusy: tc.busy, ocrError: tc.err, source: tc.source, ocrText: tc.text}
			if got := classifyOCRView(s); got != tc.want {
				t.Fatalf("classifyOCRView = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestOCRViewPhaseString(t *testing.T) {
	cases := map[ocrViewPhase]string{
		ocrViewIdle:        "idle",
		ocrViewRecognizing: "recognizing",
		ocrViewError:       "error",
		ocrViewEditing:     "editing",
		ocrViewPhase(99):   "unknown",
	}
	for p, want := range cases {
		if got := p.String(); got != want {
			t.Fatalf("phase(%d).String() = %q, want %q", p, got, want)
		}
	}
}

// TestOCRRenderDecisionsMatchLegacy 是切换读取点的零行为变化判据：
// 遍历四个字段的所有组合，把新谓词三元组与改动前 renderScreenshotSource
// 的三个 if 渲染决策逐项对比。改动前的判断作为规格写在这里。
func TestOCRRenderDecisionsMatchLegacy(t *testing.T) {
	values := []string{"", "text"}
	for _, busy := range []bool{false, true} {
		for _, errText := range values {
			for _, source := range values {
				for _, ocrText := range values {
					s := snapshot{ocrBusy: busy, ocrError: errText, source: source, ocrText: ocrText}

					// 改动前的渲染决策：busy 命中后 return，后两块不渲染。
					legacySpinner := s.ocrBusy
					legacyErrorBox := !s.ocrBusy && s.ocrError != ""
					legacyEditor := !s.ocrBusy && (s.source != "" || s.ocrText != "" || s.ocrError != "")

					// 改动后：三个具名谓词表达同一组叠加渲染决策。
					newSpinner := classifyOCRView(s) == ocrViewRecognizing
					newErrorBox := ocrShowErrorBox(s)
					newEditor := ocrShowEditor(s)

					if newSpinner != legacySpinner || newErrorBox != legacyErrorBox || newEditor != legacyEditor {
						t.Fatalf("busy=%v err=%q source=%q text=%q: new(%v,%v,%v) != legacy(%v,%v,%v)",
							busy, errText, source, ocrText,
							newSpinner, newErrorBox, newEditor,
							legacySpinner, legacyErrorBox, legacyEditor)
					}
				}
			}
		}
	}
}
