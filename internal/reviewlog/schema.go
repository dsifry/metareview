package reviewlog

import (
	"encoding/json"
	"strings"

	"github.com/dsifry/metareview/internal/markdown"
)

// The durable review-log header fields, owned here.
//
// This schema was written by three packages and read by a fourth, with the labels, the separator
// and the sentinels spelled out independently on each side and pinned only by string literals in
// four test files. Changing one writer's separator and its own expected literal left the whole
// suite green while real logs became unparseable — and the fourth writer, artifactreview, was
// simply never given the fields at all. One owner, one round-trip test, and every writer calls in.
const (
	HeadLabel         = "Head:"
	CoveredPathsLabel = "Covered paths:"
	// UnknownHead is written when a review has no head to record. It is a positive statement of
	// ignorance, distinct from a log old enough to carry no Head line at all.
	UnknownHead = "unknown"
	// NoCoveredPaths is written when a review examined no files. Distinct from absent, which is
	// what a log predating this field carries: the first can answer "no" for a path, the second
	// cannot answer at all, and only the second must be barred from vouching.
	NoCoveredPaths = "none"
)

// EncodeCoveredPaths renders the paths for the header.
//
// JSON, not a comma-joined list. Git paths may contain commas and `git diff --name-only` prints
// them unquoted, so a comma-joined list did not round-trip: one such path parsed into two, the
// real file could never be matched back — permanently UNREVIEWED, and unclearable, because every
// re-run wrote the same corrupted line — while the fragments falsely marked other files reviewed.
// This repository's own branch carries a path named `0,`.
func EncodeCoveredPaths(paths []string) string {
	if len(paths) == 0 {
		return NoCoveredPaths
	}
	b, err := json.Marshal(paths)
	if err != nil {
		// Unreachable for []string, and a lie would be worse than a refusal: say nothing is known
		// rather than emit a line that parses to the wrong set.
		return NoCoveredPaths
	}
	return string(b)
}

// DecodeCoveredPaths reads that field back. known is false when the log said nothing at all,
// which is not the same as saying "none" — see NoCoveredPaths.
func DecodeCoveredPaths(text string) (paths []string, known bool) {
	text = strings.TrimSpace(text)
	switch text {
	case "":
		return nil, false
	case NoCoveredPaths:
		return nil, true
	}
	if err := json.Unmarshal([]byte(text), &paths); err != nil {
		// A legacy comma-joined line, or something corrupted. Refuse it rather than guess: a
		// wrong path set silently clears files nobody reviewed.
		return nil, false
	}
	return paths, true
}

// HeaderLine renders one header field, sanitised. PlainText strips newlines and control
// characters, which is what stops a value from becoming additional header lines.
func HeaderLine(label, value string) string {
	return label + " " + markdown.InlineCode(value) + "\n\n"
}
