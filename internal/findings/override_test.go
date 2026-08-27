package findings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func seedRecord(t *testing.T, root string, record Record) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".metareview"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONL(findingsPath(root), []Record{record}); err != nil {
		t.Fatal(err)
	}
}

func openBlocker(id string) Record {
	return Record{
		SchemaVersion:  1,
		ID:             id,
		RunID:          "mrv-run-1",
		Scope:          "pr-ready",
		Reviewer:       "architecture-reviewer",
		Severity:       "high",
		Classification: "blocking",
		Status:         "open",
		Title:          "Review context risk",
		Fingerprint:    "pr:architecture:context-risk",
		Target:         map[string]string{"type": "branch", "id": "feature"},
		CreatedAt:      "2026-08-27T00:00:00Z",
		UpdatedAt:      "2026-08-27T00:00:00Z",
	}
}

func loadOne(t *testing.T, root string) Record {
	t.Helper()
	records, err := readJSONL(findingsPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("want exactly one record, got %d", len(records))
	}
	return records[0]
}

func TestRequestOverrideMarksPendingAndKeepsBlocking(t *testing.T) {
	root := t.TempDir()
	seedRecord(t, root, openBlocker("mrvf-1"))

	if err := RequestOverride(root, "mrvf-1", OverrideRequest{
		By:         "orchestrator",
		Reason:     "chain exhausted at three attempts; blockers closed in the final revision",
		Escalation: "artifact review chain exhausted (attempt 3 of 3)",
		Now:        "2026-08-27T01:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	record := loadOne(t, root)
	if record.Status != StatusOverridePending {
		t.Fatalf("status = %q, want %q", record.Status, StatusOverridePending)
	}
	if record.OverrideRequestedBy != "orchestrator" || record.OverrideRequestReason == "" || record.OverrideRequestedAt == "" {
		t.Fatalf("request provenance is incomplete: %+v", record)
	}
	if record.OverrideEscalation == "" {
		t.Fatal("the escalation context must be recorded")
	}
	// A pending override still blocks: CI must stay red until it is granted.
	if len(unresolvedBlockingFrom([]Record{record})) != 1 {
		t.Fatal("a pending override must still count as a blocker")
	}
}

func TestGrantOverrideClearsBlockingWithAttribution(t *testing.T) {
	root := t.TempDir()
	seedRecord(t, root, openBlocker("mrvf-1"))
	if err := RequestOverride(root, "mrvf-1", OverrideRequest{
		By: "orchestrator", Reason: "three lens passes were enough evidence to proceed", Now: "2026-08-27T01:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	if err := GrantOverride(root, "mrvf-1", OverrideGrant{
		By: "dsifry@warmstart.ai", Reason: "reviewed the evidence and accept the exception", Now: "2026-08-27T02:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	record := loadOne(t, root)
	if record.Status != StatusOverridden {
		t.Fatalf("status = %q, want %q", record.Status, StatusOverridden)
	}
	if record.OverrideGrantedBy != "dsifry@warmstart.ai" || record.OverrideGrantReason == "" || record.OverrideGrantedAt == "" {
		t.Fatalf("grant provenance is incomplete: %+v", record)
	}
	if record.OverrideRequestedBy != "orchestrator" {
		t.Fatal("granting must not erase who requested the override")
	}
	if len(unresolvedBlockingFrom([]Record{record})) != 0 {
		t.Fatal("a granted override must stop blocking")
	}
	if record.FixedInRunID != "" {
		t.Fatal("an override is not a fix; fixedInRunId must stay empty")
	}
}

func TestGrantWithoutRequestIsAllowed(t *testing.T) {
	root := t.TempDir()
	seedRecord(t, root, openBlocker("mrvf-1"))
	if err := GrantOverride(root, "mrvf-1", OverrideGrant{
		By: "dsifry@warmstart.ai", Reason: "accepted directly; no agent escalation preceded it", Now: "2026-08-27T02:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	if loadOne(t, root).Status != StatusOverridden {
		t.Fatal("a human may override an open finding directly")
	}
}

func TestOverrideRequiresSubstantiveReasonAndActor(t *testing.T) {
	root := t.TempDir()
	seedRecord(t, root, openBlocker("mrvf-1"))
	if err := RequestOverride(root, "mrvf-1", OverrideRequest{By: "orchestrator", Reason: "because", Now: "t"}); err == nil {
		t.Fatal("a trivial reason must be refused")
	}
	if err := RequestOverride(root, "mrvf-1", OverrideRequest{Reason: "a perfectly good explanation of the exception", Now: "t"}); err == nil {
		t.Fatal("an override with no actor must be refused")
	}
	if err := GrantOverride(root, "mrvf-1", OverrideGrant{By: "someone", Reason: "ok", Now: "t"}); err == nil {
		t.Fatal("a trivial grant reason must be refused")
	}
	if loadOne(t, root).Status != "open" {
		t.Fatal("a refused override must not change the record")
	}
}

func TestOverrideUnknownOrClosedFinding(t *testing.T) {
	root := t.TempDir()
	seedRecord(t, root, openBlocker("mrvf-1"))
	if err := RequestOverride(root, "missing", OverrideRequest{By: "a", Reason: "a good reason for the exception", Now: "t"}); err == nil {
		t.Fatal("an unknown finding id must be refused")
	}
	fixed := openBlocker("mrvf-2")
	fixed.Status = "fixed"
	if err := writeJSONL(findingsPath(root), []Record{fixed}); err != nil {
		t.Fatal(err)
	}
	if err := RequestOverride(root, "mrvf-2", OverrideRequest{By: "a", Reason: "a good reason for the exception", Now: "t"}); err == nil {
		t.Fatal("overriding an already-closed finding must be refused")
	}
}

// A recurring condition must not spawn a duplicate record beside its override,
// and must not silently reopen.
func TestReconcileKeepsOverrideWhenTheFindingRecurs(t *testing.T) {
	root := t.TempDir()
	seedRecord(t, root, openBlocker("mrvf-1"))
	if err := GrantOverride(root, "mrvf-1", OverrideGrant{
		By: "dsifry@warmstart.ai", Reason: "accepted while the underlying condition persists", Now: "2026-08-27T02:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	run := Run{ID: "mrv-run-2", Scope: "pr-ready", Target: map[string]string{"type": "branch", "id": "feature"}, RepoRoot: root}
	same := Input{
		Reviewer: "architecture-reviewer", Severity: "high", Classification: "blocking",
		Title: "Review context risk", Fingerprint: "pr:architecture:context-risk",
	}
	if _, err := Reconcile(root, run, []Input{same}, Options{}); err != nil {
		t.Fatal(err)
	}
	records, err := readJSONL(findingsPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("the recurring finding must reuse its overridden record, got %d records", len(records))
	}
	if records[0].Status != StatusOverridden {
		t.Fatalf("status = %q, want the override to persist", records[0].Status)
	}
}

func TestOverridesAreRenderedInTheIndex(t *testing.T) {
	root := t.TempDir()
	seedRecord(t, root, openBlocker("mrvf-1"))
	if err := RequestOverride(root, "mrvf-1", OverrideRequest{
		By: "orchestrator", Reason: "the reviewer was asleep and the evidence was sufficient", Now: "2026-08-27T01:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	records, err := readJSONL(findingsPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if err := RenderIndexWithRecords(root, records); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, "docs", "metareview", "FINDINGS.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, "Process Overrides") {
		t.Fatalf("FINDINGS.md must surface overrides:\n%s", text)
	}
	for _, want := range []string{"pending", "orchestrator", "the reviewer was asleep"} {
		if !strings.Contains(text, want) {
			t.Fatalf("FINDINGS.md is missing %q:\n%s", want, text)
		}
	}
}

func TestListOverrides(t *testing.T) {
	root := t.TempDir()
	seedRecord(t, root, openBlocker("mrvf-1"))
	if err := RequestOverride(root, "mrvf-1", OverrideRequest{
		By: "orchestrator", Reason: "an explanation long enough to be meaningful", Now: "t",
	}); err != nil {
		t.Fatal(err)
	}
	all, err := ListOverrides(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].Status != StatusOverridePending {
		t.Fatalf("ListOverrides = %+v", all)
	}
	pending, err := PendingOverrides(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("PendingOverrides = %+v", pending)
	}
	if err := GrantOverride(root, "mrvf-1", OverrideGrant{By: "human", Reason: "acknowledged the exception", Now: "t"}); err != nil {
		t.Fatal(err)
	}
	pending, err = PendingOverrides(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("a granted override must not stay pending: %+v", pending)
	}
}
