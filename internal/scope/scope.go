// Package scope decides which recorded obligations belong to the branch in hand (#177). One rule, shared by the
// abandoned-run scan and the findings ledger (#178, findings.ScopedBlocking): an item recorded at commit H on branch N is in scope when N is the
// current branch or one of its former names (a `git branch -m`, or `-c`, its reflog records, while no live branch
// holds that name) — which survives rebase, amend and rename — or when H lies in merge-base(HEAD, default base)..HEAD, which covers detached snapshots and stacked
// branches. An item recorded before branches were (no N) is in scope unless git shows its head belongs nowhere here:
// not in the range, not one of the current branch's past heads, and unreachable from HEAD (or pruned). Items recorded
// on a branch that still exists elsewhere belong to that branch; the rest are orphaned.
//
// Why both legs: reachability alone lets `git rebase` silently clear a gate (the recorded head becomes unreachable),
// so routine git use would switch it off; the branch name alone misses detached snapshots and stacked work.
//
// Load makes a fixed number of git calls however many items are classified (#177 AC-4.9): the current branch (and,
// on a detached HEAD, where a rebase keeps its head-name; on a mis-spelled HEAD, whether it resolves), the configured
// remotes, one listing of local and remote branches, the fork point, one rev-list of the range into a set, and the current branch's reflog. Only legacy items ask more: up to two calls per
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
	// known is false when any git call Load makes failed (not a git repository, a timeout, a broken ref): nothing can
	// then be shown to belong elsewhere, so everything is in scope — never a gate cleared by an unreadable repository.
	known bool
	// former holds the current branch's earlier names, from the rename and copy entries its reflog carries; Classify
	// counts one only while no live branch holds it.
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
	// No promisor fetches: a partial clone would otherwise go to the network for a pruned legacy head.
	out, _, code, err := gate.RealExec(ctx, dir, []string{"GIT_NO_LAZY_FETCH=1"}, args...)
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
	// Local branches, and the remote default branches (for each configured remote — a remote name may itself contain a
	// slash — the branch its HEAD names, and its main and master) whose commits the range leaves out.
	remotes, err := git(root, "remote")
	if err != nil {
		return s
	}
	out, err := git(root, "for-each-ref", "--format=%(refname) %(symref)", "refs/heads", "refs/remotes")
	if err != nil {
		return s
	}
	refs := map[string]string{} // refname -> the ref it points at, for a symbolic ref (a remote's HEAD)
	var order []string
	for _, line := range strings.Split(out, "\n") {
		ref, target, _ := strings.Cut(strings.TrimSpace(line), " ")
		if ref == "" {
			continue
		}
		refs[ref] = strings.TrimSpace(target)
		order = append(order, ref)
	}
	defaults := remoteDefaultRefs(strings.Fields(remotes), refs)
	var remoteDefaults []string
	for _, ref := range order {
		if name := branchName(ref); name != "" {
			s.branches[name] = true
		} else if defaults[ref] {
			remoteDefaults = append(remoteDefaults, ref)
		}
	}
	if s.Current != "" && !s.branches[s.Current] {
		// HEAD spelled other than any listed branch: on a case-insensitive filesystem `git checkout Feat` lands on
		// feat with HEAD spelled Feat, and the spelling resolves. Where it does not resolve (an unborn branch — on a
		// case-sensitive filesystem Feat is its own branch) it is not folded.
		switch _, err := git(root, "rev-parse", "--verify", "--quiet", "refs/heads/"+s.Current); {
		case err == nil:
			s.Current = Canonical(s.Current, s.branches)
		case code(err) != 1:
			return s
		}
	}
	base, ok, err := forkPoint(root)
	if err != nil {
		return s
	}
	if ok {
		// Never what a remote default branch already has: a branch cut from a fresh origin/main while local main lags
		// would otherwise take in every merged branch's commits — and their runs.
		args := append([]string{"rev-list", base + "..HEAD", "--not"}, remoteDefaults...)
		out, err := git(root, append(args, "--")...)
		if err != nil {
			return s
		}
		for _, sha := range strings.Fields(out) {
			s.inRange[sha] = true
		}
	}
	if s.Current != "" && s.branches[s.Current] {
		// The branch's reflog: its former names, so a rewrite followed by `git branch -m` (or `-c` then deleting the
		// original — both carry the reflog along) still owns the runs of the name it had; and its past heads, for
		// legacy items. A repository that keeps no branch reflogs (a bare one's default) simply has none; an unborn
		// branch (`checkout --orphan`, before its first commit) has no ref to read.
		if err := s.readReflog(root, git); err != nil {
			return s
		}
	}
	s.known = true
	return s
}

