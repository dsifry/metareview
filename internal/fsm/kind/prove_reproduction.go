package kind

import (
	"context"

	"github.com/dsifry/metareview/internal/fsm/gate"
	"github.com/dsifry/metareview/internal/fsm/run"
	"github.com/dsifry/metareview/internal/mutation"
)

// ReproductionProver is the production prover for the reproduction form: it replays each committed
// test across the pre/post-fix trees through internal/mutation.Reproducer, which checks out the
// pre-fix tree in a throwaway git worktree, overlays only the fix's test-only files, requires an
// assertion (not compile) fail-before, applies the full fix, and requires a pass-after. The four
// outcome values are identical to run.PinOutcome, so results map by value; Proven stays true exactly
// when the outcome is proven, the one thing the gate accepts.
type ReproductionProver struct {
	// Exec is the hardened git shell-out the engine runs in the repo (wiring passes d.Exec). It is
	// required in production; the run's git access arrives here because ExecInput carries no git
	// handle. Only stdout and the exit code are forwarded — a reproduction reads git output, it does
	// not need stderr.
	Exec gate.Exec
	// Run, when set, is the test-runner seam passed through to the engine; nil uses the real exec. It
	// exists only so the outcome→result mapping can be tested without spawning processes.
	Run func(ctx context.Context, dir string, argv []string) (int, string, error)
}

// ProveReproductions verifies each reproduction proof against the pre/post-fix trees named in the
// spec and maps each engine result back onto its originating DifferentialProof, in order.
func (rp ReproductionProver) ProveReproductions(ctx context.Context, proofs []run.DifferentialProof, spec ProveSpec) ([]run.ProofResult, error) {
	if len(proofs) == 0 {
		return nil, nil
	}
	reproofs := make([]mutation.Proof, len(proofs))
	for i, p := range proofs {
		reproofs[i] = mutation.Proof{Test: p.Test}
	}
	git := func(ctx context.Context, args ...string) (string, int, error) {
		stdout, _, code, err := rp.Exec(ctx, spec.Dir, nil, args...)
		return string(stdout), code, err
	}
	r := mutation.Reproducer{
		Dir: spec.Dir, PreFixSHA: spec.PreFixSHA, PostFixSHA: spec.PostFixSHA,
		TestCmd: spec.TestCmd, Timeout: spec.Timeout, Git: git, Run: rp.Run,
	}
	rrs, err := r.Reproduce(ctx, reproofs)
	if err != nil {
		return nil, err
	}
	out := make([]run.ProofResult, len(rrs))
	for i, rr := range rrs {
		out[i] = run.ProofResult{Proof: proofs[i], Proven: rr.Proven, Outcome: run.PinOutcome(rr.Outcome), Detail: rr.Detail}
	}
	return out, nil
}

// Provers is the production Prover: the mutation engine for the pin form, the reproduction engine for
// the reproduction form. The prove node holds one Prover; this composite routes each kind to the
// engine that can prove it, so the node stays a thin dispatcher.
type Provers struct {
	Mutation     MutationProver
	Reproduction ReproductionProver
}

func (p Provers) ProvePins(ctx context.Context, proofs []run.DifferentialProof, spec ProveSpec) ([]run.ProofResult, error) {
	return p.Mutation.ProvePins(ctx, proofs, spec)
}

func (p Provers) ProveReproductions(ctx context.Context, proofs []run.DifferentialProof, spec ProveSpec) ([]run.ProofResult, error) {
	return p.Reproduction.ProveReproductions(ctx, proofs, spec)
}
