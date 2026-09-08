package findings

import (
	"errors"
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

func TestGrantRequiresAnActor(t *testing.T) {
	root := t.TempDir()
	seedRecord(t, root, openBlocker("mrvf-1"))
	if err := GrantOverride(root, "mrvf-1", OverrideGrant{Reason: "a perfectly good explanation", Now: "t"}); err == nil {
		t.Fatal("a grant with no actor must be refused")
	}
	if loadOne(t, root).Status != "open" {
		t.Fatal("the refused grant must not change the record")
	}
}

func TestGrantOnAnAlreadyOverriddenFindingIsRefused(t *testing.T) {
	root := t.TempDir()
	seedRecord(t, root, openBlocker("mrvf-1"))
	grant := OverrideGrant{By: "human", Reason: "acknowledged the exception once", Now: "t"}
	if err := GrantOverride(root, "mrvf-1", grant); err != nil {
		t.Fatal(err)
	}
	if err := GrantOverride(root, "mrvf-1", grant); err == nil {
		t.Fatal("an overridden finding must not be overridden again")
	}
}

func TestOverridesAreOrderedNewestFirst(t *testing.T) {
	root := t.TempDir()
	first, second := openBlocker("mrvf-1"), openBlocker("mrvf-2")
	second.Fingerprint = "pr:architecture:other"
	if err := writeJSONL(findingsPath(root), []Record{first, second}); err != nil {
		t.Fatal(err)
	}
	if err := RequestOverride(root, "mrvf-1", OverrideRequest{
		By: "orchestrator", Reason: "the earlier of two exceptions", Now: "2026-08-27T01:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	if err := RequestOverride(root, "mrvf-2", OverrideRequest{
		By: "orchestrator", Reason: "the later of two exceptions", Now: "2026-08-27T09:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	all, err := ListOverrides(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].ID != "mrvf-2" {
		t.Fatalf("overrides must be newest first, got %+v", all)
	}
}

func TestOverrideReadErrorsSurface(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".metareview"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(findingsPath(root), []byte("{not json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ListOverrides(root); err == nil {
		t.Fatal("ListOverrides must surface an unreadable findings file")
	}
	if _, err := PendingOverrides(root); err == nil {
		t.Fatal("PendingOverrides must surface an unreadable findings file")
	}
	if err := RequestOverride(root, "mrvf-1", OverrideRequest{By: "a", Reason: "a good enough reason here", Now: "t"}); err == nil {
		t.Fatal("RequestOverride must surface an unreadable findings file")
	}
}

func TestOverrideWriteErrorSurfaces(t *testing.T) {
	root := t.TempDir()
	seedRecord(t, root, openBlocker("mrvf-1"))
	restore := saveRecords
	saveRecords = func(string, []Record) error { return errors.New("disk full") }
	t.Cleanup(func() { saveRecords = restore })

	err := RequestOverride(root, "mrvf-1", OverrideRequest{
		By: "orchestrator", Reason: "an explanation of the exception", Now: "t",
	})
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("err = %v, want the injected write failure", err)
	}
}

func TestOverrideLoadErrorSurfacesThroughTheSeam(t *testing.T) {
	root := t.TempDir()
	seedRecord(t, root, openBlocker("mrvf-1"))
	restore := loadRecords
	loadRecords = func(string) ([]Record, error) { return nil, errors.New("io boom") }
	t.Cleanup(func() { loadRecords = restore })

	if _, err := ListOverrides(root); err == nil || !strings.Contains(err.Error(), "io boom") {
		t.Fatalf("ListOverrides err = %v, want the injected failure", err)
	}
	if err := GrantOverride(root, "mrvf-1", OverrideGrant{
		By: "human", Reason: "acknowledged the exception", Now: "t",
	}); err == nil || !strings.Contains(err.Error(), "io boom") {
		t.Fatalf("GrantOverride err = %v, want the injected failure", err)
	}
}

// An override without a timestamp is not an audit record. Both halves must
// refuse one, and must leave the finding untouched when they do.
func TestOverrideRequiresATimestamp(t *testing.T) {
	root := t.TempDir()
	seedRecord(t, root, openBlocker("mrvf-1"))
	if err := RequestOverride(root, "mrvf-1", OverrideRequest{
		By: "orchestrator", Reason: "a perfectly good explanation of the exception", Now: "  ",
	}); err == nil {
		t.Fatal("a request with no timestamp must be refused")
	}
	if err := GrantOverride(root, "mrvf-1", OverrideGrant{
		By: "human", Reason: "acknowledged the exception here", Now: "",
	}); err == nil {
		t.Fatal("a grant with no timestamp must be refused")
	}
	if loadOne(t, root).Status != "open" {
		t.Fatal("a refused override must not change the record")
	}
}

// Timestamps are stored normalized so ordering by UpdatedAt stays reliable.
func TestOverrideTimestampsAreTrimmed(t *testing.T) {
	root := t.TempDir()
	seedRecord(t, root, openBlocker("mrvf-1"))
	if err := RequestOverride(root, "mrvf-1", OverrideRequest{
		By: "orchestrator", Reason: "an explanation long enough to be meaningful", Now: " 2026-08-27T01:00:00Z ",
	}); err != nil {
		t.Fatal(err)
	}
	if got := loadOne(t, root); got.OverrideRequestedAt != "2026-08-27T01:00:00Z" || got.UpdatedAt != "2026-08-27T01:00:00Z" {
		t.Fatalf("request timestamps were not normalized: %+v", got)
	}
	if err := GrantOverride(root, "mrvf-1", OverrideGrant{
		By: "human", Reason: "acknowledged the exception here", Now: " 2026-08-27T02:00:00Z ",
	}); err != nil {
		t.Fatal(err)
	}
	if got := loadOne(t, root); got.OverrideGrantedAt != "2026-08-27T02:00:00Z" || got.UpdatedAt != "2026-08-27T02:00:00Z" {
		t.Fatalf("grant timestamps were not normalized: %+v", got)
	}
}

// The separation of request from grant is the whole point: the actor who
// escalated cannot also acknowledge it. (`--by` remains audit metadata, not
// authentication — this closes the accidental case, not an impersonating one.)
func TestTheRequesterCannotGrantItsOwnOverride(t *testing.T) {
	root := t.TempDir()
	seedRecord(t, root, openBlocker("mrvf-1"))
	if err := RequestOverride(root, "mrvf-1", OverrideRequest{
		By: "orchestrator", Reason: "chain exhausted; recording the exception", Now: "2026-08-27T01:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	if err := GrantOverride(root, "mrvf-1", OverrideGrant{
		By: " Orchestrator ", Reason: "acknowledging my own escalation", Now: "2026-08-27T02:00:00Z",
	}); err == nil {
		t.Fatal("the requesting actor must not be able to grant its own override")
	}
	if loadOne(t, root).Status != StatusOverridePending {
		t.Fatal("the refused grant must leave the override pending and blocking")
	}
	if err := GrantOverride(root, "mrvf-1", OverrideGrant{
		By: "dsifry@warmstart.ai", Reason: "acknowledged from outside the workflow", Now: "2026-08-27T03:00:00Z",
	}); err != nil {
		t.Fatalf("a different actor must still be able to grant: %v", err)
	}
}

// FINDINGS.md is the durable audit surface: it must show both timestamps and the
// escalation context, not just actors and reasons.
func TestTheIndexRendersFullOverrideProvenance(t *testing.T) {
	root := t.TempDir()
	seedRecord(t, root, openBlocker("mrvf-1"))
	if err := RequestOverride(root, "mrvf-1", OverrideRequest{
		By:         "orchestrator",
		Reason:     "the review chain exhausted its attempts overnight",
		Escalation: "artifact review chain exhausted (attempt 3 of 3)",
		Now:        "2026-08-27T01:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	render := func() string {
		t.Helper()
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
		return string(body)
	}
	pending := render()
	for _, want := range []string{"2026-08-27T01:00:00Z", "attempt 3 of 3", "orchestrator"} {
		if !strings.Contains(pending, want) {
			t.Fatalf("the pending override is missing %q:\n%s", want, pending)
		}
	}
	if err := GrantOverride(root, "mrvf-1", OverrideGrant{
		By: "dsifry@warmstart.ai", Reason: "reviewed the evidence and accept it", Now: "2026-08-27T02:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	granted := render()
	for _, want := range []string{"2026-08-27T02:00:00Z", "2026-08-27T01:00:00Z", "attempt 3 of 3", "dsifry@warmstart.ai", "orchestrator"} {
		if !strings.Contains(granted, want) {
			t.Fatalf("the granted override is missing %q:\n%s", want, granted)
		}
	}
}

// A grant with no prior request renders without an empty requester clause.
func TestTheIndexRendersADirectGrantWithoutARequester(t *testing.T) {
	root := t.TempDir()
	seedRecord(t, root, openBlocker("mrvf-1"))
	if err := GrantOverride(root, "mrvf-1", OverrideGrant{
		By: "dsifry@warmstart.ai", Reason: "accepted directly; no agent escalation", Now: "2026-08-27T02:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	records, err := readJSONL(findingsPath(root))
	if err != nil {
		t.Fatal(err)
	}
	lines := overrideLines(records)
	if len(lines) != 1 {
		t.Fatalf("want one rendered override, got %v", lines)
	}
	if strings.Contains(lines[0], "requested by") || strings.Contains(lines[0], "escalation:") {
		t.Fatalf("a direct grant must not render an empty requester or escalation: %q", lines[0])
	}
}

// A finding whose override was requested and which is then genuinely fixed must
// leave override-pending. Reconcile's fix transition only matched status "open",
// so once RequestOverride moved a finding to override-pending nothing could move
// it again: it never became "fixed", Blocks stayed true, and `override list
// --pending` exited 1 forever with no command able to clear it. The CLI offers
// request|grant|list and no withdraw, so CI stayed red permanently.
func TestReconcileClosesAPendingOverrideThatWasActuallyFixed(t *testing.T) {
	root := t.TempDir()
	run := Run{ID: "mrv-run-1", Scope: "pr-ready", Target: map[string]string{"branch": "b"}, GitHead: "head1"}
	input := Input{Fingerprint: "pr:security:eval", Title: "Unsafe eval introduced", Severity: "critical", Classification: "blocking"}

	if _, err := Reconcile(root, run, []Input{input}, Options{}); err != nil {
		t.Fatal(err)
	}
	recs, err := readJSONL(findingsPath(root))
	if err != nil || len(recs) != 1 {
		t.Fatalf("seed: %v %d", err, len(recs))
	}
	id := recs[0].ID

	if err := RequestOverride(root, id, OverrideRequest{By: "tester", Reason: "escalated out of the workflow deliberately", Now: "2026-08-28T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if pending, err := PendingOverrides(root); err != nil || len(pending) != 1 {
		t.Fatalf("expected one pending override: %v %d", err, len(pending))
	}

	// The next run genuinely fixes it: the fingerprint is no longer found.
	next := Run{ID: "mrv-run-2", Scope: "pr-ready", Target: run.Target, GitHead: "head2"}
	if _, err := Reconcile(root, next, nil, Options{PreviousRunID: run.ID}); err != nil {
		t.Fatal(err)
	}

	recs, err = readJSONL(findingsPath(root))
	if err != nil || len(recs) != 1 {
		t.Fatalf("load: %v %d", err, len(recs))
	}
	got := recs[0]
	if Blocks(got.Status) {
		t.Fatalf("a fixed finding still blocks: status %q", got.Status)
	}
	if got.Status != "fixed" {
		t.Fatalf("status %q, want fixed", got.Status)
	}
	if got.FixedInRunID != next.ID {
		t.Fatalf("FixedInRunID %q, want %q", got.FixedInRunID, next.ID)
	}
	pending, err := PendingOverrides(root)
	if err != nil || len(pending) != 0 {
		t.Fatalf("override list --pending must be empty once the finding is fixed: %v %+v", err, pending)
	}
}

// Issue #147 review: an escalation can persist after its findings are fixed (the run
// -level hard stop outlives the finding-level fix). Lifting it is a human decision, so
// GrantOverride accepts a terminal-status (fixed) finding — the grant records the
// decision; the two-phase requester≠grantor rule still applies where a request exists.
func TestGrantOverrideAcceptsAFixedFinding(t *testing.T) {
	root := t.TempDir()
	one := Record{ID: "mrvf-esc-1", Scope: "pr-ready", Status: "fixed", Classification: "blocking", Severity: "high", FixedInRunID: "mrv-run-9"}
	if err := writeJSONL(filepath.Join(root, ".metareview", "findings.jsonl"), []Record{one}); err != nil {
		t.Fatal(err)
	}
	// two-phase: the agent files the request (with the escalation reference), the human
	// grants — requester ≠ grantor enforced by GrantOverride itself
	if err := RequestOverride(root, "mrvf-esc-1", OverrideRequest{
		By: "agent", Reason: "the escalation was marker churn; requesting the human lift", Now: "2026-09-08T00:00:00Z", Escalation: "mrv-20260907-234001",
	}); err != nil {
		t.Fatalf("RequestOverride: %v", err)
	}
	if err := GrantOverride(root, "mrvf-esc-1", OverrideGrant{By: "dsifry", Reason: "escalation was marker churn; the human decision is recorded", Now: "2026-09-08T00:00:01Z"}); err != nil {
		t.Fatalf("GrantOverride on a requested fixed finding: %v", err)
	}
	records, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if records[0].Status != StatusOverridden || records[0].OverrideGrantedBy != "dsifry" {
		t.Fatalf("status = %s, grantor = %q; want overridden by dsifry", records[0].Status, records[0].OverrideGrantedBy)
	}
	// a DIRECT grant on a fixed finding with no filed request is refused — the two-phase
	// record is what makes the escalation lift accountable
	two := Record{ID: "mrvf-esc-2", Scope: "pr-ready", Status: "fixed", Classification: "blocking", Severity: "high"}
	if err := writeJSONL(filepath.Join(root, ".metareview", "findings.jsonl"), []Record{one, two}); err != nil {
		t.Fatal(err)
	}
	if err := GrantOverride(root, "mrvf-esc-2", OverrideGrant{By: "dsifry", Reason: "direct grant without a filed request", Now: "2026-09-08T00:00:02Z"}); err == nil {
		t.Fatal("a direct grant on a fixed finding without a filed request must be refused")
	}
}

// Issue #147 review: the two-phase path must EXIST for a fixed finding attached to an
// escalated run — without it, only the single-actor direct grant could lift an
// escalation. RequestOverride accepts a fixed finding when the request references the
// escalation; the grant then enforces requester ≠ grantor.
func TestRequestOverrideAcceptsAFixedFindingWithAnEscalationReference(t *testing.T) {
	root := t.TempDir()
	one := Record{ID: "mrvf-esc-9", Scope: "pr-ready", Status: "fixed", Classification: "blocking", Severity: "high", FixedInRunID: "mrv-run-9"}
	if err := writeJSONL(filepath.Join(root, ".metareview", "findings.jsonl"), []Record{one}); err != nil {
		t.Fatal(err)
	}
	if err := RequestOverride(root, "mrvf-esc-9", OverrideRequest{
		By: "agent", Reason: "the escalation was marker churn; requesting the human lift", Now: "2026-09-08T00:00:00Z", Escalation: "mrv-20260907-234001",
	}); err != nil {
		t.Fatalf("RequestOverride on a fixed finding with an escalation reference: %v", err)
	}
	records, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if records[0].Status != StatusOverridePending || records[0].OverrideEscalation != "mrv-20260907-234001" {
		t.Fatalf("status = %s, escalation = %q; want pending with the escalation recorded", records[0].Status, records[0].OverrideEscalation)
	}
	// without the escalation reference, a fixed finding is still refused
	two := Record{ID: "mrvf-esc-10", Scope: "pr-ready", Status: "fixed", Classification: "blocking", Severity: "high"}
	if err := writeJSONL(filepath.Join(root, ".metareview", "findings.jsonl"), []Record{one, two}); err != nil {
		t.Fatal(err)
	}
	if err := RequestOverride(root, "mrvf-esc-10", OverrideRequest{By: "agent", Reason: "no escalation reference", Now: "2026-09-08T00:00:01Z"}); err == nil {
		t.Fatal("a fixed finding without an escalation reference must stay refused")
	}
}
