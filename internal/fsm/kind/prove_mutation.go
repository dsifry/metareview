package kind

import (
	"context"

	"github.com/dsifry/metareview/internal/fsm/run"
	"github.com/dsifry/metareview/internal/mutation"
)

// MutationProver is the production Prover: it verifies each pin by mutation through
// internal/mutation.Verifier, which applies From→To inside a fresh copy of the tree, requires the
// tests to FAIL, restores, and requires them to PASS. The test command is the run's consent-hashed
// one, resolved by the prove node from the snapshot and passed in via ProveSpec — never the agent's.
type MutationProver struct {
	// Run, when set, is the command seam passed through to the Verifier; nil uses the real exec. It
	// exists only so the outcome→result mapping can be tested without spawning processes.
	Run func(ctx context.Context, dir string, argv []string) (int, string, error)
}

// ProvePins converts each pin-kind proof to a mutation.Pin, verifies them together against one tree,
// and maps each mutation.PinResult back onto its originating DifferentialProof. The mutation outcome
// vocabulary (proven/survived/malformed/unverifiable) is identical to run.PinOutcome, so it maps by
// value; Proven stays true exactly when the outcome is proven, the one thing the gate accepts.
func (mp MutationProver) ProvePins(ctx context.Context, proofs []run.DifferentialProof, spec ProveSpec) ([]run.ProofResult, error) {
	if len(proofs) == 0 {
		return nil, nil
	}
	pins := make([]mutation.Pin, len(proofs))
	for i, p := range proofs {
		pins[i] = mutation.Pin{File: p.Pin.File, From: p.Pin.From, To: p.Pin.To, Test: p.Pin.Test}
	}
	v := mutation.Verifier{Dir: spec.Dir, TestCmd: spec.TestCmd, BuildCmd: spec.BuildCmd, Timeout: spec.Timeout, Run: mp.Run}
	mrs, err := v.Verify(ctx, pins)
	if err != nil {
		return nil, err
	}
	out := make([]run.ProofResult, len(mrs))
	for i, mr := range mrs {
		out[i] = run.ProofResult{Proof: proofs[i], Proven: mr.Proven, Outcome: run.PinOutcome(mr.Outcome), Detail: mr.Detail}
	}
	return out, nil
}
