# metareview context: docs/specs/2026-09-04-lights-out-sdlc-factory.md

Run ID: `mrv-20260905-055717529859000-artifact-2026-09-04-lights-out-sdlc-factory-e7ed2a69`

## Target

- Path: `docs/specs/2026-09-04-lights-out-sdlc-factory.md`
- Repository mode: `metaswarm-extension`
- Git branch: `main`
- Git head: `c0b933b`

## Artifact Excerpt

```markdown
# A lights-out software factory on the metareview FSM

**Status:** draft for review · **Date:** 2026-09-04 · **Builds on:** issue #2 (*epic: make metareview
verdicts stateful, shardable, and evidence-backed*), `docs/ARCHITECTURE.md`, the 0.9.0 FSM specs.
**Supersedes in part:** the "review gates only" boundary in `docs/integrations/metaswarm.md` and the
memory note *metareview-decompose-build-flow* ("metareview is a review/gate harness, not a decomposition
engine") — that boundary is deliberately being moved.

> **Goal.** A *lights-out software factory*: given an intent (an issue, a spec, a one-line ask), the system
> brainstorms → designs → specifies → decomposes → builds → reviews → fixes → verifies → ships → closes →
> learns, **unattended**, with every invariant that metaswarm and Superpowers currently ask a model to
> honor turned into a machine-checked gate, and every "ask the human" moment turned into either a policy
> default or a typed escalation that parks durably and resumes.

---

## 0. Executive summary

Three systems were inventoried in full (§1). The finding that shapes everything else:

- **metaswarm** covers the whole lifecycle but is **entirely prompt-enforced**. Its 4-phase loop, its
  design/plan gates, its 3-retry counters, its pr-shepherd state machine are Markdown. Its own
  `docs/coverage-enforcement.md` calls the agent checklist "the weakest gate". The deterministic pieces are
  a session-start self-heal script, a coverage pre-push hook, two PR-comment shell scripts, two external-tool
  adapters, and `gtg` (external).
- **Superpowers** supplies the *method* (brainstorm → plan → SDD/TDD → review → finish) with ~70
  observable invariants — and **zero enforcement code**. Every rule is honored by the model or not.
- **metareview** has the opposite profile: a hash-chained, replayable, consent-gated **deterministic FSM**
  with a real adjudicating judge, differential proofs (mutation-verified fixes), sharded review, evidence
  receipts, run lineage with escalation, overrides, and a git-native push gate — but it covers only the
  *review / fix / verify* third of the lifecycle. It authors nothing, schedules nothing, never runs `bd`,
  never writes to GitHub, never merges, never closes.

So the factory is not "metareview plus metaswarm's prompts". It is **metareview's engine extended to carry
the other two-thirds of the lifecycle as typed workflow families**, with the existing bug loop
(`sdlc-loop-proved`) called *fractally* as the unit that implements and verifies one task, an epic loop
that schedules dependency-free tasks into it and then runs the epic-level integration review, and a
factory loop that wraps design → decompose → epic-build → ship → close. metaswarm's and Superpowers'
rubrics, prompt recipes, and measured design lessons are reused as **node prompts and rubrics**, not as the
control plane.

The phased plan (§9) has seven phases. Phases 0–2 produce the fractal build unit and the epic loop —
enough to build the remaining phases *with* the factory (§12, dogfooding). Issue #2's open items are all
absorbed (§10): D1–D6 land in Phases 1–2, E in Phase 3, F in Phase 4, G in Phase 5.

---

## 1. Sources and method

Three exhaustive inventories were produced (read every file; each claim cited to a path):

| Inventory | Source | Notes |
|---|---|---|
| metaswarm v0.12.0 | `../metaswarm` (242 files) | 14 skills, 16 commands, 19 agent personas, 9 rubrics, 6 guides, 22 templates, hooks, scripts |
| Superpowers v6.0.3 | `~/.claude/plugins/cache/claude-plugins-official/superpowers/896224c4b187/` | 14 skills + prompt templates + SDD scripts + the project's own measured design specs |
| metareview 0.10.1 | this repo @ `c0b933b` | every command/flag, node kind, gate, rubric, state file; `gh issue view 2 --comments` |

The inventories are working documents in the session scratchpad; the facts that matter are restated here
with their citations so this spec sta
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
