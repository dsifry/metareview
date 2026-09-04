package reviewlog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/jsonl"
	"github.com/dsifry/metareview/internal/lens"
)

// Every lens's marker Slug and its Display name must normalize to the SAME key. The scaffold writes
// the Slugs into "Required lenses:" while the reviewer rows carry the Display names; if the two
// normalize apart, a declared set and its completed rows refer to different lenses and the review
// never reads as complete — the "scope-alignment" vs "Scope and alignment" trap. This guards it for
// every lens in the canonical set, including any added later, and also proves currentLensKeys()
// (derived from Display) matches what a reviewer row (Slug or Display) normalizes to.
func TestEveryLensSlugAndDisplayNormalizeToSameKey(t *testing.T) {
	for _, l := range lens.All {
		fromSlug, fromDisplay := normalizedReviewer(l.Slug), normalizedReviewer(l.Display)
		if fromSlug != fromDisplay {
			t.Errorf("lens %q: slug %q normalizes to %q but display normalizes to %q — they must fold to one key (add a canonicalLens fold)",
				l.Display, l.Slug, fromSlug, fromDisplay)
		}
	}
}

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

// allArtifactReviewerRows is the complete set of 9 required reviewer rows for a current
// (0.9.0) artifact review (the baseline a completed review must have). The fixtures below use
// the dateless run ID "mrv-artifact", which is judged against the newest lens set.
var allArtifactReviewerRows = []string{
	"| Feasibility | PASS | 0 | 0 | ok |",
	"| Completeness | PASS | 0 | 0 | ok |",
	"| Scope and alignment | PASS | 0 | 0 | ok |",
	"| Architecture | PASS | 0 | 0 | ok |",
	"| Intent preservation | PASS | 0 | 0 | ok |",
	"| Security | PASS | 0 | 0 | ok |",
	"| Testing-quality | PASS | 0 | 0 | ok |",
	"| Data-migration | PASS | 0 | 0 | ok |",
	"| Mechanical-precision | PASS | 0 | 0 | ok |",
}

