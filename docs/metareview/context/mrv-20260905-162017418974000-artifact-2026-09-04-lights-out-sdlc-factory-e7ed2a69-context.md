# metareview context: docs/specs/2026-09-04-lights-out-sdlc-factory.md

Run ID: `mrv-20260905-162017418974000-artifact-2026-09-04-lights-out-sdlc-factory-e7ed2a69`

## Target

- Path: `docs/specs/2026-09-04-lights-out-sdlc-factory.md`
- Repository mode: `metaswarm-extension`
- Git branch: `main`
- Git head: `c0b933b`

## Artifact Excerpt

```markdown
# A lights-out software factory on the metareview FSM

**Status:** reviewed, revision 10 (after nine artifact-review rounds; see §15) · **Date:** 2026-09-04
(rev 6: 2026-09-05) · **Builds on:** issue #2 (*epic: make metareview verdicts stateful, shardable, and
evidence-backed*), `docs/ARCHITECTURE.md`, the 0.9.0 FSM specs.

**Supersedes in part (declared, see §13):** (a) the "review gates only, metaswarm owns the lifecycle"
boundary in `docs/integrations/metaswarm.md`, `CLAUDE.md` *Durable Output*, `AGENTS.md` *Metaswarm Fit*,
`docs/ARCHITECTURE.md` §7, and the memory note *metareview-decompose-build-flow*; (b) `ARCHITECTURE.md`
§3's rule that "the FSM stays scope-agnostic; the agent bridges its run into a marker" — the engine writes
the review-evidence marker for a child run it verified; (c) the engine's "every failure is terminal,
recovery is a fork" error model (0.9.0 core spec §5.1) — a non-terminal *await*, a retryable error class,
routed convergence, and a machine-level cancel are added; (d) `docs/fsm/driving-a-workflow.md`'s caveat
that the audit chain is "process guarantees for a cooperating agent" — the factory's host is not a
cooperating agent, so §5.12 adds a trust boundary.

> **Goal.** A *lights-out software factory*: given an intent (an issue, a spec, a plan, a one-line ask), the
> system designs → specifies → decomposes → builds → reviews → fixes → verifies → ships → closes → learns,
> **unattended up to a PR that is ready to merge**, with every invariant that metaswarm and Superpowers
> currently ask a model to honor turned into a machine-checked gate, and every "ask the human" moment
> turned into either a policy default or a typed escalation that parks durably and resumes in place.
> Merge itself always requires one act the factory cannot perform (§5.12.5; decision §12.8, accepted).
> "Deploy" is verified through runtime receipts from declared commands; the factory does not own a
> pipeline (§11).

---

## 0. Executive summary

Three systems were inventoried in full (§1). The finding that shapes everything else:

- **metaswarm** covers the whole lifecycle but is **entirely prompt-enforced**. Its 4-phase loop, its
  design/plan gates, its 3-retry counters, its pr-shepherd state machine are Markdown. Its own
  `docs/coverage-enforcement.md` calls the agent checklist "the weakest gate".
- **Superpowers** supplies the *method* (brainstorm → plan → SDD/TDD → review → finish) with ~70
  observable invariants — and **zero enforcement code**.
- **metareview** has the opposite profile: a hash-chained, replayable, consent-gated **deterministic FSM**
  with a real adjudicating judge, differential proofs, sharded review, evidence receipts, run lineage with
  escalation, overrides, and a git-native push gate — but it covers only the *review / fix / verify* third
  of the lifecycle. It authors nothing, schedules nothing, never runs `bd`, never writes to GitHub, never
  merges, never closes.

So the factory is **metareview's engine extended to carry the other two-thirds of the lifecycle**, with the
existing bug loop wrapped — not replaced — by a new `task-build-loop` (implement → engine-verified
TDD/scope/validation → `fix-loop-proved-clean` → mutation → marker) that is called *fractally*: an
`epic-build-loop` schedules dependency-free tasks into it and then runs the epic-level integration review;
a shared `finalize-loop` produces the epic-ready and pr-ready markers at the pushed head; a `factory`
loop wraps intake → design → decompose → epic-build → finalize → ship → release. metaswarm's and
Superpowers' rubrics, prompt recipes, and measured design lessons are reused as **node prompts and
rubrics**, not as the control plane.

Four review rounds fixed the design structurally: the engine's error model cannot carry a lights-out
escalation, so §5.3 adds `await`, a retryable error class, routed convergence with answer-re-armed
windows, cancel, and parse ru
```

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
