package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The fixture is a miniature harnesseval: one run whose PR diff carries a covering spec
// (one hallucinated gap-claim, one real gap-claim, one non-claim bug), and one run on a
// URL with no cached diff (its claim must be counted as made, then skipped from the
// matrix rather than sinking the report).
func TestReportAgainstMiniCorpus(t *testing.T) {
	records, err := loadRecords("testdata/mini", "test-fw")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 4 {
		t.Fatalf("records = %d, want 4", len(records))
	}
	diffs, missing := loadDiffs("testdata/mini", records)
	if missing == nil {
		t.Fatal("the uncached PR must surface as a missing-diff error")
	}
	if len(diffs) != 1 {
		t.Fatalf("cached diffs = %d, want 1", len(diffs))
	}

	var buf bytes.Buffer
	if err := report(&buf, records, diffs, options{dir: "testdata/mini", framework: "test-fw"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"records: 4  claims detected: 3 (75.0%)",
		"hallucinated gap-claims with covering evidence in the diff: 1/1",
		"test-fw-lens/testing-quality",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
	}
	// the hallucinated claim found the covering spec; the true gap-claim did not
	if !strings.Contains(out, "evidence") || !strings.Contains(out, "no-evidence") {
		t.Errorf("matrix rows missing:\n%s", out)
	}
	// the skipped claim (uncached PR) is counted as a claim but not judged: the
	// testing-quality lens line carries only its judged claim, not the skipped one.
	if !strings.Contains(out, "test-fw-lens/testing-quality") || strings.Contains(out, "2 /    1 records") {
		t.Errorf("per-lens claim counts wrong:\n%s", out)
	}
}

func TestReportVerboseAndLimit(t *testing.T) {
	records, err := loadRecords("testdata/mini", "test-fw")
	if err != nil {
		t.Fatal(err)
	}
	diffs, _ := loadDiffs("testdata/mini", records)
	var buf bytes.Buffer
	if err := report(&buf, records, diffs, options{dir: "testdata/mini", framework: "test-fw", verbose: true, limit: 1}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "evidence: spec/models/widget_spec.rb") {
		t.Errorf("verbose output must name the covering spec:\n%s", out)
	}
	if strings.Contains(out, "evidence: \n") && strings.Count(out, "evidence: ") > 1 {
		t.Errorf("limit 1 must print one claim detail:\n%s", out)
	}
}

func TestFrameworkFilter(t *testing.T) {
	records, err := loadRecords("testdata/mini", "other-fw")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Errorf("records = %d, want 0 for a non-matching framework", len(records))
	}
	var buf bytes.Buffer
	if err := report(&buf, records, map[string]string{}, options{dir: "testdata/mini", framework: "other-fw"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "claims detected: 0") {
		t.Errorf("empty corpus report wrong:\n%s", buf.String())
	}
}

func TestClip(t *testing.T) {
	if clip("short", 10) != "short" {
		t.Error("clip must not touch short strings")
	}
	if c := clip("a-very-long-finding-text", 6); !strings.HasPrefix(c, "a-very") || !strings.HasSuffix(c, "…") {
		t.Errorf("clip = %q", c)
	}
}

func TestRealMainAndMain(t *testing.T) {
	origExit, origArgs := osExit, os.Args
	t.Cleanup(func() { osExit, os.Args = origExit, origArgs })
	captured := -1
	osExit = func(c int) { captured = c }

	// main() is only the exit wrapper around realMain; a clean run exits 0.
	os.Args = []string{"claimcheck-eval", "-harnesseval", "testdata/mini", "-framework", "test-fw"}
	main()
	if captured != 0 {
		t.Fatalf("main on the mini corpus: exit %d, want 0", captured)
	}

	// a corpus that does not parse exits 1 with the error named.
	os.Args = []string{"claimcheck-eval", "-harnesseval", "testdata/bad", "-framework", "test-fw"}
	main()
	if captured != 1 {
		t.Fatalf("main on a malformed corpus: exit %d, want 1", captured)
	}

	// flag misuse exits 2 without touching the corpus.
	os.Args = []string{"claimcheck-eval", "-nope"}
	main()
	if captured != 2 {
		t.Fatalf("main with an unknown flag: exit %d, want 2", captured)
	}
}

func TestLoadRecordsRejectsUnreadableFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "runs", "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "runs", "x", "readjudication3.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadRecords(dir, ""); err == nil {
		t.Fatal("a malformed readjudication3.json must be an error, not silent skip")
	}
}

func TestLoadDiffsRejectsUnparsableCache(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, ".cache", "pr_diffs")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	url := "https://github.com/org/repo/pull/9"
	sum := sha1.Sum([]byte(url))
	if err := os.WriteFile(filepath.Join(cache, hex.EncodeToString(sum[:])[:16]+".json"), []byte("{bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadDiffs(dir, []record{{URL: url}}); err == nil {
		t.Fatal("an unparsable cached diff must be an error, not ignored")
	}
}

// failAfter writes n bytes successfully, then fails: the errWriter must stop and report
// the first error rather than spamming a dead stream.
type failAfter struct{ n int }

func (f *failAfter) Write(p []byte) (int, error) {
	if f.n <= 0 {
		return 0, os.ErrClosed
	}
	if len(p) > f.n {
		f.n = 0
		return f.n, os.ErrClosed
	}
	f.n -= len(p)
	return len(p), nil
}

func TestReportSurfacesADeadStream(t *testing.T) {
	records, err := loadRecords("testdata/mini", "test-fw")
	if err != nil {
		t.Fatal(err)
	}
	diffs, _ := loadDiffs("testdata/mini", records)
	if err := report(&failAfter{n: 8}, records, diffs, options{dir: "testdata/mini", framework: "test-fw"}); err == nil {
		t.Fatal("a writer that died mid-report must surface its error")
	}
}

func TestLoadRecordsGlobAndReadFailures(t *testing.T) {
	// an unbalanced bracket in the corpus path makes the glob itself fail
	if _, err := loadRecords("testdata/mini[", ""); err == nil {
		t.Error("a malformed glob pattern must be an error")
	}
	// a readjudication3.json the process cannot read (mode 000) fails the load
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "runs", "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "runs", "x", "readjudication3.json")
	if err := os.WriteFile(p, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
	if _, err := loadRecords(dir, ""); err == nil {
		t.Error("an unreadable readjudication3.json must be an error")
	}
}
