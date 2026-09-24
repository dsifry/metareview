# metareview context: docs/superpowers/specs/2026-09-23-mutation-incremental-design.md

Run ID: `mrv-20260923-205431264136000-artifact-2026-09-23-mutation-incremental-design-54abffb7`

## Target

- Path: `docs/superpowers/specs/2026-09-23-mutation-incremental-design.md`
- Repository mode: `metaswarm-extension`
- Git branch: `mutation-incremental`
- Git head: `1f28c12`

## Artifact Excerpt

```markdown
# Change-driven mutation testing + attested evidence freshness — design

Status: r18 — verification pending (§11 amendment from the adoption contract, revised after reviews r15 `mrv-20260923-204202233770000-…` and r16 `mrv-20260923-204712580916000-…` and r17 `mrv-20260923-205053924374000-…`; blocking only on major or critical findings). r14 was approved by final review `mrv-20260923-201040808900000-…` (10/10 PASS). Review history: full artifact reviews r1
`mrv-20260923-182025208234000-…`, r2 `mrv-20260923-183942601569000-…`, r3
`mrv-20260923-185110177502000-…`; targeted review r4 `mrv-20260923-190310858720000-…`; verification
of r5; full pragmatic reviews r6 `mrv-20260923-192628041933000-…`, r7
`mrv-20260923-193343742411000-…`, r8 `mrv-20260923-194341093013000-…` and r9
`mrv-20260923-195021334650000-…` (3 PASS, 7 NEEDS_REVISION converging on the relative-threshold
baseline, the full job's skip rule and `fetch-state` credentials) and r10
`mrv-20260923-195626283409000-…` (8 PASS, 2 NEEDS_REVISION on one gate blocker, fixed here; the
user then chose the absolute threshold for PRs, decision 15) and r11
`mrv-20260923-200028555622000-…` (9 PASS, 1 NEEDS_REVISION on the PR summary step's cold-run
case) and r12 `mrv-20260923-200340521908000-…` (9 PASS, 1 NEEDS_REVISION on testing which exit
code fails the `full` job) and r13 `mrv-20260923-200638249049000-…` (8 PASS, 2 NEEDS_REVISION
converging on one ledger blocker: a stale-only escalation could not be lifted once its rows were
superseded; fixed here). The user decisions are in §10. Origin:
`docs/0.13.0-candidates.md` §1.

Review stance (user instruction): handle real workflows and real edge cases; do not engineer for
rare races or synthetic cases; assume developers, CI and agents act in good faith. Validity of
mutation evidence is decided only by what changed (content digests), never by dates.

## 1. Problem and engine facts

On the Keeper MVP epic (TypeScript, StrykerJS 10.0.0, Vitest 4.1, real PostgreSQL; 15,660 mutants,
1,135 tests) every one-line edit forced a ~2.5–3.5 h mutation regeneration: the project harness
binds reports to whole-tree manifests and re-runs Stryker from scratch without `--incremental`.
Scoping each run to the change (rather than running `--incremental` over the whole `mutate` set)
matters because the dry run executes every test related to the mutated files (F4); on an
integration-heavy suite that is most of the suite. The harness restores the guarantees that scoping
gives up (F1, F2).

Facts verified in StrykerJS 10.0.0 (`@stryker-mutator/core/dist/src`), its instrumenter, Vitest
4.1.11, GitHub Actions and git (reviewers reproduced F4, F5, F9 and the §7.1 rows on a real project):

- F1. `--incremental` reuses per-mutant results: a Killed mutant is reused when its killing test is
  unchanged; a non-killed one when no covering test changed. Without `--force`, kills in unchanged
  regions of an edited file are reused, not re-run — a behaviour-changing edit elsewhere in the same
  file can invalidate them. `--force` re-runs every in-scope mutant. Mutants of files that no longer
  exist are dropped.
- F2. Out-of-scope mutants of files that still exist are carried forward without re-evaluation,
  regardless of `--force`; a changed test drops out of their `killedBy`; mutants in changed regions
  of out-of-scope files are removed.
- F3. The report embeds each file's source as Stryker first read it in that run.
- F4. The dry run always happens. The Vitest runner (`related: true` default) runs only tests whose
  transformed dependency graph reaches a mutated file (type-only imports elided); zero tests ⇒
  `No tests were executed` on stdout and stderr, exit 1, nothing written (`--allowEmpty` returns
  before the report is written). The Vitest runner always uses per-test coverage, so
  `killedBy`/`coveredBy` are populated; it reports no test locations, so any edit to a test file
  invalidates all of that file's tests in Stry
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
