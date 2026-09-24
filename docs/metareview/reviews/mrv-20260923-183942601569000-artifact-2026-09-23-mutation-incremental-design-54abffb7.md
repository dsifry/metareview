# metareview: artifact review

Run ID: `mrv-20260923-183942601569000-artifact-2026-09-23-mutation-incremental-design-54abffb7`

Target: `docs/superpowers/specs/2026-09-23-mutation-incremental-design.md`

Context pack: `docs/metareview/context/mrv-20260923-183942601569000-artifact-2026-09-23-mutation-incremental-design-54abffb7-context.md`

Execution mode: `parallel-subagents`

Previous run: `mrv-20260923-182025208234000-artifact-2026-09-23-mutation-incremental-design-54abffb7`

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
| Feasibility | NEEDS_REVISION | 4 | 0 | scope with no related tests ⇒ Stryker ConfigError ⇒ permanent exit 4; `full` job skipped on incremental failure/threshold exit (Keeper break=100); full-job stop check reads wrong cache; bytes-differ rejects identical legit writes |
| Scope and alignment | NEEDS_REVISION | 2 | 0 | enforce blocks all non-Stryker (gremlins) reports; headline timing goal has no absolute criterion (G1 cross-ref wrong) |
| Completeness | NEEDS_REVISION | 7 | 0 | harness crash leaves trusted partial baseline; gate blind to post-run new files; cold attestation marks unverified kills verified; zero-mutant scope ⇒ permanent exit 4; CI exit wiring; untracked/symlink vs HEAD; no CI→gate/local warm path, enforce with no report |
| Intent preservation | NEEDS_REVISION | 4 | 0 | deferred kills labelled verified; residual off invisible at gate; Decision 3 vs §6.8 bound; latency target made relative without acceptance |
| Data-migration | NEEDS_REVISION | 2 | 0 | §6.7 golden-unchanged digest vs always-serialized Mode/Freshness contradict; gate ignores attestation schemaVersion/tool (fails open on v2) |
| Testing-quality | NEEDS_REVISION | 6 | 0 | no incremental-vs-fresh equivalence test; self-referential e2e oracles; noisy timing/reuse thresholds; stale fixtures undetected; missing negatives + no e2e gate run; Node coverage threshold unenforceable as written |
| Mechanical-precision | NEEDS_REVISION | 9 | 0 | §6.7 Mode vs golden digest; `unclassified` string + unknown category/version handling; kill-class precedence; scope/forced overlap + budget denominator; residual.mode reach; prune without invocation + bytes-differ; runtime map spec; glob corners; path-char rejection scope |
| Security | NEEDS_REVISION | 2 | 0 | fork-readable main cache may carry secrets/failure output (runtime.env hashes, statusReason); accept-current-state has no finding (self-granted override) |
| Runtime-reliability | NEEDS_REVISION | 4 | 0 | harness-kill leftovers trusted + backup destroyed; bytes-differ fails identical legit runs (pending-full never clears); CI timeouts/needs deadlock; self-written outputs make pending-full permanent |
| Architecture | NEEDS_REVISION | 7 | 0 | no-op run converts deferred kills to verified; mode-dependent classification vs Reconcile keep-old; §6.7 golden vs enforce-flip; cache last-writer-wins drops full results; forced ranges off after edits; mid-run edit sentinel; coveredBy needs perTest |

## Orchestrator Notes (not findings)

Orchestrator context and synthesis go here (e.g. checkout sparse, filtered file-not-found artifacts, consolidation narrative). This section is audit trail only — it is NOT a finding stream. Do not extract sentences from here as review findings; only the `## Findings` section and its classified `## Blocking Findings`, `## Advisory Findings`, `## Follow-up Findings`, and `## Warnings` sections contain review findings.

- Round 2 (re-review of spec r2, `--previous-run` the r1 run). All 10 required lenses ran as independent parallel subagents (read-only); all returned NEEDS_REVISION.
- r1 blockers confirmed resolved by lenses: Keeper cause (1) (now out of scope by user decision 7), deferral invisible (pendingFull attested + finding), partial-report survivor erasure, path containment, CI trust boundary, most mechanical-precision gaps.
- Cross-lens convergence (audit only): deferred/accepted kills still classified "verified" (intent, architecture, completeness, security); harness-kill leftovers + backup overwrite (completeness, runtime, data-migration); bytes-differ success predicate (feasibility, runtime, mechanical); workflow `needs`/exit-code wiring and cache last-writer-wins (feasibility, runtime, architecture, completeness); §6.7 golden digest contradiction (data-migration, architecture, mechanical); self-written outputs in snapshot (runtime, architecture); coveredBy requires perTest (architecture, feasibility); Decision 3 vs §6.8 and G1 target (scope, intent).
- Advisory consolidation and staff filter deferred again: blocking findings alone decide NEEDS_REVISION and the spec will be revised to r3.

