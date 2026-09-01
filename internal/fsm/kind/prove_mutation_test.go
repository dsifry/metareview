package kind

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/fsm/run"
)

func pinProofFile(finding, file, from, to string) run.DifferentialProof {
	return run.DifferentialProof{ID: "p-" + finding, Finding: finding, Kind: run.ProofPin, Pin: &run.Pin{ID: "p-" + finding, Finding: finding, File: file, From: from, To: to, Test: "T"}}
}

// The production prover maps each mutation outcome back onto its originating proof: a mutation that
// breaks the tests is Proven; one the tests still pass is survived. Driven through a stubbed command
// seam (no real processes) over a real fixture tree so the Verifier's own file work still runs.
func TestMutationProverMapsOutcomes(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "calc.go"), []byte("package x\n\nfunc F() int { return 10 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	proof := pinProofFile("f1", "calc.go", "return 10", "return 11")
	spec := ProveSpec{Dir: dir, TestCmd: []string{"go", "test"}, BuildCmd: []string{"true"}}

	// Tests fail exactly when the mutated line ("return 11") is present → Proven.
	provenRun := func(_ context.Context, d string, argv []string) (int, string, error) {
		if argv[0] == "true" { // the build check compiles
			return 0, "", nil
		}
		body, _ := os.ReadFile(filepath.Join(d, "calc.go"))
		if strings.Contains(string(body), "return 11") {
			return 1, "assertion failed", nil // mutated → tests fail
		}
		return 0, "", nil // original/restored → tests pass
	}
	rs, err := (MutationProver{Run: provenRun}).ProvePins(context.Background(), []run.DifferentialProof{proof}, spec)
	if err != nil || len(rs) != 1 {
		t.Fatalf("prove: %v %+v", err, rs)
	}
	if !rs[0].Proven || rs[0].Outcome != run.PinProven || rs[0].Proof.Finding != "f1" {
		t.Fatalf("a breaking mutation must map to a Proven result carrying its proof: %+v", rs[0])
	}

	// Tests always pass → the mutation survived (a test gap).
	survivedRun := func(context.Context, string, []string) (int, string, error) { return 0, "", nil }
	rs, err = (MutationProver{Run: survivedRun}).ProvePins(context.Background(), []run.DifferentialProof{proof}, spec)
	if err != nil || len(rs) != 1 || rs[0].Proven || rs[0].Outcome != run.PinSurvived {
		t.Fatalf("a mutation the tests do not catch must map to survived: %v %+v", err, rs)
	}
}

// No proofs is a no-op (nothing to prove), and a misconfigured run (a pin but no test command)
// surfaces the Verifier's error rather than reporting false blockers.
func TestMutationProverEdges(t *testing.T) {
	rs, err := (MutationProver{}).ProvePins(context.Background(), nil, ProveSpec{})
	if err != nil || rs != nil {
		t.Fatalf("no proofs must be a no-op: %v %+v", err, rs)
	}
	proof := pinProofFile("f1", "a.go", "x", "y")
	if _, err := (MutationProver{}).ProvePins(context.Background(), []run.DifferentialProof{proof}, ProveSpec{Dir: t.TempDir()}); err == nil {
		t.Fatal("a pin with no test command must surface an error, not a false pass")
	}
}
