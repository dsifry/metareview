package judge

import (
	"sort"
	"strings"

	"github.com/dsifry/metareview/internal/claimcheck"
	"github.com/dsifry/metareview/internal/fsm/run"
)

// Issue #146: the repository-side covering-test search for testing-gap claims.
//
// The diff-only search (GapClaimEvidence) reads only added diff lines, so a pre-existing
// covering test outside every changed hunk produces no evidence and the judge can confirm
// a false absence claim without seeing the test that contradicts it. This search looks at
// the repository at the pinned head instead.
//
// The design rule from #140 carries over unchanged: this search never DECIDES a claim.
// A repository test file whose content references the claim's subject is evidence a judge
// weighs — a test that exists yet does not cover the claimed behavior still makes the
// claim true — and the same EvidenceFor admission bar and ranking apply. Evidence is
// injected; the verdict stays the judge's.
//
// Git never runs inside this package. GrepPaths and ShowHead are injected seams (the
// command-seam DI pattern); production wires them in internal/fsm/cli next to the
// escalation sandbox's showFile, tests inject fakes.

// GrepPaths returns the repository paths whose content at the pinned head matches the
// pattern (an RE2 alternation of the claim's subject tokens). It mirrors git grep -l -i -E:
// no matches is an empty slice, not an error.
type GrepPaths func(pattern string) ([]string, error)

// ShowHead returns one path's content at the pinned head. Absence (a path grep named
// that is not in the tree) is (nil, false, nil); a read failure is an error. The two
// must stay distinguishable — the showFile lesson: mapping every failure to "absent"
// biases the search toward confirming the claim, because a skipped candidate is one the
// judge never sees.
type ShowHead func(path string) ([]byte, bool, error)

// MaxRepoEvidenceLines bounds how much of one repository test file enters the search
// (and later the judge's context). A file capped here is still a scored candidate; the
// cap bounds prompt size, not judgment, like MaxGapEvidenceFiles.
const MaxRepoEvidenceLines = 200

// MaxRepoCandidates bounds how many candidate files are read after the grep. Ranking
// needs content, so the cap sits on reads; candidates beyond it are the alphabetically
// last ones (grep output is sorted, and the search re-sorts to be order-independent).
// The diff-side search needs no such cap because a diff is already bounded.
const MaxRepoCandidates = 64

// RepoEvidence is one finding's repository-side search result: the admitted covering-test
// evidence (the claimcheck type, so the audit trail and the eval share one shape) and the
// bounded content of exactly the admitted paths, for the context builder to inject.
type RepoEvidence struct {
	Evidence []claimcheck.Evidence
	Content  map[string]string
}

// RepoTestEvidence searches the repository at the pinned head for test files whose
// content references a testing-gap claim's subject, strongest first, capped at max
// (<= 0 means MaxGapEvidenceFiles — the same cap as the diff side, or the two searches
// describe different evidence).
//
// Two-stage, both bounded: grep recalls candidate paths (subject-token alternation,
// filtered to test-shaped paths, support paths and the finding's own file excluded —
// the same rules EvidenceFor applies), then each candidate's content is read and ranked
// by EvidenceFor over full-file blocks. Any seam error is returned: the caller falls
// back to diff-only evidence and says so in the disclosure. An infrastructure failure
// must never read as "we searched and found nothing".
func RepoTestEvidence(grep GrepPaths, show ShowHead, f run.Finding, max int) (RepoEvidence, error) {
	if grep == nil || show == nil {
		return RepoEvidence{}, nil
	}
	strong, weak := claimcheck.SubjectTokens(claimcheck.Finding{File: f.File, Line: f.Line, Text: f.IssueText})
	if len(strong)+len(weak) == 0 {
		return RepoEvidence{}, nil // nothing to search for
	}
	pattern := "(" + strings.Join(append(append([]string{}, strong...), weak...), "|") + ")"
	paths, err := grep(pattern)
	if err != nil {
		return RepoEvidence{}, err
	}
	own := strings.ToLower(strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(f.File, "./"), "a/"), "b/"))
	cands := make([]string, 0, len(paths))
	for _, p := range paths {
		if !claimcheck.IsTestPath(p) || claimcheck.IsSupportPath(p) {
			continue
		}
		key := strings.ToLower(strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(p, "./"), "a/"), "b/"))
		if own != "" && strings.EqualFold(key, own) {
			continue
		}
		cands = append(cands, p)
	}
	if len(cands) == 0 {
		return RepoEvidence{}, nil
	}
	sort.Strings(cands)
	if len(cands) > MaxRepoCandidates {
		cands = cands[:MaxRepoCandidates]
	}
	if max <= 0 {
		max = MaxGapEvidenceFiles
	}
	blocks := make([]claimcheck.Block, 0, len(cands))
	content := make([]string, len(cands))
	for i, p := range cands {
		body, ok, err := show(p)
		if err != nil {
			return RepoEvidence{}, err
		}
		if !ok {
			continue // grep matched, the tree does not carry it: a genuine absence
		}
		blocks = append(blocks, claimcheck.Block{Path: p, Added: boundedLines(body, MaxRepoEvidenceLines)})
		content[i] = boundedContent(body, MaxRepoEvidenceLines)
	}
	ev := claimcheck.EvidenceFor(blocks, claimcheck.Finding{File: f.File, Line: f.Line, Text: f.IssueText}, max)
	out := RepoEvidence{Evidence: ev, Content: map[string]string{}}
	for _, e := range ev {
		for i, p := range cands {
			if p == e.Path && content[i] != "" {
				out.Content[p] = content[i]
			}
		}
	}
	return out, nil
}

// boundedLines splits content to at most n lines for matching, and boundedContent to at
// most n lines (terminating newline included) for injection.
func boundedLines(body []byte, n int) []string {
	text := strings.ReplaceAll(string(body), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	// A trailing newline yields one empty final element; it is not a line.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > n {
		lines = lines[:n]
	}
	return lines
}

func boundedContent(body []byte, n int) string {
	return strings.Join(boundedLines(body, n), "\n") + "\n"
}
