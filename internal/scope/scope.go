// Package scope decides which recorded obligations belong to the branch in hand (#177). One rule, shared by the
// abandoned-run scan (and, next, findings): an item recorded at commit H on branch N is in scope when N is the
// current branch — which survives rebase and amend — or when H lies in merge-base(HEAD, default base)..HEAD, which
// covers detached snapshots and stacked branches. Items recorded on a branch that still exists elsewhere belong to
// that branch; items whose branch is gone (merged and deleted, or never recorded) are orphaned.
//
// Why both legs: reachability alone lets `git rebase` silently clear a gate (the recorded head becomes unreachable),
// so routine git use would switch it off; the branch name alone misses detached snapshots and stacked work.
//
// Load makes a fixed number of git calls however many items are classified (#177 AC-4.9): the current branch, the
// fork point, one rev-list of the range into a set, and one listing of local branches.
package scope

import (
	"context"
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

// Load reads the branch in hand for the repository at root. It never fails the caller over git: when the branches
// cannot be listed at all (not a git repository) the Scope is unknown and Classify puts everything in scope; a
// readable repository with no fork point simply has an empty range, and the name leg still applies.
func Load(root string, git Runner) Scope {
	if git == nil {
		git = RealRunner
	}
	s := Scope{inRange: map[string]bool{}, branches: map[string]bool{}, root: root, git: git, ancestors: map[string]bool{}}
	if cur, err := git(root, "symbolic-ref", "--short", "-q", "HEAD"); err == nil {
		s.Current = cur
	}
	if base, ok, err := forkPoint(root); err == nil && ok {
		if out, err := git(root, "rev-list", base+"..HEAD"); err == nil {
			for _, sha := range strings.Fields(out) {
				s.inRange[sha] = true
			}
		}
	}
	if out, err := git(root, "for-each-ref", "--format=%(refname:short)", "refs/heads"); err == nil {
		s.known = true
		for _, name := range strings.Fields(out) {
			s.branches[name] = true
		}
	}
	return s
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

// reachable reports whether head is an ancestor of HEAD, one git call per distinct legacy head.
func (s Scope) reachable(head string) bool {
	if got, ok := s.ancestors[head]; ok {
		return got
	}
	_, err := s.git(s.root, "merge-base", "--is-ancestor", head, "HEAD")
	s.ancestors[head] = err == nil
	return err == nil
}
