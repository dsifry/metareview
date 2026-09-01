package kind

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dsifry/metareview/internal/fsm/errs"
	"github.com/dsifry/metareview/internal/fsm/judge"
	"github.com/dsifry/metareview/internal/fsm/machine"
	"github.com/dsifry/metareview/internal/fsm/run"
	"github.com/dsifry/metareview/internal/fsm/workflow"
	"github.com/dsifry/metareview/internal/testconv"
)

// Prove is the mutation-verify node kind (spec §3.1 / §9.1): the only deterministic non-gate node.
// It checks the differential proofs a fix declared and emits one ProofResult per proof plus a
// structural finding for every proof that did not prove, which the pins_proven gate selects on.
const Prove = "prove"

func errNoProver() error {
	return errs.E(machine.CodeExecutorFailed, "this registry has no prover", "reason", "no_prover")
}

func errNoReviewer() error {
	return errs.E(machine.CodeExecutorFailed, "this registry has no symptom reviewer", "reason", "no_reviewer")
}

// ProveSpec is everything a Prover needs to run a proof against the tree, resolved per execution from
// the run's snapshot: the consent-hashed test command (never the agent's), the optional build check,
// the working directory, the timeout, and — for the reproduction form — the pre/post-fix anchors the
// engine checks out and materializes (Snapshot.FixEntryHead / Snapshot.Head).
type ProveSpec struct {
	TestCmd    []string
	BuildCmd   []string
	Dir        string
	Timeout    time.Duration
	PreFixSHA  string // Snapshot.FixEntryHead — the pre-fix tree for reproduction/deletion proofs
	PostFixSHA string // Snapshot.Head — the post-fix tree
	// Convention is the language seam (spec §4.2): the test-file predicate, the test runner and its
	// report reader, the removed-test rule, and the trivial-pin pre-screen. It is resolved from the
	// node's `test_convention` param (default "go"); an unknown name aborts the node (fail closed).
	Convention testconv.Convention
}

// Prover verifies a fix's differential proofs and returns one ProofResult per input proof, in order,
// each carrying its originating proof. It is injected so the node is testable with a mock. The two
// forms are proven by different mechanisms so they are separate methods:
//   - ProvePins mutates a line (apply From→To, the tests must FAIL; restore, they must PASS). The
//     pins handed here have already passed the added-line bind.
//   - ProveReproductions replays a committed test across the pre/post-fix trees (fail-before with an
//     assertion, pass-after) — the preferred form (spec §9.1).
type Prover interface {
	ProvePins(ctx context.Context, proofs []run.DifferentialProof, spec ProveSpec) ([]run.ProofResult, error)
	ProveReproductions(ctx context.Context, proofs []run.DifferentialProof, spec ProveSpec) ([]run.ProofResult, error)
	// ProveDeletions proves Kind:"deletion" proofs (spec §9.4): the DeletionRef↔diff binding, a
	// fail-before/pass-after reproduction, and a whole-suite over-deletion scope check.
	ProveDeletions(ctx context.Context, proofs []run.DifferentialProof, spec ProveSpec) ([]run.ProofResult, error)
}

type proveKind struct{}

func (proveKind) Name() string { return Prove }
func (proveKind) Info() workflow.KindInfo {
	return workflow.KindInfo{DefaultExec: "fork", AllowedExec: []string{"fork"}, ValidateParams: validateProve}
}

// validateProve accepts an optional test_cmd param naming the consent-hashed AllowedCmd to run.
func validateProve(p map[string]any) error {
	for k := range p {
		if k != "test_cmd" {
			return fmt.Errorf("unknown param %s", k)
		}
		if _, ok := p["test_cmd"].(string); !ok {
			return fmt.Errorf("test_cmd must be a string")
		}
	}
	return nil
}

func (proveKind) Instructions(run.Snapshot, *workflow.Node, machine.Diff, string) (machine.Instructions, error) {
	return unsupported(Prove)
}

func (proveKind) Decode(raw json.RawMessage) (any, error) {
	var d run.Delta
	if err := strictDecode(raw, &d); err != nil {
		return nil, err
	}
	if err := checkFindings(d.Findings); err != nil {
		return nil, err
	}
	if len(d.PinResults) > run.MaxDeltaList {
		return nil, invalid("cap", fmt.Sprintf("more than %d pin results", run.MaxDeltaList))
	}
	for i, r := range d.PinResults {
		if !run.ProofWithinCaps(r.Proof) || !shortOK(string(r.Outcome)) || !within(r.Detail, run.MaxDetail) {
			return nil, invalid("cap", fmt.Sprintf("pin_results[%d] exceeds a field cap", i))
		}
	}
	if err := checkPayload(d); err != nil {
		return nil, err
	}
	return d, nil
}

