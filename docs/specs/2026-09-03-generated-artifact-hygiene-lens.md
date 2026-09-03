# Spec: generated-artifact hygiene — head-partitioned FINDINGS.md + advisory lens (R5)

Status: **approved** (maintainer sign-off 2026-09-03: head-scoped render partition; **advisory** lens, not
blocking — see "Design evolution"). Owner: this branch `generated-artifact-hygiene-lens`. From the self-review
model matrix Round 4, finding **R5** (`docs/metareview/self-review-model-matrix.md:348`): CodeRabbit caught PR
#90's review-artifacts commit rendering **6 stale cross-head findings into the committed `FINDINGS.md` while
the run reported 0 blockers** — a self-contradictory committed artifact. The manual fix was
`: > .metareview/findings.jsonl`. This spec makes metareview prevent and surface it.

## Problem

metareview reviews the **code** diff but never its own generated/committed artifacts for internal consistency.
The concrete failure: `RenderIndexWithRecords` renders **every** open blocking record in the accumulated
`.metareview/findings.jsonl` ledger, while a run's *verdict* is scoped — its own target, and its projection
routes **unrelated** blockers (a different-branch target, or an unchanged-path target — `reviewstate`
`unrelatedBranchBlocker`/`unrelatedPathBlocker`) to *historical*, not *current*. So when the ledger has
accumulated open blockers from a prior HEAD that the current run's projection treats as unrelated, the run
passes (0 current blockers) yet the repo-wide `FINDINGS.md` lists them — a committed artifact that contradicts
its own gate result. Root cause: **the ledger accumulates open findings across heads, and the render never
distinguishes a finding recorded against the current HEAD from one recorded against a stale one.**

## Why stale-head findings are cruft, not current evidence

A finding is a claim about a **specific diff at a specific head**. When the head moves, the code changed, so a
finding recorded against the old head is no longer a valid claim about the current code — exactly as the
build-B review-evidence marker is invalidated by a new head. So a stale-head finding should not render as a
*current* unresolved blocker; it should be visibly set aside for re-review or cleanup.

## Goal

1. **Stop the self-contradiction** (the fix): partition `FINDINGS.md` by HEAD — open blockers recorded against
   the current HEAD are the unresolved list; stale-HEAD blockers render under a labeled **"Stale"** section and
   are not counted in the unresolved list. So an accumulated cross-head/cross-scope ledger can no longer render
   a self-contradictory index.
2. **Surface the cruft** (the signal): a non-blocking `generated-artifact-hygiene-reviewer` **advisory** in the
   pr-ready/task-done review output when the ledger carries orphaned stale-head findings, naming the remediation.

## Design choices (resolved)

### 1. The signal → stale `GitHead`, head-scoped (any target)

An open blocking record whose `GitHead != currentHead` is stale. It is **head-scoped, not target-scoped**: the
#90 self-contradiction is inherently **cross-target** (the verdict is target-scoped and routes unrelated
blockers to historical, so a pass run reports 0 while the repo-wide index shows other-target/other-scope stale
findings). A target-scoped signal would exclude exactly those and miss #90. A record with an empty `GitHead`,
or an empty `currentHead` (HEAD unresolvable), is never stale (**fail-visible**: never move a blocker out of
the current list on missing data).

### 2. The fix → partition at render (not auto-delete/supersede in reconcile)

`RenderIndexWithRecords(root, records, currentHead)` lists current-HEAD blockers under the existing "unresolved"
heading and stale-HEAD blockers under a labeled **"## Stale (recorded against a prior HEAD — re-review or
clear)"** section. Non-destructive (the ledger and all run-chain semantics are untouched; auto-superseding in
the delicate `Reconcile` state machine was rejected). `Reconcile` passes `run.GitHead` and re-renders after
**every** review, so the committed index is always partitioned. `Reconcile` renders POST-reconcile, so a
finding this run just fixed/head-bumped is already current — the render needs no chain logic. The standalone
`RenderIndex(root)` and the override re-render resolve HEAD via `realGitHead` (an unresolvable HEAD → `""` →
everything in the main list, fail-visible).

### 3. The lens → ADVISORY, not blocking (the key pivot)

**A blocking lens was rejected during the build** after adversarial review proved it doesn't work (see Design
evolution). The `generated-artifact-hygiene-reviewer` is **advisory** — it never changes the verdict. It fires
when the ledger carries **orphaned** stale-head findings: recorded against a prior HEAD AND not in the current
run's **reconcile set** (its previous-run chain plus escalation-reset runs — `chain.ResetRunIDs`), so the
normal fix-flow (`--previous-run`) and escalation recovery are not noised. It is head-scoped (any target),
mirroring the render, and appended to `RunPRReady`/`RunTaskDone`. The advisory surfaces the cruft in the review
output (not just buried in `FINDINGS.md`) with the remediation: re-review at the current head (supersede via
`--previous-run`), or clear the cross-head ledger.

