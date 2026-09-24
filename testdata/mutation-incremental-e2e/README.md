# mutation-incremental e2e fixture

A small TypeScript project for the harness's local proof with real StrykerJS 10.0.0 and Vitest 4.1.11
(spec §7.1, §11.7). `tests/e2e-mutation-incremental.mjs` copies it to `.e2e/<run>/`, adds
`tools/mutation-incremental/`, points `stryker.command` at a recording shim, runs `npm ci`, and
commits it. Never run Stryker here in `testdata/`.

## Files

| File | Purpose |
|---|---|
| `package.json`, `package-lock.json` | pinned `@stryker-mutator/core` 10.0.0, `@stryker-mutator/vitest-runner` 10.0.0, `vitest` 4.1.11 |
| `stryker.config.json` | Vitest runner, `disableBail: true`, `mutate` equal to the harness's, `.mutation` ignored |
| `vitest.config.ts` | the `@app/` alias (`resolve.alias`), tests under `tests/` |
| `mutation-incremental.json` | harness config: alias `@app/` → `src/`, `maxForcedShare: 1`, `pendingOnPr: full-on-global`, views `core` and `edge` (overlapping) |
| `.gitignore` | `node_modules/`, `.mutation/`, `reports/`, `.stryker-tmp/` |
| `src/a.ts` | `grade(score, pass, top)`: block body lines 1–5, statements on lines 2–4; no equivalent mutants |
| `src/b.ts` | imports `grade` and `import type { Range }`; expression-bodied arrow on line 4 |
| `src/c.ts` | `LIMIT` import on line 1; `twice` on line 2, tested only by `tests/c.test.ts` |
| `src/limits.ts` | `LIMIT = 100` (no mutants) |
| `src/types.ts` | `interface Range` (no mutants) |
| `src/index.ts` | barrel `export * from './a'` (no mutants) |
| `src/e.ts` | `inc` (lines 1–4) and `dec` (lines 6–9), tested separately |
| `tests/a.test.ts` | `grade` at and around both boundaries (imports `node:assert`, `vitest`) |
| `tests/b.test.ts` | `inRange` (imports `vitest` and `./helpers/make`) |
| `tests/c.test.ts` | `twice` via `@app/c` and `./helpers/fmt` |
| `tests/index.test.ts` | `grade` through the barrel |
| `tests/e-inc.test.ts`, `tests/e-dec.test.ts` | `inc`, `dec` |
| `tests/helpers/make.ts`, `tests/helpers/fmt.ts` | support files |

A full run has 24 mutants, all killed (score 100): 12 in `src/a.ts`, 5 in `src/b.ts`, 3 in
`src/c.ts`, 4 in `src/e.ts` (a `BlockStatement` for each function and an `ArithmeticOperator` on
lines 2 and 7).

## Scenario edits (e2e rows)

| Row | Edit |
|---|---|
| `a-preserving` | `src/a.ts` line 3 → `  if (top < score) return 'over';` |
| `a-survivor` | `src/a.ts` line 3 → `  if (score >= top + 1) return 'over';` (the new `score > top + 1` mutant survives) |
| `test-b` | a case `inRange(70, range(50, 90))` added to `tests/b.test.ts` |
| `limits` | `src/limits.ts` → `export const LIMIT = 10 * 10;` |
| `helper-make` | `tests/helpers/make.ts` → `({ hi, lo })` |
| `types-only` | `src/types.ts` → `…; hi: number; }` |
| `delete-c-test` | delete `tests/c.test.ts` |
| `e-residual` | `src/e.ts` line 2 → `  const r = 1 + n;` |
| `e-flip` | `src/e.ts` line 3 (inside `inc`) → `  return 2;` |
| `lockfile` | a trailing newline appended to `package-lock.json` |
| `threshold` | `thresholds.break: 100` in `stryker.config.json`; the `at top is ok` test removed |

**Negative control `e-flip`.** Changing line 3 of `src/e.ts` from `return r;` to `return 2;` makes
the `ArithmeticOperator` mutant on line 2 (`n + 1` → `n - 1`) survive: `inc` now returns 2 whatever
`r` is, so `tests/e-inc.test.ts` no longer kills it. With the residual closure the harness re-runs
that mutant and reports it `Survived`. With the closure disabled (`MUTATION_TEST_DISABLE_RESIDUAL=1`)
Stryker reuses its old `Killed` result, because the mutant's own text and its killing test did not
change, so equivalence with a fresh run fails for exactly that mutant.
