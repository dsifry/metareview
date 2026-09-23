import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdirSync, mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { computePlan, publicPlan, RESIDUAL_MAX_BYTES } from '../lib/plan.mjs';
import { changeSet, TOOL, STATE_VERSION } from '../lib/state.mjs';
import { dedupeDeferrals, sortDeferrals, deferralKey } from '../lib/deferrals.mjs';
import { MAX_EDIT_DISTANCE } from '../lib/diff.mjs';

// The spec §7.1 fixture in miniature (same line numbers), plus src/e.ts from §11.7.
const A = 'export function clamp(n, lo, hi) {\n  if (n < lo) return lo;\n  if (n > hi) return hi;\n  return n;\n}\n';
const B = "import { clamp } from './a';\nimport type { Range } from './types';\n\nexport const inRange = (n: number, r: Range) => clamp(n, r.lo, r.hi);\n";
const C = "import { LIMIT } from './limits';\nexport const twice = (n: number) => Math.min(n * 2, LIMIT);\n";
const E = 'export function inc(n) {\n  const r = n + 1;\n  return r;\n}\n\nexport function dec(n) {\n  const r = n - 1;\n  return r;\n}\n';
const TEXTS = {
  'src/a.ts': A, 'src/b.ts': B, 'src/c.ts': C, 'src/e.ts': E,
  'src/limits.ts': 'export const LIMIT = 100;\n',
  'src/types.ts': 'export interface Range { lo: number; hi: number }\n',
  'src/index.ts': "export * from './a';\n",
};
const CATEGORIES = {
  'src/a.ts': 'mutate', 'src/b.ts': 'mutate', 'src/c.ts': 'mutate', 'src/e.ts': 'mutate',
  'src/limits.ts': 'mutate', 'src/types.ts': 'mutate', 'src/index.ts': 'mutate',
  'tests/a.test.ts': 'test', 'tests/b.test.ts': 'test', 'tests/c.test.ts': 'test', 'tests/index.test.ts': 'test',
  'tests/e-inc.test.ts': 'test', 'tests/e-dec.test.ts': 'test',
  'tests/helpers/make.ts': 'support', 'tests/helpers/fmt.ts': 'support',
  'package-lock.json': 'global',
};
const EDGES = {
  'tests/a.test.ts': ['src/a.ts'],
  'tests/b.test.ts': ['src/b.ts', 'tests/helpers/make.ts'],
  'src/b.ts': ['src/a.ts'],
  'tests/c.test.ts': ['src/c.ts', 'tests/helpers/fmt.ts'],
  'src/c.ts': ['src/limits.ts'],
  'tests/index.test.ts': ['src/index.ts'],
  'src/index.ts': ['src/a.ts'],
  'tests/e-inc.test.ts': ['src/e.ts'],
  'tests/e-dec.test.ts': ['src/e.ts'],
};
const mk = (id, start, end, status, killedBy, coveredBy) => ({ id, status, killedBy, coveredBy, mutatorName: 'M', replacement: 'r', location: { start: { line: start, column: 1 }, end: { line: end, column: 2 } } });
function report() {
  return {
    files: {
      'src/a.ts': { source: A, mutants: [mk('a1', 1, 5, 'Killed', ['ta', 'ti'], ['ta', 'tb', 'ti']), mk('a2', 2, 2, 'Killed', ['ta', 'tb'], ['ta', 'tb', 'ti']), mk('a3', 3, 3, 'Killed', ['ta'], ['ta', 'tb'])] },
      'src/b.ts': { source: B, mutants: [mk('b1', 4, 4, 'Killed', ['tb'], ['tb'])] },
      'src/c.ts': { source: C, mutants: [mk('c1', 2, 2, 'Killed', ['tc'], ['tc'])] },
      'src/e.ts': { source: E, mutants: [mk('e1', 1, 4, 'Killed', ['tei'], ['tei']), mk('e2', 2, 2, 'Killed', ['tei'], ['tei']), mk('e3', 6, 9, 'Killed', ['ted'], ['ted']), mk('e4', 7, 7, 'Killed', ['ted'], ['ted'])] },
    },
    testFiles: {
      'tests/a.test.ts': { tests: [{ id: 'ta' }] }, 'tests/b.test.ts': { tests: [{ id: 'tb' }] }, 'tests/c.test.ts': { tests: [{ id: 'tc' }] },
      'tests/index.test.ts': { tests: [{ id: 'ti' }] }, 'tests/e-inc.test.ts': { tests: [{ id: 'tei' }] }, 'tests/e-dec.test.ts': { tests: [{ id: 'ted' }] },
    },
  };
}
function graphOf(edges, open) {
  const forward = new Map();
  const reverse = new Map();
  for (const [from, tos] of Object.entries(edges)) {
    for (const to of tos) {
      if (!forward.has(from)) forward.set(from, new Set());
      forward.get(from).add(to);
      if (!reverse.has(to)) reverse.set(to, new Set());
      reverse.get(to).add(from);
    }
  }
  return { forward, reverse, open: new Map(Object.entries(open)) };
}
// edit: {path: newText}; remove: [path]; add: {path: {category, text?, digest?}} (new since the state);
// base: {path: {category, text?}} (present in the state and unchanged unless also edited).
function scenario({ edit = {}, remove = [], add = {}, base = {}, config = {}, edges = {}, open = {}, deferrals = [], mutate } = {}) {
  const texts = { ...TEXTS };
  const baseFiles = {};
  for (const [p, category] of Object.entries(CATEGORIES)) baseFiles[p] = { digest: `sha256:${p}`, category, tracked: true };
  for (const [p, { category, text = '' }] of Object.entries(base)) {
    texts[p] = text;
    baseFiles[p] = { digest: `sha256:${p}`, category, tracked: true };
  }
  const files = structuredClone(baseFiles);
  for (const [p, text] of Object.entries(edit)) {
    texts[p] = text;
    files[p] = { ...files[p], digest: `sha256:${p}:edited` };
  }
  for (const p of remove) delete files[p];
  for (const [p, { category, text = '', digest = `sha256:${p}:new` }] of Object.entries(add)) {
    texts[p] = text;
    files[p] = { digest, category, tracked: true };
  }
  const attestation = { schemaVersion: 1, tool: TOOL, stateVersion: STATE_VERSION, files: baseFiles, runtime: {}, deferrals };
  const snapshot = { files, runtime: {} };
  const rep = report();
  if (mutate) mutate(rep);
  return {
    config: { top: '/unused', editedFiles: 'whole', residualMode: 'bounded', budget: { maxForcedShare: 1, maxForcedMutants: null, maxMinutesPerInvocation: 5 }, ...config },
    snapshot,
    graph: graphOf({ ...EDGES, ...edges }, open),
    chosen: { label: '.mutation', report: rep, attestation, changes: changeSet(attestation, snapshot) },
    readText: (f) => texts[f],
  };
}
const plan = (opts, extra = {}) => computePlan({ ...scenario(opts), ...extra });
const residual = { editedFiles: 'residual' };
const A3 = A.replace('n > hi', 'n >= hi');

