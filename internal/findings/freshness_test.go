package findings

import (
	"strings"
	"testing"
)

const staleFP = "mutation:stale:enforce:stryker:src/a.ts:0123abcd"

func staleInput() Input {
	return Input{Reviewer: "mutation-freshness", Severity: "high", Classification: "blocking", Title: "Mutation evidence stale: src/a.ts changed",
		Evidence: []Evidence{{Type: "mutant", Path: "src/a.ts"}}, Fingerprint: staleFP}
}

func run(id string) Run {
	return Run{ID: id, Scope: "pr-ready", Target: map[string]string{"branch": "b"}, GitHead: "h-" + id}
}

func statusOf(t *testing.T, root, fingerprint string) []string {
	t.Helper()
	records, err := All(root)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range records {
		if r.Fingerprint == fingerprint {
			out = append(out, r.Status+"/"+r.FixedInRunID)
		}
	}
	return out
}

func TestIsFreshnessFingerprint(t *testing.T) {
	for fp, want := range map[string]bool{staleFP: true, "mutation:pending:advisory:stryker:0123abcd": true,
		"mutation:unattested:advisory:stryker:0123abcd": true, "mutation:survived:M:a.ts:1": false, "quality:todo": false} {
		if IsFreshnessFingerprint(fp) != want {
			t.Errorf("%s: want %v", fp, want)
		}
	}
	if freshnessEngine("mutation:stale:x") != "" || freshnessEngine(staleFP) != "stryker" {
		t.Error("engine field")
	}
}

func TestFreshEvidenceSupersedesAStaleRowNeverFixesIt(t *testing.T) {
	root := t.TempDir()
	engines := Options{MutationEngines: []string{"stryker"}}
	if _, err := Reconcile(root, run("r1"), []Input{staleInput()}, engines); err != nil {
		t.Fatal(err)
	}
	// A run without reports leaves the row (and any override) alone.
	if _, err := Reconcile(root, run("r2"), nil, Options{PreviousRunID: "r1"}); err != nil {
		t.Fatal(err)
	}
	if got := statusOf(t, root, staleFP); len(got) != 1 || got[0] != "open/" {
		t.Fatalf("a run without reports: %v", got)
	}
	// A gremlins-only run leaves stryker rows alone.
	if _, err := Reconcile(root, run("r3"), nil, Options{MutationEngines: []string{"gremlins"}}); err != nil {
		t.Fatal(err)
	}
	// Another target is untouched.
	other := run("r4")
	other.Target = map[string]string{"branch": "other"}
	if _, err := Reconcile(root, other, nil, engines); err != nil {
		t.Fatal(err)
	}
	if got := statusOf(t, root, staleFP); got[0] != "open/" {
		t.Fatalf("gremlins-only or another target: %v", got)
	}
	// Fresh stryker evidence, with a previous-run chain: superseded, fixedInRunId empty.
	res, err := Reconcile(root, run("r5"), nil, Options{PreviousRunID: "r1", MutationEngines: []string{"stryker"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := statusOf(t, root, staleFP); got[0] != StatusSuperseded+"/" || res.OpenBlockingCount != 0 {
		t.Fatalf("fresh evidence: %v (open blocking %d)", got, res.OpenBlockingCount)
	}
	// The fingerprint recurring opens a new row.
	if _, err := Reconcile(root, run("r6"), []Input{staleInput()}, engines); err != nil {
		t.Fatal(err)
	}
	if got := statusOf(t, root, staleFP); len(got) != 2 || got[1] != "open/" {
		t.Fatalf("recurrence: %v", got)
	}
}

func TestOverriddenFreshnessRowsStayAndSupersededRowsTakeEscalatedOverrides(t *testing.T) {
	root := t.TempDir()
	engines := Options{MutationEngines: []string{"stryker"}}
	res, err := Reconcile(root, run("r1"), []Input{staleInput()}, engines)
	if err != nil {
		t.Fatal(err)
	}
	id := res.NewFindings[0].ID
	if err := RequestOverride(root, id, OverrideRequest{By: "agent", Reason: "stale-only escalation", Now: "2026-09-24T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	// An override-pending row is superseded by fresh evidence, and keeps its request on the index.
	if _, err := Reconcile(root, run("r2"), nil, engines); err != nil {
		t.Fatal(err)
	}
	if got := statusOf(t, root, staleFP); got[0] != StatusSuperseded+"/" {
		t.Fatalf("override-pending: %v", got)
	}
	records, _ := All(root)
	if lines := overrideLines(records); len(lines) != 1 || !strings.Contains(lines[0], "[superseded]") {
		t.Errorf("override lines: %v", lines)
	}
	// A superseded freshness row accepts an override request only with an escalation.
	if err := RequestOverride(root, id, OverrideRequest{By: "agent", Reason: "a second request without escalation", Now: "2026-09-24T00:00:01Z"}); err == nil {
		t.Error("no escalation: refused")
	}
	if err := RequestOverride(root, id, OverrideRequest{By: "agent", Reason: "lift the stale-only escalation", Escalation: "ESCALATED run r9", Now: "2026-09-24T00:00:02Z"}); err != nil {
		t.Fatalf("with an escalation: %v", err)
	}
	if err := GrantOverride(root, id, OverrideGrant{By: "human", Reason: "accepted by the maintainer", Now: "2026-09-24T00:00:03Z"}); err != nil {
		t.Fatalf("grant: %v", err)
	}
	// An overridden row is never superseded.
	if _, err := Reconcile(root, run("r3"), nil, engines); err != nil {
		t.Fatal(err)
	}
	if got := statusOf(t, root, staleFP); got[0] != StatusOverridden+"/" {
		t.Errorf("overridden: %v", got)
	}
}

func TestOnlyStaleBlockers(t *testing.T) {
	stale := Record{Classification: "blocking", Severity: "high", Fingerprint: staleFP}
	other := Record{Classification: "blocking", Severity: "high", Fingerprint: "quality:todo"}
	advisory := Record{Classification: "advisory", Severity: "medium", Fingerprint: "x"}
	if !OnlyStaleBlockers([]Record{stale, advisory}) || OnlyStaleBlockers([]Record{stale, other}) || OnlyStaleBlockers([]Record{advisory}) {
		t.Error("true only when every blocker is stale mutation evidence and there is one")
	}
}
