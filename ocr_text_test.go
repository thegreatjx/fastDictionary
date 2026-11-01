package main

import (
	"strings"
	"testing"
)

func TestUnwrapOCRText(t *testing.T) {
	got := unwrapOCRText("EXTRACTED_TEXT_START\r\n第一行\r\nsecond line\r\nEXTRACTED_TEXT_END")
	if got != "第一行\r\nsecond line" {
		t.Fatalf("OCR transport markers leaked into text: %q", got)
	}
	if strings.Contains(got, "EXTRACTED_TEXT") {
		t.Fatal("marker leaked into editable OCR text")
	}
	if got := unwrapOCRText("EXTRACTED_TEXT_START\r\n\r\nEXTRACTED_TEXT_END"); got != "" {
		t.Fatalf("empty OCR result = %q", got)
	}
	if got := unwrapOCRText("plain OCR output"); got != "plain OCR output" {
		t.Fatalf("plain output changed: %q", got)
	}
}
