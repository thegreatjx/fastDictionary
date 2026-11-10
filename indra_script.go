package main

// scriptKind is the writing-system group that script.onnx predicts.
//
// The values mirror meta.json's script_groups order: base, cjk, rtl, world.
// Keeping the group as a named type stops the classifier's output from being
// passed around as a bare int, where an out-of-range value would silently fall
// through to the default recognizer choice.
type scriptKind int

const (
	scriptBase scriptKind = iota
	scriptCJK
	scriptRTL
	scriptWorld
)

func (s scriptKind) String() string {
	switch s {
	case scriptBase:
		return "base"
	case scriptCJK:
		return "cjk"
	case scriptRTL:
		return "rtl"
	case scriptWorld:
		return "world"
	default:
		return "unknown"
	}
}

// preferCJKRecognizer reports whether a line should stay on the CJK recognizer.
//
// The rule matches the original inline switch: a large line goes to the CJK
// model whatever its script (see indraLatinMinHeight), and only the cjk group
// stays on it otherwise. base, rtl and world all take rec.onnx.
func preferCJKRecognizer(s scriptKind, lineHeight float64) bool {
	if lineHeight >= indraLatinMinHeight {
		return true
	}
	return s == scriptCJK
}
