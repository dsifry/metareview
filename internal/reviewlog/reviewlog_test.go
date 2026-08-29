package reviewlog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/jsonl"
)

func TestDiscoverReviewLogsDeterministically(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "b.md"), reviewMarkdown("mrv-b", "task-2", "NEEDS_REVISION", "mrvf-b-001"))
	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "a.md"), reviewMarkdown("mrv-a", "task-1", "PASS", ""))
	mustWrite(t, filepath.Join(root, ".metareview", "findings.jsonl"), `{"id":"mrvf-b-001","runId":"mrv-b","status":"open","classification":"blocking","severity":"high","title":"Blocked","target":{"id":"task-2","type":"beads-task"}}`+"\n")

	logs, err := Discover(root)
	if err != nil {
		t.Fatalf("discover logs: %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("expected 2 logs, got %d", len(logs))
	}
	if logs[0].Path != "docs/metareview/reviews/a.md" || logs[1].Path != "docs/metareview/reviews/b.md" {
		t.Fatalf("logs not sorted by path: %+v", logs)
	}
	if logs[0].RunID != "mrv-a" || logs[0].Target != "task-1" || logs[0].Verdict != "PASS" {
		t.Fatalf("unexpected first log summary: %+v", logs[0])
	}
	if !logs[1].HasUnresolvedBlockers || len(logs[1].FindingIDs) != 1 || logs[1].FindingIDs[0] != "mrvf-b-001" {
		t.Fatalf("expected unresolved blocker summary: %+v", logs[1])
	}
}

func TestForTargetIncludesFindingsStateEvenWhenMarkdownOmitsIDs(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "task.md"), reviewMarkdown("mrv-task", "task-1", "NEEDS_REVISION", ""))
	mustWrite(t, filepath.Join(root, ".metareview", "findings.jsonl"), `{"id":"mrvf-task-001","runId":"mrv-task","status":"open","classification":"blocking","severity":"critical","title":"Unsafe","target":{"id":"task-1","type":"beads-task"}}`+"\n")

	logs, err := ForTarget(root, "task-1")
	if err != nil {
		t.Fatalf("target logs: %v", err)
	}
	if len(logs) != 1 || !logs[0].HasUnresolvedBlockers || logs[0].FindingIDs[0] != "mrvf-task-001" {
		t.Fatalf("expected blocker from findings state: %+v", logs)
	}
}

func TestArtifactNotReviewedIsUnresolved(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "artifact.md"), artifactReviewMarkdown("mrv-artifact", "docs/spec.md", "NOT_REVIEWED", nil))

	logs, err := ForTarget(root, "docs/spec.md")
	if err != nil {
		t.Fatalf("target logs: %v", err)
	}
	if len(logs) != 1 || !logs[0].HasUnresolvedBlockers {
		t.Fatalf("expected NOT_REVIEWED artifact to be unresolved: %+v", logs)
	}
}

// allArtifactReviewerRows is the complete set of 8 required reviewer rows for a 0.8.0
// artifact review (the baseline a completed review must have).
var allArtifactReviewerRows = []string{
	"| Feasibility | PASS | 0 | 0 | ok |",
	"| Completeness | PASS | 0 | 0 | ok |",
	"| Scope and alignment | PASS | 0 | 0 | ok |",
	"| Architecture | PASS | 0 | 0 | ok |",
	"| Intent preservation | PASS | 0 | 0 | ok |",
	"| Security | PASS | 0 | 0 | ok |",
	"| Testing-quality | PASS | 0 | 0 | ok |",
	"| Data-migration | PASS | 0 | 0 | ok |",
}