test('whole: behaviour-preserving edit on a.ts line 3 forces the file and the residual', () => {
  const p = plan({ edit: { 'src/a.ts': A3 } });
  assert.deepEqual([p.scope, p.forced, p.deferrals, p.pendingFull], [[], ['src/a.ts', 'src/b.ts:4-4'], [], false]);
  assert.deepEqual([p.invocation1, p.invocation2], [[], ['src/a.ts', 'src/b.ts:4-4']]);
  assert.equal(p.baseline, '.mutation');
  assert.equal(p.forcedCount, 1);
});

test('residual: a.ts line 3 keeps the file in scope, forces the changed line and the mapped closure', () => {
  const p = plan({ edit: { 'src/a.ts': A3 }, config: residual });
  // a2 (line 2, closure) and the changed line 3 merge; forcedCount counts closure mutants only.
  assert.deepEqual([p.scope, p.forced, p.forcedCount], [['src/a.ts'], ['src/a.ts:2-3', 'src/b.ts:4-4'], 2]);
});

test('residual: a block mutant opening on an edited line is forced', () => {
  // a1 (lines 1–5) starts on the edited line 1: its own text may be unchanged, so it is forced.
  const p = plan({ edit: { 'src/a.ts': A.replace('clamp(n, lo, hi)', 'clamp(n, lo, max)') }, config: residual });
  assert.deepEqual([p.scope, p.forced, p.forcedCount], [['src/a.ts'], ['src/a.ts:1-5', 'src/b.ts:4-4'], 3]);
});

