package main

import "testing"

func TestNormalizeOCRMode(t *testing.T) {
	for input, want := range map[string]string{
		"":             ocrModeLocal,
		"local":        ocrModeLocal,
		"vision":       ocrModeVision,
		"local_first":  ocrModeLocalFirst,
		"unknown-mode": ocrModeLocal,
	} {
		if got := normalizeOCRMode(input); got != want {
			t.Errorf("normalizeOCRMode(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestDefaultOCRModeIsLocal(t *testing.T) {
	if got := DefaultConfig().OCRMode; got != ocrModeLocal {
		t.Fatalf("default OCRMode = %q, want %q", got, ocrModeLocal)
	}
	if len(ocrModeLabels()) != len(ocrModeValues()) {
		t.Fatal("OCR mode labels and values must stay aligned")
	}
}
