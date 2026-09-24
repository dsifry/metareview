import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdirSync, mkdtempSync, symlinkSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { loadConfig } from '../lib/config.mjs';
import { takeSnapshot } from '../lib/snapshot.mjs';
import { specifiers, buildGraph, importerTests, openImporters, reachesFrom } from '../lib/graph.mjs';
import { makeRepo } from './helpers.mjs';

test('specifiers finds runtime imports and skips type-only and vi.mock', () => {
  const src = [
    "import a from './a';",
    "import type { T } from './types';",
    'export { b } from "./b";',
    "export type { U } from './u';",
    "import './side';",
    "const c = await import('./c');",
    "const d = require('./d');",
    "vi.mock('./mocked');",
    "const x = obj.import('./not');",
    'export type Id = string',
    "export { h } from './h'",
  ].join('\n');
  assert.deepEqual(specifiers(src).sort(), ['./a', './b', './c', './d', './h', './side']);
});

function graphRepo(extra = {}) {
  const r = makeRepo({
    config: { aliases: { '@app/': 'src/' }, ...extra },
    files: {
      'src/a.ts': 'export const a = 1;',
      'src/b.ts': "import { a } from './a.js';\nexport const b = a;",
      'src/c.ts': 'export const c = 3;',
      'src/idx/index.ts': "export * from '../a';",
      'tests/helpers/make.ts': 'export const make = 1;',
      'tests/a.test.ts': "import { a } from '../src/a';\nimport assert from 'node:assert';\nimport { test } from 'vitest';",
      'tests/b.test.ts': "import { b } from '../src/b';\nimport { make } from './helpers/make';\nimport fs from 'fs';",
      'tests/c.test.ts': "import { c } from '@app/c';\nimport data from './data.json';",
      'tests/data.json': '{}',
      'tests/i.test.ts': "import { a } from '../src/idx';",
    },
  });
  mkdirSync(join(r.top, 'node_modules/vitest'), { recursive: true });
  return r;
}

test('installed packages and builtins are not open importers', () => {
  const r = graphRepo();
  const cfg = loadConfig(r.top);
  const snap = takeSnapshot(cfg);
  const g = buildGraph(snap.files, cfg);
  assert.deepEqual(openImporters(g), []);
  assert.deepEqual([...importerTests(g, snap.files, 'src/a.ts')].sort(), ['tests/a.test.ts', 'tests/b.test.ts', 'tests/i.test.ts']);
  assert.deepEqual([...importerTests(g, snap.files, 'tests/helpers/make.ts')], ['tests/b.test.ts']);
  assert.deepEqual([...importerTests(g, snap.files, 'src/c.ts')], ['tests/c.test.ts']);
  assert.deepEqual([...importerTests(g, snap.files, 'tests/data.json')], ['tests/c.test.ts']);
  assert.deepEqual([...reachesFrom(g, 'tests/b.test.ts')].sort(), ['src/a.ts', 'src/b.ts', 'tests/b.test.ts', 'tests/helpers/make.ts']);
});

test('an npm link to a directory outside node_modules makes an open importer', () => {
  const r = graphRepo();
  const linked = mkdtempSync(join(tmpdir(), 'mi-linked-'));
  mkdirSync(join(r.top, 'node_modules/@org'), { recursive: true });
  symlinkSync(linked, join(r.top, 'node_modules/@org/linked'));
  r.write('src/l.ts', "import x from '@org/linked';");
  const cfg = loadConfig(r.top);
  const snap = takeSnapshot(cfg);
  assert.deepEqual(openImporters(buildGraph(snap.files, cfg)), [{ path: 'src/l.ts', specifiers: ['@org/linked'] }]);
});

test('workspace links into the repo and unknown bare specifiers make open importers', () => {
  const r = graphRepo({ aliases: {} });
  mkdirSync(join(r.top, 'packages/ws'), { recursive: true });
  mkdirSync(join(r.top, 'node_modules/@org'), { recursive: true });
  symlinkSync(join(r.top, 'packages/ws'), join(r.top, 'node_modules/@org/ws'));
  r.write('src/w.ts', "import x from '@org/ws/deep';\nimport y from 'not-installed';");
  r.write('tests/w.test.ts', "import { w } from '../src/w';");
  const cfg = loadConfig(r.top);
  const snap = takeSnapshot(cfg);
  const g = buildGraph(snap.files, cfg);
  assert.deepEqual(openImporters(g), [
    { path: 'src/w.ts', specifiers: ['@org/ws/deep', 'not-installed'] },
    { path: 'tests/c.test.ts', specifiers: ['@app/c'] },
  ]);
  assert.deepEqual([...importerTests(g, snap.files, 'tests/helpers/make.ts')].sort(), ['tests/b.test.ts', 'tests/c.test.ts', 'tests/w.test.ts']);
});

