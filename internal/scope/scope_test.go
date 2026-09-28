package scope

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const (
	shaAncestor = "1111111111111111111111111111111111111111"
	shaElse     = "2222222222222222222222222222222222222222"
	shaBroken   = "3333333333333333333333333333333333333333"
	shaPruned   = "4444444444444444444444444444444444444444"
)

func TestClassify(t *testing.T) {
	asked := 0
	git := func(_ string, args ...string) (string, error) { // merge-base --is-ancestor <head> HEAD; rev-parse --verify
		asked++
		if args[0] == "rev-parse" { // does git still have the commit?
			if args[3] == shaPruned+"^{commit}" {
				return "", &exitError{code: 1}
			}
			return "", &exitError{code: 128}
		}
		switch args[2] {
		case shaAncestor:
			return "", nil
		case shaBroken, shaPruned:
			return "", &exitError{code: 128}
		}
		return "", &exitError{code: 1}
	}
	s := Scope{Current: "feat", inRange: map[string]bool{"h-in-range": true}, branches: map[string]bool{"feat": true, "other": true}, known: true,
		former: map[string]bool{"feat-old": true, "other": true}, pastHeads: map[string]bool{"h-past": true}, git: git, ancestors: map[string]bool{}}
	for _, c := range []struct {
		branch, head string
		want         Class
	}{
		{"feat", "anything", InScope},         // the name leg survives rebase and amend
		{"feat-old", "anything", InScope},     // ... and a rename: the branch's former name is still its own
		{"other", "anything", OtherBranch},    // ... unless a live branch has taken the name since
		{"", "h-past", InScope},               // a legacy item at one of the branch's past heads
		{"other", "h-past", OtherBranch},      // a named item never takes the past-heads leg
		{"", "h-in-range", InScope},           // detached snapshot / legacy run, by reachability
		{"feat-a", "h-in-range", InScope},     // stacked: feat-a's commit is in feat's range
		{"other", "h-elsewhere", OtherBranch}, // a live branch's own obligation
		{"gone", "h-elsewhere", Orphaned},     // merged and deleted
		{"", shaElse, Orphaned},               // no branch recorded, out of range and unreachable (git's own "no")
		{"", shaElse, Orphaned},               // asked once, then cached
		{"", shaAncestor, InScope},            // legacy, reachable from HEAD (the default branch has no range)
		{"", shaBroken, InScope},              // git could not answer: nothing shows it belongs elsewhere
		{"", shaPruned, Orphaned},             // git no longer has the commit: nothing can reach it
		{"", "-not-a-sha", InScope},           // never handed to git, never cleared
		{"", "", InScope},                     // legacy with no head: nothing shows it belongs elsewhere
		{"other", shaAncestor, OtherBranch},   // a recorded branch never takes the legacy leg
	} {
		if got := s.Classify(c.branch, c.head); got != c.want {
			t.Errorf("Classify(%q,%q) = %v, want %v", c.branch, c.head, got, c.want)
		}
	}
	for c, want := range map[Class]string{InScope: "in-scope", OtherBranch: "other-branch", Orphaned: "orphaned"} {
		if c.String() != want {
			t.Errorf("%d.String() = %q", c, c.String())
		}
	}
	if asked != 6 {
		t.Errorf("each distinct well-formed legacy head is asked about once, asked %d", asked)
	}
	for branch, want := range map[string]bool{"feat": true, "feat-old": true, "other": false, "": false, "gone": false} {
		if s.Owns(branch) != want {
			t.Errorf("Owns(%q) = %v, want %v", branch, !want, want)
		}
	}
	if !s.PastHead("h-past") || s.PastHead("h-in-range") {
		t.Error("PastHead is the current branch's reflog heads only")
	}
	if (Scope{Current: "feat", former: map[string]bool{"feat-old": true}}).Owns("feat-old") {
		t.Error("an unknown scope owns no former name")
	}
	// An unreadable repository proves nothing belongs elsewhere: everything is in scope.
	if (Scope{}).Classify("anything", "x") != InScope {
		t.Error("an unknown scope must keep every item in scope")
	}
	if !isSHA(strings.Repeat("a", 64)) || isSHA(strings.Repeat("A", 40)) || isSHA("abc") {
		t.Error("isSHA accepts exactly lowercase 40- or 64-hex")
	}
}

