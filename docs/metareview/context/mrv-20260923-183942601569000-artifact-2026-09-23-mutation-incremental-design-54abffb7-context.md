# metareview context: docs/superpowers/specs/2026-09-23-mutation-incremental-design.md

Run ID: `mrv-20260923-183942601569000-artifact-2026-09-23-mutation-incremental-design-54abffb7`

## Target

- Path: `docs/superpowers/specs/2026-09-23-mutation-incremental-design.md`
- Repository mode: `metaswarm-extension`
- Git branch: `mutation-incremental`
- Git head: `a696774`

## Artifact Excerpt

```markdown
# Change-driven mutation testing + attested evidence freshness — design

Status: DRAFT r2 — pending re-review. Revises r1 after artifact review
`mrv-20260923-182025208234000-artifact-2026-09-23-mutation-incremental-design-54abffb7`
(NEEDS_REVISION, 10/10 lenses). The user decisions at the end are approved; the spec is not.
Origin: `docs/0.13.0-candidates.md` §1.

## 1. Problem

On the Keeper MVP epic (TypeScript, StrykerJS 10.0.0, Vitest 4.1, real PostgreSQL; 15,660 mutants,
1,135 tests, 56 mutated files) every one-line edit forced a ~2.5–3.5 h mutation regeneration,
because the project harness binds reports to whole-tree manifests and re-runs Stryker from scratch
(it deletes the previous report and never passes `--incremental`).

StrykerJS's `--incremental` reuses per-mutant results and re-runs only mutants whose code or
covering tests changed. Facts this design relies on (verified in StrykerJS 10.0.0 source,
`@stryker-mutator/core/dist/src`):

- The dry run always happens; the Vitest runner's `related: true` (default) limits it to tests
  related to the files being mutated (`3-dry-run-executor.js`, `relatedFiles: options.files`).
- `--incremental --force --mutate F` re-runs every mutant in F and keeps out-of-scope results.
- `--mutate` accepts line ranges `<file>:<startLine>[:<col>]-<endLine>[:<col>]`, lines 1-based
  (`fs/project-reader.js`, `MUTATION_RANGE_REGEX`). The list is comma-separated.
- **Out-of-scope mutants are carried forward without re-evaluation** (`mutants/incremental-differ.js`):
  a changed test silently drops out of their `killedBy`, and mutants in changed regions of
  out-of-scope files are removed (survivors included). The written report embeds the **current
  on-disk source** of every file (`reporters/mutation-test-report-helper.js`, `readOriginal()`).
  Therefore embedded source can never prove freshness, and a run is only trustworthy if every file
  that changed since the previous run was in its scope. This design makes the harness guarantee
  that and attest to it; the gate verifies the attestation.
- On an interrupted run Stryker writes partial results into the incremental file.
- Documented blind spots: changes to files that are neither mutated nor tests (helpers, fixtures,
  dependencies, config, env, snapshots) and static mutants.

metareview today ingests `--mutation-report` files and accepts a stale report silently.

## 2. Goals

- G1 — Per-change cost bounded by the change: an incremental run costs one related-tests dry run plus
  the mutants the change can affect. Measured by §6.9; external targets are set by a separate trial.
- G2 — Deterministic coverage of every change: each changed file is either re-verified in the same
  run or recorded as a visible **pending full run**. Nothing is skipped silently; nothing depends on
  a clock.
- G3 — The gate states exactly what it verified: kills are checked against a harness attestation
  and the content under review. It is an accident-level check (like override `--by`), not tamper
  proof.
- G4 — No scheduled job. A full run happens only when some change was deferred.

## 3. Non-goals

- Changing any mutation engine or editing Stryker's report format.
- Pin (`prove`) verification speed; the `prove` non-Go pin bug is a separate ticket.
- Keeper adoption and migration (its 18 reports, waiver ledger, `DEFERRED_MUTATION_UNTIL_0_13_0`
  exit): a separate follow-up (user decision 7). Nothing here touches `../thread*` repositories.
- Checkpoint/resume of long runs (candidates §7). A full run that is interrupted is re-run.
- Tamper resistance against an actor who edits both report and attestation.

## 4. Terms

- **Kill**: a mutant whose metareview status is `killed` (Stryker `Killed`). Stryker `Timeout` is
  already `unresolved` in metareview (`internal/mutation/stryker.go`) and is never a kill.
- **Categories** (from config globs, §5.2): `mutate`, `test`, `support`, `global`, `ignore`; a
  tracked or untracked non-ignored file that matc
```

## Service Inventory

No service inventory found.

## Knowledge Facts

No Beads knowledge facts found.

## Suggested Reviewers

- Feasibility
- Completeness
- Scope and alignment
- Architecture
- Intent preservation
- Security
- Testing-quality
- Data-migration
- Runtime-reliability
- Mechanical-precision
