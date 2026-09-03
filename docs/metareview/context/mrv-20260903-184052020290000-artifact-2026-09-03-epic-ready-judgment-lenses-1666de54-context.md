# metareview context: docs/specs/2026-09-03-epic-ready-judgment-lenses.md

Run ID: `mrv-20260903-184052020290000-artifact-2026-09-03-epic-ready-judgment-lenses-1666de54`

## Target

- Path: `docs/specs/2026-09-03-epic-ready-judgment-lenses.md`
- Repository mode: `metaswarm-extension`
- Git branch: `main`
- Git head: `467cbb5`

## Artifact Excerpt

```markdown
# Spec: epic-ready judgment lenses (require an adjudicated review, like build B)

Status: draft. Owner: this branch `epic-ready-judgment-lenses`. Fast-follow to
`docs/specs/2026-09-03-require-adjudicated-review.md` (build B), which already listed epic-ready lenses as a
non-goal / fast-follow.

## Problem

`review epic-ready` is the **last mechanical pass in the review family.** Build B made `pr-ready`/`task-done`
require an adjudicated adversarial lens review to PASS; `epic-ready` was explicitly left out. It runs in
`ExecutionMode: deterministic-local` (`internal/epicready/review.go:192`) and its "reviewers"
(`architecture-reviewer`, `epic-integration-reviewer`, `acceptance-reviewer`, `intent-preservation-reviewer`)
are **heuristics** in `internal/reviewers/epicready.go`: `hasEvalContradiction` (literal "use eval" vs "no
eval" string match), `violatesNoEvalIntent`, `missingChildEvidence`, `unresolvedChildBlockers`,
`missingServiceInventoryCoverage`, plus the context-risk short-circuit. These catch a narrow, hard-coded set
(the eval contradiction is a demo pattern, not a general contradiction detector) and never invoke a model. So
an epic closes — declared integration-ready — with **no adversarial judgment** over whether the children
actually cohere, whether the delivered whole preserves the parent intent, or whether a cross-child regression
slipped between the per-child reviews. ARCHITECTURE §4 already names the intended shape: *"A one-shot
`review-loop` over the roll-up is the candidate."*

## Goal

Make an epic-ready `PASS` require that an **adjudicated adversarial review ran over this epic at this head** —
reusing build B's review-evidence marker and gate reviewer, not a third mechanism — while keeping the cheap
deterministic heuristics as fast structural pre-checks (exactly as `pr-ready` kept its structural checks
under the new adversarial requirement). Provide the same labeled `in-session-emulated` weaker-evidence escape
hatch, and the same `METAREVIEW_ALLOW_MECHANICAL_PASS=1` opt-out for the migration release.

This is a **one-marker-mechanism-across-all-three-scopes** change: after it, `task-done`, `pr-ready`, and
`epic-ready` all block until an adjudicated review is recorded for their exact diff.

## The four design choices (resolved)

### 1. What surface do the epic lenses review? → the epic context pack, with the integration diff as context

epic-ready has no single "the diff." Its base..head span (what `epicready.Create` already collects via
`gitcontext.CollectWithExcludesExcept(root, options.Base, …)`) is the **integration diff** — the union of the
children's changes, `merge-base(main)..HEAD` on the epic branch. Three candidates: (a) review that integration
diff as one code diff; (b) review the rendered **epic context pack** (child review logs + child evidence +
parent intent + the roll-up) for integration/acceptance/intent gaps the per-child reviews couldn't see; (c)
both.

**Resolved: (b) is the lens surface, with (a) available to the lenses as context.** epic-ready's value is
*cross-child judgment*, not re-reviewing code the child task-done gates already covered line-by-line. The
lenses read the epic context pack `epicready.Create` already renders (`contextMarkdown` — Epic intent,
Children, Child Review Logs, Knowledge/Registries, Evidence, and the integration Diff), and are prompted to
find integration seams, acceptance-vs-intent drift, and cross-child regressions. The integration diff is
present in that pack as supporting evidence, so a lens *can* drop into the code where a cross-child concern
demands it — it simply isn't asked to re-audit every child line. This is why we do not just point the FSM
`review-loop` at the raw diff and call it epic-ready: the roll-up context is the substance.

### 2. Reuse build B's require-lenses gate? → yes, verbatim, keyed on the epic integration span

Wire `RunEpicReady` exactly as `RunPRReady`/`RunTaskDone` are wired:

- Add `RequireLenses bool` and `Adver
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
