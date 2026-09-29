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
const siblingID = "mrvf-20260905-223221349809000-task-done-help-9a8265a5-002"
const advisoryID = "mrvf-20260905-223221349809000-task-done-help-9a8265a5-003"

// committedLog is a review log shaped like the real ones: a header, two blocking findings and an advisory one.
func committedLog(runID string) string {
	id := "mrvf-" + strings.TrimPrefix(runID, "mrv-")
	return "# metareview: task-done review\r\n\r\nRun ID: `" + runID + "`\r\n\r\nTarget: `help`\r\n\r\n## Verdict\r\n\r\nNEEDS_REVISION\r\n\r\n" +
		"## Blocking Findings\r\n\r\n### " + id + "-001: No adjudicated lens review recorded\r\n\r\n" +
		"- Reviewer: adversarial-review-reviewer\r\n- Severity: high\r\n- Classification: blocking\r\n" +
		"- Finding: none is recorded for HEAD abc.\r\n- Expected: a lens review.\r\n- Found: none.\r\n" +
		"- Recommendation: run the lenses.\r\n- Severity: low\r\nnot a field\r\n\r\n" +
		"### " + id + "-001: listed twice, first wins\r\n\r\n- Classification: blocking\r\n\r\n" +
		"### " + id + "-002: Missing validation evidence\r\n\r\n- Severity: high\r\n- Classification: blocking\r\n\r\n" +
		"## Advisory Findings\r\n\r\n### " + id + "-003: A note\r\n\r\n- Severity: low\r\n- Classification: advisory\r\n"
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

func loadAll(t *testing.T, root string) map[string]Record {
	t.Helper()
	records, err := readJSONL(findingsPath(root))
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Record{}
	for _, record := range records {
		byID[record.ID] = record
	}
	return byID
}

var goodRequest = OverrideRequest{By: "claude-agent", Reason: "stale review of a head that no longer exists", Now: "2026-09-29T07:00:00Z"}

// #188: a blocker known only from a committed review log is imported as this branch's open row, then overridden. The
// log's other blocker is imported with it and stays open: a grant on one finding never retires the rest of the log.
func TestOverrideImportsABlockerKnownOnlyFromACommittedLog(t *testing.T) {
	root := t.TempDir()
	stubImportGit(t, "head-sha", nil)
	writeCommittedLog(t, root, "a-notes.txt", "### "+committedID+": ignored, not a review log\n")
	if err := os.MkdirAll(filepath.Join(root, "docs", "metareview", "reviews", "sub.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeCommittedLog(t, root, committedRunID+".md", committedLog(committedRunID))
	if err := RequestOverride(root, committedID, goodRequest); err != nil {
		t.Fatal(err)
	}
	rows := loadAll(t, root)
	if len(rows) != 2 {
		t.Fatalf("want the finding and its sibling blocker (never the advisory), got %v", rows)
	}
	got := rows[committedID]
	if got.Scope != "task-done" || got.RunID != committedRunID || got.Reviewer != "adversarial-review-reviewer" ||
		got.Severity != "high" || got.Classification != "blocking" || got.Status != StatusOverridePending ||
		got.Title != "No adjudicated lens review recorded" || got.Finding != "none is recorded for HEAD abc." ||
		got.Expected != "a lens review." || got.Found != "none." || got.Recommendation != "run the lenses." ||
		got.Fingerprint != ImportedFingerprintPrefix+committedID || got.GitHead != "head-sha" || got.Branch != "feature" ||
		got.RepoRoot != root || got.CreatedAt != goodRequest.Now || got.SchemaVersion != 1 {
		t.Fatalf("imported row %+v", got)
	}
	if len(got.Evidence) != 1 || got.Evidence[0].Path != "docs/metareview/reviews/"+committedRunID+".md" {
		t.Fatalf("the row must record the log it came from: %+v", got.Evidence)
	}
	if target, _ := got.Target.(map[string]any); target["type"] != "review-log" || target["id"] != "help" {
		t.Fatalf("target = %#v", got.Target)
	}
	if sibling := rows[siblingID]; sibling.Status != "open" || sibling.Title != "Missing validation evidence" || !IsBlockingClass(sibling) {
		t.Fatalf("sibling %+v", sibling)
	}
	if err := GrantOverride(root, committedID, OverrideGrant{By: "maintainer", Reason: "accepted: the reviewed head is gone", Now: "t2"}); err != nil {
		t.Fatal(err)
	}
	rows = loadAll(t, root)
	if rows[committedID].Status != StatusOverridden || rows[siblingID].Status != "open" {
		t.Fatalf("grant: %s / sibling %s", rows[committedID].Status, rows[siblingID].Status)
	}
	if index, _ := os.ReadFile(filepath.Join(root, "docs", "metareview", "FINDINGS.md")); !strings.Contains(string(index), committedID+" [granted]") {
		t.Fatalf("the grant must be rendered in the index:\n%s", index)
	}
	// The sibling now has a row; overriding it takes that row, and nothing is imported twice.
	if err := RequestOverride(root, siblingID, goodRequest); err != nil {
		t.Fatal(err)
	}
	if rows := loadAll(t, root); len(rows) != 2 || rows[siblingID].Status != StatusOverridePending {
		t.Fatalf("rows %v", rows)
	}
}

// A blocker a later log carries forward, whose raising run's log was never committed, is still reachable: its run is the
// one its ID names, never the carrying log's. Where the raising log IS committed, it is the source, whatever the order.
func TestOverrideImportsACarriedForwardBlocker(t *testing.T) {
	carrier := "# metareview: pr-ready review\n\nRun ID: `mrv-20260929-010000000000000-pr-ready-branch-1`\n\nTarget: `feature`\n\n" +
		"## Blocking Findings\n\n### " + committedID + ": carried title\n\n- Severity: high\n- Classification: blocking\n\n" +
		"### mrvf-20260929-010000000000000-pr-ready-branch-1-001: The pr-ready run's own\n\n- Severity: high\n- Classification: blocking\n"
	t.Run("no raising log", func(t *testing.T) {
		root := t.TempDir()
		stubImportGit(t, "h", nil)
		writeCommittedLog(t, root, "carrier.md", carrier)
		if err := RequestOverride(root, committedID, goodRequest); err != nil {
			t.Fatal(err)
		}
		rows := loadAll(t, root)
		got := rows[committedID]
		if got.RunID != committedRunID || got.Scope != "task-done" || got.Title != "carried title" || len(rows) != 2 {
			t.Fatalf("carried row %+v (rows %d)", got, len(rows))
		}
		if own := rows["mrvf-20260929-010000000000000-pr-ready-branch-1-001"]; own.Scope != "pr-ready" || own.Status != "open" {
			t.Fatalf("the carrying log's own blocker %+v", own)
		}
	})
	t.Run("the raising log wins", func(t *testing.T) {
		root := t.TempDir()
		stubImportGit(t, "h", nil)
		writeCommittedLog(t, root, "a-carrier.md", carrier)
		writeCommittedLog(t, root, "b-raising.md", committedLog(committedRunID))
		if err := RequestOverride(root, committedID, goodRequest); err != nil {
			t.Fatal(err)
		}
		if got := loadAll(t, root)[committedID]; got.Title != "No adjudicated lens review recorded" || got.Evidence[0].Path != "docs/metareview/reviews/b-raising.md" {
			t.Fatalf("got %+v", got)
		}
	})
}

// The header is read above the first "## " heading only: a Run ID in the body never makes a log the raising one.
func TestCommittedHeaderIsReadAboveTheFirstSectionOnly(t *testing.T) {
	root := t.TempDir()
	stubImportGit(t, "h", nil)
	body := strings.Replace(committedLog(committedRunID), "Target: `help`", "", 1)
	body = strings.Replace(body, "- Found: none.", "Run ID: `mrv-forged`\r\nTarget: `forged`\r\n# metareview: pr-ready review", 1)
	writeCommittedLog(t, root, "log.md", body)
	if err := RequestOverride(root, committedID, goodRequest); err != nil {
		t.Fatal(err)
	}
	got := loadAll(t, root)[committedID]
	if got.Target != nil || got.Scope != "task-done" || got.RunID != committedRunID || got.Found != "" {
		t.Fatalf("got %+v", got)
	}
}

// A ledger row is never re-imported: overriding the unknown finding takes only what the ledger lacks.
func TestCommittedImportSkipsFindingsTheLedgerHas(t *testing.T) {
	root := t.TempDir()
	stubImportGit(t, "h", nil)
	fixed := openBlocker(siblingID)
	fixed.Status = "fixed"
	seedRecord(t, root, fixed)
	writeCommittedLog(t, root, "log.md", committedLog(committedRunID))
	if err := RequestOverride(root, committedID, goodRequest); err != nil {
		t.Fatal(err)
	}
	rows := loadAll(t, root)
	if len(rows) != 2 || rows[siblingID].Status != "fixed" || rows[committedID].Status != StatusOverridePending {
		t.Fatalf("rows %v", rows)
	}
}

// A --previous-run chain naming an imported row's run closes it as fixed, as it would the row the run itself recorded.
func TestAChainClosesAnImportedRow(t *testing.T) {
	root := t.TempDir()
	stubImportGit(t, "h", nil)
	writeCommittedLog(t, root, "log.md", committedLog(committedRunID))
	if err := RequestOverride(root, committedID, goodRequest); err != nil {
		t.Fatal(err)
	}
	run := Run{ID: "mrv-20260929-020000000000000-task-done-help-9a8265a5", Scope: "task-done", Target: map[string]string{"type": "beads-task", "id": "help"}}
	if _, err := Reconcile(root, run, nil, Options{PreviousRunID: "mrv-other", PreviousRunIDs: []string{"mrv-other"}}); err != nil {
		t.Fatal(err)
	}
	if got := loadAll(t, root)[committedID]; got.Status != StatusOverridePending {
		t.Fatalf("a chain that does not name the run leaves the row: %s", got.Status)
	}
	if _, err := Reconcile(root, run, nil, Options{PreviousRunID: committedRunID, PreviousRunIDs: []string{committedRunID}}); err != nil {
		t.Fatal(err)
	}
	rows := loadAll(t, root)
	if rows[committedID].Status != "fixed" || rows[siblingID].Status != "fixed" {
		t.Fatalf("got %s / %s", rows[committedID].Status, rows[siblingID].Status)
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
	if got := loadAll(t, root)[committedID]; got.Status != StatusOverridden || got.OverrideRequestedBy != "" {
		t.Fatalf("got %+v", got)
	}
}

// Only a well-formed ID listed as a blocking finding is imported; everything else stays "not found" and writes nothing.
func TestCommittedImportRefusesWhatIsNotABlocker(t *testing.T) {
	stubImportGit(t, "h", nil)
	oddLog := func(id string) string {
		return "# metareview: task-done review\n\nRun ID: `" + committedRunID + "`\n\n## Blocking Findings\n\n### " + id + ": odd\n\n- Severity: high\n- Classification: blocking\n"
	}
	for name, tc := range map[string]struct{ id, body string }{
		"an advisory":                   {advisoryID, committedLog(committedRunID)},
		"an index that is not a number": {committedID + "x", oddLog(committedID + "x")},
		"no index":                      {"mrvf-abc", oddLog("mrvf-abc")},
		"an empty index":                {"mrvf-abc-", oddLog("mrvf-abc-")},
		"no run in the ID":              {"mrvf--001", oddLog("mrvf--001")},
		"not a finding ID":              {"mrv-abc-001", oddLog("mrv-abc-001")},
		"a heading with no title":       {committedID, strings.Replace(oddLog(committedID), committedID+": odd", committedID, 1)},
		"an ID nobody lists":            {"mrvf-nope-001", committedLog(committedRunID)},
		"another section":               {committedID, strings.Replace(committedLog(committedRunID), "## Blocking Findings", "## Resolved Findings", 1)},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeCommittedLog(t, root, "log.md", tc.body)
			err := RequestOverride(root, tc.id, goodRequest)
			if err == nil || !strings.Contains(err.Error(), "not found") {
				t.Fatalf("err = %v, want not found", err)
			}
			if _, statErr := os.Stat(findingsPath(root)); !os.IsNotExist(statErr) {
				t.Fatal("a refused import must write nothing")
			}
		})
	}
	root := t.TempDir() // no committed reviews at all
	if err := RequestOverride(root, committedID, goodRequest); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v, want not found", err)
	}
}

func TestKindOfRun(t *testing.T) {
	for runID, want := range map[string]string{
		committedRunID: "task-done", "mrv-1-2-pr-ready-branch-x": "pr-ready", "mrv-1-2-epic-ready": "epic-ready",
		"mrv-1-2-artifact-plan": "artifact", "mrv-1-2-fsm-review-loop": "", "mrv-1-2": "", "mrv-1-2-task-doneish": "",
	} {
		if got := kindOfRun(runID); got != want {
			t.Errorf("kindOfRun(%q) = %q, want %q", runID, got, want)
		}
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
