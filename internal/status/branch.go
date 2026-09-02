package status

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

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

// gitDeadline bounds this package's own git invocations. gitcontext bounds its own with
// gitcontext.Deadline, and that is where the stall risk actually is: CollectWithExcludes runs
// first and does the work that waits on index.lock.
//
// gitDeadline bounds a single git invocation.
//
// The Stop gate runs SYNCHRONOUSLY: the host waits for it before ending the session. With no
// deadline, a git subprocess that stalls — a stale index.lock, a filesystem that stops answering,
// a repository large enough for rev-list to crawl — holds session end for the host's whole command
// budget, and a hook the host then cancels renders no decision at all. Twenty seconds is far more
// than rev-list needs on any repository this reviews, and failing fast is safe: ResolveBranchScope
// treats a git error as an unresolvable scope, which now BLOCKS rather than passing.
// A var rather than a const so the timeout path itself can be tested: it is the branch that only
// runs when something has already gone wrong, which is exactly the branch that must not be taken
// on trust.
var gitDeadline = 20 * time.Second

func realGit(root string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitDeadline)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...) // #nosec G204 -- args are literals and validated refs
	cmd.Dir = root
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("git %s after %s: %w", args[0], gitDeadline, gitcontext.ErrTimeout)
	}
	return out, err
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
		// A TIMEOUT is not an ordinary rev-list failure. Ordinary failure is tolerated because
		// rev-list is not the authority on whether the scope is usable — the changed files are,
		// and they came from gitcontext above. A timeout means git is stalled, so the commit set
		// is not merely narrow but unknown, and an unknown scope must block rather than quietly
		// answer with fewer commits than the branch has. Swallowing it made the deadline
		// decorative: the error was discarded and the caller saw a partial scope with no error,
		// exactly as if nothing had gone wrong.
		// The sentinel first; the text as a fallback because RunGit is an exported seam and an
		// outside implementation cannot be expected to wrap our error type.
		if errors.Is(err, gitcontext.ErrTimeout) || strings.Contains(err.Error(), "timed out") {
			return BranchScope{}, err
		}
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
	inScope, _ := s.scopeRoute(r)
	return inScope
}

// InScopeByCommit reports whether the review examined one of THIS branch's commits, as opposed to
// merely naming a file the branch also touched. Only the first can vouch for a file's current
// contents, and keeping the two apart is the whole difference between a review and a coincidence.
func (s BranchScope) InScopeByCommit(r reviewlog.Summary) bool {
	_, byCommit := s.scopeRoute(r)
	return byCommit
}

// scopeRoute returns whether the review is in scope, and whether it got there by commit identity.
func (s BranchScope) scopeRoute(r reviewlog.Summary) (inScope, byCommit bool) {
	if r.HeadSHA != "" && s.Commits[r.HeadSHA] {
		return true, true
	}
	// A review OF a document this branch changed also speaks to this branch, whatever commit it
	// names. Commit identity alone silently dropped every artifact review: the scaffold is
	// normally the first thing on a branch, so the head it records is the branch BASE, which
	// rev-list base..HEAD excludes by construction. Their NOT_REVIEWED blockers vanished from the
	// scoped answer while the unscoped one still reported them — the same repository at the same
	// commit giving opposite answers, which is exactly the inversion this scoping was meant to
	// end.
	if r.Target == "" {
		return false, false
	}
	for _, f := range s.Files {
		if f == r.Target {
			return true, false
		}
	}
	return false, false
}

// Unreviewed lists the branch's changed files that no review in scope has read.
//
// This is the Completion Rule made mechanical: "before saying work is done, run the gate" means
// nothing if a file can be changed and never looked at. A review with no CoveredPaths — every
// review recorded before that field existed — cannot vouch for a file, so it does not.
func (s BranchScope) Unreviewed(logs []reviewlog.Summary) []string {
	reviewed := map[string]bool{}
	for _, r := range logs {
		inScope, byCommit := s.scopeRoute(r)
		if !inScope || r.HasUnresolvedBlockers {
			// A review that is itself blocked has not cleared anything it read.
			continue
		}
		if !r.CoveredPathsKnown {
			// It never said what it read, so it vouches for nothing. This is the flag's reason to
			// exist: "examined nothing" is an answer and "cannot say" is not, and treating them
			// alike is what let a log predating the field clear a file it never opened.
			continue
		}
		if !byCommit {
			// In scope only because it NAMES a file this branch changed — the artifact-review
			// route. Such a review examined some other commit, so it may answer for the document
			// it is about and for nothing else. Crediting its whole CoveredPaths set here is how a
			// stale review cleared current work: a PASS on another branch that had read
			// internal/foo.go marked this branch's rewritten internal/foo.go as reviewed, which is
			// precisely the failure the comment above says commit identity exists to prevent,
			// reintroduced by the exception added beneath it.
			reviewed[r.Target] = true
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

// ChangeKinds maps each changed path (vs base, through the working tree) to its git change type:
// "added", "modified", "deleted", or "renamed" (keyed on the NEW path). It is language-agnostic — used
// only to frame the review. A path the diff does not classify, and an untracked file (not in the diff),
// are left absent; the caller defaults those to "modified".
//
// Rename detection is git's, and it is a HEURISTIC: git may pair a deleted file with a similar-enough
// added file and report the pair as a rename. That is tolerable for framing — a rename and a delete+add
// both warrant the same impact review (references to the old name/path) — but it means the label is
// git's best guess, not ground truth.
func ChangeKinds(root, base string, run RunGit) map[string]string {
	kinds := map[string]string{}
	if run == nil {
		run = realGit
	}
	baseSHA := base
	if baseSHA == "" {
		scope, err := ResolveBranchScope(root, base, run)
		if err != nil {
			return kinds
		}
		baseSHA = scope.Base
	}
	out, err := run(root, "diff", "--name-status", "--find-renames", baseSHA)
	if err != nil {
		return kinds
	}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Split(line, "\t")
		if len(f) < 2 {
			continue
		}
		switch {
		case f[0] == "A":
			kinds[f[1]] = "added"
		case f[0] == "D":
			kinds[f[1]] = "deleted"
		case strings.HasPrefix(f[0], "R") && len(f) == 3:
			kinds[f[2]] = "renamed" // status like R100/R087; new path is the last field
		case strings.HasPrefix(f[0], "C") && len(f) == 3:
			kinds[f[2]] = "added" // a copy is new content at the new path
		default:
			kinds[f[len(f)-1]] = "modified"
		}
	}
	return kinds
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
