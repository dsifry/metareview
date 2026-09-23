# metareview: artifact review

Run ID: `mrv-20260923-210334450256000-artifact-2026-09-23-mutation-incremental-design-54abffb7`

Target: `docs/superpowers/specs/2026-09-23-mutation-incremental-design.md`

Context pack: `docs/metareview/context/mrv-20260923-210334450256000-artifact-2026-09-23-mutation-incremental-design-54abffb7-context.md`

Execution mode: `parallel-subagents`

Previous run: `mrv-20260923-205431264136000-artifact-2026-09-23-mutation-incremental-design-54abffb7`

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

- r19 full 10-lens review (final stance; the Keeper-shaped §11.7 rows exercised at the Keeper agent's request): 9 PASS, 1 NEEDS_REVISION (feasibility). Completeness, intent preservation and mechanical precision raised the same gap as an advisory.
- r20 fixes it and folds in the converging advisories: both checks required; the job timeout includes kill grace and verifier time, with `verify.timeoutMinutes` required under `pendingOnPr` ≠ `allow`; the sweep limited to `sameRunTarget` and engine; seed mutant renumbering; `inherited` defaults to false; decision 19 aligned with contract §7. r20 gets a full 10-lens pass.
| Scope and alignment | PASS | 0 | 0 | §11 matches contract incl. §7 and decision 21; generic; advisories: mark contract K2 bullet superseded by §7, align decision 19 wording with the adopted-mode bar |
| Security | PASS | 0 | 0 | no credential or privilege path; pr-full read-only, never writes state; caches ref-scoped; advisories: single-line JSON for pending_causes and env-passed outputs (injection), job-level env reaches verifier, bad view map blocks main publishing |
| Testing-quality | PASS | 0 | 0 | every Keeper-shaped §11.7 behaviour has a row or test with hand-derived expectations; advisories: pr-full failure row and fake-exit test, push-during-sweep is static-test only, re-run list View column, override scope per view in Go test |
| Architecture | PASS | 0 | 0 | Keeper rows exercised: full-on-global routing, push during sweep, view-aware supersede, planner/gate agreement all hold; advisories: pr-full catch-up before full (cost), both jobs required checks, summary 'verdict deferred to pr-full', cache recency note |
| Completeness | PASS | 0 | 0 | every Keeper-shaped row has a defined outcome; full sweep has no time limit; advisories: add budget reason to the timeout bucket for totality, name the e-flip-residual mutant, repeated-timeout cost note, summary 'superseded by pr-full' |
| Feasibility | NEEDS_REVISION | 1 | 0 | MAJOR: a counted budget deferral carried from main (hot file the PR also edits) has no pending_cause class, so the PR fails with no cause and no sweep, breaking decision 21; classify it as `timeout`; advisories: count such PRs in the viability replay, verifier time in job sizing, carried no-reachable-tests summary |
| Runtime-reliability | PASS | 0 | 0 | every r19 PR path traced, no silent pass or stuck state; advisories: both jobs must be required checks, job timeout must include kill grace and per-view verify time (else clock-dependent red), pr-full sizing, watch main's full job |
| Intent preservation | PASS | 0 | 0 | no path merges a PR's own unverified change or turns a PR green without its sweep; advisories: budget reason bucket (converges with feasibility major), pr-full required check, job timeout vs verifier time, carried no-reachable-tests wording |
| Mechanical-precision | PASS | 0 | 0 | precedence, routing, gating, cache keys, rendering all deterministic; advisories: budget reason has no class (converges), global hides unreachable (docs), close routing with a catch-all, note swallowed verifier exit 1 |
| Data-migration | PASS | 0 | 0 | rows always clearable, old fields fail safe, seeds stay pending, keys distinct; advisories: sweep limited to sameRunTarget and engine, one unviewed cutover run, renumber mutant ids on seed merge, missing inherited = false |

Orchestrator context and synthesis go here (e.g. checkout sparse, filtered file-not-found artifacts, consolidation narrative). This section is audit trail only — it is NOT a finding stream. Do not extract sentences from here as review findings; only the `## Findings` section and its classified `## Blocking Findings`, `## Advisory Findings`, `## Follow-up Findings`, and `## Warnings` sections contain review findings.

## Findings

No reviewer findings recorded yet.

## Blocking Findings

### Feasibility

- **[MAJOR] A counted budget deferral carried from main has no `pending_cause` class**: a PR editing a hot file named in main's `forced set … exceeds budget …` deferral gets `none`. A strict verifier then fails it with no cause and no sweep, contradicting decision 21. Fix: classify it as `timeout` (routes to the sweep); `none` only when nothing is counted. Evidence: §11.2, §5.4 budget.