### 4. Non-goal: byte-comparing the committed FINDINGS.md to a fresh render

The stale-`GitHead` signal is the precise root-cause detector; a byte-compare of `HEAD:FINDINGS.md` vs a fresh
render is noisy (any pending legitimate difference trips it). Left as a possible follow-up.

## Design evolution (why the lens is advisory, not blocking)

The maintainer first approved "render-partition + blocking lens." The build's own adversarial review then
proved a blocking lens cannot work:

- **Redundant for same-target.** The verdict (`openForRun`) already counts every same-target open blocker
  regardless of head, so a same-target stale blocker already blocks — a target-scoped blocking lens changes no
  verdict. (Verified empirically.)
- **Misses #90 when target-scoped.** #90 is cross-target (above), so a target-scoped signal excludes it.
- **False-positive when head-scoped-and-blocking.** A head-scoped blocking lens would block on a legitimate
  other-branch open blocker sharing the ledger (you'd have to clear it to review your branch).

The maintainer redirected to a **head-scoped render partition + advisory lens**: the render fixes the
committed-artifact contradiction, and the advisory gives the "clean the ledger" signal without the friction or
redundancy. (The correctness review also caught that the earlier chain-exclusion omitted `ResetRunIDs`,
breaking escalation recovery — folded into choice 3's reconcile-set exclusion.)

## Injection points (as built)

- `internal/findings/findings.go` — `RenderIndexWithRecords(root, records, currentHead)` partitions via
  `currentHeadBlockers`/`staleHeadBlockers` (unexported, head-scoped); `Reconcile` passes `run.GitHead`;
  `RenderIndex(root)` and override resolve HEAD via `realGitHead` (`""` on error → fail-visible).
  `StaleHeadBlockersInLedger(root, currentHead, reconcileRunIDs)` reads the ledger and returns orphaned
  stale-head findings (head-scoped, excluding the reconcile-set runs) for the advisory.
- `internal/reviewers/{prready,taskdone}.go` — `HygieneContext{StaleHeadBlockers []findings.Record}` on the
  context, and an **advisory** `generatedArtifactHygieneFindings(...)` appended to `RunPRReady`/`RunTaskDone`
  (new `internal/reviewers/hygiene.go`).
- `internal/prready/review.go` + `internal/taskdone/review.go` — fill the hygiene context from
  `findings.StaleHeadBlockersInLedger(root, git.HeadSHA, previousRunIDs ∪ chain.ResetRunIDs)`. task-done hoists
  its `runchain.Resolve` above the reviewer (resolved once, reused by Reconcile) to have the reconcile set.
- Docs: ARCHITECTURE §6 (FINDINGS.md is head-partitioned + advisory); both rubrics (the advisory, not a block).

## Test plan (TDD)

- **findings unit**: the render partitions current-HEAD vs stale-HEAD (any target) with correct ordering; no
  stale → no Stale section; empty currentHead / empty record head → all in main (fail-visible);
  `StaleHeadBlockersInLedger` excludes the reconcile set (chain + reset) and an empty id excludes nothing;
  `RenderIndex` resolves the real HEAD and `realGitHead` is `""` outside a repo.
- **reviewer unit**: `generatedArtifactHygieneFindings` is **advisory** (Classification `advisory`), fires on
  ≥1 stale-head record, passes on none, names the remediation; singular/plural wording.
- **behavioral end-to-end** (shell, the #90 shape): a repo whose ledger carries an open blocker for a
  DIFFERENT branch at a stale HEAD; `review pr-ready` **PASSES** (the projection routes it to historical), the
  review carries the advisory hygiene note, and `FINDINGS.md` shows the blocker under "## Stale", not the
  unresolved list. Negative: a clean ledger → no advisory, no Stale section.
- **Coverage**: `internal/findings` (floor 92.7), `internal/reviewers` (floor 97.7), `internal/prready` /
  `internal/taskdone` stay ≥ floor. Mutation-cover the partition + advisory + the reconcile-set exclusion.

## Non-goals

- A **blocking** hygiene lens (Design evolution rejects it — advisory only).
- Auto-deleting/superseding stale findings in `Reconcile` (render partition is the fix).
- Byte-comparing the committed FINDINGS.md to a fresh render (choice 4; follow-up).
- Changing the ledger schema (`GitHead` is an existing field).
