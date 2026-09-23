import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { mergeReports } from '../lib/seed.mjs';
import { readCandidate } from '../lib/state.mjs';
import { A, cli, reportFor, runRepo } from './fake.mjs';

const at = (line) => ({ start: { line, column: 1 }, end: { line, column: 5 } });
const mu = (id, mutatorName, replacement, status, line, killedBy, coveredBy) => ({ id, mutatorName, replacement, status, killedBy, coveredBy, location: at(line) });
// X and Y share the line-2 and line-3 mutants; line 1 holds three mutants at one position (two
// mutators, two replacements), as Stryker produces for one expression.
const X = {
  schemaVersion: '1',
  files: { 'src/a.ts': { language: 'typescript', source: A, mutants: [
    mu('9', 'M', 'r', 'Survived', 2, undefined, ['x1']),
    mu('8', 'M', 'r', 'CompileError', 3, undefined, undefined),
  ] } },
  testFiles: { 'tests/a.test.ts': { tests: [{ id: 'x1', name: 'low' }] } },
};
const Y = {
  schemaVersion: '1',
  files: { 'src/a.ts': { language: 'typescript', source: A, mutants: [
    mu('3', 'M', 'r', 'Killed', 2, ['y2'], ['y2', 'y1']),
    mu('4', 'N', 's', 'NoCoverage', 1, undefined, []),
    mu('5', 'N', 'r', 'NoCoverage', 1, undefined, []),
    mu('6', 'M', 'r', 'NoCoverage', 1, undefined, []),
    mu('7', 'M', 'r', 'Survived', 3, undefined, ['y1']),
  ] } },
  testFiles: { 'tests/a.test.ts': { tests: [{ id: 'y1', name: 'high' }, { id: 'y2', name: 'low' }] } },
};

test('mergeReports re-keys tests by (file, name), merges mutants and renumbers both', () => {
  for (const order of [[X, Y], [Y, X]]) {
    const merged = mergeReports(order);
    assert.deepEqual(merged.testFiles, { 'tests/a.test.ts': { tests: [{ id: '0', name: 'high' }, { id: '1', name: 'low' }] } });
    const mutants = merged.files['src/a.ts'].mutants.map((m) => [m.id, m.location.start.line, m.mutatorName, m.replacement, m.status, m.killedBy, m.coveredBy]);
    assert.deepEqual(mutants, [
      ['0', 1, 'M', 'r', 'NoCoverage', [], []],
      ['1', 1, 'N', 'r', 'NoCoverage', [], []],
      ['2', 1, 'N', 's', 'NoCoverage', [], []],
      ['3', 2, 'M', 'r', 'Killed', ['1'], ['0', '1']],
      ['4', 3, 'M', 'r', 'Survived', [], ['0']],
    ]);
    assert.equal(merged.files['src/a.ts'].source, A);
  }
});

test('seed bootstraps a state whose kills are pending until a full run', async () => {
  const r = runRepo({ config: { views: { inline: { core: ['src/**'] } } } });
  writeFileSync(join(r.top, '.fake/full.json'), JSON.stringify(reportFor()));
  const res = await cli(r, ['seed', '--from', '.fake/full.json']);
  assert.equal(res.code, 0);
  assert.equal(res.stdout, 'seed: score=50 thresholdBreak=false\n');
  assert.match(res.stderr, /command=seed invocations=0 scope=0 forced=0 deferrals=1 pending_full=true\n$/);
  const att = r.read('.mutation/attestation.json');
  assert.deepEqual([att.mode, att.lastFullAt], ['seed', null]);
  assert.deepEqual(att.deferrals, [{ reason: 'seeded from .fake/full.json', paths: ['*'], inherited: false }]);
  assert.deepEqual(att.viewSummaries.core, { Killed: 1, Survived: 1, pendingCounted: 1, pendingInherited: 0 });
  assert.equal(readCandidate(join(r.top, '.mutation'), { label: 's', primary: true }).usable, true);
  assert.deepEqual(r.read('.mutation/incremental.json').testFiles['tests/a.test.ts'].tests, [{ id: '0', name: 'clamps low' }]);
});

test('seed refuses usable state unless --replace, which first keeps a copy', async () => {
  const r = runRepo();
  writeFileSync(join(r.top, '.fake/full.json'), JSON.stringify(reportFor()));
  const empty = await cli(r, ['seed', '--from', '.fake/full.json', '--replace']); // nothing to keep yet
  assert.equal(empty.code, 0);
  const again = await cli(r, ['seed', '--from', '.fake/full.json']);
  assert.equal(again.code, 2);
  assert.match(again.stderr, /usable state exists in \.mutation; pass --replace/);
  assert.equal((await cli(r, ['seed', '--from', '.fake/full.json', '--replace'])).code, 0);
  const kept = readdirSync(join(r.top, '.mutation/replaced')).sort();
  assert.equal(kept.length, 2);
  assert.deepEqual(readdirSync(join(r.top, '.mutation/replaced', kept[0])), []);
  assert.deepEqual(readdirSync(join(r.top, '.mutation/replaced', kept[1])).sort(), ['attestation.json', 'incremental.json']);
});

test('seed input errors are exit 2', async () => {
  const r = runRepo();
  writeFileSync(join(r.top, '.fake/bad.json'), '{"files":[]}');
  writeFileSync(join(r.top, '.fake/x.json'), JSON.stringify(reportFor()));
  writeFileSync(join(r.top, '.fake/y.json'), JSON.stringify(reportFor(A.replace('lo;', 'lo ;'))));
  const cases = [
    [['seed'], /seed needs --from/],
    [['seed', '--from', '.fake/missing.json'], /seed: cannot read \.fake\/missing\.json/],
    [['seed', '--from', '.fake/bad.json'], /\.fake\/bad\.json is not a mutation-testing report/],
    [['seed', '--from', '.fake/x.json', '--from', '.fake/y.json'], /src\/a\.ts has a different source in the given reports/],
  ];
  for (const [args, message] of cases) {
    const res = await cli(r, args);
    assert.equal(res.code, 2, args.join(' '));
    assert.match(res.stderr, message);
  }
});
