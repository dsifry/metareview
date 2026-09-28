package cli

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/fsm/machine"
)

// linkedWorktree adds a linked worktree of the harness repo at HEAD, on its own branch (a run records the branch it is
// for, #177), and returns its real path.
func (h *harness) linkedWorktree() string {
	h.t.Helper()
	wt := filepath.Join(h.t.TempDir(), "linked")
	git(h.t, h.root, "worktree", "add", "-q", "-b", "linked-"+filepath.Base(filepath.Dir(wt)), wt)
	real, err := filepath.EvalSymlinks(wt)
	if err != nil {
		h.t.Fatal(err)
	}
	return real
}

// TestLinkedWorktreeStoreAndWorkRoots pins which root each FSM output uses when a run is driven from a
// linked worktree (#172). The run itself and its runs.jsonl row are store-level state (run ids are unique
// across the store, and record.Exists checks that row), so they live under the store root — the main
// worktree. Work output — the export bundle, which is committed — belongs to the worktree that asked for it.
func TestLinkedWorktreeStoreAndWorkRoots(t *testing.T) {
	h := newHarness(t)
	h.file("../.gitignore", "mock/\nfixtures/\nexp/\nsmall/\ndocs/\n.metareview/runs.jsonl\n")
	git(t, h.root, "add", ".gitignore")
	git(t, h.root, "commit", "-q", "-m", "ignore runs.jsonl")
	wt := h.linkedWorktree()
	h.cwd = wt

	id := h.must(StatusOK, 0, h.mockInit()...)["run_id"].(string)
	h.must(machine.StatusNeedsInput, 3, "advance", "--run", id)
	h.stdin = `{"findings":[]}`
	h.must(StatusOK, 0, "record", "node-output", "--node", "discover", "--data", "-", "--run", id)
	h.stdin = ""
	if env := h.must(machine.StatusDone, 0, "advance", "--run", id); env["outcome"] != "clean" {
		t.Fatalf("done: %v", env)
	}

	// AC-2.2 (#173): the run directory and its terminal row, exactly once, in git's common directory — neither
	// checkout's .metareview/ holds either.
	common := filepath.Join(h.root, ".git", "metareview")
	if _, err := os.Stat(filepath.Join(common, "runs", id, "audit.jsonl")); err != nil {
		t.Fatalf("run not in the common-dir store: %v", err)
	}
	rows, _ := os.ReadFile(filepath.Join(common, "runs.jsonl"))
	if !strings.Contains(string(rows), `"id":"`+id+`"`) {
		t.Fatalf("terminal row not in the common-dir ledger: %s", rows)
	}
	for _, checkout := range []string{h.root, wt} {
		if _, err := os.Stat(filepath.Join(checkout, ".metareview", "runs", id)); err == nil {
			t.Fatalf("run duplicated into %s", checkout)
		}
		if _, err := os.Stat(filepath.Join(checkout, ".metareview", "runs.jsonl")); err == nil {
			t.Fatalf("terminal row written into %s", checkout)
		}
	}
	// The same run is visible from the main checkout.
	h.cwd = h.root
	if env := h.must(StatusOK, 0, "state", "--run", id); env["outcome"] != "clean" {
		t.Fatalf("state from the main checkout: %v", env)
	}
	h.cwd = wt

	// Work root: a default export lands in the worktree that ran the command, never in the main checkout.
	env := h.must(StatusOK, 0, "export", "--run", id)
	want := filepath.Join(wt, "docs", "metareview", "fsm", id)
	if env["out"] != want {
		t.Fatalf("export out = %v, want %s", env["out"], want)
	}
	manifest, err := os.ReadFile(filepath.Join(want, "manifest.json"))
	if err != nil {
		t.Fatalf("bundle not in the linked worktree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(h.root, "docs", "metareview", "fsm", id)); err == nil {
		t.Fatal("bundle written into the main checkout")
	}
	for _, name := range []string{"manifest.json", "snapshot.json", "audit.redacted.jsonl"} {
		b, _ := os.ReadFile(filepath.Join(want, name))
		if strings.Contains(string(b), wt) || strings.Contains(string(b), h.root) {
			t.Fatalf("%s leaks an absolute checkout path", name)
		}
	}
	_ = manifest

	// From the main checkout, the default export still lands in the main checkout (single-checkout behavior).
	h.cwd = h.root
	if err := os.RemoveAll(want); err != nil {
		t.Fatal(err)
	}
	env = h.must(StatusOK, 0, "export", "--run", id)
	if env["out"] != filepath.Join(h.root, "docs", "metareview", "fsm", id) {
		t.Fatalf("export from main: out = %v", env["out"])
	}
}

// buildsCommonDirPath reports whether a line names a path in the shared store under git's common directory (#173):
// the split form `"metareview", "<name>"` without the checkout's leading dot or a docs/ parent.
func buildsCommonDirPath(code string) bool {
	return strings.Contains(code, `"metareview", "`) && !strings.Contains(code, `"docs", "metareview"`) && !strings.Contains(code, `".metareview"`)
}

// buildsRootedPath reports whether a line of code names a .metareview or docs/metareview path, in either the
// split form (filepath.Join(root, ".metareview", …), "docs", "metareview") or a slash-joined string literal
// (".metareview/runs.jsonl", "docs/metareview/…").
func buildsRootedPath(code string) bool {
	if buildsCommonDirPath(code) {
		return true
	}
	for _, lit := range []string{`".metareview"`, `"docs", "metareview"`, `".metareview/`, `"docs/metareview`} {
		if strings.Contains(code, lit) {
			return true
		}
	}
	return false
}

// TestFSMRootsAreDeclared keeps #169/#172 from recurring inside the FSM: every line in internal/fsm that
// names a .metareview, docs/metareview or git-common-dir metareview path must say which root it means — `root: store`
// (shared state) or `root: work` (the checkout the command runs in) — on that line or within the three lines above.
// Since #173 `root: store` has two homes, so a common-dir site (buildsCommonDirPath) must also name "common dir"
// (e.g. `root: store (git's common directory)`); a site that is not a store path at all may say `root: none`. It is a tripwire over those literal path forms (split elements and slash-joined strings), not
// proof: a path assembled any other way is not seen. Comment lines are skipped.
func TestFSMRootsAreDeclared(t *testing.T) {
	const window = 3
	fsmDir := filepath.Join("..")
	sites := 0
	err := filepath.WalkDir(fsmDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path) // #nosec G304 -- walking this repository's own sources
		if err != nil {
			return err
		}
		lines := strings.Split(string(src), "\n")
		for i, line := range lines {
			code := strings.TrimSpace(line)
			if strings.HasPrefix(code, "//") || !buildsRootedPath(code) {
				continue
			}
			sites++
			declared := false
			for j := max(0, i-window); j <= i; j++ {
				if strings.Contains(lines[j], "root: store") || strings.Contains(lines[j], "root: work") || strings.Contains(lines[j], "root: none") {
					declared = true
				}
			}
			if !declared {
				t.Errorf("%s:%d builds a .metareview/docs/common-dir path without a `root: store`, `root: work` or `root: none` declaration", path, i+1)
			}
			// `root: store` alone is ambiguous since #173 (the anchor checkout or git's common directory): a
			// common-dir site must say which.
			if buildsCommonDirPath(code) {
				store, common, none := false, false, false
				for j := max(0, i-window); j <= i; j++ {
					store = store || strings.Contains(lines[j], "root: store")
					common = common || strings.Contains(lines[j], "common dir")
					none = none || strings.Contains(lines[j], "root: none")
				}
				if !none && (!store || !common) {
					t.Errorf("%s:%d builds a common-dir store path without a `root: store (git's common directory)` declaration", path, i+1)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if sites == 0 {
		t.Fatal("found no .metareview/docs path sites; the walk is not looking at internal/fsm")
	}
}

// TestExportWithoutACheckoutFallsBackToTheStoreRoot: run from inside the git directory there is no work tree
// (`rev-parse --show-toplevel` fails) but the store still resolves, so a default export lands under the store
// root, exactly as a single checkout would.
func TestExportWithoutACheckoutFallsBackToTheStoreRoot(t *testing.T) {
	h := newHarness(t)
	id := h.must(StatusOK, 0, h.mockInit()...)["run_id"].(string)
	h.cwd = filepath.Join(h.root, ".git")
	env := h.must(StatusOK, 0, "export", "--run", id)
	if want := filepath.Join(h.root, "docs", "metareview", "fsm", id); env["out"] != want {
		t.Fatalf("export out = %v, want the store-root fallback %s", env["out"], want)
	}
}

// TestTerminalRowNeverDirtiesACheckout (#173): the terminal ledger lives in git's common directory, so no checkout —
// main or linked, ignoring runs.jsonl or not — gains an untracked file or a not-ignored warning from a run.
func TestTerminalRowNeverDirtiesACheckout(t *testing.T) {
	h := newHarness(t)
	wt := h.linkedWorktree()
	for _, cwd := range []string{h.root, wt} {
		h.cwd = cwd
		env := h.must(StatusOK, 0, h.mockInit()...)
		if w := env["warnings"].([]any); len(w) != 0 {
			t.Fatalf("init from %s: want no warnings, got %v", cwd, w)
		}
		if _, err := os.Stat(filepath.Join(cwd, ".metareview", "runs.jsonl")); err == nil {
			t.Fatalf("init from %s wrote a checkout ledger", cwd)
		}
	}
}

// TestExportDoesNotFallBackOnAGitFailureInsideAWorktree: the store-root fallback is only for "there is no work
// tree around cwd" (cwd inside .git). If --show-toplevel fails for any other reason inside a real worktree,
// falling back would silently write the bundle to the main checkout — the bug #172 fixes — so it must fail.
func TestExportDoesNotFallBackOnAGitFailureInsideAWorktree(t *testing.T) {
	h := newHarness(t)
	wt := h.linkedWorktree()
	h.cwd = wt
	id := h.must(StatusOK, 0, h.mockInit()...)["run_id"].(string)
	real := h.deps.Exec
	h.deps.Exec = func(ctx context.Context, dir string, env []string, args ...string) ([]byte, []byte, int, error) {
		for _, a := range args {
			if a == "--show-toplevel" {
				return nil, []byte("fatal: simulated failure"), 128, nil
			}
		}
		return real(ctx, dir, env, args...)
	}
	if env, code := h.run("export", "--run", id); code == 0 {
		t.Fatalf("export must fail rather than fall back to the main checkout; got %v", env)
	}
	if _, err := os.Stat(filepath.Join(h.root, "docs", "metareview", "fsm", id)); err == nil {
		t.Fatal("bundle written into the main checkout")
	}
}

// #174: with a bare main worktree the store is still git's common directory (#173); a command run from a linked
// worktree anchors on that worktree, which is the checkout it has. From the bare directory itself there is none.
func TestBareMainWorktreeAnchorsOnTheLinkedWorktree(t *testing.T) {
	base, _ := filepath.EvalSymlinks(t.TempDir())
	bare := filepath.Join(base, "repo.git")
	git(t, base, "init", "-q", "--bare", "-b", "main", bare)
	seed := filepath.Join(base, "seed")
	git(t, base, "clone", "-q", bare, seed)
	git(t, seed, "commit", "-q", "--allow-empty", "-m", "base")
	git(t, seed, "push", "-q", "origin", "main")
	wt := filepath.Join(base, "main")
	git(t, bare, "worktree", "add", "-q", wt, "main")
	c := &ctxDeps{ctx: context.Background(), deps: RealDeps(), cwd: wt}
	root, err := c.storeRoot()
	if err != nil || root != wt {
		t.Fatalf("a linked worktree of a bare repository anchors on itself: %q %v", root, err)
	}
	c.cwd = bare
	if _, err := c.storeRoot(); err == nil {
		t.Fatal("the bare directory itself has no checkout to anchor on")
	}
}

// initBranch reads the branch a run's init event recorded (#177).
func initBranch(t *testing.T, h *harness, id string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(h.root, ".git", "metareview", "runs", id, "audit.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var ev struct {
		Data struct {
			Branch string `json:"branch"`
		} `json:"data"`
	}
	first, _, _ := strings.Cut(string(raw), "\n")
	if err := json.Unmarshal([]byte(first), &ev); err != nil {
		t.Fatal(err)
	}
	return ev.Data.Branch
}

// #177: a run records the branch it is for — the checked-out one, or --for-branch — and a detached HEAD must name it.
func TestInitRecordsTheBranchAndRequiresOneWhenDetached(t *testing.T) {
	h := newHarness(t)
	id := h.must(StatusOK, 0, h.mockInit()...)["run_id"].(string)
	if got := initBranch(t, h, id); got != "main" {
		t.Fatalf("init on main must record branch main, got %q", got)
	}
	// On a branch, --for-branch may only restate it: naming another would file this branch's run under that one, and
	// an abandoned run would stop blocking the branch it was actually reviewing.
	e := h.mustErr(CodeUsage, 2, append(h.mockInit(), "--for-branch", "elsewhere")...)
	if d := e["error"].(map[string]any)["detail"].(string); !strings.Contains(d, "main") || !strings.Contains(d, "elsewhere") {
		t.Fatalf("the refusal must name both branches: %v", e)
	}
	id = h.must(StatusOK, 0, append(h.mockInit(), "--for-branch", "main")...)["run_id"].(string)
	if got := initBranch(t, h, id); got != "main" {
		t.Fatalf("restating the checked-out branch is fine, got %q", got)
	}
	git(t, h.root, "branch", "feat")
	git(t, h.root, "checkout", "-q", "--detach")
	// Detached, the name must be a local branch: a typo, a remote-tracking name or a full ref would never match the
	// name leg, so the run would be orphaned — blocking nothing — as soon as the real branch is rebased.
	for _, bad := range []string{"fea", "origin/feat", "refs/heads/feat", "FEAT"} { // FEAT: a case-insensitive FS resolves it
		e := h.mustErr(CodeUsage, 2, append(h.mockInit(), "--for-branch", bad)...)
		if !strings.Contains(e["error"].(map[string]any)["detail"].(string), "not a local branch") {
			t.Fatalf("--for-branch %s must be refused as not a local branch: %v", bad, e)
		}
	}
	e = h.mustErr(CodeUsage, 2, h.mockInit()...)
	if !strings.Contains(e["error"].(map[string]any)["detail"].(string), "--for-branch") {
		t.Fatalf("the refusal must name --for-branch: %v", e)
	}
	id = h.must(StatusOK, 0, append(h.mockInit(), "--for-branch", "feat")...)["run_id"].(string)
	if got := initBranch(t, h, id); got != "feat" {
		t.Fatalf("--for-branch must be recorded, got %q", got)
	}
}

// A symbolic-ref that fails for any reason other than git's "detached" (exit 1) is a git failure, never read as a
// detached HEAD — which would tell the driver to pass --for-branch it does not need.
func TestInitReportsASymbolicRefFailureAsGit(t *testing.T) {
	h := newHarness(t)
	realExec := h.deps.Exec
	h.deps.Exec = func(ctx context.Context, dir string, env []string, args ...string) ([]byte, []byte, int, error) {
		if len(args) > 0 && args[0] == "symbolic-ref" {
			return []byte("fatal: broken"), nil, 128, nil
		}
		return realExec(ctx, dir, env, args...)
	}
	e := h.mustErr("ERR_GIT", 2, h.mockInit()...)
	if strings.Contains(e["error"].(map[string]any)["detail"].(string), "detached") {
		t.Fatalf("a git failure must not read as a detached HEAD: %v", e)
	}
}

// A tag named like the branch makes git's short name "heads/<branch>"; init records the branch itself.
func TestInitRecordsTheBranchBesideASameNamedTag(t *testing.T) {
	h := newHarness(t)
	git(t, h.root, "tag", "main")
	id := h.must(StatusOK, 0, h.mockInit()...)["run_id"].(string)
	if got := initBranch(t, h, id); got != "main" {
		t.Fatalf("a same-named tag must not change the recorded branch, got %q", got)
	}
}

// Checking --for-branch: git's "no" is not a local branch; git failing is ERR_GIT, with the failure itself.
func TestInitForBranchCheckTellsGitFailingFromNo(t *testing.T) {
	h := newHarness(t)
	git(t, h.root, "branch", "feat")
	git(t, h.root, "checkout", "-q", "--detach")
	realExec := h.deps.Exec
	fail := func(stdout []byte, code int, err error) {
		h.deps.Exec = func(ctx context.Context, dir string, env []string, args ...string) ([]byte, []byte, int, error) {
			if len(args) > 0 && args[0] == "for-each-ref" {
				return stdout, nil, code, err
			}
			return realExec(ctx, dir, env, args...)
		}
	}
	fail(nil, 128, nil)
	e := h.mustErr("ERR_GIT", 2, append(h.mockInit(), "--for-branch", "feat")...)
	if d := e["error"].(map[string]any)["detail"].(string); !strings.Contains(d, "for-each-ref exited 128") {
		t.Fatalf("a failing check is git's failure, not a missing branch: %v", e)
	}
	fail(nil, -1, context.DeadlineExceeded)
	e = h.mustErr("ERR_GIT", 2, append(h.mockInit(), "--for-branch", "feat")...)
	if d := e["error"].(map[string]any)["detail"].(string); !strings.Contains(d, "deadline") {
		t.Fatalf("a git that did not run says why: %v", e)
	}
}

// On a case-insensitive filesystem `git checkout MAIN` lands on main with HEAD spelled MAIN; init records the branch
// as git lists it, or status — comparing exactly — would never match the run again.
func TestInitRecordsTheBranchAsGitListsIt(t *testing.T) {
	h := newHarness(t)
	git(t, h.root, "symbolic-ref", "HEAD", "refs/heads/MAIN")
	if exec.Command("git", "-C", h.root, "rev-parse", "--verify", "--quiet", "refs/heads/MAIN").Run() != nil {
		t.Skip("case-sensitive filesystem: MAIN is an unborn branch of its own, not main")
	}
	id := h.must(StatusOK, 0, h.mockInit()...)["run_id"].(string)
	if got := initBranch(t, h, id); got != "main" {
		t.Fatalf("a mis-cased HEAD must be recorded as the listed branch, got %q", got)
	}
	// Restating the checked-out branch in its HEAD spelling is still restating it.
	id = h.must(StatusOK, 0, append(h.mockInit(), "--for-branch", "MAIN")...)["run_id"].(string)
	if got := initBranch(t, h, id); got != "main" {
		t.Fatalf("a mis-cased HEAD must be recorded as the listed branch, got %q", got)
	}
}

// The fold decision, whatever the filesystem: a HEAD spelled MAIN is folded to main only when git resolves that
// spelling; git's "no" leaves it (an unborn branch of its own on a case-sensitive filesystem); git failing is ERR_GIT.
func TestInitFoldsAMisSpelledHEADOnlyWhenGitResolvesIt(t *testing.T) {
	for _, c := range []struct {
		code int
		want string
	}{{0, "main"}, {1, "MAIN"}, {128, ""}} {
		h := newHarness(t)
		realExec := h.deps.Exec
		h.deps.Exec = func(ctx context.Context, dir string, env []string, args ...string) ([]byte, []byte, int, error) {
			switch {
			case len(args) > 0 && args[0] == "symbolic-ref":
				return []byte("refs/heads/MAIN\n"), nil, 0, nil
			case len(args) > 3 && args[0] == "rev-parse" && args[1] == "--verify" && args[3] == "refs/heads/MAIN":
				return nil, nil, c.code, nil
			}
			return realExec(ctx, dir, env, args...)
		}
		if c.want == "" {
			e := h.mustErr("ERR_GIT", 2, h.mockInit()...)
			if d := e["error"].(map[string]any)["detail"].(string); !strings.Contains(d, "rev-parse exited 128") {
				t.Fatalf("a failing check is git's failure: %v", e)
			}
			continue
		}
		id := h.must(StatusOK, 0, h.mockInit()...)["run_id"].(string)
		if got := initBranch(t, h, id); got != c.want {
			t.Fatalf("verify exit %d: recorded %q, want %q", c.code, got, c.want)
		}
	}
}