test('residual: a block mutant closing on an edited line is forced', () => {
  const p = plan({ edit: { 'src/a.ts': A.replace('  return n;\n}\n', '  return n;\n};\n') }, config: residual });
  assert.deepEqual([p.scope, p.forced], [['src/a.ts'], ['src/a.ts:1-5', 'src/b.ts:4-4']]);
});

test('residual: a line inserted inside a block is re-run by the scope, the rest is mapped', () => {
  const p = plan({ edit: { 'src/a.ts': A.replace('  if (n > hi)', '  n = n;\n  if (n > hi)') }, config: residual });
  assert.deepEqual([p.scope, p.forced], [['src/a.ts'], ['src/a.ts:2-4', 'src/b.ts:4-4']]);
});

test('residual: support forcing skips mutants on the changed hunk of a residual-edited file', () => {
  const p = plan({ edit: { 'src/a.ts': A3, 'tests/helpers/make.ts': 'x' }, config: residual });
  assert.deepEqual([p.scope, p.forced, p.forcedCount], [['src/a.ts'], ['src/a.ts:2-3', 'src/b.ts:4-4'], 2]);
});

test('residual: e.ts closure is narrower than the file', () => {
  const p = plan({ edit: { 'src/e.ts': E.replace('n + 1', 'n + 2') }, config: residual });
  assert.deepEqual([p.scope, p.forced], [['src/e.ts'], ['src/e.ts:2-2']]);
});

test('residual: a pure deletion forces no new-side lines, only the closure', () => {
  const p = plan({ edit: { 'src/e.ts': E.replace('  return r;\n}\n\nexport function dec', '}\n\nexport function dec') }, config: residual });
  assert.deepEqual([p.scope, p.forced], [['src/e.ts'], ['src/e.ts:2-2']]);
});

test('residual: inserted line shifts mapped ranges; a hunk outside every mutant adds importer tests', () => {
  const p = plan({ edit: { 'src/a.ts': `// note\n${A}` }, config: residual });
  assert.deepEqual([p.scope, p.forced], [['src/a.ts'], ['src/a.ts:1-6', 'src/b.ts:4-4']]);
});

test('residual: a capped diff is handled like the whole-file fallback', () => {
  const many = Array.from({ length: MAX_EDIT_DISTANCE + 1 }, (_, i) => `// ${i}`).join('\n');
  const p = plan({ edit: { 'src/a.ts': `${A}${many}\n` }, config: residual });
  assert.deepEqual([p.scope, p.forced], [[], ['src/a.ts', 'src/b.ts:4-4']]);
});

test('residual with residual.mode off records only the closure it skipped', () => {
  const p = plan({ edit: { 'src/a.ts': A3 }, config: { ...residual, residualMode: 'off' } });
  assert.deepEqual([p.scope, p.forced], [['src/a.ts'], ['src/a.ts:3-3']]);
  assert.deepEqual(p.deferrals, [{ reason: 'residual off: src/a.ts', paths: ['src/a.ts', 'src/b.ts'] }]);
});

test('whole and residual: a hunk outside every mutant adds the file importer tests (K4)', () => {
  const edges = { 'tests/c.test.ts': ['src/c.ts', 'tests/helpers/fmt.ts', 'src/a.ts'] };
  const appended = `${A}export const extra = 1;\n`;
  assert.deepEqual(plan({ edit: { 'src/a.ts': A3 }, edges }).forced, ['src/a.ts', 'src/b.ts:4-4']);
  assert.deepEqual(plan({ edit: { 'src/a.ts': appended }, edges }).forced, ['src/a.ts', 'src/b.ts:4-4', 'src/c.ts:2-2']);
  const r = plan({ edit: { 'src/a.ts': appended }, edges, config: residual });
  assert.deepEqual([r.scope, r.forced], [['src/a.ts'], ['src/a.ts:1-6', 'src/b.ts:4-4', 'src/c.ts:2-2']]);
});

