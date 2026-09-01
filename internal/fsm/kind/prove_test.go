package kind

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	results       []run.ProofResult
	err           error
	gotProofs     []run.DifferentialProof
	gotSpec       ProveSpec
	reproResults  []run.ProofResult
	reproErr      error
	gotRepro      []run.DifferentialProof
	gotReproSpec  ProveSpec
	deleteResults []run.ProofResult
	deleteErr     error
	gotDelete     []run.DifferentialProof
	gotDeleteSpec ProveSpec
}

func (m *mockProver) ProvePins(_ context.Context, proofs []run.DifferentialProof, spec ProveSpec) ([]run.ProofResult, error) {
	if len(proofs) == 0 { // mirror the real prover: nothing to verify
		return nil, nil
	}
	m.gotProofs, m.gotSpec = proofs, spec
	if m.err != nil {
		return nil, m.err
	}
	return m.results, nil
}

func (m *mockProver) ProveReproductions(_ context.Context, proofs []run.DifferentialProof, spec ProveSpec) ([]run.ProofResult, error) {
	m.gotRepro, m.gotReproSpec = proofs, spec
	if m.reproErr != nil {
		return nil, m.reproErr
	}
	return m.reproResults, nil
}

func (m *mockProver) ProveDeletions(_ context.Context, proofs []run.DifferentialProof, spec ProveSpec) ([]run.ProofResult, error) {
	m.gotDelete, m.gotDeleteSpec = proofs, spec
	if m.deleteErr != nil {
		return nil, m.deleteErr
	}
	return m.deleteResults, nil
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

// fileDiff builds a well-formed unified diff (with the "diff --git" header the parser keys on) that
// ADDS the given lines to file.
func fileDiff(file string, added ...string) string {
	d := "diff --git a/" + file + " b/" + file + "\n--- a/" + file + "\n+++ b/" + file + "\n@@ -1,1 +1," + fmt.Sprint(len(added)+1) + " @@\n"
	for _, l := range added {
		d += "+" + l + "\n"
	}
	return d
}

// fakeReviewer is a §9.2 symptom reviewer for tests: decision true = the failure matches the
// finding's symptom (no veto); an err simulates a reviewer that could not run (fail-closed veto).
type fakeReviewer struct {
	decision bool
	err      error
	calls    int
	gotInput []judge.SymptomInput
}

func (f *fakeReviewer) Call(_ context.Context, r judge.Request) (judge.Verdict, error) {
	f.calls++
	if si, ok := r.Input.(judge.SymptomInput); ok {
		f.gotInput = append(f.gotInput, si)
	}
	if f.err != nil {
		return judge.Verdict{}, f.err
	}
	return judge.Verdict{Kind: r.Kind, Decision: f.decision}, nil
}

func runProve(t *testing.T, snap run.Snapshot, diff string, prover Prover) run.Delta {
	t.Helper()
	// A matches-by-default reviewer: it runs only on a Proven reproduction and lets it stand, so a
	// test that is not about the §9.2 veto behaves exactly as before.
	return runProveWith(t, snap, diff, prover, &fakeReviewer{decision: true})
}

func runProveWith(t *testing.T, snap run.Snapshot, diff string, prover Prover, reviewer judge.Judge) run.Delta {
	t.Helper()
	r := mustNew(t, judge.NewMock(judge.Script{}), true)
	r.execs[Prove] = &proveExec{prover: prover, symptom: reviewer}
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
	d := runProve(t, run.Snapshot{Unproven: []run.DifferentialProof{p}}, fileDiff("a.go", "addedLine"), mp)
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
	d := runProve(t, run.Snapshot{Unproven: []run.DifferentialProof{p}}, fileDiff("a.go", "addedLine"), mp)
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
	d := runProve(t, run.Snapshot{Unproven: []run.DifferentialProof{p}}, "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1,2 +1,2 @@\n contextLine\n-contextLine\n+different\n", mp)
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

// A reproduction proof is routed to the reproduction engine (ProveReproductions), never the pin
// prover, and the pre/post-fix anchors reach the engine through the spec. A proven reproduction emits
// no marker, so pins_proven passes.
func TestProveReproductionRoutesToReproductionEngine(t *testing.T) {
	p := reproDP("f1")
	mp := &mockProver{reproResults: []run.ProofResult{provenResult(p)}}
	snap := run.Snapshot{Unproven: []run.DifferentialProof{p}, FixEntryHead: "preSHA", Head: "postSHA"}
	d := runProve(t, snap, "@@\n+whatever\n", mp)
	if len(mp.gotProofs) != 0 {
		t.Fatalf("a reproduction proof must NOT reach the pin prover: %+v", mp.gotProofs)
	}
	if len(mp.gotRepro) != 1 || mp.gotRepro[0].Finding != "f1" {
		t.Fatalf("a reproduction proof must reach the reproduction engine: %+v", mp.gotRepro)
	}
	if mp.gotReproSpec.PreFixSHA != "preSHA" || mp.gotReproSpec.PostFixSHA != "postSHA" {
		t.Fatalf("the pre/post-fix anchors must reach the engine: %+v", mp.gotReproSpec)
	}
	if len(d.PinResults) != 1 || !d.PinResults[0].Proven {
		t.Fatalf("expected one proven reproduction result: %+v", d.PinResults)
	}
	if len(d.Findings) != 0 {
		t.Fatalf("a proven reproduction must emit no marker finding: %+v", d.Findings)
	}
}

// A survived reproduction (the fix's test does not exercise the fault) emits a blocking unproven-fix
// marker the pins_proven gate reddens on — the reproduction form is gated exactly as a pin.
func TestProveSurvivedReproductionBlocks(t *testing.T) {
	p := reproDP("f1")
	mp := &mockProver{reproResults: []run.ProofResult{{Proof: p, Proven: false, Outcome: run.PinSurvived, Detail: "passes pre-fix"}}}
	snap := run.Snapshot{Unproven: []run.DifferentialProof{p}, FixEntryHead: "pre", Head: "post"}
	d := runProve(t, snap, "@@\n+whatever\n", mp)
	if len(d.Findings) != 1 || d.Findings[0].Source != run.SourceMutationVerify || d.Findings[0].Category != run.CategoryUnprovenFix {
		t.Fatalf("a survived reproduction must emit an unproven-fix marker: %+v", d.Findings)
	}
}

// A reproduction engine error aborts the run rather than being swallowed as a pass.
func TestProveReproductionErrorAborts(t *testing.T) {
	p := reproDP("f1")
	mp := &mockProver{reproErr: errors.New("worktree exploded")}
	r := mustNew(t, judge.NewMock(judge.Script{}), true)
	r.execs[Prove] = &proveExec{prover: mp}
	ex, _ := r.Executor(Prove)
	snap := run.Snapshot{Unproven: []run.DifferentialProof{p}, FixEntryHead: "pre", Head: "post"}
	if _, err := ex.Execute(context.Background(), machine.ExecInput{Snap: snap, Node: proveNode, Diff: machine.Diff{Text: "@@\n+x\n"}, Audit: (&audits{}).fn}); err == nil {
		t.Fatal("a reproduction engine error must abort the run")
	}
}

// A deletion engine error aborts the run rather than being swallowed as a pass.
func TestProveDeletionErrorAborts(t *testing.T) {
	p := deletionDP("f1", "a.go", "gone")
	mp := &mockProver{deleteErr: errors.New("worktree exploded")}
	r := mustNew(t, judge.NewMock(judge.Script{}), true)
	r.execs[Prove] = &proveExec{prover: mp, symptom: &fakeReviewer{decision: true}}
	ex, _ := r.Executor(Prove)
	snap := run.Snapshot{Unproven: []run.DifferentialProof{p}, Confirmed: []run.Bug{{ID: "f1", File: "a.go"}}, FixEntryHead: "pre", Head: "post"}
	if _, err := ex.Execute(context.Background(), machine.ExecInput{Snap: snap, Node: proveNode, Diff: machine.Diff{Text: "@@\n+x\n"}, Audit: (&audits{}).fn}); err == nil {
		t.Fatal("a deletion engine error must abort the run")
	}
}

func deletionDP(finding, file, removed string) run.DifferentialProof {
	return run.DifferentialProof{ID: "d-" + finding, Finding: finding, Kind: run.ProofDeletion, Test: "T", Deletes: &run.DeletionRef{File: file, Removed: removed}}
}

// A deletion in the finding's OWN file routes to the deletion engine (ProveDeletions), never the pin
// prover, with the pre/post-fix anchors on the spec. A proven deletion emits no marker.
func TestProveDeletionRoutesToDeletionEngine(t *testing.T) {
	p := deletionDP("f1", "a.go", "gone")
	mp := &mockProver{deleteResults: []run.ProofResult{provenResult(p)}}
	snap := run.Snapshot{Unproven: []run.DifferentialProof{p}, Confirmed: []run.Bug{{ID: "f1", File: "a.go"}}, FixEntryHead: "preSHA", Head: "postSHA"}
	d := runProve(t, snap, "@@\n+whatever\n", mp)
	if len(mp.gotProofs) != 0 || len(mp.gotRepro) != 0 {
		t.Fatalf("a deletion must reach neither the pin nor the reproduction prover: %+v %+v", mp.gotProofs, mp.gotRepro)
	}
	if len(mp.gotDelete) != 1 || mp.gotDeleteSpec.PreFixSHA != "preSHA" || mp.gotDeleteSpec.PostFixSHA != "postSHA" {
		t.Fatalf("a deletion must reach the deletion engine with the anchors: %+v %+v", mp.gotDelete, mp.gotDeleteSpec)
	}
	if len(d.Findings) != 0 || len(d.PinResults) != 1 || !d.PinResults[0].Proven {
		t.Fatalf("a proven deletion stands with no marker: results %+v findings %+v", d.PinResults, d.Findings)
	}
}

// The own-file bind: a deletion whose file is NOT the finding's own file is malformed and never reaches
// the engine — the own-file guarantee (§9.1) that keeps the proven code and the class gate's line the same.
func TestProveDeletionOwnFileBind(t *testing.T) {
	p := deletionDP("f1", "other.go", "gone")
	mp := &mockProver{}
	snap := run.Snapshot{Unproven: []run.DifferentialProof{p}, Confirmed: []run.Bug{{ID: "f1", File: "a.go"}}, FixEntryHead: "pre", Head: "post"}
	d := runProve(t, snap, "@@\n+x\n", mp)
	if len(mp.gotDelete) != 0 {
		t.Fatalf("a cross-file deletion must NOT reach the engine: %+v", mp.gotDelete)
	}
	if len(d.PinResults) != 1 || d.PinResults[0].Outcome != run.PinMalformed {
		t.Fatalf("a deletion outside the finding's file must be malformed: %+v", d.PinResults)
	}
	// Unlike a malformed PIN (advisory), a malformed DELETION BLOCKS — the deletion IS the fix's proof
	// and a pure deletion has no owed-pin backup, so an advisory marker would be a silent pass.
	if len(d.Findings) != 1 || d.Findings[0].Category != run.CategoryUnprovenFix || !run.ProofCategoryBlocks(d.Findings[0].Category) {
		t.Fatalf("a malformed deletion must emit a BLOCKING marker: %+v", d.Findings)
	}
}

// An unknown proof kind cannot occur through decode (the one-of is enforced), but if one reached the
// node it must ABORT (surface the invariant violation), never be silently skipped into a pass.
func TestProveUnknownKindAborts(t *testing.T) {
	p := run.DifferentialProof{ID: "u1", Finding: "f1", Kind: run.ProofPin, Pin: &run.Pin{File: "a.go", From: "x", To: "y", Test: "T"}}
	p.Kind = "mystery" // force an unknown kind past the decode-time one-of check
	r := mustNew(t, judge.NewMock(judge.Script{}), true)
	r.execs[Prove] = &proveExec{prover: &mockProver{}, symptom: &fakeReviewer{decision: true}}
	ex, _ := r.Executor(Prove)
	_, err := ex.Execute(context.Background(), machine.ExecInput{Snap: run.Snapshot{Unproven: []run.DifferentialProof{p}}, Node: proveNode, Diff: machine.Diff{Text: "@@\n+x\n"}, Audit: (&audits{}).fn})
	if !errs.Is(err, machine.CodeExecutorFailed) {
		t.Fatalf("an unknown kind must abort: %v", err)
	}
}

// A nil prover fails closed (ERR_EXECUTOR_FAILED{no_prover}), exactly as a judge-less adjudicate does.
func TestProveNilProverFailsClosed(t *testing.T) {
	r := mustNew(t, judge.NewMock(judge.Script{}), true)
	r.execs[Prove] = &proveExec{prover: nil}
	ex, _ := r.Executor(Prove)
	_, err := ex.Execute(context.Background(), machine.ExecInput{Snap: run.Snapshot{Unproven: []run.DifferentialProof{pinDP("f1", "x")}}, Node: proveNode, Diff: machine.Diff{Text: fileDiff("a.go", "x")}, Audit: (&audits{}).fn})
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
	_, err := ex.Execute(context.Background(), machine.ExecInput{Snap: run.Snapshot{Unproven: []run.DifferentialProof{p}}, Node: proveNode, Diff: machine.Diff{Text: fileDiff("a.go", "addedLine")}, Audit: (&audits{}).fn})
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
	runProve(t, snap, fileDiff("a.go", "addedLine"), mp)
	if len(mp.gotSpec.TestCmd) != 3 || mp.gotSpec.TestCmd[0] != "go" || mp.gotSpec.Dir != "/w" {
		t.Fatalf("spec must come from the snapshot: %+v", mp.gotSpec)
	}
}

// pins_proven clause (a): a confirmed finding that OWES a pin (the fix added a line in its own file and
// that file's package has tests) but has no Proven proof must block, even when the fix declared no pin
// at all — the vacuous-pass hole (#24) the gate exists to close. A cross-file remedy, a no-test
// package, or a finding already backed by a Proven pin owes nothing and emits no marker.
func TestProveClauseAOwedPin(t *testing.T) {
	dir := t.TempDir()
	mustWrite := func(rel, body string) {
		if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("pkg/foo.go", "package pkg\n")
	mustWrite("pkg/foo_test.go", "package pkg\n") // pkg HAS tests
	mustWrite("bare/bar.go", "package bare\n")    // bare has NO tests

	owed := run.Bug{ID: "owed", Desc: "d", File: "pkg/foo.go", Verdict: run.VerdictRealButUngold}
	diffTouchesPkg := fileDiff("pkg/foo.go", "newLine")

	// (1) owes a pin, none supplied → a blocking owed-pin marker naming the file.
	d := runProve(t, run.Snapshot{WorkDir: dir, Confirmed: []run.Bug{owed}}, diffTouchesPkg, &mockProver{})
	if len(d.Findings) != 1 || d.Findings[0].Category != run.CategoryUnprovenFix || d.Findings[0].File != "pkg/foo.go" || d.Findings[0].Source != run.SourceMutationVerify {
		t.Fatalf("an owed, unsupplied finding must block: %+v", d.Findings)
	}

	// (2) the fix touched only a DIFFERENT file → the finding owes no pin here (cross-file remedy).
	d = runProve(t, run.Snapshot{WorkDir: dir, Confirmed: []run.Bug{owed}}, fileDiff("other/z.go", "elsewhere"), &mockProver{})
	if len(d.Findings) != 0 {
		t.Fatalf("a cross-file remedy owes no pin: %+v", d.Findings)
	}

	// (3) the finding's package has no test files → owes no pin (unpinnable, so not blocked).
	d = runProve(t, run.Snapshot{WorkDir: dir, Confirmed: []run.Bug{{ID: "b2", File: "bare/bar.go", Verdict: run.VerdictRealButUngold}}}, fileDiff("bare/bar.go", "x"), &mockProver{})
	if len(d.Findings) != 0 {
		t.Fatalf("a no-test package owes no pin: %+v", d.Findings)
	}

	// (4) a Proven pin for the finding covers it → no owed marker (the supplied proof spoke for it).
	proof := pinDP("owed", "newLine")
	proof.Pin.File = "pkg/foo.go"
	d = runProve(t, run.Snapshot{WorkDir: dir, Confirmed: []run.Bug{owed}, Unproven: []run.DifferentialProof{proof}}, diffTouchesPkg, &mockProver{results: []run.ProofResult{provenResult(proof)}})
	if len(d.Findings) != 0 {
		t.Fatalf("a finding backed by a Proven pin owes no additional marker: %+v", d.Findings)
	}

	// (5) the finding-1 hole: a MALFORMED (advisory) supplied pin must NOT let an owed finding escape.
	// The pin's From is not an added line → malformed (advisory); clause (a) still emits a BLOCKING
	// owed marker, so the gate reddens.
	mal := pinDP("owed", "notAnAddedLine")
	mal.Pin.File = "pkg/foo.go"
	d = runProve(t, run.Snapshot{WorkDir: dir, Confirmed: []run.Bug{owed}, Unproven: []run.DifferentialProof{mal}}, diffTouchesPkg, &mockProver{})
	var advisory, blocking int
	for _, f := range d.Findings {
		switch f.Category {
		case run.CategoryMalformedPin:
			advisory++
		case run.CategoryUnprovenFix:
			blocking++
		}
	}
	if advisory != 1 || blocking != 1 {
		t.Fatalf("a malformed (advisory) pin must not let an owed finding escape clause (a): %+v", d.Findings)
	}

	// (6) a Proven REPRODUCTION for the finding discharges clause (a) exactly as a Proven pin does —
	// the reproduction form satisfies its finding via a committed test, not an owned pin, so the
	// owed-pin marker must not fire (settled keys on ANY Proven proof kind; spec §9.1).
	repro := reproDP("owed")
	d = runProve(t, run.Snapshot{WorkDir: dir, Confirmed: []run.Bug{owed}, FixEntryHead: "pre", Head: "post", Unproven: []run.DifferentialProof{repro}}, diffTouchesPkg, &mockProver{reproResults: []run.ProofResult{provenResult(repro)}})
	if len(d.Findings) != 0 {
		t.Fatalf("a finding backed by a Proven reproduction owes no owed-pin marker: %+v", d.Findings)
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
	runProve(t, snap, fileDiff("a.go", "addedLine"), mp)
	if mp.gotSpec.TestCmd != nil {
		t.Fatalf("an unconsented test_cmd must resolve to no command: %+v", mp.gotSpec.TestCmd)
	}
}

// If the emitted delta would exceed a persistence cap, the executor fails closed on its own Decode
// rather than reporting a success the fold would then refuse.
func TestProveOutputOverCapFailsClosed(t *testing.T) {
	open := make([]run.DifferentialProof, run.MaxDeltaList+1)
	for i := range open {
		// Deletion has no engine in this build, so each is unverifiable → one result + one finding.
		open[i] = run.DifferentialProof{ID: fmt.Sprint(i), Finding: fmt.Sprint(i), Kind: run.ProofDeletion, Deletes: &run.DeletionRef{File: "a.go", Removed: "x"}}
	}
	r := mustNew(t, judge.NewMock(judge.Script{}), true)
	r.execs[Prove] = &proveExec{prover: &mockProver{}}
	ex, _ := r.Executor(Prove)
	_, err := ex.Execute(context.Background(), machine.ExecInput{Snap: run.Snapshot{Unproven: open}, Node: proveNode, Diff: machine.Diff{Text: ""}, Audit: (&audits{}).fn})
	if !errs.Is(err, CodeNodeOutputInvalid) {
		t.Fatalf("an over-cap prove output must fail closed: %v", err)
	}
}

func TestIsAddedLineInFile(t *testing.T) {
	// Two files, each with its own added line. a.go added newCode(); b.go added other().
	diff := "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1,2 +1,3 @@\n ctx\n-old\n+\tnewCode()\n" +
		"diff --git a/b.go b/b.go\n--- a/b.go\n+++ b/b.go\n@@ -1,1 +1,2 @@\n+\tother()\n"
	if !isAddedLineInFile(diff, "a.go", "newCode()") {
		t.Error("text on a + line in the file must bind")
	}
	if isAddedLineInFile(diff, "a.go", "old") {
		t.Error("a removed line must not bind")
	}
	if isAddedLineInFile(diff, "a.go", "ctx") {
		t.Error("a context line must not bind")
	}
	// The load-bearing fix: an added line in ANOTHER file must not bind the pin in a.go.
	if isAddedLineInFile(diff, "a.go", "other()") {
		t.Error("an added line in a different file must not bind the pin (file scoping)")
	}
	if !isAddedLineInFile(diff, "b.go", "other()") {
		t.Error("the same text does bind when the pin names the file it was added to")
	}
	if isAddedLineInFile(diff, "a.go", "") || isAddedLineInFile(diff, "", "newCode()") {
		t.Error("an empty From or file binds to nothing")
	}
}

// §9.2: a Proven reproduction whose pre-fix failure MATCHES the finding's own symptom stands — the
// reviewer runs once, is handed the finding/test/fail-before output, and adds no finding.
func TestProveReproductionSymptomMatchSettles(t *testing.T) {
	p := reproDP("f1")
	mp := &mockProver{reproResults: []run.ProofResult{{Proof: p, Proven: true, Outcome: run.PinProven, FailBefore: "=== RUN   T\n--- FAIL: T\nwant X got Y\n"}}}
	rev := &fakeReviewer{decision: true}
	snap := run.Snapshot{Unproven: []run.DifferentialProof{p}, Confirmed: []run.Bug{{ID: "f1", Desc: "X not Y"}}, FixEntryHead: "pre", Head: "post"}
	d := runProveWith(t, snap, "@@\n+x\n", mp, rev)
	if rev.calls != 1 {
		t.Fatalf("the reviewer must run exactly once on a proven reproduction: %d", rev.calls)
	}
	if len(rev.gotInput) != 1 || rev.gotInput[0].Test != "T" || rev.gotInput[0].Bug.ID != "f1" || !strings.Contains(rev.gotInput[0].FailOutput, "want X got Y") {
		t.Fatalf("reviewer input must carry the finding, test, and fail-before output: %+v", rev.gotInput)
	}
	if len(d.Findings) != 0 {
		t.Fatalf("a matched symptom must not veto: %+v", d.Findings)
	}
	if len(d.PinResults) != 1 || !d.PinResults[0].Proven {
		t.Fatalf("the reproduction stays proven: %+v", d.PinResults)
	}
}

// §9.2 veto: a symptom MISMATCH (reviewer decision false) blocks — a blocking marker the pins_proven
// gate reddens on, and the finding is NOT settled.
func TestProveReproductionSymptomMismatchVetoes(t *testing.T) {
	p := reproDP("f1")
	mp := &mockProver{reproResults: []run.ProofResult{{Proof: p, Proven: true, Outcome: run.PinProven, FailBefore: "--- FAIL: T\nunrelated panic\n"}}}
	snap := run.Snapshot{Unproven: []run.DifferentialProof{p}, FixEntryHead: "pre", Head: "post"}
	d := runProveWith(t, snap, "@@\n+x\n", mp, &fakeReviewer{decision: false})
	if len(d.Findings) != 1 || d.Findings[0].Source != run.SourceMutationVerify || d.Findings[0].Category != run.CategoryUnverifiable || d.Findings[0].Severity != "high" {
		t.Fatalf("a symptom mismatch must emit a blocking veto finding: %+v", d.Findings)
	}
	if !run.ProofCategoryBlocks(d.Findings[0].Category) {
		t.Fatal("the veto finding's category must block")
	}
}

// §9.2 fail-closed: a reviewer that cannot run (error/timeout after the retry ladder) vetoes — never
// approves — and does so with a blocking finding, not a run abort (NEEDS_REVISION lets the fix retry).
func TestProveReproductionReviewerErrorFailsClosed(t *testing.T) {
	p := reproDP("f1")
	mp := &mockProver{reproResults: []run.ProofResult{{Proof: p, Proven: true, Outcome: run.PinProven, FailBefore: "--- FAIL: T\n"}}}
	snap := run.Snapshot{Unproven: []run.DifferentialProof{p}, FixEntryHead: "pre", Head: "post"}
	d := runProveWith(t, snap, "@@\n+x\n", mp, &fakeReviewer{err: errors.New("reviewer down")})
	if len(d.Findings) != 1 || d.Findings[0].Category != run.CategoryUnverifiable {
		t.Fatalf("a reviewer error must fail closed to a blocking veto: %+v", d.Findings)
	}
}

// A nil reviewer with a Proven reproduction to judge is a wiring error surfaced as ERR_EXECUTOR_FAILED
// — never a silent pass.
func TestProveNilReviewerFailsClosed(t *testing.T) {
	p := reproDP("f1")
	mp := &mockProver{reproResults: []run.ProofResult{{Proof: p, Proven: true, Outcome: run.PinProven, FailBefore: "--- FAIL: T\n"}}}
	r := mustNew(t, judge.NewMock(judge.Script{}), true)
	r.execs[Prove] = &proveExec{prover: mp, symptom: nil}
	ex, _ := r.Executor(Prove)
	snap := run.Snapshot{Unproven: []run.DifferentialProof{p}, FixEntryHead: "pre", Head: "post"}
	if _, err := ex.Execute(context.Background(), machine.ExecInput{Snap: snap, Node: proveNode, Diff: machine.Diff{Text: "@@\n+x\n"}, Audit: (&audits{}).fn}); !errs.Is(err, machine.CodeExecutorFailed) {
		t.Fatalf("a nil reviewer with a proven reproduction must fail closed: %v", err)
	}
}

// The §9.2 reviewer applies to reproductions only: a Proven PIN is never symptom-reviewed.
func TestProveProvenPinIsNotReviewed(t *testing.T) {
	p := pinDP("f1", "addedLine")
	mp := &mockProver{results: []run.ProofResult{provenResult(p)}}
	rev := &fakeReviewer{decision: true}
	d := runProveWith(t, run.Snapshot{Unproven: []run.DifferentialProof{p}}, fileDiff("a.go", "addedLine"), mp, rev)
	if rev.calls != 0 {
		t.Fatalf("a pin proof must not be symptom-reviewed: %d", rev.calls)
	}
	if len(d.Findings) != 0 {
		t.Fatalf("a proven pin must stand: %+v", d.Findings)
	}
}

// A reviewer CONFIGURATION error (no/invalid model, missing key/url) can never be cured by re-fixing,
// so it must ABORT (surface the wiring bug), not fail-closed-veto every reproduction forever.
func TestProveReviewerConfigErrorAborts(t *testing.T) {
	p := reproDP("f1")
	mp := &mockProver{reproResults: []run.ProofResult{{Proof: p, Proven: true, Outcome: run.PinProven, FailBefore: "--- FAIL: T\n"}}}
	r := mustNew(t, judge.NewMock(judge.Script{}), true)
	r.execs[Prove] = &proveExec{prover: mp, symptom: &fakeReviewer{err: errs.E(judge.CodeJudgeModel, "no model configured")}}
	ex, _ := r.Executor(Prove)
	snap := run.Snapshot{Unproven: []run.DifferentialProof{p}, FixEntryHead: "pre", Head: "post"}
	if _, err := ex.Execute(context.Background(), machine.ExecInput{Snap: snap, Node: proveNode, Diff: machine.Diff{Text: "@@\n+x\n"}, Audit: (&audits{}).fn}); !errs.Is(err, judge.CodeJudgeModel) {
		t.Fatalf("a reviewer config error must abort (surface), not veto: %v", err)
	}
}

// A malformed REPRODUCTION blocks (it is the fix's proof), unlike a malformed pin which is advisory.
func TestProveMalformedReproductionBlocks(t *testing.T) {
	p := reproDP("f1")
	mp := &mockProver{reproResults: []run.ProofResult{{Proof: p, Proven: false, Outcome: run.PinMalformed, Detail: "compile-error fail-before"}}}
	d := runProve(t, run.Snapshot{Unproven: []run.DifferentialProof{p}, FixEntryHead: "pre", Head: "post"}, "@@\n+x\n", mp)
	if len(d.Findings) != 1 || d.Findings[0].Category != run.CategoryUnprovenFix || !run.ProofCategoryBlocks(d.Findings[0].Category) {
		t.Fatalf("a malformed reproduction must block, not be advisory: %+v", d.Findings)
	}
}

// §9.6: a test deletion that un-kills a previously-PROVEN pin (it now survives) blocks.
func TestProveTestDeletionUnkillsProvenPin(t *testing.T) {
	pin := pinDP("f1", "guard") // File a.go
	mp := &mockProver{results: []run.ProofResult{{Proof: pin, Proven: false, Outcome: run.PinSurvived, Detail: "no longer caught"}}}
	diff := "diff --git a/x_test.go b/x_test.go\n--- a/x_test.go\n+++ /dev/null\n@@ -1,3 +0,0 @@\n-package x\n-func TestX(t *testing.T) {\n-}\n"
	// A proven REPRODUCTION in the set is skipped (§9.6 mutation-kill re-verifies pins only); only the
	// pin is re-verified.
	d := runProve(t, run.Snapshot{Proven: []run.DifferentialProof{reproDP("f2"), pin}}, diff, mp)
	if len(d.Findings) != 1 || d.Findings[0].Category != run.CategoryUnprovenFix || d.Findings[0].Source != run.SourceMutationVerify {
		t.Fatalf("un-killing a proven pin via test deletion must block: %+v", d.Findings)
	}
	if len(mp.gotProofs) != 1 || mp.gotProofs[0].ID != pin.ID {
		t.Fatalf("the proven pin must be re-verified: %+v", mp.gotProofs)
	}
}

// A test deletion where the proven pin STILL holds (re-verifies proven) → no regression.
func TestProveTestDeletionProvenPinStillHolds(t *testing.T) {
	pin := pinDP("f1", "guard")
	mp := &mockProver{results: []run.ProofResult{provenResult(pin)}}
	diff := "diff --git a/x_test.go b/x_test.go\n--- a/x_test.go\n+++ b/x_test.go\n@@ -1,4 +1,2 @@\n package x\n-func TestGone(t *testing.T) {\n-}\n func TestKept(t *testing.T) {}\n"
	d := runProve(t, run.Snapshot{Proven: []run.DifferentialProof{pin}}, diff, mp)
	if len(d.Findings) != 0 {
		t.Fatalf("a still-held pin after a test deletion must not block: %+v", d.Findings)
	}
}

// No test deleted → the §9.6 check is inert (no re-verification, no finding), even with proven pins.
func TestProveNoTestDeletionIsInert(t *testing.T) {
	pin := pinDP("f1", "guard")
	mp := &mockProver{results: []run.ProofResult{{Proof: pin, Proven: false, Outcome: run.PinSurvived}}}
	d := runProve(t, run.Snapshot{Proven: []run.DifferentialProof{pin}}, fileDiff("a.go", "just a change"), mp)
	if len(d.Findings) != 0 {
		t.Fatalf("no test deletion → no §9.6 finding: %+v", d.Findings)
	}
	if len(mp.gotProofs) != 0 {
		t.Fatalf("no re-verification when no test was deleted: %+v", mp.gotProofs)
	}
}

// The paired code+test deletion: when the pinned line was itself removed, re-verification returns
// MALFORMED (From no longer anchors), which is NOT counted as a regression — no special-case exclusion,
// so a still-live pin can never be dropped by a substring match on removed lines.
func TestProveTestDeletionRemovedTargetIsMalformedNotRegression(t *testing.T) {
	pin := pinDP("f1", "guardLine") // File a.go
	mp := &mockProver{results: []run.ProofResult{{Proof: pin, Proven: false, Outcome: run.PinMalformed, Detail: "anchor absent (line removed)"}}}
	diff := "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1,2 +1,1 @@\n-guardLine\n context\n" +
		"diff --git a/x_test.go b/x_test.go\n--- a/x_test.go\n+++ /dev/null\n@@ -1,2 +0,0 @@\n-package x\n-func TestX(t *testing.T) {}\n"
	d := runProve(t, run.Snapshot{Proven: []run.DifferentialProof{pin}}, diff, mp)
	if len(d.Findings) != 0 {
		t.Fatalf("a removed-target pin re-verifies malformed and must not block: %+v", d.Findings)
	}
	if len(mp.gotProofs) != 1 {
		t.Fatalf("the pin is re-verified (no pre-filter); the Verifier's malformed result is what spares it: %+v", mp.gotProofs)
	}
}

// A test deletion when the run has no proven PINS (only a reproduction) has nothing to re-verify —
// no finding, no re-verification call.
func TestProveTestDeletionNoProvenPins(t *testing.T) {
	mp := &mockProver{results: []run.ProofResult{{Proof: pinDP("f1", "guard"), Proven: false, Outcome: run.PinSurvived}}}
	diff := "diff --git a/x_test.go b/x_test.go\n--- a/x_test.go\n+++ /dev/null\n@@ -1,2 +0,0 @@\n-package x\n-func TestX(t *testing.T) {}\n"
	d := runProve(t, run.Snapshot{Proven: []run.DifferentialProof{reproDP("f1")}}, diff, mp)
	if len(d.Findings) != 0 || len(mp.gotProofs) != 0 {
		t.Fatalf("no proven pins → nothing to re-verify: findings %+v proofs %+v", d.Findings, mp.gotProofs)
	}
}

// A re-verification error aborts the run rather than being swallowed.
func TestProveTestDeletionRecheckErrorAborts(t *testing.T) {
	pin := pinDP("f1", "guard")
	mp := &mockProver{err: errors.New("verifier exploded")}
	r := mustNew(t, judge.NewMock(judge.Script{}), true)
	r.execs[Prove] = &proveExec{prover: mp, symptom: &fakeReviewer{decision: true}}
	ex, _ := r.Executor(Prove)
	diff := "diff --git a/x_test.go b/x_test.go\n--- a/x_test.go\n+++ /dev/null\n@@ -1,2 +0,0 @@\n-package x\n-func TestX(t *testing.T) {}\n"
	snap := run.Snapshot{Proven: []run.DifferentialProof{pin}}
	if _, err := ex.Execute(context.Background(), machine.ExecInput{Snap: snap, Node: proveNode, Diff: machine.Diff{Text: diff}, Audit: (&audits{}).fn}); err == nil {
		t.Fatal("a re-verification error must abort the run")
	}
}

func TestDeletesATestAndRemovedLine(t *testing.T) {
	yes := "diff --git a/x_test.go b/x_test.go\n--- a/x_test.go\n+++ b/x_test.go\n@@ -1,1 +0,0 @@\n-func TestFoo(t *testing.T) {}\n"
	if !deletesATest(yes) {
		t.Fatal("a removed test func must be detected")
	}
	for _, kind := range []string{"Benchmark", "Fuzz", "Example"} {
		if !deletesATest("@@\n-func " + kind + "Bar(") {
			t.Fatalf("a removed %s func must be detected", kind)
		}
	}
	// Go-legal forms that must be DETECTED: whitespace before the paren, an empty suffix (`Test`),
	// an underscore/Unicode suffix.
	for _, ok := range []string{"-func TestFoo (t *testing.T) {", "-func Test(t *testing.T){", "-func Test_x(t *testing.T){", "-func TestÜ(t *testing.T){", "-func TestMain(m *testing.M){"} {
		if !deletesATest(ok) {
			t.Fatalf("a Go-legal removed test decl must be detected: %q", ok)
		}
	}
	// Must NOT count: a lowercase suffix (not a test per Go's rule), a non-test func, the "---" header.
	for _, no := range []string{"-func Testhelper(t *testing.T) {", "-func helper() {}", "--- a/x_test.go"} {
		if deletesATest(no) {
			t.Fatalf("a non-test removed line must not count: %q", no)
		}
	}
}
