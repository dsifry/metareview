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
)

func TestClassify(t *testing.T) {
	asked := 0
	git := func(_ string, args ...string) (string, error) { // merge-base --is-ancestor <head> HEAD
		asked++
		switch args[2] {
		case shaAncestor:
			return "", nil
		case shaBroken:
			return "", &exitError{code: 128} // a missing object proves nothing
		}
		return "", &exitError{code: 1}
	}
	s := Scope{Current: "feat", inRange: map[string]bool{"h-in-range": true}, branches: map[string]bool{"feat": true, "other": true}, known: true,
		git: git, ancestors: map[string]bool{}}
	for _, c := range []struct {
		branch, head string
		want         Class
	}{
		{"feat", "anything", InScope},         // the name leg survives rebase and amend
		{"", "h-in-range", InScope},           // detached snapshot / legacy run, by reachability
		{"feat-a", "h-in-range", InScope},     // stacked: feat-a's commit is in feat's range
		{"other", "h-elsewhere", OtherBranch}, // a live branch's own obligation
		{"gone", "h-elsewhere", Orphaned},     // merged and deleted
		{"", shaElse, Orphaned},               // no branch recorded, out of range and unreachable (git's own "no")
		{"", shaElse, Orphaned},               // asked once, then cached
		{"", shaAncestor, InScope},            // legacy, reachable from HEAD (the default branch has no range)
		{"", shaBroken, InScope},              // git could not answer: nothing shows it belongs elsewhere
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
	if asked != 3 {
		t.Errorf("each distinct well-formed legacy head is asked about once, asked %d", asked)
	}
	// An unreadable repository proves nothing belongs elsewhere: everything is in scope.
	if (Scope{}).Classify("anything", "x") != InScope {
		t.Error("an unknown scope must keep every item in scope")
	}
	if !isSHA(strings.Repeat("a", 64)) || isSHA(strings.Repeat("A", 40)) || isSHA("abc") {
		t.Error("isSHA accepts exactly lowercase 40- or 64-hex")
	}
}

// fakeGit answers Load's calls; fail names the call that errors (with its exit code, -1 for no exit): a subcommand,
// or "own" for the reflog's rev-list.
func fakeGit(calls *int, fail string, failCode int) Runner {
	return func(_ string, args ...string) (string, error) {
		*calls++
		name := args[0]
		if name == "rev-list" && args[1] != "base..HEAD" {
			name = "own"
		}
		if name == fail {
			if failCode < 0 {
				return "", errors.New("timed out")
			}
			return "", &exitError{code: failCode}
		}
		switch name {
		case "symbolic-ref":
			return "feat", nil
		case "rev-list":
			return "c1\nc2", nil
		case "reflog":
			return "r1\nr2\nshared", nil
		case "own": // the reflog heads the fork point cannot reach
			if strings.Join(args, " ") != "rev-list r1 r2 shared --not base --" {
				return "", errors.New("unexpected " + strings.Join(args, " "))
			}
			return "r1\nr2", nil
		case "rev-parse":
			return "rebase-merge/head-name\nrebase-apply/head-name", nil
		default:
			return "feat\nmain", nil
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
	if s.Current != "feat" || !s.inRange["c2"] || !s.inRange["r2"] || s.inRange["shared"] || !s.branches["main"] || !s.known {
		t.Fatalf("Load parsed %+v", s)
	}
	// No fork point (the default branch) is not a failure: an empty range and no reflog leg, the scope still known.
	forkPoint = func(string) (string, bool, error) { return "", false, nil }
	if s := Load("/repo", fakeGit(&calls, "", 0)); !s.known || s.inRange["c1"] || s.inRange["r1"] {
		t.Fatalf("no fork point leaves the range empty and the scope known: %+v", s)
	}
	// A branch with no reflog (a bare repository's default) adds nothing and asks nothing more.
	forkPoint = func(string) (string, bool, error) { return "base", true, nil }
	calls = 0
	noReflog := func(dir string, args ...string) (string, error) {
		if args[0] == "reflog" {
			calls++
			return "", nil
		}
		return fakeGit(&calls, "", 0)(dir, args...)
	}
	if s := Load("/repo", noReflog); !s.known || s.inRange["r1"] || calls != 4 {
		t.Fatalf("no reflog: %d calls, %+v", calls, s)
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
	}{{"symbolic-ref", 128}, {"symbolic-ref", -1}, {"rev-list", 128}, {"reflog", -1}, {"own", 128}, {"for-each-ref", 128}} {
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
	calls := 0
	forkPoint = func(string) (string, bool, error) { return "base", true, nil }
	if s := Load("/repo", fakeGit(&calls, "symbolic-ref", 1)); !s.known || s.Current != "feat" || !s.inRange["r1"] {
		t.Fatalf("mid-rebase the branch being rebased is current, with its reflog: %+v", s)
	}
	readFile = func(string) ([]byte, error) { return nil, os.ErrNotExist }
	if s := Load("/repo", fakeGit(&calls, "symbolic-ref", 1)); !s.known || s.Current != "" || s.inRange["r1"] {
		t.Fatalf("a plain detached HEAD has no current branch and no reflog leg: %+v", s)
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

// RealRunner against a real repository: the calls Load makes, the reflog leg surviving amend + rename, a mid-rebase
// HEAD, and a failing command's exit.
func TestLoadAgainstARealRepository(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	env := append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
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
	// Amend, then rename: the name and range legs both lose the run; the reflog carried by the rename keeps it.
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
