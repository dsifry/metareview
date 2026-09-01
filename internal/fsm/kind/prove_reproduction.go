package kind

import (
	"context"
	"strings"

	"github.com/dsifry/metareview/internal/fsm/gate"
	"github.com/dsifry/metareview/internal/fsm/judge"
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

// gitFn is a repo-bound git invocation returning stdout, the exit code, and a spawn error.
type gitFn func(ctx context.Context, args ...string) (string, int, error)

// engine builds the reproduction engine and its repo-bound git seam for a spec.
func (rp ReproductionProver) engine(spec ProveSpec) (mutation.Reproducer, gitFn) {
	git := gitFn(func(ctx context.Context, args ...string) (string, int, error) {
		stdout, _, code, err := rp.Exec(ctx, spec.Dir, nil, args...)
		return string(stdout), code, err
	})
	return mutation.Reproducer{
		Dir: spec.Dir, PreFixSHA: spec.PreFixSHA, PostFixSHA: spec.PostFixSHA,
		TestCmd: spec.TestCmd, Timeout: spec.Timeout, Git: git, Run: rp.Run,
	}, git
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
	r, _ := rp.engine(spec)
	rrs, err := r.Reproduce(ctx, reproofs)
	if err != nil {
		return nil, err
	}
	out := make([]run.ProofResult, len(rrs))
	for i, rr := range rrs {
		out[i] = run.ProofResult{Proof: proofs[i], Proven: rr.Proven, Outcome: run.PinOutcome(rr.Outcome), Detail: rr.Detail, FailBefore: rr.FailBefore}
	}
	return out, nil
}

// ProveDeletions verifies each deletion proof (spec §9.4): the DeletionRef↔reviewed-diff binding, then
// the fail-before/pass-after reproduction (reusing the engine — the deletion is materialized when the
// post-fix tree is applied), then the whole-suite over-deletion scope check. All three must hold.
func (rp ReproductionProver) ProveDeletions(ctx context.Context, proofs []run.DifferentialProof, spec ProveSpec) ([]run.ProofResult, error) {
	if len(proofs) == 0 {
		return nil, nil
	}
	r, git := rp.engine(spec)
	out := make([]run.ProofResult, len(proofs))
	for i, p := range proofs {
		out[i] = rp.proveDeletion(ctx, r, git, p, spec)
	}
	return out, nil
}

func (rp ReproductionProver) proveDeletion(ctx context.Context, r mutation.Reproducer, git gitFn, p run.DifferentialProof, spec ProveSpec) run.ProofResult {
	res := func(o run.PinOutcome, detail string) run.ProofResult {
		return run.ProofResult{Proof: p, Proven: false, Outcome: o, Detail: detail}
	}
	del := p.Deletes // non-nil: the one-of invariant is enforced at decode
	// (a) DeletionRef↔reviewed-diff binding. The fix commit's own diff must remove exactly the span
	//     from its file, anchored at the fix's parent (the pre-fix commit). A free-floating or
	//     mismatched span is malformed (the CLAIM is bad), never a silent pass.
	if del.File == "" || del.Removed == "" {
		return res(run.PinMalformed, "the deletion names no file or no removed span")
	}
	if spec.PreFixSHA == "" || spec.PostFixSHA == "" {
		return res(run.PinUnverifiable, "no pre/post-fix anchor to verify the deletion against")
	}
	if del.ParentSHA != spec.PreFixSHA {
		return res(run.PinMalformed, "the DeletionRef ParentSHA is not the fix's parent (pre-fix) commit")
	}
	// --no-ext-diff/--no-textconv: gate.RealExec disables the external diff driver by emptying
	// GIT_EXTERNAL_DIFF, which makes a TEXTUAL `git diff` die ("cannot run") unless the driver is
	// turned off here too (the reproduction's --name-status sidesteps it; this textual read cannot).
	diff, code, err := git(ctx, "diff", "--no-color", "--no-ext-diff", "--no-textconv", "--end-of-options", spec.PreFixSHA, spec.PostFixSHA, "--", del.File)
	if err != nil || code != 0 {
		return res(run.PinUnverifiable, "could not read the fix's diff for the deleted file")
	}
	if !spanIsRemoved(judge.RemovedLinesInFile(diff, del.File), del.Removed) {
		return res(run.PinMalformed, "the reviewed diff does not remove the DeletionRef span from its file")
	}
	// (b) fail-before/pass-after reproduction — reuse the engine; the deletion is materialized when the
	//     post-fix tree is applied (step d removes the deleted file).
	rrs, err := r.Reproduce(ctx, []mutation.Proof{{Test: p.Test}})
	if err != nil {
		return res(run.PinUnverifiable, "the reproduction check could not run: "+err.Error())
	}
	rr := rrs[0]
	if !rr.Proven {
		return run.ProofResult{Proof: p, Proven: false, Outcome: run.PinOutcome(rr.Outcome), Detail: rr.Detail, FailBefore: rr.FailBefore}
	}
	// (c) whole-suite over-deletion scope check — the deletion must break no EXISTING test.
	scope := r.ScopeSuite(ctx)
	if scope.Outcome != mutation.PinProven {
		return run.ProofResult{Proof: p, Proven: false, Outcome: run.PinOutcome(scope.Outcome), Detail: scope.Detail, FailBefore: rr.FailBefore}
	}
	return run.ProofResult{Proof: p, Proven: true, Outcome: run.PinProven, Detail: rr.Detail, FailBefore: rr.FailBefore}
}

// spanIsRemoved reports whether every non-empty line of span appears among the diff's removed lines
// (and span has at least one such line). It is the deterministic half of the deletion binding: the
// reviewed change actually deletes this span from its file.
func spanIsRemoved(removed []string, span string) bool {
	set := make(map[string]bool, len(removed))
	for _, l := range removed {
		set[l] = true
	}
	found := false
	for _, l := range strings.Split(span, "\n") {
		if l == "" {
			continue
		}
		found = true
		if !set[l] {
			return false
		}
	}
	return found
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

func (p Provers) ProveDeletions(ctx context.Context, proofs []run.DifferentialProof, spec ProveSpec) ([]run.ProofResult, error) {
	return p.Reproduction.ProveDeletions(ctx, proofs, spec)
}
