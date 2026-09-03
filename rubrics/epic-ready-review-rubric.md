# Epic-Ready Review Rubric

Use for `metareview review epic-ready <epic-id-or-path>` and for the adjudicated adversarial review recorded
via `record-lenses --scope epic-ready` (the `epic-review-loop` FSM workflow points its lenses here).

Epic-ready reviews a **roll-up over already-reviewed children**, not one task's diff. The per-child task-done
gates already reviewed each child's own diff; epic-ready adds the **cross-child judgment no single-child review
could have**. It has two layers:

1. **Deterministic pre-checks (roll-up freshness).** Fast structural gates over the *current* child state,
   re-evaluated on every run: child acceptance evidence present, no unresolved child blockers, no cross-task
   contradiction, no epic-intent drift, durable service/codepath changes registered. These guard that the
   roll-up the lenses judge is still current.
2. **Adjudicated adversarial lenses (integration judgment).** The substantive review, over the epic's
   **integration diff** (`base..head` — the union of the children's changes) read *with the roll-up as
   context* (child verdicts, parent epic intent). Run these as independent subagents; the judge adjudicates.

## Lenses (run as adversarial subagents over the integration diff, with the roll-up as context)

- **epic-integration** — Do the children compose into a coherent whole? Look for integration seams,
  duplicated or contradictory abstractions, an interface one child exports that another consumes
  incompatibly, shared state two children mutate under different assumptions, and wiring that no single-child
  diff revealed because each child only saw its own slice.
- **acceptance-vs-intent** — Does the *delivered whole* satisfy the parent epic's stated acceptance criteria
  and intent — not merely each child's local acceptance? Flag an epic whose children each passed but whose sum
  does not meet the parent goal, and any drift from an explicit epic constraint (e.g. "without executing user
  input").
- **cross-child-regression** — Did a later child break an earlier child's behavior in a way no single-child
  task-done review could catch (each reviewed only its own diff)? Look for a change in a shared module, a
  contract, or a test that a sibling child depended on.
- **architecture-coherence** — Is the epic structurally consistent? Durable service/codepath additions
  registered in the service inventory/knowledge, layering respected across the children, no ad-hoc divergence
  between children solving the same problem two ways.

Name the lenses that actually ran in `record-lenses --lenses …` (they are recorded per epic, independent of the
pr-ready/task-done lens set). Add or adjust lenses here as the epic review sharpens — this rubric is the single
place the epic lens set is defined; changing it does not touch the task-done or pr-ready rubrics.

## Blocking Policy

Critical and high findings block epic readiness.

Block when:

- Child tasks contradict each other or the parent epic intent.
- Any child task lacks passing task-done or acceptance evidence.
- Any child task or child epic has unresolved blocking findings.
- Final integration drifts from the original epic intent, or the delivered whole fails the parent acceptance
  criteria even though each child passed.
- A later child regressed an earlier child's behavior.
- The children integrate incoherently (conflicting abstractions/interfaces/shared state).
- Durable service/codepath changes lack registry or knowledge coverage.
- No adjudicated adversarial review is recorded for this integration diff at this head (or the recorded review
  is not a pass, or the working tree is unattested).

## Pass Policy

Pass only when the current review has no blocking findings, prior blocking findings for the previous run are
resolved, and an adjudicated adversarial review is recorded for the current `base..head` integration diff.
Passing evidence should cite child review logs, validation output, changed files, and the parent epic intent.

In advisory mode (not a Beads/metaswarm repo), use `PASS_ADVISORY` when evidence is present; a recorded
`in-session-emulated` review also renders as advisory (weaker, non-independent) evidence.
