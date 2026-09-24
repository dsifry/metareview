# metareview: artifact review

Run ID: `mrv-20260923-190310858720000-artifact-2026-09-23-mutation-incremental-design-54abffb7`

Target: `docs/superpowers/specs/2026-09-23-mutation-incremental-design.md`

Context pack: `docs/metareview/context/mrv-20260923-190310858720000-artifact-2026-09-23-mutation-incremental-design-54abffb7-context.md`

Execution mode: `parallel-subagents`

Previous run: `mrv-20260923-185110177502000-artifact-2026-09-23-mutation-incremental-design-54abffb7`

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
| Scope and alignment | PASS (carried from r3) | 0 | 0 | not re-run: passed round 3 (targeted re-review per user decision 10) |
| Intent preservation | NEEDS_REVISION | 1 | 0 | r3 blocker resolved; new: acceptedState not carried by incremental runs (accepted ⇒ verified), and if carried nothing triggers the clearing full run |
| Completeness | NEEDS_REVISION | 1 | 0 | all 3 r3 blockers resolved; new: support/unclassified files imported via aliases/bare specifiers by some tests are partially forced and then verified |
| Security | NEEDS_REVISION | 1 | 0 | both r3 blockers resolved; new: incremental runs drop acceptedState ⇒ accepted kills become verified (converges with intent) |
| Data-migration | NEEDS_REVISION | 1 | 0 | r3 #1 and routed severity resolved, #2 partially; new: a recurring superseded fingerprint is swallowed by Reconcile (verified by orchestrator), so enforce passes silently |
| Runtime-reliability | NEEDS_REVISION | 2 | 0 | R3-2/R3-3 resolved; R3-1 partial: signal/crash paths leave orphan groups that write shared-named work files; new: lock holder never re-checks ownership (nonce), path-based release/heartbeat |
| Mechanical-precision | NEEDS_REVISION | 3 | 0 | all 7 r3 blockers resolved; new: helper-edit oracle contradicts step 3 (and budget); plan sort order self-contradictory; gate misses T_Y residual cause that the planner forces |
| Testing-quality | NEEDS_REVISION | 2 | 0 | r3 blockers resolved (one partly); new: helper-edit oracle wrong under step 3 + budget; delete-`src/b.ts` oracle unreachable (b.test still imports it) |
| Feasibility | NEEDS_REVISION | 2 | 0 | all 5 r3 blockers resolved; new: zero-mutant selection file ⇒ permanent exit 4 (converges with architecture); actions/cache path lists differ between save and restore ⇒ every run cold |
| Architecture | NEEDS_REVISION | 2 | 0 | all 5 r3 blockers resolved; new: zero-mutant scope file absent from report `files` ⇒ permanent exit 4; supersede rule lacks sameRunTarget (task-done clears pr-ready enforce rows/overrides) |

## Orchestrator Notes (not findings)

- Round 4: targeted re-review of spec r4 per user decision 10 — the 9 lenses that failed round 3 re-ran, each verifying its own round-3 blockers and hunting defects introduced by r4; scope and alignment carried its round-3 PASS.
- Round-3 blocker disposition: all resolved except runtime-reliability R3-1 (partial: signal/crash paths) and data-migration #2 (partial: first-cause-only fingerprint). Every lens found new, narrower defects; all returned NEEDS_REVISION.
- Cross-lens convergence (audit only): acceptedState not carried by incremental runs (intent, security); zero-mutant selection file ⇒ permanent exit 4 (feasibility, architecture); fixture oracles vs step 3/budget (testing, mechanical-precision); supersede/Reconcile lifecycle (data-migration, architecture).
- Orchestrator verified the data-migration Reconcile finding: `internal/findings/findings.go:155-163` treats every non-`fixed` row, including `superseded`, as active.
- Advisory consolidation deferred.

Orchestrator context and synthesis go here (e.g. checkout sparse, filtered file-not-found artifacts, consolidation narrative). This section is audit trail only — it is NOT a finding stream. Do not extract sentences from here as review findings; only the `## Findings` section and its classified `## Blocking Findings`, `## Advisory Findings`, `## Follow-up Findings`, and `## Warnings` sections contain review findings.

## Findings

## Blocking Findings

### Intent preservation

- **[P1, 75] Accepted state becomes verified after the first incremental run**: §5.5 says only that a full run clears `acceptedState`; nothing tells incremental runs to carry it, so a fresh attestation writes `null` and §6.3 moves accepted kills to `verified`. Carrying it instead never triggers a full run (`pending_full` ignores it; G4), so enforce blocks permanently. Carry it forward, make `pending_full` true when it is non-null, amend G4, and add a seed-accept → incremental test. Evidence: spec §5.1, §5.5, §5.6 step 8, §5.7, §5.8, §6.3.

### Completeness

- **[P1, 75] Partially resolvable importers silently verify kills**: when a support (or unclassified) file is imported relatively by one test and via an alias/bare specifier by another, step 3 finds a non-empty importer set, forces only the resolved test's kills, records the new digest, and the gate then reports the other test's kills `verified`. Needs a fail-closed rule for unresolved non-relative specifiers in the graph (or alias resolution + stated residual) and an alias scenario. Evidence: spec §5.4 import graph, steps 3–4, §6.3, §8.

