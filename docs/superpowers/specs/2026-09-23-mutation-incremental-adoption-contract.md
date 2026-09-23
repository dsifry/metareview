# Mutation-incremental: Keeper-shaped adoption contract

Status: draft for review. Companion to the locked design
`docs/superpowers/specs/2026-09-23-mutation-incremental-design.md` (r14, approved). This document
states what the template must support for a real, demanding consumer, so its interface can be
checked before Plan 1 freezes it. Items marked **spec change** become a single r15 amendment to the
design once this contract is accepted. The Keeper migration steps themselves are a separate,
later spec (§6).

## 1. Consumer profile (Keeper, as reported by its maintainers)

- 15,660 mutants, 1,135 tests, integration tests against real PostgreSQL; full sweep 2.5–3.5 h.
- 16 verification units with per-unit mutate patterns (`MUTATION_TEST_TREE_PATHS`,
  `keeper-test-strategy.json`); 18 current reports at different scopes.
- **Any test can kill any unit's mutants.** Stryker runs with no test filter (Vitest
  `related: true`); e.g. a WU08 file's kills come from 7 test files, 1 of them WU08's own, 2 added
  by a later unit. Unit `files[]` is ownership, not test selection.
- Bar: zero surviving valid mutants, enforced through a reviewed waiver ledger (2,871 entries:
  `proven_equivalent`, `statically_tested`, `unexecuted_static`) and
  `regenerate-keeper-mutation-waivers.mjs`. Evidence is the accept gate.
- `ignoreStatic: true`; `disableBail` not set. Most-edited files are large (`App.tsx`, `main.ts`).
- Unit ownership overlaps: 5 source files belong to more than one unit (`apps/web/src/App.tsx`,
  `apps/server/src/main.ts`, `apps/worker/src/main.ts` to four units each;
  `packages/domain/src/invitations.ts` to WU04+WU13; `apps/web/src/features/story.tsx` to
  WU06+WU16). Besides 16 per-unit pattern sets there are 3 per-milestone full scopes
  (`full_wu06/11/16`; 38 files for wu11). Membership's source of truth is
  `keeper-test-strategy.json`.

## 2. Requirements and proposed template support

**K1. A survivor bar other than a score (answers A1, A3, B6).** With waivers kept outside
Stryker, waived survivors count as `Survived` and `thresholds.break: 100` can never pass.
*Proposal (spec change):* optional config `verify: {"command": [argv]}`, run after every commit of
state with `MUTATION_REPORT=<stateDir>/incremental.json` and `MUTATION_ATTESTATION=…` in its
environment; a non-zero exit is reported like a threshold break (exit 1, state still committed).
The waiver ledger stays project-owned and authoritative. Recommended waiver identity:
`(file, mutatorName, location, replacement)` plus the file's digest, so a waiver on an edited file
is stale by the same rule as a stale kill (mutant ids are report-local and unusable). The design
states explicitly: **CI enforces the project's bar; the metareview gate judges freshness.** A
gate-side survivor bar is out of scope for 0.13.0.

**K2. Pending on PRs (answers A3).** Decision 2 makes a pending PR green. For an
evidence-as-accept-gate project that lets unverified mutants merge.
`mutation:pending` stays advisory even under enforce, so this is the only mechanism that stops
unverified mutants merging. *Decided (user, 2026-09-23; spec change):* config
`pendingOnPr: "allow" | "full" | "full-on-global"` (default `allow`, decision 2 unchanged for
everyone else):
- `full`: a PR run that ends pending runs `run --mode full` in the PR job.
- `full-on-global`: pending caused by a `global` or runtime input (the whole tree must be
  re-established anyway) runs the full sweep; pending from any other cause (budget, time budget,
  support with no importer, no reachable tests) fails the PR (exit 1) with the deferral causes in
  the step summary. This is Keeper's setting: the common hot-file case should not cost 3.5 h.
- Consequence, documented: with `full` or `full-on-global` the PR job's timeout must fit a full
  sweep (for Keeper ~240–350 min instead of 60).
