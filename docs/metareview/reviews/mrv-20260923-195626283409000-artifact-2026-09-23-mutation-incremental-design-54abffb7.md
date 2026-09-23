# metareview: artifact review

Run ID: `mrv-20260923-195626283409000-artifact-2026-09-23-mutation-incremental-design-54abffb7`

Target: `docs/superpowers/specs/2026-09-23-mutation-incremental-design.md`

Context pack: `docs/metareview/context/mrv-20260923-195626283409000-artifact-2026-09-23-mutation-incremental-design-54abffb7-context.md`

Execution mode: `parallel-subagents`

Previous run: `mrv-20260923-195021334650000-artifact-2026-09-23-mutation-incremental-design-54abffb7`

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

- Round r10: full 10-lens review with the pragmatic brief. 8 PASS, 2 NEEDS_REVISION converging on one blocker (gate cause for changed zero-mutant files). All r9 blockers verified fixed.
- User decision after this round: PR jobs use the absolute threshold (decision 15 revised); the relative reference machinery is removed.
| Scope and alignment | PASS | 0 | 0 | all §10 decisions honoured; advisories: un-rebased PR vs improved main under relative, PR raising break, spec density |
| Completeness | PASS | 0 | 0 | all workflows traced; advisories: pass `<stateDir>/incremental.json` to the gate, cold-run step summary, merge_group, local relative, main re-runs merged diff |
| Security | PASS | 0 | 0 | token scoping sound; advisories: mask header value, append GIT_CONFIG index, token wording at §5.7, rulesets must allow force-push |
| Intent preservation | NEEDS_REVISION | 1 | 0 | gate cause 3 never fires for a changed zero-mutant `mutate` file, so kills depending on it report verified (planner handles it, gate does not) |
| Data-migration | PASS | 0 | 0 | new prefixes clash with nothing; engine-scoped supersede sound; advisories: monorepo partial report set, dropping an engine, positional fingerprint parsing |
| Feasibility | PASS | 0 | 0 | reference score, catch-up, GIT_CONFIG_* header, permissions {} all buildable; advisories: catch-up counts toward full-job timeout, barrels trip the budget, URL-scoped extraheader and append to GIT_CONFIG_COUNT, name full job setup steps |
| Architecture | PASS | 0 | 0 | all flows converge; advisories: green catch-up result discarded (save to cache), relative vs improved main needs rebase note, seed is local-only |
| Mechanical-precision | PASS | 0 | 0 | all r10 text pins one outcome; advisories: Node-version compare strips `v`/skips aliases, `baseline` raw vs resolved, empty token = unset, append GIT_CONFIG_COUNT, reference ignores deferral ranking by design |
| Testing-quality | NEEDS_REVISION | 1 | 0 | converges with intent: gate §6.3 has no cause for a changed zero-mutant `mutate` file, and no gate row tests it; advisories: report vs attestation `files` in index row, strict row forcedCount > 1, name fixture for no-reference row |
| Runtime-reliability | PASS | 0 | 0 | every failure path ends correct, visible, recoverable; advisories: green catch-up discarded, catch-up counts toward job timeout, re-running an old main run, step summary `if: always()` tolerating a missing attestation |

Orchestrator context and synthesis go here (e.g. checkout sparse, filtered file-not-found artifacts, consolidation narrative). This section is audit trail only — it is NOT a finding stream. Do not extract sentences from here as review findings; only the `## Findings` section and its classified `## Blocking Findings`, `## Advisory Findings`, `## Follow-up Findings`, and `## Warnings` sections contain review findings.

## Findings

No reviewer findings recorded yet.

## Blocking Findings

### Intent preservation

- **[P1, 80] The gate reports kills verified after a changed zero-mutant `mutate` file** (constants, barrels): §6.3 cause 3 reads only the changed file's mutants' `coveredBy`, which is empty, so the planner forces the dependent kills (§5.4 step 4) but the gate calls them verified before the harness re-runs. Make a changed `mutate` file with no mutants in the report a blanket cause like support/global; add a gate row. Evidence: spec §6.3, §5.4 step 4, §7.1.

### Testing-quality

- **[P1, 80] Converges with intent preservation** (gate misses the zero-mutant cause; no gate row covers it). Evidence: spec §6.3, §7.1.
