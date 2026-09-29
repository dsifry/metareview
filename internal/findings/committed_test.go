package findings

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/scope"
)

const committedRunID = "mrv-20260905-223221349809000-task-done-help-9a8265a5"
const committedID = "mrvf-20260905-223221349809000-task-done-help-9a8265a5-001"

// committedLog is a review log shaped like the real ones: a header, a blocking finding and an advisory one.
func committedLog(runID string) string {
	id := "mrvf-" + strings.TrimPrefix(runID, "mrv-")
	return "# metareview: task-done review\r\n\r\nRun ID: `" + runID + "`\r\n\r\nTarget: `help`\r\n\r\n## Verdict\r\n\r\nNEEDS_REVISION\r\n\r\n" +
		"## Blocking Findings\r\n\r\n### " + id + "-001: No adjudicated lens review recorded\r\n\r\n" +
		"- Reviewer: adversarial-review-reviewer\r\n- Severity: high\r\n- Classification: blocking\r\n" +
		"- Finding: none is recorded for HEAD abc.\r\n- Expected: a lens review.\r\n- Found: none.\r\n" +
		"- Recommendation: run the lenses.\r\n- Severity: low\r\nnot a field\r\n\r\n" +
		"## Advisory Findings\r\n\r\n### " + id + "-002: A note\r\n\r\n- Severity: low\r\n- Classification: advisory\r\n"
}

func writeCommittedLog(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(root, "docs", "metareview", "reviews")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func stubImportGit(t *testing.T, head string, headErr error) {
	t.Helper()
	origHead, origScope := headOf, loadScope
	headOf = func(string) (string, error) { return head, headErr }
	loadScope = func(string) scope.Scope { return scope.Scope{Current: "feature"} }
	t.Cleanup(func() { headOf, loadScope = origHead, origScope })
}

var goodRequest = OverrideRequest{By: "claude-agent", Reason: "stale review of a head that no longer exists", Now: "2026-09-29T07:00:00Z"}

// #188: a blocker known only from a committed review log is imported as this branch's open row, then overridden.
func TestOverrideImportsABlockerKnownOnlyFromACommittedLog(t *testing.T) {
	root := t.TempDir()
	stubImportGit(t, "head-sha", nil)
	writeCommittedLog(t, root, "a-notes.txt", "### "+committedID+": ignored, not a review log\n")
	if err := os.MkdirAll(filepath.Join(root, "docs", "metareview", "reviews", "sub.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A later log that quotes the ID (a previous-run note, another run's section) is not where it was raised.
	writeCommittedLog(t, root, "b-later.md", "# metareview: pr-ready review\n\nRun ID: `mrv-later`\n\n## Blocking Findings\n\n### "+committedID+": quoted\n\n- Classification: blocking\n")
	writeCommittedLog(t, root, committedRunID+".md", committedLog(committedRunID))
	if err := RequestOverride(root, committedID, goodRequest); err != nil {
		t.Fatal(err)
	}
	got := loadOne(t, root)
	want := Record{
		SchemaVersion: 1, ID: committedID, RunID: committedRunID, Scope: "task-done",
		Reviewer: "adversarial-review-reviewer", Severity: "high", Classification: "blocking",
		Status: StatusOverridePending, Title: "No adjudicated lens review recorded",
		Finding: "none is recorded for HEAD abc.", Expected: "a lens review.", Found: "none.", Recommendation: "run the lenses.",
		Fingerprint: ImportedFingerprintPrefix + committedID, GitHead: "head-sha", Branch: "feature", RepoRoot: root,
		CreatedAt: goodRequest.Now,
	}
	if got.Scope != want.Scope || got.RunID != want.RunID || got.Reviewer != want.Reviewer || got.Severity != want.Severity ||
		got.Classification != want.Classification || got.Status != want.Status || got.Title != want.Title ||
		got.Finding != want.Finding || got.Expected != want.Expected || got.Found != want.Found ||
		got.Recommendation != want.Recommendation || got.Fingerprint != want.Fingerprint || got.GitHead != want.GitHead ||
		got.Branch != want.Branch || got.RepoRoot != want.RepoRoot || got.CreatedAt != want.CreatedAt || got.SchemaVersion != 1 {
		t.Fatalf("imported row\n got %+v\nwant %+v", got, want)
	}
	if len(got.Evidence) != 1 || got.Evidence[0].Path != "docs/metareview/reviews/"+committedRunID+".md" {
		t.Fatalf("the row must record the log it came from: %+v", got.Evidence)
	}
	if target, _ := got.Target.(map[string]any); target["type"] != "review-log" || target["id"] != "help" {
		t.Fatalf("target = %#v", got.Target)
	}
	if !Blocks(got.Status) {
		t.Fatal("a requested override keeps blocking")
	}
	if err := GrantOverride(root, committedID, OverrideGrant{By: "maintainer", Reason: "accepted: the reviewed head is gone", Now: "t2"}); err != nil {
		t.Fatal(err)
	}
	if got := loadOne(t, root); got.Status != StatusOverridden {
		t.Fatalf("grant: status %s", got.Status)
	}
	if index, _ := os.ReadFile(filepath.Join(root, "docs", "metareview", "FINDINGS.md")); !strings.Contains(string(index), committedID+" [granted]") {
		t.Fatalf("the grant must be rendered in the index:\n%s", index)
	}
}

// A human may grant directly on a committed-only blocker, as on any open one.
func TestGrantImportsACommittedOnlyBlocker(t *testing.T) {
	root := t.TempDir()
	stubImportGit(t, "h", nil)
	writeCommittedLog(t, root, committedRunID+".md", committedLog(committedRunID))
	if err := GrantOverride(root, committedID, OverrideGrant{By: "maintainer", Reason: "accepted: the reviewed head is gone", Now: "t"}); err != nil {
		t.Fatal(err)
	}
	if got := loadOne(t, root); got.Status != StatusOverridden || got.OverrideRequestedBy != "" {
		t.Fatalf("got %+v", got)
	}
}

// Only a blocking finding under its own run's log is imported; everything else stays "not found".
func TestCommittedImportRefusesWhatIsNotItsOwnRunsBlocker(t *testing.T) {
	stubImportGit(t, "h", nil)
	for name, tc := range map[string]struct{ id, runID string }{
		"an advisory":                       {"mrvf-20260905-223221349809000-task-done-help-9a8265a5-002", committedRunID},
		"a run whose ID extends the log's":  {"mrvf-20260905-223221349809000-task-done-help-9a8265a5-x-001", committedRunID},
		"an index that is not a number":     {"mrvf-20260905-223221349809000-task-done-help-9a8265a5-abc", committedRunID},
		"a bare prefix":                     {"mrvf-20260905-223221349809000-task-done-help-9a8265a5-", committedRunID},
		"a log that never names its run ID": {committedID, ""},
		"an ID nobody raised":               {"mrvf-nope-001", committedRunID},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			body := committedLog(committedRunID)
			if tc.runID == "" {
				body = strings.Replace(body, "Run ID: `"+committedRunID+"`", "", 1)
			}
			writeCommittedLog(t, root, "log.md", body)
			err := RequestOverride(root, tc.id, goodRequest)
			if err == nil || !strings.Contains(err.Error(), "not found") {
				t.Fatalf("err = %v, want not found", err)
			}
			if _, statErr := os.Stat(findingsPath(root)); !os.IsNotExist(statErr) {
				t.Fatal("a refused import must write nothing")
			}
		})
	}
	// Its own log, but not under "## Blocking Findings" (a resolved or quoted entry): not an open blocker to import.
	root := t.TempDir()
	writeCommittedLog(t, root, "log.md", strings.Replace(committedLog(committedRunID), "## Blocking Findings", "## Resolved Findings", 1))
	if err := RequestOverride(root, committedID, goodRequest); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v, want not found", err)
	}
	root = t.TempDir() // no committed reviews at all
	if err := RequestOverride(root, committedID, goodRequest); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v, want not found", err)
	}
}

