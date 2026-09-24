import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { main, parseArgs } from '../lib/main.mjs';
import { nodeVersionWarning } from '../lib/nodever.mjs';
import { loadConfig } from '../lib/config.mjs';
import { takeSnapshot } from '../lib/snapshot.mjs';
import { makeRepo, writeState } from './helpers.mjs';
import { loadHarnessConfig } from '../lib/inputs.mjs';

const capture = () => {
  const out = { text: '', write(s) { this.text += s; } };
  return out;
};
async function run(args, cwd) {
  const stdout = capture();
  const stderr = capture();
  const code = await main(args, { stdout, stderr, cwd });
  return { code, stdout: stdout.text, stderr: stderr.text };
}

test('parseArgs', () => {
  assert.deepEqual(parseArgs(['plan', '--also-state', 'a', '--config', 'c.json', '--also-state', 'b']), {
    command: 'plan', config: 'c.json', mode: undefined, maxMinutes: undefined, remote: undefined, kind: undefined,
    job: undefined, exitCode: undefined, alsoState: ['a', 'b'], from: [], pr: false, replace: false,
  });
  assert.deepEqual(parseArgs(['run', '--mode', 'incremental', '--pr', '--max-minutes', '60', '--replace', '--from', 'r1', '--from', 'r2', '--remote', 'up', '--kind', 'inc', '--job', 'pr', '--exit-code', '1']), {
    command: 'run', config: undefined, mode: 'incremental', maxMinutes: '60', remote: 'up', kind: 'inc',
    job: 'pr', exitCode: '1', alsoState: [], from: ['r1', 'r2'], pr: true, replace: true,
  });
  assert.throws(() => parseArgs(['plan', '--config']), (e) => e.exitCode === 2);
  assert.throws(() => parseArgs(['plan', '--bogus']), (e) => e.exitCode === 2);
  assert.deepEqual(parseArgs([]).command, undefined);
});

test('plan on a repo without state is cold', async () => {
  const r = makeRepo({ files: { 'src/a.ts': 'export const a = 1;\n' } });
  const res = await run(['plan'], r.top);
  assert.equal(res.code, 0);
  const plan = JSON.parse(res.stdout);
  assert.equal(plan.cold, true);
  assert.deepEqual(plan.deferrals, [{ reason: 'no usable state', paths: ['*'] }]);
  assert.match(res.stderr, /mutation-incremental: command=plan invocations=0 scope=0 forced=0 deferrals=1 pending_full=true\n$/);
});

test('plan adopts a warm state and reports the change', async () => {
  const r = makeRepo({ files: { 'src/a.ts': 'export const a = 1;\n' } });
  const cfg = loadConfig(r.top);
  const snap = takeSnapshot(cfg);
  // The also-state lives under stateDir, which the snapshot excludes, so it is not itself a change.
  const report = {
    files: { 'src/a.ts': { language: 'typescript', source: 'export const a = 1;\n', mutants: [{ id: '1', mutatorName: 'M', replacement: 'r', status: 'Killed', killedBy: ['t'], coveredBy: ['t'], location: { start: { line: 1, column: 1 }, end: { line: 1, column: 20 } } }] } },
    testFiles: { 'tests/a.test.ts': { tests: [{ id: 't', name: 'a' }] } },
  };
  writeState(join(r.top, '.mutation/remote'), { report, attestation: { files: snap.files, runtime: snap.runtime } });
  r.write('src/a.ts', 'export const a = 2;\n');
  const res = await run(['plan', '--also-state', '.mutation/remote'], r.top);
  assert.equal(res.code, 0);
  const plan = JSON.parse(res.stdout);
  assert.equal(plan.baseline, '.mutation/remote');
  assert.deepEqual(plan.changes, [{ path: 'src/a.ts', category: 'mutate', kind: 'changed' }]);
  // Residual mode (the default): the edited file goes whole into the unforced scope and its
  // changed line is forced.
  assert.deepEqual([plan.scope, plan.forced], [['src/a.ts'], ['src/a.ts:1-1']]);
});

test('unexpected internal errors are exit 4, never 1', async () => {
  const r = makeRepo();
  const stderr = capture();
  const code = await main(['plan'], { stdout: { write() { throw new Error('boom'); } }, stderr, cwd: r.top });
  assert.equal(code, 4);
  assert.match(stderr.text, /internal error: boom/);
});

test('usage errors are exit 2 with a message', async () => {
  const r = makeRepo();
  assert.equal((await run(['nope'], r.top)).code, 2);
  assert.match((await run([], r.top)).stderr, /usage/);
  assert.equal((await run(['plan', '--config', 'missing.json'], r.top)).code, 2);
  const notRepo = await run(['plan'], mkdtempSync(join(tmpdir(), 'mi-norepo-')));
  assert.equal(notRepo.code, 2);
  assert.match(notRepo.stderr, /not inside a git repository/);
});

test('plan validates views before printing', async () => {
  const r = makeRepo({ config: { views: { inline: { u: ['src/a.ts'] } } }, files: { 'src/a.ts': 'a', 'src/b.ts': 'b' } });
  const res = await run(['plan'], r.top);
  assert.equal(res.code, 2);
  assert.match(res.stderr, /src\/b.ts belongs to no view/);
});

test('plan prints the Node-version warning before the summary', async () => {
  const r = makeRepo({ files: { '.nvmrc': '1.2\n' } });
  const res = await run(['plan'], r.top);
  assert.match(res.stderr, /warning: \.nvmrc pins Node 1\.2 but this is v\d+.*\nmutation-incremental: command=plan/);
});

test('nodeVersionWarning: prefix match on components, aliases skipped, first file wins', () => {
  const r = makeRepo({ files: { '.nvmrc': 'v22\n', '.node-version': '1.0.0' } });
  assert.equal(nodeVersionWarning(r.top, 'v22.9.0'), null);
  assert.match(nodeVersionWarning(r.top, 'v2.9.0'), /pins Node 22/);
  const alias = makeRepo({ files: { '.nvmrc': 'lts/iron\n', '.node-version': '1.0.0' } });
  assert.equal(nodeVersionWarning(alias.top, 'v22.9.0'), null);
  const nv = makeRepo({ files: { '.node-version': '20.1' } });
  assert.match(nodeVersionWarning(nv.top, 'v20.11.0'), /\.node-version pins Node 20\.1/);
  assert.equal(nodeVersionWarning(makeRepo().top, 'v22.9.0'), null);
});

test('loadHarnessConfig prints the Node-version warning only when asked', () => {
  const r = makeRepo({ files: { '.nvmrc': '1.2\n' } });
  const err = capture();
  loadHarnessConfig({ cwd: r.top, stderr: err }, {}, false);
  assert.equal(err.text, '');
  loadHarnessConfig({ cwd: r.top, stderr: err }, {}, true);
  assert.match(err.text, /pins Node 1\.2/);
});

test('cli.mjs runs main with process.argv', async () => {
  const saved = process.argv;
  process.argv = [process.execPath, 'cli.mjs', '--bogus'];
  try {
    await import('../cli.mjs');
    assert.equal(process.exitCode, 2);
  } finally {
    process.argv = saved;
    process.exitCode = 0;
  }
});
