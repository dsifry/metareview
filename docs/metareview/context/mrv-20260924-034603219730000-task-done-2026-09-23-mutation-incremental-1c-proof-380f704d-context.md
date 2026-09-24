# metareview task-done context

Run ID: `mrv-20260924-034603219730000-task-done-2026-09-23-mutation-incremental-1c-proof-380f704d`

## Task

# Mutation-Incremental Harness — Plan 1c: Workflow, Real-Stryker Proof and Docs

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the harness adoptable and prove it against real StrykerJS. This plan delivers:
- the GitHub workflow template (`incremental-pr`, `pr-full`, `incremental-main`, `full`), with routing computed in tested JavaScript;
- a `summary` command for the step summary;
- a static Go test of the workflow;
- the planner golden-output guard;
- a real TypeScript fixture;
- a local e2e runner with real StrykerJS 10 and Vitest 4. It checks equivalence (every incremental kill is a fresh kill) and includes a negative control;
- the user documentation.

**Architecture:**
- **Routing in JavaScript.** `run` gains a `route` output (`verdict|sweep|fail`) computed by `routeFor(pendingOnPr, cause, pr)`, so the workflow stays a thin router. The step summary is a harness command (`summary`) that reads the canonical state and main's fetched state.
- **Static workflow test.** `mutationworkflow_test.go` in the root package parses the template with `gopkg.in/yaml.v3`. It asserts the §7.2 static properties and executes the two verdict shell steps with fake exit codes.
- **Local e2e.** `tests/e2e-mutation-incremental.mjs` copies the fixture into `.e2e/<run-id>/`, installs its pinned dependencies, and drives the real CLI through a shim that records Stryker's argv. It commits the resulting real reports under `testdata/mutation-incremental/real/` for Plan 2's gate tests.

**Tech Stack:** Node ≥ 22.8, StrykerJS 10.0.0 with `@stryker-mutator/vitest-runner` 10.0.0, Vitest 4.1.11, GitHub Actions YAML, Go 1.26 with `gopkg.in/yaml.v3` (already a dependency).

**Spec:** `docs/superpowers/specs/2026-09-23-mutation-incremental-design.md` (r21). This plan implements:
- §5.7 (the workflow);
- §7.1 (the fixture and a real-Stryker e2e, with the scope below);
- §7.2's static workflow test and step-summary tests;
- §9 (documentation);
- the §11 parts of each: §11.2 routing, jobs and summary; §11.6 golden guard and docs; §11.7 rows.

**Plan series:** 1a (planner, done) → 1b (execution, done) → **1c (this plan)** → 2 (`2026-09-23-mutation-incremental-gate.md`: the Go gate, §6).

## Global Constraints

- Node ≥ 22.8, ESM, **no npm dependencies in the template**. The fixture has its own pinned `devDependencies`, installed only inside `.e2e/`.
- 100% line/branch/function coverage for every non-test `.mjs` in the template. The Go wrapper enforces it. Go stays at 100% per gated package; the new Go test lives in the root package, which the gate excludes.
- The e2e never uses `/tmp`: it runs in `<worktree>/.e2e/<run-id>/` (gitignored). It needs network for `npm ci` and is **not** run in CI.
- Workflow security (§5.7):
  - workflow-level `permissions: {}`;
  - `contents: write` only on `incremental-main` and `full`;
  - every checkout uses `fetch-depth: 0` and `persist-credentials: false`;
  - `MUTATION_STATE_TOKEN: ${{ github.token }}` appears only on `fetch-state`/`publish-state` steps;
  - outputs reach shell steps through `env:`, never through `${{ }}` inside `run:`.
- Never read or run anything in `../thread*` repositories.
- Review stance (user): real workflows and real edge cases; assume trust; no engineering for rare races. Block only for a credible normal-usage failure.
- Each task ends with a commit; messages end with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

## Review Focus

1. **A PR's verdict never depends on runner speed under `full-on-global`.** A timeout routes to `pr-full`; only `unreachable` fails `incremental-pr`. Task 1, test `routeFor follows §11.2`; Task 3, the verdict-step fake-exit table.
2. **The `full` job's verdict reads the right exit code.** It uses the full run's exit code when that step ran, and the catch-up's otherwise; an empty code is red. Task 3, the full-job fake-exit table.
3. **Incremental results equal a fresh run.** Every kill the harness keeps outside a deferral is a kill in a fresh non-incremental run of the same tree. This holds across the edit rows with real Stryker, and the negative control shows the check can fail. Task 6, rows `equivalence` and `e-flip`.
4. **Planner semantics cannot change silently.** Changed golden outputs fail CI until `stateVersion` is bumped. Task 4, test `planner golden outputs are pinned to stateVersion`.
5. **The token never reaches `run` steps or `pr-full`.** Task 3, the static test's token assertions.

## Plan decisions beyond the spec text (flagged for review)

- **`--pr` is accepted under `pendingOnPr: "allow"` and changes nothing there.** Under `allow`, the run is budgeted, `MUTATION_RUN_KIND=other`, there is no shortcut, and `--max-minutes` is ignored. This reverses a Plan 1b decision. The spec says the template passes `--pr` only when `pendingOnPr` ≠ `allow`, which would require the workflow to read the config. With this rule the template always passes it and gets the spec's behaviour for every mode. `--pr` with `--mode full` and `--max-minutes` without `--pr` stay exit 2.
- **Routing is computed by the harness:**
  - `routeFor(pendingOnPr, cause, pr)` → `verdict` (the exit code decides), `sweep` (verdict deferred to `pr-full`) or `fail` (`unreachable` under `full-on-global`);
  - `run` writes it to `GITHUB_OUTPUT` as `route` on exit 0/1;
  - the workflow's shell steps only compare strings.
- **The step summary is a harness command,** `summary --job pr|pr-full|main|full --exit-code <n> [--pr]`. It never fails the job (`|| true`), and it renders reasons and paths as code spans. It shows `mutation-state/full`'s completion time (review advisory). It lists inherited deferrals as "inherited from main, cleared by main's full run".
- **Fixture function `grade(score, pass, top)` instead of `clamp`.** A clamp has equivalent mutants at its boundaries (`n <= lo` returns `lo` = `n`), so a clamp fixture always has survivors. §7.1's rows assume "base fixture has no survivors". The line layout is the same: a block body on lines 1–5, with three statements on lines 2–4.
- **E2E scope:** the rows below run with real Stryker.
  - Rows that need the Go gate (§7.1 "gate") move to Plan 2.
  - Rows that only re-prove planner arithmetic already pinned by Plan 1a's unit tests are replaced by the equivalence check. The equivalence check is stronger: it proves the kills, not the forced list.
  - The workflow job rows are covered by the static test and the fake-exit tables. §9's external-trial checklist confirms them on real GitHub Actions.
- **Real reports are exported, not replayed in CI.** The e2e writes each row's final attestation and report to `testdata/mutation-incremental/real/<row>/`, with `projectRoot` rewritten to `.` and `reportSha256` recomputed, as input for Plan 2's classification tests. §7.2's CI replay and manifest hash become follow-ups: the golden guard (Task 4) already pins planner semantics in CI.
- **Test-only residual switch.** `MUTATION_TEST_DISABLE_RESIDUAL=1` sets `computePlan`'s `disableResidual` in `run` (§11.7 negative control). The docs say it is for the harness's own tests.

---

## File Structure

```
templates/mutation-incremental/
  github-workflow.yml          # NEW: the workflow template (§5.7, §11.2)
  lib/deferrals.mjs            # + routeFor
  lib/run.mjs                  # --pr under allow, `route` output, test-only residual switch
  lib/main.mjs                 # + --job/--exit-code options, `summary` command
  lib/summary.mjs              # NEW: summaryMarkdown, summaryCommand
  test/{run,summary,golden}.test.mjs
testdata/mutation-incremental/planner-golden.json      # NEW: golden plan outputs + stateVersion
testdata/mutation-incremental-e2e/                     # NEW: the fixture project (README lists every file)
testdata/mutation-incremental/real/<row>/              # NEW: real attestation + report per e2e row
mutationworkflow_test.go                               # NEW: static workflow test + fake-exit tables
tests/e2e-mutation-incremental.mjs                     # NEW: local real-Stryker proof
tests/e2e-mutation-incremental.sh                      # NEW: thin wrapper (spec names the script)
docs/mutation-harness.md                               # NEW: user documentation (§9)
USAGE.md, docs/quickstart.md, CHANGELOG.md, docs/ARCHITECTURE.md, docs/0.13.0-candidates.md   # links and notes
```

---

### Task 1: `--pr` under every mode, the `route` output, the test-only residual switch

**Files:**
- Modify: `templates/mutation-incremental/lib/deferrals.mjs` (append `routeFor`), `lib/run.mjs`
- Modify: `templates/mutation-incremental/test/run.test.mjs`, `test/deferrals.test.mjs` (append)

**Interfaces:**
- Produces:
  - `routeFor(pendingOnPr, cause, pr): 'verdict'|'sweep'|'fail'`;
  - `run` writes `route=<…>` to `GITHUB_OUTPUT` on exit 0/1;
  - `run` passes `disableResidual: process.env.MUTATION_TEST_DISABLE_RESIDUAL === '1'` to `computePlan`.

- [ ] **Step 1: Write the failing tests**

Append to `templates/mutation-incremental/test/deferrals.test.mjs`:

```js
import { routeFor } from '../lib/deferrals.mjs';

test('routeFor follows §11.2', () => {
  const rows = [
    // pendingOnPr, cause, pr → route
    ['allow', 'global', true, 'verdict'],
    ['full', 'timeout', false, 'verdict'],
    ['full', 'none', true, 'verdict'],
    ['full', 'unreachable', true, 'sweep'],
    ['full', 'other', true, 'sweep'],
    ['full-on-global', 'global', true, 'sweep'],
    ['full-on-global', 'timeout', true, 'sweep'],
    ['full-on-global', 'unreachable', true, 'fail'],
    ['full-on-global', 'none', true, 'verdict'],
  ];
  for (const [mode, cause, pr, route] of rows) assert.equal(routeFor(mode, cause, pr), route, `${mode} ${cause} ${pr}`);
});
```

In `templates/mutation-incremental/test/run.test.mjs`, make these changes:
- `route` joins every successful output. In `run --mode full starts fresh …` the expected output becomes `{ pending_full: 'false', exit_code: '0', pending_cause: 'none', pending_causes: '[]', route: 'verdict' }`. In `a cold incremental run with no report …` it becomes `{ pending_full: 'true', exit_code: '0', pending_cause: 'global', pending_causes: '[{"reason":"no usable state","paths":["*"]}]', route: 'verdict' }`.
- In `run option errors are exit 2 and still write outputs`, delete the last four lines (the `allow` case).
- Append:

```js
test('under allow, --pr and --max-minutes change nothing; the route is the verdict', async () => {
  const r = runRepo();
  await warm(r);
  r.write('src/a.ts', A3);
  const res = await cli(r, ['run', '--mode', 'incremental', '--pr', '--max-minutes', '0.0001']);
  assert.equal(res.code, 0);
  assert.equal(r.calls().length, 2); // not timed out: --max-minutes is ignored under allow
  assert.equal(res.output.route, 'verdict');
});

test('a routed PR run routes its cause: a timeout to the sweep, an unreachable module to fail', async () => {
  const r = runRepo({ config: ROUTED });
  await warm(r);
  r.write('src/a.ts', A3);
  r.steps([{ sleepMs: 10000 }]);
  const slow = await cli(r, ['run', '--mode', 'incremental', '--pr', '--max-minutes', '0.005']);
  assert.deepEqual([slow.code, slow.output.pending_cause, slow.output.route], [0, 'timeout', 'sweep']);
  r.steps([{ report: reportFor() }]);
  await warm(r);
  rmSync(join(r.top, 'tests/a.test.ts'));
  r.steps([{ exit: 1, output: 'No tests were executed' }]);
  const gone = await cli(r, ['run', '--mode', 'incremental', '--pr']);
  assert.deepEqual([gone.code, gone.output.pending_cause, gone.output.route], [0, 'unreachable', 'fail']);
});

test('the test-only residual switch drops the coverage closure', async () => {
  const r = runRepo();
  await warm(r);
  r.write('src/a.ts', A3);
  process.env.MUTATION_TEST_DISABLE_RESIDUAL = '1';
  try {
    assert.equal((await cli(r, ['run', '--mode', 'incremental'])).code, 0);
  } finally {
    delete process.env.MUTATION_TEST_DISABLE_RESIDUAL;
  }
  // Without the closure only the changed line is forced (the closure adds the line-2 kill: 2-3).
  assert.deepEqual(r.calls()[1], inv(2, '--force', '--mutate', 'src/a.ts:3-3'));
});
```

- [ ] **Step 2: Run to verify failure**

Run: `node --test templates/mutation-incremental/test/run.test.mjs templates/mutation-incremental/test/deferrals.test.mjs`
Expected: FAIL — `routeFor` is not exported; outputs lack `route`; `--pr` under allow is still exit 2; the residual switch is not read.

- [ ] **Step 3: Implement**

Append to `templates/mutation-incremental/lib/deferrals.mjs`:

```js
// Spec §11.2 Routing. Without --pr, or under allow, the exit code alone decides ("verdict"). Under
// full every cause but none routes to the sweep; under full-on-global only unreachable fails the PR,
// so a PR's verdict never depends on runner speed.
export function routeFor(pendingOnPr, cause, pr) {
  if (!pr || pendingOnPr === 'allow' || cause === 'none') return 'verdict';
  return pendingOnPr === 'full-on-global' && cause === 'unreachable' ? 'fail' : 'sweep';
}
```

Edit `templates/mutation-incremental/lib/run.mjs`:

1. Import `routeFor` with the other deferral helpers:

```js
import { classifyDeferrals, dedupeDeferrals, deferralKey, pendingCause, routeFor, sortDeferrals } from './deferrals.mjs';
```

2. In `executeIncremental`, replace the `computePlan` call and the shortcut line with (a PR run is routed only when `pendingOnPr` is not `allow`):

```js
  const plan = computePlan({ config, snapshot, graph, chosen, unbudgeted: ctx.routed, disableResidual: process.env.MUTATION_TEST_DISABLE_RESIDUAL === '1' });
```

```js
  const shortcut = ctx.routed && pendingCause(planned).cause === 'global';
```

3. In `runCommand`, delete the line that throws `--pr is for pendingOnPr …`. Replace the `const ctx = …` line with:

```js
    // Spec §11.2: --pr and --max-minutes take effect only when pendingOnPr routes PRs; under allow a
    // PR run is an ordinary run (budgeted, MUTATION_RUN_KIND=other).
    const routed = options.pr && config.pendingOnPr !== 'allow';
    const ctx = { config, inputs, views, options, interrupt, routed, maxMinutes: routed ? maxMinutes : null, graceMs: io.killGraceMs ?? KILL_GRACE_MS, onOutput: (chunk) => io.stderr.write(chunk) };
```

4. Pass `pr: routed` to `runVerify` (instead of `pr: options.pr`). Extend the success-path `writeOutputs` with the route:

```js
    writeOutputs([['pending_full', pendingFull], ['exit_code', code], ['pending_cause', cause], ['pending_causes', causesJSON(counted)], ['route', routeFor(config.pendingOnPr, cause, options.pr)]]);
```

5. Remove `UsageError` from the `errors.mjs` import only if nothing else in the file uses it. (`checkOptions` still does, so keep it.)

- [ ] **Step 4: Run the suite with coverage**

Run: `sh .superpowers/sdd/2026-09-23-mutation-incremental-1c-proof/cov.sh`
Expected: all pass; no uncovered entries.

- [ ] **Step 5: Commit**

```bash
git add templates/mutation-incremental/lib/deferrals.mjs templates/mutation-incremental/lib/run.mjs templates/mutation-incremental/test/run.test.mjs templates/mutation-incremental/test/deferrals.test.mjs
git commit -m "feat(mutation-incremental): route output, --pr under every mode, test-only residual switch

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: The step summary command

**Files:**
- Create: `templates/mutation-incremental/lib/summary.mjs`, `test/summary.test.mjs`
- Modify: `lib/main.mjs` (`--job`, `--exit-code`, register `summary`), `test/main.test.mjs` (the `parseArgs` expectations gain `job` and `exitCode`)

**Interfaces:**
- Consumes: `loadHarnessConfig`, `summaryLine`, `primaryCandidate`, `readCandidate`, `canonicalPendingFull`, `pendingCause`, `routeFor`, `deferralKey`.
- Produces:
  - `summaryMarkdown({job, exitCode, config, state, remotes, pr}): string`;
  - `summaryCommand(io, options): Promise<0>`, which appends to `$GITHUB_STEP_SUMMARY`, or writes to stdout when that is unset.

**Content (spec §5.7 step 5, §11.2):**
- **Empty exit code:** only "run step did not execute or did not finish (see earlier step)".
- **Otherwise:**
  - the exit code;
  - `no usable state` when the canonical state is not usable;
  - else the pending flag and the score against `thresholds.break`. The score line adds "includes pending kills" when pending, and "previous state (this run committed nothing)" when the exit code is not 0/1;
  - counted deferrals as code spans;
  - inherited ones as "inherited from main, cleared by main's full run".
- **On exit 0/1 of a routed PR:**
  - "verdict deferred to pr-full" for `sweep`;
  - for `fail`, each counted `no reachable tests` with "add a test or remove the module", plus "(also on main)" when main's fetched state carries the identical deferral.
- **For `--job pr`:**
  - main's score from the first usable of `remote/inc` and `remote/full` ("main state unavailable" when neither is), labelled "includes pending kills" when that state has deferrals;
  - `mutation-state/full`'s completion time and last full run.

- [ ] **Step 1: Write the failing tests**

`templates/mutation-incremental/test/summary.test.mjs`:

```js
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, rmSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { summaryMarkdown } from '../lib/summary.mjs';
import { writeState } from './helpers.mjs';
import { cli, reportFor, runRepo, warm } from './fake.mjs';

const config = (extra = {}) => ({ thresholdBreak: null, pendingOnPr: 'allow', ...extra });
const usable = (attestation) => ({ usable: true, attestation: { score: 50, completedAt: 'c', lastFullAt: 'l', deferrals: [], ...attestation } });
const unusable = { usable: false };
const md = (extra) => summaryMarkdown({ job: 'main', exitCode: '0', config: config(), state: usable({}), remotes: [unusable, unusable], pr: false, ...extra });

test('an empty exit code says only that the run step did not finish', () => {
  assert.equal(md({ exitCode: '' }), '### Mutation testing (main)\n\n- run step did not execute or did not finish (see earlier step)\n');
});

test('no usable state, the score, the threshold and the previous-state label', () => {
  assert.match(md({ state: unusable }), /- no usable state\n/);
  assert.match(md({}), /- score: 50\.00 \(no thresholds\.break\)\n/);
  assert.match(md({ config: config({ thresholdBreak: 60 }) }), /- score: 50\.00, below thresholds\.break 60\n/);
  assert.match(md({ config: config({ thresholdBreak: 40 }) }), /- score: 50\.00, at or above thresholds\.break 40\n/);
  assert.match(md({ state: usable({ score: null }) }), /- score: n\/a/);
  assert.match(md({ exitCode: '4' }), /- score: 50\.00 \(no thresholds\.break\) — previous state \(this run committed nothing\)\n/);
});

