# metareview: artifact review

Run ID: `mrv-20260923-194341093013000-artifact-2026-09-23-mutation-incremental-design-54abffb7`

Target: `docs/superpowers/specs/2026-09-23-mutation-incremental-design.md`

Context pack: `docs/metareview/context/mrv-20260923-194341093013000-artifact-2026-09-23-mutation-incremental-design-54abffb7-context.md`

Execution mode: `parallel-subagents`

Previous run: `mrv-20260923-193343742411000-artifact-2026-09-23-mutation-incremental-design-54abffb7`

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
| Security | NEEDS_REVISION | 1 | 0 | `contents: write` cannot be scoped to a step or made conditional per event; every step in the mutation jobs could push |
| Scope and alignment | PASS | 0 | 0 | no drift; advisories held (main threshold break reddens PRs, "ordering only" wording, lock/manifest deletion candidates) |
| Intent preservation | PASS | 0 | 0 | all intents and decisions 1–14 preserved; advisories held (timestamps order only, gremlins score summary, gate coarseness note, threshold-reddens-PRs needs a decision) |
| Data-migration | PASS | 0 | 0 | ledger/overrides/digest/state sound; advisories held (supersede only rows of supplied reports, override-line rendering scope, docs on dropping reports) |
| Feasibility | PASS | 0 | 0 | reproduced on real StrykerJS 10 + git bare remote; should-fix: split incremental job per event for permissions; advisories held (no --depth on fetch, ls-remote for missing branch, committer identity, NaN score ⇒ false, killedBy as a set) |
| Runtime-reliability | NEEDS_REVISION | 1 | 0 | a full run ending in "No tests were executed" (e.g. a dependency bump breaking every import) could re-attest old kills as a successful full run |
| Testing-quality | NEEDS_REVISION | 1 | 0 | "add a test to kill a survivor" is never exercised against the real engine (b2 row conditional and resolves to []) |
| Mechanical-precision | NEEDS_REVISION | 2 | 0 | adoption winner undefined when the primary is usable; plan `forcedCount` pre/post budget drop undefined |
| Completeness | NEEDS_REVISION | 1 | 0 | default `runtime.commands: node --version` makes local state permanently pending whenever local Node differs from CI's |
| Architecture | PASS | 0 | 0 | harness/gate agree, state converges, ledger sound; advisories held (adoption tie-break, adopt only when primary has deferrals, permissions per job, publish no-op exit 0, upgrade note) |

## Orchestrator Notes (not findings)

- Round r8: full 10-lens adversarial review with the user's pragmatic brief. 5 PASS (scope and alignment, intent preservation, data-migration, architecture, feasibility), 5 NEEDS_REVISION with one or two narrow blockers each: job permissions (security; feasibility and architecture concur), default `node --version` runtime input (completeness), survivor-kill e2e scenario (testing), adoption winner and `forcedCount` definition (mechanical), full-mode no-tests outcome (runtime).
- Policy question raised by two lenses (scope, intent): a threshold break on main turns every PR red — needs a user decision.
- Advisory consolidation deferred.

Orchestrator context and synthesis go here (e.g. checkout sparse, filtered file-not-found artifacts, consolidation narrative). This section is audit trail only — it is NOT a finding stream. Do not extract sentences from here as review findings; only the `## Findings` section and its classified `## Blocking Findings`, `## Advisory Findings`, `## Follow-up Findings`, and `## Warnings` sections contain review findings.

## Findings

## Blocking Findings

### Runtime-reliability

- **[P1, 75] "No tests were executed" in full mode has no defined outcome**: a dependency bump that breaks every test's imports makes the full dry run find zero tests (`3-dry-run-executor.js:46`); steps 5–6 read literally re-attest the previous report as a successful full run (`deferrals: []`, new `lastFullAt`), which is published and adopted everywhere, and the gate calls old kills verified. In full mode this outcome must be an engine failure (exit 4, nothing committed). Evidence: spec §5.6 steps 5–6, §5.5; engine source.

### Testing-quality

- **[P2, 70] The survivor-killing workflow is never run against the real engine**: the fixture has no survivors in the base state, the `tests/b2.test.ts` row is conditional and resolves to `[]`, so a broken step-2 survivor branch (or Stryker not re-running a survivor for a new covering test under `--incremental` without `--force`) passes. Add a survivor variant scenario: new case in `tests/a.test.ts` kills it; assert scope `[src/a.ts]`, the mutant becomes Killed, equivalence holds. Evidence: spec §5.4 step 2, §7.1.

### Mechanical-precision

- **[P2, 75] The adoption winner is undefined when the usable primary is beaten by several candidates** (e.g. `remote/full` and `remote/inc` with the same newer `lastFullAt`), changing plan and argv. Evidence: spec §5.4 Adopt.
- **[P2, 75] The plan's `forcedCount` after a budget overflow is undefined** (pre- or post-drop; edited files included or not). Evidence: spec §5.4 Normalise, Plan JSON, §7.1 expectations.

### Completeness

- **[P1, 75] Local state stays pending forever when local Node differs from CI's**: the shipped `runtime.commands: [["node","--version"]]` digest from an adopted CI state differs on a laptop, deferring `["*"]` on every adoption; only a multi-hour local full run clears it, and the next adoption undoes that. Pin Node via a tracked file (e.g. `.nvmrc` as a global input) instead of a default runtime command, and add an e2e row. Evidence: spec §5.2, §5.4, §5.7.

### Security

- **[P2, 75] The write token is broader than `publish-state` and the requested scoping is not expressible**: GitHub Actions `permissions:` are per job/workflow, not per step and not conditional per event, and `actions/checkout` persists the token by default, so every step of the 60/350-minute mutation jobs (npm postinstall, tests, Stryker) could push. Keep mutation jobs `contents: read` and publish from a small main-only job (or pass the token only to the publish step with `persist-credentials: false`); fix the static test. Evidence: spec §5.7, §7.2.
