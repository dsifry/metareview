# metareview task-done context

Run ID: `mrv-20260901-203559273493000-task-done-fix-proof-binds-fix-diff-3ad76b46`

## Task

Advisory task target: fix-proof-binds-fix-diff

## Git

- Base: `9291168e55036cd1f7e799f75e589242605d73f5`
- Head: `6006c9a04264d2917d4c29f4a1c2f0819e375123`
- Branch: `fix-proof-binds-fix-diff`
- Gate effect: `gate`

## Context Profile

- Raw diff bytes: `9198`
- Filtered diff bytes: `5345`
- Risk level: `none`
- Generated files excluded: docs/metareview/evidence/fix-proof-binds-fix-diff.md

## Context Shard Plan

Not sharded.

## Review Manifest

- Manifest verdict: `PASS`
- Source manifest hash: not sharded
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- internal/fsm/kind/prove.go
- internal/fsm/machine/machine.go
- internal/fsm/machine/machine_test.go
- internal/fsm/workflow/workflow.go

### Local changes (not sharded)
- .claude/worktrees/agent-af9d648e34ca9450a/

### Path Dispositions
- docs/metareview/evidence/fix-proof-binds-fix-diff.md: generated (metareview generated review artifact excluded from source manifest)

### Manifest Blockers
No manifest blockers.

## Changed Files

- internal/fsm/kind/prove.go
- internal/fsm/machine/machine.go
- internal/fsm/machine/machine_test.go
- internal/fsm/workflow/workflow.go
- .claude/worktrees/agent-af9d648e34ca9450a/

## Diff