test('pending kills, counted and inherited deferrals as code spans', () => {
  const deferrals = [
    { reason: 'global input changed: package-lock.json', paths: ['*'], inherited: true },
    { reason: 'time budget exceeded', paths: ['src/a.ts', 'src/`b`.ts'], inherited: false },
  ];
  const text = md({ state: usable({ deferrals }) });
  assert.match(text, /- pending full run: true\n/);
  assert.match(text, /\(includes pending kills\)/);
  assert.match(text, /- deferred: `time budget exceeded` on `src\/a\.ts`, `src\/'b'\.ts`\n/);
  assert.match(text, /- inherited from main, cleared by main's full run: `global input changed: package-lock\.json` on `\*`\n/);
});

test('routed PR runs: the sweep, an unreachable module (and whether main has it)', () => {
  const routed = config({ pendingOnPr: 'full-on-global' });
  const timeout = usable({ deferrals: [{ reason: 'time budget exceeded', paths: ['src/a.ts'], inherited: false }] });
  assert.match(md({ job: 'pr', config: routed, state: timeout, pr: true }), /- verdict deferred to pr-full\n/);
  assert.doesNotMatch(md({ job: 'pr', config: routed, state: timeout, pr: true, exitCode: '4' }), /verdict deferred/);
  const gone = { reason: 'no reachable tests: src/c.ts', paths: ['src/c.ts'], inherited: false };
  const state = usable({ deferrals: [gone] });
  const onMain = usable({ deferrals: [{ reason: 'no reachable tests: src/c.ts', paths: ['src/c.ts'] }] });
  assert.match(md({ job: 'pr', config: routed, state, pr: true }), /- `no reachable tests: src\/c\.ts`: add a test or remove the module\n/);
  assert.match(md({ job: 'pr', config: routed, state, pr: true, remotes: [onMain, unusable] }), /add a test or remove the module \(also on main\)\n/);
});

test("the PR job shows main's score and the age of mutation-state/full", () => {
  assert.match(md({ job: 'pr' }), /- main: main state unavailable\n$/);
  const inc = usable({ score: 90, deferrals: [{ reason: 'no usable state', paths: ['*'] }] });
  const full = usable({ score: 80, completedAt: '2026-09-23T10:00:00.000Z', lastFullAt: '2026-09-22T10:00:00.000Z' });
  const text = md({ job: 'pr', remotes: [inc, full] });
  assert.match(text, /- main \(mutation-state\/inc\): score 90\.00 \(no thresholds\.break\) \(includes pending kills\)\n/);
  assert.match(text, /- mutation-state\/full: completed 2026-09-23T10:00:00\.000Z, last full run 2026-09-22T10:00:00\.000Z\n/);
  assert.match(md({ job: 'pr', remotes: [unusable, full] }), /- main \(mutation-state\/full\): score 80\.00/);
  assert.doesNotMatch(md({ job: 'main', remotes: [inc, full] }), /main \(/);
});

test('summary appends to GITHUB_STEP_SUMMARY, or prints without it; bad --job is exit 2', async () => {
  const r = runRepo();
  await warm(r);
  writeState(join(r.top, '.mutation/remote/full'), { report: reportFor(), attestation: { completedAt: 'c2', lastFullAt: 'l2' } });
  const file = join(r.top, '.fake/summary.md');
  writeFileSync(file, 'before\n');
  const saved = process.env.GITHUB_STEP_SUMMARY; // set on GitHub runners
  try {
    process.env.GITHUB_STEP_SUMMARY = file;
    const res = await cli(r, ['summary', '--job', 'pr', '--exit-code', '0']);
    assert.equal(res.code, 0);
    assert.match(res.stderr, /command=summary invocations=0 scope=0 forced=0 deferrals=0 pending_full=false\n$/);
    const text = readFileSync(file, 'utf8');
    assert.match(text, /^before\n### Mutation testing \(pr\)\n/);
    assert.match(text, /mutation-state\/full: completed c2, last full run l2/);
    delete process.env.GITHUB_STEP_SUMMARY;
    rmSync(join(r.top, '.mutation/attestation.json'));
    const printed = await cli(r, ['summary', '--job', 'main']);
    assert.equal(printed.stdout, '### Mutation testing (main)\n\n- run step did not execute or did not finish (see earlier step)\n');
    assert.equal((await cli(r, ['summary', '--job', 'nope'])).code, 2);
  } finally {
    if (saved === undefined) delete process.env.GITHUB_STEP_SUMMARY;
    else process.env.GITHUB_STEP_SUMMARY = saved;
  }
});
```

In `templates/mutation-incremental/test/main.test.mjs`, the two `parseArgs` expectations gain `job: undefined, exitCode: undefined`. Also add `'--job', 'pr', '--exit-code', '1'` to the second call's arguments, and expect `job: 'pr', exitCode: '1'`.

- [ ] **Step 2: Run to verify failure**

Run: `node --test templates/mutation-incremental/test/summary.test.mjs templates/mutation-incremental/test/main.test.mjs`
Expected: FAIL — `lib/summary.mjs` does not exist; `parseArgs` does not know `--job`/`--exit-code`.

- [ ] **Step 3: Implement**

`templates/mutation-incremental/lib/summary.mjs`:

```js
import { appendFileSync } from 'node:fs';
import { join } from 'node:path';
import { UsageError } from './errors.mjs';
import { loadHarnessConfig, summaryLine } from './inputs.mjs';
import { canonicalPendingFull, primaryCandidate, readCandidate } from './state.mjs';
import { deferralKey, pendingCause, routeFor } from './deferrals.mjs';

const JOBS = ['pr', 'pr-full', 'main', 'full'];

// Reasons and paths are rendered as code spans (review advisory); a backtick cannot close one early.
const code = (text) => `\`${String(text).replaceAll('`', "'")}\``;

function scoreText(score, thresholdBreak) {
  const value = score === null ? 'n/a' : score.toFixed(2);
  if (thresholdBreak === null) return `${value} (no thresholds.break)`;
  return `${value}, ${score !== null && score < thresholdBreak ? 'below' : 'at or above'} thresholds.break ${thresholdBreak}`;
}

const deferralLine = (d) => `${code(d.reason)} on ${d.paths.map(code).join(', ')}`;

function mainLines(remotes, thresholdBreak) {
  const [inc, full] = remotes;
  const main = [inc, full].find((c) => c.usable);
  const lines = main === undefined
    ? ['- main: main state unavailable']
    : [`- main (${main === inc ? 'mutation-state/inc' : 'mutation-state/full'}): score ${scoreText(main.attestation.score, thresholdBreak)}${main.attestation.deferrals.length > 0 ? ' (includes pending kills)' : ''}`];
  if (full.usable) lines.push(`- mutation-state/full: completed ${full.attestation.completedAt}, last full run ${full.attestation.lastFullAt}`);
  return lines;
}

// Spec §5.7 step 5 and §11.2: what the run did, what is pending and why, and (on PRs) main's score.
export function summaryMarkdown({ job, exitCode, config, state, remotes, pr }) {
  const lines = [`### Mutation testing (${job})`, ''];
  if (exitCode === '') return `${[...lines, '- run step did not execute or did not finish (see earlier step)'].join('\n')}\n`;
  lines.push(`- exit code: ${code(exitCode)}`);
  const committed = exitCode === '0' || exitCode === '1';
  if (!state.usable) lines.push('- no usable state');
  else {
    const att = state.attestation;
    const pending = att.deferrals.length > 0;
    lines.push(`- pending full run: ${pending}`);
    lines.push(`- score: ${scoreText(att.score, config.thresholdBreak)}${pending ? ' (includes pending kills)' : ''}${committed ? '' : ' — previous state (this run committed nothing)'}`);
    const counted = att.deferrals.filter((d) => !d.inherited);
    for (const d of counted) lines.push(`- deferred: ${deferralLine(d)}`);
    for (const d of att.deferrals.filter((x) => x.inherited)) lines.push(`- inherited from main, cleared by main's full run: ${deferralLine(d)}`);
    const route = committed ? routeFor(config.pendingOnPr, pendingCause(att.deferrals).cause, pr) : 'verdict';
    if (route === 'sweep') lines.push('- verdict deferred to pr-full');
    if (route === 'fail') {
      const mainKeys = new Set(remotes.filter((c) => c.usable).flatMap((c) => c.attestation.deferrals.map(deferralKey)));
      for (const d of counted.filter((x) => x.reason.startsWith('no reachable tests: '))) {
        lines.push(`- ${code(d.reason)}: add a test or remove the module${mainKeys.has(deferralKey(d)) ? ' (also on main)' : ''}`);
      }
    }
  }
  if (job === 'pr') lines.push(...mainLines(remotes, config.thresholdBreak));
  return `${lines.join('\n')}\n`;
}

// `summary --job pr|pr-full|main|full --exit-code <n> [--pr]`: appends to $GITHUB_STEP_SUMMARY (or
// prints). The workflow runs it with `if: always()` and `|| true`, so it never fails a job.
export async function summaryCommand(io, options) {
  if (!JOBS.includes(options.job)) throw new UsageError('summary needs --job pr|pr-full|main|full');
  const { config } = loadHarnessConfig(io, options, false);
  const text = summaryMarkdown({
    job: options.job,
    exitCode: options.exitCode ?? '',
    config,
    state: primaryCandidate(config),
    remotes: ['inc', 'full'].map((kind) => readCandidate(join(config.stateDir, 'remote', kind), { label: `mutation-state/${kind}`, primary: false })),
    pr: options.pr,
  });
  const file = process.env.GITHUB_STEP_SUMMARY;
  if (file) appendFileSync(file, text);
  else io.stdout.write(text);
  io.stderr.write(summaryLine({ command: 'summary', pendingFull: canonicalPendingFull(config) }));
  return 0;
}
```

Edit `templates/mutation-incremental/lib/main.mjs`:
- Add `'--job': 'job', '--exit-code': 'exitCode'` to `VALUE_OPTIONS`.
- Add `job: undefined, exitCode: undefined` to `parseArgs`'s initial object.
- Add the usage line `  summary --job pr|pr-full|main|full [--exit-code <n>] [--pr]`.
- Add the import `import { summaryCommand } from './summary.mjs';` and the table entry `summary: summaryCommand,`.

- [ ] **Step 4: Run the suite with coverage**

Run: `sh .superpowers/sdd/2026-09-23-mutation-incremental-1c-proof/cov.sh`
Expected: all pass; no uncovered entries.

- [ ] **Step 5: Commit**

```bash
git add templates/mutation-incremental/lib/summary.mjs templates/mutation-incremental/lib/main.mjs templates/mutation-incremental/test/summary.test.mjs templates/mutation-incremental/test/main.test.mjs
git commit -m "feat(mutation-incremental): step summary command

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---
### Task 3: The workflow template and its static test

**Files:**
- Create: `templates/mutation-incremental/github-workflow.yml`
- Create: `mutationworkflow_test.go` (root package `metareview`)

**Interfaces:**
- Consumes: the CLI surface of Tasks 1–2 and Plan 1b. That is `fetch-state`, `run` (with outputs `exit_code`, `pending_full`, `pending_cause`, `route`), `publish-state`, and `summary`.
- Produces: the template projects copy to `.github/workflows/mutation.yml`.

- [ ] **Step 1: Write the failing Go test**

`mutationworkflow_test.go`:

```go
package metareview

import (
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The workflow template's static contract (spec §5.7, §7.2, §11.2) and its two verdict shell steps,
// executed with fake exit codes.

type wfStep struct {
	Name            string            `yaml:"name"`
	ID              string            `yaml:"id"`
	If              string            `yaml:"if"`
	Uses            string            `yaml:"uses"`
	Run             string            `yaml:"run"`
	With            map[string]any    `yaml:"with"`
	Env             map[string]string `yaml:"env"`
	ContinueOnError bool              `yaml:"continue-on-error"`
}

type wfJob struct {
	If          string            `yaml:"if"`
	Needs       string            `yaml:"needs"`
	Permissions map[string]string `yaml:"permissions"`
	Concurrency struct {
		Group            string `yaml:"group"`
		CancelInProgress bool   `yaml:"cancel-in-progress"`
	} `yaml:"concurrency"`
	Outputs map[string]string `yaml:"outputs"`
	Env     map[string]string `yaml:"env"`
	Steps   []wfStep          `yaml:"steps"`
}

type workflow struct {
	On   map[string]any    `yaml:"on"`
	Env  map[string]string `yaml:"env"`
	Jobs map[string]wfJob  `yaml:"jobs"`
}

const workflowPath = "templates/mutation-incremental/github-workflow.yml"

func loadWorkflow(t *testing.T) (workflow, string) {
	t.Helper()
	src, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatal(err)
	}
	var wf workflow
	if err := yaml.Unmarshal(src, &wf); err != nil {
		t.Fatal(err)
	}
	return wf, string(src)
}

func stepIndex(job wfJob, match func(wfStep) bool) int {
	return slices.IndexFunc(job.Steps, match)
}

func named(name string) func(wfStep) bool { return func(s wfStep) bool { return s.Name == name } }

func TestMutationWorkflowStatic(t *testing.T) {
	wf, src := loadWorkflow(t)
	if len(wf.On) != 2 || wf.On["push"] == nil || !mapHas(wf.On, "pull_request") {
		t.Errorf("triggers must be push and pull_request only: %v", wf.On)
	}
	if !strings.Contains(src, "\npermissions: {}\n") {
		t.Error("workflow-level permissions must be {}")
	}
	if strings.Contains(src, "--threshold") {
		t.Error("no --threshold flag anywhere (decision 15)")
	}
	wantPerms := map[string]string{"incremental-pr": "read", "pr-full": "read", "incremental-main": "write", "full": "write"}
	if len(wf.Jobs) != len(wantPerms) {
		t.Fatalf("jobs = %d, want %d", len(wf.Jobs), len(wantPerms))
	}
	if _, ok := wf.Env["MUTATION_STATE_TOKEN"]; ok {
		t.Error("MUTATION_STATE_TOKEN must not be workflow-level")
	}
	var savedKeys []string
	var cachePaths []string
	for name, job := range wf.Jobs {
		if got := job.Permissions["contents"]; got != wantPerms[name] || len(job.Permissions) != 1 {
			t.Errorf("%s: permissions %v, want contents: %s only", name, job.Permissions, wantPerms[name])
		}
		if len(job.Env) != 0 {
			t.Errorf("%s: no job-level env (the token is per step)", name)
		}
		if !strings.HasPrefix(job.Steps[0].Uses, "actions/checkout@") {
			t.Errorf("%s: checkout must be the first step", name)
		}
		install := stepIndex(job, named("install"))
		for i, s := range job.Steps {
			if strings.HasPrefix(s.Uses, "actions/checkout@") && (s.With["fetch-depth"] != 0 || s.With["persist-credentials"] != false) {
				t.Errorf("%s: checkout needs fetch-depth: 0 and persist-credentials: false", name)
			}
			if strings.Contains(s.Run, "${{") {
				t.Errorf("%s/%s: pass outputs through env:, never ${{ }} inside run:", name, s.Name)
			}
			remote := strings.Contains(s.Run, " fetch-state") || strings.Contains(s.Run, " publish-state")
			if _, has := s.Env["MUTATION_STATE_TOKEN"]; has != remote {
				t.Errorf("%s/%s: MUTATION_STATE_TOKEN only (and always) on fetch-state/publish-state steps", name, s.Name)
			}
			if remote && s.ContinueOnError {
				t.Errorf("%s/%s: publish/fetch steps are never continue-on-error", name, s.Name)
			}
			if strings.Contains(s.Run, " fetch-state") && i < install {
				t.Errorf("%s: checkout and project setup precede fetch-state", name)
			}
			if strings.Contains(s.Run, " run --mode") && !s.ContinueOnError {
				t.Errorf("%s/%s: run steps are continue-on-error (a later step decides)", name, s.Name)
			}
			if strings.HasPrefix(s.Run, `node "$MUTATION_CLI" summary`) && s.If != "always()" {
				t.Errorf("%s/%s: the summary is if: always()", name, s.Name)
			}
			if strings.Contains(s.Run, `"$MUTATION_CLI" summary`) && !strings.Contains(s.Run, "|| true") {
				t.Errorf("%s/%s: the summary never fails the job", name, s.Name)
			}
			if strings.HasPrefix(s.Uses, "actions/cache/") {
				cachePaths = append(cachePaths, s.With["path"].(string))
				if strings.HasPrefix(s.Uses, "actions/cache/save@") {
					savedKeys = append(savedKeys, s.With["key"].(string))
					if !strings.Contains(s.If, "hashFiles('.mutation/attestation.json') != ''") {
						t.Errorf("%s/%s: cache saves require the attestation", name, s.Name)
					}
				}
			}
			if strings.Contains(s.Run, " publish-state") {
				save := stepIndex(job, func(x wfStep) bool { return strings.HasPrefix(x.Uses, "actions/cache/save@") && x.If == s.If })
				if save < 0 || save > i {
					t.Errorf("%s/%s: the cache is saved (same condition) before publishing", name, s.Name)
				}
			}
			if strings.HasPrefix(s.Uses, "actions/cache/") && strings.HasPrefix(name, "incremental-pr") && !strings.Contains(s.With["key"].(string), "mutation-pr-${{ github.event.pull_request.number }}-") {
				t.Errorf("%s: N in PR cache keys is github.event.pull_request.number", name)
			}
		}
	}
	for _, p := range cachePaths {
		if p != cachePaths[0] {
			t.Errorf("every cache step uses the identical path list (F10): %q vs %q", p, cachePaths[0])
		}
	}
	slices.Sort(savedKeys)
	if len(slices.Compact(slices.Clone(savedKeys))) != len(savedKeys) {
		t.Errorf("every saved cache key is distinct: %v", savedKeys)
	}

	pr, prFull, main, full := wf.Jobs["incremental-pr"], wf.Jobs["pr-full"], wf.Jobs["incremental-main"], wf.Jobs["full"]
	if !pr.Concurrency.CancelInProgress || main.Concurrency.CancelInProgress || full.Concurrency.CancelInProgress || prFull.Concurrency.CancelInProgress {
		t.Error("only incremental-pr cancels in progress")
	}
	if prFull.Needs != "incremental-pr" || !strings.Contains(prFull.If, "needs.incremental-pr.outputs.route == 'sweep'") || !strings.Contains(prFull.If, "exit_code == '1'") {
		t.Errorf("pr-full needs incremental-pr and runs only on a sweep route with exit 0/1: %q", prFull.If)
	}
	if strings.Contains(stepsText(prFull), "MUTATION_STATE_TOKEN") || strings.Contains(stepsText(prFull), "publish-state") {
		t.Error("pr-full has no token and no publish step")
	}
	if full.Needs != "incremental-main" || !strings.Contains(full.If, "!cancelled()") || !strings.Contains(full.If, "needs.incremental-main.outputs.pending_full == 'true'") {
		t.Errorf("full needs incremental-main, runs when not cancelled and pending: %q", full.If)
	}
	catchup, fullRun := stepIndex(full, named("catch-up")), stepIndex(full, named("full run"))
	if catchup < 0 || fullRun < catchup {
		t.Error("the full job runs the catch-up before run --mode full")
	}
	publishCatchup := full.Steps[stepIndex(full, named("publish catch-up"))]
	if !strings.Contains(publishCatchup.If, "steps.catchup.outputs.pending_full == 'false'") {
		t.Error("the catch-up is published only when it cleared pending")
	}
	if !strings.Contains(stepsText(main), "publish-state --kind inc") || strings.Contains(main.Steps[stepIndex(main, named("summary"))].Run, "--job pr") {
		t.Error("incremental-main publishes inc; its summary omits main's score (--job main)")
	}
	for _, key := range []string{"exit_code", "pending_full"} {
		if main.Outputs[key] == "" {
			t.Errorf("incremental-main exposes %s", key)
		}
	}
	for _, key := range []string{"exit_code", "route"} {
		if pr.Outputs[key] == "" {
			t.Errorf("incremental-pr exposes %s", key)
		}
	}
}

func mapHas(m map[string]any, key string) bool { _, ok := m[key]; return ok }

func stepsText(job wfJob) string {
	out, _ := yaml.Marshal(job.Steps)
	return string(out)
}

func runVerdict(t *testing.T, script string, env map[string]string) int {
	t.Helper()
	cmd := exec.Command("sh", "-c", script)
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	cmd.Stdout, cmd.Stderr = nil, nil
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return exit.ExitCode()
		}
		t.Fatal(err)
	}
	return 0
}

func TestMutationWorkflowVerdicts(t *testing.T) {
	wf, _ := loadWorkflow(t)
	verdict := func(job string) string {
		j := wf.Jobs[job]
		return j.Steps[stepIndex(j, named("verdict"))].Run
	}
	// incremental-pr (spec §11.2): a sweep route is green (pr-full decides), fail and any other
	// non-zero exit are red, an empty exit code is red.
	for _, c := range []struct{ exit, route string; want int }{
		{"0", "verdict", 0}, {"1", "verdict", 1}, {"0", "sweep", 0}, {"1", "sweep", 0},
		{"0", "fail", 1}, {"1", "fail", 1}, {"", "", 1}, {"2", "", 1}, {"3", "", 1}, {"4", "", 1}, {"130", "", 1},
	} {
		if got := runVerdict(t, verdict("incremental-pr"), map[string]string{"EXIT_CODE": c.exit, "ROUTE": c.route}); got != c.want {
			t.Errorf("incremental-pr exit=%q route=%q: got %d, want %d", c.exit, c.route, got, c.want)
		}
	}
	// full (spec §5.7 step 5): the full run's exit code when that step ran, else the catch-up's.
	for _, c := range []struct{ outcome, fullExit, catchupExit string; want int }{
		{"skipped", "", "0", 0}, {"skipped", "", "1", 1}, {"skipped", "", "", 1},
		{"success", "0", "0", 0}, {"failure", "1", "0", 1}, {"failure", "", "0", 1}, {"failure", "4", "4", 1},
	} {
		env := map[string]string{"FULL_OUTCOME": c.outcome, "FULL_EXIT": c.fullExit, "CATCHUP_EXIT": c.catchupExit}
		if got := runVerdict(t, verdict("full"), env); got != c.want {
			t.Errorf("full outcome=%q full=%q catch-up=%q: got %d, want %d", c.outcome, c.fullExit, c.catchupExit, got, c.want)
		}
	}
	for _, job := range []string{"incremental-main", "pr-full"} {
		for _, c := range []struct{ exit string; want int }{{"0", 0}, {"1", 1}, {"", 1}, {"4", 1}} {
			if got := runVerdict(t, verdict(job), map[string]string{"EXIT_CODE": c.exit}); got != c.want {
				t.Errorf("%s exit=%q: got %d, want %d", job, c.exit, got, c.want)
			}
		}
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test -count=1 -run 'TestMutationWorkflow' .`
Expected: FAIL — the template file does not exist.

- [ ] **Step 3: Write the workflow template**

`templates/mutation-incremental/github-workflow.yml`:

```yaml
# metareview mutation-incremental CI (spec §5.7, §11.2). Copy to .github/workflows/mutation.yml,
# together with tools/mutation-incremental/ and mutation-incremental.json (docs/mutation-harness.md).
#
# - Make both incremental-pr and pr-full required checks (a skipped job satisfies a required check).
# - Edit the push branch if the default branch is not main.
# - Replace the "project setup" steps with the project's own (Node from .nvmrc, npm ci, services).
# - Outputs reach shell steps through env:, never through ${{ }} inside run:.
name: mutation

on:
  push:
    branches: [main]
  pull_request:

permissions: {}

env:
  MUTATION_CLI: tools/mutation-incremental/cli.mjs
  # Per-invocation minutes for PR runs when pendingOnPr is full or full-on-global (ignored under allow).
  MUTATION_PR_MAX_MINUTES: '60'

jobs:
  incremental-pr:
    if: github.event_name == 'pull_request'
    runs-on: ubuntu-latest
    # >= setup + 2 x (MUTATION_PR_MAX_MINUTES + 1) + max(1, views) x (verify.timeoutMinutes + 0.5)
    timeout-minutes: 150
    permissions:
      contents: read
    concurrency:
      group: mutation-inc-${{ github.ref }}
      cancel-in-progress: true
    outputs:
      exit_code: ${{ steps.run.outputs.exit_code }}
      route: ${{ steps.run.outputs.route }}
    steps:
      - name: checkout
        uses: actions/checkout@v4
        with:
          fetch-depth: 0
          persist-credentials: false
      # >>> project setup >>>
      - name: setup node
        uses: actions/setup-node@v4
        with:
          node-version-file: .nvmrc
      - name: install
        run: npm ci
      # <<< project setup <<<
      - name: restore state cache
        uses: actions/cache/restore@v4
        with:
          path: |
            .mutation/attestation.json
            .mutation/incremental.json
          key: mutation-pr-${{ github.event.pull_request.number }}-${{ github.sha }}
          restore-keys: |
            mutation-pr-${{ github.event.pull_request.number }}-
            mutation-main-
      - name: fetch state
        run: node "$MUTATION_CLI" fetch-state
        env:
          MUTATION_STATE_TOKEN: ${{ github.token }}
      - name: run
        id: run
        continue-on-error: true
        run: node "$MUTATION_CLI" run --mode incremental --pr --max-minutes "$MUTATION_PR_MAX_MINUTES" --also-state .mutation/remote/full --also-state .mutation/remote/inc
      - name: save state cache
        if: (steps.run.outputs.exit_code == '0' || steps.run.outputs.exit_code == '1') && steps.run.outputs.route == 'verdict' && hashFiles('.mutation/attestation.json') != ''
        uses: actions/cache/save@v4
        with:
          path: |
            .mutation/attestation.json
            .mutation/incremental.json
          key: mutation-pr-${{ github.event.pull_request.number }}-${{ github.sha }}-${{ github.run_id }}-${{ github.run_attempt }}
      - name: summary
        if: always()
        env:
          EXIT_CODE: ${{ steps.run.outputs.exit_code }}
        run: node "$MUTATION_CLI" summary --job pr --pr --exit-code "$EXIT_CODE" || true
      - name: verdict
        env:
          EXIT_CODE: ${{ steps.run.outputs.exit_code }}
          ROUTE: ${{ steps.run.outputs.route }}
        run: |
          case "$EXIT_CODE:$ROUTE" in
            0:sweep|1:sweep) echo "verdict deferred to pr-full" ;;
            0:verdict) ;;
            0:fail|1:fail) echo "no reachable tests (see the step summary)"; exit 1 ;;
            *) echo "mutation run exit code '$EXIT_CODE'"; exit 1 ;;
          esac

  pr-full:
    needs: incremental-pr
    if: needs.incremental-pr.outputs.route == 'sweep' && (needs.incremental-pr.outputs.exit_code == '0' || needs.incremental-pr.outputs.exit_code == '1')
    runs-on: ubuntu-latest
    # >= setup + one full run + max(1, views) x verify.timeoutMinutes
    timeout-minutes: 350
    permissions:
      contents: read
    concurrency:
      group: mutation-pr-full-${{ github.event.pull_request.number }}
      cancel-in-progress: false
    steps:
      - name: checkout
        uses: actions/checkout@v4
        with:
          fetch-depth: 0
          persist-credentials: false
      # >>> project setup >>>
      - name: setup node
        uses: actions/setup-node@v4
        with:
          node-version-file: .nvmrc
      - name: install
        run: npm ci
      # <<< project setup <<<
      - name: run
        id: run
        continue-on-error: true
        run: node "$MUTATION_CLI" run --mode full
      - name: save state cache
        if: (steps.run.outputs.exit_code == '0' || steps.run.outputs.exit_code == '1') && hashFiles('.mutation/attestation.json') != ''
        uses: actions/cache/save@v4
        with:
          path: |
            .mutation/attestation.json
            .mutation/incremental.json
          key: mutation-pr-${{ github.event.pull_request.number }}-${{ github.sha }}-${{ github.run_id }}-${{ github.run_attempt }}-full
      - name: summary
        if: always()
        env:
          EXIT_CODE: ${{ steps.run.outputs.exit_code }}
        run: node "$MUTATION_CLI" summary --job pr-full --exit-code "$EXIT_CODE" || true
      - name: verdict
        env:
          EXIT_CODE: ${{ steps.run.outputs.exit_code }}
        run: |
          if [ "$EXIT_CODE" != "0" ]; then echo "mutation sweep exit code '$EXIT_CODE'"; exit 1; fi

  incremental-main:
    if: github.event_name == 'push'
    runs-on: ubuntu-latest
    # >= setup + 2 x (maxMinutesPerInvocation + 1) + max(1, views) x (verify.timeoutMinutes + 0.5)
    timeout-minutes: 60
    permissions:
      contents: write
    concurrency:
      group: mutation-inc-${{ github.ref }}
      cancel-in-progress: false
    outputs:
      exit_code: ${{ steps.run.outputs.exit_code }}
      pending_full: ${{ steps.run.outputs.pending_full }}
    steps:
      - name: checkout
        uses: actions/checkout@v4
        with:
          fetch-depth: 0
          persist-credentials: false
      # >>> project setup >>>
      - name: setup node
        uses: actions/setup-node@v4
        with:
          node-version-file: .nvmrc
      - name: install
        run: npm ci
      # <<< project setup <<<
      - name: restore state cache
        uses: actions/cache/restore@v4
        with:
          path: |
            .mutation/attestation.json
            .mutation/incremental.json
          key: mutation-main-${{ github.sha }}
          restore-keys: |
            mutation-main-
      - name: fetch state
        run: node "$MUTATION_CLI" fetch-state
        env:
          MUTATION_STATE_TOKEN: ${{ github.token }}
      - name: run
        id: run
        continue-on-error: true
        run: node "$MUTATION_CLI" run --mode incremental --also-state .mutation/remote/full --also-state .mutation/remote/inc
      - name: save state cache
        if: (steps.run.outputs.exit_code == '0' || steps.run.outputs.exit_code == '1') && hashFiles('.mutation/attestation.json') != ''
        uses: actions/cache/save@v4
        with:
          path: |
            .mutation/attestation.json
            .mutation/incremental.json
          key: mutation-main-${{ github.sha }}-${{ github.run_id }}-${{ github.run_attempt }}
      - name: publish inc
        if: (steps.run.outputs.exit_code == '0' || steps.run.outputs.exit_code == '1') && hashFiles('.mutation/attestation.json') != ''
        run: node "$MUTATION_CLI" publish-state --kind inc
        env:
          MUTATION_STATE_TOKEN: ${{ github.token }}
      - name: summary
        if: always()
        env:
          EXIT_CODE: ${{ steps.run.outputs.exit_code }}
        run: node "$MUTATION_CLI" summary --job main --exit-code "$EXIT_CODE" || true
      - name: verdict
        env:
          EXIT_CODE: ${{ steps.run.outputs.exit_code }}
        run: |
          if [ "$EXIT_CODE" != "0" ]; then echo "mutation run exit code '$EXIT_CODE'"; exit 1; fi

  full:
    needs: incremental-main
    if: ${{ !cancelled() && needs.incremental-main.outputs.pending_full == 'true' }}
    runs-on: ubuntu-latest
    # >= setup + catch-up (2 x maxMinutesPerInvocation) + full run + 2 x max(1, views) x verify.timeoutMinutes
    timeout-minutes: 350
    permissions:
      contents: write
    concurrency:
      group: mutation-main-full
      cancel-in-progress: false
    steps:
      - name: checkout
        uses: actions/checkout@v4
        with:
          fetch-depth: 0
          persist-credentials: false
      # >>> project setup >>>
      - name: setup node
        uses: actions/setup-node@v4
        with:
          node-version-file: .nvmrc
      - name: install
        run: npm ci
      # <<< project setup <<<
      - name: fetch state
        run: node "$MUTATION_CLI" fetch-state
        env:
          MUTATION_STATE_TOKEN: ${{ github.token }}
      - name: catch-up
        id: catchup
        continue-on-error: true
        run: node "$MUTATION_CLI" run --mode incremental --also-state .mutation/remote/full --also-state .mutation/remote/inc
      - name: save catch-up cache
        if: (steps.catchup.outputs.exit_code == '0' || steps.catchup.outputs.exit_code == '1') && steps.catchup.outputs.pending_full == 'false' && hashFiles('.mutation/attestation.json') != ''
        uses: actions/cache/save@v4
        with:
          path: |
            .mutation/attestation.json
            .mutation/incremental.json
          key: mutation-main-${{ github.sha }}-${{ github.run_id }}-${{ github.run_attempt }}-catchup
      - name: publish catch-up
        if: (steps.catchup.outputs.exit_code == '0' || steps.catchup.outputs.exit_code == '1') && steps.catchup.outputs.pending_full == 'false' && hashFiles('.mutation/attestation.json') != ''
        run: node "$MUTATION_CLI" publish-state --kind full
        env:
          MUTATION_STATE_TOKEN: ${{ github.token }}
      - name: full run
        id: full
        if: ${{ !((steps.catchup.outputs.exit_code == '0' || steps.catchup.outputs.exit_code == '1') && steps.catchup.outputs.pending_full == 'false') }}
        continue-on-error: true
        run: node "$MUTATION_CLI" run --mode full
      - name: save full cache
        if: (steps.full.outputs.exit_code == '0' || steps.full.outputs.exit_code == '1') && hashFiles('.mutation/attestation.json') != ''
        uses: actions/cache/save@v4
        with:
          path: |
            .mutation/attestation.json
            .mutation/incremental.json
          key: mutation-main-${{ github.sha }}-${{ github.run_id }}-${{ github.run_attempt }}-full
      - name: publish full
        if: (steps.full.outputs.exit_code == '0' || steps.full.outputs.exit_code == '1') && hashFiles('.mutation/attestation.json') != ''
        run: node "$MUTATION_CLI" publish-state --kind full
        env:
          MUTATION_STATE_TOKEN: ${{ github.token }}
      - name: summary
        if: always()
        env:
          FULL_OUTCOME: ${{ steps.full.outcome }}
          FULL_EXIT: ${{ steps.full.outputs.exit_code }}
          CATCHUP_EXIT: ${{ steps.catchup.outputs.exit_code }}
        run: |
          if [ "$FULL_OUTCOME" = "skipped" ]; then final="$CATCHUP_EXIT"; else final="$FULL_EXIT"; fi
          node "$MUTATION_CLI" summary --job full --exit-code "$final" || true
      - name: verdict
        env:
          FULL_OUTCOME: ${{ steps.full.outcome }}
          FULL_EXIT: ${{ steps.full.outputs.exit_code }}
          CATCHUP_EXIT: ${{ steps.catchup.outputs.exit_code }}
        run: |
          if [ "$FULL_OUTCOME" = "skipped" ]; then final="$CATCHUP_EXIT"; else final="$FULL_EXIT"; fi
          if [ "$final" != "0" ]; then echo "mutation full job exit code '$final'"; exit 1; fi
```

Note on the static test: the `full` job's summary step starts with a shell `if`, not `node "$MUTATION_CLI" summary`, so the `if: always()` assertion (which matches on that prefix) skips it; the step has `if: always()` anyway. The `|| true` assertion matches any script containing `"$MUTATION_CLI" summary`, so it covers this step too.

- [ ] **Step 4: Run to verify it passes**

Run: `go test -count=1 -run 'TestMutationWorkflow' . && go vet .`
Expected: `ok`; vet clean.

- [ ] **Step 5: Commit**

```bash
git add templates/mutation-incremental/github-workflow.yml mutationworkflow_test.go
git commit -m "feat(mutation-incremental): GitHub workflow template with static and fake-exit tests

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 4: The planner golden-output guard (spec §11.6)

**Files:**
- Create: `templates/mutation-incremental/test/golden.test.mjs`, `testdata/mutation-incremental/planner-golden.json` (generated in Step 2)

**Interfaces:**
- Consumes: `runRepo`, `warm`, `cli`, `A` from `test/fake.mjs`; `STATE_VERSION`.

- [ ] **Step 1: Write the test**

`templates/mutation-incremental/test/golden.test.mjs`:

```js
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { STATE_VERSION } from '../lib/state.mjs';
import { A, cli, runRepo, warm } from './fake.mjs';

// Spec §11.6: stateVersion is bumped whenever planning semantics change. These plans are pinned
// to it: changed outputs fail until STATE_VERSION is bumped and the file regenerated with
// UPDATE_GOLDEN=1. Regenerating without a bump is refused.
const GOLDEN = new URL('../../../testdata/mutation-incremental/planner-golden.json', import.meta.url);
const A3 = A.replace('n > hi', 'n >= hi');
const HELPER = { 'tests/helpers/h.ts': 'export const h = 1;\n', 'tests/a.test.ts': "import { clamp } from '../src/a';\nimport { h } from './helpers/h';\n" };

const SCENARIOS = {
  cold: { cold: true },
  'residual-edit': { edit: (r) => r.write('src/a.ts', A3) },
  'whole-edit': { config: { editedFiles: 'whole' }, edit: (r) => r.write('src/a.ts', A3) },
  'test-edit': { edit: (r) => r.write('tests/a.test.ts', "import { clamp } from '../src/a';\n// another case\n") },
  'support-edit': { files: HELPER, edit: (r) => r.write('tests/helpers/h.ts', 'export const h = 2;\n') },
  'global-edit': { files: { 'package.json': '{}\n' }, edit: (r) => r.write('package.json', '{"x":1}\n') },
  'new-module': { edit: (r) => r.write('src/n.ts', 'export const n = 1;\n') },
  'deleted-test': { edit: (r) => rmSync(join(r.top, 'tests/a.test.ts')) },
};

async function currentOutputs() {
  const out = {};
  for (const [name, s] of Object.entries(SCENARIOS)) {
    const r = runRepo({ config: s.config ?? {}, files: s.files ?? {} });
    if (!s.cold) {
      await warm(r);
      s.edit(r);
    }
    out[name] = JSON.parse((await cli(r, ['plan'])).stdout);
  }
  return out;
}

test('planner golden outputs are pinned to stateVersion', async () => {
  const current = await currentOutputs();
  const golden = existsSync(GOLDEN) ? JSON.parse(readFileSync(GOLDEN, 'utf8')) : null;
  if (process.env.UPDATE_GOLDEN === '1') {
    const unchanged = golden !== null && JSON.stringify(golden.outputs) === JSON.stringify(current);
    assert.ok(golden === null || golden.stateVersion !== STATE_VERSION || unchanged, 'planner outputs changed: bump STATE_VERSION in lib/state.mjs first (spec §11.6)');
    writeFileSync(GOLDEN, `${JSON.stringify({ stateVersion: STATE_VERSION, outputs: current }, null, 2)}\n`);
    return;
  }
  assert.notEqual(golden, null, 'missing planner-golden.json: generate it with UPDATE_GOLDEN=1');
  assert.equal(golden.stateVersion, STATE_VERSION, 'STATE_VERSION changed: regenerate planner-golden.json with UPDATE_GOLDEN=1');
  assert.deepEqual(current, golden.outputs, 'planner outputs changed: bump STATE_VERSION in lib/state.mjs, then regenerate with UPDATE_GOLDEN=1 (spec §11.6)');
});
```

- [ ] **Step 2: Generate the golden file, then verify the guard**

Run: `UPDATE_GOLDEN=1 node --test templates/mutation-incremental/test/golden.test.mjs`
Expected: PASS; `testdata/mutation-incremental/planner-golden.json` exists with `"stateVersion": 1`. Read it and check that each scenario's plan is what Plan 1a's rules predict:
- the residual edit forces `src/a.ts:2-3` with scope `[src/a.ts]`;
- the whole edit forces `src/a.ts`;
- the global edit has a `["*"]` deferral;
- the deleted test scopes `src/a.ts`.

Run: `node --test templates/mutation-incremental/test/golden.test.mjs`
Expected: PASS.

Then prove the guard fires. Temporarily edit `lib/plan.mjs` so `forcedCount` is off by one (`forcedCount: forcedCount + 1` in the returned object) and run the test again.
Expected: FAIL with "planner outputs changed". Revert the edit and confirm PASS.

- [ ] **Step 3: Run the suite with coverage**

Run: `sh .superpowers/sdd/2026-09-23-mutation-incremental-1c-proof/cov.sh`
Expected: all pass; no uncovered entries.

- [ ] **Step 4: Commit**

```bash
git add templates/mutation-incremental/test/golden.test.mjs testdata/mutation-incremental/planner-golden.json
git commit -m "test(mutation-incremental): pin planner golden outputs to stateVersion

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 5: The real fixture project

**Files:**
- Create: `testdata/mutation-incremental-e2e/` with every file below, plus `package-lock.json` (generated in Step 2).

The layout follows spec §7.1 and §11.7, with `grade` in place of `clamp` (flagged decision). Line numbers matter: the e2e rows edit exact lines.

- [ ] **Step 1: Write the fixture**

`testdata/mutation-incremental-e2e/package.json`:

```json
{
  "name": "mutation-incremental-e2e-fixture",
  "private": true,
  "type": "module",
  "scripts": { "test": "vitest run" },
  "devDependencies": {
    "@stryker-mutator/core": "10.0.0",
    "@stryker-mutator/vitest-runner": "10.0.0",
    "vitest": "4.1.11"
  }
}
```

`testdata/mutation-incremental-e2e/.gitignore`:

```
node_modules/
.mutation/
reports/
.stryker-tmp/
```

`testdata/mutation-incremental-e2e/stryker.config.json`:

```json
{
  "testRunner": "vitest",
  "vitest": { "configFile": "vitest.config.ts" },
  "mutate": ["src/**/*.ts"],
  "ignorePatterns": [".mutation"],
  "disableBail": true,
  "reporters": ["clear-text"],
  "thresholds": { "high": 80, "low": 60, "break": null },
  "tempDirName": ".stryker-tmp"
}
```

`testdata/mutation-incremental-e2e/vitest.config.ts`:

```ts
import { fileURLToPath } from 'node:url';
import { defineConfig } from 'vitest/config';

export default defineConfig({
  resolve: { alias: [{ find: /^@app\//, replacement: fileURLToPath(new URL('./src/', import.meta.url)) }] },
  test: { include: ['tests/**/*.test.ts'] },
});
```

`testdata/mutation-incremental-e2e/mutation-incremental.json` (the e2e script rewrites `stryker.command` to its recording shim before the first commit):

```json
{
  "schemaVersion": 1,
  "stateDir": ".mutation",
  "stryker": { "command": ["npx", "--no-install", "stryker"], "configFile": "stryker.config.json", "extraArgs": [] },
  "mutate": ["src/**/*.ts"],
  "test": ["tests/**/*.test.ts"],
  "support": ["tests/helpers/**"],
  "global": ["package.json", "package-lock.json", "stryker.config.json", "vitest.config.ts"],
  "ignore": ["README.md", ".gitignore", "reports/**"],
  "aliases": { "@app/": "src/" },
  "runtime": { "commands": [], "env": [] },
  "budget": { "maxForcedShare": 1, "maxForcedMutants": null, "maxMinutesPerInvocation": 10 },
  "residual": { "mode": "bounded" },
  "pendingOnPr": "full-on-global",
  "views": { "inline": { "core": ["src/**/*.ts"], "edge": ["src/c.ts", "src/e.ts"] } }
}
```

`testdata/mutation-incremental-e2e/src/a.ts` (block body on lines 1–5, statements on lines 2–4, no equivalent mutants):

```ts
export function grade(score: number, pass: number, top: number): string {
  if (score < pass) return 'fail';
  if (score > top) return 'over';
  return 'ok';
}
```

`testdata/mutation-incremental-e2e/src/b.ts`:

```ts
import { grade } from './a';
import type { Range } from './types';

export const inRange = (n: number, r: Range) => grade(n, r.lo, r.hi) === 'ok';
```

`testdata/mutation-incremental-e2e/src/c.ts`:

```ts
import { LIMIT } from './limits';
export const twice = (n: number) => Math.min(n * 2, LIMIT);
```

`testdata/mutation-incremental-e2e/src/limits.ts`:

```ts
export const LIMIT = 100;
```

`testdata/mutation-incremental-e2e/src/types.ts`:

```ts
export interface Range { lo: number; hi: number }
```

`testdata/mutation-incremental-e2e/src/index.ts`:

```ts
export * from './a';
```

`testdata/mutation-incremental-e2e/src/e.ts` (two independent multi-line functions, §11.7):

```ts
export function inc(n: number): number {
  const r = n + 1;
  return r;
}

export function dec(n: number): number {
  const r = n - 1;
  return r;
}
```

`testdata/mutation-incremental-e2e/tests/a.test.ts`:

```ts
import assert from 'node:assert';
import { test } from 'vitest';
import { grade } from '../src/a';

test('below pass fails', () => assert.equal(grade(10, 50, 90), 'fail'));
test('at pass is ok', () => assert.equal(grade(50, 50, 90), 'ok'));
test('at top is ok', () => assert.equal(grade(90, 50, 90), 'ok'));
test('above top is over', () => assert.equal(grade(95, 50, 90), 'over'));
```

`testdata/mutation-incremental-e2e/tests/b.test.ts`:

```ts
import { expect, test } from 'vitest';
import { inRange } from '../src/b';
import { range } from './helpers/make';

test('inside the range', () => expect(inRange(60, range(50, 90))).toBe(true));
test('outside the range', () => expect(inRange(10, range(50, 90))).toBe(false));
```

`testdata/mutation-incremental-e2e/tests/c.test.ts`:

```ts
import { expect, test } from 'vitest';
import { twice } from '@app/c';
import { fmt } from './helpers/fmt';

test('doubles small numbers', () => expect(fmt(twice(3))).toBe('6'));
test('caps at LIMIT', () => expect(fmt(twice(80))).toBe('100'));
```

`testdata/mutation-incremental-e2e/tests/index.test.ts`:

```ts
import { expect, test } from 'vitest';
import { grade } from '../src/index';

test('barrel re-exports grade', () => expect(grade(70, 50, 90)).toBe('ok'));
```

`testdata/mutation-incremental-e2e/tests/e-inc.test.ts`:

```ts
import { expect, test } from 'vitest';
import { inc } from '../src/e';

test('inc', () => expect(inc(1)).toBe(2));
```

`testdata/mutation-incremental-e2e/tests/e-dec.test.ts`:

```ts
import { expect, test } from 'vitest';
import { dec } from '../src/e';

test('dec', () => expect(dec(1)).toBe(0));
```

`testdata/mutation-incremental-e2e/tests/helpers/make.ts`:

```ts
export const range = (lo: number, hi: number) => ({ lo, hi });
```

`testdata/mutation-incremental-e2e/tests/helpers/fmt.ts`:

```ts
export const fmt = (n: number) => String(n);
```

`testdata/mutation-incremental-e2e/README.md`: list every file above with a one-line purpose. Then list each e2e row's exact edit, using the table in Task 6 Step 1. Name the negative control `e-flip`:
- **Edit:** line 3 of `src/e.ts`, `  return r;` (inside `inc`) → `  return 2;`.
- **Effect:** the ArithmeticOperator mutant on line 2 (`n - 1`) was killed by `tests/e-inc.test.ts` and now survives, because `inc` returns 2 whatever `r` is.
- **What the control shows:** with the residual closure disabled (`MUTATION_TEST_DISABLE_RESIDUAL=1`), that kill is reused unchanged. Equivalence then fails for exactly this mutant.

- [ ] **Step 2: Generate the lockfile and check the tests pass**

Run (the scratch copy lives under the gitignored `.e2e/`):

```bash
rm -rf .e2e/fixture-check && mkdir -p .e2e && cp -R testdata/mutation-incremental-e2e .e2e/fixture-check
(cd .e2e/fixture-check && npm install --no-audit --no-fund && npx vitest run)
cp .e2e/fixture-check/package-lock.json testdata/mutation-incremental-e2e/package-lock.json
```

Expected: `npm install` succeeds, and Vitest reports 12 passed tests in 6 files.

- [ ] **Step 3: Check the base fixture has no survivors with real Stryker**

Run: `(cd .e2e/fixture-check && npx stryker run stryker.config.json --incremental --incrementalFile .mutation/probe.json --force)`
Expected: exit 0, and the clear-text report shows no `Survived` and no `NoCoverage` mutants. If Stryker reports a survivor, add the missing assertion to the owning test file, record a ruling in the ledger naming the mutant, and re-run.

- [ ] **Step 4: Commit**

```bash
git add testdata/mutation-incremental-e2e
git commit -m "test(mutation-incremental): real TypeScript fixture for the e2e proof

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---
### Task 6: The real-Stryker e2e proof

**Files:**
- Create: `tests/e2e-mutation-incremental.mjs`, `tests/e2e-mutation-incremental.sh`
- Create (generated by the run): `testdata/mutation-incremental/real/<row>/{attestation.json,incremental.json}`

**Interfaces:**
- Consumes: the fixture (Task 5), the template (`cli.mjs`, `lib/`), and the CLI surface of Plans 1b–1c.
- Produces: a PASS/FAIL line per check, `.e2e/<run>/results.json`, a log per CLI call, and the exported real reports.

**Rows** (each starts from the committed tree and the `full` row's state unless it says otherwise):

| Row | Edit / action | Must hold |
|---|---|---|
| `cold` | no state | `plan` is cold with `no usable state`; `run` exits 0 with no invocation and `pending_full=true` |
| `full` | `run --mode full` | exit 0, one invocation, deferrals `[]`, `lastFullAt = completedAt`, score 100 (no survivors) |
| `no-change` | none | exit 0, no invocation |
| `a-preserving` | `src/a.ts` line 3 → `  if (top < score) return 'over';` | exit 0, no deferral, **equivalence** |
| `a-survivor` | `src/a.ts` line 3 → `  if (score >= top + 1) return 'over';` | exit 0, a `Survived` mutant on line 3, **equivalence** |
| `test-b` | a third case appended to `tests/b.test.ts` | scope `[src/a.ts, src/b.ts]`, **equivalence** |
| `limits` | `src/limits.ts` → `export const LIMIT = 10 * 10;` | forced has `src/limits.ts` and a `src/c.ts` range, no deferral, **equivalence** |
| `helper-make` | `tests/helpers/make.ts` → `({ hi, lo })` | forced has `src/a.ts` and `src/b.ts` ranges, no open importers, **equivalence** |
| `types-only` | `src/types.ts` → `…; hi: number; }` | one invocation (no tests reach it), no deferral, `pending_full=false`, next plan has no changes |
| `delete-c-test` | delete `tests/c.test.ts` | deferral `no reachable tests: src/c.ts`, cause `unreachable`, **equivalence** outside `src/c.ts` |
| `e-residual` | `src/e.ts` line 2 → `  const r = 1 + n;` | no forced range in `dec` (lines 6–9), **equivalence** |
| `e-flip` | `src/e.ts` line 3 → `  return 2;` | line 2's ArithmeticOperator mutant is `Survived`, **equivalence** |
| `e-flip-control` | same edit with `MUTATION_TEST_DISABLE_RESIDUAL=1` | equivalence fails for **exactly** `src/e.ts:2 ArithmeticOperator` |
| `lockfile` | `package-lock.json` + newline | deferral `global input changed: package-lock.json`, no invocation, `pending_full=true` |
| `time-budget` | `a-preserving`'s edit; shim sleeps 20 s; `run --pr --max-minutes 0.05` | exit 0, `time budget exceeded`, `route=sweep` |
| `sighup` | `a-preserving`'s edit; shim sleeps 30 s; SIGHUP after 5 s | exit 130, attestation unchanged, no lock, no `work/` |
| `stryker-killed` | `a-preserving`'s edit; shim SIGKILLs itself | exit 4, attestation unchanged |
| `full-no-tests` | shim prints `No tests were executed`, exit 1 | `run --mode full` exits 4, attestation unchanged |
| `publish-fetch` | `publish-state --kind inc`; clone; `fetch-state`; `run --also-state .mutation/remote/inc` | all exit 0; the clone runs nothing and has no deferrals |
| `seed` | no state; `seed --from <baseline report>`, then again with `--replace` | deferral `seeded from …`; the old pair kept in `replaced/<ts>/` |
| `threshold` | `thresholds.break: 100`; drop the `at top is ok` test | `run --mode full` exits 1; an unchanged re-run exits 1 again |

**Equivalence** (spec §7.1): a fresh non-incremental Stryker run on the same tree (`--incremental --incrementalFile .mutation/fresh.json --force`, starting from no file). Every `Killed` mutant in the harness's report is checked, matched by `(file, mutatorName, location, replacement)`, except in files named by a deferral. A `"*"` deferral skips the comparison. Each checked mutant must be `Killed` (or `Timeout`) in the fresh run. The number of compared kills is printed, and zero compared is a failure.

- [ ] **Step 1: Write the script**

`tests/e2e-mutation-incremental.sh`:

```sh
#!/bin/sh
# Local real-Stryker proof of the mutation-incremental harness (spec §7.1). Needs network; not in CI.
exec node "$(dirname "$0")/e2e-mutation-incremental.mjs" "$@"
```

`tests/e2e-mutation-incremental.mjs`:

```js
#!/usr/bin/env node
// Local proof of the mutation-incremental harness against real StrykerJS 10 and Vitest 4 (spec §7.1,
// §11.7). Copies testdata/mutation-incremental-e2e into .e2e/<run>/ (never /tmp), installs its pinned
// dependencies (network), and drives the real CLI through a shim that records Stryker's argv.
// Not run in CI. Usage: node tests/e2e-mutation-incremental.mjs [row ...]   (the full row always runs)
import { execFileSync, spawn, spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { cpSync, existsSync, mkdirSync, readFileSync, readdirSync, rmSync, symlinkSync, writeFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const FIXTURE = join(ROOT, 'testdata/mutation-incremental-e2e');
const TEMPLATE = join(ROOT, 'templates/mutation-incremental');
const REAL = join(ROOT, 'testdata/mutation-incremental/real');
const WORK = join(ROOT, '.e2e', `e2e-${new Date().toISOString().replace(/[:.]/g, '-')}`);
const REPO = join(WORK, 'repo');
const BARE = join(WORK, 'remote.git');
const ONLY = new Set(process.argv.slice(2));
const STATE_FILES = ['attestation.json', 'incremental.json'];

const SHIM = `// Records Stryker's argv, then runs the real Stryker. shim.json can make it sleep, die or find no tests.
import { appendFileSync, existsSync, readFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
const dir = process.env.E2E_DIR;
appendFileSync(dir + '/calls.jsonl', JSON.stringify(process.argv.slice(2)) + '\\n');
const mode = existsSync(dir + '/shim.json') ? JSON.parse(readFileSync(dir + '/shim.json', 'utf8')) : {};
if (mode.sleepMs) await new Promise((r) => setTimeout(r, mode.sleepMs));
if (mode.die) process.kill(process.pid, 'SIGKILL');
if (mode.noTests) { console.log('INFO No tests were executed. Stryker will exit prematurely.'); process.exit(1); }
const r = spawnSync(process.execPath, ['node_modules/.bin/stryker', ...process.argv.slice(2)], { stdio: 'inherit' });
process.exit(r.status ?? 1);
`;

const results = [];
function check(label, ok, detail = '') {
  results.push({ label, ok: Boolean(ok), detail: ok ? '' : String(detail) });
  console.log(`${ok ? 'PASS' : 'FAIL'} ${label}${ok ? '' : ` — ${detail}`}`);
}
const git = (...args) => execFileSync('git', args, { cwd: REPO, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] });
const readJSON = (rel, dir = REPO) => JSON.parse(readFileSync(join(dir, rel), 'utf8'));
const stateText = (dir = REPO) => readFileSync(join(dir, '.mutation/attestation.json'), 'utf8');

let callNo = 0;
// Runs the harness CLI; returns {code, stdout, stderr, output}. signalAfterMs sends SIGHUP.
function cli(args, { dir = REPO, env = {}, signalAfterMs = null } = {}) {
  return new Promise((done) => {
    const n = ++callNo;
    const outFile = join(WORK, `output-${n}`);
    writeFileSync(outFile, '');
    const child = spawn(process.execPath, ['tools/mutation-incremental/cli.mjs', ...args], {
      cwd: dir,
      env: { ...process.env, E2E_DIR: WORK, GITHUB_OUTPUT: outFile, ...env },
      stdio: ['ignore', 'pipe', 'pipe'],
    });
    let stdout = '';
    let stderr = '';
    child.stdout.on('data', (c) => { stdout += c; });
    child.stderr.on('data', (c) => { stderr += c; });
    const timer = signalAfterMs === null ? null : setTimeout(() => child.kill('SIGHUP'), signalAfterMs);
    child.on('close', (code) => {
      clearTimeout(timer);
      writeFileSync(join(WORK, `log-${n}.txt`), `$ cli ${args.join(' ')}   (cwd ${dir})\nexit ${code}\n${stderr}`);
      const lines = readFileSync(outFile, 'utf8').split('\n').filter(Boolean);
      const output = Object.fromEntries(lines.map((l) => [l.slice(0, l.indexOf('=')), l.slice(l.indexOf('=') + 1)]));
      done({ code, stdout, stderr, output });
    });
  });
}
const plan = async (args = []) => JSON.parse((await cli(['plan', ...args])).stdout);
const calls = () => (existsSync(join(WORK, 'calls.jsonl')) ? readFileSync(join(WORK, 'calls.jsonl'), 'utf8').split('\n').filter(Boolean).map((l) => JSON.parse(l)) : []);
function shim(mode) {
  rmSync(join(WORK, 'calls.jsonl'), { force: true });
  if (mode) writeFileSync(join(WORK, 'shim.json'), JSON.stringify(mode));
  else rmSync(join(WORK, 'shim.json'), { force: true });
}
function edit(rel, from, to) {
  const path = join(REPO, rel);
  const text = readFileSync(path, 'utf8');
  if (!text.includes(from)) throw new Error(`${rel}: ${JSON.stringify(from)} not found`);
  writeFileSync(path, text.replace(from, to));
}
// Back to the committed tree; with `state`, the full row's state.
function reset({ state = true } = {}) {
  git('checkout', '--', '.');
  git('clean', '-fdq');
  rmSync(join(REPO, '.mutation'), { recursive: true, force: true });
  if (state) {
    mkdirSync(join(REPO, '.mutation'), { recursive: true });
    for (const f of STATE_FILES) cpSync(join(WORK, 'baseline', f), join(REPO, '.mutation', f));
  }
  shim(null);
}

const mutantKey = (file, m) => JSON.stringify([file, m.mutatorName, m.location, m.replacement]);
function freshStatuses() {
  const rel = '.mutation/fresh.json';
  rmSync(join(REPO, rel), { force: true });
  const r = spawnSync(process.execPath, ['node_modules/.bin/stryker', 'run', 'stryker.config.json', '--incremental', '--incrementalFile', rel, '--force'], { cwd: REPO, encoding: 'utf8', maxBuffer: 1 << 28 });
  if (r.status !== 0 && r.status !== 1) throw new Error(`fresh Stryker run failed (${r.status}): ${String(r.stdout).slice(-2000)}`);
  const statuses = new Map();
  for (const [file, entry] of Object.entries(readJSON(rel).files)) for (const m of entry.mutants) statuses.set(mutantKey(file, m), m.status);
  return statuses;
}
// Spec §7.1 equivalence: every kill outside a deferral is a kill in a fresh run of the same tree.
function equivalence() {
  const att = readJSON('.mutation/attestation.json');
  if (att.deferrals.some((d) => d.paths.includes('*'))) return { compared: 0, mismatches: [] };
  const deferred = new Set(att.deferrals.flatMap((d) => d.paths));
  const fresh = freshStatuses();
  const mismatches = [];
  let compared = 0;
  for (const [file, entry] of Object.entries(readJSON('.mutation/incremental.json').files)) {
    if (deferred.has(file)) continue;
    for (const m of entry.mutants) {
      if (m.status !== 'Killed') continue;
      compared++;
      const status = fresh.get(mutantKey(file, m)) ?? 'absent';
      if (status !== 'Killed' && status !== 'Timeout') mismatches.push(`${file}:${m.location.start.line} ${m.mutatorName} ${JSON.stringify(m.replacement)} → fresh ${status}`);
    }
  }
  return { compared, mismatches };
}
function checkEquivalence(row) {
  const e = equivalence();
  check(`${row}: equivalence (${e.compared} kills compared)`, e.compared > 0 && e.mismatches.length === 0, e.mismatches.join('; ') || 'nothing compared');
}
// Real reports for Plan 2's gate tests: projectRoot rewritten to ".", reportSha256 recomputed.
function exportRow(row) {
  const dir = join(REAL, row);
  mkdirSync(dir, { recursive: true });
  const report = readJSON('.mutation/incremental.json');
  report.projectRoot = '.';
  const bytes = `${JSON.stringify(report)}\n`;
  const att = readJSON('.mutation/attestation.json');
  att.reportSha256 = createHash('sha256').update(bytes).digest('hex');
  writeFileSync(join(dir, 'incremental.json'), bytes);
  writeFileSync(join(dir, 'attestation.json'), `${JSON.stringify(att, null, 2)}\n`);
}
const mutantsAt = (file, line) => (readJSON('.mutation/incremental.json').files[file]?.mutants ?? []).filter((m) => m.location.start.line === line);
const deferralReasons = () => readJSON('.mutation/attestation.json').deferrals.map((d) => d.reason);

async function editRow(row, apply, { args = ['run', '--mode', 'incremental'], env = {}, expect = () => [], equivalent = true } = {}) {
  reset();
  apply();
  const p = await plan();
  const r = await cli(args, { env });
  check(`${row}: exit 0`, r.code === 0, `exit ${r.code}: ${r.stderr.slice(-800)}`);
  if (r.code !== 0) return { p, r };
  for (const [label, ok, detail] of expect(p, r)) check(`${row}: ${label}`, ok, detail);
  if (equivalent) checkEquivalence(row);
  exportRow(row);
  return { p, r };
}

const A_LINE3 = "  if (score > top) return 'over';";
const preserveA = () => edit('src/a.ts', A_LINE3, "  if (top < score) return 'over';");

const ROWS = [
  ['cold', async () => {
    reset({ state: false });
    const p = await plan();
    check('cold: plan is cold with "no usable state"', p.cold && p.deferrals.some((d) => d.reason === 'no usable state'), JSON.stringify(p.deferrals));
    const r = await cli(['run', '--mode', 'incremental']);
    check('cold: exit 0, no invocation, pending_full=true', r.code === 0 && calls().length === 0 && r.output.pending_full === 'true', `${r.code} ${calls().length} ${JSON.stringify(r.output)}`);
  }],
  ['full', async () => {
    reset({ state: false });
    const r = await cli(['run', '--mode', 'full']);
    check('full: exit 0 with one invocation', r.code === 0 && calls().length === 1, `exit ${r.code}: ${r.stderr.slice(-800)}`);
    if (r.code !== 0) throw new Error('the full row failed; every other row needs its state');
    const att = readJSON('.mutation/attestation.json');
    check('full: no deferrals, lastFullAt = completedAt', att.deferrals.length === 0 && att.lastFullAt === att.completedAt, JSON.stringify(att.deferrals));
    check('full: the base fixture has no survivors (score 100)', att.score === 100, `score ${att.score}`);
    mkdirSync(join(WORK, 'baseline'), { recursive: true });
    for (const f of STATE_FILES) cpSync(join(REPO, '.mutation', f), join(WORK, 'baseline', f));
    exportRow('full');
  }],
  ['no-change', async () => {
    reset();
    const r = await cli(['run', '--mode', 'incremental']);
    check('no-change: exit 0, no invocation', r.code === 0 && calls().length === 0, `${r.code} ${calls().length}`);
  }],
  ['a-preserving', () => editRow('a-preserving', preserveA, {
    expect: () => [['no deferral', deferralReasons().length === 0, deferralReasons()]],
  })],
  ['a-survivor', () => editRow('a-survivor', () => edit('src/a.ts', A_LINE3, "  if (score >= top + 1) return 'over';"), {
    expect: () => [['a Survived mutant on line 3', mutantsAt('src/a.ts', 3).some((m) => m.status === 'Survived'), JSON.stringify(mutantsAt('src/a.ts', 3).map((m) => m.status))]],
  })],
  ['test-b', () => editRow('test-b', () => edit('tests/b.test.ts', "test('outside the range'", "test('another inside case', () => expect(inRange(70, range(50, 90))).toBe(true));\ntest('outside the range'"), {
    expect: (p) => [['scope [src/a.ts, src/b.ts]', JSON.stringify(p.scope) === '["src/a.ts","src/b.ts"]', JSON.stringify(p.scope)]],
  })],
  ['limits', () => editRow('limits', () => edit('src/limits.ts', 'LIMIT = 100;', 'LIMIT = 10 * 10;'), {
    expect: (p) => [
      ['forced has src/limits.ts and a src/c.ts range', p.forced.includes('src/limits.ts') && p.forced.some((f) => f.startsWith('src/c.ts:')), JSON.stringify(p.forced)],
      ['no deferral', deferralReasons().length === 0, deferralReasons()],
    ],
  })],
  ['helper-make', () => editRow('helper-make', () => edit('tests/helpers/make.ts', '({ lo, hi })', '({ hi, lo })'), {
    expect: (p) => [
      ['forced has src/a.ts and src/b.ts ranges', p.forced.some((f) => f.startsWith('src/a.ts')) && p.forced.some((f) => f.startsWith('src/b.ts')), JSON.stringify(p.forced)],
      ['vitest and node:assert are packages, not open importers', p.openImporters.length === 0, JSON.stringify(p.openImporters)],
    ],
  })],
  ['types-only', async () => {
    const { r } = await editRow('types-only', () => edit('src/types.ts', 'hi: number }', 'hi: number; }'), {
      expect: (_p, res) => [
        ['one invocation, no deferral, pending_full=false', calls().length === 1 && deferralReasons().length === 0 && res.output.pending_full === 'false', `${calls().length} ${deferralReasons()} ${res.output.pending_full}`],
      ],
    });
    if (r.code === 0) {
      const next = await plan();
      check('types-only: the next plan has no changes', next.changes.length === 0, JSON.stringify(next.changes));
    }
  }],
  ['delete-c-test', () => editRow('delete-c-test', () => rmSync(join(REPO, 'tests/c.test.ts')), {
    expect: (_p, r) => [
      ['deferral "no reachable tests: src/c.ts"', deferralReasons().includes('no reachable tests: src/c.ts'), deferralReasons()],
      ['cause unreachable', r.output.pending_cause === 'unreachable', r.output.pending_cause],
    ],
  })],
  ['e-residual', () => editRow('e-residual', () => edit('src/e.ts', '  const r = n + 1;', '  const r = 1 + n;'), {
    expect: () => [['no forced range inside dec (lines 6-9)', !calls().flat().some((a) => /src\/e\.ts:[6-9]/.test(a)), JSON.stringify(calls())]],
  })],
  ['e-flip', () => editRow('e-flip', () => edit('src/e.ts', '  return r;', '  return 2;'), {
    expect: () => [['line 2 ArithmeticOperator now Survived', mutantsAt('src/e.ts', 2).some((m) => m.mutatorName === 'ArithmeticOperator' && m.status === 'Survived'), JSON.stringify(mutantsAt('src/e.ts', 2).map((m) => [m.mutatorName, m.status]))]],
  })],
  ['e-flip-control', async () => {
    const { r } = await editRow('e-flip-control', () => edit('src/e.ts', '  return r;', '  return 2;'), { env: { MUTATION_TEST_DISABLE_RESIDUAL: '1' }, equivalent: false });
    if (r.code !== 0) return;
    const e = equivalence();
    check('e-flip-control: equivalence fails for exactly src/e.ts:2 ArithmeticOperator', e.mismatches.length === 1 && e.mismatches[0].startsWith('src/e.ts:2 ArithmeticOperator'), e.mismatches.join('; ') || 'no mismatch');
  }],
  ['lockfile', () => editRow('lockfile', () => writeFileSync(join(REPO, 'package-lock.json'), `${readFileSync(join(REPO, 'package-lock.json'), 'utf8')}\n`), {
    equivalent: false,
    expect: (_p, r) => [
      ['deferral "global input changed: package-lock.json"', deferralReasons().includes('global input changed: package-lock.json'), deferralReasons()],
      ['no invocation, pending_full=true', calls().length === 0 && r.output.pending_full === 'true', `${calls().length} ${r.output.pending_full}`],
    ],
  })],
  ['time-budget', () => editRow('time-budget', () => { preserveA(); shim({ sleepMs: 20000 }); }, {
    args: ['run', '--mode', 'incremental', '--pr', '--max-minutes', '0.05'],
    equivalent: false,
    expect: (_p, r) => [
      ['deferral "time budget exceeded"', deferralReasons().includes('time budget exceeded'), deferralReasons()],
      ['route=sweep (never red for speed)', r.output.route === 'sweep', r.output.route],
    ],
  })],
  ['sighup', async () => {
    reset();
    preserveA();
    shim({ sleepMs: 30000 });
    const before = stateText();
    const r = await cli(['run', '--mode', 'incremental'], { signalAfterMs: 5000 });
    check('sighup: exit 130', r.code === 130, `exit ${r.code}`);
    check('sighup: state unchanged, no lock, no work/', stateText() === before && !existsSync(join(REPO, '.mutation/lock')) && !existsSync(join(REPO, '.mutation/work')), 'state or lock or work/ left behind');
  }],
  ['stryker-killed', async () => {
    reset();
    preserveA();
    shim({ die: true });
    const before = stateText();
    const r = await cli(['run', '--mode', 'incremental']);
    check('stryker-killed: exit 4, state unchanged', r.code === 4 && stateText() === before, `exit ${r.code}`);
  }],
  ['full-no-tests', async () => {
    reset();
    shim({ noTests: true });
    const before = stateText();
    const r = await cli(['run', '--mode', 'full']);
    check('full-no-tests: exit 4, nothing committed', r.code === 4 && stateText() === before, `exit ${r.code}`);
  }],
  ['publish-fetch', async () => {
    reset();
    const pub = await cli(['publish-state', '--kind', 'inc']);
    check('publish-fetch: publish-state exit 0', pub.code === 0, pub.stderr.slice(-500));
    const clone = join(WORK, 'clone');
    rmSync(clone, { recursive: true, force: true });
    execFileSync('git', ['clone', '-q', REPO, clone]);
    execFileSync('git', ['remote', 'set-url', 'origin', BARE], { cwd: clone });
    symlinkSync(join(REPO, 'node_modules'), join(clone, 'node_modules'));
    // A symlink does not match the `node_modules/` directory pattern; exclude it so it is no input.
    writeFileSync(join(clone, '.git/info/exclude'), 'node_modules\n');
    const got = await cli(['fetch-state'], { dir: clone });
    check('publish-fetch: fetch-state exit 0 with remote/inc', got.code === 0 && existsSync(join(clone, '.mutation/remote/inc/incremental.json')), got.stderr.slice(-500));
    const run = await cli(['run', '--mode', 'incremental', '--also-state', '.mutation/remote/inc'], { dir: clone });
    const att = run.code === 0 ? readJSON('.mutation/attestation.json', clone) : null;
    check('publish-fetch: the clone adopts it (warm, nothing to run, no deferrals)', run.code === 0 && calls().length === 0 && att.deferrals.length === 0, `exit ${run.code} calls ${calls().length} ${JSON.stringify(att?.deferrals)}`);
  }],
  ['seed', async () => {
    reset({ state: false });
    const from = join(WORK, 'baseline/incremental.json');
    const r = await cli(['seed', '--from', from]);
    check('seed: exit 0 with "seeded from"', r.code === 0 && deferralReasons().includes(`seeded from ${from}`), `exit ${r.code} ${r.stderr.slice(-300)}`);
    const again = await cli(['seed', '--from', from, '--replace']);
    const kept = existsSync(join(REPO, '.mutation/replaced')) ? readdirSync(join(REPO, '.mutation/replaced')) : [];
    check('seed: --replace keeps the old pair', again.code === 0 && kept.length === 1, `exit ${again.code} kept ${kept}`);
  }],
  ['threshold', async () => {
    reset();
    edit('stryker.config.json', '"break": null', '"break": 100');
    edit('tests/a.test.ts', "test('at top is ok', () => assert.equal(grade(90, 50, 90), 'ok'));\n", '');
    const r = await cli(['run', '--mode', 'full']);
    check('threshold: a survivor below break 100 is exit 1', r.code === 1, `exit ${r.code}`);
    const again = await cli(['run', '--mode', 'incremental']);
    check('threshold: an unchanged re-run is exit 1 again', again.code === 1, `exit ${again.code}`);
  }],
];

async function setup() {
  mkdirSync(WORK, { recursive: true });
  cpSync(FIXTURE, REPO, { recursive: true });
  mkdirSync(join(REPO, 'tools/mutation-incremental'), { recursive: true });
  cpSync(join(TEMPLATE, 'cli.mjs'), join(REPO, 'tools/mutation-incremental/cli.mjs'));
  cpSync(join(TEMPLATE, 'lib'), join(REPO, 'tools/mutation-incremental/lib'), { recursive: true });
  writeFileSync(join(WORK, 'shim.mjs'), SHIM);
  const config = readJSON('mutation-incremental.json');
  config.stryker.command = [process.execPath, join(WORK, 'shim.mjs')];
  writeFileSync(join(REPO, 'mutation-incremental.json'), `${JSON.stringify(config, null, 2)}\n`);
  execFileSync('npm', ['ci', '--no-audit', '--no-fund'], { cwd: REPO, stdio: 'inherit' });
  git('init', '-q');
  git('config', 'user.email', 'e2e@example.com');
  git('config', 'user.name', 'e2e');
  git('add', '-A');
  git('commit', '-qm', 'fixture');
  execFileSync('git', ['init', '-q', '--bare', BARE]);
  git('remote', 'add', 'origin', BARE);
}

await setup();
for (const [row, run] of ROWS) {
  if (ONLY.size > 0 && row !== 'full' && !ONLY.has(row)) continue;
  const started = Date.now();
  try {
    await run();
  } catch (e) {
    check(`${row}: ran without an error`, false, e.stack);
    if (row === 'full') break;
  }
  console.log(`  (${row}: ${((Date.now() - started) / 1000).toFixed(1)} s)`);
}
const failed = results.filter((r) => !r.ok);
writeFileSync(join(WORK, 'results.json'), `${JSON.stringify(results, null, 2)}\n`);
console.log(`\n${results.length - failed.length}/${results.length} checks passed; work dir ${WORK}`);
process.exitCode = failed.length > 0 ? 1 : 0;
```

- [ ] **Step 2: Run the proof**

Run: `sh tests/e2e-mutation-incremental.sh 2>&1 | tee .e2e/e2e-last.log`
Expected: every check prints PASS, and the final line reads `N/N checks passed`. Each equivalence line reports a non-zero number of compared kills. `testdata/mutation-incremental/real/` gains one directory per committed row.

If a row fails, open its `log-<n>.txt` in the work dir. Then use superpowers:systematic-debugging:
- **A harness defect** gets a failing unit test in the owning `test/*.test.mjs` first (TDD). Fix it and re-run the unit suite with coverage, then this proof.
- **A wrong expectation about Stryker's behaviour** gets its expectation corrected here. Record a ledger ruling naming the observed Stryker behaviour.
- **Never weaken the equivalence check or the negative control.**

- [ ] **Step 3: Commit**

```bash
git add tests/e2e-mutation-incremental.mjs tests/e2e-mutation-incremental.sh testdata/mutation-incremental/real
git commit -m "test(mutation-incremental): real-Stryker e2e proof with equivalence and negative control

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 7: Documentation (spec §9, §11.1–§11.6 docs items)

**Files:**
- Create: `docs/mutation-harness.md`
- Modify: `USAGE.md`, `docs/quickstart.md` (a link each), `CHANGELOG.md` (Unreleased), `docs/ARCHITECTURE.md` (a section), `docs/0.13.0-candidates.md` (§1 notes), `templates/mutation-incremental/lib/config.mjs` (a comment on `MUTATION_ALLOW_TMP_STATE`)

- [ ] **Step 1: Write `docs/mutation-harness.md`**

Use these sections, in order. Every statement listed must appear, in plain words, with each command shown in a fenced `bash` block.

1. **What it is.** It is change-driven StrykerJS mutation testing with attested, change-based freshness. 0.13.0 ships the harness plus the freshness gate. The ≤ 2 min p50 bar for a one-line edit is pending the external trial.
2. **Set up.**
   - Copy `templates/mutation-incremental/` (without `test/`) to `tools/mutation-incremental/`. Copy `mutation-incremental.example.json` to `mutation-incremental.json`, and `github-workflow.yml` to `.github/workflows/mutation.yml`.
   - The Stryker config requires `testRunner: "vitest"`, `disableBail: true`, `mutate` equal to the harness `mutate`, and `ignorePatterns` containing the state dir. `inPlace` is not allowed.
   - Gitignore `<stateDir>/`. Pin Node with a tracked `.nvmrc`.
   - Make `incremental-pr` and `pr-full` required checks.
   - Repository rulesets must allow updates and force-pushes on `mutation-state/*`, with the bypass limited to GitHub Actions.
   - `MUTATION_STATE_TOKEN` is `${{ github.token }}` on the fetch/publish steps only. Keep `github.token` rather than a PAT, because a PAT's pushes trigger other workflows.
   - Edit the push branch if the default branch is not `main`. Optionally add `paths-ignore` mirroring `ignore`, keeping the `full` job's `needs` chain.
3. **Configuration reference.** Cover every key of §5.2 and the optional `editedFiles`, `pendingOnPr`, `allowBail`, `verify` and `views`, with defaults and validation rules. Include sizing guidance:
   - `maxMinutesPerInvocation` is a safety ceiling, not a target;
   - the budget is sized against the ≤ 2 min bar;
   - setup, the catch-up and the full run must fit the job timeout;
   - the hosted runner's 360-minute cap. Above about 13 minutes of verifier time per view, use a single-pass verifier.
4. **What goes where.** Cover:
   - the categories and the `ignore` list;
   - meta-artifacts: files that tests import or read go in `support`, tooling-only JSON and docs go in `ignore`, and `global` is reserved for inputs everything depends on;
   - `aliases` and open importers. `plan` lists them;
   - a gitignored generated module imported by a relative path makes its importers open importers;
   - files read by path, and computed `import(x)`, are invisible to the import graph;
   - `import type` / `verbatimModuleSyntax`: editing a widely imported barrel or constants file often exceeds the budget;
   - paths containing `, { } [ ] ( ) ! * ? :` cannot be passed to `--mutate`.
5. **Commands and exit codes.** Cover `plan`, `run`, `seed`, `fetch-state`, `publish-state` (CI-only), `break-lock` and `summary`. List exit codes 0/1/2/3/4/130 and their precedence. List the `GITHUB_OUTPUT` keys: `pending_full`, `exit_code`, `pending_cause`, `pending_causes` (with its overflow form) and `route`.
6. **How CI works.**
   - The four jobs, the state branches (`mutation-state/inc`, and `mutation-state/full`, which is "main's latest state without deferrals" and may be `mode: incremental` with an older `lastFullAt`), and the cache fast path.
   - The catch-up, the `pr-full` sweep, and the routing table for `allow`, `full` and `full-on-global`. Under `full-on-global` a PR's verdict never depends on runner speed; the remaining speed-bounded reds are the verifier timeout and setup.
   - What things cost:
     - global inputs, including tuning-only config edits, cost one full run on main;
     - a threshold break turns every PR red until main is fixed (the summary shows main's score);
     - a PR that changes a global input, or carries its own budget or time deferral, re-runs its delta on each push;
     - main re-runs each merged PR's delta;
     - re-running an old main workflow publishes an older, still valid state;
     - a cancelled job commits nothing;
     - a red `incremental-main` on a global change triggers no full run (re-run it);
     - repeated red `full` jobs, and a `full` job hitting its timeout, never clear on their own (raise the timeout or narrow `mutate`);
     - `incremental-pr` is not a merge-queue check unless `merge_group` is added.
7. **Local use.**
   - `fetch-state`, then `run --mode incremental --also-state .mutation/remote/full --also-state .mutation/remote/inc`.
   - Warm-start a new worktree with `--also-state <first checkout>/.mutation`.
   - Seeding helps local runs only; CI's first run is still a full run.
   - A local `run --mode full` clears pending kills locally.
   - A local run never turns a PR green, and CI PR state is not gate evidence: run the harness locally before `pr-ready`.
8. **Verifier and views.**
   - The verifier's environment variables (§11.1).
   - It runs once per view, and not on a cold run with nothing committed.
   - Counted vs inherited pending. A strict survivor-bar verifier treats counted pending as unverified only on PR runs (`MUTATION_RUN_KIND=pr`).
   - The waiver identity `(file, mutatorName, location, replacement)` plus the file digest.
   - A single-pass verifier over the views in the attestation.
   - View completeness is exit 2, so a new file must be assigned to a view first.
   - Verifier output is public.
9. **Security and privacy.**
   - State, caches, summaries and logs are public on public repositories. Tests must not print environment or config values, and `runtime.env` holds digests only; never list secrets there.
   - The state branches are as trusted as the repository's write collaborators.
   - Private repositories without fork Actions run no PR job for forks.
10. **Upgrading and cutover.** Write numbered steps:
    1. Adopt with one state and views.
    2. Keep `pendingOnPr: "allow"` until `mutation-state/full` exists.
    3. The first main push runs the full sweep and publishes.
    4. Existing reports show `unattested`, as advisory, until then.
    5. Run one unviewed gate run with reports to clear earlier unviewed rows.
    6. Enforce only after `mutation-state/full` exists.

    Also say that upgrading the harness means re-copying the template: the upgrade PR runs cold and green-pending, and main pays one full run.
11. **The gate.** The gate reads `<stateDir>/incremental.json` (not Stryker's `reports/mutation/mutation.json`) and every report on gated runs. Commit before re-running for `pr-ready`. State that the gate's freshness findings are delivered by the metareview gate in the same 0.13.0 release, and link `--mutation-report`'s help text.
12. **Known residuals** (spec §8).
13. **External-trial checklist.**
    - Measure the ≤ 2 min p50.
    - Measure the share of PR runs that end pending (or pay a sweep under `full-on-global`).
    - Confirm on real GitHub Actions that `full` runs after a red `incremental-main`.
    - Confirm the URL-scoped auth header matches the remote.
14. **Troubleshooting.** `break-lock`, and reading the step summary. `MUTATION_ALLOW_TMP_STATE` and `MUTATION_TEST_DISABLE_RESIDUAL` exist for the harness's own tests only.

- [ ] **Step 2: Link and note**

- `USAGE.md` and `docs/quickstart.md`: one line each, linking `docs/mutation-harness.md` under mutation testing.
- `CHANGELOG.md`, Unreleased: "mutation-incremental harness template (change-driven StrykerJS runs, attested state, CI workflow); gate integration follows in 0.13.0; the ≤ 2 min bar is pending the external trial".
- `docs/ARCHITECTURE.md`: a short section, "Mutation evidence freshness". It covers where the harness lives (`templates/mutation-incremental/`), what the attestation contract is (§5.5), and that the gate judges freshness while CI enforces the project's bar.
- `docs/0.13.0-candidates.md` §1:
  - next to the anchor-pin bullet, note that the transferable idea is scoped, attested binding;
  - add the external-trial follow-up (≤ 2 min p50, and the share of PR runs ending pending);
  - add the survivor-severity follow-up (§3).
- `templates/mutation-incremental/lib/config.mjs`: above the `MUTATION_ALLOW_TMP_STATE` check, add the comment `// Test-only escape hatch (the harness's own unit tests create repositories under the OS temp dir).` (review follow-up from Plan 1a).

- [ ] **Step 3: Verify**

Run: `sh .superpowers/sdd/2026-09-23-mutation-incremental-1c-proof/cov.sh && go test -count=1 -run 'TestMutation' . && go vet ./...`
Expected: all pass. Then read `docs/mutation-harness.md` once against §9's list and tick each item.

- [ ] **Step 4: Commit**

```bash
git add docs/mutation-harness.md USAGE.md docs/quickstart.md CHANGELOG.md docs/ARCHITECTURE.md docs/0.13.0-candidates.md templates/mutation-incremental/lib/config.mjs
git commit -m "docs(mutation-incremental): harness guide, cutover, links and notes

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

## Follow-ups (recorded, not in 1c)

- A CI test replaying the planner over the committed `real/<row>/` trees, and the §7.2 manifest-hash test. The golden guard (Task 4) already pins planner semantics in CI; the replay adds real-report inputs. Worth adding once Plan 2 consumes `real/`.
- The §7.1 rows that emulate CI jobs, beyond the static test and the fake-exit tables: the catch-up variants, and a sweep during a push. They are confirmed by the external trial checklist.
- The §7.1 `gate` rows: Plan 2.
- The survivor-variant row for step 2's "new case kills a survivor", with real Stryker. Plan 1a's unit tests cover the planner side.

## Self-Review (done while writing)

- **Spec coverage in 1c:**

  | Spec section | Task |
  |---|---|
  | §5.7 workflow (jobs, permissions, cache keys, catch-up, publish, summary step) | 3 |
  | §5.7 step 5 / §11.2 step summary | 2 |
  | §11.2 routing, `pr-full`, `--pr`/`--max-minutes` in the template | 1, 3 |
  | §7.1 fixture, e2e rows, equivalence, negative control | 5, 6 |
  | §7.2 static workflow test, fake-exit tests, step-summary tests | 2, 3 |
  | §11.6 golden-output guard | 4 |
  | §9 + §11 documentation deliverables | 7 |

- **Placeholders:** the code steps are complete. The docs step lists every required statement and section rather than final prose. That is deliberate for prose, and the verification step checks it against §9.
- **Type consistency:** `routeFor(pendingOnPr, cause, pr)` is used identically in `run.mjs`, `summary.mjs` and the tests. The workflow's step names (`run`, `catch-up`, `full run`, `publish catch-up`, `summary`, `verdict`, `install`) match the static test's lookups.
- **Review Focus:** each of the five items has a named test or row.


## Git

- Base: `ec388da607cf22da263d1980298d828b63dcf9a2`
- Head: `ea56c987aa0824755e58e6f18ee4eaa89d4b8a96`
- Branch: `mutation-incremental`
- Gate effect: `gate`

## Context Profile

- Raw diff bytes: `579630`
- Filtered diff bytes: `579630`
- Risk level: `context-risk`
- Risk reasons: `DIFF_TRUNCATED`, `LARGE_DIFF`

## Context Shard Plan

- Source diff hash: `67f52303ee9b5680`
- Plan hash: `b93351e9eeedc69d`
- shard-0 (`02b65ebca17c222e`, 28212 bytes): docs/ARCHITECTURE.md, templates/mutation-incremental/github-workflow.yml, testdata/mutation-incremental-e2e/.gitignore, testdata/mutation-incremental/planner-golden.json, testdata/mutation-incremental/real/a-survivor/attestation.json — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-9b915ca9/b93351e9eeedc69d/shard-0.md`
- shard-1 (`bd7ff7696993a41f`, 33070 bytes): templates/mutation-incremental/lib/config.mjs, testdata/mutation-incremental-e2e/package.json, testdata/mutation-incremental-e2e/tests/c.test.ts, testdata/mutation-incremental/real/delete-c-test/attestation.json, testdata/mutation-incremental/real/lockfile/attestation.json, testdata/mutation-incremental/real/test-b/attestation.json — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-9b915ca9/b93351e9eeedc69d/shard-1.md`
- shard-2 (`ee83c38a82189ce1`, 30357 bytes): docs/0.13.0-candidates.md, templates/mutation-incremental/test/main.test.mjs, testdata/mutation-incremental-e2e/src/b.ts, testdata/mutation-incremental-e2e/src/e.ts, testdata/mutation-incremental/real/e-flip/incremental.json, testdata/mutation-incremental/real/e-residual/attestation.json — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-9b915ca9/b93351e9eeedc69d/shard-2.md`
- shard-3 (`f14d312d953dc406`, 29300 bytes): testdata/mutation-incremental-e2e/tests/b.test.ts, testdata/mutation-incremental/real/time-budget/incremental.json, testdata/mutation-incremental/real/types-only/incremental.json — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-9b915ca9/b93351e9eeedc69d/shard-3.md`
- shard-4 (`a9868b3a10c3ed42`, 5273 bytes): USAGE.md, templates/mutation-incremental/test/run.test.mjs, testdata/mutation-incremental-e2e/tests/e-dec.test.ts — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-9b915ca9/b93351e9eeedc69d/shard-4.md`
- shard-5 (`76b7dbb407d59823`, 47669 bytes): templates/mutation-incremental/lib/summary.mjs, testdata/mutation-incremental-e2e/src/a.ts, testdata/mutation-incremental-e2e/src/index.ts, testdata/mutation-incremental/real/a-preserving/incremental.json, testdata/mutation-incremental/real/e-flip/attestation.json, testdata/mutation-incremental/real/limits/incremental.json — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-9b915ca9/b93351e9eeedc69d/shard-5.md`
- shard-6 (`0fd11d94a467a04d`, 40323 bytes): docs/mutation-harness.md, templates/mutation-incremental/lib/main.mjs, testdata/mutation-incremental-e2e/tests/helpers/make.ts, testdata/mutation-incremental/real/e-residual/incremental.json — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-9b915ca9/b93351e9eeedc69d/shard-6.md`
- shard-7 (`a50746ff86964498`, 416 bytes): testdata/mutation-incremental-e2e/tests/index.test.ts — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-9b915ca9/b93351e9eeedc69d/shard-7.md`
- shard-8 (`a98bad4e3cde4704`, 10696 bytes): testdata/mutation-incremental-e2e/tests/helpers/fmt.ts, testdata/mutation-incremental/real/types-only/attestation.json — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-9b915ca9/b93351e9eeedc69d/shard-8.md`
- shard-9 (`a3b47f7c08b97f90`, 59985 bytes): testdata/mutation-incremental-e2e/package-lock.json — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-9b915ca9/b93351e9eeedc69d/shard-9.md`
- shard-9-2 (`da9b22fc3d2a483b`, 59979 bytes): testdata/mutation-incremental-e2e/package-lock.json — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-9b915ca9/b93351e9eeedc69d/shard-9-2.md`
- shard-9-3 (`0fcf0ad6388aa71a`, 49313 bytes): testdata/mutation-incremental-e2e/package-lock.json, testdata/mutation-incremental/real/a-survivor/incremental.json, testdata/mutation-incremental/real/e-flip-control/incremental.json, testdata/mutation-incremental/real/limits/attestation.json — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-9b915ca9/b93351e9eeedc69d/shard-9-3.md`
- shard-9-4 (`db3aaa3a3adbfcb3`, 20813 bytes): tests/e2e-mutation-incremental.mjs, tests/e2e-mutation-incremental.sh — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-9b915ca9/b93351e9eeedc69d/shard-9-4.md`
- shard-a (`30ae1eca3e407663`, 23162 bytes): docs/quickstart.md, templates/mutation-incremental/test/summary.test.mjs, testdata/mutation-incremental-e2e/stryker.config.json, testdata/mutation-incremental-e2e/vitest.config.ts, testdata/mutation-incremental/real/delete-c-test/incremental.json — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-9b915ca9/b93351e9eeedc69d/shard-a.md`
- shard-b (`19120e5df6755b83`, 1599 bytes): templates/mutation-incremental/test/deferrals.test.mjs — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-9b915ca9/b93351e9eeedc69d/shard-b.md`
- shard-c (`621c99763c9fc8e5`, 635 bytes): testdata/mutation-incremental-e2e/tests/a.test.ts — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-9b915ca9/b93351e9eeedc69d/shard-c.md`
- shard-d (`e904830968d0073e`, 54294 bytes): testdata/mutation-incremental-e2e/README.md, testdata/mutation-incremental/real/a-preserving/attestation.json, testdata/mutation-incremental/real/full/attestation.json, testdata/mutation-incremental/real/lockfile/incremental.json, testdata/mutation-incremental/real/test-b/incremental.json — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-9b915ca9/b93351e9eeedc69d/shard-d.md`
- shard-e (`fd6dad821251a3d1`, 57827 bytes): mutationworkflow_test.go, templates/mutation-incremental/lib/run.mjs, testdata/mutation-incremental-e2e/mutation-incremental.json, testdata/mutation-incremental-e2e/src/c.ts, testdata/mutation-incremental-e2e/src/limits.ts, testdata/mutation-incremental-e2e/tests/e-inc.test.ts, testdata/mutation-incremental/real/full/incremental.json, testdata/mutation-incremental/real/helper-make/attestation.json, testdata/mutation-incremental/real/helper-make/incremental.json — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-9b915ca9/b93351e9eeedc69d/shard-e.md`
- shard-e-2 (`25e44e08bfda978c`, 10685 bytes): testdata/mutation-incremental/real/time-budget/attestation.json — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-9b915ca9/b93351e9eeedc69d/shard-e-2.md`
- shard-f (`08229cdb8535a70d`, 16022 bytes): CHANGELOG.md, templates/mutation-incremental/lib/deferrals.mjs, templates/mutation-incremental/test/golden.test.mjs, testdata/mutation-incremental-e2e/src/types.ts, testdata/mutation-incremental/real/e-flip-control/attestation.json — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-9b915ca9/b93351e9eeedc69d/shard-f.md`

## Review Manifest

- Manifest verdict: `NEEDS_REVISION`
- Source manifest hash: `b93351e9eeedc69d`
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- CHANGELOG.md
- USAGE.md
- docs/0.13.0-candidates.md
- docs/ARCHITECTURE.md
- docs/mutation-harness.md
- docs/quickstart.md
- mutationworkflow_test.go
- templates/mutation-incremental/github-workflow.yml
- templates/mutation-incremental/lib/config.mjs
- templates/mutation-incremental/lib/deferrals.mjs
- templates/mutation-incremental/lib/main.mjs
- templates/mutation-incremental/lib/run.mjs
- templates/mutation-incremental/lib/summary.mjs
- templates/mutation-incremental/test/deferrals.test.mjs
- templates/mutation-incremental/test/golden.test.mjs
- templates/mutation-incremental/test/main.test.mjs
- templates/mutation-incremental/test/run.test.mjs
- templates/mutation-incremental/test/summary.test.mjs
- testdata/mutation-incremental-e2e/.gitignore
- testdata/mutation-incremental-e2e/README.md
- testdata/mutation-incremental-e2e/mutation-incremental.json
- testdata/mutation-incremental-e2e/package-lock.json
- testdata/mutation-incremental-e2e/package.json
- testdata/mutation-incremental-e2e/src/a.ts
- testdata/mutation-incremental-e2e/src/b.ts
- testdata/mutation-incremental-e2e/src/c.ts
- testdata/mutation-incremental-e2e/src/e.ts
- testdata/mutation-incremental-e2e/src/index.ts
- testdata/mutation-incremental-e2e/src/limits.ts
- testdata/mutation-incremental-e2e/src/types.ts
- testdata/mutation-incremental-e2e/stryker.config.json
- testdata/mutation-incremental-e2e/tests/a.test.ts
- testdata/mutation-incremental-e2e/tests/b.test.ts
- testdata/mutation-incremental-e2e/tests/c.test.ts
- testdata/mutation-incremental-e2e/tests/e-dec.test.ts
- testdata/mutation-incremental-e2e/tests/e-inc.test.ts
- testdata/mutation-incremental-e2e/tests/helpers/fmt.ts
- testdata/mutation-incremental-e2e/tests/helpers/make.ts
- testdata/mutation-incremental-e2e/tests/index.test.ts
- testdata/mutation-incremental-e2e/vitest.config.ts
- testdata/mutation-incremental/planner-golden.json
- testdata/mutation-incremental/real/a-preserving/attestation.json
- testdata/mutation-incremental/real/a-preserving/incremental.json
- testdata/mutation-incremental/real/a-survivor/attestation.json
- testdata/mutation-incremental/real/a-survivor/incremental.json
- testdata/mutation-incremental/real/delete-c-test/attestation.json
- testdata/mutation-incremental/real/delete-c-test/incremental.json
- testdata/mutation-incremental/real/e-flip-control/attestation.json
- testdata/mutation-incremental/real/e-flip-control/incremental.json
- testdata/mutation-incremental/real/e-flip/attestation.json
- testdata/mutation-incremental/real/e-flip/incremental.json
- testdata/mutation-incremental/real/e-residual/attestation.json
- testdata/mutation-incremental/real/e-residual/incremental.json
- testdata/mutation-incremental/real/full/attestation.json
- testdata/mutation-incremental/real/full/incremental.json
- testdata/mutation-incremental/real/helper-make/attestation.json
- testdata/mutation-incremental/real/helper-make/incremental.json
- testdata/mutation-incremental/real/limits/attestation.json
- testdata/mutation-incremental/real/limits/incremental.json
- testdata/mutation-incremental/real/lockfile/attestation.json
- testdata/mutation-incremental/real/lockfile/incremental.json
- testdata/mutation-incremental/real/test-b/attestation.json
- testdata/mutation-incremental/real/test-b/incremental.json
- testdata/mutation-incremental/real/time-budget/attestation.json
- testdata/mutation-incremental/real/time-budget/incremental.json
- testdata/mutation-incremental/real/types-only/attestation.json
- testdata/mutation-incremental/real/types-only/incremental.json
- tests/e2e-mutation-incremental.mjs
- tests/e2e-mutation-incremental.sh

### Shards
- shard-0: docs/ARCHITECTURE.md, templates/mutation-incremental/github-workflow.yml, testdata/mutation-incremental-e2e/.gitignore, testdata/mutation-incremental/planner-golden.json, testdata/mutation-incremental/real/a-survivor/attestation.json
- shard-1: templates/mutation-incremental/lib/config.mjs, testdata/mutation-incremental-e2e/package.json, testdata/mutation-incremental-e2e/tests/c.test.ts, testdata/mutation-incremental/real/delete-c-test/attestation.json, testdata/mutation-incremental/real/lockfile/attestation.json, testdata/mutation-incremental/real/test-b/attestation.json
- shard-2: docs/0.13.0-candidates.md, templates/mutation-incremental/test/main.test.mjs, testdata/mutation-incremental-e2e/src/b.ts, testdata/mutation-incremental-e2e/src/e.ts, testdata/mutation-incremental/real/e-flip/incremental.json, testdata/mutation-incremental/real/e-residual/attestation.json
- shard-3: testdata/mutation-incremental-e2e/tests/b.test.ts, testdata/mutation-incremental/real/time-budget/incremental.json, testdata/mutation-incremental/real/types-only/incremental.json
- shard-4: USAGE.md, templates/mutation-incremental/test/run.test.mjs, testdata/mutation-incremental-e2e/tests/e-dec.test.ts
- shard-5: templates/mutation-incremental/lib/summary.mjs, testdata/mutation-incremental-e2e/src/a.ts, testdata/mutation-incremental-e2e/src/index.ts, testdata/mutation-incremental/real/a-preserving/incremental.json, testdata/mutation-incremental/real/e-flip/attestation.json, testdata/mutation-incremental/real/limits/incremental.json
- shard-6: docs/mutation-harness.md, templates/mutation-incremental/lib/main.mjs, testdata/mutation-incremental-e2e/tests/helpers/make.ts, testdata/mutation-incremental/real/e-residual/incremental.json
- shard-7: testdata/mutation-incremental-e2e/tests/index.test.ts
- shard-8: testdata/mutation-incremental-e2e/tests/helpers/fmt.ts, testdata/mutation-incremental/real/types-only/attestation.json
- shard-9: testdata/mutation-incremental-e2e/package-lock.json
- shard-9-2: testdata/mutation-incremental-e2e/package-lock.json
- shard-9-3: testdata/mutation-incremental-e2e/package-lock.json, testdata/mutation-incremental/real/a-survivor/incremental.json, testdata/mutation-incremental/real/e-flip-control/incremental.json, testdata/mutation-incremental/real/limits/attestation.json
- shard-9-4: tests/e2e-mutation-incremental.mjs, tests/e2e-mutation-incremental.sh
- shard-a: docs/quickstart.md, templates/mutation-incremental/test/summary.test.mjs, testdata/mutation-incremental-e2e/stryker.config.json, testdata/mutation-incremental-e2e/vitest.config.ts, testdata/mutation-incremental/real/delete-c-test/incremental.json
- shard-b: templates/mutation-incremental/test/deferrals.test.mjs
- shard-c: testdata/mutation-incremental-e2e/tests/a.test.ts
- shard-d: testdata/mutation-incremental-e2e/README.md, testdata/mutation-incremental/real/a-preserving/attestation.json, testdata/mutation-incremental/real/full/attestation.json, testdata/mutation-incremental/real/lockfile/incremental.json, testdata/mutation-incremental/real/test-b/incremental.json
- shard-e: mutationworkflow_test.go, templates/mutation-incremental/lib/run.mjs, testdata/mutation-incremental-e2e/mutation-incremental.json, testdata/mutation-incremental-e2e/src/c.ts, testdata/mutation-incremental-e2e/src/limits.ts, testdata/mutation-incremental-e2e/tests/e-inc.test.ts, testdata/mutation-incremental/real/full/incremental.json, testdata/mutation-incremental/real/helper-make/attestation.json, testdata/mutation-incremental/real/helper-make/incremental.json
- shard-e-2: testdata/mutation-incremental/real/time-budget/attestation.json
- shard-f: CHANGELOG.md, templates/mutation-incremental/lib/deferrals.mjs, templates/mutation-incremental/test/golden.test.mjs, testdata/mutation-incremental-e2e/src/types.ts, testdata/mutation-incremental/real/e-flip-control/attestation.json

### Manifest Blockers
- missing cross-shard result
- missing shard result for shard-0
- missing shard result for shard-1
- missing shard result for shard-2
- missing shard result for shard-3
- missing shard result for shard-4
- missing shard result for shard-5
- missing shard result for shard-6
- missing shard result for shard-7
- missing shard result for shard-8
- missing shard result for shard-9
- missing shard result for shard-9-2
- missing shard result for shard-9-3
- missing shard result for shard-9-4
- missing shard result for shard-a
- missing shard result for shard-b
- missing shard result for shard-c
- missing shard result for shard-d
- missing shard result for shard-e
- missing shard result for shard-e-2
- missing shard result for shard-f

## Changed Files

- CHANGELOG.md
- USAGE.md
- docs/0.13.0-candidates.md
- docs/ARCHITECTURE.md
- docs/mutation-harness.md
- docs/quickstart.md
- mutationworkflow_test.go
- templates/mutation-incremental/github-workflow.yml
- templates/mutation-incremental/lib/config.mjs
- templates/mutation-incremental/lib/deferrals.mjs
- templates/mutation-incremental/lib/main.mjs
- templates/mutation-incremental/lib/run.mjs
- templates/mutation-incremental/lib/summary.mjs
- templates/mutation-incremental/test/deferrals.test.mjs
- templates/mutation-incremental/test/golden.test.mjs
- templates/mutation-incremental/test/main.test.mjs
- templates/mutation-incremental/test/run.test.mjs
- templates/mutation-incremental/test/summary.test.mjs
- testdata/mutation-incremental-e2e/.gitignore
- testdata/mutation-incremental-e2e/README.md
- testdata/mutation-incremental-e2e/mutation-incremental.json
- testdata/mutation-incremental-e2e/package-lock.json
- testdata/mutation-incremental-e2e/package.json
- testdata/mutation-incremental-e2e/src/a.ts
- testdata/mutation-incremental-e2e/src/b.ts
- testdata/mutation-incremental-e2e/src/c.ts
- testdata/mutation-incremental-e2e/src/e.ts
- testdata/mutation-incremental-e2e/src/index.ts
- testdata/mutation-incremental-e2e/src/limits.ts
- testdata/mutation-incremental-e2e/src/types.ts
- testdata/mutation-incremental-e2e/stryker.config.json
- testdata/mutation-incremental-e2e/tests/a.test.ts
- testdata/mutation-incremental-e2e/tests/b.test.ts
- testdata/mutation-incremental-e2e/tests/c.test.ts
- testdata/mutation-incremental-e2e/tests/e-dec.test.ts
- testdata/mutation-incremental-e2e/tests/e-inc.test.ts
- testdata/mutation-incremental-e2e/tests/helpers/fmt.ts
- testdata/mutation-incremental-e2e/tests/helpers/make.ts
- testdata/mutation-incremental-e2e/tests/index.test.ts
- testdata/mutation-incremental-e2e/vitest.config.ts
- testdata/mutation-incremental/planner-golden.json
- testdata/mutation-incremental/real/a-preserving/attestation.json
- testdata/mutation-incremental/real/a-preserving/incremental.json
- testdata/mutation-incremental/real/a-survivor/attestation.json
- testdata/mutation-incremental/real/a-survivor/incremental.json
- testdata/mutation-incremental/real/delete-c-test/attestation.json
- testdata/mutation-incremental/real/delete-c-test/incremental.json
- testdata/mutation-incremental/real/e-flip-control/attestation.json
- testdata/mutation-incremental/real/e-flip-control/incremental.json
- testdata/mutation-incremental/real/e-flip/attestation.json
- testdata/mutation-incremental/real/e-flip/incremental.json
- testdata/mutation-incremental/real/e-residual/attestation.json
- testdata/mutation-incremental/real/e-residual/incremental.json
- testdata/mutation-incremental/real/full/attestation.json
- testdata/mutation-incremental/real/full/incremental.json
- testdata/mutation-incremental/real/helper-make/attestation.json
- testdata/mutation-incremental/real/helper-make/incremental.json
- testdata/mutation-incremental/real/limits/attestation.json
- testdata/mutation-incremental/real/limits/incremental.json
- testdata/mutation-incremental/real/lockfile/attestation.json
- testdata/mutation-incremental/real/lockfile/incremental.json
- testdata/mutation-incremental/real/test-b/attestation.json
- testdata/mutation-incremental/real/test-b/incremental.json
- testdata/mutation-incremental/real/time-budget/attestation.json
- testdata/mutation-incremental/real/time-budget/incremental.json
- testdata/mutation-incremental/real/types-only/attestation.json
- testdata/mutation-incremental/real/types-only/incremental.json
- tests/e2e-mutation-incremental.mjs
- tests/e2e-mutation-incremental.sh

## Diff

````diff
diff --git a/CHANGELOG.md b/CHANGELOG.md
index 3ed59db..7ee14c1 100644
--- a/CHANGELOG.md
+++ b/CHANGELOG.md
@@ -1,5 +1,21 @@
 # Changelog
 
+## Unreleased
+
+### Added
+
+- **mutation-incremental harness template (`templates/mutation-incremental/`).** Change-driven
+  StrykerJS runs: the harness plans from content digests (never dates), re-runs only what a change
+  can affect, and records what it verified in an attested state (`attestation.json` +
+  `incremental.json`). Work it cannot re-verify within its budget or time limit is deferred to a
+  full run, visibly. Commands: `plan`, `run`, `seed`, `fetch-state`, `publish-state`, `break-lock`
+  and `summary`. A GitHub workflow template keeps state on `mutation-state/{inc,full}` branches
+  (cache as a fast path), with PR routing (`pendingOnPr`), a per-view project verifier, and read-time
+  views. Proven locally against real StrykerJS 10 and Vitest 4, including an equivalence check that
+  every incremental kill is a fresh kill. Gate integration (evidence freshness) follows in 0.13.0.
+  The ≤ 2 min p50 bar for a one-line edit is pending the external trial. Guide:
+  `docs/mutation-harness.md`.
+
 ## 0.12.0 - 2026-09-10
 
 ### Added
diff --git a/USAGE.md b/USAGE.md
index 899ebfb..8df8793 100644
--- a/USAGE.md
+++ b/USAGE.md
@@ -156,6 +156,9 @@ Coverage tells you what *ran*; it does not tell you what's *tested*. metareview
   `metareview review task-done <target> --base <ref> --mutation-report <file>` (gremlins JSON for Go; the
   Stryker schema for TS; mutmut for Python). A run whose mutation summary is dishonest (e.g. timeouts
   scored as kills) is refused rather than trusted.
+- **Change-driven StrykerJS runs.** For TypeScript/JavaScript projects, the mutation-incremental harness
+  re-runs only what a change can affect and attests exactly what it verified, so a one-line edit does not
+  cost a full mutation run. See [docs/mutation-harness.md](docs/mutation-harness.md).
 - **Coverage gate.** The repo ships a Go-native coverage gate (`make cover`) that holds critical packages
   at 100% of statements and floors the rest, so coverage can only ratchet up.
 
diff --git a/docs/0.13.0-candidates.md b/docs/0.13.0-candidates.md
index 05a83f3..3635483 100644
--- a/docs/0.13.0-candidates.md
+++ b/docs/0.13.0-candidates.md
@@ -46,6 +46,13 @@ hand-rolled harness, not a metareview requirement.
   digests of the files it *actually mutated* and the tests Stryker's per-test coverage
   *actually selected*, never a whole-tree manifest.
 - Prefer / document metareview's anchor-pin model for project-generated harnesses.
+  The transferable idea is scoped, attested binding: pins verify targeted fix claims, not a
+  mutation score. For StrykerJS the harness template (`docs/mutation-harness.md`) implements the
+  scoped binding.
+- Follow-up: the external trial measures a one-line edit's incremental run (≤ 2 min p50) and the
+  share of PR runs ending pending (or, under `full-on-global`, paying a sweep; bar ≤ 20%).
+- Follow-up: the severity of existing survivor findings (medium, counted as warnings by
+  `classForCount`) is a separate ticket (spec §3).
 - Ship a scaffold generator (or template) for project verification harnesses so teams do
   not hand-roll a whole-tree-bound one.
 - Where whole-tree binding is unavoidable, add **incremental invalidation**: re-run only
diff --git a/docs/ARCHITECTURE.md b/docs/ARCHITECTURE.md
index e685cfe..912ae6c 100644
--- a/docs/ARCHITECTURE.md
+++ b/docs/ARCHITECTURE.md
@@ -233,6 +233,13 @@ list below is illustrative, omitting e.g. `judge`, `gate`, `converge`, `export`)
   30`; `--workers 8 --timeout-coefficient 120` is a faster config with a few flaky timeouts. Timeouts are
   recompile contention, not real survivors. 100% line coverage still leaves killable mutants — construct the
   distinguishing test before calling a survivor equivalent.
+- **Mutation evidence freshness (StrykerJS):** the change-driven harness is a zero-dependency Node template in
+  `templates/mutation-incremental/` that projects copy to `tools/mutation-incremental/`. The repository owns
+  what runs; metareview owns the attestation contract (`<stateDir>/attestation.json`, spec §5.5) and the
+  glob dialect (vectors in `testdata/mutation-incremental/`). CI enforces the project's own bar (threshold
+  and/or verifier); the gate judges only freshness. The template's Node suite runs under `go test` at 100%
+  coverage (`mutationtemplate_test.go`), and its workflow has a static test (`mutationworkflow_test.go`). The
+  real-Stryker proof is local-only (`tests/e2e-mutation-incremental.mjs`). Guide: `docs/mutation-harness.md`.
 - **Review-first, then bots:** run metareview's adversarial review BEFORE opening the PR, so CodeRabbit/Cursor
   measure only the *residual* we missed — the recall yardstick. Reviewing after the PR opens confounds it.
 
diff --git a/docs/mutation-harness.md b/docs/mutation-harness.md
new file mode 100644
index 0000000..17d3b2d
--- /dev/null
+++ b/docs/mutation-harness.md
@@ -0,0 +1,353 @@
+# Change-driven mutation testing (StrykerJS)
+
+metareview ships a harness template that runs StrykerJS only for what a change can affect, and records
+exactly what it verified in an attested state. The gate then judges whether mutation evidence is still
+fresh. Validity depends only on what changed (content digests), never on dates.
+
+Status in 0.13.0: harness plus freshness gate. The external trial's pass bar — a one-line edit's
+incremental run at ≤ 2 min p50 — is still pending (see the checklist at the end).
+
+Design and rationale: `docs/superpowers/specs/2026-09-23-mutation-incremental-design.md`.
+
+## 1. What it does
+
+- `run --mode incremental` plans from the changes since the last attested state and runs Stryker at
+  most twice:
+  - an unforced invocation over the files whose kills may have changed (including edited files);
+  - a `--force` invocation over edited files and the mutants their covering tests could affect.
+- Anything it cannot re-verify within its budget or time limit is recorded as a **deferral**. A
+  deferral means a full run is pending; the state carries it until a full run or a clean catch-up
+  clears it.
+- CI keeps the state on two git branches (`mutation-state/inc`, `mutation-state/full`), with
+  `actions/cache` as a fast path. A full run happens only when something was deferred. There are no
+  scheduled jobs.
+
+## 2. Set up
+
+1. Copy the template into the repository, without its `test/` directory:
+
+   ```bash
+   mkdir -p tools/mutation-incremental
+   cp -R <metareview>/templates/mutation-incremental/cli.mjs <metareview>/templates/mutation-incremental/lib tools/mutation-incremental/
+   cp <metareview>/templates/mutation-incremental/mutation-incremental.example.json mutation-incremental.json
+   cp <metareview>/templates/mutation-incremental/github-workflow.yml .github/workflows/mutation.yml
+   ```
+
+2. Configure Stryker (JSON config, StrykerJS 10.x, installed in `node_modules`):
+   - `"testRunner": "vitest"`;
+   - `"disableBail": true`, so `killedBy` lists every killing test and does not depend on test order;
+   - a `mutate` array equal to the harness config's `mutate`;
+   - `ignorePatterns` containing the state dir (`".mutation"`);
+   - no `"inPlace": true`.
+3. Gitignore the state dir (`.mutation/`). Pin Node with a tracked `.nvmrc` that both
+   `setup-node` and developers use. The file is a global input, so a laptop with a different Node
+   never pollutes CI state.
+4. Edit the workflow's project-setup steps (Node, `npm ci`, services). Edit the `push` branch if the
+   default branch is not `main`. Optionally add `paths-ignore` mirroring `ignore`, but keep the
+   `full` job's `needs` chain intact.
+5. Make **both** `incremental-pr` and `pr-full` required checks. A skipped job satisfies a required
+   check.
+6. Repository rulesets that restrict updates or force-pushes on all branches must allow both on
+   `mutation-state/*`. Limit the bypass to GitHub Actions.
+7. The workflow sets `MUTATION_STATE_TOKEN: ${{ github.token }}` on the `fetch-state` and
+   `publish-state` steps only. Keep `github.token` rather than a personal access token: a PAT's
+   pushes trigger other workflows. Local runs need no token.
+
+## 3. Configuration (`mutation-incremental.json`)
+
+All keys below are required unless marked optional. The values shown are the defaults of the
+example file.
+
+| Key | Meaning |
+|---|---|
+| `schemaVersion` | `1` |
+| `stateDir` | `.mutation` — a dedicated directory inside the repo, not under the OS temp dir, not matching `mutate`/`test`/`support` |
+| `stryker` | `command` (argv, no shell), `configFile` (JSON), `extraArgs` |
+| `mutate`, `test`, `support`, `global`, `ignore` | glob lists (below) |
+| `aliases` | specifier prefix → repo path prefix, e.g. `{"@app/": "src/"}` |
+| `runtime` | `commands` (argv whose stdout digest is an input) and `env` (variable names, stored as sha256 digests only) |
+| `budget` | `maxForcedShare` (0.25), `maxForcedMutants` (null), `maxMinutesPerInvocation` (20) |
+| `residual.mode` | `bounded` (default; the budget applies), `strict` (no budget), `off` (records `residual off` deferrals instead of forcing) |
+| `editedFiles` (optional) | `residual` (default: an edited file goes whole into the unforced scope; its changed lines and its coverage closure are forced) or `whole` (the edited file is forced whole) |
+| `pendingOnPr` (optional) | `allow` (default), `full` or `full-on-global` (§6) |
+| `allowBail` (optional) | allow a Stryker config without `disableBail` (refused with `editedFiles: "residual"`) |
+| `verify` (optional) | `{"command": [argv], "timeoutMinutes": n or null}` — the project's own bar (§8); `timeoutMinutes` is required when `pendingOnPr` is not `allow` |
+| `views` (optional) | `{"command": [argv]}` printing `{"name": [patterns]}`, or `{"inline": {...}}` (§8) |
+
+Sizing:
+
+- `maxMinutesPerInvocation` is a safety ceiling, not a target. Size the budget and the ceiling so a
+  typical one-line change stays well under the ≤ 2 min bar.
+- Two invocations plus setup must fit the CI job timeout. Setup, the catch-up and the full run must
+  fit the `full` job's timeout together.
+- GitHub-hosted runners cap a job at 360 minutes. Above roughly 13 minutes of verifier time per view,
+  use a single-pass verifier (§8).
+
+## 4. What goes where
+
+- **Categories.** The first list that matches, in the order `global`, `support`, `test`, `mutate`,
+  decides a path's category. Paths matching `ignore` are excluded. Anything else is `unclassified`.
+  The config file and `tools/mutation-incremental/**` are always `global`.
+- **What each category costs.**
+  - A `global` change defers everything to a full run.
+  - A `support` change forces the kills of its importer tests.
+  - An `unclassified` change forces the kills of its importer tests, or defers everything when
+    nothing imports it.
+  - Add docs, CI config and editor files to `ignore`.
+- **Meta-artifacts.**
+  - Files that tests import or read belong in `support`; tooling-only JSON and docs in `ignore`.
+  - `tests/**` is `test`; helpers and factories are `support`.
+  - `global` is only for inputs everything depends on: lockfile, Stryker/Vitest/TS config,
+    migrations, `.nvmrc`.
+- **Aliases and open importers.** A relative or alias specifier that reaches no file in the
+  repository, and is not an installed package or a Node builtin, makes its file an **open
+  importer**. Every test reaching an open importer is treated as reaching everything. `plan` lists
+  open importers and their specifiers, so you can add `aliases`.
+  - A gitignored generated module imported by a relative path makes its importers open importers.
+  - Files read by path at runtime, and computed `import(x)`, are invisible to the import graph.
+- **Barrels and constants.** Editing a widely imported barrel or constants file often exceeds the
+  budget and defers to a full run. Prefer `import type` (or `verbatimModuleSyntax`) for type-only
+  imports.
+- **Paths Stryker cannot take.** A path containing `, { } [ ] ( ) ! * ? :` cannot be passed to
+  `--mutate`, and `plan`/`run` exit 2 naming it (e.g. Next.js `app/(auth)/[id]/page.tsx`). Keep such
+  directories out of `mutate`.
+
+## 5. Commands, exit codes and outputs
+
+```bash
+node tools/mutation-incremental/cli.mjs plan [--also-state <dir>]...
+node tools/mutation-incremental/cli.mjs run --mode incremental|full [--also-state <dir>]... [--pr [--max-minutes <m>]]
+node tools/mutation-incremental/cli.mjs seed --from <report>... [--replace]
+node tools/mutation-incremental/cli.mjs fetch-state [--remote <name>]
+node tools/mutation-incremental/cli.mjs publish-state --kind inc|full [--remote <name>]
+node tools/mutation-incremental/cli.mjs break-lock
+node tools/mutation-incremental/cli.mjs summary --job pr|pr-full|main|full [--exit-code <n>] [--pr]
+```
+
+- `plan` prints the plan as JSON and changes nothing.
+- `publish-state` is CI-only. It publishes only a usable state; otherwise it says why and exits 0.
+- `summary` writes the step summary.
+
+Every command runs from the git top-level, and every command prints one summary line on stderr.
+
+Exit codes:
+
+| Code | Meaning |
+|---|---|
+| 0 | ok (deferrals included) |
+| 1 | ok, but the score is below `thresholds.break` or the verifier failed (the state is committed) |
+| 2 | config, usage or validation error |
+| 3 | lock held (`break-lock` clears a lock left by a crashed run on another machine) |
+| 4 | engine failure — nothing committed; re-running retries the same plan |
+| 130 | interrupted — nothing committed |
+
+When both apply, the higher-priority code wins: 2 > 3 > 130 > 4 > 1 > 0.
+
+With `GITHUB_OUTPUT` set, `run` appends:
+
+- on every exit path: `pending_full` and `exit_code`;
+- on exit 0 or 1, also:
+  - `pending_cause`: `global`, `unreachable`, `other`, `timeout` or `none`;
+  - `pending_causes`: single-line JSON of the counted deferrals, at most 50 entries, then
+    `{"more": n}`;
+  - `route`: `verdict`, `sweep` or `fail` (§6).
+
+## 6. How CI works
+
+**Jobs.**
+
+| Job | When | What it does |
+|---|---|---|
+| `incremental-pr` | every PR push | Fetches main's state, then runs incremental with `--pr` over its own cached state and main's. Saves its cache, writes the summary, and decides by exit code and route. |
+| `pr-full` | after `incremental-pr`, only when the route is `sweep` | Runs `run --mode full` (the sweep); red unless it exits 0. Never has a token, never publishes. |
+| `incremental-main` | every push to main | Same as the PR job without `--pr`; publishes `mutation-state/inc`. |
+| `full` | after `incremental-main`, when `pending_full` is true | First a **catch-up** incremental run from the freshest published state. If that clears pending, it publishes `mutation-state/full` and stops; otherwise it runs a full run and publishes it. |
+
+**State branches.**
+
+- `mutation-state/inc` is main's latest incremental state.
+- `mutation-state/full` is "main's latest state without deferrals". It may carry
+  `mode: incremental` with an older `lastFullAt`.
+- Each branch is a single parentless commit, force-pushed, and has one writer, so nothing expires.
+- The cache is a fast path. A miss falls back to the branches, never to a cold start.
+
+**Pending on PRs** (`pendingOnPr`). What happens when a PR run ends with a counted deferral:
+
+| Mode | Cause `global` | `timeout` | `unreachable` | `other` |
+|---|---|---|---|---|
+| `allow` | green, pending | green, pending | green, pending | green, pending |
+| `full` | sweep | sweep | sweep | sweep |
+| `full-on-global` | sweep | sweep | **PR fails** | sweep |
+
+- A deferral is **inherited**, not counted, only when main's published state carries the identical
+  deferral about inputs the PR has not changed. Inherited deferrals never route or fail anything.
+  The summary lists them as "inherited from main, cleared by main's full run".
+- Under `full-on-global`, a PR's verdict never depends on runner speed within the harness's own
+  limits: a timeout only makes the PR slower (it pays the sweep). The remaining speed-bounded reds
+  are the verifier's timeout, setup, and the `pr-full` job timeout. An overrun is red and
+  re-runnable, never silently green.
+
+**Sizing under `pendingOnPr: "full"` or `"full-on-global"`.**
+
+- The workflow variable `MUTATION_PR_MAX_MINUTES` (`<m>`, default 60) replaces
+  `maxMinutesPerInvocation` on routed PR runs. Size it so a timeout is rare. A timeout costs a sweep,
+  not a failure.
+- `incremental-pr`'s `timeout-minutes` must be at least:
+
+  setup + 2 × (`<m>` + 1) + max(1, views) × (`verify.timeoutMinutes` + 0.5)
+
+  This keeps a job-level timeout from pre-empting the harness and turning a PR red because of runner
+  speed. The template's 150 fits the defaults (60 minutes, one view, no verifier); raise it as views
+  and verifier time grow.
+- `pr-full`'s `timeout-minutes` must be at least setup + one full run + max(1, views) ×
+  `verify.timeoutMinutes`. Size it like main's `full` job. An overrun is red and re-runnable.
+- Under `full`, every PR that exceeds `<m>` pays a sweep.
+- A PR that keeps timing out pays a sweep on every push until `<m>` is raised.
+- A force-push during a running sweep lets that sweep finish (its state is reused by the next push)
+  and queues one more sweep if the new commit also needs one.
+- A PR rebased (or not rebased) during main's pending window may pay a sweep when its lockfile digest
+  differs from main's. That is correct, just slower.
+- A push whose `incremental-pr` finishes before a running sweep saves its cache re-plans from the
+  older state, and may pay one more sweep.
+- Watch main's `full` job: while it is red, PRs stay green on inherited pending.
+
+**What things cost.**
+
+- A change to a global input costs one full run on main. That includes a dependency bump, and
+  tuning-only config edits (`thresholds.break`, `budget`, `ignore`).
+- A threshold break turns every PR red until main is fixed. The step summary shows main's score, so
+  a PR that is red only because main is below the threshold says so.
+- A PR that changes a global input (e.g. a lockfile bump), or carries its own budget or time
+  deferral, re-runs its delta from main's state on each push.
+- Main re-runs each merged PR's delta: PR caches are per PR.
+- Re-running an old main workflow publishes an older, still valid state.
+- A cancelled job commits nothing.
+- A red `incremental-main` on a global change triggers no full run for that commit; re-run it.
+- Repeated red `full` jobs (timeout or exit 4) never clear on their own. A `full` job that hits its
+  timeout will not clear by itself either: raise the timeout or narrow `mutate`.
+- `incremental-pr` is not a merge-queue check unless `merge_group` is added to the triggers.
+- Private repositories without fork Actions run no PR job for forks.
+
+## 7. Local use
+
+```bash
+node tools/mutation-incremental/cli.mjs fetch-state
+node tools/mutation-incremental/cli.mjs run --mode incremental --also-state .mutation/remote/full --also-state .mutation/remote/inc
+```
+
+- A new worktree can warm-start from the first checkout:
+  `run --mode incremental --also-state <first checkout>/.mutation`.
+- `seed --from <full Stryker report>` bootstraps local state. Its kills stay pending until a full
+  run. Seeding helps local runs only: CI's first run is still a full run.
+- A local `run --mode full` clears pending kills locally.
+- A local run never turns a PR green: no local state is imported into CI. CI's PR state is not gate
+  evidence either, so run the harness locally before `pr-ready`.
+
+## 8. Verifier and views
+
+**Verifier.** CI enforces the project's bar, a threshold and/or a verifier; the gate judges only
+freshness.
+
+- After `run` commits state, the verifier runs once per view (once without views), without a shell,
+  in its own process group, with these variables:
+  - `MUTATION_REPORT` and `MUTATION_ATTESTATION` (absolute paths);
+  - per view, `MUTATION_VIEW`, `MUTATION_VIEW_PATTERNS` (JSON) and `MUTATION_VIEW_FILES` (JSON);
+  - `MUTATION_RUN_KIND`: `pr` on routed PR runs, `other` otherwise.
+- Any failure is exit 1, and stderr names the view. That includes a non-zero exit, a command that
+  cannot be spawned, and exceeding `timeoutMinutes`.
+- The verifier does not run when nothing was committed, and `seed` never runs it.
+- Kills covered by a deferral still show their previous status in the report. The attestation marks
+  each deferral `inherited`, and each view summary splits pending kills into `pendingCounted` and
+  `pendingInherited`.
+- A verifier that enforces a survivor bar treats **counted** pending kills as unverified on PR runs
+  (`MUTATION_RUN_KIND=pr`) and ignores inherited ones. On other runs, pending never fails it.
+- Identify waivers by `(file, mutatorName, location, replacement)` plus the file's digest; mutant
+  ids are only meaningful inside one report.
+- When per-view invocations are costly, write a single-pass verifier that loops over the views in
+  the attestation.
+
+**Views.** Views are named, possibly overlapping pattern sets over `mutate`.
+
+- They are read-time only: they never change planning or validity, and a view-map change invalidates
+  no kill.
+- `run` and `plan` exit 2 before any invocation when:
+  - a view matches a non-`mutate` file;
+  - a name is empty or duplicated;
+  - a `mutate` file is in no view. Assign a new file to a view before it has mutants.
+- Views may overlap; per-view counts are never summed.
+
+## 9. Security and privacy
+
+- The state files, caches, job summaries and logs embed sources and test output. On public
+  repositories anyone can read them, including fork PRs. Never put secrets in them:
+  - tests must not print environment or config values;
+  - `runtime.env` stores digests only, but never list secrets there;
+  - verifier output lands in public logs and summaries.
+- The state branches are as trusted as the repository's write collaborators.
+- The token reaches git only as an HTTP header, through `GIT_CONFIG_*` (never argv), and is masked
+  first under GitHub Actions.
+
+## 10. Upgrading and cutover
+
+1. Adopt the harness with one state and your views.
+2. Keep `pendingOnPr: "allow"` until `mutation-state/full` exists. Before then every PR run is cold,
+   and under `full` or `full-on-global` would pay a full sweep.
+3. The first push to main runs the full sweep and publishes `mutation-state/full`.
+4. Until then, existing reports show as `unattested` (advisory).
+5. Run the gate once with the reports and no `--mutation-view`, to clear earlier unviewed findings.
+6. Enforce, and switch `pendingOnPr` if wanted, only after `mutation-state/full` exists.
+
+Upgrading the harness means re-copying the template. The upgrade PR runs cold and green-pending,
+and main pays one full run (`stateVersion` changes make older states cold).
+
+## 11. The gate
+
+- Pass `<stateDir>/incremental.json` — not Stryker's `reports/mutation/mutation.json` — and every
+  report on gated runs.
+- Commit before re-running the harness for `pr-ready`.
+- The metareview gate's freshness findings (stale, pending, unattested), `--mutation-view`, and the
+  enforcement modes ship in the same 0.13.0 release; see `metareview review --help` for
+  `--mutation-report`.
+- An enforced stale finding clears only on a later run that supplies reports, or by override.
+  Overrides on pending or unattested findings last only for that report. Every mutation override is
+  tied to the mode in its fingerprint, and an override on one view does not cover the same cause
+  under another view.
+- Default enforcement is planned for 0.14.0.
+
+## 12. Known residuals
+
+- Mutant-free code inside a changed module that also has mutants (a constant next to functions) can
+  affect tests outside its mutants' coverage. The residual closure reads that coverage, so it misses
+  tests that reach only the constant. A changed module with no mutants at all uses its importer
+  tests instead.
+- A new module with no test yet produces no mutants until a test reaches it.
+- Deleting a module together with its own test, with nothing else changed, leaves its old survivors
+  in the report until the next run that invokes Stryker.
+- Computed specifiers and files read by path are invisible to the import graph.
+- State outside the repository, beyond the declared `runtime` inputs, is not tracked.
+- The gate trusts the harness's attestation (accident-level, not tamper-proof).
+- Stryker semantics are version-specific: the harness requires StrykerJS 10.x with the Vitest
+  runner. Yarn Plug'n'Play is unsupported.
+- `residual` edited-file handling reaches same-file kills only through the changed hunks' covering
+  tests. Full runs backstop it.
+
+## 13. External-trial checklist
+
+- Measure a one-line edit's incremental run: the pass bar is ≤ 2 min p50.
+- Measure the share of PR runs that end pending. Under `full-on-global`, measure the share that pay a
+  sweep; the bar is ≤ 20%.
+- Confirm on real GitHub Actions that `full` runs after a red `incremental-main` (outputs of a failed
+  job).
+- Confirm that the URL-scoped auth header matches the remote.
+
+## 14. Troubleshooting
+
+- **Exit 3:** another run holds `<stateDir>/lock`. A lock left by a crashed run on the same machine
+  is replaced automatically; `break-lock` removes one left on another machine.
+- **"no usable state":** there is no attestation, or it is from another `stateVersion`. Incremental
+  runs execute nothing until a full run (or `seed`/`fetch-state`) provides a state.
+- **Read the step summary.** It shows the exit code, the pending causes with their paths, inherited
+  deferrals, the score against the threshold, and (on PRs) main's score and when
+  `mutation-state/full` last completed.
+- `MUTATION_ALLOW_TMP_STATE` and `MUTATION_TEST_DISABLE_RESIDUAL` exist for the harness's own tests
+  only.
diff --git a/docs/quickstart.md b/docs/quickstart.md
index aae8da9..a7cbb35 100644
--- a/docs/quickstart.md
+++ b/docs/quickstart.md
@@ -62,6 +62,9 @@ engine errors) collapse into one blocking finding, because they are one fact rat
 defects — the run measured less than it claims. metareview computes the score itself and counts
 undecided against it rather than trusting the engine's summary.
 
+For StrykerJS projects, the change-driven harness keeps that report fresh without full reruns on
+every edit: [docs/mutation-harness.md](mutation-harness.md).
+
 
 `artifact` creates an incomplete review scaffold for specs, plans, and docs. The command exits nonzero while the scaffold is still `NOT_REVIEWED`; complete every required reviewer row and update the verdict before treating the artifact as reviewed. Artifact review runs the ten required lenses as parallel subagents by default. Use `in-session-emulated` only when subagents are unavailable or the human explicitly requests no delegation, and state that the review is not independently adversarial and is weaker evidence. Use `--scaffold-only` only when scaffold creation itself is the intended action. Completed logs classify findings into `## Blocking Findings`, `## Advisory Findings` (real, important, not defects — gated by stated consequence, steel-man rebuttal, and convergence, then one staff-bar filter; blocking/defect findings never pass through it), `## Follow-up Findings`, and `## Warnings`. `task-done` runs after a local task or chunk claims done. `epic-ready` runs when child tasks are complete. `pr-ready` runs before push or merge readiness. `learn --post-merge` runs after confirmed PR merge.
 
diff --git a/mutationworkflow_test.go b/mutationworkflow_test.go
new file mode 100644
index 0000000..ca71186
--- /dev/null
+++ b/mutationworkflow_test.go
@@ -0,0 +1,256 @@
+package metareview
+
+import (
+	"os"
+	"os/exec"
+	"slices"
+	"strings"
+	"testing"
+
+	"gopkg.in/yaml.v3"
+)
+
+// The workflow template's static contract (spec §5.7, §7.2, §11.2) and its two verdict shell steps,
+// executed with fake exit codes.
+
+type wfStep struct {
+	Name            string            `yaml:"name"`
+	ID              string            `yaml:"id"`
+	If              string            `yaml:"if"`
+	Uses            string            `yaml:"uses"`
+	Run             string            `yaml:"run"`
+	With            map[string]any    `yaml:"with"`
+	Env             map[string]string `yaml:"env"`
+	ContinueOnError bool              `yaml:"continue-on-error"`
+}
+
+type wfJob struct {
+	If          string            `yaml:"if"`
+	Needs       string            `yaml:"needs"`
+	Permissions map[string]string `yaml:"permissions"`
+	Concurrency struct {
+		Group            string `yaml:"group"`
+		CancelInProgress bool   `yaml:"cancel-in-progress"`
+	} `yaml:"concurrency"`
+	Outputs map[string]string `yaml:"outputs"`
+	Env     map[string]string `yaml:"env"`
+	Steps   []wfStep          `yaml:"steps"`
+}
+
+type workflow struct {
+	On   map[string]any    `yaml:"on"`
+	Env  map[string]string `yaml:"env"`
+	Jobs map[string]wfJob  `yaml:"jobs"`
+}
+
+const workflowPath = "templates/mutation-incremental/github-workflow.yml"
+
+func loadWorkflow(t *testing.T) (workflow, string) {
+	t.Helper()
+	src, err := os.ReadFile(workflowPath)
+	if err != nil {
+		t.Fatal(err)
+	}
+	var wf workflow
+	if err := yaml.Unmarshal(src, &wf); err != nil {
+		t.Fatal(err)
+	}
+	return wf, string(src)
+}
+
+func stepIndex(job wfJob, match func(wfStep) bool) int {
+	return slices.IndexFunc(job.Steps, match)
+}
+
+func named(name string) func(wfStep) bool { return func(s wfStep) bool { return s.Name == name } }
+
+func TestMutationWorkflowStatic(t *testing.T) {
+	wf, src := loadWorkflow(t)
+	if len(wf.On) != 2 || wf.On["push"] == nil || !mapHas(wf.On, "pull_request") {
+		t.Errorf("triggers must be push and pull_request only: %v", wf.On)
+	}
+	if !strings.Contains(src, "\npermissions: {}\n") {
+		t.Error("workflow-level permissions must be {}")
+	}
+	if strings.Contains(src, "--threshold") {
+		t.Error("no --threshold flag anywhere (decision 15)")
+	}
+	wantPerms := map[string]string{"incremental-pr": "read", "pr-full": "read", "incremental-main": "write", "full": "write"}
+	if len(wf.Jobs) != len(wantPerms) {
+		t.Fatalf("jobs = %d, want %d", len(wf.Jobs), len(wantPerms))
+	}
+	if _, ok := wf.Env["MUTATION_STATE_TOKEN"]; ok {
+		t.Error("MUTATION_STATE_TOKEN must not be workflow-level")
+	}
+	var savedKeys []string
+	var cachePaths []string
+	for name, job := range wf.Jobs {
+		if got := job.Permissions["contents"]; got != wantPerms[name] || len(job.Permissions) != 1 {
+			t.Errorf("%s: permissions %v, want contents: %s only", name, job.Permissions, wantPerms[name])
+		}
+		if len(job.Env) != 0 {
+			t.Errorf("%s: no job-level env (the token is per step)", name)
+		}
+		if !strings.HasPrefix(job.Steps[0].Uses, "actions/checkout@") {
+			t.Errorf("%s: checkout must be the first step", name)
+		}
+		install := stepIndex(job, named("install"))
+		for i, s := range job.Steps {
+			if strings.HasPrefix(s.Uses, "actions/checkout@") && (s.With["fetch-depth"] != 0 || s.With["persist-credentials"] != false) {
+				t.Errorf("%s: checkout needs fetch-depth: 0 and persist-credentials: false", name)
+			}
+			if strings.Contains(s.Run, "${{") {
+				t.Errorf("%s/%s: pass outputs through env:, never ${{ }} inside run:", name, s.Name)
+			}
+			remote := strings.Contains(s.Run, " fetch-state") || strings.Contains(s.Run, " publish-state")
+			if _, has := s.Env["MUTATION_STATE_TOKEN"]; has != remote {
+				t.Errorf("%s/%s: MUTATION_STATE_TOKEN only (and always) on fetch-state/publish-state steps", name, s.Name)
+			}
+			if remote && s.ContinueOnError {
+				t.Errorf("%s/%s: publish/fetch steps are never continue-on-error", name, s.Name)
+			}
+			if strings.Contains(s.Run, " fetch-state") && i < install {
+				t.Errorf("%s: checkout and project setup precede fetch-state", name)
+			}
+			if strings.Contains(s.Run, " run --mode") && !s.ContinueOnError {
+				t.Errorf("%s/%s: run steps are continue-on-error (a later step decides)", name, s.Name)
+			}
+			if strings.HasPrefix(s.Run, `node "$MUTATION_CLI" summary`) && s.If != "always()" {
+				t.Errorf("%s/%s: the summary is if: always()", name, s.Name)
+			}
+			if strings.Contains(s.Run, `"$MUTATION_CLI" summary`) && !strings.Contains(s.Run, "|| true") {
+				t.Errorf("%s/%s: the summary never fails the job", name, s.Name)
+			}
+			if strings.HasPrefix(s.Uses, "actions/cache/") {
+				cachePaths = append(cachePaths, s.With["path"].(string))
+				if strings.HasPrefix(s.Uses, "actions/cache/save@") {
+					savedKeys = append(savedKeys, s.With["key"].(string))
+					if !strings.Contains(s.If, "hashFiles('.mutation/attestation.json') != ''") {
+						t.Errorf("%s/%s: cache saves require the attestation", name, s.Name)
+					}
+				}
+			}
+			if strings.Contains(s.Run, " publish-state") {
+				save := stepIndex(job, func(x wfStep) bool { return strings.HasPrefix(x.Uses, "actions/cache/save@") && x.If == s.If })
+				if save < 0 || save > i {
+					t.Errorf("%s/%s: the cache is saved (same condition) before publishing", name, s.Name)
+				}
+			}
+			if strings.HasPrefix(s.Uses, "actions/cache/") && strings.HasPrefix(name, "incremental-pr") && !strings.Contains(s.With["key"].(string), "mutation-pr-${{ github.event.pull_request.number }}-") {
+				t.Errorf("%s: N in PR cache keys is github.event.pull_request.number", name)
+			}
+		}
+	}
+	for _, p := range cachePaths {
+		if p != cachePaths[0] {
+			t.Errorf("every cache step uses the identical path list (F10): %q vs %q", p, cachePaths[0])
+		}
+	}
+	slices.Sort(savedKeys)
+	if len(slices.Compact(slices.Clone(savedKeys))) != len(savedKeys) {
+		t.Errorf("every saved cache key is distinct: %v", savedKeys)
+	}
+
+	pr, prFull, main, full := wf.Jobs["incremental-pr"], wf.Jobs["pr-full"], wf.Jobs["incremental-main"], wf.Jobs["full"]
+	if !pr.Concurrency.CancelInProgress || main.Concurrency.CancelInProgress || full.Concurrency.CancelInProgress || prFull.Concurrency.CancelInProgress {
+		t.Error("only incremental-pr cancels in progress")
+	}
+	if prFull.Needs != "incremental-pr" || !strings.Contains(prFull.If, "needs.incremental-pr.outputs.route == 'sweep'") || !strings.Contains(prFull.If, "exit_code == '1'") {
+		t.Errorf("pr-full needs incremental-pr and runs only on a sweep route with exit 0/1: %q", prFull.If)
+	}
+	if strings.Contains(stepsText(prFull), "MUTATION_STATE_TOKEN") || strings.Contains(stepsText(prFull), "publish-state") {
+		t.Error("pr-full has no token and no publish step")
+	}
+	if full.Needs != "incremental-main" || !strings.Contains(full.If, "!cancelled()") || !strings.Contains(full.If, "needs.incremental-main.outputs.pending_full == 'true'") {
+		t.Errorf("full needs incremental-main, runs when not cancelled and pending: %q", full.If)
+	}
+	catchup, fullRun := stepIndex(full, named("catch-up")), stepIndex(full, named("full run"))
+	if catchup < 0 || fullRun < catchup {
+		t.Error("the full job runs the catch-up before run --mode full")
+	}
+	publishCatchup := full.Steps[stepIndex(full, named("publish catch-up"))]
+	if !strings.Contains(publishCatchup.If, "steps.catchup.outputs.pending_full == 'false'") {
+		t.Error("the catch-up is published only when it cleared pending")
+	}
+	if !strings.Contains(stepsText(main), "publish-state --kind inc") || strings.Contains(main.Steps[stepIndex(main, named("summary"))].Run, "--job pr") {
+		t.Error("incremental-main publishes inc; its summary omits main's score (--job main)")
+	}
+	for _, key := range []string{"exit_code", "pending_full"} {
+		if main.Outputs[key] == "" {
+			t.Errorf("incremental-main exposes %s", key)
+		}
+	}
+	for _, key := range []string{"exit_code", "route"} {
+		if pr.Outputs[key] == "" {
+			t.Errorf("incremental-pr exposes %s", key)
+		}
+	}
+}
+
+func mapHas(m map[string]any, key string) bool { _, ok := m[key]; return ok }
+
+func stepsText(job wfJob) string {
+	out, _ := yaml.Marshal(job.Steps)
+	return string(out)
+}
+
+func runVerdict(t *testing.T, script string, env map[string]string) int {
+	t.Helper()
+	cmd := exec.Command("sh", "-c", script)
+	cmd.Env = os.Environ()
+	for k, v := range env {
+		cmd.Env = append(cmd.Env, k+"="+v)
+	}
+	cmd.Stdout, cmd.Stderr = nil, nil
+	if err := cmd.Run(); err != nil {
+		if exit, ok := err.(*exec.ExitError); ok {
+			return exit.ExitCode()
+		}
+		t.Fatal(err)
+	}
+	return 0
+}
+
+func TestMutationWorkflowVerdicts(t *testing.T) {
+	wf, _ := loadWorkflow(t)
+	verdict := func(job string) string {
+		j := wf.Jobs[job]
+		return j.Steps[stepIndex(j, named("verdict"))].Run
+	}
+	// incremental-pr (spec §11.2): a sweep route is green (pr-full decides), fail and any other
+	// non-zero exit are red, an empty exit code is red.
+	for _, c := range []struct {
+		exit, route string
+		want        int
+	}{
+		{"0", "verdict", 0}, {"1", "verdict", 1}, {"0", "sweep", 0}, {"1", "sweep", 0},
+		{"0", "fail", 1}, {"1", "fail", 1}, {"", "", 1}, {"2", "", 1}, {"3", "", 1}, {"4", "", 1}, {"130", "", 1},
+	} {
+		if got := runVerdict(t, verdict("incremental-pr"), map[string]string{"EXIT_CODE": c.exit, "ROUTE": c.route}); got != c.want {
+			t.Errorf("incremental-pr exit=%q route=%q: got %d, want %d", c.exit, c.route, got, c.want)
+		}
+	}
+	// full (spec §5.7 step 5): the full run's exit code when that step ran, else the catch-up's.
+	for _, c := range []struct {
+		outcome, fullExit, catchupExit string
+		want                           int
+	}{
+		{"skipped", "", "0", 0}, {"skipped", "", "1", 1}, {"skipped", "", "", 1},
+		{"success", "0", "0", 0}, {"failure", "1", "0", 1}, {"failure", "", "0", 1}, {"failure", "4", "4", 1},
+	} {
+		env := map[string]string{"FULL_OUTCOME": c.outcome, "FULL_EXIT": c.fullExit, "CATCHUP_EXIT": c.catchupExit}
+		if got := runVerdict(t, verdict("full"), env); got != c.want {
+			t.Errorf("full outcome=%q full=%q catch-up=%q: got %d, want %d", c.outcome, c.fullExit, c.catchupExit, got, c.want)
+		}
+	}
+	for _, job := range []string{"incremental-main", "pr-full"} {
+		for _, c := range []struct {
+			exit string
+			want int
+		}{{"0", 0}, {"1", 1}, {"", 1}, {"4", 1}} {
+			if got := runVerdict(t, verdict(job), map[string]string{"EXIT_CODE": c.exit}); got != c.want {
+				t.Errorf("%s exit=%q: got %d, want %d", job, c.exit, got, c.want)
+			}
+		}
+	}
+}
diff --git a/templates/mutation-incremental/github-workflow.yml b/templates/mutation-incremental/github-workflow.yml
new file mode 100644
index 0000000..c901473
--- /dev/null
+++ b/templates/mutation-incremental/github-workflow.yml
@@ -0,0 +1,289 @@
+# metareview mutation-incremental CI (spec §5.7, §11.2). Copy to .github/workflows/mutation.yml,
+# together with tools/mutation-incremental/ and mutation-incremental.json (docs/mutation-harness.md).
+#
+# - Make both incremental-pr and pr-full required checks (a skipped job satisfies a required check).
+# - Edit the push branch if the default branch is not main.
+# - Replace the "project setup" steps with the project's own (Node from .nvmrc, npm ci, services).
+# - Outputs reach shell steps through env:, never through ${{ }} inside run:.
+name: mutation
+
+on:
+  push:
+    branches: [main]
+  pull_request:
+
+permissions: {}
+
+env:
+  MUTATION_CLI: tools/mutation-incremental/cli.mjs
+  # Per-invocation minutes for PR runs when pendingOnPr is full or full-on-global (ignored under allow).
+  MUTATION_PR_MAX_MINUTES: '60'
+
+jobs:
+  incremental-pr:
+    if: github.event_name == 'pull_request'
+    runs-on: ubuntu-latest
+    # >= setup + 2 x (MUTATION_PR_MAX_MINUTES + 1) + max(1, views) x (verify.timeoutMinutes + 0.5)
+    timeout-minutes: 150
+    permissions:
+      contents: read
+    concurrency:
+      group: mutation-inc-${{ github.ref }}
+      cancel-in-progress: true
+    outputs:
+      exit_code: ${{ steps.run.outputs.exit_code }}
+      route: ${{ steps.run.outputs.route }}
+    steps:
+      - name: checkout
+        uses: actions/checkout@v4
+        with:
+          fetch-depth: 0
+          persist-credentials: false
+      # >>> project setup >>>
+      - name: setup node
+        uses: actions/setup-node@v4
+        with:
+          node-version-file: .nvmrc
+      - name: install
+        run: npm ci
+      # <<< project setup <<<
+      - name: restore state cache
+        uses: actions/cache/restore@v4
+        with:
+          path: |
+            .mutation/attestation.json
+            .mutation/incremental.json
+          key: mutation-pr-${{ github.event.pull_request.number }}-${{ github.sha }}
+          restore-keys: |
+            mutation-pr-${{ github.event.pull_request.number }}-
+            mutation-main-
+      - name: fetch state
+        run: node "$MUTATION_CLI" fetch-state
+        env:
+          MUTATION_STATE_TOKEN: ${{ github.token }}
+      - name: run
+        id: run
+        continue-on-error: true
+        run: node "$MUTATION_CLI" run --mode incremental --pr --max-minutes "$MUTATION_PR_MAX_MINUTES" --also-state .mutation/remote/full --also-state .mutation/remote/inc
+      - name: save state cache
+        if: (steps.run.outputs.exit_code == '0' || steps.run.outputs.exit_code == '1') && steps.run.outputs.route == 'verdict' && hashFiles('.mutation/attestation.json') != ''
+        uses: actions/cache/save@v4
+        with:
+          path: |
+            .mutation/attestation.json
+            .mutation/incremental.json
+          key: mutation-pr-${{ github.event.pull_request.number }}-${{ github.sha }}-${{ github.run_id }}-${{ github.run_attempt }}
+      - name: summary
+        if: always()
+        env:
+          EXIT_CODE: ${{ steps.run.outputs.exit_code }}
+        run: node "$MUTATION_CLI" summary --job pr --pr --exit-code "$EXIT_CODE" || true
+      - name: verdict
+        env:
+          EXIT_CODE: ${{ steps.run.outputs.exit_code }}
+          ROUTE: ${{ steps.run.outputs.route }}
+        run: |
+          case "$EXIT_CODE:$ROUTE" in
+            0:sweep|1:sweep) echo "verdict deferred to pr-full" ;;
+            0:verdict) ;;
+            0:fail|1:fail) echo "no reachable tests (see the step summary)"; exit 1 ;;
+            *) echo "mutation run exit code '$EXIT_CODE'"; exit 1 ;;
+          esac
+
+  pr-full:
+    needs: incremental-pr
+    if: needs.incremental-pr.outputs.route == 'sweep' && (needs.incremental-pr.outputs.exit_code == '0' || needs.incremental-pr.outputs.exit_code == '1')
+    runs-on: ubuntu-latest
+    # >= setup + one full run + max(1, views) x verify.timeoutMinutes
+    timeout-minutes: 350
+    permissions:
+      contents: read
+    concurrency:
+      group: mutation-pr-full-${{ github.event.pull_request.number }}
+      cancel-in-progress: false
+    steps:
+      - name: checkout
+        uses: actions/checkout@v4
+        with:
+          fetch-depth: 0
+          persist-credentials: false
+      # >>> project setup >>>
+      - name: setup node
+        uses: actions/setup-node@v4
+        with:
+          node-version-file: .nvmrc
+      - name: install
+        run: npm ci
+      # <<< project setup <<<
+      - name: run
+        id: run
+        continue-on-error: true
+        run: node "$MUTATION_CLI" run --mode full
+      - name: save state cache
+        if: (steps.run.outputs.exit_code == '0' || steps.run.outputs.exit_code == '1') && hashFiles('.mutation/attestation.json') != ''
+        uses: actions/cache/save@v4
+        with:
+          path: |
+            .mutation/attestation.json
+            .mutation/incremental.json
+          key: mutation-pr-${{ github.event.pull_request.number }}-${{ github.sha }}-${{ github.run_id }}-${{ github.run_attempt }}-full
+      - name: summary
+        if: always()
+        env:
+          EXIT_CODE: ${{ steps.run.outputs.exit_code }}
+        run: node "$MUTATION_CLI" summary --job pr-full --exit-code "$EXIT_CODE" || true
+      - name: verdict
+        env:
+          EXIT_CODE: ${{ steps.run.outputs.exit_code }}
+        run: |
+          if [ "$EXIT_CODE" != "0" ]; then echo "mutation sweep exit code '$EXIT_CODE'"; exit 1; fi
+
+  incremental-main:
+    if: github.event_name == 'push'
+    runs-on: ubuntu-latest
+    # >= setup + 2 x (maxMinutesPerInvocation + 1) + max(1, views) x (verify.timeoutMinutes + 0.5)
+    timeout-minutes: 60
+    permissions:
+      contents: write
+    concurrency:
+      group: mutation-inc-${{ github.ref }}
+      cancel-in-progress: false
+    outputs:
+      exit_code: ${{ steps.run.outputs.exit_code }}
+      pending_full: ${{ steps.run.outputs.pending_full }}
+    steps:
+      - name: checkout
+        uses: actions/checkout@v4
+        with:
+          fetch-depth: 0
+          persist-credentials: false
+      # >>> project setup >>>
+      - name: setup node
+        uses: actions/setup-node@v4
+        with:
+          node-version-file: .nvmrc
+      - name: install
+        run: npm ci
+      # <<< project setup <<<
+      - name: restore state cache
+        uses: actions/cache/restore@v4
+        with:
+          path: |
+            .mutation/attestation.json
+            .mutation/incremental.json
+          key: mutation-main-${{ github.sha }}
+          restore-keys: |
+            mutation-main-
+      - name: fetch state
+        run: node "$MUTATION_CLI" fetch-state
+        env:
+          MUTATION_STATE_TOKEN: ${{ github.token }}
+      - name: run
+        id: run
+        continue-on-error: true
+        run: node "$MUTATION_CLI" run --mode incremental --also-state .mutation/remote/full --also-state .mutation/remote/inc
+      - name: save state cache
+        if: (steps.run.outputs.exit_code == '0' || steps.run.outputs.exit_code == '1') && hashFiles('.mutation/attestation.json') != ''
+        uses: actions/cache/save@v4
+        with:
+          path: |
+            .mutation/attestation.json
+            .mutation/incremental.json
+          key: mutation-main-${{ github.sha }}-${{ github.run_id }}-${{ github.run_attempt }}
+      - name: publish inc
+        if: (steps.run.outputs.exit_code == '0' || steps.run.outputs.exit_code == '1') && hashFiles('.mutation/attestation.json') != ''
+        run: node "$MUTATION_CLI" publish-state --kind inc
+        env:
+          MUTATION_STATE_TOKEN: ${{ github.token }}
+      - name: summary
+        if: always()
+        env:
+          EXIT_CODE: ${{ steps.run.outputs.exit_code }}
+        run: node "$MUTATION_CLI" summary --job main --exit-code "$EXIT_CODE" || true
+      - name: verdict
+        env:
+          EXIT_CODE: ${{ steps.run.outputs.exit_code }}
+        run: |
+          if [ "$EXIT_CODE" != "0" ]; then echo "mutation run exit code '$EXIT_CODE'"; exit 1; fi
+
+  full:
+    needs: incremental-main
+    if: ${{ !cancelled() && needs.incremental-main.outputs.pending_full == 'true' }}
+    runs-on: ubuntu-latest
+    # >= setup + catch-up (2 x maxMinutesPerInvocation) + full run + 2 x max(1, views) x verify.timeoutMinutes
+    timeout-minutes: 350
+    permissions:
+      contents: write
+    concurrency:
+      group: mutation-main-full
+      cancel-in-progress: false
+    steps:
+      - name: checkout
+        uses: actions/checkout@v4
+        with:
+          fetch-depth: 0
+          persist-credentials: false
+      # >>> project setup >>>
+      - name: setup node
+        uses: actions/setup-node@v4
+        with:
+          node-version-file: .nvmrc
+      - name: install
+        run: npm ci
+      # <<< project setup <<<
+      - name: fetch state
+        run: node "$MUTATION_CLI" fetch-state
+        env:
+          MUTATION_STATE_TOKEN: ${{ github.token }}
+      - name: catch-up
+        id: catchup
+        continue-on-error: true
+        run: node "$MUTATION_CLI" run --mode incremental --also-state .mutation/remote/full --also-state .mutation/remote/inc
+      - name: save catch-up cache
+        if: (steps.catchup.outputs.exit_code == '0' || steps.catchup.outputs.exit_code == '1') && steps.catchup.outputs.pending_full == 'false' && hashFiles('.mutation/attestation.json') != ''
+        uses: actions/cache/save@v4
+        with:
+          path: |
+            .mutation/attestation.json
+            .mutation/incremental.json
+          key: mutation-main-${{ github.sha }}-${{ github.run_id }}-${{ github.run_attempt }}-catchup
+      - name: publish catch-up
+        if: (steps.catchup.outputs.exit_code == '0' || steps.catchup.outputs.exit_code == '1') && steps.catchup.outputs.pending_full == 'false' && hashFiles('.mutation/attestation.json') != ''
+        run: node "$MUTATION_CLI" publish-state --kind full
+        env:
+          MUTATION_STATE_TOKEN: ${{ github.token }}
+      - name: full run
+        id: full
+        if: ${{ !((steps.catchup.outputs.exit_code == '0' || steps.catchup.outputs.exit_code == '1') && steps.catchup.outputs.pending_full == 'false') }}
+        continue-on-error: true
+        run: node "$MUTATION_CLI" run --mode full
+      - name: save full cache
+        if: (steps.full.outputs.exit_code == '0' || steps.full.outputs.exit_code == '1') && hashFiles('.mutation/attestation.json') != ''
+        uses: actions/cache/save@v4
+        with:
+          path: |
+            .mutation/attestation.json
+            .mutation/incremental.json
+          key: mutation-main-${{ github.sha }}-${{ github.run_id }}-${{ github.run_attempt }}-full
+      - name: publish full
+        if: (steps.full.outputs.exit_code == '0' || steps.full.outputs.exit_code == '1') && hashFiles('.mutation/attestation.json') != ''
+        run: node "$MUTATION_CLI" publish-state --kind full
+        env:
+          MUTATION_STATE_TOKEN: ${{ github.token }}
+      - name: summary
+        if: always()
+        env:
+          FULL_OUTCOME: ${{ steps.full.outcome }}
+          FULL_EXIT: ${{ steps.full.outputs.exit_code }}
+          CATCHUP_EXIT: ${{ steps.catchup.outputs.exit_code }}
+        run: |
+          if [ "$FULL_OUTCOME" = "skipped" ]; then final="$CATCHUP_EXIT"; else final="$FULL_EXIT"; fi
+          node "$MUTATION_CLI" summary --job full --exit-code "$final" || true
+      - name: verdict
+        env:
+          FULL_OUTCOME: ${{ steps.full.outcome }}
+          FULL_EXIT: ${{ steps.full.outputs.exit_code }}
+          CATCHUP_EXIT: ${{ steps.catchup.outputs.exit_code }}
+        run: |
+          if [ "$FULL_OUTCOME" = "skipped" ]; then final="$CATCHUP_EXIT"; else final="$FULL_EXIT"; fi
+          if [ "$final" != "0" ]; then echo "mutation full job exit code '$final'"; exit 1; fi
diff --git a/templates/mutation-incremental/lib/config.mjs b/templates/mutation-incremental/lib/config.mjs
index ca6ea08..a452fdd 100644
--- a/templates/mutation-incremental/lib/config.mjs
+++ b/templates/mutation-incremental/lib/config.mjs
@@ -96,6 +96,7 @@ export function loadConfig(top, configArg) {
 
   const stateDir = resolve(top, raw.stateDir);
   if (!inside(top, stateDir) || stateDir === resolve(top)) fail('stateDir must be inside the repository');
+  // Test-only escape hatch (the harness's own unit tests create repositories under the OS temp dir).
   if (!process.env.MUTATION_ALLOW_TMP_STATE) {
     for (const t of [tmpdir(), '/tmp']) {
       if (inside(realpathSync(t), realpathSync(top))) fail('stateDir must not be under the OS temp directory (it is purged)');
diff --git a/templates/mutation-incremental/lib/deferrals.mjs b/templates/mutation-incremental/lib/deferrals.mjs
index 87274b0..f974f2b 100644
--- a/templates/mutation-incremental/lib/deferrals.mjs
+++ b/templates/mutation-incremental/lib/deferrals.mjs
@@ -77,3 +77,11 @@ export function pendingCause(classified) {
   const cause = ['unreachable', 'other', 'timeout'].find((c) => infos.some((i) => i.cause === c)) ?? 'none';
   return { cause, counted };
 }
+
+// Spec §11.2 Routing. Without --pr, or under allow, the exit code alone decides ("verdict"). Under
+// full every cause but none routes to the sweep; under full-on-global only unreachable fails the PR,
+// so a PR's verdict never depends on runner speed.
+export function routeFor(pendingOnPr, cause, pr) {
+  if (!pr || pendingOnPr === 'allow' || cause === 'none') return 'verdict';
+  return pendingOnPr === 'full-on-global' && cause === 'unreachable' ? 'fail' : 'sweep';
+}
diff --git a/templates/mutation-incremental/lib/main.mjs b/templates/mutation-incremental/lib/main.mjs
index 6e45f84..79fdc92 100644
--- a/templates/mutation-incremental/lib/main.mjs
+++ b/templates/mutation-incremental/lib/main.mjs
@@ -7,6 +7,7 @@ import { breakLockCommand } from './lock.mjs';
 import { runCommand } from './run.mjs';
 import { seedCommand } from './seed.mjs';
 import { fetchStateCommand, publishStateCommand } from './remote.mjs';
+import { summaryCommand } from './summary.mjs';
 
 const USAGE = `usage: cli.mjs <command> [--config <path>]
   plan [--also-state <dir>]...
@@ -14,15 +15,16 @@ const USAGE = `usage: cli.mjs <command> [--config <path>]
   seed --from <report>... [--replace]
   fetch-state [--remote <name>]
   publish-state --kind inc|full [--remote <name>]
-  break-lock`;
+  break-lock
+  summary --job pr|pr-full|main|full [--exit-code <n>] [--pr]`;
 
-const VALUE_OPTIONS = { '--config': 'config', '--mode': 'mode', '--max-minutes': 'maxMinutes', '--remote': 'remote', '--kind': 'kind' };
+const VALUE_OPTIONS = { '--config': 'config', '--mode': 'mode', '--max-minutes': 'maxMinutes', '--remote': 'remote', '--kind': 'kind', '--job': 'job', '--exit-code': 'exitCode' };
 const LIST_OPTIONS = { '--also-state': 'alsoState', '--from': 'from' };
 const FLAG_OPTIONS = { '--pr': 'pr', '--replace': 'replace' };
 
 // One option grammar for every command; each command reads the options it uses (spec §5.1).
 export function parseArgs(args) {
-  const out = { command: args[0], config: undefined, mode: undefined, maxMinutes: undefined, remote: undefined, kind: undefined, alsoState: [], from: [], pr: false, replace: false };
+  const out = { command: args[0], config: undefined, mode: undefined, maxMinutes: undefined, remote: undefined, kind: undefined, job: undefined, exitCode: undefined, alsoState: [], from: [], pr: false, replace: false };
   for (let i = 1; i < args.length; i++) {
     const a = args[i];
     if (Object.hasOwn(FLAG_OPTIONS, a)) {
@@ -63,6 +65,7 @@ const COMMANDS = {
   'fetch-state': fetchStateCommand,
   'publish-state': publishStateCommand,
   'break-lock': breakLockCommand,
+  summary: summaryCommand,
 };
 
 export async function main(args, io = { stdout: process.stdout, stderr: process.stderr, cwd: process.cwd() }) {
diff --git a/templates/mutation-incremental/lib/run.mjs b/templates/mutation-incremental/lib/run.mjs
index e854832..4eb648a 100644
--- a/templates/mutation-incremental/lib/run.mjs
+++ b/templates/mutation-incremental/lib/run.mjs
@@ -3,7 +3,7 @@ import { join } from 'node:path';
 import { EngineError, InterruptedError, UsageError } from './errors.mjs';
 import { loadHarnessConfig, planInputs, summaryLine } from './inputs.mjs';
 import { computePlan } from './plan.mjs';
-import { classifyDeferrals, dedupeDeferrals, deferralKey, pendingCause, sortDeferrals } from './deferrals.mjs';
+import { classifyDeferrals, dedupeDeferrals, deferralKey, pendingCause, routeFor, sortDeferrals } from './deferrals.mjs';
 import { resolveViews, viewSummaries } from './views.mjs';
 import { acquireLock } from './lock.mjs';
 import { buildAttestation, commitState, installAdopted, markChangedDuringRun, WORK_DIR } from './attest.mjs';
@@ -58,15 +58,15 @@ function noReachableDeferrals(files, report) {
 }
 
 async function executeIncremental(ctx) {
-  const { config, inputs, options, interrupt } = ctx;
+  const { config, inputs, interrupt } = ctx;
   const { snapshot, graph, candidates, chosen } = inputs;
   // Spec §11.2 (i): main's fetched states count whether or not they are usable.
   const alsoAttestations = candidates.filter((c) => !c.primary && c.attestation !== null).map((c) => c.attestation);
   if (chosen !== null && !chosen.primary) installAdopted(config.stateDir, chosen.dir);
-  const plan = computePlan({ config, snapshot, graph, chosen, unbudgeted: options.pr });
+  const plan = computePlan({ config, snapshot, graph, chosen, unbudgeted: ctx.routed, disableResidual: process.env.MUTATION_TEST_DISABLE_RESIDUAL === '1' });
   // Counted/inherited at plan time decides the shortcut (spec §11.2; review advisory).
   const planned = classifyDeferrals({ deferrals: plan.deferrals, alsoAttestations, snapshot, cold: plan.cold });
-  const shortcut = options.pr && pendingCause(planned).cause === 'global';
+  const shortcut = ctx.routed && pendingCause(planned).cause === 'global';
   let latest = readOutput(join(config.stateDir, REPORT_FILE));
   const produced = [];
   let invocations = 0;
@@ -145,7 +145,6 @@ export async function runCommand(io, options) {
   try {
     const maxMinutes = checkOptions(options);
     ({ config } = loadHarnessConfig(io, options, true));
-    if (options.pr && config.pendingOnPr === 'allow') throw new UsageError('--pr is for pendingOnPr "full" or "full-on-global"; under "allow" the workflow never passes it');
     mkdirSync(config.stateDir, { recursive: true });
     release = acquireLock(config.stateDir);
     const work = join(config.stateDir, WORK_DIR);
@@ -154,20 +153,23 @@ export async function runCommand(io, options) {
     const inputs = planInputs(config.top, config, options.alsoState);
     // Spec §11.3: view completeness is checked at plan time, before any invocation.
     const views = resolveViews(config, inputs.snapshot);
-    const ctx = { config, inputs, views, options, interrupt, maxMinutes, graceMs: io.killGraceMs ?? KILL_GRACE_MS, onOutput: (chunk) => io.stderr.write(chunk) };
+    // Spec §11.2: --pr and --max-minutes take effect only when pendingOnPr routes PRs; under allow a
+    // PR run is an ordinary run (budgeted, MUTATION_RUN_KIND=other).
+    const routed = options.pr && config.pendingOnPr !== 'allow';
+    const ctx = { config, inputs, views, options, interrupt, routed, maxMinutes: routed ? maxMinutes : null, graceMs: io.killGraceMs ?? KILL_GRACE_MS, onOutput: (chunk) => io.stderr.write(chunk) };
     const result = options.mode === 'full' ? await executeFull(ctx) : await executeIncremental(ctx);
     const attestation = commit(ctx, result);
     const failures = [];
     if (attestation !== null) {
       if (attestation.thresholdBreak) failures.push(`score ${attestation.score} is below thresholds.break ${config.thresholdBreak}`);
-      failures.push(...(await runVerify(config, { views, snapshot: inputs.snapshot, pr: options.pr, interrupt, graceMs: ctx.graceMs, onOutput: ctx.onOutput })));
+      failures.push(...(await runVerify(config, { views, snapshot: inputs.snapshot, pr: routed, interrupt, graceMs: ctx.graceMs, onOutput: ctx.onOutput })));
     }
     if (interrupt.interrupted) throw new InterruptedError('interrupted; the state is committed');
     for (const f of failures) io.stderr.write(`mutation-incremental: ${f}\n`);
     const code = failures.length > 0 ? 1 : 0;
     const pendingFull = attestation === null || result.classified.length > 0;
     const { cause, counted } = pendingCause(result.classified);
-    writeOutputs([['pending_full', pendingFull], ['exit_code', code], ['pending_cause', cause], ['pending_causes', causesJSON(counted)]]);
+    writeOutputs([['pending_full', pendingFull], ['exit_code', code], ['pending_cause', cause], ['pending_causes', causesJSON(counted)], ['route', routeFor(config.pendingOnPr, cause, options.pr)]]);
     io.stderr.write(summaryLine({
       command: 'run',
       invocations: result.invocations,
diff --git a/templates/mutation-incremental/lib/summary.mjs b/templates/mutation-incremental/lib/summary.mjs
new file mode 100644
index 0000000..d30414e
--- /dev/null
+++ b/templates/mutation-incremental/lib/summary.mjs
@@ -0,0 +1,77 @@
+import { appendFileSync } from 'node:fs';
+import { join } from 'node:path';
+import { UsageError } from './errors.mjs';
+import { loadHarnessConfig, summaryLine } from './inputs.mjs';
+import { canonicalPendingFull, primaryCandidate, readCandidate } from './state.mjs';
+import { deferralKey, pendingCause, routeFor } from './deferrals.mjs';
+
+const JOBS = ['pr', 'pr-full', 'main', 'full'];
+
+// Reasons and paths are rendered as code spans (review advisory); a backtick cannot close one early.
+const code = (text) => `\`${String(text).replaceAll('`', "'")}\``;
+
+function scoreText(score, thresholdBreak) {
+  const value = score === null ? 'n/a' : score.toFixed(2);
+  if (thresholdBreak === null) return `${value} (no thresholds.break)`;
+  return `${value}, ${score !== null && score < thresholdBreak ? 'below' : 'at or above'} thresholds.break ${thresholdBreak}`;
+}
+
+const deferralLine = (d) => `${code(d.reason)} on ${d.paths.map(code).join(', ')}`;
+
+function mainLines(remotes, thresholdBreak) {
+  const [inc, full] = remotes;
+  const main = [inc, full].find((c) => c.usable);
+  const lines = main === undefined
+    ? ['- main: main state unavailable']
+    : [`- main (${main === inc ? 'mutation-state/inc' : 'mutation-state/full'}): score ${scoreText(main.attestation.score, thresholdBreak)}${main.attestation.deferrals.length > 0 ? ' (includes pending kills)' : ''}`];
+  if (full.usable) lines.push(`- mutation-state/full: completed ${full.attestation.completedAt}, last full run ${full.attestation.lastFullAt}`);
+  return lines;
+}
+
+// Spec §5.7 step 5 and §11.2: what the run did, what is pending and why, and (on PRs) main's score.
+export function summaryMarkdown({ job, exitCode, config, state, remotes, pr }) {
+  const lines = [`### Mutation testing (${job})`, ''];
+  if (exitCode === '') return `${[...lines, '- run step did not execute or did not finish (see earlier step)'].join('\n')}\n`;
+  lines.push(`- exit code: ${code(exitCode)}`);
+  const committed = exitCode === '0' || exitCode === '1';
+  if (!state.usable) lines.push('- no usable state');
+  else {
+    const att = state.attestation;
+    const pending = att.deferrals.length > 0;
+    lines.push(`- pending full run: ${pending}`);
+    lines.push(`- score: ${scoreText(att.score, config.thresholdBreak)}${pending ? ' (includes pending kills)' : ''}${committed ? '' : ' — previous state (this run committed nothing)'}`);
+    const counted = att.deferrals.filter((d) => !d.inherited);
+    for (const d of counted) lines.push(`- deferred: ${deferralLine(d)}`);
+    for (const d of att.deferrals.filter((x) => x.inherited)) lines.push(`- inherited from main, cleared by main's full run: ${deferralLine(d)}`);
+    const route = committed ? routeFor(config.pendingOnPr, pendingCause(att.deferrals).cause, pr) : 'verdict';
+    if (route === 'sweep') lines.push('- verdict deferred to pr-full');
+    if (route === 'fail') {
+      const mainKeys = new Set(remotes.filter((c) => c.usable).flatMap((c) => c.attestation.deferrals.map(deferralKey)));
+      for (const d of counted.filter((x) => x.reason.startsWith('no reachable tests: '))) {
+        lines.push(`- ${code(d.reason)}: add a test or remove the module${mainKeys.has(deferralKey(d)) ? ' (also on main)' : ''}`);
+      }
+    }
+  }
+  if (job === 'pr') lines.push(...mainLines(remotes, config.thresholdBreak));
+  return `${lines.join('\n')}\n`;
+}
+
+// `summary --job pr|pr-full|main|full --exit-code <n> [--pr]`: appends to $GITHUB_STEP_SUMMARY (or
+// prints). The workflow runs it with `if: always()` and `|| true`, so it never fails a job.
+export async function summaryCommand(io, options) {
+  if (!JOBS.includes(options.job)) throw new UsageError('summary needs --job pr|pr-full|main|full');
+  const { config } = loadHarnessConfig(io, options, false);
+  const text = summaryMarkdown({
+    job: options.job,
+    exitCode: options.exitCode ?? '',
+    config,
+    state: primaryCandidate(config),
+    remotes: ['inc', 'full'].map((kind) => readCandidate(join(config.stateDir, 'remote', kind), { label: `mutation-state/${kind}`, primary: false })),
+    pr: options.pr,
+  });
+  const file = process.env.GITHUB_STEP_SUMMARY;
+  if (file) appendFileSync(file, text);
+  else io.stdout.write(text);
+  io.stderr.write(summaryLine({ command: 'summary', pendingFull: canonicalPendingFull(config) }));
+  return 0;
+}
diff --git a/templates/mutation-incremental/test/deferrals.test.mjs b/templates/mutation-incremental/test/deferrals.test.mjs
index 909e5e9..c7b60da 100644
--- a/templates/mutation-incremental/test/deferrals.test.mjs
+++ b/templates/mutation-incremental/test/deferrals.test.mjs
@@ -1,6 +1,7 @@
 import { test } from 'node:test';
 import assert from 'node:assert/strict';
 import { reasonInfo, classifyDeferrals, pendingCause, deferralKey } from '../lib/deferrals.mjs';
+import { routeFor } from '../lib/deferrals.mjs';
 
 test('reasonInfo: every canonical reason and its named key', () => {
   const cases = [
@@ -73,3 +74,20 @@ test('pendingCause precedence: global > unreachable > other > timeout > none', (
   assert.deepEqual(pendingCause([c('blocked by deferred scope', ['a']), c('x', ['*'], true)]).counted, [c('blocked by deferred scope', ['a'])]);
   assert.throws(() => pendingCause([c('mystery', ['a'])]), (e) => e.exitCode === 2);
 });
+
+
+test('routeFor follows §11.2', () => {
+  const rows = [
+    // pendingOnPr, cause, pr → route
+    ['allow', 'global', true, 'verdict'],
+    ['full', 'timeout', false, 'verdict'],
+    ['full', 'none', true, 'verdict'],
+    ['full', 'unreachable', true, 'sweep'],
+    ['full', 'other', true, 'sweep'],
+    ['full-on-global', 'global', true, 'sweep'],
+    ['full-on-global', 'timeout', true, 'sweep'],
+    ['full-on-global', 'unreachable', true, 'fail'],
+    ['full-on-global', 'none', true, 'verdict'],
+  ];
+  for (const [mode, cause, pr, route] of rows) assert.equal(routeFor(mode, cause, pr), route, `${mode} ${cause} ${pr}`);
+});
diff --git a/templates/mutation-incremental/test/golden.test.mjs b/templates/mutation-incremental/test/golden.test.mjs
new file mode 100644
index 0000000..eef13e7
--- /dev/null
+++ b/templates/mutation-incremental/test/golden.test.mjs
@@ -0,0 +1,51 @@
+import { test } from 'node:test';
+import assert from 'node:assert/strict';
+import { existsSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
+import { join } from 'node:path';
+import { STATE_VERSION } from '../lib/state.mjs';
+import { A, cli, runRepo, warm } from './fake.mjs';
+
+// Spec §11.6: stateVersion is bumped whenever planning semantics change. These plans are pinned
+// to it: changed outputs fail until STATE_VERSION is bumped and the file regenerated with
+// UPDATE_GOLDEN=1. Regenerating without a bump is refused.
+const GOLDEN = new URL('../../../testdata/mutation-incremental/planner-golden.json', import.meta.url);
+const A3 = A.replace('n > hi', 'n >= hi');
+const HELPER = { 'tests/helpers/h.ts': 'export const h = 1;\n', 'tests/a.test.ts': "import { clamp } from '../src/a';\nimport { h } from './helpers/h';\n" };
+
+const SCENARIOS = {
+  cold: { cold: true },
+  'residual-edit': { edit: (r) => r.write('src/a.ts', A3) },
+  'whole-edit': { config: { editedFiles: 'whole' }, edit: (r) => r.write('src/a.ts', A3) },
+  'test-edit': { edit: (r) => r.write('tests/a.test.ts', "import { clamp } from '../src/a';\n// another case\n") },
+  'support-edit': { files: HELPER, edit: (r) => r.write('tests/helpers/h.ts', 'export const h = 2;\n') },
+  'global-edit': { files: { 'package.json': '{}\n' }, edit: (r) => r.write('package.json', '{"x":1}\n') },
+  'new-module': { edit: (r) => r.write('src/n.ts', 'export const n = 1;\n') },
+  'deleted-test': { edit: (r) => rmSync(join(r.top, 'tests/a.test.ts')) },
+};
+
+async function currentOutputs() {
+  const out = {};
+  for (const [name, s] of Object.entries(SCENARIOS)) {
+    const r = runRepo({ config: s.config ?? {}, files: s.files ?? {} });
+    if (!s.cold) {
+      await warm(r);
+      s.edit(r);
+    }
+    out[name] = JSON.parse((await cli(r, ['plan'])).stdout);
+  }
+  return out;
+}
+
+test('planner golden outputs are pinned to stateVersion', async () => {
+  const current = await currentOutputs();
+  const golden = existsSync(GOLDEN) ? JSON.parse(readFileSync(GOLDEN, 'utf8')) : null;
+  if (process.env.UPDATE_GOLDEN === '1') {
+    const unchanged = golden !== null && JSON.stringify(golden.outputs) === JSON.stringify(current);
+    assert.ok(golden === null || golden.stateVersion !== STATE_VERSION || unchanged, 'planner outputs changed: bump STATE_VERSION in lib/state.mjs first (spec §11.6)');
+    writeFileSync(GOLDEN, `${JSON.stringify({ stateVersion: STATE_VERSION, outputs: current }, null, 2)}\n`);
+    return;
+  }
+  assert.notEqual(golden, null, 'missing planner-golden.json: generate it with UPDATE_GOLDEN=1');
+  assert.equal(golden.stateVersion, STATE_VERSION, 'STATE_VERSION changed: regenerate planner-golden.json with UPDATE_GOLDEN=1');
+  assert.deepEqual(current, golden.outputs, 'planner outputs changed: bump STATE_VERSION in lib/state.mjs, then regenerate with UPDATE_GOLDEN=1 (spec §11.6)');
+});
diff --git a/templates/mutation-incremental/test/main.test.mjs b/templates/mutation-incremental/test/main.test.mjs
index 68535d7..c9e4fc3 100644
--- a/templates/mutation-incremental/test/main.test.mjs
+++ b/templates/mutation-incremental/test/main.test.mjs
@@ -24,11 +24,11 @@ async function run(args, cwd) {
 test('parseArgs', () => {
   assert.deepEqual(parseArgs(['plan', '--also-state', 'a', '--config', 'c.json', '--also-state', 'b']), {
     command: 'plan', config: 'c.json', mode: undefined, maxMinutes: undefined, remote: undefined, kind: undefined,
-    alsoState: ['a', 'b'], from: [], pr: false, replace: false,
+    job: undefined, exitCode: undefined, alsoState: ['a', 'b'], from: [], pr: false, replace: false,
   });
-  assert.deepEqual(parseArgs(['run', '--mode', 'incremental', '--pr', '--max-minutes', '60', '--replace', '--from', 'r1', '--from', 'r2', '--remote', 'up', '--kind', 'inc']), {
+  assert.deepEqual(parseArgs(['run', '--mode', 'incremental', '--pr', '--max-minutes', '60', '--replace', '--from', 'r1', '--from', 'r2', '--remote', 'up', '--kind', 'inc', '--job', 'pr', '--exit-code', '1']), {
     command: 'run', config: undefined, mode: 'incremental', maxMinutes: '60', remote: 'up', kind: 'inc',
-    alsoState: [], from: ['r1', 'r2'], pr: true, replace: true,
+    job: 'pr', exitCode: '1', alsoState: [], from: ['r1', 'r2'], pr: true, replace: true,
   });
   assert.throws(() => parseArgs(['plan', '--config']), (e) => e.exitCode === 2);
   assert.throws(() => parseArgs(['plan', '--bogus']), (e) => e.exitCode === 2);
diff --git a/templates/mutation-incremental/test/run.test.mjs b/templates/mutation-incremental/test/run.test.mjs
index 4b6d025..453cf8e 100644
--- a/templates/mutation-incremental/test/run.test.mjs
+++ b/templates/mutation-incremental/test/run.test.mjs
@@ -32,7 +32,7 @@ test('run --mode full starts fresh, attests the report and clears pending', asyn
   assert.equal(usable(r), true);
   assert.equal(existsSync(join(r.top, '.mutation/work')), false);
   assert.equal(existsSync(join(r.top, '.mutation/lock')), false);
-  assert.deepEqual(res.output, { pending_full: 'false', exit_code: '0', pending_cause: 'none', pending_causes: '[]' });
+  assert.deepEqual(res.output, { pending_full: 'false', exit_code: '0', pending_cause: 'none', pending_causes: '[]', route: 'verdict' });
   assert.match(res.stderr, /command=run invocations=1 scope=0 forced=0 deferrals=0 pending_full=false\n$/);
 });
 
@@ -100,7 +100,7 @@ test('a cold incremental run with no report runs nothing and writes nothing', as
   assert.equal(res.code, 0);
   assert.deepEqual(r.calls(), []);
   assert.equal(existsSync(join(r.top, '.mutation/attestation.json')), false);
-  assert.deepEqual(res.output, { pending_full: 'true', exit_code: '0', pending_cause: 'global', pending_causes: '[{"reason":"no usable state","paths":["*"]}]' });
+  assert.deepEqual(res.output, { pending_full: 'true', exit_code: '0', pending_cause: 'global', pending_causes: '[{"reason":"no usable state","paths":["*"]}]', route: 'verdict' });
 });
 
 test('a cold run re-attests an existing report with lastFullAt null and no usable state', async () => {
@@ -264,9 +264,45 @@ test('run option errors are exit 2 and still write outputs', async () => {
     assert.equal(res.code, 2, args.join(' '));
     assert.deepEqual(res.output, { pending_full: 'false', exit_code: '2' }, args.join(' '));
   }
-  const allow = await cli(r, ['run', '--mode', 'incremental', '--pr']);
-  assert.deepEqual([allow.code, allow.output.pending_full], [2, 'true']);
-  assert.match(allow.stderr, /--pr is for pendingOnPr "full" or "full-on-global"/);
+});
+
+test('under allow, --pr and --max-minutes change nothing; the route is the verdict', async () => {
+  const r = runRepo();
+  await warm(r);
+  r.write('src/a.ts', A3);
+  const res = await cli(r, ['run', '--mode', 'incremental', '--pr', '--max-minutes', '0.0001']);
+  assert.equal(res.code, 0);
+  assert.equal(r.calls().length, 2); // not timed out: --max-minutes is ignored under allow
+  assert.equal(res.output.route, 'verdict');
+});
+
+test('a routed PR run routes its cause: a timeout to the sweep, an unreachable module to fail', async () => {
+  const r = runRepo({ config: ROUTED });
+  await warm(r);
+  r.write('src/a.ts', A3);
+  r.steps([{ sleepMs: 10000 }]);
+  const slow = await cli(r, ['run', '--mode', 'incremental', '--pr', '--max-minutes', '0.005']);
+  assert.deepEqual([slow.code, slow.output.pending_cause, slow.output.route], [0, 'timeout', 'sweep']);
+  r.steps([{ report: reportFor() }]);
+  await warm(r);
+  rmSync(join(r.top, 'tests/a.test.ts'));
+  r.steps([{ exit: 1, output: 'No tests were executed' }]);
+  const gone = await cli(r, ['run', '--mode', 'incremental', '--pr']);
+  assert.deepEqual([gone.code, gone.output.pending_cause, gone.output.route], [0, 'unreachable', 'fail']);
+});
+
+test('the test-only residual switch drops the coverage closure', async () => {
+  const r = runRepo();
+  await warm(r);
+  r.write('src/a.ts', A3);
+  process.env.MUTATION_TEST_DISABLE_RESIDUAL = '1';
+  try {
+    assert.equal((await cli(r, ['run', '--mode', 'incremental'])).code, 0);
+  } finally {
+    delete process.env.MUTATION_TEST_DISABLE_RESIDUAL;
+  }
+  // Without the closure only the changed line is forced (the closure adds the line-2 kill: 2-3).
+  assert.deepEqual(r.calls()[1], inv(2, '--force', '--mutate', 'src/a.ts:3-3'));
 });
 
 test('a file edited while Stryker runs is attested as changed-during-run', async () => {
diff --git a/templates/mutation-incremental/test/summary.test.mjs b/templates/mutation-incremental/test/summary.test.mjs
new file mode 100644
index 0000000..7303313
--- /dev/null
+++ b/templates/mutation-incremental/test/summary.test.mjs
@@ -0,0 +1,86 @@
+import { test } from 'node:test';
+import assert from 'node:assert/strict';
+import { readFileSync, rmSync, writeFileSync } from 'node:fs';
+import { join } from 'node:path';
+import { summaryMarkdown } from '../lib/summary.mjs';
+import { writeState } from './helpers.mjs';
+import { cli, reportFor, runRepo, warm } from './fake.mjs';
+
+const config = (extra = {}) => ({ thresholdBreak: null, pendingOnPr: 'allow', ...extra });
+const usable = (attestation) => ({ usable: true, attestation: { score: 50, completedAt: 'c', lastFullAt: 'l', deferrals: [], ...attestation } });
+const unusable = { usable: false };
+const md = (extra) => summaryMarkdown({ job: 'main', exitCode: '0', config: config(), state: usable({}), remotes: [unusable, unusable], pr: false, ...extra });
+
+test('an empty exit code says only that the run step did not finish', () => {
+  assert.equal(md({ exitCode: '' }), '### Mutation testing (main)\n\n- run step did not execute or did not finish (see earlier step)\n');
+});
+
+test('no usable state, the score, the threshold and the previous-state label', () => {
+  assert.match(md({ state: unusable }), /- no usable state\n/);
+  assert.match(md({}), /- score: 50\.00 \(no thresholds\.break\)\n/);
+  assert.match(md({ config: config({ thresholdBreak: 60 }) }), /- score: 50\.00, below thresholds\.break 60\n/);
+  assert.match(md({ config: config({ thresholdBreak: 40 }) }), /- score: 50\.00, at or above thresholds\.break 40\n/);
+  assert.match(md({ state: usable({ score: null }) }), /- score: n\/a/);
+  assert.match(md({ exitCode: '4' }), /- score: 50\.00 \(no thresholds\.break\) — previous state \(this run committed nothing\)\n/);
+});
+
+test('pending kills, counted and inherited deferrals as code spans', () => {
+  const deferrals = [
+    { reason: 'global input changed: package-lock.json', paths: ['*'], inherited: true },
+    { reason: 'time budget exceeded', paths: ['src/a.ts', 'src/`b`.ts'], inherited: false },
+  ];
+  const text = md({ state: usable({ deferrals }) });
+  assert.match(text, /- pending full run: true\n/);
+  assert.match(text, /\(includes pending kills\)/);
+  assert.match(text, /- deferred: `time budget exceeded` on `src\/a\.ts`, `src\/'b'\.ts`\n/);
+  assert.match(text, /- inherited from main, cleared by main's full run: `global input changed: package-lock\.json` on `\*`\n/);
+});
+
+test('routed PR runs: the sweep, an unreachable module (and whether main has it)', () => {
+  const routed = config({ pendingOnPr: 'full-on-global' });
+  const timeout = usable({ deferrals: [{ reason: 'time budget exceeded', paths: ['src/a.ts'], inherited: false }] });
+  assert.match(md({ job: 'pr', config: routed, state: timeout, pr: true }), /- verdict deferred to pr-full\n/);
+  assert.doesNotMatch(md({ job: 'pr', config: routed, state: timeout, pr: true, exitCode: '4' }), /verdict deferred/);
+  const gone = { reason: 'no reachable tests: src/c.ts', paths: ['src/c.ts'], inherited: false };
+  const state = usable({ deferrals: [gone] });
+  const onMain = usable({ deferrals: [{ reason: 'no reachable tests: src/c.ts', paths: ['src/c.ts'] }] });
+  assert.match(md({ job: 'pr', config: routed, state, pr: true }), /- `no reachable tests: src\/c\.ts`: add a test or remove the module\n/);
+  assert.match(md({ job: 'pr', config: routed, state, pr: true, remotes: [onMain, unusable] }), /add a test or remove the module \(also on main\)\n/);
+});
+
+test("the PR job shows main's score and the age of mutation-state/full", () => {
+  assert.match(md({ job: 'pr' }), /- main: main state unavailable\n$/);
+  const inc = usable({ score: 90, deferrals: [{ reason: 'no usable state', paths: ['*'] }] });
+  const full = usable({ score: 80, completedAt: '2026-09-23T10:00:00.000Z', lastFullAt: '2026-09-22T10:00:00.000Z' });
+  const text = md({ job: 'pr', remotes: [inc, full] });
+  assert.match(text, /- main \(mutation-state\/inc\): score 90\.00 \(no thresholds\.break\) \(includes pending kills\)\n/);
+  assert.match(text, /- mutation-state\/full: completed 2026-09-23T10:00:00\.000Z, last full run 2026-09-22T10:00:00\.000Z\n/);
+  assert.match(md({ job: 'pr', remotes: [unusable, full] }), /- main \(mutation-state\/full\): score 80\.00/);
+  assert.doesNotMatch(md({ job: 'main', remotes: [inc, full] }), /main \(/);
+});
+
+test('summary appends to GITHUB_STEP_SUMMARY, or prints without it; bad --job is exit 2', async () => {
+  const r = runRepo();
+  await warm(r);
+  writeState(join(r.top, '.mutation/remote/full'), { report: reportFor(), attestation: { completedAt: 'c2', lastFullAt: 'l2', score: 50 } });
+  const file = join(r.top, '.fake/summary.md');
+  writeFileSync(file, 'before\n');
+  const saved = process.env.GITHUB_STEP_SUMMARY; // set on GitHub runners
+  try {
+    process.env.GITHUB_STEP_SUMMARY = file;
+    const res = await cli(r, ['summary', '--job', 'pr', '--exit-code', '0']);
+    assert.equal(res.code, 0);
+    assert.match(res.stderr, /command=summary invocations=0 scope=0 forced=0 deferrals=0 pending_full=false\n$/);
+    const text = readFileSync(file, 'utf8');
+    assert.match(text, /^before\n### Mutation testing \(pr\)\n/);
+    assert.match(text, /mutation-state\/full: completed c2, last full run l2/);
+    delete process.env.GITHUB_STEP_SUMMARY;
+    rmSync(join(r.top, '.mutation/attestation.json'));
+    const printed = await cli(r, ['summary', '--job', 'main']);
+    assert.equal(printed.stdout, '### Mutation testing (main)\n\n- run step did not execute or did not finish (see earlier step)\n');
+    assert.equal((await cli(r, ['summary', '--job', 'nope'])).code, 2);
+  } finally {
+    if (saved === undefined) delete process.env.GITHUB_STEP_SUMMARY;
+    else process.env.GITHUB_STEP_SUMMARY = saved;
+  }
+});
diff --git a/testdata/mutation-incremental-e2e/.gitignore b/testdata/mutation-incremental-e2e/.gitignore
new file mode 100644
index 0000000..c65f641
--- /dev/null
+++ b/testdata/mutation-incremental-e2e/.gitignore
@@ -0,0 +1,4 @@
+node_modules/
+.mutation/
+reports/
+.stryker-tmp/
diff --git a/testdata/mutation-incremental-e2e/README.md b/testdata/mutation-incremental-e2e/README.md
new file mode 100644
index 0000000..57a80b0
--- /dev/null
+++ b/testdata/mutation-incremental-e2e/README.md
@@ -0,0 +1,56 @@
+# mutation-incremental e2e fixture
+
+A small TypeScript project for the harness's local proof with real StrykerJS 10.0.0 and Vitest 4.1.11
+(spec §7.1, §11.7). `tests/e2e-mutation-incremental.mjs` copies it to `.e2e/<run>/`, adds
+`tools/mutation-incremental/`, points `stryker.command` at a recording shim, runs `npm ci`, and
+commits it. Never run Stryker here in `testdata/`.
+
+## Files
+
+| File | Purpose |
+|---|---|
+| `package.json`, `package-lock.json` | pinned `@stryker-mutator/core` 10.0.0, `@stryker-mutator/vitest-runner` 10.0.0, `vitest` 4.1.11 |
+| `stryker.config.json` | Vitest runner, `disableBail: true`, `mutate` equal to the harness's, `.mutation` ignored |
+| `vitest.config.ts` | the `@app/` alias (`resolve.alias`), tests under `tests/` |
+| `mutation-incremental.json` | harness config: alias `@app/` → `src/`, `maxForcedShare: 1`, `pendingOnPr: full-on-global`, views `core` and `edge` (overlapping) |
+| `.gitignore` | `node_modules/`, `.mutation/`, `reports/`, `.stryker-tmp/` |
+| `src/a.ts` | `grade(score, pass, top)`: block body lines 1–5, statements on lines 2–4; no equivalent mutants |
+| `src/b.ts` | imports `grade` and `import type { Range }`; expression-bodied arrow on line 4 |
+| `src/c.ts` | `LIMIT` import on line 1; `twice` on line 2, tested only by `tests/c.test.ts` |
+| `src/limits.ts` | `LIMIT = 100` (no mutants) |
+| `src/types.ts` | `interface Range` (no mutants) |
+| `src/index.ts` | barrel `export * from './a'` (no mutants) |
+| `src/e.ts` | `inc` (lines 1–4) and `dec` (lines 6–9), tested separately |
+| `tests/a.test.ts` | `grade` at and around both boundaries (imports `node:assert`, `vitest`) |
+| `tests/b.test.ts` | `inRange` (imports `vitest` and `./helpers/make`) |
+| `tests/c.test.ts` | `twice` via `@app/c` and `./helpers/fmt` |
+| `tests/index.test.ts` | `grade` through the barrel |
+| `tests/e-inc.test.ts`, `tests/e-dec.test.ts` | `inc`, `dec` |
+| `tests/helpers/make.ts`, `tests/helpers/fmt.ts` | support files |
+
+A full run has 24 mutants, all killed (score 100): 12 in `src/a.ts`, 5 in `src/b.ts`, 3 in
+`src/c.ts`, 4 in `src/e.ts` (a `BlockStatement` for each function and an `ArithmeticOperator` on
+lines 2 and 7).
+
+## Scenario edits (e2e rows)
+
+| Row | Edit |
+|---|---|
+| `a-preserving` | `src/a.ts` line 3 → `  if (top < score) return 'over';` |
+| `a-survivor` | `src/a.ts` line 3 → `  if (score >= top + 1) return 'over';` (the new `score > top + 1` mutant survives) |
+| `test-b` | a case `inRange(70, range(50, 90))` added to `tests/b.test.ts` |
+| `limits` | `src/limits.ts` → `export const LIMIT = 10 * 10;` |
+| `helper-make` | `tests/helpers/make.ts` → `({ hi, lo })` |
+| `types-only` | `src/types.ts` → `…; hi: number; }` |
+| `delete-c-test` | delete `tests/c.test.ts` |
+| `e-residual` | `src/e.ts` line 2 → `  const r = 1 + n;` |
+| `e-flip` | `src/e.ts` line 3 (inside `inc`) → `  return 2;` |
+| `lockfile` | a trailing newline appended to `package-lock.json` |
+| `threshold` | `thresholds.break: 100` in `stryker.config.json`; the `at top is ok` test removed |
+
+**Negative control `e-flip`.** Changing line 3 of `src/e.ts` from `return r;` to `return 2;` makes
+the `ArithmeticOperator` mutant on line 2 (`n + 1` → `n - 1`) survive: `inc` now returns 2 whatever
+`r` is, so `tests/e-inc.test.ts` no longer kills it. With the residual closure the harness re-runs
+that mutant and reports it `Survived`. With the closure disabled (`MUTATION_TEST_DISABLE_RESIDUAL=1`)
+Stryker reuses its old `Killed` result, because the mutant's own text and its killing test did not
+change, so equivalence with a fresh run fails for exactly that mutant.
diff --git a/testdata/mutation-incremental-e2e/mutation-incremental.json b/testdata/mutation-incremental-e2e/mutation-incremental.json
new file mode 100644
index 0000000..ee38a26
--- /dev/null
+++ b/testdata/mutation-incremental-e2e/mutation-incremental.json
@@ -0,0 +1,16 @@
+{
+  "schemaVersion": 1,
+  "stateDir": ".mutation",
+  "stryker": { "command": ["npx", "--no-install", "stryker"], "configFile": "stryker.config.json", "extraArgs": [] },
+  "mutate": ["src/**/*.ts"],
+  "test": ["tests/**/*.test.ts"],
+  "support": ["tests/helpers/**"],
+  "global": ["package.json", "package-lock.json", "stryker.config.json", "vitest.config.ts"],
+  "ignore": ["README.md", ".gitignore", "reports/**"],
+  "aliases": { "@app/": "src/" },
+  "runtime": { "commands": [], "env": [] },
+  "budget": { "maxForcedShare": 1, "maxForcedMutants": null, "maxMinutesPerInvocation": 10 },
+  "residual": { "mode": "bounded" },
+  "pendingOnPr": "full-on-global",
+  "views": { "inline": { "core": ["src/**/*.ts"], "edge": ["src/c.ts", "src/e.ts"] } }
+}
diff --git a/testdata/mutation-incremental-e2e/package-lock.json b/testdata/mutation-incremental-e2e/package-lock.json
new file mode 100644
index 0000000..a2d01eb
--- /dev/null
+++ b/testdata/mutation-incremental-e2e/package-lock.json
@@ -0,0 +1,3545 @@
+{
+  "name": "mutation-incremental-e2e-fixture",
+  "lockfileVersion": 3,
+  "requires": true,
+  "packages": {
+    "": {
+      "name": "mutation-incremental-e2e-fixture",
+      "devDependencies": {
+        "@stryker-mutator/core": "10.0.0",
+        "@stryker-mutator/vitest-runner": "10.0.0",
+        "vitest": "4.1.11"
+      }
+    },
+    "node_modules/@babel/code-frame": {
+      "version": "8.0.6",
+      "resolved": "https://registry.npmjs.org/@babel/code-frame/-/code-frame-8.0.6.tgz",
+      "integrity": "sha512-uMHhvmDmwFiLuiozaoRYTZHqMFuu3fem/X9JY78lohAuLoh8LSGGC8JqUzo2cVuBsBxkjMZNk56CE7ock8Xxkw==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@babel/helper-validator-identifier": "^8.0.6",
+        "js-tokens": "^10.0.0"
+      },
+      "engines": {
+        "node": "^22.18.0 || >=24.11.0"
+      }
+    },
+    "node_modules/@babel/compat-data": {
+      "version": "8.0.5",
+      "resolved": "https://registry.npmjs.org/@babel/compat-data/-/compat-data-8.0.5.tgz",
+      "integrity": "sha512-YLsYoQMvL8l8WrGpN3Zj7O1wK5LEBN+cQtux7BcuHyxIXve724XG+zuJ1n3U1cUweRtTzQOA4IHbuQw3N34SZw==",
+      "dev": true,
+      "license": "MIT",
+      "engines": {
+        "node": "^22.18.0 || >=24.11.0"
+      }
+    },
+    "node_modules/@babel/core": {
+      "version": "8.0.6",
+      "resolved": "https://registry.npmjs.org/@babel/core/-/core-8.0.6.tgz",
+      "integrity": "sha512-5zwYt1V4ji3mXKKRIAkohSJ29WepxNwxQi+7y7P5AcVPh1dcxnZBU2cfxHnPwS+u9+lZnXPsv30TQaF2xg+Tfg==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@babel/code-frame": "^8.0.6",
+        "@babel/generator": "^8.0.6",
+        "@babel/helper-compilation-targets": "^8.0.6",
+        "@babel/helpers": "^8.0.5",
+        "@babel/parser": "^8.0.6",
+        "@babel/template": "^8.0.0",
+        "@babel/traverse": "^8.0.6",
+        "@babel/types": "^8.0.6",
+        "@types/gensync": "^1.0.5",
+        "convert-source-map": "^2.0.0",
+        "empathic": "^2.0.1",
+        "gensync": "^1.0.0-beta.2",
+        "import-meta-resolve": "^4.2.0",
+        "json5": "^2.2.3",
+        "obug": "^2.1.1",
+        "verkit": "^0.3.2"
+      },
+      "engines": {
+        "node": "^22.18.0 || >=24.11.0"
+      },
+      "funding": {
+        "type": "opencollective",
+        "url": "https://opencollective.com/babel"
+      }
+    },
+    "node_modules/@babel/generator": {
+      "version": "8.0.6",
+      "resolved": "https://registry.npmjs.org/@babel/generator/-/generator-8.0.6.tgz",
+      "integrity": "sha512-/pcanjdRsonCexCuxgGD8fKnBLLzk2i9uM4kmcmGyyev2R5YDs+rafMYZ89RvaGY1mW8T7C5au0TyYaEeHBGng==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@babel/parser": "^8.0.6",
+        "@babel/types": "^8.0.6",
+        "@jridgewell/gen-mapping": "0.4.0-beta.0",
+        "@jridgewell/trace-mapping": "^0.3.31",
+        "@types/jsesc": "^2.5.0",
+        "jsesc": "^3.0.2"
+      },
+      "engines": {
+        "node": "^22.18.0 || >=24.11.0"
+      }
+    },
+    "node_modules/@babel/helper-annotate-as-pure": {
+      "version": "8.0.0",
+      "resolved": "https://registry.npmjs.org/@babel/helper-annotate-as-pure/-/helper-annotate-as-pure-8.0.0.tgz",
+      "integrity": "sha512-NSpMkMsvvZqzThJ0p1B02cbtA2ObEyfBvq950bmNkyxsxvcxwhvvCB036rKhlEnuBBo30bOrk13u3FzlKSoRrw==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@babel/types": "^8.0.0"
+      },
+      "engines": {
+        "node": "^22.18.0 || >=24.11.0"
+      }
+    },
+    "node_modules/@babel/helper-compilation-targets": {
+      "version": "8.0.6",
+      "resolved": "https://registry.npmjs.org/@babel/helper-compilation-targets/-/helper-compilation-targets-8.0.6.tgz",
+      "integrity": "sha512-HTrtZ4+iA0+kzdbCgvdSk1U4pEl3BvpGnlUbualCGNbrVhhQvhgBzurTUmqpWyvyHbWYWaIfdx2mOlL9LJjsew==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@babel/compat-data": "^8.0.5",
+        "@babel/helper-validator-option": "^8.0.0",
+        "browserslist": "^4.24.0",
+        "flru": "1.0.2",
+        "verkit": "^0.3.2"
+      },
+      "engines": {
+        "node": "^22.18.0 || >=24.11.0"
+      }
+    },
+    "node_modules/@babel/helper-create-class-features-plugin": {
+      "version": "8.0.6",
+      "resolved": "https://registry.npmjs.org/@babel/helper-create-class-features-plugin/-/helper-create-class-features-plugin-8.0.6.tgz",
+      "integrity": "sha512-Hy7wG2Pk1LnAkFMq0CbBk789XMTYVzYGxR7tKgk+TOpZckQRME3Tdou4mMT/8DFjJFxFZ8R6UsS4DR/duBKlVA==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@babel/helper-annotate-as-pure": "^8.0.0",
+        "@babel/helper-member-expression-to-functions": "^8.0.5",
+        "@babel/helper-optimise-call-expression": "^8.0.0",
+        "@babel/helper-replace-supers": "^8.0.1",
+        "@babel/helper-skip-transparent-expression-wrappers": "^8.0.0",
+        "@babel/traverse": "^8.0.6",
+        "verkit": "^0.3.2"
+      },
+      "engines": {
+        "node": "^22.18.0 || >=24.11.0"
+      },
+      "peerDependencies": {
+        "@babel/core": "^8.0.0"
+      }
+    },
+    "node_modules/@babel/helper-globals": {
+      "version": "8.0.6",
+      "resolved": "https://registry.npmjs.org/@babel/helper-globals/-/helper-globals-8.0.6.tgz",
+      "integrity": "sha512-OdnA/N52LhzFpeUuVz5YGuSWjc7TPHiFmVYZbPlSgExocO2GwbxVpSni9MVs5wwW+wZ4kPuwBEBN9pIPDYz1mA==",
+      "dev": true,
+      "license": "MIT",
+      "engines": {
+        "node": "^22.18.0 || >=24.11.0"
+      }
+    },
+    "node_modules/@babel/helper-member-expression-to-functions": {
+      "version": "8.0.5",
+      "resolved": "https://registry.npmjs.org/@babel/helper-member-expression-to-functions/-/helper-member-expression-to-functions-8.0.5.tgz",
+      "integrity": "sha512-GLe05QD98BkNFTkjaqeqF9QSRoHLKrrB4tpoplyQuuPrQJ72rtmkM4wvqJLX/sjZPqkbK5MUYSsuUTqdJoOVjA==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@babel/traverse": "^8.0.5",
+        "@babel/types": "^8.0.5"
+      },
+      "engines": {
+        "node": "^22.18.0 || >=24.11.0"
+      }
+    },
+    "node_modules/@babel/helper-module-imports": {
+      "version": "8.0.0",
+      "resolved": "https://registry.npmjs.org/@babel/helper-module-imports/-/helper-module-imports-8.0.0.tgz",
+      "integrity": "sha512-NZ7mSS93o4ndX4KrbD7W8Sf3QT8Qe24PrnFyUcuOPDzK6faqDFKjY9RG7he7+I7FdiQ4llpnosFqzrXa+Vy3Ew==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@babel/traverse": "^8.0.0",
+        "@babel/types": "^8.0.0"
+      },
+      "engines": {
+        "node": "^22.18.0 || >=24.11.0"
+      }
+    },
+    "node_modules/@babel/helper-module-transforms": {
+      "version": "8.0.6",
+      "resolved": "https://registry.npmjs.org/@babel/helper-module-transforms/-/helper-module-transforms-8.0.6.tgz",
+      "integrity": "sha512-YUi+3X0cb0v97xKhuqkLdrwQtP6XXv1eVbhLRCFyMwG481UskbMQk4mjH7K/Jg3oJ1KLskvFwwq4kF/QnABkWQ==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@babel/helper-module-imports": "^8.0.0",
+        "@babel/helper-validator-identifier": "^8.0.6",
+        "@babel/traverse": "^8.0.6"
+      },
+      "engines": {
+        "node": "^22.18.0 || >=24.11.0"
+      },
+      "peerDependencies": {
+        "@babel/core": "^8.0.0"
+      }
+    },
+    "node_modules/@babel/helper-optimise-call-expression": {
+      "version": "8.0.0",
+      "resolved": "https://registry.npmjs.org/@babel/helper-optimise-call-expression/-/helper-optimise-call-expression-8.0.0.tgz",
+      "integrity": "sha512-3W6satvtPuCUkUx63S2jMoW9EQNYkADgs1HTfufmL7gCmAulHMKupA/12WNz4A0GMMFn/YnWWwqOT9IZrJHQjg==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@babel/types": "^8.0.0"
+      },
+      "engines": {
+        "node": "^22.18.0 || >=24.11.0"
+      }
+    },
+    "node_modules/@babel/helper-plugin-utils": {
+      "version": "8.0.1",
+      "resolved": "https://registry.npmjs.org/@babel/helper-plugin-utils/-/helper-plugin-utils-8.0.1.tgz",
+      "integrity": "sha512-3PKFgjTyPlhFhorfP+SjKQxLViIL++zWjFOO4hGriYU+Bsm983DxEM1JmDRJVWXV0O9npu+xXRqz7Pbd3mh70g==",
+      "dev": true,
+      "license": "MIT",
+      "engines": {
+        "node": "^22.18.0 || >=24.11.0"
+      },
+      "peerDependencies": {
+        "@babel/core": "^8.0.0"
+      }
+    },
+    "node_modules/@babel/helper-replace-supers": {
+      "version": "8.0.1",
+      "resolved": "https://registry.npmjs.org/@babel/helper-replace-supers/-/helper-replace-supers-8.0.1.tgz",
+      "integrity": "sha512-B1SZADIcy3tmH8CmWvj4SHi/oAPom4UL3uknTc2QRNsPVLFk/sPnZvQL/8kj7Y5omvjMqie0vklvs6XM4OLW5Q==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@babel/helper-member-expression-to-functions": "^8.0.0",
+        "@babel/helper-optimise-call-expression": "^8.0.0",
+        "@babel/traverse": "^8.0.0"
+      },
+      "engines": {
+        "node": "^22.18.0 || >=24.11.0"
+      },
+      "peerDependencies": {
+        "@babel/core": "^8.0.0"
+      }
+    },
+    "node_modules/@babel/helper-skip-transparent-expression-wrappers": {
+      "version": "8.0.0",
+      "resolved": "https://registry.npmjs.org/@babel/helper-skip-transparent-expression-wrappers/-/helper-skip-transparent-expression-wrappers-8.0.0.tgz",
+      "integrity": "sha512-xmCA9kP3IhySsqhzwIdWGlDN/1A4cCKNBO/uwZx/3YzmDoMePwno2Q5/Bq0q+tYaKbeF940YiKV/kaW8Mzvpjw==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@babel/traverse": "^8.0.0",
+        "@babel/types": "^8.0.0"
+      },
+      "engines": {
+        "node": "^22.18.0 || >=24.11.0"
+      }
+    },
+    "node_modules/@babel/helper-string-parser": {
+      "version": "8.0.6",
+      "resolved": "https://registry.npmjs.org/@babel/helper-string-parser/-/helper-string-parser-8.0.6.tgz",
+      "integrity": "sha512-+6HYEBnsDRZHJtUTAEfMQd0zMDqdFHS7C+5vAsEZqBUr3vEMWOWlHJ0Jz1gS8jJIlCvLsuA6ivmeoqzH936YcA==",
+      "dev": true,
+      "license": "MIT",
+      "engines": {
+        "node": "^22.18.0 || >=24.11.0"
+      }
+    },
+    "node_modules/@babel/helper-validator-identifier": {
+      "version": "8.0.6",
+      "resolved": "https://registry.npmjs.org/@babel/helper-validator-identifier/-/helper-validator-identifier-8.0.6.tgz",
+      "integrity": "sha512-HYx0YmuchRxRSpnjvcNffnPS8DCauKNmnumIuHG3jnQAVewRA7kfJGwF13ayEpSZs7s4R/oBH4eppuXcAQd5JQ==",
+      "dev": true,
+      "license": "MIT",
+      "engines": {
+        "node": "^22.18.0 || >=24.11.0"
+      }
+    },
+    "node_modules/@babel/helper-validator-option": {
+      "version": "8.0.0",
+      "resolved": "https://registry.npmjs.org/@babel/helper-validator-option/-/helper-validator-option-8.0.0.tgz",
+      "integrity": "sha512-U4Dybxh4WESWHt5XhBeExi4DrY0/DNK1aHpQbsrQXCUbFHuMweT0TpLEWKvaraV2Y6fS+ZXunsZ8zIuZIgvF2Q==",
+      "dev": true,
+      "license": "MIT",
+      "engines": {
+        "node": "^22.18.0 || >=24.11.0"
+      }
+    },
+    "node_modules/@babel/helpers": {
+      "version": "8.0.5",
+      "resolved": "https://registry.npmjs.org/@babel/helpers/-/helpers-8.0.5.tgz",
+      "integrity": "sha512-fQtPOXjYOYv85PIdwotp2TJGVYOycX0PQq+l844fFAxOULtBy8BVF35GyeueX0r4KvDthqPH5xAI1clQPk/2uA==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@babel/template": "^8.0.0",
+        "@babel/types": "^8.0.5"
+      },
+      "engines": {
+        "node": "^22.18.0 || >=24.11.0"
+      }
+    },
+    "node_modules/@babel/parser": {
+      "version": "8.0.6",
+      "resolved": "https://registry.npmjs.org/@babel/parser/-/parser-8.0.6.tgz",
+      "integrity": "sha512-LpGDIYJAzc3Y3PT9x+FxBrGYu2PlWxLmyN7SwPTzaX0LSwEmFJBsRnPBqmaXF2r1v+Zhz6lNiEJ+7yKtM5+Ugg==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@babel/types": "^8.0.6"
+      },
+      "bin": {
+        "parser": "bin/babel-parser.js"
+      },
+      "engines": {
+        "node": "^22.18.0 || >=24.11.0"
+      }
+    },
+    "node_modules/@babel/plugin-proposal-decorators": {
+      "version": "8.0.2",
+      "resolved": "https://registry.npmjs.org/@babel/plugin-proposal-decorators/-/plugin-proposal-decorators-8.0.2.tgz",
+      "integrity": "sha512-+C6O6KKXU7BBq1GNaIkFJxrALUVGRcr+WeWm4OcuRl3h+l/CmNfcTLMrT2Lm3uvGBimBH/8pEBRrXJFLoO67Gg==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@babel/helper-create-class-features-plugin": "^8.0.1",
+        "@babel/helper-plugin-utils": "^8.0.1",
+        "@babel/plugin-syntax-decorators": "^8.0.1"
+      },
+      "engines": {
+        "node": "^22.18.0 || >=24.11.0"
+      },
+      "peerDependencies": {
+        "@babel/core": "^8.0.0"
+      }
+    },
+    "node_modules/@babel/plugin-syntax-decorators": {
+      "version": "8.0.1",
+      "resolved": "https://registry.npmjs.org/@babel/plugin-syntax-decorators/-/plugin-syntax-decorators-8.0.1.tgz",
+      "integrity": "sha512-NI+0S/6MvR6GlcQFwjDZ+WIc2qvG6TXN534lYs9llNldwW4b7Dh6KTtk030FA0xWdYGs4t1lWo+OEWN8wGB+Nw==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@babel/helper-plugin-utils": "^8.0.1"
+      },
+      "engines": {
+        "node": "^22.18.0 || >=24.11.0"
+      },
+      "peerDependencies": {
+        "@babel/core": "^8.0.0"
+      }
+    },
+    "node_modules/@babel/plugin-syntax-jsx": {
+      "version": "8.0.1",
+      "resolved": "https://registry.npmjs.org/@babel/plugin-syntax-jsx/-/plugin-syntax-jsx-8.0.1.tgz",
+      "integrity": "sha512-n0jtCOxEovhU7METqSQjcZO9pX53nu9uNIjMS+hEt+Nt9jA7oOZoBIgbCxhhASmF6T6rPDGge5UAvh6Z4eFz/g==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@babel/helper-plugin-utils": "^8.0.1"
+      },
+      "engines": {
+        "node": "^22.18.0 || >=24.11.0"
+      },
+      "peerDependencies": {
+        "@babel/core": "^8.0.0"
+      }
+    },
+    "node_modules/@babel/plugin-syntax-typescript": {
+      "version": "8.0.3",
+      "resolved": "https://registry.npmjs.org/@babel/plugin-syntax-typescript/-/plugin-syntax-typescript-8.0.3.tgz",
+      "integrity": "sha512-jmTPwps7oSQSZaV1SxkQ3C12UWyufGysGc5OzDpZzvPAIX4mO7dJT3hoqkWVrSImvkcMiknir1iLN1SNV/CZzg==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@babel/helper-plugin-utils": "^8.0.1"
+      },
+      "engines": {
+        "node": "^22.18.0 || >=24.11.0"
+      },
+      "peerDependencies": {
+        "@babel/core": "^8.0.0"
+      }
+    },
+    "node_modules/@babel/plugin-transform-destructuring": {
+      "version": "8.0.5",
+      "resolved": "https://registry.npmjs.org/@babel/plugin-transform-destructuring/-/plugin-transform-destructuring-8.0.5.tgz",
+      "integrity": "sha512-evjxc5fGvpXG2WMGSalpX8IJZVumLIDgk5r5eRhvXRDqFX/GPWq1BeRfJalPCcwJfrR5A/wvegUhx9W5Nr7/mg==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@babel/helper-plugin-utils": "^8.0.1"
+      },
+      "engines": {
+        "node": "^22.18.0 || >=24.11.0"
+      },
+      "peerDependencies": {
+        "@babel/core": "^8.0.0"
+      }
+    },
+    "node_modules/@babel/plugin-transform-explicit-resource-management": {
+      "version": "8.0.5",
+      "resolved": "https://registry.npmjs.org/@babel/plugin-transform-explicit-resource-management/-/plugin-transform-explicit-resource-management-8.0.5.tgz",
+      "integrity": "sha512-Vr20jk/ZxRGu30o0jYvcKl2Pf0tiyr8H9q2ZGxYD6yjOpv9DdP6//I85q0cSOdADZhsu/ZZoApjGVCfl4xnVJw==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@babel/helper-plugin-utils": "^8.0.1",
+        "@babel/plugin-transform-destructuring": "^8.0.5"
+      },
+      "engines": {
+        "node": "^22.18.0 || >=24.11.0"
+      },
+      "peerDependencies": {
+        "@babel/core": "^8.0.0"
+      }
+    },
+    "node_modules/@babel/plugin-transform-modules-commonjs": {
+      "version": "8.0.1",
+      "resolved": "https://registry.npmjs.org/@babel/plugin-transform-modules-commonjs/-/plugin-transform-modules-commonjs-8.0.1.tgz",
+      "integrity": "sha512-PMuzulWrrzFNmY3lXSk/tV9NRb7y0eZZLJY4UEo2TKszroxvUZHAPPi+T9FDyrQhod+TQA+t+8/QYaaMpiEuhA==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@babel/helper-module-transforms": "^8.0.1",
+        "@babel/helper-plugin-utils": "^8.0.1"
+      },
+      "engines": {
+        "node": "^22.18.0 || >=24.11.0"
+      },
+      "peerDependencies": {
+        "@babel/core": "^8.0.0"
+      }
+    },
+    "node_modules/@babel/plugin-transform-react-display-name": {
+      "version": "8.0.1",
+      "resolved": "https://registry.npmjs.org/@babel/plugin-transform-react-display-name/-/plugin-transform-react-display-name-8.0.1.tgz",
+      "integrity": "sha512-soLishXlkyu6jcICPyO3HEP7A3GCzKEnn7XfvYrImuWEOwFAz93qShmWSYPf5ww0ZkO4By0zsN2bVIDF54fSdA==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@babel/helper-plugin-utils": "^8.0.1"
+      },
+      "engines": {
+        "node": "^22.18.0 || >=24.11.0"
+      },
+      "peerDependencies": {
+        "@babel/core": "^8.0.0"
+      }
+    },
+    "node_modules/@babel/plugin-transform-react-jsx": {
+      "version": "8.0.1",
+      "resolved": "https://registry.npmjs.org/@babel/plugin-transform-react-jsx/-/plugin-transform-react-jsx-8.0.1.tgz",
+      "integrity": "sha512-NgkoF7Uq+30TmOPDdNUimT0Nta02uVjqJRFNlVWKrbOCu/CkzfHa4aMnIs0lMpkMmZmWA1e42Va+F04i/pY1zw==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@babel/helper-annotate-as-pure": "^8.0.0",
+        "@babel/helper-module-imports": "^8.0.0",
+        "@babel/helper-plugin-utils": "^8.0.1",
+        "@babel/plugin-syntax-jsx": "^8.0.1",
+        "@babel/types": "^8.0.0"
+      },
+      "engines": {
+        "node": "^22.18.0 || >=24.11.0"
+      },
+      "peerDependencies": {
+        "@babel/core": "^8.0.0"
+      }
+    },
+    "node_modules/@babel/plugin-transform-react-jsx-development": {
+      "version": "8.0.1",
+      "resolved": "https://registry.npmjs.org/@babel/plugin-transform-react-jsx-development/-/plugin-transform-react-jsx-development-8.0.1.tgz",
+      "integrity": "sha512-Hb+HUZpV9KFHjm+F+P3aLDMi8QXU9l3ROCQv20z18Me2sGyW5nNNR5YTevNlgHvCpFek3BnAwhDGq/BRndXViw==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@babel/plugin-transform-react-jsx": "^8.0.1"
+      },
+      "engines": {
+        "node": "^22.18.0 || >=24.11.0"
+      },
+      "peerDependencies": {
+        "@babel/core": "^8.0.0"
+      }
+    },
+    "node_modules/@babel/plugin-transform-react-pure-annotations": {
+      "version": "8.0.1",
+      "resolved": "https://registry.npmjs.org/@babel/plugin-transform-react-pure-annotations/-/plugin-transform-react-pure-annotations-8.0.1.tgz",
+      "integrity": "sha512-7/8UwU8hoPBurXa9tUiTTC8aACTRy5tCqLUtqikHp2eGiWoEB57AduOdbQ71OOMTEvawKrGhv3WfzkDpI+/oSg==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@babel/helper-annotate-as-pure": "^8.0.0",
+        "@babel/helper-plugin-utils": "^8.0.1"
+      },
+      "engines": {
+        "node": "^22.18.0 || >=24.11.0"
+      },
+      "peerDependencies": {
+        "@babel/core": "^8.0.0"
+      }
+    },
+    "node_modules/@babel/plugin-transform-typescript": {
+      "version": "8.0.6",
+      "resolved": "https://registry.npmjs.org/@babel/plugin-transform-typescript/-/plugin-transform-typescript-8.0.6.tgz",
+      "integrity": "sha512-WF3yRlQtClzScvQbWct00ix63+w3DPiyU0WmCBU4YqK539n3VvJFElElpevoD8wfu6953CbiNoUZBi2B7/5jXw==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@babel/helper-annotate-as-pure": "^8.0.0",
+        "@babel/helper-create-class-features-plugin": "^8.0.6",
+        "@babel/helper-plugin-utils": "^8.0.1",
+        "@babel/helper-skip-transparent-expression-wrappers": "^8.0.0",
+        "@babel/plugin-syntax-typescript": "^8.0.3"
+      },
+      "engines": {
+        "node": "^22.18.0 || >=24.11.0"
+      },
+      "peerDependencies": {
+        "@babel/core": "^8.0.0"
+      }
+    },
+    "node_modules/@babel/preset-react": {
+      "version": "8.0.1",
+      "resolved": "https://registry.npmjs.org/@babel/preset-react/-/preset-react-8.0.1.tgz",
+      "integrity": "sha512-jrFuPp/pTddFZbtmWhdLNAYc6UMcpboeUPnw0BBrm4nOmcAko/1TRcFi1PzWCeOFRU+VaSiKmat87W1HvR7mIg==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@babel/helper-plugin-utils": "^8.0.1",
+        "@babel/helper-validator-option": "^8.0.0",
+        "@babel/plugin-transform-react-display-name": "^8.0.1",
+        "@babel/plugin-transform-react-jsx": "^8.0.1",
+        "@babel/plugin-transform-react-jsx-development": "^8.0.1",
+        "@babel/plugin-transform-react-pure-annotations": "^8.0.1"
+      },
+      "engines": {
+        "node": "^22.18.0 || >=24.11.0"
+      },
+      "peerDependencies": {
+        "@babel/core": "^8.0.0"
+      }
+    },
+    "node_modules/@babel/preset-typescript": {
+      "version": "8.0.1",
+      "resolved": "https://registry.npmjs.org/@babel/preset-typescript/-/preset-typescript-8.0.1.tgz",
+      "integrity": "sha512-qrPhQIN1NLrPmzgazF9XKQqXrOcp/WJly+K+6ReFonn24FZqRJO7clxOJo6Ni75L+2vAqI3cHVU2OJLBxoPp5A==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@babel/helper-plugin-utils": "^8.0.1",
+        "@babel/helper-validator-option": "^8.0.0",
+        "@babel/plugin-transform-modules-commonjs": "^8.0.1",
+        "@babel/plugin-transform-typescript": "^8.0.1"
+      },
+      "engines": {
+        "node": "^22.18.0 || >=24.11.0"
+      },
+      "peerDependencies": {
+        "@babel/core": "^8.0.0"
+      }
+    },
+    "node_modules/@babel/template": {
+      "version": "8.0.0",
+      "resolved": "https://registry.npmjs.org/@babel/template/-/template-8.0.0.tgz",
+      "integrity": "sha512-eAD0QW/AlbamBbw0FeGiwasbCVPq5ncW0HNVyLP3B9czqLyh4gvw+5JTSNt6le9+ziAU7mqDZsKTHf3jTb4chQ==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@babel/code-frame": "^8.0.0",
+        "@babel/parser": "^8.0.0",
+        "@babel/types": "^8.0.0"
+      },
+      "engines": {
+        "node": "^22.18.0 || >=24.11.0"
+      }
+    },
+    "node_modules/@babel/traverse": {
+      "version": "8.0.6",
+      "resolved": "https://registry.npmjs.org/@babel/traverse/-/traverse-8.0.6.tgz",
+      "integrity": "sha512-ilYV+qLStGzSKLipJlETdvdtpK3rAuwfCmCoGKv15MU4EoeIuLkespSNoNLFGOPSciEHx9TLWtN5mv0fz5w5mQ==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@babel/code-frame": "^8.0.6",
+        "@babel/generator": "^8.0.6",
+        "@babel/helper-globals": "^8.0.6",
+        "@babel/parser": "^8.0.6",
+        "@babel/template": "^8.0.0",
+        "@babel/types": "^8.0.6",
+        "obug": "^2.1.1"
+      },
+      "engines": {
+        "node": "^22.18.0 || >=24.11.0"
+      }
+    },
+    "node_modules/@babel/types": {
+      "version": "8.0.6",
+      "resolved": "https://registry.npmjs.org/@babel/types/-/types-8.0.6.tgz",
+      "integrity": "sha512-c+xgSWboV2pdwXP67BUWDVRHi2BO5Q3KwxyFVs7taVTY+fsK47/L51ZXQoV9rozHJDNHvJ4v4WEsI/IlxV2l2A==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@babel/helper-string-parser": "^8.0.6",
+        "@babel/helper-validator-identifier": "^8.0.6"
+      },
+      "engines": {
+        "node": "^22.18.0 || >=24.11.0"
+      }
+    },
+    "node_modules/@inquirer/ansi": {
+      "version": "2.0.8",
+      "resolved": "https://registry.npmjs.org/@inquirer/ansi/-/ansi-2.0.8.tgz",
+      "integrity": "sha512-WpQM+Ti6Z40EFwwt+uL2p4UabT+W179zHp6HhLVOzfbwnVn05IPO/eXIZXGNqcT1jbQ15SujNLzQ39k4QPPxBQ==",
+      "dev": true,
+      "license": "MIT",
+      "engines": {
+        "node": ">=23.5.0 || ^22.13.0 || ^20.17.0"
+      }
+    },
+    "node_modules/@inquirer/checkbox": {
+      "version": "5.2.5",
+      "resolved": "https://registry.npmjs.org/@inquirer/checkbox/-/checkbox-5.2.5.tgz",
+      "integrity": "sha512-bRt8J8m+Fot9CXv+zNQGXUq2ET0MggR1fPz7v6edN6MFYmsbfGnMmkmWZJEegMKqrAC8ej/o1sqisHZXZJMAfQ==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@inquirer/ansi": "^2.0.8",
+        "@inquirer/core": "^12.0.3",
+        "@inquirer/figures": "^2.0.9",
+        "@inquirer/type": "4.1.1"
+      },
+      "engines": {
+        "node": ">=23.5.0 || ^22.13.0 || ^20.17.0"
+      },
+      "peerDependencies": {
+        "@types/node": ">=18"
+      },
+      "peerDependenciesMeta": {
+        "@types/node": {
+          "optional": true
+        }
+      }
+    },
+    "node_modules/@inquirer/confirm": {
+      "version": "6.3.2",
+      "resolved": "https://registry.npmjs.org/@inquirer/confirm/-/confirm-6.3.2.tgz",
+      "integrity": "sha512-Xvr/0HggjddPtGppuqVmxhTw+Hr8PvsZ/k0HmOEaAqQEt80OITNkFWnsdNmyT0/eM4Ab+iJLx2R8rctlEyfSVg==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@inquirer/core": "^12.0.3",
+        "@inquirer/type": "4.1.1"
+      },
+      "engines": {
+        "node": ">=23.5.0 || ^22.13.0 || ^20.17.0"
+      },
+      "peerDependencies": {
+        "@types/node": ">=18"
+      },
+      "peerDependenciesMeta": {
+        "@types/node": {
+          "optional": true
+        }
+      }
+    },
+    "node_modules/@inquirer/core": {
+      "version": "12.0.3",
+      "resolved": "https://registry.npmjs.org/@inquirer/core/-/core-12.0.3.tgz",
+      "integrity": "sha512-wsSy0sznmXwkty+2PzZwx00Cazc/E0r0B7mAzdGROz2Ct+DFZXaK7WDjGZvgjRldxH5ZhFVfF2lgkYrqgOw2KA==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@inquirer/ansi": "^2.0.8",
+        "@inquirer/figures": "^2.0.9",
+        "@inquirer/type": "4.1.1",
+        "cli-width": "^4.1.0",
+        "fast-wrap-ansi": "^0.2.0",
+        "mute-stream": "^3.0.0",
+        "signal-exit": "^4.1.0"
+      },
+      "engines": {
+        "node": ">=23.5.0 || ^22.13.0 || ^20.17.0"
+      },
+      "peerDependencies": {
+        "@types/node": ">=18"
+      },
+      "peerDependenciesMeta": {
+        "@types/node": {
+          "optional": true
+        }
+      }
+    },
+    "node_modules/@inquirer/editor": {
+      "version": "5.3.3",
+      "resolved": "https://registry.npmjs.org/@inquirer/editor/-/editor-5.3.3.tgz",
+      "integrity": "sha512-YsKkS2q63IiLtaDK/9nqzdComN97SDQrmKiyNggN+ceP4ty+Z6VwyTz3FpjeUWeW1Efss2xHFKCC9sx7hnrsxg==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@inquirer/core": "^12.0.3",
+        "@inquirer/external-editor": "^3.0.5",
+        "@inquirer/type": "4.1.1"
+      },
+      "engines": {
+        "node": ">=23.5.0 || ^22.13.0 || ^20.17.0"
+      },
+      "peerDependencies": {
+        "@types/node": ">=18"
+      },
+      "peerDependenciesMeta": {
+        "@types/node": {
+          "optional": true
+        }
+      }
+    },
+    "node_modules/@inquirer/expand": {
+      "version": "5.1.5",
+      "resolved": "https://registry.npmjs.org/@inquirer/expand/-/expand-5.1.5.tgz",
+      "integrity": "sha512-uHuXLmXW+TtIfT/9vSBotypAkqn1n34Ul+CLGPos/xANyO4Ff5xZzkYhbKR4NEcfVK4a9mHQOpwVZzluSHFRGw==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@inquirer/core": "^12.0.3",
+        "@inquirer/type": "4.1.1"
+      },
+      "engines": {
+        "node": ">=23.5.0 || ^22.13.0 || ^20.17.0"
+      },
+      "peerDependencies": {
+        "@types/node": ">=18"
+      },
+      "peerDependenciesMeta": {
+        "@types/node": {
+          "optional": true
+        }
+      }
+    },
+    "node_modules/@inquirer/external-editor": {
+      "version": "3.0.5",
+      "resolved": "https://registry.npmjs.org/@inquirer/external-editor/-/external-editor-3.0.5.tgz",
+      "integrity": "sha512-f3QQJRIX5ZEneBHNUIuPjmbdzHnmRFJA8r2dkcb8q+OM5Uv5KtnuAttQumnrjcBVBM3mcTX1CkmtAkU58VRZxg==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "chardet": "^2.1.1",
+        "iconv-lite": "^0.7.2"
+      },
+      "engines": {
+        "node": ">=23.5.0 || ^22.13.0 || ^20.17.0"
+      },
+      "peerDependencies": {
+        "@types/node": ">=18"
+      },
+      "peerDependenciesMeta": {
+        "@types/node": {
+          "optional": true
+        }
+      }
+    },
+    "node_modules/@inquirer/figures": {
+      "version": "2.0.9",
+      "resolved": "https://registry.npmjs.org/@inquirer/figures/-/figures-2.0.9.tgz",
+      "integrity": "sha512-EAWgUTGQ/Umgga51dE3B2PUHbufuXarDfg86uVgoSgNHNNQnyFKcOrQLWVqYMghuSyHh8+2HUH0Js9cTC1WAdg==",
+      "dev": true,
+      "license": "MIT",
+      "engines": {
+        "node": ">=23.5.0 || ^22.13.0 || ^20.17.0"
+      }
+    },
+    "node_modules/@inquirer/input": {
+      "version": "5.1.6",
+      "resolved": "https://registry.npmjs.org/@inquirer/input/-/input-5.1.6.tgz",
+      "integrity": "sha512-HtcJhB2QFVXbLuJ5S3syhNbTUVxYvwqV4VRBDkQceBloC9bmTViUoRFP5PbSaDZb3HzfPmpuU/gG4ybVBz4FHA==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@inquirer/core": "^12.0.3",
+        "@inquirer/type": "4.1.1"
+      },
+      "engines": {
+        "node": ">=23.5.0 || ^22.13.0 || ^20.17.0"
+      },
+      "peerDependencies": {
+        "@types/node": ">=18"
+      },
+      "peerDependenciesMeta": {
+        "@types/node": {
+          "optional": true
+        }
+      }
+    },
+    "node_modules/@inquirer/number": {
+      "version": "4.2.3",
+      "resolved": "https://registry.npmjs.org/@inquirer/number/-/number-4.2.3.tgz",
+      "integrity": "sha512-6Yuwh1NGSbu1Lo4N1EWjXs1jKRntLg/ZCwhmeorEHde90v1XxAozdbd4Iu30eOQLW+6h1hp2O9ujNfLSbTPJnA==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@inquirer/core": "^12.0.3",
+        "@inquirer/type": "4.1.1"
+      },
+      "engines": {
+        "node": ">=23.5.0 || ^22.13.0 || ^20.17.0"
+      },
+      "peerDependencies": {
+        "@types/node": ">=18"
+      },
+      "peerDependenciesMeta": {
+        "@types/node": {
+          "optional": true
+        }
+      }
+    },
+    "node_modules/@inquirer/password": {
+      "version": "5.2.2",
+      "resolved": "https://registry.npmjs.org/@inquirer/password/-/password-5.2.2.tgz",
+      "integrity": "sha512-W9zYdyzogK+6110mqwaSJWCBu2yA5Q/OfnGSjjZB1bNpHlmUozXxTl0+QOZBNeVd6Qo81/qT75gW05gLAtITxw==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@inquirer/ansi": "^2.0.8",
+        "@inquirer/core": "^12.0.3",
+        "@inquirer/type": "4.1.1"
+      },
+      "engines": {
+        "node": ">=23.5.0 || ^22.13.0 || ^20.17.0"
+      },
+      "peerDependencies": {
+        "@types/node": ">=18"
+      },
+      "peerDependenciesMeta": {
+        "@types/node": {
+          "optional": true
+        }
+      }
+    },
+    "node_modules/@inquirer/prompts": {
+      "version": "8.7.2",
+      "resolved": "https://registry.npmjs.org/@inquirer/prompts/-/prompts-8.7.2.tgz",
+      "integrity": "sha512-QoRB4wFIjgH5iOhSjoIKMkTvSHDuV+O3OITlIqAYO0oK5x364GJILXiMBvlPiE+klg7Xx9tq5XVqQHcGUDYYPA==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@inquirer/checkbox": "^5.2.5",
+        "@inquirer/confirm": "^6.3.2",
+        "@inquirer/editor": "^5.3.3",
+        "@inquirer/expand": "^5.1.5",
+        "@inquirer/input": "^5.1.6",
+        "@inquirer/number": "^4.2.3",
+        "@inquirer/password": "^5.2.2",
+        "@inquirer/rawlist": "^5.3.5",
+        "@inquirer/search": "^4.3.3",
+        "@inquirer/select": "^5.2.5"
+      },
+      "engines": {
+        "node": ">=23.5.0 || ^22.13.0 || ^20.17.0"
+      },
+      "peerDependencies": {
+        "@types/node": ">=18"
+      },
+      "peerDependenciesMeta": {
+        "@types/node": {
+          "optional": true
+        }
+      }
+    },
+    "node_modules/@inquirer/rawlist": {
+      "version": "5.3.5",
+      "resolved": "https://registry.npmjs.org/@inquirer/rawlist/-/rawlist-5.3.5.tgz",
+      "integrity": "sha512-1oHky1ONfCOwNrnkQGDE1oaSij/3fI6HFMSf2H/WsGO2lEyDX9My82iggITSy9ddSZ8yk8j9v41OI0fVoSIoaA==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@inquirer/core": "^12.0.3",
+        "@inquirer/type": "4.1.1"
+      },
+      "engines": {
+        "node": ">=23.5.0 || ^22.13.0 || ^20.17.0"
+      },
+      "peerDependencies": {
+        "@types/node": ">=18"
+      },
+      "peerDependenciesMeta": {
+        "@types/node": {
+          "optional": true
+        }
+      }
+    },
+    "node_modules/@inquirer/search": {
+      "version": "4.3.3",
+      "resolved": "https://registry.npmjs.org/@inquirer/search/-/search-4.3.3.tgz",
+      "integrity": "sha512-fyuIU1Nbpvwlikjg3gXwJFDI11+EFjqQ7P+iByfmivIKQ1vmaykNrD/vy5unHuUqUpsOsnvJ25//tPF7E/RBRA==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@inquirer/core": "^12.0.3",
+        "@inquirer/figures": "^2.0.9",
+        "@inquirer/type": "4.1.1"
+      },
+      "engines": {
+        "node": ">=23.5.0 || ^22.13.0 || ^20.17.0"
+      },
+      "peerDependencies": {
+        "@types/node": ">=18"
+      },
+      "peerDependenciesMeta": {
+        "@types/node": {
+          "optional": true
+        }
+      }
+    },
+    "node_modules/@inquirer/select": {
+      "version": "5.2.5",
+      "resolved": "https://registry.npmjs.org/@inquirer/select/-/select-5.2.5.tgz",
+      "integrity": "sha512-9kc15hr8r/kI+3DO/xLog5nOzTz1jqsHXa6JBFzmQKhkoJ8Slda1I1L/uD8ZSZ9tF1yp79wwXe7mclvX1rqR2Q==",
+      "dev": true,
+      "license": "MIT",
+      "dependencies": {
+        "@inquirer/ansi": "^2.0.8",
+        "@inquirer/core": "^12.0.3",
+        "@inquirer/figures"



````

## Knowledge And Registries

Service inventory: none

No service inventory found.

Knowledge facts:

No Beads knowledge facts found.

## Evidence

No external validation evidence supplied.
