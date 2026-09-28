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

func TestClassify(t *testing.T) {
	asked := 0
	git := func(_ string, args ...string) (string, error) { // merge-base --is-ancestor <head> HEAD
		asked++
		if args[2] == "h-ancestor" {
			return "", nil
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
		{"", "h-elsewhere", Orphaned},         // no branch recorded, out of range and unreachable
		{"", "h-elsewhere", Orphaned},         // asked once, then cached
		{"", "h-ancestor", InScope},           // legacy, reachable from HEAD (the default branch has no range)
		{"", "", InScope},                     // legacy with no head: nothing shows it belongs elsewhere
		{"other", "h-ancestor", OtherBranch},  // a recorded branch never takes the legacy leg
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
	if asked != 2 {
		t.Errorf("each distinct legacy head is asked about once, asked %d", asked)
	}
	// An unreadable repository proves nothing belongs elsewhere: everything is in scope.
	if (Scope{}).Classify("anything", "x") != InScope {
		t.Error("an unknown scope must keep every item in scope")
	}
}

// AC-4.9: Load makes a fixed number of git calls, however many items are classified afterwards.
func TestLoadMakesAFixedNumberOfGitCalls(t *testing.T) {
	orig := forkPoint
	t.Cleanup(func() { forkPoint = orig })
	forkPoint = func(string) (string, bool, error) { return "base", true, nil }
	calls := 0
	git := func(_ string, args ...string) (string, error) {
		calls++
		switch args[0] {
		case "symbolic-ref":
			return "feat", nil
		case "rev-list":
			return "c1\nc2", nil
		default:
			return "feat\nmain", nil
		}
	}
	s := Load("/repo", git)
	for i := 0; i < 2000; i++ { // 2,000 runs over 50 branches
		s.Classify("b"+strconv.Itoa(i%50), "h"+strconv.Itoa(i))
	}
	if calls != 3 {
		t.Fatalf("Load must make exactly 3 git calls (plus the fork point), made %d", calls)
	}
	if s.Current != "feat" || !s.inRange["c2"] || !s.branches["main"] || !s.known {
		t.Fatalf("Load parsed %+v", s)
	}
	// Failures degrade, never fail: no branch, no fork point, no range, no branch list.
	forkPoint = func(string) (string, bool, error) { return "", false, errors.New("no fork point") }
	if s := Load("/repo", func(string, ...string) (string, error) { return "", errors.New("boom") }); s.Current != "" || len(s.inRange) != 0 || len(s.branches) != 0 || s.known {
		t.Fatalf("a repository git cannot read yields an empty scope, got %+v", s)
	}
}

// RealRunner against a real repository: the three calls Load makes, and a failing command's exit.
func TestLoadAgainstARealRepository(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	run := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = root
		c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t", "GIT_CONFIG_GLOBAL=/dev/null")
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
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
	if s.Current != "feat" || !s.inRange[head] || !s.branches["main"] || !s.branches["feat"] {
		t.Fatalf("Load(real repo) = %+v", s)
	}
	if _, err := RealRunner(root, "rev-parse", "--verify", "no-such-ref"); err == nil || !strings.Contains(err.Error(), "git exited") {
		t.Fatalf("a failing git command is an error naming its exit, got %v", err)
	}
	if _, err := RealRunner(filepath.Join(root, "missing-dir"), "status"); err == nil {
		t.Fatal("git that cannot start in a missing dir is an error")
	}
}
