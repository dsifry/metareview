# metareview: artifact review

Run ID: `mrv-20260923-204712580916000-artifact-2026-09-23-mutation-incremental-design-54abffb7`

Target: `docs/superpowers/specs/2026-09-23-mutation-incremental-design.md`

Context pack: `docs/metareview/context/mrv-20260923-204712580916000-artifact-2026-09-23-mutation-incremental-design-54abffb7-context.md`

Execution mode: `parallel-subagents`

Previous run: `mrv-20260923-204202233770000-artifact-2026-09-23-mutation-incremental-design-54abffb7`

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

- r16 review (10 lenses, blocking only on major or critical findings): 7 PASS, 3 NEEDS_REVISION with narrow major findings in §11.2, §11.1 and §11.3. All six r15 majors are verified fixed; feasibility re-checked §11.4.2 against the StrykerJS 10 source.
- r17 fixes all three. Per the decision 10 precedent it is verified by the lenses that raised them (completeness, intent preservation, runtime reliability, mechanical precision).
| Feasibility | PASS | 0 | 0 | §11.4.2 verified in StrykerJS 10 source (whole-file unforced rebuild; forced ranges carry out-of-scope mutants); advisories: rebased PR during main pending pays a sweep, time-budget exits under full-on-global, Myers vs diff-match-patch mapping, single-pass verifier, run plan locally after adding a module |
| Completeness | NEEDS_REVISION | 1 | 0 | MAJOR: verifier cannot tell inherited from counted pending (not in attestation or env), so a survivor-bar verifier fails every PR during main's pending window or ignores the PR's own pending |
| Scope and alignment | PASS | 0 | 0 | §11 implements K1–K9 exactly, additions minimal; advisory: verifier inherited/counted visibility |
| Architecture | PASS | 0 | 0 | no major findings |
| Intent preservation | NEEDS_REVISION | 1 | 0 | MAJOR: inheritance compares only {reason, paths}, so a PR's own lockfile or file change matching main's pending deferral is classed inherited and merges unverified; require unchanged digests of the named paths |
| Security | PASS | 0 | 0 | no major findings |
| Testing quality | PASS | 0 | 0 | every core §11 behaviour tested; new rows derivable by hand |
| Data migration | PASS | 0 | 0 | no major findings |
| Runtime reliability | NEEDS_REVISION | 1 | 0 | MAJOR: converges with intent (PR editing a global input matched as inherited, summary falsely says main's full run clears it) |
| Mechanical precision | NEEDS_REVISION | 1 | 0 | MAJOR: renamed-view sweep applies when no attested report carries views, so a view-scoped run can supersede its own new blocker and other views' rows; restrict to runs with a views map and to earlier rows not produced by this run |

Orchestrator context and synthesis go here (e.g. checkout sparse, filtered file-not-found artifacts, consolidation narrative). This section is audit trail only — it is NOT a finding stream. Do not extract sentences from here as review findings; only the `## Findings` section and its classified `## Blocking Findings`, `## Advisory Findings`, `## Follow-up Findings`, and `## Warnings` sections contain review findings.

## Findings

No reviewer findings recorded yet.

## Blocking Findings

- **[MAJOR] Inheritance compares only `{reason, paths}`** (intent-preservation, runtime-reliability): a PR that changes the same lockfile main is still re-establishing is classed as inherited and merges unverified. Fix: also require unchanged digests of the named paths or keys. Evidence: §11.2, decision 20.
- **[MAJOR] The verifier cannot distinguish inherited from counted pending** (completeness): a survivor-bar verifier either fails every PR during main's pending window or ignores the PR's own pending. Fix: an `inherited` flag per deferral, a split `viewSummaries`, and `MUTATION_RUN_KIND`. Evidence: §11.1, §11.2, §11.3.
- **[MAJOR] The renamed-view sweep applies with no view map** (mechanical-precision): a view-scoped run on an unattested or view-less report can supersede its own new blocker and other views' rows. Fix: restrict the sweep to runs with a `views`-carrying attested report, and to earlier rows this run did not produce. Evidence: §11.3, §6.5.