func TestArtifactMissingRequiredReviewerRowsIsUnresolved(t *testing.T) {
	// Each required lens must be enforced: remove exactly one from the complete set and
	// assert the review is unresolved. Covers the original 5 + the 3 new 0.8.0 lenses
	// (Security, Testing-quality, Data-migration) + the 0.9.0 Mechanical-precision lens —
	// the prior 2-row fixture only omitted Feasibility/Completeness and so did not exercise
	// the new enforcement.
	for _, omit := range []string{
		"Feasibility", "Completeness", "Scope and alignment", "Architecture",
		"Intent preservation", "Security", "Testing-quality", "Data-migration",
		"Mechanical-precision",
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
		{"mrv-20260824-1-artifact-a-1", v08Lenses},
		{"mrv-20260829-1-artifact-a-1", v08Lenses},
		{"mrv-20260830-1-artifact-a-1", v08Lenses},
		// The newest era is pinned against the FROZEN v09Lenses, not the live currentLenses. That is
		// what makes the era table's promise hold: growing lens.All (and with it currentLenses)
		// without cutting a new frozen snapshot and era would make eraLenses("mrv-20260831-…")
		// return more than nine and fail here, instead of silently expanding what every 2026-08-31+
		// log must cover. A dateless id maps to the newest era, so it too is v09Lenses.
		{"mrv-20260831-1-artifact-a-1", v09Lenses},
		{"mrv-20260901-1-artifact-a-1", v09Lenses},
		{"mrv-notadate-1-artifact-a-1", v09Lenses},
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
	future := append(append([]string{}, currentLenses...), "supplychain")
	lensEras = append(append([]lensEra{}, lensEras...), lensEra{from: "20270101", lenses: future})
	if !sameLensSet(eraLenses("mrv-20260829-1-artifact-a-1"), v08Lenses) {
		t.Error("adding a later era must not change what an earlier log is judged against")
	}
	if !sameLensSet(eraLenses("mrv-20270102-1-artifact-a-1"), future) {
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

// A declaration that repeats a lens is malformed, and malformed is not "the legacy rubric".
// Comparing only the count of UNIQUE names let "the five legacy lenses, one of them twice" match
// legacyLenses, so a pre-cutoff log could satisfy the gate with five rows on an invalid marker.
func TestDuplicateLensDeclarationIsNotAShippedRubric(t *testing.T) {
	dup := append(append([]string{}, legacyLenses...), legacyLenses[0])
	if known := knownRubric(dup); known != nil {
		t.Errorf("a declaration repeating a lens matched %v; it names no shipped rubric", known)
	}
	if known := knownRubric(legacyLenses); known == nil {
		t.Error("the legacy rubric itself must still be recognised")
	}
	if known := knownRubric(v08Lenses); known == nil {
		t.Error("the frozen 0.8.0 eight-lens rubric must still be recognised")
	}
	if known := knownRubric(v09Lenses); known == nil {
		t.Error("the frozen 0.9.0 nine-lens rubric must still be recognised")
	}
	if known := knownRubric(currentLenses); known == nil {
		t.Error("the current rubric must still be recognised")
	}
}

// HeadSHA and CoveredPaths live on the run record and have to be joined onto the summary, or the
// two questions that make scoping possible cannot be asked: "which commit was this review of"
// and "which files did it actually read". Before this, status could only compare target strings —
// and a review's target is a task id or the literal `current branch`, never a source path, so
// asking about a file matched nothing and the file reported as clear.
func TestSummaryCarriesHeadAndCoveredPathsFromTheRunRecord(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "one.md"), reviewMarkdown("mrv-1", "t-1", "PASS", ""))
	mustWrite(t, filepath.Join(root, ".metareview", "runs.jsonl"),
		`{"id":"mrv-1","scope":"task-done","verdict":"PASS","headSha":"abc1234def","baseSha":"base9999","coveredPaths":["internal/a.go","internal/b.go"]}`+"\n")

	logs, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("got %d summaries, want 1", len(logs))
	}
	if logs[0].HeadSHA != "abc1234def" {
		t.Errorf("HeadSHA = %q, want the run record's", logs[0].HeadSHA)
	}
	// Base joins from the run record alongside head so same-head dedup can key on the full identity (#99).
	if logs[0].BaseSHA != "base9999" {
		t.Errorf("BaseSHA = %q, want the run record's", logs[0].BaseSHA)
	}
	if len(logs[0].CoveredPaths) != 2 || logs[0].CoveredPaths[0] != "internal/a.go" {
		t.Errorf("CoveredPaths = %v, want the run record's", logs[0].CoveredPaths)
	}
	// A review recorded before these fields existed carries neither, and must report them empty
	// rather than borrowing another run's — empty means "unknown", which is what stops an old log
	// from answering for a file it never read.
	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "zero.md"), reviewMarkdown("mrv-0", "t-0", "PASS", ""))
	logs, err = Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range logs {
		if l.RunID == "mrv-0" && (l.HeadSHA != "" || l.BaseSHA != "" || len(l.CoveredPaths) != 0) {
			t.Errorf("a legacy review must carry nothing: %+v", l)
		}
	}
}

