package main

type ocrViewPhase uint8

const (
	ocrViewIdle ocrViewPhase = iota
	ocrViewRecognizing
	ocrViewError
	ocrViewEditing
)

func (p ocrViewPhase) String() string {
	switch p {
	case ocrViewIdle:
		return "idle"
	case ocrViewRecognizing:
		return "recognizing"
	case ocrViewError:
		return "error"
	case ocrViewEditing:
		return "editing"
	default:
		return "unknown"
	}
}

func classifyOCRView(s snapshot) ocrViewPhase {
	switch {
	case s.ocrBusy:
		return ocrViewRecognizing
	case s.ocrError != "":
		return ocrViewError
	case s.source != "" || s.ocrText != "":
		return ocrViewEditing
	default:
		return ocrViewIdle
	}
}

func ocrShowErrorBox(s snapshot) bool {
	return classifyOCRView(s) == ocrViewError
}

func ocrShowEditor(s snapshot) bool {
	p := classifyOCRView(s)
	return p == ocrViewError || p == ocrViewEditing
}
