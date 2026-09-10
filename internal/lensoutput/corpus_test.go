package lensoutput

import (
	"encoding/json"
	"os"
	"testing"
)

// TestConformanceCorpus is the typed-contract conformance corpus: data-driven cases over a
// hard-PR fixture (a large multi-hunk, multi-file diff with a rename, realistic anchors,
// and line numbers chosen to sit inside hunks, inside slack, and outside both). Every case
// pins the bucket a lens's entry must land in — adding a case is a data edit, not a code
// edit, so regression pinning stays cheap while the contract stays frozen.
//
// The corpus is wired into CI two ways: it runs in the ordinary `go test ./...` sweep, and
// tests/go/test-lens-conformance.sh runs it explicitly (plus the judge cap-retry corpus) so
// a conformance failure is legible as a contract failure, not one lost among unit tests.
func TestConformanceCorpus(t *testing.T) {
	diff, err := os.ReadFile("testdata/hard-pr.diff")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Cases []struct {
			Name  string          `json:"name"`
			Entry json.RawMessage `json:"entry"`
			Want  string          `json:"want"`
		} `json:"cases"`
	}
	data, err := os.ReadFile("testdata/corpus.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &corpus); err != nil {
		t.Fatal(err)
	}
	if len(corpus.Cases) == 0 {
		t.Fatal("empty corpus")
	}
	seen := make(map[string]bool)
	for _, c := range corpus.Cases {
		if seen[c.Name] {
			t.Errorf("duplicate case %q", c.Name)
		}
		seen[c.Name] = true
		payload, err := json.Marshal(map[string]any{"findings": []json.RawMessage{c.Entry}})
		if err != nil {
			t.Fatal(err)
		}
		kept, stats := ValidatePayload(payload, string(diff))
		var got string
		switch {
		case stats.Schema == 1:
			got = "schema"
		case stats.Enum == 1:
			got = "enum"
		case stats.Anchor == 1:
			got = "anchor"
		case stats.Suppression == 1:
			got = "suppression"
		case stats.Kept == 1:
			got = "kept"
		default:
			t.Errorf("%s: entry landed in no bucket (stats %+v)", c.Name, stats)
			continue
		}
		if got != c.Want {
			t.Errorf("%s: bucket %q want %q", c.Name, got, c.Want)
		}
		if c.Want == "kept" && len(kept) != 1 {
			t.Errorf("%s: kept %d findings want 1", c.Name, len(kept))
		}
	}
}

// TestConformanceCorpusExhaustiveBuckets guarantees every bucket has at least one pinned
// case: a bucket with no case is a bucket that can silently change behavior.
func TestConformanceCorpusExhaustiveBuckets(t *testing.T) {
	data, err := os.ReadFile("testdata/corpus.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Cases []struct {
			Want string `json:"want"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &corpus); err != nil {
		t.Fatal(err)
	}
	for _, bucket := range []string{"kept", "schema", "enum", "anchor", "suppression"} {
		found := false
		for _, c := range corpus.Cases {
			if c.Want == bucket {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("bucket %q has no corpus case", bucket)
		}
	}
}
