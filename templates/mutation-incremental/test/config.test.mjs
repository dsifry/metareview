import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdirSync, rmSync } from 'node:fs';
import { join } from 'node:path';
import { loadConfig, TOOL_DIR_GLOB } from '../lib/config.mjs';
import { makeRepo, baseConfig } from './helpers.mjs';

const cfg = (extra) => JSON.stringify({ ...baseConfig(), ...extra });

test('loads a valid config with defaults for the optional keys', () => {
  const { top } = makeRepo({ stryker: { tempDirName: '.st', jsonReporter: { fileName: 'r/m.json' }, htmlReporter: { fileName: 'r/m.html' }, thresholds: { break: 80 } } });
  const c = loadConfig(top);
  assert.deepEqual(c.lists.global, ['package.json', 'mutation-incremental.json', TOOL_DIR_GLOB]);
  assert.deepEqual(c.exclusions, ['.mutation/**', '.st/**', 'r/m.json', 'r/m.html']);
  assert.equal(c.engineVersion, '10.0.0');
  assert.equal(c.stateRel, '.mutation');
  assert.equal(c.stateDirRaw, '.mutation');
  assert.equal(c.residualMode, 'bounded');
  assert.equal(c.editedFiles, 'residual');
  assert.equal(c.pendingOnPr, 'allow');
  assert.equal(c.allowBail, false);
  assert.equal(c.verify, null);
  assert.equal(c.views, null);
  assert.equal(c.thresholdBreak, 80);
  assert.equal(c.disableBail, true);
});

test('accepts the optional keys', () => {
  const { top, write } = makeRepo();
  write('mutation-incremental.json', cfg({
    editedFiles: 'whole', pendingOnPr: 'full-on-global', allowBail: true,
    verify: { command: ['node', 'v.mjs'], timeoutMinutes: 5 }, views: { inline: { u1: ['src/**'] } },
  }));
  write('stryker.config.json', JSON.stringify({ testRunner: 'vitest', mutate: ['src/**/*.ts'], ignorePatterns: ['.mutation'] }));
  const c = loadConfig(top);
  assert.equal(c.editedFiles, 'whole');
  assert.equal(c.pendingOnPr, 'full-on-global');
  assert.deepEqual(c.verify, { command: ['node', 'v.mjs'], timeoutMinutes: 5 });
  assert.deepEqual(c.views, { inline: { u1: ['src/**'] } });
  assert.equal(c.thresholdBreak, null);
  assert.equal(c.disableBail, false);
});

test('accepts views from a command and a verifier without a timeout', () => {
  const { top, write } = makeRepo();
  write('mutation-incremental.json', cfg({ views: { command: ['node', 'views.mjs'] }, verify: { command: ['node', 'v.mjs'] } }));
  const c = loadConfig(top);
  assert.deepEqual(c.views, { command: ['node', 'views.mjs'] });
  assert.deepEqual(c.verify, { command: ['node', 'v.mjs'], timeoutMinutes: null });
  assert.deepEqual(loadConfig(makeRepo().top).exclusions, ['.mutation/**', '.stryker-tmp/**', 'reports/mutation/mutation.json', 'reports/mutation/mutation.html']);
});