test('residual falls back to whole above RESIDUAL_MAX_BYTES', () => {
  const s = scenario({ edit: { 'src/a.ts': A3 }, config: residual });
  const p = computePlan({ ...s, readText: (f) => (f === 'src/a.ts' ? 'x'.repeat(RESIDUAL_MAX_BYTES + 1) : '') });
  assert.equal(p.forced[0], 'src/a.ts');
  assert.deepEqual(p.scope, []);
});

test('zero-mutant module: importer tests drive the residual (limits.ts, index.ts)', () => {
  assert.deepEqual(plan({ edit: { 'src/limits.ts': 'export const LIMIT = 50;\n' } }).forced, ['src/c.ts:2-2', 'src/limits.ts']);
  assert.deepEqual(plan({ edit: { 'src/index.ts': "export * from './a.js';\n" }, config: residual }).forced, ['src/a.ts:1-5', 'src/index.ts']);
  // A new test importing the module has no ids in the report yet; it adds nothing to T_Y.
  const withNewTest = plan({ edit: { 'src/limits.ts': 'export const LIMIT = 50;\n' }, add: { 'tests/l.test.ts': { category: 'test' } }, edges: { 'tests/l.test.ts': ['src/limits.ts'] } });
  assert.deepEqual(withNewTest.forced, ['src/c.ts:2-2', 'src/limits.ts']);
});

test('support forcing skips a file that is already edited whole', () => {
  assert.deepEqual(plan({ edit: { 'src/a.ts': A3, 'tests/helpers/make.ts': 'x' } }).forced, ['src/a.ts', 'src/b.ts:4-4']);
});

test('forced spans merge per file and sort by start, then end', () => {
  const p = plan({
    edit: { 'tests/helpers/make.ts': 'x' },
    mutate: (r) => { r.files['src/e.ts'].mutants = [mk('x1', 2, 2, 'Killed', ['tb'], ['tb']), mk('x2', 7, 9, 'Killed', ['tb'], ['tb']), mk('x3', 7, 7, 'Killed', ['tb'], ['tb'])]; },
  });
  assert.deepEqual(p.forced, ['src/a.ts:1-5', 'src/b.ts:4-4', 'src/e.ts:2-2', 'src/e.ts:7-9']);
});

test('edit tests/b.test.ts scopes the files its tests cover', () => {
  assert.deepEqual(plan({ edit: { 'tests/b.test.ts': 'x' } }).scope, ['src/a.ts', 'src/b.ts']);
});

test('support edit forces its importer tests mutants as merged ranges', () => {
  const p = plan({ edit: { 'tests/helpers/make.ts': 'x' } });
  assert.deepEqual([p.forced, p.forcedCount], [['src/a.ts:1-5', 'src/b.ts:4-4'], 4]);
});

test('budget overflow drops forced entries and returns moved files to scope', () => {
  const budget = { maxForcedShare: 1, maxForcedMutants: 1, maxMinutesPerInvocation: 5 };
  const p = plan({ edit: { 'tests/helpers/make.ts': 'x' }, config: { budget } });
  assert.deepEqual([p.forced, p.scope, p.forcedCount], [[], [], 4]);
  assert.deepEqual(p.deferrals, [{ reason: 'forced set 4 exceeds budget 1', paths: ['src/a.ts', 'src/b.ts'] }]);
  const both = plan({ edit: { 'tests/helpers/make.ts': 'x', 'tests/b.test.ts': 'y' }, config: { budget } });
  assert.deepEqual([both.forced, both.scope], [[], ['src/a.ts', 'src/b.ts']]);
  const share = plan({ edit: { 'tests/helpers/make.ts': 'x' }, config: { budget: { ...budget, maxForcedShare: 0.25, maxForcedMutants: 100 } } });
  assert.equal(share.deferrals[0].reason, 'forced set 4 exceeds budget 2');
});

