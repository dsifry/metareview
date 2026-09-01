package run

import (
	"encoding/json"
	"testing"
)

// The one-of invariant is the whole reason DifferentialProof exists as a tagged union rather than
// three separate structs: exactly one payload shape is populated for each Kind, and a record that
// violates it is meaningless — a "pin" with no pin to mutate, or a "reproduction" carrying a stray
// deletion, cannot be proven by any engine. Valid() is the predicate every decode path and every
// gate leans on, so each Kind × payload combination is pinned here.
func TestDifferentialProofOneOfInvariant(t *testing.T) {
	pin := &Pin{ID: "p", Finding: "f", File: "a.go", From: "+x", To: "y", Test: "T"}
	del := &DeletionRef{File: "a.go", ParentSHA: "sha", FileBlob: "blob", Removed: "gone"}
	for _, tc := range []struct {
		name string
		dp   DifferentialProof
		ok   bool
	}{
		{"pin populated", DifferentialProof{Kind: ProofPin, Pin: pin}, true},
		{"deletion populated", DifferentialProof{Kind: ProofDeletion, Deletes: del}, true},
		{"reproduction bare", DifferentialProof{Kind: ProofReproduction, Test: "T"}, true},

		{"pin missing its payload", DifferentialProof{Kind: ProofPin}, false},
		{"pin carrying a deletion", DifferentialProof{Kind: ProofPin, Pin: pin, Deletes: del}, false},
		{"deletion missing its payload", DifferentialProof{Kind: ProofDeletion}, false},
		{"deletion carrying a pin", DifferentialProof{Kind: ProofDeletion, Deletes: del, Pin: pin}, false},
		{"reproduction carrying a pin", DifferentialProof{Kind: ProofReproduction, Pin: pin}, false},
		{"reproduction carrying a deletion", DifferentialProof{Kind: ProofReproduction, Deletes: del}, false},
		{"unknown kind", DifferentialProof{Kind: "bogus", Pin: pin}, false},
		{"empty kind", DifferentialProof{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.dp.Valid()
			if (err == nil) != tc.ok {
				t.Fatalf("Valid() = %v, want ok=%v", err, tc.ok)
			}
		})
	}
}

