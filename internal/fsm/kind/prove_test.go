package kind

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/fsm/errs"
	"github.com/dsifry/metareview/internal/fsm/judge"
	"github.com/dsifry/metareview/internal/fsm/machine"
	"github.com/dsifry/metareview/internal/fsm/run"
	"github.com/dsifry/metareview/internal/fsm/workflow"
)

var proveNode = &workflow.Node{Name: "prove", Kind: Prove, Exec: "fork"}

type mockProver struct {
	results   []run.ProofResult
	err       error
	gotProofs []run.DifferentialProof
	gotSpec   ProveSpec
}

func (m *mockProver) ProvePins(_ context.Context, proofs []run.DifferentialProof, spec ProveSpec) ([]run.ProofResult, error) {
	m.gotProofs, m.gotSpec = proofs, spec
	if m.err != nil {
		return nil, m.err
	}
	return m.results, nil
}

func pinDP(finding, from string) run.DifferentialProof {
	return run.DifferentialProof{ID: "p-" + finding, Finding: finding, Kind: run.ProofPin, Pin: &run.Pin{ID: "p-" + finding, Finding: finding, File: "a.go", From: from, To: "y", Test: "T"}}
}

func reproDP(finding string) run.DifferentialProof {
	return run.DifferentialProof{ID: "r-" + finding, Finding: finding, Kind: run.ProofReproduction, Test: "T"}
}

func provenResult(p run.DifferentialProof) run.ProofResult {
	return run.ProofResult{Proof: p, Proven: true, Outcome: run.PinProven}
}

func runProve(t *testing.T, snap run.Snapshot, diff string, prover Prover) run.Delta {
	t.Helper()
	r := mustNew(t, judge.NewMock(judge.Script{}), true)
	r.execs[Prove] = &proveExec{prover: prover}
	ex, _ := r.Executor(Prove)
	raw, err := ex.Execute(context.Background(), machine.ExecInput{Snap: snap, Node: proveNode, Diff: machine.Diff{Text: diff}, Audit: (&audits{}).fn})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	// The executor's own output must always pass the kind's Decode (validity by construction).
	if _, derr := (proveKind{}).Decode(raw); derr != nil {
		t.Fatalf("prove output must decode: %v", derr)
	}
	var d run.Delta
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	return d
}

// A proven pin emits a Proven result and NO marker finding — the pins_proven gate then passes.
func TestProveProvenPinEmitsNoFinding(t *testing.T) {
	p := pinDP("f1", "addedLine")
	mp := &mockProver{results: []run.ProofResult{provenResult(p)}}
	d := runProve(t, run.Snapshot{Unproven: []run.DifferentialProof{p}}, "@@\n+addedLine\n", mp)
	if len(mp.gotProofs) != 1 {
		t.Fatalf("the bound pin must reach the prover: %+v", mp.gotProofs)
	}
	if len(d.PinResults) != 1 || !d.PinResults[0].Proven {
		t.Fatalf("expected one Proven result: %+v", d.PinResults)
	}
	if len(d.Findings) != 0 {
		t.Fatalf("a Proven pin must emit no marker finding: %+v", d.Findings)
	}
}

// A survived pin emits a blocking unproven-fix marker finding the gate reddens on.
func TestProveSurvivedPinEmitsBlockingFinding(t *testing.T) {
	p := pinDP("f1", "addedLine")
	mp := &mockProver{results: []run.ProofResult{{Proof: p, Proven: false, Outcome: run.PinSurvived, Detail: "tests still passed"}}}
	d := runProve(t, run.Snapshot{Unproven: []run.DifferentialProof{p}}, "@@\n+addedLine\n", mp)
	if len(d.Findings) != 1 || d.Findings[0].Source != run.SourceMutationVerify || d.Findings[0].Category != run.CategoryUnprovenFix {
		t.Fatalf("a survived pin must emit an unproven-fix marker: %+v", d.Findings)
	}
	if d.Findings[0].File != "a.go" || d.Findings[0].Severity != "high" {
		t.Fatalf("the marker must carry the pin's file and a high severity: %+v", d.Findings[0])
	}
}

// A pin whose From is not a line the fix added never reaches the prover: it is malformed (advisory),
// the CLAIM is bad. The added-line bind is what ties a mutation to code the fix introduced.
func TestProveAddedLineBindRejectsNonAddedFrom(t *testing.T) {
	p := pinDP("f1", "contextLine")
	mp := &mockProver{}
	// The diff carries contextLine only as an unchanged context line and as a removed line — never "+".
	d := runProve(t, run.Snapshot{Unproven: []run.DifferentialProof{p}}, "@@\n contextLine\n-contextLine\n+different\n", mp)
	if len(mp.gotProofs) != 0 {
		t.Fatalf("an unbound pin must NOT reach the prover: %+v", mp.gotProofs)
	}
	if len(d.PinResults) != 1 || d.PinResults[0].Outcome != run.PinMalformed {
		t.Fatalf("a pin failing the added-line bind must be malformed: %+v", d.PinResults)
	}
	if len(d.Findings) != 1 || d.Findings[0].Category != run.CategoryMalformedPin || d.Findings[0].Severity != "medium" {
		t.Fatalf("a malformed pin must emit an advisory malformed-pin marker: %+v", d.Findings)
	}
}

