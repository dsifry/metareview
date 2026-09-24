# metareview: artifact review

Run ID: `mrv-20260923-210703118968000-artifact-2026-09-23-mutation-incremental-design-54abffb7`

Target: `docs/superpowers/specs/2026-09-23-mutation-incremental-design.md`

Context pack: `docs/metareview/context/mrv-20260923-210703118968000-artifact-2026-09-23-mutation-incremental-design-54abffb7-context.md`

Execution mode: `parallel-subagents`

Previous run: `mrv-20260923-210334450256000-artifact-2026-09-23-mutation-incremental-design-54abffb7`

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

- r20 full 10-lens review (final stance, Keeper-shaped rows exercised): 9 PASS, 1 NEEDS_REVISION (intent preservation). The r19 major (the budget reason had no cause class) is verified fixed by feasibility and all others.
- r21 fixes the new major: a deferral produced by an invocation this run executed is always counted. It also fixes the §11.2/§11.7 cache-on-exit-1 contradiction (flagged by 4 lenses), states decision 21's one remaining speed-dependent bound (the `pr-full` job timeout), and folds in the sizing and seed advisories. r21 gets a full 10-lens pass.
| Scope and alignment | PASS | 0 | 0 | K1–K9 and every §7 decision covered; decisions 2/12 amended explicitly, 16–21 traced; additions all enforce contract properties; advisories: contract K9 and K2 `full` bullet wording, keep Keeper as examples in docs |
| Security | PASS | 0 | 0 | credential model holds; pr-full has no token; caches ref-scoped; outputs single-line JSON via env; advisories: summary shows paths as-is, views output public, runtime token note |
| Completeness | PASS | 0 | 0 | every Keeper-shaped row maps to a normative rule; advisories: pr-full exit-1 cache save vs §11.7 row wording, unreachable+timeout summary, pendingInherited uses the same advisory pending fingerprint |
| Architecture | PASS | 0 | 0 | job structure, cancellation, timeouts, planner/gate agreement and view-aware ledger hold; extends existing sameRunTarget/openForRun; advisories: fix §11.7 row to 'exit 1 red, cache saved / exit 4 red, no save', repeat cache note in adopter docs |
| Testing-quality | PASS | 0 | 0 | every Keeper-shaped row derivable by hand and catches the plausible bugs; advisories: static test pins incremental-pr cache condition and timeout formula, carried-unreachable summary row or drop, e-flip-residual mutant in expectations.json |
| Mechanical-precision | PASS | 0 | 0 | cause classes exhaustive, routing decidable, inherited default and precedence unambiguous; advisories: pr-full exit-1 cache wording (converges), max(1, views) in timeout formula, seed status precedence and renumber order |
| Data-migration | PASS | 0 | 0 | no data loss, no stuck state, nothing unverified reported verified; unviewed rows never block viewed runs; advisories: seed status precedence, missing stateVersion = mismatch and gate never reads it, cutover run is tidy-up only |
| Runtime-reliability | PASS | 0 | 0 | no silent pass, stuck state, or speed-dependent red within the harness's limits; advisories: pr-full exit-1 cache row (converges), decision 21 caveat for pr-full job timeout, verifier kill grace and setup definition |
| Feasibility | PASS | 0 | 0 | r20 routing, shortcut, timeout formula (fits 360 min for Keeper at T ≤ 14), pr-full sizing and failure handling buildable; advisories: pr-full exit-1 cache row (converges), main full job sizing lacks verifier time, cap pending_causes size |
| Intent preservation | NEEDS_REVISION | 1 | 0 | MAJOR: a PR's own timeout on a scope identical to main's carried timeout deferral passes (i)+(ii) and is classed inherited, so the PR goes green without its sweep; deferrals produced by an invocation this run executed must always be counted |

Orchestrator context and synthesis go here (e.g. checkout sparse, filtered file-not-found artifacts, consolidation narrative). This section is audit trail only — it is NOT a finding stream. Do not extract sentences from here as review findings; only the `## Findings` section and its classified `## Blocking Findings`, `## Advisory Findings`, `## Follow-up Findings`, and `## Warnings` sections contain review findings.

## Findings

No reviewer findings recorded yet.

## Blocking Findings

### Intent preservation

- **[MAJOR] A PR's own timeout can be classed "inherited"**: when a PR's invocation times out on a scope identical to main's carried `time budget exceeded` deferral with unchanged digests, rules (i)+(ii) mark it inherited, so `pending_cause=none` and `pr-full` is skipped, contradicting decision 21. Fix: deferrals produced by an invocation this run executed are always counted. Evidence: §11.2.