test('strict and unbudgeted runs ignore the budget', () => {
  const budget = { maxForcedShare: 1, maxForcedMutants: 1, maxMinutesPerInvocation: 5 };
  assert.deepEqual(plan({ edit: { 'tests/helpers/make.ts': 'x' }, config: { budget, residualMode: 'strict' } }).forced, ['src/a.ts:1-5', 'src/b.ts:4-4']);
  assert.deepEqual(plan({ edit: { 'tests/helpers/make.ts': 'x' }, config: { budget } }, { unbudgeted: true }).forced, ['src/a.ts:1-5', 'src/b.ts:4-4']);
});

test('delete b.ts and b.test.ts together', () => {
  const p = plan({ remove: ['src/b.ts', 'tests/b.test.ts'] });
  assert.deepEqual([p.scope, p.forced, p.forcedCount], [[], ['src/a.ts'], 3]);
});

test('git mv c.ts to d.ts with its test updated', () => {
  const p = plan({
    remove: ['src/c.ts'],
    add: { 'src/d.ts': { category: 'mutate', text: C } },
    edit: { 'tests/c.test.ts': 'x' },
    edges: { 'tests/c.test.ts': ['src/d.ts', 'tests/helpers/fmt.ts'], 'src/d.ts': ['src/limits.ts'] },
  });
  assert.deepEqual([p.forced, p.scope], [['src/d.ts'], ['src/limits.ts']]);
});

test('residual off records what it would have forced; disableResidual forces nothing', () => {
  const off = plan({ edit: { 'src/a.ts': A3 }, config: { residualMode: 'off' } });
  assert.deepEqual([off.forced, off.deferrals], [['src/a.ts'], [{ reason: 'residual off: src/a.ts', paths: ['src/b.ts'] }]]);
  assert.deepEqual(plan({ edit: { 'src/a.ts': A3 } }, { disableResidual: true }).forced, ['src/a.ts']);
});

test('cold plan', () => {
  const p = computePlan({ ...scenario(), chosen: null });
  assert.deepEqual(publicPlan(p), {
    baseline: null, cold: true, pendingFull: true, scope: [], forced: [], forcedCount: 0,
    deferrals: [{ reason: 'no usable state', paths: ['*'] }], changes: [], openImporters: [], unclassified: [],
  });
});

test('global, support and unclassified deferrals', () => {
  const lock = plan({ edit: { 'package-lock.json': 'x' } });
  assert.deepEqual([lock.deferrals, lock.pendingFull], [[{ reason: 'global input changed: package-lock.json', paths: ['*'] }], true]);
  assert.deepEqual(plan({ add: { 'tests/helpers/orphan.ts': { category: 'support' } } }).deferrals, [{ reason: 'support tests/helpers/orphan.ts has no resolvable importer', paths: ['*'] }]);
  assert.deepEqual(plan({ remove: ['tests/helpers/fmt.ts'] }).deferrals, [{ reason: 'support tests/helpers/fmt.ts deleted', paths: ['*'] }]);
  const orphan = plan({ add: { 'scripts/x.sh': { category: 'unclassified' } } });
  assert.deepEqual([orphan.deferrals, orphan.unclassified], [[{ reason: 'unclassified scripts/x.sh has no resolvable importer', paths: ['*'] }], ['scripts/x.sh']]);
  assert.deepEqual(plan({ base: { 'scripts/y.sh': { category: 'unclassified' } }, remove: ['scripts/y.sh'] }).deferrals, []);
});

test('unclassified input with an importer forces the kills of its importer tests', () => {
  const opts = { base: { 'fixtures/x.json': { category: 'unclassified' } }, edit: { 'fixtures/x.json': '{}' }, edges: { 'tests/a.test.ts': ['src/a.ts', 'fixtures/x.json'] } };
  const p = plan(opts);
  assert.deepEqual([p.forced, p.unclassified], [['src/a.ts:1-5'], ['fixtures/x.json']]);
  assert.deepEqual(plan({ ...opts, config: { residualMode: 'off' } }).deferrals, [{ reason: 'residual off: fixtures/x.json', paths: ['src/a.ts'] }]);
});

