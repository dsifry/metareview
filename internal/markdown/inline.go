package markdown

import "strings"

// FirstInlineCode reads back what InlineCode wrote: the first inline-code span on a line,
// whatever fence width it uses.
//
// It lives beside InlineCode because the two are one contract and were previously written apart —
// InlineCode widens its fence when the content contains a backtick, while the reader matched only
// `([^`]+)` and so returned the fragment before the first backtick. A path or a value containing a
// backtick therefore round-tripped to something shorter and different, silently.
func FirstInlineCode(line string) string {
	open := strings.IndexByte(line, '`')
	if open < 0 {
		return ""
	}
	n := 0
	for open+n < len(line) && line[open+n] == '`' {
		n++
	}
	fence := strings.Repeat("`", n)
	rest := line[open+n:]
	// The closing fence is the next run of EXACTLY n backticks; a longer run is not a match.
	for i := 0; i+n <= len(rest); i++ {
		if rest[i] != '`' {
			continue
		}
		run := 0
		for i+run < len(rest) && rest[i+run] == '`' {
			run++
		}
		if run == n {
			text := rest[:i]
			// InlineCode pads with one space when the content starts or ends with a backtick;
			// CommonMark strips exactly one such pair on read.
			if len(text) >= 2 && strings.HasPrefix(text, " ") && strings.HasSuffix(text, " ") &&
				(strings.HasPrefix(text[1:], "`") || strings.HasSuffix(text[:len(text)-1], "`")) {
				text = text[1 : len(text)-1]
			}
			return text
		}
		i += run - 1
	}
	_ = fence
	return ""
}
