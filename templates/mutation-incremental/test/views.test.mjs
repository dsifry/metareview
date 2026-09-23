import { test } from 'node:test';
import assert from 'node:assert/strict';
import { resolveViews, viewFiles, viewSummaries } from '../lib/views.mjs';

const snapshot = { files: { 'src/a.ts': { category: 'mutate' }, 'src/b.ts': { category: 'mutate' }, 'tests/a.test.ts': { category: 'test' } } };
const config = (views) => ({ top: '/unused', views });
const ok = (stdout) => () => ({ status: 0, stdout });

test('resolveViews: absent, inline and command', () => {
  assert.equal(resolveViews(config(null), snapshot), null);
  assert.deepEqual(resolveViews(config({ inline: { u2: ['src/b.ts'], u1: ['src/**/*.ts'] } }), snapshot), { u1: ['src/**/*.ts'], u2: ['src/b.ts'] });
  assert.deepEqual(resolveViews(config({ command: ['x'] }), snapshot, { run: ok('{"u.1":["src/*.ts"]}') }), { 'u.1': ['src/*.ts'] });
});

test('resolveViews runs a real views command', () => {
  const command = [process.execPath, '-e', 'console.log(JSON.stringify({u:["src/**"]}))'];
  assert.deepEqual(resolveViews({ top: process.cwd(), views: { command } }, snapshot), { u: ['src/**'] });
});

test('resolveViews: command failures and bad shapes are exit 2', () => {
  const bad = [
    [() => ({ status: 1, stdout: '' }), /views command .* failed/],
    [() => ({ status: null, stdout: '', error: new Error('ETIMEDOUT') }), /ETIMEDOUT/],
    [ok('nope'), /did not print JSON/],
    [ok('[]'), /name → patterns/],
    [ok('{"u":["src/a.ts"],"u":["src/b.ts"]}'), /duplicate view name "u"/],
    [ok('{"":["src/a.ts"]}'), /invalid view name/],
    [ok('{"a b":["src/a.ts"]}'), /invalid view name/],
    [ok('{"u":"src/a.ts"}'), /array of patterns/],
  ];
  for (const [run, message] of bad) {
    assert.throws(() => resolveViews(config({ command: ['x'] }), snapshot, { run }), (e) => e.exitCode === 2 && message.test(e.message), String(message));
  }
});

test('resolveViews: a view matching a non-mutate file is exit 2', () => {
  assert.throws(() => resolveViews(config({ inline: { u: ['**/*.ts'] } }), snapshot), /view u matches tests\/a.test.ts/);
});

test('resolveViews: a mutate file in no view is exit 2', () => {
  assert.throws(() => resolveViews(config({ inline: { u: ['src/a.ts'] } }), snapshot), (e) => e.exitCode === 2 && /src\/b.ts belongs to no view/.test(e.message));
});

test('viewFiles resolves each view to its files', () => {
  assert.deepEqual(viewFiles({ u1: ['src/**'], u2: ['src/b.ts'] }, snapshot), { u1: ['src/a.ts', 'src/b.ts'], u2: ['src/b.ts'] });
});

test('viewSummaries: overlapping views count a mutant in both; counted beats inherited', () => {
  const report = { files: {
    'src/a.ts': { mutants: [{ status: 'Killed' }, { status: 'Survived' }] },
    'src/b.ts': { mutants: [{ status: 'Killed' }, { status: 'Killed' }] },
    'src/c.ts': {},
  } };
  const classified = [
    { reason: 'global input changed: x', paths: ['*'], inherited: true },
    { reason: 'no reachable tests: src/b.ts', paths: ['src/b.ts'], inherited: false },
  ];
  assert.deepEqual(viewSummaries(report, { u1: ['src/**'], u2: ['src/b.ts'] }, classified), {
    u1: { Killed: 3, Survived: 1, pendingCounted: 2, pendingInherited: 1 },
    u2: { Killed: 2, pendingCounted: 2, pendingInherited: 0 },
  });
  assert.deepEqual(viewSummaries(report, { u: ['src/a.ts'] }, []), { u: { Killed: 1, Survived: 1, pendingCounted: 0, pendingInherited: 0 } });
  assert.equal(viewSummaries(report, null, classified), null);
});
