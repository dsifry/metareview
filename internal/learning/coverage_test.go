package learning

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/dsifry/metareview/internal/findings"
	"github.com/dsifry/metareview/internal/githubcontext"
	"github.com/dsifry/metareview/internal/knowledge"
	"github.com/dsifry/metareview/internal/learnsource"
	"github.com/dsifry/metareview/internal/reviewlog"
	"github.com/dsifry/metareview/internal/sessionhistory"
)

// --- candidates.go via ExtractCandidates ----------------------------------

// A calibration disposition can come from the finding TITLE when status/classification do not name
// it; a finding that names none is not a calibration candidate. And an open (unfixed) knowledge
// candidate carries medium confidence, where a fixed one is high.
func TestExtractCandidatesTitleDispositionAndMediumConfidence(t *testing.T) {
	result := ExtractCandidates(Input{
		Findings: []findings.Record{
			{ID: "t-fp", Status: "open", Title: "Turned out a false positive", Finding: "not a real issue"},
			{ID: "t-rb", Status: "open", Title: "Rebutted after discussion", Finding: "existing tests cover it"},
			{ID: "t-ar", Status: "open", Title: "Accepted risk for now", Finding: "temporary compatibility"},
			{ID: "t-none", Status: "open", Title: "ordinary finding", Finding: "nothing dispositive"},
			// open knowledge candidate -> medium confidence (not fixed, no FixedInRunID)
			{ID: "t-kc", Status: "open", KnowledgeCandidate: true,
				Title:   "Reviewers must check the service inventory before adding paths",
				Finding: "reviewers should verify inventory"},
		},
	})
	for _, d := range []string{"false-positive", "rebutted", "accepted-risk"} {
		assertCalibration(t, result.Calibration, d)
	}
	// The medium-confidence open knowledge candidate should be present.
	found := false
	for _, c := range result.Knowledge {
		if c.Kind == "knowledge-candidate" && c.Confidence == "medium" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a medium-confidence open knowledge candidate: %+v", result.Knowledge)
	}
}

// repeatedBlockerThemes keys on the fingerprint when the title is blank, and ignores a theme seen
// only once.
func TestRepeatedBlockerThemesFingerprintKeyAndSingletonSkip(t *testing.T) {
	blocker := func(id, fp string) findings.Record {
		return findings.Record{ID: id, Classification: "blocking", Severity: "high", Status: "open", Fingerprint: fp, Finding: "some blocker body"}
	}
	result := ExtractCandidates(Input{
		Findings: []findings.Record{
			blocker("b1", "fp-shared"), // title blank -> fingerprint key
			blocker("b2", "fp-shared"),
			blocker("b3", "fp-lonely"), // seen once -> skipped
			blocker("b4", ""),          // blank title AND fingerprint -> no key, skipped entirely
		},
	})
	themes := 0
	for _, c := range result.Knowledge {
		if c.Kind == "repeated-blocker-theme" {
			themes++
		}
	}
	if themes != 1 {
		t.Fatalf("exactly the fingerprint-shared theme should repeat, got %d: %+v", themes, result.Knowledge)
	}
}

// githubReviewBlockers accepts a non-CHANGES_REQUESTED review whose body uses blocker language, and
// returns nothing when the GitHub context is unavailable.
func TestGitHubReviewBlockersLanguageAndUnavailable(t *testing.T) {
	result := ExtractCandidates(Input{
		GitHub: githubcontext.Context{
			Available: true, PRNumber: "7",
			Reviews: []githubcontext.Entry{
				{State: "COMMENTED", URL: "u", Body: "You must not merge without a migration."},
				{State: "COMMENTED", URL: "empty", Body: ""}, // empty body -> skipped
			},
		},
	})
	assertKnowledge(t, result.Knowledge, "github-review-blocker", "GitHub review blocker")

	none := ExtractCandidates(Input{GitHub: githubcontext.Context{Available: false, Reviews: []githubcontext.Entry{{State: "CHANGES_REQUESTED", Body: "must fix"}}}})
	for _, c := range none.Knowledge {
		if c.Kind == "github-review-blocker" {
			t.Fatalf("an unavailable GitHub context must yield no review blockers: %+v", none.Knowledge)
		}
	}
}

