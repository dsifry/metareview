# metareview: artifact review

Run ID: `mrv-20260923-182025208234000-artifact-2026-09-23-mutation-incremental-design-54abffb7`

Target: `docs/superpowers/specs/2026-09-23-mutation-incremental-design.md`

Context pack: `docs/metareview/context/mrv-20260923-182025208234000-artifact-2026-09-23-mutation-incremental-design-54abffb7-context.md`

Execution mode: `parallel-subagents`

Previous run: `none`

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
| Scope and alignment | NEEDS_REVISION | 2 | 0 | cause (1) whole-tree binding has no remedy/adoption contract; tier-C deferral invisible to the gate |
| Intent preservation | NEEDS_REVISION | 6 | 0 | deferral read as evidence; score removal defeats ramp; cold path and drift run reintroduce long/no-op runs |
| Testing-quality | NEEDS_REVISION | 5 | 0 | no real-engine output in CI; headline timing unasserted; fail-closed negatives untested; digest claim untested; no Keeper-trial criteria |
| Completeness | NEEDS_REVISION | 6 | 0 | state file never persisted in CI; unclassified changes pass silently; deleted files; full→incr cache hand-off and races; deferral/cold outcomes; UNBOUND under enforce |
| Mechanical-precision | NEEDS_REVISION | 12 | 0 | CLI, state schema, config/glob, tier combination, import resolution, comparison, finding contract, env values, exemption predicate, log format, threshold detection, cache keys undefined |
| Architecture | NEEDS_REVISION | 4 | 0 | pr-ready digest reuse bypasses enforce; gate certifies less than it claims; UNBOUND kills accepted; state/cache coupling undefined |
| Security | NEEDS_REVISION | 5 | 0 | partial fresh report hides survivors; no path containment; gate certifies unseen inputs; accept-current-state unaudited; CI cache/artifact trust boundary |
| Runtime-reliability | NEEDS_REVISION | 4 | 0 | "successful run" undefined (crash vs threshold); main workflows race on cache; state file not persisted; deferral invisible |
| Feasibility | NEEDS_REVISION | 2 | 0 | Stryker re-embeds current source for every file at write time, so embedded-source equality cannot prove freshness for out-of-scope edits and silently drops changed-region survivors; cache keys miss/collide |
| Data-migration | NEEDS_REVISION | 4 | 0 | enforce flip reuses old PASS via digest; re-rooting vs fingerprint stability; state file unversioned; no Keeper migration path |

## Orchestrator Notes (not findings)

Orchestrator context and synthesis go here (e.g. checkout sparse, filtered file-not-found artifacts, consolidation narrative). This section is audit trail only — it is NOT a finding stream. Do not extract sentences from here as review findings; only the `## Findings` section and its classified `## Blocking Findings`, `## Advisory Findings`, `## Follow-up Findings`, and `## Warnings` sections contain review findings.

- All 10 required lenses ran as independent parallel subagents (read-only); all returned NEEDS_REVISION.
- Cross-lens convergence (audit only): the gate cannot see harness-side state (tier-C deferral, support/global digests, accept-current-state) — raised independently by scope, intent, completeness, architecture, security, runtime-reliability, data-migration; state-file persistence and full-run/incremental cache hand-off — completeness, architecture, runtime-reliability, feasibility; pr-ready digest reuse bypassing enforce — architecture, data-migration.
- Advisory consolidation and the staff-filter pass were NOT performed for this run: the verdict is NEEDS_REVISION on blocking findings alone and the spec will be substantially rewritten, so advisories are deferred to the re-review (`--previous-run` this run) rather than written unfiltered. The Advisory Findings section is therefore intentionally absent in this run.

## Findings

## Blocking Findings

### Feasibility

