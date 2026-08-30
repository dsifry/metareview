package markdown

import "testing"

// FirstInlineCode is the READ half of InlineCode, and the pair has to round-trip: the review log
// writes scoping values as inline code and reads them back to decide whether a review still
// covers the current branch. A value that does not survive the round trip silently changes the
// scope of a gate. This shipped with no test at all — the coverage floor caught it, review did not.
func TestInlineCodeRoundTrip(t *testing.T) {
	for _, value := range []string{
		"abc",
		"a b c",
		"",
		"`",              // a lone backtick: the writer pads, the reader must unpad
		"``",             // a run the writer must out-fence
		"a`b",            // an interior backtick needs no padding
		"`leading",       // padded because it STARTS with a backtick
		"trailing`",      // padded because it ENDS with one
		"`both`",         // padded on both sides
		"a``b```c",       // the longest interior run decides the fence
		" already space", // leading space that is NOT padding must survive
		"deadbeef1234",   // the ordinary case: a SHA
		"docs/a.md",
	} {
		if got := FirstInlineCode(InlineCode(value)); got != value {
			t.Errorf("round trip changed the value:\n  in   %q\n  wrote %q\n  out  %q", value, InlineCode(value), got)
		}
	}
}

// Reading text this package did not write. A malformed line must yield "" rather than a
// half-parsed value, because a wrong scope reads as a valid one.
func TestFirstInlineCodeOnForeignText(t *testing.T) {
	for name, tc := range map[string]struct{ line, want string }{
		"no code at all":                {"just prose", ""},
		"unterminated fence":            {"a `b c", ""},
		"unterminated double":           {"a ``b` c", ""},
		"empty span":                    {"a `` b", ""},
		"first of several":              {"`one` and `two`", "one"},
		"prose either side":             {"see `x` here", "x"},
		"a longer run is not a close":   {"`a``b`", "a``b"},
		"double fence closes on double": {"``a`b`` c", "a`b"},
		"backtick at line start":        {"`x` y", "x"},
		"only backticks":                {"``", ""},
	} {
		if got := FirstInlineCode(tc.line); got != tc.want {
			t.Errorf("%s: FirstInlineCode(%q) = %q, want %q", name, tc.line, got, tc.want)
		}
	}
}