// A mis-cased checkout (`git checkout Feat` for feat on a case-insensitive filesystem) is the branch git lists.
func TestCanonical(t *testing.T) {
	branches := map[string]bool{"feat": true, "Dup": true, "dup": true}
	for in, want := range map[string]string{"": "", "feat": "feat", "Feat": "feat", "FEAT": "feat", "DUP": "DUP", "gone": "gone"} {
		if got := Canonical(in, branches); got != want {
			t.Errorf("Canonical(%q) = %q, want %q", in, got, want)
		}
	}
	calls := 0
	git := func(dir string, args ...string) (string, error) {
		if args[0] == "symbolic-ref" {
			calls++
			return "refs/heads/Feat", nil
		}
		return fakeGit(&calls, "", 0)(dir, args...)
	}
	orig := forkPoint
	t.Cleanup(func() { forkPoint = orig })
	forkPoint = func(string) (string, bool, error) { return "", false, nil }
	if s := Load("/repo", git); s.Current != "feat" || !s.former["first"] {
		t.Fatalf("a mis-cased HEAD is the listed branch, with its reflog: %+v", s)
	}
}

// Only a configured remote's own main/master is a remote default branch — the remote name may contain a slash.
func TestRemoteDefaultRefs(t *testing.T) {
	refs := remoteDefaultRefs([]string{"origin", "team/alice"})
	for ref, want := range map[string]bool{
		"refs/remotes/origin/main": true, "refs/remotes/origin/master": true, "refs/remotes/team/alice/main": true,
		"refs/remotes/origin/alice/main": false, "refs/remotes/team/main": false, "refs/heads/main": false,
	} {
		if refs[ref] != want {
			t.Errorf("remote default %q = %v", ref, refs[ref])
		}
	}
	// The range leaves out exactly those refs.
	orig := forkPoint
	t.Cleanup(func() { forkPoint = orig })
	forkPoint = func(string) (string, bool, error) { return "base", true, nil }
	var rangeArgs string
	calls := 0
	git := func(dir string, args ...string) (string, error) {
		switch args[0] {
		case "for-each-ref":
			return "refs/heads/feat\nrefs/remotes/origin/main\nrefs/remotes/origin/alice/main\nrefs/remotes/origin/HEAD\nrefs/remotes/team/alice/master", nil
		case "rev-list":
			rangeArgs = strings.Join(args, " ")
		}
		return fakeGit(&calls, "", 0)(dir, args...)
	}
	Load("/repo", git)
	if rangeArgs != "rev-list base..HEAD --not refs/remotes/origin/main refs/remotes/team/alice/master --" {
		t.Fatalf("range args: %q", rangeArgs)
	}
}

// A HEAD spelled other than any listed branch is folded only when git resolves that spelling (a case-insensitive
// filesystem); an unborn branch is left alone, and a failing check leaves the scope unknown.
func TestLoadFoldsOnlyAResolvingSpelling(t *testing.T) {
	orig := forkPoint
	t.Cleanup(func() { forkPoint = orig })
	forkPoint = func(string) (string, bool, error) { return "", false, nil }
	for _, c := range []struct {
		verify  error
		current string
		known   bool
	}{{nil, "feat", true}, {&exitError{code: 1}, "Feat", true}, {&exitError{code: 128}, "Feat", false}} {
		calls := 0
		git := func(dir string, args ...string) (string, error) {
			switch args[0] {
			case "symbolic-ref":
				return "refs/heads/Feat", nil
			case "rev-parse":
				return "", c.verify
			}
			return fakeGit(&calls, "", 0)(dir, args...)
		}
		if s := Load("/repo", git); s.Current != c.current || s.known != c.known {
			t.Errorf("verify %v: Current %q known %v, want %q %v", c.verify, s.Current, s.known, c.current, c.known)
		}
	}
}