func TestSummaryAuthenticatesReviewerInputMetadataAgainstLocalRunRecord(t *testing.T) {
	root := t.TempDir()
	rel := "docs/metareview/reviews/mrv-pr.md"
	mustWrite(t, filepath.Join(root, rel),
		"# metareview: pr-ready review\n\nRun ID: `mrv-pr`\n\nTarget: `current branch`\n\nReviewer input digest: `sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa`\n\n## Verdict\n\nPASS\n")
	mustWrite(t, filepath.Join(root, ".metareview", "runs.jsonl"),
		`{"id":"mrv-pr","scope":"pr-ready","target":{"type":"branch","id":"feature"},"status":"passed","verdict":"PASS","executionMode":"deterministic-local","baseSha":"base","headSha":"head","reviewLogPath":"docs/metareview/reviews/mrv-pr.md","reviewers":["one","two"],"reviewInputDigest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`+"\n")

	logs, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	got := logs[0]
	if !got.RunRecordAuthenticated || got.BaseSHA != "base" || got.ReviewInputDigest == "" {
		t.Fatalf("expected authenticated input metadata from matching local record: %+v", got)
	}
	if got.TargetRecord["id"] != "feature" || len(got.Reviewers) != 2 {
		t.Fatalf("expected target and reviewer identity from local record: %+v", got)
	}

	// The local record is not allowed to authenticate an edited committed verdict.
	mustWrite(t, filepath.Join(root, rel),
		"# metareview: pr-ready review\n\nRun ID: `mrv-pr`\n\nTarget: `current branch`\n\nReviewer input digest: `sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa`\n\n## Verdict\n\nNEEDS_REVISION\n")
	logs, err = Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if logs[0].RunRecordAuthenticated {
		t.Fatalf("a verdict mismatch must make reuse metadata unauthenticated: %+v", logs[0])
	}

	mustWrite(t, filepath.Join(root, rel),
		"# metareview: pr-ready review\n\nRun ID: `mrv-pr`\n\nTarget: `current branch`\n\nReviewer input digest: `sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb`\n\n## Verdict\n\nPASS\n")
	logs, err = Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if logs[0].RunRecordAuthenticated {
		t.Fatalf("a stale committed digest must not authenticate the local run: %+v", logs[0])
	}

	// A matching digest and verdict still cannot let one run record authenticate
	// a different review document.
	mustWrite(t, filepath.Join(root, rel),
		"# metareview: pr-ready review\n\nRun ID: `mrv-pr`\n\nTarget: `current branch`\n\nReviewer input digest: `sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa`\n\n## Verdict\n\nPASS\n")
	mustWrite(t, filepath.Join(root, ".metareview", "runs.jsonl"),
		`{"id":"mrv-pr","scope":"pr-ready","target":{"type":"branch","id":"feature"},"status":"passed","verdict":"PASS","executionMode":"deterministic-local","baseSha":"base","headSha":"head","reviewLogPath":"docs/metareview/reviews/other.md","reviewers":["one","two"],"reviewInputDigest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`+"\n")
	logs, err = Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if logs[0].RunRecordAuthenticated {
		t.Fatalf("a run record naming another review must not authenticate: %+v", logs[0])
	}
}

// Scoping has to survive leaving the machine that produced the review. HeadSHA and CoveredPaths
// lived only in .metareview/runs.jsonl, which is untracked — so a clone, a fresh worktree or a
// CI checkout had review logs it could not attribute to any commit, every file read as
// UNREVIEWED, and no historical blocker was ever in scope. Found by rebuilding the self-test
// worktree from scratch, which is exactly what that exercise was for.
func TestSummaryReadsHeadAndPathsFromTheCommittedLogAlone(t *testing.T) {
	root := t.TempDir() // no .metareview/runs.jsonl at all, as in a fresh clone
	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "one.md"),
		"# metareview: pr-ready review\n\nRun ID: `mrv-1`\n\nTarget: `current branch`\n\n"+
			"Head: `abc1234def`\n\nCovered paths: `[\"internal/a.go\",\"internal/b.go\"]`\n\n## Verdict\n\nPASS\n")

	logs, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("got %d, want 1", len(logs))
	}
	if logs[0].HeadSHA != "abc1234def" {
		t.Errorf("HeadSHA = %q: the committed log must carry it", logs[0].HeadSHA)
	}
	if len(logs[0].CoveredPaths) != 2 || logs[0].CoveredPaths[1] != "internal/b.go" {
		t.Errorf("CoveredPaths = %v, want both files", logs[0].CoveredPaths)
	}
}

