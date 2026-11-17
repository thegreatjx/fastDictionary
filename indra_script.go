package main

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

func preferCJKRecognizer(s scriptKind, lineHeight float64) bool {
	if lineHeight >= indraLatinMinHeight {
		return true
	}
	return s == scriptCJK
}