## Findings

## Blocking Findings

### Feasibility

- **[P1, 75] A scope whose files have no related tests fails permanently**: the dry run passes only mutated files as `relatedFiles` (`@stryker-mutator/core` `3-dry-run-executor.js:84-95`; Vitest `related` default true), so zero tests ⇒ `ConfigError('No tests were executed…')` (`:46`) with no incremental write; `--allowEmpty` returns before `reportAll` (`4-mutation-test-executor.js:49`). The harness exits 4 forever and never sets pending full. Evidence: cited source; spec §5.4 step 1, §5.6 step 4.
- **[P2, 75] The `full` job never runs when `incremental` fails**: `if:` without a status function implies `success()`; Stryker's threshold exit 1 covers the whole carried-forward report (`determineExitCode`), and Keeper sets `thresholds.break: 100`, so one survivor anywhere blocks every full run. Evidence: spec §5.7; `~/Developer/thread-keeper-mvp/stryker.config.json`.
- **[P2, 75] The full job's stop check reads the newest cache, not its own commit's state**, so a second global change is skipped after an earlier full run cleared the flag. Evidence: spec §5.7.
- **[P2, 50] "Bytes differ" rejects legitimate identical writes** (the report has no timestamp/performance block). Evidence: spec §5.6 step 4.

### Scope and alignment

- **[P1, 75] `enforce` makes every gremlins (non-Stryker) report a permanent blocker.** §6.1 classes all gremlins reports unattested and §6.5 blocks unattested reports in enforce at pr-ready/epic-ready, while only the Stryker template can attest; with 0.14.0 default enforcement (Decision 3) any Go repo — metareview included — can never pass with a mutation report attached, contrary to `internal/reviewers/mutation.go`'s stated principles. Evidence: spec §6.1, §6.5, §9.3; `internal/mutation/gremlins.go`.
- **[P2, 50] The user's headline goal has no matching acceptance criterion**: G1 cites §6.9 (the learning filter) instead of §5.9; §5.9 asserts only "≤ 25% of the full run", which at Keeper scale accepts ~45 min; the external trial has no named home. Evidence: spec §2 G1, §5.9, §3.

### Completeness

- **[P1, 75] A harness crash leaves a partial report the next run trusts**: §5.4 never checks the prior report against `A.reportSha256`, and §5.6 step 2 overwrites a leftover `.prev` (SIGKILL/power loss skip cleanup), destroying the good copy and binding partial carried-forward kills into a new attestation. Evidence: spec §5.4, §5.6, §5.9 (crash row kills Stryker only).
- **[P1, 75] The gate is blind to files added after the run** (new global/support/unclassified files are not in the attestation, and the gate has no category/ignore knowledge), so kills stay verified. Evidence: spec §6.3.
- **[P1, 75] A cold incremental run writes an attestation that makes an unverified report's kills "verified"** (only an advisory pending-full line signals it). Evidence: spec §5.4 cold, §5.6 step 7, §6.6.
- **[P2, 75] A changed zero-mutant file (e.g. type-only) in scope leaves report bytes unchanged ⇒ exit 4 forever**, and `needs:` then skips the full job. Evidence: spec §5.4 step 1, §5.6 step 4, §7.
- **[P2, 75] Exit-1 (threshold) and exit-4 handling in the workflow is unspecified**, so a failed `incremental` step skips `full`. Evidence: spec §5.7.
- **[P2, 50] Untracked files and committed symlinks are attested but read as absent/unverifiable at pr-ready (HEAD)**, staling every kill. Evidence: spec §5.4 snapshot, §6.2, §6.3.
- **[P2, 50] No path from CI state to the gate or to a developer's local cache; enforce with no report supplied raises nothing.** Evidence: spec §5.7, §6.5.

### Intent preservation