func (proveKind) Reduce(_ run.Snapshot, out any) (run.Delta, error) {
	return out.(run.Delta), nil
}

// proveExec is the fork executor. It reads the fix's declared proofs from the snapshot's open gaps
// (Snapshot.Unproven), checks each, and emits results plus a marker finding per unproven proof.
type proveExec struct {
	prover Prover
	// symptom is the §9.2 "fails-for-the-right-reason" reviewer (T1.4): an injected AI judge that
	// vetoes a PROVEN reproduction whose pre-fix failure is not the finding's own symptom. Veto-only —
	// it can only block, never certify. Required whenever a Proven reproduction must be reviewed; a nil
	// reviewer there is a wiring error (fail closed via errNoReviewer).
	symptom judge.Judge
}

// testCmdParam returns the AllowedCmd argv named by the node's test_cmd param, or nil.
func testCmdParam(snap run.Snapshot, node *workflow.Node) []string {
	name, _ := node.Params["test_cmd"].(string)
	if name == "" {
		return nil
	}
	for _, c := range snap.AllowedCmds {
		if c.Name == name {
			return append([]string(nil), c.Argv...)
		}
	}
	return nil
}

// conventionParam resolves the node's `test_convention` param to a language convention. An absent or
// empty param defaults to "go", so every existing workflow keeps working unchanged. An unknown name is
// a configuration error that ABORTS the node (fail closed) — never a silent fall-through to Go, which
// would score a non-Go repository by Go rules.
func conventionParam(node *workflow.Node) (testconv.Convention, error) {
	name, _ := node.Params["test_convention"].(string)
	if name == "" {
		name = "go"
	}
	conv, ok := testconv.For(name)
	if !ok {
		return nil, errs.E(machine.CodeExecutorFailed, "prove: unknown test_convention "+name, "reason", "unknown_test_convention")
	}
	return conv, nil
}

