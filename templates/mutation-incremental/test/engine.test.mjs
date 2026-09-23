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