// A reproduction/deletion proof has no engine in this build: it is unverifiable (blocking), never a
// silent pass.
func TestProveReproductionIsUnverifiableInThisBuild(t *testing.T) {
	p := reproDP("f1")
	mp := &mockProver{}
	d := runProve(t, run.Snapshot{Unproven: []run.DifferentialProof{p}}, "@@\n+whatever\n", mp)
	if len(mp.gotProofs) != 0 {
		t.Fatalf("a reproduction proof must not reach the pin prover: %+v", mp.gotProofs)
	}
	if len(d.PinResults) != 1 || d.PinResults[0].Outcome != run.PinUnverifiable {
		t.Fatalf("a reproduction proof must be unverifiable here: %+v", d.PinResults)
	}
	if len(d.Findings) != 1 || d.Findings[0].Category != run.CategoryUnverifiable {
		t.Fatalf("an unverifiable proof must emit an unverifiable marker: %+v", d.Findings)
	}
}

// A nil prover fails closed (ERR_EXECUTOR_FAILED{no_prover}), exactly as a judge-less adjudicate does.
func TestProveNilProverFailsClosed(t *testing.T) {
	r := mustNew(t, judge.NewMock(judge.Script{}), true)
	r.execs[Prove] = &proveExec{prover: nil}
	ex, _ := r.Executor(Prove)
	_, err := ex.Execute(context.Background(), machine.ExecInput{Snap: run.Snapshot{Unproven: []run.DifferentialProof{pinDP("f1", "x")}}, Node: proveNode, Diff: machine.Diff{Text: "+x"}, Audit: (&audits{}).fn})
	if !errs.Is(err, machine.CodeExecutorFailed) {
		t.Fatalf("a nil prover must fail closed: %v", err)
	}
}

// A prover error aborts the run rather than being swallowed as a pass.
func TestProveProverErrorAborts(t *testing.T) {
	p := pinDP("f1", "addedLine")
	r := mustNew(t, judge.NewMock(judge.Script{}), true)
	r.execs[Prove] = &proveExec{prover: &mockProver{err: errors.New("runner down")}}
	ex, _ := r.Executor(Prove)
	_, err := ex.Execute(context.Background(), machine.ExecInput{Snap: run.Snapshot{Unproven: []run.DifferentialProof{p}}, Node: proveNode, Diff: machine.Diff{Text: "+addedLine"}, Audit: (&audits{}).fn})
	if err == nil || err.Error() != "runner down" {
		t.Fatalf("a prover error must abort: %v", err)
	}
}

// The test command and working dir are resolved from the run's snapshot (consent-hashed cmd by name,
// work dir), never from the agent.
func TestProveResolvesSpecFromSnapshot(t *testing.T) {
	p := pinDP("f1", "addedLine")
	mp := &mockProver{results: []run.ProofResult{provenResult(p)}}
	snap := run.Snapshot{
		Unproven:    []run.DifferentialProof{p},
		WorkDir:     "/w",
		AllowedCmds: []run.AllowedCmd{{Name: "test", Argv: []string{"go", "test", "./..."}}},
	}
	proveNode.Params = map[string]any{"test_cmd": "test"}
	defer func() { proveNode.Params = nil }()
	runProve(t, snap, "@@\n+addedLine\n", mp)
	if len(mp.gotSpec.TestCmd) != 3 || mp.gotSpec.TestCmd[0] != "go" || mp.gotSpec.Dir != "/w" {
		t.Fatalf("spec must come from the snapshot: %+v", mp.gotSpec)
	}
}

