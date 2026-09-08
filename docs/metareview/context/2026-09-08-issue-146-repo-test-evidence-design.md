# Issue #146 design: repository-side test evidence for testing-gap claims

Issue: #140 follow-up — search repository test evidence (not just the diff) before
adjudicating testing-gap claims. Opened from the CodeRabbit threads on PR #145
(`internal/fsm/kind/kind.go:613`, `internal/fsm/judge/prompts.go:60`).

## Problem

PR #145's covering-test evidence (`judge.GapClaimEvidence` → `claimcheck.EvidenceFor`)
reads only **added diff lines**. A pre-existing covering test outside every changed hunk
produces no evidence, so the judge can confirm a false absence claim without ever seeing
the test that contradicts it. The prompt criterion has the same shape: it tells the judge
to check the evidence *included*, but nothing requires a subject-specific search record
before `is_real` may be true.

## Design (unchanged from the review sketch; four build phases + one measure)

### Phase 1 — claimcheck: full-file blocks (leaf, no new matching logic)

`claimcheck.Block{Path, Added}` already carries exactly what `EvidenceFor` matches on:
added lines. A repository file at HEAD becomes the same shape with `Added` = the file's
full content. No scoring/ranking change; the same strong/weak admission bar applies to
whole files. Only additions:

- `SubjectTokens(f Finding) (strong, weak []string)` — exported wrapper over the existing
  `subjectTokens`, so the git layer can build a candidate-selection pattern without the
  leaf importing anything. Sorted, lowercase.
- Doc note on `Block.Added`: may carry full-file content (repo search), not only diff
  additions; `EvidenceFor` is agnostic.

Tests-first: `EvidenceFor` over full-file blocks (admission bar, own-file skip, support
skip, cap) — behavior already correct, tests pin it before the repo layer leans on it.

### Phase 2 — judge: `RepoTestEvidence` behind injectable git seams

New `internal/fsm/judge/repoevidence.go`. The command-seam DI pattern (§9): the judge
package never spawns git; production wires the seams in `internal/fsm/cli` next to
escalation's `showFile`.

```go
// GrepPaths returns the repo paths at the pinned head whose content matches pattern.
type GrepPaths func(pattern string) ([]string, error)
// ShowHead returns one path's content at the pinned head.
type ShowHead func(path string) ([]byte, bool, error)

func RepoTestEvidence(grep GrepPaths, show ShowHead, f run.Finding, max int,
) ([]claimcheck.Evidence, error)
```

Two-stage search, both bounded:

1. **Candidates** — `git grep -l -i -E "<alternation>" <head>` (strong ∪ weak tokens),
   filtered with `claimcheck.IsTestPath` / `IsSupportPath`, own file skipped (same rule
   as `EvidenceFor`). Grep recalls; `EvidenceFor` scores and admits.
2. **Content + ranking** — `git show head:<path>` per surviving candidate, split to lines,
   run through `EvidenceFor` (blocks with full-file content). Cap `MaxGapEvidenceFiles`.

Fail-open contract: any seam error → `(nil, err)` at the seam boundary, and the caller
falls back to diff-only evidence with the failure noted in the disclosure. An
infrastructure failure must never read as "we searched and found nothing".

### Phase 3 — wiring: context builder + executor + escalation

- `ContextForGapClaim` gains repo blocks (optional; `nil` = today's behavior byte-for-byte
  when no repo search ran). Dedup by normalized path **before** the cap; diff evidence is
  supplemental, repo evidence primary. Disclosure names both sources.
- `kind.Deps` gains `RepoSearch func(ctx, snap, finding) ([]claimcheck.Evidence, error)`.
  `nil` disables (mock runs, judge-less registries). The executor appends repo evidence
  paths to `claim.Evidence` so the audit trail shows exactly what the judge saw.
- Escalation (`escalationFor`): repo-evidence paths join the sandbox materialization list,
  or the second opinion gets *worse* evidence than the primary.