func TestArtifactMissingRequiredReviewerRowsIsUnresolved(t *testing.T) {
	// Each required lens must be enforced: remove exactly one from the complete set and
	// assert the review is unresolved. Covers the original 5 + the 3 new 0.8.0 lenses
	// (Security, Testing-quality, Data-migration) — the prior 2-row fixture only omitted
	// Feasibility/Completeness and so did not exercise the new enforcement.
	for _, omit := range []string{
		"Feasibility", "Completeness", "Scope and alignment", "Architecture",
		"Intent preservation", "Security", "Testing-quality", "Data-migration",
	} {
		omit := omit
		t.Run("missing_"+strings.ReplaceAll(strings.ReplaceAll(omit, " ", "_"), "-", "_"), func(t *testing.T) {
			root := t.TempDir()
			rows := make([]string, 0, len(allArtifactReviewerRows))
			for _, r := range allArtifactReviewerRows {
				if strings.Contains(r, omit) {
					continue
				}
				rows = append(rows, r)
			}
			mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "artifact.md"),
				artifactReviewMarkdown("mrv-artifact", "docs/spec.md", "PASS", rows))

			logs, err := ForTarget(root, "docs/spec.md")
			if err != nil {
				t.Fatalf("target logs: %v", err)
			}
			if len(logs) != 1 || !logs[0].HasUnresolvedBlockers {
				t.Fatalf("expected missing %s reviewer row to be unresolved: %+v", omit, logs)
			}
		})
	}
}

func TestCompletedArtifactReviewIsNotUnresolved(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "artifact.md"),
		artifactReviewMarkdown("mrv-artifact", "docs/spec.md", "PASS", allArtifactReviewerRows))

	logs, err := ForTarget(root, "docs/spec.md")
	if err != nil {
		t.Fatalf("target logs: %v", err)
	}
	if len(logs) != 1 || logs[0].HasUnresolvedBlockers {
		t.Fatalf("expected completed artifact review not to be unresolved: %+v", logs)
	}
}

func TestPassReviewMentioningHistoricalNeedsRevisionIsNotUnresolved(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "task.md"),
		"# metareview: task-done review\n\n"+
			"Run ID: `mrv-task`\n\n"+
			"Target: `task-1`\n\n"+
			"## Verdict\n\nPASS\n\n"+
			"## Notes\n\nPrevious run mrv-old was NEEDS_REVISION; this run fixed it.\n\n"+
			"## Findings\n\nNo blocking findings.\n")

	logs, err := ForTarget(root, "task-1")
	if err != nil {
		t.Fatalf("target logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected one log, got %+v", logs)
	}
	if logs[0].HasUnresolvedBlockers {
		t.Fatalf("historical NEEDS_REVISION prose must not poison PASS: %+v", logs[0])
	}
}

func TestDiscoverParsesLegacyPreviousRunFromMarkdown(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "task.md"),
		"# metareview: pr-ready review\n\n"+
			"Run ID: `mrv-task`\n\n"+
			"Target: `current branch`\n\n"+
			"Context pack: `docs/metareview/context/mrv-task-context.md`\n\n"+
			"Previous run: `mrv-root`\n\n"+
			"## Verdict\n\nNEEDS_REVISION\n")

	logs, err := Discover(root)
	if err != nil {
		t.Fatalf("discover logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected one log, got %+v", logs)
	}
	if logs[0].PreviousRunID != "mrv-root" {
		t.Fatalf("expected previous run from Markdown, got %+v", logs[0])
	}
	if logs[0].ContextRel != "docs/metareview/context/mrv-task-context.md" {
		t.Fatalf("expected context pack from Markdown, got %+v", logs[0])
	}
}

func TestEscalatedVerdictIsUnresolved(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "task.md"), reviewMarkdown("mrv-task", "task-1", "ESCALATED", ""))

	logs, err := ForTarget(root, "task-1")
	if err != nil {
		t.Fatalf("target logs: %v", err)
	}
	if len(logs) != 1 || !logs[0].HasUnresolvedBlockers {
		t.Fatalf("expected ESCALATED to be unresolved: %+v", logs)
	}
}

