# metareview: artifact review

Run ID: `mrv-20260923-200638249049000-artifact-2026-09-23-mutation-incremental-design-54abffb7`

Target: `docs/superpowers/specs/2026-09-23-mutation-incremental-design.md`

Context pack: `docs/metareview/context/mrv-20260923-200638249049000-artifact-2026-09-23-mutation-incremental-design-54abffb7-context.md`

Execution mode: `parallel-subagents`

Previous run: `mrv-20260923-200340521908000-artifact-2026-09-23-mutation-incremental-design-54abffb7`

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

- Round r13: full 10-lens review with the pragmatic brief. 8 PASS, 2 NEEDS_REVISION (architecture, data-migration) converging on one ledger blocker, confirmed in code (`internal/findings/override.go:74-75, 116-117` accept only `open` or `fixed`-with-escalation rows).
- Human direction (user, 2026-09-23): the review has converged. r14 fixes the blocker and folds in the advisories, then gets one final full review in which only major or critical findings block.
| Feasibility | PASS | 0 | 0 | all r13 changes buildable; overridden-exclusion matches existing code; advisories: compute `final_exit` in a shell step (an `&&`/`||` expression falls through on empty output), summary line naming a rejected publish |
| Security | PASS | 0 | 0 | credentials, forks, caches, trust model and override separation all sound; advisories: tests must not print env/config, no job-level github.token (static check), private repos without fork Actions |
| Scope and alignment | PASS | 0 | 0 | all 15 decisions and origin §1 honoured, no creep; advisories: surface full-job timeout in summary, move process decisions 10–11 out of §10, trim status header, split §9 in the plan |
| Completeness | PASS | 0 | 0 | every workflow end to end incl. first adoption, upgrades, overrides; advisories: repeatedly failing full run retries each push (doc), catch-up could skip when plan defers `*`, local full run clears PR pending, define `N` as the PR number |
| Architecture | NEEDS_REVISION | 1 | 0 | a stale-only escalation cannot be lifted once its rows are superseded: RequestOverride/GrantOverride accept only open or fixed-with-escalation (override.go:74-75, 116-117); convergence and planner/gate agreement hold |
| Mechanical-precision | PASS | 0 | 0 | final_exit, rejected publish, overridden exclusion, main-score order, .nvmrc all determined; advisories: empty exit_code prints only its line, label main's score when it has deferrals, .nvmrc alias wording |
| Runtime-reliability | PASS | 0 | 0 | every failure path ends red or visibly pending; advisories: red incremental-main on a global push triggers no full run (doc re-run), 'did not execute or did not finish' wording, name full-job timeout in summary |
| Testing-quality | PASS | 0 | 0 | every §7.1 value re-derived; r13 tests catch the obvious bugs; advisories: fake-exit case (catch-up 0 + pending, full 1), static assert catch-up publish only when pending_full false, green-catch-up row baseline value |
| Data-migration | NEEDS_REVISION | 1 | 0 | converges with architecture: superseded stale rows cannot be overridden, so a stale-only escalation cannot be lifted (override.go:74-75, 116-117); advisories: old grants accumulate in the override list |
| Intent preservation | PASS | 0 | 0 | every intent and decision intact; advisories: PR self-created deferrals re-run the delta each push (doc), surface full-job timeout, gate-stricter stale spike after zero-mutant edits (doc) |

Orchestrator context and synthesis go here (e.g. checkout sparse, filtered file-not-found artifacts, consolidation narrative). This section is audit trail only — it is NOT a finding stream. Do not extract sentences from here as review findings; only the `## Findings` section and its classified `## Blocking Findings`, `## Advisory Findings`, `## Follow-up Findings`, and `## Warnings` sections contain review findings.

## Findings

No reviewer findings recorded yet.

## Blocking Findings

### Architecture

- **[P1, 80] A stale-only escalation cannot be lifted once its rows are superseded**: after an escalation at 2 × `maxAttempts`, refreshing the evidence supersedes the stale rows. `RequestOverride` and `GrantOverride` refuse `superseded` rows, and `EscalationLiftedByOverrides` needs `overridden` ones, so no command clears the hard stop. Treat a superseded mutation row with an escalation like `fixed`. Evidence: spec §6.5, §6.8; `internal/findings/override.go:74-75, 116-117`; `internal/reviewstate/projector.go:583`.

### Data-migration

- **[P1, 80] Converges with architecture** (superseded stale rows cannot be overridden, so the escalation stays stuck). Evidence: same.
