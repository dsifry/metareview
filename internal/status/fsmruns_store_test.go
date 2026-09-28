package status

import (
	"os"
	"os/exec"
	"path/filepath"
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
	got := DiscoverAbandonedRuns(root)
	if len(got) != 1 || got[0].RunID != "mrv-mine-0000001" {
		t.Fatalf("want only this worktree's run, got %+v", got)
	}
	// A 0.13.x run not yet migrated, in this worktree's own .metareview/runs, is still seen (one release).
	writeRun(t, root, "mrv-legacy-00001",
		`{"type":"init","at":"2026-09-27T00:00:00Z","state":"discover","data":{"workflow":"t"}}`,
		`{"type":"transition","at":"2026-09-27T00:00:01Z","state":"discover","data":{"to":"fix","to_kind":"agent-edit"}}`)
	if got := DiscoverAbandonedRuns(root); len(got) != 2 {
		t.Fatalf("want the store run and the legacy run, got %+v", got)
	}
}