// A malformed record must be rejected AT DECODE, not silently accepted and left for a downstream
// gate to trip over (spec §9.1: "enforced at decode"). The custom UnmarshalJSON is the gate: a
// one-of violation and an unknown field both fail the decode.
func TestDifferentialProofDecodeRejectsMalformed(t *testing.T) {
	good := `{"id":"i","finding":"f","kind":"pin","pin":{"id":"i","finding":"f","file":"a.go","from":"+x","to":"y","test":"T"}}`
	var dp DifferentialProof
	if err := json.Unmarshal([]byte(good), &dp); err != nil {
		t.Fatalf("a valid pin proof must decode: %v", err)
	}
	if dp.Kind != ProofPin || dp.Pin == nil || dp.Pin.From != "+x" {
		t.Fatalf("decoded proof lost its payload: %+v", dp)
	}
	for _, tc := range []struct {
		name string
		raw  string
	}{
		{"pin kind but a deletes payload", `{"kind":"pin","pin":{"file":"a.go"},"deletes":{"file":"a.go","removed":"x"}}`},
		{"pin kind with no payload", `{"kind":"pin"}`},
		{"reproduction carrying a pin", `{"kind":"reproduction","pin":{"file":"a.go"}}`},
		{"deletion with no payload", `{"kind":"deletion"}`},
		{"unknown kind", `{"kind":"bogus"}`},
		{"unknown field", `{"kind":"reproduction","test":"T","bogus":1}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var d DifferentialProof
			if err := json.Unmarshal([]byte(tc.raw), &d); err == nil {
				t.Errorf("a malformed proof must be rejected at decode, got %+v", d)
			}
		})
	}
}

// ProofResult generalizes PinResult: the same Proven / Outcome / Detail verdict, but over a
// DifferentialProof of ANY kind rather than a bare Pin. Proven must agree with a PinProven outcome
// and the embedded proof must survive the round-trip.
func TestProofResultRoundTripsAndCarriesTheGeneralProof(t *testing.T) {
	pr := ProofResult{
		Proof:   DifferentialProof{ID: "i", Finding: "f", Kind: ProofReproduction, Test: "T"},
		Proven:  true,
		Outcome: PinProven,
		Detail:  "fail-before/pass-after held",
	}
	b := MarshalCanonical(pr)
	var got ProofResult
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("round-trip: %v", err)
	}
	if got.Proof.Kind != ProofReproduction || got.Proof.Test != "T" {
		t.Errorf("the general proof was not carried: %+v", got.Proof)
	}
	if !got.Proven || got.Outcome != PinProven {
		t.Errorf("the verdict was not carried: %+v", got)
	}
}

// A contradictory result — proven disagreeing with the outcome, or an unrecognised outcome — must be
// rejected at decode, so it never reaches the persisted wire (Proven is true iff Outcome==PinProven).
func TestProofResultDecodeRejectsContradiction(t *testing.T) {
	good := []string{
		`{"proof":{"kind":"reproduction","test":"T"},"proven":true,"outcome":"proven"}`,
		`{"proof":{"kind":"reproduction","test":"T"},"proven":false,"outcome":"survived"}`,
		`{"proof":{"kind":"reproduction","test":"T"},"proven":false}`, // no outcome, not proven
	}
	for _, s := range good {
		var r ProofResult
		if err := json.Unmarshal([]byte(s), &r); err != nil {
			t.Errorf("a consistent result must decode: %v (%s)", err, s)
		}
	}
	bad := []string{
		`{"proof":{"kind":"reproduction","test":"T"},"proven":true,"outcome":"survived"}`, // proven≠outcome
		`{"proof":{"kind":"reproduction","test":"T"},"proven":false,"outcome":"proven"}`,  // proven≠outcome
		`{"proof":{"kind":"reproduction","test":"T"},"proven":true}`,                      // proven but no proven outcome
		`{"proof":{"kind":"reproduction","test":"T"},"proven":false,"outcome":"bogus"}`,   // unknown outcome
		`{"proof":{"kind":"reproduction","test":"T"},"proven":false,"zzz":1}`,             // unknown field
		`{"proven":false}`,                        // no proof at all → invalid embedded proof
		`{"proof":{"kind":"pin"},"proven":false}`, // proof kind:pin with no pin payload
	}
	for _, s := range bad {
		var r ProofResult
		if err := json.Unmarshal([]byte(s), &r); err == nil {
			t.Errorf("a contradictory/invalid result must be rejected: %s", s)
		}
	}
}

// DeriveProofID fills the id the fix node's agent is told not to hand-compute. A pin reuses PinID so
// the killed-mutant set dedupes as before; a reproduction/deletion hashes its identifying content.
func TestDeriveProofID(t *testing.T) {
	pin := DifferentialProof{Finding: "f", Kind: ProofPin, Pin: &Pin{File: "a.go", From: "g", To: "b"}}
	if got, want := DeriveProofID(pin), PinID("f", "a.go", "g", "b"); got != want {
		t.Fatalf("a pin id must equal PinID: %q != %q", got, want)
	}
	// A pin with a nil payload (never valid, but must not panic) falls through to the content hash.
	if DeriveProofID(DifferentialProof{Finding: "f", Kind: ProofPin}) == "" {
		t.Fatal("a pin with nil payload must still derive a non-empty id")
	}
	repro := DifferentialProof{Finding: "f", Kind: ProofReproduction, Test: "T"}
	id := DeriveProofID(repro)
	if id == "" || DeriveProofID(repro) != id {
		t.Fatal("a reproduction id must be non-empty and stable")
	}
	// A deletion folds its file+removed span into the id, so two deletions differing only there differ.
	del1 := DifferentialProof{Finding: "f", Kind: ProofDeletion, Test: "T", Deletes: &DeletionRef{File: "a.go", Removed: "x"}}
	del2 := DifferentialProof{Finding: "f", Kind: ProofDeletion, Test: "T", Deletes: &DeletionRef{File: "a.go", Removed: "y"}}
	if DeriveProofID(del1) == "" || DeriveProofID(del1) == DeriveProofID(del2) {
		t.Fatal("a deletion id must be non-empty and distinguish the removed span")
	}
}
