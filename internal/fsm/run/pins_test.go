package run

import "testing"

func TestPinIDIsIdempotentAndContentDerived(t *testing.T) {
	a := PinID("f1", "a.go", "x", "y")
	if a != PinID("f1", "a.go", "x", "y") {
		t.Error("same content must yield the same id")
	}
	// Any component change changes the id.
	for _, other := range []string{PinID("f2", "a.go", "x", "y"), PinID("f1", "b.go", "x", "y"), PinID("f1", "a.go", "z", "y"), PinID("f1", "a.go", "x", "z")} {
		if other == a {
			t.Errorf("a different pin must not share the id %s", a)
		}
	}
	if len(a) != 32 {
		t.Errorf("id should be 128 bits of hex, got %d chars", len(a))
	}
}

func TestPinOutcomeValid(t *testing.T) {
	for _, o := range []PinOutcome{PinProven, PinSurvived, PinMalformed, PinUnverifiable} {
		if !o.Valid() {
			t.Errorf("%q must be valid", o)
		}
	}
	for _, o := range []PinOutcome{"", "nope", "PROVEN"} {
		if PinOutcome(o).Valid() {
			t.Errorf("%q must not be valid", o)
		}
	}
}

func TestSnapshotCloneCopiesUnproven(t *testing.T) {
	s := Snapshot{Unproven: []DifferentialProof{{ID: "a", Finding: "f1", Kind: ProofPin, Pin: &Pin{ID: "a", From: "+x"}}}}
	c := s.Clone()
	c.Unproven[0].ID = "mutated"
	if s.Unproven[0].ID != "a" {
		t.Error("Clone must deep-copy Unproven, not alias the slice")
	}
	// The Pin pointer must be fresh too: a shallow slice copy would share it, and a mutation
	// through the clone would reach the original's payload.
	c.Unproven[0].Pin.From = "+mutated"
	if s.Unproven[0].Pin.From != "+x" {
		t.Error("Clone must deep-copy the proof's Pin pointer, not alias it")
	}

	// The Deletes pointer of a deletion proof must be fresh for the same reason.
	d := Snapshot{Unproven: []DifferentialProof{{ID: "b", Finding: "f2", Kind: ProofDeletion, Deletes: &DeletionRef{File: "a.go", Removed: "gone"}}}}
	dc := d.Clone()
	dc.Unproven[0].Deletes.Removed = "mutated"
	if d.Unproven[0].Deletes.Removed != "gone" {
		t.Error("Clone must deep-copy the proof's Deletes pointer, not alias it")
	}
}
