package epicready

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/reviewlog"
)

// The writer seam, end to end. Iteration 0 found "the writer is untested while the parser is
// tested" and the fix added a seam test for taskdone ONLY — reproducing, in the tests, the exact
// three-of-four omission the finding was about. Verified before this existed: deleting the two
// population lines from Create()'s meta literal left the whole suite green while every log wrote
// `Head: unknown` / `Covered paths: none`, putting the review out of branch scope and leaving
// every file it read UNREVIEWED in any clone.
//
// .metareview is REMOVED before reading, because the run record wins over the log where it
// exists, so discovering in place cannot tell a working writer from a broken one. That removal is
// the clone, worktree or CI checkout these fields exist for.
func TestCreateRecordsScopeInTheCommittedLog(t *testing.T) {
	root := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".beads/issues.jsonl", `{"id":"task-1","title":"T","description":"d","acceptance":["a"],"status":"closed"}`+"\n"+`{"id":"epic-1","title":"E","description":"d","acceptance":["a"],"children":["task-1"]}`+"\n")
	write("lib/keep.go", "package lib\n")
	git("init", "-q", "-b", "main")
	git("add", ".")
	git("commit", "-qm", "base")
	base := git("rev-parse", "HEAD")
	write("lib/od,d.go", "package lib // a comma in a path, which git prints unquoted\n")
	git("add", ".")
	git("commit", "-qm", "work")
	head := git("rev-parse", "HEAD")

	if _, err := Create(root, "epic-1", Options{Base: base}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := os.RemoveAll(filepath.Join(root, ".metareview")); err != nil {
		t.Fatal(err)
	}
	logs, err := reviewlog.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) == 0 {
		t.Fatal("no review log was written")
	}
	got := logs[0]
	if got.HeadSHA != head {
		t.Errorf("HeadSHA = %q, want the real head %q", got.HeadSHA, head)
	}
	if !got.CoveredPathsKnown {
		t.Fatal("the review must record WHICH files it read, not leave it unknown")
	}
	var sawComma bool
	for _, p := range got.CoveredPaths {
		if p == "lib/od,d.go" {
			sawComma = true
		}
		if p == "lib/od" || p == "d.go" {
			t.Errorf("the comma path was split on the round trip: %v", got.CoveredPaths)
		}
	}
	if !sawComma {
		t.Errorf("covered paths %v is missing the changed file", got.CoveredPaths)
	}
}
