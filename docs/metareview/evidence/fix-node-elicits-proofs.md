# Evidence — the fix node elicits differential proofs (Gap #1 / the #24 vacuous-pass)

**Task:** close the linchpin gap surfaced during the end-to-end readiness review — the `agent-edit`
`fix` node never asked the agent for differential proofs, so on a real proof run the fix declared none
and `prove`/`pins_proven` either **passed vacuously** or **blocked on an owed pin the agent was never
told how to satisfy**. Everything in the pins/bug-class epic (T0.1–T2.2) and the language conventions
were dark code until this. **Base:** `main` (`49029b2`).

## The gap (verified in code, not argued)

`agentEdit.Instructions` said `Return ONLY {"commit":"<sha>","summary":"..."}` and its `OutputSchema`
was `{commit, summary}` — no `pins`. The `editOut` struct and `Decode` already accepted, bounded, and
validated a `Pins []DifferentialProof` (one-of, `MaxPins`, per-field caps), and `Reduce` propagated it
to `Delta.Pins` — the whole downstream is wired (`agentedit_pins_test.go`, T1.2). Only the elicitation
was missing, and the node's own comment named it "the #24 vacuous-pass, where pins_proven passed on
evidence it never saw."

## The fix (`internal/fsm/kind/kind.go`, `internal/fsm/run/proof.go`)

1. **`agentEdit.Instructions` now elicits a proof per fixed bug**, describing the three forms and their
   exact binding rules so a declared proof matches what the prover verifies:
   - **reproduction** (preferred): a test added/changed in the commit that fails (assertion, not
     compile) pre-fix and passes post-fix — `{kind:"reproduction", finding, test}`.
   - **pin**: for an added guard line — `{kind:"pin", finding, pin:{file, from, to, test}}`, where
     `from` is the exact text of a line the commit ADDED in that file (no leading `+`, appearing once)
     and `to` a compiling break — matching `isAddedLineInFile` + the mutation apply.
   - **deletion**: for removed code — `{kind:"deletion", finding, deletes:{file, removed}, test}`.
   - States the owed-pin rule (a fix adding a line in the bug's own file, in a package with tests, MUST
     carry a proof) and that a cross-file/no-test remedy owes none. `OutputSchema` now includes `pins`.
2. **Ids are derived, not hand-computed.** Asking an LLM to compute a stable sha256 content-hash id is
   unreliable, so the agent omits ids and `agentEdit.Reduce` fills them: a pin's id is `PinID(finding,
   file, from, to)` (so `Snapshot.Proven` — the §9.6 killed-mutant set — dedupes exactly as before), and
   a reproduction/deletion id is a new `run.DeriveProofID` content hash. Reduce also defaults a pin's
   own `Finding`/`Test` from the proof level, keeps any id the agent DID supply (idempotent
   reference/override key), and preserves the nil-`Pins` shape for a proofless fix.

## Preserved contracts
- The commit schema (`^[0-9a-f]{7,40}$`) and `unfixed_bugs` untrusted input are unchanged — the driver
  loop and `TestK5Instructions` still hold.
- `Decode` is untouched (it already validated declared proofs); `{commit, summary}`-only fixes still
  decode and reduce to nil `Pins`.

## Tests, coverage, mutation verification
- `internal/fsm/kind` and `internal/fsm/run` remain **100.0%**.
- New tests: `TestAgentEditInstructionsElicitProofs` (the instructions/schema ask for all three forms +
  `pins`, and preserve the commit/`unfixed_bugs` contract); `TestAgentEditReduceDerivesIDs` (pin id →
  `PinID`, pin Finding/Test defaulting, reproduction id → stable `DeriveProofID`, a supplied id kept, a
  proofless fix keeps nil `Pins`); `TestDeriveProofID` (pin→`PinID`, nil-payload non-panic, reproduction
  stable, deletion distinguishes the removed span).
- **Mutation-verified** (file-backup, line-targeted, re-run): all killed — the instructions/schema
  dropping `pins` (the linchpin regression), Reduce skipping proof-id or pin-id derivation, and
  `DeriveProofID`'s pin branch returning the hash instead of `PinID`. Tree confirmed clean.
- `gofmt`/`go vet` clean; full `go test ./...` green.

## Shepherding round 1 (Cursor Bugbot — 1× High, fixed)

- **Deletion proofs omitted the required `ParentSHA`.** The elicitation asks for a deletion as
  `{file, removed}`, but `proveDeletion` rejects any deletion whose `DeletionRef.ParentSHA` is not the
  fix's pre-fix commit — so every declared deletion would have been DOA (malformed). Fixed: `Reduce`
  now fills `Deletes.ParentSHA` from `Snapshot.FixEntryHead` (set on entry to the fix node; the same
  value `prove` uses as `PreFixSHA`), a machine-known SHA the agent cannot name. It is not part of
  `DeriveProofID`, so filling it does not shift the proof id. Regression test + mutation-verified.
- (Also: a CI-only staticcheck SA4000 in a test's stability check, fixed.)

## What this unlocks
A real `sdlc-loop-proved` run can now actually declare and verify proofs at the fix step — the
prerequisite for a meaningful end-to-end shakedown on a live repo (the next step). It does not by itself
address the Go-hardwired/undocumented proved workflow or the `--base` footgun (separate follow-ups).