// fakeGit answers Load's calls; fail names the subcommand that errors (with its exit code, -1 for no exit).
func fakeGit(calls *int, fail string, failCode int) Runner {
	return func(_ string, args ...string) (string, error) {
		*calls++
		if args[0] == fail {
			if failCode < 0 {
				return "", errors.New("timed out")
			}
			return "", &exitError{code: failCode}
		}
		switch args[0] {
		case "symbolic-ref":
			return "refs/heads/feat", nil
		case "remote":
			return "origin\nteam/alice", nil
		case "rev-list":
			return "c1\nc2", nil
		case "reflog": // newest first
			return "r2 Branch: renamed refs/heads/feat-old to refs/heads/feat\n" +
				"r2 branch: renamed refs/heads/first to refs/heads/feat-old\n" +
				"r2 rebase (finish): refs/heads/first onto base\n" +
				"r1 commit: work\n" +
				"\n" + // a blank line is skipped
				"r1 Branch: renamed refs/tags/v1 to refs/heads/x\n" + // not a local branch: no former name
				"r1 Branch: renamed jgit-old to jgit-new\n" + // JGit writes short names
				"r1 Branch: copied refs/heads/orig to refs/heads/first\n" +
				"parent branch: Created from HEAD", nil
		case "rev-parse":
			return "rebase-merge/head-name\nrebase-apply/head-name", nil
		default:
			return "refs/heads/feat\nrefs/heads/main", nil
		}
	}
}

// AC-4.9: Load makes a fixed number of git calls, however many items are classified afterwards.
func TestLoadMakesAFixedNumberOfGitCalls(t *testing.T) {
	orig := forkPoint
	t.Cleanup(func() { forkPoint = orig })
	forkPoint = func(string) (string, bool, error) { return "base", true, nil }
	calls := 0
	s := Load("/repo", fakeGit(&calls, "", 0))
	for i := 0; i < 2000; i++ { // 2,000 runs over 50 branches
		s.Classify("b"+strconv.Itoa(i%50), "h"+strconv.Itoa(i))
	}
	if calls != 5 {
		t.Fatalf("Load must make exactly 5 git calls (plus the fork point), made %d", calls)
	}
	if s.Current != "feat" || !s.inRange["c2"] || s.inRange["r1"] || !s.pastHeads["r1"] || !s.pastHeads["parent"] ||
		!s.former["feat-old"] || !s.former["first"] || !s.former["orig"] || !s.former["jgit-old"] || len(s.former) != 4 || !s.branches["main"] || !s.known {
		t.Fatalf("Load parsed %+v", s)
	}
	// No fork point (the default branch) is not a failure: an empty range, the reflog still read, the scope known.
	forkPoint = func(string) (string, bool, error) { return "", false, nil }
	if s := Load("/repo", fakeGit(&calls, "", 0)); !s.known || s.inRange["c1"] || !s.former["first"] {
		t.Fatalf("no fork point leaves the range empty and the scope known: %+v", s)
	}
}

// A git call that fails never narrows the scope: it leaves it unknown, and everything blocks.
func TestLoadFailsClosed(t *testing.T) {
	orig := forkPoint
	t.Cleanup(func() { forkPoint = orig })
	forkPoint = func(string) (string, bool, error) { return "base", true, nil }
	for _, c := range []struct {
		fail string
		code int
	}{{"symbolic-ref", 128}, {"symbolic-ref", -1}, {"rev-list", 128}, {"reflog", -1}, {"remote", 128}, {"for-each-ref", 128}} {
		calls := 0
		if s := Load("/repo", fakeGit(&calls, c.fail, c.code)); s.known || s.Classify("other", "x") != InScope {
			t.Errorf("%s failing (%d) must leave the scope unknown, got %+v", c.fail, c.code, s)
		}
	}
	forkPoint = func(string) (string, bool, error) { return "", false, errors.New("git timed out") }
	calls := 0
	if s := Load("/repo", fakeGit(&calls, "", 0)); s.known {
		t.Error("a fork point that errors must leave the scope unknown")
	}
}

