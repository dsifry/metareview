# metareview task-done context

Run ID: `mrv-20260901-051912009394000-task-done-t1-3-pins-proven-gate-pr-a-51440dbf`

## Task

# T1.3 (PR-A) evidence — the pin-fallback differential gate: Unproven lifecycle + pins_proven + prove node + sdlc-loop-proved

**Task:** T1.3 (Epic E1), PR-A of the agreed two-PR split. PR-A ships the **pin-fallback** differential
gate end to end; PR-B adds the **preferred reproduction-execution engine**.
**Spec:** §3.1 (the deterministic sandwich, `pins_proven` clauses, added-line bind), §9.1 (ONE
differential gate; a pin is the `Kind:"pin"` case), §2.4 R4 (`Unproven` add/clear/re-add).
**Plan:** `docs/plans/2026-08-31-pins-bug-class-decomposition.md` T1.3. **Depends-on:** T1.1, T1.2,
T0.1 (all merged). **Base:** `main` (`f7f1de7`).

## What landed (five phases, each TDD + mutation-verified, `internal/fsm/*` at 100%)

**1. `Unproven` fold lifecycle** (`internal/fsm/run/fold.go`, §2.4 R4). The `DeltaApplied` fold now
maintains `Snapshot.Unproven` in temporal order: a fix's declared `Delta.Pins` **ADD** their `Finding`
gaps; a `Proven` `Delta.PinResults` **CLEARs** the matching gap; a re-declared proof **RE-ADDs** a
regressed finding and is never cleared against a stale historical `Proven` (clearing is driven only by
a `PinResult` processed here). `addUnproven` keys by `Finding` (a re-declare replaces in place, never
duplicates); `cow()` clones `Unproven`. `Unproven` stays derived — re-folding reproduces it.

