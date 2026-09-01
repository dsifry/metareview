package kind

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/fsm/judge"
	"github.com/dsifry/metareview/internal/fsm/machine"
	"github.com/dsifry/metareview/internal/fsm/run"
	"github.com/dsifry/metareview/internal/fsm/workflow"
)

// T1.2 regression — the #24 vacuous-pass. agentEdit.Reduce returned run.Delta{Commit: ...} and
// dropped everything else, so the proofs a fix declared never reached the Delta, and pins_proven
// (T1.3) then passed on evidence it never saw. A gate can only fail on what reaches it, so the seam
// MUST carry a fix's declared proofs end-to-end: Decode accepts them, Reduce propagates them to
// Delta.Pins. A Reduce that drops Pins reopens #24, so the mutation that drops them must redden this.
func TestAgentEditReduceCarriesProofsSoTheGateCanFail(t *testing.T) {
	r := mustNew(t, judge.NewMock(judge.Script{}), true)
	k, _ := r.Kind(AgentEdit)
	raw := `{"commit":"abc1234","summary":"s","pins":[` +
		`{"id":"i","finding":"f","kind":"pin","pin":{"id":"i","finding":"f","file":"a.go","from":"+x","to":"y","test":"T"}}]}`
	out, err := k.Decode(json.RawMessage(raw))
	if err != nil {
		t.Fatalf("a fix declaring a pin must decode: %v", err)
	}
	d, err := k.Reduce(run.Snapshot{}, out)
	if err != nil {
		t.Fatalf("reduce: %v", err)
	}
	if len(d.Pins) != 1 || d.Pins[0].Finding != "f" || d.Pins[0].Pin == nil || d.Pins[0].Pin.From != "+x" {
		t.Fatalf("the seam dropped the fix's declared proofs (the #24 vacuous-pass): %+v", d.Pins)
	}
	// The commit must still be carried — fixing the drop must not lose what already worked.
	if d.Commit != "abc1234" {
		t.Fatalf("the commit must still be carried: %q", d.Commit)
	}
}

// Gap #1: the fix node must ELICIT proofs, not just accept them. Its instructions and output schema
// must ask for the three differential-proof forms — without this a fix declares none and prove passes
// vacuously (#24) or blocks on an owed pin the agent was never told how to satisfy.
func TestAgentEditInstructionsElicitProofs(t *testing.T) {
	r := mustNew(t, judge.NewMock(judge.Script{}), true)
	ae, _ := r.Kind(AgentEdit)
	snap := run.Snapshot{AllFound: []run.Bug{{ID: "b1", Desc: "x"}}}
	ins, err := ae.Instructions(snap, &workflow.Node{Name: "fix"}, machine.Diff{}, "n")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"reproduction", "pin", "deletion", "finding", "pins"} {
		if !strings.Contains(ins.Text, want) {
			t.Fatalf("fix instructions must elicit proofs (missing %q):\n%s", want, ins.Text)
		}
	}
	if !strings.Contains(string(ins.OutputSchema), `"pins"`) {
		t.Fatalf("output schema must request pins: %s", ins.OutputSchema)
	}
	// The commit contract the driver relies on must survive the rewrite.
	if !strings.Contains(string(ins.OutputSchema), "^[0-9a-f]{7,40}$") || ins.Untrusted[0] != "unfixed_bugs" {
		t.Fatalf("the commit schema and unfixed_bugs untrusted input must be preserved: %+v", ins)
	}
}

