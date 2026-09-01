package run

import (
	"strings"
	"testing"
)

// The proof carriers Delta.Pins / Delta.PinResults are persisted (§2.4), so every per-field and
// per-list cap of §2.3 must apply to them exactly as it does to Findings/Confirmed/Status —
// otherwise an oversized proof enters the audit store and no shell test could tell (the same blind
// spot TestWithinCapsGuardsEverySandboxEvidenceField documents). withinCaps is the sole predicate
// that stops it, so each field it must guard has to be able to fail it.
func TestWithinCapsGuardsProofFields(t *testing.T) {
	long := strings.Repeat("x", MaxShort+1)
	bigText := strings.Repeat("x", MaxText+1)
	bigDetail := strings.Repeat("x", MaxDetail+1)

	okPin := &Pin{ID: "i", Finding: "f", File: "a.go", From: "+x", To: "y", Test: "T"}
	base := &DeltaAppliedData{Delta: Delta{
		Pins:       []DifferentialProof{{ID: "i", Finding: "f", Kind: ProofPin, Pin: okPin}},
		PinResults: []ProofResult{{Proof: DifferentialProof{ID: "i", Finding: "f", Kind: ProofReproduction, Test: "T"}, Outcome: PinProven, Proven: true}},
	}}
	if !withinCaps(base) {
		t.Fatal("the baseline proof record must be admissible")
	}

	for _, tc := range []struct {
		field string
		set   func(*DeltaAppliedData)
	}{
		{"proof.ID", func(d *DeltaAppliedData) { d.Pins[0].ID = long }},
		{"proof.Finding", func(d *DeltaAppliedData) { d.Pins[0].Finding = long }},
		{"proof.Kind", func(d *DeltaAppliedData) { d.Pins[0].Kind = long }},
		{"proof.Test", func(d *DeltaAppliedData) { d.Pins[0].Test = long }},
		{"pin.File", func(d *DeltaAppliedData) { d.Pins[0].Pin.File = long }},
		{"pin.From", func(d *DeltaAppliedData) { d.Pins[0].Pin.From = bigText }},
		{"pin.To", func(d *DeltaAppliedData) { d.Pins[0].Pin.To = bigText }},
		{"deletes.File", func(d *DeltaAppliedData) {
			d.Pins[0] = DifferentialProof{Kind: ProofDeletion, Deletes: &DeletionRef{File: long, Removed: "x"}}
		}},
		{"deletes.Removed", func(d *DeltaAppliedData) {
			d.Pins[0] = DifferentialProof{Kind: ProofDeletion, Deletes: &DeletionRef{File: "a.go", Removed: bigText}}
		}},
		{"result.proof.ID", func(d *DeltaAppliedData) { d.PinResults[0].Proof.ID = long }},
		{"result.Detail", func(d *DeltaAppliedData) { d.PinResults[0].Detail = bigDetail }},
	} {
		t.Run(tc.field, func(t *testing.T) {
			d := &DeltaAppliedData{Delta: Delta{
				Pins:       []DifferentialProof{{ID: "i", Finding: "f", Kind: ProofPin, Pin: &Pin{ID: "i", Finding: "f", File: "a.go", From: "+x", To: "y", Test: "T"}}},
				PinResults: []ProofResult{{Proof: DifferentialProof{ID: "i", Finding: "f", Kind: ProofReproduction, Test: "T"}, Outcome: PinProven, Proven: true}},
			}}
			tc.set(d)
			if withinCaps(d) {
				t.Errorf("an oversized %s was admitted to the audit store", tc.field)
			}
		})
	}
}
