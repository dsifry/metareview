package kind

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/dsifry/metareview/internal/fsm/judge"
	"github.com/dsifry/metareview/internal/fsm/machine"
	"github.com/dsifry/metareview/internal/fsm/run"
	"github.com/dsifry/metareview/internal/fsm/workflow"
)

// End to end with a REAL mutation.Verifier (via MutationProver) and a REAL `go test`: a pin that was
// proven in an earlier round is re-verified on the current tree AFTER its test has been deleted. With
// the detecting test gone, the mutation survives — the §9.6 mutation-kill non-regression gate emits a
// blocking finding. No seams mocked on the prover side.
func TestProveTestDeletionRealGit(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// The current (post-deletion) tree: production present, but the boundary test that killed the pin
	// is GONE (only a test that does not exercise the boundary remains, so the suite is green).
	write("go.mod", "module fixture\n\ngo 1.22\n")
	write("calc.go", "package fixture\n\nfunc Allow(n int) bool { return n < 10 }\n")
	write("calc_test.go", "package fixture\n\nimport \"testing\"\n\nfunc TestItRuns(t *testing.T) { _ = Allow(1) }\n")

	// The pin proven earlier: mutating `n < 10`→`n <= 10` was caught by TestBoundary (now deleted).
	pin := run.DifferentialProof{ID: "p1", Finding: "f1", Kind: run.ProofPin, Pin: &run.Pin{ID: "p1", Finding: "f1", File: "calc.go", From: "n < 10", To: "n <= 10", Test: "TestBoundary"}}
	// The fix diff for this round DELETES TestBoundary.
	diff := "diff --git a/calc_test.go b/calc_test.go\n--- a/calc_test.go\n+++ b/calc_test.go\n@@ -3,4 +3,1 @@\n-func TestBoundary(t *testing.T) {\n-\tif Allow(10) { t.Fatal(\"no\") }\n-}\n func TestItRuns(t *testing.T) { _ = Allow(1) }\n"

	r := mustNew(t, judge.NewMock(judge.Script{}), true)
	r.execs[Prove] = &proveExec{prover: Provers{Mutation: MutationProver{}}, symptom: &fakeReviewer{decision: true}}
	ex, _ := r.Executor(Prove)
	node := &workflow.Node{Name: "prove", Kind: Prove, Exec: "fork", Params: map[string]any{"test_cmd": "test"}}
	snap := run.Snapshot{
		Proven:      []run.DifferentialProof{pin},
		WorkDir:     dir,
		AllowedCmds: []run.AllowedCmd{{Name: "test", Argv: []string{"go", "test", "./..."}}},
	}
	out, err := ex.Execute(context.Background(), machine.ExecInput{Snap: snap, Node: node, Diff: machine.Diff{Text: diff}, Audit: (&audits{}).fn})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var d run.Delta
	if err := json.Unmarshal(out, &d); err != nil {
		t.Fatal(err)
	}
	if len(d.Findings) != 1 || d.Findings[0].Category != run.CategoryUnprovenFix || d.Findings[0].File != "calc.go" {
		t.Fatalf("deleting the detecting test must un-kill the pin and block: %+v", d.Findings)
	}
}
