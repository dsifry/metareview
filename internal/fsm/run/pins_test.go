package run

import "testing"

// pinProof is a Kind:"pin" DifferentialProof for a given finding id.
func pinProof(finding string) DifferentialProof {
	return DifferentialProof{ID: "p-" + finding, Finding: finding, Kind: ProofPin, Pin: &Pin{ID: "p-" + finding, Finding: finding, File: "a.go", From: "+x", To: "y", Test: "T"}}
}

// provenFor is a Proven ProofResult for a given finding id.
func provenFor(finding string) ProofResult {
	return ProofResult{Proof: pinProof(finding), Proven: true, Outcome: PinProven}
}

// unprovenFindings lists the finding ids currently open in Unproven, in order.
func unprovenFindings(s Snapshot) []string {
	var out []string
	for _, p := range s.Unproven {
		out = append(out, p.Finding)
	}
	return out
}

// The Unproven gap lifecycle (spec §2.4 R4): a fix's declared proof ADDs its Finding gap; a later
// Proven prove-result CLEARs it; a re-declared proof for a regressed finding RE-ADDs it — and a
// re-added gap is NOT cleared against the stale historical Proven (only a NEW prove result clears).
func TestFoldUnprovenLifecycle(t *testing.T) {
	b := NewBuilder(runA)
	b.Init(baseInit())
	// [1,2] fix declares proofs for f1 and f2 → both gaps enter Unproven (ADD)
	b.Event(TypeNodeOutput, out(`{}`), WithNode("fix"))
	b.Event(TypeDeltaApplied, deltaFor(`{}`, Delta{Pins: []DifferentialProof{pinProof("f1"), pinProof("f2")}}), WithNode("fix"))
	// [3,4] prove proves f1 → f1 clears, f2 remains (CLEAR)
	b.Event(TypeNodeOutput, out(`{}`), WithNode("prove"))
	b.Event(TypeDeltaApplied, deltaFor(`{}`, Delta{PinResults: []ProofResult{provenFor("f1")}}), WithNode("prove"))
	// [5,6] a later fix re-declares a proof for f1 (the fix regressed) → f1 re-enters (RE-ADD), and it
	// must NOT be auto-cleared by the earlier Proven result for f1.
	b.Event(TypeNodeOutput, out(`{}`), WithNode("fix2"))
	b.Event(TypeDeltaApplied, deltaFor(`{}`, Delta{Pins: []DifferentialProof{pinProof("f1")}}), WithNode("fix2"))
	evs := b.Events()

	eq := func(prefix int, want ...string) {
		got := unprovenFindings(mustFold(t, evs[:prefix]))
		if len(got) != len(want) {
			t.Fatalf("after %d events, Unproven=%v want %v", prefix, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("after %d events, Unproven=%v want %v", prefix, got, want)
			}
		}
	}
	eq(3, "f1", "f2") // ADD
	eq(5, "f2")       // CLEAR f1
	eq(7, "f2", "f1") // RE-ADD f1 (not cleared by the stale historical Proven)
}

// provenIDs lists the proof ids currently in Proven, in order.
func provenIDs(s Snapshot) []string {
	var out []string
	for _, p := range s.Proven {
		out = append(out, p.ID)
	}
	return out
}

// The Proven set (the run's killed-mutant set for §9.6): a Proven prove-result ADDs its proof; a
// non-Proven result adds nothing; re-proving the same proof id dedupes (replaces in place, never a
// duplicate); a distinct proof appends in temporal order.
func TestFoldProvenLifecycle(t *testing.T) {
	b := NewBuilder(runA)
	b.Init(baseInit())
	b.Event(TypeNodeOutput, out(`{}`), WithNode("fix"))
	b.Event(TypeDeltaApplied, deltaFor(`{}`, Delta{Pins: []DifferentialProof{pinProof("f1"), pinProof("f2")}}), WithNode("fix"))
	// prove: f1 proven, f2 survives (not proven) → Proven has only p-f1.
	b.Event(TypeNodeOutput, out(`{}`), WithNode("prove"))
	b.Event(TypeDeltaApplied, deltaFor(`{}`, Delta{PinResults: []ProofResult{provenFor("f1"), {Proof: pinProof("f2"), Proven: false, Outcome: PinSurvived}}}), WithNode("prove"))
	// a later round re-proves f1 (same id → dedupe) and proves a new f3 (append).
	b.Event(TypeNodeOutput, out(`{}`), WithNode("prove2"))
	b.Event(TypeDeltaApplied, deltaFor(`{}`, Delta{PinResults: []ProofResult{provenFor("f1"), provenFor("f3")}}), WithNode("prove2"))
	evs := b.Events()

	eq := func(prefix int, want ...string) {
		got := provenIDs(mustFold(t, evs[:prefix]))
		if len(got) != len(want) {
			t.Fatalf("after %d events, Proven=%v want %v", prefix, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("after %d events, Proven=%v want %v", prefix, got, want)
			}
		}
	}
	eq(3)                 // nothing proven yet
	eq(5, "p-f1")         // f1 proven; f2 survived → not added
	eq(7, "p-f1", "p-f3") // f1 re-proven (deduped in place); f3 appended
}

// A non-Proven prove-result must NOT clear the gap (only Proven closes it), and re-declaring a proof
// for an already-open finding keeps a SINGLE gap, never a duplicate.
func TestFoldUnprovenGuards(t *testing.T) {
	b := NewBuilder(runA)
	b.Init(baseInit())
	b.Event(TypeNodeOutput, out(`{}`), WithNode("fix"))
	b.Event(TypeDeltaApplied, deltaFor(`{}`, Delta{Pins: []DifferentialProof{pinProof("f1")}}), WithNode("fix"))
	// a survived (non-Proven) result must leave f1 open
	b.Event(TypeNodeOutput, out(`{}`), WithNode("prove"))
	b.Event(TypeDeltaApplied, deltaFor(`{}`, Delta{PinResults: []ProofResult{{Proof: pinProof("f1"), Proven: false, Outcome: PinSurvived}}}), WithNode("prove"))
	// re-declaring f1 must keep a single gap, not a second copy
	b.Event(TypeNodeOutput, out(`{}`), WithNode("fix2"))
	b.Event(TypeDeltaApplied, deltaFor(`{}`, Delta{Pins: []DifferentialProof{pinProof("f1")}}), WithNode("fix2"))
	evs := b.Events()
	// After the survived result (before any re-declare), f1 must STILL be open — a non-Proven result
	// must not clear it. Asserted at this prefix so a later re-add cannot mask an erroneous clear.
	if got := unprovenFindings(mustFold(t, evs[:5])); len(got) != 1 || got[0] != "f1" {
		t.Fatalf("a survived (non-Proven) result must not clear the gap: %v", got)
	}
	if got := unprovenFindings(mustFold(t, evs)); len(got) != 1 || got[0] != "f1" {
		t.Fatalf("re-declaring an already-open finding must not duplicate the gap: %v", got)
	}
}

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

// ProofCategoryBlocks marks the two blocking prove outcomes (unproven fix, unverifiable tree) and
// leaves the advisory malformed pin and everything else non-blocking — the shared rule the gate and
// the prove node both key on.
func TestProofCategoryBlocks(t *testing.T) {
	for cat, want := range map[string]bool{
		CategoryUnprovenFix:  true,
		CategoryUnverifiable: true,
		CategoryMalformedPin: false,
		"":                   false,
		"something-else":     false,
	} {
		if ProofCategoryBlocks(cat) != want {
			t.Errorf("ProofCategoryBlocks(%q) = %v, want %v", cat, ProofCategoryBlocks(cat), want)
		}
	}
}
