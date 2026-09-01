package run

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

// DeriveProofID returns the idempotent content id for a proof whose author left ID empty (the fix node
// elicits proofs but does not ask an agent to hand-compute a hash). It is pure and stable across
// machines/replays. A pin reuses PinID (its {finding,file,from,to} content hash, so the killed-mutant
// set in Snapshot.Proven dedupes exactly as before); a reproduction/deletion hashes its identifying
// content — the finding, kind, test, and (for a deletion) the removed span and file.
func DeriveProofID(p DifferentialProof) string {
	if p.Kind == ProofPin && p.Pin != nil {
		return PinID(p.Finding, p.Pin.File, p.Pin.From, p.Pin.To)
	}
	var delFile, delRemoved string
	if p.Deletes != nil {
		delFile, delRemoved = p.Deletes.File, p.Deletes.Removed
	}
	h := sha256.Sum256([]byte(p.Finding + "\x00" + p.Kind + "\x00" + p.Test + "\x00" + delFile + "\x00" + delRemoved))
	return fmt.Sprintf("%x", h[:16])
}

// DifferentialProof generalizes Pin (spec §9.1): a proof is "a test that changes outcome across
// two tree-states," and a mutate-a-line pin is only ONE form of it. The preferred form is a
// reproduction test (the real fault: fail on the pre-fix tree, pass on the post-fix tree); a
// deletion is proven the same way and names the removed span. A pin is the Kind=="pin" case, not a
// separate model — Delta.Pins/PinResults and Snapshot.Unproven all carry this general shape.
//
// The honest claim differs by form: a proven reproduction test says the BEHAVIOR is under test; a
// proven pin says the ADDED LINE is. Neither is "proof" that the fix is correct.
type DifferentialProof struct {
	// ID is the idempotent content hash of the proof — the reference/override key (was Pin.ID).
	// It is DifferentialProof.ID, never Pin.ID, because a reproduction/deletion proof has Pin==nil,
	// so keying an override on Pin.ID would leave those proofs unclearable (spec §9.1).
	ID string `json:"id"`
	// Finding is the confirmed-finding id this proof answers — the clear key for Snapshot.Unproven,
	// stable across a reworded fix exactly as Pin.Finding is.
	Finding string `json:"finding"`
	// Kind selects which payload is populated: exactly one of Pin/Deletes per the one-of invariant.
	Kind string `json:"kind"`
	// Test names the committed test in the reviewed diff for the reproduction/deletion forms (for
	// the report and a by-hand re-run). For a pin it is advisory and duplicated inside Pin.Test.
	Test string `json:"test,omitempty"`
	// Pin is set iff Kind=="pin": the {File,From,To} mutation to apply.
	Pin *Pin `json:"pin,omitempty"`
	// Deletes is set iff Kind=="deletion": the removed span and its durable anchor.
	Deletes *DeletionRef `json:"deletes,omitempty"`
}

// Proof kinds. Exactly one payload shape is populated for each (the one-of invariant).
const (
	ProofReproduction = "reproduction" // a committed test using the real fault; Pin and Deletes nil
	ProofPin          = "pin"          // a mutate-a-line pin; Pin set, Deletes nil
	ProofDeletion     = "deletion"     // a removed span proven fail-before/pass-after; Deletes set, Pin nil
)

// DeletionRef is the Kind=="deletion" payload: the removed span plus a durable, content-addressed
// anchor to where it still exists in history. The removed text is NOT a git blob (blobs are whole
// files), so a deletion carries both the whole-file blob (the container) and the exact removed span
// (its own identity) — spec §9.4.
type DeletionRef struct {
	File      string `json:"file"`                 // repo-relative path the code was removed from
	ParentSHA string `json:"parent_sha,omitempty"` // a commit where the pre-deletion file still exists
	FileBlob  string `json:"file_blob,omitempty"`  // git blob hash of the WHOLE ParentSHA:File
	Removed   string `json:"removed"`              // the exact removed span — the deletion's identity
}

