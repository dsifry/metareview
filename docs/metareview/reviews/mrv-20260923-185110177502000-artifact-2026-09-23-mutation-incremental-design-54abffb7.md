# metareview: artifact review

Run ID: `mrv-20260923-185110177502000-artifact-2026-09-23-mutation-incremental-design-54abffb7`

Target: `docs/superpowers/specs/2026-09-23-mutation-incremental-design.md`

Context pack: `docs/metareview/context/mrv-20260923-185110177502000-artifact-2026-09-23-mutation-incremental-design-54abffb7-context.md`

Execution mode: `parallel-subagents`

Previous run: `mrv-20260923-183942601569000-artifact-2026-09-23-mutation-incremental-design-54abffb7`

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
| Scope and alignment | PASS | 0 | 0 | r2 blockers resolved; advisories held for consolidation |
| Intent preservation | NEEDS_REVISION | 1 | 0 | example `maxForcedMutants: 1000` silently overrides decision 5's approved 25% default |
| Testing-quality | NEEDS_REVISION | 6 | 0 | one-line-edit oracle wrong for fixture + oracles not derivable pre-run; equivalence lacks negative control/matching key/Timeout rule; fixture lacks own git root; Node coverage misses unloaded modules; manifest ignores fixture drift; types.ts row ambiguous |
| Mechanical-precision | NEEDS_REVISION | 7 | 0 | gate lacks exclusion/precedence data; §4 vs §5.3 ignore contradiction; deferral mutantIds unstable; no-related-tests all-or-per-file; fingerprint mode/digest8 edge cases; two-invocation exit code; "changed unclassified" vs new/deleted |
| Data-migration | NEEDS_REVISION | 3 | 0 | rotating fingerprints mark old rows (incl. override-pending) `fixed`; digest8 ignores stale-causing inputs; medium+blocking counts as warning so enforce never blocks (out-of-lens, verified) |
| Feasibility | NEEDS_REVISION | 5 | 0 | graph "no related tests" ≠ Vitest related (type-only ⇒ permanent exit 4); configFile never passed; full mode keeps mutants of files leaving `mutate`; upload-artifact drops hidden `.mutation`; coverage-include ignores unloaded modules |
| Architecture | NEEDS_REVISION | 5 | 0 | deferral mutantIds are report-local (verified→wrong class); changed-during-run sentinel turns scope deferrals into permanent stale; HEAD-mode skips committed tracked:false files; digest8 ignores causing inputs; exclusion set missing from attestation |
| Completeness | NEEDS_REVISION | 3 | 0 | deferral mutantIds unstable (per-run counter) ⇒ deferred kills later "verified"; changed test absent from R.testFiles selects nothing silently; pending_full value on exits 2/3/4 unspecified |
| Runtime-reliability | NEEDS_REVISION | 3 | 0 | timeout kills only npx (orphan Stryker overwrites next input); lock not mutually exclusive / container & macOS boottime lockouts / lock propagates via artifact; concurrency group starves full job |
| Security | NEEDS_REVISION | 2 | 0 | accept-current-state self-clears an enforce blocker; `gh run download --name` without run id is spoofable by fork artifacts |

## Orchestrator Notes (not findings)

- Round 3 (re-review of spec r3). All 10 lenses ran as independent parallel subagents; 1 PASS (scope and alignment), 9 NEEDS_REVISION. Three-round limit reached; escalated to the user, who chose option 3: revise to r4 and re-run only the failed lenses with an adversarial check scoped to changed sections.
- Cross-lens convergence (audit only): report-local mutant ids in deferrals (architecture, completeness, mechanical); digest8/fingerprint rotation and `fixed` marking (data-migration, architecture); accept-current-state as self-override (security); timeout kill/process group (runtime, feasibility, security); lock exclusivity (runtime, security); gate lacking exclusion data (mechanical, architecture); fixture/oracle precision (testing).
- Orchestrator verified the routed out-of-lens finding: `internal/findings/findings.go:806-815` counts `blocking` as a blocker only at `critical`/`high` severity.
- Advisory consolidation deferred to the final review round.

Orchestrator context and synthesis go here (e.g. checkout sparse, filtered file-not-found artifacts, consolidation narrative). This section is audit trail only — it is NOT a finding stream. Do not extract sentences from here as review findings; only the `## Findings` section and its classified `## Blocking Findings`, `## Advisory Findings`, `## Follow-up Findings`, and `## Warnings` sections contain review findings.

## Findings

## Blocking Findings

### Intent preservation

- **[P2, 75] The shipped default `maxForcedMutants: 1000` overrides decision 5's approved 25% threshold**: §5.4 defers when either limit is exceeded, so at Keeper scale (15,660 mutants) the effective default is ~6.4%, deferring forced sets the user's rule would run and triggering more full runs; decision 5 was changed without a "supersedes" note. Evidence: spec §5.2 example, §5.4 budget, §10.5.

