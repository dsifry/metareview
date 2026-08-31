package contextpack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dsifry/metareview/internal/lens"
)

// The Suggested Reviewers block must list exactly the canonical lens set (lens.Displays()), in order.
// This is the guard that stops the list drifting: it silently sat at the original five names for
// two releases (security / testing-quality / data-migration already required) because it was a
// hard-coded copy divorced from the source. The block is now derived from lens.Displays(), so this
// test fails if anyone re-hard-codes a divergent list, drops a lens, or lets the two get out of
// sync — adding a lens to lens.Displays() updates the block by construction and keeps this green.
func TestSuggestedReviewersDeriveFromCanonicalLenses(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "A.md"), []byte("# artifact\n\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Build(root, "A.md", time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(res.ContextRel)))
	if err != nil {
		t.Fatal(err)
	}
	idx := strings.Index(string(body), "## Suggested Reviewers")
	if idx < 0 {
		t.Fatal("context pack has no Suggested Reviewers section")
	}
	var got []string
	for _, line := range strings.Split(string(body)[idx:], "\n") {
		if strings.HasPrefix(line, "- ") {
			got = append(got, strings.TrimPrefix(line, "- "))
		}
	}
	if len(got) != len(lens.Displays()) {
		t.Fatalf("Suggested Reviewers lists %d lenses, lens.Displays() has %d: %v", len(got), len(lens.Displays()), got)
	}
	for i, want := range lens.Displays() {
		if got[i] != want {
			t.Errorf("reviewer %d = %q, want %q (the block must derive from lens.Displays(), not a copy)", i, got[i], want)
		}
	}
}