// A failure reading the logs or HEAD surfaces instead of reading as "not found"; a failed transition writes nothing.
func TestCommittedImportFailuresSurface(t *testing.T) {
	boom := errors.New("boom")
	t.Run("reviews dir", func(t *testing.T) {
		root := t.TempDir()
		orig := readReviewsDir
		readReviewsDir = func(string) ([]os.DirEntry, error) { return nil, boom }
		t.Cleanup(func() { readReviewsDir = orig })
		if err := RequestOverride(root, committedID, goodRequest); !errors.Is(err, boom) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("a log", func(t *testing.T) {
		root := t.TempDir()
		writeCommittedLog(t, root, "log.md", committedLog(committedRunID))
		orig := readReviewLog
		readReviewLog = func(string) ([]byte, error) { return nil, boom }
		t.Cleanup(func() { readReviewLog = orig })
		if err := RequestOverride(root, committedID, goodRequest); !errors.Is(err, boom) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("HEAD", func(t *testing.T) {
		root := t.TempDir()
		stubImportGit(t, "", boom)
		writeCommittedLog(t, root, "log.md", committedLog(committedRunID))
		if err := RequestOverride(root, committedID, goodRequest); !errors.Is(err, boom) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("the transition", func(t *testing.T) {
		root := t.TempDir()
		stubImportGit(t, "h", nil)
		writeCommittedLog(t, root, "log.md", committedLog(committedRunID))
		orig := saveRecords
		saveRecords = func(string, []Record) error { return boom }
		t.Cleanup(func() { saveRecords = orig })
		if err := RequestOverride(root, committedID, goodRequest); !errors.Is(err, boom) {
			t.Fatalf("err = %v", err)
		}
	})
}