// Valid enforces the one-of invariant (spec §9.1): exactly one payload is populated for each Kind.
// A reproduction proof carries neither Pin nor Deletes (the Test is the whole artifact); a pin
// carries a Pin only; a deletion carries a Deletes only. Any other combination — or an unknown
// Kind — is a malformed record that no engine can prove, and is rejected rather than silently
// accepted.
func (p DifferentialProof) Valid() error {
	switch p.Kind {
	case ProofPin:
		if p.Pin == nil {
			return fmt.Errorf("proof kind %q requires a pin", p.Kind)
		}
		if p.Deletes != nil {
			return fmt.Errorf("proof kind %q must not carry a deletion", p.Kind)
		}
	case ProofDeletion:
		if p.Deletes == nil {
			return fmt.Errorf("proof kind %q requires a deletion", p.Kind)
		}
		if p.Pin != nil {
			return fmt.Errorf("proof kind %q must not carry a pin", p.Kind)
		}
	case ProofReproduction:
		if p.Pin != nil || p.Deletes != nil {
			return fmt.Errorf("proof kind %q must carry neither a pin nor a deletion", p.Kind)
		}
	default:
		return fmt.Errorf("unknown proof kind %q", p.Kind)
	}
	return nil
}

// UnmarshalJSON decodes a proof and rejects a malformed one AT DECODE (spec §9.1). It refuses
// unknown fields — the same strictness fold.go's decoder applies to the rest of the wire — and then
// enforces the one-of invariant, so a bad record never reaches a downstream gate.
func (p *DifferentialProof) UnmarshalJSON(b []byte) error {
	type alias DifferentialProof
	var a alias
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil {
		return err
	}
	dp := DifferentialProof(a)
	if err := dp.Valid(); err != nil {
		return err
	}
	*p = dp
	return nil
}

// ProofResult generalizes PinResult: the same Proven/Outcome/Detail verdict, over a
// DifferentialProof of any kind rather than a bare Pin. Proven is the only value a gate accepts,
// true exactly when the differential held (break/fail-before fails and restore/pass-after passes).
// The embedded Proof.ID is the reference/override key.
type ProofResult struct {
	Proof   DifferentialProof `json:"proof"`
	Proven  bool              `json:"proven"`
	Outcome PinOutcome        `json:"outcome,omitempty"`
	Detail  string            `json:"detail,omitempty"`
	// FailBefore is the reproduction target test's pre-fix output — the assertion the §9.2 symptom
	// reviewer judges against the finding's own symptom. It is transient reviewer input consumed
	// inside the prove node, never persisted (json:"-"): on replay the reviewer's verdict already
	// lives in the recorded findings, so this need not survive the wire.
	FailBefore string `json:"-"`
}

// UnmarshalJSON rejects a contradictory result at decode: an unrecognised outcome, or a Proven flag
// that disagrees with the outcome. Proven is true exactly when the outcome is PinProven, so a record
// claiming proven:true with outcome:"survived" — or a bogus outcome PinOutcome.Valid() would reject
// — never reaches the persisted wire. Unknown fields are refused, as elsewhere on the wire.
func (r *ProofResult) UnmarshalJSON(b []byte) error {
	type alias ProofResult
	var a alias
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil {
		return err
	}
	pr := ProofResult(a)
	// The embedded proof must itself be valid. An OMITTED "proof" leaves a zero-value
	// DifferentialProof whose own UnmarshalJSON never ran, so the one-of invariant must be checked
	// here too — a result with no provable proof is meaningless.
	if err := pr.Proof.Valid(); err != nil {
		return fmt.Errorf("proof result carries an invalid proof: %w", err)
	}
	if pr.Outcome != "" && !pr.Outcome.Valid() {
		return fmt.Errorf("unknown proof outcome %q", pr.Outcome)
	}
	if pr.Proven != (pr.Outcome == PinProven) {
		return fmt.Errorf("proven=%v contradicts outcome %q", pr.Proven, pr.Outcome)
	}
	*r = pr
	return nil
}
