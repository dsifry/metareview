package githooktest

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
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
		if !strings.HasPrefix(kv, "GIT_") && !strings.HasPrefix(kv, "METAREVIEW_") && !strings.HasPrefix(kv, "XDG_DATA_HOME=") && !strings.HasPrefix(kv, "XDG_CACHE_HOME=") && !strings.HasPrefix(kv, "HOME=") && !strings.HasPrefix(kv, "TMPDIR=") {
			env = append(env, kv)
		}
	}
	// OPENAI_API_KEY: init checks the judge is configured; a run left at its first node never calls it.
	// TMPDIR: private, so the Stop hook's loop-guard state (mr-j30) never leaks between tests or runs.
	tmp := filepath.Join(home, "tmp")
	_ = os.MkdirAll(tmp, 0o700)
	return append(env, "HOME="+home, "XDG_DATA_HOME="+filepath.Join(home, "xdg"), "TMPDIR="+tmp, "GIT_CONFIG_GLOBAL=/dev/null",
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

// runStopHook runs the Stop hook in dir with payload on stdin and returns its stdout and stderr.
func (l layout) runStopHook(dir, payload string, extra ...string) (string, string) {
	l.t.Helper()
	c := exec.Command("bash", filepath.Join(repoRoot(l.t), "hooks", "pre-finish.sh"))
	c.Dir = dir
	c.Env = append(append(append([]string{}, l.env...), "METAREVIEW_BIN="+l.bin), extra...)
	c.Stdin = strings.NewReader(payload)
	var out, errText strings.Builder
	c.Stdout, c.Stderr = &out, &errText
	_ = c.Run()
	return strings.TrimSpace(out.String()), strings.TrimSpace(errText.String())
}

func (l layout) hookStderr(dir, payload string) (string, string) { return l.runStopHook(dir, payload) }

func blockDecision(out string) bool {
	var d struct{ Decision string }
	return json.Unmarshal([]byte(out), &d) == nil && d.Decision == "block"
}

// blockedRepo seeds a repository on branch feat with an abandoned run — a blocker its session cannot
// clear by itself — so the Stop hook refuses it.
func (l layout) blockedRepo(home string) string {
	l.t.Helper()
	repo := filepath.Join(home, "repo")
	if err := os.Mkdir(repo, 0o700); err != nil {
		l.t.Fatal(err)
	}
	l.seed(repo)
	l.optIn(repo)
	l.git(repo, "checkout", "-q", "-b", "feat")
	l.abandon(repo)
	return repo
}

// mr-j30: a host that never sets `stop_hook_active` (Codex reports it false during hook-driven
// continuations) must still not livelock — the hook blocks a few times, then stands down loudly on
// the SAME blockers. A pass clears the record, so a session that clears its blockers gates afresh.
func TestStopHookBreaksAContinuationLoopWithoutTheHostFlag(t *testing.T) {
	l, home := newLayout(t)
	repo := l.blockedRepo(home)
	// The payload carries no stop_hook_active, exactly as the captured Codex continuations did.
	payload := `{"session_id":"sess-loop"}`
	for i := 1; i <= 3; i++ {
		out, errText := l.hookStderr(repo, payload)
		if i < 3 {
			if !blockDecision(out) {
				t.Fatalf("attempt %d must block, got stdout %q stderr %q", i, out, errText)
			}
			continue
		}
		if out != "" || !strings.Contains(errText, "yielding after a repeated block") {
			t.Fatalf("attempt %d must stand down loudly, got stdout %q stderr %q", i, out, errText)
		}
	}
	// A pass clears the record: after the blockers clear, the gate blocks afresh rather than yielding at once.
	l.git(repo, "checkout", "-q", "-b", "clean", "main")
	if out, errText := l.hookStderr(repo, payload); out != "" || errText != "" {
		t.Fatalf("a clean branch must pass silently, got stdout %q stderr %q", out, errText)
	}
	l.git(repo, "checkout", "-q", "feat")
	if out, _ := l.hookStderr(repo, payload); !blockDecision(out) {
		t.Fatalf("after a pass the gate must block afresh, got %q", out)
	}
}

// The session-wide cap bounds a loop even when the per-set count never trips: the blocker set CHANGES
// between refusals (so n resets), and only the total cap can stand the gate down.
func TestStopHookSessionWideCapStandsDownTheGate(t *testing.T) {
	l, home := newLayout(t)
	repo := l.blockedRepo(home)
	payload := `{"session_id":"sess-total"}`
	limits := []string{"METAREVIEW_STOP_GUARD_LIMIT=100", "METAREVIEW_STOP_GUARD_TOTAL=2"}
	if out, _ := l.runStopHook(repo, payload, limits...); !blockDecision(out) {
		t.Fatalf("the first refusal must block, got %q", out)
	}
	l.abandon(repo) // a second abandoned run CHANGES the blocker set, so the per-set count resets
	out, errText := l.runStopHook(repo, payload, limits...)
	if out != "" || !strings.Contains(errText, "yielding after a repeated block") {
		t.Fatalf("the session-wide cap must stand the gate down on a changing set, got stdout %q stderr %q", out, errText)
	}
}

// A persistently BROKEN gate (status exits neither 0 nor 1) must also be bounded, not refuse forever.
func TestStopHookBreaksABrokenGateLoop(t *testing.T) {
	l, home := newLayout(t)
	repo := filepath.Join(home, "repo")
	if err := os.Mkdir(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	l.seed(repo)
	l.optIn(repo)
	broken := filepath.Join(home, "broken-metareview")
	if err := os.WriteFile(broken, []byte("#!/bin/sh\nexit 2\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	payload := `{"session_id":"sess-broken"}`
	brokenBin := "METAREVIEW_BIN=" + broken
	for i := 1; i <= 3; i++ {
		out, errText := l.runStopHook(repo, payload, brokenBin)
		if i < 3 {
			if !blockDecision(out) {
				t.Fatalf("attempt %d must block on a broken gate, got stdout %q stderr %q", i, out, errText)
			}
			continue
		}
		if out != "" || !strings.Contains(errText, "yielding after a repeated block") {
			t.Fatalf("attempt %d must stand down loudly, got stdout %q stderr %q", i, out, errText)
		}
	}
}

// A pass clears the record: with the per-set limit at 2, a block, a clean pass, then the same block again
// must BLOCK (if the pass had not cleared the count, the second refusal would stand the gate down).
func TestStopHookPassClearsTheLoopGuard(t *testing.T) {
	l, home := newLayout(t)
	repo := l.blockedRepo(home)
	payload := `{"session_id":"sess-pass"}`
	limits := []string{"METAREVIEW_STOP_GUARD_LIMIT=2", "METAREVIEW_STOP_GUARD_TOTAL=2"}
	if out, _ := l.runStopHook(repo, payload, limits...); !blockDecision(out) {
		t.Fatalf("the first refusal must block, got %q", out)
	}
	// A pass on a branch with nothing to clear.
	l.git(repo, "checkout", "-q", "-b", "clean", "main")
	if out, errText := l.runStopHook(repo, payload, limits...); out != "" || errText != "" {
		t.Fatalf("a clean branch must pass silently, got stdout %q stderr %q", out, errText)
	}
	// Back on the blocked branch: with the count cleared this is refusal 1, so it must BLOCK, not stand down.
	l.git(repo, "checkout", "-q", "feat")
	if out, errText := l.runStopHook(repo, payload, limits...); !blockDecision(out) {
		t.Fatalf("a pass must clear the loop-guard count, so the next refusal blocks: stdout %q stderr %q", out, errText)
	}
}

// The missing-binary path is the other refusal a session cannot clear from inside, and it carries the
// same guard: it blocks a few times, then stands down loudly.
func TestStopHookBreaksAMissingBinaryLoop(t *testing.T) {
	l, home := newLayout(t)
	repo := filepath.Join(home, "repo")
	if err := os.Mkdir(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	l.seed(repo)
	l.optIn(repo)
	payload := `{"session_id":"sess-missing"}`
	missing := "METAREVIEW_BIN=" + filepath.Join(home, "absent")
	for i := 1; i <= 3; i++ {
		out, errText := l.runStopHook(repo, payload, missing)
		if i < 3 {
			if !blockDecision(out) {
				t.Fatalf("attempt %d must block, got stdout %q stderr %q", i, out, errText)
			}
			continue
		}
		if out != "" || !strings.Contains(errText, "yielding after a repeated block") {
			t.Fatalf("attempt %d must stand down loudly, got stdout %q stderr %q", i, out, errText)
		}
	}
}

// A stand-down clears the record too: after it, the gate is NOT permanently off — the next Stop with the
// same blockers blocks afresh (with LIMIT=2, a stale count would stand it down immediately).
func TestStopHookStandDownClearsTheRecord(t *testing.T) {
	l, home := newLayout(t)
	repo := l.blockedRepo(home)
	payload := `{"session_id":"sess-resume"}`
	limits := []string{"METAREVIEW_STOP_GUARD_LIMIT=2", "METAREVIEW_STOP_GUARD_TOTAL=2"}
	if out, _ := l.runStopHook(repo, payload, limits...); !blockDecision(out) {
		t.Fatalf("refusal 1 must block, got %q", out)
	}
	if out, errText := l.runStopHook(repo, payload, limits...); out != "" || !strings.Contains(errText, "yielding after a repeated block") {
		t.Fatalf("refusal 2 must stand down, got stdout %q stderr %q", out, errText)
	}
	if out, errText := l.runStopHook(repo, payload, limits...); !blockDecision(out) {
		t.Fatalf("after a stand-down the next refusal must block afresh: stdout %q stderr %q", out, errText)
	}
}

// A DIFFERENT blocker set restarts the per-set count: with LIMIT=2, a second refusal about NEW work must
// block, not stand down (only the session-wide cap bounds a set that keeps changing).
func TestStopHookDifferentBlockersRestartTheCount(t *testing.T) {
	l, home := newLayout(t)
	repo := l.blockedRepo(home)
	payload := `{"session_id":"sess-progress"}`
	limits := []string{"METAREVIEW_STOP_GUARD_LIMIT=2", "METAREVIEW_STOP_GUARD_TOTAL=100"}
	if out, _ := l.runStopHook(repo, payload, limits...); !blockDecision(out) {
		t.Fatalf("refusal 1 must block, got %q", out)
	}
	l.abandon(repo) // a second abandoned run changes the blocker set
	if out, errText := l.runStopHook(repo, payload, limits...); !blockDecision(out) {
		t.Fatalf("a different blocker set must block afresh: stdout %q stderr %q", out, errText)
	}
}

// mr-j30: a counter planted in a state directory the user does not own or that others can write to must not
// force an early stand-down. The guard rejects such a directory and falls back, so a poisoned world-writable
// cache dir still leaves the gate blocking.
func TestStopHookIgnoresAPoisonedStateDir(t *testing.T) {
	l, home := newLayout(t)
	repo := l.blockedRepo(home)
	payload := `{"session_id":"sess-poison"}`
	guardDir := filepath.Join(home, ".cache", "metareview", "stop-gate")
	if err := os.MkdirAll(guardDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// A world-writable dir another local user could have created, holding a spent counter at the session's key.
	key := fmt.Sprintf("%x", sha256.Sum256([]byte("sess-poison")))[:32]
	if err := os.WriteFile(filepath.Join(guardDir, key), []byte("99 deadbeef 99"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(guardDir, 0o777); err != nil {
		t.Fatal(err)
	}
	// The planted count (99) would stand the gate down at once; the guard must instead ignore the dir.
	if out, errText := l.runStopHook(repo, payload); !blockDecision(out) {
		t.Fatalf("a poisoned state dir must not stand the gate down: stdout %q stderr %q", out, errText)
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