func TestDiscoverMergesRunAttemptMetadata(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "task.md"), reviewMarkdown("mrv-task", "task-1", "ESCALATED", ""))
	mustWrite(t, filepath.Join(root, ".metareview", "runs.jsonl"), `{"id":"mrv-root","scope":"task-done","target":{"type":"path","id":"task-1"},"verdict":"NEEDS_REVISION","attemptNumber":1,"maxAttempts":3}`+"\n"+
		`{"id":"mrv-task","scope":"task-done","target":{"type":"path","id":"task-1"},"verdict":"ESCALATED","previousRunId":"mrv-root","attemptNumber":3,"maxAttempts":3,"blockingFindingCount":1,"advisoryFindingCount":2,"followUpFindingCount":1}`+"\n")

	logs, err := ForTarget(root, "task-1")
	if err != nil {
		t.Fatalf("target logs: %v", err)
	}
	log := logs[0]
	if log.AttemptNumber != 3 || log.MaxAttempts != 3 || log.BlockingFindingCount != 1 || log.AdvisoryFindingCount != 2 || log.FollowUpFindingCount != 1 {
		t.Fatalf("expected run metadata merged into summary: %+v", log)
	}
	if len(log.RunChain) != 2 || log.RunChain[0].ID != "mrv-root" || log.RunChain[1].ID != "mrv-task" {
		t.Fatalf("expected full run chain in summary: %+v", log.RunChain)
	}
}

func TestDiscoverSurfacesUnknownClassificationWarning(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "task.md"), reviewMarkdown("mrv-task", "task-1", "PASS_ADVISORY", ""))
	mustWrite(t, filepath.Join(root, ".metareview", "runs.jsonl"), `{"id":"mrv-task","attemptNumber":1,"maxAttempts":3,"warningFindingCount":1}`+"\n")

	logs, err := ForTarget(root, "task-1")
	if err != nil {
		t.Fatalf("target logs: %v", err)
	}
	if len(logs[0].Warnings) != 1 || !strings.Contains(logs[0].Warnings[0], "unknown finding classification") {
		t.Fatalf("expected unknown-classification warning in review log summary: %+v", logs[0].Warnings)
	}
}

func reviewMarkdown(runID, target, verdict, findingID string) string {
	finding := "No blocking findings.\n"
	if findingID != "" {
		finding = "### " + findingID + ": Blocked\n"
	}
	return "# metareview: task-done review\n\n" +
		"Run ID: `" + runID + "`\n\n" +
		"Target: `" + target + "`\n\n" +
		"## Verdict\n\n" + verdict + "\n\n" +
		"## Findings\n\n" + finding
}

func artifactReviewMarkdown(runID, target, verdict string, rows []string) string {
	table := "| Reviewer | Verdict | Blocking | Warnings | Notes |\n| --- | --- | ---: | ---: | --- |\n"
	for _, row := range rows {
		table += row + "\n"
	}
	return "# metareview: artifact review\n\n" +
		"Run ID: `" + runID + "`\n\n" +
		"Target: `" + target + "`\n\n" +
		"## Verdict\n\n" + verdict + "\n\n" +
		"## Reviewer Results\n\n" + table + "\n" +
		"## Findings\n\nNo blocking findings.\n"
}

func TestOverridePendingFindingsAreUnresolvedBlockers(t *testing.T) {
	// A pending override must be treated as an unresolved blocker in the review log.
	// The isOpenBlocker function must use findings.Blocks(status) to correctly
	// identify both "open" and "override-pending" statuses as blocking.
	// Using a PASS verdict ensures only findings (not verdict) determine unresolved status.
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "task.md"), reviewMarkdown("mrv-task", "task-1", "PASS", "mrvf-task-001"))
	mustWrite(t, filepath.Join(root, ".metareview", "findings.jsonl"), `{"id":"mrvf-task-001","runId":"mrv-task","status":"override-pending","classification":"blocking","severity":"high","title":"Escalated blocker","target":{"id":"task-1","type":"beads-task"}}`+"\n")

	logs, err := ForTarget(root, "task-1")
	if err != nil {
		t.Fatalf("target logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	if !logs[0].HasUnresolvedBlockers {
		t.Fatalf("override-pending finding must mark the review as having unresolved blockers; "+
			"isOpenBlocker must use findings.Blocks() to treat override-pending as blocking: %+v", logs[0])
	}
	if len(logs[0].FindingIDs) != 1 || logs[0].FindingIDs[0] != "mrvf-task-001" {
		t.Fatalf("expected override-pending finding in summary: %+v", logs[0])
	}
}

