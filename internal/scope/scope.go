// Package scope decides which recorded obligations belong to the branch in hand (#177). One rule, shared by the
// abandoned-run scan (and, next, findings): an item recorded at commit H on branch N is in scope when N is the
// current branch — which survives rebase and amend — or when H lies in merge-base(HEAD, default base)..HEAD, which
// covers detached snapshots and stacked branches, or in the current branch's reflog, which survives a rewrite followed
// by a rename. Items recorded on a branch that still exists elsewhere belong to
// that branch; items whose branch is gone (merged and deleted, or never recorded) are orphaned.
//
// Why both legs: reachability alone lets `git rebase` silently clear a gate (the recorded head becomes unreachable),
// so routine git use would switch it off; the branch name alone misses detached snapshots and stacked work.
//
// Load makes a fixed number of git calls however many items are classified (#177 AC-4.9): the current branch, the
// fork point, one rev-list of the range into a set, the branch's reflog and the part of it that is the branch's own
// (one rev-list per 512 reflog heads), and one listing of local branches.
package scope

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dsifry/metareview/internal/fsm/gate"
	"github.com/dsifry/metareview/internal/gitcontext"
)

// Class is where a recorded item belongs relative to the branch in hand.
type Class int

const (
	// InScope items are this branch's obligations: they block.
	InScope Class = iota
	// OtherBranch items belong to a branch that still exists: they are that branch's, never a blocker here.
	OtherBranch
	// Orphaned items belong to no live branch (merged and deleted, or recorded without a branch and out of range).
	Orphaned
)

func (c Class) String() string {
	switch c {
	case InScope:
		return "in-scope"
	case OtherBranch:
		return "other-branch"
	default:
		return "orphaned"
	}
}

// Scope is the branch in hand, loaded once and consulted per item.
type Scope struct {
	// Current is the checked-out branch, empty on a detached HEAD.
	Current  string
	inRange  map[string]bool
	branches map[string]bool
	// known is false when the repository's branches could not be read (not a git repository): nothing can then be
	// shown to belong elsewhere, so everything is in scope — never a gate cleared by an unreadable repository.
	known bool
	// root and git answer the legacy leg: an item recorded before branches were (no branch) whose head is outside the
	// range is in scope when that head is reachable from HEAD. Asked per legacy item, cached; new items never ask.
	root      string
	git       Runner
	ancestors map[string]bool
}

// Runner runs one git command in dir and returns its trimmed stdout; a non-zero exit is an error. It is the seam the
// call-count test (AC-4.9) counts through.
type Runner func(dir string, args ...string) (string, error)

// RealRunner runs git exactly as the FSM does (gate.RealExec: GIT_* scrubbed, system config off).
func RealRunner(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, _, code, err := gate.RealExec(ctx, dir, nil, args...)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", &exitError{code: code}
	}
	return strings.TrimSpace(string(out)), nil
}

type exitError struct{ code int }

func (e *exitError) Error() string { return "git exited " + strconv.Itoa(e.code) }

// forkPoint is the seam over gitcontext.ForkPoint (merge-base(HEAD, main|master), false on the default branch).
var forkPoint = gitcontext.ForkPoint

// readFile is the seam over reading an in-progress rebase's head-name.
var readFile = os.ReadFile

// code is a git failure's exit code, or -1 when git did not run to an exit (a timeout, a spawn failure).
func code(err error) int {
	var e *exitError
	if errors.As(err, &e) {
		return e.code
	}
	return -1
}

// Load reads the branch in hand for the repository at root. It never fails the caller over git, and it never fails
// open: any git call that errors (other than git's own "no" — a detached HEAD) leaves the Scope unknown, and Classify
// then puts everything in scope, so a stalled or broken git can block too much but never clear a gate. A readable
// repository with no fork point simply has an empty range, and the other legs still apply.
func Load(root string, git Runner) Scope {
	if git == nil {
		git = RealRunner
	}
	s := Scope{inRange: map[string]bool{}, branches: map[string]bool{}, root: root, git: git, ancestors: map[string]bool{}}
	// Full refnames throughout: git's --short form turns ambiguous ("heads/feat") the moment a tag shares a branch's
	// name, which would unmatch the name leg.
	cur, err := git(root, "symbolic-ref", "-q", "HEAD")
	switch {
	case err == nil:
		s.Current = branchName(cur)
	case code(err) == 1: // detached — or mid-rebase, which is still the branch being rebased
		s.Current, err = rebasing(root, git)
		if err != nil {
			return s
		}
	default:
		return s
	}
	base, ok, err := forkPoint(root)
	if err != nil {
		return s
	}
	if ok {
		out, err := git(root, "rev-list", base+"..HEAD")
		if err != nil {
			return s
		}
		for _, sha := range strings.Fields(out) {
			s.inRange[sha] = true
		}
		if s.Current != "" {
			// The reflog leg: every head this branch has had, so a rebase or amend followed by `git branch -m` (which
			// carries the reflog along) still finds the run the branch was reviewing. Only the heads the fork point
			// cannot reach: the reflog starts where the branch was created, a commit its siblings share. A repository
			// that keeps no branch reflogs (a bare one's default) simply has none to add; with no fork point (the
			// default branch itself) there is no own work to tell apart, and the name leg stands alone.
			if err := s.addReflog(root, git, base); err != nil {
				return s
			}
		}
	}
	out, err := git(root, "for-each-ref", "--format=%(refname)", "refs/heads")
	if err != nil {
		return s
	}
	for _, ref := range strings.Fields(out) {
		s.branches[branchName(ref)] = true
	}
	s.known = true
	return s
}

