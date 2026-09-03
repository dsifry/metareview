# Spec: require an adjudicated lens review for task-done / pr-ready

Status: draft (build B). Owner: this branch `require-adjudicated-review`.

## Problem

`review pr-ready` and `review task-done` run in `ExecutionMode: deterministic-local`
(`internal/prready/review.go:251`, `internal/taskdone/review.go:228`). Their "reviewer" rows are **structural
checks** (`internal/reviewers/{prready,epicready}.go` — validation evidence present, no unresolved prior
blockers, clean tree, PR-evidence section). **No adversarial lens ever runs.** So a small diff PASSes with no
AI review — the mechanistic pass. Only artifact review requires the real thing (scaffold `NOT_REVIEWED` →
agent fills lens rows). The FSM's `review-lenses`→`match-then-adjudicate` engine *is* the real review, but it
lives behind a separate entry point (`metareview fsm`) and its findings land in the run store
(`.metareview/runs/<id>/audit.jsonl`), which the gate never reads. See docs/ARCHITECTURE.md §3.

## Goal

Make `PASS` require that an **adjudicated lens review ran over this diff at this head** — reusing the FSM's
existing `review-lenses`+`adjudicate` engine, not a third mechanism. Keep the deterministic structural checks
as additional gates. Provide a labeled `in-session-emulated` weaker-evidence escape hatch, exactly as artifact
review does. epic-ready stays structural for now (§4 of ARCHITECTURE: its judgment lenses are a fast-follow).

## Mechanism — the review-evidence marker (the bridge)

The gate cannot see FSM runs today. Bridge them with a small, durable **review-evidence marker** that the
deterministic gate already-reads path can check:

1. **Producing it:** when an FSM `review-loop`/`sdlc-loop-clean` run reaches its terminal adjudicated state,
   it writes a `reviewEvidence` record into `.metareview/runs.jsonl` (the log `reviewlog.Discover` /
   `runchain` already read) — NOT into the private audit log. Fields: `{kind: "review-evidence", scope:
   "pr-ready"|"task-done", headSha, baseSha, lensSet: [...], adjudicatedVerdict, confirmedFindingIds,
   executionMode: "subagent-adjudicated" | "in-session-emulated", fromFsmRunId}`. A thin recorder in
   `internal/reviewstate` (new) writes it; the FSM `done`/terminal handler calls it, and a CLI seam
   (`metareview review record-lenses …`, or folded into `fsm`) lets the agent record an
   `in-session-emulated` one when subagents are unavailable.

2. **Requiring it:** add a new reviewer to `reviewers.RunPRReady` (and the task-done equivalent):
   `adversarial-review-reviewer`. It BLOCKS ("no adjudicated lens review recorded for HEAD <sha>") unless a
   `review-evidence` marker exists whose `headSha` == the current review head and `scope` matches. Its
   confirmed findings are folded in as blocking/advisory so a review that *found* bugs also blocks until
   they're fixed+re-reviewed. `in-session-emulated` satisfies the requirement but is rendered in the review
   `.md` as weaker, non-independent evidence.

3. **Currency:** the marker is keyed on `headSha`; any new commit invalidates it (a fresh review is required),
   matching how the push gate already reasons about the branch head. Editing a file → new head → re-review.

## Injection points (verified in-tree)

- `internal/reviewers/prready.go` `RunPRReady` — add the `adversarial-review-reviewer` finding (block when no
  marker for HEAD; fold confirmed findings). Mirror in the task-done reviewer set.
- `internal/prready/review.go` (~L196, ~L242) and `internal/taskdone/review.go` — pass the current head +
  discovered review-evidence markers into the reviewer context; keep the structural checks.
- `internal/reviewstate` (new recorder) + `internal/reviewlog` (discover the marker) + `internal/fsm` terminal
  handler (emit the marker) + `cmd/metareview/main.go` (the record-lenses seam / fold into `fsm`).
- Review `.md` rendering: show the adversarial-review row + its lens set + execution mode, and the
  Completion-Requirements framing when the marker is absent (`NOT_REVIEWED`-style), mirroring
  `internal/artifactreview`.

## Escape hatch

`in-session-emulated`: the agent ran the lenses inline (no subagents) and recorded the marker with that mode.
The gate accepts it but the review `.md` labels it *not independently adversarial, weaker evidence* — the same
wording `internal/artifactreview/review.go:166` already uses. Never the silent default.

## Migration

Every existing `deterministic-local` PASS now reads as "needs an adjudicated review" — expected and correct.
The push gate (`PushGate` → requires a PASS pr-ready) therefore starts blocking a branch until its lenses have
actually run. Document in CLAUDE.md/AGENTS.md and ARCHITECTURE.md §3 (flip "intended direction" to "how it
works"). Feature-flag the enforcement (`--require-lenses` default on; `METAREVIEW_ALLOW_MECHANICAL_PASS=1`
opt-out) for one release so in-flight repos aren't wedged.

## Test plan (TDD)

- reviewstate recorder round-trips a marker; discovery finds it by head+scope.
- `RunPRReady`: blocks with no marker; passes with a matching-head marker; still blocks when the marker
  carries confirmed blocking findings; stale-head marker does not satisfy.
- `in-session-emulated` marker satisfies the gate and is rendered as weaker evidence.
- End-to-end behavioral: fresh diff → pr-ready blocks (NOT_REVIEWED-style) → record a review-loop marker →
  pr-ready PASSes → new commit → blocks again.
- Feature-flag off restores the old deterministic pass (for the migration release).
- Mutation-cover the new reviewer branch (block/allow/stale/confirmed-findings).

## Non-goals

- epic-ready judgment lenses (fast-follow).
- Rewiring the FSM's internal audit storage (only add the outward marker).
- An MCP server (separate track).

## Post-review hardening (dogfood, 2026-09-03)

Three independent adversarial reviewers (security, correctness, test-integrity) ran over the implementation
diff before merge. The correctness reviewer confirmed the gate **fails closed** (a corrupt `runs.jsonl`
blocks, never silently PASSes). Two real gate-logic gaps were found and fixed:

- **Forged independence.** A hand-typed `record-lenses --mode subagent-adjudicated --verdict PASS` satisfied
  the gate as full-strength independent evidence with no advisory trace. Fixed: `subagent-adjudicated` is
  admitted only with a `--from-run` naming an FSM run that exists on disk; a self-attested review has no such
  run and must record `in-session-emulated` (advisory). Verifying the referenced run's own verdict/head is a
  documented follow-up.
- **Base blindness.** Currency matched on HEAD only, so a review over a narrow `HEAD~1..HEAD` satisfied a
  gate run over a wider `main..HEAD`. Fixed: `LatestReviewEvidence` matches the exact `base..HEAD` pair.

Also: the latest-marker tie-break moved from lexical `CreatedAt` compare (which inverts on an exact-zero
nanosecond second) to **last-recorded-wins** (append order), the safer direction for a re-review downgrade.

Deferred as follow-ups: verifying `--from-run`'s recorded verdict/head; crediting a marker for a dirty
working tree under `--include-working-tree`; surfacing a distinct "store unreadable" diagnostic instead of
the generic "no review recorded" when `runs.jsonl` is corrupt.