- **[P1, 75] Kills covered by a deferral are classified `verified` and scored**, because step 7 rewrites `attestation.files` to the new snapshot; contradicts "a deferral can never be read as evidence" (candidates §1) and G3. Needs a **deferred** kill class driven by per-reason affected paths in the attestation. Evidence: spec §5.5, §5.6 step 7, §6.3, §6.5, §6.6.
- **[P1, 75] `residual.mode: off` skips are recorded only in the attestation and never surfaced by the gate**, contradicting G2 and the user's "practical considerations in the config" (explicit, not invisible). Evidence: spec §5.4 step 4, §6.
- **[P1, 50] Decision 3 ("never escalate") contradicts §6.8 (escalate at 2 × maxAttempts).** Evidence: spec §9.3, §6.8.
- **[P1, 50] The user's absolute latency target was replaced by a relative one without recorded acceptance.** Evidence: spec §2 G1, §5.9, §5.7 (60 min job timeout).

### Data-migration

- **[P2, 75] §6.7 contradicts itself**: always-serialized `Mode`/`Freshness` on `MutationContext` (hashed via `PRReadyContext.Mutation`, `internal/prready/review.go:139`) changes every stored `ReviewInputDigest`, so the promised golden "serializes exactly as before" cannot pass; pick `omitempty` semantics or state one-time reuse invalidation. Evidence: spec §6.7; `internal/reviewers/prready.go`.
- **[P2, 75] The gate never checks attestation `schemaVersion`/`tool`**, so a future v2 attestation is read with v1 semantics and kills are marked verified (fail open); the harness already treats a wrong version as none. Evidence: spec §6.1 vs §5.4.

### Testing-quality

- **[P1, 75] The core premise (incremental result ≡ fresh full run, up to listed residuals) is never tested**; every §5.9 row checks what the planner chose, not whether it sufficed. Evidence: spec §5.9, §1.
- **[P1, 75] E2E oracles restate the planner's rules and cannot catch planner bugs**; they must be literal expected paths/ranges, and "no Stryker process" needs an invocation-counting shim. Evidence: spec §5.9 rows 3, 5–7.
- **[P1, 75] Timing/reuse thresholds are noisy and don't measure G1** (fixed costs dominate a small fixture; reuse % parses version-specific log text; "one invocation" conflicts with §5.4 step 4 forcing). Use deterministic counts and specify the fixture layout. Evidence: spec §2 G1, §5.9 row 4.
- **[P1, 75] Committed real-report fixtures and local proof can silently go stale** — no evidence manifest tying them to template/lockfile/config hashes and tree states, no CI check. Evidence: spec §5.9, §8.
- **[P1, 75] Missing negatives**: pending full persists until full mode; attestation for another report / hash mismatch ⇒ unattested; enforce mode per gate; Timeout never a kill; residual/unclassified scenarios per `residual.mode`; no end-to-end gate run on the e2e tree. Evidence: spec §5.5, §5.9, §6.1, §6.4, §8.
- **[P2, 75] Node 100% coverage is unenforceable as written** (threshold flags need Node ≥ 22.8; unloaded modules are uncounted without an include glob). Evidence: spec §8.

### Mechanical-precision

- **[P2, 75]** §6.7: serialized `Mode` breaks the golden-unchanged digest unless `omitempty` with advisory as zero; `Freshness` shape/JSON names undefined. Evidence: spec §6.7; `internal/reviewers/mutation.go:20`.
- **[P2, 75]** The `unclassified` category string, gate handling of unknown category values, and wrong/unparseable attestation (`schemaVersion`, `tool`) are undefined. Evidence: spec §5.5, §6.1, §6.3.
- **[P2, 75]** Kill-class precedence (stale vs unbound) and the class of a kill whose report key fails containment are undefined. Evidence: spec §6.2, §6.3.
- **[P2, 75]** Scope/forced overlap (dropped? counted in budget? second invocation?) and the `|R.mutants|` denominator are undefined; §5.9's "one invocation" depends on it. Evidence: spec §5.4.
- **[P2, 50]** Which steps `residual.mode` governs, and the shape/lifetime of `residual.accepted`, are ambiguous. Evidence: spec §5.4 steps 3–4, §5.5.
- **[P2, 75]** Prune on a no-invocation run, `reportSha256` after prune, and "bytes differ" misclassifying a legitimately identical report are undefined. Evidence: spec §5.6 steps 4–7.
- **[P2, 75]** `runtime` map key format, hashed content, unset vs empty env, change-reason text, and end-of-run re-hash are undefined. Evidence: spec §5.2, §5.5.
- **[P2, 75]** Glob corner cases (trailing `**`, `?`/`{}` vs `/`, `./`, dotfiles, case) and what "not `ignore`" means under precedence are undefined. Evidence: spec §5.2, §5.4.
- **[P2, 50]** Scope of the special-character path rejection (whole repo vs scoped/forced; `plan` too) is undefined. Evidence: spec §5.4.

