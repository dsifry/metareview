package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/fsm/machine"
)

// linkedWorktree adds a detached linked worktree of the harness repo at HEAD and returns its real path.
func (h *harness) linkedWorktree() string {
	h.t.Helper()
	wt := filepath.Join(h.t.TempDir(), "linked")
	git(h.t, h.root, "worktree", "add", "-q", "--detach", wt)
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
				if !none && !(store && common) {
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
