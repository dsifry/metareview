# metareview task-done context

Run ID: `mrv-20260923-233246532872000-task-done-2026-09-23-mutation-incremental-1b-execution-1dfefa30`

## Task

# Mutation-Incremental Harness — Plan 1b: Execution, State and Remote Commands

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the harness run for real. This plan adds:
- `run --mode incremental|full`, which locks, adopts, plans, invokes Stryker, judges each invocation, commits the attestation, then verifies and checks the threshold;
- `seed`, `fetch-state`, `publish-state` and `break-lock`;
- the `GITHUB_OUTPUT` contract (`pending_full`, `exit_code`, `pending_cause`, `pending_causes`).

**Architecture:** New modules sit beside Plan 1a's planner in `templates/mutation-incremental/lib/`, one responsibility each:
- `proc` (process groups, timeouts, interrupts);
- `lock`;
- `attest` (attestation building and the two-rename commit);
- `engine` (one Stryker invocation and its judgement);
- `verify`;
- `run`, `seed` and `remote` (the commands).

`main.mjs` keeps a single option grammar and a command table. Tests drive the real CLI in-process against a scripted fake Stryker (`test/fake-stryker.mjs`) inside throwaway git repositories. Real Stryker runs only in Plan 1c's e2e.

**Tech Stack:** Node ≥ 22.8 (ESM, `node:test`, built-in coverage, `child_process` with process groups), git plumbing, Go 1.26 (the existing wrapper test only).

**Spec:** `docs/superpowers/specs/2026-09-23-mutation-incremental-design.md` (r21, approved). Read §§1–10 through §11. This plan implements:
- §5.1 (the remaining commands and exit codes);
- §5.4 Adopt's copy;
- §5.5 (attestation writing);
- §5.6 (execution);
- §5.7's `fetch-state`/`publish-state`;
- §5.8 and §11.6 (seed);
- §11.1 (verify);
- §11.2 (`--pr`, `--max-minutes`, counted/inherited on the attestation, `pending_cause`, the shortcut);
- §11.3's `views`/`viewSummaries` in the attestation.

Companion: `docs/superpowers/specs/2026-09-23-mutation-incremental-adoption-contract.md`.

**Plan series:** 1a (`2026-09-23-mutation-incremental-1a-planner.md`, done on this branch) → **1b (this plan)** → 1c (`2026-09-23-mutation-incremental-1c-proof.md`: workflow template, fixture, e2e with real Stryker, docs) → 2 (`2026-09-23-mutation-incremental-gate.md`: the Go gate).

## Global Constraints

- Node ≥ 22.8, ESM `.mjs`, **no npm dependencies** in the template (spec §5).
- Template files live in `templates/mutation-incremental/`; tests in `templates/mutation-incremental/test/*.test.mjs`.
- Node coverage: 100% lines, branches and functions for every non-test `.mjs` in the template. The Go wrapper `mutationtemplate_test.go` asserts it; `test/**` is excluded from coverage.
- Exit codes (spec §5.1):
  - 0 ok (deferrals included);
  - 1 ok, but the threshold or the verifier fails;
  - 2 config/usage/validation;
  - 3 lock held;
  - 4 engine failure (nothing committed);
  - 130 interrupted.

  Precedence: 2 > 3 > 130 > 4 > 1 > 0. An error that is not one of the harness's own error classes exits 4, never 1.
- Attestation constants: `schemaVersion` 1, `tool` `metareview-mutation-incremental`, `toolVersion` `0.13.0` (informational), `stateVersion` 1.
- Every command runs with cwd = the git top-level. Child processes (Stryker, the verifier) are spawned without a shell in their own process group. On a timeout or an interrupt they get SIGTERM, then SIGKILL after 30 s.
- Unit tests may create repositories under `os.tmpdir()` (`test/helpers.mjs` sets `MUTATION_ALLOW_TMP_STATE`). Unit tests never run real Stryker.
- Tests simulate signals with `process.emit('SIGHUP')`. The test runner has its own SIGINT/SIGTERM handling, so tests never emit SIGINT or SIGTERM themselves.
- Never read or run anything in `../thread*` repositories.
- Review stance (user): real workflows and real edge cases; assume trust; no engineering for rare races. Block only for a credible normal-usage failure.
- Each task ends with a commit; messages end with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

## Review Focus

1. **An interrupt or an engine failure mid-run leaves the previous state untouched.** It removes `work/` and releases the lock, and the next run starts cleanly. Task 6, tests `an interrupt stops Stryker, commits nothing and exits 130` and `an invocation failure is exit 4 and keeps the previous state`.
2. **A run that finds the lock held never touches the other run's `work/`.** Task 6, test `a held lock is exit 3 and leaves the other run alone`.
3. **A PR run's own timeout counts against it even when main carries the identical deferral** (spec §11.2 exception). Task 6, test `a PR's own timeout is counted even when main carries the identical deferral`.
4. **`GITHUB_OUTPUT` is written on every exit path.** This includes exit 2 before the config is readable (`pending_full=false`), exit 3, exit 4 and 130, so the workflow's routing never reads an empty value after a real run. Task 6, tests `run option errors are exit 2 and still write outputs`, `a held lock …`, `an interrupt …` and `an unexpected error is exit 4 in the outputs too`.
5. **A state branch that disappeared from the remote leaves no stale local copy to be adopted.** Task 8, test `fetch-state removes a copy whose branch is gone`.

## Plan decisions beyond the spec text (flagged for review)

- **`--pr` is refused under `pendingOnPr: "allow"` and with `--mode full`; `--max-minutes` requires `--pr`** (exit 2). The spec says the template passes `--pr` only when `pendingOnPr` ≠ `allow`, and that the sweep is never `--pr`. Refusing the other combinations turns a miswired workflow into a visible error instead of a silently different verdict.
- **`pending_causes` form** (review advisory):
  - a single-line JSON array of at most 50 `{reason, paths}` entries, followed, when there are more, by one `{"more": <n>}` element;
  - `JSON.stringify` escapes control characters;
  - paths never contain a newline, because the planner exits 2 on them.
- **Counted/inherited is computed at plan time for the shortcut** (review advisory), and again after execution with the keys this run's invocations produced.
- **Stryker's and the verifier's output go to the harness's stderr.** stdout carries only command results (plan JSON, `seed`'s score, `publish-state`'s commit, `break-lock`'s lock content).
- **`score` is stored unrounded;** `thresholdBreak` compares the same value.
- **Without `views` the attestation has no `views`/`viewSummaries` keys** (the gate reads "attested without views", spec §11.3).
- **Verifier environment paths are absolute.**
- **`fetch-state` removes `<stateDir>/remote/<kind>/` before fetching each kind,** so a branch that no longer exists leaves nothing to adopt.
- **Failure classes for the git commands:** a failed `ls-remote`, `fetch`, `show` or `push` is exit 2, as the spec requires for a rejected push. A failed git plumbing command on the local repository (`hash-object`, `mktree`, `commit-tree`) is an internal error, exit 4.
- **An unreadable lock file counts as held** (exit 3; `break-lock` clears it). A lock with the same hostname and a dead pid is replaced (spec §5.6).
- **`run` removes `work/` only while it holds the lock** (on a held lock it would otherwise destroy the other run's work).
- **The invocation file follows the invocation's position:** `work/inv-2.json` is used for the forced invocation even when invocation 1 had nothing to run.
- **`seed` always re-keys and renumbers,** also for a single report, so a seeded report has one shape. Its deferral reason uses the `--from` path as typed.
- **An interrupt during the verifier exits 130 with the state committed** (the spec's precedence puts 130 above 1).
- **The kill grace is injectable** (`io.killGraceMs`, default 30 000 ms) so tests can exercise SIGKILL escalation in milliseconds.

---

## File Structure

```
templates/mutation-incremental/
  cli.mjs                 # unchanged entry point
  lib/errors.mjs          # + InterruptedError (130)
  lib/inputs.mjs          # NEW: summaryLine, loadHarnessConfig, planInputs (moved out of main.mjs)
  lib/main.mjs            # option grammar for every command, command table, exit-code mapping
  lib/state.mjs           # + primaryCandidate, canonicalPendingFull
  lib/snapshot.mjs        # + fileDigest (the digest a snapshot records for one path)
  lib/proc.mjs            # NEW: createInterrupt, installSignalHandlers, signalGroup, spawnGroup
  lib/lock.mjs            # NEW: acquireLock, pidAlive, breakLockCommand (§5.6 step 1)
  lib/attest.mjs          # NEW: mutationScore, buildAttestation, markChangedDuringRun, commitState, installAdopted
  lib/engine.mjs          # NEW: strykerArgv, parseReport, invoke (§5.6 steps 4–5)
  lib/verify.mjs          # NEW: runVerify (§11.1)
  lib/run.mjs             # NEW: runCommand, causesJSON (§5.6, §11.2)
  lib/seed.mjs            # NEW: mergeReports, seedCommand (§5.8, §11.6)
  lib/remote.mjs          # NEW: authEnv, fetchStateCommand, publishStateCommand (§5.7)
  test/fake-stryker.mjs   # NEW: scripted Stryker test double
  test/fake.mjs           # NEW: runRepo, reportFor, cli, warm
  test/{proc,lock,attest,engine,verify,run,seed,remote}.test.mjs   # NEW
```

---

### Task 1: Foundations — shared inputs, option grammar, exit codes, process groups

**Files:**
- Create: `templates/mutation-incremental/lib/inputs.mjs`, `lib/proc.mjs`, `test/proc.test.mjs`
- Modify: `lib/errors.mjs` (append), `lib/state.mjs` (append), `lib/main.mjs` (rewrite)
- Modify: `test/json.test.mjs`, `test/state.test.mjs` (append), `test/main.test.mjs` (replace the `parseArgs` test, append one test)

**Interfaces:**
- Consumes (1a):
  - `loadConfig(top, configArg)`;
  - `takeSnapshot(config)`;
  - `buildGraph(files, config)`;
  - `readCandidate(dir, {label, primary})`;
  - `adopt(candidates, snapshot)`;
  - `nodeVersionWarning(top)`;
  - `computePlan`, `publicPlan`, `resolveViews`.
- Produces:
  - `InterruptedError` (`exitCode` 130);
  - `summaryLine({command, invocations, scope, forced, deferrals, pendingFull}): string`;
  - `loadHarnessConfig(io, options, warn: boolean): {top, config}`;
  - `planInputs(top, config, alsoState: string[]): {snapshot, graph, candidates, chosen}`;
  - `primaryCandidate(config): Candidate`;
  - `canonicalPendingFull(config): boolean`;
  - `parseArgs(args)`, which returns `{command, config, mode, maxMinutes, remote, kind, alsoState: [], from: [], pr: boolean, replace: boolean}`;
  - `createInterrupt(): {interrupted, onInterrupt(fn): unsubscribe, trigger()}`;
  - `installSignalHandlers(interrupt): uninstall`;
  - `signalGroup(pid, signal): boolean`;
  - `spawnGroup(argv, {cwd, env, timeoutMs, graceMs, interrupt, onOutput}): Promise<{code, signal, timedOut, interrupted, error}>`;
  - `KILL_GRACE_MS` (30 000).

- [ ] **Step 1: Write the failing tests**

`templates/mutation-incremental/test/proc.test.mjs`:

```js
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, mkdtempSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { createInterrupt, installSignalHandlers, KILL_GRACE_MS, signalGroup, spawnGroup } from '../lib/proc.mjs';

const node = (script) => [process.execPath, '-e', script];
const opts = (extra) => ({ cwd: tmpdir(), env: process.env, timeoutMs: null, graceMs: 200, interrupt: createInterrupt(), onOutput: () => {}, ...extra });

test('spawnGroup reports the exit code and forwards both output streams', async () => {
  let out = '';
  const r = await spawnGroup(node("process.stdout.write('o'); process.stderr.write('e'); process.exit(3)"), opts({ onOutput: (c) => { out += c; } }));
  assert.deepEqual([r.code, r.timedOut, r.interrupted, r.error], [3, false, false, null]);
  assert.deepEqual([...out].sort(), ['e', 'o']);
  assert.equal(KILL_GRACE_MS, 30_000);
});

test('spawnGroup passes the given environment', async () => {
  const r = await spawnGroup(node("process.exit(process.env.MI_X === 'y' ? 0 : 5)"), opts({ env: { ...process.env, MI_X: 'y' } }));
  assert.equal(r.code, 0);
});

test('a timeout terminates the whole process group', async () => {
  const dir = mkdtempSync(join(tmpdir(), 'mi-proc-'));
  const late = join(dir, 'late');
  const grandchild = `setTimeout(() => require("fs").writeFileSync(${JSON.stringify(late)}, "x"), 1000)`;
  const script = `require('child_process').spawn(process.execPath, ['-e', ${JSON.stringify(grandchild)}], { stdio: 'ignore' }); setTimeout(() => {}, 20000);`;
  const r = await spawnGroup(node(script), opts({ cwd: dir, timeoutMs: 400 }));
  assert.equal(r.timedOut, true);
  await new Promise((res) => setTimeout(res, 1300));
  assert.equal(existsSync(late), false);
});

test('a child that ignores SIGTERM is killed after the grace period', async () => {
  const r = await spawnGroup(node("process.on('SIGTERM', () => {}); setTimeout(() => {}, 20000);"), opts({ timeoutMs: 400, graceMs: 200 }));
  assert.deepEqual([r.timedOut, r.signal], [true, 'SIGKILL']);
});

test('an interrupt stops the child; after an interrupt nothing new is spawned', async () => {
  const interrupt = createInterrupt();
  setTimeout(() => interrupt.trigger(), 300);
  const r = await spawnGroup(node('setTimeout(() => {}, 20000)'), opts({ interrupt }));
  assert.equal(r.interrupted, true);
  const dir = mkdtempSync(join(tmpdir(), 'mi-proc-'));
  const again = await spawnGroup(node("require('fs').writeFileSync('ran', 'x')"), opts({ interrupt, cwd: dir }));
  assert.equal(again.interrupted, true);
  assert.equal(existsSync(join(dir, 'ran')), false);
});

test('a command that cannot be spawned reports the error', async () => {
  const r = await spawnGroup(['/nonexistent/mi-command'], opts({}));
  assert.equal(r.code, null);
  assert.match(r.error.message, /ENOENT/);
});

test('signalGroup is false for a group that does not exist', () => {
  const { pid } = spawnSync(process.execPath, ['-e', '']);
  assert.equal(signalGroup(pid, 'SIGTERM'), false);
});

test('installSignalHandlers routes signals to the interrupt until removed', () => {
  const interrupt = createInterrupt();
  const before = process.listenerCount('SIGHUP');
  const uninstall = installSignalHandlers(interrupt);
  process.emit('SIGHUP');
  assert.equal(interrupt.interrupted, true);
  uninstall();
  assert.equal(process.listenerCount('SIGHUP'), before);
});
```

Append to `templates/mutation-incremental/test/json.test.mjs`:

```js
import { InterruptedError } from '../lib/errors.mjs';

test('InterruptedError is exit 130', () => {
  assert.equal(new InterruptedError('x').exitCode, 130);
});
```

Append to `templates/mutation-incremental/test/state.test.mjs`:

```js
import { canonicalPendingFull, primaryCandidate } from '../lib/state.mjs';
import { loadConfig } from '../lib/config.mjs';
import { makeRepo } from './helpers.mjs';

test('canonicalPendingFull: an unusable state or one with deferrals is a pending full run', () => {
  const r = makeRepo();
  const config = loadConfig(r.top);
  assert.equal(canonicalPendingFull(config), true);
  writeState(config.stateDir, { report: { files: {} } });
  assert.equal(canonicalPendingFull(config), false);
  writeState(config.stateDir, { report: { files: {} }, attestation: { deferrals: [{ reason: 'no usable state', paths: ['*'] }] } });
  assert.equal(canonicalPendingFull(config), true);
  assert.deepEqual([primaryCandidate(config).label, primaryCandidate(config).primary], ['.mutation', true]);
});
```

In `templates/mutation-incremental/test/main.test.mjs`, replace the whole `test('parseArgs', …)` block with:

```js
test('parseArgs', () => {
  assert.deepEqual(parseArgs(['plan', '--also-state', 'a', '--config', 'c.json', '--also-state', 'b']), {
    command: 'plan', config: 'c.json', mode: undefined, maxMinutes: undefined, remote: undefined, kind: undefined,
    alsoState: ['a', 'b'], from: [], pr: false, replace: false,
  });
  assert.deepEqual(parseArgs(['run', '--mode', 'incremental', '--pr', '--max-minutes', '60', '--replace', '--from', 'r1', '--from', 'r2', '--remote', 'up', '--kind', 'inc']), {
    command: 'run', config: undefined, mode: 'incremental', maxMinutes: '60', remote: 'up', kind: 'inc',
    alsoState: [], from: ['r1', 'r2'], pr: true, replace: true,
  });
  assert.throws(() => parseArgs(['plan', '--config']), (e) => e.exitCode === 2);
  assert.throws(() => parseArgs(['plan', '--bogus']), (e) => e.exitCode === 2);
  assert.deepEqual(parseArgs([]).command, undefined);
});
```

and append to the same file (add `import { loadHarnessConfig } from '../lib/inputs.mjs';` to its imports):

```js
test('loadHarnessConfig prints the Node-version warning only when asked', () => {
  const r = makeRepo({ files: { '.nvmrc': '1.2\n' } });
  const err = capture();
  loadHarnessConfig({ cwd: r.top, stderr: err }, {}, false);
  assert.equal(err.text, '');
  loadHarnessConfig({ cwd: r.top, stderr: err }, {}, true);
  assert.match(err.text, /pins Node 1\.2/);
});
```

- [ ] **Step 2: Run them to verify they fail**

Run: `node --test templates/mutation-incremental/test/proc.test.mjs templates/mutation-incremental/test/main.test.mjs templates/mutation-incremental/test/json.test.mjs templates/mutation-incremental/test/state.test.mjs`
Expected: FAIL — `lib/proc.mjs` and `lib/inputs.mjs` do not exist; `InterruptedError`, `canonicalPendingFull` are not exported.

- [ ] **Step 3: Implement**

Append to `templates/mutation-incremental/lib/errors.mjs`:

```js
export class InterruptedError extends Error {
  constructor(message) {
    super(message);
    this.exitCode = 130;
  }
}
```

Append to `templates/mutation-incremental/lib/state.mjs`:

```js
export const primaryCandidate = (config) => readCandidate(config.stateDir, { label: config.stateDirRaw, primary: true });

// Spec §4: a pending full run is a state that is not usable, or one whose attestation has deferrals.
export function canonicalPendingFull(config) {
  const state = primaryCandidate(config);
  return !state.usable || state.attestation.deferrals.length > 0;
}
```

`templates/mutation-incremental/lib/inputs.mjs`:

```js
import { execFileSync } from 'node:child_process';
import { resolve } from 'node:path';
import { UsageError } from './errors.mjs';
import { loadConfig } from './config.mjs';
import { takeSnapshot } from './snapshot.mjs';
import { buildGraph } from './graph.mjs';
import { adopt, primaryCandidate, readCandidate } from './state.mjs';
import { nodeVersionWarning } from './nodever.mjs';

// Spec §5.1: every command prints exactly one summary line on stderr.
export const summaryLine = ({ command, invocations = 0, scope = 0, forced = 0, deferrals = 0, pendingFull = false }) =>
  `mutation-incremental: command=${command} invocations=${invocations} scope=${scope} forced=${forced} deferrals=${deferrals} pending_full=${pendingFull}\n`;

