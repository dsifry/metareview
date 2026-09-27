package epicready

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An incremental review (#176) records the token it was asked for, beside the checkpoint SHA it resolved to.
func TestEpicReadyIncrementalRecordsTheToken(t *testing.T) {
	root := epicRepo(t)
	if _, err := Create(root, "epic-1", Options{Base: "main", Incremental: true}); err != nil {
		t.Fatal(err)
	}
	runs, err := os.ReadFile(filepath.Join(root, ".metareview", "runs.jsonl"))
	if err != nil || !strings.Contains(string(runs), `"requestedBase":"last-reviewed"`) {
		t.Fatalf("the run record must name the last-reviewed token (%v):\n%s", err, runs)
	}
}
