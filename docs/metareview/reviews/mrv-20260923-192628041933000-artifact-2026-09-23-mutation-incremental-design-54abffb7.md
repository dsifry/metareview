# metareview: artifact review

Run ID: `mrv-20260923-192628041933000-artifact-2026-09-23-mutation-incremental-design-54abffb7`

Target: `docs/superpowers/specs/2026-09-23-mutation-incremental-design.md`

Context pack: `docs/metareview/context/mrv-20260923-192628041933000-artifact-2026-09-23-mutation-incremental-design-54abffb7-context.md`

Execution mode: `parallel-subagents`

Previous run: `mrv-20260923-190310858720000-artifact-2026-09-23-mutation-incremental-design-54abffb7`

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
| Security | PASS | 0 | 0 | pragmatic brief; advisories held for consolidation |
| Scope and alignment | PASS | 0 | 0 | all requests/decisions map; advisories (fresh-worktree warm start, over-engineering) held |
| Intent preservation | PASS | 0 | 0 | all intents/decisions preserved; advisories (step summary for deferrals, §8 no-tests exception, 0.14.0 note) held |
| Architecture | NEEDS_REVISION | 1 | 0 | no-tests outcome reports kills verified (deleted only test; scope-no-tests skips forced helper run) |
| Feasibility | NEEDS_REVISION | 2 | 0 | package rule makes normal npm/pnpm deps unresolved (verified); no-tests ⇒ record nothing loses new files and leaves deleted-test kills verified (verified on real Stryker) |
| Mechanical-precision | NEEDS_REVISION | 3 | 0 | no-tests outcome leaves old kills verified and invocation-2 gating undefined; scope membership of whole-file forced files undefined; "cause for a kill" ambiguous for findings |
| Runtime-reliability | NEEDS_REVISION | 3 | 0 | no-tests outcome silently skips invocation 2 and never re-scopes new files; full-job threshold break never saves (converges with completeness); re-run of a threshold-failed job turns green |
| Completeness | NEEDS_REVISION | 2 | 0 | full-job threshold break skips the cache save ⇒ full run repeats every push; new module run before its first test is never scoped once the test lands |
| Testing-quality | NEEDS_REVISION | 4 | 0 | barrel row gets no-tests (nothing imports it); a+types row is 2 invocations; package-rule wording makes npm installs open importers; ranges ignore BlockStatement spans |
| Data-migration | NEEDS_REVISION | 1 | 0 | a gate run without `--mutation-report` supersedes granted overrides, which then re-block |

## Orchestrator Notes (not findings)

- Round r6: full 10-lens adversarial review with the user's pragmatic brief (real workflows and real edge cases, assume trust, no overengineering). 3 PASS (security, scope and alignment, intent preservation), 7 NEEDS_REVISION.
- Convergence (audit only): the r6 "No tests were executed ⇒ record nothing" rule (feasibility, architecture, runtime, mechanical, completeness); the §5.4 package-rule wording (feasibility, testing); full-job threshold break never saving (completeness, runtime); supersede on report-less runs (data-migration, architecture advisory).
- Orchestrator verified: runtime's proposed `--allowEmpty` fix does not write a report (`4-mutation-test-executor.js:49` returns before `reportAll`), consistent with feasibility's experiment.
- Advisory consolidation deferred to the next round.

Orchestrator context and synthesis go here (e.g. checkout sparse, filtered file-not-found artifacts, consolidation narrative). This section is audit trail only — it is NOT a finding stream. Do not extract sentences from here as review findings; only the `## Findings` section and its classified `## Blocking Findings`, `## Advisory Findings`, `## Follow-up Findings`, and `## Warnings` sections contain review findings.

## Findings

## Blocking Findings

### Architecture

- **[P1, 75] The `No tests were executed` outcome reports kills verified in two normal workflows**: (a) deleting the only test file of a module that stays (R keeps Killed mutants with dangling `killedBy`, no deferral, gate says verified); (b) a scope that reaches no tests alongside a helper edit skips the forced invocation with no deferral. Count the outcome as done for sequencing; defer selected files that still have Killed mutants in `R`; add e2e rows. Evidence: spec §5.6 step 5, §5.4 steps 2–3; `incremental-differ.js:463-482`.

### Feasibility