func TestOverriddenFindingsAreNotUnresolvedBlockers(t *testing.T) {
	// A granted override must NOT be treated as an unresolved blocker.
	// Only "open" and "override-pending" statuses should be blocking.
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "task.md"), reviewMarkdown("mrv-task", "task-1", "PASS", "mrvf-task-001"))
	mustWrite(t, filepath.Join(root, ".metareview", "findings.jsonl"), `{"id":"mrvf-task-001","runId":"mrv-task","status":"overridden","classification":"blocking","severity":"high","title":"Granted override","target":{"id":"task-1","type":"beads-task"}}`+"\n")

	logs, err := ForTarget(root, "task-1")
	if err != nil {
		t.Fatalf("target logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	if logs[0].HasUnresolvedBlockers {
		t.Fatalf("overridden (granted) finding must not mark the review as having unresolved blockers: %+v", logs[0])
	}
}

func TestSpecContractFindingsAreAlwaysUnresolvedBlockers(t *testing.T) {
	// Spec-contract findings are always blocking regardless of severity.
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "task.md"), reviewMarkdown("mrv-task", "task-1", "PASS", "mrvf-task-001"))
	mustWrite(t, filepath.Join(root, ".metareview", "findings.jsonl"), `{"id":"mrvf-task-001","runId":"mrv-task","status":"open","classification":"spec-contract","severity":"medium","title":"Spec contract violation","target":{"id":"task-1","type":"beads-task"}}`+"\n")

	logs, err := ForTarget(root, "task-1")
	if err != nil {
		t.Fatalf("target logs: %v", err)
	}
	if len(logs) != 1 || !logs[0].HasUnresolvedBlockers {
		t.Fatalf("spec-contract finding must always be blocking, even with medium severity: %+v", logs)
	}
}

func TestBlockingWithCriticalSeverityIsUnresolvedBlocker(t *testing.T) {
	// Blocking classification with critical severity is a blocker.
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "task.md"), reviewMarkdown("mrv-task", "task-1", "PASS", "mrvf-task-001"))
	mustWrite(t, filepath.Join(root, ".metareview", "findings.jsonl"), `{"id":"mrvf-task-001","runId":"mrv-task","status":"open","classification":"blocking","severity":"critical","title":"Critical blocking issue","target":{"id":"task-1","type":"beads-task"}}`+"\n")

	logs, err := ForTarget(root, "task-1")
	if err != nil {
		t.Fatalf("target logs: %v", err)
	}
	if len(logs) != 1 || !logs[0].HasUnresolvedBlockers {
		t.Fatalf("blocking finding with critical severity must be blocking: %+v", logs)
	}
}

func TestBlockingWithLowSeverityIsNotUnresolvedBlocker(t *testing.T) {
	// Blocking classification with low severity is NOT a blocker (only high/critical matter).
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "task.md"), reviewMarkdown("mrv-task", "task-1", "PASS", "mrvf-task-001"))
	mustWrite(t, filepath.Join(root, ".metareview", "findings.jsonl"), `{"id":"mrvf-task-001","runId":"mrv-task","status":"open","classification":"blocking","severity":"low","title":"Low severity blocking issue","target":{"id":"task-1","type":"beads-task"}}`+"\n")

	logs, err := ForTarget(root, "task-1")
	if err != nil {
		t.Fatalf("target logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	if logs[0].HasUnresolvedBlockers {
		t.Fatalf("blocking finding with low severity must not be blocking: %+v", logs[0])
	}
}