const invalid = [
  ['missing file', (r) => rmSync(join(r.top, 'mutation-incremental.json')), /not found/],
  ['unparseable', (r) => r.write('mutation-incremental.json', '{'), /cannot parse/],
  ['not an object', (r) => r.write('mutation-incremental.json', '[]'), /must be a JSON object/],
  ['duplicate key', (r) => r.write('mutation-incremental.json', cfg({}).replace('"schemaVersion":1', '"schemaVersion":1,"schemaVersion":1')), /duplicate key "schemaVersion"/],
  ['missing key', (r) => { const c = baseConfig(); delete c.budget; r.write('mutation-incremental.json', JSON.stringify(c)); }, /missing key budget/],
  ['unknown key', (r) => r.write('mutation-incremental.json', cfg({ extra: 1 })), /unknown key extra/],
  ['schemaVersion', (r) => r.write('mutation-incremental.json', cfg({ schemaVersion: 2 })), /schemaVersion/],
  ['list type', (r) => r.write('mutation-incremental.json', cfg({ test: 'x' })), /test must be an array/],
  ['aliases', (r) => r.write('mutation-incremental.json', cfg({ aliases: { a: 1 } })), /aliases/],
  ['residual', (r) => r.write('mutation-incremental.json', cfg({ residual: { mode: 'x' } })), /residual.mode/],
  ['residual null', (r) => r.write('mutation-incremental.json', cfg({ residual: null })), /residual.mode/],
  ['budget null', (r) => r.write('mutation-incremental.json', cfg({ budget: null })), /maxForcedShare/],
  ['stryker null', (r) => r.write('mutation-incremental.json', cfg({ stryker: null })), /stryker needs/],
  ['runtime null', (r) => r.write('mutation-incremental.json', cfg({ runtime: null })), /runtime needs/],
  ['share', (r) => r.write('mutation-incremental.json', cfg({ budget: { ...baseConfig().budget, maxForcedShare: 0 } })), /maxForcedShare/],
  ['maxForcedMutants', (r) => r.write('mutation-incremental.json', cfg({ budget: { ...baseConfig().budget, maxForcedMutants: 1.5 } })), /maxForcedMutants/],
  ['maxForcedMutants zero', (r) => r.write('mutation-incremental.json', cfg({ budget: { ...baseConfig().budget, maxForcedMutants: 0 } })), /maxForcedMutants/],
  ['minutes', (r) => r.write('mutation-incremental.json', cfg({ budget: { ...baseConfig().budget, maxMinutesPerInvocation: 0 } })), /maxMinutesPerInvocation/],
  ['stryker shape', (r) => r.write('mutation-incremental.json', cfg({ stryker: { command: [] } })), /stryker needs/],
  ['runtime shape', (r) => r.write('mutation-incremental.json', cfg({ runtime: {} })), /runtime needs/],
  ['runtime command shape', (r) => r.write('mutation-incremental.json', cfg({ runtime: { commands: ['node'], env: [] } })), /runtime needs/],
  ['runtime env shape', (r) => r.write('mutation-incremental.json', cfg({ runtime: { commands: [], env: [1] } })), /runtime needs/],
  ['editedFiles', (r) => r.write('mutation-incremental.json', cfg({ editedFiles: 'x' })), /editedFiles/],
  ['pendingOnPr', (r) => r.write('mutation-incremental.json', cfg({ pendingOnPr: 'x' })), /pendingOnPr/],
  ['allowBail type', (r) => r.write('mutation-incremental.json', cfg({ allowBail: 'yes' })), /allowBail must be a boolean/],
  ['verify shape', (r) => r.write('mutation-incremental.json', cfg({ verify: { command: 'x' } })), /verify must be/],
  ['verify extra key', (r) => r.write('mutation-incremental.json', cfg({ verify: { command: ['x'], other: 1 } })), /verify must be/],
  ['verify not an object', (r) => r.write('mutation-incremental.json', cfg({ verify: 'x' })), /verify must be/],
  ['verify bad timeout', (r) => r.write('mutation-incremental.json', cfg({ verify: { command: ['x'], timeoutMinutes: 0 } })), /verify must be/],
  ['verify timeout required', (r) => r.write('mutation-incremental.json', cfg({ pendingOnPr: 'full', verify: { command: ['x'], timeoutMinutes: null } })), /timeoutMinutes is required/],
  ['views shape', (r) => r.write('mutation-incremental.json', cfg({ views: { command: ['x'], inline: {} } })), /views must be/],
  ['views array', (r) => r.write('mutation-incremental.json', cfg({ views: [] })), /views must be/],
  ['views inline array', (r) => r.write('mutation-incremental.json', cfg({ views: { inline: [] } })), /views must be/],
  ['off with full-on-global', (r) => r.write('mutation-incremental.json', cfg({ residual: { mode: 'off' }, pendingOnPr: 'full-on-global', verify: null })), /residual.mode off/],
  ['stateDir outside', (r) => r.write('mutation-incremental.json', cfg({ stateDir: '..' })), /inside the repository/],
  ['stateDir is top', (r) => r.write('mutation-incremental.json', cfg({ stateDir: '.' })), /inside the repository/],
  ['stateDir matches mutate', (r) => {
    r.write('mutation-incremental.json', cfg({ stateDir: 'src/state', mutate: ['src/**'] }));
    r.write('stryker.config.json', JSON.stringify({ testRunner: 'vitest', mutate: ['src/**'], ignorePatterns: ['src/state'], disableBail: true }));
  }, /must not match a mutate pattern/],
  ['stryker json', (r) => r.write('stryker.config.json', 'nope'), /readable JSON/],
  ['runner', (r) => r.write('stryker.config.json', JSON.stringify({ testRunner: 'jest', mutate: ['src/**/*.ts'], ignorePatterns: ['.mutation'], disableBail: true })), /testRunner "vitest"/],
  ['inPlace', (r) => r.write('stryker.config.json', JSON.stringify({ testRunner: 'vitest', inPlace: true, mutate: ['src/**/*.ts'], ignorePatterns: ['.mutation'], disableBail: true })), /inPlace/],
  ['mutate mismatch', (r) => r.write('stryker.config.json', JSON.stringify({ testRunner: 'vitest', mutate: ['lib/**'], ignorePatterns: ['.mutation'], disableBail: true })), /must equal config mutate/],
  ['ignorePatterns', (r) => r.write('stryker.config.json', JSON.stringify({ testRunner: 'vitest', mutate: ['src/**/*.ts'], disableBail: true })), /ignorePatterns must contain/],
  ['bail', (r) => r.write('stryker.config.json', JSON.stringify({ testRunner: 'vitest', mutate: ['src/**/*.ts'], ignorePatterns: ['.mutation'] })), /disableBail: true/],
  ['allowBail with residual', (r) => r.write('mutation-incremental.json', cfg({ allowBail: true })), /allowBail is refused/],
  ['core missing', (r) => rmSync(join(r.top, 'node_modules'), { recursive: true }), /not installed/],
  ['core version', (r) => r.write('node_modules/@stryker-mutator/core/package.json', JSON.stringify({ version: '9.1.0' })), /StrykerJS 10.x/],
];

