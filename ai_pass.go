package main

type aiPassPhase uint8

const (
	aiPassIdle aiPassPhase = iota
	aiPassStreaming
	aiPassProse
	aiPassParseFailed
	aiPassStructured
)

func (p aiPassPhase) String() string {
	switch p {
	case aiPassIdle:
		return "idle"
	case aiPassStreaming:
		return "streaming"
	case aiPassProse:
		return "prose"
	case aiPassParseFailed:
		return "parse_failed"
	case aiPassStructured:
		return "structured"
	default:
		return "unknown"
	}
}

func classifyAIResult(s snapshot) aiPassPhase {
	switch {
	case s.aiBusy:
		return aiPassStreaming
	case s.aiText == "":
		return aiPassIdle
	case !looksLikeStructuredPayload(s.aiText):
		return aiPassProse
	default:
		return aiPassParseFailed
	}
}

func cardReadyForMode(s snapshot) bool {
	switch s.mode {
	case modeGrammar:
		return s.grammarResult != nil
	case modeDictionary:
		return s.dictResult != nil
	case modeWriting:
		return s.writingResult != nil
	default:
		return false
	}
}

func classifyAIView(s snapshot) aiPassPhase {
	if cardReadyForMode(s) {
		return aiPassStructured
	}
	return classifyAIResult(s)
}