- **Open (needs design before r15):** under `full-on-global` a failed PR "requires a local run", but
  local state never reaches the PR job (PR state lives only in its own cache; `publish-state` is
  CI-only), so a local run cannot turn the PR green. Candidate resolutions: (a) the PR job re-runs
  the deferred selection with no budget or time ceiling (bounded by the job timeout; for a hot-file
  deferral minutes to under an hour, not 3.5 h); (b) a sanctioned way to carry a local run's state
  to the PR job (e.g. committing `<stateDir>` files to the PR branch, verified by digest). (a) needs
  no new trust path and is recommended.

**K3. One state, per-unit views (answers A2).** Scopes share one test corpus and one import graph,
so per-scope state adds no accuracy and costs 16 dry runs per change (what Keeper pays today).
*Decided (user, 2026-09-23; spec change):* one harness state whose `mutate` is the union of all
views; views are named pattern sets over that union. Requirements:
1. **Views overlap; they are not a partition.** One mutant may belong to several views. Per-view
   counts are computed per view and never summed across views.
2. **Views have any granularity.** Arbitrary named pattern sets (per unit, per milestone, …).
3. **Completeness, exit 2 on violation:** every view pattern matches only files inside the
   `mutate` union, and every file with mutants belongs to at least one view, so no mutant is
   unowned.
4. **Single source of truth.** Views come from a project-owned command,
   `views: {"command": [argv]}`, printing `{"<name>": [patterns]}` (Keeper: derived from
   `keeper-test-strategy.json`); never duplicated in the harness config. View membership is
   presentation only: changing it invalidates no kill and is not a `global` input.
5. **The verifier gets the view.** `verify.command` (K1) runs once per view with
   `MUTATION_VIEW=<name>` and the view's patterns, so per-unit evidence is produced by the
   project's verifier over the one report.
After each commit the attestation carries per-view summaries (kill classes, survivors), and the gate
accepts `--mutation-view <name>` to scope its classification and findings to that view.

**K4. Static code with `ignoreStatic: true` (answers A4).** The freshness guarantee holds (code
never mutated has no kills to go stale), but the §8 gap widens: a changed module-scope constant in
a file that also has mutants can flip kills elsewhere unseen. *Proposal (spec change):* when a
changed hunk of a `mutate` file lies outside every mutant's location range, add the file's importer
tests to `T_Y` (the rule already used for zero-mutant files); the gate mirrors it. `statically_tested`
and direct pins map onto metareview anchor pins once R3 (prove cannot verify non-Go pins) is fixed.

**K5. Edited large files (answers C8, C9).** Decision 12 (edited files forced whole-file) makes a
one-line edit to a 1,000-mutant file cost ~10–15 min, and past `maxMinutesPerInvocation` it goes
pending, so a hot file could put most merges back on a full run. Whole-file was a simplicity
choice, not required by F1. *Decided (user, 2026-09-23; spec change, revises decision 12):* config
`editedFiles: "residual" | "whole"`, **default `"residual"`**; `whole` keeps r14's behaviour. The
r15 amendment names each of these conditions explicitly, because each is a place the closure could
leak:
1. Mutants inside changed hunks are mandatory and budget-exempt; mutants forced by the residual
   closure are budget-subject (budget overflow defers them as today).
2. A changed hunk containing no mutant falls back to the file's importer tests (K4).
3. `disableBail: true` is a hard prerequisite of `residual` mode (K6's `allowBail` is refused when
   `editedFiles` is `residual`), because the closure reads `killedBy`.
4. Hunks are computed from content: the file's attested source (the `source` embedded in the
   report's `files` entry) against the current bytes, never from commits or `git diff`. A file with
   no attested source (new, or no mutants) is handled as today.
5. The closure is by coverage, not proximity: force = changed-hunk mutants ∪ every mutant (any file)
   killed by any test covering a changed-hunk mutant.
6. The viability measurement (§3) reports both modes and which one Keeper would run.
Stated plainly: `residual` widens the documented §8 residual compared with `whole` (behaviour
changes reaching same-file kills through paths no covering test exercises). It is acceptable only
because the same class is already accepted for cross-file kills and is backstopped by full runs.

