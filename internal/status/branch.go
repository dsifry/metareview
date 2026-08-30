package status

import (
	"os/exec"
	"strings"

	"github.com/dsifry/metareview/internal/gitcontext"
	"github.com/dsifry/metareview/internal/reviewlog"
)

// BranchScope is the work in hand: the commits this branch adds over its base, and the files it
// changes. It is what makes the Stop hook usable.
//
// Unscoped, `status` answers over the whole review history — 73 blockers on this repository,
// the oldest from 2026-05 — so a hook wired to it refuses every session for work it never
// touched. That is a livelock, and a gate an operator must disable is not a gate. Scoping by
// target string was the first attempt and could not work: a review's target is a task id or the
// literal `current branch`, never a source path.
type BranchScope struct {
	Base    string
	Head    string
	Commits map[string]bool
	Files   []string
}

// RunGit is the git seam; nil uses the real binary.
type RunGit func(root string, args ...string) ([]byte, error)

func realGit(root string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...) // #nosec G204 -- args are literals and validated refs
	cmd.Dir = root
	return cmd.Output()
}

// ResolveBranchScope works out what this branch changed. base may be empty, in which case
// gitcontext resolves it the way every other command does (merge-base with main, then master,
// then HEAD~1) so the answer matches what a review would have been run against.
func ResolveBranchScope(root, base string, run RunGit) (BranchScope, error) {
	if run == nil {
		run = realGit
	}
	ctx, err := gitcontext.Collect(root, base)
	if err != nil {
		return BranchScope{}, err
	}
	s := BranchScope{Base: ctx.BaseSHA, Head: ctx.HeadSHA, Commits: map[string]bool{}, Files: ctx.ChangedFiles}
	// HEAD is always in scope, set BEFORE anything that can fail. rev-list excludes it on a merge
	// or an empty range, and an early return on rev-list failing used to skip this line entirely
	// — which left no commit in scope at all, so a review of the branch tip counted for nothing.
	if ctx.HeadSHA != "" {
		s.Commits[ctx.HeadSHA] = true
	}
	// Every commit the branch adds. A review of any of them is a review OF THIS WORK, which is
	// what lets an earlier attempt in the same chain still count.
	out, err := run(root, "rev-list", "--no-merges", ctx.BaseSHA+".."+ctx.HeadSHA)
	if err != nil {
		// rev-list is not the authority on whether the scope is usable — the changed files are,
		// and they came from gitcontext above.
		return s, nil
	}
	for _, line := range strings.Fields(string(out)) {
		s.Commits[line] = true
	}
	return s, nil
}

// InScope reports whether a review speaks to this branch: it reviewed one of the branch's own
// commits.
//
// Commit identity is the whole test, and "it read a file this branch also changed" deliberately
// is NOT. A review on another branch that happens to have read internal/foo.go looked at a
// different version of internal/foo.go, and letting it vouch for this one is exactly how a stale
// review clears current work — the failure this scoping exists to prevent, reintroduced one
// level down. A review of this branch's own commits is the only thing that has seen this code.
func (s BranchScope) InScope(r reviewlog.Summary) bool {
	return r.HeadSHA != "" && s.Commits[r.HeadSHA]
}

// Unreviewed lists the branch's changed files that no review in scope has read.
//
// This is the Completion Rule made mechanical: "before saying work is done, run the gate" means
// nothing if a file can be changed and never looked at. A review with no CoveredPaths — every
// review recorded before that field existed — cannot vouch for a file, so it does not.
func (s BranchScope) Unreviewed(logs []reviewlog.Summary) []string {
	reviewed := map[string]bool{}
	for _, r := range logs {
		if !s.InScope(r) || r.HasUnresolvedBlockers {
			// A review that is itself blocked has not cleared anything it read.
			continue
		}
		for _, p := range r.CoveredPaths {
			reviewed[p] = true
		}
	}
	out := []string{}
	for _, f := range s.Files {
		if !reviewed[f] {
			out = append(out, f)
		}
	}
	return out
}