**2. `pins_proven` gate** (`internal/fsm/gate/gates.go`). Blocks the fix→verify transition while the
round's prove node reports an unproven fix, selecting on the finding's **structural provenance**
(`Source==mutation-verify` + a **blocking** `Category`: `unproven-fix` or `unverifiable`) — **never on
issue text** (the #24 lesson). `malformed-pin` is advisory. This is the deterministic differential
PASS path; the §9.2 symptom reviewer (T1.4) can only veto it.

**3. `prove` node kind** (`internal/fsm/kind/prove.go`) — the mutation-verify node, the only
deterministic non-gate node. It reads the fix's declared proofs from `Snapshot.Unproven`, enforces the
**added-line bind** (a pin's `From` must appear on a `+` line of the reviewed diff, else `malformed`),
verifies pin-kind proofs through an injected `Prover`, and emits one `ProofResult` per proof plus a
structural marker `Finding` (`Source=mutation-verify`, `Category` by outcome) for every proof that did
not prove. Reproduction/deletion proofs are **unverifiable in this build** (fail closed — the engine
is PR-B / T1.5). A nil prover or a prover error fails closed.
- **`ProveSpec`** carries the test command (the run's consent-hashed cmd, resolved by name from the
  snapshot's `AllowedCmds`) and work dir — never the agent's.
- **`MutationProver`** (`prove_mutation.go`): the production `Prover`, wrapping
  `internal/mutation.Verifier` (mutate→fail/restore→pass), wired into the CLI registry
  (`cli/wiring.go`). Outcome vocabulary maps by value to `run.PinOutcome`.

**4. `sdlc-loop-proved.yaml`** — `discover→adjudicate→fix→prove→verify` with
`fix→prove[commit_exists]→verify[pins_proven]` and a consent-hashed `test` cmd.

**5. Machine-level integration** (`machine_test.go` `TestSdlcLoopProvedGatesOnPins`) — drives the real
workflow end to end and witnesses `pins_proven` **both green and red**: a proven pin clears the gap and
advances fix→prove→verify; a survived pin blocks at `pins_proven` (`StatusGateFailed`) with the gap left
open. The gate is observed failing — a gate never seen red is not a gate (#24).

## Scope (the two-PR split, approved)

PR-A is the **pin fallback**; the **reproduction-execution engine** (checkout pre-fix tree → overlay
test-only files → run → apply fix → re-run, via a real `git worktree`) is **PR-B**. Until then a
reproduction/deletion proof is `unverifiable` (fail closed), so a fix using the preferred reproduction
form is intentionally blocked in PR-A. The §9.2 fails-for-the-right-reason reviewer is **T1.4**;
deletion verification is **T1.5** (reuses PR-B's engine).

## TDD + mutation verification (applied, observed red, restored — file-backup restores, line-targeted)

| Predicate | Mutation | Killing test |
| --- | --- | --- |
| Unproven ADD | drop the add loop | `TestFoldUnprovenLifecycle` |
| Unproven CLEAR (Proven guard) | `if r.Proven` → `if false` | `TestFoldUnprovenLifecycle` |
| Unproven CLEAR is Proven-only | clear unconditionally | `TestFoldUnprovenGuards` |
| Unproven ADD replace-in-place | always append | `TestFoldUnprovenGuards` |
| pins_proven Source guard | drop it | `TestPinsProvenGate` |
| pins_proven blocking categories | drop `unverifiable`; widen to `malformed` | `TestPinsProvenGate` |
| prove added-line bind | skip the bind | `TestProveAddedLineBindRejectsNonAddedFrom` |
| prove reproduction→unverifiable | send to prover | `TestProveReproductionIsUnverifiableInThisBuild` |
| prove non-Proven marker guard | emit for Proven too | `TestProveProvenPinEmitsNoFinding` |
| prove category mapping | survived→wrong category | `TestProveSurvivedPinEmitsBlockingFinding` |
| isAddedLine `+++` guard | drop it | `TestIsAddedLine` |

## Verification (all green)

- `go build ./...` — OK.
- `go test ./...` — all `ok`, no failures.
- `bash tests/coverage.sh` — coverage gate passed; `internal/fsm/{run,gate,kind,cli,machine,workflow}`
  and `workflows` at 100.0%.
- `gofmt -l internal cmd workflows` — clean.


## Git

- Base: `f7f1de760a4c19dc2ec3499995a91706d4bb027e`
- Head: `49b619348bc753d576e866345fb4f2967c441d9d`
- Branch: `t1.3-pins-proven-gate`
- Gate effect: `gate`

## Context Profile

- Raw diff bytes: `56003`
- Filtered diff bytes: `56003`
- Risk level: `none`

## Context Shard Plan

Not sharded.

## Review Manifest

- Manifest verdict: `PASS`
- Source manifest hash: not sharded
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- docs/metareview/evidence/t1.3-pins-proven-gate-pr-a.md
- internal/fsm/cli/cli_test.go
- internal/fsm/cli/wiring.go
- internal/fsm/gate/gate_test.go
- internal/fsm/gate/gates.go
- internal/fsm/kind/kind.go
- internal/fsm/kind/kind_test.go
- internal/fsm/kind/prove.go
- internal/fsm/kind/prove_mutation.go
- internal/fsm/kind/prove_mutation_test.go
- internal/fsm/kind/prove_test.go
- internal/fsm/machine/machine_test.go
- internal/fsm/run/fold.go
- internal/fsm/run/pins_test.go
- internal/fsm/workflow/workflow_test.go
- workflows/embed_test.go
- workflows/sdlc-loop-proved.yaml

### Manifest Blockers
No manifest blockers.

## Changed Files

- docs/metareview/evidence/t1.3-pins-proven-gate-pr-a.md
- internal/fsm/cli/cli_test.go
- internal/fsm/cli/wiring.go
- internal/fsm/gate/gate_test.go
- internal/fsm/gate/gates.go
- internal/fsm/kind/kind.go
- internal/fsm/kind/kind_test.go
- internal/fsm/kind/prove.go
- internal/fsm/kind/prove_mutation.go
- internal/fsm/kind/prove_mutation_test.go
- internal/fsm/kind/prove_test.go
- internal/fsm/machine/machine_test.go
- internal/fsm/run/fold.go
- internal/fsm/run/pins_test.go
- internal/fsm/workflow/workflow_test.go
- workflows/embed_test.go
- workflows/sdlc-loop-proved.yaml

## Diff

```diff
diff --git a/docs/metareview/evidence/t1.3-pins-proven-gate-pr-a.md b/docs/metareview/evidence/t1.3-pins-proven-gate-pr-a.md
new file mode 100644
index 0000000..901aa0a
--- /dev/null
+++ b/docs/metareview/evidence/t1.3-pins-proven-gate-pr-a.md
@@ -0,0 +1,76 @@
+# T1.3 (PR-A) evidence — the pin-fallback differential gate: Unproven lifecycle + pins_proven + prove node + sdlc-loop-proved
+
+**Task:** T1.3 (Epic E1), PR-A of the agreed two-PR split. PR-A ships the **pin-fallback** differential
+gate end to end; PR-B adds the **preferred reproduction-execution engine**.
+**Spec:** §3.1 (the deterministic sandwich, `pins_proven` clauses, added-line bind), §9.1 (ONE
+differential gate; a pin is the `Kind:"pin"` case), §2.4 R4 (`Unproven` add/clear/re-add).
+**Plan:** `docs/plans/2026-08-31-pins-bug-class-decomposition.md` T1.3. **Depends-on:** T1.1, T1.2,
+T0.1 (all merged). **Base:** `main` (`f7f1de7`).
+
+## What landed (five phases, each TDD + mutation-verified, `internal/fsm/*` at 100%)
+
+**1. `Unproven` fold lifecycle** (`internal/fsm/run/fold.go`, §2.4 R4). The `DeltaApplied` fold now
+maintains `Snapshot.Unproven` in temporal order: a fix's declared `Delta.Pins` **ADD** their `Finding`
+gaps; a `Proven` `Delta.PinResults` **CLEARs** the matching gap; a re-declared proof **RE-ADDs** a
+regressed finding and is never cleared against a stale historical `Proven` (clearing is driven only by
+a `PinResult` processed here). `addUnproven` keys by `Finding` (a re-declare replaces in place, never
+duplicates); `cow()` clones `Unproven`. `Unproven` stays derived — re-folding reproduces it.
+
+**2. `pins_proven` gate** (`internal/fsm/gate/gates.go`). Blocks the fix→verify transition while the
+round's prove node reports an unproven fix, selecting on the finding's **structural provenance**
+(`Source==mutation-verify` + a **blocking** `Category`: `unproven-fix` or `unverifiable`) — **never on
+issue text** (the #24 lesson). `malformed-pin` is advisory. This is the deterministic differential
+PASS path; the §9.2 symptom reviewer (T1.4) can only veto it.
+
+**3. `prove` node kind** (`internal/fsm/kind/prove.go`) — the mutation-verify node, the only
+deterministic non-gate node. It reads the fix's declared proofs from `Snapshot.Unproven`, enforces the
+**added-line bind** (a pin's `From` must appear on a `+` line of the reviewed diff, else `malformed`),
+verifies pin-kind proofs through an injected `Prover`, and emits one `ProofResult` per proof plus a
+structural marker `Finding` (`Source=mutation-verify`, `Category` by outcome) for every proof that did
+not prove. Reproduction/deletion proofs are **unverifiable in this build** (fail closed — the engine
+is PR-B / T1.5). A nil prover or a prover error fails closed.
+- **`ProveSpec`** carries the test command (the run's consent-hashed cmd, resolved by name from the
+  snapshot's `AllowedCmds`) and work dir — never the agent's.
+- **`MutationProver`** (`prove_mutation.go`): the production `Prover`, wrapping
+  `internal/mutation.Verifier` (mutate→fail/restore→pass), wired into the CLI registry
+  (`cli/wiring.go`). Outcome vocabulary maps by value to `run.PinOutcome`.
+
+**4. `sdlc-loop-proved.yaml`** — `discover→adjudicate→fix→prove→verify` with
+`fix→prove[commit_exists]→verify[pins_proven]` and a consent-hashed `test` cmd.
+
+**5. Machine-level integration** (`machine_test.go` `TestSdlcLoopProvedGatesOnPins`) — drives the real
+workflow end to end and witnesses `pins_proven` **both green and red**: a proven pin clears the gap and
+advances fix→prove→verify; a survived pin blocks at `pins_proven` (`StatusGateFailed`) with the gap left
+open. The gate is observed failing — a gate never seen red is not a gate (#24).
+
+## Scope (the two-PR split, approved)
+
+PR-A is the **pin fallback**; the **reproduction-execution engine** (checkout pre-fix tree → overlay
+test-only files → run → apply fix → re-run, via a real `git worktree`) is **PR-B**. Until then a
+reproduction/deletion proof is `unverifiable` (fail closed), so a fix using the preferred reproduction
+form is intentionally blocked in PR-A. The §9.2 fails-for-the-right-reason reviewer is **T1.4**;
+deletion verification is **T1.5** (reuses PR-B's engine).
+
+## TDD + mutation verification (applied, observed red, restored — file-backup restores, line-targeted)
+
+| Predicate | Mutation | Killing test |
+| --- | --- | --- |
+| Unproven ADD | drop the add loop | `TestFoldUnprovenLifecycle` |
+| Unproven CLEAR (Proven guard) | `if r.Proven` → `if false` | `TestFoldUnprovenLifecycle` |
+| Unproven CLEAR is Proven-only | clear unconditionally | `TestFoldUnprovenGuards` |
+| Unproven ADD replace-in-place | always append | `TestFoldUnprovenGuards` |
+| pins_proven Source guard | drop it | `TestPinsProvenGate` |
+| pins_proven blocking categories | drop `unverifiable`; widen to `malformed` | `TestPinsProvenGate` |
+| prove added-line bind | skip the bind | `TestProveAddedLineBindRejectsNonAddedFrom` |
+| prove reproduction→unverifiable | send to prover | `TestProveReproductionIsUnverifiableInThisBuild` |
+| prove non-Proven marker guard | emit for Proven too | `TestProveProvenPinEmitsNoFinding` |
+| prove category mapping | survived→wrong category | `TestProveSurvivedPinEmitsBlockingFinding` |
+| isAddedLine `+++` guard | drop it | `TestIsAddedLine` |
+
+## Verification (all green)
+
+- `go build ./...` — OK.
+- `go test ./...` — all `ok`, no failures.
+- `bash tests/coverage.sh` — coverage gate passed; `internal/fsm/{run,gate,kind,cli,machine,workflow}`
+  and `workflows` at 100.0%.
+- `gofmt -l internal cmd workflows` — clean.
diff --git a/internal/fsm/cli/cli_test.go b/internal/fsm/cli/cli_test.go
index bde04d6..5cd5c24 100644
--- a/internal/fsm/cli/cli_test.go
+++ b/internal/fsm/cli/cli_test.go
@@ -172,9 +172,12 @@ func TestUsageAndPrompt(t *testing.T) {
 	}
 	env := h.must(StatusOK, 0, "workflows")
 	list := env["workflows"].([]any)
-	if len(list) != 2 || list[1].(map[string]any)["name"] != "sdlc-loop" || len(list[1].(map[string]any)["states"].([]any)) != 6 {
+	if len(list) != 3 || list[1].(map[string]any)["name"] != "sdlc-loop" || len(list[1].(map[string]any)["states"].([]any)) != 6 {
 		t.Fatalf("workflows: %v", list)
 	}
+	if list[2].(map[string]any)["name"] != "sdlc-loop-proved" || len(list[2].(map[string]any)["states"].([]any)) != 7 {
+		t.Fatalf("sdlc-loop-proved not listed: %v", list)
+	}
 	// outside a repository: state refuses, workflows works
 	h.cwd = t.TempDir()
 	h.mustErr(CodeNotARepo, 2, "state")
diff --git a/internal/fsm/cli/wiring.go b/internal/fsm/cli/wiring.go
index 4a854b7..8475153 100644
--- a/internal/fsm/cli/wiring.go
+++ b/internal/fsm/cli/wiring.go
@@ -260,7 +260,7 @@ func (c *ctxDeps) machineDeps(root string, scenario *mockai.Scenario, mode judge
 			return machine.Deps{}, err
 		}
 	}
-	kinds, _ := kind.New(kind.Deps{Judge: j, Mock: scenario != nil, Escalate: c.escalation(root, scenario, mode)}) // consistent by construction: a mock judge iff a scenario
+	kinds, _ := kind.New(kind.Deps{Judge: j, Mock: scenario != nil, Escalate: c.escalation(root, scenario, mode), Prove: kind.MutationProver{}}) // consistent by construction: a mock judge iff a scenario
 	d := c.deps
 	md := machine.Deps{
 		Store: d.Store(root), Sidecar: d.Sidecar(root), Kinds: kinds,
diff --git a/internal/fsm/gate/gate_test.go b/internal/fsm/gate/gate_test.go
index 5aca880..7ade01e 100644
--- a/internal/fsm/gate/gate_test.go
+++ b/internal/fsm/gate/gate_test.go
@@ -66,12 +66,51 @@ func TestG1Gates(t *testing.T) {
 	if _, ok := Builtin("nope"); ok {
 		t.Fatal("unknown gate")
 	}
-	want := []string{"all_fixed", "bugs_remain", "commit_exists", "confirmed_empty", "confirmed_nonempty", "findings_empty", "findings_nonempty", "nothing_confirmed", "nothing_found"}
+	want := []string{"all_fixed", "bugs_remain", "commit_exists", "confirmed_empty", "confirmed_nonempty", "findings_empty", "findings_nonempty", "nothing_confirmed", "nothing_found", "pins_proven"}
 	if strings.Join(Names(), ",") != strings.Join(want, ",") {
 		t.Fatalf("Names: %v", Names())
 	}
 }
 
+// pins_proven selects on the finding's structural provenance (Source == mutation-verify + a blocking
+// Category), never on issue text. A survived (unproven-fix) or unverifiable result blocks; a
+// malformed-pin is advisory; a same-category finding from a DIFFERENT source (a discover finding)
+// must not block — the Source guard is what keeps the gate from firing on ordinary review prose.
+func TestPinsProvenGate(t *testing.T) {
+	ctx := context.Background()
+	g, ok := Builtin("pins_proven")
+	if !ok {
+		t.Fatal("missing gate pins_proven")
+	}
+	mv := func(cat string) run.Finding {
+		return run.Finding{IssueText: "x", File: "a.go", Source: run.SourceMutationVerify, Category: cat}
+	}
+	cases := []struct {
+		name string
+		s    run.Snapshot
+		code string // "" → pass
+	}{
+		{"empty passes", run.Snapshot{}, ""},
+		{"every proof proven passes", run.Snapshot{Findings: []run.Finding{{IssueText: "ordinary finding"}}}, ""},
+		{"unproven-fix blocks", run.Snapshot{Findings: []run.Finding{mv(run.CategoryUnprovenFix)}}, CodePinsUnproven},
+		{"unverifiable blocks", run.Snapshot{Findings: []run.Finding{mv(run.CategoryUnverifiable)}}, CodePinsUnproven},
+		{"malformed-pin is advisory, does not block", run.Snapshot{Findings: []run.Finding{mv(run.CategoryMalformedPin)}}, ""},
+		{"same category but a different source does not block", run.Snapshot{Findings: []run.Finding{{IssueText: "x", File: "a.go", Category: run.CategoryUnprovenFix}}}, ""},
+	}
+	for _, c := range cases {
+		err := g(ctx, c.s, &Fake{})
+		if c.code == "" {
+			if err != nil {
+				t.Errorf("%s: unexpected %+v", c.name, err)
+			}
+			continue
+		}
+		if err == nil || err.Code != c.code || err.Gate != "pins_proven" || err.Detail == "" {
+			t.Errorf("%s: got %+v want %s", c.name, err, c.code)
+		}
+	}
+}
+
 func TestG1CommitExists(t *testing.T) {
 	ctx := context.Background()
 	g, _ := Builtin("commit_exists")
diff --git a/internal/fsm/gate/gates.go b/internal/fsm/gate/gates.go
index 6ba3433..8debaf6 100644
--- a/internal/fsm/gate/gates.go
+++ b/internal/fsm/gate/gates.go
@@ -20,6 +20,7 @@ const (
 	CodeBugsRemain       = "ERR_BUGS_REMAIN"
 	CodeAllFixed         = "ERR_ALL_FIXED"
 	CodeBugsKnown        = "ERR_BUGS_KNOWN"
+	CodePinsUnproven     = "ERR_PINS_UNPROVEN"
 )
 
 // Gate evaluates a snapshot; nil means pass.
@@ -76,6 +77,24 @@ var builtin = map[string]Gate{
 		}
 		return nil
 	},
+	// pins_proven blocks the fix→verify transition while the round's `prove` node reports an unproven
+	// fix. It selects on the finding's structural provenance — Source == mutation-verify and a BLOCKING
+	// Category (unproven-fix or unverifiable) — never on issue text (the #24 lesson): a prove result of
+	// `survived` (a real test gap) or `unverifiable` (the tree could not answer) reddens the gate, while
+	// a `malformed-pin` is advisory and does not block. A round with no such finding — every supplied
+	// proof Proven, nothing owed left unproven — passes. This is the deterministic differential PASS
+	// path; the §9.2 symptom reviewer (T1.4) can only veto it, never certify.
+	"pins_proven": func(_ context.Context, s run.Snapshot, _ Git) *run.GateError {
+		for _, f := range s.Findings {
+			if f.Source != run.SourceMutationVerify {
+				continue
+			}
+			if f.Category == run.CategoryUnprovenFix || f.Category == run.CategoryUnverifiable {
+				return fail("pins_proven", CodePinsUnproven, fmt.Sprintf("unproven fix (%s) at %s", f.Category, f.File))
+			}
+		}
+		return nil
+	},
 	"commit_exists": commitExists,
 	"all_fixed": func(_ context.Context, s run.Snapshot, _ Git) *run.GateError {
 		if converge.AllFixed(s) {
diff --git a/internal/fsm/kind/kind.go b/internal/fsm/kind/kind.go
index a28b4a5..616db15 100644
--- a/internal/fsm/kind/kind.go
+++ b/internal/fsm/kind/kind.go
@@ -61,6 +61,10 @@ var commitPattern = regexp.MustCompile(`^[0-9a-f]{7,40}$`)
 type Deps struct {
 	Judge judge.Judge
 	Mock  bool
+	// Prove verifies the differential proofs a fix declares (the mutation-verify `prove` node). Optional:
+	// nil leaves a prove node failing ERR_EXECUTOR_FAILED{reason: no_prover}, the same fail-closed shape
+	// a judge-less adjudicate node has.
+	Prove Prover
 	// Escalate, when set, gives a rejected cross-file candidate a second opinion from a
 	// judge with wider evidence access (see internal/fsm/sandbox). Optional: nil disables
 	// escalation entirely and the primary judge's verdict stands.
@@ -123,9 +127,11 @@ func New(d Deps) (*Registry, error) {
 	r.kinds[AgentEdit] = agentEdit{}
 	r.kinds[StillPresent] = stillPresentKind{}
 	r.kinds[Cmd] = cmdKind{}
+	r.kinds[Prove] = proveKind{}
 	r.execs[MatchThenAdjudicate] = &adjudicateExec{judge: d.Judge, escalate: d.Escalate}
 	r.execs[StillPresent] = &stillPresentExec{judge: d.Judge}
 	r.execs[Cmd] = cmdExec{}
+	r.execs[Prove] = &proveExec{prover: d.Prove}
 	return r, nil
 }
 
diff --git a/internal/fsm/kind/kind_test.go b/internal/fsm/kind/kind_test.go
index 94b689f..a456274 100644
--- a/internal/fsm/kind/kind_test.go
+++ b/internal/fsm/kind/kind_test.go
@@ -1130,6 +1130,9 @@ func TestKindInfoDeclaresWhichKindsCallAJudge(t *testing.T) {
 		ReviewLenses: false,
 		AgentEdit:    false,
 		Cmd:          false,
+		// prove is exec: fork but calls a Prover (mutation engine), not a judge — nothing for judge
+		// pre-flight to validate.
+		Prove: false,
 	}
 	info := r.Info()
 	for name, needs := range want {
diff --git a/internal/fsm/kind/prove.go b/internal/fsm/kind/prove.go
new file mode 100644
index 0000000..bb7f309
--- /dev/null
+++ b/internal/fsm/kind/prove.go
@@ -0,0 +1,203 @@
+package kind
+
+import (
+	"context"
+	"encoding/json"
+	"fmt"
+	"strings"
+	"time"
+
+	"github.com/dsifry/metareview/internal/fsm/errs"
+	"github.com/dsifry/metareview/internal/fsm/machine"
+	"github.com/dsifry/metareview/internal/fsm/run"
+	"github.com/dsifry/metareview/internal/fsm/workflow"
+)
+
+// Prove is the mutation-verify node kind (spec §3.1 / §9.1): the only deterministic non-gate node.
+// It checks the differential proofs a fix declared and emits one ProofResult per proof plus a
+// structural finding for every proof that did not prove, which the pins_proven gate selects on.
+const Prove = "prove"
+
+func errNoProver() error {
+	return errs.E(machine.CodeExecutorFailed, "this registry has no prover", "reason", "no_prover")
+}
+
+// ProveSpec is everything a Prover needs to run a proof against the tree, resolved per execution from
+// the run's snapshot: the consent-hashed test command (never the agent's), the optional build check,
+// the working directory, and the timeout.
+type ProveSpec struct {
+	TestCmd  []string
+	BuildCmd []string
+	Dir      string
+	Timeout  time.Duration
+}
+
+// Prover verifies pin-kind differential proofs by mutation — apply From→To, the tests must FAIL;
+// restore, they must PASS — and returns one ProofResult per input proof, in order, each carrying its
+// originating proof. Injected so the node is testable with a mock; production wraps
+// internal/mutation.Verifier. The proofs handed here have already passed the added-line bind.
+type Prover interface {
+	ProvePins(ctx context.Context, proofs []run.DifferentialProof, spec ProveSpec) ([]run.ProofResult, error)
+}
+
+type proveKind struct{}
+
+func (proveKind) Name() string { return Prove }
+func (proveKind) Info() workflow.KindInfo {
+	return workflow.KindInfo{DefaultExec: "fork", AllowedExec: []string{"fork"}, ValidateParams: validateProve}
+}
+
+// validateProve accepts an optional test_cmd param naming the consent-hashed AllowedCmd to run.
+func validateProve(p map[string]any) error {
+	for k := range p {
+		if k != "test_cmd" {
+			return fmt.Errorf("unknown param %s", k)
+		}
+		if _, ok := p["test_cmd"].(string); !ok {
+			return fmt.Errorf("test_cmd must be a string")
+		}
+	}
+	return nil
+}
+
+func (proveKind) Instructions(run.Snapshot, *workflow.Node, machine.Diff, string) (machine.Instructions, error) {
+	return unsupported(Prove)
+}
+
+func (proveKind) Decode(raw json.RawMessage) (any, error) {
+	var d run.Delta
+	if err := strictDecode(raw, &d); err != nil {
+		return nil, err
+	}
+	if err := checkFindings(d.Findings); err != nil {
+		return nil, err
+	}
+	if len(d.PinResults) > run.MaxDeltaList {
+		return nil, invalid("cap", fmt.Sprintf("more than %d pin results", run.MaxDeltaList))
+	}
+	for i, r := range d.PinResults {
+		if !run.ProofWithinCaps(r.Proof) || !shortOK(string(r.Outcome)) || !within(r.Detail, run.MaxDetail) {
+			return nil, invalid("cap", fmt.Sprintf("pin_results[%d] exceeds a field cap", i))
+		}
+	}
+	if err := checkPayload(d); err != nil {
+		return nil, err
+	}
+	return d, nil
+}
+
+func (proveKind) Reduce(_ run.Snapshot, out any) (run.Delta, error) {
+	return out.(run.Delta), nil
+}
+
+// proveExec is the fork executor. It reads the fix's declared proofs from the snapshot's open gaps
+// (Snapshot.Unproven), checks each, and emits results plus a marker finding per unproven proof.
+type proveExec struct {
+	prover Prover
+}
+
+// testCmdParam returns the AllowedCmd argv named by the node's test_cmd param, or nil.
+func testCmdParam(snap run.Snapshot, node *workflow.Node) []string {
+	name, _ := node.Params["test_cmd"].(string)
+	if name == "" {
+		return nil
+	}
+	for _, c := range snap.AllowedCmds {
+		if c.Name == name {
+			return append([]string(nil), c.Argv...)
+		}
+	}
+	return nil
+}
+
+func (e *proveExec) Execute(ctx context.Context, in machine.ExecInput) (json.RawMessage, error) {
+	if e.prover == nil {
+		return nil, errNoProver()
+	}
+	var toProve []run.DifferentialProof
+	var results []run.ProofResult
+	for _, proof := range in.Snap.Unproven {
+		switch proof.Kind {
+		case run.ProofPin:
+			// Kind:pin guarantees a non-nil Pin — DifferentialProof decode enforces the one-of, and
+			// Unproven only ever holds decoded proofs — so Pin is dereferenced without a nil check.
+			// Added-line bind (spec §3.1): the pin's From must be a line the fix ADDED — a "+" line in
+			// the reviewed diff — never a context or removed line, which a mutation could not attribute
+			// to the fix. A pin that fails the bind is malformed: the CLAIM is bad, rewrite the pin.
+			if !isAddedLine(in.Diff.Text, proof.Pin.From) {
+				results = append(results, proofResult(proof, run.PinMalformed, "pin From is not a line the fix added"))
+				continue
+			}
+			toProve = append(toProve, proof)
+		default:
+			// Reproduction/deletion proving is not wired in this build (the execution engine lands in a
+			// follow-up). Fail closed: unverifiable blocks, it never silently passes.
+			results = append(results, proofResult(proof, run.PinUnverifiable, "differential proving for kind "+proof.Kind+" is not available in this build"))
+		}
+	}
+	verified, err := e.prover.ProvePins(ctx, toProve, ProveSpec{TestCmd: testCmdParam(in.Snap, in.Node), Dir: proveDir(in.Snap)})
+	if err != nil {
+		return nil, err
+	}
+	results = append(results, verified...)
+
+	delta := run.Delta{PinResults: results}
+	for _, r := range results {
+		if !r.Proven {
+			delta.Findings = append(delta.Findings, proofFinding(r))
+		}
+	}
+	raw := json.RawMessage(run.MarshalCanonical(delta))
+	if _, err := (proveKind{}).Decode(raw); err != nil {
+		return nil, err
+	}
+	return raw, nil
+}
+
+// proveDir is the directory the prover runs in: the run's work dir, falling back to the repo root.
+func proveDir(s run.Snapshot) string {
+	if s.WorkDir != "" {
+		return s.WorkDir
+	}
+	return s.RepoRoot
+}
+
+// proofResult builds a ProofResult for a proof the executor itself decided (bind failure or an
+// unsupported kind), never Proven.
+func proofResult(proof run.DifferentialProof, outcome run.PinOutcome, detail string) run.ProofResult {
+	return run.ProofResult{Proof: proof, Proven: false, Outcome: outcome, Detail: detail}
+}
+
+// isAddedLine reports whether from appears within an ADDED ("+", not "+++") line of a unified diff.
+// The bind is to added CODE, not exact-line equality: from is the text to replace, which lives on a
+// line the fix introduced. An empty from can bind to nothing.
+func isAddedLine(diff, from string) bool {
+	if from == "" {
+		return false
+	}
+	for _, line := range strings.Split(diff, "\n") {
+		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") && strings.Contains(line[1:], from) {
+			return true
+		}
+	}
+	return false
+}
+
+// proofFinding turns an unproven ProofResult into the structural marker the pins_proven gate selects
+// on: Source is mutation-verify and Category encodes the outcome (never issue text). Severity mirrors
+// the gate's blocking rule — high for a blocking outcome, medium for the advisory malformed pin.
+func proofFinding(r run.ProofResult) run.Finding {
+	category, severity := run.CategoryUnverifiable, "high"
+	switch r.Outcome {
+	case run.PinSurvived:
+		category, severity = run.CategoryUnprovenFix, "high"
+	case run.PinMalformed:
+		category, severity = run.CategoryMalformedPin, "medium"
+	}
+	file := ""
+	if r.Proof.Pin != nil {
+		file = r.Proof.Pin.File
+	}
+	text := fmt.Sprintf("prove: %s for finding %s", r.Outcome, r.Proof.Finding)
+	return run.Finding{IssueText: text, File: file, Severity: severity, Category: category, Source: run.SourceMutationVerify}
+}
diff --git a/internal/fsm/kind/prove_mutation.go b/internal/fsm/kind/prove_mutation.go
new file mode 100644
index 0000000..a37911c
--- /dev/null
+++ b/internal/fsm/kind/prove_mutation.go
@@ -0,0 +1,42 @@
+package kind
+
+import (
+	"context"
+
+	"github.com/dsifry/metareview/internal/fsm/run"
+	"github.com/dsifry/metareview/internal/mutation"
+)
+
+// MutationProver is the production Prover: it verifies each pin by mutation through
+// internal/mutation.Verifier, which applies From→To inside a fresh copy of the tree, requires the
+// tests to FAIL, restores, and requires them to PASS. The test command is the run's consent-hashed
+// one, resolved by the prove node from the snapshot and passed in via ProveSpec — never the agent's.
+type MutationProver struct {
+	// Run, when set, is the command seam passed through to the Verifier; nil uses the real exec. It
+	// exists only so the outcome→result mapping can be tested without spawning processes.
+	Run func(ctx context.Context, dir string, argv []string) (int, string, error)
+}
+
+// ProvePins converts each pin-kind proof to a mutation.Pin, verifies them together against one tree,
+// and maps each mutation.PinResult back onto its originating DifferentialProof. The mutation outcome
+// vocabulary (proven/survived/malformed/unverifiable) is identical to run.PinOutcome, so it maps by
+// value; Proven stays true exactly when the outcome is proven, the one thing the gate accepts.
+func (mp MutationProver) ProvePins(ctx context.Context, proofs []run.DifferentialProof, spec ProveSpec) ([]run.ProofResult, error) {
+	if len(proofs) == 0 {
+		return nil, nil
+	}
+	pins := make([]mutation.Pin, len(proofs))
+	for i, p := range proofs {
+		pins[i] = mutation.Pin{File: p.Pin.File, From: p.Pin.From, To: p.Pin.To, Test: p.Pin.Test}
+	}
+	v := mutation.Verifier{Dir: spec.Dir, TestCmd: spec.TestCmd, BuildCmd: spec.BuildCmd, Timeout: spec.Timeout, Run: mp.Run}
+	mrs, err := v.Verify(ctx, pins)
+	if err != nil {
+		return nil, err
+	}
+	out := make([]run.ProofResult, len(mrs))
+	for i, mr := range mrs {
+		out[i] = run.ProofResult{Proof: proofs[i], Proven: mr.Proven, Outcome: run.PinOutcome(mr.Outcome), Detail: mr.Detail}
+	}
+	return out, nil
+}
diff --git a/internal/fsm/kind/prove_mutation_test.go b/internal/fsm/kind/prove_mutation_test.go
new file mode 100644
index 0000000..03593d7
--- /dev/null
+++ b/internal/fsm/kind/prove_mutation_test.go
@@ -0,0 +1,66 @@
+package kind
+
+import (
+	"context"
+	"os"
+	"path/filepath"
+	"strings"
+	"testing"
+
+	"github.com/dsifry/metareview/internal/fsm/run"
+)
+
+func pinProofFile(finding, file, from, to string) run.DifferentialProof {
+	return run.DifferentialProof{ID: "p-" + finding, Finding: finding, Kind: run.ProofPin, Pin: &run.Pin{ID: "p-" + finding, Finding: finding, File: file, From: from, To: to, Test: "T"}}
+}
+
+// The production prover maps each mutation outcome back onto its originating proof: a mutation that
+// breaks the tests is Proven; one the tests still pass is survived. Driven through a stubbed command
+// seam (no real processes) over a real fixture tree so the Verifier's own file work still runs.
+func TestMutationProverMapsOutcomes(t *testing.T) {
+	dir := t.TempDir()
+	if err := os.WriteFile(filepath.Join(dir, "calc.go"), []byte("package x\n\nfunc F() int { return 10 }\n"), 0o644); err != nil {
+		t.Fatal(err)
+	}
+	proof := pinProofFile("f1", "calc.go", "return 10", "return 11")
+	spec := ProveSpec{Dir: dir, TestCmd: []string{"go", "test"}, BuildCmd: []string{"true"}}
+
+	// Tests fail exactly when the mutated line ("return 11") is present → Proven.
+	provenRun := func(_ context.Context, d string, argv []string) (int, string, error) {
+		if argv[0] == "true" { // the build check compiles
+			return 0, "", nil
+		}
+		body, _ := os.ReadFile(filepath.Join(d, "calc.go"))
+		if strings.Contains(string(body), "return 11") {
+			return 1, "assertion failed", nil // mutated → tests fail
+		}
+		return 0, "", nil // original/restored → tests pass
+	}
+	rs, err := (MutationProver{Run: provenRun}).ProvePins(context.Background(), []run.DifferentialProof{proof}, spec)
+	if err != nil || len(rs) != 1 {
+		t.Fatalf("prove: %v %+v", err, rs)
+	}
+	if !rs[0].Proven || rs[0].Outcome != run.PinProven || rs[0].Proof.Finding != "f1" {
+		t.Fatalf("a breaking mutation must map to a Proven result carrying its proof: %+v", rs[0])
+	}
+
+	// Tests always pass → the mutation survived (a test gap).
+	survivedRun := func(context.Context, string, []string) (int, string, error) { return 0, "", nil }
+	rs, err = (MutationProver{Run: survivedRun}).ProvePins(context.Background(), []run.DifferentialProof{proof}, spec)
+	if err != nil || len(rs) != 1 || rs[0].Proven || rs[0].Outcome != run.PinSurvived {
+		t.Fatalf("a mutation the tests do not catch must map to survived: %v %+v", err, rs)
+	}
+}
+
+// No proofs is a no-op (nothing to prove), and a misconfigured run (a pin but no test command)
+// surfaces the Verifier's error rather than reporting false blockers.
+func TestMutationProverEdges(t *testing.T) {
+	rs, err := (MutationProver{}).ProvePins(context.Background(), nil, ProveSpec{})
+	if err != nil || rs != nil {
+		t.Fatalf("no proofs must be a no-op: %v %+v", err, rs)
+	}
+	proof := pinProofFile("f1", "a.go", "x", "y")
+	if _, err := (MutationProver{}).ProvePins(context.Background(), []run.DifferentialProof{proof}, ProveSpec{Dir: t.TempDir()}); err == nil {
+		t.Fatal("a pin with no test command must surface an error, not a false pass")
+	}
+}
diff --git a/internal/fsm/kind/prove_test.go b/internal/fsm/kind/prove_test.go
new file mode 100644
index 0000000..0a0e72d
--- /dev/null
+++ b/internal/fsm/kind/prove_test.go
@@ -0,0 +1,284 @@
+package kind
+
+import (
+	"context"
+	"encoding/json"
+	"errors"
+	"fmt"
+	"strings"
+	"testing"
+
+	"github.com/dsifry/metareview/internal/fsm/errs"
+	"github.com/dsifry/metareview/internal/fsm/judge"
+	"github.com/dsifry/metareview/internal/fsm/machine"
+	"github.com/dsifry/metareview/internal/fsm/run"
+	"github.com/dsifry/metareview/internal/fsm/workflow"
+)
+
+var proveNode = &workflow.Node{Name: "prove", Kind: Prove, Exec: "fork"}
+
+type mockProver struct {
+	results   []run.ProofResult
+	err       error
+	gotProofs []run.DifferentialProof
+	gotSpec   ProveSpec
+}
+
+func (m *mockProver) ProvePins(_ context.Context, proofs []run.DifferentialProof, spec ProveSpec) ([]run.ProofResult, error) {
+	m.gotProofs, m.gotSpec = proofs, spec
+	if m.err != nil {
+		return nil, m.err
+	}
+	return m.results, nil
+}
+
+func pinDP(finding, from string) run.DifferentialProof {
+	return run.DifferentialProof{ID: "p-" + finding, Finding: finding, Kind: run.ProofPin, Pin: &run.Pin{ID: "p-" + finding, Finding: finding, File: "a.go", From: from, To: "y", Test: "T"}}
+}
+
+func reproDP(finding string) run.DifferentialProof {
+	return run.DifferentialProof{ID: "r-" + finding, Finding: finding, Kind: run.ProofReproduction, Test: "T"}
+}
+
+func provenResult(p run.DifferentialProof) run.ProofResult {
+	return run.ProofResult{Proof: p, Proven: true, Outcome: run.PinProven}
+}
+
+func runProve(t *testing.T, snap run.Snapshot, diff string, prover Prover) run.Delta {
+	t.Helper()
+	r := mustNew(t, judge.NewMock(judge.Script{}), true)
+	r.execs[Prove] = &proveExec{prover: prover}
+	ex, _ := r.Executor(Prove)
+	raw, err := ex.Execute(context.Background(), machine.ExecInput{Snap: snap, Node: proveNode, Diff: machine.Diff{Text: diff}, Audit: (&audits{}).fn})
+	if err != nil {
+		t.Fatalf("execute: %v", err)
+	}
+	// The executor's own output must always pass the kind's Decode (validity by construction).
+	if _, derr := (proveKind{}).Decode(raw); derr != nil {
+		t.Fatalf("prove output must decode: %v", derr)
+	}
+	var d run.Delta
+	if err := json.Unmarshal(raw, &d); err != nil {
+		t.Fatal(err)
+	}
+	return d
+}
+
+// A proven pin emits a Proven result and NO marker finding — the pins_proven gate then passes.
+func TestProveProvenPinEmitsNoFinding(t *testing.T) {
+	p := pinDP("f1", "addedLine")
+	mp := &mockProver{results: []run.ProofResult{provenResult(p)}}
+	d := runProve(t, run.Snapshot{Unproven: []run.DifferentialProof{p}}, "@@\n+addedLine\n", mp)
+	if len(mp.gotProofs) != 1 {
+		t.Fatalf("the bound pin must reach the prover: %+v", mp.gotProofs)
+	}
+	if len(d.PinResults) != 1 || !d.PinResults[0].Proven {
+		t.Fatalf("expected one Proven result: %+v", d.PinResults)
+	}
+	if len(d.Findings) != 0 {
+		t.Fatalf("a Proven pin must emit no marker finding: %+v", d.Findings)
+	}
+}
+
+// A survived pin emits a blocking unproven-fix marker finding the gate reddens on.
+func TestProveSurvivedPinEmitsBlockingFinding(t *testing.T) {
+	p := pinDP("f1", "addedLine")
+	mp := &mockProver{results: []run.ProofResult{{Proof: p, Proven: false, Outcome: run.PinSurvived, Detail: "tests still passed"}}}
+	d := runProve(t, run.Snapshot{Unproven: []run.DifferentialProof{p}}, "@@\n+addedLine\n", mp)
+	if len(d.Findings) != 1 || d.Findings[0].Source != run.SourceMutationVerify || d.Findings[0].Category != run.CategoryUnprovenFix {
+		t.Fatalf("a survived pin must emit an unproven-fix marker: %+v", d.Findings)
+	}
+	if d.Findings[0].File != "a.go" || d.Findings[0].Severity != "high" {
+		t.Fatalf("the marker must carry the pin's file and a high severity: %+v", d.Findings[0])
+	}
+}
+
+// A pin whose From is not a line the fix added never reaches the prover: it is malformed (advisory),
+// the CLAIM is bad. The added-line bind is what ties a mutation to code the fix introduced.
+func TestProveAddedLineBindRejectsNonAddedFrom(t *testing.T) {
+	p := pinDP("f1", "contextLine")
+	mp := &mockProver{}
+	// The diff carries contextLine only as an unchanged context line and as a removed line — never "+".
+	d := runProve(t, run.Snapshot{Unproven: []run.DifferentialProof{p}}, "@@\n contextLine\n-contextLine\n+different\n", mp)
+	if len(mp.gotProofs) != 0 {
+		t.Fatalf("an unbound pin must NOT reach the prover: %+v", mp.gotProofs)
+	}
+	if len(d.PinResults) != 1 || d.PinResults[0].Outcome != run.PinMalformed {
+		t.Fatalf("a pin failing the added-line bind must be malformed: %+v", d.PinResults)
+	}
+	if len(d.Findings) != 1 || d.Findings[0].Category != run.CategoryMalformedPin || d.Findings[0].Severity != "medium" {
+		t.Fatalf("a malformed pin must emit an advisory malformed-pin marker: %+v", d.Findings)
+	}
+}
+
+// A reproduction/deletion proof has no engine in this build: it is unverifiable (blocking), never a
+// silent pass.
+func TestProveReproductionIsUnverifiableInThisBuild(t *testing.T) {
+	p := reproDP("f1")
+	mp := &mockProver{}
+	d := runProve(t, run.Snapshot{Unproven: []run.DifferentialProof{p}}, "@@\n+whatever\n", mp)
+	if len(mp.gotProofs) != 0 {
+		t.Fatalf("a reproduction proof must not reach the pin prover: %+v", mp.gotProofs)
+	}
+	if len(d.PinResults) != 1 || d.PinResults[0].Outcome != run.PinUnverifiable {
+		t.Fatalf("a reproduction proof must be unverifiable here: %+v", d.PinResults)
+	}
+	if len(d.Findings) != 1 || d.Findings[0].Category != run.CategoryUnverifiable {
+		t.Fatalf("an unverifiable proof must emit an unverifiable marker: %+v", d.Findings)
+	}
+}
+
+// A nil prover fails closed (ERR_EXECUTOR_FAILED{no_prover}), exactly as a judge-less adjudicate does.
+func TestProveNilProverFailsClosed(t *testing.T) {
+	r := mustNew(t, judge.NewMock(judge.Script{}), true)
+	r.execs[Prove] = &proveExec{prover: nil}
+	ex, _ := r.Executor(Prove)
+	_, err := ex.Execute(context.Background(), machine.ExecInput{Snap: run.Snapshot{Unproven: []run.DifferentialProof{pinDP("f1", "x")}}, Node: proveNode, Diff: machine.Diff{Text: "+x"}, Audit: (&audits{}).fn})
+	if !errs.Is(err, machine.CodeExecutorFailed) {
+		t.Fatalf("a nil prover must fail closed: %v", err)
+	}
+}
+
+// A prover error aborts the run rather than being swallowed as a pass.
+func TestProveProverErrorAborts(t *testing.T) {
+	p := pinDP("f1", "addedLine")
+	r := mustNew(t, judge.NewMock(judge.Script{}), true)
+	r.execs[Prove] = &proveExec{prover: &mockProver{err: errors.New("runner down")}}
+	ex, _ := r.Executor(Prove)
+	_, err := ex.Execute(context.Background(), machine.ExecInput{Snap: run.Snapshot{Unproven: []run.DifferentialProof{p}}, Node: proveNode, Diff: machine.Diff{Text: "+addedLine"}, Audit: (&audits{}).fn})
+	if err == nil || err.Error() != "runner down" {
+		t.Fatalf("a prover error must abort: %v", err)
+	}
+}
+
+// The test command and working dir are resolved from the run's snapshot (consent-hashed cmd by name,
+// work dir), never from the agent.
+func TestProveResolvesSpecFromSnapshot(t *testing.T) {
+	p := pinDP("f1", "addedLine")
+	mp := &mockProver{results: []run.ProofResult{provenResult(p)}}
+	snap := run.Snapshot{
+		Unproven:    []run.DifferentialProof{p},
+		WorkDir:     "/w",
+		AllowedCmds: []run.AllowedCmd{{Name: "test", Argv: []string{"go", "test", "./..."}}},
+	}
+	proveNode.Params = map[string]any{"test_cmd": "test"}
+	defer func() { proveNode.Params = nil }()
+	runProve(t, snap, "@@\n+addedLine\n", mp)
+	if len(mp.gotSpec.TestCmd) != 3 || mp.gotSpec.TestCmd[0] != "go" || mp.gotSpec.Dir != "/w" {
+		t.Fatalf("spec must come from the snapshot: %+v", mp.gotSpec)
+	}
+}
+
+// The prove kind's host contract: it is fork-only, host-unsupported (Instructions), decodes and
+// re-serialises its own Delta, and validates the test_cmd param.
+func TestProveKindContract(t *testing.T) {
+	k := proveKind{}
+	if k.Name() != Prove {
+		t.Fatalf("name %q", k.Name())
+	}
+	info := k.Info()
+	if info.DefaultExec != "fork" || len(info.AllowedExec) != 1 || info.AllowedExec[0] != "fork" || info.NeedsJudge || info.ValidateParams == nil {
+		t.Fatalf("info %+v", info)
+	}
+	if _, err := k.Instructions(run.Snapshot{}, proveNode, machine.Diff{}, "nonce"); !errs.Is(err, CodeExecUnsupported) {
+		t.Fatalf("prove must be host-unsupported: %v", err)
+	}
+	// validateProve
+	if err := validateProve(map[string]any{"test_cmd": "go-test"}); err != nil {
+		t.Fatalf("valid test_cmd rejected: %v", err)
+	}
+	if validateProve(map[string]any{"bogus": 1}) == nil {
+		t.Fatal("unknown param must be rejected")
+	}
+	if validateProve(map[string]any{"test_cmd": 7}) == nil {
+		t.Fatal("non-string test_cmd must be rejected")
+	}
+	// Reduce passes the decoded Delta through.
+	d, err := k.Decode(json.RawMessage(`{}`))
+	if err != nil {
+		t.Fatal(err)
+	}
+	if red, err := k.Reduce(run.Snapshot{}, d); err != nil || red.PinResults != nil {
+		t.Fatalf("reduce: %v %+v", err, red)
+	}
+	// A well-formed result + finding decodes.
+	good := run.Delta{
+		PinResults: []run.ProofResult{{Proof: reproDP("f1"), Proven: true, Outcome: run.PinProven}},
+		Findings:   []run.Finding{{IssueText: "x", Source: run.SourceMutationVerify, Category: run.CategoryUnprovenFix}},
+	}
+	if _, err := k.Decode(run.MarshalCanonical(good)); err != nil {
+		t.Fatalf("valid prove delta rejected: %v", err)
+	}
+	// Decode error branches.
+	badReason := func(raw []byte, reason string) {
+		t.Helper()
+		_, err := k.Decode(raw)
+		if !errs.Is(err, CodeNodeOutputInvalid) || errs.As(err).Field("reason") != reason {
+			t.Fatalf("want %s got %v", reason, err)
+		}
+	}
+	badReason([]byte(`{"zzz":1}`), "decode")
+	badReason(run.MarshalCanonical(run.Delta{Findings: []run.Finding{{IssueText: ""}}}), "empty")
+	// pin_results count over cap
+	many := make([]run.ProofResult, run.MaxDeltaList+1)
+	for i := range many {
+		many[i] = run.ProofResult{Proof: reproDP("f"), Proven: true, Outcome: run.PinProven}
+	}
+	badReason(run.MarshalCanonical(run.Delta{PinResults: many}), "cap")
+	// a per-field cap: an over-cap Detail
+	badReason(run.MarshalCanonical(run.Delta{PinResults: []run.ProofResult{{Proof: reproDP("f"), Detail: strings.Repeat("x", run.MaxDetail+1)}}}), "cap")
+	// payload over cap: many max-size findings pass their own caps but overflow the envelope
+	fat := make([]run.Finding, 200)
+	for i := range fat {
+		fat[i] = run.Finding{IssueText: strings.Repeat("x", run.MaxText-2)}
+	}
+	badReason(run.MarshalCanonical(run.Delta{Findings: fat}), "cap")
+}
+
+// A test_cmd naming a cmd the run never consented to resolves to no command — the prover then reports
+// unverifiable rather than running something unsanctioned.
+func TestProveUnconsentedTestCmdResolvesToNothing(t *testing.T) {
+	mp := &mockProver{results: []run.ProofResult{provenResult(pinDP("f1", "addedLine"))}}
+	snap := run.Snapshot{Unproven: []run.DifferentialProof{pinDP("f1", "addedLine")}, AllowedCmds: []run.AllowedCmd{{Name: "build"}}}
+	proveNode.Params = map[string]any{"test_cmd": "test"} // not present among AllowedCmds
+	defer func() { proveNode.Params = nil }()
+	runProve(t, snap, "@@\n+addedLine\n", mp)
+	if mp.gotSpec.TestCmd != nil {
+		t.Fatalf("an unconsented test_cmd must resolve to no command: %+v", mp.gotSpec.TestCmd)
+	}
+}
+
+// If the emitted delta would exceed a persistence cap, the executor fails closed on its own Decode
+// rather than reporting a success the fold would then refuse.
+func TestProveOutputOverCapFailsClosed(t *testing.T) {
+	open := make([]run.DifferentialProof, run.MaxDeltaList+1)
+	for i := range open {
+		open[i] = reproDP(fmt.Sprint(i)) // each is unverifiable → one result + one finding
+	}
+	r := mustNew(t, judge.NewMock(judge.Script{}), true)
+	r.execs[Prove] = &proveExec{prover: &mockProver{}}
+	ex, _ := r.Executor(Prove)
+	_, err := ex.Execute(context.Background(), machine.ExecInput{Snap: run.Snapshot{Unproven: open}, Node: proveNode, Diff: machine.Diff{Text: ""}, Audit: (&audits{}).fn})
+	if !errs.Is(err, CodeNodeOutputInvalid) {
+		t.Fatalf("an over-cap prove output must fail closed: %v", err)
+	}
+}
+
+func TestIsAddedLine(t *testing.T) {
+	diff := "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1,2 +1,3 @@\n ctx\n-old\n+\tnewCode()\n"
+	if !isAddedLine(diff, "newCode()") {
+		t.Error("text on a + line must bind")
+	}
+	if isAddedLine(diff, "old") {
+		t.Error("a removed line must not bind")
+	}
+	if isAddedLine(diff, "ctx") {
+		t.Error("a context line must not bind")
+	}
+	if isAddedLine(diff, "a.go") {
+		t.Error("the +++ header must not count as an added line")
+	}
+	if isAddedLine(diff, "") {
+		t.Error("an empty From binds to nothing")
+	}
+}
diff --git a/internal/fsm/machine/machine_test.go b/internal/fsm/machine/machine_test.go
index 6cdc69d..72d29b8 100644
--- a/internal/fsm/machine/machine_test.go
+++ b/internal/fsm/machine/machine_test.go
@@ -4,6 +4,7 @@ import (
 	"context"
 	"encoding/json"
 	"errors"
+	"fmt"
 	"io"
 	"os"
 	"strings"
@@ -1892,3 +1893,97 @@ func renamed(body any) []byte {
 	text = strings.Replace(text, "workflow: review-loop", "workflow: review-loop-test", 1)
 	return []byte(text)
 }
+
+// TestSdlcLoopProvedGatesOnPins drives the real sdlc-loop-proved workflow through the machine and
+// witnesses the pins_proven gate BOTH green and red — a gate never seen failing is not a gate (#24).
+// A fix declares a pin (the T1.2 seam), the fold opens an Unproven gap, the prove node reports the
+// test-controlled outcome, and pins_proven either advances fix→prove→verify (proven) or blocks
+// (survived). The prove node is faked here (its internals are unit-tested); this exercises the
+// workflow routing, the Unproven fold lifecycle, and the gate wired together.
+func TestSdlcLoopProvedGatesOnPins(t *testing.T) {
+	for _, tc := range []struct {
+		name     string
+		outcome  run.PinOutcome
+		toVerify bool
+	}{
+		{"a proven pin advances to verify", run.PinProven, true},
+		{"a survived pin blocks at pins_proven", run.PinSurvived, false},
+	} {
+		t.Run(tc.name, func(t *testing.T) {
+			h := newHarness(t)
+			h.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+			// resolve `go` so the workflow's consent-hashed test cmd resolves at init
+			h.deps.LookPath = func(n string) (string, error) {
+				if n == "go" {
+					return "/usr/bin/go", nil
+				}
+				return "", errors.New("not found")
+			}
+			h.deps.FileHash = func(p string) (string, error) {
+				if p == "/usr/bin/go" {
+					return "hgo", nil
+				}
+				return "", errors.New("no such file")
+			}
+			// agent-edit carries the fix's declared pins into the Delta (the T1.2 seam)
+			h.reg.kinds["agent-edit"].decode = func(raw json.RawMessage) (any, error) {
+				var d run.Delta
+				dec := json.NewDecoder(strings.NewReader(string(raw)))
+				dec.DisallowUnknownFields()
+				if err := dec.Decode(&d); err != nil {
+					return nil, err
+				}
+				return d, nil
+			}
+			h.reg.kinds["agent-edit"].reduce = func(_ run.Snapshot, out any) (run.Delta, error) { return out.(run.Delta), nil }
+			// the prove node reads the folded gaps and reports the test-controlled outcome
+			h.reg.kinds["prove"] = &fakeKind{name: "prove", info: workflow.KindInfo{DefaultExec: "fork", AllowedExec: []string{"fork"}}}
+			h.reg.execs["prove"] = &fakeExecutor{fn: func(in ExecInput) (json.RawMessage, error) {
+				var d run.Delta
+				for _, p := range in.Snap.Unproven {
+					proven := tc.outcome == run.PinProven
+					d.PinResults = append(d.PinResults, run.ProofResult{Proof: p, Proven: proven, Outcome: tc.outcome})
+					if !proven {
+						d.Findings = append(d.Findings, run.Finding{IssueText: "unproven fix", File: "f.go", Source: run.SourceMutationVerify, Category: run.CategoryUnprovenFix})
+					}
+				}
+				return json.RawMessage(run.MarshalCanonical(d)), nil
+			}}
+
+			// consent: init once to learn the cmds sha, then init for real
+			_, err := h.init(InitOptions{Workflow: "sdlc-loop-proved", Vars: sdlcVars})
+			sha := wantCodeE(t, err, CodeCmdsNotAllowed).Field("sha")
+			m := h.mustInit(InitOptions{Workflow: "sdlc-loop-proved", Vars: sdlcVars, AllowCustomCmds: sha})
+
+			h.advance(m) // enter discover
+			h.record(m, "discover", findings(1))
+			h.advance(m) // → adjudicate
+			h.advance(m) // → fix (one confirmed bug)
+			finding := run.FindingKey("f.go", "bug a")
+			fix := fmt.Sprintf(`{"commit":%q,"pins":[{"id":"p1","finding":%q,"kind":"pin","pin":{"id":"p1","finding":%q,"file":"f.go","from":"+x","to":"y","test":"T"}}]}`, shaFix, finding, finding)
+			h.record(m, "fix", fix)
+			if r := h.advance(m); r.To != "prove" {
+				t.Fatalf("fix must advance to prove: %+v", r)
+			}
+			if got := m.View().Snapshot.Unproven; len(got) != 1 || got[0].Finding != finding {
+				t.Fatalf("the fix's pin must open exactly one Unproven gap for its finding: %+v", got)
+			}
+			r := h.advance(m) // prove runs; pins_proven is evaluated
+			if tc.toVerify {
+				if r.To != "verify" {
+					t.Fatalf("a proven pin must clear the gate and advance to verify: %+v", r)
+				}
+				if got := m.View().Snapshot.Unproven; len(got) != 0 {
+					t.Fatalf("a proven pin must clear the Unproven gap: %+v", got)
+				}
+			} else {
+				if r.Status != StatusGateFailed || r.Gate.Name != "pins_proven" || r.Gate.Error.Code != gate.CodePinsUnproven {
+					t.Fatalf("a survived pin must block at pins_proven: %+v", r)
+				}
+				if got := m.View().Snapshot.Unproven; len(got) != 1 {
+					t.Fatalf("a survived pin must leave the Unproven gap open: %+v", got)
+				}
+			}
+		})
+	}
+}
diff --git a/internal/fsm/run/fold.go b/internal/fsm/run/fold.go
index c6e0040..6b7bd82 100644
--- a/internal/fsm/run/fold.go
+++ b/internal/fsm/run/fold.go
@@ -148,6 +148,20 @@ func Apply(st FoldState, ev Event) (FoldState, error) {
 			}
 			next.Status = p.Status
 		}
+		// Unproven gap lifecycle (§2.4 R4), maintained in temporal fold order. A fix node's declared
+		// proofs ADD their Finding gaps; a Proven prove-result CLEARs the matching gap. Re-add is
+		// automatic: a later fix re-declaring a proof for a regressed finding adds it again, and it is
+		// never cleared against a stale historical Proven because clearing is driven only by a
+		// PinResult processed here, never by recomputation against past state. Snapshot.Unproven is
+		// derived (never its own event), so re-folding the log reproduces it.
+		for _, proof := range p.Pins {
+			next.Unproven = addUnproven(next.Unproven, proof)
+		}
+		for _, r := range p.PinResults {
+			if r.Proven {
+				next.Unproven = clearUnproven(next.Unproven, r.Proof.Finding)
+			}
+		}
 		next.Unfixed = countUnfixed(next.AllFound, next.Status)
 		next.Applied[k] = true
 	case *LLMCallData:
@@ -270,6 +284,7 @@ func (st FoldState) cow() FoldState {
 	next.Goldens = append([]Golden{}, st.Goldens...)
 	next.Findings = append([]Finding{}, st.Findings...)
 	next.Confirmed = append([]Bug{}, st.Confirmed...)
+	next.Unproven = cloneProofs(st.Unproven)
 	next.AllFound = append([]Bug{}, st.AllFound...)
 	next.Status = append([]BugStatus{}, st.Status...)
 	next.PrevUnfixed = cloneInt(st.PrevUnfixed)
@@ -619,6 +634,31 @@ func withinCaps(p any) bool {
 	return true
 }
 
+// addUnproven records a declared proof's Finding gap as open, keyed by Finding. A re-declared proof
+// for a Finding already open replaces the entry in place (keeping its position) so a regressed fix's
+// re-added gap carries the latest proof, not a stale one; a new Finding appends.
+func addUnproven(open []DifferentialProof, proof DifferentialProof) []DifferentialProof {
+	for i := range open {
+		if open[i].Finding == proof.Finding {
+			out := cloneProofs(open)
+			out[i] = proof
+			return out
+		}
+	}
+	return append(cloneProofs(open), proof)
+}
+
+// clearUnproven removes every gap whose Finding matches (a Proven result closes it).
+func clearUnproven(open []DifferentialProof, finding string) []DifferentialProof {
+	out := make([]DifferentialProof, 0, len(open))
+	for _, p := range open {
+		if p.Finding != finding {
+			out = append(out, p)
+		}
+	}
+	return out
+}
+
 // unfixedIDs is the id of every AllFound bug with no fixed status, in AllFound order so the
 // recorded set is stable between folds of the same log.
 func unfixedIDs(all []Bug, status []BugStatus) []string {
diff --git a/internal/fsm/run/pins_test.go b/internal/fsm/run/pins_test.go
index 3021eac..9e76227 100644
--- a/internal/fsm/run/pins_test.go
+++ b/internal/fsm/run/pins_test.go
@@ -2,6 +2,83 @@ package run
 
 import "testing"
 
+// pinProof is a Kind:"pin" DifferentialProof for a given finding id.
+func pinProof(finding string) DifferentialProof {
+	return DifferentialProof{ID: "p-" + finding, Finding: finding, Kind: ProofPin, Pin: &Pin{ID: "p-" + finding, Finding: finding, File: "a.go", From: "+x", To: "y", Test: "T"}}
+}
+
+// provenFor is a Proven ProofResult for a given finding id.
+func provenFor(finding string) ProofResult {
+	return ProofResult{Proof: pinProof(finding), Proven: true, Outcome: PinProven}
+}
+
+// unprovenFindings lists the finding ids currently open in Unproven, in order.
+func unprovenFindings(s Snapshot) []string {
+	var out []string
+	for _, p := range s.Unproven {
+		out = append(out, p.Finding)
+	}
+	return out
+}
+
+// The Unproven gap lifecycle (spec §2.4 R4): a fix's declared proof ADDs its Finding gap; a later
+// Proven prove-result CLEARs it; a re-declared proof for a regressed finding RE-ADDs it — and a
+// re-added gap is NOT cleared against the stale historical Proven (only a NEW prove result clears).
+func TestFoldUnprovenLifecycle(t *testing.T) {
+	b := NewBuilder(runA)
+	b.Init(baseInit())
+	// [1,2] fix declares proofs for f1 and f2 → both gaps enter Unproven (ADD)
+	b.Event(TypeNodeOutput, out(`{}`), WithNode("fix"))
+	b.Event(TypeDeltaApplied, deltaFor(`{}`, Delta{Pins: []DifferentialProof{pinProof("f1"), pinProof("f2")}}), WithNode("fix"))
+	// [3,4] prove proves f1 → f1 clears, f2 remains (CLEAR)
+	b.Event(TypeNodeOutput, out(`{}`), WithNode("prove"))
+	b.Event(TypeDeltaApplied, deltaFor(`{}`, Delta{PinResults: []ProofResult{provenFor("f1")}}), WithNode("prove"))
+	// [5,6] a later fix re-declares a proof for f1 (the fix regressed) → f1 re-enters (RE-ADD), and it
+	// must NOT be auto-cleared by the earlier Proven result for f1.
+	b.Event(TypeNodeOutput, out(`{}`), WithNode("fix2"))
+	b.Event(TypeDeltaApplied, deltaFor(`{}`, Delta{Pins: []DifferentialProof{pinProof("f1")}}), WithNode("fix2"))
+	evs := b.Events()
+
+	eq := func(prefix int, want ...string) {
+		got := unprovenFindings(mustFold(t, evs[:prefix]))
+		if len(got) != len(want) {
+			t.Fatalf("after %d events, Unproven=%v want %v", prefix, got, want)
+		}
+		for i := range want {
+			if got[i] != want[i] {
+				t.Fatalf("after %d events, Unproven=%v want %v", prefix, got, want)
+			}
+		}
+	}
+	eq(3, "f1", "f2") // ADD
+	eq(5, "f2")       // CLEAR f1
+	eq(7, "f2", "f1") // RE-ADD f1 (not cleared by the stale historical Proven)
+}
+
+// A non-Proven prove-result must NOT clear the gap (only Proven closes it), and re-declaring a proof
+// for an already-open finding keeps a SINGLE gap, never a duplicate.
+func TestFoldUnprovenGuards(t *testing.T) {
+	b := NewBuilder(runA)
+	b.Init(baseInit())
+	b.Event(TypeNodeOutput, out(`{}`), WithNode("fix"))
+	b.Event(TypeDeltaApplied, deltaFor(`{}`, Delta{Pins: []DifferentialProof{pinProof("f1")}}), WithNode("fix"))
+	// a survived (non-Proven) result must leave f1 open
+	b.Event(TypeNodeOutput, out(`{}`), WithNode("prove"))
+	b.Event(TypeDeltaApplied, deltaFor(`{}`, Delta{PinResults: []ProofResult{{Proof: pinProof("f1"), Proven: false, Outcome: PinSurvived}}}), WithNode("prove"))
+	// re-declaring f1 must keep a single gap, not a second copy
+	b.Event(TypeNodeOutput, out(`{}`), WithNode("fix2"))
+	b.Event(TypeDeltaApplied, deltaFor(`{}`, Delta{Pins: []DifferentialProof{pinProof("f1")}}), WithNode("fix2"))
+	evs := b.Events()
+	// After the survived result (before any re-declare), f1 must STILL be open — a non-Proven result
+	// must not clear it. Asserted at this prefix so a later re-add cannot mask an erroneous clear.
+	if got := unprovenFindings(mustFold(t, evs[:5])); len(got) != 1 || got[0] != "f1" {
+		t.Fatalf("a survived (non-Proven) result must not clear the gap: %v", got)
+	}
+	if got := unprovenFindings(mustFold(t, evs)); len(got) != 1 || got[0] != "f1" {
+		t.Fatalf("re-declaring an already-open finding must not duplicate the gap: %v", got)
+	}
+}
+
 func TestPinIDIsIdempotentAndContentDerived(t *testing.T) {
 	a := PinID("f1", "a.go", "x", "y")
 	if a != PinID("f1", "a.go", "x", "y") {
diff --git a/internal/fsm/workflow/workflow_test.go b/internal/fsm/workflow/workflow_test.go
index 9723a4e..e9b08b8 100644
--- a/internal/fsm/workflow/workflow_test.go
+++ b/internal/fsm/workflow/workflow_test.go
@@ -33,6 +33,14 @@ func kinds() map[string]KindInfo {
 		"agent-edit":            {DefaultExec: "inline", AllowedExec: []string{"inline", "subagent"}},
 		"still-present":         {DefaultExec: "fork", AllowedExec: []string{"fork"}},
 		"cmd":                   {DefaultExec: "fork", AllowedExec: []string{"fork"}},
+		"prove": {DefaultExec: "fork", AllowedExec: []string{"fork"}, ValidateParams: func(p map[string]any) error {
+			for k := range p {
+				if k != "test_cmd" {
+					return errors.New("unknown param " + k)
+				}
+			}
+			return nil
+		}},
 	}
 }
 
@@ -113,6 +121,23 @@ func TestW1Shipped(t *testing.T) {
 	if !sdlc.IsTerminal("done") || !sdlc.IsTerminal("failed") || sdlc.IsTerminal("verify") {
 		t.Fatal("IsTerminal")
 	}
+	// sdlc-loop-proved inserts a prove node gated by pins_proven between fix and verify.
+	proved := mustParse(t, string(must(workflows.Read("sdlc-loop-proved"))))
+	if len(proved.Transitions) != 8 || proved.Nodes["prove"].Kind != "prove" || proved.Nodes["prove"].Params["test_cmd"] != "test" {
+		t.Fatalf("sdlc-loop-proved shape: %+v", proved.Nodes["prove"])
+	}
+	var fixToProve, proveToVerify bool
+	for _, tr := range proved.Transitions {
+		if tr.From == "fix" && tr.To == "prove" && tr.Gate == "commit_exists" {
+			fixToProve = true
+		}
+		if tr.From == "prove" && tr.To == "verify" && tr.Gate == "pins_proven" {
+			proveToVerify = true
+		}
+	}
+	if !fixToProve || !proveToVerify {
+		t.Fatalf("sdlc-loop-proved must route fix→prove[commit_exists]→verify[pins_proven]: %+v", proved.Transitions)
+	}
 	rl := mustParse(t, string(must(workflows.Read("review-loop"))))
 	if rl.LoopTransition() != nil || rl.Convergence != nil || len(rl.Outgoing("adjudicate")) != 2 {
 		t.Fatal("review-loop shape")
diff --git a/workflows/embed_test.go b/workflows/embed_test.go
index f5572d5..91b26bd 100644
--- a/workflows/embed_test.go
+++ b/workflows/embed_test.go
@@ -7,7 +7,7 @@ import (
 
 func TestNamesAndRead(t *testing.T) {
 	names := Names()
-	if len(names) != 2 || names[0] != "review-loop" || names[1] != "sdlc-loop" {
+	if len(names) != 3 || names[0] != "review-loop" || names[1] != "sdlc-loop" || names[2] != "sdlc-loop-proved" {
 		t.Fatalf("Names = %v", names)
 	}
 	for _, n := range names {
diff --git a/workflows/sdlc-loop-proved.yaml b/workflows/sdlc-loop-proved.yaml
new file mode 100644
index 0000000..5263f8f
--- /dev/null
+++ b/workflows/sdlc-loop-proved.yaml
@@ -0,0 +1,28 @@
+workflow: sdlc-loop-proved
+version: 1
+vars:
+  REVIEWER:     {default: claude-opus-5}
+  JUDGE:        {required: true}
+  REV_EFFORT:   {default: low}
+  JUDGE_EFFORT: {required: true}   # Pareto-selected with JUDGE (spec §17); pinned to medium under --calibration
+states: [discover, adjudicate, fix, prove, verify, done, failed]
+cmds:
+  test: {argv: [go, test, ./...], timeout: 600}   # the consent-hashed command `prove` mutates against
+transitions:                                  # ordered
+  - {from: discover,   to: done,       gate: nothing_found,      outcome: clean}   # iteration 0 only: refuses once bugs are known
+  - {from: discover,   to: adjudicate, gate: findings_nonempty}
+  - {from: adjudicate, to: done,       gate: nothing_confirmed,  outcome: clean}
+  - {from: adjudicate, to: fix,        gate: confirmed_nonempty}
+  - {from: fix,        to: prove,      gate: commit_exists}
+  - {from: prove,      to: verify,     gate: pins_proven}    # a fix's declared proofs must all hold
+  - {from: verify,     to: done,       gate: all_fixed,   outcome: fixed}
+  - {from: verify,     to: discover,   gate: bugs_remain, loop: true}
+nodes:
+  discover:   {kind: review-lenses,        exec: subagent, model: $REVIEWER, effort: $REV_EFFORT}
+  adjudicate: {kind: match-then-adjudicate, exec: fork,     model: $JUDGE, effort: $JUDGE_EFFORT}
+  fix:        {kind: agent-edit}                                   # exec inferred from the kind (inline)
+  prove:      {kind: prove, test_cmd: test}                        # mutation-verify; exec inferred (fork)
+  verify:     {kind: still-present,         model: $JUDGE, effort: $JUDGE_EFFORT}   # exec inferred (fork)
+convergence:
+  any: [no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}]
+repo_mode: advisory



```

## Knowledge And Registries

Service inventory: none

No service inventory found.

Knowledge facts:

No Beads knowledge facts found.

## Evidence

# T1.3 (PR-A) evidence — the pin-fallback differential gate: Unproven lifecycle + pins_proven + prove node + sdlc-loop-proved

**Task:** T1.3 (Epic E1), PR-A of the agreed two-PR split. PR-A ships the **pin-fallback** differential
gate end to end; PR-B adds the **preferred reproduction-execution engine**.
**Spec:** §3.1 (the deterministic sandwich, `pins_proven` clauses, added-line bind), §9.1 (ONE
differential gate; a pin is the `Kind:"pin"` case), §2.4 R4 (`Unproven` add/clear/re-add).
**Plan:** `docs/plans/2026-08-31-pins-bug-class-decomposition.md` T1.3. **Depends-on:** T1.1, T1.2,
T0.1 (all merged). **Base:** `main` (`f7f1de7`).

## What landed (five phases, each TDD + mutation-verified, `internal/fsm/*` at 100%)

**1. `Unproven` fold lifecycle** (`internal/fsm/run/fold.go`, §2.4 R4). The `DeltaApplied` fold now
maintains `Snapshot.Unproven` in temporal order: a fix's declared `Delta.Pins` **ADD** their `Finding`
gaps; a `Proven` `Delta.PinResults` **CLEARs** the matching gap; a re-declared proof **RE-ADDs** a
regressed finding and is never cleared against a stale historical `Proven` (clearing is driven only by
a `PinResult` processed here). `addUnproven` keys by `Finding` (a re-declare replaces in place, never
duplicates); `cow()` clones `Unproven`. `Unproven` stays derived — re-folding reproduces it.

**2. `pins_proven` gate** (`internal/fsm/gate/gates.go`). Blocks the fix→verify transition while the
round's prove node reports an unproven fix, selecting on the finding's **structural provenance**
(`Source==mutation-verify` + a **blocking** `Category`: `unproven-fix` or `unverifiable`) — **never on
issue text** (the #24 lesson). `malformed-pin` is advisory. This is the deterministic differential
PASS path; the §9.2 symptom reviewer (T1.4) can only veto it.

**3. `prove` node kind** (`internal/fsm/kind/prove.go`) — the mutation-verify node, the only
deterministic non-gate node. It reads the fix's declared proofs from `Snapshot.Unproven`, enforces the
**added-line bind** (a pin's `From` must appear on a `+` line of the reviewed diff, else `malformed`),
verifies pin-kind proofs through an injected `Prover`, and emits one `ProofResult` per proof plus a
structural marker `Finding` (`Source=mutation-verify`, `Category` by outcome) for every proof that did
not prove. Reproduction/deletion proofs are **unverifiable in this build** (fail closed — the engine
is PR-B / T1.5). A nil prover or a prover error fails closed.
- **`ProveSpec`** carries the test command (the run's consent-hashed cmd, resolved by name from the
  snapshot's `AllowedCmds`) and work dir — never the agent's.
- **`MutationProver`** (`prove_mutation.go`): the production `Prover`, wrapping
  `internal/mutation.Verifier` (mutate→fail/restore→pass), wired into the CLI registry
  (`cli/wiring.go`). Outcome vocabulary maps by value to `run.PinOutcome`.

**4. `sdlc-loop-proved.yaml`** — `discover→adjudicate→fix→prove→verify` with
`fix→prove[commit_exists]→verify[pins_proven]` and a consent-hashed `test` cmd.

**5. Machine-level integration** (`machine_test.go` `TestSdlcLoopProvedGatesOnPins`) — drives the real
workflow end to end and witnesses `pins_proven` **both green and red**: a proven pin clears the gap and
advances fix→prove→verify; a survived pin blocks at `pins_proven` (`StatusGateFailed`) with the gap left
open. The gate is observed failing — a gate never seen red is not a gate (#24).

## Scope (the two-PR split, approved)

PR-A is the **pin fallback**; the **reproduction-execution engine** (checkout pre-fix tree → overlay
test-only files → run → apply fix → re-run, via a real `git worktree`) is **PR-B**. Until then a
reproduction/deletion proof is `unverifiable` (fail closed), so a fix using the preferred reproduction
form is intentionally blocked in PR-A. The §9.2 fails-for-the-right-reason reviewer is **T1.4**;
deletion verification is **T1.5** (reuses PR-B's engine).

## TDD + mutation verification (applied, observed red, restored — file-backup restores, line-targeted)

| Predicate | Mutation | Killing test |
| --- | --- | --- |
| Unproven ADD | drop the add loop | `TestFoldUnprovenLifecycle` |
| Unproven CLEAR (Proven guard) | `if r.Proven` → `if false` | `TestFoldUnprovenLifecycle` |
| Unproven CLEAR is Proven-only | clear unconditionally | `TestFoldUnprovenGuards` |
| Unproven ADD replace-in-place | always append | `TestFoldUnprovenGuards` |
| pins_proven Source guard | drop it | `TestPinsProvenGate` |
| pins_proven blocking categories | drop `unverifiable`; widen to `malformed` | `TestPinsProvenGate` |
| prove added-line bind | skip the bind | `TestProveAddedLineBindRejectsNonAddedFrom` |
| prove reproduction→unverifiable | send to prover | `TestProveReproductionIsUnverifiableInThisBuild` |
| prove non-Proven marker guard | emit for Proven too | `TestProveProvenPinEmitsNoFinding` |
| prove category mapping | survived→wrong category | `TestProveSurvivedPinEmitsBlockingFinding` |
| isAddedLine `+++` guard | drop it | `TestIsAddedLine` |

## Verification (all green)

- `go build ./...` — OK.
- `go test ./...` — all `ok`, no failures.
- `bash tests/coverage.sh` — coverage gate passed; `internal/fsm/{run,gate,kind,cli,machine,workflow}`
  and `workflows` at 100.0%.
- `gofmt -l internal cmd workflows` — clean.