**K6. `disableBail` validation (answers D13).** Without it `killedBy` depends on test order
(F4): soundness mostly holds (an unchanged killing test still kills) but plans are not
reproducible. *Proposal (spec change):* config validation fails (exit 2) unless the Stryker config
has `disableBail: true` or the harness config sets `allowBail: true`; `allowBail` is refused in
`editedFiles: "residual"` mode (K5.3). Keeper supports requiring it. The cost of `disableBail` on an
integration suite is measured in the trial.

**K7. Meta-artifacts (answers D11).** Guidance, no spec change: files tests import or read →
`support`; files only tooling reads (catalog/decomposition JSON, docs) → `ignore`; `global` only for
inputs everything depends on (lockfile, Stryker/Vitest config, `.nvmrc`); `tests/**` → `test`,
helpers/factories → `support`. Files read by path at runtime are invisible to the import graph;
list them in `support` with an explicit importer or accept their `["*"]` deferral.

**K8. Upgrades without a full run (answers D10).** *Proposal (spec change):* `stateVersion`,
bumped only when planning or attestation semantics change; `toolVersion` stays informational.
Patch releases keep state.

**K9. Cutover (answers B5).** Documented sequence: stay advisory; adopt the harness with one
state (K3); first main push runs the full sweep and publishes; until then existing reports show
`unattested` (advisory). Enforce only after `mutation-state/full` exists. `seed` is optional
(seeded kills stay pending until that full run); with K3 it takes one merged report
(`seed --from` repeatable, merging disjoint files).

## 3. Viability gate (answers C7)

Before any migration work, measure on Keeper without running Stryker: replay `plan` (side-effect
free) over the last N merged commits against Keeper's existing reports, once `../thread` has
settled. Record per commit: forced and scoped mutant counts under whole-file (decision 12) and
in-file residual (K5), budget/time deferrals, and predicted pending share. Estimate minutes from
the full sweep's per-mutant rate (≈ 0.7 s). Proceed with adoption only if the projected p50 for
single-file edits is ≤ 2 min and the projected pending share is **≤ 20%** (≤ 10% comfortable),
measured with `residual` mode. Derivation (user): with a full sweep on pending PRs, added latency ≈
P × 2.5–3.5 h; at P ≥ 30% one batched sweep per unit merge is competitive and far more
predictable. Measurement conditions: a settled `../thread`, plan only (no Stryker); the last N
merged commits, never synthetic edits; `residual` and `whole` side by side; deferral **causes**
recorded (global, hot-file budget/time, support without importer), not just counts. Above 20% even
with `residual`: do not adopt; keep the batched close-out and revisit with the §7 trial's real
numbers. Fallback levers, in order: `residual`; a larger `maxMinutesPerInvocation`; narrowing
`mutate` for generated/boilerplate files; `pendingOnPr: "full-on-global"`.

## 4. Decisions

Decided 2026-09-23: K2 (`allow|full|full-on-global`), K3 (one state + views, five
requirements), K5 (`editedFiles`, default `residual`, six conditions), §3 (≤ 20%).
K2 gap settled (user, 2026-09-23): option (a), option (b) rejected (no local state imported into CI).
Under `full`/`full-on-global` the PR run is unbudgeted (scoped to the deferred closure, not whole
files), bounded by the job timeout and failing with a named reason when it does not fit, and still
attested; only this run's deferrals count, inherited ones belong to main's full run. K3 additions:
the attestation records the resolved view map; views are read-time only; `verify` reads the whole
report per view, with per-mutant waivers. All of this is specified in the design's §11 (r15).

## 5. Unchanged by this contract

Change-based validity, state branches, the full job and catch-up, the absolute threshold, the gate's
kill classes and ledger lifecycle.

## 6. Ownership

The Keeper adoption spec (migration steps, waiver-ledger bridge script, unit views mapping) is
written after the template passes its local proof and the §3 viability gate, within 0.13.0, and is
tracked as its own bead.

Note (design r16): whole-tree deferrals (`["*"]`: `no usable state`, a deleted or importer-less
support file, an importer-less unclassified file) are routed to the full sweep rather than failing
the PR under `full-on-global`, because a PR cannot clear them itself; only per-file causes fail it.
