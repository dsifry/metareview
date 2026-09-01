# Evidence — Pin.ID/Finding consistency (CodeRabbit follow-up to #51)

**Task:** the one substantive CodeRabbit finding on the merged Gap #1 PR (#51). CodeRabbit reviewed
late (it had been in manual-trigger mode / recovering from load) and flagged `internal/fsm/kind/
kind.go` (Functional Correctness, Minor). **Base:** `main` (`bfa0b2e`).

## The finding (valid)

In `agentEdit.Reduce`, a pin's nested `pin.finding` was only *defaulted* from the outer proof
`finding` when empty, but `Pin.ID` was then derived from the **outer** `p.Finding`. So if a pin
supplied a nested `pin.finding` that differed from the outer one, `Pin.Finding` kept the nested value
while `Pin.ID` hashed the outer — the id could disagree with the `Finding` field it is documented to
hash (`PinID` = hash of the pin's own `{Finding,File,From,To}`). An edge case (the fix-node
instructions don't ask for a nested finding), but a real inconsistency.

## The fix

The outer proof `Finding` is the canonical chain link — the key `Snapshot.Unproven` clears by — so it
is now **authoritative** over a nested `pin.finding`: `Reduce` sets `p.Pin.Finding = p.Finding`
(overwrite, not default-when-empty) and derives `Pin.ID = PinID(p.Pin.Finding, …)`. A stray nested
finding can no longer make the id hash a different value than the field it names. This mirrors the
authoritative-`ParentSHA` decision from #51's round 2 (a machine-canonical field is set, not merely
defaulted).

## Tests, coverage, mutation verification

- `internal/fsm/kind` stays **100.0%**.
- New regression case in `TestAgentEditReduceDerivesIDs`: a pin with `finding:"outer"` and a nested
  `pin.finding:"nested"` reduces to `Pin.Finding == "outer"` and `Pin.ID == proof.ID ==
  PinID("outer", …)` — exactly the divergence CodeRabbit asked to be covered.
- **Mutation-verified**: reverting to default-when-empty (the pre-fix behavior) reddens the new test.
- `gofmt`/`go vet`/golangci-lint clean; full `go test ./...` green.

## Not included
CodeRabbit's other #51 finding — MD022 (Markdown headings need surrounding blank lines) across four
generated/evidence `.md` files — is a cosmetic markdown-lint nit with no correctness impact; the proper
fix is in the artifact generator and is left as an optional cleanup, not bundled here.