### Phase 4 — prompt criterion (RubricAddendum) + disclosure

Templates stay byte-pinned to harnesseval. The metareview-owned addendum replaces the
"the evidence is the diff, not the repository" caveat with: a testing-gap claim is real
only when the reasoning records the search result — which test files were shown (diff
hunks and repository-head candidates) and why none covers the claimed behavior.

### Phase 5 — eval: structural pass in cmd/claimcheck-eval

`-repos <dir>` over the harnesseval lab's `.cache/mrv_repos` checkouts: run the repo-side
search over the 343-claim corpus, report the confusion matrix evidence-found ×
ground-truth-verdict, same shape as the diff-only numbers (14/21, 149). The A/B re-judge
(old vs new prompt) needs model spend and stays open in the issue.

## Cost, determinism, audit

- One `git grep` per distinct subject-token set (cache per claim in a run); `git show`
  capped per file; files capped at `MaxGapEvidenceFiles`.
- Everything derives from the pinned head SHA → `DiffContextHash` stays replayable.
- Repo evidence paths land in `claim.Evidence` → the audit trail records what was offered.
- Mock/judge-less runs skip the search entirely.
- Repo paths come from git, not model prose; still pass through `sandbox.Materialize`'s
  containment rules on the escalation path.

## Gates

TDD (tests fail first), `make cover` 100% on touched packages, gremlins
(`--workers 1 --timeout-coefficient 30`) on `internal/claimcheck`, `internal/fsm/judge`,
`internal/fsm/kind`, evidence receipts, task-done adjudicated review, record-lenses.

## Addendum (issue #147): the push gate honors the findings ledger

Repairing this branch exposed #147: the push gate blocks on review logs' frozen
HasUnresolvedBlockers/verdict, but its own remedy ("record an override reason") writes to
the findings ledger, which the push gate never reads — and non-linear --previous-run runs
leave sibling logs no linear chain can supersede. The reconciliation predicate already
exists (prready's reconcileReview, applied by gateReviewLogs inside the pr-ready review);
the push gate just never applies it.

Fix: the predicate moves to reviewstate (the designated gate/pr-ready shared layer, per
the StaleSameHeadRunIDs precedent), and status.buildFor applies it — a LogBlocks log whose
blocker-class findings are ALL resolved in the ledger (fixed / override-granted /
superseded, IsBlockingClass only, ≥1 resolver required) is historical and no longer
blocks, whatever its verdict. Fail-closed: an unreadable ledger clears nothing; an open
blocker-class finding, an unknown finding ID, or advisory-class rows keep the log
blocking. The frozen log file is never modified — the ledger is the live reconciliation
authority, exactly as the override system's own documentation promises.

### Revision (post-review): the lenses' three blockers on the first reconciliation

The nine-lens round on the gate change found three real flaws, all fixed stricter:

1. **Unknown finding IDs failed open.** A log referencing [resolved-id, unknown-id] cleared
   — the unknown blocker had no ledger vouching. Now LogResolvedInLedger requires the
   ledger to know EVERY finding ID the log references; an unknown ID is an unvouched
   blocker, never a resolved one. (prready's report-prose rendering keeps its lenient
   reading — it renders, it does not gate.)
2. **ESCALATED logs were clearable by ordinary ledger resolution.** LogBlocks documents an
   ESCALATED verdict as "a hard stop that a later clean re-run must not erase". Fixes and
   superseded rows never lift one now: the gate reconciles them only when EVERY
   blocker-class finding they reference carries an explicit OVERRIDE GRANT (grantor
   recorded) — the recorded human decision, via the existing two-phase override
   machinery. To make that reachable, GrantOverride accepts a terminal-status (fixed)
   finding: an escalation can persist after its findings are fixed, and lifting it is
   precisely the human decision the grant records.
3. **The reconciliation block was duplicated in buildFor and buildForBranch** (the drift
   hazard #147 itself documents). Extracted into one helper.
