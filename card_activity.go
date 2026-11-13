package main

// requestToken identifies one in-flight request of a kind. Every new request
// takes the next value; a response whose token no longer matches the field it
// was issued from is stale and must be dropped rather than written back.
type requestToken uint64

// cardActivity names the one request currently allowed to own the card area.
// The four "busy" flags stay the source of truth; this type only fixes the
// reading order so layout, tab caching and empty states agree on which of them
// wins when more than one is set.
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

// busy reports whether this activity means a request is still in flight.
func (a cardActivity) busy() bool {
	return a != cardActivityIdle
}

// cardActivityOf resolves the independent flags into one value. The order is
// deliberate: a screenshot OCR pass owns the whole card, then an AI pass, then
// a refine, then a machine translation, and finally the notes checklist, which
// only decorates a finished card.
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

// cardHasPendingWork is the one place that answers "is anything still coming?".
// Tab caching uses it to keep loading tabs out of the restore cache, and the
// translate pane uses it to hold back the empty state while a request runs.
func cardHasPendingWork(s snapshot) bool {
	return cardActivityOf(s).busy()
}

// notesViewPhase is the explicit, mutually exclusive phase of the "猜你想存"
// strip. Rendering switches on this value instead of guessing a state from a
// combination of notesBusy and len(notes); the conversion lives here only.
// requestToken 已经提供「发起新任务取新代次、回调写入前校验」的 Epoch 语义，
// 所有异步路径共用它，因此不重复引入第二套 AsyncEpoch。
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