- **[P1, 75] Embedded-source equality does not prove kill freshness, and narrowed runs silently drop survivors.** In StrykerJS 10.0.0, `dist/src/mutants/incremental-differ.js` carries out-of-scope mutants forward with their old `status` without calling `mutantCanBeReused`, rebuilding `killedBy`/`coveredBy` via `testKeysToId` (changed tests silently dropped ⇒ `status: Killed, killedBy: []`); `performFileDiff` removes (does not re-run or mark) mutants in changed regions of out-of-scope files, survivors included; and `reporters/mutation-test-report-helper.js` (`toFileResult`/`toTestFile`) embeds `readOriginal()` — current on-disk content — for every file. So after any `--incremental --mutate <subset>` run with edits outside the subset, the gate classifies those kills FRESH and the planner treats the edited files as unchanged forever. The gate can detect only post-report edits. Evidence: cited StrykerJS source paths; spec §Part 2 per-kill rules, §Goals 3.
- **[P2, 75] Cache keys miss or collide**: a full run saving only `stryker-full-<hash>` is never matched by the `stryker-incr-` restore prefix (tier C pending forever), and saving `stryker-incr-<sha>` collides with the per-push job's immutable key; the state file is not in the cached path. Evidence: spec §Warm cache, §Workflows.

### Scope and alignment