test('relative and alias specifiers that resolve to no file in the snapshot make open importers', () => {
  const r = graphRepo();
  r.write('tests/u.test.ts', "import x from '../../outside';\nimport y from './missing';\nimport z from '@app/nope';");
  const cfg = loadConfig(r.top);
  const snap = takeSnapshot(cfg);
  const g = buildGraph(snap.files, cfg);
  assert.deepEqual(openImporters(g), [{ path: 'tests/u.test.ts', specifiers: ['../../outside', './missing', '@app/nope'] }]);
  assert.deepEqual([...importerTests(g, snap.files, 'src/c.ts')].sort(), ['tests/c.test.ts', 'tests/u.test.ts']);
});

test('the longest alias key wins', () => {
  const r = graphRepo({ aliases: { '@app/': 'lib/', '@app/c': 'src/c' } });
  const cfg = loadConfig(r.top);
  const snap = takeSnapshot(cfg);
  const g = buildGraph(snap.files, cfg);
  assert.deepEqual([...importerTests(g, snap.files, 'src/c.ts')], ['tests/c.test.ts']);
  assert.deepEqual(openImporters(g), []);
});

test('openImporters sorts by path and de-duplicates specifiers', () => {
  const open = new Map([['b.ts', ['y']], ['a.ts', ['x', 'x']], ['c.ts', ['z']]]);
  assert.deepEqual(openImporters({ open }), [
    { path: 'a.ts', specifiers: ['x'] },
    { path: 'b.ts', specifiers: ['y'] },
    { path: 'c.ts', specifiers: ['z'] },
  ]);
});

test('files over 1 MiB, symlinks and non-source files are not parsed', () => {
  const r = graphRepo();
  r.write('src/big.ts', `import './a';\n${'x'.repeat((1 << 20) + 1)}`);
  symlinkSync('b.ts', join(r.top, 'src/alias.ts'));
  const cfg = loadConfig(r.top);
  const snap = takeSnapshot(cfg);
  const g = buildGraph(snap.files, cfg);
  assert.equal([...(g.reverse.get('src/a.ts') ?? [])].includes('src/big.ts'), false);
  // An unparsed source file's imports are unknown, so it is an open importer (the conservative side).
  assert.deepEqual(openImporters(g), [{ path: 'src/big.ts', specifiers: ['<source over 1 MiB>'] }]);
  assert.equal(g.forward.has('src/alias.ts') || g.open.has('src/alias.ts'), false);
  assert.equal(g.forward.has('tests/data.json') || g.open.has('tests/data.json'), false);
});

test('directory specifiers resolve to the directory index', () => {
  const r = graphRepo();
  r.write('src/idx/dot.test.ts', "import { a } from '.';");
  r.write('src/idx/slash.test.ts', "import { a } from './';");
  r.write('src/idx/sub/up.test.ts', "import { a } from '..';");
  r.write('src/idx.ts', 'export const sibling = 1;'); // a directory specifier never names this file
  r.write('index.ts', 'export const top = 1;');
  r.write('root.test.ts', "import { top } from '.';");
  mkdirSync(join(r.top, 'node_modules'), { recursive: true }); // '.' must not probe the repo's own node_modules
  const cfg = loadConfig(r.top);
  const snap = takeSnapshot(cfg);
  const g = buildGraph(snap.files, cfg);
  for (const t of ['src/idx/dot.test.ts', 'src/idx/slash.test.ts', 'src/idx/sub/up.test.ts']) {
    assert.deepEqual([...(g.forward.get(t) ?? [])], ['src/idx/index.ts'], t);
  }
  assert.deepEqual([...(g.forward.get('root.test.ts') ?? [])], ['index.ts']);
  assert.deepEqual(openImporters(g), []);
});

test('an alias key that prefixes a scoped package name does not hide the installed package', () => {
  // A Vite-style "@" alias: "@/a" is the alias, "@testing-library/react" is still a package.
  const r = graphRepo({ aliases: { '@': 'src' } });
  mkdirSync(join(r.top, 'node_modules/@testing-library/react'), { recursive: true });
  r.write('tests/r.test.ts', "import { render } from '@testing-library/react';\nimport { a } from '@/a';\nimport z from '@/nope';");
  const cfg = loadConfig(r.top);
  const snap = takeSnapshot(cfg);
  const g = buildGraph(snap.files, cfg);
  assert.deepEqual(openImporters(g), [
    { path: 'tests/c.test.ts', specifiers: ['@app/c'] },
    { path: 'tests/r.test.ts', specifiers: ['@/nope'] },
  ]);
  assert.ok(importerTests(g, snap.files, 'src/a.ts').has('tests/r.test.ts'));
});
