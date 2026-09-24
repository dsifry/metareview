# metareview context: docs/superpowers/specs/2026-09-23-mutation-incremental-design.md

Run ID: `mrv-20260923-185110177502000-artifact-2026-09-23-mutation-incremental-design-54abffb7`

## Target

- Path: `docs/superpowers/specs/2026-09-23-mutation-incremental-design.md`
- Repository mode: `metaswarm-extension`
- Git branch: `mutation-incremental`
- Git head: `a696774`

## Artifact Excerpt

```markdown
# Change-driven mutation testing + attested evidence freshness — design

Status: DRAFT r3 — pending re-review. Revises r2 after artifact review
`mrv-20260923-183942601569000-artifact-2026-09-23-mutation-incremental-design-54abffb7`
(NEEDS_REVISION, 10/10 lenses; r1 was `mrv-20260923-182025208234000-…`). The user decisions in §10
are approved; the spec is not. Origin: `docs/0.13.0-candidates.md` §1.

## 1. Problem and engine facts

On the Keeper MVP epic (TypeScript, StrykerJS 10.0.0, Vitest 4.1, real PostgreSQL; 15,660 mutants,
1,135 tests) every one-line edit forced a ~2.5–3.5 h mutation regeneration: the project harness
binds reports to whole-tree manifests and re-runs Stryker from scratch without `--incremental`.

Facts this design relies on, verified in StrykerJS 10.0.0 (`@stryker-mutator/core/dist/src`) and
the Vitest runner:

- F1. `--incremental` reuses per-mutant results. `--incremental --force --mutate F` re-runs every
  mutant in F and carries forward out-of-scope mutants. Mutants of files that no longer exist are
  dropped.
- F2. Out-of-scope mutants are carried forward **without re-evaluation** (`incremental-differ.js`);
  a changed test silently drops out of their `killedBy`; mutants in changed regions of out-of-scope
  files are removed. A run is trustworthy only if every file changed since the previous run was in
  its scope. The harness guarantees this and attests to it.
- F3. The report embeds each file's source as Stryker **first read it in that run**
  (`fs/project-file.js`, `readOriginal()` caches).
- F4. The dry run always happens. With the Vitest runner, `related: true` (default) limits it to
  tests related to the mutated files (`3-dry-run-executor.js`); if that yields zero tests Stryker
  throws `No tests were executed` and writes nothing. The Vitest runner always uses per-test
  coverage, so `coveredBy`/`killedBy` are populated.
- F5. `--mutate` accepts `<file>:<startLine>-<endLine>` (1-based; `fs/project-reader.js`
  `MUTATION_RANGE_REGEX`), comma-separated; CLI `--mutate` replaces the config's `mutate`.
- F6. The incremental file is written in place (`fs.writeFile`). On SIGINT/SIGTERM/SIGHUP/SIGABRT
  Stryker writes partial results to it and exits 128+n; SIGKILL writes nothing. A "no tests"
  dry run and other thrown errors exit 1 without writing. A threshold break exits 1 after writing,
  computed over the whole report including carried-forward mutants.
- F7. The report has no timestamp; identical inputs can produce identical bytes.

metareview today ingests `--mutation-report` files and accepts a stale report silently.

## 2. Goals

- G1 — Per-change cost bounded by the change: an incremental run costs one related-tests dry run
  plus the mutants the change can affect. Locally proven by deterministic counts (§7). The external
  trial's pass bar is a one-line edit ≤ 2 min p50 (decision 9), tracked as a follow-up entry in
  `docs/0.13.0-candidates.md`.
- G2 — Every change is either re-verified in the same run or recorded as **deferred**, visibly, and
  kills depending on deferred work are never reported as verified. Nothing depends on a clock.
- G3 — The gate states exactly what it verified. It is an accident-level check (like override
  `--by`), not tamper proof.
- G4 — No scheduled job. A full run happens only when something was deferred.

## 3. Non-goals

- Changing any mutation engine or editing Stryker's report format (the harness removes entries of
  deleted files only — §5.6 step 7).
- Test runners other than StrykerJS's Vitest runner in the template (validated in §5.2).
- Pin (`prove`) verification speed; the `prove` non-Go pin bug is a separate ticket.
- Keeper adoption and migration: a separate follow-up (decision 7).
- Checkpoint/resume of long runs (candidates §7).
- Requiring mutation evidence: a gate given no `--mutation-report` behaves exactly as today.
- Tamper resistance against an actor who edits both report and attestation.

## 4. Terms

- **Kill**: a muta
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
