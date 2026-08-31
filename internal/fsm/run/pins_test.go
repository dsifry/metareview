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
	s := Snapshot{Unproven: []Pin{{ID: "a", Finding: "f1"}}}
	c := s.Clone()
	c.Unproven[0].ID = "mutated"
	if s.Unproven[0].ID != "a" {
		t.Error("Clone must deep-copy Unproven, not alias it")
	}
}
