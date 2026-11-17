package main

import (
	"errors"
	"testing"
)

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
	if looksUntranslated("Mina is at the beach.", "Mina 在海滩上。", "en", "zh-Hans") {
		t.Fatal("keeping a proper noun is not an echo")
	}
	if looksUntranslated(source, source, "pt", "pt") {
		t.Fatal("same-language pass-through must not be flagged")
	}
}

func TestPinnedEchoIsNotAFailure(t *testing.T) {
	echoed := finalizePinned("fastDictionary", "", "zh-Hans", raceResult{
		Text: "fastDictionary", Engine: "微软", Detected: "en",
	})
	if echoed.Err != nil {
		t.Fatalf("an echoed answer must not be an error: %v", echoed.Err)
	}
	if !echoed.Untranslated {
		t.Fatal("an echoed answer must be marked untranslated")
	}
	if echoed.Engine != "微软" {
		t.Fatalf("engine tag lost: %q", echoed.Engine)
	}

	translated := finalizePinned("fast dictionary", "", "zh-Hans", raceResult{
		Text: "快速词典", Engine: "微软", Detected: "en",
	})
	if translated.Err != nil || translated.Untranslated {
		t.Fatalf("a real translation was misclassified: err=%v untranslated=%v",
			translated.Err, translated.Untranslated)
	}

	failed := finalizePinned("hello", "", "zh-Hans", raceResult{Engine: "微软", Err: errors.New("http 429")})
	if failed.Err == nil || failed.Untranslated {
		t.Fatalf("a transport failure must stay an error: err=%v untranslated=%v", failed.Err, failed.Untranslated)
	}
}

func TestPickEchoAnswerPrefersAnotherEngine(t *testing.T) {
	echo := raceResult{Text: "Azure", Engine: "微软", Untranslated: true}
	real := raceResult{Text: "天蓝色", Engine: "谷歌"}
	if got, ok := pickEchoAnswer(echo, real); !ok || got.Text != "天蓝色" || got.Engine != "谷歌" {
		t.Fatalf("fallback not used: %+v ok=%v", got, ok)
	}
	alsoEcho := raceResult{Text: "Azure", Engine: "谷歌", Untranslated: true}
	if got, ok := pickEchoAnswer(echo, alsoEcho); ok || got.Engine != "微软" {
		t.Fatalf("an echo was taken as a fallback: %+v ok=%v", got, ok)
	}
	if _, ok := pickEchoAnswer(echo, raceResult{Err: errors.New("http 429")}); ok {
		t.Fatal("a failed fallback must not replace the echo")
	}
	if _, ok := pickEchoAnswer(echo, raceResult{Text: "   "}); ok {
		t.Fatal("a blank fallback must not replace the echo")
	}
}
