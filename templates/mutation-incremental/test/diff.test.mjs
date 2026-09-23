import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { lineDiff, hunkIntersects, splitLines, normalizeText, MAX_EDIT_DISTANCE } from '../lib/diff.mjs';

const vectors = JSON.parse(readFileSync(new URL('../../../testdata/mutation-incremental/diff-vectors.json', import.meta.url), 'utf8'));

test('diff vectors', () => {
  assert.equal(vectors.maxEditDistance, MAX_EDIT_DISTANCE);
  for (const v of vectors.cases) assert.deepEqual(lineDiff(v.old, v.new).hunks, v.hunks, v.name);
});

test('normalisation and line splitting', () => {
  assert.equal(normalizeText('\ufeffa\r\nb\rc'), 'a\nb\nc');
  assert.deepEqual(splitLines(''), []);
  assert.deepEqual(splitLines('a\n\nb\n'), ['a', '', 'b']);
});

test('oldToNew maps unchanged lines across an insertion', () => {
  const { oldToNew } = lineDiff('a\nb\nc\n', 'x\na\nb\nc\n');
  assert.deepEqual(oldToNew, [null, 2, 3, 4]);
  const del = lineDiff('a\nb\nc\n', 'a\nc\n');
  assert.deepEqual(del.oldToNew, [null, 1, null, 2]);
});

test('hunkIntersects: overlap for a changed range, containment for an insertion point', () => {
  const changed = { oldStart: 3, oldEnd: 3, newStart: 3, newEnd: 3 };
  assert.equal(hunkIntersects(1, 5, changed), true);
  assert.equal(hunkIntersects(2, 2, changed), false);
  const insertAfter3 = { oldStart: 4, oldEnd: 3, newStart: 4, newEnd: 4 };
  assert.equal(hunkIntersects(1, 5, insertAfter3), true);
  assert.equal(hunkIntersects(1, 3, insertAfter3), false);
  assert.equal(hunkIntersects(4, 5, insertAfter3), false);
});

test('above MAX_EDIT_DISTANCE lineDiff returns null (the planner treats the file as whole)', () => {
  const a = Array.from({ length: MAX_EDIT_DISTANCE + 1 }, (_, i) => `a${i}`).join('\n');
  const b = Array.from({ length: MAX_EDIT_DISTANCE + 1 }, (_, i) => `b${i}`).join('\n');
  assert.equal(lineDiff(a, b), null);
});
