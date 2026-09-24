# metareview context: docs/superpowers/specs/2026-09-23-mutation-incremental-design.md

Run ID: `mrv-20260923-193343742411000-artifact-2026-09-23-mutation-incremental-design-54abffb7`

## Target

- Path: `docs/superpowers/specs/2026-09-23-mutation-incremental-design.md`
- Repository mode: `metaswarm-extension`
- Git branch: `mutation-incremental`
- Git head: `267f716`

## Artifact Excerpt

```markdown
# Change-driven mutation testing + attested evidence freshness — design

Status: r7 — pending re-review. Review history: full artifact reviews r1
`mrv-20260923-182025208234000-…`, r2 `mrv-20260923-183942601569000-…`, r3
`mrv-20260923-185110177502000-…`; targeted review r4 `mrv-20260923-190310858720000-…`; verification
of r5; full pragmatic review r6 `mrv-20260923-192628041933000-…` (3 PASS, 7 NEEDS_REVISION on six
converging issues, all fixed here). The user decisions are in §10. Origin:
`docs/0.13.0-candidates.md` §1.

Review stance for everything that follows (user instruction): handle real workflows and real edge
cases; do not engineer for rare races or synthetic cases; assume developers, CI and agents act in
good faith.

## 1. Problem and engine facts

On the Keeper MVP epic (TypeScript, StrykerJS 10.0.0, Vitest 4.1, real PostgreSQL; 15,660 mutants,
1,135 tests) every one-line edit forced a ~2.5–3.5 h mutation regeneration: the project harness
binds reports to whole-tree manifests and re-runs Stryker from scratch without `--incremental`.
Scoping each run to the change (rather than running `--incremental` over the whole `mutate` set)
matters because the dry run executes every test related to the mutated files (F4); on an
integration-heavy suite that is most of the suite. The harness restores the guarantees that scoping
gives up (F2).

Facts verified in StrykerJS 10.0.0 (`@stryker-mutator/core/dist/src`), its instrumenter, Vitest
4.1.11, GitHub Actions and git (the r5 verification re-ran F4, F5, F9, F10 on a real project):

- F1. `--incremental` reuses per-mutant results: a Killed mutant is reused when its killing test is
  unchanged; a non-killed one when no covering test changed. `--force` re-runs every in-scope
  mutant. Mutants of files that no longer exist are dropped.
- F2. Out-of-scope mutants of files that still exist are carried forward without re-evaluation,
  regardless of `--force`; a changed test drops out of their `killedBy`; mutants in changed regions
  of out-of-scope files are removed.
- F3. The report embeds each file's source as Stryker first read it in that run.
- F4. The dry run always happens. The Vitest runner (`related: true` default) runs only tests whose
  transformed dependency graph reaches a mutated file (type-only imports elided); zero tests ⇒
  `No tests were executed` on stdout and stderr, exit 1, nothing written (`--allowEmpty` does not
  help: it returns before the report is written). The Vitest runner always uses per-test coverage,
  so `killedBy`/`coveredBy` are populated; it reports no test locations, so any edit to a test file
  invalidates all of that file's tests in Stryker's own differ.
- F5. `--mutate` accepts `<file>` and `<file>:<startLine>-<endLine>` (1-based), comma-separated,
  combinable with `--force`, and replaces the config's `mutate`. Stryker's matcher is minimatch with
  `dot: false`.
- F6. The incremental file (`--incrementalFile`) is read and written in place. On
  SIGINT/SIGTERM/SIGHUP/SIGABRT Stryker writes partial results and exits 128+n. Thrown errors exit 1
  without writing. A threshold break exits 1 after writing, computed over the whole report.
- F7. Mutant ids are unique only within one report. The report has no timestamp.
- F8. Without a positional config file Stryker searches its own list of names.
- F9. The report's `files` contains only files that produced at least one mutant; a selected
  zero-mutant file (type-only, barrel, constants) is absent, and a run whose selection has no mutants
  but reachable tests succeeds with `files: {}`.
- F10. `actions/cache` includes the exact `path` list in each entry's version: a restore only hits
  entries saved with the identical list.

metareview today ingests `--mutation-report` files and accepts a stale report silently.

## 2. Goals

- G1 — Per-change cost bounded by the change: one related-tests dry run plus the mutants the change
  can affect. Proven locally by deterministic counts (§7); the e
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
