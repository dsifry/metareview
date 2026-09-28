// Package scope decides which recorded obligations belong to the branch in hand (#177). One rule, shared by the
// abandoned-run scan (and, next, findings): an item recorded at commit H on branch N is in scope when N is the
// current branch or one of its former names (a `git branch -m` its reflog records) — which survives rebase, amend and
// rename — or when H lies in merge-base(HEAD, default base)..HEAD, which covers detached snapshots and stacked
// branches. An item recorded before branches were (no N) is in scope unless git shows its head belongs nowhere here:
// not in the range, not one of the current branch's past heads, and unreachable from HEAD (or pruned). Items recorded
// on a branch that still exists elsewhere belong to that branch; the rest are orphaned.
//
// Why both legs: reachability alone lets `git rebase` silently clear a gate (the recorded head becomes unreachable),
// so routine git use would switch it off; the branch name alone misses detached snapshots and stacked work.
//
// Load makes a fixed number of git calls however many items are classified (#177 AC-4.9): the current branch (and,
// on a detached HEAD, where a rebase keeps its head-name), the fork point, one rev-list of the range into a set, the
// current branch's reflog, and one listing of local branches. Only legacy items ask more: up to two calls per
// distinct legacy head, cached.
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
	// former holds the current branch's earlier names, from the rename entries its reflog carries.
	former map[string]bool
	// pastHeads holds every head the current branch's reflog records: a legacy item at one of them was this branch's.
	pastHeads map[string]bool
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
	s := Scope{inRange: map[string]bool{}, branches: map[string]bool{}, former: map[string]bool{}, pastHeads: map[string]bool{},
		root: root, git: git, ancestors: map[string]bool{}}
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
	}
	if s.Current != "" {
		// The branch's reflog: its former names, so a rewrite followed by `git branch -m` (which carries the reflog
		// along) still owns the runs of the name it had; and its past heads, for legacy items. A repository that keeps
		// no branch reflogs (a bare one's default) simply has none.
		if err := s.readReflog(root, git); err != nil {
			return s
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

// readReflog reads the current branch's reflog: every head it has had, and every name it was renamed from.
func (s Scope) readReflog(root string, git Runner) error {
	out, err := git(root, "reflog", "show", "--format=%H %gs", "refs/heads/"+s.Current, "--")
	if err != nil {
		return err
	}
	for _, line := range strings.Split(out, "\n") {
		sha, subject, _ := strings.Cut(strings.TrimSpace(line), " ")
		if sha == "" {
			continue
		}
		s.pastHeads[sha] = true
		// "Branch: renamed refs/heads/<old> to refs/heads/<new>" — git's own message; the case has varied.
		if rest, ok := cutPrefixFold(subject, "branch: renamed "); ok {
			from, _, _ := strings.Cut(rest, " to ")
			if name := branchName(from); name != "" {
				s.former[name] = true
			}
		}
	}
	return nil
}

func cutPrefixFold(s, prefix string) (string, bool) {
	if len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix) {
		return s[len(prefix):], true
	}
	return "", false
}

// BranchName is a full refname's branch; anything that is not a local branch ("detached HEAD", which git writes as
// the head-name of a rebase begun detached) is no branch at all. fsm init records branches by the same rule.
func BranchName(ref string) string {
	return branchName(ref)
}

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
	case branch != "" && (branch == s.Current || s.former[branch]):
		return InScope
	case head != "" && s.inRange[head]:
		return InScope
	case branch == "" && (head == "" || s.pastHeads[head] || s.reachable(head)):
		return InScope
	case branch != "" && s.branches[branch]:
		return OtherBranch
	default:
		return Orphaned
	}
}

// reachable reports whether head is an ancestor of HEAD: one git call per distinct legacy head, and a second when
// that one fails. Only git's own "no" (exit 1), or a commit git no longer has, is unreachable: a malformed head or a
// failed call proves nothing, so it stays in scope.
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