// Reduce derives the ids the agent is told NOT to hand-compute, defaults a pin's Finding/Test from the
// proof level, and keeps a supplied id. A proofless fix keeps the nil-Pins shape.
func TestAgentEditReduceDerivesIDs(t *testing.T) {
	r := mustNew(t, judge.NewMock(judge.Script{}), true)
	k, _ := r.Kind(AgentEdit)
	// A pin with no id/finding/test on the pin payload: Reduce fills them and derives the id as PinID.
	raw := `{"commit":"abc1234","summary":"s","pins":[{"finding":"f","kind":"pin","test":"TestX","pin":{"file":"a.go","from":"guard","to":"broken"}}]}`
	out, err := k.Decode(json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	d, err := k.Reduce(run.Snapshot{}, out)
	if err != nil {
		t.Fatal(err)
	}
	p := d.Pins[0]
	wantID := run.PinID("f", "a.go", "guard", "broken")
	if p.ID != wantID || p.Pin.ID != wantID {
		t.Fatalf("pin ids must derive to PinID: proof=%q pin=%q want=%q", p.ID, p.Pin.ID, wantID)
	}
	if p.Pin.Finding != "f" || p.Pin.Test != "TestX" {
		t.Fatalf("pin Finding/Test must default from the proof level: %+v", p.Pin)
	}
	// A reproduction with no id: DeriveProofID gives a stable non-empty content id.
	repro := `{"commit":"abc1234","summary":"s","pins":[{"finding":"f2","kind":"reproduction","test":"TestY"}]}`
	out, _ = k.Decode(json.RawMessage(repro))
	d, _ = k.Reduce(run.Snapshot{}, out)
	if d.Pins[0].ID == "" || d.Pins[0].ID != run.DeriveProofID(run.DifferentialProof{Finding: "f2", Kind: run.ProofReproduction, Test: "TestY"}) {
		t.Fatalf("reproduction id must derive stably: %q", d.Pins[0].ID)
	}
	// A supplied id is kept (idempotent reference/override key).
	kept := `{"commit":"abc1234","summary":"s","pins":[{"id":"keepme","finding":"f","kind":"reproduction","test":"T"}]}`
	out, _ = k.Decode(json.RawMessage(kept))
	d, _ = k.Reduce(run.Snapshot{}, out)
	if d.Pins[0].ID != "keepme" {
		t.Fatalf("a supplied id must be kept: %q", d.Pins[0].ID)
	}
	// A proofless fix keeps nil Pins.
	out, _ = k.Decode(json.RawMessage(`{"commit":"abc1234","summary":"s"}`))
	d, _ = k.Reduce(run.Snapshot{}, out)
	if d.Pins != nil {
		t.Fatalf("a proofless fix must keep nil Pins, got %+v", d.Pins)
	}
	// A deletion proof's ParentSHA is filled from the snapshot's FixEntryHead (the pre-fix commit the
	// prover requires); the agent never supplies it. Without this every deletion proof is DOA.
	del := `{"commit":"abc1234","summary":"s","pins":[{"finding":"f","kind":"deletion","test":"T","deletes":{"file":"a.go","removed":"gone()"}}]}`
	out, _ = k.Decode(json.RawMessage(del))
	d, _ = k.Reduce(run.Snapshot{FixEntryHead: "prefixsha"}, out)
	if d.Pins[0].Deletes.ParentSHA != "prefixsha" {
		t.Fatalf("a deletion's ParentSHA must be filled from FixEntryHead: %q", d.Pins[0].Deletes.ParentSHA)
	}
	// ParentSHA has exactly one valid value (the pre-fix commit), so a stray agent-supplied value is
	// OVERWRITTEN authoritatively, not left to be rejected by the prover as malformed.
	delWrong := `{"commit":"abc1234","summary":"s","pins":[{"finding":"f","kind":"deletion","test":"T","deletes":{"file":"a.go","removed":"gone()","parent_sha":"wrongsha"}}]}`
	out, _ = k.Decode(json.RawMessage(delWrong))
	d, _ = k.Reduce(run.Snapshot{FixEntryHead: "prefixsha"}, out)
	if d.Pins[0].Deletes.ParentSHA != "prefixsha" {
		t.Fatalf("a supplied ParentSHA must be overwritten with FixEntryHead: %q", d.Pins[0].Deletes.ParentSHA)
	}
}

// Decode bounds and validates the declared proofs before they can reach the wire: a malformed proof
// (the one-of invariant violated) is rejected at decode, and more than MaxPins proofs is refused.
func TestAgentEditDecodeBoundsAndValidatesProofs(t *testing.T) {
	r := mustNew(t, judge.NewMock(judge.Script{}), true)
	k, _ := r.Kind(AgentEdit)

	// A malformed proof — kind "pin" with no pin payload — is rejected at decode.
	if _, err := k.Decode(json.RawMessage(`{"commit":"abc1234","summary":"s","pins":[{"kind":"pin"}]}`)); err == nil {
		t.Error("a malformed proof must be rejected at decode")
	}

	valid := run.DifferentialProof{Kind: run.ProofReproduction, Test: "t"}
	many := make([]run.DifferentialProof, run.MaxPins+1)
	for i := range many {
		many[i] = valid
	}
	over := string(run.MarshalCanonical(editOut{Commit: "abc1234", Summary: "s", Pins: many}))
	if _, err := k.Decode(json.RawMessage(over)); err == nil {
		t.Errorf("more than %d proofs must be refused", run.MaxPins)
	}
	atCap := string(run.MarshalCanonical(editOut{Commit: "abc1234", Summary: "s", Pins: many[:run.MaxPins]}))
	if _, err := k.Decode(json.RawMessage(atCap)); err != nil {
		t.Errorf("exactly %d proofs must be accepted: %v", run.MaxPins, err)
	}

	// The count and per-field caps are not enough: MaxPins pins each with per-field-valid but large
	// From/To aggregate past MaxPayload, so the canonical-size check must reject them before the wire
	// and the fold refuses them after the executor already reported success (the sibling Decodes' lesson).
	big := strings.Repeat("x", run.MaxText-10) // canonical ≤ MaxText, so per-field passes
	aggr := make([]run.DifferentialProof, run.MaxPins)
	for i := range aggr {
		aggr[i] = run.DifferentialProof{ID: "i", Finding: "f", Kind: run.ProofPin, Pin: &run.Pin{ID: "i", Finding: "f", File: "a.go", From: big, To: big, Test: "T"}}
	}
	fat := string(run.MarshalCanonical(editOut{Commit: "abc1234", Summary: "s", Pins: aggr}))
	if _, err := k.Decode(json.RawMessage(fat)); err == nil {
		t.Error("proofs aggregating over the payload budget must be refused at decode")
	}

	// A single over-cap FIELD (From just over MaxText) fits under the aggregate MaxPayload, so the
	// per-field caps must be applied at decode too — or it passes here and fails at fold, after the
	// executor already reported success.
	overField := &run.Pin{ID: "i", Finding: "f", File: "a.go", From: "+" + strings.Repeat("x", run.MaxText), To: "y", Test: "T"}
	perField := string(run.MarshalCanonical(editOut{Commit: "abc1234", Summary: "s", Pins: []run.DifferentialProof{{ID: "i", Finding: "f", Kind: run.ProofPin, Pin: overField}}}))
	if _, err := k.Decode(json.RawMessage(perField)); err == nil {
		t.Error("a proof with an over-cap per-field value must be refused at decode")
	}
}
