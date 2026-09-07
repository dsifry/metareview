package main

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/dsifry/metareview/internal/fsm/run"
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
	// put the UNCACHED claim first: it is counted as made but skipped, and must not
	// consume the printed-detail limit
	var lead record
	rest := records[:0:0]
	for i, r := range records {
		if r.URL == "https://github.com/org/repo/pull/2" {
			lead = records[i]
		} else {
			rest = append(rest, r)
		}
	}
	ordered := append([]record{lead}, rest...)
	var buf bytes.Buffer
	if err := report(&buf, ordered, diffs, options{dir: "testdata/mini", framework: "test-fw", verbose: true, limit: 1}); err != nil {
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

// Bugbot #145: an empty cached diff must fail with a message that says WHY (empty),
// not a nil-wrapping %!w(<nil>).
func TestEmptyCachedDiffErrorNamesTheProblem(t *testing.T) {
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
	err := evaluate(options{dir: dir, framework: "f"}, &out, &errb)
	if err == nil {
		t.Fatal("an empty cached diff must fail the run")
	}
	if strings.Contains(err.Error(), "%!w") || !strings.Contains(err.Error(), "empty") {
		t.Errorf("the error must name the emptiness, not wrap nil: %v", err)
	}
}

// Bugbot #145: a leading uncached claim must not consume the -verbose limit — the limit
// counts printed detail records, not claims made.
func TestVerboseLimitCountsPrintedDetailsOnly(t *testing.T) {
	// mini corpus: r2's URL is uncached and its record IS a claim (counted, skipped);
	// r1's claim has the covering spec (printed). limit 1 must still print r1's detail.
	records, err := loadRecords("testdata/mini", "test-fw")
	if err != nil {
		t.Fatal(err)
	}
	diffs, _, fatal := loadDiffs("testdata/mini", records)
	if fatal != nil {
		t.Fatal(fatal)
	}
	// put the UNCACHED claim first: it is counted as made but skipped, and must not
	// consume the printed-detail limit
	var lead record
	rest := records[:0:0]
	for i, r := range records {
		if r.URL == "https://github.com/org/repo/pull/2" {
			lead = records[i]
		} else {
			rest = append(rest, r)
		}
	}
	ordered := append([]record{lead}, rest...)
	var buf bytes.Buffer
	if err := report(&buf, ordered, diffs, options{dir: "testdata/mini", framework: "test-fw", verbose: true, limit: 1}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "evidence: spec/models/widget_spec.rb") {
		t.Errorf("a skipped uncached claim consumed the limit; the measurable claim's detail is missing:\n%s", buf.String())
	}
}

// ---- issue #146: the repo-side structural pass over the corpus ----

// gitFixture builds a corpus clone whose refs/pull/N/head carries the given files, the
// shape the operator's -repos directory provides (one clone per PR repo; the pass pins
// each PR's head rev by fetching its pull ref).
func gitFixture(t *testing.T, reposDir string, files map[string]string) (cloneDir, rev string) {
	t.Helper()
	origin := t.TempDir()
	gitRun := func(dir string, args ...string) string {
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	for p, body := range files {
		full := filepath.Join(origin, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitRun(origin, "init", "-q")
	// hermetic identity: the repo's test helpers never depend on the operator's global
	// git config, and neither does this fixture
	gitRun(origin, "config", "user.email", "fixture@example.invalid")
	gitRun(origin, "config", "user.name", "fixture")
	gitRun(origin, "add", "-A")
	gitRun(origin, "commit", "-q", "-m", "spec")
	gitRun(origin, "update-ref", "refs/pull/7/head", "HEAD")
	rev = gitRun(origin, "rev-parse", "refs/pull/7/head")
	cloneDir = filepath.Join(reposDir, "org", "repo")
	if out, err := exec.Command("git", "clone", "-q", origin, cloneDir).CombinedOutput(); err != nil {
		t.Fatalf("clone: %v\n%s", err, out)
	}
	return cloneDir, rev
}

func TestRepoPassResolvesAndSearchesThePinnedHead(t *testing.T) {
	reposDir := t.TempDir()
	gitFixture(t, reposDir, map[string]string{
		"spec/models/widget_spec.rb": "let!(:widget) { Fabricate(:widget) }\nexpect(widget.shine).to eq(true)\n",
	})
	pass, missing := resolveRepos(options{repos: reposDir}, []record{
		{URL: "https://github.com/org/repo/pull/7"},
	}, realGit, realGitRaw)
	if missing != nil {
		t.Fatalf("missing = %v; want none", missing)
	}
	if pass.rev("https://github.com/org/repo/pull/7") == "" {
		t.Fatal("the PR head rev was not pinned")
	}
	ev, err := pass.search("https://github.com/org/repo/pull/7", run.Finding{
		File: "app/models/widget.rb", IssueText: "the widget polish path has no test asserting the shine"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if !ev.Ran || len(ev.Evidence) != 1 || ev.Evidence[0].Path != "spec/models/widget_spec.rb" {
		t.Fatalf("evidence = %+v ran=%v; want the covering spec", ev.Evidence, ev.Ran)
	}
	// A URL with no clone and an unparseable URL are disclosed, not fatal.
	_, missing = resolveRepos(options{repos: reposDir}, []record{
		{URL: "https://github.com/org/absent/pull/1"},
		{URL: "not-a-pr-url"},
	}, realGit, realGitRaw)
	if missing == nil || !strings.Contains(missing.Error(), "org/absent#1") || !strings.Contains(missing.Error(), "not-a-pr-url") {
		t.Errorf("missing = %v; want both disclosed", missing)
	}
}

// With -repos the matrix gains the combined rows. The fixture must truly produce each
// classification (the row labels are printed unconditionally, so asserting the label
// alone is vacuous) — this parses the matrix cells and asserts the hallucination column.
func TestReportReposPassRows(t *testing.T) {
	reposDir := t.TempDir()
	gitFixture(t, reposDir, map[string]string{
		// claim 1's covering spec is in the diff AND at head → "both"
		"spec/models/widget_spec.rb": "let!(:widget) { Fabricate(:widget) }\nexpect(widget.shine).to be_present\n",
		// claim 2's covering spec exists ONLY at head → "repo-only"
		"spec/models/helper_spec.rb": "describe 'the helper module' do\n  it 'asserts something' do\n  end\nend\n",
	})
	records := []record{
		{URL: "https://github.com/org/repo/pull/7", IssueText: "the widget polish path has no test asserting the shine", NewVerdict: "hallucination", SourceLens: "lens-a"},
		{URL: "https://github.com/org/repo/pull/7", IssueText: "zero tests exist for the helper module", NewVerdict: "hallucination", SourceLens: "lens-a"},
		// subject only in the DIFF's spec hunks, absent from the head tree → "diff-only"
		{URL: "https://github.com/org/repo/pull/7", IssueText: "the polish and shimmer behavior is untested", NewVerdict: "hallucination", SourceLens: "lens-b"},
		// subject no source carries → "no-evidence"
		{URL: "https://github.com/org/repo/pull/7", IssueText: "the frobnicate quux behavior is untested", NewVerdict: "hallucination", SourceLens: "lens-b"},
	}
	// the diff carries the widget spec hunks (widget+shine) and a polish spec the head
	// tree does not have; the head tree carries no polish/shimmer content
	diff := "diff --git a/app/models/widget.rb b/app/models/widget.rb\n--- a/app/models/widget.rb\n+++ b/app/models/widget.rb\n@@ -3,2 +3,4 @@\n" +
		"+  def polish(g)\n+    g.try(:shimmer)\n" +
		"diff --git a/spec/models/widget_spec.rb b/spec/models/widget_spec.rb\n--- a/spec/models/widget_spec.rb\n+++ b/spec/models/widget_spec.rb\n@@ -5,2 +5,4 @@\n" +
		"+    let!(:widget) { Fabricate(:widget) }\n+    expect(widget.shine).to be_present\n" +
		"diff --git a/spec/models/polish_spec.rb b/spec/models/polish_spec.rb\n--- /dev/null\n+++ b/spec/models/polish_spec.rb\n@@ -0,0 +1,2 @@\n" +
		"+  it 'has shimmer' do\n+    expect(widget.polish).to be_present\n"
	var buf bytes.Buffer
	if err := report(&buf, records, map[string]string{"https://github.com/org/repo/pull/7": diff},
		options{repos: reposDir, dir: "testdata/mini", framework: "test-fw"}); err != nil {
		t.Fatal(err)
	}
	got := matrixCells(buf.String(), []string{"both", "diff-only", "repo-only", "no-evidence"})
	want := map[string]int{"both": 1, "diff-only": 1, "repo-only": 1, "no-evidence": 1}
	for _, row := range []string{"both", "diff-only", "repo-only", "no-evidence"} {
		if got[row] != want[row] {
			t.Errorf("matrix row %q hallucination cell = %d, want %d\n%s", row, got[row], want[row], buf.String())
		}
	}
}

// matrixCells parses the report's matrix rows into the hallucination column's count.
func matrixCells(out string, rows []string) map[string]int {
	cells := map[string]int{}
	for _, line := range strings.Split(out, "\n") {
		for _, row := range rows {
			if !strings.HasPrefix(line, row) || strings.Contains(line, "evidence\\v2") {
				continue
			}
			fields := strings.Fields(strings.TrimPrefix(line, row))
			if len(fields) >= 3 { // bug, important_non_bug, hallucination, unresolved
				if n, err := strconv.Atoi(fields[2]); err == nil {
					cells[row] = n
				}
			}
		}
	}
	return cells
}

// A claim whose PR has no clone is disclosed and measured on the diff dimension alone;
// the repo dimension must not silently shrink the matrix.
func TestReportReposPassDisclosesMissingClones(t *testing.T) {
	reposDir := t.TempDir()
	gitFixture(t, reposDir, map[string]string{
		"spec/models/widget_spec.rb": "let!(:widget) { Fabricate(:widget) }\nexpect(widget.shine).to eq(true)\n",
	})
	records := []record{
		{URL: "https://github.com/org/repo/pull/7", IssueText: "the widget polish path has no test asserting the shine", NewVerdict: "hallucination", SourceLens: "lens-a"},
		{URL: "https://github.com/org/absent/pull/1", IssueText: "zero tests exist for the helper module", NewVerdict: "hallucination", SourceLens: "lens-a"},
	}
	diff := "diff --git a/spec/models/widget_spec.rb b/spec/models/widget_spec.rb\n--- a/spec/models/widget_spec.rb\n+++ b/spec/models/widget_spec.rb\n@@ -5,2 +5,4 @@\n" +
		"+    expect(widget.shine).to eq(true)\n"
	var buf bytes.Buffer
	if err := report(&buf, records, map[string]string{
		"https://github.com/org/repo/pull/7": diff, "https://github.com/org/absent/pull/1": diff,
	}, options{repos: reposDir, dir: "testdata/mini", framework: "test-fw"}); err != nil {
		t.Fatal(err)
	}
	if out := buf.String(); !strings.Contains(out, "without a repo clone") {
		t.Errorf("missing clones must be disclosed:\n%s", out)
	}
}

// realGit surfaces git's exit code as code (not error) and a failed process start as
// error with code -1.
func TestRealGitSurfacesExitCodesAndStartFailures(t *testing.T) {
	origin := t.TempDir()
	if out, err := exec.Command("git", "-C", origin, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	s, code, err := realGit(context.Background(), origin, "rev-parse", "--is-inside-work-tree")
	if err != nil || code != 0 || s != "true" {
		t.Fatalf("realGit = %q, %d, %v", s, code, err)
	}
	if _, code, err := realGit(context.Background(), origin, "rev-parse", "NOPE"); err != nil || code == 0 {
		t.Fatalf("a git failure must travel as code: %d, %v", code, err)
	}
	if _, code, err := realGit(context.Background(), filepath.Join(origin, "missing"), "status"); err == nil || code != -1 {
		t.Fatalf("a start failure must be an error: %d, %v", code, err)
	}
}

// The resolve step's failure modes are each disclosed: a fetch that fails and a
// rev-parse that fails on an existing clone.
func TestRepoPassFetchAndRevFailuresAreDisclosed(t *testing.T) {
	reposDir := t.TempDir()
	cloneDir := filepath.Join(reposDir, "org", "repo")
	if err := os.MkdirAll(cloneDir, 0o755); err != nil {
		t.Fatal(err)
	}
	records := []record{{URL: "https://github.com/org/repo/pull/7"}}
	calls := 0
	fakeRaw := func(ctx context.Context, dir string, args ...string) ([]byte, int, error) {
		return []byte("abc"), 0, nil
	}
	fake := func(ctx context.Context, dir string, args ...string) (string, int, error) {
		calls++
		if args[0] == "fetch" {
			return "", 128, nil
		}
		return "", 0, nil
	}
	if _, missing := resolveRepos(options{repos: reposDir}, records, fake, fakeRaw); missing == nil || !strings.Contains(missing.Error(), "fetch failed") {
		t.Errorf("missing = %v; want the fetch failure disclosed", missing)
	}
	fake = func(ctx context.Context, dir string, args ...string) (string, int, error) {
		calls++
		if args[0] == "rev-parse" {
			return "", 0, nil // rev-parse succeeded but printed nothing
		}
		return "abc", 0, nil
	}
	if _, missing := resolveRepos(options{repos: reposDir}, records, fake, fakeRaw); missing == nil || !strings.Contains(missing.Error(), "rev-parse failed") {
		t.Errorf("missing = %v; want the rev-parse failure disclosed", missing)
	}
}

// A URL the pass never resolved yields a non-ran search, not an error.
func TestRepoPassSearchWithoutRev(t *testing.T) {
	p := &repoPass{revs: map[string]string{}, dirs: map[string]string{}}
	ev, err := p.search("https://github.com/org/repo/pull/1", run.Finding{IssueText: "no tests"})
	if err != nil || ev.Ran {
		t.Errorf("search = %+v, %v; want non-ran, nil", ev, err)
	}
}

// The git seams keep absence and failure distinct, and surface failures.
func TestRepoPassSeamErrors(t *testing.T) {
	p := &repoPass{runGit: func(ctx context.Context, dir string, args ...string) (string, int, error) {
		return "", 0, context.Canceled
	}, runRaw: func(ctx context.Context, dir string, args ...string) ([]byte, int, error) {
		return nil, 0, context.Canceled
	}}
	if _, err := p.grepHead(context.Background(), "d", "rev")("pat"); err == nil {
		t.Error("a grep transport error must surface")
	}
	if _, _, err := p.showHead(context.Background(), "d", "rev")("p"); err == nil {
		t.Error("an ls-tree transport error must surface")
	}
	// grep is read raw and NUL-separated (-z): a path with spaces survives, and the rev:
	// prefix is stripped per NUL entry
	p.runRaw = func(ctx context.Context, dir string, args ...string) ([]byte, int, error) {
		return []byte("\x00rev:spec with space.rb\x00"), 0, nil
	}
	if paths, err := p.grepHead(context.Background(), "d", "rev")("pat"); err != nil || len(paths) != 1 || paths[0] != "spec with space.rb" {
		t.Errorf("NUL-separated grep output = %v, %v", paths, err)
	}
	p.runRaw = func(ctx context.Context, dir string, args ...string) ([]byte, int, error) {
		return nil, 128, nil
	}
	if _, err := p.grepHead(context.Background(), "d", "rev")("pat"); err == nil {
		t.Error("a grep failure exit must surface")
	}
	calls := 0
	p.runGit = func(ctx context.Context, dir string, args ...string) (string, int, error) {
		calls++
		if args[0] == "ls-tree" {
			return "", 128, nil
		}
		return "", 0, nil
	}
	if _, _, err := p.showHead(context.Background(), "d", "rev")("p"); err == nil {
		t.Error("an ls-tree failure exit must surface")
	}
	p.runRaw = func(ctx context.Context, dir string, args ...string) ([]byte, int, error) {
		calls++
		if args[0] == "cat-file" {
			return nil, 1, nil
		}
		return []byte(revZ), 0, nil // ls-tree lists the path; cat-file then fails
	}
	if _, _, err := p.showHead(context.Background(), "d", "rev")("p"); err == nil {
		t.Error("a cat-file failure exit must surface")
	}
	p.runRaw = func(ctx context.Context, dir string, args ...string) ([]byte, int, error) {
		if args[0] == "cat-file" {
			return nil, 0, context.Canceled
		}
		return []byte(revZ), 0, nil
	}
	if _, _, err := p.showHead(context.Background(), "d", "rev")("p"); err == nil {
		t.Error("a cat-file transport error must surface")
	}
	// absence: ls-tree lists nothing
	p.runGit = func(ctx context.Context, dir string, args ...string) (string, int, error) {
		return "", 0, nil
	}
	if body, ok, err := p.showHead(context.Background(), "d", "rev")("p"); err != nil || ok || body != nil {
		t.Errorf("absent path = %v, %v, %v; want nil, false, nil", body, ok, err)
	}
}

const revZ = "rev\x00path\x00"

// A per-claim search error during the report is counted and disclosed, never fatal —
// the claim stays measured on the diff dimension (driven through report's git seam).
func TestReportReposPassSearchErrorsAreCounted(t *testing.T) {
	reposDir := t.TempDir()
	gitFixture(t, reposDir, map[string]string{
		"spec/models/widget_spec.rb": "let!(:widget) { Fabricate(:widget) }\nexpect(widget.shine).to eq(true)\n",
	})
	records := []record{
		{URL: "https://github.com/org/repo/pull/7", IssueText: "the widget polish path has no test asserting the shine", NewVerdict: "hallucination", SourceLens: "lens-a"},
	}
	diff := "diff --git a/app/models/widget.rb b/app/models/widget.rb\n--- a/app/models/widget.rb\n+++ b/app/models/widget.rb\n@@ -3,2 +3,4 @@\n" +
		"+  def polish(g)\n+    g.try(:shine)\n"
	prev := reportGitRaw
	reportGitRaw = func(ctx context.Context, dir string, args ...string) ([]byte, int, error) {
		if args[0] == "grep" {
			return nil, 128, nil
		}
		return prev(ctx, dir, args...)
	}
	defer func() { reportGitRaw = prev }()
	var buf bytes.Buffer
	if err := report(&buf, records, map[string]string{"https://github.com/org/repo/pull/7": diff},
		options{repos: reposDir, dir: "testdata/mini", framework: "test-fw"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "repo search errors: 1") {
		t.Errorf("search errors must be counted and disclosed:\n%s", out)
	}
	if !strings.Contains(out, "diff-only") {
		t.Errorf("the failed-search claim stays measured on the diff:\n%s", out)
	}
}

// A record URL whose org/repo are not plain path components is refused: the -repos pass
// joins them into filesystem paths, and ".." (or anything shell-adjacent) must never
// reach dirExists.
func TestRepoPassRejectsUnsafeURLComponents(t *testing.T) {
	reposDir := t.TempDir()
	misleading := filepath.Join(reposDir, "..", "escape")
	if err := os.MkdirAll(misleading, 0o755); err != nil {
		t.Fatal(err)
	}
	pass, missing := resolveRepos(options{repos: reposDir}, []record{
		{URL: "https://github.com/../escape/pull/7"},
	}, realGit, realGitRaw)
	if pass.rev("https://github.com/../escape/pull/7") != "" {
		t.Error("a traversal URL must not resolve to a rev")
	}
	if missing == nil {
		t.Error("the traversal URL must be disclosed as missing, not silently used")
	}
}

// Blob reads must be byte-exact: a spec whose content has leading/trailing whitespace or
// interior blank lines must reach the search untouched (realGit's trim would shift lines).
func TestRepoPassShowHeadIsByteExact(t *testing.T) {
	reposDir := t.TempDir()
	gitFixture(t, reposDir, map[string]string{
		"spec/models/widget_spec.rb": "\n\nlet!(:widget) { Fabricate(:widget) }\n\nexpect(widget.shine).to eq(true)\n\n",
	})
	pass, missing := resolveRepos(options{repos: reposDir}, []record{
		{URL: "https://github.com/org/repo/pull/7"},
	}, realGit, realGitRaw)
	if missing != nil {
		t.Fatalf("missing = %v", missing)
	}
	ev, err := pass.search("https://github.com/org/repo/pull/7", run.Finding{
		File: "app/models/widget.rb", IssueText: "the widget polish path has no test asserting the shine"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	body := ev.Content["spec/models/widget_spec.rb"]
	if !strings.HasPrefix(body, "\n\nlet!(:widget)") {
		t.Errorf("blob content was trimmed — line numbers shift: %q", body)
	}
}

// Claims whose PR has no clone and claims whose search errored get their OWN matrix rows,
// so the repo dimension's gaps are explicit instead of folded into diff-only/no-evidence.
func TestReportReposPassSeparateGapRows(t *testing.T) {
	reposDir := t.TempDir()
	gitFixture(t, reposDir, map[string]string{
		"spec/models/widget_spec.rb": "let!(:widget) { Fabricate(:widget) }\nexpect(widget.shine).to eq(true)\n",
	})
	records := []record{
		{URL: "https://github.com/org/repo/pull/7", IssueText: "the widget polish path has no test asserting the shine", NewVerdict: "hallucination", SourceLens: "lens-a"},
		{URL: "https://github.com/org/absent/pull/1", IssueText: "zero tests exist for the helper module", NewVerdict: "hallucination", SourceLens: "lens-a"},
	}
	diff := "diff --git a/spec/models/widget_spec.rb b/spec/models/widget_spec.rb\n--- a/spec/models/widget_spec.rb\n+++ b/spec/models/widget_spec.rb\n@@ -5,2 +5,4 @@\n" +
		"+    expect(widget.shine).to eq(true)\n"
	prev := reportGit
	reportGit = func(ctx context.Context, dir string, args ...string) (string, int, error) {
		if args[0] == "grep" {
			return "", 128, nil
		}
		return prev(ctx, dir, args...)
	}
	defer func() { reportGit = prev }()
	var buf bytes.Buffer
	if err := report(&buf, records, map[string]string{
		"https://github.com/org/repo/pull/7": diff, "https://github.com/org/absent/pull/1": diff,
	}, options{repos: reposDir, dir: "testdata/mini", framework: "test-fw"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"no-clone", "repo-error"} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing the %q row:\n%s", want, out)
		}
	}
}

// The traversal guard is character-level: separators, leading dots, and empty components
// are refused; plain names pass.
func TestSafeComponent(t *testing.T) {
	yes := []string{"grafana", "my-repo", "repo.name", "repo_name", "R2"}
	no := []string{"", ".", "..", "../x", "a/b", "-lead", ".hidden", "a b", "a\tb"}
	for _, s := range yes {
		if !safeComponent(s) {
			t.Errorf("safeComponent(%q) = false; want true", s)
		}
	}
	for _, s := range no {
		if safeComponent(s) {
			t.Errorf("safeComponent(%q) = true; want false", s)
		}
	}
}

// showHead keeps absence and failure distinct at the raw seam too.
func TestRepoPassShowHeadTransportError(t *testing.T) {
	okGit := func(ctx context.Context, dir string, args ...string) (string, int, error) {
		return "p\x00", 0, nil
	}
	p := &repoPass{runGit: okGit, runRaw: func(ctx context.Context, dir string, args ...string) ([]byte, int, error) {
		return nil, 0, context.Canceled
	}}
	if _, _, err := p.showHead(context.Background(), "d", "rev")("p"); err == nil {
		t.Error("a cat-file transport error must surface")
	}
	p.runRaw = func(ctx context.Context, dir string, args ...string) ([]byte, int, error) {
		return nil, 128, nil
	}
	if _, _, err := p.showHead(context.Background(), "d", "rev")("p"); err == nil {
		t.Error("a cat-file failure exit must surface")
	}
	p.runRaw = func(ctx context.Context, dir string, args ...string) ([]byte, int, error) {
		return nil, 0, nil
	}
	p.runGit = func(ctx context.Context, dir string, args ...string) (string, int, error) {
		return "", 0, nil // ls-tree lists nothing: the path is absent
	}
	if body, ok, err := p.showHead(context.Background(), "d", "rev")("p"); err != nil || ok || body != nil {
		t.Errorf("absent path = %v, %v, %v; want nil, false, nil", body, ok, err)
	}
}

// The hallucination rollup counts claims the repo dimension MEASURED: no-clone and
// repo-error rows are infrastructure gaps, not "without covering evidence" — folding
// them in overstates the search's recall against hallucinations.
func TestReportReposPassRollupExcludesInfraRows(t *testing.T) {
	reposDir := t.TempDir()
	gitFixture(t, reposDir, map[string]string{
		"spec/models/widget_spec.rb": "let!(:widget) { Fabricate(:widget) }\nexpect(widget.shine).to eq(true)\n",
	})
	records := []record{
		{URL: "https://github.com/org/repo/pull/7", IssueText: "the widget polish path has no test asserting the shine", NewVerdict: "hallucination", SourceLens: "lens-a"},
		{URL: "https://github.com/org/absent/pull/1", IssueText: "zero tests exist for the helper module", NewVerdict: "hallucination", SourceLens: "lens-a"},
	}
	diff := "diff --git a/spec/models/widget_spec.rb b/spec/models/widget_spec.rb\n--- a/spec/models/widget_spec.rb\n+++ b/spec/models/widget_spec.rb\n@@ -5,2 +5,4 @@\n" +
		"+    expect(widget.shine).to eq(true)\n"
	var buf bytes.Buffer
	if err := report(&buf, records, map[string]string{
		"https://github.com/org/repo/pull/7": diff, "https://github.com/org/absent/pull/1": diff,
	}, options{repos: reposDir, dir: "testdata/mini", framework: "test-fw"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	// one hallucinated claim with evidence (both), one unmeasured on the repo dimension
	if !strings.Contains(out, "hallucinated gap-claims with covering evidence in the diff: 1/1") {
		t.Errorf("the rollup must exclude no-clone and repo-error rows:\n%s", out)
	}
}

// The operator-facing flat clone layout (<repos>/org-repo) resolves when the nested form
// is absent — a documented contract, so a regression to it must fail a test.
func TestRepoPassResolvesFlatLayout(t *testing.T) {
	reposDir := t.TempDir()
	origin := t.TempDir()
	gitRun := func(dir string, args ...string) string {
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	spec := "spec/models/widget_spec.rb"
	if err := os.MkdirAll(filepath.Join(origin, "spec/models"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(origin, spec), []byte("let!(:widget) { Fabricate(:widget) }\nexpect(widget.shine).to eq(true)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(origin, "init", "-q")
	gitRun(origin, "config", "user.email", "fixture@example.invalid")
	gitRun(origin, "config", "user.name", "fixture")
	gitRun(origin, "add", "-A")
	gitRun(origin, "commit", "-q", "-m", "spec")
	gitRun(origin, "update-ref", "refs/pull/7/head", "HEAD")
	cloneDir := filepath.Join(reposDir, "org-repo") // FLAT: no org/ directory level
	if out, err := exec.Command("git", "clone", "-q", origin, cloneDir).CombinedOutput(); err != nil {
		t.Fatalf("clone: %v\n%s", err, out)
	}
	pass, missing := resolveRepos(options{repos: reposDir}, []record{
		{URL: "https://github.com/org/repo/pull/7"},
	}, realGit, realGitRaw)
	if missing != nil {
		t.Fatalf("missing = %v; want the flat layout resolved", missing)
	}
	ev, err := pass.search("https://github.com/org/repo/pull/7", run.Finding{
		File: "app/models/widget.rb", IssueText: "the widget polish path has no test asserting the shine"})
	if err != nil || len(ev.Evidence) != 1 {
		t.Errorf("evidence = %+v, %v; want the flat-layout clone searched", ev.Evidence, err)
	}
}
