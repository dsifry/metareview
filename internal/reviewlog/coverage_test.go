package reviewlog

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// --- pure helpers ---

func TestPreviousRunID(t *testing.T) {
	if previousRunID(" none ") != "" {
		t.Error("'none' -> empty")
	}
	if previousRunID(" mrv-x ") != "mrv-x" {
		t.Error("a real id is trimmed and returned")
	}
}

func TestDecodeAttempt(t *testing.T) {
	cases := []struct {
		in   string
		a, m int
	}{
		{"2/3", 2, 3},
		{"noslash", 0, 0},
		{"1/2/3", 0, 0},
		{"x/3", 0, 0},
		{"0/3", 0, 0},
	}
	for _, c := range cases {
		if a, m := decodeAttempt(c.in); a != c.a || m != c.m {
			t.Errorf("decodeAttempt(%q) = %d,%d want %d,%d", c.in, a, m, c.a, c.m)
		}
	}
}

func TestReviewKind(t *testing.T) {
	cases := map[string]string{
		"# metareview: artifact review": "artifact",
		"task-done review":              "task-done",
		"epic-ready review":             "epic-ready",
		"pr-ready review":               "pr-ready",
		"something else":                "",
	}
	for line, want := range cases {
		if got := reviewKind(line); got != want {
			t.Errorf("reviewKind(%q) = %q want %q", line, got, want)
		}
	}
}

func TestRunDate(t *testing.T) {
	if runDate("short") != "" {
		t.Error("an id without a date segment -> empty")
	}
	if runDate("mrv-20260101-123") != "20260101" {
		t.Errorf("runDate should extract the date: %q", runDate("mrv-20260101-123"))
	}
}

func TestSameLensSet(t *testing.T) {
	if sameLensSet([]string{"a"}, []string{"a", "b"}) {
		t.Error("different lengths are not equal")
	}
	if sameLensSet([]string{"a", "b"}, []string{"a", "c"}) {
		t.Error("a differing member is not equal")
	}
	if !sameLensSet([]string{"a", "b"}, []string{"b", "a"}) {
		t.Error("same set, any order, is equal")
	}
}

func TestReviewerVerdictComplete(t *testing.T) {
	for _, v := range []string{"PASS", "pass_advisory", "NEEDS_REVISION", "ESCALATE", "NOT_APPLICABLE"} {
		if !reviewerVerdictComplete(v) {
			t.Errorf("%q should be complete", v)
		}
	}
	if reviewerVerdictComplete("MAYBE") {
		t.Error("an unknown verdict is not complete")
	}
}

func TestEncodeCoveredPaths(t *testing.T) {
	if EncodeCoveredPaths(nil) != NoCoveredPaths {
		t.Error("no paths -> the no-covered sentinel")
	}
	got := EncodeCoveredPaths([]string{"src/a.go"})
	if !strings.Contains(got, "src/a.go") {
		t.Errorf("covered paths should be JSON-encoded: %q", got)
	}
}

// --- readFindings / readLocalRunMetadata ---

func TestReadFindingsAndLocalRuns(t *testing.T) {
	// Missing files are not errors.
	if recs, err := readFindings(t.TempDir()); err != nil || len(recs) != 0 {
		t.Fatalf("readFindings missing: %v %v", recs, err)
	}
	if recs, err := readLocalRuns(t.TempDir()); err != nil || recs != nil {
		t.Fatalf("readLocalRuns missing: %v %v", recs, err)
	}
	// Blank lines skipped; corrupt lines error.
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, ".metareview", "findings.jsonl"), "\n{\"id\":\"a\"}\n")
	if recs, err := readFindings(root); err != nil || len(recs) != 1 {
		t.Fatalf("readFindings valid: %v %v", recs, err)
	}
	bad := t.TempDir()
	mustWrite(t, filepath.Join(bad, ".metareview", "findings.jsonl"), "{bad\n")
	if _, err := readFindings(bad); err == nil {
		t.Fatal("corrupt findings line should error")
	}
	badRuns := t.TempDir()
	mustWrite(t, filepath.Join(badRuns, ".metareview", "runs.jsonl"), "\n{bad\n")
	if _, err := readLocalRuns(badRuns); err == nil {
		t.Fatal("corrupt runs line should error")
	}
	// A valid local run parses.
	okRuns := t.TempDir()
	mustWrite(t, filepath.Join(okRuns, ".metareview", "runs.jsonl"), `{"id":"mrv-a"}`+"\n")
	if recs, err := readLocalRuns(okRuns); err != nil || len(recs) != 1 {
		t.Fatalf("readLocalRuns valid: %v %v", recs, err)
	}
}

func TestReadFindingsOpenError(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("unreadable-file perms do not hold for root/Windows")
	}
	root := t.TempDir()
	p := filepath.Join(root, ".metareview", "findings.jsonl")
	mustWrite(t, p, "{}")
	if err := os.Chmod(p, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
	if _, err := readFindings(root); err == nil {
		t.Fatal("unreadable findings.jsonl should surface an open error")
	}
	// Same for runs.jsonl.
	root2 := t.TempDir()
	p2 := filepath.Join(root2, ".metareview", "runs.jsonl")
	mustWrite(t, p2, "{}")
	if err := os.Chmod(p2, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(p2, 0o644) })
	if _, err := readLocalRuns(root2); err == nil {
		t.Fatal("unreadable runs.jsonl should surface an open error")
	}
}

