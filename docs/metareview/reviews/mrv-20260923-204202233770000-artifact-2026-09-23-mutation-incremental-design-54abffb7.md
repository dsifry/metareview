# metareview: artifact review

Run ID: `mrv-20260923-204202233770000-artifact-2026-09-23-mutation-incremental-design-54abffb7`

Target: `docs/superpowers/specs/2026-09-23-mutation-incremental-design.md`

Context pack: `docs/metareview/context/mrv-20260923-204202233770000-artifact-2026-09-23-mutation-incremental-design-54abffb7-context.md`

Execution mode: `parallel-subagents`

Previous run: `mrv-20260923-201040808900000-artifact-2026-09-23-mutation-incremental-design-54abffb7`

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

## Orchestrator Notes (not findings)

- Final-style review of the r15 amendment (§11), blocking only on major or critical findings. 2 PASS (security, scope), 1 PASS with advisories (testing), 7 NEEDS_REVISION with major findings in six distinct problems, all confined to §11:
  - a cold or `["*"]` deferral leaves a `full-on-global` PR red forever (completeness, mechanical, architecture advisory);
  - a PR's own deferrals count as inherited after it adopts its own cache (intent, runtime);
  - views are ignored by `openForRun`/supersede, and renamed views strand rows (architecture, data-migration);
  - `--mutation-view` is undefined for unattested or view-less reports (mechanical);
  - hunk-range forcing drops mutants spanning an edit, verified in StrykerJS 10 source (feasibility);
  - PR job sizing leaves no time for the triggered full run (runtime).
- r16 rewrites §11 to fix all six and folds in the advisories.
| Security | PASS | 0 | 0 | verify/views commands add no trust boundary (repo code, run step holds no token, forks read-only); advisories: run commands only from `run`, publish uses the recorded view map, child env, fork runner cost, verifier output public |
| Scope and alignment | PASS | 0 | 0 | §11 implements K1–K9 exactly, generic config only; advisories: completeness stricter than K3.3, residual off × editedFiles residual, noisy out-of-mutant-hunk cause, decision 19 wording, neither threshold nor verify |
| Testing-quality | PASS | 0 | 0 | residual closure, inherited pending_cause, completeness, per-view evidence all tested; advisories: say which editedFiles mode the r14 rows use, residual negative control with a narrower closure, name pure-deletion and validation cases |
| Completeness | NEEDS_REVISION | 1 | 0 | MAJOR: a cold PR run under full-on-global (stateVersion bump, first adoption, no state) records `no usable state`, counts as `other` and fails with no way to recover; count it as `global` |
| Intent preservation | NEEDS_REVISION | 1 | 0 | MAJOR: with no main state, a PR adopts its own pending cache and its own deferrals count as inherited, so the job goes green under full/full-on-global; record each deferral's origin (pr/main); advisories: verifier must read pending, pass resolved files per view, views command failure exit 2 |
| Data-migration | NEEDS_REVISION | 1 | 0 | MAJOR: a renamed or removed view leaves a blocking stale row nothing can supersede; supersede rows whose view is absent from the report's recorded map; advisories: restrict view names, seed merge compares source only, unattested + view, stateVersion discipline test |
| Architecture | NEEDS_REVISION | 1 | 0 | MAJOR: openForRun and supersede ignore views, so a view-scoped run is judged by other views' rows and a renamed view's rows can never clear; advisories: cold ⇒ global, shared line-diff contract, verify spawn failure after commit, job order under full |
| Mechanical-precision | NEEDS_REVISION | 2 | 0 | MAJOR: any `["*"]` deferral (no usable state, support deleted/no importer, unclassified) leaves a full-on-global PR red forever; MAJOR: --mutation-view undefined for unattested or view-less reports; advisories: list-rule matching, old-side wording, seed verify, per-view outputs |
| Feasibility | NEEDS_REVISION | 1 | 0 | MAJOR (verified in StrykerJS 10 source): forcing only hunk ranges drops mutants spanning the edit (instrumenter requires full containment; differ discards changed-span mutants), so they vanish while the state looks fresh; put Y whole-file in the unforced scope invocation and keep T_H forced; advisories: seed id re-keying, EOL normalisation, pure-insertion intersection, full turns time deferrals into sweeps |
| Runtime-reliability | NEEDS_REVISION | 2 | 0 | MAJOR: PR-caused deferrals become inherited on re-run when the PR adopts its own cache (converges with intent); MAJOR: PR job sizing leaves no time for the triggered full run; advisories: pending_cause only on exit 0/1, views command failure, verify hang/cold run, diff size cap, no reachable tests under full-on-global |

Orchestrator context and synthesis go here (e.g. checkout sparse, filtered file-not-found artifacts, consolidation narrative). This section is audit trail only — it is NOT a finding stream. Do not extract sentences from here as review findings; only the `## Findings` section and its classified `## Blocking Findings`, `## Advisory Findings`, `## Follow-up Findings`, and `## Warnings` sections contain review findings.

## Findings

No reviewer findings recorded yet.

## Blocking Findings

- **[MAJOR] A cold or `["*"]` deferral leaves a `full-on-global` PR red with no way out** (completeness, mechanical-precision): `no usable state` (stateVersion bump, first adoption), deleted or importer-less support files count as `other`. Fix: counted `["*"]` deferrals mean `global`. Evidence: §11.2, §5.4.
- **[MAJOR] A PR's own deferrals become "inherited" when it adopts its own cache** (intent-preservation, runtime-reliability): with no usable main state, adoption picks the PR's cache, and the job goes green on re-run. Fix: inherited only if the same deferral is in main's fetched state. Evidence: §11.2, §5.4 Adopt, §5.7 step 4.
- **[MAJOR] Views are ignored by the ledger** (architecture, data-migration): `openForRun` counts other views' rows, and a renamed view's rows can never be superseded. Fix: view-aware counting and supersede, and supersede rows whose view left the map. Evidence: §11.3, §6.5, `internal/findings/findings.go:173-187, 826`.
- **[MAJOR] `--mutation-view` is undefined for unattested or view-less reports** (mechanical-precision). Evidence: §11.3, §11.6.
- **[MAJOR] Hunk-range forcing drops mutants spanning an edit** (feasibility; StrykerJS 10 `babel-transformer.js:145-148`, `incremental-differ.js`): the block around a changed line leaves the report while the state looks fresh. Fix: edited file whole-file in the unforced scope invocation. Evidence: §11.4.
- **[MAJOR] PR job sizing leaves no time for the triggered full run** (runtime-reliability). Fix: timeout ≥ setup + 2×m + full sweep, plus a shortcut straight to the full run when the plan already needs it. Evidence: §11.2.
