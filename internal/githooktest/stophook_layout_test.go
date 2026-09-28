package githooktest

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
)

// The Stop hook (hooks/pre-finish.sh) against the real CLI, in every repository layout metareview supports (#177
// AC-4.11): a run abandoned on one branch blocks that branch and nothing else — in a single checkout switching
// branches, in a main checkout with a linked worktree, and in the bare-clone + worktrees layout.

// TestMain removes the CLI realBin built, so a run leaves nothing behind in the system temp directory.
func TestMain(m *testing.M) {
	code := m.Run()
	if builtBin != "" {
		_ = os.RemoveAll(filepath.Dir(builtBin))
	}
	os.Exit(code)
}

var (
	buildOnce sync.Once
	builtBin  string
	buildErr  error
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..")
}

// realBin builds cmd/metareview once per test binary.
func realBin(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "mrv-stophook-")
		if err != nil {
			buildErr = err
			return
		}
		builtBin = filepath.Join(dir, "metareview")
		c := exec.Command("go", "build", "-o", builtBin, "./cmd/metareview")
		c.Dir = repoRoot(t)
		if out, err := c.CombinedOutput(); err != nil {
			buildErr = &buildFailure{string(out)}
		}
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return builtBin
}

type buildFailure struct{ out string }

func (b *buildFailure) Error() string { return "go build: " + b.out }

// layoutEnv isolates git and metareview from the operator's machine: no global git config, a private data home.
func layoutEnv(t *testing.T, home string) []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") && !strings.HasPrefix(kv, "METAREVIEW_") && !strings.HasPrefix(kv, "XDG_DATA_HOME=") && !strings.HasPrefix(kv, "HOME=") {
			env = append(env, kv)
		}
	}
	// OPENAI_API_KEY: init checks the judge is configured; a run left at its first node never calls it.
	return append(env, "HOME="+home, "XDG_DATA_HOME="+filepath.Join(home, "xdg"), "GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1", "OPENAI_API_KEY=unused", "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e")
}

type layout struct {
	t   *testing.T
	env []string
	bin string
}

func (l layout) run(dir, name string, args ...string) (string, int) {
	l.t.Helper()
	c := exec.Command(name, args...)
	c.Dir = dir
	c.Env = l.env
	out, err := c.Output()
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			l.t.Fatalf("%s %v: %v", name, args, err)
		}
	}
	return strings.TrimSpace(string(out)), c.ProcessState.ExitCode()
}

func (l layout) git(dir string, args ...string) string {
	l.t.Helper()
	out, code := l.run(dir, "git", args...)
	if code != 0 {
		l.t.Fatalf("git %v in %s exited %d", args, dir, code)
	}
	return out
}

// abandon starts a review-loop in dir and leaves it at its first node.
func (l layout) abandon(dir string) string {
	l.t.Helper()
	out, code := l.run(dir, l.bin, "fsm", "init", "--workflow", "review-loop", "--base", "HEAD~1", "--var", "JUDGE=gpt-5.2", "--var", "JUDGE_EFFORT=medium")
	if code != 0 {
		l.t.Fatalf("fsm init in %s exited %d: %s", dir, code, out)
	}
	var got struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil || got.RunID == "" {
		l.t.Fatalf("fsm init output %q: %v", out, err)
	}
	if _, code := l.run(dir, l.bin, "fsm", "advance", "--run", got.RunID); code != 0 && code != 3 {
		l.t.Fatalf("fsm advance exited %d", code)
	}
	return got.RunID
}

// hook runs the Stop hook in dir and returns its stdout: empty passes, a JSON block decision blocks.
func (l layout) hook(dir string) string {
	l.t.Helper()
	c := exec.Command("bash", filepath.Join(repoRoot(l.t), "hooks", "pre-finish.sh"))
	c.Dir = dir
	c.Env = append(l.env, "METAREVIEW_BIN="+l.bin)
	c.Stdin = strings.NewReader("{}")
	out, _ := c.Output()
	return strings.TrimSpace(string(out))
}

// mustBlockOn asserts the Stop hook in dir blocks on an abandoned run of branch, and that the branch-scoped status the
// hook reads holds exactly the runs want — the hook's reason names only the first blocker, so the set is checked at
// the source.
func (l layout) mustBlockOn(dir, branch string, want ...string) {
	l.t.Helper()
	out := l.hook(dir)
	var d struct{ Decision, Reason string }
	if err := json.Unmarshal([]byte(out), &d); err != nil || d.Decision != "block" || !strings.Contains(d.Reason, "(branch "+branch+") [ABANDONED]") {
		l.t.Fatalf("the Stop hook in %s must block on branch %s's abandoned run, got %q", dir, branch, out)
	}
	if got := l.mustClear(dir); strings.Join(got, ",") != strings.Join(want, ",") {
		l.t.Fatalf("in %s must_clear holds runs %v, want exactly %v", dir, got, want)
	}
}

