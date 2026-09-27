package prready

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// #175: the run record and the committed context pack name the --base as typed beside the SHA it resolved to.
func TestPRReadyRecordsTheRequestedBase(t *testing.T) {
	root := smallPRReadyRepo(t)
	result, err := Create(root, Options{Base: "main"})
	if err != nil {
		t.Fatal(err)
	}
	runs, err := os.ReadFile(filepath.Join(root, ".metareview", "runs.jsonl"))
	if err != nil || !strings.Contains(string(runs), `"requestedBase":"main"`) {
		t.Fatalf("the pr-ready run record must carry requestedBase (%v):\n%s", err, runs)
	}
	pack, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(result.ContextRel)))
	if err != nil || !strings.Contains(string(pack), "- Requested base: `main`") {
		t.Fatalf("the context pack must name the requested base (%v):\n%s", err, pack)
	}
}

// An incremental pr-ready (#176) reviews from the checkpoint but scopes blockers to the whole branch, and records
// the token it was asked for.
func TestPRReadyIncrementalRecordsTheToken(t *testing.T) {
	root := smallPRReadyRepo(t)
	if _, err := Create(root, Options{Base: "HEAD", Incremental: true}); err != nil {
		t.Fatal(err)
	}
	runs, err := os.ReadFile(filepath.Join(root, ".metareview", "runs.jsonl"))
	if err != nil || !strings.Contains(string(runs), `"requestedBase":"last-reviewed"`) {
		t.Fatalf("the run record must name the last-reviewed token (%v):\n%s", err, runs)
	}
}

// When the whole-branch blocker scope cannot be worked out, the incremental review fails rather than narrowing it.
func TestPRReadyIncrementalFailsWithoutAWholeBranchScope(t *testing.T) {
	root := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"config", "user.email", "t@t"}, {"config", "user.name", "t"}} {
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %v %s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "."}, {"commit", "-q", "-m", "only"}} {
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %v %s", args, err, out)
		}
	}
	// One commit on main: an explicit HEAD base resolves, but no default base (no fork point, no HEAD~1) exists.
	if _, err := Create(root, Options{Base: "HEAD", Incremental: true}); err == nil {
		t.Fatal("an incremental review with no whole-branch scope must fail")
	}
}