// Detached: git's "no" from symbolic-ref. Mid-rebase HEAD is detached too, and the branch being rebased is still the
// branch in hand; a plain detached HEAD has no name leg.
func TestLoadDetachedAndMidRebase(t *testing.T) {
	origFP, origRead := forkPoint, readFile
	t.Cleanup(func() { forkPoint, readFile = origFP, origRead })
	forkPoint = func(string) (string, bool, error) { return "", false, nil }
	readFile = func(p string) ([]byte, error) {
		if p == filepath.Join("/repo", "rebase-apply/head-name") {
			return []byte("refs/heads/feat\n"), nil
		}
		return nil, os.ErrNotExist
	}
	if BranchName("detached HEAD") != "" || BranchName("refs/tags/feat") != "" || BranchName("refs/heads/a/b\n") != "a/b" {
		t.Fatal("branchName keeps only local branches")
	}
	calls := 0
	if s := Load("/repo", fakeGit(&calls, "symbolic-ref", 1)); !s.known || s.Current != "feat" || !s.pastHeads["r1"] {
		t.Fatalf("mid-rebase the branch being rebased is current, with its reflog: %+v", s)
	}
	readFile = func(string) ([]byte, error) { return nil, os.ErrNotExist }
	if s := Load("/repo", fakeGit(&calls, "symbolic-ref", 1)); !s.known || s.Current != "" || s.pastHeads["r1"] {
		t.Fatalf("a plain detached HEAD has no current branch and no reflog read: %+v", s)
	}
	// rev-parse --git-path failing while detached leaves the scope unknown.
	broken := func(_ string, args ...string) (string, error) {
		if args[0] == "symbolic-ref" {
			return "", &exitError{code: 1}
		}
		return "", &exitError{code: 128}
	}
	if s := Load("/repo", broken); s.known {
		t.Fatal("an unreadable rebase state must leave the scope unknown")
	}
	// An absolute --git-path (GIT_DIR outside the checkout) is read as given, and blank lines are skipped.
	var read []string
	readFile = func(p string) ([]byte, error) { read = append(read, p); return nil, os.ErrNotExist }
	abs := func(_ string, args ...string) (string, error) {
		switch args[0] {
		case "symbolic-ref":
			return "", &exitError{code: 1}
		case "rev-parse":
			return "/abs/rebase-merge/head-name\n\n", nil
		}
		return "", nil
	}
	Load("/repo", abs)
	if len(read) != 1 || read[0] != "/abs/rebase-merge/head-name" {
		t.Fatalf("read %q", read)
	}
}

// RealRunner against a real repository: the calls Load makes, a former name surviving amend + rename, a mid-rebase
// HEAD, and a failing command's exit.
func TestLoadAgainstARealRepository(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	var env []string // no inherited GIT_*: a hook's GIT_DIR would aim every call below at another repository
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			env = append(env, kv)
		}
	}
	env = append(env, "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "GIT_EDITOR=true")
	gitIn := func(args ...string) error {
		c := exec.Command("git", args...)
		c.Dir = root
		c.Env = env
		_, err := c.CombinedOutput()
		return err
	}
	run := func(args ...string) {
		t.Helper()
		if err := gitIn(args...); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, "f.txt"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		run("add", "f.txt")
	}
	run("init", "-q", "-b", "main")
	run("commit", "-q", "--allow-empty", "-m", "base")
	run("checkout", "-q", "-b", "feat")
	run("commit", "-q", "--allow-empty", "-m", "work")
	head, err := RealRunner(root, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	s := Load(root, nil)
	if s.Current != "feat" || !s.inRange[head] || !s.branches["main"] || !s.branches["feat"] || !s.known {
		t.Fatalf("Load(real repo) = %+v", s)
	}
	// Amend, then rename: the range loses the run; the rename entry its reflog carries keeps the old name its own.
	run("commit", "-q", "--amend", "--allow-empty", "-m", "work v2")
	run("branch", "-m", "feat", "feat-v2")
	if got := Load(root, nil).Classify("feat", head); got != InScope {
		t.Fatalf("an amended and renamed branch must still own its run, got %v", got)
	}
	// Mid-rebase: HEAD detached on a conflict, the branch being rebased is still current.
	write("a\n")
	run("commit", "-q", "-m", "f on feat-v2")
	run("checkout", "-q", "main")
	write("b\n")
	run("commit", "-q", "-m", "f on main")
	run("checkout", "-q", "feat-v2")
	if err := gitIn("rebase", "main"); err == nil {
		t.Fatal("setup: the rebase must stop on a conflict")
	}
	if s := Load(root, nil); s.Current != "feat-v2" || !s.known || s.Classify("feat", head) != InScope {
		t.Fatalf("mid-rebase the rebased branch keeps its runs: %+v", s)
	}
	if _, err := RealRunner(root, "rev-parse", "--verify", "no-such-ref"); err == nil || !strings.Contains(err.Error(), "git exited") {
		t.Fatalf("a failing git command is an error naming its exit, got %v", err)
	}
	if _, err := RealRunner(filepath.Join(root, "missing-dir"), "status"); err == nil || code(err) != -1 {
		t.Fatalf("git that cannot start in a missing dir is an error with no exit code, got %v", err)
	}
}