// mustClear is the run ids `status --json --scope branch` — the hook's query — says must be cleared in dir.
func (l layout) mustClear(dir string) []string {
	l.t.Helper()
	out, _ := l.run(dir, l.bin, "status", "--json", "--scope", "branch")
	var r struct {
		MustClear []struct {
			RunID string `json:"run_id"`
		} `json:"must_clear"`
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		l.t.Fatalf("status --json in %s: %v: %s", dir, err, out)
	}
	var ids []string
	for _, b := range r.MustClear {
		if b.RunID != "" {
			ids = append(ids, b.RunID)
		}
	}
	sort.Strings(ids)
	return ids
}

func (l layout) mustPass(dir string) {
	l.t.Helper()
	if out := l.hook(dir); out != "" {
		l.t.Fatalf("the Stop hook in %s must pass — another branch's run is not this branch's obligation — got %q", dir, out)
	}
}

func newLayout(t *testing.T) (layout, string) {
	if testing.Short() {
		t.Skip("builds and runs the real CLI")
	}
	home, _ := filepath.EvalSymlinks(t.TempDir())
	return layout{t: t, env: layoutEnv(t, home), bin: realBin(t)}, home
}

// seed makes a repository at dir on main with two commits (fsm init reviews HEAD~1..HEAD) and opts it into the gate.
func (l layout) seed(dir string) {
	l.t.Helper()
	l.git(dir, "init", "-q", "-b", "main")
	for _, f := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte(f+"\n"), 0o600); err != nil {
			l.t.Fatal(err)
		}
		l.git(dir, "add", f)
		l.git(dir, "commit", "-q", "-m", f)
	}
}

func (l layout) optIn(dir string) {
	l.t.Helper()
	if out, code := l.run(dir, l.bin, "setup", "--enable-stop-gate"); code != 0 {
		l.t.Fatalf("setup --enable-stop-gate exited %d: %s", code, out)
	}
}

func TestStopHookScopesAbandonedRunsInASingleCheckout(t *testing.T) {
	l, home := newLayout(t)
	repo := filepath.Join(home, "repo")
	if err := os.Mkdir(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	l.seed(repo)
	l.optIn(repo)
	l.git(repo, "checkout", "-q", "-b", "feat")
	id := l.abandon(repo)
	l.mustBlockOn(repo, "feat", id)
	l.git(repo, "checkout", "-q", "-b", "other", "main")
	l.mustPass(repo)
	l.git(repo, "checkout", "-q", "feat")
	l.mustBlockOn(repo, "feat", id)
}

func TestStopHookScopesAbandonedRunsAcrossLinkedWorktrees(t *testing.T) {
	l, home := newLayout(t)
	repo := filepath.Join(home, "repo")
	if err := os.Mkdir(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	l.seed(repo)
	l.optIn(repo)
	// The main checkout moves to a branch of its own with nothing unreviewed, so only runs can block it.
	l.git(repo, "checkout", "-q", "-b", "other")
	wt := filepath.Join(home, "wt")
	l.git(repo, "worktree", "add", "-q", "-b", "feat", wt, "main")
	id := l.abandon(wt)
	l.mustBlockOn(wt, "feat", id)
	l.mustPass(repo) // the main checkout is not on feat
	otherID := l.abandon(repo)
	l.mustBlockOn(repo, "other", otherID)
	l.mustBlockOn(wt, "feat", id) // exactly feat's: other's run never leaks into it
}

func TestStopHookScopesAbandonedRunsInABareLayout(t *testing.T) {
	l, home := newLayout(t)
	seed := filepath.Join(home, "seed")
	if err := os.Mkdir(seed, 0o700); err != nil {
		t.Fatal(err)
	}
	l.seed(seed)
	bare := filepath.Join(home, "repo.git")
	l.git(home, "clone", "-q", "--bare", seed, bare)
	mainWT, featWT := filepath.Join(home, "main"), filepath.Join(home, "feat")
	l.git(bare, "worktree", "add", "-q", "-b", "other", mainWT, "main") // nothing unreviewed: only runs can block
	l.git(bare, "worktree", "add", "-q", "-b", "feat", featWT, "main")
	l.optIn(mainWT)
	id := l.abandon(featWT)
	l.mustBlockOn(featWT, "feat", id)
	l.mustPass(mainWT)
	otherID := l.abandon(mainWT)
	l.mustBlockOn(mainWT, "other", otherID)
	l.mustBlockOn(featWT, "feat", id)
}
