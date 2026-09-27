package cli

import (
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

	// Store root: the run directory and its terminal row, exactly once, under the main worktree.
	if _, err := os.Stat(filepath.Join(h.root, ".metareview", "runs", id, "audit.jsonl")); err != nil {
		t.Fatalf("run not in the store root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wt, ".metareview", "runs", id)); err == nil {
		t.Fatal("run duplicated into the linked worktree")
	}
	rows, _ := os.ReadFile(filepath.Join(h.root, ".metareview", "runs.jsonl"))
	if !strings.Contains(string(rows), `"id":"`+id+`"`) {
		t.Fatalf("terminal row not in the store root's runs.jsonl: %s", rows)
	}
	if _, err := os.Stat(filepath.Join(wt, ".metareview", "runs.jsonl")); err == nil {
		t.Fatal("terminal row written into the linked worktree")
	}

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

// buildsRootedPath reports whether a line of code names a .metareview or docs/metareview path, in either the
// split form (filepath.Join(root, ".metareview", …), "docs", "metareview") or a slash-joined string literal
// (".metareview/runs.jsonl", "docs/metareview/…").
func buildsRootedPath(code string) bool {
	for _, lit := range []string{`".metareview"`, `"docs", "metareview"`, `".metareview/`, `"docs/metareview`} {
		if strings.Contains(code, lit) {
			return true
		}
	}
	return false
}

// TestFSMRootsAreDeclared keeps #169/#172 from recurring inside the FSM: every line in internal/fsm that
// names a .metareview or docs/metareview path must say which root it means — `root: store` (shared state,
// the main worktree) or `root: work` (the checkout the command runs in) — on that line or within the three
// lines above. It is a tripwire over those literal path forms (split elements and slash-joined strings), not
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
				if strings.Contains(lines[j], "root: store") || strings.Contains(lines[j], "root: work") {
					declared = true
				}
			}
			if !declared {
				t.Errorf("%s:%d builds a .metareview/docs path without a `root: store` or `root: work` declaration", path, i+1)
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

// TestRunsIgnoredChecksTheStoreRoot: the terminal runs.jsonl row is written under the store root, so the
// not-ignored warning must ask the store root, not the worktree the run was started from. Here the main
// checkout ignores runs.jsonl in a commit the linked worktree does not have.
func TestRunsIgnoredChecksTheStoreRoot(t *testing.T) {
	h := newHarness(t)
	wt := h.linkedWorktree() // at a commit whose .gitignore does not cover runs.jsonl
	h.file("../.gitignore", "mock/\nfixtures/\nexp/\nsmall/\ndocs/\n.metareview/runs.jsonl\n")
	git(t, h.root, "add", ".gitignore")
	git(t, h.root, "commit", "-q", "-m", "ignore runs.jsonl in the main checkout only")
	h.cwd = wt
	env := h.must(StatusOK, 0, h.mockInit()...)
	if w := env["warnings"].([]any); len(w) != 0 {
		t.Fatalf("runs.jsonl is ignored where it is written (the store root); no warning expected, got %v", w)
	}
	// And the other direction: a store root that does not ignore it warns, naming the store root.
	h.file("../.gitignore", "mock/\nfixtures/\nexp/\nsmall/\ndocs/\n")
	git(t, h.root, "add", ".gitignore")
	git(t, h.root, "commit", "-q", "-m", "stop ignoring runs.jsonl")
	env = h.must(StatusOK, 0, h.mockInit()...)
	w := env["warnings"].([]any)
	if len(w) != 1 || w[0].(map[string]any)["code"] != WarnRunsNotIgnored || !strings.Contains(w[0].(map[string]any)["detail"].(string), h.root) {
		t.Fatalf("want one runs-not-ignored warning naming the store root %s, got %v", h.root, w)
	}
}
