package kind

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/dsifry/metareview/internal/fsm/errs"
	"github.com/dsifry/metareview/internal/fsm/judge"
	"github.com/dsifry/metareview/internal/fsm/machine"
	"github.com/dsifry/metareview/internal/fsm/run"
	"github.com/dsifry/metareview/internal/fsm/workflow"
)

// Prove is the mutation-verify node kind (spec §3.1 / §9.1): the only deterministic non-gate node.
// It checks the differential proofs a fix declared and emits one ProofResult per proof plus a
// structural finding for every proof that did not prove, which the pins_proven gate selects on.
const Prove = "prove"

func errNoProver() error {
	return errs.E(machine.CodeExecutorFailed, "this registry has no prover", "reason", "no_prover")
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

func (e *proveExec) Execute(ctx context.Context, in machine.ExecInput) (json.RawMessage, error) {
	if e.prover == nil {
		return nil, errNoProver()
	}
	var toProve, toReproduce []run.DifferentialProof
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
		default:
			// Deletion (T1.5) and any unknown kind have no engine here. Fail closed: unverifiable
			// blocks, it never silently passes.
			results = append(results, proofResult(proof, run.PinUnverifiable, "differential proving for kind "+proof.Kind+" is not available in this build"))
		}
	}
	dir := proveDir(in.Snap)
	spec := ProveSpec{TestCmd: testCmdParam(in.Snap, in.Node), Dir: dir, PreFixSHA: in.Snap.FixEntryHead, PostFixSHA: in.Snap.Head}
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

	delta := run.Delta{PinResults: results}
	// A finding is settled iff a Proven proof cleared it; a finding whose only supplied proofs were
	// non-Proven is NOT settled — a malformed pin (advisory) in particular must not let it escape
	// clause (a). Track findings already carrying a BLOCKING supplied marker so clause (a) does not
	// duplicate one, and settled (Proven) findings so it skips them.
	settled, blocked := map[string]bool{}, map[string]bool{}
	for _, r := range results {
		if r.Proven {
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
		if owesPin(in.Diff.Text, dir, b.File) {
			delta.Findings = append(delta.Findings, owedPinMarker(b))
		}
	}
	raw := json.RawMessage(run.MarshalCanonical(delta))
	if _, err := (proveKind{}).Decode(raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// proveDir is the directory the prover runs in: the run's work dir, falling back to the repo root.
func proveDir(s run.Snapshot) string {
	if s.WorkDir != "" {
		return s.WorkDir
	}
	return s.RepoRoot
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
func owesPin(diff, dir, file string) bool {
	return file != "" && len(judge.AddedLinesInFile(diff, file)) > 0 && packageHasTests(dir, file)
}

// packageHasTests reports whether the directory holding file contains Go test files (*_test.go). It
// is the machine-determinable "package that has test files" of §4.2; a non-Go partition would key on
// its own test convention here.
func packageHasTests(dir, file string) bool {
	matches, err := filepath.Glob(filepath.Join(dir, filepath.Dir(file), "*_test.go"))
	return err == nil && len(matches) > 0
}

// owedPinMarker is the blocking finding for a confirmed finding that owes a pin the fix never
// supplied. Like proofFinding it selects on structural provenance, never issue text.
func owedPinMarker(b run.Bug) run.Finding {
	return run.Finding{IssueText: "prove: confirmed finding owes a pin but the fix supplied none", File: b.File, Severity: "high", Category: run.CategoryUnprovenFix, Source: run.SourceMutationVerify}
}

// proofFinding turns an unproven ProofResult into the structural marker the pins_proven gate selects
// on: Source is mutation-verify and Category encodes the outcome (never issue text). Severity mirrors
// the gate's blocking rule — high for a blocking outcome, medium for the advisory malformed pin.
func proofFinding(r run.ProofResult) run.Finding {
	category, severity := run.CategoryUnverifiable, "high"
	switch r.Outcome {
	case run.PinSurvived:
		category, severity = run.CategoryUnprovenFix, "high"
	case run.PinMalformed:
		category, severity = run.CategoryMalformedPin, "medium"
	}
	file := ""
	if r.Proof.Pin != nil {
		file = r.Proof.Pin.File
	}
	text := fmt.Sprintf("prove: %s for finding %s", r.Outcome, r.Proof.Finding)
	return run.Finding{IssueText: text, File: file, Severity: severity, Category: category, Source: run.SourceMutationVerify}
}
