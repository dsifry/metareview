package kind

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dsifry/metareview/internal/fsm/errs"
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
// the working directory, and the timeout.
type ProveSpec struct {
	TestCmd  []string
	BuildCmd []string
	Dir      string
	Timeout  time.Duration
}

// Prover verifies pin-kind differential proofs by mutation — apply From→To, the tests must FAIL;
// restore, they must PASS — and returns one ProofResult per input proof, in order, each carrying its
// originating proof. Injected so the node is testable with a mock; production wraps
// internal/mutation.Verifier. The proofs handed here have already passed the added-line bind.
type Prover interface {
	ProvePins(ctx context.Context, proofs []run.DifferentialProof, spec ProveSpec) ([]run.ProofResult, error)
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
	var toProve []run.DifferentialProof
	var results []run.ProofResult
	for _, proof := range in.Snap.Unproven {
		switch proof.Kind {
		case run.ProofPin:
			// Kind:pin guarantees a non-nil Pin — DifferentialProof decode enforces the one-of, and
			// Unproven only ever holds decoded proofs — so Pin is dereferenced without a nil check.
			// Added-line bind (spec §3.1): the pin's From must be a line the fix ADDED — a "+" line in
			// the reviewed diff — never a context or removed line, which a mutation could not attribute
			// to the fix. A pin that fails the bind is malformed: the CLAIM is bad, rewrite the pin.
			if !isAddedLine(in.Diff.Text, proof.Pin.From) {
				results = append(results, proofResult(proof, run.PinMalformed, "pin From is not a line the fix added"))
				continue
			}
			toProve = append(toProve, proof)
		default:
			// Reproduction/deletion proving is not wired in this build (the execution engine lands in a
			// follow-up). Fail closed: unverifiable blocks, it never silently passes.
			results = append(results, proofResult(proof, run.PinUnverifiable, "differential proving for kind "+proof.Kind+" is not available in this build"))
		}
	}
	verified, err := e.prover.ProvePins(ctx, toProve, ProveSpec{TestCmd: testCmdParam(in.Snap, in.Node), Dir: proveDir(in.Snap)})
	if err != nil {
		return nil, err
	}
	results = append(results, verified...)

	delta := run.Delta{PinResults: results}
	for _, r := range results {
		if !r.Proven {
			delta.Findings = append(delta.Findings, proofFinding(r))
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

// isAddedLine reports whether from appears within an ADDED ("+", not "+++") line of a unified diff.
// The bind is to added CODE, not exact-line equality: from is the text to replace, which lives on a
// line the fix introduced. An empty from can bind to nothing.
func isAddedLine(diff, from string) bool {
	if from == "" {
		return false
	}
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") && strings.Contains(line[1:], from) {
			return true
		}
	}
	return false
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