### Security

- **[P1, 50] One incremental run converts `acceptedState` to `verified`** (no carry-forward rule for non-full runs), dissolving the enforce blocker without a full run; converges with the intent-preservation finding. Evidence: spec §5.5, §5.8, §6.3.

### Data-migration

- **[P1, 100] A recurring superseded fingerprint is swallowed**: `Reconcile` builds `activeExisting` from every non-`fixed` row including `superseded` (`internal/findings/findings.go:155-163`, verified by orchestrator), and `superseded` is not `Blocks()` (`internal/findings/override.go:53-54`). Content/mode-addressed mutation fingerprints recur routinely (edit+revert, mode flip back, a run without reports), so the returning enforce finding never blocks. The spec must treat superseded `mutation:*` rows as inactive so recurrence opens a new row. Evidence: cited lines; spec §6.5.

### Runtime-reliability

- **[P1, 75] (R3-1 not fully resolved) Orphaned engine groups can write into a later run's work files**: step 9 forwards SIGINT/SIGTERM without escalation or waiting; SIGHUP/uncaught errors/OOM are unhandled; the timeout path waits only for the leader; Stryker's exit handler recreates `work/` (`reporters/mutation-test-report-helper.js:39-47,125-128`); fixed names `inv-k.json` collide across runs, and the judge accepts the foreign bytes. Needs per-run nonce work dirs, group SIGKILL + wait-until-ESRCH on every exit path before lock release, pgid in the lock. Evidence: cited source; spec §5.6 steps 2, 4, 9.
- **[P1, 50] The lock holder never verifies ownership**: a suspended holder or failed heartbeat allows takeover; the old holder then touches/deletes the new owner's lock by path and may commit another run's work file against its own snapshot. Needs a lock nonce checked before heartbeat and commit, fatal heartbeat failure, nonce-matched release, and filesystem-clock age comparison. Evidence: spec §5.6 steps 1, 2, 8, 9.

### Mechanical-precision

- **[P2, 75] The helper-edit oracle contradicts step 3**: the same ids that put `src/a.ts` in scope for the `tests/b.test.ts` edit also force `src/a.ts` mutants for the helper edit, and on the small fixture the bounded budget likely empties forced; the expected `[src/b.ts:4-4]` is wrong under the rules. Evidence: spec §5.4 steps 2–3, Normalise; §7.1.
- **[P2, 75] Plan sort order contradicts itself** ("byte order" vs "file then start line"; object-array sort keys undefined), changing plan JSON and `--mutate` argv. Evidence: spec §5.4.
- **[P2, 50] The gate lacks the residual cause the planner forces**: a changed other `mutate` file Y is not a stale cause for kills whose `killedBy` intersects Y's covering tests, so those kills are `verified` while the harness would force them (G3). Evidence: spec §5.4 step 4, §6.3.

### Testing-quality

- **[P1, 75] The helper-edit oracle `forced [src/b.ts:4-4]` is wrong under §5.4 step 3**: `b.test` covers `src/a.ts` mutants too, so forced includes `src/a.ts` ranges and likely exceeds `floor(0.25×N)`, producing a budget deferral instead. Evidence: spec §5.4 step 3, Normalise, §5.2, §7.1.
- **[P1, 75] The "delete `src/b.ts`" oracle is unreachable**: no scope for deleted mutate files, residual forcing likely defers, and if it runs `tests/b.test.ts` still imports `./b` so the dry run fails (exit 4); F1's drop only happens in a successful run. The scenario must also change `b.test.ts` and state the resulting plan. Evidence: spec F1, F6, §5.4 steps 1, 4, §5.6 step 5, §7.1.

### Feasibility

- **[P1, 75] A zero-mutant file in a selection fails every run permanently** (`toFileResults`, `mutation-test-report-helper.js:171-179`; barrels, types, constants modules reached by real imports). Evidence: cited source; spec §5.6 step 5.
- **[P1, 75] `actions/cache` save and restore path lists never match**: the cache version hashes the `path` input, so restoring `<stateDir>` never hits entries saved as the two files, and restoring into `.mutation-full/` never hits entries saved from `stateDir`; every PR is cold and every main push triggers a full run. Use identical literal path lists per family and assert it statically. Evidence: spec §5.7 steps 1, 2, 4 and full job.

### Architecture

- **[P1, 75] A zero-mutant scope file makes multi-file incremental runs fail permanently**: Stryker builds `files` only from files with mutant results (`reporters/mutation-test-report-helper.js:171-172`), so the §5.6 judge ("`files` contains every file of the selection") rejects `[src/a.ts, src/types.ts]` ⇒ exit 4 with no deferral ⇒ repeats forever. Judge only files with mutants; give absent selected files their snapshot digest; add an e2e row editing both. Evidence: cited source; spec §5.6 step 5, §7.1.
- **[P2, 75] The supersede rule has no `sameRunTarget` guard**: a task-done run (always advisory mode) supersedes pr-ready's enforce rows and their overrides, which recur as open on the next pr-ready run; every other Reconcile transition is target-guarded (`internal/findings/findings.go:124,140,269-277`). Evidence: cited lines; spec §6.4, §6.5.