func (e *proveExec) Execute(ctx context.Context, in machine.ExecInput) (json.RawMessage, error) {
	if e.prover == nil {
		return nil, errNoProver()
	}
	var toProve, toReproduce, toDelete []run.DifferentialProof
	var results []run.ProofResult
	for _, proof := range in.Snap.Unproven {
		switch proof.Kind {
		case run.ProofPin:
			// Kind:pin guarantees a non-nil Pin — DifferentialProof decode enforces the one-of, and
			// Unproven only ever holds decoded proofs — so Pin is dereferenced without a nil check.
			// Added-line bind (spec §3.1): the pin's From must be a line the fix ADDED **in the pin's
			// own file** — a "+" line under that file's diff section — never a context/removed line, nor
			// an added line in some OTHER file. A pin that fails the bind is malformed: rewrite the pin.
			if !isAddedLineInFile(in.Diff.Text, proof.Pin.File, proof.Pin.From) {
				results = append(results, proofResult(proof, run.PinMalformed, "pin From is not a line the fix added in the pin's file"))
				continue
			}
			toProve = append(toProve, proof)
		case run.ProofReproduction:
			// The preferred form (spec §9.1): a committed test replayed across the pre/post-fix trees.
			// It carries no added-line bind — that binding is retired for reproduction and discharged by
			// the §9.2 symptom reviewer (T1.4). The engine checks out the pre-fix tree, overlays the
			// fix's test-only files, and requires an assertion fail-before / pass-after.
			toReproduce = append(toReproduce, proof)
		case run.ProofDeletion:
			// Own-file bind (spec §9.1): a deletion discharges clause (a) only when it removes code from
			// the finding's OWN file — the same own-file guarantee a pin's File gives, so the proven code
			// and the class gate's touched line stay the same code. A deletion elsewhere is malformed.
			// (This is DeletionRef.File==Finding.File, distinct from §9.5's Root.File structural gate.)
			b := confirmedBug(in.Snap.Confirmed, proof.Finding)
			if judge.NormalizePath(proof.Deletes.File) != judge.NormalizePath(b.File) {
				results = append(results, proofResult(proof, run.PinMalformed, "the deletion is not in the finding's own file"))
				continue
			}
			toDelete = append(toDelete, proof)
		default:
			// DifferentialProof decode enforces the one-of, so Unproven never holds an unknown kind. This
			// is a defensive invariant guard: if one somehow arrived, abort rather than silently skip it
			// (a skipped proof would leave its finding neither settled nor blocked — a silent pass).
			return nil, errs.E(machine.CodeExecutorFailed, "prove cannot handle proof kind "+proof.Kind, "reason", "unknown_kind")
		}
	}
	dir := proveDir(in.Snap)
	conv, err := conventionParam(in.Node)
	if err != nil {
		return nil, err
	}
	spec := ProveSpec{TestCmd: testCmdParam(in.Snap, in.Node), Dir: dir, PreFixSHA: in.Snap.FixEntryHead, PostFixSHA: in.Snap.Head, Convention: conv}
	verified, err := e.prover.ProvePins(ctx, toProve, spec)
	if err != nil {
		return nil, err
	}
	results = append(results, verified...)
	reproved, err := e.prover.ProveReproductions(ctx, toReproduce, spec)
	if err != nil {
		return nil, err
	}
	results = append(results, reproved...)
	deleted, err := e.prover.ProveDeletions(ctx, toDelete, spec)
	if err != nil {
		return nil, err
	}
	results = append(results, deleted...)

	delta := run.Delta{PinResults: results}
	// A finding is settled iff a Proven proof cleared it; a finding whose only supplied proofs were
	// non-Proven is NOT settled — a malformed pin (advisory) in particular must not let it escape
	// clause (a). Track findings already carrying a BLOCKING supplied marker so clause (a) does not
	// duplicate one, and settled (Proven) findings so it skips them.
	settled, blocked := map[string]bool{}, map[string]bool{}
	reviewIdx := in.StartIndex
	for _, r := range results {
		if r.Proven {
			// §9.2 veto: a Proven reproduction is the deterministic PASS path, but the pre-fix failure
			// must be the finding's OWN symptom. The injected reviewer can only veto (block), never
			// certify — so on a mismatch (or a fail-closed reviewer error) the finding is NOT settled and
			// a blocking marker is emitted, exactly as pins_proven treats any unproven fix.
			if r.Proof.Kind == run.ProofReproduction || r.Proof.Kind == run.ProofDeletion {
				veto, err := e.symptomVeto(ctx, in, r, reviewIdx)
				reviewIdx++
				if err != nil {
					return nil, err
				}
				if veto {
					delta.Findings = append(delta.Findings, symptomVetoMarker(r))
					blocked[r.Proof.Finding] = true
					continue
				}
			}
			settled[r.Proof.Finding] = true
			continue
		}
		f := proofFinding(r)
		delta.Findings = append(delta.Findings, f)
		if run.ProofCategoryBlocks(f.Category) {
			blocked[r.Proof.Finding] = true
		}
	}
	// pins_proven clause (a) — the non-vacuous half (spec §3.1). A fix that OWES a pin but supplied
	// none (or only a malformed, advisory one) would otherwise leave the gate green (the #24 shape).
	// For every confirmed finding that owes a pin — the fix added a line in the finding's OWN file and
	// that file's package has tests — with no Proven proof and no blocking marker yet, emit one naming
	// the file. A finding that owes no pin (cross-file remedy, or a no-test package) is exempt by
	// construction, never a silent pass.
	for _, b := range in.Snap.Confirmed {
		if settled[b.ID] || blocked[b.ID] {
			continue
		}
		if owesPin(spec.Convention, in.Diff.Text, dir, b.File) {
			delta.Findings = append(delta.Findings, owedPinMarker(b))
		}
	}
	// §9.6 test-deletion gate — mutation-kill non-regression (the always-run deterministic half). If
	// this fix DELETED a test, a green suite proves nothing: no mutant killed before the deletion may
	// survive after it. Re-verify the run's PROVEN pins on the post-deletion tree; a pin that now
	// SURVIVES means the deleted test was its detector → block. A pin whose mutated line the fix itself
	// removed is excluded (its target is legitimately gone).
	stale, err := e.testDeletionRegressions(ctx, in, spec)
	if err != nil {
		return nil, err
	}
	delta.Findings = append(delta.Findings, stale...)

	raw := json.RawMessage(run.MarshalCanonical(delta))
	if _, err := (proveKind{}).Decode(raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// testDeletionRegressions runs the §9.6 mutation-kill non-regression check. It is a no-op unless the
// fix deleted a test. Then it re-verifies every previously-PROVEN pin on the current post-deletion
// tree; a pin that now SURVIVES (the mutation still applies but the tests no longer catch it) means the
// deleted test was its sole detector — a blocking regression. The paired code+test deletion needs no
// special-case exclusion: when the pinned line was itself removed, its From no longer anchors, so the
// Verifier returns MALFORMED, not SURVIVED — and only SURVIVED counts. (An explicit "was the target
// removed" pre-filter was tried and removed: a substring match on removed lines dropped still-live pins
// like From "n < 10" against a removed "n < 100", silently bypassing the gate.)
func (e *proveExec) testDeletionRegressions(ctx context.Context, in machine.ExecInput, spec ProveSpec) ([]run.Finding, error) {
	if !spec.Convention.DeletesATest(in.Diff.Text) {
		return nil, nil
	}
	var recheck []run.DifferentialProof
	for _, p := range in.Snap.Proven {
		if p.Kind != run.ProofPin || p.Pin == nil {
			continue
		}
		recheck = append(recheck, p)
	}
	if len(recheck) == 0 {
		return nil, nil
	}
	results, err := e.prover.ProvePins(ctx, recheck, spec)
	if err != nil {
		return nil, err
	}
	var out []run.Finding
	for _, r := range results {
		if r.Outcome == run.PinSurvived {
			out = append(out, testDeletionMarker(r))
		}
	}
	return out, nil
}

// testDeletionMarker is the blocking finding for a previously-proven pin the test deletion un-killed
// (§9.6). Like the other prove markers it selects on structural provenance (Source mutation-verify +
// a blocking Category), so pins_proven reddens on it.
func testDeletionMarker(r run.ProofResult) run.Finding {
	file := ""
	if r.Proof.Pin != nil {
		file = r.Proof.Pin.File
	}
	return run.Finding{
		IssueText: fmt.Sprintf("prove: deleting a test un-killed a proven fix — pin for finding %s now survives (§9.6 mutation-kill non-regression)", r.Proof.Finding),
		File:      file,
		Severity:  "high",
		Category:  run.CategoryUnprovenFix,
		Source:    run.SourceMutationVerify,
	}
}

// proveDir is the directory the prover runs in: the run's work dir, falling back to the repo root.
func proveDir(s run.Snapshot) string {
	if s.WorkDir != "" {
		return s.WorkDir
	}
	return s.RepoRoot
}

// symptomVeto runs the §9.2 reviewer on one Proven reproduction and reports whether it must be
// vetoed. A mismatch verdict vetoes; fail-closed, so does a reviewer error/timeout/parse-failure — a
// missing veto can never become a silent pass (spec §9.2). The reviewer is required here: a nil
// reviewer with a Proven reproduction to judge is a wiring error, surfaced (never treated as approval).
func (e *proveExec) symptomVeto(ctx context.Context, in machine.ExecInput, r run.ProofResult, index int) (bool, error) {
	if e.symptom == nil {
		return false, errNoReviewer()
	}
	bug := confirmedBug(in.Snap.Confirmed, r.Proof.Finding)
	v, err := call(ctx, e.symptom, in, judge.Request{
		Kind:  judge.KindSymptom,
		Index: index,
		Input: judge.SymptomInput{Bug: bug, Test: r.Proof.Test, FailOutput: r.FailBefore},
	})
	if err != nil {
		// A CONFIGURATION error (no/invalid model, missing key/url, unsupported effort) means the
		// reviewer can never run — re-fixing cannot cure it, so a fail-closed veto would block every
		// reproduction forever. Surface it instead, like a nil reviewer. A TRANSIENT error (timeout,
		// HTTP/transport after the retry ladder) is the case §9.2 means: fail closed to a blocking veto.
		if isReviewerConfigError(err) {
			return false, err
		}
		return true, nil
	}
	// Decision true = the failure exhibits the finding's own symptom (no veto). Decision false — a
	// mismatch, or a parse failure Parse decided false — vetoes.
	return !v.Decision, nil
}

// isReviewerConfigError reports whether err is a judge misconfiguration — a model/key/url/effort
// problem that no re-fix can cure. Such an error must abort (surface the wiring bug) rather than
// fail-closed-veto forever; a transient reviewer failure is handled the other way.
func isReviewerConfigError(err error) bool {
	switch errs.Code(err) {
	case judge.CodeJudgeModel, judge.CodeJudgeKey, judge.CodeJudgeURL, judge.CodeJudgeEffortUnsupported:
		return true
	}
	return false
}

// confirmedBug returns the confirmed finding with id, or a zero Bug when none matches (the reviewer
// then judges against an empty symptom, which cannot match — fail closed).
func confirmedBug(bugs []run.Bug, id string) run.Bug {
	for _, b := range bugs {
		if b.ID == id {
			return b
		}
	}
	return run.Bug{}
}

// symptomVetoMarker is the blocking finding for a Proven reproduction the §9.2 reviewer vetoed. Like
// proofFinding it selects on structural provenance (Source mutation-verify + a blocking Category), so
// pins_proven reddens on it exactly as on any unproven fix.
func symptomVetoMarker(r run.ProofResult) run.Finding {
	return run.Finding{
		IssueText: fmt.Sprintf("prove: the reproduction for finding %s does not exhibit the finding's own symptom (§9.2 veto)", r.Proof.Finding),
		Severity:  "high",
		Category:  run.CategoryUnverifiable,
		Source:    run.SourceMutationVerify,
	}
}

// proofResult builds a ProofResult for a proof the executor itself decided (bind failure or an
// unsupported kind), never Proven.
func proofResult(proof run.DifferentialProof, outcome run.PinOutcome, detail string) run.ProofResult {
	return run.ProofResult{Proof: proof, Proven: false, Outcome: outcome, Detail: detail}
}

// isAddedLineInFile reports whether from appears within a line the fix added to file's own diff
// section (via judge.AddedLinesInFile, which keys sections on "diff --git" so a "+++"-prefixed added
// line is not mistaken for a header). The bind is to added CODE in the pin's own file, not exact-line
// equality (from is the text to replace, which lives on an added line). An empty from or file binds
// to nothing.
func isAddedLineInFile(diff, file, from string) bool {
	if from == "" || file == "" {
		return false
	}
	for _, added := range judge.AddedLinesInFile(diff, file) {
		if strings.Contains(added, from) {
			return true
		}
	}
	return false
}

// owesPin reports whether a confirmed finding in file must be backed by a proof (spec §3.1): the fix
// added a line in the finding's OWN file AND that file's package has tests. A cross-file remedy (no
// added line in the finding's file) or a no-test package owes no pin — auditable exemptions, never a
// silent pass.
func owesPin(conv testconv.Convention, diff, dir, file string) bool {
	return file != "" && len(judge.AddedLinesInFile(diff, file)) > 0 && conv.DirHasTests(dir, file)
}

// owedPinMarker is the blocking finding for a confirmed finding that owes a pin the fix never
// supplied. Like proofFinding it selects on structural provenance, never issue text.
func owedPinMarker(b run.Bug) run.Finding {
	return run.Finding{IssueText: "prove: confirmed finding owes a pin but the fix supplied none", File: b.File, Severity: "high", Category: run.CategoryUnprovenFix, Source: run.SourceMutationVerify}
}

// proofFinding turns an unproven ProofResult into the structural marker the pins_proven gate selects
// on: Source is mutation-verify and Category encodes the outcome (never issue text). Severity mirrors
// the gate's blocking rule.
//
// A malformed PIN is ADVISORY: a pin is an optional extra claim, and clause (a)'s owed-pin marker is
// the blocking backup when the fix owes one. A malformed REPRODUCTION or DELETION is BLOCKING: that
// proof IS the fix's proof (the preferred forms), and a fix adds no owed-pin backup for it — a pure
// deletion adds no line at all, so owesPin never fires. Treating a bad reproduction/deletion proof as
// merely advisory would let a fix pass with a proof that never held (the #24 vacuous-pass shape).
func proofFinding(r run.ProofResult) run.Finding {
	category, severity := run.CategoryUnverifiable, "high"
	switch r.Outcome {
	case run.PinSurvived:
		category, severity = run.CategoryUnprovenFix, "high"
	case run.PinMalformed:
		if r.Proof.Kind == run.ProofPin {
			category, severity = run.CategoryMalformedPin, "medium"
		} else {
			category, severity = run.CategoryUnprovenFix, "high"
		}
	}
	file := ""
	if r.Proof.Pin != nil {
		file = r.Proof.Pin.File
	}
	text := fmt.Sprintf("prove: %s for finding %s", r.Outcome, r.Proof.Finding)
	return run.Finding{IssueText: text, File: file, Severity: severity, Category: category, Source: run.SourceMutationVerify}
}