// --- Discover error branches ---

func TestDiscoverErrorBranches(t *testing.T) {
	// readFindings error (corrupt findings.jsonl).
	r1 := t.TempDir()
	mustWrite(t, filepath.Join(r1, ".metareview", "findings.jsonl"), "{bad\n")
	if _, err := Discover(r1); err == nil {
		t.Error("corrupt findings -> Discover error")
	}
	// runchain.ReadRuns error (corrupt runs.jsonl, findings absent).
	r2 := t.TempDir()
	mustWrite(t, filepath.Join(r2, ".metareview", "runs.jsonl"), "{bad\n")
	if _, err := Discover(r2); err == nil {
		t.Error("corrupt runs -> Discover error")
	}
	// readLocalRuns error, isolated via the seam (a corrupt runs.jsonl would fail runchain first).
	r3 := t.TempDir()
	orig := readLocalRuns
	t.Cleanup(func() { readLocalRuns = orig })
	readLocalRuns = func(string) ([]localRunMetadata, error) { return nil, errors.New("local boom") }
	if _, err := Discover(r3); err == nil || !strings.Contains(err.Error(), "local boom") {
		t.Errorf("readLocalRuns error should propagate: %v", err)
	}
	readLocalRuns = orig
	// No reviews dir -> empty, no error.
	r4 := t.TempDir()
	mustWrite(t, filepath.Join(r4, ".metareview", "findings.jsonl"), "")
	if logs, err := Discover(r4); err != nil || len(logs) != 0 {
		t.Errorf("no reviews dir -> empty: %v %v", logs, err)
	}
	// reviews is a file -> ReadDir error.
	r5 := t.TempDir()
	mustWrite(t, filepath.Join(r5, "docs", "metareview", "reviews"), "x")
	if _, err := Discover(r5); err == nil {
		t.Error("reviews-as-file -> ReadDir error")
	}
	// A subdir and a non-.md file in reviews are skipped.
	r6 := t.TempDir()
	mustWrite(t, filepath.Join(r6, "docs", "metareview", "reviews", "notes.txt"), "x")
	if err := os.MkdirAll(filepath.Join(r6, "docs", "metareview", "reviews", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(r6, "docs", "metareview", "reviews", "a.md"), reviewMarkdown("mrv-a", "task-1", "PASS", ""))
	logs, err := Discover(r6)
	if err != nil || len(logs) != 1 {
		t.Fatalf("non-md entries skipped, one md read: %v %v", logs, err)
	}
}

func TestDiscoverUnreadableReviewFile(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("unreadable-file perms do not hold for root/Windows")
	}
	root := t.TempDir()
	md := filepath.Join(root, "docs", "metareview", "reviews", "a.md")
	mustWrite(t, md, reviewMarkdown("mrv-a", "task-1", "PASS", ""))
	if err := os.Chmod(md, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(md, 0o644) })
	if _, err := Discover(root); err == nil {
		t.Fatal("an unreadable review .md should surface a read error")
	}
}

// A review log whose run id has a broken chain in runs.jsonl records a warning (ChainTo failure).
func TestDiscoverRecordsBrokenChainWarning(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "a.md"), reviewMarkdown("mrv-child", "task-1", "NEEDS_REVISION", ""))
	// runs.jsonl authenticates mrv-child but its previous run is missing, so ChainTo fails.
	mustWrite(t, filepath.Join(root, ".metareview", "runs.jsonl"),
		`{"id":"mrv-child","previousRunId":"mrv-gone","scope":"task-done"}`+"\n")
	logs, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected one log, got %d", len(logs))
	}
	found := false
	for _, w := range logs[0].Warnings {
		if strings.Contains(w, "run chain unavailable") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a broken-chain warning, got %+v", logs[0].Warnings)
	}
}

func TestForTargetPropagatesDiscoverError(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, ".metareview", "findings.jsonl"), "{bad\n")
	if _, err := ForTarget(root, "task-1"); err == nil {
		t.Fatal("ForTarget should surface the Discover error")
	}
}

func TestSameLensSetDuplicateInA(t *testing.T) {
	// Equal outer lengths but a repeats a name: the unique-count check rejects it.
	if sameLensSet([]string{"x", "x"}, []string{"x", "y"}) {
		t.Error("a repeated lens must not match a two-member set")
	}
}

func TestNextNonEmpty(t *testing.T) {
	lines := []string{"a", "", "b"}
	if nextNonEmpty(lines, 1) != "b" {
		t.Errorf("nextNonEmpty should skip blanks: %q", nextNonEmpty(lines, 1))
	}
	if nextNonEmpty([]string{"a", "", "  "}, 1) != "" {
		t.Error("all-blank from start -> empty")
	}
}

func TestReadLocalRunsScannerError(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, ".metareview", "runs.jsonl"), strings.Repeat("a", (1<<20)+16)+"\n")
	if _, err := readLocalRuns(root); err == nil {
		t.Fatal("an over-long runs line should surface a scanner error")
	}
}
