package prready

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dsifry/metareview/internal/findings"
)

// TestGrantedOverrideRetiresABlockerKnownOnlyFromACommittedLog is #188: a stale task-done blocker that exists only in a
// committed review log (no ledger row on this clone) blocks pr-ready, and the documented escalation path reaches it. A
// request imports the finding and keeps it blocking; the grant from outside the workflow clears the gate.
func TestGrantedOverrideRetiresABlockerKnownOnlyFromACommittedLog(t *testing.T) {
	t.Setenv("METAREVIEW_ALLOW_MECHANICAL_PASS", "1")
	evidence := filepath.Join(t.TempDir(), "evidence.md")
	if err := os.WriteFile(evidence, []byte("go test ./... exited 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root, a, _ := taskReviewRepo(t)
	runID := "mrv-20260905-223221349809000-task-done-help-9a8265a5"
	writeTaskDoneLog(t, root, runID, "help", a, "main", "seed.txt")
	id := "mrvf-20260905-223221349809000-task-done-help-9a8265a5-001"
	now := time.Date(2026, 9, 29, 7, 0, 0, 0, time.UTC)
	blocked := func() bool {
		t.Helper()
		result, err := Create(root, Options{Base: "main", EvidencePath: evidence, Now: now})
		if err != nil {
			t.Fatal(err)
		}
		body, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(result.ReviewRel)))
		now = now.Add(time.Minute)
		return result.Blocking && strings.Contains(string(body), "Blocked targets: help")
	}
	if !blocked() {
		t.Fatal("the committed stale blocker must block before any override")
	}
	if err := findings.RequestOverride(root, id, findings.OverrideRequest{By: "claude-agent", Reason: "stale review of a head that no longer exists", Now: "2026-09-29T07:10:00Z"}); err != nil {
		t.Fatalf("request on a committed-only blocker: %v", err)
	}
	if !blocked() {
		t.Fatal("a requested override must keep blocking until granted")
	}
	if err := findings.GrantOverride(root, id, findings.OverrideGrant{By: "maintainer@example.com", Reason: "accepted: the reviewed head is gone", Now: "2026-09-29T07:20:00Z"}); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if blocked() {
		t.Fatal("the granted override must clear the committed blocker")
	}
}

// The P1 the #188 review found: a grant on ONE blocker of a committed-only log must not retire the log's other blocker.
// Every blocker of the log is imported with the first override, so pr-ready keeps blocking until each is resolved.
func TestGrantOnOneBlockerKeepsTheCommittedLogsOtherBlocker(t *testing.T) {
	t.Setenv("METAREVIEW_ALLOW_MECHANICAL_PASS", "1")
	evidence := filepath.Join(t.TempDir(), "evidence.md")
	if err := os.WriteFile(evidence, []byte("go test ./... exited 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root, a, _ := taskReviewRepo(t)
	runID := "mrv-20260905-223221349809000-task-done-help-9a8265a5"
	writeTaskDoneLog(t, root, runID, "help", a, "main", "seed.txt")
	logPath := filepath.Join(root, "docs", "metareview", "reviews", runID+".md")
	body, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	second := "### mrvf-20260905-223221349809000-task-done-help-9a8265a5-002: Missing validation evidence\n\n" +
		"- Reviewer: test-reviewer\n- Severity: high\n- Classification: blocking\n\n\n## Advisory Findings"
	if err := os.WriteFile(logPath, []byte(strings.Replace(string(body), "## Advisory Findings", second, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 29, 7, 0, 0, 0, time.UTC)
	blocked := func() bool {
		t.Helper()
		result, err := Create(root, Options{Base: "main", EvidencePath: evidence, Now: now})
		if err != nil {
			t.Fatal(err)
		}
		now = now.Add(time.Minute)
		return result.Blocking
	}
	for i, id := range []string{"mrvf-20260905-223221349809000-task-done-help-9a8265a5-001", "mrvf-20260905-223221349809000-task-done-help-9a8265a5-002"} {
		if err := findings.GrantOverride(root, id, findings.OverrideGrant{By: "maintainer@example.com", Reason: "accepted: the reviewed head is gone", Now: "2026-09-29T07:20:00Z"}); err != nil {
			t.Fatalf("grant %s: %v", id, err)
		}
		if got, want := blocked(), i == 0; got != want {
			t.Fatalf("after granting %d of 2 blockers: blocking=%v, want %v", i+1, got, want)
		}
	}
}
