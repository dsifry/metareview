# metareview: artifact review

Run ID: `mrv-20260923-200028555622000-artifact-2026-09-23-mutation-incremental-design-54abffb7`

Target: `docs/superpowers/specs/2026-09-23-mutation-incremental-design.md`

Context pack: `docs/metareview/context/mrv-20260923-200028555622000-artifact-2026-09-23-mutation-incremental-design-54abffb7-context.md`

Execution mode: `parallel-subagents`

Previous run: `mrv-20260923-195626283409000-artifact-2026-09-23-mutation-incremental-design-54abffb7`

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

- Round r11: full 10-lens review with the pragmatic brief. 9 PASS, 1 NEEDS_REVISION (testing-quality). The r10 gate blocker and the switch to the absolute threshold were verified by every lens.
- r12 fixes the single blocker, folds in the advisories and has the `full` job publish a green catch-up (architecture advisory). It is re-reviewed by all ten lenses so every required row is fresh.
| Scope and alignment | PASS | 0 | 0 | decision 15 applied consistently, no relative residue; all §10 decisions honoured; advisories: optional trims (Node warning, seed), overlapping threshold rows |
| Completeness | PASS | 0 | 0 | all workflows traced; advisories: step 5 cold-run wording, missing `--also-state` dir is simply unusable, full job green on catch-up threshold break, PR CI state is not gate evidence |
| Security | PASS | 0 | 0 | credential wiring holds for same-repo, fork, Dependabot, private, local; advisories: step summary/logs are public too, token is CI-only, restrict ruleset bypass to Actions |
| Feasibility | PASS | 0 | 0 | URL-scoped extraheader, add-mask, catch-up cache all workable; advisories: step 5 wording, green full job on threshold break, unit-test the header key, state non-numeric Node values skipped |
| Data-migration | PASS | 0 | 0 | nothing released needs migrating; zero-mutant cause self-clears; advisories: full job's cache key collides with incremental-main's in the same run (add `-full`), show main's score only when its report matches its hash |
| Intent preservation | PASS | 0 | 0 | all intents and §10 decisions intact; advisories: tuning-only config edits trigger a full run (document or drop `thresholds` from the digest), PR summary score may include pending kills, step 5 wording |
| Mechanical-precision | PASS | 0 | 0 | every r11 item pinned; advisories: catch-up exit 1 green vs red, trim `.nvmrc` and precedence, step 5 wording, seed exit on a break |
| Testing-quality | NEEDS_REVISION | 1 | 0 | step 5 labels a cold first run exit 2 (it exits 0) and no row runs the job emulation cold, so a summary step that fails on a missing attestation would redden every first-adoption job; advisories: PR threshold row has no `remote/inc` (fall back to `remote/full`), gate row should say one stale finding and that the pending finding remains |
| Runtime-reliability | PASS | 0 | 0 | every failure path correct, visible, recoverable; advisories: step 5 missing-attestation vs empty exit_code messages, verify failed-job outputs feed `full` in the trial, cancel grace shorter than 30 s |

Orchestrator context and synthesis go here (e.g. checkout sparse, filtered file-not-found artifacts, consolidation narrative). This section is audit trail only — it is NOT a finding stream. Do not extract sentences from here as review findings; only the `## Findings` section and its classified `## Blocking Findings`, `## Advisory Findings`, `## Follow-up Findings`, and `## Warnings` sections contain review findings.

## Findings

No reviewer findings recorded yet.

## Blocking Findings

### Testing-quality

- **[P2, 70] The PR summary step's cold-run case has the wrong exit code and no test**: step 5 says "(cold first run, exit 2)" but a cold first run exits 0 with no attestation. No row runs the job emulation cold, so a summary step that fails on a missing attestation would turn every first-adoption job red undetected. Fix the wording and extend the cold row to the job emulation. Evidence: spec §5.7 step 5, §5.6 step 7, §7.1.
