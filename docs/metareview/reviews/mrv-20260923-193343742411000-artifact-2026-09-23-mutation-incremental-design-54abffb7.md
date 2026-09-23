# metareview: artifact review

Run ID: `mrv-20260923-193343742411000-artifact-2026-09-23-mutation-incremental-design-54abffb7`

Target: `docs/superpowers/specs/2026-09-23-mutation-incremental-design.md`

Context pack: `docs/metareview/context/mrv-20260923-193343742411000-artifact-2026-09-23-mutation-incremental-design-54abffb7-context.md`

Execution mode: `parallel-subagents`

Previous run: `mrv-20260923-192628041933000-artifact-2026-09-23-mutation-incremental-design-54abffb7`

Required lenses: `feasibility, completeness, scope-alignment, architecture, intent-preservation, security, testing-quality, data-migration, runtime-reliability, mechanical-precision`

## Verdict

NEEDS_REVISION

## Completion Requirements

This scaffold is not a completed review. Artifact review defaults to parallel subagents for the required lenses. The artifact-review workflow is explicit authorization to delegate those lenses. Only use `in-session-emulated` when subagents are unavailable or the human explicitly requested no delegation; if used, state that the review is not independently adversarial and treat it as weaker evidence. Completion requires every required reviewer row to be populated, each reviewer to have a verdict, blocking findings to be fixed and re-reviewed or explicitly human-accepted, and the aggregate verdict to be the actual artifact-review verdict returned by the reviewer set rather than a fixed example result.

## Reviewer Prompts

Use `rubrics/artifact-review-rubric.md` and the context pack above. Run these lenses as parallel subagents by default before aggregation:

- Feasibility
- Completeness
- Scope and alignment
- Architecture
- Intent preservation
- Security (see `rubrics/security-review-rubric.md`)
- Testing-quality (see `rubrics/testing-quality-rubric.md`)
- Data-migration (see `rubrics/data-migration-rubric.md`)
- Runtime-reliability
- Mechanical-precision (see `rubrics/mechanical-precision-rubric.md`)

## Reviewer Results

| Reviewer | Verdict | Blocking | Warnings | Notes |
| --- | --- | ---: | ---: | --- |
| Security | PASS | 0 | 0 | pragmatic brief; advisories held (gitignore stateDir, env digests note) |
| Intent preservation | PASS | 0 | 0 | all intents and decisions 1–11 preserved; advisories held (tie pending-advisory to decision 2, measure pending share in trial, "planned" wording) |
| Scope and alignment | PASS | 0 | 0 | no drift; advisories held (seed --from-state vs --also-state overlap, lock optional, artifact fallback on cache miss, dependency bumps cost a full run) |
| Testing-quality | PASS | 0 | 0 | every §7.1 row derives from §5.4/§5.6; advisories held (delete-b row depends on Stryker bail — set `disableBail: true` in fixture; name assertion points for cold/threshold/new-test rows) |
| Feasibility | PASS | 0 | 0 | r7 rows reproduced on real StrykerJS 10 + Vitest 4.1.11; advisories held (compute thresholdBreak from report + config, killedBy order-dependence, yarn PnP wording, vitest alias in fixture) |
| Runtime-reliability | NEEDS_REVISION | 1 | 0 | fixing a threshold break by editing the Stryker config leaves `thresholdBreak` carried forward, so the PR stays red until merge |
| Architecture | NEEDS_REVISION | 2 | 0 | budget overflow returns a file to scope without its residual deferral (new in r7); changed file's own unchanged-region kills reused and reported verified (converges with completeness) |
| Mechanical-precision | NEEDS_REVISION | 2 | 0 | whether a test that is itself an open importer counts; re-run list row key when one file has several recorded causes |
| Completeness | NEEDS_REVISION | 2 | 0 | same-file kills in an edited file are reused (Stryker F1) and reported verified; a new/uncovered module whose first test lands in an existing test file is never scoped |
| Data-migration | PASS | 0 | 0 | ledger/digest/state paths sound; advisories held (reset path must also skip the prefixes; separate gate-contract version from planner-rules version; doc note on clearing enforce rows) |

## Orchestrator Notes (not findings)

- Round r7: full 10-lens adversarial review with the user's pragmatic brief. 6 PASS (security, intent preservation, scope and alignment, testing-quality, data-migration, feasibility), 4 NEEDS_REVISION (completeness, architecture, mechanical-precision, runtime-reliability) on narrow real-workflow blockers.
- Convergence (audit only): same-file kill reuse in edited files (completeness, architecture).
- User decisions after this round: force the whole edited file; state stored on change-based git branches with actions/cache as a fast path within GitHub's 7-day window (user asked why any caches are timed — only GitHub's eviction and artifact retention are; freshness is purely content-based).
- Advisory consolidation deferred to the next round.

Orchestrator context and synthesis go here (e.g. checkout sparse, filtered file-not-found artifacts, consolidation narrative). This section is audit trail only — it is NOT a finding stream. Do not extract sentences from here as review findings; only the `## Findings` section and its classified `## Blocking Findings`, `## Advisory Findings`, `## Follow-up Findings`, and `## Warnings` sections contain review findings.

## Findings

## Blocking Findings

### Runtime-reliability

- **[P2, 75] Lowering the threshold (or deleting the survivor's file) cannot clear a carried `thresholdBreak` on a PR**: the config edit is `global` ⇒ `["*"]` deferral and no invocation, the flag carries forward, and full runs happen only on main, so the PR stays red until merge. Carry the flag only when no global/runtime input changed; otherwise write `false` (the pending full run recomputes it). Evidence: spec §5.1, §5.4 step 5, §5.5.

### Architecture

- **[P1, 75] Budget overflow returns a file to scope without its residual deferral** (new in r7): a file both scoped (step 2) and residual-forced goes back to un-forced scope, Stryker reuses its kills whose killing tests are unchanged (`incremental-differ.js`, `mutantCanBeReused`), and no deferral covers it, so the gate reports them verified. Record the budget deferral on every file that had forced mutants. Evidence: spec §5.4 Normalise.
- **[P1, 75] A changed file's own unchanged-region kills are reused without re-running** (`collectReusableMutantsByKey` → `performFileDiff` drops only changed-region mutants); the residual rule excludes the changed file itself, and the gate reports them verified. Converges with completeness. Evidence: spec §5.4 steps 1 and 4; engine source.

### Mechanical-precision

- **[P2, 75] It is unstated whether a test file that is itself an open importer counts as "reaching an open importer"**, which changes plan output, deferrals and `pending_full` for tests importing unaliased workspace packages. Evidence: spec §5.4 import graph.
- **[P2, 75] The re-run list says one row per file, but one file can have several recorded causes**; specify one row per (file, recorded cause). Evidence: spec §6.3, §6.6.

### Completeness

- **[P1, 70] Kills in unchanged regions of an edited file are reused, not re-verified, and reported verified**: scope runs without `--force`, Stryker reuses a Killed result whose killing test is unchanged (F1), and the residual step only forces kills "in other files"; a behaviour-changing edit on one line can flip a kill elsewhere in the same file. Evidence: spec §5.4 steps 1 and 4, §5.6 step 4, §6.3, F1.
- **[P1, 75] A new or wholly-uncovered module whose first test lands in an existing test file is never scoped**: step 2 adds modules absent from `R.files` only for new test files or files without an entry; edits that import the module into an existing test file, or wire it into tested code, leave it unmutated until a full run, with no finding or deferral. Evidence: spec §5.4 step 2, §5.6 step 5, §8.