### Testing-quality

- **[P1, 75] The "one-line edit in `src/a.ts`" oracle is wrong for the documented fixture** (T_Y includes `b.test.ts`; `a.test.ts` never loads `b.ts`), and "listed" kills/bounds are not derivable before a real run, reintroducing self-referential oracles. Oracles must be mutant identities (file, mutator, line) derivable by hand from committed fixture source. Evidence: spec §5.4 step 4, §7.1.
- **[P1, 75] The equivalence row has no negative control** (e.g. `residual.mode: off` must fail it), no cross-run matching key (ids are per-run; match on file/mutator/location/replacement), and no Timeout-vs-Killed rule. Evidence: spec §7.1.
- **[P1, 75] The e2e fixture has no git root of its own**, so `git rev-parse --show-toplevel` resolves to metareview; requires `git init`, commit, `.gitignore` for `node_modules`, and committing edits for the pr-ready gate row. Evidence: spec §5.1, §6.2, §7.1.
- **[P1, 75] Node coverage thresholds count only loaded modules**; the wrapper must assert every template `.mjs` appears at 100%. Evidence: spec §7.2.
- **[P2, 75] The manifest check ignores fixture sources, oracles and per-scenario trees**, and offline replay needs committed post-edit contents, not just digests. Evidence: spec §7.1, §7.2.
- **[P2, 50] The `src/types.ts` row is ambiguous**: if `a.ts` imports it, it is reachable ⇒ zero-mutant scope ⇒ no write ⇒ exit 4; pin the fixture import and cover zero-mutant scope explicitly. Evidence: spec §5.4, §5.6 step 5, §7.1.

### Mechanical-precision

- **[P2, 75]** The gate cannot apply §4 exclusions and §5.3 precedence: the attestation carries only `ignore` (no stateDir, reporter paths, or category lists), so state/report files and `tests/helpers/x.md` classify differently per implementer. Evidence: spec §4, §5.3, §5.5, §6.3.
- **[P2, 75]** §4 ("excluded: paths matching `ignore`") contradicts §5.3 ("excluded only if it matches no other list"). Evidence: spec §4, §5.3.
- **[P2, 50]** Accumulated deferral `mutantIds` are per-run Stryker ids, so a later run's pending set names different mutants; identity must be location-based. Evidence: spec §5.5, §6.3.
- **[P2, 75]** "No related tests" is ambiguous (all-or-nothing vs per file) and its traversal set differs from steps 3/4. Evidence: spec §5.4 normalisation.
- **[P2, 75]** Fingerprint components undefined for task-done under enforce (`<mode>`), unverifiable paths (`<digest8>`), and whether hex follows the `sha256:`/`symlink:` prefix. Evidence: spec §6.4, §6.5.
- **[P2, 50]** Exit code when invocations disagree (scope exit 1, forced exit 0 or timeout) is undefined, which drives CI. Evidence: spec §5.1, §5.6.
- **[P2, 50]** Step 4 handles only "changed" unclassified paths; new/deleted unclassified paths are unspecified while the gate treats new ones as changes. Evidence: spec §5.4 step 4, §6.3.

### Data-migration

- **[P2, 100] Rotating fingerprints (`<mode>`, `<digest8>`, `<reportkey>`) make `Reconcile` mark the old row `fixed` with a real `fixedInRunId`, including `override-pending` rows** (`internal/findings/findings.go:139-147`), breaking the overrides-vs-fixes audit separation and polluting `fixed` consumers beyond the §6.9 filter (e.g. `internal/learning/candidates.go:232`). Rotated `mutation:*` rows must become `StatusSuperseded` (`findings.go:194`, precedent `supersedeLegacyContextRisk` at `:208`). Evidence: cited lines; spec §6.5, §6.9.
- **[P2, 100] `<digest8>` covers only the stale file's own digest**, so an override persists while the test/support/global inputs that caused staleness keep changing. Evidence: spec §6.3, §6.5.
- **[P1, 100] (out-of-lens, routed; verified by orchestrator read of `internal/findings/findings.go:806-815`) "All severity `medium`" means enforce never blocks**: `classForCount` counts `blocking` findings as blockers only at `critical`/`high` severity, so a medium blocking finding counts as a warning ⇒ PASS_ADVISORY, and the §6.8 exemption never triggers. Evidence: cited lines; spec §6.5.

### Feasibility

