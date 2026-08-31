# metareview context: docs/plans/2026-08-31-pins-bug-class-decomposition.md

Run ID: `mrv-20260831-211218885790000-artifact-2026-08-31-pins-bug-class-decomposition-9ce5428b`

## Target

- Path: `docs/plans/2026-08-31-pins-bug-class-decomposition.md`
- Repository mode: `metaswarm-extension`
- Git branch: `decompose-pins-bug-class`
- Git head: `0bac9cf`

## Artifact Excerpt

````markdown
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

### T1.1
````

## Service Inventory

No service inventory found.

## Knowledge Facts

No Beads knowledge facts found.

## Suggested Reviewers

- Feasibility
- Completeness
- Scope and alignment
- Architecture
- Intent preservation
- Security
- Testing-quality
- Data-migration
- Mechanical-precision
