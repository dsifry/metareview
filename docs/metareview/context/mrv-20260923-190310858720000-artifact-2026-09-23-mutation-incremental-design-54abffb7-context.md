# metareview context: docs/superpowers/specs/2026-09-23-mutation-incremental-design.md

Run ID: `mrv-20260923-190310858720000-artifact-2026-09-23-mutation-incremental-design-54abffb7`

## Target

- Path: `docs/superpowers/specs/2026-09-23-mutation-incremental-design.md`
- Repository mode: `metaswarm-extension`
- Git branch: `mutation-incremental`
- Git head: `a696774`

## Artifact Excerpt

```markdown
# Change-driven mutation testing + attested evidence freshness — design

Status: DRAFT r4 — pending targeted re-review. Revises r3 after artifact review
`mrv-20260923-185110177502000-artifact-2026-09-23-mutation-incremental-design-54abffb7`
(NEEDS_REVISION: 9/10 lenses; scope and alignment PASS). Earlier rounds: `mrv-20260923-182025208234000-…`
(r1), `mrv-20260923-183942601569000-…` (r2). Per the user's escalation decision, r4 is re-reviewed
only by the lenses that failed r3, scoped to the changed sections. The user decisions in §10 are
approved; the spec is not. Origin: `docs/0.13.0-candidates.md` §1.

## 1. Problem and engine facts

On the Keeper MVP epic (TypeScript, StrykerJS 10.0.0, Vitest 4.1, real PostgreSQL; 15,660 mutants,
1,135 tests) every one-line edit forced a ~2.5–3.5 h mutation regeneration: the project harness
binds reports to whole-tree manifests and re-runs Stryker from scratch without `--incremental`.
Scoping each run to the change (rather than running `--incremental` over the whole `mutate` set)
matters because the dry run executes every test related to the mutated files (F4); on an
integration-heavy suite that is most of the suite. The harness restores the guarantees that scoping
gives up (F2).

Facts verified in StrykerJS 10.0.0 (`@stryker-mutator/core/dist/src`), its instrumenter, and Vitest
4.1.11:

- F1. `--incremental` reuses per-mutant results: a Killed mutant is reused when its killing test is
  unchanged; a non-killed one when no covering test changed. Within scope and without `--force`,
  kills in unchanged regions of a changed file are therefore reused, not re-run. `--force` re-runs
  every in-scope mutant. Mutants of files that no longer exist are dropped.
- F2. Out-of-scope mutants of files that still exist are carried forward **without
  re-evaluation, regardless of `--force`** (`mutants/incremental-differ.js`); a changed test drops
  out of their `killedBy`; mutants in changed regions of out-of-scope files are removed.
- F3. The report embeds each file's source as Stryker first read it in that run.
- F4. The dry run always happens. The Vitest runner's `related: true` (default) runs only tests
  whose **transformed** dependency graph reaches a mutated file (type-only imports are elided); zero
  tests ⇒ `ConfigError('No tests were executed…')`, exit 1, nothing written. The Vitest runner
  always uses per-test coverage, so `killedBy`/`coveredBy` are populated.
- F5. `--mutate` accepts `<file>` and `<file>:<startLine>-<endLine>` (1-based), comma-separated, and
  replaces the config's `mutate`. Stryker's matcher is minimatch with `dot: false`.
- F6. The incremental file (`--incrementalFile`) is both read and written in place. On
  SIGINT/SIGTERM/SIGHUP/SIGABRT Stryker writes partial results and exits 128+n; SIGKILL writes
  nothing. Thrown errors exit 1 without writing. A threshold break exits 1 after writing, computed
  over the whole report. npm's `npx` forwards SIGINT/SIGTERM but not SIGKILL.
- F7. Mutant ids are unique only within one report (`incremental-differ.js`: carried-forward ids are
  re-keyed; instrumenter ids come from a per-run counter). The report has no timestamp.
- F8. Without a positional config file Stryker searches its own list of names (`.conf` before
  `.config`).

metareview today ingests `--mutation-report` files and accepts a stale report silently.

## 2. Goals

- G1 — Per-change cost bounded by the change: one related-tests dry run plus the mutants the change
  can affect. Proven locally by deterministic counts (§7); the external trial's pass bar is a
  one-line edit ≤ 2 min p50 (decision 9), tracked as a follow-up entry in `docs/0.13.0-candidates.md`.
- G2 — Every change is re-verified in the same run or recorded as **deferred**, visibly, and kills
  that depend on deferred work are never reported as verified. Planning is clock-free; the only
  time-dependent outcome is the configured per-invocation time budget, which always defers.
  Exception, d
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
