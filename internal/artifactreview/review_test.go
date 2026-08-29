package artifactreview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The scaffold must declare the lens set it was written under. This is the producing half of the
// grandfathering contract in internal/reviewlog: a log that does not declare its set is judged by
// the set current at its run date, so a scaffold that silently lost this line would make every
// review written afterwards answerable to lenses that need not have existed when it ran.
func TestScaffoldDeclaresItsLensSet(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".metareview"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "A.md"), []byte("# artifact\n\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Create(root, "A.md", "", time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(res.ReviewRel)))
	if err != nil {
		t.Fatalf("read review: %v", err)
	}
	// Assert on the marker line itself. Every lens name also appears in the scaffold's Reviewer
	// Prompts list, so a whole-document Contains passes even when the marker declares one lens -
	// verified by mutation: truncating the declared set to Reviewers[:1] left a document-wide
	// check green.
	var marker string
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, "Required lenses:") {
			marker = line
			break
		}
	}
	if marker == "" {
		t.Fatal("the artifact scaffold must record the lens set it was written under")
	}
	for _, lens := range []string{"feasibility", "completeness", "scope-alignment", "architecture", "intent-preservation", "security", "testing-quality", "data-migration"} {
		if !strings.Contains(marker, lens) {
			t.Errorf("declared lens set is missing %q: %s", lens, marker)
		}
	}
}
