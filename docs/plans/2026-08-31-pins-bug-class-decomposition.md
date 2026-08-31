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
  id is mandatory). The `(file, normalized-text)` key is applied at **all six `BugID`-deriving sites**
  (§3.3: `dedupCandidates`, the `allIDs` preflight `kind.go:489`, `dedupBugs` `kind.go:563`,
  matched-golden `kind.go:517`, and the second-opinion branches `kind.go:594/602/605`) with a
  same-text/different-file regression over every path. **Override migration (§2.4 round-2, §3.3):**
  because this changes the id derivation, keyed overrides are **migrated or versioned on the change,
  never silently** — an override keyed on a pre-change id must not orphan.
- **Status:** OPEN (⧗). Not de-risked; the precision spike does not cover recall.

---

## Epic E1 — Ship 1: the differential-proof gate — reproduction preferred, pin fallback (shippable core)

§9.1 realignment: `prove` is **ONE differential-test gate**. A **reproduction test is the preferred,
strictly-dominant form** (it also proves the assertion targets the finding); the **mutate-a-line pin
is the fallback** when no real failing input can be constructed. Honest claim, both halves (§9.1): a
proven **reproduction test says the behavior is under test**; a proven **pin says the added line is** —
never "proof"/"correct." Complete, standalone improvement over the fix node taking the agent's word
(spec §6 Ship 1). Depends on E0 for self-containment.

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
  still `Fold`); `DifferentialProof.ID` is the override key. **One-way-door gate (§2.4):** the first
  deploy that writes these new `Delta` fields is gated as irreversible — once a run persists them, an
  older/rolled-back binary can no longer `Fold` that run; there is **no downgrade path**, and the
  rollout must record that gate (a deploy-time decision, not silent).
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

### T1.3 — Build the unified `prove`/`pins_proven` differential gate (BOTH proof forms) + wire into `sdlc-loop-proved`
- **Spec:** §6 chunk 3, §3.1, §9.1 (ONE differential-test gate: reproduction preferred, pin fallback),
  §9.1 execution contract for a committed reproduction/deletion test.
- **Depends-on:** T1.1, T1.2, E0 (for `Unproven` clear-by-`Finding`).
- **DI:** `prove` takes an **injected test-command runner** and **git access**; the consent-hashed cmd
  is invoked opaque (no coverage instrumentation assumed, §4.2). Mock-AI for the fix node in tests.
  The reproduction-execution engine and the pin mutate/restore engine sit behind one `prove` interface.
- **Reproduction path (PREFERRED) — the §9.1 execution engine:** (a) check out the **pre-fix** tree,
  (b) **overlay only the test-only files** the fix adds/changes (helpers included, never production
  code), (c) run the target test — it must **FAIL with an assertion (not compile/import) error**,
  (d) apply the full fix, (e) re-run — it must **PASS**. This engine is **also what T1.5's deletion
  proof runs on** (a deletion is reproduction-execution-shaped — fail-before/pass-after via a committed
  test — though its `Kind` is `"deletion"` per §2.1/§9.1; T1.5 reuses this engine).
- **Pin path (FALLBACK):** added-line bind — reject a pin whose `From` is not a `+` line; mutate→fail,
  restore→pass.
- **TDD:** L2 integration — (repro) a reproduction test that does NOT fail-before, or a pre-fix run
  that errors on **compile not assertion**, drives `prove` red; a genuine fail-before/pass-after
  clears. (pin) a fix emitting a pin that does NOT hold its line drives `pins_proven` **red**; the
  added-line bind rejects a non-`+` `From`. `Unproven` add/clear/re-add lifecycle has witnessed
  transitions. Mutation-verify each gate predicate — e.g. a mutant that **accepts a compile/import
  error as a valid fail-before** must redden the assertion-vs-compile test.
- **Acceptance:** the differential gate accepts a **reproduction proof (preferred)** via the execution
  engine above, and a **pin (fallback)**; owes-a-proof quantifier with **own-file clause (a)** per
  §9.1's per-kind rule (pin: `File==Finding.File`; reproduction: discharged by T1.4's §9.2 reviewer);
  deletion valve (R3) + `Unproven` lifecycle (R4) wired; every gate has a witnessed red and green
  (§7 L2). Honest property asserted per form (behavior-under-test vs added-line-guarded); never "proof."
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
- **DI:** `DeletionRef` verification uses **injected git access** (parent blob + diff parse). The
  fail-before/pass-after check **runs on T1.3's reproduction-execution engine** — this task adds
  `DeletionRef` verification + the over-deletion scope check, NOT a second execution engine.