- **[P1, 75] Cause (1) is diagnosed but never remedied.** Problem §cause (1) (whole-tree manifest binding in Keeper's verifier) has no counterpart in Part 1/Part 2: an incremental report produced after any edit still fails Keeper's `STALE_MUTATION_REPORT` whole-tree check, so the user's slowdown persists for Keeper and `DEFERRED_MUTATION_UNTIL_0_13_0` has no exit. Missing: the harness contract (drop manifest binding; rely on Part 2 per-file/per-test freshness) as candidates §1's "publish the scoped binding pattern", plus a named adoption step. Evidence: spec §Problem, §Part 1; `docs/0.13.0-candidates.md` §1.
- **[P2, 75] Tier-C deferral is silent, contradicting Goal 2.** In incremental mode tier C is deferred but nothing reaches the gate (Part 2 reads only report fields), so a PR changing lockfile/config/migrations passes pr-ready with every kill FRESH; tier B's "orphan support file ⇒ escalate to C (fail closed)" becomes fail-open on PRs. Needs an explicit deferral signal (harness marker/exit status and/or advisory gate finding). Evidence: spec §Goals, §Part 1 tier table, §Part 2.

### Intent preservation

- **[P1, 75] Deferred tier C is invisible to the gate, so its kills are read as FRESH evidence**, contradicting the Keeper deferral intent ("can never be read as evidence", candidates §1) and the spec's "never silently" goal; `--accept-current-state` also erases tier-C detection with no gate-visible trace. Evidence: spec §Part 1 tier table, §Part 2.
- **[P1, 50] Tier B "fail closed" is fail-open on PRs** (orphan support file ⇒ C ⇒ deferred while the gate reports FRESH). Evidence: spec §Part 1 tier B row.
- **[P2, 75] "Stale kills leave the score" defeats the advisory ramp**: the reduced score can trip an ordinary score blocker that blocks, consumes `--max-attempts` and escalates, even at task-done. Evidence: spec §Part 2 vs Decisions 3.
- **[P2, 50] A residual is labelled "documented, accepted" without user acceptance**, while the gate certifies those kills FRESH (contradicts Goal 3). Evidence: spec §Known residuals, §Decisions.
- **[P2, 50] The cold path runs a multi-hour full run on ordinary PRs** after routine cache eviction or on forks. Evidence: spec §Part 1 tier table.
- **[P2, 50] The weekly drift run fires when main moved for irrelevant reasons**, contradicting "no scheduled job that burns a runner when nothing relevant changed". Evidence: spec §Goals, §Workflows.

### Testing-quality

- **[P1, 75] No CI test exercises real StrykerJS output**: e2e is opt-in/network-only and CI installs no Stryker; fake-Stryker and hand-built Go fixtures share the author's schema assumptions. Commit real reports from the e2e as fixtures for both Node and Go tests. Evidence: spec §Verification; `tests/run-all.sh`; `.github/workflows/test.yml`.
- **[P1, 75] The headline "seconds to a couple of minutes" is neither defined nor asserted** (no metric, threshold, or ratio; toy fixture timing says nothing about Keeper scale). Evidence: spec §Goals, §Verification.
- **[P1, 75] Fail-closed "must NOT" behaviours are not listed as tests**: global digests not advancing after deferral (two-run test), orphan support ⇒ C, alias/re-export/dynamic-import support files, UNBOUND never a ledger finding, stale+real blocker still consuming attempts. Evidence: spec §Verification.
- **[P2, 75] The "serialized form unchanged" digest claim has no golden test.** Evidence: spec §Part 2 first bullet.
- **[P2, 50] The Keeper trial has no success criteria** (what "proven locally" means). Evidence: spec §Non-goals, §Decisions 1.

### Completeness

- **[P1, 75] The state file has no CI persistence path**, so every fresh runner sees "no state ⇒ C ⇒ deferred" and tier B is never detected in CI. Evidence: spec §Warm cache, tier table.
- **[P1, 75] Changes to files in no declared class pass silently** (snapshots, env, static mutants, `mutate`-excluded sources, undeclared helpers). Evidence: spec §Problem blind spots vs tier table and §Known residuals.
- **[P2, 75] Deleting/renaming a mutated file yields planner tier `none` while the gate keeps the file's survivors blocking forever.** Evidence: spec §Part 1 tier A, §Part 2 per-kill rules.
- **[P1, 50] Full-run results never reach the `stryker-incr-*` cache, and the two main workflows race on immutable cache keys.** Evidence: spec §Workflows, §Warm cache.
- **[P2, 75] Tier-C deferral and cold-on-PR have no defined observable outcome.** Evidence: spec §Part 1 tier table.
- **[P2, 75] Core acceptance criteria missing**: numeric timing target; retention of untouched files after `--mutate <subset>`; definition of "successful run" (threshold exit, crash, truncated JSON); which file is passed to `--mutation-report`. Evidence: spec §Verification.
- **[P2, 50] UNBOUND has no enforcement rule, so `enforce` is bypassable** by reports lacking sources/test sources/per-test coverage. Evidence: spec §Part 2; `internal/mutation/stryker.go:19-34`.

### Mechanical-precision

- **[P1, 75]** Template CLI unspecified (entry point, commands, flags, exit codes, deferral output). Evidence: spec §Part 1.
- **[P1, 75]** State-file location, schema, digest algorithm and persistence unspecified. Evidence: spec §Part 1.
- **[P1, 75]** Declaration of mutate/test/support/global globs unspecified (file, dialect, `!` negation, root, defaults, overlap precedence). Evidence: spec §Part 1 tier table.
- **[P1, 75]** Test→file mapping (`killedBy` vs `coveredBy` vs union), test→id mapping, and "non-killed" statuses unspecified. Evidence: spec §Part 1 tier A.
- **[P1, 75]** Tier combination/precedence unstated (A+B invocations; whether A/B run when C deferred; deleted files; "new"). Evidence: spec §Part 1.
- **[P1, 50]** Import-graph resolution rules unspecified (forms, extensions, index, `.js`→`.ts`, aliases, workspaces, transitivity). Evidence: spec §Part 1 tier B.
- **[P1, 75]** Gate comparison semantics unspecified (byte-exact vs EOL-normalized; re-rooting suffix; partial-layout handling). Evidence: spec §Part 2.
- **[P1, 75]** Stale finding contract incomplete (severity, class per ramp, title, evidence); survivor fingerprints lack engine so per-(engine, file) dropping cannot be applied; mutant IDs vs `json:"-"`. Evidence: spec §Part 2; `internal/mutation/report.go:151`.
- **[P2, 75]** `METAREVIEW_MUTATION_FRESHNESS` values other than `enforce` undefined. Evidence: spec §Part 2.
- **[P1, 50]** Escalation-exemption predicate ambiguous (prefix vs class; carried-over blockers; counter semantics). Evidence: spec §Part 2.
- **[P2, 50]** Review-log section format undefined. Evidence: spec §Part 2.
- **[P2, 50]** Threshold detection, issue title and dedupe in the post-merge workflow undefined; cache key formats partly undefined. Evidence: spec §Workflows, §Warm cache.

### Architecture

- **[P1, 75] pr-ready verdict reuse bypasses `enforce`**: the reviewer-input digest (`internal/prready/review.go:125-137,186-207,358-369`) covers none of the enforce flag, embedded sources or freshness classification, so a PASS_ADVISORY recorded without enforce is reused unchanged after enforce is set; the spec's "serialized form unchanged" intent is backwards. Evidence: cited lines; spec §Part 2.
- **[P1, 75] The gate certifies less than it claims**: tier B/C invalidations live only in the harness state file, which the gate never reads, so kills the planner knows are pending re-validation classify FRESH; residuals omit support/global inputs. Evidence: spec §Goals, §Part 2, §Known residuals.
- **[P1, 50] UNBOUND kills are still accepted under enforce**, violating "only kills need proof" for schema reports that omit proof (distinct from gremlins, which cannot embed sources). Evidence: spec §Part 2.
- **[P2, 75] State/cache coupling undefined**: state not cached ⇒ permanent deferral; full run does not refresh the incr cache ⇒ tier C permanently pending. Evidence: spec §Warm cache, §Workflows.

### Security

- **[P1, 75] Overlapping-report precedence lets a partial fresh report erase real survivors**: a narrower honest run (line-range `--mutate`, fewer mutators) embeds the full current source, so the stale report's survivors elsewhere in the file are dropped without being killed. Evidence: spec §Part 2 "Overlapping reports".
- **[P1, 75] No containment rule for report keys / `projectRoot` re-rooting**: `../` keys, absolute keys after re-rooting, or symlinks let a kill bind to an old out-of-repo copy (FRESH while the reviewed file changed) and let the gate read arbitrary/hanging files; compare the pin guard at `internal/mutation/verify.go:122-124`. Evidence: spec §Part 2 "Paths".
- **[P2, 75] The gate labels kills certified while blind to support/global inputs** (e.g. a PR editing `vitest.config.ts` to exclude tests stays FRESH). Evidence: spec §Goals, §Part 2.
- **[P2, 50] `--accept-current-state` silently disables blind-spot detection with no audit trail**, usable by an agent to skip a long run. Evidence: spec §Warm cache.
- **[P2, 50] CI cache/artifact trust boundary unspecified** (save only on push to main; no PR code in cache-saving jobs under `pull_request_target`/`workflow_run`; artifact fallback restricted to own main push runs). Evidence: spec §Warm cache, §Workflows.

### Runtime-reliability

- **[P1, 75] "Successful run" is undefined though it gates state refresh**: Stryker exits 1 both for threshold failure (report written) and failed dry run (no report) (`~/Developer/thread-keeper-mvp/bin/check-keeper-quality.mjs:2323-2343`); misclassification either masks tier B permanently or re-forces it forever; no atomic writes; truncated incremental file behaviour undefined. Evidence: spec §Part 1.
- **[P1, 75] The two main-push workflows race on immutable cache keys**, so the full run's refreshed state never reaches the `stryker-incr-` lineage and tier C stays pending forever; no `concurrency:` group. Evidence: spec §Workflows, §Warm cache.
- **[P1, 50] State file not persisted with the cache**, so CI plans default to "no state ⇒ C ⇒ deferred" and skip support detection silently. Evidence: spec §Warm cache, tier table.
- **[P2, 75] Deferred tier C has no machine-readable record**, so gate-visible evidence is FRESH while a full run is pending. Evidence: spec §Part 1, §Part 2.

### Data-migration

- **[P1, 75] Flipping `METAREVIEW_MUTATION_FRESHNESS=enforce` reuses a prior PASS**: `reusableVerdict` (`internal/prready/review.go:186-206`) matches on a digest built from `reviewerCtx` + `GateEffect` (`:358-368`), which excludes enforce mode and embedded sources; precedent for doing it right: serialized `RequireLenses` (`internal/reviewers/prready.go:25`). Evidence: cited lines; spec §Part 2.
- **[P2, 50] Re-rooting vs `Mutant.File` is unspecified and both readings break persisted state**: rewriting `File` changes every survivor/uncovered/unresolved fingerprint (`internal/mutation/report.go:151,182,207`), orphaning ledger records and overrides; not rewriting leaves the stale fingerprint's path form machine-dependent (`report.go:265-270`). Evidence: `internal/mutation/stryker.go:50`; spec §Part 2 "Paths".
- **[P2, 50] Harness state file has no version, corrupt-file rule, or rule for newly declared inputs**, so these fall into silent deferral. Evidence: spec §Part 1.
- **[P2, 50] No migration contract from Keeper's state**: `seed` takes one report vs Keeper's 18 overlapping reports; waiver ledger (`~/Developer/thread-keeper-mvp/bin/check-keeper-quality.mjs:747-843`) has no mapping to metareview overrides; no exit condition for `DEFERRED_MUTATION_UNTIL_0_13_0`. Evidence: cited lines; spec §Part 1 warm cache.

