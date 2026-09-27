package baseref

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type repo struct {
	t    *testing.T
	root string
}

func newRepo(t *testing.T) *repo {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r := &repo{t: t, root: root}
	r.git("init", "-q", "-b", "main")
	r.commitFile("base.txt", "base")
	return r
}

func (r *repo) git(args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = r.root
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (r *repo) commitFile(name, content string) string {
	r.t.Helper()
	if err := os.WriteFile(filepath.Join(r.root, name), []byte(content+"\n"), 0o644); err != nil {
		r.t.Fatal(err)
	}
	r.git("add", name)
	r.git("commit", "-q", "-m", name)
	return r.git("rev-parse", "HEAD")
}

// run adapts real git to a Runner: stdout, whether it exited 0, and no execution error.
func (r *repo) run(args ...string) (string, bool, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.root
	out, err := cmd.Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return "", false, nil
	}
	return strings.TrimSpace(string(out)), err == nil, err
}

// feature returns a repo on branch feat that forked from main at fork, after which main advanced to mainTip.
func feature(t *testing.T) (r *repo, fork, mainTip string) {
	r = newRepo(t)
	fork = r.git("rev-parse", "HEAD")
	r.git("checkout", "-q", "-b", "feat")
	r.commitFile("feat.txt", "feat")
	r.git("checkout", "-q", "main")
	mainTip = r.commitFile("other.txt", "main moved on")
	r.git("checkout", "-q", "feat")
	return r, fork, mainTip
}

// A branch name is the point this work forked from it — merge-base(HEAD, ref) — not its moving tip (#175).
func TestABranchNameResolvesToTheMergeBase(t *testing.T) {
	r, fork, mainTip := feature(t)
	r.git("update-ref", "refs/remotes/origin/main", mainTip)
	for _, ref := range []string{"main", "origin/main", "refs/heads/main", "refs/remotes/origin/main"} {
		got, err := Resolve(r.run, ref)
		if err != nil || got != fork {
			t.Errorf("Resolve(%q) = %s, %v; want the fork point %s, not the tip %s", ref, got, err, fork, mainTip)
		}
	}
}

// A commit or revision expression names an exact commit and is honoured as given (AC-3.3).
func TestAnExactRevisionIsHonoured(t *testing.T) {
	r, _, mainTip := feature(t)
	head := r.git("rev-parse", "HEAD")
	r.git("tag", "v1", mainTip)
	for ref, want := range map[string]string{
		mainTip:      mainTip,
		mainTip[:12]: mainTip,
		"HEAD":       head,
		"HEAD~1":     r.git("rev-parse", "HEAD~1"),
		"v1":         mainTip,
	} {
		if got, err := Resolve(r.run, ref); err != nil || got != want {
			t.Errorf("Resolve(%q) = %s, %v; want %s", ref, got, err, want)
		}
	}
}

// A branch whose name looks like a SHA prefix resolves as the branch — git's own precedence for a short name —
// while a full 40-hex SHA is always the commit (AC-3.4).
func TestAmbiguousNamesResolveDeterministically(t *testing.T) {
	r, fork, mainTip := feature(t)
	prefix := mainTip[:7]
	r.git("branch", prefix, "main")
	if got, err := Resolve(r.run, prefix); err != nil || got != fork {
		t.Errorf("a branch named %q resolves to its merge-base %s, got %s (%v)", prefix, fork, got, err)
	}
	r.git("branch", mainTip, "main") // git allows a 40-hex branch name; the SHA still wins
	if got, err := Resolve(r.run, mainTip); err != nil || got != mainTip {
		t.Errorf("a full SHA is the commit %s even when a branch shares the name, got %s (%v)", mainTip, got, err)
	}
}

// On the branch itself the merge-base is HEAD: an empty range, exactly as the tip gave before.
func TestTheBranchYouAreOnIsAnEmptyRange(t *testing.T) {
	r := newRepo(t)
	head := r.git("rev-parse", "HEAD")
	if got, err := Resolve(r.run, "main"); err != nil || got != head {
		t.Errorf("Resolve(main) on main = %s, %v; want HEAD %s", got, err, head)
	}
}

func TestUnresolvableBasesFail(t *testing.T) {
	r, _, _ := feature(t)
	if _, err := Resolve(r.run, "no-such-ref"); err == nil {
		t.Error("an unknown ref must fail")
	}
	// A branch with no common history has no merge-base: fail rather than silently use its tip.
	r.git("checkout", "-q", "--orphan", "island")
	r.commitFile("island.txt", "island")
	r.git("checkout", "-q", "feat")
	if _, err := Resolve(r.run, "island"); err == nil || strings.Contains(err.Error(), "shallow") {
		t.Errorf("a branch with no merge-base in a full clone must fail without a shallow-clone hint: %v", err)
	}
}

// An execution failure (a timeout, a missing git) is returned as is, never read as "not a branch".
func TestRunnerErrorsPropagate(t *testing.T) {
	boom := errors.New("boom")
	for _, failAt := range []string{"rev-parse", "show-ref", "merge-base"} {
		run := func(args ...string) (string, bool, error) {
			if args[0] == failAt {
				return "", false, boom
			}
			switch args[0] {
			case "rev-parse":
				return strings.Repeat("a", 40), true, nil
			case "show-ref":
				return "", true, nil
			}
			return strings.Repeat("b", 40), true, nil
		}
		if _, err := Resolve(run, "main"); !errors.Is(err, boom) {
			t.Errorf("failure in %s: err = %v, want boom", failAt, err)
		}
	}
}

// A tag that shares a branch's name must not bend the branch rule: rev-parse prefers refs/tags, so resolving the
// short name and then merge-basing would use the TAG's commit. The branch ref itself is resolved.
func TestABranchNamedLikeATagUsesTheBranch(t *testing.T) {
	r, fork, _ := feature(t)
	r.git("tag", "main", "feat") // a tag named main, pointing somewhere else entirely
	if got, err := Resolve(r.run, "main"); err != nil || got != fork {
		t.Errorf("Resolve(main) = %s, %v; want the branch's fork point %s", got, err, fork)
	}
}

// In a shallow clone the fork point is outside the fetched history; the error must say so and how to fix it.
func TestAShallowCloneNamesTheCause(t *testing.T) {
	origin, _, _ := feature(t)
	dir := filepath.Join(t.TempDir(), "clone")
	clone := exec.Command("git", "clone", "-q", "--depth", "1", "--branch", "feat", "file://"+origin.root, dir)
	if out, err := clone.CombinedOutput(); err != nil {
		t.Fatalf("clone: %v\n%s", err, out)
	}
	r := &repo{t: t, root: dir}
	r.git("fetch", "-q", "--depth", "1", "origin", "main:refs/remotes/origin/main")
	_, err := Resolve(r.run, "origin/main")
	if err == nil || !strings.Contains(err.Error(), "shallow") || !strings.Contains(err.Error(), "fetch-depth") {
		t.Fatalf("err = %v, want a shallow-clone hint", err)
	}
}
