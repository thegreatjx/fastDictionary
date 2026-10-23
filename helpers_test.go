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
