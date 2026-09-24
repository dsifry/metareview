# metareview: artifact review

Run ID: `mrv-20260923-195021334650000-artifact-2026-09-23-mutation-incremental-design-54abffb7`

Target: `docs/superpowers/specs/2026-09-23-mutation-incremental-design.md`

Context pack: `docs/metareview/context/mrv-20260923-195021334650000-artifact-2026-09-23-mutation-incremental-design-54abffb7-context.md`

Execution mode: `parallel-subagents`

Previous run: `mrv-20260923-194341093013000-artifact-2026-09-23-mutation-incremental-design-54abffb7`

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
| Security | PASS | 0 | 0 | r8 blocker fixed (per-event jobs, no persisted credentials, token only on publish steps); advisories held |
| Scope and alignment | NEEDS_REVISION | 1 | 0 | relative threshold baseline becomes the PR's own previous state, so a threshold-breaking PR turns green on its next push |
| Intent preservation | NEEDS_REVISION | 1 | 0 | same relative-threshold baseline drift (converges with scope); null-baseline semantics advisory |
| Completeness | NEEDS_REVISION | 2 | 0 | relative-threshold baseline drift (converges); full job re-plans from remote/full and skips, losing execution-derived deferrals |
| Architecture | NEEDS_REVISION | 1 | 0 | relative baseline drift (converges); convergence traced sound; advisories: deferring PRs re-run whole delta, stale inc deferrals after skipped full job, pick main baseline at PR snapshot |
| Feasibility | NEEDS_REVISION | 2 | 0 | fetch-state has no credential on private repos (persist-credentials false, token only on publish); relative baseline drift (converges); advisories: Basic extraheader via GIT_CONFIG_*, null baseline, Vitest test-span nuance |
| Testing-quality | PASS | 0 | 0 | every §7.1 row re-derived; advisories: relative fail rows, null-baseline row, plan `baseline` field, strict row mutant count, survivor-variant start state |
| Data-migration | PASS | 0 | 0 | ledger/override/digest sound; advisories: scope supersede to supplied engines, mode in fingerprint drops overrides on mode change, `[superseded]` round-trip |
| Runtime-reliability | NEEDS_REVISION | 2 | 0 | relative baseline drift (converges); execution-derived deferrals never trigger a full run (converges with completeness B2); advisories: broken import counted as no-tests, full job should save cache before publish |
| Mechanical-precision | NEEDS_REVISION | 1 | 0 | relative threshold exit undefined with no baseline or null baseline score (harness upgrade window); advisories: revert row yields new fingerprint, two mutant-identity definitions, `( ) [ ]` paths (App Router) exit 2, seed/plan exit codes, `["*"]` wording in §4 |

## Orchestrator Notes (not findings)

- Round r9: full 10-lens adversarial review with the user's pragmatic brief. 3 PASS (security, testing-quality, data-migration), 7 NEEDS_REVISION.
- Convergence (audit only): the relative-threshold baseline drifts to the PR's own cached state (scope, intent, completeness, architecture, feasibility, runtime — 6 lenses); the full job's `plan` skip loses execution-derived deferrals (completeness, runtime); relative threshold undefined without a baseline (mechanical; testing and intent as advisories). Single-lens: `fetch-state` lacks a credential on private repos (feasibility).
- All r8 blockers verified fixed by the lenses that raised them.

Orchestrator context and synthesis go here (e.g. checkout sparse, filtered file-not-found artifacts, consolidation narrative). This section is audit trail only — it is NOT a finding stream. Do not extract sentences from here as review findings; only the `## Findings` section and its classified `## Blocking Findings`, `## Advisory Findings`, `## Follow-up Findings`, and `## Warnings` sections contain review findings.

## Findings

## Blocking Findings

### Intent preservation

- **[P1, 75] Converges with scope and alignment**: the PR's own cached, threshold-breaking state becomes the relative baseline, so a second push or re-run turns the job green. Evidence: spec §5.1, §5.4, §5.7.

### Completeness

- **[P1, 75] Converges with scope and alignment** (relative baseline drift). Evidence: spec §5.1, §5.7.
- **[P1, 75] The full job can skip a needed full run**: its plan adopts `remote/full` (no deferrals preferred) and a re-plan cannot reproduce execution-derived deferrals (`time budget exceeded`, `blocked by deferred scope`, `no reachable tests`), so `pendingFull=false` and the full run is skipped while main's pending persists. Skip only when `remote/full` has no deferrals and an empty change set against the current tree. Evidence: spec §5.7 full job, §5.4 Adopt.

### Scope and alignment

- **[P1, 75] The relative threshold baseline drifts to the PR's own state**: after a PR's first threshold-breaking run is cached (exit 1 saves), the next push or re-run adopts that state as baseline, so the score is neither below-break-relative nor lower, and the job turns green — contradicting decision 15 and "a re-run of a red job stays red". Compare against main's state instead and add a re-run row. Evidence: spec §5.1, §5.4 Adopt, §5.7 step 4.

### Architecture

- **[P1, 75] Converges with scope and alignment** (relative baseline drift); fix: take the relative baseline only from main candidates and carry it in the attestation. Evidence: spec §5.1, §5.4 Adopt, §5.7, decision 15.

### Feasibility

- **[P1, 80] `fetch-state` has no credential on private repositories**: every checkout sets `persist-credentials: false` and `MUTATION_STATE_TOKEN` reaches only `publish-state`, so `ls-remote` fails auth and every mutation job exits 2. Give the token (read-only on PRs) to `fetch-state` too. Evidence: spec §5.7, §7.2 static test.
- **[P1, 75] Converges with scope and alignment** (relative baseline drift). Evidence: spec §5.1, §5.4, §7.1.

### Runtime-reliability

- **[P1, 100] Converges with scope and alignment** (relative baseline drift, incl. "Re-run failed jobs"). Evidence: spec §5.1, §5.4, §5.7.
- **[P1, 75] Converges with completeness** (full job re-plans from `remote/full` and skips, so run-time deferrals keep main pending and replay grows). Evidence: spec §5.4, §5.6 step 5, §5.7.

### Mechanical-precision

- **[P2, 75] `--threshold relative` is undefined with no usable baseline or a null baseline score** — a normal harness upgrade makes all state unusable, so implementers diverge (all PRs red vs. all green). Evidence: spec §5.1, §5.4 Usable, §5.5.
