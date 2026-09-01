package learning

import (
	"strings"

	"github.com/dsifry/metareview/internal/findings"
)

// trajectoryFlags is the §9.7 trajectory monitor's derivable half: advisory flags read directly from
// the reviewed diff and the override records — no new wire fields. It raises the two flags that need no
// deferred fix-trajectory record: a whitespace/comment-only "fix", and a self-served override. The
// oracle-edit/answer-lookup flags (which need the ⧗ fix-trajectory record) are intentionally absent.
// All flags are ADVISORY (a monitor, not a gate).
func trajectoryFlags(diff string, records []findings.Record) []Candidate {
	var out []Candidate
	if whitespaceOnly(diff) {
		out = append(out, Candidate{
			Kind:           "trajectory-flag",
			Text:           "Whitespace/comment-only change: the fix altered no substantive code (§9.7). A green suite after such a change is not evidence the reported bug was addressed.",
			Scope:          "process-integrity",
			Provenance:     "§9.7 trajectory monitor: the reviewed diff is whitespace/comment-only",
			SourceRefs:     []SourceRef{{Type: "diff"}},
			Confidence:     "high",
			ProposedTarget: "process-review",
			Disposition:    "advisory",
		})
	}
	for _, r := range records {
		if !selfServed(r) {
			continue
		}
		out = append(out, Candidate{
			Kind:           "trajectory-flag",
			Text:           "Self-served override: the exception for " + firstNonEmpty(r.Title, r.Finding, r.ID) + " was not the product of a distinct requester and granter (§9.7/§4.1).",
			Scope:          "process-integrity",
			Provenance:     "§9.7 trajectory monitor: override requester == granter (or granted with no request)",
			SourceRefs:     findingRefs(r),
			Confidence:     "high",
			ProposedTarget: "process-review",
			Disposition:    "advisory",
		})
	}
	return out
}

// selfServed reports whether a GRANTED override was self-served: the §4.1 separation of requester from
// granter was not honored — the same actor on both sides, or a grant with no request at all. A
// legitimate override (a distinct requester AND granter) does NOT fire. The grant CLI already refuses a
// requester who re-grants (override.go), so this monitor catches the residual the CLI cannot: a direct
// grant on an unrequested finding, or a record where the two actors coincide.
func selfServed(r findings.Record) bool {
	if r.OverrideGrantedBy == "" {
		return false // not granted → nothing to flag
	}
	return r.OverrideRequestedBy == "" || strings.EqualFold(r.OverrideRequestedBy, r.OverrideGrantedBy)
}

// whitespaceOnly reports whether a unified diff changes only whitespace and/or comments. It works
// PER `diff --git` SECTION and IN ORDER, on the RAW section text (no path normalization — so two files
// under literal `a/`/`b/` dirs cannot collide), scanning content lines positionally (only lines after
// an `@@` hunk header, so a `--- a/x`/`+++ b/x` file header is never mistaken for content and neither
// is `++i`/`--i` content mistaken for a header). A section whose ordered added and removed code lines
// are identical after normalization changed only whitespace/comments; if ANY section's code lines
// differ (a real edit, an added/removed line, a cross-file move — which lands lines in only one
// section — or a reorder, order-sensitive) the diff is substantive and not flagged. A diff with no
// content lines is not flagged.
func whitespaceOnly(diff string) bool {
	sections := gitSections(diff)
	if len(sections) == 0 {
		return false
	}
	sawContent := false
	for _, sec := range sections {
		add, rem := hunkLines(sec)
		if len(add) > 0 || len(rem) > 0 {
			sawContent = true
		}
		if !equalSeq(codeLines(add), codeLines(rem)) {
			return false // a substantive change in this section
		}
	}
	return sawContent
}

// gitSections splits a unified diff into one string per `diff --git` file section. A diff carrying no
// `diff --git` header (e.g. a bare hunk) is one section; an empty diff is none.
func gitSections(diff string) []string {
	if !strings.Contains(diff, "diff --git ") {
		if strings.TrimSpace(diff) == "" {
			return nil
		}
		return []string{diff}
	}
	var secs []string
	var cur strings.Builder
	started := false
	flush := func() {
		if started {
			secs = append(secs, cur.String())
			cur.Reset()
		}
	}
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			flush()
			started = true
		}
		if started {
			cur.WriteString(line)
			cur.WriteString("\n")
		}
	}
	flush()
	return secs
}

// hunkLines returns the raw added and removed CONTENT lines of one file section (leading +/- stripped),
// in order. Only lines after an `@@` header count, so the `--- a/x` / `+++ b/x` file headers (before
// the first hunk) are excluded while `++i` / `--i` content (inside a hunk) is kept.
func hunkLines(section string) (add, rem []string) {
	inHunk := false
	for _, line := range strings.Split(section, "\n") {
		switch {
		case strings.HasPrefix(line, "@@"):
			inHunk = true
		case !inHunk:
			// diff --git / index / mode / --- / +++ header lines
		case strings.HasPrefix(line, "+"):
			add = append(add, line[1:])
		case strings.HasPrefix(line, "-"):
			rem = append(rem, line[1:])
		}
	}
	return add, rem
}

// codeLines drops blank and full-line comment lines and collapses interior whitespace, preserving
// order. A full-line comment is `//` followed by a space (or a bare `//`) — NOT a Go directive such as
// `//go:build` (`//` immediately followed by a non-space), which is meaningful and kept. Ambiguous
// prefixes (`*` pointer deref, `#` JS-private/preprocessor, `/*`) are deliberately NOT treated as
// comments — a change to them is real code.
func codeLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if n := normalizeCode(l); n != "" {
			out = append(out, n)
		}
	}
	return out
}

func normalizeCode(s string) string {
	t := strings.TrimSpace(s)
	if t == "" || t == "//" || strings.HasPrefix(t, "// ") {
		return ""
	}
	return strings.Join(strings.Fields(t), " ")
}

// equalSeq reports whether two string slices are identical in length, contents, AND order (so a
// reorder of the same lines is NOT equal).
func equalSeq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
