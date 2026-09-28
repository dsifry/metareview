package status

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// writeStoreRun writes a run into git's common-dir store (#173) with the work_dir its init recorded.
func writeStoreRun(t *testing.T, common, id, workDir string) {
	t.Helper()
	dir := filepath.Join(common, "metareview", "runs", id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "workflow.yaml"), []byte(testWorkflow), 0o600); err != nil {
		t.Fatal(err)
	}
	audit := `{"type":"init","at":"2026-09-27T00:00:00Z","state":"discover","data":{"workflow":"t","work_dir":"` + workDir + `"}}` + "\n" +
		`{"type":"transition","at":"2026-09-27T00:00:01Z","state":"discover","data":{"to":"fix","to_kind":"agent-edit"}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "audit.jsonl"), []byte(audit), 0o600); err != nil {
		t.Fatal(err)
	}
}

// #173: runs now live in the shared common-dir store. status reports the abandoned ones this worktree started — its
// own work_dir — and never another worktree's (a false block on a branch that did not start it).
func TestAbandonedRunsComeFromTheSharedStoreScopedToThisWorktree(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "init", "-q", "-b", "main", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	common := filepath.Join(root, ".git")
	writeStoreRun(t, common, "mrv-mine-0000001", root)
	writeStoreRun(t, common, "mrv-other-000001", filepath.Join(root, "..", "elsewhere"))
	// "other" names a worktree that no longer exists: unattributable, so the main checkout reports it.
	if got := strings.Join(ids(DiscoverAbandonedRuns(root)), ","); got != "mrv-mine-0000001,mrv-other-000001" {
		t.Fatalf("want this worktree's run and the unattributable one, got %s", got)
	}
	// A 0.13.x run not yet migrated, in this worktree's own .metareview/runs, is still seen (one release).
	writeRun(t, root, "mrv-legacy-00001",
		`{"type":"init","at":"2026-09-27T00:00:00Z","state":"discover","data":{"workflow":"t"}}`,
		`{"type":"transition","at":"2026-09-27T00:00:01Z","state":"discover","data":{"to":"fix","to_kind":"agent-edit"}}`)
	if got := DiscoverAbandonedRuns(root); len(got) != 3 {
		t.Fatalf("want the store run and the legacy run, got %+v", got)
	}
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
}

func ids(runs []AbandonedRun) []string {
	var out []string
	for _, r := range runs {
		out = append(out, r.RunID)
	}
	return out
}

// A run belongs to the worktree that CONTAINS its work_dir — `fsm init --work-dir` accepts any directory inside a
// worktree — and a run whose work_dir no longer exists (a removed worktree) is reported from the main checkout rather
// than from nowhere. A legacy (0.13.x) run in the main checkout's .metareview/runs is attributed the same way
// (#173 review: a subdirectory work_dir had let an abandoned run escape the Stop gate).
func TestAbandonedRunsAreAttributedToTheirContainingWorktree(t *testing.T) {
	base, _ := filepath.EvalSymlinks(t.TempDir())
	main := filepath.Join(base, "main")
	gitRun(t, base, "init", "-q", "-b", "main", main)
	gitRun(t, main, "commit", "-q", "--allow-empty", "-m", "base")
	wt := filepath.Join(base, "wt")
	gitRun(t, main, "worktree", "add", "-q", "-b", "feat", wt)
	if err := os.MkdirAll(filepath.Join(main, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	common := filepath.Join(main, ".git")
	writeStoreRun(t, common, "mrv-sub-00000001", filepath.Join(main, "sub"))
	writeStoreRun(t, common, "mrv-gone-0000001", filepath.Join(base, "removed-worktree"))
	writeStoreRun(t, common, "mrv-wt-000000001", wt)
	// A 0.13.x run from the linked worktree, still in the main checkout's legacy store.
	legacy := filepath.Join(main, ".metareview", "runs", "mrv-legacy-wt-01")
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(legacy, "workflow.yaml"), []byte(testWorkflow), 0o600)
	_ = os.WriteFile(filepath.Join(legacy, "audit.jsonl"), []byte(`{"type":"init","at":"t","state":"discover","data":{"workflow":"t","work_dir":"`+wt+`"}}`+"\n"+
		`{"type":"transition","at":"t","state":"discover","data":{"to":"fix","to_kind":"agent-edit"}}`+"\n"), 0o600)

	if got := strings.Join(ids(DiscoverAbandonedRuns(main)), ","); got != "mrv-gone-0000001,mrv-sub-00000001" {
		t.Errorf("main checkout: got %s", got)
	}
	if got := strings.Join(ids(DiscoverAbandonedRuns(wt)), ","); got != "mrv-legacy-wt-01,mrv-wt-000000001" {
		t.Errorf("linked worktree: got %s", got)
	}
	if !LegacyRunsPending(wt) || LegacyRunsPending(t.TempDir()) {
		t.Error("LegacyRunsPending must report the main checkout's unmigrated 0.13.x store")
	}
}

func TestCanonicalAndLegacyBookkeeping(t *testing.T) {
	if got := canonical("/no/such/dir/./x"); got != "/no/such/dir/x" {
		t.Errorf("canonical of a missing path = %q, want the cleaned path", got)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".metareview", "runs", ".torn"), 0o700); err != nil {
		t.Fatal(err)
	}
	if LegacyRunsPending(root) {
		t.Error("a legacy store holding only its own bookkeeping has nothing to migrate")
	}
}