func TestContainsBlockerLanguage(t *testing.T) {
	for _, s := range []string{"this is a Blocker", "you MUST do x", "do not merge"} {
		if !containsBlockerLanguage(s) {
			t.Fatalf("%q should read as blocker language", s)
		}
	}
	if containsBlockerLanguage("a calm neutral sentence") {
		t.Fatalf("neutral text is not blocker language")
	}
}

func TestCompactCandidatesDropsEmptyAndUnsourced(t *testing.T) {
	got := compactCandidates([]Candidate{
		{Text: "", SourceRefs: []SourceRef{{Type: "x"}}}, // empty text -> dropped
		{Text: "kept but unsourced"},                     // no source refs -> dropped
		{Text: "kept", SourceRefs: []SourceRef{{Type: "finding", ID: "f1"}}},
	})
	if len(got) != 1 || got[0].Text != "kept" {
		t.Fatalf("only the sourced non-empty candidate should survive: %+v", got)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if firstNonEmpty("", "  ", "\tx\t") != "x" {
		t.Fatalf("first non-blank, trimmed")
	}
	if firstNonEmpty("", "   ") != "" {
		t.Fatalf("all-blank -> empty")
	}
}

// --- prune.go via PruneCandidates -----------------------------------------

func TestPruneDiscardReasons(t *testing.T) {
	cand := func(text, target, conf string) Candidate {
		return Candidate{Text: text, ProposedTarget: target, Confidence: conf, SourceRefs: []SourceRef{{Type: "finding", ID: "f"}}}
	}
	res := PruneCandidates(PruneInput{
		Candidates: []Candidate{
			cand("Reviewers must check inventory before adding paths", "repository-knowledge", "medium"), // accepted
			cand("Update service-inventory registry entry", "service-inventory", "medium"),               // registry-update
			cand("just some words with no directive", "repository-knowledge", "medium"),                  // no reviewer-behavior change -> self-evident
		},
	})
	if len(res.Accepted) != 1 {
		t.Fatalf("only the behavior-changing candidate should be accepted: %+v", res.Accepted)
	}
	reasons := map[string]bool{}
	for _, d := range res.Discarded {
		reasons[d.Reason] = true
	}
	if !reasons[DiscardRegistryUpdateNotKnowledge] || !reasons[DiscardSelfEvident] {
		t.Fatalf("expected registry-update and self-evident discards: %+v", res.Discarded)
	}
}

// alreadyCovered matches a candidate against existing knowledge (and ignores empty text / empty
// facts).
func TestPruneAlreadyCovered(t *testing.T) {
	res := PruneCandidates(PruneInput{
		Candidates: []Candidate{
			{Text: "Reviewers must verify the service inventory", Confidence: "high", SourceRefs: []SourceRef{{Type: "finding", ID: "f"}}},
			{Text: "", Confidence: "high", SourceRefs: []SourceRef{{Type: "finding", ID: "g"}}}, // empty needle
		},
		Knowledge: knowledge.Context{Facts: []knowledge.Fact{
			{Text: ""}, // empty-fact haystack is skipped (reached before the matching fact)
			{Text: "Reviewers must verify the service inventory before adding paths"},
		}},
	})
	covered := false
	for _, d := range res.Discarded {
		if d.Reason == DiscardAlreadyCovered {
			covered = true
		}
	}
	if !covered {
		t.Fatalf("a candidate already in the knowledge base should be discarded as covered: %+v", res.Discarded)
	}
}

// --- review.go direct helpers ---------------------------------------------

func TestSourceRefText(t *testing.T) {
	if sourceRefText(nil) != "none" {
		t.Fatalf("no refs -> \"none\"")
	}
	got := sourceRefText([]SourceRef{{Type: "finding", ID: "f1"}, {Type: "github-review", URL: "u"}})
	if !strings.Contains(got, "finding f1") || !strings.Contains(got, "github-review u") {
		t.Fatalf("unexpected ref text: %q", got)
	}
}

func TestDiscardMarkdown(t *testing.T) {
	if !strings.Contains(discardMarkdown("run-1", nil), "No discarded learning candidates.") {
		t.Fatalf("empty discard list should say so")
	}
	md := discardMarkdown("run-1", []DiscardedCandidate{{Candidate: Candidate{Text: "some candidate"}, Reason: "self-evident"}})
	if !strings.Contains(md, "self-evident: some candidate") {
		t.Fatalf("discard markdown should render the reason and text: %q", md)
	}
}

func TestAcceptedMarkdownRendersAllSections(t *testing.T) {
	ref := []SourceRef{{Type: "finding", ID: "f1"}}
	md := acceptedMarkdown("run-1", ReviewOptions{PostMergePR: "9"},
		learnsource.Context{}, sessionhistory.Context{},
		[]Candidate{{Text: "accepted one", Provenance: "p", Confidence: "high", SourceRefs: ref}},
		[]Candidate{{Disposition: "false-positive", Text: "calib one"}},
		[]Candidate{{Text: "flag one", Provenance: "p", SourceRefs: ref}},
	)
	for _, want := range []string{"accepted one", "false-positive: calib one", "flag one", "finding f1"} {
		if !strings.Contains(md, want) {
			t.Fatalf("accepted markdown missing %q:\n%s", want, md)
		}
	}
}

func TestSnapshotBranches(t *testing.T) {
	dir := t.TempDir()
	if s := snapshot(filepath.Join(dir, "missing")); s.existed {
		t.Fatalf("a missing path must snapshot as not existing")
	}
	if s := snapshot(dir); !s.existed || !s.isDir {
		t.Fatalf("a directory must snapshot as an existing dir")
	}
	f := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(f, []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	if s := snapshot(f); !s.existed || s.isDir || string(s.content) != "body" {
		t.Fatalf("a file must snapshot its content")
	}
}

func TestRestoreSnapshots(t *testing.T) {
	dir := t.TempDir()
	toRemove := filepath.Join(dir, "remove.txt")
	if err := os.WriteFile(toRemove, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	dirToRecreate := filepath.Join(dir, "made", "dir")
	fileToRewrite := filepath.Join(dir, "nested", "file.txt")

	restoreSnapshots(map[string]fileSnapshot{
		toRemove:      {existed: false},             // was absent -> remove it
		dirToRecreate: {existed: true, isDir: true}, // restore a directory
		fileToRewrite: {existed: true, content: []byte("restored")},
	})

	if _, err := os.Stat(toRemove); !os.IsNotExist(err) {
		t.Fatalf("an absent-in-snapshot path must be removed")
	}
	if info, err := os.Stat(dirToRecreate); err != nil || !info.IsDir() {
		t.Fatalf("a dir snapshot must be recreated as a dir")
	}
	if body, err := os.ReadFile(fileToRewrite); err != nil || string(body) != "restored" {
		t.Fatalf("a file snapshot must be rewritten with its content")
	}
}

func TestRemoveEmptyLearningDirs(t *testing.T) {
	root := t.TempDir()
	learningDir := filepath.Join(root, "docs", "metareview", "learning")
	if err := os.MkdirAll(learningDir, 0o755); err != nil {
		t.Fatal(err)
	}
	removeEmptyLearningDirs(root) // must not panic and should remove the empty leaf
	if _, err := os.Stat(learningDir); !os.IsNotExist(err) {
		t.Fatalf("an empty learning dir should be removed")
	}
}

// RunPostMerge restores the tree and aborts when a write fails: pre-placing a FILE where the
// learning directory must be created makes MkdirAll fail, exercising the error/restore branch.
// A failure in a LATE write (calibration) must roll the run back: the accepted log written earlier
// is removed, and a file that pre-existed the run keeps its original content. This asserts the
// documented rollback postcondition, not merely that an error was returned.
func TestRunPostMergeRestoresOnWriteFailure(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	pr := "9"
	_, acceptedRel, _ := learningPaths(pr, now)
	root := newLearningRepo(t)
	// A ledger yielding a calibration candidate so WriteCalibration actually attempts (and fails).
	ledger := `{"schemaVersion":1,"id":"mrvf-fp","status":"false-positive","title":"was a false positive","finding":"not real"}` + "\n"
	if err := os.WriteFile(filepath.Join(root, ".metareview", "findings.jsonl"), []byte(ledger), 0o644); err != nil {
		t.Fatal(err)
	}
	// A pre-existing runs ledger with sentinel content: the run must NOT leave it altered on failure.
	runsPath := filepath.Join(root, ".metareview", "learning-runs.jsonl")
	if err := os.WriteFile(runsPath, []byte("SENTINEL\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Block the (late) calibration write so accepted/discard/knowledge succeed first, then it fails.
	mustDir(t, filepath.Join(root, ".metareview", "calibration.jsonl"))

	if _, err := RunPostMerge(root, ReviewOptions{PostMergePR: pr, Base: "HEAD", HomeDir: t.TempDir(), Now: now}); err == nil {
		t.Fatalf("a write failure must surface an error")
	}
	// The accepted log was written before calibration failed; rollback must remove it.
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(acceptedRel))); !os.IsNotExist(err) {
		t.Fatalf("the accepted log written before the failure must be rolled back, stat err=%v", err)
	}
	// The pre-existing runs ledger must be restored to its original content.
	if body, err := os.ReadFile(runsPath); err != nil || string(body) != "SENTINEL\n" {
		t.Fatalf("a pre-existing file must survive rollback unchanged, got %q err=%v", body, err)
	}
}

func TestSourceRefsCollectsLogAndSignalPaths(t *testing.T) {
	// A review log with a path and a session signal with a path both appear; empty paths are skipped.
	src := learnsource.Context{ReviewLogs: []reviewlog.Summary{
		{Path: "docs/metareview/reviews/r.md"},
		{Path: ""},
	}}
	got := sourceRefs(src, sessionhistory.Context{Signals: []sessionhistory.Signal{{Path: "s.jsonl"}, {Path: ""}}})
	if len(got) != 2 || got[0] != "docs/metareview/reviews/r.md" || got[1] != "s.jsonl" {
		t.Fatalf("sourceRefs should collect non-empty log and signal paths, got %v", got)
	}
}

func TestGitSectionsNonGitText(t *testing.T) {
	if secs := gitSections("plain non-git text"); len(secs) != 1 || secs[0] != "plain non-git text" {
		t.Fatalf("non-git non-empty diff should be one section, got %v", secs)
	}
	if secs := gitSections("   "); secs != nil {
		t.Fatalf("whitespace-only diff should be nil, got %v", secs)
	}
}

func TestSnapshotReadError(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("unreadable-file case needs a non-root POSIX host")
	}
	f := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(f, []byte("x"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(f, 0o644) })
	if s := snapshot(f); s.existed {
		t.Fatalf("an unreadable file must snapshot as not existing (read failed)")
	}
}

// WriteKnowledge/WriteCalibration surface a write failure: a directory at the target path makes the
// underlying append fail.
func TestWriteKnowledgeAndCalibrationSurfaceWriteErrors(t *testing.T) {
	cand := []Candidate{{Text: "a lesson", Scope: "repository-review", SourceRefs: []SourceRef{{Type: "finding", ID: "f"}}}}

	kroot := t.TempDir()
	kpath := filepath.Join(kroot, filepath.FromSlash(knowledgeRelPath(kroot)))
	if err := os.MkdirAll(kpath, 0o755); err != nil { // a dir where the file must go
		t.Fatal(err)
	}
	if _, err := WriteKnowledge(kroot, "run-1", cand); err == nil {
		t.Fatalf("WriteKnowledge must surface a write failure")
	}

	croot := t.TempDir()
	cpath := filepath.Join(croot, ".metareview", "calibration.jsonl")
	if err := os.MkdirAll(cpath, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteCalibration(croot, "run-1", cand); err == nil {
		t.Fatalf("WriteCalibration must surface a write failure")
	}
}

// RunPostMerge defaults the clock when Now is zero, and aborts on a bad base or a corrupt findings
// ledger.
func TestRunPostMergeEarlyBranches(t *testing.T) {
	root := newLearningRepo(t)
	res, err := RunPostMerge(root, ReviewOptions{PostMergePR: "1", Base: "HEAD", HomeDir: t.TempDir()})
	if err != nil {
		t.Fatalf("a zero Now should default to the wall clock: %v", err)
	}
	// The substitution actually happened: the run-id is not the one a zero-value clock would produce.
	zeroRunID, _, _ := learningPaths("1", time.Time{})
	if res.RunID == zeroRunID || res.RunID == "" {
		t.Fatalf("a zero Now must be replaced by the wall clock, got run id %q (zero-clock id %q)", res.RunID, zeroRunID)
	}

	if _, err := RunPostMerge(newLearningRepo(t), ReviewOptions{PostMergePR: "1", Base: "a..b", HomeDir: t.TempDir()}); err == nil {
		t.Fatalf("an invalid base must surface a learnsource error")
	}

	// The findings-ledger read guard: unreachable via a broken file (learnsource reads the same
	// ledger earlier and fails first), so the seam forces it.
	func() {
		real := readFindings
		defer func() { readFindings = real }()
		readFindings = func(string) ([]findings.Record, error) { return nil, errors.New("ledger boom") }
		if _, err := RunPostMerge(newLearningRepo(t), ReviewOptions{PostMergePR: "1", Base: "HEAD", HomeDir: t.TempDir()}); err == nil {
			t.Fatalf("a findings-read failure must surface an error")
		}
	}()

	// A session root whose walk hits an unreadable subdirectory makes session collection fail.
	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		sroot := newLearningRepo(t)
		sessionDir := t.TempDir()
		locked := filepath.Join(sessionDir, "locked")
		if err := os.MkdirAll(locked, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
		if _, err := RunPostMerge(sroot, ReviewOptions{PostMergePR: "1", Base: "HEAD", HomeDir: t.TempDir(), SessionRoot: sessionDir}); err == nil {
			t.Fatalf("an unreadable session root must surface an error")
		}
	}
}

// Each write inside RunPostMerge's transactional block can fail; placing a directory at the
// computed target for one write at a time exercises each guard in isolation. Fixed Now+PR makes the
// run-id (and thus the accepted/discard paths) deterministic.
func TestRunPostMergeInnerWriteErrors(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	pr := "9"
	_, acceptedRel, discardRel := learningPaths(pr, now)
	// A findings ledger that yields an accepted knowledge candidate and a calibration candidate, so
	// WriteKnowledge/WriteCalibration actually attempt a write.
	ledger := `{"schemaVersion":1,"id":"mrvf-kc","status":"fixed","fixedInRunId":"mrv-x","classification":"blocking","severity":"high","knowledgeCandidate":true,"title":"Reviewers should require a migration plan before approving schema changes","finding":"a schema change merged without a migration plan"}` + "\n" +
		`{"schemaVersion":1,"id":"mrvf-fp","status":"false-positive","title":"was a false positive","finding":"not real"}` + "\n"

	run := func(setup func(root string)) error {
		root := newLearningRepo(t)
		if err := os.WriteFile(filepath.Join(root, ".metareview", "findings.jsonl"), []byte(ledger), 0o644); err != nil {
			t.Fatal(err)
		}
		setup(root)
		_, err := RunPostMerge(root, ReviewOptions{PostMergePR: pr, Base: "HEAD", HomeDir: t.TempDir(), Now: now})
		return err
	}

	cases := []struct {
		name  string
		setup func(root string)
	}{
		{"git policy", func(root string) { mustDir(t, filepath.Join(root, ".gitignore")) }},
		{"learning dir", func(root string) { // a FILE where the learning dir must be created -> MkdirAll fails
			mustDir(t, filepath.Join(root, "docs", "metareview"))
			if err := os.WriteFile(filepath.Join(root, "docs", "metareview", "learning"), []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"accepted log", func(root string) { mustDir(t, filepath.Join(root, filepath.FromSlash(acceptedRel))) }},
		{"discard log", func(root string) { mustDir(t, filepath.Join(root, filepath.FromSlash(discardRel))) }},
		{"knowledge", func(root string) { mustDir(t, filepath.Join(root, filepath.FromSlash(knowledgeRelPath(root)))) }},
		{"calibration", func(root string) { mustDir(t, filepath.Join(root, ".metareview", "calibration.jsonl")) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := run(tc.setup); err == nil {
				t.Fatalf("a %s write failure must surface an error", tc.name)
			}
		})
	}
}

func mustDir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

// --- helpers ---------------------------------------------------------------

func newLearningRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".metareview"), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@x", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@x")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(root, "f.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-q", "-m", "base")
	return root
}
