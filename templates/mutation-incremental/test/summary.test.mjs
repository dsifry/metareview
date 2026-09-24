import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, rmSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { summaryMarkdown } from '../lib/summary.mjs';
import { writeState } from './helpers.mjs';
import { cli, reportFor, runRepo, warm } from './fake.mjs';

const config = (extra = {}) => ({ thresholdBreak: null, pendingOnPr: 'allow', ...extra });
const usable = (attestation) => ({ usable: true, attestation: { score: 50, completedAt: 'c', lastFullAt: 'l', deferrals: [], ...attestation } });
const unusable = { usable: false };
const md = (extra) => summaryMarkdown({ job: 'main', exitCode: '0', config: config(), state: usable({}), remotes: [unusable, unusable], pr: false, ...extra });

test('an empty exit code says only that the run step did not finish', () => {
  assert.equal(md({ exitCode: '' }), '### Mutation testing (main)\n\n- run step did not execute or did not finish (see earlier step)\n');
});

test('no usable state, the score, the threshold and the previous-state label', () => {
  assert.match(md({ state: unusable }), /- no usable state\n/);
  assert.match(md({}), /- score: 50\.00 \(no thresholds\.break\)\n/);
  assert.match(md({ config: config({ thresholdBreak: 60 }) }), /- score: 50\.00, below thresholds\.break 60\n/);
  assert.match(md({ config: config({ thresholdBreak: 40 }) }), /- score: 50\.00, at or above thresholds\.break 40\n/);
  assert.match(md({ state: usable({ score: null }) }), /- score: n\/a/);
  assert.match(md({ exitCode: '4' }), /- score: 50\.00 \(no thresholds\.break\) — previous state \(this run committed nothing\)\n/);
});

test('pending kills, counted and inherited deferrals as code spans', () => {
  const deferrals = [
    { reason: 'global input changed: package-lock.json', paths: ['*'], inherited: true },
    { reason: 'time budget exceeded', paths: ['src/a.ts', 'src/`b`.ts'], inherited: false },
  ];
  const text = md({ state: usable({ deferrals }) });
  assert.match(text, /- pending full run: true\n/);
  assert.match(text, /\(includes pending kills\)/);
  assert.match(text, /- deferred: `time budget exceeded` on `src\/a\.ts`, `src\/'b'\.ts`\n/);
  assert.match(text, /- inherited from main, cleared by main's full run: `global input changed: package-lock\.json` on `\*`\n/);
});

test('routed PR runs: the sweep, an unreachable module (and whether main has it)', () => {
  const routed = config({ pendingOnPr: 'full-on-global' });
  const timeout = usable({ deferrals: [{ reason: 'time budget exceeded', paths: ['src/a.ts'], inherited: false }] });
  assert.match(md({ job: 'pr', config: routed, state: timeout, pr: true }), /- verdict deferred to pr-full\n/);
  assert.doesNotMatch(md({ job: 'pr', config: routed, state: timeout, pr: true, exitCode: '4' }), /verdict deferred/);
  const gone = { reason: 'no reachable tests: src/c.ts', paths: ['src/c.ts'], inherited: false };
  const state = usable({ deferrals: [gone] });
  const onMain = usable({ deferrals: [{ reason: 'no reachable tests: src/c.ts', paths: ['src/c.ts'] }] });
  assert.match(md({ job: 'pr', config: routed, state, pr: true }), /- `no reachable tests: src\/c\.ts`: add a test or remove the module\n/);
  assert.match(md({ job: 'pr', config: routed, state, pr: true, remotes: [onMain, unusable] }), /add a test or remove the module \(also on main\)\n/);
});

test("the PR job shows main's score and the age of mutation-state/full", () => {
  assert.match(md({ job: 'pr' }), /- main: main state unavailable\n$/);
  const inc = usable({ score: 90, deferrals: [{ reason: 'no usable state', paths: ['*'] }] });
  const full = usable({ score: 80, completedAt: '2026-09-23T10:00:00.000Z', lastFullAt: '2026-09-22T10:00:00.000Z' });
  const text = md({ job: 'pr', remotes: [inc, full] });
  assert.match(text, /- main \(mutation-state\/inc\): score 90\.00 \(no thresholds\.break\) \(includes pending kills\)\n/);
  assert.match(text, /- mutation-state\/full: completed 2026-09-23T10:00:00\.000Z, last full run 2026-09-22T10:00:00\.000Z\n/);
  assert.match(md({ job: 'pr', remotes: [unusable, full] }), /- main \(mutation-state\/full\): score 80\.00/);
  assert.doesNotMatch(md({ job: 'main', remotes: [inc, full] }), /main \(/);
});

test('summary appends to GITHUB_STEP_SUMMARY, or prints without it; bad --job is exit 2', async () => {
  const r = runRepo();
  await warm(r);
  writeState(join(r.top, '.mutation/remote/full'), { report: reportFor(), attestation: { completedAt: 'c2', lastFullAt: 'l2', score: 50 } });
  const file = join(r.top, '.fake/summary.md');
  writeFileSync(file, 'before\n');
  const saved = process.env.GITHUB_STEP_SUMMARY; // set on GitHub runners
  try {
    process.env.GITHUB_STEP_SUMMARY = file;
    const res = await cli(r, ['summary', '--job', 'pr', '--exit-code', '0']);
    assert.equal(res.code, 0);
    assert.match(res.stderr, /command=summary invocations=0 scope=0 forced=0 deferrals=0 pending_full=false\n$/);
    const text = readFileSync(file, 'utf8');
    assert.match(text, /^before\n### Mutation testing \(pr\)\n/);
    assert.match(text, /mutation-state\/full: completed c2, last full run l2/);
    delete process.env.GITHUB_STEP_SUMMARY;
    rmSync(join(r.top, '.mutation/attestation.json'));
    const printed = await cli(r, ['summary', '--job', 'main']);
    assert.equal(printed.stdout, '### Mutation testing (main)\n\n- run step did not execute or did not finish (see earlier step)\n');
    assert.equal((await cli(r, ['summary', '--job', 'nope'])).code, 2);
  } finally {
    if (saved === undefined) delete process.env.GITHUB_STEP_SUMMARY;
    else process.env.GITHUB_STEP_SUMMARY = saved;
  }
});