// The prove kind's host contract: it is fork-only, host-unsupported (Instructions), decodes and
// re-serialises its own Delta, and validates the test_cmd param.
func TestProveKindContract(t *testing.T) {
	k := proveKind{}
	if k.Name() != Prove {
		t.Fatalf("name %q", k.Name())
	}
	info := k.Info()
	if info.DefaultExec != "fork" || len(info.AllowedExec) != 1 || info.AllowedExec[0] != "fork" || info.NeedsJudge || info.ValidateParams == nil {
		t.Fatalf("info %+v", info)
	}
	if _, err := k.Instructions(run.Snapshot{}, proveNode, machine.Diff{}, "nonce"); !errs.Is(err, CodeExecUnsupported) {
		t.Fatalf("prove must be host-unsupported: %v", err)
	}
	// validateProve
	if err := validateProve(map[string]any{"test_cmd": "go-test"}); err != nil {
		t.Fatalf("valid test_cmd rejected: %v", err)
	}
	if validateProve(map[string]any{"bogus": 1}) == nil {
		t.Fatal("unknown param must be rejected")
	}
	if validateProve(map[string]any{"test_cmd": 7}) == nil {
		t.Fatal("non-string test_cmd must be rejected")
	}
	// Reduce passes the decoded Delta through.
	d, err := k.Decode(json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if red, err := k.Reduce(run.Snapshot{}, d); err != nil || red.PinResults != nil {
		t.Fatalf("reduce: %v %+v", err, red)
	}
	// A well-formed result + finding decodes.
	good := run.Delta{
		PinResults: []run.ProofResult{{Proof: reproDP("f1"), Proven: true, Outcome: run.PinProven}},
		Findings:   []run.Finding{{IssueText: "x", Source: run.SourceMutationVerify, Category: run.CategoryUnprovenFix}},
	}
	if _, err := k.Decode(run.MarshalCanonical(good)); err != nil {
		t.Fatalf("valid prove delta rejected: %v", err)
	}
	// Decode error branches.
	badReason := func(raw []byte, reason string) {
		t.Helper()
		_, err := k.Decode(raw)
		if !errs.Is(err, CodeNodeOutputInvalid) || errs.As(err).Field("reason") != reason {
			t.Fatalf("want %s got %v", reason, err)
		}
	}
	badReason([]byte(`{"zzz":1}`), "decode")
	badReason(run.MarshalCanonical(run.Delta{Findings: []run.Finding{{IssueText: ""}}}), "empty")
	// pin_results count over cap
	many := make([]run.ProofResult, run.MaxDeltaList+1)
	for i := range many {
		many[i] = run.ProofResult{Proof: reproDP("f"), Proven: true, Outcome: run.PinProven}
	}
	badReason(run.MarshalCanonical(run.Delta{PinResults: many}), "cap")
	// a per-field cap: an over-cap Detail
	badReason(run.MarshalCanonical(run.Delta{PinResults: []run.ProofResult{{Proof: reproDP("f"), Detail: strings.Repeat("x", run.MaxDetail+1)}}}), "cap")
	// payload over cap: many max-size findings pass their own caps but overflow the envelope
	fat := make([]run.Finding, 200)
	for i := range fat {
		fat[i] = run.Finding{IssueText: strings.Repeat("x", run.MaxText-2)}
	}
	badReason(run.MarshalCanonical(run.Delta{Findings: fat}), "cap")
}

// A test_cmd naming a cmd the run never consented to resolves to no command — the prover then reports
// unverifiable rather than running something unsanctioned.
func TestProveUnconsentedTestCmdResolvesToNothing(t *testing.T) {
	mp := &mockProver{results: []run.ProofResult{provenResult(pinDP("f1", "addedLine"))}}
	snap := run.Snapshot{Unproven: []run.DifferentialProof{pinDP("f1", "addedLine")}, AllowedCmds: []run.AllowedCmd{{Name: "build"}}}
	proveNode.Params = map[string]any{"test_cmd": "test"} // not present among AllowedCmds
	defer func() { proveNode.Params = nil }()
	runProve(t, snap, "@@\n+addedLine\n", mp)
	if mp.gotSpec.TestCmd != nil {
		t.Fatalf("an unconsented test_cmd must resolve to no command: %+v", mp.gotSpec.TestCmd)
	}
}

// If the emitted delta would exceed a persistence cap, the executor fails closed on its own Decode
// rather than reporting a success the fold would then refuse.
func TestProveOutputOverCapFailsClosed(t *testing.T) {
	open := make([]run.DifferentialProof, run.MaxDeltaList+1)
	for i := range open {
		open[i] = reproDP(fmt.Sprint(i)) // each is unverifiable → one result + one finding
	}
	r := mustNew(t, judge.NewMock(judge.Script{}), true)
	r.execs[Prove] = &proveExec{prover: &mockProver{}}
	ex, _ := r.Executor(Prove)
	_, err := ex.Execute(context.Background(), machine.ExecInput{Snap: run.Snapshot{Unproven: open}, Node: proveNode, Diff: machine.Diff{Text: ""}, Audit: (&audits{}).fn})
	if !errs.Is(err, CodeNodeOutputInvalid) {
		t.Fatalf("an over-cap prove output must fail closed: %v", err)
	}
}

func TestIsAddedLine(t *testing.T) {
	diff := "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1,2 +1,3 @@\n ctx\n-old\n+\tnewCode()\n"
	if !isAddedLine(diff, "newCode()") {
		t.Error("text on a + line must bind")
	}
	if isAddedLine(diff, "old") {
		t.Error("a removed line must not bind")
	}
	if isAddedLine(diff, "ctx") {
		t.Error("a context line must not bind")
	}
	if isAddedLine(diff, "a.go") {
		t.Error("the +++ header must not count as an added line")
	}
	if isAddedLine(diff, "") {
		t.Error("an empty From binds to nothing")
	}
}