// "none" and absent are different answers, and the type must be able to hold the difference.
// The previous version of this test asserted that BOTH returned nil — it proved they were
// identical while its name claimed the opposite, and three comments described a distinction no
// code implemented. A review that examined no files can answer "no" for a path; one written
// before the field existed cannot answer at all, and only the second must be barred from
// vouching.
func TestCoveredPathsDistinguishesNoneFromUnknown(t *testing.T) {
	if paths, known := DecodeCoveredPaths(NoCoveredPaths); !known || len(paths) != 0 {
		t.Errorf(`"none" must decode as KNOWN and empty, got %v known=%v`, paths, known)
	}
	if paths, known := DecodeCoveredPaths(""); known || paths != nil {
		t.Errorf("an absent field must decode as UNKNOWN, got %v known=%v", paths, known)
	}
	// A legacy comma-joined line is not a path list. Refusing it is deliberate: guessing would
	// silently mark files reviewed that nobody read.
	if paths, known := DecodeCoveredPaths("a.go, b.go"); known || paths != nil {
		t.Errorf("an unparseable legacy line must be unknown, not guessed: %v known=%v", paths, known)
	}
	// An unknown head is written as the literal `unknown` and must not become a SHA.
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "u.md"),
		"# metareview: task-done review\n\nRun ID: `mrv-u`\n\nTarget: `t`\n\nHead: `unknown`\n\n## Verdict\n\nPASS\n")
	logs, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if logs[0].HeadSHA != "" {
		t.Errorf("an unknown head must stay empty, got %q", logs[0].HeadSHA)
	}
}

// The LOCAL RUN RECORD wins over the committed log, and the log is the fallback that lets a
// clone answer at all. This assertion used to be the other way round, and that was a security
// regression: the markdown travels in a pull request and is editable by anyone who can commit,
// while .metareview/runs.jsonl is locally produced and gitignored — the only copy an attacker
// cannot supply — and nothing verifies a log's self-asserted head against the commit containing it.
func TestTheLocalRunRecordWinsOverTheCommittedLog(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "one.md"),
		"# metareview: pr-ready review\n\nRun ID: `mrv-1`\n\nTarget: `t`\n\n"+
			"Head: `fromlog`\n\nCovered paths: `[\"log.go\"]`\n\n## Verdict\n\nPASS\n")
	mustWrite(t, filepath.Join(root, ".metareview", "runs.jsonl"),
		`{"id":"mrv-1","scope":"pr-ready","verdict":"PASS","headSha":"fromrun","coveredPaths":["run.go"]}`+"\n")

	logs, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if logs[0].HeadSHA != "fromrun" || len(logs[0].CoveredPaths) != 1 || logs[0].CoveredPaths[0] != "run.go" {
		t.Errorf("the local run record must win: head=%q paths=%v", logs[0].HeadSHA, logs[0].CoveredPaths)
	}
	// ...and a log with no run record still answers from the committed artifact, which is the
	// whole point of writing it there: a clone has no run record at all.
	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "two.md"),
		"# metareview: pr-ready review\n\nRun ID: `mrv-2`\n\nTarget: `t`\n\n## Verdict\n\nPASS\n")
	mustWrite(t, filepath.Join(root, ".metareview", "runs.jsonl"),
		`{"id":"mrv-1","scope":"pr-ready","verdict":"PASS","headSha":"fromrun","coveredPaths":["run.go"]}`+"\n"+
			`{"id":"mrv-2","scope":"pr-ready","verdict":"PASS","headSha":"legacy","coveredPaths":["legacy.go"]}`+"\n")
	logs, err = Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	// Looked up rather than filtered inside the loop. The previous form guarded its only
	// assertion behind `if l.RunID == "mrv-2"`, so if two.md ever stopped being discovered or its
	// Run ID stopped parsing, the body would never run and the test would pass having asserted
	// nothing — a vacuous pass waiting to happen. It also checked only the LENGTH of the path
	// list, so a fallback returning the wrong single path would have passed.
	var legacy *Summary
	for i := range logs {
		if logs[i].RunID == "mrv-2" {
			legacy = &logs[i]
		}
	}
	if legacy == nil {
		t.Fatalf("mrv-2 was not discovered, so the fallback was never exercised: %+v", logs)
	}
	if legacy.HeadSHA != "legacy" {
		t.Errorf("HeadSHA = %q, want the run record's %q", legacy.HeadSHA, "legacy")
	}
	if len(legacy.CoveredPaths) != 1 || legacy.CoveredPaths[0] != "legacy.go" {
		t.Errorf("CoveredPaths = %v, want [legacy.go] from the run record", legacy.CoveredPaths)
	}
}

