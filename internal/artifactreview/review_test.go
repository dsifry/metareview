package artifactreview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/reviewlog"
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

// Round-trip: a scaffold this package writes, completed with the lens rows this package's own
// prompt list names, must read as complete in internal/reviewlog.
//
// The hand-written fixtures in reviewlog_test.go cannot catch a disagreement between producer and
// consumer, because the fixture author writes both halves. They missed exactly that: the scaffold
// declares "scope-alignment" (normalizing to scopealignment) while the reviewer row it asks for is
// "Scope and alignment" (scopeandalignment), so a fully completed new review stayed flagged
// forever - the standing-override failure the lens-era change exists to remove, reintroduced.
func TestCompletedScaffoldReadsAsCompleteInReviewLog(t *testing.T) {
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
	path := filepath.Join(root, filepath.FromSlash(res.ReviewRel))
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Complete the scaffold the way a reviewer would: a verdict, and one row per lens named in the
	// scaffold's own Reviewer Prompts list.
	text := strings.Replace(string(body), "## Verdict\n\nNOT_REVIEWED", "## Verdict\n\nPASS", 1)
	rows := ""
	for _, lens := range []string{"Feasibility", "Completeness", "Scope and alignment", "Architecture", "Intent preservation", "Security", "Testing-quality", "Data-migration"} {
		rows += "| " + lens + " | PASS | 0 | 0 | ok |\n"
	}
	text = strings.Replace(text, "| --- | --- | ---: | ---: | --- |\n", "| --- | --- | ---: | ---: | --- |\n"+rows, 1)
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}

	logs, err := reviewlog.Discover(root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	var found bool
	for _, log := range logs {
		if log.RunID != res.RunID {
			continue
		}
		found = true
		if log.HasUnresolvedBlockers {
			t.Errorf("a completed artifact review must not report unresolved blockers; declared lens set does not match the reviewer rows the scaffold asks for")
		}
	}
	if !found {
		t.Fatalf("review log %s not discovered", res.RunID)
	}
}