- **[P1, 75] The package rule treats every normal npm/pnpm dependency as unresolved**: `createRequire(...).resolve('vitest')` realpath is inside the top-level (so is pnpm's `.pnpm`), making every vitest-importing test an open importer and every helper edit a full-run deferral; contradicts §7.1. A package is a builtin or a specifier whose resolved realpath contains a `node_modules/` segment. Evidence: experiment in `scratchpad/lens6-feasibility/p`; spec §5.4.
- **[P1, 75] "No tests were executed ⇒ record nothing" loses results** (verified on StrykerJS 10.0.0): a new source file without a test never enters `R` and is never re-planned; when the only test reaching a file is deleted, `R` keeps `Killed` with stale `killedBy` and the gate reports them verified; `--allowEmpty` writes an identical report. Evidence: experiment; spec §5.6 step 5.

### Mechanical-precision

- **[P1, 75] The `No tests were executed` outcome leaves existing kills verified** (e.g. deleting `tests/c.test.ts` alone: `src/c.ts` has kills in `R` but no deferral is recorded) and leaves invocation-2 gating undefined. Record a deferral on selected files that have kills in `R`; state that invocation 2 still runs. Evidence: spec §5.6 step 5, §5.4 step 2.
- **[P2, 75] Whether a whole-file-forced file stays in scope is undefined**, changing plan JSON, argv and budget overflow behaviour for mutually covering edits. Evidence: spec §5.4 Normalise.
- **[P2, 50] "Every changed path that is a cause for at least one kill" is ambiguous** (any applicable cause vs recorded first cause), changing the finding set for a dependency bump plus a code edit. Evidence: spec §6.3, §6.5.

### Runtime-reliability

- **[P1, 85] The `No tests were executed` outcome silently drops work**: invocation 2 runs only after a *successful* invocation 1, so a scope that yields no tests skips the forced set with no deferral (kills reported verified); and a new file with no tests yet is attested but never re-scoped when its test lands. (The lens's proposed `--allowEmpty` fix does not write a report: `4-mutation-test-executor.js:49` returns before `reportAll` — verified by the orchestrator; the fix must be in the planner/runner rules instead.) Evidence: spec §5.6 steps 4–5, §5.4 step 2.
- **[P1, 80] A threshold break in the CI `full` job never saves its state**, so the full run repeats on every push (converges with completeness). Evidence: spec §5.7.
- **[P2, 75] Re-running a threshold-failed job turns green**: the re-run restores the saved state, has 0 invocations and exits 0. The committed report's threshold status must be recorded and carried forward. Evidence: spec §5.1, §5.6 step 6.

### Completeness

- **[P1, 75] A threshold break in the CI `full` job loops**: the failing step skips the copy/save, so the full state is never cached and every later main push starts another multi-hour full run. The full job needs `continue-on-error`, save on exit 0/1, then fail. Evidence: spec §5.7.
- **[P1, 75] A new module run before its first test is never scoped later**: `No tests were executed` records its snapshot digest while it stays absent from `R`; when its test arrives, step 2 scopes only files in `R`, so its mutants never appear until a full run — a silent blind spot. Step 2 (new test / test without entry) must also scope `mutate` files in `S` absent from `R.files`. Evidence: spec §5.4 step 2, §5.6 step 5.

### Testing-quality

- **[P2, 75] The `src/index.ts` row expects success but nothing imports the barrel**, so Vitest `related` finds no tests ⇒ `No tests were executed` ⇒ nothing recorded; the F9 success path goes untested. Evidence: spec §7.1, F4, §5.6 step 5.
- **[P2, 75] The `src/a.ts` + `src/types.ts` row expects 1 invocation; residual forcing of `src/b.ts` kills makes it 2.** Evidence: spec §5.4 step 4, §5.6 step 4, §7.1.
- **[P1, 75] The package rule as worded makes every npm-installed dependency (realpath inside the top-level) unresolved**, turning ordinary tests into open importers and contradicting the helper rows; the rule must exclude only node_modules entries whose realpath leaves every `node_modules` directory (workspace links). Evidence: spec §5.4 import graph; §7.1 helper rows.
- **[P2, 50] Forced ranges ignore multi-line BlockStatement mutants** (function body at lines 1–5), so `src/a.ts:2-4` / `src/c.ts:2-2` are wrong unless the fixture layout is pinned. Evidence: spec §5.4 ranges, §7.1.

### Data-migration

- **[P2, 70] A gate run without `--mutation-report` supersedes every granted `mutation:*` override on its target**; the next run with reports reproduces the identical fingerprint and opens a new blocking row, so an accepted exception re-blocks and needs a fresh request/grant. Apply the supersede rule only when the run supplied at least one report (or only to rows whose engine/report were inputs). Evidence: spec §6.5; `cmd/metareview/main.go:118-120` (reports optional).