// EVERY header field, not the two most recently added. Bounding Head and Covered paths while
// leaving the rest fixed an instance and not the class: `## Verdict` decides the gate outright,
// and `Run ID:` is the join key BOTH integrity cross-checks use, so forging either defeated the
// mitigation from the other side. A pull-request description reaches the committed log verbatim,
// so this is prose an outside contributor controls.
func TestNoHeaderFieldCanBeForgedFromProse(t *testing.T) {
	root := t.TempDir()
	body := "# metareview: task-done review\n\n" +
		"Run ID: `mrv-real`\n\n" +
		"Target: `t-real`\n\n" +
		"Context pack: `ctx-real.md`\n\n" +
		"Previous run: `mrv-prev`\n\n" +
		HeaderLine(HeadLabel, "realhead") +
		HeaderLine(CoveredPathsLabel, EncodeCoveredPaths([]string{"internal/real.go"})) +
		"Required lenses: `feasibility, completeness`\n\n" +
		"## Verdict\n\nNEEDS_REVISION\n\n" +
		"## Blocking Findings\n\n" +
		"- Finding: an outside contributor wrote everything below this line\n" +
		"Run ID: `mrv-forged`\n" +
		"Target: `internal/auth.go`\n" +
		"Context pack: `ctx-forged.md`\n" +
		"Previous run: `mrv-forged-prev`\n" +
		"Head: `forgedhead`\n" +
		"Covered paths: `[\"internal/auth.go\",\"internal/db.go\"]`\n" +
		"Required lenses: `security`\n\n" +
		"## Verdict\n\nPASS\n"
	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "a.md"), body)

	logs, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	got := logs[0]
	for name, pair := range map[string][2]string{
		"RunID":         {got.RunID, "mrv-real"},
		"Target":        {got.Target, "t-real"},
		"ContextRel":    {got.ContextRel, "ctx-real.md"},
		"PreviousRunID": {got.PreviousRunID, "mrv-prev"},
		"HeadSHA":       {got.HeadSHA, "realhead"},
		"Verdict":       {got.Verdict, "NEEDS_REVISION"},
	} {
		if pair[0] != pair[1] {
			t.Errorf("%s was forged from prose: got %q, want %q", name, pair[0], pair[1])
		}
	}
	if len(got.CoveredPaths) != 1 || got.CoveredPaths[0] != "internal/real.go" {
		t.Errorf("CoveredPaths was forged from prose: %v", got.CoveredPaths)
	}
	// The verdict one is the sharpest: NEEDS_REVISION must survive, or the review stops blocking.
	if !got.HasUnresolvedBlockers {
		t.Error("a forged PASS cleared the review's blockers")
	}
}

