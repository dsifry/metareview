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
	// The SAME exclusions the reviews use. A review's covered paths come from an exclude-filtered
	// context, so metareview's own generated artifacts — .metareview/** and docs/metareview/** —
	// can never appear in one. Enumerating the unfiltered set here meant every committed review
	// log and context pack was permanently unreviewed, and each new review committed three more
	// files that no future review could ever clear. On a repository that commits its review
	// artifacts, which CLAUDE.md requires, the branch scope could never reach a clean state: the
	// exact livelock this scoping exists to prevent, one level down.
	ctx, err := gitcontext.CollectWithExcludes(root, base, GeneratedMetareviewPathExcludes())
	if err != nil {
		return BranchScope{}, err
	}
	// Committed changes AND uncommitted ones. ChangedFiles is `git diff --name-only base..HEAD`,
	// so staged, working-tree and untracked files were invisible — and a Stop hook fires exactly
	// when an agent is about to finish, which is the moment work is most likely to be written and
	// not yet committed. `git add newfeature.go` with no commit made the hook emit nothing and
	// exit 0. The unscoped query it replaced at least failed closed; this default failed open on
	// the single most common state at Stop time.
	files := append([]string(nil), ctx.ChangedFiles...)
	files = appendUnseen(files, ctx.StagedFiles, ctx.WorkingTreeFiles, ctx.UntrackedFiles)
	s := BranchScope{Base: ctx.BaseSHA, Head: ctx.HeadSHA, Commits: map[string]bool{}, Files: files}
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
	if r.HeadSHA != "" && s.Commits[r.HeadSHA] {
		return true
	}
	// A review OF a document this branch changed also speaks to this branch, whatever commit it
	// names. Commit identity alone silently dropped every artifact review: the scaffold is
	// normally the first thing on a branch, so the head it records is the branch BASE, which
	// rev-list base..HEAD excludes by construction. Their NOT_REVIEWED blockers vanished from the
	// scoped answer while the unscoped one still reported them — the same repository at the same
	// commit giving opposite answers, which is exactly the inversion this scoping was meant to
	// end.
	if r.Target == "" {
		return false
	}
	for _, f := range s.Files {
		if f == r.Target {
			return true
		}
	}
	return false
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
		if !r.CoveredPathsKnown {
			// It never said what it read, so it vouches for nothing. This is the flag's reason to
			// exist: "examined nothing" is an answer and "cannot say" is not, and treating them
			// alike is what let a log predating the field clear a file it never opened.
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

// GeneratedMetareviewPathExcludes are the paths metareview itself writes. They are excluded from
// a review's source context, so nothing can ever record having reviewed them, so they must be
// excluded from the set a review is measured against too. Exported and defined once: it was
// already duplicated privately in the review packages, and this file needing a fourth copy is
// what made the divergence visible.
func GeneratedMetareviewPathExcludes() []string {
	return []string{".metareview", ".metareview/**", "docs/metareview", "docs/metareview/**"}
}

// appendUnseen adds every path not already present, preserving order so the report reads the same
// way twice. Uncommitted paths are excluded the same way committed ones are: metareview's own
// generated artifacts can never appear in a review's covered paths, so counting them would
// livelock the scope.
func appendUnseen(base []string, more ...[]string) []string {
	seen := make(map[string]bool, len(base))
	for _, p := range base {
		seen[p] = true
	}
	for _, list := range more {
		for _, p := range list {
			if p == "" || seen[p] || isGeneratedMetareviewPath(p) {
				continue
			}
			seen[p] = true
			base = append(base, p)
		}
	}
	return base
}

// isGeneratedMetareviewPath reports whether a path is one metareview itself writes. The exclude
// globs are applied by git for the committed set; uncommitted paths never pass through git's
// pathspec, so the same rule is applied here.
func isGeneratedMetareviewPath(p string) bool {
	for _, prefix := range []string{".metareview/", "docs/metareview/"} {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return p == ".metareview" || p == "docs/metareview"
}
