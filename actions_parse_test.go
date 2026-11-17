package main

import (
	"strings"
	"testing"
)

func TestFormatActionPrompt(t *testing.T) {
	tpl := "Translate {{selection}} into {{targetLanguage}}. Context: {{paragraphs}}"
	got := FormatActionPrompt(tpl, "量价背离", "the previous paragraph", "Chinese")
	want := "Translate 量价背离 into Chinese. Context: the previous paragraph"
	if got != want {
		t.Fatalf("FormatActionPrompt = %q, want %q", got, want)
	}

	if got := FormatActionPrompt("just a prompt", "x", "y", "z"); got != "just a prompt" {
		t.Fatalf("template without placeholders was rewritten: %q", got)
	}

	got = FormatActionPrompt("{{selection}} and {{selection}}", "abc", "", "")
	if got != "abc and abc" {
		t.Fatalf("repeated placeholder = %q, want %q", got, "abc and abc")
	}

	if got := FormatActionPrompt("A{{selection}}B", "", "", ""); got != "AB" {
		t.Fatalf("empty substitution = %q, want %q", got, "AB")
	}
}

func TestFirstNonEmpty(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want string
	}{
		{"first wins", []string{"a", "b"}, "a"},
		{"skips blanks", []string{"", "   ", "c"}, "c"},
		{"trims", []string{"  padded  "}, "padded"},
		{"all blank", []string{"", " "}, ""},
		{"none", nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := firstNonEmpty(c.in...); got != c.want {
				t.Fatalf("firstNonEmpty(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestFirstNonNil(t *testing.T) {
	var nilStr *string
	s := "value"
	if got := firstNonNil(nil, nilStr, &s); got != &s {
		t.Fatalf("firstNonNil did not skip nils, got %v", got)
	}
	if got := firstNonNil(nil, nilStr); got != nil {
		t.Fatalf("firstNonNil(all nil) = %v, want nil", got)
	}
}

func TestParseMemoryTips(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		if got := parseMemoryTips(nil); got != "" {
			t.Fatalf("parseMemoryTips(nil) = %q, want empty", got)
		}
	})
	t.Run("string is trimmed", func(t *testing.T) {
		if got := parseMemoryTips("  think of the roots  "); got != "think of the roots" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("list is joined with newlines", func(t *testing.T) {
		got := parseMemoryTips([]any{"one", "  ", "two"})
		if got != "one\ntwo" {
			t.Fatalf("parseMemoryTips(list) = %q, want %q", got, "one\ntwo")
		}
	})
	t.Run("empty list", func(t *testing.T) {
		if got := parseMemoryTips([]any{}); got != "" {
			t.Fatalf("parseMemoryTips(empty list) = %q, want empty", got)
		}
	})
	t.Run("unknown type falls back to fmt", func(t *testing.T) {
		if got := parseMemoryTips(42); got != "42" {
			t.Fatalf("parseMemoryTips(42) = %q, want %q", got, "42")
		}
	})
}

func TestCleanJSONText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", `{"a":1}`, `{"a":1}`},
		{"json fence", "```json\n{\"a\":1}\n```", `{"a":1}`},
		{"bare fence", "```\n{\"a\":1}\n```", `{"a":1}`},
		{"surrounding space", "  {\"a\":1}  ", `{"a":1}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := CleanJSONText(c.in); got != c.want {
				t.Fatalf("CleanJSONText(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestCleanJSONTextSurvivesProse(t *testing.T) {
	in := "Here is the result:\n```json\n{\"word\":\"apple\"}\n```"
	got := CleanJSONText(in)
	if !strings.Contains(got, `"word"`) {
		t.Fatalf("CleanJSONText dropped the JSON body: %q", got)
	}
	if strings.HasSuffix(got, "```") {
		t.Fatalf("CleanJSONText left the closing fence behind: %q", got)
	}
	if !strings.HasPrefix(got, "Here is the result:") {
		t.Fatalf("CleanJSONText was expected to keep the prose, got %q", got)
	}
}

func TestParseDictionaryResultRecoversFromProse(t *testing.T) {
	raw := "Sure! Here is the dictionary entry:\n```json\n" +
		`{"term":"apple","phonetic":"/ˈæp.əl/","partOfSpeech":"noun","definition":"a round fruit"}` +
		"\n```"
	got := ParseDictionaryResult(raw)
	if got == nil {
		t.Fatal("ParseDictionaryResult returned nil for a recoverable response")
	}
	if got.Term != "apple" || got.Definition != "a round fruit" {
		t.Fatalf("recovered fields are wrong: term=%q definition=%q", got.Term, got.Definition)
	}
	if got.Phonetic != "/ˈæp.əl/" || got.PartOfSpeech != "noun" {
		t.Fatalf("phonetic/pos lost: %q / %q", got.Phonetic, got.PartOfSpeech)
	}
}

func TestParseDictionaryResultChineseFallback(t *testing.T) {
	raw := `{"词条":"量价背离","音标":"liàng jià bèi lí","词性":"名词","释义":"价格与成交量方向相反","句子":"这是一个量价背离的形态","句中词形":"量价背离","记忆技巧":"量增价跌"}` + "\n"
	got := ParseDictionaryResult(raw)
	if got == nil {
		t.Fatal("ParseDictionaryResult returned nil for Chinese keys")
	}
	if got.Term != "量价背离" {
		t.Fatalf("term = %q, want 量价背离", got.Term)
	}
	if got.Definition == "" || got.PartOfSpeech != "名词" {
		t.Fatalf("Chinese fields lost: definition=%q pos=%q", got.Definition, got.PartOfSpeech)
	}
	if got.ContextTerm != "量价背离" {
		t.Fatalf("context term = %q", got.ContextTerm)
	}
	if got.MemoryTips == "" {
		t.Fatal("memory tips lost")
	}
}

func TestParseDictionaryResultRejectsGarbage(t *testing.T) {
	if got := ParseDictionaryResult("I cannot help with that."); got != nil {
		t.Fatalf("garbage produced a result: %+v", got)
	}
	if got := ParseDictionaryResult(""); got != nil {
		t.Fatalf("empty produced a result: %+v", got)
	}
}
