package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
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
	diffs, missing, fatal := loadDiffs("testdata/mini", records)
	if fatal != nil {
		t.Fatal(fatal)
	}
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
	diffs, _, fatal := loadDiffs("testdata/mini", records)
	if fatal != nil {
		t.Fatal(fatal)
	}
	var buf bytes.Buffer
	if err := report(&buf, records, diffs, options{dir: "testdata/mini", framework: "test-fw", verbose: true, limit: 1}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "evidence: spec/models/widget_spec.rb") {
		t.Errorf("verbose output must name the covering spec:\n%s", out)
	}
	if strings.Count(out, "evidence: ") != 1 {
		t.Errorf("limit 1 must print exactly one claim detail record:\n%s", out)
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
	if _, _, fatal := loadDiffs(dir, []record{{URL: url}}); fatal == nil {
		t.Fatal("an unparsable cached diff must be a hard error, not ignored")
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
	diffs, _, fatal := loadDiffs("testdata/mini", records)
	if fatal != nil {
		t.Fatal(fatal)
	}
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

// The confirmed P2s: an empty corpus must not print NaN%, and a corrupt (unparsable)
// cached diff must fail the run loudly — only a MISSING cache file may be skipped.
func TestReportEmptyCorpusHasNoNaN(t *testing.T) {
	var buf bytes.Buffer
	if err := report(&buf, nil, map[string]string{}, options{dir: "testdata/mini", framework: "none"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "NaN") {
		t.Errorf("empty corpus printed NaN:\n%s", buf.String())
	}
}

func TestEvaluateFailsOnUnparsableCacheButSkipsMissing(t *testing.T) {
	// missing: reported to stderr, report continues, exit clean
	var out, errb bytes.Buffer
	if err := evaluate(options{dir: "testdata/mini", framework: "test-fw"}, &out, &errb); err != nil {
		t.Fatalf("a merely missing diff must not fail the run: %v", err)
	}
	if !strings.Contains(errb.String(), "diff for") {
		t.Errorf("the missing diff must be named on stderr:\n%s", errb.String())
	}

	// unparsable: the tool's data is corrupt; fail loudly instead of reporting an
	// all-no-evidence matrix with a success exit code
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "runs", "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".cache", "pr_diffs"), 0o755); err != nil {
		t.Fatal(err)
	}
	url := "https://github.com/org/repo/pull/1"
	sum := sha1.Sum([]byte(url))
	if err := os.WriteFile(filepath.Join(dir, ".cache", "pr_diffs", hex.EncodeToString(sum[:])[:16]+".json"), []byte("{bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "runs", "x", "readjudication3.json"),
		[]byte(`{"run_id":"x","url":"`+url+`","framework":"f","records":[{"issue_text":"no tests","source_lens":"l","old_verdict":"h","new_verdict":"h"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := evaluate(options{dir: dir, framework: "f"}, &out, &errb); err == nil {
		t.Fatal("an unparsable cached diff must fail the run, not silently report no-evidence")
	}
}

// The confirmed shard-review P2s: skipped claims (no cached diff) must be disclosed in the
// report so the matrix sums visibly; every missing URL is named; clip never splits a rune.
func TestReportDisclosesSkippedClaims(t *testing.T) {
	// r2's URL has no cached diff: its one claim is counted but skipped from the matrix.
	records, err := loadRecords("testdata/mini", "test-fw")
	if err != nil {
		t.Fatal(err)
	}
	_, missing, fatal := loadDiffs("testdata/mini", records)
	if fatal != nil {
		t.Fatal(fatal)
	}
	var out, errb bytes.Buffer
	if err := evaluate(options{dir: "testdata/mini", framework: "test-fw"}, &out, &errb); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, "claims skipped (no cached diff): 1") {
		t.Errorf("the report must disclose skipped claims so the matrix visibly sums:\n%s", s)
	}
	// every missing URL is named on stderr, not one map-iteration-random victim
	if !strings.Contains(errb.String(), "org/repo/pull/2") {
		t.Errorf("the missing URL must be named:\n%s", errb.String())
	}
	if missing == nil || !strings.Contains(missing.Error(), "org/repo/pull/2") {
		t.Errorf("loadDiffs' missing error must name the URL: %v", missing)
	}
}

func TestClipIsRuneSafe(t *testing.T) {
	// a multibyte char at the cut boundary must not be split
	s := strings.Repeat("é", 10) // 20 bytes
	if c := clip(s, 7); !utf8.ValidString(c) {
		t.Errorf("clip produced invalid UTF-8: %q", c)
	}
}

// An entry that is valid JSON but carries an empty diff is corrupt data wearing a valid
// shape: it must be the hard error, not a silent all-no-evidence matrix.
func TestEmptyCachedDiffIsAFatalError(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "runs", "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".cache", "pr_diffs"), 0o755); err != nil {
		t.Fatal(err)
	}
	url := "https://github.com/org/repo/pull/1"
	sum := sha1.Sum([]byte(url))
	if err := os.WriteFile(filepath.Join(dir, ".cache", "pr_diffs", hex.EncodeToString(sum[:])[:16]+".json"), []byte(`{"diff":""}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "runs", "x", "readjudication3.json"),
		[]byte(`{"run_id":"x","url":"`+url+`","framework":"f","records":[{"issue_text":"no tests","source_lens":"l","old_verdict":"h","new_verdict":"h"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if err := evaluate(options{dir: dir, framework: "f"}, &out, &errb); err == nil {
		t.Fatal("an empty cached diff must fail the run, not report no-evidence")
	}
}

// A flag misuse must say WHY it failed, not exit 2 into silence.
func TestRealMainNamesFlagErrors(t *testing.T) {
	origExit, origArgs, origStderr := osExit, os.Args, os.Stderr
	t.Cleanup(func() { osExit, os.Args, os.Stderr = origExit, origArgs, origStderr })
	captured := -1
	osExit = func(c int) { captured = c }
	os.Args = []string{"claimcheck-eval", "-limit", "notanumber"}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	main()
	_ = w.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	if captured != 2 {
		t.Fatalf("exit = %d, want 2", captured)
	}
	if buf.String() == "" {
		t.Error("a flag error must be named on stderr, not a bare exit code")
	}
}

// CodeRabbit #145: a cache path that exists but cannot be read (a directory, a
// permission failure) is NOT "missing" — it is corrupt data and must be fatal.
func TestUnreadableCachedDiffIsFatal(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "runs", "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".cache", "pr_diffs"), 0o755); err != nil {
		t.Fatal(err)
	}
	url := "https://github.com/org/repo/pull/1"
	sum := sha1.Sum([]byte(url))
	// a directory where the cache file belongs: ReadFile fails with EISDIR, not ENOENT
	if err := os.Mkdir(filepath.Join(dir, ".cache", "pr_diffs", hex.EncodeToString(sum[:])[:16]+".json"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "runs", "x", "readjudication3.json"),
		[]byte(`{"run_id":"x","url":"`+url+`","framework":"f","records":[{"issue_text":"no tests","source_lens":"l","old_verdict":"h","new_verdict":"h"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if err := evaluate(options{dir: dir, framework: "f"}, &out, &errb); err == nil {
		t.Fatal("an unreadable (non-missing) cached diff must fail the run")
	}
}

// CodeRabbit #145: a gap claim whose own file is a changed test file must not use that
// file as covering evidence — the finding's file is derived from the leading path in
// issue_text and passed through to run.Finding.
func TestFindingFileDerivedFromLeadingPath(t *testing.T) {
	for _, tc := range []struct {
		text string
		want string
	}{
		{"app/models/topic_embed.rb:36-43 — no test verifies the category", "app/models/topic_embed.rb"},
		{"spec/models/topic_embed_spec.rb has no test verifying anything", "spec/models/topic_embed_spec.rb"},
		{"no tests at all for anything", ""},
	} {
		if got := findingFileFromText(tc.text); got != tc.want {
			t.Errorf("findingFileFromText(%q) = %q, want %q", tc.text, got, tc.want)
		}
	}
}
