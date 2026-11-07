package main

import (
	"testing"
)

func TestCleanSelectionText(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "multiple spaces",
			input:    "Hello    world   this   is   a   test",
			expected: "Hello world this is a test",
		},
		{
			name:     "zero width chars",
			input:    "Hel\u200Blo\uFEFF wor\u200Cld",
			expected: "Hello world",
		},
		{
			name:     "pdf soft linebreaks in single paragraph",
			input:    "This is a line\n   broken across   multiple\n   pdf lines.",
			expected: "This is a line broken across multiple pdf lines.",
		},
		{
			name:     "paragraphs preserved",
			input:    "Paragraph one   with spaces.\n\n\nParagraph two   with   spaces.",
			expected: "Paragraph one with spaces.\n\nParagraph two with spaces.",
		},
		{
			name:     "tabs and windows newlines",
			input:    "\t\tHello\r\n\tWorld\r\n\r\nNext\tparagraph\t",
			expected: "Hello World\n\nNext paragraph",
		},
		{
			name:     "cjk space collapsing",
			input:    "剪 切   等 线   复 制 格 式 刷 1 1 én 四 川 省 宜 宾 市 兴 文 县",
			expected: "剪切等线复制格式刷 1 1 én 四川省宜宾市兴文县",
		},
		{
			name:     "cjk mixed with english and symbols",
			input:    "深 模 块 (Deep   Modules) 与 追踪   子弹 (Tracer Bullet)",
			expected: "深模块 (Deep Modules) 与追踪子弹 (Tracer Bullet)",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CleanSelectionText(tc.input)
			if got != tc.expected {
				t.Errorf("got %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestFlattenTranslation(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty",
			input:    "   \n\t  ",
			expected: "",
		},
		{
			name:     "chinese multiple paragraphs with empty lines",
			input:    "等等，OpenAI居然真的在做这个？！\n\n\n他们把Codex的线束直接嵌入到Agents SDK里\n\n你无需自行构建和维护执行工具，可以直接在SDK中使用\nCodex",
			expected: "等等，OpenAI居然真的在做这个？！他们把Codex的线束直接嵌入到Agents SDK里你无需自行构建和维护执行工具，可以直接在SDK中使用 Codex",
		},
		{
			name:     "english multiple lines preserves word breaks",
			input:    "First line of text.\n\nSecond line of   text\nwith a break.",
			expected: "First line of text. Second line of text with a break.",
		},
		{
			name:     "zero width chars stripped",
			input:    "测试\u200B\uFEFF文本\n\n换行\u200C测试",
			expected: "测试文本换行测试",
		},
		{
			name:     "cjk with punctuation and latin mix",
			input:    "还在Alpha阶段，\n\n但我真的很想看看他们会把这个玩法推进到\n什么程度",
			expected: "还在Alpha阶段，但我真的很想看看他们会把这个玩法推进到什么程度",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := flattenTranslation(tc.input)
			if got != tc.expected {
				t.Errorf("got %q, want %q", got, tc.expected)
			}
		})
	}
}