### Security

- **[P2, 75] The main-branch cache is readable by every PR including forks, and the spec does not constrain its contents**: reports carry `statusReason` failure output and full sources; attestations carry unsalted hashes of `runtime.env` values (e.g. `DATABASE_URL`); committed real fixtures may carry host paths. Requires: state that the state dir is public to PRs, no secrets in mutation jobs, reject secret-bearing `runtime.env` names, scrub committed fixtures. Evidence: spec §5.5, §5.7, §5.9.
- **[P2, 50] `seed --accept-current-state` is a self-granted freshness override with no finding**, so under enforce an old report passes cleanly and nothing reaches the ledger or learning (conflicts with CLAUDE.md "Process Overrides" requester≠granter and G2). Requires an advisory `mutation:accepted-state:<engine>` finding and learning-filter entry. Evidence: spec §5.8, §6.5, §6.6.

### Runtime-reliability

- **[P0, 75] Leftovers from a killed harness are trusted and destroy the backup**: Stryker writes `incremental.json` in place with plain `fs.writeFile` and writes `partialResults` on SIGINT/SIGTERM (`@stryker-mutator/core` `unexpected-exit-handler.js`, `mutation-test-report-helper.js:40-48`); §5.4 accepts any parseable R without checking `A.reportSha256`, §5.6 step 2 overwrites the good `.prev`, and unreported out-of-scope survivors vanish (false clean). Evidence: cited engine source; spec §5.4, §5.6.
- **[P1, 75] "Bytes differ" fails legitimate runs**: full-mode argv is constant and the report has no timestamp, so a result-neutral global change yields identical bytes ⇒ exit 4 forever ⇒ pending full never clears and main stays red. Evidence: spec §5.6 step 4.
- **[P1, 75] CI timeouts deadlock**: a 25%-of-mutants budget (~3,900 mutants on Keeper) exceeds the 60 min job; a timed-out job saves no cache so the next push grows; `full` is skipped when `incremental` fails/times out/exits 1 unless `if:` uses `!cancelled()`. Evidence: spec §5.4 budget, §5.7.
- **[P1, 50] Self-written outputs (`stateDir`, Stryker `reports/`, `.stryker-tmp/`) enter the snapshot as unclassified changes**, making pending full permanent. Evidence: spec §5.4 snapshot, §5.5.

### Architecture

- **[P1, 75] A harness run that re-verified nothing converts deferred kills to "verified"**, so the same edit blocks (pre-run §6.3 stale) or passes (post-run, advisory pending-full) depending on a no-op run; needs a `pending` kill class driven by recorded deferred paths. Evidence: spec §5.5, §5.4 steps 4–5, §5.8, §6.3, §6.5.
- **[P1, 75] Stale classification is mode-dependent but the fingerprint is not, and `Reconcile` keeps the existing row's classification** (`internal/findings/findings.go:152-164`), so an enforce flip never blocks an already-recorded advisory stale file (and vice versa). Evidence: cited lines; spec §6.5.
- **[P1, 75] §6.7's golden-unchanged requirement contradicts digest change on enforce flip**, reintroducing reuse via `reusableVerdict` (`internal/prready/review.go:186-207`); the golden protects nothing since `ReviewerImplementation: version.Version` is already in the digest (`:358-369`). Evidence: cited lines; spec §6.7.
- **[P1, 75] CI cache lineage is last-writer-wins**: an incremental job started before a full run finishes saves a newer key with sticky pending-full, discarding the full run's results; frequent pushes ⇒ pending never clears. Evidence: spec §5.5, §5.7.
- **[P2, 75] Forced ranges from R's line numbers target the wrong code when the same file is also in scope (edited)**; those mutants are reused by the scope run and never re-checked. Evidence: spec §5.4, §5.6 step 3.
- **[P2, 50] The mid-run-edit rule is unsound** (StrykerJS caches `readOriginal()` at first read, `fs/project-file.js:46-58`; revert-during-run yields a false match; files new-and-edited during a cold run are unattested yet credited). Evidence: cited source; spec §1, §5.5.
- **[P2, 50] Planner steps 2 and 4 need `coveredBy`, which exists only under `coverageAnalysis: "perTest"`**; without it residual forcing silently selects nothing. Evidence: spec §5.4.
