package cli

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/fsm/gate"
	"github.com/dsifry/metareview/internal/fsm/machine"
)

// toLegacy moves a run and its ledger row back into the 0.13.x layout: the main checkout's .metareview/.
func toLegacy(t *testing.T, h *harness, id string) {
	t.Helper()
	common := filepath.Join(h.root, ".git", "metareview")
	legacy := filepath.Join(h.root, ".metareview")
	if err := os.MkdirAll(filepath.Join(legacy, "runs"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(common, "runs", id), filepath.Join(legacy, "runs", id)); err != nil {
		t.Fatal(err)
	}
	if rows, err := os.ReadFile(filepath.Join(common, "runs.jsonl")); err == nil {
		if err := os.WriteFile(filepath.Join(legacy, "runs.jsonl"), rows, 0o644); err != nil {
			t.Fatal(err)
		}
		_ = os.Remove(filepath.Join(common, "runs.jsonl"))
	}
}

func warningCodes(env map[string]any) []string {
	var codes []string
	for _, w := range env["warnings"].([]any) {
		codes = append(codes, w.(map[string]any)["code"].(string))
	}
	return codes
}

// AC-2.5 (#173), end to end: a 0.13.x store in the main checkout is migrated into git's common directory by the next
// fsm command — byte-identical, announced once, then silent — and the run is fully usable.
func TestFSMMigratesTheLegacyStore(t *testing.T) {
	h := newHarness(t)
	id := h.must(StatusOK, 0, h.mockInit()...)["run_id"].(string)
	h.must(machine.StatusNeedsInput, 3, "advance", "--run", id)
	toLegacy(t, h, id)
	before, _ := os.ReadFile(filepath.Join(h.root, ".metareview", "runs", id, "audit.jsonl"))

	env := h.must(StatusOK, 0, "state", "--run", id)
	if codes := warningCodes(env); len(codes) != 1 || codes[0] != WarnStoreMigrated {
		t.Fatalf("the first command must announce the migration: %v", env["warnings"])
	}
	after, err := os.ReadFile(filepath.Join(h.root, ".git", "metareview", "runs", id, "audit.jsonl"))
	if err != nil || sha256.Sum256(before) != sha256.Sum256(after) {
		t.Fatalf("migrated audit must be byte-identical: %v", err)
	}
	if env := h.must(StatusOK, 0, "state", "--run", id); len(warningCodes(env)) != 0 {
		t.Fatalf("a second command must be silent: %v", env["warnings"])
	}
	// The migrated ledger row still reserves the id.
	h.mustErr("ERR_RUN_EXISTS", 2, append(h.mockInit(), "--run-id", id)...)
}

// A run in both places, or a ledger row that differs, is reported and never merged.
func TestFSMStoreMigrationCollisions(t *testing.T) {
	h := newHarness(t)
	id := h.must(StatusOK, 0, h.mockInit()...)["run_id"].(string)
	common := filepath.Join(h.root, ".git", "metareview")
	legacy := filepath.Join(h.root, ".metareview")
	_ = os.MkdirAll(filepath.Join(legacy, "runs"), 0o700)
	if err := os.CopyFS(filepath.Join(legacy, "runs", id), os.DirFS(filepath.Join(common, "runs", id))); err != nil {
		t.Fatal(err)
	}
	// A row conflict on another id (a collided run's own row stays behind with it, so it cannot conflict).
	other := "mrv-row-conflict-000001"
	row := `{"schemaVersion":1,"id":"` + other + `","scope":"fsm-sdlc-loop","headSha":"different","workflowHash":"x"}` + "\n"
	_ = os.WriteFile(filepath.Join(common, "runs.jsonl"), []byte(`{"schemaVersion":1,"id":"`+other+`","scope":"fsm-sdlc-loop","headSha":"h","workflowHash":"x"}`+"\n"), 0o644)
	_ = os.WriteFile(filepath.Join(legacy, "runs.jsonl"), []byte(row), 0o644)
	env := h.must(StatusOK, 0, "state", "--run", id)
	codes := strings.Join(warningCodes(env), ",")
	if codes != WarnStoreCollision+","+WarnStoreCollision {
		t.Fatalf("want a run collision and a row conflict, got %v", env["warnings"])
	}
	if _, err := os.Stat(filepath.Join(legacy, "runs", id, "audit.jsonl")); err != nil {
		t.Fatal("the legacy copy must survive a collision")
	}
	// roots() passes the collision to the row migration: the collided run's own legacy row stays behind with it.
	collidedRow := `{"schemaVersion":1,"id":"` + id + `","scope":"fsm-sdlc-loop","headSha":"legacy","workflowHash":"x"}` + "\n"
	_ = os.WriteFile(filepath.Join(legacy, "runs.jsonl"), []byte(row+collidedRow), 0o644)
	h.must(StatusOK, 0, "state", "--run", id)
	if ledger, _ := os.ReadFile(filepath.Join(common, "runs.jsonl")); strings.Contains(string(ledger), `"id":"`+id+`"`) {
		t.Fatalf("a collided run's legacy row must not be copied: %s", ledger)
	}
}

// A migration that cannot run fails the command rather than proceeding against a half-moved store.
func TestFSMStoreMigrationFailures(t *testing.T) {
	h := newHarness(t)
	legacy := filepath.Join(h.root, ".metareview")
	_ = os.MkdirAll(legacy, 0o700)
	_ = os.WriteFile(filepath.Join(legacy, "runs"), []byte("x"), 0o600) // runs is a file
	h.mustErr("ERR_STORE_PATH", 2, "state")
	_ = os.Remove(filepath.Join(legacy, "runs"))
	_ = os.MkdirAll(filepath.Join(legacy, "runs.jsonl"), 0o700) // the legacy ledger is a directory
	// ERR_NO_RUNS would also exit nonzero here, so pin the code: the failure must be the migration's own.
	if env, code := h.run("state"); code == 0 || env["code"] == "ERR_NO_RUNS" {
		t.Fatalf("an unreadable legacy ledger must fail the command with its own error, got %v (exit %d)", env["code"], code)
	}
	_ = os.RemoveAll(filepath.Join(legacy, "runs.jsonl"))

	// git cannot name the common directory although it names the main worktree.
	real := h.deps.Exec
	h.deps.Exec = func(ctx context.Context, dir string, env []string, args ...string) ([]byte, []byte, int, error) {
		for _, a := range args {
			if a == "--git-common-dir" {
				return nil, nil, 128, nil
			}
		}
		return real(ctx, dir, env, args...)
	}
	h.mustErr(CodeNotARepo, 2, "state")
	h.deps.Exec = gate.RealExec
}

// AC-2.3 (#173): `git clean -fdX` in the main checkout — which wiped a 0.13.x store living in its ignored
// .metareview/ — removes no run, because the store is in git's common directory.
func TestGitCleanLeavesTheStore(t *testing.T) {
	h := newHarness(t)
	id := h.must(StatusOK, 0, h.mockInit()...)["run_id"].(string)
	git(t, h.root, "clean", "-fdX")
	git(t, h.root, "clean", "-fdx")
	if _, err := os.Stat(filepath.Join(h.root, ".git", "metareview", "runs", id, "audit.jsonl")); err != nil {
		t.Fatalf("git clean removed a run: %v", err)
	}
	// (The mock scenario the run replays was an ignored file, so clean took it; the run itself is intact and listed.)
	if lines := strings.Join(StatusLines(context.Background(), h.deps, h.root), "\n"); !strings.Contains(lines, id) {
		t.Fatalf("the run must still be listed: %s", lines)
	}
}

// AC-2.9 (#173): nothing metareview writes lands among git's own per-worktree files (.git/worktrees/<name>/: gitdir,
// HEAD, commondir, locked, …), where a name could collide with one git owns.
func TestNothingIsWrittenIntoGitsWorktreeDirs(t *testing.T) {
	h := newHarness(t)
	wt := h.linkedWorktree()
	wtAdmin := filepath.Join(h.root, ".git", "worktrees")
	snapshot := func() map[string]bool {
		files := map[string]bool{}
		_ = filepath.WalkDir(wtAdmin, func(p string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				rel, _ := filepath.Rel(wtAdmin, p)
				files[rel] = true
			}
			return nil
		})
		return files
	}
	before := snapshot()
	h.cwd = wt
	id := h.must(StatusOK, 0, h.mockInit()...)["run_id"].(string)
	h.must(machine.StatusNeedsInput, 3, "advance", "--run", id)
	for f := range snapshot() {
		if !before[f] && !strings.Contains(f, string(filepath.Separator)+"metareview"+string(filepath.Separator)) &&
			!strings.HasSuffix(f, "index") && !strings.HasSuffix(f, "logs/HEAD") && !strings.HasSuffix(f, "ORIG_HEAD") {
			t.Errorf("new file among git's per-worktree files, outside a metareview/ directory: %s", f)
		}
	}
}

// Plain `metareview status` is read-only, so it never migrates; after an upgrade it must still say that 0.13.x runs
// are waiting in the legacy store rather than report none.
func TestStatusLinesReportsUnmigratedLegacyRuns(t *testing.T) {
	h := newHarness(t)
	id := h.must(StatusOK, 0, h.mockInit()...)["run_id"].(string)
	toLegacy(t, h, id)
	_ = os.MkdirAll(filepath.Join(h.root, ".metareview", "runs", "not a run id"), 0o700) // never migrated: not counted
	lines := strings.Join(StatusLines(context.Background(), h.deps, h.root), "\n")
	if !strings.Contains(lines, "fsm runs: none") || !strings.Contains(lines, "1 0.13.x run(s) not yet migrated") {
		t.Fatalf("status must report the unmigrated legacy run: %s", lines)
	}
	h.must(StatusOK, 0, "state", "--run", id) // migrates
	if lines := strings.Join(StatusLines(context.Background(), h.deps, h.root), "\n"); strings.Contains(lines, "not yet migrated") {
		t.Fatalf("a migrated store has nothing pending: %s", lines)
	}
}
