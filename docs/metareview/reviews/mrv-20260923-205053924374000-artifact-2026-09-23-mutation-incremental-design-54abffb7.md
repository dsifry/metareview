# metareview: artifact review

Run ID: `mrv-20260923-205053924374000-artifact-2026-09-23-mutation-incremental-design-54abffb7`

Target: `docs/superpowers/specs/2026-09-23-mutation-incremental-design.md`

Context pack: `docs/metareview/context/mrv-20260923-205053924374000-artifact-2026-09-23-mutation-incremental-design-54abffb7-context.md`

Execution mode: `parallel-subagents`

Previous run: `mrv-20260923-204712580916000-artifact-2026-09-23-mutation-incremental-design-54abffb7`

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

- r17 review (10 lenses, blocking only on major or critical findings): 9 PASS, 1 NEEDS_REVISION (intent preservation). All three r16 majors are verified fixed by the lenses that raised them (completeness, runtime, mechanical all PASS).
- The intent finding is partly overstated. Its scenario assumes a PR that adopts main's re-attested state "runs nothing". Runtime reliability's trace shows the PR's own scope and edits still execute, because inherited deferrals never suppress execution (now stated in §11.2). The real residue is narrow and was flagged independently by runtime reliability: a PR run that is itself cold (zero invocations) can inherit `no usable state` from main's parsed-but-unusable attestation and pass. r18 closes it (a cold run's `no usable state` is always counted) without making every PR during main's pending window pay a full sweep. r18 is verified by intent preservation and runtime reliability.
| Feasibility | PASS | 0 | 0 | r17 changes buildable (reasons name at most one path/key; flag known at plan time; verifier exit 1 still emits pending_cause); advisories: stale PR base pays a sweep, structured `subject` field for reasons, static test for exit capture, verifier cost |
| Intent preservation | NEEDS_REVISION | 1 | 0 | MAJOR: an inherited `["*"]` deferral (e.g. `no usable state` on main after a stateVersion bump) passes (ii) trivially and hides the PR's own edited files, so the PR goes green under full-on-global; never inherit `["*"]` deferrals when pendingOnPr ≠ allow |
| Architecture | PASS | 0 | 0 | planner/gate agreement holds, view-aware ledger converges; advisories: renamed-view sweep leaves unviewed rows alone, spell out the PR `-full` cache key, state full-mode-never-defers dependency, list same-file residual in §8 |
| Testing-quality | PASS | 0 | 0 | core behaviours tested; advisories: inherited vector with unchanged digest and nameless reason, verifier exit 1 on counted pending, sweep boundary tests, inherited:false default, viewSummaries expected values |
| Completeness | PASS | 0 | 0 | all four r17 fixes hold, every failure names its remedy; advisories: pass `--pr` only when pendingOnPr ≠ allow, hot-file time deferral on a PR, post-bump inherited no-usable-state (converges with intent), don't sum per-view pending, follow-up full run kind |
| Mechanical-precision | PASS | 0 | 0 | no unsafe divergence; advisories: absent path on both sides = same digest, cached-state wording, counted takes precedence per kill, `--pr` only under ≠ allow, unviewed rows not swept |
| Security | PASS | 0 | 0 | no new privilege boundary; inheritance cannot be faked from PR state; advisories: verifier treats state files read-only, no secrets in run env, PR cache ref-scoped |
| Scope and alignment | PASS | 0 | 0 | K1–K9 and contract §4 covered, generic; advisories: record the support-no-importer → global change in the contract, inline views, seed merge breadth, small additions, stricter completeness |
| Runtime-reliability | PASS | 0 | 0 | lockfile window, stateVersion bump, failure paths and exit codes traced, no silent pass; advisories: a cold PR run can inherit `no usable state` and run nothing (narrow), spurious full-on-global failures during main pending, absent-path digest, canonical reasons |
| Data-migration | PASS | 0 | 0 | attestation additions additive, inherited flag can only tighten, sweep always recoverable, seed kills stay pending; advisories: viewless rows outside the sweep, structured view field, seed merge key, seeded reason, §5.5 wording |

Orchestrator context and synthesis go here (e.g. checkout sparse, filtered file-not-found artifacts, consolidation narrative). This section is audit trail only — it is NOT a finding stream. Do not extract sentences from here as review findings; only the `## Findings` section and its classified `## Blocking Findings`, `## Advisory Findings`, `## Follow-up Findings`, and `## Warnings` sections contain review findings.

## Findings

No reviewer findings recorded yet.

## Blocking Findings

### Intent preservation

- **[MAJOR] An inherited `["*"]` deferral could let a PR pass without its changes verified**: narrowed on analysis to a PR run that is itself cold (zero invocations) inheriting `no usable state`. Fixed in r18 (§11.2: a cold run's `no usable state` is always counted, and inherited deferrals never suppress execution). Evidence: §11.2, §5.6 step 7.
