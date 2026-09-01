package kind

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/fsm/judge"
	"github.com/dsifry/metareview/internal/fsm/run"
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