```diff
diff --git a/internal/fsm/kind/prove.go b/internal/fsm/kind/prove.go
index 98b2426..5aa4f9f 100644
--- a/internal/fsm/kind/prove.go
+++ b/internal/fsm/kind/prove.go
@@ -64,7 +64,9 @@ type proveKind struct{}
 
 func (proveKind) Name() string { return Prove }
 func (proveKind) Info() workflow.KindInfo {
-	return workflow.KindInfo{DefaultExec: "fork", AllowedExec: []string{"fork"}, ValidateParams: validateProve}
+	// FixScopedDiff: prove binds its pins and owed-pin check against the FIX's diff (FixEntryHead..head),
+	// not base..head — a loop fix that restores code nets out against the original base otherwise.
+	return workflow.KindInfo{DefaultExec: "fork", AllowedExec: []string{"fork"}, ValidateParams: validateProve, FixScopedDiff: true}
 }
 
 // validateProve accepts an optional test_cmd param naming the consent-hashed AllowedCmd to run.
diff --git a/internal/fsm/machine/machine.go b/internal/fsm/machine/machine.go
index a1ff6e3..ab75377 100644
--- a/internal/fsm/machine/machine.go
+++ b/internal/fsm/machine/machine.go
@@ -652,7 +652,16 @@ func (s *session) runNode(node *workflow.Node, head string) (AdvanceResult, bool
 	kind, _ := s.m.deps.Kinds.Kind(node.Kind)
 	k := run.Key(node.Name, snap.Iteration)
 	if _, has := snap.NodeOutputs[k]; !has {
-		text, truncated, err := s.git.Diff(s.ctx, snap.BaseSHA, head, MaxDiffBytes)
+		// Most kinds review the change under review (base..head). A fix-scoped kind (prove) instead
+		// binds against what the FIX changed (FixEntryHead..head): in the loop, base is the original
+		// pre-bug commit, so a fix that restores or re-touches code nets out against base and its added
+		// lines vanish — which silently defeats the pin added-line bind and owesPin (found by the first
+		// live shakedown). FixEntryHead is the pre-fix anchor, so FixEntryHead..head is the fix's own diff.
+		diffFrom := snap.BaseSHA
+		if kind.Info().FixScopedDiff && snap.FixEntryHead != "" {
+			diffFrom = snap.FixEntryHead
+		}
+		text, truncated, err := s.git.Diff(s.ctx, diffFrom, head, MaxDiffBytes)
 		if err != nil {
 			return AdvanceResult{}, true, err
 		}
diff --git a/internal/fsm/machine/machine_test.go b/internal/fsm/machine/machine_test.go
index eb4fa5d..7a75437 100644
--- a/internal/fsm/machine/machine_test.go
+++ b/internal/fsm/machine/machine_test.go
@@ -2223,3 +2223,41 @@ func TestSdlcLoopProvedGatesOnDeletions(t *testing.T) {
 		})
 	}
 }
+
+// A kind whose Info().FixScopedDiff is set must be handed the FIX's own diff (FixEntryHead..head),
+// not the reviewed change (base..head) — the machine picks the diff baseline per kind. This is the
+// fix for the base..head net-out the first live shakedown found (a restore-type fix vanished against
+// the original base). still-present stands in as a fix-scoped kind here; the machine keys generically
+// on Info().FixScopedDiff, not on any specific kind.
+func TestRunNodeFixScopedDiffUsesFixEntryHead(t *testing.T) {
+	h := newHarness(t)
+	h.git.def.Diffs[shaBase+".."+shaHead] = "BASEDIFF"             // the reviewed change (base..head)
+	h.git.def.Diffs[shaHead+".."+shaHead] = "FIXDIFF"              // the fix's own diff (FixEntryHead..head)
+	h.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1} // so commit_exists sees the fix's commit
+	h.reg.kinds["still-present"].info.FixScopedDiff = true
+	m := h.mustInit(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
+	h.advance(m)
+	h.record(m, "discover", findings(1))
+	if r := h.advance(m); r.To != "adjudicate" {
+		t.Fatalf("→adjudicate: %+v", r)
+	}
+	if r := h.advance(m); r.To != "fix" {
+		t.Fatalf("→fix: %+v", r)
+	}
+	if m.View().Snapshot.FixEntryHead != shaHead {
+		t.Fatalf("FixEntryHead must be set on fix entry: %+v", m.View().Snapshot)
+	}
+	h.advance(m) // fix needs input
+	h.record(m, "fix", `{"commit":"`+shaFix+`","summary":"fixed"}`)
+	if r := h.advance(m); r.To != "verify" {
+		t.Fatalf("→verify: %+v", r)
+	}
+	h.advance(m) // runs the still-present (verify) executor
+	// The review node (adjudicate) saw base..head; the fix-scoped node (verify) saw FixEntryHead..head.
+	if got := h.reg.execs["match-then-adjudicate"].calls[0].Diff.Text; got != "BASEDIFF" {
+		t.Fatalf("a review kind must see the base..head diff, got %q", got)
+	}
+	if got := h.reg.execs["still-present"].calls[0].Diff.Text; got != "FIXDIFF" {
+		t.Fatalf("a fix-scoped kind must see the FixEntryHead..head diff, got %q", got)
+	}
+}
diff --git a/internal/fsm/workflow/workflow.go b/internal/fsm/workflow/workflow.go
index a1f8841..2f04a37 100644
--- a/internal/fsm/workflow/workflow.go
+++ b/internal/fsm/workflow/workflow.go
@@ -86,6 +86,11 @@ type KindInfo struct {
 	// the cmd kind forks a subprocess and carries no model — so callers that validate judge
 	// configuration (machine's Preflight) must key on this rather than on Exec.
 	NeedsJudge bool
+	// FixScopedDiff marks a kind whose diff must be the FIX's own diff (FixEntryHead..head), not the
+	// reviewed change (base..head). The prove kind sets it: its pin added-line bind and owed-pin check
+	// must see what the fix changed, which in a loop differs from base..head (a restore-type fix nets
+	// out against the original base). The machine falls back to base..head when FixEntryHead is unset.
+	FixScopedDiff bool
 }
 
 // Options parameterizes Parse. Kinds is required.



```

