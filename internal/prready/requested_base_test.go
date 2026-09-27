package prready

import (
	"os"
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
