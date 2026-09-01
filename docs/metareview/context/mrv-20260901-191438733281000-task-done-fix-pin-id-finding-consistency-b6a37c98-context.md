# metareview task-done context

Run ID: `mrv-20260901-191438733281000-task-done-fix-pin-id-finding-consistency-b6a37c98`

## Task

Advisory task target: fix-pin-id-finding-consistency

## Git

- Base: `bfa0b2ec19b747fe9d900a455b8b873e24913855`
- Head: `2f1c4986eaf0a58cdc29c79c5d4fd558c41afcf7`
- Branch: `fix-pin-id-finding-consistency`
- Gate effect: `gate`

## Context Profile

- Raw diff bytes: `5369`
- Filtered diff bytes: `2770`
- Risk level: `none`
- Generated files excluded: docs/metareview/evidence/fix-pin-id-finding-consistency.md

## Context Shard Plan

Not sharded.

## Review Manifest

- Manifest verdict: `PASS`
- Source manifest hash: not sharded
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- internal/fsm/kind/agentedit_pins_test.go
- internal/fsm/kind/kind.go

### Local changes (not sharded)
- .claude/worktrees/agent-af9d648e34ca9450a/

### Path Dispositions
- docs/metareview/evidence/fix-pin-id-finding-consistency.md: generated (metareview generated review artifact excluded from source manifest)

### Manifest Blockers
No manifest blockers.

## Changed Files

- internal/fsm/kind/agentedit_pins_test.go
- internal/fsm/kind/kind.go
- .claude/worktrees/agent-af9d648e34ca9450a/

## Diff

```diff
diff --git a/internal/fsm/kind/agentedit_pins_test.go b/internal/fsm/kind/agentedit_pins_test.go
index 27880d4..670dbb6 100644
--- a/internal/fsm/kind/agentedit_pins_test.go
+++ b/internal/fsm/kind/agentedit_pins_test.go
@@ -86,6 +86,23 @@ func TestAgentEditReduceDerivesIDs(t *testing.T) {
 	if p.Pin.Finding != "f" || p.Pin.Test != "TestX" {
 		t.Fatalf("pin Finding/Test must default from the proof level: %+v", p.Pin)
 	}
+	// A nested pin.finding that DIFFERS from the outer finding must not survive: the outer is
+	// authoritative, so Pin.Finding is overwritten to it and Pin.ID hashes that same value — the id can
+	// never disagree with the field it names (CodeRabbit #51).
+	diverge := `{"commit":"abc1234","summary":"s","pins":[{"finding":"outer","kind":"pin","test":"T","pin":{"finding":"nested","file":"a.go","from":"g","to":"b"}}]}`
+	out, err = k.Decode(json.RawMessage(diverge))
+	if err != nil {
+		t.Fatal(err)
+	}
+	d, _ = k.Reduce(run.Snapshot{}, out)
+	pd := d.Pins[0]
+	if pd.Pin.Finding != "outer" {
+		t.Fatalf("the outer finding must be authoritative over a nested one: %q", pd.Pin.Finding)
+	}
+	if want := run.PinID("outer", "a.go", "g", "b"); pd.Pin.ID != want || pd.ID != want {
+		t.Fatalf("Pin.ID must hash the (authoritative) finding it names: proof=%q pin=%q want=%q", pd.ID, pd.Pin.ID, want)
+	}
+
 	// A reproduction with no id: DeriveProofID gives a stable non-empty content id.
 	repro := `{"commit":"abc1234","summary":"s","pins":[{"finding":"f2","kind":"reproduction","test":"TestY"}]}`
 	out, _ = k.Decode(json.RawMessage(repro))
diff --git a/internal/fsm/kind/kind.go b/internal/fsm/kind/kind.go
index 4eb35cd..f813373 100644
--- a/internal/fsm/kind/kind.go
+++ b/internal/fsm/kind/kind.go
@@ -734,14 +734,16 @@ func (agentEdit) Reduce(s run.Snapshot, out any) (run.Delta, error) {
 	pins := make([]run.DifferentialProof, len(o.Pins))
 	for i, p := range o.Pins {
 		if p.Kind == run.ProofPin && p.Pin != nil {
-			if p.Pin.Finding == "" {
-				p.Pin.Finding = p.Finding
-			}
+			// The outer proof Finding is the canonical chain link (what Snapshot.Unproven clears by), so
+			// it is AUTHORITATIVE over a nested pin.finding — set, never merely defaulted. This keeps
+			// Pin.ID (the hash of the pin's OWN {Finding,File,From,To}) consistent with Pin.Finding: a
+			// stray nested finding cannot make the id hash a different value than the field it names.
+			p.Pin.Finding = p.Finding
 			if p.Pin.Test == "" {
 				p.Pin.Test = p.Test
 			}
 			if p.Pin.ID == "" {
-				p.Pin.ID = run.PinID(p.Finding, p.Pin.File, p.Pin.From, p.Pin.To)
+				p.Pin.ID = run.PinID(p.Pin.Finding, p.Pin.File, p.Pin.From, p.Pin.To)
 			}
 		}
 		// A deletion's ParentSHA is the fix's pre-fix commit (FixEntryHead, set on entry to the fix



```

## Knowledge And Registries

Service inventory: none

No service inventory found.

Knowledge facts:

No Beads knowledge facts found.

## Evidence

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