func mustWrite(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// readFindings shares jsonl.NewScanner with every other .jsonl reader; this pins that it
// uses it. findings.jsonl carries reviewer-supplied Found/Evidence strings, so a max-length
// line is realistic, and a bare-cap scanner fails the WHOLE file — every finding, not one.
func TestReadFindingsAcceptsAnExactlyMaxLengthLine(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".metareview"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	rec := findingRecord{ID: "mrvf-1", RunID: "r1", Status: "open", Target: map[string]any{"pad": ""}}
	encode := func(v findingRecord) []byte {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return b
	}
	if pad := jsonl.MaxLineBytes - len(encode(rec)); pad > 0 {
		rec.Target["pad"] = strings.Repeat("x", pad)
	}
	line := encode(rec)
	if len(line) != jsonl.MaxLineBytes {
		t.Fatalf("fixture is %d bytes, want %d", len(line), jsonl.MaxLineBytes)
	}
	body := append(append(line, '\n'), append(encode(findingRecord{ID: "mrvf-2", RunID: "r1", Status: "open"}), '\n')...)
	if err := os.WriteFile(filepath.Join(root, ".metareview", "findings.jsonl"), body, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := readFindings(root)
	if err != nil {
		t.Fatalf("readFindings: %v", err)
	}
	if len(got) != 2 || got[0].ID != "mrvf-1" || got[1].ID != "mrvf-2" {
		t.Fatalf("got %d records %+v, want both", len(got), got)
	}
}

// Grandfathering: a completed artifact review must be judged against the lens set that was
// required when it ran, not the set required today. Three lenses were added after the earliest
// reviews in this repository (security in 0.7.0, testing-quality and data-migration in 0.8.0);
// without this, every historical artifact review turns into a permanent blocker the moment
// somebody edits the file it reviewed, clearable only by a standing override.
//
// The grandfather is bounded, which is the point of the cutoff: a log written today cannot get
// the easier legacy rubric by simply omitting the marker, because "no marker" only means "legacy"
// for run IDs that predate the marker itself.
func TestArtifactLensSetIsGrandfathered(t *testing.T) {
	rows := func(names ...string) string {
		out := "## Reviewer Results\n\n| Reviewer | Verdict | Blocking | Notes |\n| --- | --- | ---: | --- |\n"
		for _, n := range names {
			out += "| " + n + " | PASS | 0 | ok |\n"
		}
		return out
	}
	five := []string{"Feasibility", "Completeness", "Scope and alignment", "Architecture", "Intent preservation"}
	eight := append(append([]string{}, five...), "Security", "Testing-quality", "Data-migration")
	log := func(runID, declared, table string) string {
		s := "# metareview: artifact review\n\nRun ID: `" + runID + "`\n\nTarget: `A.md`\n\n"
		if declared != "" {
			s += "Required lenses: `" + declared + "`\n\n"
		}
		return s + "## Verdict\n\nPASS\n\n" + table
	}
	all := "feasibility, completeness, scope-and-alignment, architecture, intent-preservation, security, testing-quality, data-migration"

	for _, tc := range []struct {
		name     string
		text     string
		blocking bool
	}{
		{"legacy log, five lenses, no marker", log("mrv-20260705-1-artifact-a-1", "", rows(five...)), false},
		{"legacy log missing an original lens still blocks", log("mrv-20260705-1-artifact-a-1", "", rows(five[:4]...)), true},
		{"post-cutoff log with only five lenses blocks", log("mrv-20260829-1-artifact-a-1", "", rows(five...)), true},
		{"post-cutoff log with eight lenses passes", log("mrv-20260829-1-artifact-a-1", all, rows(eight...)), false},
		{"declared set is what counts, not the cutoff", log("mrv-20260705-1-artifact-a-1", all, rows(five...)), true},
		// An all-digit run of eight characters is not a date. Accepting one as legacy would hand the
		// five-lens rubric to any log carrying a plausible-looking id, which is the opposite of a
		// grandfather bounded by verifiable provenance.
		// A declaration may strengthen what a log is held to; it may never weaken it. Declaring the
		// legacy five on a post-cutoff log is an opt-out of security, testing-quality and
		// data-migration - the exact escape the unmarked path already refuses after the cutoff.
		{"post-cutoff log cannot declare the legacy rubric", log("mrv-20260829-1-artifact-a-1", "feasibility, completeness, scope-and-alignment, architecture, intent-preservation", rows(five...)), true},
		// A declaration is provenance, not permission. Accepting an arbitrary one lets a log name a
		// one-lens rubric, satisfy it with a single row, and be reported complete - so a current
		// review could skip security, testing-quality and data-migration by declaring them away.
		{"partial declared set does not certify itself", log("mrv-20260829-1-artifact-a-1", "feasibility", rows(five[:1]...)), true},
		{"unknown lens in the declaration falls back to current", log("mrv-20260829-1-artifact-a-1", "feasibility, completeness, vibes", rows(five...)), true},
		{"pre-cutoff log cannot declare its way out either", log("mrv-20260705-1-artifact-a-1", "feasibility", rows(five[:1]...)), true},
		{"impossible calendar date is not legacy", log("mrv-20260230-1-artifact-a-1", "", rows(five...)), true},
		{"month 13 is not legacy", log("mrv-20261305-1-artifact-a-1", "", rows(five...)), true},
		{"day 00 is not legacy", log("mrv-20260700-1-artifact-a-1", "", rows(five...)), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := parseMarkdown("docs/metareview/reviews/x.md", tc.text).HasUnresolvedBlockers
			if got != tc.blocking {
				t.Errorf("HasUnresolvedBlockers = %v, want %v", got, tc.blocking)
			}
		})
	}
}

// The era table is what keeps the marker meaningful across the NEXT lens addition. Judging a log
// against a live "current" set means every existing declaration stops matching the day a lens is
// added, and every completed review becomes incomplete again - the standing-override failure the
// marker exists to prevent, returning at exactly the moment it is needed. Eras are keyed by the
// date a rubric took effect, so adding a lens means appending an era and leaves older logs alone.
func TestLensErasAreKeyedByDate(t *testing.T) {
	for _, tc := range []struct {
		runID string
		want  []string
	}{
		{"mrv-20260705-1-artifact-a-1", legacyLenses},
		{"mrv-20260823-1-artifact-a-1", legacyLenses},
		{"mrv-20260824-1-artifact-a-1", currentLenses},
		{"mrv-20260829-1-artifact-a-1", currentLenses},
		{"mrv-notadate-1-artifact-a-1", currentLenses},
	} {
		got := eraLenses(tc.runID)
		if !sameLensSet(got, tc.want) {
			t.Errorf("eraLenses(%s) = %v, want %v", tc.runID, got, tc.want)
		}
	}
	// Appending a future era must not reach back. This is the property that breaks if the rule is
	// "whatever the current set happens to be".
	saved := lensEras
	defer func() { lensEras = saved }()
	ninth := append(append([]string{}, currentLenses...), "supplychain")
	lensEras = append(append([]lensEra{}, lensEras...), lensEra{from: "20270101", lenses: ninth})
	if !sameLensSet(eraLenses("mrv-20260829-1-artifact-a-1"), currentLenses) {
		t.Error("adding a later era must not change what an earlier log is judged against")
	}
	if !sameLensSet(eraLenses("mrv-20270102-1-artifact-a-1"), ninth) {
		t.Error("a log written in the new era must be judged against it")
	}
}

// The severity policy is "critical and high block; medium and low do not", and only three of its
// four cases were pinned - medium had none, so widening the set from (critical|high) to
// != "low" left the suite green and silently promoted every medium finding to a blocker. The
// enumeration is now complete, which is the point of an enumeration.
func TestOnlyCriticalAndHighSeveritiesBlock(t *testing.T) {
	for _, tc := range []struct {
		severity string
		blocks   bool
	}{
		{"critical", true},
		{"high", true},
		{"medium", false},
		{"low", false},
		{"", false},
	} {
		t.Run(tc.severity, func(t *testing.T) {
			got := isOpenBlocker(findingRecord{Status: "open", Classification: "blocking", Severity: tc.severity})
			if got != tc.blocks {
				t.Errorf("severity %q: isOpenBlocker = %v, want %v", tc.severity, got, tc.blocks)
			}
		})
	}
	// spec-contract blocks whatever its severity, which is the one exception to the rule above.
	if !isOpenBlocker(findingRecord{Status: "open", Classification: "spec-contract", Severity: "low"}) {
		t.Error("a spec-contract finding must block regardless of severity")
	}
}
