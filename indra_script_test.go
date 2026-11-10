package main

import (
	"testing"
)

func TestScriptKindString(t *testing.T) {
	cases := []struct {
		kind scriptKind
		want string
	}{
		{scriptBase, "base"},
		{scriptCJK, "cjk"},
		{scriptRTL, "rtl"},
		{scriptWorld, "world"},
		{scriptKind(99), "unknown"},
	}
	for _, c := range cases {
		if got := c.kind.String(); got != c.want {
			t.Fatalf("scriptKind(%d).String() = %q, want %q", int(c.kind), got, c.want)
		}
	}
}

func TestPreferCJKRecognizer(t *testing.T) {
	cases := []struct {
		name       string
		kind       scriptKind
		lineHeight float64
		want       bool
	}{
		{"base small takes latin", scriptBase, 10, false},
		{"cjk small stays cjk", scriptCJK, 10, true},
		{"rtl small takes latin", scriptRTL, 10, false},
		{"world small takes latin", scriptWorld, 10, false},
		{"unknown small takes latin", scriptKind(99), 10, false},
		{"base large takes cjk", scriptBase, float64(indraLatinMinHeight), true},
		{"rtl large takes cjk", scriptRTL, float64(indraLatinMinHeight), true},
		{"world large takes cjk", scriptWorld, float64(indraLatinMinHeight), true},
	}
	for _, c := range cases {
		if got := preferCJKRecognizer(c.kind, c.lineHeight); got != c.want {
			t.Fatalf("%s: preferCJKRecognizer(%v, %v) = %v, want %v", c.name, c.kind, c.lineHeight, got, c.want)
		}
	}
}

// TestIndraScriptSelectionIsTyped pins the wiring: the classifier must return
// the named type and readLine must route through the shared predicate, with the
// old ink-ratio heuristic and the bare-int constant block gone.
func TestIndraScriptSelectionIsTyped(t *testing.T) {
	src := readSourceFile(t, "indra_ocr.go")
	if !containsText(src, "func (e *indraEngine) classifyScript(page grayImage, box lineBox) scriptKind {") {
		t.Fatalf("classifyScript should return scriptKind, keeping the script group a named type")
	}
	if !containsText(src, "preferCJKRecognizer(e.classifyScript(page, box), box.Y1-box.Y0)") {
		t.Fatalf("readLine should ask preferCJKRecognizer which recognizer to use")
	}
	if n := countOccurrences(src, "lineLooksCJK"); n != 0 {
		t.Fatalf("the unused ink-ratio heuristic should be gone, found %d references", n)
	}
	script := readSourceFile(t, "indra_script.go")
	if !containsText(script, "type scriptKind int") {
		t.Fatalf("scriptKind should be declared as its own type")
	}
}

// TestIndraStatusReportsLatinPrerequisites pins that the status line only claims
// the Latin recognizer when readLine's three prerequisites are all present.
func TestIndraStatusReportsLatinPrerequisites(t *testing.T) {
	src := readSourceFile(t, "indra_ocr.go")
	body := sourceFuncBody(t, src, "func indraStatus() string {")
	if !containsText(body, "e.recLatin == nil || e.script == nil || len(e.vocabLatin) == 0") {
		t.Fatalf("indraStatus must require recLatin, script and vocabLatin together, otherwise it promises a Latin mode readLine never uses")
	}
}
