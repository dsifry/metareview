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