- **[P1, 75] The graph-based "no related tests" pre-check diverges from Vitest `related`**: Vitest walks transformed deps (`vitest/dist/chunks/cli-api.CnMVyzaz.js:11576-11590`), eliding type-only imports and `vi.mock` strings, so a type-only file the harness deems related yields `ConfigError('No tests were executed…')` (`3-dry-run-executor.js:46`), no write, exit 4 on every run. Evidence: cited source; spec §5.4, §5.4.1, §7.1.
- **[P2, 75] The argv never passes `stryker.configFile`**, so Stryker's own search (`config/config-file-formats.js:12-18`, `.conf` before `.config`) can load a different file than the one validated. Evidence: spec §5.6 step 4.
- **[P2, 75] Full mode starts from the pre-run copy**, and the differ carries forward old mutants of files that left `mutate` regardless of `--force`, so they read as verified after a "successful full run". Start full mode with no work file. Evidence: `incremental-differ.js`; spec §5.6.
- **[P2, 75] `actions/upload-artifact` v4.4+ excludes hidden files by default**, so uploading `.mutation` produces an empty artifact; requires `include-hidden-files: true`. Evidence: spec §5.7, §5.8.
- **[P2, 75] `--test-coverage-include` does not count never-loaded modules** (experiment on Node 25.9 exited 0 at 100% thresholds with an unloaded included module). Evidence: spec §7.2.

### Architecture

- **[P1, 75] Deferral `mutantIds` are report-local** (`@stryker-mutator/core` `mutants/incremental-differ.js:16` "only unique within 1 report", carried-forward `id: mutantKey` at `:149`), so accumulated deferrals miss and deferred kills fall through to `verified`, breaking G2. Record deferrals at file granularity or re-key on every commit. Evidence: cited source; spec §5.4, §5.5, §6.3.
- **[P1, 75] The `changed-during-run` sentinel turns scope-level deferrals (timeout, no related tests) into permanent `stale`**: the committed report still embeds the old source, the sentinel never matches, class 1 precedes class 2, and the planner re-defers forever. Evidence: spec §5.5, §5.6 steps 5–6, §6.3.
- **[P2, 75] HEAD mode skips `tracked: false` attested paths even when they are now committed**, so committed content never mutation-tested is classified verified. Skip only when absent from HEAD. Evidence: spec §5.4 snapshot, §6.3.
- **[P2, 75] `<digest8>` hashes only the kill's file, not the causing inputs**, so overrides outlive changed tests/global inputs, and own-file re-edits churn rows as `fixed`. Evidence: spec §6.3, §6.5; `internal/findings/findings.go:139-148`.
- **[P2, 50] The attestation omits the resolved excluded-path set**, so the gate classifies non-ignored report outputs as new unclassified files. Evidence: spec §4, §5.5, §6.3.

### Completeness

- **[P1, 75] Deferral `mutantIds` cannot identify mutants in later reports**: StrykerJS assigns ids from a per-run counter (`@stryker-mutator/instrumenter` `mutant-collector.js:18`), so budget/residual-off/forced-failed/time-budget deferrals carried forward point at different or no mutants and those kills fall through to `verified`; needs a stable identity (file + location + mutator + replacement) and a two-run verification row. Evidence: cited source; spec §5.4, §5.5, §6.3.
- **[P1, 75] A changed `test` file absent from `R.testFiles` selects nothing and records no deferral**, and the gate's covering-test check never fires, so the edit is silently verified. Evidence: spec §5.4 step 2, §6.3.
- **[P2, 75] `pending_full` on exits 2/3/4 is unspecified**, and `false` after exit 4 with existing deferrals skips the only clearing path. Evidence: spec §5.1, §5.7.

### Runtime-reliability

- **[P1, 75] The timeout kill reaches only the `npx` wrapper**: Stryker and its workers survive, and Stryker's signal handler (`unexpected-exit-handler.js`, `mutation-test-report-helper.js:39-47`) later writes partial results in place to the same work file the next invocation is using. Requires process-group spawn/kill, waiting for exit before reset, and per-invocation work files. Evidence: cited source; spec §5.6 steps 4–5.
- **[P1, 75] The lock is not mutually exclusive and can permanently lock out runs**: rename-over takeover is not CAS; macOS `kern.boottime` shifts with clock steps; container pid/boot_id reuse ⇒ perpetual exit 3 or never-taken-over foreign locks; `lock`/`work/` propagate through the artifact and `seed --from-state`. Evidence: spec §5.6 step 1, §5.7, §5.8.
- **[P2, 75] The shared `mutation-main` concurrency group starves the full job**: GitHub keeps one pending entry per group and a newer queued job cancels the pending one, so on a busy main the full run never starts. Evidence: spec §5.7.

### Security

- **[P1, 75] `--accept-current-state` lets the actor facing an enforce blocker clear it** (locally unrestricted; `mutation:accepted` is advisory in every mode), contrary to CLAUDE.md "Process Overrides". Make `mutation:accepted` blocking in enforce and include an `acceptedState` digest in its fingerprint. Evidence: spec §5.8, §6.3, §6.5; CLAUDE.md.
- **[P2, 75] `gh run download --name mutation-state-main` without a run id resolves repository-wide**, so a fork PR's run can supply a crafted, self-consistent state that the gate then reports as verified. Resolve the run id from a successful push run on main first; copy only the two state files as regular files. Evidence: spec §5.8.

