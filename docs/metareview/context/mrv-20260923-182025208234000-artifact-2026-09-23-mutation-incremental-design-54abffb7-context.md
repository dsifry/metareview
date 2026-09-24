# metareview context: docs/superpowers/specs/2026-09-23-mutation-incremental-design.md

Run ID: `mrv-20260923-182025208234000-artifact-2026-09-23-mutation-incremental-design-54abffb7`

## Target

- Path: `docs/superpowers/specs/2026-09-23-mutation-incremental-design.md`
- Repository mode: `metaswarm-extension`
- Git branch: `mutation-incremental`
- Git head: `a696774`

## Artifact Excerpt

```markdown
# Change-driven mutation testing + gate-side evidence freshness — design

Status: DRAFT — pending adversarial review. The user decisions recorded at the end are approved;
the spec as a whole is not.
Origin: `docs/0.13.0-candidates.md` §1. The gate-side design (Part 2) descends from three rounds
of isolated adversarial review (all FAIL; round-3 fixes folded in, not re-reviewed). Part 1's
content-based planner, tiers, warm cache and workflows are new and unreviewed.

## Problem

On the Keeper MVP epic (TypeScript, StrykerJS 10, Vitest 4, real PostgreSQL; 15,660 mutants,
1,135 tests, 56 mutated files) every one-line edit forced a full ~2.5–3.5 h mutation regeneration.
Two causes, both in the project harness rather than metareview:

1. Keeper's harness binds each report to whole-tree manifests, so any edit invalidates every report.
2. It runs `stryker run --mutate <scope>` from scratch and deletes the previous report first, so no
   result is ever reused.

StrykerJS already ships the fix for (2): `--incremental` reuses per-mutant results (diffing each
file's embedded source against the current file with diff-match-patch) and re-runs only mutants
whose code or covering tests changed. Its documented blind spots are changes to files that are
neither mutated nor test files (helpers, fixtures, dependencies, config, env, snapshots) and static
mutants. The dry run is mandatory on every invocation; the Vitest runner's default `related: true`
limits it to tests related to the mutated files.

metareview itself only ingests reports (`--mutation-report`) and today accepts a stale report
silently.

## Goals

- Per-change mutation cost of seconds to a few minutes, not hours.
- `--incremental`'s blind spots are closed by *what changed*, never silently.
- Reused results are certified at the gate, so reuse is trustworthy evidence.
- No scheduled job that burns a runner when nothing relevant changed.

## Non-goals

- Changing any mutation engine. Owning or versioning a mutation report format (the
  mutation-testing-report-schema stays metareview's only input contract).
- Pin (`prove`) verification speed. The `prove` non-Go pin bug is a separate ticket.
- Touching the Keeper (`../thread*`) repositories. They are tested only after the template is
  proven locally here AND the user says their work has settled.

## Part 1 — Harness template (`templates/mutation-incremental/`)

A zero-dependency Node (>=18) tool a project copies into its repo. The repository owns what runs
("metareview owns the translation, the repository owns which engine runs and against what").

### Change classification (content-based, no commit anchor)

The planner compares the current checkout against the **incremental report itself** (which embeds
every mutated file's source and every test file's source) plus a small harness-owned state file
of digests for inputs the report cannot see. It therefore works with uncommitted changes and with a
cache produced at any commit.

| Tier | Detected by | Action |
|---|---|---|
| none | nothing below | no Stryker invocation |
| A | mutated source differs from its embedded source; new mutate-able file; test file differs/deleted (→ files whose mutants those tests cover); new test file (→ every file with a non-killed mutant) | `stryker run --incremental --mutate <files>` |
| B | a declared test-support file's digest changed (helpers/factories/fixtures) | find importing tests (static relative-import graph), then the files their tests cover: `--incremental --force --mutate <files>`. A changed support file nothing imports ⇒ escalate to C (fail closed). |
| C | a declared global input's digest changed (lockfile, Stryker/Vitest/tsconfig, migrations); or no state file | full forced run. **Incremental mode defers it** (user decision: post-merge on main, not on PRs); full mode runs it. |
| cold | no incremental report | full run |

State digests for test-support are refreshed after any successful run; global digests only after a
successfu
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
