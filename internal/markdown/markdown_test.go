package markdown

import "testing"

// PlainText's contract: newlines, carriage returns, and control characters are removed so a value
// can never break out of a single markdown line/header field; ordinary runes pass through unchanged.
func TestPlainTextStripsNewlinesAndControlRunes(t *testing.T) {
	cases := map[string]string{
		"plain":          "plain",
		"a\nb":           "ab",  // newline dropped
		"a\rb":           "ab",  // carriage return dropped
		"a\tb":           "ab",  // tab is a control rune, dropped
		"a\x00b\x07c":    "abc", // NUL and BEL dropped
		"line1\r\nline2": "line1line2",
		"kéep unicode ✓": "kéep unicode ✓", // non-control multibyte runes survive
	}
	for in, want := range cases {
		if got := PlainText(in); got != want {
			t.Fatalf("PlainText(%q) = %q, want %q", in, got, want)
		}
	}
}

// FencedCodeBlock's contract: it wraps content in a fence of backticks that is (a) at least 3 long
// and (b) always longer than the longest run of backticks inside the content, so the block can never
// be closed early by its own body. The language tag sits on the opening fence.
func TestFencedCodeBlock(t *testing.T) {
	cases := []struct {
		name     string
		language string
		content  string
		want     string
	}{
		{name: "no backticks uses a 3-fence", language: "go", content: "x := 1", want: "```go\nx := 1\n```"},
		{name: "empty language", language: "", content: "plain", want: "```\nplain\n```"},
		{name: "a run of two backticks still fits in a 3-fence", language: "", content: "a ``b", want: "```\na ``b\n```"},
		{name: "a run of three backticks forces a 4-fence", language: "md", content: "```", want: "````md\n```\n````"},
		{name: "a run of four backticks forces a 5-fence", language: "", content: "x ```` y", want: "`````\nx ```` y\n`````"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FencedCodeBlock(tc.language, tc.content); got != tc.want {
				t.Fatalf("FencedCodeBlock(%q,%q) =\n%q\nwant\n%q", tc.language, tc.content, got, tc.want)
			}
		})
	}
}
