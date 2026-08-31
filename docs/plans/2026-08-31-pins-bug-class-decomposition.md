# Decomposition — Pins & Bug.Class

**Source spec:** `docs/specs/2026-08-31-pins-and-bug-class.md` (approved; §9 residuals fixed in #34).
**Build flow:** metareview option 1 — this plan is reviewed via `metareview review artifact`; each
task is built **TDD + DI first** (mock-AI, enforced 100% coverage, mutation-verified) and gated with
`review-task-done` → `review-epic-ready` → `review-pr-ready` → `learn --post-merge`. Tasks are
slug-identified (no BEADS in this repo — see the metaswarm-integration note in the spec).

## How to read this

- Epics group tasks by the spec's own **Ship 1 / Ship 2** split (§6) plus the enforcement-hardening
  layer (§9) and the validation layer (§7–§8). **This plan does not re-decide the design** — it maps
  the approved spec to buildable units; where it and the spec disagree, the spec wins.
- Every task carries: **spec refs**, **depends-on**, a **DI contract** (what is injected, what is
  mocked), a **TDD contract** (the test observed failing first + the mutation that must redden it),
  **acceptance criteria**, and **status**.
- **Global build rule (spec §7 L1):** no code lands without a test observed failing without it; every
  fix is mutation-verified; `verify.go` must produce all four `PinOutcome` values from their distinct
  causes.
- **Global DI rule (project convention):** each node/gate sits behind an interface; the test-command
  runner, git access, and the AI judge/agent are **injected**; the AI is **mock-AI** in every unit
  and integration test (`mockai`). A `mock:true` FSM row never satisfies a gate.
- **⧗ = unresolved prerequisite** (a spike or a design decision) that gates the tasks depending on it.

## Dependency spine (the critical path)

```
E0 finding-identity (⧗ recall+precision floors)
   └─> E1 Ship 1: Pins  ── shippable, honest "added lines are test-guarded under mutation"
          └─> E2 enforcement hardening (§9.5 also needs E3.classify)
   └─> E3 Ship 2: Bug.Class  (also needs ⧗ continuity + ⧗ end-to-end-replay spikes)
E4 validation/efficacy runs parallel; gates the efficacy CLAIM, not the loop.
```

`require_pins`/`Unproven` key on `Finding` identity, so **E0 gates Ship 1's self-containment** — it is
a Ship-1 task, open (⧗), not "done" (spec §6 corrects the earlier "de-risked" reading).

---

## Epic E0 — Cross-round-stable finding identity (⧗ prerequisite, gates E1 and E3)

**Why an epic of its own:** `Unproven` clears/re-adds on `Pin.Finding`; a re-discovered finding that
got a *new* id never clears its old gap — a recall/false-split failure. The spike measured only the
**precision** direction (0% false-merge over 611); the **recall-under-paraphrase** direction is
**unmeasured** and is the actual blocker (spec §6, §5).

### T0.1 — Paraphrase ground-truth set + recall/precision floors
- **Spec:** §6 Ship-1 prerequisite, §5, §9.8.
- **Depends-on:** — (first task).
- **DI:** the identity function is pure `(file, normalized-text) -> key`; no AI. The corpus loader is
  injected so the floor test runs off a fixture, not live runs.
- **TDD:** lock a paraphrase ground-truth set (same fault, reworded) BEFORE tuning; a recall-floor
  test fails on the current lexical key if paraphrases split; a precision-floor test fails if distinct
  findings merge; a same-text/different-file regression test guards the 0%-false-merge property.
- **Acceptance:** BOTH floors (recall on paraphrase set, precision on the 611-corpus) are met and the
  algorithm is **frozen** only then. Embeddings are advisory-only, never the id (§9.8 — deterministic
  id is mandatory).
- **Status:** OPEN (⧗). Not de-risked; the precision spike does not cover recall.

---

## Epic E1 — Ship 1: Pins / proof-of-fix (shippable core)

Honest claim: *"added lines are test-guarded under mutation,"* never "proof." Complete, standalone
improvement over the fix node taking the agent's word (spec §6 Ship 1). Depends on E0 for
self-containment.

### T1.1 — Port the pins + DifferentialProof data model (§2, §9.1)
- **Spec:** §2.1–§2.4, §9.1.
- **Depends-on:** —.
- **DI:** pure types + a `Reduce`/`Fold` over injected `Delta`; no external deps.
- **TDD:** table tests for each `PinOutcome` value and the `DifferentialProof` one-of invariant
  (`Kind:"pin"` ⇒ `Pin` set/`Deletes` nil; `"deletion"` ⇒ `Deletes` set; `"reproduction"` ⇒ both nil);
  a decode test rejects a malformed record (one-of violated). Mutation: dropping the one-of check must
  redden a test.
- **Acceptance:** `Pin`/`DifferentialProof`/`PinResult`/`ProofResult`/`PinOutcome`/`Unproven` and
  `Finding.Source`/`Category` land; `SchemaVersion` stays 1, additive-optional (the 10 existing runs
  still `Fold`); `DifferentialProof.ID` is the override key.
- **Status:** PARTIAL — pins subset done on `pins-proof-of-fix` (prove engine ported; `Pin{ID,Finding}`,
  `PinResult`, `PinOutcome`, `Unproven`). Remaining: the `DifferentialProof`/`ProofResult`
  generalization (Kind/one-of) per §9.1.

### T1.2 — Fix the `agentEdit.Reduce` seam + the gate-can-fail regression test
- **Spec:** §6 chunk 2; the #24 vacuous-pass (`Reduce` dropped `Pins` → `pins_proven` vacuously true).
- **Depends-on:** T1.1.
- **DI:** `Reduce` is a pure function of `(prev, delta)`; tested directly.
- **TDD:** the regression test is authored **red first** — a `Reduce` that drops `Pins` makes
  `pins_proven` pass on an unproven fix; the fixed `Reduce` reddens then greens it. Mutation: the
  `Reduce`-drops-`Pins` mutant MUST redden this test (spec §7 L2 "the one that matters most").
- **Acceptance:** the seam carries `Pins`/`PinResults` end-to-end; a witnessed red and green exist.
- **Status:** OPEN.

### T1.3 — Wire `prove`/`pins_proven` into `sdlc-loop-proved`
- **Spec:** §6 chunk 3, §3.1, §9.1 (the deterministic differential gate).
- **Depends-on:** T1.1, T1.2, E0 (for `Unproven` clear-by-`Finding`).
- **DI:** `prove` takes an **injected test-command runner** and **git access**; the consent-hashed cmd
  is invoked opaque (no coverage instrumentation assumed, §4.2). Mock-AI for the fix node in tests.
- **TDD:** L2 integration — a fix emitting a pin that does NOT hold its line drives `pins_proven`
  **red**; the added-line bind rejects a pin whose `From` is not a `+` line; `Unproven` add/clear/re-add
  lifecycle has witnessed transitions. Mutation-verify each gate predicate.
- **Acceptance:** added-line bind (R1) + reduced owes-a-pin quantifier with **own-file clause (a)** +
  deletion valve (R3) + `Unproven` lifecycle (R4) all wired; every gate has a witnessed red and green
  (§7 L2). Honest property asserted in output; never "proof."
- **Status:** OPEN.

### T1.4 — Right-reason veto-only reviewer (§9.2)
- **Spec:** §9.2, §9.1 (own-file binding migration for the reproduction form).
- **Depends-on:** T1.3.
- **DI:** the symptom-match reviewer is an **injected AI judge** (mock-AI in tests); it is NOT a
  deterministic gate.
- **TDD:** a mismatch raises `NEEDS_REVISION`; a match never yields a PASS (the deterministic
  differential is the sole PASS path); **fail-closed** — a mock reviewer timeout/error/garbage output
  reddens (blocks), never approves. Mutation: turning the error path into "approve" must redden.
- **Acceptance:** veto-only behavior + fail-closed hold; the own-file binding is discharged here for
  the reproduction form (retired from clause (a) per §9.1).
- **Status:** OPEN.

### T1.5 — Deletion as a first-class provable fix (§9.4)
- **Spec:** §9.4, §9.1 (`Kind:"deletion"`), §4.6 (deletion is no longer a valve).
- **Depends-on:** T1.1, T1.3.
- **DI:** `DeletionRef` verification uses **injected git access** (parent blob + diff parse).
- **TDD:** a `Kind:"deletion"` proof with a fail-before/pass-after reproduction test clears; the
  `DeletionRef`↔reviewed-diff binding (⧗ build contract) rejects a free-floating or mismatched span;
  clause (a) own-file for a deletion requires `DeletionRef.File == Finding.File` (NOT `Root.File` — that
  is §9.5's separate gate). Mutation: dropping the diff-binding must redden.
- **Acceptance:** a pure deletion is provable (no added line needed); a bogus `DeletionRef` cannot hold
  F2P.
- **Status:** OPEN.

### T1.6 — Self-validating-loop anti-gaming guards (§9.3) + trivial-pin reject (§9.8 R7)
- **Spec:** §9.3, §9.8 R7, §2.2 (`malformed` widened).
- **Depends-on:** T1.1, T1.3.
- **DI:** AST pre-screen is pure; the "capture failing test on the untouched tree first" step uses the
  injected git/runner.
- **TDD:** the committed-test guard reddens if a test is deletable without a coverage/mutation signal
  (links to E2.T2.1); the pre-fix run must be an assertion failure not a compile error; a
  comment/whitespace/dead-code pin is classified `malformed` by the pre-screen **before** the break
  step (never surfaces as `survived`). Mutation: removing the pre-screen must let a trivial pin reach
  `survived` and redden the reject-test.
- **Acceptance:** the three §9.3 guards + the R7 pre-screen hold.
- **Status:** OPEN.

---

## Epic E2 — Enforcement hardening (attaches after E1 core)

### T2.1 — Test-deletion gate (§9.6)
- **Spec:** §9.6.
- **Depends-on:** T1.3 (reuses the pins/mutation engine).
- **DI:** injected runner; a **coverage-profile probe** that reports whether the cmd yields a per-line
  profile (§4.2: opaque cmd may not).
- **TDD:** mutation-kill non-regression is the deterministic gate (a previously-killed mutant surviving
  after a test deletion reddens); coverage non-regression runs **only where a line profile is
  obtainable** (before/after diff), and is recorded unavailable otherwise; a mutant whose target span
  was removed in the same commit is excluded. Mutation: the mutant-identity collision test.
- **Acceptance:** a code+test paired deletion passes; a sole-detector test deletion reddens; the gate
  degrades to mutation-only under an opaque cmd.
- **Status:** OPEN.

### T2.2 — Trajectory monitor (§9.7)
- **Spec:** §9.7.
- **Depends-on:** T1.3.
- **DI:** the monitor reads the **injected diff + override records** for the derivable flags; the
  oracle-edit/answer-lookup flags depend on a **⧗ fix-trajectory record** (a new additive-optional
  §2.4 field, schema deferred to build).
- **TDD:** whitespace/comment-only and self-served-override flags fire from a fixture diff/override log;
  the trajectory-dependent flags are gated on the ⧗ record and tested once it exists.
- **Acceptance:** the diff/override flags ship; the trajectory flags are cleanly blocked on the ⧗ record.
- **Status:** OPEN (partly ⧗).

### T2.3 — Guard-and-Go rejection (§9.5) — **gated on E3 classify**
- **Spec:** §9.5, §9.4.
- **Depends-on:** T1.5 **and E3.T3.1** (needs `classify`'s `Remedy` tag + `BugClass.Root`).
- **DI:** the `DeletionRef`↔`Root` match is deterministic; `classify` supplies `Remedy`/`Root`.
- **TDD:** a `Remedy:"structural"` class requires a fix `DeletionRef` whose `File==Root.File` and whose
  `Removed` contains `Root.Span`; an additive-only "guard around the root" fix does NOT close the class.
  Mutation: dropping the match lets the guard-around dodge pass.
- **Acceptance:** the additive dodge of a required deletion is rejected for structural classes.
- **Status:** BLOCKED on E3.T3.1.

---

## Epic E3 — Ship 2: Bug.Class (gated on ⧗ spikes)

**⧗ prerequisites (each a small design+spike before code):** E0 finding identity; a
durable-identity/continuity spike (classify continues open classes without id churn); an end-to-end
replay spike (class created → partially fixed → carried → cleared only on member `pins_proven`) — the
acceptance test for the whole path. `classify` is present from the first shipped version (no
fixer-grouping interim, §3.2 inv. 4).

### T3.0 — ⧗ Continuity + end-to-end-replay spikes
- **Spec:** §5, §6 Ship-2 prerequisites, §3.2.
- **Depends-on:** E0.
- **Acceptance:** continuity spike shows classify continues an open class across rounds without id
  churn; replay spike demonstrates the full create→carry→clear path on fixture data. Both **pre-locked**.
- **Status:** OPEN (⧗).

### T3.1 — `classify` node
- **Spec:** §3.2, §6 chunk 4, §4.4/§4.5.
- **Depends-on:** T3.0.
- **DI:** `classify` is a set-level **injected AI** call (mock-AI in tests) over open `Snapshot.Classes`;
  grouping is **advisory with a recorded-reason override** (§4.5), not binding.
- **TDD:** dedup-by-defect `(file, normalized-text)` collapses duplicates before the call; a blind
  classify pass over the locked L3 corpus meets the numeric bar (T4.1); dissolution keyed on
  `BugClass.ID`. Mutation: the id-minting/carry path.
- **Acceptance:** minted carried `BugClass.ID`s; errs toward splitting (safe) not lumping; advisory.
- **Status:** OPEN (⧗).

### T3.2 — `require_classes` + `classes_enumerated`
- **Spec:** §3.2, §6 chunk 5.
- **Depends-on:** T3.1, T1.3.
- **DI:** deterministic gates over the enumerated members + the diff.
- **TDD:** `classes_enumerated` reddens when an enumerated `fixed` instance is absent from the diff
  (the §5 class-in-diff probe); the **own-location** member check binds the proven line and the class
  gate's touched line to the same code; a `fixed-elsewhere` disposition resolves via a Proven proof in
  the remedy file. Mutation: dropping own-location re-opens the #24 self-clearing shape.
- **Acceptance:** a class clears only on validated member resolution; dissolution/override keyed on the
  durable `BugClass.ID`.
- **Status:** OPEN (⧗).

### T3.3 — Post-merge recurrence abuse detector
- **Spec:** §3.2, §6 chunk 6.
- **Depends-on:** T3.2, `learn --post-merge`.
- **Acceptance:** sibling recurrence after a class fix is measured post-merge (the abuse detector the
  advisory valves rely on) — or the unguarded valves are recorded as a scoped decision.
- **Status:** OPEN (⧗).

---

## Epic E4 — Validation & efficacy (parallel; gates the efficacy claim, not the loop)

### T4.1 — L3 behavioural corpus + the numeric bar
- **Spec:** §7 L3.
- **Depends-on:** —.
- **Acceptance:** planted-defect fixtures with **pre-locked** ground truth; absolute guardrails (a
  proven fix never blocked; a confirmed finding never dropped by grouping; a planted unproven fix never
  passed) at 100%; the numeric bar (recall floor, max dangerous-merge rate, corpus size, protocol) is
  **defined here, not deferred** (the classify spike was viability-only, N=1, post-hoc ground truth).
- **Status:** OPEN.

### T4.2 — Control-arm efficacy harness
- **Spec:** §8.
- **Depends-on:** T1.3 (needs a working proved loop to compare).
- **Acceptance:** full arm vs control arm (post-Stop-hook, pins+classify disabled); both arms replayed
  from an **identical pre-fix confirmed-finding state per bug** (the isolation §8 flags as unsolved via
  post-hoc intersection). Metrics 1–4 reported; metric-1 needs E0's stable identity.
- **Status:** OPEN (design gap called out in §8 — resolve before claiming efficacy).

### T4.3 — Cross-region sibling oracle (metric 2)
- **Spec:** §8 (open caveat), §4.4.
- **Depends-on:** T3.2.
- **Acceptance:** a cross-file-capable sibling oracle (the §4.4 same-file signature cannot see the
  cross-file "top and bottom" class that is classify's reason to exist) measures metric 2.
- **Status:** OPEN.

### T4.4 — Gold acceptance: replay #24–#28
- **Spec:** §8 "the gold acceptance test."
- **Depends-on:** E1 (and E3 for the class claims).
- **Acceptance:** replaying #24–#28 through the proved loop reproduces the expected proven/blocked
  outcomes end-to-end.
- **Status:** OPEN.

### T4.5 — ⧗ Escape-hatch probe (`unverifiable` blocks)
- **Spec:** §5 remaining spike, §4.1.
- **Depends-on:** T1.3.
- **Acceptance:** a fix that deliberately makes a pin `unverifiable` (delete the test cmd) is blocked
  by decision #1's semantics, and the override keyed on `DifferentialProof.ID` is physically pullable.
- **Status:** OPEN (⧗).

---

## Sequencing summary

1. **E0.T0.1** (finding identity) — unblocks E1 self-containment and E3.
2. **E1** T1.1→T1.2→T1.3, then T1.4/T1.5/T1.6 (parallelizable after T1.3). Ship 1 is releasable here.
3. **E2** T2.1/T2.2 after T1.3; **T2.3 waits on E3.T3.1**.
4. **E3** T3.0 spikes → T3.1 → T3.2 → T3.3.
5. **E4** runs alongside; T4.2/T4.4 need a working proved loop; the efficacy **isolation design gap
   (§8) is resolved before any efficacy number is claimed**.

**Current state:** E1.T1.1 is PARTIAL on `pins-proof-of-fix` (pins subset, chunks 1–2). Everything
else is OPEN; ⧗ tasks carry an unresolved spike/decision and must not be built past their gate.