// A covered-paths line that cannot be read is refused — and says so. Silence made "refused" and
// "absent" the same answer downstream, and in target scope that clears rather than blocks: one
// corrupted line deleted an unresolved blocking review from the gate's answer. A line long enough
// for an editor to wrap is enough to trigger it.
func TestAnUnreadableCoveredPathsLineIsReportedNotIgnored(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "a.md"),
		"# metareview: task-done review\n\nRun ID: `mrv-1`\n\nTarget: `t`\n\n"+
			"Covered paths: `a.go, b.go`\n\n## Verdict\n\nNEEDS_REVISION\n")
	logs, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if logs[0].CoveredPathsKnown {
		t.Error("a legacy comma line must not be read as a known path set")
	}
	var warned bool
	for _, w := range logs[0].Warnings {
		if strings.Contains(w, "covered paths could not be read") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("refusing the line must be reported, not silent: %v", logs[0].Warnings)
	}
}

// The case that separates the two guards. First-match-wins and header-bounding overlap whenever
// the genuine field is PRESENT — the real value is seen first, so either guard alone suffices and
// removing one is invisible. The distinguishing document is one whose genuine field is ABSENT and
// whose only header-shaped line is in prose: then only header-bounding stops the forgery.
//
// This is the third time in this codebase that two overlapping guards hid each other's removal.
// A test that only exercises them together proves the pair, not the parts.
func TestProseCannotSupplyAHeaderFieldTheHeaderOmitted(t *testing.T) {
	root := t.TempDir()
	// A minimal, legitimate header: no Run ID, no Target, no Head, no Covered paths, no lenses.
	body := "# metareview: task-done review\n\n" +
		"## Verdict\n\nNEEDS_REVISION\n\n" +
		"## Blocking Findings\n\n" +
		"- Finding: everything below is text an outside contributor supplied\n" +
		"Run ID: `mrv-forged`\n" +
		"Target: `internal/auth.go`\n" +
		"Context pack: `ctx-forged.md`\n" +
		"Previous run: `mrv-forged-prev`\n" +
		"Head: `forgedhead`\n" +
		"Covered paths: `[\"internal/auth.go\"]`\n" +
		"Required lenses: `security`\n"
	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "a.md"), body)

	logs, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	got := logs[0]
	for name, value := range map[string]string{
		"RunID":         got.RunID,
		"Target":        got.Target,
		"ContextRel":    got.ContextRel,
		"PreviousRunID": got.PreviousRunID,
		"HeadSHA":       got.HeadSHA,
	} {
		if value != "" {
			t.Errorf("%s was supplied from prose for a header that omitted it: %q", name, value)
		}
	}
	if got.CoveredPathsKnown || len(got.CoveredPaths) != 0 {
		t.Errorf("covered paths were supplied from prose: %v known=%v", got.CoveredPaths, got.CoveredPathsKnown)
	}
	// A forged Head is the one that moves a review between branch scopes, so it must stay empty
	// rather than becoming a plausible-looking SHA.
	if got.HeadSHA == "forgedhead" {
		t.Error("prose set the head, which decides whether this review is in branch scope at all")
	}
}

// Required lenses is header-bounded for consistency, but the protection that actually holds is
// elsewhere and is worth stating: requiredLenses falls back to the ERA DEFAULT when a review
// declares nothing, and the default is the full set. So an empty or missing declaration makes a
// review stricter, never laxer, and prose cannot shrink the requirement by supplying one.
//
// This test exists because an earlier version of it asserted that forging the lens line changed
// the outcome. It does not — mutating the guard survives, honestly — and a test whose name
// promises a protection the code gets from somewhere else is worse than no test: it is the
// "asserts the opposite of what it claims" defect this loop has now found three times.
func TestAnUndeclaredLensSetFallsBackToTheStrictestDefault(t *testing.T) {
	full := requiredLenses(nil, "mrv-20260830-000000000000000-artifact-x")
	if len(full) == 0 {
		t.Fatal("an undeclared lens set must fall back to a non-empty default")
	}
	narrowed := requiredLenses([]string{"feasibility"}, "mrv-20260830-000000000000000-artifact-x")
	if len(narrowed) < len(full) {
		t.Errorf("a declaration may only strengthen: declared %d, default %d", len(narrowed), len(full))
	}
}