test('a test that newly reaches an uncovered module scopes it', () => {
  const p = plan({ base: { 'src/u.ts': { category: 'mutate' } }, add: { 'tests/u.test.ts': { category: 'test' } }, edges: { 'tests/u.test.ts': ['src/u.ts'] } });
  assert.deepEqual(p.scope, ['src/u.ts']);
});

test('a new module arriving with its first test is forced whole; its new test has no ids yet', () => {
  const p = plan({ add: { 'src/v.ts': { category: 'mutate' }, 'tests/v.test.ts': { category: 'test' } }, edges: { 'tests/v.test.ts': ['src/v.ts'] } });
  assert.deepEqual([p.forced, p.scope], [['src/v.ts'], []]);
});

test('a test that reaches an open importer scopes every uncovered or weak file', () => {
  const p = plan({ add: { 'tests/w.test.ts': { category: 'test' } }, open: { 'tests/w.test.ts': ['@x/y'] } });
  assert.deepEqual(p.scope, ['src/index.ts', 'src/limits.ts', 'src/types.ts']);
  assert.deepEqual(p.openImporters, [{ path: 'tests/w.test.ts', specifiers: ['@x/y'] }]);
});

test('a test reaching an open importer also scopes reported files with survivors', () => {
  const p = plan({ add: { 'tests/w.test.ts': { category: 'test' } }, open: { 'tests/w.test.ts': ['@x/y'] }, mutate: (r) => { r.files['src/c.ts'].mutants[0].status = 'Survived'; } });
  assert.deepEqual(p.scope, ['src/c.ts', 'src/index.ts', 'src/limits.ts', 'src/types.ts']);
});

test('a new case in an existing test scopes a file with a survivor', () => {
  const p = plan({ edit: { 'tests/a.test.ts': 'x' }, mutate: (r) => { r.files['src/a.ts'].mutants[2].status = 'Survived'; } });
  assert.deepEqual(p.scope, ['src/a.ts']);
});

test('prior deferrals carry into the plan; pendingFull follows them', () => {
  const prior = [{ reason: 'no reachable tests: src/c.ts', paths: ['src/c.ts'] }];
  const p = plan({ deferrals: prior });
  assert.deepEqual([p.deferrals, p.pendingFull, p.added], [prior, true, []]);
});

test('a symlinked mutate file and a path Stryker cannot take', () => {
  assert.deepEqual(plan({ add: { 'src/s.ts': { category: 'mutate', digest: 'symlink:abc' } } }).forced, []);
  assert.throws(() => plan({ add: { 'src/x,y.ts': { category: 'mutate' } } }), (e) => e.exitCode === 2 && /--mutate/.test(e.message));
});

test('deferral helpers: canonical, dedupe and sort', () => {
  const d1 = { reason: 'b', paths: ['z', 'a'] };
  const d2 = { reason: 'b', paths: ['a', 'z'], inherited: true };
  const d3 = { reason: 'a', paths: ['*'] };
  assert.equal(deferralKey(d1), deferralKey(d2));
  assert.deepEqual(dedupeDeferrals([d1], [d2, d3]), [{ reason: 'b', paths: ['a', 'z'] }, { reason: 'a', paths: ['*'] }]);
  assert.deepEqual(sortDeferrals([d1, d3, { reason: 'b', paths: ['0'] }]).map((d) => d.paths[0]), ['*', '0', 'z']);
});

test('a support edit does not force mutants of a module deleted in the same change', () => {
  const p = plan({ edit: { 'tests/helpers/make.ts': 'x' }, remove: ['src/b.ts'] });
  assert.deepEqual(p.forced, ['src/a.ts:1-5']);
});

test('without readText the edited source is read from the repository', () => {
  const top = mkdtempSync(join(tmpdir(), 'mi-plan-'));
  mkdirSync(join(top, 'src'));
  writeFileSync(join(top, 'src/a.ts'), A3);
  const s = scenario({ edit: { 'src/a.ts': A3 }, config: { ...residual, top } });
  const p = computePlan({ ...s, readText: undefined });
  assert.deepEqual([p.scope, p.forced], [['src/a.ts'], ['src/a.ts:2-3', 'src/b.ts:4-4']]);
});