// reflogBatch bounds one rev-list's argument list, so a long-lived branch's reflog never overruns ARG_MAX.
const reflogBatch = 512

// addReflog adds the current branch's past heads that base cannot reach. Heads only, never their ancestors, and never
// the head the branch was created at: that is where it forked from — main, or another feature branch it was stacked
// on — and it names that branch's work, not this one's. One reflog call and one rev-list per reflogBatch heads.
func (s Scope) addReflog(root string, git Runner, base string) error {
	out, err := git(root, "reflog", "show", "--format=%H %gs", "refs/heads/"+s.Current, "--")
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	var heads []string
	for _, line := range strings.Split(out, "\n") {
		sha, subject, _ := strings.Cut(strings.TrimSpace(line), " ")
		if sha == "" || seen[sha] || strings.HasPrefix(subject, "branch: Created from") {
			continue
		}
		seen[sha] = true
		heads = append(heads, sha)
	}
	for len(heads) > 0 {
		n := min(len(heads), reflogBatch)
		batch := heads[:n]
		heads = heads[n:]
		own, err := git(root, append(append([]string{"rev-list"}, batch...), "--not", base, "--")...)
		if err != nil {
			return err
		}
		for _, sha := range strings.Fields(own) {
			if seen[sha] {
				s.inRange[sha] = true
			}
		}
	}
	return nil
}

// branchName is a full refname's branch; anything that is not a local branch ("detached HEAD", which git writes as
// the head-name of a rebase begun detached) is no branch at all.
func branchName(ref string) string {
	if name, ok := strings.CutPrefix(strings.TrimSpace(ref), "refs/heads/"); ok {
		return name
	}
	return ""
}

// rebasing names the branch an in-progress rebase is rewriting ("" when HEAD is simply detached, or the rebase began
// detached): mid-rebase HEAD is detached, and the branch's own runs must keep blocking while an agent sits on a
// conflict.
func rebasing(root string, git Runner) (string, error) {
	out, err := git(root, "rev-parse", "--git-path", "rebase-merge/head-name", "--git-path", "rebase-apply/head-name")
	if err != nil {
		return "", err
	}
	for _, p := range strings.Split(out, "\n") {
		if p = strings.TrimSpace(p); p == "" {
			continue
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(root, p)
		}
		if b, err := readFile(p); err == nil {
			return branchName(string(b)), nil
		}
	}
	return "", nil
}

// Classify places an item recorded on branch (empty when none was recorded) at commit head. An item recorded before
// branches were (no branch) cannot be shown to belong elsewhere unless its head is known and unreachable from HEAD,
// so an upgrade never silently clears a legacy obligation — on the default branch (no fork point, an empty range)
// included.
func (s Scope) Classify(branch, head string) Class {
	switch {
	case !s.known:
		return InScope
	case branch != "" && branch == s.Current:
		return InScope
	case head != "" && s.inRange[head]:
		return InScope
	case branch == "" && (head == "" || s.reachable(head)):
		return InScope
	case branch != "" && s.branches[branch]:
		return OtherBranch
	default:
		return Orphaned
	}
}

// reachable reports whether head is an ancestor of HEAD, one git call per distinct legacy head. Only git's own "no"
// (exit 1), or a commit git no longer has, is unreachable: a malformed head or a failed call proves nothing, so it
// stays in scope.
func (s Scope) reachable(head string) bool {
	if got, ok := s.ancestors[head]; ok {
		return got
	}
	got := true
	if isSHA(head) {
		_, err := s.git(s.root, "merge-base", "--is-ancestor", head, "HEAD")
		got = err == nil || code(err) != 1
		if got && err != nil {
			// A commit git no longer has (pruned once nothing reached it) cannot be reachable: the only failure that
			// proves something. rev-parse's "no" is exit 1; any other failure still proves nothing.
			_, verr := s.git(s.root, "rev-parse", "--verify", "--quiet", head+"^{commit}")
			got = code(verr) != 1
		}
	}
	s.ancestors[head] = got
	return got
}

func isSHA(h string) bool {
	if len(h) != 40 && len(h) != 64 {
		return false
	}
	for _, c := range h {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