function topLevel(cwd) {
  try {
    return execFileSync('git', ['rev-parse', '--show-toplevel'], { cwd, encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
  } catch {
    throw new UsageError('not inside a git repository');
  }
}

// Every command: the git top-level and the validated config (spec §5.1). plan and run also print
// the Node-version warning.
export function loadHarnessConfig(io, options, warn) {
  const top = topLevel(io.cwd);
  const config = loadConfig(top, options.config);
  const warning = warn ? nodeVersionWarning(top) : null;
  if (warning) io.stderr.write(`${warning}\n`);
  return { top, config };
}

// Everything planning needs, with adoption in memory only (spec §5.4).
export function planInputs(top, config, alsoState) {
  const snapshot = takeSnapshot(config);
  const graph = buildGraph(snapshot.files, config);
  const candidates = [primaryCandidate(config), ...alsoState.map((dir) => readCandidate(resolve(top, dir), { label: dir, primary: false }))];
  return { snapshot, graph, candidates, chosen: adopt(candidates, snapshot) };
}
```

`templates/mutation-incremental/lib/proc.mjs`:

```js
import { spawn } from 'node:child_process';

// Spec §5.6 step 4: on a timeout or an interrupt the group gets SIGTERM, then SIGKILL after 30 s.
export const KILL_GRACE_MS = 30_000;

// One interrupt per command run: signal handlers trigger it, running children subscribe to it.
export function createInterrupt() {
  const listeners = new Set();
  const interrupt = {
    interrupted: false,
    onInterrupt(fn) {
      listeners.add(fn);
      return () => listeners.delete(fn);
    },
    trigger() {
      interrupt.interrupted = true;
      for (const fn of [...listeners]) fn();
    },
  };
  return interrupt;
}

// Spec §5.6: SIGINT, SIGTERM and SIGHUP to the harness stop the running child and the run.
export function installSignalHandlers(interrupt) {
  const signals = ['SIGINT', 'SIGTERM', 'SIGHUP'];
  const handler = () => interrupt.trigger();
  for (const s of signals) process.on(s, handler);
  return () => {
    for (const s of signals) process.off(s, handler);
  };
}

// Signals a whole process group; false when it no longer exists.
export function signalGroup(pid, signal) {
  try {
    process.kill(-pid, signal);
    return true;
  } catch {
    return false;
  }
}

const NOT_RUN = { code: null, signal: null, timedOut: false, interrupted: true, error: null };

// Runs argv without a shell in its own process group (detached), forwarding its output. A spawn
// failure resolves with `error` set (Node may also emit 'close'; the promise settles once).
export function spawnGroup(argv, { cwd, env, timeoutMs, graceMs, interrupt, onOutput }) {
  if (interrupt.interrupted) return Promise.resolve(NOT_RUN);
  return new Promise((settle) => {
    const child = spawn(argv[0], argv.slice(1), { cwd, env, detached: true, stdio: ['ignore', 'pipe', 'pipe'] });
    let timedOut = false;
    let interrupted = false;
    let killTimer = null;
    const stop = () => {
      signalGroup(child.pid, 'SIGTERM');
      killTimer = setTimeout(() => signalGroup(child.pid, 'SIGKILL'), graceMs);
    };
    const timer = timeoutMs === null ? null : setTimeout(() => {
      timedOut = true;
      stop();
    }, timeoutMs);
    const unsubscribe = interrupt.onInterrupt(() => {
      interrupted = true;
      stop();
    });
    child.stdout.on('data', onOutput);
    child.stderr.on('data', onOutput);
    const finish = (code, signal, error) => {
      clearTimeout(timer);
      clearTimeout(killTimer);
      unsubscribe();
      settle({ code, signal, timedOut, interrupted, error });
    };
    child.on('error', (error) => finish(null, null, error));
    child.on('close', (code, signal) => finish(code, signal, null));
  });
}
```

`templates/mutation-incremental/lib/main.mjs` (replaces the file):

```js
import { UsageError } from './errors.mjs';
import { canonicalJSON } from './json.mjs';
import { computePlan, publicPlan } from './plan.mjs';
import { resolveViews } from './views.mjs';
import { loadHarnessConfig, planInputs, summaryLine } from './inputs.mjs';

const USAGE = `usage: cli.mjs <command> [--config <path>]
  plan [--also-state <dir>]...
  run --mode incremental|full [--also-state <dir>]... [--pr [--max-minutes <m>]]
  seed --from <report>... [--replace]
  fetch-state [--remote <name>]
  publish-state --kind inc|full [--remote <name>]
  break-lock`;

const VALUE_OPTIONS = { '--config': 'config', '--mode': 'mode', '--max-minutes': 'maxMinutes', '--remote': 'remote', '--kind': 'kind' };
const LIST_OPTIONS = { '--also-state': 'alsoState', '--from': 'from' };
const FLAG_OPTIONS = { '--pr': 'pr', '--replace': 'replace' };

// One option grammar for every command; each command reads the options it uses (spec §5.1).
export function parseArgs(args) {
  const out = { command: args[0], config: undefined, mode: undefined, maxMinutes: undefined, remote: undefined, kind: undefined, alsoState: [], from: [], pr: false, replace: false };
  for (let i = 1; i < args.length; i++) {
    const a = args[i];
    if (Object.hasOwn(FLAG_OPTIONS, a)) {
      out[FLAG_OPTIONS[a]] = true;
      continue;
    }
    const single = Object.hasOwn(VALUE_OPTIONS, a);
    if (!single && !Object.hasOwn(LIST_OPTIONS, a)) throw new UsageError(`unknown option ${a}`);
    if (i + 1 >= args.length) throw new UsageError(`${a} needs a value`);
    const value = args[++i];
    if (single) out[VALUE_OPTIONS[a]] = value;
    else out[LIST_OPTIONS[a]].push(value);
  }
  return out;
}

function planCommand(io, options) {
  const { top, config } = loadHarnessConfig(io, options, true);
  const { snapshot, graph, chosen } = planInputs(top, config, options.alsoState);
  const plan = computePlan({ config, snapshot, graph, chosen });
  resolveViews(config, snapshot);
  io.stdout.write(canonicalJSON(publicPlan(plan)));
  io.stderr.write(summaryLine({
    command: 'plan',
    invocations: Number(plan.invocation1.length > 0) + Number(plan.invocation2.length > 0),
    scope: plan.scope.length,
    forced: plan.forced.length,
    deferrals: plan.deferrals.length,
    pendingFull: plan.pendingFull,
  }));
  return 0;
}

const COMMANDS = { plan: planCommand };

export async function main(args, io = { stdout: process.stdout, stderr: process.stderr, cwd: process.cwd() }) {
  try {
    const options = parseArgs(args);
    const command = COMMANDS[options.command];
    if (!command) throw new UsageError(options.command === undefined ? USAGE : `unknown command ${options.command}\n${USAGE}`);
    return await command(io, options);
  } catch (e) {
    // Known failures carry their exit code (2, 3, 4, 130). Anything else is an internal error and
    // exits 4: for run, exit 1 means "ok, state committed", so a crash must never produce it.
    const known = Number.isInteger(e.exitCode);
    io.stderr.write(`mutation-incremental: ${known ? 'error' : 'internal error'}: ${e.message}\n`);
    io.stderr.write(summaryLine({ command: args[0] ?? 'none' }));
    return known ? e.exitCode : 4;
  }
}
```

- [ ] **Step 4: Run the suite to verify it passes with 100% coverage**

Run: `sh .superpowers/sdd/2026-09-23-mutation-incremental-1b-execution/cov.sh` (the executor's coverage script: the whole Node suite with coverage, listing any uncovered line, branch or function)
Expected: all tests pass; no uncovered entries.

- [ ] **Step 5: Commit**

```bash
git add templates/mutation-incremental/lib/inputs.mjs templates/mutation-incremental/lib/proc.mjs templates/mutation-incremental/lib/errors.mjs templates/mutation-incremental/lib/state.mjs templates/mutation-incremental/lib/main.mjs templates/mutation-incremental/test/proc.test.mjs templates/mutation-incremental/test/json.test.mjs templates/mutation-incremental/test/state.test.mjs templates/mutation-incremental/test/main.test.mjs
git commit -m "feat(mutation-incremental): shared inputs, option grammar, exit codes and process groups

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: Lock and `break-lock`

**Files:**
- Create: `templates/mutation-incremental/lib/lock.mjs`, `test/lock.test.mjs`
- Modify: `lib/main.mjs` (register `break-lock`)

**Interfaces:**
- Consumes: `LockHeldError`, `loadHarnessConfig`, `summaryLine`, `canonicalPendingFull`.
- Produces:
  - `LOCK_FILE` (`'lock'`);
  - `pidAlive(pid): boolean`;
  - `acquireLock(stateDir, {pid, host, now}): release()`. The caller creates `stateDir`. It throws `LockHeldError` (exit 3);
  - `breakLockCommand(io, options): 0`.

- [ ] **Step 1: Write the failing tests**

`templates/mutation-incremental/test/lock.test.mjs`:

```js
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, writeFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { hostname, tmpdir } from 'node:os';
import { join } from 'node:path';
import { acquireLock, LOCK_FILE, pidAlive } from '../lib/lock.mjs';
import { main } from '../lib/main.mjs';
import { makeRepo } from './helpers.mjs';

const stateDir = () => mkdtempSync(join(tmpdir(), 'mi-lock-'));
const deadPid = () => spawnSync(process.execPath, ['-e', '']).pid;
const now = new Date('2026-09-23T18:00:00.000Z');
const capture = () => ({ text: '', write(s) { this.text += s; } });

test('acquireLock writes pid, hostname and startedAt; release removes it', () => {
  const dir = stateDir();
  const release = acquireLock(dir, { pid: 42, host: 'h', now });
  assert.deepEqual(JSON.parse(readFileSync(join(dir, LOCK_FILE), 'utf8')), { pid: 42, hostname: 'h', startedAt: '2026-09-23T18:00:00.000Z' });
  release();
  assert.equal(existsSync(join(dir, LOCK_FILE)), false);
  const own = stateDir();
  const releaseOwn = acquireLock(own);
  assert.deepEqual(Object.keys(JSON.parse(readFileSync(join(own, LOCK_FILE), 'utf8'))), ['pid', 'hostname', 'startedAt']);
  assert.equal(JSON.parse(readFileSync(join(own, LOCK_FILE), 'utf8')).pid, process.pid);
  releaseOwn();
});

test('a lock held by a live process on this host is exit 3 naming it', () => {
  const dir = stateDir();
  acquireLock(dir, { pid: process.pid, host: hostname(), now });
  assert.throws(() => acquireLock(dir, { pid: 7, host: hostname(), now }), (e) => e.exitCode === 3 && e.message.includes(`pid ${process.pid}`) && /break-lock/.test(e.message));
});

test('a lock left by a dead process on this host is replaced', () => {
  const dir = stateDir();
  acquireLock(dir, { pid: deadPid(), host: hostname(), now });
  acquireLock(dir, { pid: 7, host: hostname(), now });
  assert.equal(JSON.parse(readFileSync(join(dir, LOCK_FILE), 'utf8')).pid, 7);
});

test('a lock from another machine or an unreadable lock is held', () => {
  const dir = stateDir();
  acquireLock(dir, { pid: deadPid(), host: 'other-machine', now });
  assert.throws(() => acquireLock(dir, { pid: 7, host: hostname(), now }), (e) => e.exitCode === 3 && /other-machine/.test(e.message));
  const junk = stateDir();
  writeFileSync(join(junk, LOCK_FILE), '');
  assert.throws(() => acquireLock(junk, { pid: 7, host: hostname(), now }), (e) => e.exitCode === 3 && /unreadable lock/.test(e.message));
});

test('acquireLock rethrows anything but an existing lock', () => {
  assert.throws(() => acquireLock(join(stateDir(), 'missing'), { pid: 7, host: 'h', now }), (e) => e.code === 'ENOENT');
});

test('pidAlive', () => {
  assert.equal(pidAlive(process.pid), true);
  assert.equal(pidAlive(deadPid()), false);
  assert.equal(pidAlive(1), true); // init/launchd: EPERM for a normal user, alive either way
});

test('break-lock prints and removes the lock, or says there is none', async () => {
  const r = makeRepo();
  const run = async () => {
    const stdout = capture();
    const stderr = capture();
    const code = await main(['break-lock'], { stdout, stderr, cwd: r.top });
    return { code, stdout: stdout.text, stderr: stderr.text };
  };
  const none = await run();
  assert.equal(none.code, 0);
  assert.match(none.stdout, /no lock at .*\.mutation\/lock/);
  assert.match(none.stderr, /command=break-lock invocations=0 scope=0 forced=0 deferrals=0 pending_full=true\n$/);
  mkdirSync(join(r.top, '.mutation'), { recursive: true });
  writeFileSync(join(r.top, '.mutation/lock'), '{"pid":1,"hostname":"ci","startedAt":"x"}\n');
  const held = await run();
  assert.match(held.stdout, /\{"pid":1,"hostname":"ci","startedAt":"x"\}\nbreak-lock: removed /);
  assert.equal(existsSync(join(r.top, '.mutation/lock')), false);
});
```

- [ ] **Step 2: Run to verify failure**

Run: `node --test templates/mutation-incremental/test/lock.test.mjs`
Expected: FAIL — `lib/lock.mjs` does not exist.

- [ ] **Step 3: Implement**

`templates/mutation-incremental/lib/lock.mjs`:

```js
import { readFileSync, rmSync, writeFileSync } from 'node:fs';
import { hostname } from 'node:os';
import { join } from 'node:path';
import { LockHeldError } from './errors.mjs';
import { loadHarnessConfig, summaryLine } from './inputs.mjs';
import { canonicalPendingFull } from './state.mjs';

export const LOCK_FILE = 'lock';

const readText = (path) => {
  try {
    return readFileSync(path, 'utf8');
  } catch {
    return null;
  }
};

function readLock(path) {
  try {
    return JSON.parse(readText(path));
  } catch {
    return null;
  }
}

// kill(pid, 0) probes without signalling; EPERM means the process exists under another user.
export function pidAlive(pid) {
  try {
    process.kill(pid, 0);
    return true;
  } catch (e) {
    return e.code === 'EPERM';
  }
}

// Spec §5.6 step 1: create <stateDir>/lock with O_EXCL. A lock from this host whose pid is gone is
// a crashed run and is replaced; anything else is held (exit 3). The caller creates stateDir.
export function acquireLock(stateDir, { pid = process.pid, host = hostname(), now = new Date() } = {}) {
  const path = join(stateDir, LOCK_FILE);
  const content = `${JSON.stringify({ pid, hostname: host, startedAt: now.toISOString() })}\n`;
  try {
    writeFileSync(path, content, { flag: 'wx' });
  } catch (e) {
    if (e.code !== 'EEXIST') throw e;
    const held = readLock(path);
    if (held === null || held.hostname !== host || pidAlive(held.pid)) {
      const who = held === null ? 'unreadable lock' : `pid ${held.pid} on ${held.hostname} since ${held.startedAt}`;
      throw new LockHeldError(`lock held: ${path} (${who}); if no run is active, run break-lock`);
    }
    writeFileSync(path, content);
  }
  return () => rmSync(path, { force: true });
}

// Spec §5.1: print and remove <stateDir>/lock (clears a lock left on another machine).
export async function breakLockCommand(io, options) {
  const { config } = loadHarnessConfig(io, options, false);
  const path = join(config.stateDir, LOCK_FILE);
  const text = readText(path);
  if (text === null) io.stdout.write(`break-lock: no lock at ${path}\n`);
  else {
    io.stdout.write(text);
    rmSync(path, { force: true });
    io.stdout.write(`break-lock: removed ${path}\n`);
  }
  io.stderr.write(summaryLine({ command: 'break-lock', pendingFull: canonicalPendingFull(config) }));
  return 0;
}
```

Edit `templates/mutation-incremental/lib/main.mjs` — add the import and the table entry:

```js
import { breakLockCommand } from './lock.mjs';
```

```js
const COMMANDS = { plan: planCommand, 'break-lock': breakLockCommand };
```

- [ ] **Step 4: Run the suite with coverage**

Run: `sh .superpowers/sdd/2026-09-23-mutation-incremental-1b-execution/cov.sh`
Expected: all pass; no uncovered entries.

- [ ] **Step 5: Commit**

```bash
git add templates/mutation-incremental/lib/lock.mjs templates/mutation-incremental/lib/main.mjs templates/mutation-incremental/test/lock.test.mjs
git commit -m "feat(mutation-incremental): state lock and break-lock

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---
### Task 3: Attestation writing and the commit

**Files:**
- Create: `templates/mutation-incremental/lib/attest.mjs`, `test/attest.test.mjs`
- Modify: `lib/snapshot.mjs` (export `fileDigest`, use it in `takeSnapshot`)

**Interfaces:**
- Consumes: `sha256`, `canonicalJSON`, the constants of `state.mjs`, `config.{engineVersion, thresholdBreak, lists, exclusions, configRel, configPath, top}`.
- Produces:
  - `fileDigest(config, path): string|null`;
  - `WORK_DIR` (`'work'`), `CHANGED_DURING_RUN`;
  - `mutationScore(report): number|null`;
  - `buildAttestation({config, files, runtime, reportBytes, report, mode, completedAt, lastFullAt, deferrals, views, summaries}): object`;
  - `markChangedDuringRun(config, files): files`;
  - `commitState(stateDir, outputPath, attestation)`;
  - `installAdopted(stateDir, fromDir)`.

- [ ] **Step 1: Write the failing tests**

`templates/mutation-incremental/test/attest.test.mjs`:

```js
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { loadConfig } from '../lib/config.mjs';
import { sha256, takeSnapshot } from '../lib/snapshot.mjs';
import { buildAttestation, CHANGED_DURING_RUN, commitState, installAdopted, markChangedDuringRun, mutationScore, WORK_DIR } from '../lib/attest.mjs';
import { readCandidate, STATE_VERSION } from '../lib/state.mjs';
import { makeRepo, writeState } from './helpers.mjs';

const m = (status) => ({ status });
const report = { files: { 'src/a.ts': { mutants: [m('Killed'), m('Survived')] } } };

function fixture() {
  const r = makeRepo({ files: { 'src/a.ts': 'a', 'src/b.ts': 'b' } });
  const config = loadConfig(r.top);
  return { r, config, snapshot: takeSnapshot(config) };
}
const build = (config, snapshot, extra = {}) => buildAttestation({
  config, files: snapshot.files, runtime: snapshot.runtime, reportBytes: Buffer.from('{}'), report, mode: 'incremental',
  completedAt: 'c', lastFullAt: 'l', deferrals: [], views: null, summaries: null, ...extra,
});

test('mutationScore follows Stryker: Timeout counts as detected, other statuses are ignored', () => {
  assert.equal(mutationScore({ files: { a: { mutants: [m('Killed'), m('Timeout'), m('Survived'), m('NoCoverage'), m('CompileError'), m('Ignored')] } } }), 50);
  assert.equal(mutationScore({ files: { a: { mutants: [m('CompileError')] }, b: {} } }), null);
});

test('buildAttestation writes the §5.5 contract', () => {
  const { config, snapshot } = fixture();
  const att = build(config, snapshot);
  assert.deepEqual(Object.keys(att).sort(), [
    'completedAt', 'deferrals', 'engine', 'engineVersion', 'exclusions', 'files', 'lastFullAt', 'lists', 'mode', 'report',
    'reportSha256', 'runtime', 'schemaVersion', 'score', 'stateVersion', 'thresholdBreak', 'tool', 'toolVersion',
  ]);
  assert.deepEqual(
    [att.schemaVersion, att.tool, att.toolVersion, att.stateVersion, att.engine, att.engineVersion, att.report, att.reportSha256, att.score],
    [1, 'metareview-mutation-incremental', '0.13.0', STATE_VERSION, 'stryker', '10.0.0', 'incremental.json', sha256('{}'), 50],
  );
  assert.equal(att.thresholdBreak, false); // no thresholds.break configured
  assert.deepEqual([att.files, att.lists, att.exclusions], [snapshot.files, config.lists, config.exclusions]);
});

test('thresholdBreak: strictly below the break, never for a null score', () => {
  const { config, snapshot } = fixture();
  assert.equal(build({ ...config, thresholdBreak: 60 }, snapshot).thresholdBreak, true);
  assert.equal(build({ ...config, thresholdBreak: 50 }, snapshot).thresholdBreak, false);
  assert.equal(build({ ...config, thresholdBreak: 60 }, snapshot, { report: { files: {} } }).thresholdBreak, false);
});

test('views and viewSummaries are written only with views', () => {
  const { config, snapshot } = fixture();
  const att = build(config, snapshot, { views: { core: ['src/**'] }, summaries: { core: { Killed: 1 } } });
  assert.deepEqual([att.views, att.viewSummaries], [{ core: ['src/**'] }, { core: { Killed: 1 } }]);
});

test('markChangedDuringRun flags edited and deleted files and keeps the rest', () => {
  const { r, config, snapshot } = fixture();
  r.write('src/a.ts', 'changed');
  rmSync(join(r.top, 'src/b.ts'));
  const files = markChangedDuringRun(config, snapshot.files);
  assert.deepEqual([files['src/a.ts'].digest, files['src/b.ts'].digest], [CHANGED_DURING_RUN, CHANGED_DURING_RUN]);
  assert.deepEqual(files['mutation-incremental.json'], snapshot.files['mutation-incremental.json']);
  assert.deepEqual(files['stryker.config.json'], snapshot.files['stryker.config.json']);
});

test('commitState renames the output and then the attestation into place and removes work/', () => {
  const { config, snapshot } = fixture();
  const work = join(config.stateDir, WORK_DIR);
  mkdirSync(work, { recursive: true });
  const bytes = Buffer.from(JSON.stringify(report));
  writeFileSync(join(work, 'inv-2.json'), bytes);
  commitState(config.stateDir, join(work, 'inv-2.json'), build(config, snapshot, { reportBytes: bytes }));
  assert.equal(readCandidate(config.stateDir, { label: 's', primary: true }).usable, true);
  assert.equal(existsSync(work), false);
  // An unchanged canonical output is only re-attested.
  commitState(config.stateDir, join(config.stateDir, 'incremental.json'), build(config, snapshot, { reportBytes: bytes, mode: 'full' }));
  assert.equal(JSON.parse(readFileSync(join(config.stateDir, 'attestation.json'), 'utf8')).mode, 'full');
  assert.equal(readCandidate(config.stateDir, { label: 's', primary: true }).usable, true);
});

test('installAdopted copies both files of the adopted state into stateDir', () => {
  const { r, config } = fixture();
  writeState(join(r.top, '.mutation/remote/full'), { report });
  installAdopted(config.stateDir, join(r.top, '.mutation/remote/full'));
  assert.equal(readCandidate(config.stateDir, { label: 's', primary: true }).usable, true);
});
```

- [ ] **Step 2: Run to verify failure**

Run: `node --test templates/mutation-incremental/test/attest.test.mjs`
Expected: FAIL — `lib/attest.mjs` does not exist.

- [ ] **Step 3: Implement**

Edit `templates/mutation-incremental/lib/snapshot.mjs`. After the `configDigest` function, add:

```js
// The digest a snapshot records for one path: the config file without its views (spec K3.4),
// anything else by content or link text.
export const fileDigest = (config, path) => (path === config.configRel ? configDigest(config.configPath) : digestOf(join(config.top, path)));
```

and in `takeSnapshot` replace

```js
    const digest = path === config.configRel ? configDigest(config.configPath) : digestOf(join(config.top, path));
```

with

```js
    const digest = fileDigest(config, path);
```

`templates/mutation-incremental/lib/attest.mjs`:

```js
import { copyFileSync, mkdirSync, renameSync, rmSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { canonicalJSON } from './json.mjs';
import { fileDigest, sha256 } from './snapshot.mjs';
import { ATTESTATION_FILE, REPORT_FILE, STATE_VERSION, TOOL, TOOL_VERSION } from './state.mjs';

export const WORK_DIR = 'work';
export const CHANGED_DURING_RUN = 'changed-during-run';
const SCORED = ['Killed', 'Timeout', 'Survived', 'NoCoverage'];

// Stryker's definition (spec §5.5): (Killed + Timeout) / (Killed + Timeout + Survived + NoCoverage)
// × 100, or null when that denominator is 0.
export function mutationScore(report) {
  const n = Object.fromEntries(SCORED.map((s) => [s, 0]));
  for (const entry of Object.values(report.files)) {
    for (const m of entry.mutants ?? []) if (Object.hasOwn(n, m.status)) n[m.status]++;
  }
  const denominator = n.Killed + n.Timeout + n.Survived + n.NoCoverage;
  return denominator === 0 ? null : ((n.Killed + n.Timeout) / denominator) * 100;
}

// Spec §5.5 with §11.2 (inherited flags on deferrals), §11.3 (views) and §11.6 (stateVersion).
export function buildAttestation({ config, files, runtime, reportBytes, report, mode, completedAt, lastFullAt, deferrals, views, summaries }) {
  const score = mutationScore(report);
  const attestation = {
    schemaVersion: 1,
    tool: TOOL,
    toolVersion: TOOL_VERSION,
    stateVersion: STATE_VERSION,
    engine: 'stryker',
    engineVersion: config.engineVersion,
    mode,
    completedAt,
    lastFullAt,
    report: REPORT_FILE,
    reportSha256: sha256(reportBytes),
    score,
    // A null score or an absent threshold never breaks.
    thresholdBreak: score !== null && config.thresholdBreak !== null && score < config.thresholdBreak,
    lists: config.lists,
    exclusions: config.exclusions,
    files,
    runtime,
    deferrals,
  };
  if (views !== null) Object.assign(attestation, { views, viewSummaries: summaries });
  return attestation;
}

// Spec §5.5: a path whose digest at the end of the run differs from the start-of-run snapshot is
// recorded as changed-during-run, so it never matches and is re-planned next time.
export function markChangedDuringRun(config, files) {
  return Object.fromEntries(Object.entries(files).map(([path, info]) => [path, fileDigest(config, path) === info.digest ? info : { ...info, digest: CHANGED_DURING_RUN }]));
}

// Spec §5.6 step 7: the attestation goes to work/, the output is renamed over incremental.json,
// then the attestation over attestation.json. A crash between the renames leaves a pair whose
// hashes disagree, which is cold next time and never trusted.
export function commitState(stateDir, outputPath, attestation) {
  const work = join(stateDir, WORK_DIR);
  mkdirSync(work, { recursive: true });
  const pending = join(work, ATTESTATION_FILE);
  writeFileSync(pending, canonicalJSON(attestation));
  const canonical = join(stateDir, REPORT_FILE);
  if (outputPath !== canonical) renameSync(outputPath, canonical);
  renameSync(pending, join(stateDir, ATTESTATION_FILE));
  rmSync(work, { recursive: true, force: true });
}

// Spec §5.4 Adopt: run copies an adopted state's two files into stateDir (temp file + rename),
// report first, so an interrupted copy leaves hashes that disagree.
export function installAdopted(stateDir, fromDir) {
  const work = join(stateDir, WORK_DIR);
  mkdirSync(work, { recursive: true });
  for (const name of [REPORT_FILE, ATTESTATION_FILE]) {
    const tmp = join(work, `adopt-${name}`);
    copyFileSync(join(fromDir, name), tmp);
    renameSync(tmp, join(stateDir, name));
  }
}
```

- [ ] **Step 4: Run the suite with coverage**

Run: `sh .superpowers/sdd/2026-09-23-mutation-incremental-1b-execution/cov.sh`
Expected: all pass; no uncovered entries.

- [ ] **Step 5: Commit**

```bash
git add templates/mutation-incremental/lib/attest.mjs templates/mutation-incremental/lib/snapshot.mjs templates/mutation-incremental/test/attest.test.mjs
git commit -m "feat(mutation-incremental): attestation building and the two-rename commit

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 4: One Stryker invocation, judged — with a scripted fake Stryker

**Files:**
- Create: `templates/mutation-incremental/lib/engine.mjs`, `test/engine.test.mjs`
- Create: `templates/mutation-incremental/test/fake-stryker.mjs`, `test/fake.mjs` (test helpers used by Tasks 4–8)

**Interfaces:**
- Consumes: `spawnGroup`, `WORK_DIR`, `config.{stryker, stateDir, stateRel, top}`.
- Produces:
  - `NO_TESTS`;
  - `strykerArgv(config, incrementalFile, {force, mutate}): string[]`;
  - `parseReport(bytes): object|null`;
  - `invoke(config, {index, input, force, mutate, timeoutMs, interrupt, graceMs, onOutput}): Promise<{outcome: 'success', path, bytes, report} | {outcome: 'no-tests'|'timeout'|'interrupted'} | {outcome: 'failed', detail}>`. The caller creates `<stateDir>/work/`.
- Test helpers (`test/fake.mjs`):
  - `A` (the fixture `src/a.ts`);
  - `reportFor(source)`;
  - `runRepo({config, stryker, files, steps})`, which returns a `makeRepo` result plus `steps(list)`, `calls()` and `read(rel)`;
  - `cli(r, args, {killGraceMs})`, which returns `{code, stdout, stderr, output}` (`output` is the parsed `GITHUB_OUTPUT`);
  - `warm(r)`.

- [ ] **Step 1: Write the test double and helpers**

`templates/mutation-incremental/test/fake-stryker.mjs`:

```js
// Test double for `stryker run` (spec F4–F6), driven by .fake/steps.json in the cwd: one step per
// invocation (the last step repeats). A step may print `output`, write `touch` files, sleep
// `sleepMs` (ignoring SIGTERM with `ignoreTerm`), write `report` (an object, or "input" to rewrite
// the incremental file's own bytes) and exit with `exit` (default 0). Each call's argv is appended
// to .fake/calls.jsonl.
import { appendFileSync, existsSync, readFileSync, writeFileSync } from 'node:fs';

const argv = process.argv.slice(2);
const log = '.fake/calls.jsonl';
const n = existsSync(log) ? readFileSync(log, 'utf8').split('\n').filter(Boolean).length : 0;
appendFileSync(log, `${JSON.stringify(argv)}\n`);
const steps = JSON.parse(readFileSync('.fake/steps.json', 'utf8'));
const step = steps[Math.min(n, steps.length - 1)];
const file = argv[argv.indexOf('--incrementalFile') + 1];
if (step.ignoreTerm) process.on('SIGTERM', () => {});
if (step.output) process.stdout.write(step.output);
for (const [path, text] of Object.entries(step.touch ?? {})) writeFileSync(path, text);
await new Promise((resolve) => setTimeout(resolve, step.sleepMs ?? 0));
if (step.report === 'input') writeFileSync(file, readFileSync(file));
else if (step.report) writeFileSync(file, JSON.stringify(step.report));
process.exit(step.exit ?? 0);
```

`templates/mutation-incremental/test/fake.mjs`:

```js
import { existsSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { main } from '../lib/main.mjs';
import { makeRepo } from './helpers.mjs';

const FAKE = readFileSync(new URL('./fake-stryker.mjs', import.meta.url), 'utf8');

// src/a.ts: line 2 has a kill (by tests/a.test.ts), line 3 a survivor it covers.
export const A = 'export function clamp(n, lo, hi) {\n  if (n < lo) return lo;\n  if (n > hi) return hi;\n  return n;\n}\n';
const mutant = (id, line, status, killedBy) => ({
  id, mutatorName: 'ConditionalExpression', replacement: 'true', status, killedBy, coveredBy: ['t1'],
  location: { start: { line, column: 7 }, end: { line, column: 13 } },
});
export function reportFor(source = A) {
  return {
    schemaVersion: '1',
    thresholds: { high: 80, low: 60 },
    files: { 'src/a.ts': { language: 'typescript', source, mutants: [mutant('1', 2, 'Killed', ['t1']), mutant('2', 3, 'Survived', [])] } },
    testFiles: { 'tests/a.test.ts': { tests: [{ id: 't1', name: 'clamps low' }] } },
  };
}

// A throwaway repository whose Stryker is the fake. .fake/ (the fake, its steps, its call log and
// GITHUB_OUTPUT) is gitignored, so it never enters a snapshot.
export function runRepo({ config = {}, stryker = {}, files = {}, steps = [{ report: reportFor() }] } = {}) {
  const r = makeRepo({
    config: { stryker: { command: [process.execPath, '.fake/stryker.mjs'], configFile: 'stryker.config.json', extraArgs: [] }, ...config },
    stryker,
    files: {
      '.gitignore': 'node_modules/\n.mutation/\n.fake/\n',
      '.fake/stryker.mjs': FAKE,
      'src/a.ts': A,
      'tests/a.test.ts': "import { clamp } from '../src/a';\n",
      ...files,
    },
  });
  const log = join(r.top, '.fake/calls.jsonl');
  // Replacing the steps also restarts the call log, so step 0 is the next invocation.
  r.steps = (list) => {
    writeFileSync(join(r.top, '.fake/steps.json'), JSON.stringify(list));
    rmSync(log, { force: true });
  };
  r.calls = () => (existsSync(log) ? readFileSync(log, 'utf8').split('\n').filter(Boolean).map((l) => JSON.parse(l)) : []);
  r.read = (rel) => JSON.parse(readFileSync(join(r.top, rel), 'utf8'));
  r.steps(steps);
  return r;
}

const capture = () => ({ text: '', write(s) { this.text += String(s); } });

// Runs the CLI in-process with GITHUB_OUTPUT pointed at .fake/output, returned parsed.
export async function cli(r, args, { killGraceMs = 200 } = {}) {
  const stdout = capture();
  const stderr = capture();
  const outFile = join(r.top, '.fake/output');
  writeFileSync(outFile, '');
  const saved = process.env.GITHUB_OUTPUT;
  process.env.GITHUB_OUTPUT = outFile;
  try {
    const code = await main(args, { stdout, stderr, cwd: r.top, killGraceMs });
    const lines = readFileSync(outFile, 'utf8').split('\n').filter(Boolean);
    const output = Object.fromEntries(lines.map((l) => [l.slice(0, l.indexOf('=')), l.slice(l.indexOf('=') + 1)]));
    return { code, stdout: stdout.text, stderr: stderr.text, output };
  } finally {
    if (saved === undefined) delete process.env.GITHUB_OUTPUT;
    else process.env.GITHUB_OUTPUT = saved;
  }
}

// A full run that leaves a usable state; later invocations echo their input file.
export async function warm(r) {
  const res = await cli(r, ['run', '--mode', 'full']);
  if (res.code !== 0) throw new Error(`warm-up full run failed (${res.code}): ${res.stderr}`);
  r.steps([{ report: 'input' }]);
}
```

- [ ] **Step 2: Write the failing engine tests**

`templates/mutation-incremental/test/engine.test.mjs`:

```js
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { loadConfig } from '../lib/config.mjs';
import { invoke, NO_TESTS, parseReport, strykerArgv } from '../lib/engine.mjs';
import { createInterrupt } from '../lib/proc.mjs';
import { reportFor, runRepo } from './fake.mjs';

function setup(steps, extra = {}) {
  const r = runRepo({ steps, ...extra });
  const config = loadConfig(r.top);
  mkdirSync(join(config.stateDir, 'work'), { recursive: true });
  return { r, config };
}
const call = (config, over = {}) => invoke(config, {
  index: 1, input: null, force: false, mutate: null, timeoutMs: null, interrupt: createInterrupt(), graceMs: 200, onOutput: () => {}, ...over,
});

test('strykerArgv builds the invocation without a shell', () => {
  const config = { stryker: { command: ['npx', 'stryker'], configFile: 's.json', extraArgs: ['--concurrency', '2'] } };
  assert.deepEqual(strykerArgv(config, '.m/work/inv-2.json', { force: true, mutate: ['a.ts', 'b.ts:1-4'] }),
    ['npx', 'stryker', 'run', 's.json', '--incremental', '--incrementalFile', '.m/work/inv-2.json', '--force', '--mutate', 'a.ts,b.ts:1-4', '--concurrency', '2']);
  assert.deepEqual(strykerArgv(config, 'f.json', { force: false, mutate: null }),
    ['npx', 'stryker', 'run', 's.json', '--incremental', '--incrementalFile', 'f.json', '--concurrency', '2']);
});

test('parseReport accepts only an object with a files object', () => {
  for (const bad of ['x', 'null', '[]', '{}', '{"files":[]}', '{"files":null}', '{"files":5}']) assert.equal(parseReport(Buffer.from(bad)), null, bad);
  assert.deepEqual(parseReport(Buffer.from('{"files":{}}')), { files: {} });
});

test('a written, parseable output with exit 0 or 1 is a success', async () => {
  for (const exit of [0, 1]) {
    const { config } = setup([{ report: reportFor(), exit }]);
    const r = await call(config);
    assert.equal(r.outcome, 'success');
    assert.equal(r.path, join(config.stateDir, 'work/inv-1.json'));
    assert.deepEqual(r.report, reportFor());
    assert.equal(r.bytes.toString(), JSON.stringify(reportFor()));
  }
});

test('the input is copied to work/inv-<k>.json first; rewriting it counts as written', async () => {
  const { r, config } = setup([{ report: 'input' }]);
  const input = join(config.stateDir, 'incremental.json');
  writeFileSync(input, JSON.stringify(reportFor()));
  const res = await call(config, { index: 2, input, force: true, mutate: ['src/a.ts:2-3'] });
  assert.equal(res.outcome, 'success');
  assert.deepEqual(r.calls(), [['run', 'stryker.config.json', '--incremental', '--incrementalFile', '.mutation/work/inv-2.json', '--force', '--mutate', 'src/a.ts:2-3']]);
});

test('exit 1 with nothing written and "No tests were executed" is no-tests; output is forwarded', async () => {
  const { config } = setup([{ exit: 1, output: `INFO ${NO_TESTS}. Stryker will exit prematurely.\n` }]);
  let seen = '';
  const r = await call(config, { onOutput: (c) => { seen += c; } });
  assert.equal(r.outcome, 'no-tests');
  assert.match(seen, /No tests were executed/);
});

test('an unwritten output, an unparseable one or another exit code is a failure', async () => {
  const cases = [
    [{ exit: 1 }, /exit 1/],
    [{ exit: 0 }, /exit 0/],
    [{ exit: 2, report: reportFor() }, /exit 2/],
    [{ exit: 0, report: [1] }, /exit 0/],
  ];
  for (const [step, detail] of cases) {
    const { config } = setup([step]);
    const r = await call(config);
    assert.equal(r.outcome, 'failed', JSON.stringify(step));
    assert.match(r.detail, detail);
  }
});

test('a timeout and an interrupt are reported as such', async () => {
  const { config } = setup([{ sleepMs: 10000 }]);
  assert.equal((await call(config, { timeoutMs: 400 })).outcome, 'timeout');
  const interrupt = createInterrupt();
  setTimeout(() => interrupt.trigger(), 400);
  assert.equal((await call(config, { interrupt })).outcome, 'interrupted');
});

test('a Stryker command that cannot be spawned is a failure naming the error', async () => {
  const { config } = setup([{}], { config: { stryker: { command: ['/nonexistent/stryker'], configFile: 'stryker.config.json', extraArgs: [] } } });
  const r = await call(config);
  assert.equal(r.outcome, 'failed');
  assert.match(r.detail, /ENOENT/);
});
```

- [ ] **Step 3: Run to verify failure**

Run: `node --test templates/mutation-incremental/test/engine.test.mjs`
Expected: FAIL — `lib/engine.mjs` does not exist.

- [ ] **Step 4: Implement**

`templates/mutation-incremental/lib/engine.mjs`:

```js
import { copyFileSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { spawnGroup } from './proc.mjs';
import { WORK_DIR } from './attest.mjs';

// Stryker prints this on stdout and stderr when the dry run finds no tests (spec F4).
export const NO_TESTS = 'No tests were executed';

// Spec §5.6 step 4: argv, no shell.
export function strykerArgv(config, incrementalFile, { force, mutate }) {
  return [
    ...config.stryker.command, 'run', config.stryker.configFile,
    '--incremental', '--incrementalFile', incrementalFile,
    ...(force ? ['--force'] : []),
    ...(mutate === null ? [] : ['--mutate', mutate.join(',')]),
    ...config.stryker.extraArgs,
  ];
}

const isObject = (v) => v !== null && typeof v === 'object' && !Array.isArray(v);

// A mutation-testing-report with a `files` object (selected files absent from it have zero
// mutants, spec F9), or null.
export function parseReport(bytes) {
  let report;
  try {
    report = JSON.parse(bytes.toString('utf8'));
  } catch {
    return null;
  }
  return isObject(report) && isObject(report.files) ? report : null;
}

// mtime (ns), size and inode: "written during the invocation" (spec §5.6 step 5).
function stamp(path) {
  try {
    const s = statSync(path, { bigint: true });
    return `${s.mtimeNs}:${s.size}:${s.ino}`;
  } catch {
    return null;
  }
}

// One invocation (spec §5.6 steps 4–5). `input` is copied to work/inv-<index>.json first; null in
// full mode, where Stryker starts fresh. The caller creates work/.
export async function invoke(config, { index, input, force, mutate, timeoutMs, interrupt, graceMs, onOutput }) {
  const name = `inv-${index}.json`;
  const file = join(config.stateDir, WORK_DIR, name);
  if (input !== null) copyFileSync(input, file);
  const before = stamp(file);
  let tail = '';
  let noTests = false;
  const r = await spawnGroup(strykerArgv(config, `${config.stateRel}/${WORK_DIR}/${name}`, { force, mutate }), {
    cwd: config.top,
    env: process.env,
    timeoutMs,
    graceMs,
    interrupt,
    onOutput: (chunk) => {
      onOutput(chunk);
      const text = tail + chunk.toString('utf8');
      if (text.includes(NO_TESTS)) noTests = true;
      tail = text.slice(-NO_TESTS.length);
    },
  });
  if (r.interrupted) return { outcome: 'interrupted' };
  if (r.timedOut) return { outcome: 'timeout' };
  const after = stamp(file);
  const written = after !== null && after !== before;
  if (written && (r.code === 0 || r.code === 1)) {
    const bytes = readFileSync(file);
    const report = parseReport(bytes);
    if (report !== null) return { outcome: 'success', path: file, bytes, report };
  }
  if (!written && r.code === 1 && noTests) return { outcome: 'no-tests' };
  return { outcome: 'failed', detail: r.error ? r.error.message : `exit ${r.code}` };
}
```

- [ ] **Step 5: Run the suite with coverage**

Run: `sh .superpowers/sdd/2026-09-23-mutation-incremental-1b-execution/cov.sh`
Expected: all pass; no uncovered entries.

- [ ] **Step 6: Commit**

```bash
git add templates/mutation-incremental/lib/engine.mjs templates/mutation-incremental/test/engine.test.mjs templates/mutation-incremental/test/fake-stryker.mjs templates/mutation-incremental/test/fake.mjs
git commit -m "feat(mutation-incremental): judged Stryker invocation with a scripted test double

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 5: The project verifier

**Files:**
- Create: `templates/mutation-incremental/lib/verify.mjs`, `test/verify.test.mjs`

**Interfaces:**
- Consumes: `spawnGroup`, `viewFiles(views, snapshot)`, `REPORT_FILE`, `ATTESTATION_FILE`, `config.verify`.
- Produces: `runVerify(config, {views, snapshot, pr, interrupt, graceMs, onOutput}): Promise<string[]>` (one line per failure; empty when all pass or nothing is configured; an interrupt stops the loop).

- [ ] **Step 1: Write the failing tests**

`templates/mutation-incremental/test/verify.test.mjs`:

```js
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { loadConfig } from '../lib/config.mjs';
import { takeSnapshot } from '../lib/snapshot.mjs';
import { createInterrupt } from '../lib/proc.mjs';
import { runVerify } from '../lib/verify.mjs';
import { runRepo } from './fake.mjs';

const verifyConfig = (script, timeoutMinutes = null) => ({ verify: { command: [process.execPath, '-e', script], timeoutMinutes } });
// The verifier records what it was given in .fake/verify.jsonl.
const RECORD = "require('fs').appendFileSync('.fake/verify.jsonl', JSON.stringify({ view: process.env.MUTATION_VIEW ?? null, patterns: process.env.MUTATION_VIEW_PATTERNS ?? null, files: process.env.MUTATION_VIEW_FILES ?? null, kind: process.env.MUTATION_RUN_KIND, report: process.env.MUTATION_REPORT, attestation: process.env.MUTATION_ATTESTATION }) + '\\n');";
const records = (r) => readFileSync(join(r.top, '.fake/verify.jsonl'), 'utf8').split('\n').filter(Boolean).map((l) => JSON.parse(l));
const go = (config, extra = {}) => runVerify(config, {
  views: null, snapshot: takeSnapshot(config), pr: false, interrupt: createInterrupt(), graceMs: 200, onOutput: () => {}, ...extra,
});
const VIEWS = { core: ['src/**'], hot: ['src/a.ts'] };

test('nothing configured runs nothing', async () => {
  assert.deepEqual(await go(loadConfig(runRepo().top)), []);
});

test('without views the verifier runs once, with the report paths and no view variables', async () => {
  const r = runRepo({ config: verifyConfig(RECORD) });
  const config = loadConfig(r.top);
  process.env.MUTATION_VIEW = 'leaked';
  try {
    assert.deepEqual(await go(config), []);
  } finally {
    delete process.env.MUTATION_VIEW;
  }
  assert.deepEqual(records(r), [{
    view: null, patterns: null, files: null, kind: 'other',
    report: join(config.stateDir, 'incremental.json'), attestation: join(config.stateDir, 'attestation.json'),
  }]);
});

test('with views it runs once per view, names each failing view and marks PR runs', async () => {
  const r = runRepo({ config: verifyConfig(`${RECORD} process.exit(process.env.MUTATION_VIEW === 'hot' ? 3 : 0);`), files: { 'src/b.ts': 'export const b = 1;\n' } });
  const config = loadConfig(r.top);
  assert.deepEqual(await go(config, { views: VIEWS, pr: true }), ['verify (view hot): exit 3']);
  assert.deepEqual(records(r).map((x) => [x.view, x.patterns, x.files, x.kind]), [
    ['core', '["src/**"]', '["src/a.ts","src/b.ts"]', 'pr'],
    ['hot', '["src/a.ts"]', '["src/a.ts"]', 'pr'],
  ]);
});

test('a verifier that cannot be spawned or exceeds its timeout fails', async () => {
  const missing = runRepo({ config: { verify: { command: ['/nonexistent/verifier'], timeoutMinutes: null } } });
  assert.match((await go(loadConfig(missing.top)))[0], /^verify: cannot run \/nonexistent\/verifier: .*ENOENT/);
  const slow = runRepo({ config: verifyConfig('setTimeout(() => {}, 20000)', 0.005) });
  assert.deepEqual(await go(loadConfig(slow.top)), ['verify: exceeded verify.timeoutMinutes (0.005)']);
});

test('an interrupt stops the loop without a failure line', async () => {
  const r = runRepo({ config: verifyConfig(`${RECORD} setTimeout(() => {}, 20000);`) });
  const interrupt = createInterrupt();
  setTimeout(() => interrupt.trigger(), 600);
  assert.deepEqual(await go(loadConfig(r.top), { views: VIEWS, interrupt }), []);
  assert.equal(records(r).length, 1);
});
```

- [ ] **Step 2: Run to verify failure**

Run: `node --test templates/mutation-incremental/test/verify.test.mjs`
Expected: FAIL — `lib/verify.mjs` does not exist.

- [ ] **Step 3: Implement**

`templates/mutation-incremental/lib/verify.mjs`:

```js
import { join } from 'node:path';
import { spawnGroup } from './proc.mjs';
import { ATTESTATION_FILE, REPORT_FILE } from './state.mjs';
import { viewFiles } from './views.mjs';

const VIEW_VARS = ['MUTATION_VIEW', 'MUTATION_VIEW_PATTERNS', 'MUTATION_VIEW_FILES'];

// Spec §11.1: after state is committed, run the project's verifier once per view (once without
// views), no shell, cwd = top-level, own process group. Any failure (non-zero exit, a command that
// cannot be spawned, exceeding timeoutMinutes) is one returned line naming the view; the run then
// exits 1. An interrupt stops the loop; the caller exits 130.
export async function runVerify(config, { views, snapshot, pr, interrupt, graceMs, onOutput }) {
  if (config.verify === null) return [];
  const base = {
    ...process.env,
    MUTATION_REPORT: join(config.stateDir, REPORT_FILE),
    MUTATION_ATTESTATION: join(config.stateDir, ATTESTATION_FILE),
    MUTATION_RUN_KIND: pr ? 'pr' : 'other',
  };
  for (const name of VIEW_VARS) delete base[name];
  const runs = views === null
    ? [{ label: 'verify', env: base }]
    : Object.entries(views).map(([name, patterns]) => ({
      label: `verify (view ${name})`,
      env: { ...base, MUTATION_VIEW: name, MUTATION_VIEW_PATTERNS: JSON.stringify(patterns), MUTATION_VIEW_FILES: JSON.stringify(viewFiles({ [name]: patterns }, snapshot)[name]) },
    }));
  const timeoutMs = config.verify.timeoutMinutes === null ? null : config.verify.timeoutMinutes * 60_000;
  const failures = [];
  for (const run of runs) {
    const r = await spawnGroup(config.verify.command, { cwd: config.top, env: run.env, timeoutMs, graceMs, interrupt, onOutput });
    if (r.interrupted) break;
    if (r.error) failures.push(`${run.label}: cannot run ${config.verify.command[0]}: ${r.error.message}`);
    else if (r.timedOut) failures.push(`${run.label}: exceeded verify.timeoutMinutes (${config.verify.timeoutMinutes})`);
    else if (r.code !== 0) failures.push(`${run.label}: exit ${r.code}`);
  }
  return failures;
}
```

- [ ] **Step 4: Run the suite with coverage**

Run: `sh .superpowers/sdd/2026-09-23-mutation-incremental-1b-execution/cov.sh`
Expected: all pass; no uncovered entries.

- [ ] **Step 5: Commit**

```bash
git add templates/mutation-incremental/lib/verify.mjs templates/mutation-incremental/test/verify.test.mjs
git commit -m "feat(mutation-incremental): per-view project verifier

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---
### Task 6: The `run` command

**Files:**
- Create: `templates/mutation-incremental/lib/run.mjs`, `test/run.test.mjs`
- Modify: `lib/main.mjs` (register `run`)

**Interfaces:**
- Consumes:
  - `loadHarnessConfig`, `planInputs`, `summaryLine`, `acquireLock`;
  - `computePlan({config, snapshot, graph, chosen, unbudgeted})`, which returns a plan with `cold`, `deferrals`, `scope`, `forced`, `invocation1` and `invocation2`;
  - `classifyDeferrals`, `pendingCause`, `dedupeDeferrals`, `sortDeferrals`, `deferralKey`;
  - `resolveViews`, `viewSummaries`;
  - `buildAttestation`, `commitState`, `installAdopted`, `markChangedDuringRun`;
  - `invoke`, `parseReport`, `NO_TESTS`;
  - `runVerify`;
  - `createInterrupt`, `installSignalHandlers`, `KILL_GRACE_MS`;
  - `canonicalPendingFull`.
- Produces:
  - `runCommand(io, options): Promise<0|1>`, which throws the known errors (2, 3, 4, 130) after writing outputs;
  - `causesJSON(counted): string`;
  - `MAX_CAUSES` (50).

**Behaviour (spec §5.6 and §11.2), in order:**

1. **Options.** `--mode` is required. `--pr` is refused with `--mode full` and under `pendingOnPr: "allow"`. `--max-minutes` needs `--pr`.
2. **Lock**, then reset `work/`.
3. **Adopt and plan.** Snapshot, graph, adoption, then `resolveViews`, which checks view completeness before any invocation.
4. **Execute.**
   - **Full mode:** one invocation with no input, no `--mutate` and no timeout. A no-tests result is exit 4.
   - **Incremental mode:**
     - copy a non-primary adopted state into `stateDir`;
     - plan (`unbudgeted` for `--pr`);
     - classify at plan time; under `--pr`, a counted `global` cause is the shortcut;
     - otherwise run invocation 1 (scope, no `--force`), then invocation 2 (edited + forced, `--force`), unless invocation 1 timed out, in which case invocation 2's files get `blocked by deferred scope`;
     - judge each invocation: success is the latest output; no-tests defers selected files with kills; a timeout defers the selection's files; an interrupt is 130; a failure is 4.
5. **Commit.** A cold run with no `incremental.json` commits nothing.
6. **Threshold and verifier** (exit 1).
7. **Outputs.** `GITHUB_OUTPUT` and the summary line. On every error path, `pending_full` and `exit_code` are written before rethrowing.
8. **Cleanup.** `work/` is removed and the lock released only if this run holds the lock.

- [ ] **Step 1: Write the failing tests**

`templates/mutation-incremental/test/run.test.mjs`:

```js
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { hostname } from 'node:os';
import { join } from 'node:path';
import { main } from '../lib/main.mjs';
import { causesJSON, MAX_CAUSES } from '../lib/run.mjs';
import { readCandidate, STATE_VERSION } from '../lib/state.mjs';
import { sha256 } from '../lib/snapshot.mjs';
import { writeState } from './helpers.mjs';
import { A, cli, reportFor, runRepo, warm } from './fake.mjs';

const A3 = A.replace('n > hi', 'n >= hi');
const ROUTED = { pendingOnPr: 'full-on-global' };
const usable = (r) => readCandidate(join(r.top, '.mutation'), { label: '.mutation', primary: true }).usable;
const attestationText = (r) => readFileSync(join(r.top, '.mutation/attestation.json'), 'utf8');
const inv = (k, ...rest) => ['run', 'stryker.config.json', '--incremental', '--incrementalFile', `.mutation/work/inv-${k}.json`, ...rest];

// Main's published state: this repo's own state with the given deferrals.
function mainState(r, deferrals) {
  const { reportSha256, ...rest } = r.read('.mutation/attestation.json');
  writeState(join(r.top, '.mutation/remote/inc'), { report: reportFor(), attestation: { ...rest, deferrals } });
}

test('run --mode full starts fresh, attests the report and clears pending', async () => {
  const r = runRepo();
  const res = await cli(r, ['run', '--mode', 'full']);
  assert.equal(res.code, 0);
  assert.deepEqual(r.calls(), [inv(1)]);
  const att = r.read('.mutation/attestation.json');
  assert.deepEqual([att.mode, att.lastFullAt, att.deferrals, att.score], ['full', att.completedAt, [], 50]);
  assert.equal(usable(r), true);
  assert.equal(existsSync(join(r.top, '.mutation/work')), false);
  assert.equal(existsSync(join(r.top, '.mutation/lock')), false);
  assert.deepEqual(res.output, { pending_full: 'false', exit_code: '0', pending_cause: 'none', pending_causes: '[]' });
  assert.match(res.stderr, /command=run invocations=1 scope=0 forced=0 deferrals=0 pending_full=false\n$/);
});

test('a full run that finds no tests is an engine failure and commits nothing', async () => {
  const r = runRepo({ steps: [{ exit: 1, output: 'INFO No tests were executed. Stryker will exit prematurely.\n' }] });
  const res = await cli(r, ['run', '--mode', 'full']);
  assert.equal(res.code, 4);
  assert.match(res.stderr, /the full run found no tests/);
  assert.equal(existsSync(join(r.top, '.mutation/attestation.json')), false);
  assert.deepEqual(res.output, { pending_full: 'true', exit_code: '4' });
  r.steps([{ exit: 2 }]);
  const failed = await cli(r, ['run', '--mode', 'full']);
  assert.equal(failed.code, 4);
  assert.match(failed.stderr, /the full run failed \(exit 2\)/);
});

test('an edit runs the planned invocations and carries lastFullAt', async () => {
  const r = runRepo();
  await warm(r);
  const full = r.read('.mutation/attestation.json');
  r.write('src/a.ts', A3);
  const plan = JSON.parse((await cli(r, ['plan'])).stdout);
  assert.deepEqual([plan.scope, plan.forced], [['src/a.ts'], ['src/a.ts:2-3']]);
  const res = await cli(r, ['run', '--mode', 'incremental']);
  assert.equal(res.code, 0);
  assert.deepEqual(r.calls(), [inv(1, '--mutate', 'src/a.ts'), inv(2, '--force', '--mutate', 'src/a.ts:2-3')]);
  const att = r.read('.mutation/attestation.json');
  assert.deepEqual([att.mode, att.lastFullAt, att.deferrals], ['incremental', full.completedAt, []]);
  assert.equal(att.files['src/a.ts'].digest, `sha256:${sha256(A3)}`);
  assert.match(res.stderr, /command=run invocations=2 scope=1 forced=1 deferrals=0 pending_full=false\n$/);
});

test('no reachable tests defers only files with kills and is not a failure', async () => {
  const r = runRepo();
  await warm(r);
  r.write('src/a.ts', A3);
  r.write('src/n.ts', 'export const n = 1;\n'); // a new module before its first test: no kills
  r.steps([{ exit: 1, output: 'No tests were executed' }]);
  const res = await cli(r, ['run', '--mode', 'incremental']);
  assert.equal(res.code, 0);
  assert.equal(r.calls().length, 2);
  assert.deepEqual(r.read('.mutation/attestation.json').deferrals, [{ reason: 'no reachable tests: src/a.ts', paths: ['src/a.ts'], inherited: false }]);
  assert.equal(res.output.pending_cause, 'unreachable');
  assert.deepEqual(JSON.parse(res.output.pending_causes), [{ reason: 'no reachable tests: src/a.ts', paths: ['src/a.ts'] }]);
});

test('a timed-out invocation 1 defers its files and blocks invocation 2', async () => {
  const r = runRepo({ config: { budget: { maxForcedShare: 1, maxForcedMutants: null, maxMinutesPerInvocation: 0.005 } } });
  await warm(r); // full mode has no time limit
  r.write('src/a.ts', A3);
  r.steps([{ sleepMs: 10000 }]);
  const res = await cli(r, ['run', '--mode', 'incremental']);
  assert.equal(res.code, 0);
  assert.equal(r.calls().length, 1);
  assert.deepEqual(r.read('.mutation/attestation.json').deferrals, [
    { reason: 'blocked by deferred scope', paths: ['src/a.ts'], inherited: false },
    { reason: 'time budget exceeded', paths: ['src/a.ts'], inherited: false },
  ]);
  assert.equal(res.output.pending_cause, 'timeout');
});

test('a cold incremental run with no report runs nothing and writes nothing', async () => {
  const r = runRepo();
  const res = await cli(r, ['run', '--mode', 'incremental']);
  assert.equal(res.code, 0);
  assert.deepEqual(r.calls(), []);
  assert.equal(existsSync(join(r.top, '.mutation/attestation.json')), false);
  assert.deepEqual(res.output, { pending_full: 'true', exit_code: '0', pending_cause: 'global', pending_causes: '[{"reason":"no usable state","paths":["*"]}]' });
});

test('a cold run re-attests an existing report with lastFullAt null and no usable state', async () => {
  const r = runRepo();
  await warm(r);
  writeFileSync(join(r.top, '.mutation/attestation.json'), JSON.stringify({ ...r.read('.mutation/attestation.json'), stateVersion: STATE_VERSION + 1 }));
  const res = await cli(r, ['run', '--mode', 'incremental']);
  const att = r.read('.mutation/attestation.json');
  assert.deepEqual([res.code, att.lastFullAt, att.stateVersion, r.calls()], [0, null, STATE_VERSION, []]);
  assert.deepEqual(att.deferrals, [{ reason: 'no usable state', paths: ['*'], inherited: false }]);
});

test('a cold run over an unparseable report commits nothing', async () => {
  const r = runRepo();
  mkdirSync(join(r.top, '.mutation'), { recursive: true });
  writeFileSync(join(r.top, '.mutation/incremental.json'), 'garbage');
  const res = await cli(r, ['run', '--mode', 'incremental']);
  assert.deepEqual([res.code, res.output.pending_full], [0, 'true']);
  assert.equal(existsSync(join(r.top, '.mutation/attestation.json')), false);
});

test("main's pending deferral about unchanged inputs is inherited; the PR's own edits still run", async () => {
  const r = runRepo();
  await warm(r);
  mainState(r, [{ reason: 'global input changed: package.json', paths: ['*'] }]);
  rmSync(join(r.top, '.mutation/attestation.json')); // no state of its own: the run adopts main's
  r.write('src/a.ts', A3);
  const res = await cli(r, ['run', '--mode', 'incremental', '--also-state', '.mutation/remote/inc']);
  assert.equal(res.code, 0);
  assert.equal(r.calls().length, 2);
  assert.deepEqual(r.read('.mutation/attestation.json').deferrals, [{ reason: 'global input changed: package.json', paths: ['*'], inherited: true }]);
  assert.deepEqual([res.output.pending_full, res.output.pending_cause, res.output.pending_causes], ['true', 'none', '[]']);
  assert.equal(usable(r), true);
});

test('a PR run with a counted global deferral takes the shortcut', async () => {
  const r = runRepo({ config: ROUTED, files: { 'package.json': '{}\n' } });
  await warm(r);
  r.write('package.json', '{"x":1}\n');
  const res = await cli(r, ['run', '--mode', 'incremental', '--pr', '--max-minutes', '5']);
  assert.equal(res.code, 0);
  assert.deepEqual(r.calls(), []);
  assert.equal(res.output.pending_cause, 'global');
  assert.deepEqual(r.read('.mutation/attestation.json').deferrals, [{ reason: 'global input changed: package.json', paths: ['*'], inherited: false }]);
});

test("a PR's own timeout is counted even when main carries the identical deferral", async () => {
  const r = runRepo({ config: ROUTED });
  await warm(r);
  mainState(r, [{ reason: 'time budget exceeded', paths: ['src/a.ts'] }]);
  r.write('src/a.ts', A3);
  r.steps([{ sleepMs: 10000 }]);
  const res = await cli(r, ['run', '--mode', 'incremental', '--pr', '--max-minutes', '0.005', '--also-state', '.mutation/remote/inc']);
  assert.equal(res.code, 0);
  const own = r.read('.mutation/attestation.json').deferrals.find((d) => d.reason === 'time budget exceeded');
  assert.deepEqual(own, { reason: 'time budget exceeded', paths: ['src/a.ts'], inherited: false });
  assert.equal(res.output.pending_cause, 'timeout');
});

test('a routed PR run is unbudgeted; other runs defer a forced set over budget', async () => {
  const budget = { maxForcedShare: 0.25, maxForcedMutants: null, maxMinutesPerInvocation: 5 };
  const r = runRepo({
    config: { ...ROUTED, budget },
    files: { 'tests/helpers/h.ts': 'export const h = 1;\n', 'tests/a.test.ts': "import { clamp } from '../src/a';\nimport { h } from './helpers/h';\n" },
  });
  await warm(r);
  r.write('tests/helpers/h.ts', 'export const h = 2;\n');
  const plain = JSON.parse((await cli(r, ['plan'])).stdout);
  assert.deepEqual(plain.deferrals.map((d) => d.reason), ['forced set 2 exceeds budget 0']);
  const res = await cli(r, ['run', '--mode', 'incremental', '--pr']);
  assert.equal(res.code, 0);
  assert.deepEqual(r.calls(), [inv(2, '--force', '--mutate', 'src/a.ts:2-3')]);
  assert.deepEqual(r.read('.mutation/attestation.json').deferrals, []);
});

test('a score below thresholds.break is exit 1 with the state committed', async () => {
  const r = runRepo({ stryker: { thresholds: { break: 60 } } });
  const res = await cli(r, ['run', '--mode', 'full']);
  assert.equal(res.code, 1);
  assert.match(res.stderr, /score 50 is below thresholds.break 60/);
  assert.equal(r.read('.mutation/attestation.json').thresholdBreak, true);
  assert.deepEqual([res.output.exit_code, res.output.pending_cause], ['1', 'none']);
});

const verifier = (script) => ({ verify: { command: [process.execPath, '-e', script], timeoutMinutes: null } });

test('a failing verifier is exit 1 naming the view; it does not run when nothing was committed', async () => {
  const r = runRepo({ config: { views: { inline: { core: ['src/**'], hot: ['src/a.ts'] } }, ...verifier("process.exit(process.env.MUTATION_VIEW === 'hot' ? 3 : 0)") } });
  const cold = await cli(r, ['run', '--mode', 'incremental']);
  assert.equal(cold.code, 0);
  const res = await cli(r, ['run', '--mode', 'full']);
  assert.equal(res.code, 1);
  assert.match(res.stderr, /mutation-incremental: verify \(view hot\): exit 3/);
  assert.doesNotMatch(res.stderr, /view core/);
  const att = r.read('.mutation/attestation.json');
  assert.deepEqual(att.views, { core: ['src/**'], hot: ['src/a.ts'] });
  assert.deepEqual(att.viewSummaries.hot, { Killed: 1, Survived: 1, pendingCounted: 0, pendingInherited: 0 });
});

test('an interrupt stops Stryker, commits nothing and exits 130', async () => {
  const r = runRepo();
  await warm(r);
  const before = attestationText(r);
  r.write('src/a.ts', A3);
  r.steps([{ sleepMs: 10000 }]);
  const timer = setTimeout(() => process.emit('SIGHUP'), 600);
  const res = await cli(r, ['run', '--mode', 'incremental']);
  clearTimeout(timer);
  assert.equal(res.code, 130);
  assert.equal(attestationText(r), before);
  assert.equal(existsSync(join(r.top, '.mutation/work')), false);
  assert.equal(existsSync(join(r.top, '.mutation/lock')), false);
  assert.deepEqual(res.output, { pending_full: 'false', exit_code: '130' });
  // A full run is interrupted the same way.
  r.steps([{ sleepMs: 10000 }]);
  const again = setTimeout(() => process.emit('SIGHUP'), 600);
  const full = await cli(r, ['run', '--mode', 'full']);
  clearTimeout(again);
  assert.equal(full.code, 130);
  assert.equal(attestationText(r), before);
});

test('an interrupt during the verifier exits 130 with the state committed', async () => {
  const r = runRepo({ config: verifier('setTimeout(() => {}, 20000)') });
  const timer = setTimeout(() => process.emit('SIGHUP'), 1000);
  const res = await cli(r, ['run', '--mode', 'full']);
  clearTimeout(timer);
  assert.equal(res.code, 130);
  assert.equal(usable(r), true);
});

test('an invocation failure is exit 4 and keeps the previous state', async () => {
  const r = runRepo();
  await warm(r);
  const before = attestationText(r);
  r.write('src/a.ts', A3);
  r.steps([{ exit: 2 }]);
  const res = await cli(r, ['run', '--mode', 'incremental']);
  assert.equal(res.code, 4);
  assert.match(res.stderr, /stryker invocation 1 failed \(exit 2\); nothing committed/);
  assert.equal(attestationText(r), before);
  assert.equal(existsSync(join(r.top, '.mutation/work')), false);
  assert.deepEqual(res.output, { pending_full: 'false', exit_code: '4' });
});

test('a held lock is exit 3 and leaves the other run alone', async () => {
  const r = runRepo();
  mkdirSync(join(r.top, '.mutation/work'), { recursive: true });
  writeFileSync(join(r.top, '.mutation/lock'), JSON.stringify({ pid: process.pid, hostname: hostname(), startedAt: 'x' }));
  const res = await cli(r, ['run', '--mode', 'full']);
  assert.equal(res.code, 3);
  assert.equal(existsSync(join(r.top, '.mutation/work')), true);
  assert.equal(existsSync(join(r.top, '.mutation/lock')), true);
  assert.deepEqual(res.output, { pending_full: 'true', exit_code: '3' });
});

test('run option errors are exit 2 and still write outputs', async () => {
  const r = runRepo();
  for (const args of [['run'], ['run', '--mode', 'full', '--pr'], ['run', '--mode', 'incremental', '--max-minutes', '5'], ['run', '--mode', 'incremental', '--pr', '--max-minutes', 'x']]) {
    const res = await cli(r, args);
    assert.equal(res.code, 2, args.join(' '));
    assert.deepEqual(res.output, { pending_full: 'false', exit_code: '2' }, args.join(' '));
  }
  const allow = await cli(r, ['run', '--mode', 'incremental', '--pr']);
  assert.deepEqual([allow.code, allow.output.pending_full], [2, 'true']);
  assert.match(allow.stderr, /--pr is for pendingOnPr "full" or "full-on-global"/);
});

test('a file edited while Stryker runs is attested as changed-during-run', async () => {
  const r = runRepo({ files: { 'src/b.ts': 'export const b = 1;\n' }, steps: [{ report: reportFor(), touch: { 'src/b.ts': 'export const b = 2;\n' } }] });
  assert.equal((await cli(r, ['run', '--mode', 'full'])).code, 0);
  assert.equal(r.read('.mutation/attestation.json').files['src/b.ts'].digest, 'changed-during-run');
});

test('an unexpected error is exit 4 in the outputs too', async () => {
  const r = runRepo();
  writeFileSync(join(r.top, '.mutation'), 'not a directory');
  const res = await cli(r, ['run', '--mode', 'full']);
  assert.deepEqual([res.code, res.output], [4, { pending_full: 'true', exit_code: '4' }]);
});

test('without GITHUB_OUTPUT nothing is appended', async () => {
  const r = runRepo();
  const saved = process.env.GITHUB_OUTPUT;
  delete process.env.GITHUB_OUTPUT;
  try {
    const quiet = { write() {} };
    // Also the default 30 s kill grace (nothing is killed here).
    assert.equal(await main(['run', '--mode', 'full'], { stdout: quiet, stderr: quiet, cwd: r.top }), 0);
  } finally {
    if (saved !== undefined) process.env.GITHUB_OUTPUT = saved;
  }
});

test('pending_causes is single-line JSON, capped with a count of the rest', () => {
  const many = Array.from({ length: MAX_CAUSES + 2 }, (_, i) => ({ reason: `no reachable tests: f${i}`, paths: [`f${i}`], inherited: false }));
  const parsed = JSON.parse(causesJSON(many));
  assert.equal(parsed.length, MAX_CAUSES + 1);
  assert.deepEqual(parsed.at(-1), { more: 2 });
  const odd = causesJSON([{ reason: 'no reachable tests: a\u0007b', paths: ['a\u0007b'], inherited: false }]);
  assert.doesNotMatch(odd, /[\u0000-\u001f]/);
  assert.deepEqual(JSON.parse(odd), [{ reason: 'no reachable tests: a\u0007b', paths: ['a\u0007b'] }]);
});
```

- [ ] **Step 2: Run to verify failure**

Run: `node --test templates/mutation-incremental/test/run.test.mjs`
Expected: FAIL — `lib/run.mjs` does not exist.

- [ ] **Step 3: Implement**

`templates/mutation-incremental/lib/run.mjs`:

```js
import { appendFileSync, existsSync, mkdirSync, readFileSync, rmSync } from 'node:fs';
import { join } from 'node:path';
import { EngineError, InterruptedError, UsageError } from './errors.mjs';
import { loadHarnessConfig, planInputs, summaryLine } from './inputs.mjs';
import { computePlan } from './plan.mjs';
import { classifyDeferrals, dedupeDeferrals, deferralKey, pendingCause, sortDeferrals } from './deferrals.mjs';
import { resolveViews, viewSummaries } from './views.mjs';
import { acquireLock } from './lock.mjs';
import { buildAttestation, commitState, installAdopted, markChangedDuringRun, WORK_DIR } from './attest.mjs';
import { invoke, NO_TESTS, parseReport } from './engine.mjs';
import { runVerify } from './verify.mjs';
import { createInterrupt, installSignalHandlers, KILL_GRACE_MS } from './proc.mjs';
import { canonicalPendingFull, REPORT_FILE } from './state.mjs';

export const MAX_CAUSES = 50;

// Forced entries are `<file>` or `<file>:<start>-<end>`; paths never contain ':' (the planner exits 2).
const fileOf = (entry) => entry.split(':')[0];

function checkOptions(options) {
  if (options.mode !== 'incremental' && options.mode !== 'full') throw new UsageError('run needs --mode incremental|full');
  if (options.pr && options.mode === 'full') throw new UsageError('--pr marks an incremental PR run; the sweep (--mode full) is never --pr');
  if (options.maxMinutes === undefined) return null;
  if (!options.pr) throw new UsageError('--max-minutes applies only to --pr runs');
  const minutes = Number(options.maxMinutes);
  if (!(minutes > 0)) throw new UsageError('--max-minutes must be a positive number');
  return minutes;
}

// Spec §5.1: appended on every exit path when GITHUB_OUTPUT is set.
function writeOutputs(entries) {
  const file = process.env.GITHUB_OUTPUT;
  if (!file) return;
  appendFileSync(file, entries.map(([key, value]) => `${key}=${value}\n`).join(''));
}

// Spec §11.2: the counted {reason, paths} as single-line JSON, at most MAX_CAUSES entries followed
// by {"more": n}. JSON.stringify escapes control characters; paths never contain a newline.
export function causesJSON(counted) {
  const list = counted.map((d) => ({ reason: d.reason, paths: d.paths }));
  const shown = list.length > MAX_CAUSES ? [...list.slice(0, MAX_CAUSES), { more: list.length - MAX_CAUSES }] : list;
  return JSON.stringify(shown);
}

function readOutput(path) {
  if (!existsSync(path)) return null;
  const bytes = readFileSync(path);
  const report = parseReport(bytes);
  return report === null ? null : { path, bytes, report };
}

// Spec §5.6 step 5: selected files with kills lose their killing tests; files without kills (type-only
// files, a new module before its first test) record nothing.
function noReachableDeferrals(files, report) {
  return files
    .filter((f) => (report.files[f]?.mutants ?? []).some((m) => m.status === 'Killed'))
    .map((f) => ({ reason: `no reachable tests: ${f}`, paths: [f] }));
}

async function executeIncremental(ctx) {
  const { config, inputs, options, interrupt } = ctx;
  const { snapshot, graph, candidates, chosen } = inputs;
  // Spec §11.2 (i): main's fetched states count whether or not they are usable.
  const alsoAttestations = candidates.filter((c) => !c.primary && c.attestation !== null).map((c) => c.attestation);
  if (chosen !== null && !chosen.primary) installAdopted(config.stateDir, chosen.dir);
  const plan = computePlan({ config, snapshot, graph, chosen, unbudgeted: options.pr });
  // Counted/inherited at plan time decides the shortcut (spec §11.2; review advisory).
  const planned = classifyDeferrals({ deferrals: plan.deferrals, alsoAttestations, snapshot, cold: plan.cold });
  const shortcut = options.pr && pendingCause(planned).cause === 'global';
  let latest = readOutput(join(config.stateDir, REPORT_FILE));
  const produced = [];
  let invocations = 0;
  if (!plan.cold && !shortcut) {
    const timeoutMs = (ctx.maxMinutes ?? config.budget.maxMinutesPerInvocation) * 60_000;
    let blocked = false;
    const steps = [{ list: plan.invocation1, force: false }, { list: plan.invocation2, force: true }];
    for (const [i, { list, force }] of steps.entries()) {
      if (list.length === 0) continue;
      const files = [...new Set(list.map(fileOf))].sort();
      if (blocked) {
        produced.push({ reason: 'blocked by deferred scope', paths: files });
        continue;
      }
      const r = await invoke(config, { index: i + 1, input: latest.path, force, mutate: list, timeoutMs, interrupt, graceMs: ctx.graceMs, onOutput: ctx.onOutput });
      invocations++;
      if (r.outcome === 'interrupted') throw new InterruptedError('interrupted; nothing committed');
      if (r.outcome === 'failed') throw new EngineError(`stryker invocation ${i + 1} failed (${r.detail}); nothing committed`);
      if (r.outcome === 'success') latest = r;
      else if (r.outcome === 'timeout') {
        produced.push({ reason: 'time budget exceeded', paths: files });
        blocked = true;
      } else produced.push(...noReachableDeferrals(files, latest.report));
    }
  }
  const deferrals = sortDeferrals(dedupeDeferrals(plan.deferrals, produced));
  const producedKeys = new Set(produced.map(deferralKey));
  return {
    plan,
    latest,
    invocations,
    mode: 'incremental',
    lastFullAt: chosen === null ? null : chosen.attestation.lastFullAt,
    classified: classifyDeferrals({ deferrals, alsoAttestations, snapshot, producedKeys, cold: plan.cold }),
  };
}

async function executeFull(ctx) {
  const r = await invoke(ctx.config, { index: 1, input: null, force: false, mutate: null, timeoutMs: null, interrupt: ctx.interrupt, graceMs: ctx.graceMs, onOutput: ctx.onOutput });
  if (r.outcome === 'interrupted') throw new InterruptedError('interrupted; nothing committed');
  // A full run that finds no tests (e.g. a dependency bump broke every import) is never attested.
  if (r.outcome === 'no-tests') throw new EngineError(`the full run found no tests (${NO_TESTS}); nothing committed`);
  if (r.outcome === 'failed') throw new EngineError(`the full run failed (${r.detail}); nothing committed`);
  return { plan: null, latest: r, invocations: 1, mode: 'full', lastFullAt: null, classified: [] };
}

// Spec §5.6 steps 6–7. A cold run with no incremental.json commits nothing.
function commit(ctx, result) {
  const { config, inputs, views } = ctx;
  if (result.latest === null) return null;
  const completedAt = new Date().toISOString();
  const attestation = buildAttestation({
    config,
    files: markChangedDuringRun(config, inputs.snapshot.files),
    runtime: inputs.snapshot.runtime,
    reportBytes: result.latest.bytes,
    report: result.latest.report,
    mode: result.mode,
    completedAt,
    lastFullAt: result.mode === 'full' ? completedAt : result.lastFullAt,
    deferrals: result.classified,
    views,
    summaries: viewSummaries(result.latest.report, views, result.classified),
  });
  commitState(config.stateDir, result.latest.path, attestation);
  return attestation;
}

// Spec §5.1 `run`, §5.6, §11.1, §11.2.
export async function runCommand(io, options) {
  const interrupt = createInterrupt();
  const uninstall = installSignalHandlers(interrupt);
  let config = null;
  let release = null;
  try {
    const maxMinutes = checkOptions(options);
    ({ config } = loadHarnessConfig(io, options, true));
    if (options.pr && config.pendingOnPr === 'allow') throw new UsageError('--pr is for pendingOnPr "full" or "full-on-global"; under "allow" the workflow never passes it');
    mkdirSync(config.stateDir, { recursive: true });
    release = acquireLock(config.stateDir);
    const work = join(config.stateDir, WORK_DIR);
    rmSync(work, { recursive: true, force: true });
    mkdirSync(work);
    const inputs = planInputs(config.top, config, options.alsoState);
    // Spec §11.3: view completeness is checked at plan time, before any invocation.
    const views = resolveViews(config, inputs.snapshot);
    const ctx = { config, inputs, views, options, interrupt, maxMinutes, graceMs: io.killGraceMs ?? KILL_GRACE_MS, onOutput: (chunk) => io.stderr.write(chunk) };
    const result = options.mode === 'full' ? await executeFull(ctx) : await executeIncremental(ctx);
    const attestation = commit(ctx, result);
    const failures = [];
    if (attestation !== null) {
      if (attestation.thresholdBreak) failures.push(`score ${attestation.score} is below thresholds.break ${config.thresholdBreak}`);
      failures.push(...(await runVerify(config, { views, snapshot: inputs.snapshot, pr: options.pr, interrupt, graceMs: ctx.graceMs, onOutput: ctx.onOutput })));
    }
    if (interrupt.interrupted) throw new InterruptedError('interrupted; the state is committed');
    for (const f of failures) io.stderr.write(`mutation-incremental: ${f}\n`);
    const code = failures.length > 0 ? 1 : 0;
    const pendingFull = attestation === null || result.classified.length > 0;
    const { cause, counted } = pendingCause(result.classified);
    writeOutputs([['pending_full', pendingFull], ['exit_code', code], ['pending_cause', cause], ['pending_causes', causesJSON(counted)]]);
    io.stderr.write(summaryLine({
      command: 'run',
      invocations: result.invocations,
      scope: result.plan?.scope.length ?? 0,
      forced: result.plan?.forced.length ?? 0,
      deferrals: result.classified.length,
      pendingFull,
    }));
    return code;
  } catch (e) {
    writeOutputs([['pending_full', config === null ? false : canonicalPendingFull(config)], ['exit_code', Number.isInteger(e.exitCode) ? e.exitCode : 4]]);
    throw e;
  } finally {
    // Only the lock holder may touch work/: on exit 3 it belongs to the other run.
    if (release !== null) {
      rmSync(join(config.stateDir, WORK_DIR), { recursive: true, force: true });
      release();
    }
    uninstall();
  }
}
```

Edit `templates/mutation-incremental/lib/main.mjs` — add the import and the table entry:

```js
import { runCommand } from './run.mjs';
```

```js
const COMMANDS = { plan: planCommand, run: runCommand, 'break-lock': breakLockCommand };
```

- [ ] **Step 4: Run the suite with coverage**

Run: `sh .superpowers/sdd/2026-09-23-mutation-incremental-1b-execution/cov.sh`
Expected: all pass; no uncovered entries.

- [ ] **Step 5: Commit**

```bash
git add templates/mutation-incremental/lib/run.mjs templates/mutation-incremental/lib/main.mjs templates/mutation-incremental/test/run.test.mjs
git commit -m "feat(mutation-incremental): run command with commit, verify, pending cause and outputs

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 7: `seed`

**Files:**
- Create: `templates/mutation-incremental/lib/seed.mjs`, `test/seed.test.mjs`
- Modify: `lib/main.mjs` (register `seed`)

**Interfaces:**
- Consumes:
  - `loadHarnessConfig`, `summaryLine`, `acquireLock`, `takeSnapshot`;
  - `resolveViews`, `viewSummaries`;
  - `dedupeDeferrals`, `sortDeferrals`;
  - `buildAttestation`, `commitState`, `WORK_DIR`;
  - `parseReport`, `primaryCandidate`.
- Produces:
  - `mergeReports(reports): report`;
  - `seedCommand(io, options): Promise<0>` (exit 2 on refusal or bad input; never 1).

- [ ] **Step 1: Write the failing tests**

`templates/mutation-incremental/test/seed.test.mjs`:

```js
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { mergeReports } from '../lib/seed.mjs';
import { readCandidate } from '../lib/state.mjs';
import { A, cli, reportFor, runRepo } from './fake.mjs';

const at = (line) => ({ start: { line, column: 1 }, end: { line, column: 5 } });
const mu = (id, mutatorName, replacement, status, line, killedBy, coveredBy) => ({ id, mutatorName, replacement, status, killedBy, coveredBy, location: at(line) });
// X and Y share the line-2 and line-3 mutants; line 1 holds three mutants at one position (two
// mutators, two replacements), as Stryker produces for one expression.
const X = {
  schemaVersion: '1',
  files: { 'src/a.ts': { language: 'typescript', source: A, mutants: [
    mu('9', 'M', 'r', 'Survived', 2, undefined, ['x1']),
    mu('8', 'M', 'r', 'CompileError', 3, undefined, undefined),
  ] } },
  testFiles: { 'tests/a.test.ts': { tests: [{ id: 'x1', name: 'low' }] } },
};
const Y = {
  schemaVersion: '1',
  files: { 'src/a.ts': { language: 'typescript', source: A, mutants: [
    mu('3', 'M', 'r', 'Killed', 2, ['y2'], ['y2', 'y1']),
    mu('4', 'N', 's', 'NoCoverage', 1, undefined, []),
    mu('5', 'N', 'r', 'NoCoverage', 1, undefined, []),
    mu('6', 'M', 'r', 'NoCoverage', 1, undefined, []),
    mu('7', 'M', 'r', 'Survived', 3, undefined, ['y1']),
  ] } },
  testFiles: { 'tests/a.test.ts': { tests: [{ id: 'y1', name: 'high' }, { id: 'y2', name: 'low' }] } },
};

test('mergeReports re-keys tests by (file, name), merges mutants and renumbers both', () => {
  for (const order of [[X, Y], [Y, X]]) {
    const merged = mergeReports(order);
    assert.deepEqual(merged.testFiles, { 'tests/a.test.ts': { tests: [{ id: '0', name: 'high' }, { id: '1', name: 'low' }] } });
    const mutants = merged.files['src/a.ts'].mutants.map((m) => [m.id, m.location.start.line, m.mutatorName, m.replacement, m.status, m.killedBy, m.coveredBy]);
    assert.deepEqual(mutants, [
      ['0', 1, 'M', 'r', 'NoCoverage', [], []],
      ['1', 1, 'N', 'r', 'NoCoverage', [], []],
      ['2', 1, 'N', 's', 'NoCoverage', [], []],
      ['3', 2, 'M', 'r', 'Killed', ['1'], ['0', '1']],
      ['4', 3, 'M', 'r', 'Survived', [], ['0']],
    ]);
    assert.equal(merged.files['src/a.ts'].source, A);
  }
});

test('seed bootstraps a state whose kills are pending until a full run', async () => {
  const r = runRepo({ config: { views: { inline: { core: ['src/**'] } } } });
  writeFileSync(join(r.top, '.fake/full.json'), JSON.stringify(reportFor()));
  const res = await cli(r, ['seed', '--from', '.fake/full.json']);
  assert.equal(res.code, 0);
  assert.equal(res.stdout, 'seed: score=50 thresholdBreak=false\n');
  assert.match(res.stderr, /command=seed invocations=0 scope=0 forced=0 deferrals=1 pending_full=true\n$/);
  const att = r.read('.mutation/attestation.json');
  assert.deepEqual([att.mode, att.lastFullAt], ['seed', null]);
  assert.deepEqual(att.deferrals, [{ reason: 'seeded from .fake/full.json', paths: ['*'], inherited: false }]);
  assert.deepEqual(att.viewSummaries.core, { Killed: 1, Survived: 1, pendingCounted: 1, pendingInherited: 0 });
  assert.equal(readCandidate(join(r.top, '.mutation'), { label: 's', primary: true }).usable, true);
  assert.deepEqual(r.read('.mutation/incremental.json').testFiles['tests/a.test.ts'].tests, [{ id: '0', name: 'clamps low' }]);
});

test('seed refuses usable state unless --replace, which first keeps a copy', async () => {
  const r = runRepo();
  writeFileSync(join(r.top, '.fake/full.json'), JSON.stringify(reportFor()));
  const empty = await cli(r, ['seed', '--from', '.fake/full.json', '--replace']); // nothing to keep yet
  assert.equal(empty.code, 0);
  const again = await cli(r, ['seed', '--from', '.fake/full.json']);
  assert.equal(again.code, 2);
  assert.match(again.stderr, /usable state exists in \.mutation; pass --replace/);
  assert.equal((await cli(r, ['seed', '--from', '.fake/full.json', '--replace'])).code, 0);
  const kept = readdirSync(join(r.top, '.mutation/replaced')).sort();
  assert.equal(kept.length, 2);
  assert.deepEqual(readdirSync(join(r.top, '.mutation/replaced', kept[0])), []);
  assert.deepEqual(readdirSync(join(r.top, '.mutation/replaced', kept[1])).sort(), ['attestation.json', 'incremental.json']);
});

test('seed input errors are exit 2', async () => {
  const r = runRepo();
  writeFileSync(join(r.top, '.fake/bad.json'), '{"files":[]}');
  writeFileSync(join(r.top, '.fake/x.json'), JSON.stringify(reportFor()));
  writeFileSync(join(r.top, '.fake/y.json'), JSON.stringify(reportFor(A.replace('lo;', 'lo ;'))));
  const cases = [
    [['seed'], /seed needs --from/],
    [['seed', '--from', '.fake/missing.json'], /seed: cannot read \.fake\/missing\.json/],
    [['seed', '--from', '.fake/bad.json'], /\.fake\/bad\.json is not a mutation-testing report/],
    [['seed', '--from', '.fake/x.json', '--from', '.fake/y.json'], /src\/a\.ts has a different source in the given reports/],
  ];
  for (const [args, message] of cases) {
    const res = await cli(r, args);
    assert.equal(res.code, 2, args.join(' '));
    assert.match(res.stderr, message);
  }
});
```

Note on the `--replace` test: the two backup directories are named by UTC timestamp with millisecond precision. The first seed and the third run are separated by a second seed and two full CLI runs, so their names differ and sort in time order.

- [ ] **Step 2: Run to verify failure**

Run: `node --test templates/mutation-incremental/test/seed.test.mjs`
Expected: FAIL — `lib/seed.mjs` does not exist.

- [ ] **Step 3: Implement**

`templates/mutation-incremental/lib/seed.mjs`:

```js
import { copyFileSync, existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { UsageError } from './errors.mjs';
import { canonicalJSON } from './json.mjs';
import { loadHarnessConfig, summaryLine } from './inputs.mjs';
import { acquireLock } from './lock.mjs';
import { takeSnapshot } from './snapshot.mjs';
import { resolveViews, viewSummaries } from './views.mjs';
import { dedupeDeferrals, sortDeferrals } from './deferrals.mjs';
import { buildAttestation, commitState, WORK_DIR } from './attest.mjs';
import { parseReport } from './engine.mjs';
import { ATTESTATION_FILE, REPORT_FILE, primaryCandidate } from './state.mjs';

const cmp = (a, b) => (a < b ? -1 : a > b ? 1 : 0);
// Spec §11.6: a mutant in several reports takes the strongest status.
const RANK = { Killed: 4, Timeout: 3, Survived: 2, NoCoverage: 1 };
const rank = (status) => RANK[status] ?? 0;
const position = (m) => [m.location.start.line, m.location.start.column, m.location.end.line, m.location.end.column];
const byPosition = (a, b) => {
  const pa = position(a);
  const pb = position(b);
  return pa.reduce((acc, v, i) => acc || v - pb[i], 0) || cmp(a.mutatorName, b.mutatorName) || cmp(String(a.replacement), String(b.replacement));
};
const union = (a, b) => [...new Set([...a, ...b])].sort((x, y) => Number(x) - Number(y));

// Spec §11.6: tests are re-keyed by (file, name) and renumbered; mutants are matched by
// (file, mutatorName, location, replacement), merged (status by precedence, test lists unioned),
// and renumbered in byte order of file, then location. A path in several reports must have the
// same source.
export function mergeReports(reports) {
  const tests = new Map();
  const testFileEntries = new Map();
  const localKeys = reports.map((report) => {
    const local = new Map();
    for (const [file, entry] of Object.entries(report.testFiles)) {
      if (!testFileEntries.has(file)) testFileEntries.set(file, entry);
      for (const t of entry.tests) {
        const key = `${file}\u0000${t.name}`;
        local.set(String(t.id), key);
        if (!tests.has(key)) tests.set(key, { file, test: t });
      }
    }
    return local;
  });
  const keys = [...tests.keys()].sort(cmp);
  const newId = new Map(keys.map((k, i) => [k, String(i)]));
  const files = new Map();
  reports.forEach((report, i) => {
    const remap = (ids) => (ids ?? []).map((id) => newId.get(localKeys[i].get(String(id))));
    for (const [path, entry] of Object.entries(report.files)) {
      if (!files.has(path)) files.set(path, { entry, mutants: new Map() });
      const file = files.get(path);
      if (file.entry.source !== entry.source) throw new UsageError(`seed: ${path} has a different source in the given reports`);
      for (const m of entry.mutants) {
        const key = JSON.stringify([m.mutatorName, position(m), m.replacement]);
        const merged = { ...m, killedBy: remap(m.killedBy), coveredBy: remap(m.coveredBy) };
        const prev = file.mutants.get(key);
        if (prev === undefined) {
          file.mutants.set(key, merged);
          continue;
        }
        if (rank(m.status) > rank(prev.status)) prev.status = m.status;
        prev.killedBy = union(prev.killedBy, merged.killedBy);
        prev.coveredBy = union(prev.coveredBy, merged.coveredBy);
      }
    }
  });
  let next = 0;
  const outFiles = {};
  for (const path of [...files.keys()].sort(cmp)) {
    const { entry, mutants } = files.get(path);
    outFiles[path] = { ...entry, mutants: [...mutants.values()].sort(byPosition).map((m) => ({ ...m, id: String(next++) })) };
  }
  const outTests = {};
  for (const key of keys) {
    const { file, test } = tests.get(key);
    outTests[file] ??= { ...testFileEntries.get(file), tests: [] };
    outTests[file].tests.push({ ...test, id: newId.get(key) });
  }
  return { ...reports[0], files: outFiles, testFiles: outTests };
}

function readReport(top, path) {
  let bytes;
  try {
    bytes = readFileSync(resolve(top, path));
  } catch (e) {
    throw new UsageError(`seed: cannot read ${path}: ${e.message}`);
  }
  const report = parseReport(bytes);
  if (report === null) throw new UsageError(`seed: ${path} is not a mutation-testing report (no files object)`);
  return report;
}

// Spec §5.8: --replace first copies the current pair into replaced/<UTC timestamp>/.
function keepCurrent(stateDir) {
  const dir = join(stateDir, 'replaced', new Date().toISOString().replaceAll(':', '-'));
  mkdirSync(dir, { recursive: true });
  for (const name of [ATTESTATION_FILE, REPORT_FILE]) {
    if (existsSync(join(stateDir, name))) copyFileSync(join(stateDir, name), join(dir, name));
  }
}

// Spec §5.8, §11.6: bootstrap from full reports. The kills stay pending ("seeded from <report>" on
// ["*"]) until the first full run on main. Never exits 1: it prints the score instead.
export async function seedCommand(io, options) {
  if (options.from.length === 0) throw new UsageError('seed needs --from <report> (repeatable)');
  const { top, config } = loadHarnessConfig(io, options, false);
  mkdirSync(config.stateDir, { recursive: true });
  const release = acquireLock(config.stateDir);
  try {
    if (primaryCandidate(config).usable && !options.replace) throw new UsageError(`usable state exists in ${config.stateDirRaw}; pass --replace to seed over it`);
    const merged = mergeReports(options.from.map((p) => readReport(top, p)));
    const snapshot = takeSnapshot(config);
    const views = resolveViews(config, snapshot);
    if (options.replace) keepCurrent(config.stateDir);
    const work = join(config.stateDir, WORK_DIR);
    mkdirSync(work, { recursive: true });
    const output = join(work, REPORT_FILE);
    const bytes = Buffer.from(canonicalJSON(merged));
    writeFileSync(output, bytes);
    const deferrals = sortDeferrals(dedupeDeferrals([], options.from.map((p) => ({ reason: `seeded from ${p}`, paths: ['*'] })))).map((d) => ({ ...d, inherited: false }));
    const attestation = buildAttestation({
      config,
      files: snapshot.files,
      runtime: snapshot.runtime,
      reportBytes: bytes,
      report: merged,
      mode: 'seed',
      completedAt: new Date().toISOString(),
      lastFullAt: null,
      deferrals,
      views,
      summaries: viewSummaries(merged, views, deferrals),
    });
    commitState(config.stateDir, output, attestation);
    io.stdout.write(`seed: score=${attestation.score} thresholdBreak=${attestation.thresholdBreak}\n`);
    io.stderr.write(summaryLine({ command: 'seed', deferrals: deferrals.length, pendingFull: true }));
    return 0;
  } finally {
    release();
  }
}
```

Edit `templates/mutation-incremental/lib/main.mjs` — add the import and the table entry:

```js
import { seedCommand } from './seed.mjs';
```

```js
const COMMANDS = { plan: planCommand, run: runCommand, seed: seedCommand, 'break-lock': breakLockCommand };
```

- [ ] **Step 4: Run the suite with coverage**

Run: `sh .superpowers/sdd/2026-09-23-mutation-incremental-1b-execution/cov.sh`
Expected: all pass; no uncovered entries.

- [ ] **Step 5: Commit**

```bash
git add templates/mutation-incremental/lib/seed.mjs templates/mutation-incremental/lib/main.mjs templates/mutation-incremental/test/seed.test.mjs
git commit -m "feat(mutation-incremental): seed from one or more full reports

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 8: `fetch-state` and `publish-state`

**Files:**
- Create: `templates/mutation-incremental/lib/remote.mjs`, `test/remote.test.mjs`
- Modify: `lib/main.mjs` (register both commands)

**Interfaces:**
- Consumes: `loadHarnessConfig`, `summaryLine`, `primaryCandidate`, `canonicalPendingFull`, `ATTESTATION_FILE`, `REPORT_FILE`.
- Produces:
  - `KINDS` (`['inc', 'full']`);
  - `authEnv(env, url, token): {env, mask}`;
  - `fetchStateCommand(io, options)`;
  - `publishStateCommand(io, options)`.

- [ ] **Step 1: Write the failing tests**

`templates/mutation-incremental/test/remote.test.mjs`:

```js
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { chmodSync, existsSync, mkdirSync, mkdtempSync, readFileSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { authEnv } from '../lib/remote.mjs';
import { readCandidate } from '../lib/state.mjs';
import { cli, runRepo, warm } from './fake.mjs';

function bareRemote(r) {
  const bare = mkdtempSync(join(tmpdir(), 'mi-remote-'));
  execFileSync('git', ['init', '-q', '--bare', bare]);
  r.git('remote', 'add', 'origin', bare);
  return bare;
}
const usable = (dir) => readCandidate(dir, { label: 'x', primary: false }).usable;

test('authEnv passes the token as an extraheader after existing GIT_CONFIG entries', () => {
  assert.deepEqual(authEnv({ A: '1' }, 'https://h/r.git', ''), { env: { A: '1' }, mask: null });
  assert.deepEqual(authEnv({ A: '1' }, 'https://h/r.git', undefined), { env: { A: '1' }, mask: null });
  const b64 = Buffer.from('x-access-token:tok').toString('base64');
  assert.deepEqual(authEnv({}, 'https://h/r.git', 'tok'), {
    env: { GIT_CONFIG_COUNT: '1', GIT_CONFIG_KEY_0: 'http.https://h/r.git.extraheader', GIT_CONFIG_VALUE_0: `AUTHORIZATION: basic ${b64}` },
    mask: b64,
  });
  const more = authEnv({ GIT_CONFIG_COUNT: '2' }, 'u', 'tok').env;
  assert.deepEqual([more.GIT_CONFIG_COUNT, more.GIT_CONFIG_KEY_2], ['3', 'http.u.extraheader']);
});

test('publish-state pushes a parentless commit that fetch-state reads back in another checkout', async () => {
  const r = runRepo();
  await warm(r);
  const bare = bareRemote(r);
  const pub = await cli(r, ['publish-state', '--kind', 'full']);
  assert.equal(pub.code, 0);
  assert.match(pub.stdout, /^publish-state: pushed [0-9a-f]{40} to refs\/heads\/mutation-state\/full\n$/);
  assert.match(pub.stderr, /command=publish-state invocations=0 scope=0 forced=0 deferrals=0 pending_full=false\n$/);
  const head = execFileSync('git', ['log', '-1', '--format=%P|%an <%ae>|%cn', 'refs/heads/mutation-state/full'], { cwd: bare, encoding: 'utf8' }).trim();
  assert.equal(head, '|mutation-incremental <noreply@localhost>|mutation-incremental');

  const other = runRepo();
  other.git('remote', 'add', 'origin', bare);
  const got = await cli(other, ['fetch-state']);
  assert.equal(got.code, 0);
  assert.match(got.stderr, /refs\/heads\/mutation-state\/inc is not on origin; skipped/);
  assert.equal(readFileSync(join(other.top, '.mutation/remote/full/incremental.json'), 'utf8'), readFileSync(join(r.top, '.mutation/incremental.json'), 'utf8'));
  assert.equal(usable(join(other.top, '.mutation/remote/full')), true);
  // The other checkout adopts it; its files are identical, so there is nothing to run.
  const run = await cli(other, ['run', '--mode', 'incremental', '--also-state', '.mutation/remote/full']);
  assert.equal(run.code, 0);
  assert.deepEqual(other.calls(), []);
  assert.equal(usable(join(other.top, '.mutation')), true);
});

test('fetch-state removes a copy whose branch is gone', async () => {
  const r = runRepo();
  bareRemote(r);
  mkdirSync(join(r.top, '.mutation/remote/inc'), { recursive: true });
  writeFileSync(join(r.top, '.mutation/remote/inc/attestation.json'), '{}');
  const res = await cli(r, ['fetch-state']);
  assert.equal(res.code, 0);
  assert.equal(existsSync(join(r.top, '.mutation/remote/inc')), false);
  assert.match(res.stderr, /command=fetch-state invocations=0 scope=0 forced=0 deferrals=0 pending_full=true\n$/);
});

test('an unreachable or unknown remote is exit 2', async () => {
  const r = runRepo();
  r.git('remote', 'add', 'origin', join(tmpdir(), 'mi-no-such-remote.git'));
  const unreachable = await cli(r, ['fetch-state']);
  assert.equal(unreachable.code, 2);
  assert.match(unreachable.stderr, /fetch-state: git ls-remote origin failed/);
  const unknown = await cli(r, ['fetch-state', '--remote', 'nope']);
  assert.equal(unknown.code, 2);
  assert.match(unknown.stderr, /unknown git remote nope/);
});

test('a state branch without both files is exit 2', async () => {
  const r = runRepo();
  bareRemote(r);
  const git = (args, input) => execFileSync('git', args, { cwd: r.top, input, encoding: 'utf8' }).trim();
  const blob = git(['hash-object', '-w', '--stdin'], '{}');
  const tree = git(['mktree'], `100644 blob ${blob}\tattestation.json\n`);
  const commit = git(['commit-tree', tree, '-m', 'x']);
  r.git('push', '-q', 'origin', `${commit}:refs/heads/mutation-state/inc`);
  const res = await cli(r, ['fetch-state']);
  assert.equal(res.code, 2);
  assert.match(res.stderr, /fetch-state: reading incremental\.json from refs\/heads\/mutation-state\/inc failed/);
});

test('publish-state without usable state publishes nothing; a rejected push or a missing kind is exit 2', async () => {
  const r = runRepo();
  const bare = bareRemote(r);
  const none = await cli(r, ['publish-state', '--kind', 'inc']);
  assert.equal(none.code, 0);
  assert.match(none.stdout, /canonical state is not usable \(missing attestation\); nothing published/);
  await warm(r);
  writeFileSync(join(bare, 'hooks/pre-receive'), '#!/bin/sh\necho "refs are protected" >&2\nexit 1\n');
  chmodSync(join(bare, 'hooks/pre-receive'), 0o755);
  const rejected = await cli(r, ['publish-state', '--kind', 'inc']);
  assert.equal(rejected.code, 2);
  assert.match(rejected.stderr, /publish-state: push to origin failed: [\s\S]*refs are protected/);
  const noKind = await cli(r, ['publish-state']);
  assert.equal(noKind.code, 2);
  assert.match(noKind.stderr, /publish-state needs --kind inc\|full/);
});

test('with MUTATION_STATE_TOKEN under GitHub Actions the header value is masked first', async () => {
  const r = runRepo();
  bareRemote(r);
  const saved = { token: process.env.MUTATION_STATE_TOKEN, actions: process.env.GITHUB_ACTIONS };
  process.env.MUTATION_STATE_TOKEN = 'tok';
  process.env.GITHUB_ACTIONS = 'true';
  try {
    const res = await cli(r, ['fetch-state']);
    assert.equal(res.code, 0);
    assert.equal(res.stdout, `::add-mask::${Buffer.from('x-access-token:tok').toString('base64')}\n`);
  } finally {
    for (const [name, value] of [['MUTATION_STATE_TOKEN', saved.token], ['GITHUB_ACTIONS', saved.actions]]) {
      if (value === undefined) delete process.env[name];
      else process.env[name] = value;
    }
  }
});
```

- [ ] **Step 2: Run to verify failure**

Run: `node --test templates/mutation-incremental/test/remote.test.mjs`
Expected: FAIL — `lib/remote.mjs` does not exist.

- [ ] **Step 3: Implement**

`templates/mutation-incremental/lib/remote.mjs`:

```js
import { execFileSync, spawnSync } from 'node:child_process';
import { mkdirSync, renameSync, rmSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { UsageError } from './errors.mjs';
import { loadHarnessConfig, summaryLine } from './inputs.mjs';
import { ATTESTATION_FILE, REPORT_FILE, canonicalPendingFull, primaryCandidate } from './state.mjs';

export const KINDS = ['inc', 'full'];
const AUTHOR = { GIT_AUTHOR_NAME: 'mutation-incremental', GIT_AUTHOR_EMAIL: 'noreply@localhost', GIT_COMMITTER_NAME: 'mutation-incremental', GIT_COMMITTER_EMAIL: 'noreply@localhost' };
const branchRef = (kind) => `refs/heads/mutation-state/${kind}`;

// Spec §5.7: the token travels as an http extraheader through GIT_CONFIG_* (never argv), appended
// after entries already in the environment. An empty or unset token means git's own credentials.
export function authEnv(env, url, token) {
  if (!token) return { env, mask: null };
  const mask = Buffer.from(`x-access-token:${token}`).toString('base64');
  const n = Number(env.GIT_CONFIG_COUNT ?? 0);
  return {
    env: { ...env, GIT_CONFIG_COUNT: String(n + 1), [`GIT_CONFIG_KEY_${n}`]: `http.${url}.extraheader`, [`GIT_CONFIG_VALUE_${n}`]: `AUTHORIZATION: basic ${mask}` },
    mask,
  };
}

function remoteEnv(io, top, remote) {
  const url = spawnSync('git', ['remote', 'get-url', remote], { cwd: top, encoding: 'utf8' });
  if (url.status !== 0) throw new UsageError(`unknown git remote ${remote}`);
  const { env, mask } = authEnv(process.env, url.stdout.trim(), process.env.MUTATION_STATE_TOKEN);
  if (mask !== null && process.env.GITHUB_ACTIONS) io.stdout.write(`::add-mask::${mask}\n`);
  return env;
}

// A failed ls-remote, fetch, show or push is exit 2 (spec §5.7: network and auth failures are visible).
function must(result, what, command) {
  if (result.status !== 0) throw new UsageError(`${command}: ${what} failed: ${String(result.stderr).trim()}`);
  return result.stdout;
}

// Spec §5.7: list the state branches (a missing one is skipped), fetch each present one without
// --depth, and write its two files into <stateDir>/remote/<kind>/. A kind's old copy is removed
// first, so a branch that is gone leaves nothing to adopt.
export async function fetchStateCommand(io, options) {
  const { top, config } = loadHarnessConfig(io, options, false);
  const remote = options.remote ?? 'origin';
  const env = remoteEnv(io, top, remote);
  const listing = must(spawnSync('git', ['ls-remote', remote, 'refs/heads/mutation-state/*'], { cwd: top, env, encoding: 'utf8' }), `git ls-remote ${remote}`, 'fetch-state');
  const present = new Set(listing.split('\n').filter(Boolean).map((line) => line.split('\t')[1]));
  for (const kind of KINDS) {
    const dir = join(config.stateDir, 'remote', kind);
    rmSync(dir, { recursive: true, force: true });
    if (!present.has(branchRef(kind))) {
      io.stderr.write(`fetch-state: ${branchRef(kind)} is not on ${remote}; skipped\n`);
      continue;
    }
    must(spawnSync('git', ['fetch', '--no-tags', remote, branchRef(kind)], { cwd: top, env, encoding: 'utf8' }), `git fetch ${branchRef(kind)}`, 'fetch-state');
    mkdirSync(dir, { recursive: true });
    for (const name of [ATTESTATION_FILE, REPORT_FILE]) {
      const bytes = must(spawnSync('git', ['show', `FETCH_HEAD:${name}`], { cwd: top, env }), `reading ${name} from ${branchRef(kind)}`, 'fetch-state');
      writeFileSync(join(dir, `${name}.tmp`), bytes);
      renameSync(join(dir, `${name}.tmp`), join(dir, name));
    }
  }
  io.stderr.write(summaryLine({ command: 'fetch-state', pendingFull: canonicalPendingFull(config) }));
  return 0;
}

// Spec §5.7: a parentless commit holding the canonical pair, built with plumbing and force-pushed
// to mutation-state/<kind>. Only a usable state is published; otherwise it says why and exits 0.
export async function publishStateCommand(io, options) {
  if (!KINDS.includes(options.kind)) throw new UsageError('publish-state needs --kind inc|full');
  const { top, config } = loadHarnessConfig(io, options, false);
  const remote = options.remote ?? 'origin';
  const state = primaryCandidate(config);
  if (!state.usable) {
    io.stdout.write(`publish-state: canonical state is not usable (${state.reason}); nothing published\n`);
  } else {
    const env = remoteEnv(io, top, remote);
    const plumb = (args, input, extra) => execFileSync('git', args, { cwd: top, env: { ...env, ...extra }, input, encoding: 'utf8' }).trim();
    const blob = (name) => plumb(['hash-object', '-w', join(config.stateDir, name)], undefined, {});
    const tree = plumb(['mktree'], `100644 blob ${blob(ATTESTATION_FILE)}\t${ATTESTATION_FILE}\n100644 blob ${blob(REPORT_FILE)}\t${REPORT_FILE}\n`, {});
    const commit = plumb(['commit-tree', tree, '-m', `mutation-state ${options.kind}`], undefined, AUTHOR);
    must(spawnSync('git', ['push', '--force', remote, `${commit}:${branchRef(options.kind)}`], { cwd: top, env, encoding: 'utf8' }), `push to ${remote}`, 'publish-state');
    io.stdout.write(`publish-state: pushed ${commit} to ${branchRef(options.kind)}\n`);
  }
  io.stderr.write(summaryLine({ command: 'publish-state', pendingFull: canonicalPendingFull(config) }));
  return 0;
}
```

Edit `templates/mutation-incremental/lib/main.mjs` — add the import and the table entries:

```js
import { fetchStateCommand, publishStateCommand } from './remote.mjs';
```

```js
const COMMANDS = {
  plan: planCommand,
  run: runCommand,
  seed: seedCommand,
  'fetch-state': fetchStateCommand,
  'publish-state': publishStateCommand,
  'break-lock': breakLockCommand,
};
```

- [ ] **Step 4: Run the suite with coverage, then the repository gates**

Run: `sh .superpowers/sdd/2026-09-23-mutation-incremental-1b-execution/cov.sh`
Expected: all pass; no uncovered entries.

Run: `go test -count=1 -run TestMutationIncrementalTemplate . && go vet ./...`
Expected: `ok`; vet clean.

- [ ] **Step 5: Commit**

```bash
git add templates/mutation-incremental/lib/remote.mjs templates/mutation-incremental/lib/main.mjs templates/mutation-incremental/test/remote.test.mjs
git commit -m "feat(mutation-incremental): fetch-state and publish-state over mutation-state branches

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

## Follow-ups (recorded, not in 1b: no credible normal-usage failure)

- `publish-state` takes no lock. Each branch has a single writer in CI (§5.7), and a local publish is documented as CI-only.
- A lock whose pid was reused by an unrelated live process on the same host stays held until `break-lock` (rare; visible, not silent).
- `fetch-state` fetches the two kinds one after the other (two round trips).

## Self-Review (done while writing)

- **Spec coverage in 1b:**

  | Spec section | Task |
  |---|---|
  | §5.1 `run`/`seed`/`fetch-state`/`publish-state`/`break-lock`, exit codes and precedence, `GITHUB_OUTPUT`, summary lines | 1, 2, 6, 7, 8 |
  | §5.4 Adopt (run copies the adopted pair) | 3 (`installAdopted`), 6 |
  | §5.5 attestation (score, thresholdBreak, lastFullAt, changed-during-run, deferrals) | 3, 6 |
  | §5.6 steps 1–7 (lock, work dir, invoke, judge, commit, signals, kill grace) | 1, 2, 3, 4, 6 |
  | §5.7 `fetch-state`/`publish-state`, credentials via `GIT_CONFIG_*`, `::add-mask::` | 8 |
  | §5.8 + §11.6 seed (lock, `--replace` backup, merge and re-keying, no verify) | 7 |
  | §11.1 verify (environment, per view, spawn failure, timeout, exit 1, not on a cold run) | 5, 6 |
  | §11.2 `--pr`, `--max-minutes`, unbudgeted, `inherited` flags, `pending_cause`/`pending_causes`, shortcut | 6 |
  | §11.3 `views`/`viewSummaries` in the attestation | 3, 6, 7 |

  Deferred to 1c:
  - the workflow template (§5.7 jobs, the step summary of §5.7 step 5 and §11.2, `pr-full`);
  - the §7.1 fixture and e2e with real Stryker;
  - the `stateVersion` golden-output guard (§11.6);
  - docs.

  Deferred to Plan 2: §6.
- **Placeholders:** none. Every code step is complete, and every edit to an existing file shows the exact lines.
- **Type consistency:** these names are used identically across tasks:
  - `loadHarnessConfig(io, options, warn)`, `planInputs(top, config, alsoState)`;
  - `spawnGroup(argv, {cwd, env, timeoutMs, graceMs, interrupt, onOutput})`;
  - `invoke(config, {index, input, force, mutate, timeoutMs, interrupt, graceMs, onOutput})`;
  - `buildAttestation({config, files, runtime, reportBytes, report, mode, completedAt, lastFullAt, deferrals, views, summaries})`;
  - `commitState(stateDir, outputPath, attestation)`;
  - `runVerify(config, {views, snapshot, pr, interrupt, graceMs, onOutput})`;
  - `runRepo`/`cli`/`warm`.
- **Review Focus:** each of the five items has a named test in its owning task.


## Git

- Base: `e96d25df109e61e2fa291246deaa076005020e1c`
- Head: `17e20594e606b087a0739b5094e2c0fca2e85750`
- Branch: `mutation-incremental`
- Gate effect: `gate`

## Context Profile

- Raw diff bytes: `111295`
- Filtered diff bytes: `111295`
- Risk level: `none`

## Context Shard Plan

Not sharded.

## Review Manifest

- Manifest verdict: `PASS`
- Source manifest hash: not sharded
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- templates/mutation-incremental/lib/attest.mjs
- templates/mutation-incremental/lib/engine.mjs
- templates/mutation-incremental/lib/errors.mjs
- templates/mutation-incremental/lib/inputs.mjs
- templates/mutation-incremental/lib/lock.mjs
- templates/mutation-incremental/lib/main.mjs
- templates/mutation-incremental/lib/proc.mjs
- templates/mutation-incremental/lib/remote.mjs
- templates/mutation-incremental/lib/run.mjs
- templates/mutation-incremental/lib/seed.mjs
- templates/mutation-incremental/lib/snapshot.mjs
- templates/mutation-incremental/lib/state.mjs
- templates/mutation-incremental/lib/verify.mjs
- templates/mutation-incremental/test/attest.test.mjs
- templates/mutation-incremental/test/engine.test.mjs
- templates/mutation-incremental/test/fake-stryker.mjs
- templates/mutation-incremental/test/fake.mjs
- templates/mutation-incremental/test/json.test.mjs
- templates/mutation-incremental/test/lock.test.mjs
- templates/mutation-incremental/test/main.test.mjs
- templates/mutation-incremental/test/proc.test.mjs
- templates/mutation-incremental/test/remote.test.mjs
- templates/mutation-incremental/test/run.test.mjs
- templates/mutation-incremental/test/seed.test.mjs
- templates/mutation-incremental/test/state.test.mjs
- templates/mutation-incremental/test/verify.test.mjs

### Manifest Blockers
No manifest blockers.

## Changed Files

- templates/mutation-incremental/lib/attest.mjs
- templates/mutation-incremental/lib/engine.mjs
- templates/mutation-incremental/lib/errors.mjs
- templates/mutation-incremental/lib/inputs.mjs
- templates/mutation-incremental/lib/lock.mjs
- templates/mutation-incremental/lib/main.mjs
- templates/mutation-incremental/lib/proc.mjs
- templates/mutation-incremental/lib/remote.mjs
- templates/mutation-incremental/lib/run.mjs
- templates/mutation-incremental/lib/seed.mjs
- templates/mutation-incremental/lib/snapshot.mjs
- templates/mutation-incremental/lib/state.mjs
- templates/mutation-incremental/lib/verify.mjs
- templates/mutation-incremental/test/attest.test.mjs
- templates/mutation-incremental/test/engine.test.mjs
- templates/mutation-incremental/test/fake-stryker.mjs
- templates/mutation-incremental/test/fake.mjs
- templates/mutation-incremental/test/json.test.mjs
- templates/mutation-incremental/test/lock.test.mjs
- templates/mutation-incremental/test/main.test.mjs
- templates/mutation-incremental/test/proc.test.mjs
- templates/mutation-incremental/test/remote.test.mjs
- templates/mutation-incremental/test/run.test.mjs
- templates/mutation-incremental/test/seed.test.mjs
- templates/mutation-incremental/test/state.test.mjs
- templates/mutation-incremental/test/verify.test.mjs

## Diff

```diff
diff --git a/templates/mutation-incremental/lib/attest.mjs b/templates/mutation-incremental/lib/attest.mjs
new file mode 100644
index 0000000..00c06b2
--- /dev/null
+++ b/templates/mutation-incremental/lib/attest.mjs
@@ -0,0 +1,80 @@
+import { copyFileSync, mkdirSync, renameSync, rmSync, writeFileSync } from 'node:fs';
+import { join } from 'node:path';
+import { canonicalJSON } from './json.mjs';
+import { fileDigest, sha256 } from './snapshot.mjs';
+import { ATTESTATION_FILE, REPORT_FILE, STATE_VERSION, TOOL, TOOL_VERSION } from './state.mjs';
+
+export const WORK_DIR = 'work';
+export const CHANGED_DURING_RUN = 'changed-during-run';
+const SCORED = ['Killed', 'Timeout', 'Survived', 'NoCoverage'];
+
+// Stryker's definition (spec §5.5): (Killed + Timeout) / (Killed + Timeout + Survived + NoCoverage)
+// × 100, or null when that denominator is 0.
+export function mutationScore(report) {
+  const n = Object.fromEntries(SCORED.map((s) => [s, 0]));
+  for (const entry of Object.values(report.files)) {
+    for (const m of entry.mutants ?? []) if (Object.hasOwn(n, m.status)) n[m.status]++;
+  }
+  const denominator = n.Killed + n.Timeout + n.Survived + n.NoCoverage;
+  return denominator === 0 ? null : ((n.Killed + n.Timeout) / denominator) * 100;
+}
+
+// Spec §5.5 with §11.2 (inherited flags on deferrals), §11.3 (views) and §11.6 (stateVersion).
+export function buildAttestation({ config, files, runtime, reportBytes, report, mode, completedAt, lastFullAt, deferrals, views, summaries }) {
+  const score = mutationScore(report);
+  const attestation = {
+    schemaVersion: 1,
+    tool: TOOL,
+    toolVersion: TOOL_VERSION,
+    stateVersion: STATE_VERSION,
+    engine: 'stryker',
+    engineVersion: config.engineVersion,
+    mode,
+    completedAt,
+    lastFullAt,
+    report: REPORT_FILE,
+    reportSha256: sha256(reportBytes),
+    score,
+    // A null score or an absent threshold never breaks.
+    thresholdBreak: score !== null && config.thresholdBreak !== null && score < config.thresholdBreak,
+    lists: config.lists,
+    exclusions: config.exclusions,
+    files,
+    runtime,
+    deferrals,
+  };
+  if (views !== null) Object.assign(attestation, { views, viewSummaries: summaries });
+  return attestation;
+}
+
+// Spec §5.5: a path whose digest at the end of the run differs from the start-of-run snapshot is
+// recorded as changed-during-run, so it never matches and is re-planned next time.
+export function markChangedDuringRun(config, files) {
+  return Object.fromEntries(Object.entries(files).map(([path, info]) => [path, fileDigest(config, path) === info.digest ? info : { ...info, digest: CHANGED_DURING_RUN }]));
+}
+
+// Spec §5.6 step 7: the attestation goes to work/, the output is renamed over incremental.json,
+// then the attestation over attestation.json. A crash between the renames leaves a pair whose
+// hashes disagree, which is cold next time and never trusted.
+export function commitState(stateDir, outputPath, attestation) {
+  const work = join(stateDir, WORK_DIR);
+  mkdirSync(work, { recursive: true });
+  const pending = join(work, ATTESTATION_FILE);
+  writeFileSync(pending, canonicalJSON(attestation));
+  const canonical = join(stateDir, REPORT_FILE);
+  if (outputPath !== canonical) renameSync(outputPath, canonical);
+  renameSync(pending, join(stateDir, ATTESTATION_FILE));
+  rmSync(work, { recursive: true, force: true });
+}
+
+// Spec §5.4 Adopt: run copies an adopted state's two files into stateDir (temp file + rename),
+// report first, so an interrupted copy leaves hashes that disagree.
+export function installAdopted(stateDir, fromDir) {
+  const work = join(stateDir, WORK_DIR);
+  mkdirSync(work, { recursive: true });
+  for (const name of [REPORT_FILE, ATTESTATION_FILE]) {
+    const tmp = join(work, `adopt-${name}`);
+    copyFileSync(join(fromDir, name), tmp);
+    renameSync(tmp, join(stateDir, name));
+  }
+}
diff --git a/templates/mutation-incremental/lib/engine.mjs b/templates/mutation-incremental/lib/engine.mjs
new file mode 100644
index 0000000..84e06ab
--- /dev/null
+++ b/templates/mutation-incremental/lib/engine.mjs
@@ -0,0 +1,77 @@
+import { copyFileSync, readFileSync, statSync } from 'node:fs';
+import { join } from 'node:path';
+import { spawnGroup } from './proc.mjs';
+import { WORK_DIR } from './attest.mjs';
+
+// Stryker prints this on stdout and stderr when the dry run finds no tests (spec F4).
+export const NO_TESTS = 'No tests were executed';
+
+// Spec §5.6 step 4: argv, no shell.
+export function strykerArgv(config, incrementalFile, { force, mutate }) {
+  return [
+    ...config.stryker.command, 'run', config.stryker.configFile,
+    '--incremental', '--incrementalFile', incrementalFile,
+    ...(force ? ['--force'] : []),
+    ...(mutate === null ? [] : ['--mutate', mutate.join(',')]),
+    ...config.stryker.extraArgs,
+  ];
+}
+
+const isObject = (v) => v !== null && typeof v === 'object' && !Array.isArray(v);
+
+// A mutation-testing-report with a `files` object (selected files absent from it have zero
+// mutants, spec F9), or null.
+export function parseReport(bytes) {
+  let report;
+  try {
+    report = JSON.parse(bytes.toString('utf8'));
+  } catch {
+    return null;
+  }
+  return isObject(report) && isObject(report.files) ? report : null;
+}
+
+// mtime (ns), size and inode: "written during the invocation" (spec §5.6 step 5).
+function stamp(path) {
+  try {
+    const s = statSync(path, { bigint: true });
+    return `${s.mtimeNs}:${s.size}:${s.ino}`;
+  } catch {
+    return null;
+  }
+}
+
+// One invocation (spec §5.6 steps 4–5). `input` is copied to work/inv-<index>.json first; null in
+// full mode, where Stryker starts fresh. The caller creates work/.
+export async function invoke(config, { index, input, force, mutate, timeoutMs, interrupt, graceMs, onOutput }) {
+  const name = `inv-${index}.json`;
+  const file = join(config.stateDir, WORK_DIR, name);
+  if (input !== null) copyFileSync(input, file);
+  const before = stamp(file);
+  let tail = '';
+  let noTests = false;
+  const r = await spawnGroup(strykerArgv(config, `${config.stateRel}/${WORK_DIR}/${name}`, { force, mutate }), {
+    cwd: config.top,
+    env: process.env,
+    timeoutMs,
+    graceMs,
+    interrupt,
+    onOutput: (chunk) => {
+      onOutput(chunk);
+      const text = tail + chunk.toString('utf8');
+      if (text.includes(NO_TESTS)) noTests = true;
+      tail = text.slice(-NO_TESTS.length);
+    },
+  });
+  if (r.interrupted) return { outcome: 'interrupted' };
+  if (r.timedOut) return { outcome: 'timeout' };
+  const after = stamp(file);
+  const written = after !== null && after !== before;
+  if (written && (r.code === 0 || r.code === 1)) {
+    const bytes = readFileSync(file);
+    const report = parseReport(bytes);
+    if (report !== null) return { outcome: 'success', path: file, bytes, report };
+  }
+  if (!written && r.code === 1 && noTests) return { outcome: 'no-tests' };
+  return { outcome: 'failed', detail: r.error ? r.error.message : `exit ${r.code}` };
+}
diff --git a/templates/mutation-incremental/lib/errors.mjs b/templates/mutation-incremental/lib/errors.mjs
index 1fcbc5a..557c47d 100644
--- a/templates/mutation-incremental/lib/errors.mjs
+++ b/templates/mutation-incremental/lib/errors.mjs
@@ -19,3 +19,10 @@ export class EngineError extends Error {
     this.exitCode = 4;
   }
 }
+
+export class InterruptedError extends Error {
+  constructor(message) {
+    super(message);
+    this.exitCode = 130;
+  }
+}
diff --git a/templates/mutation-incremental/lib/inputs.mjs b/templates/mutation-incremental/lib/inputs.mjs
new file mode 100644
index 0000000..88c322b
--- /dev/null
+++ b/templates/mutation-incremental/lib/inputs.mjs
@@ -0,0 +1,38 @@
+import { execFileSync } from 'node:child_process';
+import { resolve } from 'node:path';
+import { UsageError } from './errors.mjs';
+import { loadConfig } from './config.mjs';
+import { takeSnapshot } from './snapshot.mjs';
+import { buildGraph } from './graph.mjs';
+import { adopt, primaryCandidate, readCandidate } from './state.mjs';
+import { nodeVersionWarning } from './nodever.mjs';
+
+// Spec §5.1: every command prints exactly one summary line on stderr.
+export const summaryLine = ({ command, invocations = 0, scope = 0, forced = 0, deferrals = 0, pendingFull = false }) =>
+  `mutation-incremental: command=${command} invocations=${invocations} scope=${scope} forced=${forced} deferrals=${deferrals} pending_full=${pendingFull}\n`;
+
+function topLevel(cwd) {
+  try {
+    return execFileSync('git', ['rev-parse', '--show-toplevel'], { cwd, encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
+  } catch {
+    throw new UsageError('not inside a git repository');
+  }
+}
+
+// Every command: the git top-level and the validated config (spec §5.1). plan and run also print
+// the Node-version warning.
+export function loadHarnessConfig(io, options, warn) {
+  const top = topLevel(io.cwd);
+  const config = loadConfig(top, options.config);
+  const warning = warn ? nodeVersionWarning(top) : null;
+  if (warning) io.stderr.write(`${warning}\n`);
+  return { top, config };
+}
+
+// Everything planning needs, with adoption in memory only (spec §5.4).
+export function planInputs(top, config, alsoState) {
+  const snapshot = takeSnapshot(config);
+  const graph = buildGraph(snapshot.files, config);
+  const candidates = [primaryCandidate(config), ...alsoState.map((dir) => readCandidate(resolve(top, dir), { label: dir, primary: false }))];
+  return { snapshot, graph, candidates, chosen: adopt(candidates, snapshot) };
+}
diff --git a/templates/mutation-incremental/lib/lock.mjs b/templates/mutation-incremental/lib/lock.mjs
new file mode 100644
index 0000000..7aa7a53
--- /dev/null
+++ b/templates/mutation-incremental/lib/lock.mjs
@@ -0,0 +1,68 @@
+import { readFileSync, rmSync, writeFileSync } from 'node:fs';
+import { hostname } from 'node:os';
+import { join } from 'node:path';
+import { LockHeldError } from './errors.mjs';
+import { loadHarnessConfig, summaryLine } from './inputs.mjs';
+import { canonicalPendingFull } from './state.mjs';
+
+export const LOCK_FILE = 'lock';
+
+const readText = (path) => {
+  try {
+    return readFileSync(path, 'utf8');
+  } catch {
+    return null;
+  }
+};
+
+function readLock(path) {
+  try {
+    return JSON.parse(readText(path));
+  } catch {
+    return null;
+  }
+}
+
+// kill(pid, 0) probes without signalling; EPERM means the process exists under another user.
+export function pidAlive(pid) {
+  try {
+    process.kill(pid, 0);
+    return true;
+  } catch (e) {
+    return e.code === 'EPERM';
+  }
+}
+
+// Spec §5.6 step 1: create <stateDir>/lock with O_EXCL. A lock from this host whose pid is gone is
+// a crashed run and is replaced; anything else is held (exit 3). The caller creates stateDir.
+export function acquireLock(stateDir, { pid = process.pid, host = hostname(), now = new Date() } = {}) {
+  const path = join(stateDir, LOCK_FILE);
+  const content = `${JSON.stringify({ pid, hostname: host, startedAt: now.toISOString() })}\n`;
+  try {
+    writeFileSync(path, content, { flag: 'wx' });
+  } catch (e) {
+    if (e.code !== 'EEXIST') throw e;
+    const held = readLock(path);
+    if (held === null || held.hostname !== host || pidAlive(held.pid)) {
+      const who = held === null ? 'unreadable lock' : `pid ${held.pid} on ${held.hostname} since ${held.startedAt}`;
+      throw new LockHeldError(`lock held: ${path} (${who}); if no run is active, run break-lock`);
+    }
+    writeFileSync(path, content);
+  }
+  return () => rmSync(path, { force: true });
+}
+
+// Spec §5.1: print and remove <stateDir>/lock (clears a lock left on another machine).
+export async function breakLockCommand(io, options) {
+  const { config } = loadHarnessConfig(io, options, false);
+  const path = join(config.stateDir, LOCK_FILE);
+  const text = readText(path);
+  if (text === null) io.stdout.write(`break-lock: no lock at ${path}\n`);
+  else {
+    io.stdout.write(text);
+    rmSync(path, { force: true });
+    io.stdout.write(`break-lock: removed ${path}\n`);
+  }
+  io.stderr.write(summaryLine({ command: 'break-lock', pendingFull: canonicalPendingFull(config) }));
+  return 0;
+}
diff --git a/templates/mutation-incremental/lib/main.mjs b/templates/mutation-incremental/lib/main.mjs
index dc4aed7..6e45f84 100644
--- a/templates/mutation-incremental/lib/main.mjs
+++ b/templates/mutation-incremental/lib/main.mjs
@@ -1,60 +1,47 @@
-import { execFileSync } from 'node:child_process';
-import { resolve } from 'node:path';
 import { UsageError } from './errors.mjs';
 import { canonicalJSON } from './json.mjs';
-import { loadConfig } from './config.mjs';
-import { takeSnapshot } from './snapshot.mjs';
-import { buildGraph } from './graph.mjs';
-import { readCandidate, adopt } from './state.mjs';
 import { computePlan, publicPlan } from './plan.mjs';
 import { resolveViews } from './views.mjs';
-import { nodeVersionWarning } from './nodever.mjs';
+import { loadHarnessConfig, planInputs, summaryLine } from './inputs.mjs';
+import { breakLockCommand } from './lock.mjs';
+import { runCommand } from './run.mjs';
+import { seedCommand } from './seed.mjs';
+import { fetchStateCommand, publishStateCommand } from './remote.mjs';
 
-const USAGE = 'usage: cli.mjs plan [--config <path>] [--also-state <dir>]...';
+const USAGE = `usage: cli.mjs <command> [--config <path>]
+  plan [--also-state <dir>]...
+  run --mode incremental|full [--also-state <dir>]... [--pr [--max-minutes <m>]]
+  seed --from <report>... [--replace]
+  fetch-state [--remote <name>]
+  publish-state --kind inc|full [--remote <name>]
+  break-lock`;
 
+const VALUE_OPTIONS = { '--config': 'config', '--mode': 'mode', '--max-minutes': 'maxMinutes', '--remote': 'remote', '--kind': 'kind' };
+const LIST_OPTIONS = { '--also-state': 'alsoState', '--from': 'from' };
+const FLAG_OPTIONS = { '--pr': 'pr', '--replace': 'replace' };
+
+// One option grammar for every command; each command reads the options it uses (spec §5.1).
 export function parseArgs(args) {
-  const out = { command: args[0], config: undefined, alsoState: [], rest: [] };
+  const out = { command: args[0], config: undefined, mode: undefined, maxMinutes: undefined, remote: undefined, kind: undefined, alsoState: [], from: [], pr: false, replace: false };
   for (let i = 1; i < args.length; i++) {
     const a = args[i];
-    const value = () => {
-      if (i + 1 >= args.length) throw new UsageError(`${a} needs a value`);
-      return args[++i];
-    };
-    if (a === '--config') out.config = value();
-    else if (a === '--also-state') out.alsoState.push(value());
-    else throw new UsageError(`unknown option ${a}`);
+    if (Object.hasOwn(FLAG_OPTIONS, a)) {
+      out[FLAG_OPTIONS[a]] = true;
+      continue;
+    }
+    const single = Object.hasOwn(VALUE_OPTIONS, a);
+    if (!single && !Object.hasOwn(LIST_OPTIONS, a)) throw new UsageError(`unknown option ${a}`);
+    if (i + 1 >= args.length) throw new UsageError(`${a} needs a value`);
+    const value = args[++i];
+    if (single) out[VALUE_OPTIONS[a]] = value;
+    else out[LIST_OPTIONS[a]].push(value);
   }
   return out;
 }
 
-export const summaryLine = ({ command, invocations = 0, scope = 0, forced = 0, deferrals = 0, pendingFull = false }) =>
-  `mutation-incremental: command=${command} invocations=${invocations} scope=${scope} forced=${forced} deferrals=${deferrals} pending_full=${pendingFull}\n`;
-
-function topLevel(cwd) {
-  try {
-    return execFileSync('git', ['rev-parse', '--show-toplevel'], { cwd, encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
-  } catch {
-    throw new UsageError('not inside a git repository');
-  }
-}
-
-// Shared with Plan 1b's run command: everything planning needs, with adoption in memory only.
-export function loadPlanInputs(io, options) {
-  const top = topLevel(io.cwd);
-  const config = loadConfig(top, options.config);
-  const warning = nodeVersionWarning(top);
-  if (warning) io.stderr.write(`${warning}\n`);
-  const snapshot = takeSnapshot(config);
-  const graph = buildGraph(snapshot.files, config);
-  const candidates = [
-    readCandidate(config.stateDir, { label: config.stateDirRaw, primary: true }),
-    ...options.alsoState.map((dir) => readCandidate(resolve(top, dir), { label: dir, primary: false })),
-  ];
-  return { top, config, snapshot, graph, candidates, chosen: adopt(candidates, snapshot) };
-}
-
 function planCommand(io, options) {
-  const { config, snapshot, graph, chosen } = loadPlanInputs(io, options);
+  const { top, config } = loadHarnessConfig(io, options, true);
+  const { snapshot, graph, chosen } = planInputs(top, config, options.alsoState);
   const plan = computePlan({ config, snapshot, graph, chosen });
   resolveViews(config, snapshot);
   io.stdout.write(canonicalJSON(publicPlan(plan)));
@@ -69,19 +56,27 @@ function planCommand(io, options) {
   return 0;
 }
 
-const COMMANDS = { plan: planCommand };
+const COMMANDS = {
+  plan: planCommand,
+  run: runCommand,
+  seed: seedCommand,
+  'fetch-state': fetchStateCommand,
+  'publish-state': publishStateCommand,
+  'break-lock': breakLockCommand,
+};
 
 export async function main(args, io = { stdout: process.stdout, stderr: process.stderr, cwd: process.cwd() }) {
   try {
     const options = parseArgs(args);
     const command = COMMANDS[options.command];
-    if (!command) throw new UsageError(options.command === undefined ? USAGE : `unknown command ${options.command}; ${USAGE}`);
+    if (!command) throw new UsageError(options.command === undefined ? USAGE : `unknown command ${options.command}\n${USAGE}`);
     return await command(io, options);
   } catch (e) {
-    // Exit 1 means "ok, state committed" for run (Plan 1b), so an internal crash is exit 4.
-    const usage = e instanceof UsageError;
-    io.stderr.write(`mutation-incremental: ${usage ? 'error' : 'internal error'}: ${e.message}\n`);
+    // Known failures carry their exit code (2, 3, 4, 130). Anything else is an internal error and
+    // exits 4: for run, exit 1 means "ok, state committed", so a crash must never produce it.
+    const known = Number.isInteger(e.exitCode);
+    io.stderr.write(`mutation-incremental: ${known ? 'error' : 'internal error'}: ${e.message}\n`);
     io.stderr.write(summaryLine({ command: args[0] ?? 'none' }));
-    return usage ? e.exitCode : 4;
+    return known ? e.exitCode : 4;
   }
 }
diff --git a/templates/mutation-incremental/lib/proc.mjs b/templates/mutation-incremental/lib/proc.mjs
new file mode 100644
index 0000000..d57aa0d
--- /dev/null
+++ b/templates/mutation-incremental/lib/proc.mjs
@@ -0,0 +1,77 @@
+import { spawn } from 'node:child_process';
+
+// Spec §5.6 step 4: on a timeout or an interrupt the group gets SIGTERM, then SIGKILL after 30 s.
+export const KILL_GRACE_MS = 30_000;
+
+// One interrupt per command run: signal handlers trigger it, running children subscribe to it.
+export function createInterrupt() {
+  const listeners = new Set();
+  const interrupt = {
+    interrupted: false,
+    onInterrupt(fn) {
+      listeners.add(fn);
+      return () => listeners.delete(fn);
+    },
+    trigger() {
+      interrupt.interrupted = true;
+      for (const fn of [...listeners]) fn();
+    },
+  };
+  return interrupt;
+}
+
+// Spec §5.6: SIGINT, SIGTERM and SIGHUP to the harness stop the running child and the run.
+export function installSignalHandlers(interrupt) {
+  const signals = ['SIGINT', 'SIGTERM', 'SIGHUP'];
+  const handler = () => interrupt.trigger();
+  for (const s of signals) process.on(s, handler);
+  return () => {
+    for (const s of signals) process.off(s, handler);
+  };
+}
+
+// Signals a whole process group; false when it no longer exists.
+export function signalGroup(pid, signal) {
+  try {
+    process.kill(-pid, signal);
+    return true;
+  } catch {
+    return false;
+  }
+}
+
+const NOT_RUN = { code: null, signal: null, timedOut: false, interrupted: true, error: null };
+
+// Runs argv without a shell in its own process group (detached), forwarding its output. A spawn
+// failure resolves with `error` set (Node may also emit 'close'; the promise settles once).
+export function spawnGroup(argv, { cwd, env, timeoutMs, graceMs, interrupt, onOutput }) {
+  if (interrupt.interrupted) return Promise.resolve(NOT_RUN);
+  return new Promise((settle) => {
+    const child = spawn(argv[0], argv.slice(1), { cwd, env, detached: true, stdio: ['ignore', 'pipe', 'pipe'] });
+    let timedOut = false;
+    let interrupted = false;
+    let killTimer = null;
+    const stop = () => {
+      signalGroup(child.pid, 'SIGTERM');
+      killTimer = setTimeout(() => signalGroup(child.pid, 'SIGKILL'), graceMs);
+    };
+    const timer = timeoutMs === null ? null : setTimeout(() => {
+      timedOut = true;
+      stop();
+    }, timeoutMs);
+    const unsubscribe = interrupt.onInterrupt(() => {
+      interrupted = true;
+      stop();
+    });
+    child.stdout.on('data', onOutput);
+    child.stderr.on('data', onOutput);
+    const finish = (code, signal, error) => {
+      clearTimeout(timer);
+      clearTimeout(killTimer);
+      unsubscribe();
+      settle({ code, signal, timedOut, interrupted, error });
+    };
+    child.on('error', (error) => finish(null, null, error));
+    child.on('close', (code, signal) => finish(code, signal, null));
+  });
+}
diff --git a/templates/mutation-incremental/lib/remote.mjs b/templates/mutation-incremental/lib/remote.mjs
new file mode 100644
index 0000000..ba7376c
--- /dev/null
+++ b/templates/mutation-incremental/lib/remote.mjs
@@ -0,0 +1,87 @@
+import { execFileSync, spawnSync } from 'node:child_process';
+import { mkdirSync, renameSync, rmSync, writeFileSync } from 'node:fs';
+import { join } from 'node:path';
+import { UsageError } from './errors.mjs';
+import { loadHarnessConfig, summaryLine } from './inputs.mjs';
+import { ATTESTATION_FILE, REPORT_FILE, canonicalPendingFull, primaryCandidate } from './state.mjs';
+
+export const KINDS = ['inc', 'full'];
+const AUTHOR = { GIT_AUTHOR_NAME: 'mutation-incremental', GIT_AUTHOR_EMAIL: 'noreply@localhost', GIT_COMMITTER_NAME: 'mutation-incremental', GIT_COMMITTER_EMAIL: 'noreply@localhost' };
+const branchRef = (kind) => `refs/heads/mutation-state/${kind}`;
+
+// Spec §5.7: the token travels as an http extraheader through GIT_CONFIG_* (never argv), appended
+// after entries already in the environment. An empty or unset token means git's own credentials.
+export function authEnv(env, url, token) {
+  if (!token) return { env, mask: null };
+  const mask = Buffer.from(`x-access-token:${token}`).toString('base64');
+  const n = Number(env.GIT_CONFIG_COUNT ?? 0);
+  return {
+    env: { ...env, GIT_CONFIG_COUNT: String(n + 1), [`GIT_CONFIG_KEY_${n}`]: `http.${url}.extraheader`, [`GIT_CONFIG_VALUE_${n}`]: `AUTHORIZATION: basic ${mask}` },
+    mask,
+  };
+}
+
+function remoteEnv(io, top, remote) {
+  const url = spawnSync('git', ['remote', 'get-url', remote], { cwd: top, encoding: 'utf8' });
+  if (url.status !== 0) throw new UsageError(`unknown git remote ${remote}`);
+  const { env, mask } = authEnv(process.env, url.stdout.trim(), process.env.MUTATION_STATE_TOKEN);
+  if (mask !== null && process.env.GITHUB_ACTIONS) io.stdout.write(`::add-mask::${mask}\n`);
+  return env;
+}
+
+// A failed ls-remote, fetch, show or push is exit 2 (spec §5.7: network and auth failures are visible).
+function must(result, what, command) {
+  if (result.status !== 0) throw new UsageError(`${command}: ${what} failed: ${String(result.stderr).trim()}`);
+  return result.stdout;
+}
+
+// Spec §5.7: list the state branches (a missing one is skipped), fetch each present one without
+// --depth, and write its two files into <stateDir>/remote/<kind>/. A kind's old copy is removed
+// first, so a branch that is gone leaves nothing to adopt.
+export async function fetchStateCommand(io, options) {
+  const { top, config } = loadHarnessConfig(io, options, false);
+  const remote = options.remote ?? 'origin';
+  const env = remoteEnv(io, top, remote);
+  const listing = must(spawnSync('git', ['ls-remote', remote, 'refs/heads/mutation-state/*'], { cwd: top, env, encoding: 'utf8' }), `git ls-remote ${remote}`, 'fetch-state');
+  const present = new Set(listing.split('\n').filter(Boolean).map((line) => line.split('\t')[1]));
+  for (const kind of KINDS) {
+    const dir = join(config.stateDir, 'remote', kind);
+    rmSync(dir, { recursive: true, force: true });
+    if (!present.has(branchRef(kind))) {
+      io.stderr.write(`fetch-state: ${branchRef(kind)} is not on ${remote}; skipped\n`);
+      continue;
+    }
+    must(spawnSync('git', ['fetch', '--no-tags', remote, branchRef(kind)], { cwd: top, env, encoding: 'utf8' }), `git fetch ${branchRef(kind)}`, 'fetch-state');
+    mkdirSync(dir, { recursive: true });
+    for (const name of [ATTESTATION_FILE, REPORT_FILE]) {
+      // Reports embed every mutated file's source: far beyond spawnSync's 1 MiB default buffer.
+      const bytes = must(spawnSync('git', ['show', `FETCH_HEAD:${name}`], { cwd: top, env, maxBuffer: 1 << 30 }), `reading ${name} from ${branchRef(kind)}`, 'fetch-state');
+      writeFileSync(join(dir, `${name}.tmp`), bytes);
+      renameSync(join(dir, `${name}.tmp`), join(dir, name));
+    }
+  }
+  io.stderr.write(summaryLine({ command: 'fetch-state', pendingFull: canonicalPendingFull(config) }));
+  return 0;
+}
+
+// Spec §5.7: a parentless commit holding the canonical pair, built with plumbing and force-pushed
+// to mutation-state/<kind>. Only a usable state is published; otherwise it says why and exits 0.
+export async function publishStateCommand(io, options) {
+  if (!KINDS.includes(options.kind)) throw new UsageError('publish-state needs --kind inc|full');
+  const { top, config } = loadHarnessConfig(io, options, false);
+  const remote = options.remote ?? 'origin';
+  const state = primaryCandidate(config);
+  if (!state.usable) {
+    io.stdout.write(`publish-state: canonical state is not usable (${state.reason}); nothing published\n`);
+  } else {
+    const env = remoteEnv(io, top, remote);
+    const plumb = (args, input, extra) => execFileSync('git', args, { cwd: top, env: { ...env, ...extra }, input, encoding: 'utf8' }).trim();
+    const blob = (name) => plumb(['hash-object', '-w', join(config.stateDir, name)], undefined, {});
+    const tree = plumb(['mktree'], `100644 blob ${blob(ATTESTATION_FILE)}\t${ATTESTATION_FILE}\n100644 blob ${blob(REPORT_FILE)}\t${REPORT_FILE}\n`, {});
+    const commit = plumb(['commit-tree', tree, '-m', `mutation-state ${options.kind}`], undefined, AUTHOR);
+    must(spawnSync('git', ['push', '--force', remote, `${commit}:${branchRef(options.kind)}`], { cwd: top, env, encoding: 'utf8' }), `push to ${remote}`, 'publish-state');
+    io.stdout.write(`publish-state: pushed ${commit} to ${branchRef(options.kind)}\n`);
+  }
+  io.stderr.write(summaryLine({ command: 'publish-state', pendingFull: canonicalPendingFull(config) }));
+  return 0;
+}
diff --git a/templates/mutation-incremental/lib/run.mjs b/templates/mutation-incremental/lib/run.mjs
new file mode 100644
index 0000000..e854832
--- /dev/null
+++ b/templates/mutation-incremental/lib/run.mjs
@@ -0,0 +1,192 @@
+import { appendFileSync, existsSync, mkdirSync, readFileSync, rmSync } from 'node:fs';
+import { join } from 'node:path';
+import { EngineError, InterruptedError, UsageError } from './errors.mjs';
+import { loadHarnessConfig, planInputs, summaryLine } from './inputs.mjs';
+import { computePlan } from './plan.mjs';
+import { classifyDeferrals, dedupeDeferrals, deferralKey, pendingCause, sortDeferrals } from './deferrals.mjs';
+import { resolveViews, viewSummaries } from './views.mjs';
+import { acquireLock } from './lock.mjs';
+import { buildAttestation, commitState, installAdopted, markChangedDuringRun, WORK_DIR } from './attest.mjs';
+import { invoke, NO_TESTS, parseReport } from './engine.mjs';
+import { runVerify } from './verify.mjs';
+import { createInterrupt, installSignalHandlers, KILL_GRACE_MS } from './proc.mjs';
+import { canonicalPendingFull, REPORT_FILE } from './state.mjs';
+
+export const MAX_CAUSES = 50;
+
+// Forced entries are `<file>` or `<file>:<start>-<end>`; paths never contain ':' (the planner exits 2).
+const fileOf = (entry) => entry.split(':')[0];
+
+function checkOptions(options) {
+  if (options.mode !== 'incremental' && options.mode !== 'full') throw new UsageError('run needs --mode incremental|full');
+  if (options.pr && options.mode === 'full') throw new UsageError('--pr marks an incremental PR run; the sweep (--mode full) is never --pr');
+  if (options.maxMinutes === undefined) return null;
+  if (!options.pr) throw new UsageError('--max-minutes applies only to --pr runs');
+  const minutes = Number(options.maxMinutes);
+  if (!(minutes > 0)) throw new UsageError('--max-minutes must be a positive number');
+  return minutes;
+}
+
+// Spec §5.1: appended on every exit path when GITHUB_OUTPUT is set.
+function writeOutputs(entries) {
+  const file = process.env.GITHUB_OUTPUT;
+  if (!file) return;
+  appendFileSync(file, entries.map(([key, value]) => `${key}=${value}\n`).join(''));
+}
+
+// Spec §11.2: the counted {reason, paths} as single-line JSON, at most MAX_CAUSES entries followed
+// by {"more": n}. JSON.stringify escapes control characters; paths never contain a newline.
+export function causesJSON(counted) {
+  const list = counted.map((d) => ({ reason: d.reason, paths: d.paths }));
+  const shown = list.length > MAX_CAUSES ? [...list.slice(0, MAX_CAUSES), { more: list.length - MAX_CAUSES }] : list;
+  return JSON.stringify(shown);
+}
+
+function readOutput(path) {
+  if (!existsSync(path)) return null;
+  const bytes = readFileSync(path);
+  const report = parseReport(bytes);
+  return report === null ? null : { path, bytes, report };
+}
+
+// Spec §5.6 step 5: selected files with kills lose their killing tests; files without kills (type-only
+// files, a new module before its first test) record nothing.
+function noReachableDeferrals(files, report) {
+  return files
+    .filter((f) => (report.files[f]?.mutants ?? []).some((m) => m.status === 'Killed'))
+    .map((f) => ({ reason: `no reachable tests: ${f}`, paths: [f] }));
+}
+
+async function executeIncremental(ctx) {
+  const { config, inputs, options, interrupt } = ctx;
+  const { snapshot, graph, candidates, chosen } = inputs;
+  // Spec §11.2 (i): main's fetched states count whether or not they are usable.
+  const alsoAttestations = candidates.filter((c) => !c.primary && c.attestation !== null).map((c) => c.attestation);
+  if (chosen !== null && !chosen.primary) installAdopted(config.stateDir, chosen.dir);
+  const plan = computePlan({ config, snapshot, graph, chosen, unbudgeted: options.pr });
+  // Counted/inherited at plan time decides the shortcut (spec §11.2; review advisory).
+  const planned = classifyDeferrals({ deferrals: plan.deferrals, alsoAttestations, snapshot, cold: plan.cold });
+  const shortcut = options.pr && pendingCause(planned).cause === 'global';
+  let latest = readOutput(join(config.stateDir, REPORT_FILE));
+  const produced = [];
+  let invocations = 0;
+  if (!plan.cold && !shortcut) {
+    const timeoutMs = (ctx.maxMinutes ?? config.budget.maxMinutesPerInvocation) * 60_000;
+    let blocked = false;
+    const steps = [{ list: plan.invocation1, force: false }, { list: plan.invocation2, force: true }];
+    for (const [i, { list, force }] of steps.entries()) {
+      if (list.length === 0) continue;
+      const files = [...new Set(list.map(fileOf))].sort();
+      if (blocked) {
+        produced.push({ reason: 'blocked by deferred scope', paths: files });
+        continue;
+      }
+      const r = await invoke(config, { index: i + 1, input: latest.path, force, mutate: list, timeoutMs, interrupt, graceMs: ctx.graceMs, onOutput: ctx.onOutput });
+      invocations++;
+      if (r.outcome === 'interrupted') throw new InterruptedError('interrupted; nothing committed');
+      if (r.outcome === 'failed') throw new EngineError(`stryker invocation ${i + 1} failed (${r.detail}); nothing committed`);
+      if (r.outcome === 'success') latest = r;
+      else if (r.outcome === 'timeout') {
+        produced.push({ reason: 'time budget exceeded', paths: files });
+        blocked = true;
+      } else produced.push(...noReachableDeferrals(files, latest.report));
+    }
+  }
+  const deferrals = sortDeferrals(dedupeDeferrals(plan.deferrals, produced));
+  const producedKeys = new Set(produced.map(deferralKey));
+  return {
+    plan,
+    latest,
+    invocations,
+    mode: 'incremental',
+    lastFullAt: chosen === null ? null : chosen.attestation.lastFullAt,
+    classified: classifyDeferrals({ deferrals, alsoAttestations, snapshot, producedKeys, cold: plan.cold }),
+  };
+}
+
+async function executeFull(ctx) {
+  const r = await invoke(ctx.config, { index: 1, input: null, force: false, mutate: null, timeoutMs: null, interrupt: ctx.interrupt, graceMs: ctx.graceMs, onOutput: ctx.onOutput });
+  if (r.outcome === 'interrupted') throw new InterruptedError('interrupted; nothing committed');
+  // A full run that finds no tests (e.g. a dependency bump broke every import) is never attested.
+  if (r.outcome === 'no-tests') throw new EngineError(`the full run found no tests (${NO_TESTS}); nothing committed`);
+  if (r.outcome === 'failed') throw new EngineError(`the full run failed (${r.detail}); nothing committed`);
+  return { plan: null, latest: r, invocations: 1, mode: 'full', lastFullAt: null, classified: [] };
+}
+
+// Spec §5.6 steps 6–7. A cold run with no incremental.json commits nothing.
+function commit(ctx, result) {
+  const { config, inputs, views } = ctx;
+  if (result.latest === null) return null;
+  const completedAt = new Date().toISOString();
+  const attestation = buildAttestation({
+    config,
+    files: markChangedDuringRun(config, inputs.snapshot.files),
+    runtime: inputs.snapshot.runtime,
+    reportBytes: result.latest.bytes,
+    report: result.latest.report,
+    mode: result.mode,
+    completedAt,
+    lastFullAt: result.mode === 'full' ? completedAt : result.lastFullAt,
+    deferrals: result.classified,
+    views,
+    summaries: viewSummaries(result.latest.report, views, result.classified),
+  });
+  commitState(config.stateDir, result.latest.path, attestation);
+  return attestation;
+}
+
+// Spec §5.1 `run`, §5.6, §11.1, §11.2.
+export async function runCommand(io, options) {
+  const interrupt = createInterrupt();
+  const uninstall = installSignalHandlers(interrupt);
+  let config = null;
+  let release = null;
+  let exitCode = 0;
+  try {
+    const maxMinutes = checkOptions(options);
+    ({ config } = loadHarnessConfig(io, options, true));
+    if (options.pr && config.pendingOnPr === 'allow') throw new UsageError('--pr is for pendingOnPr "full" or "full-on-global"; under "allow" the workflow never passes it');
+    mkdirSync(config.stateDir, { recursive: true });
+    release = acquireLock(config.stateDir);
+    const work = join(config.stateDir, WORK_DIR);
+    rmSync(work, { recursive: true, force: true });
+    mkdirSync(work);
+    const inputs = planInputs(config.top, config, options.alsoState);
+    // Spec §11.3: view completeness is checked at plan time, before any invocation.
+    const views = resolveViews(config, inputs.snapshot);
+    const ctx = { config, inputs, views, options, interrupt, maxMinutes, graceMs: io.killGraceMs ?? KILL_GRACE_MS, onOutput: (chunk) => io.stderr.write(chunk) };
+    const result = options.mode === 'full' ? await executeFull(ctx) : await executeIncremental(ctx);
+    const attestation = commit(ctx, result);
+    const failures = [];
+    if (attestation !== null) {
+      if (attestation.thresholdBreak) failures.push(`score ${attestation.score} is below thresholds.break ${config.thresholdBreak}`);
+      failures.push(...(await runVerify(config, { views, snapshot: inputs.snapshot, pr: options.pr, interrupt, graceMs: ctx.graceMs, onOutput: ctx.onOutput })));
+    }
+    if (interrupt.interrupted) throw new InterruptedError('interrupted; the state is committed');
+    for (const f of failures) io.stderr.write(`mutation-incremental: ${f}\n`);
+    const code = failures.length > 0 ? 1 : 0;
+    const pendingFull = attestation === null || result.classified.length > 0;
+    const { cause, counted } = pendingCause(result.classified);
+    writeOutputs([['pending_full', pendingFull], ['exit_code', code], ['pending_cause', cause], ['pending_causes', causesJSON(counted)]]);
+    io.stderr.write(summaryLine({
+      command: 'run',
+      invocations: result.invocations,
+      scope: result.plan?.scope.length ?? 0,
+      forced: result.plan?.forced.length ?? 0,
+      deferrals: result.classified.length,
+      pendingFull,
+    }));
+    exitCode = code;
+  } catch (e) {
+    writeOutputs([['pending_full', config === null ? false : canonicalPendingFull(config)], ['exit_code', Number.isInteger(e.exitCode) ? e.exitCode : 4]]);
+    throw e;
+  } finally {
+    // Only the lock holder may touch work/: on exit 3 it belongs to the other run.
+    if (release !== null) {
+      rmSync(join(config.stateDir, WORK_DIR), { recursive: true, force: true });
+      release();
+    }
+    uninstall();
+  }
+  return exitCode;
+}
diff --git a/templates/mutation-incremental/lib/seed.mjs b/templates/mutation-incremental/lib/seed.mjs
new file mode 100644
index 0000000..640f167
--- /dev/null
+++ b/templates/mutation-incremental/lib/seed.mjs
@@ -0,0 +1,143 @@
+import { copyFileSync, existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
+import { join, resolve } from 'node:path';
+import { UsageError } from './errors.mjs';
+import { canonicalJSON } from './json.mjs';
+import { loadHarnessConfig, summaryLine } from './inputs.mjs';
+import { acquireLock } from './lock.mjs';
+import { takeSnapshot } from './snapshot.mjs';
+import { resolveViews, viewSummaries } from './views.mjs';
+import { dedupeDeferrals, sortDeferrals } from './deferrals.mjs';
+import { buildAttestation, commitState, WORK_DIR } from './attest.mjs';
+import { parseReport } from './engine.mjs';
+import { ATTESTATION_FILE, REPORT_FILE, primaryCandidate } from './state.mjs';
+
+const cmp = (a, b) => (a < b ? -1 : a > b ? 1 : 0);
+// Spec §11.6: a mutant in several reports takes the strongest status.
+const RANK = { Killed: 4, Timeout: 3, Survived: 2, NoCoverage: 1 };
+const rank = (status) => RANK[status] ?? 0;
+const position = (m) => [m.location.start.line, m.location.start.column, m.location.end.line, m.location.end.column];
+const byPosition = (a, b) => {
+  const pa = position(a);
+  const pb = position(b);
+  return pa.reduce((acc, v, i) => acc || v - pb[i], 0) || cmp(a.mutatorName, b.mutatorName) || cmp(String(a.replacement), String(b.replacement));
+};
+const union = (a, b) => [...new Set([...a, ...b])].sort((x, y) => Number(x) - Number(y));
+
+// Spec §11.6: tests are re-keyed by (file, name) and renumbered; mutants are matched by
+// (file, mutatorName, location, replacement), merged (status by precedence, test lists unioned),
+// and renumbered in byte order of file, then location. A path in several reports must have the
+// same source.
+export function mergeReports(reports) {
+  const tests = new Map();
+  const testFileEntries = new Map();
+  const localKeys = reports.map((report) => {
+    const local = new Map();
+    for (const [file, entry] of Object.entries(report.testFiles)) {
+      if (!testFileEntries.has(file)) testFileEntries.set(file, entry);
+      for (const t of entry.tests) {
+        const key = `${file}\u0000${t.name}`;
+        local.set(String(t.id), key);
+        if (!tests.has(key)) tests.set(key, { file, test: t });
+      }
+    }
+    return local;
+  });
+  const keys = [...tests.keys()].sort(cmp);
+  const newId = new Map(keys.map((k, i) => [k, String(i)]));
+  const files = new Map();
+  reports.forEach((report, i) => {
+    const remap = (ids) => (ids ?? []).map((id) => newId.get(localKeys[i].get(String(id))));
+    for (const [path, entry] of Object.entries(report.files)) {
+      if (!files.has(path)) files.set(path, { entry, mutants: new Map() });
+      const file = files.get(path);
+      if (file.entry.source !== entry.source) throw new UsageError(`seed: ${path} has a different source in the given reports`);
+      for (const m of entry.mutants) {
+        const key = JSON.stringify([m.mutatorName, position(m), m.replacement]);
+        const merged = { ...m, killedBy: remap(m.killedBy), coveredBy: remap(m.coveredBy) };
+        const prev = file.mutants.get(key);
+        if (prev === undefined) {
+          file.mutants.set(key, merged);
+          continue;
+        }
+        if (rank(m.status) > rank(prev.status)) prev.status = m.status;
+        prev.killedBy = union(prev.killedBy, merged.killedBy);
+        prev.coveredBy = union(prev.coveredBy, merged.coveredBy);
+      }
+    }
+  });
+  let next = 0;
+  const outFiles = {};
+  for (const path of [...files.keys()].sort(cmp)) {
+    const { entry, mutants } = files.get(path);
+    outFiles[path] = { ...entry, mutants: [...mutants.values()].sort(byPosition).map((m) => ({ ...m, id: String(next++) })) };
+  }
+  const outTests = {};
+  for (const key of keys) {
+    const { file, test } = tests.get(key);
+    outTests[file] ??= { ...testFileEntries.get(file), tests: [] };
+    outTests[file].tests.push({ ...test, id: newId.get(key) });
+  }
+  return { ...reports[0], files: outFiles, testFiles: outTests };
+}
+
+function readReport(top, path) {
+  let bytes;
+  try {
+    bytes = readFileSync(resolve(top, path));
+  } catch (e) {
+    throw new UsageError(`seed: cannot read ${path}: ${e.message}`);
+  }
+  const report = parseReport(bytes);
+  if (report === null) throw new UsageError(`seed: ${path} is not a mutation-testing report (no files object)`);
+  return report;
+}
+
+// Spec §5.8: --replace first copies the current pair into replaced/<UTC timestamp>/.
+function keepCurrent(stateDir) {
+  const dir = join(stateDir, 'replaced', new Date().toISOString().replaceAll(':', '-'));
+  mkdirSync(dir, { recursive: true });
+  for (const name of [ATTESTATION_FILE, REPORT_FILE]) {
+    if (existsSync(join(stateDir, name))) copyFileSync(join(stateDir, name), join(dir, name));
+  }
+}
+
+// Spec §5.8, §11.6: bootstrap from full reports. The kills stay pending ("seeded from <report>" on
+// ["*"]) until the first full run on main. Never exits 1: it prints the score instead.
+export async function seedCommand(io, options) {
+  if (options.from.length === 0) throw new UsageError('seed needs --from <report> (repeatable)');
+  const { top, config } = loadHarnessConfig(io, options, false);
+  mkdirSync(config.stateDir, { recursive: true });
+  const release = acquireLock(config.stateDir);
+  try {
+    if (primaryCandidate(config).usable && !options.replace) throw new UsageError(`usable state exists in ${config.stateDirRaw}; pass --replace to seed over it`);
+    const merged = mergeReports(options.from.map((p) => readReport(top, p)));
+    const snapshot = takeSnapshot(config);
+    const views = resolveViews(config, snapshot);
+    if (options.replace) keepCurrent(config.stateDir);
+    const work = join(config.stateDir, WORK_DIR);
+    mkdirSync(work, { recursive: true });
+    const output = join(work, REPORT_FILE);
+    const bytes = Buffer.from(canonicalJSON(merged));
+    writeFileSync(output, bytes);
+    const deferrals = sortDeferrals(dedupeDeferrals([], options.from.map((p) => ({ reason: `seeded from ${p}`, paths: ['*'] })))).map((d) => ({ ...d, inherited: false }));
+    const attestation = buildAttestation({
+      config,
+      files: snapshot.files,
+      runtime: snapshot.runtime,
+      reportBytes: bytes,
+      report: merged,
+      mode: 'seed',
+      completedAt: new Date().toISOString(),
+      lastFullAt: null,
+      deferrals,
+      views,
+      summaries: viewSummaries(merged, views, deferrals),
+    });
+    commitState(config.stateDir, output, attestation);
+    io.stdout.write(`seed: score=${attestation.score} thresholdBreak=${attestation.thresholdBreak}\n`);
+    io.stderr.write(summaryLine({ command: 'seed', deferrals: deferrals.length, pendingFull: true }));
+    return 0;
+  } finally {
+    release();
+  }
+}
diff --git a/templates/mutation-incremental/lib/snapshot.mjs b/templates/mutation-incremental/lib/snapshot.mjs
index 47a5e9f..fc82b53 100644
--- a/templates/mutation-incremental/lib/snapshot.mjs
+++ b/templates/mutation-incremental/lib/snapshot.mjs
@@ -61,6 +61,10 @@ function configDigest(absPath) {
   return `sha256:${sha256(canonicalJSON(raw))}`;
 }
 
+// The digest a snapshot records for one path: the config file without its views (spec K3.4),
+// anything else by content or link text.
+export const fileDigest = (config, path) => (path === config.configRel ? configDigest(config.configPath) : digestOf(join(config.top, path)));
+
 export function takeSnapshot(config, { runCommand = defaultRunCommand } = {}) {
   const { paths, tracked } = listRepoPaths(config.top);
   const files = {};
@@ -68,7 +72,7 @@ export function takeSnapshot(config, { runCommand = defaultRunCommand } = {}) {
     if (matchList(path, config.exclusions)) continue;
     const category = categorize(path, config.lists);
     if (category === 'ignore') continue;
-    const digest = path === config.configRel ? configDigest(config.configPath) : digestOf(join(config.top, path));
+    const digest = fileDigest(config, path);
     if (digest === null) continue;
     files[path] = { digest, category, tracked: tracked.has(path) };
   }
diff --git a/templates/mutation-incremental/lib/state.mjs b/templates/mutation-incremental/lib/state.mjs
index 03ec2ec..c6968c1 100644
--- a/templates/mutation-incremental/lib/state.mjs
+++ b/templates/mutation-incremental/lib/state.mjs
@@ -100,3 +100,11 @@ export function adopt(candidates, snapshot) {
     .sort((x, y) => x.deferred - y.deferred || x.changes.length - y.changes.length || Number(y.c.primary) - Number(x.c.primary) || x.order - y.order);
   return ranked.length === 0 ? null : { ...ranked[0].c, changes: ranked[0].changes };
 }
+
+export const primaryCandidate = (config) => readCandidate(config.stateDir, { label: config.stateDirRaw, primary: true });
+
+// Spec §4: a pending full run is a state that is not usable, or one whose attestation has deferrals.
+export function canonicalPendingFull(config) {
+  const state = primaryCandidate(config);
+  return !state.usable || state.attestation.deferrals.length > 0;
+}
diff --git a/templates/mutation-incremental/lib/verify.mjs b/templates/mutation-incremental/lib/verify.mjs
new file mode 100644
index 0000000..a1fced6
--- /dev/null
+++ b/templates/mutation-incremental/lib/verify.mjs
@@ -0,0 +1,37 @@
+import { join } from 'node:path';
+import { spawnGroup } from './proc.mjs';
+import { ATTESTATION_FILE, REPORT_FILE } from './state.mjs';
+import { viewFiles } from './views.mjs';
+
+const VIEW_VARS = ['MUTATION_VIEW', 'MUTATION_VIEW_PATTERNS', 'MUTATION_VIEW_FILES'];
+
+// Spec §11.1: after state is committed, run the project's verifier once per view (once without
+// views), no shell, cwd = top-level, own process group. Any failure (non-zero exit, a command that
+// cannot be spawned, exceeding timeoutMinutes) is one returned line naming the view; the run then
+// exits 1. An interrupt stops the loop; the caller exits 130.
+export async function runVerify(config, { views, snapshot, pr, interrupt, graceMs, onOutput }) {
+  if (config.verify === null) return [];
+  const base = {
+    ...process.env,
+    MUTATION_REPORT: join(config.stateDir, REPORT_FILE),
+    MUTATION_ATTESTATION: join(config.stateDir, ATTESTATION_FILE),
+    MUTATION_RUN_KIND: pr ? 'pr' : 'other',
+  };
+  for (const name of VIEW_VARS) delete base[name];
+  const runs = views === null
+    ? [{ label: 'verify', env: base }]
+    : Object.entries(views).map(([name, patterns]) => ({
+      label: `verify (view ${name})`,
+      env: { ...base, MUTATION_VIEW: name, MUTATION_VIEW_PATTERNS: JSON.stringify(patterns), MUTATION_VIEW_FILES: JSON.stringify(viewFiles({ [name]: patterns }, snapshot)[name]) },
+    }));
+  const timeoutMs = config.verify.timeoutMinutes === null ? null : config.verify.timeoutMinutes * 60_000;
+  const failures = [];
+  for (const run of runs) {
+    const r = await spawnGroup(config.verify.command, { cwd: config.top, env: run.env, timeoutMs, graceMs, interrupt, onOutput });
+    if (r.interrupted) break;
+    if (r.error) failures.push(`${run.label}: cannot run ${config.verify.command[0]}: ${r.error.message}`);
+    else if (r.timedOut) failures.push(`${run.label}: exceeded verify.timeoutMinutes (${config.verify.timeoutMinutes})`);
+    else if (r.code !== 0) failures.push(`${run.label}: exit ${r.code}`);
+  }
+  return failures;
+}
diff --git a/templates/mutation-incremental/test/attest.test.mjs b/templates/mutation-incremental/test/attest.test.mjs
new file mode 100644
index 0000000..992d83c
--- /dev/null
+++ b/templates/mutation-incremental/test/attest.test.mjs
@@ -0,0 +1,87 @@
+import { test } from 'node:test';
+import assert from 'node:assert/strict';
+import { existsSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
+import { join } from 'node:path';
+import { loadConfig } from '../lib/config.mjs';
+import { sha256, takeSnapshot } from '../lib/snapshot.mjs';
+import { buildAttestation, CHANGED_DURING_RUN, commitState, installAdopted, markChangedDuringRun, mutationScore, WORK_DIR } from '../lib/attest.mjs';
+import { readCandidate, STATE_VERSION } from '../lib/state.mjs';
+import { makeRepo, writeState } from './helpers.mjs';
+
+const m = (status) => ({ status });
+const report = { files: { 'src/a.ts': { mutants: [m('Killed'), m('Survived')] } } };
+
+function fixture() {
+  const r = makeRepo({ files: { 'src/a.ts': 'a', 'src/b.ts': 'b' } });
+  const config = loadConfig(r.top);
+  return { r, config, snapshot: takeSnapshot(config) };
+}
+const build = (config, snapshot, extra = {}) => buildAttestation({
+  config, files: snapshot.files, runtime: snapshot.runtime, reportBytes: Buffer.from('{}'), report, mode: 'incremental',
+  completedAt: 'c', lastFullAt: 'l', deferrals: [], views: null, summaries: null, ...extra,
+});
+
+test('mutationScore follows Stryker: Timeout counts as detected, other statuses are ignored', () => {
+  assert.equal(mutationScore({ files: { a: { mutants: [m('Killed'), m('Timeout'), m('Survived'), m('NoCoverage'), m('CompileError'), m('Ignored')] } } }), 50);
+  assert.equal(mutationScore({ files: { a: { mutants: [m('CompileError')] }, b: {} } }), null);
+});
+
+test('buildAttestation writes the §5.5 contract', () => {
+  const { config, snapshot } = fixture();
+  const att = build(config, snapshot);
+  assert.deepEqual(Object.keys(att).sort(), [
+    'completedAt', 'deferrals', 'engine', 'engineVersion', 'exclusions', 'files', 'lastFullAt', 'lists', 'mode', 'report',
+    'reportSha256', 'runtime', 'schemaVersion', 'score', 'stateVersion', 'thresholdBreak', 'tool', 'toolVersion',
+  ]);
+  assert.deepEqual(
+    [att.schemaVersion, att.tool, att.toolVersion, att.stateVersion, att.engine, att.engineVersion, att.report, att.reportSha256, att.score],
+    [1, 'metareview-mutation-incremental', '0.13.0', STATE_VERSION, 'stryker', '10.0.0', 'incremental.json', sha256('{}'), 50],
+  );
+  assert.equal(att.thresholdBreak, false); // no thresholds.break configured
+  assert.deepEqual([att.files, att.lists, att.exclusions], [snapshot.files, config.lists, config.exclusions]);
+});
+
+test('thresholdBreak: strictly below the break, never for a null score', () => {
+  const { config, snapshot } = fixture();
+  assert.equal(build({ ...config, thresholdBreak: 60 }, snapshot).thresholdBreak, true);
+  assert.equal(build({ ...config, thresholdBreak: 50 }, snapshot).thresholdBreak, false);
+  assert.equal(build({ ...config, thresholdBreak: 60 }, snapshot, { report: { files: {} } }).thresholdBreak, false);
+});
+
+test('views and viewSummaries are written only with views', () => {
+  const { config, snapshot } = fixture();
+  const att = build(config, snapshot, { views: { core: ['src/**'] }, summaries: { core: { Killed: 1 } } });
+  assert.deepEqual([att.views, att.viewSummaries], [{ core: ['src/**'] }, { core: { Killed: 1 } }]);
+});
+
+test('markChangedDuringRun flags edited and deleted files and keeps the rest', () => {
+  const { r, config, snapshot } = fixture();
+  r.write('src/a.ts', 'changed');
+  rmSync(join(r.top, 'src/b.ts'));
+  const files = markChangedDuringRun(config, snapshot.files);
+  assert.deepEqual([files['src/a.ts'].digest, files['src/b.ts'].digest], [CHANGED_DURING_RUN, CHANGED_DURING_RUN]);
+  assert.deepEqual(files['mutation-incremental.json'], snapshot.files['mutation-incremental.json']);
+  assert.deepEqual(files['stryker.config.json'], snapshot.files['stryker.config.json']);
+});
+
+test('commitState renames the output and then the attestation into place and removes work/', () => {
+  const { config, snapshot } = fixture();
+  const work = join(config.stateDir, WORK_DIR);
+  mkdirSync(work, { recursive: true });
+  const bytes = Buffer.from(JSON.stringify(report));
+  writeFileSync(join(work, 'inv-2.json'), bytes);
+  commitState(config.stateDir, join(work, 'inv-2.json'), build(config, snapshot, { reportBytes: bytes }));
+  assert.equal(readCandidate(config.stateDir, { label: 's', primary: true }).usable, true);
+  assert.equal(existsSync(work), false);
+  // An unchanged canonical output is only re-attested.
+  commitState(config.stateDir, join(config.stateDir, 'incremental.json'), build(config, snapshot, { reportBytes: bytes, mode: 'full' }));
+  assert.equal(JSON.parse(readFileSync(join(config.stateDir, 'attestation.json'), 'utf8')).mode, 'full');
+  assert.equal(readCandidate(config.stateDir, { label: 's', primary: true }).usable, true);
+});
+
+test('installAdopted copies both files of the adopted state into stateDir', () => {
+  const { r, config } = fixture();
+  writeState(join(r.top, '.mutation/remote/full'), { report });
+  installAdopted(config.stateDir, join(r.top, '.mutation/remote/full'));
+  assert.equal(readCandidate(config.stateDir, { label: 's', primary: true }).usable, true);
+});
diff --git a/templates/mutation-incremental/test/engine.test.mjs b/templates/mutation-incremental/test/engine.test.mjs
new file mode 100644
index 0000000..355e64b
--- /dev/null
+++ b/templates/mutation-incremental/test/engine.test.mjs
@@ -0,0 +1,89 @@
+import { test } from 'node:test';
+import assert from 'node:assert/strict';
+import { mkdirSync, writeFileSync } from 'node:fs';
+import { join } from 'node:path';
+import { loadConfig } from '../lib/config.mjs';
+import { invoke, NO_TESTS, parseReport, strykerArgv } from '../lib/engine.mjs';
+import { createInterrupt } from '../lib/proc.mjs';
+import { reportFor, runRepo } from './fake.mjs';
+
+function setup(steps, extra = {}) {
+  const r = runRepo({ steps, ...extra });
+  const config = loadConfig(r.top);
+  mkdirSync(join(config.stateDir, 'work'), { recursive: true });
+  return { r, config };
+}
+const call = (config, over = {}) => invoke(config, {
+  index: 1, input: null, force: false, mutate: null, timeoutMs: null, interrupt: createInterrupt(), graceMs: 200, onOutput: () => {}, ...over,
+});
+
+test('strykerArgv builds the invocation without a shell', () => {
+  const config = { stryker: { command: ['npx', 'stryker'], configFile: 's.json', extraArgs: ['--concurrency', '2'] } };
+  assert.deepEqual(strykerArgv(config, '.m/work/inv-2.json', { force: true, mutate: ['a.ts', 'b.ts:1-4'] }),
+    ['npx', 'stryker', 'run', 's.json', '--incremental', '--incrementalFile', '.m/work/inv-2.json', '--force', '--mutate', 'a.ts,b.ts:1-4', '--concurrency', '2']);
+  assert.deepEqual(strykerArgv(config, 'f.json', { force: false, mutate: null }),
+    ['npx', 'stryker', 'run', 's.json', '--incremental', '--incrementalFile', 'f.json', '--concurrency', '2']);
+});
+
+test('parseReport accepts only an object with a files object', () => {
+  for (const bad of ['x', 'null', '[]', '{}', '{"files":[]}', '{"files":null}', '{"files":5}']) assert.equal(parseReport(Buffer.from(bad)), null, bad);
+  assert.deepEqual(parseReport(Buffer.from('{"files":{}}')), { files: {} });
+});
+
+test('a written, parseable output with exit 0 or 1 is a success', async () => {
+  for (const exit of [0, 1]) {
+    const { config } = setup([{ report: reportFor(), exit }]);
+    const r = await call(config);
+    assert.equal(r.outcome, 'success');
+    assert.equal(r.path, join(config.stateDir, 'work/inv-1.json'));
+    assert.deepEqual(r.report, reportFor());
+    assert.equal(r.bytes.toString(), JSON.stringify(reportFor()));
+  }
+});
+
+test('the input is copied to work/inv-<k>.json first; rewriting it counts as written', async () => {
+  const { r, config } = setup([{ report: 'input' }]);
+  const input = join(config.stateDir, 'incremental.json');
+  writeFileSync(input, JSON.stringify(reportFor()));
+  const res = await call(config, { index: 2, input, force: true, mutate: ['src/a.ts:2-3'] });
+  assert.equal(res.outcome, 'success');
+  assert.deepEqual(r.calls(), [['run', 'stryker.config.json', '--incremental', '--incrementalFile', '.mutation/work/inv-2.json', '--force', '--mutate', 'src/a.ts:2-3']]);
+});
+
+test('exit 1 with nothing written and "No tests were executed" is no-tests; output is forwarded', async () => {
+  const { config } = setup([{ exit: 1, output: `INFO ${NO_TESTS}. Stryker will exit prematurely.\n` }]);
+  let seen = '';
+  const r = await call(config, { onOutput: (c) => { seen += c; } });
+  assert.equal(r.outcome, 'no-tests');
+  assert.match(seen, /No tests were executed/);
+});
+
+test('an unwritten output, an unparseable one or another exit code is a failure', async () => {
+  const cases = [
+    [{ exit: 1 }, /exit 1/],
+    [{ exit: 0 }, /exit 0/],
+    [{ exit: 2, report: reportFor() }, /exit 2/],
+    [{ exit: 0, report: [1] }, /exit 0/],
+  ];
+  for (const [step, detail] of cases) {
+    const { config } = setup([step]);
+    const r = await call(config);
+    assert.equal(r.outcome, 'failed', JSON.stringify(step));
+    assert.match(r.detail, detail);
+  }
+});
+
+test('a timeout and an interrupt are reported as such', async () => {
+  const { config } = setup([{ sleepMs: 10000 }]);
+  assert.equal((await call(config, { timeoutMs: 400 })).outcome, 'timeout');
+  const interrupt = createInterrupt();
+  setTimeout(() => interrupt.trigger(), 400);
+  assert.equal((await call(config, { interrupt })).outcome, 'interrupted');
+});
+
+test('a Stryker command that cannot be spawned is a failure naming the error', async () => {
+  const { config } = setup([{}], { config: { stryker: { command: ['/nonexistent/stryker'], configFile: 'stryker.config.json', extraArgs: [] } } });
+  const r = await call(config);
+  assert.equal(r.outcome, 'failed');
+  assert.match(r.detail, /ENOENT/);
+});
diff --git a/templates/mutation-incremental/test/fake-stryker.mjs b/templates/mutation-incremental/test/fake-stryker.mjs
new file mode 100644
index 0000000..86ff390
--- /dev/null
+++ b/templates/mutation-incremental/test/fake-stryker.mjs
@@ -0,0 +1,21 @@
+// Test double for `stryker run` (spec F4–F6), driven by .fake/steps.json in the cwd: one step per
+// invocation (the last step repeats). A step may print `output`, write `touch` files, sleep
+// `sleepMs` (ignoring SIGTERM with `ignoreTerm`), write `report` (an object, or "input" to rewrite
+// the incremental file's own bytes) and exit with `exit` (default 0). Each call's argv is appended
+// to .fake/calls.jsonl.
+import { appendFileSync, existsSync, readFileSync, writeFileSync } from 'node:fs';
+
+const argv = process.argv.slice(2);
+const log = '.fake/calls.jsonl';
+const n = existsSync(log) ? readFileSync(log, 'utf8').split('\n').filter(Boolean).length : 0;
+appendFileSync(log, `${JSON.stringify(argv)}\n`);
+const steps = JSON.parse(readFileSync('.fake/steps.json', 'utf8'));
+const step = steps[Math.min(n, steps.length - 1)];
+const file = argv[argv.indexOf('--incrementalFile') + 1];
+if (step.ignoreTerm) process.on('SIGTERM', () => {});
+if (step.output) process.stdout.write(step.output);
+for (const [path, text] of Object.entries(step.touch ?? {})) writeFileSync(path, text);
+await new Promise((resolve) => setTimeout(resolve, step.sleepMs ?? 0));
+if (step.report === 'input') writeFileSync(file, readFileSync(file));
+else if (step.report) writeFileSync(file, JSON.stringify(step.report));
+process.exit(step.exit ?? 0);
diff --git a/templates/mutation-incremental/test/fake.mjs b/templates/mutation-incremental/test/fake.mjs
new file mode 100644
index 0000000..104149d
--- /dev/null
+++ b/templates/mutation-incremental/test/fake.mjs
@@ -0,0 +1,75 @@
+import { existsSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
+import { join } from 'node:path';
+import { main } from '../lib/main.mjs';
+import { makeRepo } from './helpers.mjs';
+
+const FAKE = readFileSync(new URL('./fake-stryker.mjs', import.meta.url), 'utf8');
+
+// src/a.ts: line 2 has a kill (by tests/a.test.ts), line 3 a survivor it covers.
+export const A = 'export function clamp(n, lo, hi) {\n  if (n < lo) return lo;\n  if (n > hi) return hi;\n  return n;\n}\n';
+const mutant = (id, line, status, killedBy) => ({
+  id, mutatorName: 'ConditionalExpression', replacement: 'true', status, killedBy, coveredBy: ['t1'],
+  location: { start: { line, column: 7 }, end: { line, column: 13 } },
+});
+export function reportFor(source = A) {
+  return {
+    schemaVersion: '1',
+    thresholds: { high: 80, low: 60 },
+    files: { 'src/a.ts': { language: 'typescript', source, mutants: [mutant('1', 2, 'Killed', ['t1']), mutant('2', 3, 'Survived', [])] } },
+    testFiles: { 'tests/a.test.ts': { tests: [{ id: 't1', name: 'clamps low' }] } },
+  };
+}
+
+// A throwaway repository whose Stryker is the fake. .fake/ (the fake, its steps, its call log and
+// GITHUB_OUTPUT) is gitignored, so it never enters a snapshot.
+export function runRepo({ config = {}, stryker = {}, files = {}, steps = [{ report: reportFor() }] } = {}) {
+  const r = makeRepo({
+    config: { stryker: { command: [process.execPath, '.fake/stryker.mjs'], configFile: 'stryker.config.json', extraArgs: [] }, ...config },
+    stryker,
+    files: {
+      '.gitignore': 'node_modules/\n.mutation/\n.fake/\n',
+      '.fake/stryker.mjs': FAKE,
+      'src/a.ts': A,
+      'tests/a.test.ts': "import { clamp } from '../src/a';\n",
+      ...files,
+    },
+  });
+  const log = join(r.top, '.fake/calls.jsonl');
+  // Replacing the steps also restarts the call log, so step 0 is the next invocation.
+  r.steps = (list) => {
+    writeFileSync(join(r.top, '.fake/steps.json'), JSON.stringify(list));
+    rmSync(log, { force: true });
+  };
+  r.calls = () => (existsSync(log) ? readFileSync(log, 'utf8').split('\n').filter(Boolean).map((l) => JSON.parse(l)) : []);
+  r.read = (rel) => JSON.parse(readFileSync(join(r.top, rel), 'utf8'));
+  r.steps(steps);
+  return r;
+}
+
+const capture = () => ({ text: '', write(s) { this.text += String(s); } });
+
+// Runs the CLI in-process with GITHUB_OUTPUT pointed at .fake/output, returned parsed.
+export async function cli(r, args, { killGraceMs = 200 } = {}) {
+  const stdout = capture();
+  const stderr = capture();
+  const outFile = join(r.top, '.fake/output');
+  writeFileSync(outFile, '');
+  const saved = process.env.GITHUB_OUTPUT;
+  process.env.GITHUB_OUTPUT = outFile;
+  try {
+    const code = await main(args, { stdout, stderr, cwd: r.top, killGraceMs });
+    const lines = readFileSync(outFile, 'utf8').split('\n').filter(Boolean);
+    const output = Object.fromEntries(lines.map((l) => [l.slice(0, l.indexOf('=')), l.slice(l.indexOf('=') + 1)]));
+    return { code, stdout: stdout.text, stderr: stderr.text, output };
+  } finally {
+    if (saved === undefined) delete process.env.GITHUB_OUTPUT;
+    else process.env.GITHUB_OUTPUT = saved;
+  }
+}
+
+// A full run that leaves a usable state; later invocations echo their input file.
+export async function warm(r) {
+  const res = await cli(r, ['run', '--mode', 'full']);
+  if (res.code !== 0) throw new Error(`warm-up full run failed (${res.code}): ${res.stderr}`);
+  r.steps([{ report: 'input' }]);
+}
diff --git a/templates/mutation-incremental/test/json.test.mjs b/templates/mutation-incremental/test/json.test.mjs
index f5c98ce..3f68f43 100644
--- a/templates/mutation-incremental/test/json.test.mjs
+++ b/templates/mutation-incremental/test/json.test.mjs
@@ -3,6 +3,7 @@ import assert from 'node:assert/strict';
 import { canonicalJSON } from '../lib/json.mjs';
 import { UsageError, LockHeldError, EngineError } from '../lib/errors.mjs';
 import { duplicateKey } from '../lib/json.mjs';
+import { InterruptedError } from '../lib/errors.mjs';
 
 test('canonicalJSON sorts keys recursively and ends with a newline', () => {
   const out = canonicalJSON({ b: 1, a: { d: [{ z: 1, y: 2 }], c: null } });
@@ -22,3 +23,8 @@ test('duplicateKey finds a repeated key at any depth and ignores keys in arrays
   assert.equal(duplicateKey('"just a string"'), null);
   assert.equal(duplicateKey('{"x\\u0041":1,"xA":2}'), 'xA');
 });
+
+
+test('InterruptedError is exit 130', () => {
+  assert.equal(new InterruptedError('x').exitCode, 130);
+});
diff --git a/templates/mutation-incremental/test/lock.test.mjs b/templates/mutation-incremental/test/lock.test.mjs
new file mode 100644
index 0000000..589538c
--- /dev/null
+++ b/templates/mutation-incremental/test/lock.test.mjs
@@ -0,0 +1,78 @@
+import { test } from 'node:test';
+import assert from 'node:assert/strict';
+import { existsSync, mkdirSync, mkdtempSync, readFileSync, writeFileSync } from 'node:fs';
+import { spawnSync } from 'node:child_process';
+import { hostname, tmpdir } from 'node:os';
+import { join } from 'node:path';
+import { acquireLock, LOCK_FILE, pidAlive } from '../lib/lock.mjs';
+import { main } from '../lib/main.mjs';
+import { makeRepo } from './helpers.mjs';
+
+const stateDir = () => mkdtempSync(join(tmpdir(), 'mi-lock-'));
+const deadPid = () => spawnSync(process.execPath, ['-e', '']).pid;
+const now = new Date('2026-09-23T18:00:00.000Z');
+const capture = () => ({ text: '', write(s) { this.text += s; } });
+
+test('acquireLock writes pid, hostname and startedAt; release removes it', () => {
+  const dir = stateDir();
+  const release = acquireLock(dir, { pid: 42, host: 'h', now });
+  assert.deepEqual(JSON.parse(readFileSync(join(dir, LOCK_FILE), 'utf8')), { pid: 42, hostname: 'h', startedAt: '2026-09-23T18:00:00.000Z' });
+  release();
+  assert.equal(existsSync(join(dir, LOCK_FILE)), false);
+  const own = stateDir();
+  const releaseOwn = acquireLock(own);
+  assert.deepEqual(Object.keys(JSON.parse(readFileSync(join(own, LOCK_FILE), 'utf8'))), ['pid', 'hostname', 'startedAt']);
+  assert.equal(JSON.parse(readFileSync(join(own, LOCK_FILE), 'utf8')).pid, process.pid);
+  releaseOwn();
+});
+
+test('a lock held by a live process on this host is exit 3 naming it', () => {
+  const dir = stateDir();
+  acquireLock(dir, { pid: process.pid, host: hostname(), now });
+  assert.throws(() => acquireLock(dir, { pid: 7, host: hostname(), now }), (e) => e.exitCode === 3 && e.message.includes(`pid ${process.pid}`) && /break-lock/.test(e.message));
+});
+
+test('a lock left by a dead process on this host is replaced', () => {
+  const dir = stateDir();
+  acquireLock(dir, { pid: deadPid(), host: hostname(), now });
+  acquireLock(dir, { pid: 7, host: hostname(), now });
+  assert.equal(JSON.parse(readFileSync(join(dir, LOCK_FILE), 'utf8')).pid, 7);
+});
+
+test('a lock from another machine or an unreadable lock is held', () => {
+  const dir = stateDir();
+  acquireLock(dir, { pid: deadPid(), host: 'other-machine', now });
+  assert.throws(() => acquireLock(dir, { pid: 7, host: hostname(), now }), (e) => e.exitCode === 3 && /other-machine/.test(e.message));
+  const junk = stateDir();
+  writeFileSync(join(junk, LOCK_FILE), '');
+  assert.throws(() => acquireLock(junk, { pid: 7, host: hostname(), now }), (e) => e.exitCode === 3 && /unreadable lock/.test(e.message));
+});
+
+test('acquireLock rethrows anything but an existing lock', () => {
+  assert.throws(() => acquireLock(join(stateDir(), 'missing'), { pid: 7, host: 'h', now }), (e) => e.code === 'ENOENT');
+});
+
+test('pidAlive', () => {
+  assert.equal(pidAlive(process.pid), true);
+  assert.equal(pidAlive(deadPid()), false);
+  assert.equal(pidAlive(1), true); // init/launchd: EPERM for a normal user, alive either way
+});
+
+test('break-lock prints and removes the lock, or says there is none', async () => {
+  const r = makeRepo();
+  const run = async () => {
+    const stdout = capture();
+    const stderr = capture();
+    const code = await main(['break-lock'], { stdout, stderr, cwd: r.top });
+    return { code, stdout: stdout.text, stderr: stderr.text };
+  };
+  const none = await run();
+  assert.equal(none.code, 0);
+  assert.match(none.stdout, /no lock at .*\.mutation\/lock/);
+  assert.match(none.stderr, /command=break-lock invocations=0 scope=0 forced=0 deferrals=0 pending_full=true\n$/);
+  mkdirSync(join(r.top, '.mutation'), { recursive: true });
+  writeFileSync(join(r.top, '.mutation/lock'), '{"pid":1,"hostname":"ci","startedAt":"x"}\n');
+  const held = await run();
+  assert.match(held.stdout, /\{"pid":1,"hostname":"ci","startedAt":"x"\}\nbreak-lock: removed /);
+  assert.equal(existsSync(join(r.top, '.mutation/lock')), false);
+});
diff --git a/templates/mutation-incremental/test/main.test.mjs b/templates/mutation-incremental/test/main.test.mjs
index 59c9069..68535d7 100644
--- a/templates/mutation-incremental/test/main.test.mjs
+++ b/templates/mutation-incremental/test/main.test.mjs
@@ -8,6 +8,7 @@ import { nodeVersionWarning } from '../lib/nodever.mjs';
 import { loadConfig } from '../lib/config.mjs';
 import { takeSnapshot } from '../lib/snapshot.mjs';
 import { makeRepo, writeState } from './helpers.mjs';
+import { loadHarnessConfig } from '../lib/inputs.mjs';
 
 const capture = () => {
   const out = { text: '', write(s) { this.text += s; } };
@@ -21,7 +22,14 @@ async function run(args, cwd) {
 }
 
 test('parseArgs', () => {
-  assert.deepEqual(parseArgs(['plan', '--also-state', 'a', '--config', 'c.json', '--also-state', 'b']), { command: 'plan', config: 'c.json', alsoState: ['a', 'b'], rest: [] });
+  assert.deepEqual(parseArgs(['plan', '--also-state', 'a', '--config', 'c.json', '--also-state', 'b']), {
+    command: 'plan', config: 'c.json', mode: undefined, maxMinutes: undefined, remote: undefined, kind: undefined,
+    alsoState: ['a', 'b'], from: [], pr: false, replace: false,
+  });
+  assert.deepEqual(parseArgs(['run', '--mode', 'incremental', '--pr', '--max-minutes', '60', '--replace', '--from', 'r1', '--from', 'r2', '--remote', 'up', '--kind', 'inc']), {
+    command: 'run', config: undefined, mode: 'incremental', maxMinutes: '60', remote: 'up', kind: 'inc',
+    alsoState: [], from: ['r1', 'r2'], pr: true, replace: true,
+  });
   assert.throws(() => parseArgs(['plan', '--config']), (e) => e.exitCode === 2);
   assert.throws(() => parseArgs(['plan', '--bogus']), (e) => e.exitCode === 2);
   assert.deepEqual(parseArgs([]).command, undefined);
@@ -100,6 +108,15 @@ test('nodeVersionWarning: prefix match on components, aliases skipped, first fil
   assert.equal(nodeVersionWarning(makeRepo().top, 'v22.9.0'), null);
 });
 
+test('loadHarnessConfig prints the Node-version warning only when asked', () => {
+  const r = makeRepo({ files: { '.nvmrc': '1.2\n' } });
+  const err = capture();
+  loadHarnessConfig({ cwd: r.top, stderr: err }, {}, false);
+  assert.equal(err.text, '');
+  loadHarnessConfig({ cwd: r.top, stderr: err }, {}, true);
+  assert.match(err.text, /pins Node 1\.2/);
+});
+
 test('cli.mjs runs main with process.argv', async () => {
   const saved = process.argv;
   process.argv = [process.execPath, 'cli.mjs', '--bogus'];
diff --git a/templates/mutation-incremental/test/proc.test.mjs b/templates/mutation-incremental/test/proc.test.mjs
new file mode 100644
index 0000000..8cbaf9b
--- /dev/null
+++ b/templates/mutation-incremental/test/proc.test.mjs
@@ -0,0 +1,71 @@
+import { test } from 'node:test';
+import assert from 'node:assert/strict';
+import { existsSync, mkdtempSync } from 'node:fs';
+import { spawnSync } from 'node:child_process';
+import { tmpdir } from 'node:os';
+import { join } from 'node:path';
+import { createInterrupt, installSignalHandlers, KILL_GRACE_MS, signalGroup, spawnGroup } from '../lib/proc.mjs';
+
+const node = (script) => [process.execPath, '-e', script];
+const opts = (extra) => ({ cwd: tmpdir(), env: process.env, timeoutMs: null, graceMs: 200, interrupt: createInterrupt(), onOutput: () => {}, ...extra });
+
+test('spawnGroup reports the exit code and forwards both output streams', async () => {
+  let out = '';
+  const r = await spawnGroup(node("process.stdout.write('o'); process.stderr.write('e'); process.exit(3)"), opts({ onOutput: (c) => { out += c; } }));
+  assert.deepEqual([r.code, r.timedOut, r.interrupted, r.error], [3, false, false, null]);
+  assert.deepEqual([...out].sort(), ['e', 'o']);
+  assert.equal(KILL_GRACE_MS, 30_000);
+});
+
+test('spawnGroup passes the given environment', async () => {
+  const r = await spawnGroup(node("process.exit(process.env.MI_X === 'y' ? 0 : 5)"), opts({ env: { ...process.env, MI_X: 'y' } }));
+  assert.equal(r.code, 0);
+});
+
+test('a timeout terminates the whole process group', async () => {
+  const dir = mkdtempSync(join(tmpdir(), 'mi-proc-'));
+  const late = join(dir, 'late');
+  const grandchild = `setTimeout(() => require("fs").writeFileSync(${JSON.stringify(late)}, "x"), 1000)`;
+  const script = `require('child_process').spawn(process.execPath, ['-e', ${JSON.stringify(grandchild)}], { stdio: 'ignore' }); setTimeout(() => {}, 20000);`;
+  const r = await spawnGroup(node(script), opts({ cwd: dir, timeoutMs: 400 }));
+  assert.equal(r.timedOut, true);
+  await new Promise((res) => setTimeout(res, 1300));
+  assert.equal(existsSync(late), false);
+});
+
+test('a child that ignores SIGTERM is killed after the grace period', async () => {
+  const r = await spawnGroup(node("process.on('SIGTERM', () => {}); setTimeout(() => {}, 20000);"), opts({ timeoutMs: 400, graceMs: 200 }));
+  assert.deepEqual([r.timedOut, r.signal], [true, 'SIGKILL']);
+});
+
+test('an interrupt stops the child; after an interrupt nothing new is spawned', async () => {
+  const interrupt = createInterrupt();
+  setTimeout(() => interrupt.trigger(), 300);
+  const r = await spawnGroup(node('setTimeout(() => {}, 20000)'), opts({ interrupt }));
+  assert.equal(r.interrupted, true);
+  const dir = mkdtempSync(join(tmpdir(), 'mi-proc-'));
+  const again = await spawnGroup(node("require('fs').writeFileSync('ran', 'x')"), opts({ interrupt, cwd: dir }));
+  assert.equal(again.interrupted, true);
+  assert.equal(existsSync(join(dir, 'ran')), false);
+});
+
+test('a command that cannot be spawned reports the error', async () => {
+  const r = await spawnGroup(['/nonexistent/mi-command'], opts({}));
+  assert.equal(r.code, null);
+  assert.match(r.error.message, /ENOENT/);
+});
+
+test('signalGroup is false for a group that does not exist', () => {
+  const { pid } = spawnSync(process.execPath, ['-e', '']);
+  assert.equal(signalGroup(pid, 'SIGTERM'), false);
+});
+
+test('installSignalHandlers routes signals to the interrupt until removed', () => {
+  const interrupt = createInterrupt();
+  const before = process.listenerCount('SIGHUP');
+  const uninstall = installSignalHandlers(interrupt);
+  process.emit('SIGHUP');
+  assert.equal(interrupt.interrupted, true);
+  uninstall();
+  assert.equal(process.listenerCount('SIGHUP'), before);
+});
diff --git a/templates/mutation-incremental/test/remote.test.mjs b/templates/mutation-incremental/test/remote.test.mjs
new file mode 100644
index 0000000..787c760
--- /dev/null
+++ b/templates/mutation-incremental/test/remote.test.mjs
@@ -0,0 +1,136 @@
+import { test } from 'node:test';
+import assert from 'node:assert/strict';
+import { execFileSync } from 'node:child_process';
+import { chmodSync, existsSync, mkdirSync, mkdtempSync, readFileSync, writeFileSync } from 'node:fs';
+import { tmpdir } from 'node:os';
+import { join } from 'node:path';
+import { authEnv } from '../lib/remote.mjs';
+import { readCandidate } from '../lib/state.mjs';
+import { cli, reportFor, runRepo, warm } from './fake.mjs';
+
+function bareRemote(r) {
+  const bare = mkdtempSync(join(tmpdir(), 'mi-remote-'));
+  execFileSync('git', ['init', '-q', '--bare', bare]);
+  r.git('remote', 'add', 'origin', bare);
+  return bare;
+}
+const usable = (dir) => readCandidate(dir, { label: 'x', primary: false }).usable;
+
+test('authEnv passes the token as an extraheader after existing GIT_CONFIG entries', () => {
+  assert.deepEqual(authEnv({ A: '1' }, 'https://h/r.git', ''), { env: { A: '1' }, mask: null });
+  assert.deepEqual(authEnv({ A: '1' }, 'https://h/r.git', undefined), { env: { A: '1' }, mask: null });
+  const b64 = Buffer.from('x-access-token:tok').toString('base64');
+  assert.deepEqual(authEnv({}, 'https://h/r.git', 'tok'), {
+    env: { GIT_CONFIG_COUNT: '1', GIT_CONFIG_KEY_0: 'http.https://h/r.git.extraheader', GIT_CONFIG_VALUE_0: `AUTHORIZATION: basic ${b64}` },
+    mask: b64,
+  });
+  const more = authEnv({ GIT_CONFIG_COUNT: '2' }, 'u', 'tok').env;
+  assert.deepEqual([more.GIT_CONFIG_COUNT, more.GIT_CONFIG_KEY_2], ['3', 'http.u.extraheader']);
+});
+
+test('publish-state pushes a parentless commit that fetch-state reads back in another checkout', async () => {
+  const r = runRepo();
+  await warm(r);
+  const bare = bareRemote(r);
+  const pub = await cli(r, ['publish-state', '--kind', 'full']);
+  assert.equal(pub.code, 0);
+  assert.match(pub.stdout, /^publish-state: pushed [0-9a-f]{40} to refs\/heads\/mutation-state\/full\n$/);
+  assert.match(pub.stderr, /command=publish-state invocations=0 scope=0 forced=0 deferrals=0 pending_full=false\n$/);
+  const head = execFileSync('git', ['log', '-1', '--format=%P|%an <%ae>|%cn', 'refs/heads/mutation-state/full'], { cwd: bare, encoding: 'utf8' }).trim();
+  assert.equal(head, '|mutation-incremental <noreply@localhost>|mutation-incremental');
+
+  const other = runRepo();
+  other.git('remote', 'add', 'origin', bare);
+  const got = await cli(other, ['fetch-state']);
+  assert.equal(got.code, 0);
+  assert.match(got.stderr, /refs\/heads\/mutation-state\/inc is not on origin; skipped/);
+  assert.equal(readFileSync(join(other.top, '.mutation/remote/full/incremental.json'), 'utf8'), readFileSync(join(r.top, '.mutation/incremental.json'), 'utf8'));
+  assert.equal(usable(join(other.top, '.mutation/remote/full')), true);
+  // The other checkout adopts it; its files are identical, so there is nothing to run.
+  const run = await cli(other, ['run', '--mode', 'incremental', '--also-state', '.mutation/remote/full']);
+  assert.equal(run.code, 0);
+  assert.deepEqual(other.calls(), []);
+  assert.equal(usable(join(other.top, '.mutation')), true);
+});
+
+test('fetch-state reads a state larger than 1 MiB (reports embed every source file)', async () => {
+  const r = runRepo({ steps: [{ report: { ...reportFor(), padding: 'x'.repeat(1_500_000) } }] });
+  assert.equal((await cli(r, ['run', '--mode', 'full'])).code, 0);
+  const bare = bareRemote(r);
+  assert.equal((await cli(r, ['publish-state', '--kind', 'full'])).code, 0);
+  const other = runRepo();
+  other.git('remote', 'add', 'origin', bare);
+  const got = await cli(other, ['fetch-state']);
+  assert.equal(got.code, 0, got.stderr);
+  assert.equal(readFileSync(join(other.top, '.mutation/remote/full/incremental.json'), 'utf8'), readFileSync(join(r.top, '.mutation/incremental.json'), 'utf8'));
+});
+
+test('fetch-state removes a copy whose branch is gone', async () => {
+  const r = runRepo();
+  bareRemote(r);
+  mkdirSync(join(r.top, '.mutation/remote/inc'), { recursive: true });
+  writeFileSync(join(r.top, '.mutation/remote/inc/attestation.json'), '{}');
+  const res = await cli(r, ['fetch-state']);
+  assert.equal(res.code, 0);
+  assert.equal(existsSync(join(r.top, '.mutation/remote/inc')), false);
+  assert.match(res.stderr, /command=fetch-state invocations=0 scope=0 forced=0 deferrals=0 pending_full=true\n$/);
+});
+
+test('an unreachable or unknown remote is exit 2', async () => {
+  const r = runRepo();
+  r.git('remote', 'add', 'origin', join(tmpdir(), 'mi-no-such-remote.git'));
+  const unreachable = await cli(r, ['fetch-state']);
+  assert.equal(unreachable.code, 2);
+  assert.match(unreachable.stderr, /fetch-state: git ls-remote origin failed/);
+  const unknown = await cli(r, ['fetch-state', '--remote', 'nope']);
+  assert.equal(unknown.code, 2);
+  assert.match(unknown.stderr, /unknown git remote nope/);
+});
+
+test('a state branch without both files is exit 2', async () => {
+  const r = runRepo();
+  bareRemote(r);
+  const git = (args, input) => execFileSync('git', args, { cwd: r.top, input, encoding: 'utf8' }).trim();
+  const blob = git(['hash-object', '-w', '--stdin'], '{}');
+  const tree = git(['mktree'], `100644 blob ${blob}\tattestation.json\n`);
+  const commit = git(['commit-tree', tree, '-m', 'x']);
+  r.git('push', '-q', 'origin', `${commit}:refs/heads/mutation-state/inc`);
+  const res = await cli(r, ['fetch-state']);
+  assert.equal(res.code, 2);
+  assert.match(res.stderr, /fetch-state: reading incremental\.json from refs\/heads\/mutation-state\/inc failed/);
+});
+
+test('publish-state without usable state publishes nothing; a rejected push or a missing kind is exit 2', async () => {
+  const r = runRepo();
+  const bare = bareRemote(r);
+  const none = await cli(r, ['publish-state', '--kind', 'inc']);
+  assert.equal(none.code, 0);
+  assert.match(none.stdout, /canonical state is not usable \(missing attestation\); nothing published/);
+  await warm(r);
+  writeFileSync(join(bare, 'hooks/pre-receive'), '#!/bin/sh\necho "refs are protected" >&2\nexit 1\n');
+  chmodSync(join(bare, 'hooks/pre-receive'), 0o755);
+  const rejected = await cli(r, ['publish-state', '--kind', 'inc']);
+  assert.equal(rejected.code, 2);
+  assert.match(rejected.stderr, /publish-state: push to origin failed: [\s\S]*refs are protected/);
+  const noKind = await cli(r, ['publish-state']);
+  assert.equal(noKind.code, 2);
+  assert.match(noKind.stderr, /publish-state needs --kind inc\|full/);
+});
+
+test('with MUTATION_STATE_TOKEN under GitHub Actions the header value is masked first', async () => {
+  const r = runRepo();
+  bareRemote(r);
+  const saved = { token: process.env.MUTATION_STATE_TOKEN, actions: process.env.GITHUB_ACTIONS };
+  process.env.MUTATION_STATE_TOKEN = 'tok';
+  process.env.GITHUB_ACTIONS = 'true';
+  try {
+    const res = await cli(r, ['fetch-state']);
+    assert.equal(res.code, 0);
+    assert.equal(res.stdout, `::add-mask::${Buffer.from('x-access-token:tok').toString('base64')}\n`);
+  } finally {
+    for (const [name, value] of [['MUTATION_STATE_TOKEN', saved.token], ['GITHUB_ACTIONS', saved.actions]]) {
+      if (value === undefined) delete process.env[name];
+      else process.env[name] = value;
+    }
+  }
+});
diff --git a/templates/mutation-incremental/test/run.test.mjs b/templates/mutation-incremental/test/run.test.mjs
new file mode 100644
index 0000000..4b6d025
--- /dev/null
+++ b/templates/mutation-incremental/test/run.test.mjs
@@ -0,0 +1,306 @@
+import { test } from 'node:test';
+import assert from 'node:assert/strict';
+import { existsSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
+import { hostname } from 'node:os';
+import { join } from 'node:path';
+import { main } from '../lib/main.mjs';
+import { causesJSON, MAX_CAUSES } from '../lib/run.mjs';
+import { readCandidate, STATE_VERSION } from '../lib/state.mjs';
+import { sha256 } from '../lib/snapshot.mjs';
+import { writeState } from './helpers.mjs';
+import { A, cli, reportFor, runRepo, warm } from './fake.mjs';
+
+const A3 = A.replace('n > hi', 'n >= hi');
+const ROUTED = { pendingOnPr: 'full-on-global' };
+const usable = (r) => readCandidate(join(r.top, '.mutation'), { label: '.mutation', primary: true }).usable;
+const attestationText = (r) => readFileSync(join(r.top, '.mutation/attestation.json'), 'utf8');
+const inv = (k, ...rest) => ['run', 'stryker.config.json', '--incremental', '--incrementalFile', `.mutation/work/inv-${k}.json`, ...rest];
+
+// Main's published state: this repo's own state with the given deferrals.
+function mainState(r, deferrals) {
+  const { reportSha256, ...rest } = r.read('.mutation/attestation.json');
+  writeState(join(r.top, '.mutation/remote/inc'), { report: reportFor(), attestation: { ...rest, deferrals } });
+}
+
+test('run --mode full starts fresh, attests the report and clears pending', async () => {
+  const r = runRepo();
+  const res = await cli(r, ['run', '--mode', 'full']);
+  assert.equal(res.code, 0);
+  assert.deepEqual(r.calls(), [inv(1)]);
+  const att = r.read('.mutation/attestation.json');
+  assert.deepEqual([att.mode, att.lastFullAt, att.deferrals, att.score], ['full', att.completedAt, [], 50]);
+  assert.equal(usable(r), true);
+  assert.equal(existsSync(join(r.top, '.mutation/work')), false);
+  assert.equal(existsSync(join(r.top, '.mutation/lock')), false);
+  assert.deepEqual(res.output, { pending_full: 'false', exit_code: '0', pending_cause: 'none', pending_causes: '[]' });
+  assert.match(res.stderr, /command=run invocations=1 scope=0 forced=0 deferrals=0 pending_full=false\n$/);
+});
+
+test('a full run that finds no tests is an engine failure and commits nothing', async () => {
+  const r = runRepo({ steps: [{ exit: 1, output: 'INFO No tests were executed. Stryker will exit prematurely.\n' }] });
+  const res = await cli(r, ['run', '--mode', 'full']);
+  assert.equal(res.code, 4);
+  assert.match(res.stderr, /the full run found no tests/);
+  assert.equal(existsSync(join(r.top, '.mutation/attestation.json')), false);
+  assert.deepEqual(res.output, { pending_full: 'true', exit_code: '4' });
+  r.steps([{ exit: 2 }]);
+  const failed = await cli(r, ['run', '--mode', 'full']);
+  assert.equal(failed.code, 4);
+  assert.match(failed.stderr, /the full run failed \(exit 2\)/);
+});
+
+test('an edit runs the planned invocations and carries lastFullAt', async () => {
+  const r = runRepo();
+  await warm(r);
+  const full = r.read('.mutation/attestation.json');
+  r.write('src/a.ts', A3);
+  const plan = JSON.parse((await cli(r, ['plan'])).stdout);
+  assert.deepEqual([plan.scope, plan.forced], [['src/a.ts'], ['src/a.ts:2-3']]);
+  const res = await cli(r, ['run', '--mode', 'incremental']);
+  assert.equal(res.code, 0);
+  assert.deepEqual(r.calls(), [inv(1, '--mutate', 'src/a.ts'), inv(2, '--force', '--mutate', 'src/a.ts:2-3')]);
+  const att = r.read('.mutation/attestation.json');
+  assert.deepEqual([att.mode, att.lastFullAt, att.deferrals], ['incremental', full.completedAt, []]);
+  assert.equal(att.files['src/a.ts'].digest, `sha256:${sha256(A3)}`);
+  assert.match(res.stderr, /command=run invocations=2 scope=1 forced=1 deferrals=0 pending_full=false\n$/);
+});
+
+test('no reachable tests defers only files with kills and is not a failure', async () => {
+  const r = runRepo();
+  await warm(r);
+  r.write('src/a.ts', A3);
+  r.write('src/n.ts', 'export const n = 1;\n'); // a new module before its first test: no kills
+  r.steps([{ exit: 1, output: 'No tests were executed' }]);
+  const res = await cli(r, ['run', '--mode', 'incremental']);
+  assert.equal(res.code, 0);
+  assert.equal(r.calls().length, 2);
+  assert.deepEqual(r.read('.mutation/attestation.json').deferrals, [{ reason: 'no reachable tests: src/a.ts', paths: ['src/a.ts'], inherited: false }]);
+  assert.equal(res.output.pending_cause, 'unreachable');
+  assert.deepEqual(JSON.parse(res.output.pending_causes), [{ reason: 'no reachable tests: src/a.ts', paths: ['src/a.ts'] }]);
+});
+
+test('a timed-out invocation 1 defers its files and blocks invocation 2', async () => {
+  const r = runRepo({ config: { budget: { maxForcedShare: 1, maxForcedMutants: null, maxMinutesPerInvocation: 0.005 } } });
+  await warm(r); // full mode has no time limit
+  r.write('src/a.ts', A3);
+  r.steps([{ sleepMs: 10000 }]);
+  const res = await cli(r, ['run', '--mode', 'incremental']);
+  assert.equal(res.code, 0);
+  assert.equal(r.calls().length, 1);
+  assert.deepEqual(r.read('.mutation/attestation.json').deferrals, [
+    { reason: 'blocked by deferred scope', paths: ['src/a.ts'], inherited: false },
+    { reason: 'time budget exceeded', paths: ['src/a.ts'], inherited: false },
+  ]);
+  assert.equal(res.output.pending_cause, 'timeout');
+});
+
+test('a cold incremental run with no report runs nothing and writes nothing', async () => {
+  const r = runRepo();
+  const res = await cli(r, ['run', '--mode', 'incremental']);
+  assert.equal(res.code, 0);
+  assert.deepEqual(r.calls(), []);
+  assert.equal(existsSync(join(r.top, '.mutation/attestation.json')), false);
+  assert.deepEqual(res.output, { pending_full: 'true', exit_code: '0', pending_cause: 'global', pending_causes: '[{"reason":"no usable state","paths":["*"]}]' });
+});
+
+test('a cold run re-attests an existing report with lastFullAt null and no usable state', async () => {
+  const r = runRepo();
+  await warm(r);
+  writeFileSync(join(r.top, '.mutation/attestation.json'), JSON.stringify({ ...r.read('.mutation/attestation.json'), stateVersion: STATE_VERSION + 1 }));
+  const res = await cli(r, ['run', '--mode', 'incremental']);
+  const att = r.read('.mutation/attestation.json');
+  assert.deepEqual([res.code, att.lastFullAt, att.stateVersion, r.calls()], [0, null, STATE_VERSION, []]);
+  assert.deepEqual(att.deferrals, [{ reason: 'no usable state', paths: ['*'], inherited: false }]);
+});
+
+test('a cold run over an unparseable report commits nothing', async () => {
+  const r = runRepo();
+  mkdirSync(join(r.top, '.mutation'), { recursive: true });
+  writeFileSync(join(r.top, '.mutation/incremental.json'), 'garbage');
+  const res = await cli(r, ['run', '--mode', 'incremental']);
+  assert.deepEqual([res.code, res.output.pending_full], [0, 'true']);
+  assert.equal(existsSync(join(r.top, '.mutation/attestation.json')), false);
+});
+
+test("main's pending deferral about unchanged inputs is inherited; the PR's own edits still run", async () => {
+  const r = runRepo();
+  await warm(r);
+  mainState(r, [{ reason: 'global input changed: package.json', paths: ['*'] }]);
+  rmSync(join(r.top, '.mutation/attestation.json')); // no state of its own: the run adopts main's
+  r.write('src/a.ts', A3);
+  const res = await cli(r, ['run', '--mode', 'incremental', '--also-state', '.mutation/remote/inc']);
+  assert.equal(res.code, 0);
+  assert.equal(r.calls().length, 2);
+  assert.deepEqual(r.read('.mutation/attestation.json').deferrals, [{ reason: 'global input changed: package.json', paths: ['*'], inherited: true }]);
+  assert.deepEqual([res.output.pending_full, res.output.pending_cause, res.output.pending_causes], ['true', 'none', '[]']);
+  assert.equal(usable(r), true);
+});
+
+test('a PR run with a counted global deferral takes the shortcut', async () => {
+  const r = runRepo({ config: ROUTED, files: { 'package.json': '{}\n' } });
+  await warm(r);
+  r.write('package.json', '{"x":1}\n');
+  const res = await cli(r, ['run', '--mode', 'incremental', '--pr', '--max-minutes', '5']);
+  assert.equal(res.code, 0);
+  assert.deepEqual(r.calls(), []);
+  assert.equal(res.output.pending_cause, 'global');
+  assert.deepEqual(r.read('.mutation/attestation.json').deferrals, [{ reason: 'global input changed: package.json', paths: ['*'], inherited: false }]);
+});
+
+test("a PR's own timeout is counted even when main carries the identical deferral", async () => {
+  const r = runRepo({ config: ROUTED });
+  await warm(r);
+  mainState(r, [{ reason: 'time budget exceeded', paths: ['src/a.ts'] }]);
+  r.write('src/a.ts', A3);
+  r.steps([{ sleepMs: 10000 }]);
+  const res = await cli(r, ['run', '--mode', 'incremental', '--pr', '--max-minutes', '0.005', '--also-state', '.mutation/remote/inc']);
+  assert.equal(res.code, 0);
+  const own = r.read('.mutation/attestation.json').deferrals.find((d) => d.reason === 'time budget exceeded');
+  assert.deepEqual(own, { reason: 'time budget exceeded', paths: ['src/a.ts'], inherited: false });
+  assert.equal(res.output.pending_cause, 'timeout');
+});
+
+test('a routed PR run is unbudgeted; other runs defer a forced set over budget', async () => {
+  const budget = { maxForcedShare: 0.25, maxForcedMutants: null, maxMinutesPerInvocation: 5 };
+  const r = runRepo({
+    config: { ...ROUTED, budget },
+    files: { 'tests/helpers/h.ts': 'export const h = 1;\n', 'tests/a.test.ts': "import { clamp } from '../src/a';\nimport { h } from './helpers/h';\n" },
+  });
+  await warm(r);
+  r.write('tests/helpers/h.ts', 'export const h = 2;\n');
+  const plain = JSON.parse((await cli(r, ['plan'])).stdout);
+  assert.deepEqual(plain.deferrals.map((d) => d.reason), ['forced set 2 exceeds budget 0']);
+  const res = await cli(r, ['run', '--mode', 'incremental', '--pr']);
+  assert.equal(res.code, 0);
+  assert.deepEqual(r.calls(), [inv(2, '--force', '--mutate', 'src/a.ts:2-3')]);
+  assert.deepEqual(r.read('.mutation/attestation.json').deferrals, []);
+});
+
+test('a score below thresholds.break is exit 1 with the state committed', async () => {
+  const r = runRepo({ stryker: { thresholds: { break: 60 } } });
+  const res = await cli(r, ['run', '--mode', 'full']);
+  assert.equal(res.code, 1);
+  assert.match(res.stderr, /score 50 is below thresholds.break 60/);
+  assert.equal(r.read('.mutation/attestation.json').thresholdBreak, true);
+  assert.deepEqual([res.output.exit_code, res.output.pending_cause], ['1', 'none']);
+});
+
+const verifier = (script) => ({ verify: { command: [process.execPath, '-e', script], timeoutMinutes: null } });
+
+test('a failing verifier is exit 1 naming the view; it does not run when nothing was committed', async () => {
+  const r = runRepo({ config: { views: { inline: { core: ['src/**'], hot: ['src/a.ts'] } }, ...verifier("process.exit(process.env.MUTATION_VIEW === 'hot' ? 3 : 0)") } });
+  const cold = await cli(r, ['run', '--mode', 'incremental']);
+  assert.equal(cold.code, 0);
+  const res = await cli(r, ['run', '--mode', 'full']);
+  assert.equal(res.code, 1);
+  assert.match(res.stderr, /mutation-incremental: verify \(view hot\): exit 3/);
+  assert.doesNotMatch(res.stderr, /view core/);
+  const att = r.read('.mutation/attestation.json');
+  assert.deepEqual(att.views, { core: ['src/**'], hot: ['src/a.ts'] });
+  assert.deepEqual(att.viewSummaries.hot, { Killed: 1, Survived: 1, pendingCounted: 0, pendingInherited: 0 });
+});
+
+test('an interrupt stops Stryker, commits nothing and exits 130', async () => {
+  const r = runRepo();
+  await warm(r);
+  const before = attestationText(r);
+  r.write('src/a.ts', A3);
+  r.steps([{ sleepMs: 10000 }]);
+  const timer = setTimeout(() => process.emit('SIGHUP'), 600);
+  const res = await cli(r, ['run', '--mode', 'incremental']);
+  clearTimeout(timer);
+  assert.equal(res.code, 130);
+  assert.equal(attestationText(r), before);
+  assert.equal(existsSync(join(r.top, '.mutation/work')), false);
+  assert.equal(existsSync(join(r.top, '.mutation/lock')), false);
+  assert.deepEqual(res.output, { pending_full: 'false', exit_code: '130' });
+  // A full run is interrupted the same way.
+  r.steps([{ sleepMs: 10000 }]);
+  const again = setTimeout(() => process.emit('SIGHUP'), 600);
+  const full = await cli(r, ['run', '--mode', 'full']);
+  clearTimeout(again);
+  assert.equal(full.code, 130);
+  assert.equal(attestationText(r), before);
+});
+
+test('an interrupt during the verifier exits 130 with the state committed', async () => {
+  const r = runRepo({ config: verifier('setTimeout(() => {}, 20000)') });
+  const timer = setTimeout(() => process.emit('SIGHUP'), 1000);
+  const res = await cli(r, ['run', '--mode', 'full']);
+  clearTimeout(timer);
+  assert.equal(res.code, 130);
+  assert.equal(usable(r), true);
+});
+
+test('an invocation failure is exit 4 and keeps the previous state', async () => {
+  const r = runRepo();
+  await warm(r);
+  const before = attestationText(r);
+  r.write('src/a.ts', A3);
+  r.steps([{ exit: 2 }]);
+  const res = await cli(r, ['run', '--mode', 'incremental']);
+  assert.equal(res.code, 4);
+  assert.match(res.stderr, /stryker invocation 1 failed \(exit 2\); nothing committed/);
+  assert.equal(attestationText(r), before);
+  assert.equal(existsSync(join(r.top, '.mutation/work')), false);
+  assert.deepEqual(res.output, { pending_full: 'false', exit_code: '4' });
+});
+
+test('a held lock is exit 3 and leaves the other run alone', async () => {
+  const r = runRepo();
+  mkdirSync(join(r.top, '.mutation/work'), { recursive: true });
+  writeFileSync(join(r.top, '.mutation/lock'), JSON.stringify({ pid: process.pid, hostname: hostname(), startedAt: 'x' }));
+  const res = await cli(r, ['run', '--mode', 'full']);
+  assert.equal(res.code, 3);
+  assert.equal(existsSync(join(r.top, '.mutation/work')), true);
+  assert.equal(existsSync(join(r.top, '.mutation/lock')), true);
+  assert.deepEqual(res.output, { pending_full: 'true', exit_code: '3' });
+});
+
+test('run option errors are exit 2 and still write outputs', async () => {
+  const r = runRepo();
+  for (const args of [['run'], ['run', '--mode', 'full', '--pr'], ['run', '--mode', 'incremental', '--max-minutes', '5'], ['run', '--mode', 'incremental', '--pr', '--max-minutes', 'x']]) {
+    const res = await cli(r, args);
+    assert.equal(res.code, 2, args.join(' '));
+    assert.deepEqual(res.output, { pending_full: 'false', exit_code: '2' }, args.join(' '));
+  }
+  const allow = await cli(r, ['run', '--mode', 'incremental', '--pr']);
+  assert.deepEqual([allow.code, allow.output.pending_full], [2, 'true']);
+  assert.match(allow.stderr, /--pr is for pendingOnPr "full" or "full-on-global"/);
+});
+
+test('a file edited while Stryker runs is attested as changed-during-run', async () => {
+  const r = runRepo({ files: { 'src/b.ts': 'export const b = 1;\n' }, steps: [{ report: reportFor(), touch: { 'src/b.ts': 'export const b = 2;\n' } }] });
+  assert.equal((await cli(r, ['run', '--mode', 'full'])).code, 0);
+  assert.equal(r.read('.mutation/attestation.json').files['src/b.ts'].digest, 'changed-during-run');
+});
+
+test('an unexpected error is exit 4 in the outputs too', async () => {
+  const r = runRepo();
+  writeFileSync(join(r.top, '.mutation'), 'not a directory');
+  const res = await cli(r, ['run', '--mode', 'full']);
+  assert.deepEqual([res.code, res.output], [4, { pending_full: 'true', exit_code: '4' }]);
+});
+
+test('without GITHUB_OUTPUT nothing is appended', async () => {
+  const r = runRepo();
+  const saved = process.env.GITHUB_OUTPUT;
+  delete process.env.GITHUB_OUTPUT;
+  try {
+    const quiet = { write() {} };
+    // Also the default 30 s kill grace (nothing is killed here).
+    assert.equal(await main(['run', '--mode', 'full'], { stdout: quiet, stderr: quiet, cwd: r.top }), 0);
+  } finally {
+    if (saved !== undefined) process.env.GITHUB_OUTPUT = saved;
+  }
+});
+
+test('pending_causes is single-line JSON, capped with a count of the rest', () => {
+  const many = Array.from({ length: MAX_CAUSES + 2 }, (_, i) => ({ reason: `no reachable tests: f${i}`, paths: [`f${i}`], inherited: false }));
+  const parsed = JSON.parse(causesJSON(many));
+  assert.equal(parsed.length, MAX_CAUSES + 1);
+  assert.deepEqual(parsed.at(-1), { more: 2 });
+  const odd = causesJSON([{ reason: 'no reachable tests: a\u0007b', paths: ['a\u0007b'], inherited: false }]);
+  assert.doesNotMatch(odd, /[\u0000-\u001f]/);
+  assert.deepEqual(JSON.parse(odd), [{ reason: 'no reachable tests: a\u0007b', paths: ['a\u0007b'] }]);
+});
diff --git a/templates/mutation-incremental/test/seed.test.mjs b/templates/mutation-incremental/test/seed.test.mjs
new file mode 100644
index 0000000..298fe24
--- /dev/null
+++ b/templates/mutation-incremental/test/seed.test.mjs
@@ -0,0 +1,95 @@
+import { test } from 'node:test';
+import assert from 'node:assert/strict';
+import { readdirSync, writeFileSync } from 'node:fs';
+import { join } from 'node:path';
+import { mergeReports } from '../lib/seed.mjs';
+import { readCandidate } from '../lib/state.mjs';
+import { A, cli, reportFor, runRepo } from './fake.mjs';
+
+const at = (line) => ({ start: { line, column: 1 }, end: { line, column: 5 } });
+const mu = (id, mutatorName, replacement, status, line, killedBy, coveredBy) => ({ id, mutatorName, replacement, status, killedBy, coveredBy, location: at(line) });
+// X and Y share the line-2 and line-3 mutants; line 1 holds three mutants at one position (two
+// mutators, two replacements), as Stryker produces for one expression.
+const X = {
+  schemaVersion: '1',
+  files: { 'src/a.ts': { language: 'typescript', source: A, mutants: [
+    mu('9', 'M', 'r', 'Survived', 2, undefined, ['x1']),
+    mu('8', 'M', 'r', 'CompileError', 3, undefined, undefined),
+  ] } },
+  testFiles: { 'tests/a.test.ts': { tests: [{ id: 'x1', name: 'low' }] } },
+};
+const Y = {
+  schemaVersion: '1',
+  files: { 'src/a.ts': { language: 'typescript', source: A, mutants: [
+    mu('3', 'M', 'r', 'Killed', 2, ['y2'], ['y2', 'y1']),
+    mu('4', 'N', 's', 'NoCoverage', 1, undefined, []),
+    mu('5', 'N', 'r', 'NoCoverage', 1, undefined, []),
+    mu('6', 'M', 'r', 'NoCoverage', 1, undefined, []),
+    mu('7', 'M', 'r', 'Survived', 3, undefined, ['y1']),
+  ] } },
+  testFiles: { 'tests/a.test.ts': { tests: [{ id: 'y1', name: 'high' }, { id: 'y2', name: 'low' }] } },
+};
+
+test('mergeReports re-keys tests by (file, name), merges mutants and renumbers both', () => {
+  for (const order of [[X, Y], [Y, X]]) {
+    const merged = mergeReports(order);
+    assert.deepEqual(merged.testFiles, { 'tests/a.test.ts': { tests: [{ id: '0', name: 'high' }, { id: '1', name: 'low' }] } });
+    const mutants = merged.files['src/a.ts'].mutants.map((m) => [m.id, m.location.start.line, m.mutatorName, m.replacement, m.status, m.killedBy, m.coveredBy]);
+    assert.deepEqual(mutants, [
+      ['0', 1, 'M', 'r', 'NoCoverage', [], []],
+      ['1', 1, 'N', 'r', 'NoCoverage', [], []],
+      ['2', 1, 'N', 's', 'NoCoverage', [], []],
+      ['3', 2, 'M', 'r', 'Killed', ['1'], ['0', '1']],
+      ['4', 3, 'M', 'r', 'Survived', [], ['0']],
+    ]);
+    assert.equal(merged.files['src/a.ts'].source, A);
+  }
+});
+
+test('seed bootstraps a state whose kills are pending until a full run', async () => {
+  const r = runRepo({ config: { views: { inline: { core: ['src/**'] } } } });
+  writeFileSync(join(r.top, '.fake/full.json'), JSON.stringify(reportFor()));
+  const res = await cli(r, ['seed', '--from', '.fake/full.json']);
+  assert.equal(res.code, 0);
+  assert.equal(res.stdout, 'seed: score=50 thresholdBreak=false\n');
+  assert.match(res.stderr, /command=seed invocations=0 scope=0 forced=0 deferrals=1 pending_full=true\n$/);
+  const att = r.read('.mutation/attestation.json');
+  assert.deepEqual([att.mode, att.lastFullAt], ['seed', null]);
+  assert.deepEqual(att.deferrals, [{ reason: 'seeded from .fake/full.json', paths: ['*'], inherited: false }]);
+  assert.deepEqual(att.viewSummaries.core, { Killed: 1, Survived: 1, pendingCounted: 1, pendingInherited: 0 });
+  assert.equal(readCandidate(join(r.top, '.mutation'), { label: 's', primary: true }).usable, true);
+  assert.deepEqual(r.read('.mutation/incremental.json').testFiles['tests/a.test.ts'].tests, [{ id: '0', name: 'clamps low' }]);
+});
+
+test('seed refuses usable state unless --replace, which first keeps a copy', async () => {
+  const r = runRepo();
+  writeFileSync(join(r.top, '.fake/full.json'), JSON.stringify(reportFor()));
+  const empty = await cli(r, ['seed', '--from', '.fake/full.json', '--replace']); // nothing to keep yet
+  assert.equal(empty.code, 0);
+  const again = await cli(r, ['seed', '--from', '.fake/full.json']);
+  assert.equal(again.code, 2);
+  assert.match(again.stderr, /usable state exists in \.mutation; pass --replace/);
+  assert.equal((await cli(r, ['seed', '--from', '.fake/full.json', '--replace'])).code, 0);
+  const kept = readdirSync(join(r.top, '.mutation/replaced')).sort();
+  assert.equal(kept.length, 2);
+  assert.deepEqual(readdirSync(join(r.top, '.mutation/replaced', kept[0])), []);
+  assert.deepEqual(readdirSync(join(r.top, '.mutation/replaced', kept[1])).sort(), ['attestation.json', 'incremental.json']);
+});
+
+test('seed input errors are exit 2', async () => {
+  const r = runRepo();
+  writeFileSync(join(r.top, '.fake/bad.json'), '{"files":[]}');
+  writeFileSync(join(r.top, '.fake/x.json'), JSON.stringify(reportFor()));
+  writeFileSync(join(r.top, '.fake/y.json'), JSON.stringify(reportFor(A.replace('lo;', 'lo ;'))));
+  const cases = [
+    [['seed'], /seed needs --from/],
+    [['seed', '--from', '.fake/missing.json'], /seed: cannot read \.fake\/missing\.json/],
+    [['seed', '--from', '.fake/bad.json'], /\.fake\/bad\.json is not a mutation-testing report/],
+    [['seed', '--from', '.fake/x.json', '--from', '.fake/y.json'], /src\/a\.ts has a different source in the given reports/],
+  ];
+  for (const [args, message] of cases) {
+    const res = await cli(r, args);
+    assert.equal(res.code, 2, args.join(' '));
+    assert.match(res.stderr, message);
+  }
+});
diff --git a/templates/mutation-incremental/test/state.test.mjs b/templates/mutation-incremental/test/state.test.mjs
index c317a86..cd8a3c5 100644
--- a/templates/mutation-incremental/test/state.test.mjs
+++ b/templates/mutation-incremental/test/state.test.mjs
@@ -6,6 +6,9 @@ import { join } from 'node:path';
 import { readCandidate, changeSet, adopt, STATE_VERSION, TOOL } from '../lib/state.mjs';
 import { sha256 } from '../lib/snapshot.mjs';
 import { writeState } from './helpers.mjs';
+import { canonicalPendingFull, primaryCandidate } from '../lib/state.mjs';
+import { loadConfig } from '../lib/config.mjs';
+import { makeRepo } from './helpers.mjs';
 
 const dir = () => mkdtempSync(join(tmpdir(), 'mi-state-'));
 const opts = { label: 'x', primary: false };
@@ -96,3 +99,15 @@ test('adopt: no deferrals first, then smallest change set, then primary, then or
   assert.equal(adopt([{ label: 'u', usable: false }], s), null);
   assert.equal(adopt([], s), null);
 });
+
+
+test('canonicalPendingFull: an unusable state or one with deferrals is a pending full run', () => {
+  const r = makeRepo();
+  const config = loadConfig(r.top);
+  assert.equal(canonicalPendingFull(config), true);
+  writeState(config.stateDir, { report: { files: {} } });
+  assert.equal(canonicalPendingFull(config), false);
+  writeState(config.stateDir, { report: { files: {} }, attestation: { deferrals: [{ reason: 'no usable state', paths: ['*'] }] } });
+  assert.equal(canonicalPendingFull(config), true);
+  assert.deepEqual([primaryCandidate(config).label, primaryCandidate(config).primary], ['.mutation', true]);
+});
diff --git a/templates/mutation-incremental/test/verify.test.mjs b/templates/mutation-incremental/test/verify.test.mjs
new file mode 100644
index 0000000..b2a21ae
--- /dev/null
+++ b/templates/mutation-incremental/test/verify.test.mjs
@@ -0,0 +1,62 @@
+import { test } from 'node:test';
+import assert from 'node:assert/strict';
+import { readFileSync } from 'node:fs';
+import { join } from 'node:path';
+import { loadConfig } from '../lib/config.mjs';
+import { takeSnapshot } from '../lib/snapshot.mjs';
+import { createInterrupt } from '../lib/proc.mjs';
+import { runVerify } from '../lib/verify.mjs';
+import { runRepo } from './fake.mjs';
+
+const verifyConfig = (script, timeoutMinutes = null) => ({ verify: { command: [process.execPath, '-e', script], timeoutMinutes } });
+// The verifier records what it was given in .fake/verify.jsonl.
+const RECORD = "require('fs').appendFileSync('.fake/verify.jsonl', JSON.stringify({ view: process.env.MUTATION_VIEW ?? null, patterns: process.env.MUTATION_VIEW_PATTERNS ?? null, files: process.env.MUTATION_VIEW_FILES ?? null, kind: process.env.MUTATION_RUN_KIND, report: process.env.MUTATION_REPORT, attestation: process.env.MUTATION_ATTESTATION }) + '\\n');";
+const records = (r) => readFileSync(join(r.top, '.fake/verify.jsonl'), 'utf8').split('\n').filter(Boolean).map((l) => JSON.parse(l));
+const go = (config, extra = {}) => runVerify(config, {
+  views: null, snapshot: takeSnapshot(config), pr: false, interrupt: createInterrupt(), graceMs: 200, onOutput: () => {}, ...extra,
+});
+const VIEWS = { core: ['src/**'], hot: ['src/a.ts'] };
+
+test('nothing configured runs nothing', async () => {
+  assert.deepEqual(await go(loadConfig(runRepo().top)), []);
+});
+
+test('without views the verifier runs once, with the report paths and no view variables', async () => {
+  const r = runRepo({ config: verifyConfig(RECORD) });
+  const config = loadConfig(r.top);
+  process.env.MUTATION_VIEW = 'leaked';
+  try {
+    assert.deepEqual(await go(config), []);
+  } finally {
+    delete process.env.MUTATION_VIEW;
+  }
+  assert.deepEqual(records(r), [{
+    view: null, patterns: null, files: null, kind: 'other',
+    report: join(config.stateDir, 'incremental.json'), attestation: join(config.stateDir, 'attestation.json'),
+  }]);
+});
+
+test('with views it runs once per view, names each failing view and marks PR runs', async () => {
+  const r = runRepo({ config: verifyConfig(`${RECORD} process.exit(process.env.MUTATION_VIEW === 'hot' ? 3 : 0);`), files: { 'src/b.ts': 'export const b = 1;\n' } });
+  const config = loadConfig(r.top);
+  assert.deepEqual(await go(config, { views: VIEWS, pr: true }), ['verify (view hot): exit 3']);
+  assert.deepEqual(records(r).map((x) => [x.view, x.patterns, x.files, x.kind]), [
+    ['core', '["src/**"]', '["src/a.ts","src/b.ts"]', 'pr'],
+    ['hot', '["src/a.ts"]', '["src/a.ts"]', 'pr'],
+  ]);
+});
+
+test('a verifier that cannot be spawned or exceeds its timeout fails', async () => {
+  const missing = runRepo({ config: { verify: { command: ['/nonexistent/verifier'], timeoutMinutes: null } } });
+  assert.match((await go(loadConfig(missing.top)))[0], /^verify: cannot run \/nonexistent\/verifier: .*ENOENT/);
+  const slow = runRepo({ config: verifyConfig('setTimeout(() => {}, 20000)', 0.005) });
+  assert.deepEqual(await go(loadConfig(slow.top)), ['verify: exceeded verify.timeoutMinutes (0.005)']);
+});
+
+test('an interrupt stops the loop without a failure line', async () => {
+  const r = runRepo({ config: verifyConfig(`${RECORD} setTimeout(() => {}, 20000);`) });
+  const interrupt = createInterrupt();
+  setTimeout(() => interrupt.trigger(), 600);
+  assert.deepEqual(await go(loadConfig(r.top), { views: VIEWS, interrupt }), []);
+  assert.equal(records(r).length, 1);
+});



```

## Knowledge And Registries

Service inventory: none

No service inventory found.

Knowledge facts:

No Beads knowledge facts found.

## Evidence

No external validation evidence supplied.