// readReflog reads the current branch's reflog: every head it has had, and every name it was renamed or copied from.
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
		// "Branch: renamed refs/heads/<old> to refs/heads/<new>", or "copied" — git's own messages; the case has varied.
		for _, verb := range []string{"branch: renamed ", "branch: copied "} {
			if rest, ok := cutPrefixFold(subject, verb); ok {
				from, _, _ := strings.Cut(rest, " to ")
				if name := formerName(from); name != "" {
					s.former[name] = true
				}
			}
		}
	}
	return nil
}

// remoteDefaultRefs is every configured remote's own default branches: the branch its refs/remotes/<r>/HEAD points at
// (what `git clone` and `git remote set-head` record), and always its main and master. Names
// are spelled exactly: refs/remotes/origin/alice/main is a namespaced branch on origin, not a default branch, while a
// remote named team/alice has refs/remotes/team/alice/main. A HEAD that points outside its own remote proves nothing and
// is ignored. listed maps each listed ref to its symref target (empty for an ordinary ref).
func remoteDefaultRefs(remotes []string, listed map[string]string) map[string]bool {
	refs := map[string]bool{}
	for _, r := range remotes {
		own := "refs/remotes/" + r + "/"
		if head := listed[own+"HEAD"]; strings.HasPrefix(head, own) && head != own+"HEAD" {
			refs[head] = true
		}
		// main and master stay excluded beside it: a HEAD recorded before a master-to-main rename, or naming develop in
		// a gitflow repository, must not pull a fresh origin/main's merged work back into the range.
		refs[own+"main"] = true
		refs[own+"master"] = true
	}
	return refs
}

// Canonical is name as git lists the branch. On a case-insensitive filesystem `git checkout Feat` resolves the loose
// ref refs/heads/feat yet leaves HEAD spelled refs/heads/Feat; recording or comparing that spelling would never match
// the branch again. A name with exactly one case-insensitive match among branches takes its spelling; anything else is
// returned as is.
func Canonical(name string, branches map[string]bool) string {
	if name == "" || branches[name] {
		return name
	}
	match := ""
	for b := range branches {
		if strings.EqualFold(b, name) {
			if match != "" {
				return name
			}
			match = b
		}
	}
	if match == "" {
		return name
	}
	return match
}

// formerName is the branch a rename or copy entry names: git writes refs/heads/<name>; JGit-based tools write the
// short name. Any other ref (refs/remotes/..., refs/tags/...) is no branch.
func formerName(ref string) string {
	if name := branchName(ref); name != "" || strings.HasPrefix(ref, "refs/") {
		return name
	}
	return strings.TrimSpace(ref)
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
	// A former name is this branch's only while no live branch holds it: a new branch that reuses the name owns
	// what is recorded under it.
	case s.Owns(branch):
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

// Owns reports whether an item recorded on branch is the branch in hand's by name alone: the current branch, or one of
// its former names that no live branch holds (Classify's name leg). A writer that keeps one row per branch uses it to
// tell its own rows from another branch's without the range leg, which also takes in a stacked lower branch's items.
// An unknown scope owns nothing but the current branch.
func (s Scope) Owns(branch string) bool {
	return branch != "" && (branch == s.Current || s.known && s.former[branch] && !s.branches[branch])
}

// Known reports whether Load could read the branch in hand; an unknown scope keeps everything in scope.
func (s Scope) Known() bool { return s.known }

// PastHead reports whether head is one the current branch has had (its reflog): proof the commit was this branch's.
func (s Scope) PastHead(head string) bool { return s.pastHeads[head] }

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
