import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { compileGlob, matchList, categorize } from '../lib/glob.mjs';

const vectors = JSON.parse(readFileSync(new URL('../../../testdata/mutation-incremental/glob-vectors.json', import.meta.url), 'utf8'));

test('glob vectors: single patterns', () => {
  for (const v of vectors.match) {
    assert.equal(compileGlob(v.pattern).test(v.path), v.match, `${v.pattern} vs ${v.path}`);
  }
});

test('glob vectors: lists with negation', () => {
  for (const v of vectors.list) assert.equal(matchList(v.path, v.list), v.match, JSON.stringify(v));
});

test('glob vectors: categorize precedence', () => {
  const { lists, cases } = vectors.categorize;
  for (const c of cases) assert.equal(categorize(c.path, lists), c.category, c.path);
});

test('compileGlob caches and handles unclosed braces literally', () => {
  assert.equal(compileGlob('a/{b'), compileGlob('a/{b'));
  assert.equal(compileGlob('a/{b').test('a/{b'), true);
  assert.equal(compileGlob('a**b').test('axxb'), true);
});
