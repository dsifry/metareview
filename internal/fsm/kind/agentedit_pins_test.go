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

	// The count cap is not enough: a single pin can still carry enough content to blow the payload
	// budget, so the canonical-size check must reject it before it reaches the wire and the fold
	// refuses it after the executor already reported success (the sibling Decodes' lesson).
	huge := &run.Pin{ID: "i", Finding: "f", File: "a.go", From: "+" + strings.Repeat("x", run.MaxPayload), To: "y", Test: "T"}
	fat := string(run.MarshalCanonical(editOut{Commit: "abc1234", Summary: "s", Pins: []run.DifferentialProof{{ID: "i", Finding: "f", Kind: run.ProofPin, Pin: huge}}}))
	if _, err := k.Decode(json.RawMessage(fat)); err == nil {
		t.Error("a proof payload over the size budget must be refused at decode")
	}
}
