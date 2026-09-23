import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { loadConfig } from '../lib/config.mjs';
import { sha256, takeSnapshot } from '../lib/snapshot.mjs';
import { buildAttestation, CHANGED_DURING_RUN, commitState, installAdopted, markChangedDuringRun, mutationScore, WORK_DIR } from '../lib/attest.mjs';
import { readCandidate, STATE_VERSION } from '../lib/state.mjs';
import { makeRepo, writeState } from './helpers.mjs';

const m = (status) => ({ status });
const report = { files: { 'src/a.ts': { mutants: [m('Killed'), m('Survived')] } } };

function fixture() {
  const r = makeRepo({ files: { 'src/a.ts': 'a', 'src/b.ts': 'b' } });
  const config = loadConfig(r.top);
  return { r, config, snapshot: takeSnapshot(config) };
}
const build = (config, snapshot, extra = {}) => buildAttestation({
  config, files: snapshot.files, runtime: snapshot.runtime, reportBytes: Buffer.from('{}'), report, mode: 'incremental',
  completedAt: 'c', lastFullAt: 'l', deferrals: [], views: null, summaries: null, ...extra,
});

test('mutationScore follows Stryker: Timeout counts as detected, other statuses are ignored', () => {
  assert.equal(mutationScore({ files: { a: { mutants: [m('Killed'), m('Timeout'), m('Survived'), m('NoCoverage'), m('CompileError'), m('Ignored')] } } }), 50);
  assert.equal(mutationScore({ files: { a: { mutants: [m('CompileError')] }, b: {} } }), null);
});

test('buildAttestation writes the §5.5 contract', () => {
  const { config, snapshot } = fixture();
  const att = build(config, snapshot);
  assert.deepEqual(Object.keys(att).sort(), [
    'completedAt', 'deferrals', 'engine', 'engineVersion', 'exclusions', 'files', 'lastFullAt', 'lists', 'mode', 'report',
    'reportSha256', 'runtime', 'schemaVersion', 'score', 'stateVersion', 'thresholdBreak', 'tool', 'toolVersion',
  ]);
  assert.deepEqual(
    [att.schemaVersion, att.tool, att.toolVersion, att.stateVersion, att.engine, att.engineVersion, att.report, att.reportSha256, att.score],
    [1, 'metareview-mutation-incremental', '0.13.0', STATE_VERSION, 'stryker', '10.0.0', 'incremental.json', sha256('{}'), 50],
  );
  assert.equal(att.thresholdBreak, false); // no thresholds.break configured
  assert.deepEqual([att.files, att.lists, att.exclusions], [snapshot.files, config.lists, config.exclusions]);
});

test('thresholdBreak: strictly below the break, never for a null score', () => {
  const { config, snapshot } = fixture();
  assert.equal(build({ ...config, thresholdBreak: 60 }, snapshot).thresholdBreak, true);
  assert.equal(build({ ...config, thresholdBreak: 50 }, snapshot).thresholdBreak, false);
  assert.equal(build({ ...config, thresholdBreak: 60 }, snapshot, { report: { files: {} } }).thresholdBreak, false);
});

test('views and viewSummaries are written only with views', () => {
  const { config, snapshot } = fixture();
  const att = build(config, snapshot, { views: { core: ['src/**'] }, summaries: { core: { Killed: 1 } } });
  assert.deepEqual([att.views, att.viewSummaries], [{ core: ['src/**'] }, { core: { Killed: 1 } }]);
});

test('markChangedDuringRun flags edited and deleted files and keeps the rest', () => {
  const { r, config, snapshot } = fixture();
  r.write('src/a.ts', 'changed');
  rmSync(join(r.top, 'src/b.ts'));
  const files = markChangedDuringRun(config, snapshot.files);
  assert.deepEqual([files['src/a.ts'].digest, files['src/b.ts'].digest], [CHANGED_DURING_RUN, CHANGED_DURING_RUN]);
  assert.deepEqual(files['mutation-incremental.json'], snapshot.files['mutation-incremental.json']);
  assert.deepEqual(files['stryker.config.json'], snapshot.files['stryker.config.json']);
});

test('commitState renames the output and then the attestation into place and removes work/', () => {
  const { config, snapshot } = fixture();
  const work = join(config.stateDir, WORK_DIR);
  mkdirSync(work, { recursive: true });
  const bytes = Buffer.from(JSON.stringify(report));
  writeFileSync(join(work, 'inv-2.json'), bytes);
  commitState(config.stateDir, join(work, 'inv-2.json'), build(config, snapshot, { reportBytes: bytes }));
  assert.equal(readCandidate(config.stateDir, { label: 's', primary: true }).usable, true);
  assert.equal(existsSync(work), false);
  // An unchanged canonical output is only re-attested.
  commitState(config.stateDir, join(config.stateDir, 'incremental.json'), build(config, snapshot, { reportBytes: bytes, mode: 'full' }));
  assert.equal(JSON.parse(readFileSync(join(config.stateDir, 'attestation.json'), 'utf8')).mode, 'full');
  assert.equal(readCandidate(config.stateDir, { label: 's', primary: true }).usable, true);
});

test('installAdopted copies both files of the adopted state into stateDir', () => {
  const { r, config } = fixture();
  writeState(join(r.top, '.mutation/remote/full'), { report });
  installAdopted(config.stateDir, join(r.top, '.mutation/remote/full'));
  assert.equal(readCandidate(config.stateDir, { label: 's', primary: true }).usable, true);
});
