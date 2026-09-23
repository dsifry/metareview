import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, writeFileSync, mkdirSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { readCandidate, changeSet, adopt, STATE_VERSION, TOOL } from '../lib/state.mjs';
import { sha256 } from '../lib/snapshot.mjs';
import { writeState } from './helpers.mjs';

const dir = () => mkdtempSync(join(tmpdir(), 'mi-state-'));
const opts = { label: 'x', primary: false };

test('readCandidate: usable pair', () => {
  const d = dir();
  writeState(d, { report: { files: {} } });
  const c = readCandidate(d, opts);
  assert.equal(c.usable, true);
  assert.deepEqual(c.report, { files: {} });
  assert.equal(c.attestation.tool, TOOL);
});

test('readCandidate: each unusable reason', () => {
  const missing = readCandidate(join(dir(), 'nope'), opts);
  assert.deepEqual([missing.usable, missing.reason, missing.attestation], [false, 'missing attestation', null]);

  const bad = dir();
  writeFileSync(join(bad, 'attestation.json'), '{');
  assert.equal(readCandidate(bad, opts).reason, 'unparseable attestation');

  const arr = dir();
  writeFileSync(join(arr, 'attestation.json'), '[]');
  assert.equal(readCandidate(arr, opts).reason, 'unparseable attestation');

  const tool = dir();
  writeState(tool, { report: {}, attestation: { tool: 'other' } });
  assert.equal(readCandidate(tool, opts).reason, 'not a harness attestation');

  const version = dir();
  writeState(version, { report: {}, attestation: { stateVersion: STATE_VERSION + 1 } });
  const v = readCandidate(version, opts);
  assert.equal(v.reason, 'stateVersion mismatch');
  assert.equal(v.attestation.stateVersion, STATE_VERSION + 1);

  const noVersion = dir();
  writeState(noVersion, { report: {}, attestation: { stateVersion: undefined } });
  assert.equal(readCandidate(noVersion, opts).reason, 'stateVersion mismatch');

  const shape = dir();
  writeState(shape, { report: {}, attestation: { deferrals: 'x' } });
  assert.equal(readCandidate(shape, opts).reason, 'not a harness attestation');

  const noReport = dir();
  writeFileSync(join(noReport, 'attestation.json'), JSON.stringify({ schemaVersion: 1, tool: TOOL, stateVersion: STATE_VERSION, files: {}, runtime: {}, deferrals: [] }));
  assert.equal(readCandidate(noReport, opts).reason, 'missing report');

  const hash = dir();
  writeState(hash, { report: {}, attestation: { reportSha256: '0' } });
  assert.equal(readCandidate(hash, opts).reason, 'report hash mismatch');

  const junk = dir();
  mkdirSync(junk, { recursive: true });
  writeFileSync(join(junk, 'incremental.json'), 'nope');
  writeFileSync(join(junk, 'attestation.json'), JSON.stringify({ schemaVersion: 1, tool: TOOL, stateVersion: STATE_VERSION, files: {}, runtime: {}, deferrals: [], reportSha256: sha256('nope') }));
  assert.equal(readCandidate(junk, opts).reason, 'unparseable report');
});

const snap = (files, runtime = {}) => ({ files, runtime });
const f = (digest, category = 'mutate') => ({ digest, category, tracked: true });

test('changeSet: changed, new, deleted, category change, runtime', () => {
  const att = { files: { 'a.ts': f('1'), 'b.ts': f('1'), 'c.ts': f('1'), 'd.ts': f('1', 'support') }, runtime: { 'env:X': 'x', 'env:GONE': 'g', 'env:SAME': 's' } };
  const s = snap({ 'a.ts': f('1'), 'b.ts': f('2'), 'd.ts': f('1', 'test'), 'e.ts': f('1') }, { 'env:X': 'y', 'env:NEW': 'n', 'env:SAME': 's' });
  assert.deepEqual(changeSet(att, s), [
    { path: 'b.ts', category: 'mutate', kind: 'changed' },
    { path: 'c.ts', category: 'mutate', kind: 'deleted' },
    { path: 'd.ts', category: 'support', kind: 'deleted' },
    { path: 'd.ts', category: 'test', kind: 'new' },
    { path: 'e.ts', category: 'mutate', kind: 'new' },
    { path: 'env:GONE', category: 'global', kind: 'deleted' },
    { path: 'env:NEW', category: 'global', kind: 'new' },
    { path: 'env:X', category: 'global', kind: 'changed' },
  ]);
  assert.deepEqual(changeSet({ files: {}, runtime: {} }, snap({})), []);
});

test('adopt: no deferrals first, then smallest change set, then primary, then order', () => {
  const s = snap({ 'a.ts': f('2'), 'b.ts': f('1') });
  const cand = (label, primary, files, deferrals = []) => ({ label, primary, usable: true, attestation: { files, runtime: {}, deferrals } });
  const near = { 'a.ts': f('2'), 'b.ts': f('1') };
  const far = { 'a.ts': f('1'), 'b.ts': f('0') };
  assert.equal(adopt([cand('p', true, near, [{ reason: 'x', paths: ['*'] }]), cand('full', false, far)], s).label, 'full');
  assert.equal(adopt([cand('p', true, far), cand('inc', false, near)], s).label, 'inc');
  assert.equal(adopt([cand('p', true, near), cand('inc', false, near)], s).label, 'p');
  assert.equal(adopt([cand('a', false, near), cand('b', false, near)], s).label, 'a');
  assert.deepEqual(adopt([cand('p', true, near)], s).changes, []);
  assert.equal(adopt([{ label: 'u', usable: false }], s), null);
  assert.equal(adopt([], s), null);
});
