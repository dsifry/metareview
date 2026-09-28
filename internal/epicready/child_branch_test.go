package epicready

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestChildBlockerOnItsOwnBranchStillSurfaces is #178's epic-ready half: a child task is usually reviewed on a branch
// of its own. Its open ledger blocker, recorded on that branch, belongs to another branch by the branch scope — yet the
// epic names the child explicitly, so epic-ready still surfaces it rather than silently dropping it.
func TestChildBlockerOnItsOwnBranchStillSurfaces(t *testing.T) {
	root := epicRepo(t)
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("switch", "-q", "-c", "child-branch", "main")
	git("commit", "-q", "--allow-empty", "-m", "child work")
	head := git("rev-parse", "HEAD")
	git("switch", "-q", "feature")
	row := `{"schemaVersion":1,"id":"f-child","runId":"mrv-child","scope":"task-done","reviewer":"r","severity":"high",` +
		`"classification":"blocking","status":"open","title":"t","fingerprint":"fp","target":{"type":"beads-task","id":"child-1"},` +
		`"branch":"child-branch","gitHead":"` + head + `"}` + "\n"
	if err := os.MkdirAll(filepath.Join(root, ".metareview"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".metareview", "findings.jsonl"), []byte(row), 0o644); err != nil {
		t.Fatal(err)
	}
	logs, err := childOpenBlockerLogs(root, []string{"child-1"})
	if err != nil || len(logs) != 1 || logs[0].Target != "child-1" {
		t.Fatalf("the child's blocker, recorded on its own branch, must surface in the epic: %+v %v", logs, err)
	}
}
