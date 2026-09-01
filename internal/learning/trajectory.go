package learning

import (
	"sort"
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

// whitespaceOnly reports whether a unified diff changes only whitespace and/or comments — i.e. after
// dropping blank and comment-only lines and collapsing interior whitespace, the added and removed CODE
// lines are identical (both may be empty, e.g. a comment-only edit). A diff with no changes at all is
// not flagged (there is nothing to flag); a substantive code change (added ≠ removed) is not flagged.
func whitespaceOnly(diff string) bool {
	var added, removed []string
	changed := false
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			// unified-diff file headers, not content
		case strings.HasPrefix(line, "+"):
			changed = true
			if n := normalizeCode(line[1:]); n != "" {
				added = append(added, n)
			}
		case strings.HasPrefix(line, "-"):
			changed = true
			if n := normalizeCode(line[1:]); n != "" {
				removed = append(removed, n)
			}
		}
	}
	if !changed {
		return false
	}
	return equalMultiset(added, removed)
}

// normalizeCode strips a source line to its substantive content: "" for a blank or comment-only line
// (Go/C/JS `//`, shell/Python `#`, or a block-comment continuation `*`), otherwise the line with
// interior whitespace collapsed. Comment prefixes cover the languages metareview reviews; the point is
// that a change confined to comments/whitespace normalizes both sides to the same code.
func normalizeCode(s string) string {
	t := strings.TrimSpace(s)
	if t == "" || strings.HasPrefix(t, "//") || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "*") || strings.HasPrefix(t, "/*") {
		return ""
	}
	return strings.Join(strings.Fields(t), " ")
}

// equalMultiset reports whether two string slices hold the same elements with the same multiplicities.
func equalMultiset(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	ac := append([]string(nil), a...)
	bc := append([]string(nil), b...)
	sort.Strings(ac)
	sort.Strings(bc)
	for i := range ac {
		if ac[i] != bc[i] {
			return false
		}
	}
	return true
}
