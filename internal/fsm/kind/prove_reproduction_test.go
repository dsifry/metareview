package kind

import (
	"context"
	"testing"

	"github.com/dsifry/metareview/internal/fsm/run"
)

// fakeExec is a gate.Exec that answers the git verbs the reproduction engine issues, driving a full
// proven path through the REAL engine without a repository — so the adapter's git closure and the
// engine→run result mapping are both exercised.
func fakeExec(t *testing.T) func(ctx context.Context, dir string, env []string, args ...string) ([]byte, []byte, int, error) {
	t.Helper()
	return func(_ context.Context, _ string, _ []string, args ...string) ([]byte, []byte, int, error) {
		switch args[0] {
		case "diff":
			return []byte("A\tpkg/x_test.go\n"), nil, 0, nil
		case "show":
			return []byte("package pkg\n"), nil, 0, nil
		default: // worktree add / remove
			return nil, nil, 0, nil
		}
	}
}

func TestReproductionProverEmpty(t *testing.T) {
	rs, err := ReproductionProver{}.ProveReproductions(context.Background(), nil, ProveSpec{})
	if err != nil || rs != nil {
		t.Fatalf("no proofs: %v %v", rs, err)
	}
}

// The adapter maps the engine's outcome onto a run.ProofResult carrying the originating proof, and
// forwards git through the injected Exec seam. A genuine assertion fail-before / pass-after is proven.
func TestReproductionProverMapsProven(t *testing.T) {
	p := reproDP("f1")
	calls := 0
	rp := ReproductionProver{
		Exec: fakeExec(t),
		Run: func(_ context.Context, _ string, _ []string) (int, string, error) {
			calls++
			if calls == 1 {
				return 1, "--- FAIL: T\n", nil
			}
			return 0, "ok\n", nil
		},
	}
	spec := ProveSpec{Dir: t.TempDir(), TestCmd: []string{"go", "test", "./..."}, PreFixSHA: "pre", PostFixSHA: "post"}
	rs, err := rp.ProveReproductions(context.Background(), []run.DifferentialProof{p}, spec)
	if err != nil {
		t.Fatalf("ProveReproductions: %v", err)
	}
	if len(rs) != 1 || !rs[0].Proven || rs[0].Outcome != run.PinProven || rs[0].Proof.ID != p.ID {
		t.Fatalf("a proven reproduction must map onto its proof: %+v", rs)
	}
}

// A misconfiguration (no test command) surfaces as an error the node then aborts on, not a silent
// pass — the engine's own guard, propagated through the adapter.
func TestReproductionProverPropagatesEngineError(t *testing.T) {
	rp := ReproductionProver{Exec: fakeExec(t)}
	spec := ProveSpec{Dir: t.TempDir(), PreFixSHA: "pre", PostFixSHA: "post"} // no TestCmd
	if _, err := rp.ProveReproductions(context.Background(), []run.DifferentialProof{reproDP("f1")}, spec); err == nil {
		t.Fatal("a missing test command must surface as an error")
	}
}

// Provers routes each kind to its own engine: pins to the mutation prover, reproductions to the
// reproduction prover.
func TestProversRoutesByKind(t *testing.T) {
	pinRun := func(_ context.Context, _ string, argv []string) (int, string, error) {
		return 1, "FAIL", nil // any non-zero so the pin path completes
	}
	reproCalls := 0
	p := Provers{
		Mutation: MutationProver{Run: pinRun},
		Reproduction: ReproductionProver{
			Exec: fakeExec(t),
			Run: func(_ context.Context, _ string, _ []string) (int, string, error) {
				reproCalls++
				if reproCalls == 1 {
					return 1, "--- FAIL: T\n", nil
				}
				return 0, "ok\n", nil
			},
		},
	}
	spec := ProveSpec{Dir: t.TempDir(), TestCmd: []string{"go", "test", "./..."}, PreFixSHA: "pre", PostFixSHA: "post"}

	// A pin flows to the mutation prover: the pin's baseline is checked and the result comes back.
	pin := pinDP("f1", "addedLine")
	if _, err := p.ProvePins(context.Background(), []run.DifferentialProof{pin}, spec); err != nil {
		t.Fatalf("ProvePins: %v", err)
	}
	// A reproduction flows to the reproduction prover and is proven end to end.
	rs, err := p.ProveReproductions(context.Background(), []run.DifferentialProof{reproDP("f2")}, spec)
	if err != nil {
		t.Fatalf("ProveReproductions: %v", err)
	}
	if len(rs) != 1 || !rs[0].Proven {
		t.Fatalf("the reproduction must be proven through the composite: %+v", rs)
	}
	if reproCalls == 0 {
		t.Fatal("the reproduction prover must have run")
	}
}