## Knowledge And Registries

Service inventory: none

No service inventory found.

Knowledge facts:

No Beads knowledge facts found.

## Evidence

# Evidence — the proof binds against the fix diff, not base..head

**Task:** the linchpin gap the first live `sdlc-loop-proved` shakedown found — the differential-proof
machinery bound against the reviewed diff (`base..head`), not the fix's own diff (`FixEntryHead..head`),
so a loop fix could pass with the proof silently bypassed. **Base:** `main` (`9291168`). See
[[proof-binds-against-base-not-fix-diff]].

## The gap (found by the shakedown, confirmed in code)

Every node — including `prove` — was handed the `BaseSHA..head` diff (`machine.go` `runNode`;
`machine/types.go`: "Diff is the base..head diff handed to kinds"). But `prove`'s pin added-line bind
(`isAddedLineInFile`) and `owesPin` (`AddedLinesInFile`) conceptually want the **fix's** diff. In the
loop, `base` is the original pre-bug commit and a fix that RESTORES removed code nets out against it, so
`git diff base..fix` is empty for that file and the fix's added line is invisible → the pin is
**malformed** (advisory), `owesPin` sees no added line → owes no proof → `pins_proven` passes on nothing
and the loop reaches `DONE(fixed)` on the LLM verify alone. The first shakedown reproduced exactly this:
pin `malformed`, `proven=0`, `unproven=1`, yet `outcome=fixed`.

## The fix

A kind now DECLARES whether it needs the fix-scoped diff, and the machine honors it:
- `workflow.KindInfo` gains **`FixScopedDiff bool`**.
- `proveKind.Info()` sets it `true`.
- `machine.runNode` computes the diff base per kind: `FixScopedDiff && FixEntryHead != ""` →
  `FixEntryHead..head` (the fix's own diff); otherwise `base..head` (unchanged for review kinds). Falls
  back to `base..head` when `FixEntryHead` is unset. `FixEntryHead` is the pre-fix anchor, so
  `FixEntryHead..head` is exactly what the fix changed — so the pin bind and `owesPin` now see the
  fix's added lines regardless of how they relate to the original base.

Declarative and generic: the machine keys on `Info().FixScopedDiff`, not on the `prove` kind by name
(no import of `kind` into `machine`).

## Verification — unit AND the re-run shakedown (the fix's whole point)

- `internal/fsm/machine`, `internal/fsm/workflow`, `internal/fsm/kind` (and all `internal/fsm/*` +
  `workflows`) remain **100.0%**. New `TestRunNodeFixScopedDiffUsesFixEntryHead`: a fix-scoped kind
  receives `FixEntryHead..head` while a review kind receives `base..head`. **Mutation-verified** (always
  using `base` reddens the new test). `gofmt`/`go vet`/golangci-lint clean; full `go test ./...` green.
- **Re-ran the exact shakedown** (constructed `Clamp` repo, `sdlc-loop-proved`, real codex judge) with
  the fixed binary. The same fix + pin that came back **malformed** before now comes back **proven**:
  `outcome=proven`, with the real `go test` output *"--- FAIL: TestClampHigh … Clamp(250) = 250, want
  100"* (mutation killed) then restore passing. **Zero** prove findings. Final: `DONE(fixed)`,
  `proven=1, unproven=0` (the pin is now in the §9.6 killed-mutant set). The deterministic proof
  actually gated the outcome — no longer riding on the LLM verify.

## Left as-is (deliberately, pending the retry we just did)
The malformed-pin-is-advisory semantics are unchanged: with the binding fixed, `owesPin` now correctly
sees the fix's added line and would demand a proof (blocking) for a fix that adds a line in the
finding's own file, so a real proofless fix blocks via the owed-pin marker — no need to flip
malformed→blocking. (Confirmed the shakedown now proves rather than passing vacuously.)