- **TDD:** a `Kind:"deletion"` proof with a fail-before/pass-after reproduction test clears; the
  `DeletionRef`↔reviewed-diff binding (⧗ build contract) rejects a free-floating or mismatched span;
  clause (a) own-file for a deletion requires `DeletionRef.File == Finding.File` (NOT `Root.File` — that
  is §9.5's separate gate). Mutation: dropping the diff-binding must redden.
- **Acceptance:** a pure deletion is provable (no added line needed); a bogus `DeletionRef` cannot hold
  F2P; a **paired whole-suite scope check** guards the ~26% over-deletion risk (§9.4 honest-limits — a
  proven deletion means "removed this fixed the bug and broke no *existing* test," not "globally
  safe"), so a deletion that breaks a test outside the reproduction set reddens.
- **Encouragement lever (§9.4 R10):** the fix prompt hands the agent **exact deletion spans, not
  prose** (spans move models 6.5–31.5 pts; prose does nothing).
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
  was removed in the same commit is excluded. Mutation: collapsing the mutant identity to
  `(File, span-content)` — dropping the site+operator — must **redden** a collision test where the same
  mutated content recurs at two sites or two operators hit one span.
- **Acceptance:** a code+test paired deletion passes; a sole-detector test deletion reddens; the gate
  degrades to mutation-only under an opaque cmd.
- **Status:** OPEN.

### T2.2 — Trajectory monitor (§9.7) + verbosity flag (§9.8 R9)
- **Spec:** §9.7, §9.8 R9.
- **Depends-on:** T1.3.
- **DI:** the monitor reads the **injected diff + override records** for the derivable flags; the
  oracle-edit/answer-lookup flags depend on a **⧗ fix-trajectory record** (a new additive-optional
  §2.4 field, schema deferred to build).
- **TDD (positives AND discriminating negatives):** whitespace/comment-only and self-served-override
  flags **fire** on their fixtures; and — the load-bearing negatives — a **substantive non-whitespace
  diff must NOT raise the whitespace flag**, and a **request+grant by two DISTINCT actors must NOT
  raise the self-served flag** (the requester==granter distinction §4.1 enforces). The R9 verbosity
  flag fires when a fix is materially larger than the minimal change (Guard-and-Go median 1.67×) and
  NOT on a minimal fix. **Mutation:** collapsing the requester/granter comparison to a constant
  (`selfServed()`→true) or stubbing `whitespaceOnly()`→true must **redden** a negative test. The
  trajectory-dependent flags are gated on the ⧗ record and tested once it exists.
- **Acceptance:** the diff/override + verbosity flags ship and discriminate (no over-flagging); the
  trajectory flags are cleanly blocked on the ⧗ record.
- **Status:** OPEN (partly ⧗).

### T2.3 — Guard-and-Go rejection (§9.5) — **gated on E3 classify**
- **Spec:** §9.5, §9.4.
- **Depends-on:** T1.5, **E3.T3.1** (produces `classify`'s `Remedy` tag + `BugClass.Root`), **and
  E3.T3.2** (§9.5 is a tightening clause ON `classes_enumerated` + the class-closing path, which T3.2
  builds — there is no gate for it to attach to until then).
- **DI:** the `DeletionRef`↔`Root` match is deterministic; `classify` supplies `Remedy`/`Root`.
- **TDD:** a `Remedy:"structural"` class requires a fix `DeletionRef` whose `File==Root.File` and whose
  `Removed` contains `Root.Span`; an additive-only "guard around the root" fix does NOT close the class.
  Mutation: dropping the match lets the guard-around dodge pass.
- **Acceptance:** the additive dodge of a required deletion is rejected for structural classes.
- **Status:** BLOCKED on E3.T3.1 and E3.T3.2.

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

### T3.1 — `classify` node (+ class data model + `Remedy`/`Root` production)
- **Spec:** §2.3/§2.4 (class types), §3.2, §6 chunk 4, §4.4/§4.5, **§9.5** (`Remedy` tag + `BugClass.Root`).
- **Depends-on:** T3.0, **T4.1** (the numeric bar its acceptance test measures against must be locked first).
- **DI:** `classify` is a set-level **injected AI** call (mock-AI in tests) over open `Snapshot.Classes`;
  grouping is **advisory with a recorded-reason override** (§4.5), not binding. Reading `Root.Span`
  source text from a finding's file+line uses **injected file/git access**.
- **Also ports the class data model:** `BugClass`/`ClassMember`/`FixClass`/`FixInstance` (§2.3/§2.4),
  additive-optional, `SchemaVersion` unchanged — the class-side analogue of T1.1. **One-way-door gate
  (§2.4, review #2):** `Delta.Classes`/`FixClasses` first reach the wire HERE, in a Ship-2 deploy
  separate from T1.1's — so the first deploy that persists them is its own irreversible one-way door
  (a Ship-1 rollback binary cannot `Fold` a run carrying these fields; `DisallowUnknownFields` rejects
  them). Gate it as recorded/no-downgrade, exactly as T1.1 gates the pins fields. The T3.4 tombstone
  map rides these class carriers and is covered by this same gate.
- **Produces (consumed by T2.3/T3.2):** each `BugClass` tagged `Remedy` (`"structural"` | `"local"`);
  for a structural class, a `BugClass.Root{File, Span}` where `Span` is **actual source text** read
  from the member's file at its line (⧗ build-time data-availability — a line number is not enough).
- **TDD:** dedup-by-defect `(file, normalized-text)` collapses duplicates before the call; a blind
  classify pass over the locked L3 corpus (T4.1) meets the numeric bar; a structural class emits a
  non-empty `Root.Span` of real source text (a fixture finding with only file+line yields the span);
  dissolution keyed on `BugClass.ID`. **Mutation:** an id-minting mutant that re-mints an existing
  class's id on carry (instead of continuing it) must redden the carry test.
- **Acceptance:** minted carried `BugClass.ID`s; `Remedy`/`Root` produced per above; errs toward
  splitting (safe) not lumping; advisory.
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

### T3.4 — ⧗ Class-merge tombstone rule
- **Spec:** §3.2 edge cases ("two open classes merge … one id continued, the other marked merged-into
  (a tombstone carrying the surviving id) so overrides keyed on the retired id still resolve — ⧗ needs
  the tombstone rule specified").
- **Depends-on:** T3.1 (the id mint/carry lifecycle this extends).
- **DI:** deterministic; the tombstone map is carried state (additive-optional §2.4).
- **TDD:** when `classify` merges two open classes, one `BugClass.ID` is continued and the other becomes
  a **tombstone carrying the surviving id**; an override keyed on the **retired** id still resolves via
  the tombstone. Mutation: dropping the tombstone lookup orphans the retired-id override (reddens).
- **Acceptance:** a merged-away class id never silently stops resolving its overrides.
- **Status:** OPEN (⧗ — the rule must be specified before code, per §3.2).

### T3.3 — Post-merge recurrence abuse detector
- **Spec:** §3.2, §6 chunk 6.
- **Depends-on:** T3.2, `learn --post-merge`.
- **Verification:** on a fixture pair of runs, a sibling recurrence after a class fix is counted; a
  clean run reports zero — the measurement is falsifiable against a known-answer fixture, not just
  asserted. (Post-merge measurement task — L4-flavored, not a unit gate.)
- **Acceptance:** sibling recurrence after a class fix is measured post-merge (the abuse detector the
  advisory valves rely on) — or the unguarded valves are recorded as a scoped decision. **Encouragement
  lever (§9.4):** post-merge learning **rewards** a class resolved by a `DeletionRef` (structural
  simplification — the inverse of the verbosity flag).
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
- **Precondition (§8):** per fixture, verify both siblings **co-exist at one base ref** (they surfaced
  across five sequential PRs and may not) before asserting a class; and because grouping is
  **non-deterministic**, run each fixture **N times with a required hit-rate over `BugClass.Members`**
  rather than a single pass. Without these the gold test is silently un-runnable or flaky.
- **Acceptance:** replaying #24–#28 through the proved loop reproduces the expected proven/blocked
  outcomes end-to-end, with the precondition satisfied.
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
2. **E1** T1.1→T1.2→T1.3 (T1.3 builds BOTH proof forms — reproduction preferred + pin fallback), then
   T1.4/T1.5/T1.6 (parallelizable after T1.3; T1.5's deletion proof runs on T1.3's reproduction
   engine). Ship 1 is releasable here.
3. **E2** T2.1/T2.2 after T1.3; **T2.3 waits on E3.T3.1 AND E3.T3.2** (it tightens the T3.2 gate).
4. **E3** T3.0 spikes → **T4.1 (numeric bar) →** T3.1 (builds classify + class types + Remedy/Root) →
   T3.2 → {T3.4 tombstone (after T3.1), T3.3 post-merge}.
5. **E4** runs alongside; **T4.1 precedes T3.1**; T4.2/T4.4 need a working proved loop; T4.4's §8
   precondition (sibling co-existence + N-runs hit-rate) is checked first; the efficacy **isolation
   design gap (§8) is resolved before any efficacy number is claimed**.

**Current state:** E1.T1.1 is PARTIAL on `pins-proof-of-fix` (pins subset, chunks 1–2). Everything
else is OPEN; ⧗ tasks carry an unresolved spike/decision and must not be built past their gate.
