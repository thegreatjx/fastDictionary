package main

type requestToken uint64

type cardActivity uint8

const (
	cardActivityIdle cardActivity = iota
	cardActivityTranslating
	cardActivityStreaming
	cardActivityRefining
	cardActivityLoadingNotes
	cardActivityRecognizing
)

func (a cardActivity) String() string {
	switch a {
	case cardActivityIdle:
		return "idle"
	case cardActivityTranslating:
		return "translating"
	case cardActivityStreaming:
		return "streaming"
	case cardActivityRefining:
		return "refining"
	case cardActivityLoadingNotes:
		return "loading_notes"
	case cardActivityRecognizing:
		return "recognizing"
	default:
		return "unknown"
	}
}

func (a cardActivity) busy() bool {
	return a != cardActivityIdle
}

func cardActivityOf(s snapshot) cardActivity {
	switch {
	case s.ocrBusy:
		return cardActivityRecognizing
	case s.aiBusy:
		return cardActivityStreaming
	case s.aiRefining:
		return cardActivityRefining
	case s.busy:
		return cardActivityTranslating
	case s.notesBusy:
		return cardActivityLoadingNotes
	default:
		return cardActivityIdle
	}
}

func cardHasPendingWork(s snapshot) bool {
	return cardActivityOf(s).busy()
}

type notesPhase uint8

const (
	notesIdle notesPhase = iota
	notesLoading
	notesReady
	notesEmpty
	notesError
)

func notesViewPhase(s snapshot) notesPhase {
	switch {
	case len(s.notes) > 0:
		return notesReady
	case s.notesBusy:
		return notesLoading
	default:
		return notesEmpty
	}
}
