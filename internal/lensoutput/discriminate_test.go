package lensoutput

import (
	"encoding/json"
	"os"
	"testing"
)

// TestRenameCaseDiscriminates proves the kept-rename-accumulation case FAILS when the
// anchor map drops later hunks of a file (the regression the review found undetectable):
// new-side line 40 is covered only by the rename entry's third hunk (37..46); the first
// two hunks' ±10 slack ends at 23 and 38.
func TestRenameCaseDiscriminates(t *testing.T) {
	diffB, _ := os.ReadFile("testdata/hard-pr.diff")
	full := AnchorMap(string(diffB))
	dropped := map[string][]LineRange{}
	for f, rs := range full { // simulate "keep only the first hunk per file"
		if len(rs) > 0 {
			dropped[f] = rs[:1]
		}
	}
	var corpus struct {
		Cases []struct {
			Name  string          `json:"name"`
			Entry json.RawMessage `json:"entry"`
			Want  string          `json:"want"`
		} `json:"cases"`
	}
	cb, _ := os.ReadFile("testdata/corpus.json")
	if err := json.Unmarshal(cb, &corpus); err != nil {
		t.Fatal(err)
	}
	found := false // the case must exist: a renamed/removed corpus case would otherwise pass vacuously (PR #162 review finding)
	for _, c := range corpus.Cases {
		if c.Name != "kept-rename-accumulation" {
			continue
		}
		found = true
		var f TypedFinding
		if err := json.Unmarshal(c.Entry, &f); err != nil {
			t.Fatal(err)
		}
		if !AnchorInDiff(f, full) {
			t.Fatalf("case must pass with full accumulation: %v", f)
		}
		if AnchorInDiff(f, dropped) {
			t.Fatalf("case must FAIL when later hunks are dropped — it does not discriminate: %v", f)
		}
	}
	if !found {
		t.Fatal("corpus case 'kept-rename-accumulation' not found — it was renamed or removed; update this test")
	}
}
