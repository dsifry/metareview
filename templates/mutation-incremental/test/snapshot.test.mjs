import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdirSync, readFileSync, rmSync, symlinkSync } from 'node:fs';
import { join } from 'node:path';
import { loadConfig } from '../lib/config.mjs';
import { takeSnapshot, digestOf, sha256 } from '../lib/snapshot.mjs';
import { indexReport } from '../lib/report.mjs';
import { makeRepo } from './helpers.mjs';

test('snapshot classifies, excludes and records tracked', () => {
  const r = makeRepo({ files: { 'src/a.ts': 'a', 'tests/a.test.ts': 't', 'README.md': 'x', 'scripts/x.sh': 's' } });
  r.write('src/new.ts', 'n');
  r.write('.mutation/attestation.json', '{}');
  r.write('reports/mutation/mutation.json', '{}'); // not gitignored: excluded by the resolved exclusions
  const snap = takeSnapshot(loadConfig(r.top));
  assert.equal(snap.files['src/a.ts'].category, 'mutate');
  assert.equal(snap.files['src/a.ts'].digest, `sha256:${sha256('a')}`);
  assert.equal(snap.files['src/a.ts'].tracked, true);
  assert.equal(snap.files['src/new.ts'].tracked, false);
  assert.equal(snap.files['tests/a.test.ts'].category, 'test');
  assert.equal(snap.files['scripts/x.sh'].category, 'unclassified');
  assert.equal(snap.files['README.md'], undefined);
  assert.equal(snap.files['.mutation/attestation.json'], undefined);
  assert.equal(snap.files['reports/mutation/mutation.json'], undefined);
  assert.equal(snap.files['mutation-incremental.json'].category, 'global');
});

test('snapshot hashes symlinks by link text and skips gitlinks and deleted tracked files', () => {
  const r = makeRepo({ files: { 'src/a.ts': 'a' } });
  symlinkSync('a.ts', join(r.top, 'src/link.ts'));
  r.git('update-index', '--add', '--cacheinfo', `160000,${'1'.repeat(40)},vendor/sub`);
  r.write('src/gone.ts', 'g');
  r.git('add', 'src/gone.ts');
  rmSync(join(r.top, 'src/gone.ts')); // tracked in the index, deleted from the working tree
  const snap = takeSnapshot(loadConfig(r.top));
  assert.equal(snap.files['src/link.ts'].digest, `symlink:${sha256('a.ts')}`);
  assert.equal(snap.files['vendor/sub'], undefined);
  assert.equal(snap.files['src/gone.ts'], undefined);
});

test('digestOf returns null for directories and missing files, rethrows other errors', () => {
  const r = makeRepo();
  mkdirSync(join(r.top, 'dir'));
  assert.equal(digestOf(join(r.top, 'dir')), null);
  assert.equal(digestOf(join(r.top, 'missing')), null);
  assert.throws(() => digestOf('\0bad'));
});

test('runtime map hashes command output and env values', () => {
  const r = makeRepo({ config: { runtime: { commands: [['node', '--version']], env: ['MI_TEST_IMAGE', 'MI_UNSET'] } } });
  process.env.MI_TEST_IMAGE = 'pg16';
  delete process.env.MI_UNSET;
  const snap = takeSnapshot(loadConfig(r.top), { runCommand: () => Buffer.from('v22.9.0\n') });
  assert.equal(snap.runtime['cmd:node\u001f--version'], sha256('v22.9.0\n'));
  assert.equal(snap.runtime['env:MI_TEST_IMAGE'], sha256('pg16'));
  assert.equal(snap.runtime['env:MI_UNSET'], 'unset');
});

test('a failing runtime command is a usage error', () => {
  const r = makeRepo({ config: { runtime: { commands: [['definitely-not-a-command-xyz']], env: [] } } });
  assert.throws(() => takeSnapshot(loadConfig(r.top)), (e) => e.exitCode === 2);
});

test('indexReport flattens mutants and test ids', () => {
  const idx = indexReport({
    files: { 'src/a.ts': { mutants: [{ id: 1, status: 'Killed', killedBy: [0], coveredBy: [0, 1], location: { start: { line: 2 }, end: { line: 3 } } }, { id: '2', status: 'NoCoverage', location: { start: { line: 4 }, end: { line: 4 } } }] } },
    testFiles: { 'tests/a.test.ts': { tests: [{ id: 0 }, { id: '1' }] }, 'tests/b.test.ts': {} },
  });
  assert.deepEqual(idx.mutants[0], { file: 'src/a.ts', id: '1', status: 'Killed', killedBy: ['0'], coveredBy: ['0', '1'], static: false, startLine: 2, endLine: 3 });
  assert.deepEqual(idx.mutants[1].killedBy, []);
  assert.deepEqual(idx.testIds, { 'tests/a.test.ts': ['0', '1'], 'tests/b.test.ts': [] });
  assert.deepEqual(indexReport({}), { mutants: [], testIds: {} });
  assert.deepEqual(indexReport({ files: { 'x.ts': {} } }).mutants, []);
});

test('the config digest ignores the inline views map and tracks every other key', () => {
  // Spec K3.4, §11.3: assigning a new file to a view invalidates no kill, so it is no global change.
  const r = makeRepo({ config: { views: { inline: { core: ['src/**'] } } } });
  const cfg = JSON.parse(readFileSync(join(r.top, 'mutation-incremental.json'), 'utf8'));
  const digest = () => takeSnapshot(loadConfig(r.top)).files['mutation-incremental.json'].digest;
  const before = digest();
  r.write('mutation-incremental.json', JSON.stringify({ ...cfg, views: { inline: { core: ['src/**'], ui: ['src/new.ts'] } } }, null, 2));
  assert.equal(digest(), before);
  r.write('mutation-incremental.json', JSON.stringify({ ...cfg, budget: { ...cfg.budget, maxForcedShare: 0.5 } }));
  assert.notEqual(digest(), before);
});
