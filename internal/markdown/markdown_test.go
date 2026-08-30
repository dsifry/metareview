package markdown

import "testing"

// InlineCode and FirstInlineCode are one contract and were written apart: the writer widens its
// fence when the content holds a backtick, and the reader matched only `([^`]+)`, so it returned
// the fragment before the first backtick. A value containing a backtick round-tripped to
// something shorter and different, silently — and in the review log that meant a covered-paths
// list truncated at the backtick, with every remaining file reading as unreviewed.
func TestInlineCodeRoundTrips(t *testing.T) {
	for _, value := range []string{
		"simple",
		"internal/a,b.go",
		`["a.go","b.go"]`,
		"has `one` backtick",
		"has ``two`` backticks",
		"`leading",
		"trailing`",
		"`both`",
		"a `` b ` c",
		"unknown",
		"none",
	} {
		line := "Field: " + InlineCode(value)
		if got := FirstInlineCode(line); got != value {
			t.Errorf("round trip of %q gave %q (encoded: %s)", value, got, line)
		}
	}
}

// A line with no inline code, or an unterminated fence, yields nothing rather than a guess: the
// caller treats empty as "the log did not say", which is the safe reading.
func TestFirstInlineCodeOnMalformedInput(t *testing.T) {
	for _, line := range []string{
		"Field: no code here",
		"",
		"Field: `unterminated",
		"Field: ``mismatched`",
	} {
		if got := FirstInlineCode(line); got != "" {
			t.Errorf("FirstInlineCode(%q) = %q, want empty", line, got)
		}
	}
	// It reads the FIRST span, not the last: a header field must not be redefined later on the
	// same line.
	if got := FirstInlineCode("Field: `first` and `second`"); got != "first" {
		t.Errorf("got %q, want the first span", got)
	}
}
