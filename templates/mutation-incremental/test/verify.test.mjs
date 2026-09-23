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