for (const [name, mutate, message] of invalid) {
  test(`rejects: ${name}`, () => {
    const repo = makeRepo();
    mutate(repo);
    assert.throws(() => loadConfig(repo.top), (e) => e.exitCode === 2 && message.test(e.message));
  });
}

test('rejects a stateDir under the OS temp dir unless allowed for tests', () => {
  const { top } = makeRepo();
  const saved = process.env.MUTATION_ALLOW_TMP_STATE;
  delete process.env.MUTATION_ALLOW_TMP_STATE;
  try {
    assert.throws(() => loadConfig(top), /temp directory/);
  } finally {
    if (saved !== undefined) process.env.MUTATION_ALLOW_TMP_STATE = saved;
  }
});

test('accepts a repository outside the temp directory', () => {
  const base = new URL('../../../.e2e/', import.meta.url).pathname;
  mkdirSync(base, { recursive: true });
  const { top } = makeRepo({ base });
  const saved = process.env.MUTATION_ALLOW_TMP_STATE;
  delete process.env.MUTATION_ALLOW_TMP_STATE;
  try {
    assert.equal(loadConfig(top).stateRel, '.mutation');
  } finally {
    if (saved !== undefined) process.env.MUTATION_ALLOW_TMP_STATE = saved;
    rmSync(top, { recursive: true, force: true });
  }
});

test('accepts ignorePatterns with a /** suffix and a relative --config path', () => {
  const { top, write } = makeRepo();
  write('stryker.config.json', JSON.stringify({ testRunner: 'vitest', mutate: ['src/**/*.ts'], ignorePatterns: ['.mutation/**'], disableBail: true }));
  write('cfg/mi.json', JSON.stringify(baseConfig()));
  const c = loadConfig(top, 'cfg/mi.json');
  assert.ok(c.lists.global.includes('cfg/mi.json'));
});
