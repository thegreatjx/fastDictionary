package main

import "testing"

// TestEchoedTranslationIsRejected is the regression test for the Portuguese
// sentence in the runtime log: 腾讯 answered in 365ms with the source text
// unchanged, won the race, and the reader saw their own sentence as the
// translation. A translation identical to its input is a failure, not a result.
func TestEchoedTranslationIsRejected(t *testing.T) {
	const source = "Porra EP mencionou Xi Jinping, minha casa é sua."
	if !looksUntranslated(source, source, "pt", "zh-Hans") {
		t.Fatal("an exact echo must count as untranslated")
	}
	if !looksUntranslated("  "+source+" ", source, "pt", "zh-Hans") {
		t.Fatal("whitespace and case should not hide an echo")
	}
	if looksUntranslated(source, "该死的 EP 提到了习近平，我的房子是你的。", "pt", "zh-Hans") {
		t.Fatal("a real translation must pass")
	}
	// A translation that keeps a name is still a translation.
	if looksUntranslated("Mina is at the beach.", "Mina 在海滩上。", "en", "zh-Hans") {
		t.Fatal("keeping a proper noun is not an echo")
	}
	// Same language on both sides: an identical result is expected, not a bug.
	if looksUntranslated(source, source, "pt", "pt") {
		t.Fatal("same-language pass-through must not be flagged")
	}
}
