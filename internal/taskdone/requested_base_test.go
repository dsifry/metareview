package taskdone

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// #175: the run record and the committed context pack name the --base as typed beside the SHA it resolved to.
func TestTaskDoneRecordsTheRequestedBase(t *testing.T) {
	root := smallTaskRepo(t)
	result, err := Create(root, smallTarget, Options{Base: "main"})
	if err != nil {
		t.Fatal(err)
	}
	runs, err := os.ReadFile(filepath.Join(root, ".metareview", "runs.jsonl"))
	if err != nil || !strings.Contains(string(runs), `"requestedBase":"main"`) {
		t.Fatalf("the task-done run record must carry requestedBase (%v):\n%s", err, runs)
	}
	pack, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(result.ContextRel)))
	if err != nil || !strings.Contains(string(pack), "- Requested base: `main`") {
		t.Fatalf("the context pack must name the requested base (%v):\n%s", err, pack)
	}
}

// An incremental review (#176) records the token it was asked for, beside the checkpoint SHA it resolved to.
func TestTaskDoneIncrementalRecordsTheToken(t *testing.T) {
	root := smallTaskRepo(t)
	if _, err := Create(root, smallTarget, Options{Base: "main", Incremental: true}); err != nil {
		t.Fatal(err)
	}
	runs, err := os.ReadFile(filepath.Join(root, ".metareview", "runs.jsonl"))
	if err != nil || !strings.Contains(string(runs), `"requestedBase":"last-reviewed"`) {
		t.Fatalf("the run record must name the last-reviewed token (%v):\n%s", err, runs)
	}
}
