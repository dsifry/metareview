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
