# metareview: artifact review

Run ID: `mrv-20260923-200340521908000-artifact-2026-09-23-mutation-incremental-design-54abffb7`

Target: `docs/superpowers/specs/2026-09-23-mutation-incremental-design.md`

Context pack: `docs/metareview/context/mrv-20260923-200340521908000-artifact-2026-09-23-mutation-incremental-design-54abffb7-context.md`

Execution mode: `parallel-subagents`

Previous run: `mrv-20260923-200028555622000-artifact-2026-09-23-mutation-incremental-design-54abffb7`

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

- Round r12: full 10-lens review with the pragmatic brief. 9 PASS, 1 NEEDS_REVISION (testing-quality). The r11 blocker is verified fixed. The published green catch-up was checked sound by architecture, intent, data-migration and runtime.
- r13 fixes the blocker. It also adopts one data-migration advisory with real impact: an `overridden` row is not superseded, so an escalation lifted by an override stays lifted. The remaining advisories are folded in as wording and docs.
| Scope and alignment | PASS | 0 | 0 | all §10 decisions and origin §1 honoured; advisories: seed marginal but keep, treat docs as its own plan task, note frequent pending on integration-heavy suites |
| Feasibility | PASS | 0 | 0 | every r12 change buildable on Actions/actions-cache; advisories: step 5 fallback wording (first hash-matching of inc, full), say lastFullAt carries through a published catch-up, doc the red catch-up case |
| Security | PASS | 0 | 0 | credential scoping, fork PRs, exposure and push loops all handled; advisories: state branches as trusted as writers, keep github.token over a PAT, private-fork fetch failure message |
| Architecture | PASS | 0 | 0 | published catch-up is always no-deferral-derived; single writer holds; planner/gate agree; advisories: §4 'cleared only by a full run' wording, §5.7 'full-run state' wording, doc `mode: incremental` on the full branch |
| Completeness | PASS | 0 | 0 | every workflow end to end, r11 fix holds; advisories: upgrading-the-harness docs, delta re-run during a long full run, non-main default branch, optional paths-ignore |
| Mechanical-precision | PASS | 0 | 0 | every r12 path pinned; advisories: summary shows restored stale state on exit 2/3/4/130, 'first hash-matching of inc, full', .nvmrc 'first file that exists', drop main's score on the main job |
| Intent preservation | PASS | 0 | 0 | all intents and 15 decisions hold; published green catch-up is as verified as a full run; advisories: §4/§5.7 wording, doc that full means no-deferrals and how to force a fresh full run, paths-ignore |
| Testing-quality | NEEDS_REVISION | 1 | 0 | no test pins which exit code the full job fails on (catch-up's when step 3 stopped, else the full run's): an unconditional read goes red after every green catch-up or green on a red full run; advisories: pending fingerprint changes after re-run, green catch-up row setup, summary-script Node tests |
| Data-migration | PASS | 0 | 0 | published catch-up consistent, upgrade costs one full run, superseded status exists; advisories: superseding an `overridden` row re-imposes a lifted escalation (supersede only open/override-pending), 0.14.0 fingerprint mode flip, restore order note |
| Runtime-reliability | PASS | 0 | 0 | every failure path traced, none silent or stuck; advisories: publish-state rejected-push exit code, label summary score on failed runs, full-job timeout never self-clears, killed-mid-run row wording |

Orchestrator context and synthesis go here (e.g. checkout sparse, filtered file-not-found artifacts, consolidation narrative). This section is audit trail only — it is NOT a finding stream. Do not extract sentences from here as review findings; only the `## Findings` section and its classified `## Blocking Findings`, `## Advisory Findings`, `## Follow-up Findings`, and `## Warnings` sections contain review findings.

## Findings

No reviewer findings recorded yet.

## Blocking Findings

### Testing-quality

- **[P2, 65] Nothing tests which exit code decides the `full` job**: step 5 fails on the catch-up's exit code when the job stopped at step 3, otherwise on the full run's. A template that always reads the full run's code goes red after every green catch-up. One that always reads the catch-up's code goes green on a red full run. The static test and the job emulation miss both. Add "the job is green" to the green-catch-up row and a static assertion on the fail step's source. Evidence: spec §5.7 `full` step 5, §7.1, §7.2.
