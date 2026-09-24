#!/usr/bin/env node
// Local proof of the mutation-incremental harness against real StrykerJS 10 and Vitest 4 (spec §7.1,
// §11.7). Copies testdata/mutation-incremental-e2e into .e2e/<run>/ (never /tmp), installs its pinned
// dependencies (network), and drives the real CLI through a shim that records Stryker's argv.
// Not run in CI. Usage: node tests/e2e-mutation-incremental.mjs [row ...]   (the full row always runs)
import { execFileSync, spawn, spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { cpSync, existsSync, mkdirSync, readFileSync, readdirSync, rmSync, symlinkSync, writeFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const FIXTURE = join(ROOT, 'testdata/mutation-incremental-e2e');
const TEMPLATE = join(ROOT, 'templates/mutation-incremental');
const REAL = join(ROOT, 'testdata/mutation-incremental/real');
const WORK = join(ROOT, '.e2e', `e2e-${new Date().toISOString().replace(/[:.]/g, '-')}`);
const REPO = join(WORK, 'repo');
const BARE = join(WORK, 'remote.git');
const ONLY = new Set(process.argv.slice(2));
const STATE_FILES = ['attestation.json', 'incremental.json'];

const SHIM = `// Records Stryker's argv, then runs the real Stryker. shim.json can make it sleep, die or find no tests.
import { appendFileSync, existsSync, readFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
const dir = process.env.E2E_DIR;
appendFileSync(dir + '/calls.jsonl', JSON.stringify(process.argv.slice(2)) + '\\n');
const mode = existsSync(dir + '/shim.json') ? JSON.parse(readFileSync(dir + '/shim.json', 'utf8')) : {};
if (mode.sleepMs) await new Promise((r) => setTimeout(r, mode.sleepMs));
if (mode.die) process.kill(process.pid, 'SIGKILL');
if (mode.noTests) { console.log('INFO No tests were executed. Stryker will exit prematurely.'); process.exit(1); }
const r = spawnSync(process.execPath, ['node_modules/.bin/stryker', ...process.argv.slice(2)], { stdio: 'inherit' });
process.exit(r.status ?? 1);
`;

const results = [];
function check(label, ok, detail = '') {
  results.push({ label, ok: Boolean(ok), detail: ok ? '' : String(detail) });
  console.log(`${ok ? 'PASS' : 'FAIL'} ${label}${ok ? '' : ` — ${detail}`}`);
}
const git = (...args) => execFileSync('git', args, { cwd: REPO, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] });
const readJSON = (rel, dir = REPO) => JSON.parse(readFileSync(join(dir, rel), 'utf8'));
const stateText = (dir = REPO) => readFileSync(join(dir, '.mutation/attestation.json'), 'utf8');

let callNo = 0;
// Runs the harness CLI; returns {code, stdout, stderr, output}. signalAfterMs sends SIGHUP.
function cli(args, { dir = REPO, env = {}, signalAfterMs = null } = {}) {
  return new Promise((done) => {
    const n = ++callNo;
    const outFile = join(WORK, `output-${n}`);
    writeFileSync(outFile, '');
    const child = spawn(process.execPath, ['tools/mutation-incremental/cli.mjs', ...args], {
      cwd: dir,
      env: { ...process.env, E2E_DIR: WORK, GITHUB_OUTPUT: outFile, ...env },
      stdio: ['ignore', 'pipe', 'pipe'],
    });
    let stdout = '';
    let stderr = '';
    child.stdout.on('data', (c) => { stdout += c; });
    child.stderr.on('data', (c) => { stderr += c; });
    const timer = signalAfterMs === null ? null : setTimeout(() => child.kill('SIGHUP'), signalAfterMs);
    child.on('close', (code) => {
      clearTimeout(timer);
      writeFileSync(join(WORK, `log-${n}.txt`), `$ cli ${args.join(' ')}   (cwd ${dir})\nexit ${code}\n${stderr}`);
      const lines = readFileSync(outFile, 'utf8').split('\n').filter(Boolean);
      const output = Object.fromEntries(lines.map((l) => [l.slice(0, l.indexOf('=')), l.slice(l.indexOf('=') + 1)]));
      done({ code, stdout, stderr, output });
    });
  });
}
const plan = async (args = []) => JSON.parse((await cli(['plan', ...args])).stdout);
const calls = () => (existsSync(join(WORK, 'calls.jsonl')) ? readFileSync(join(WORK, 'calls.jsonl'), 'utf8').split('\n').filter(Boolean).map((l) => JSON.parse(l)) : []);
function shim(mode) {
  rmSync(join(WORK, 'calls.jsonl'), { force: true });
  if (mode) writeFileSync(join(WORK, 'shim.json'), JSON.stringify(mode));
  else rmSync(join(WORK, 'shim.json'), { force: true });
}
function edit(rel, from, to) {
  const path = join(REPO, rel);
  const text = readFileSync(path, 'utf8');
  if (!text.includes(from)) throw new Error(`${rel}: ${JSON.stringify(from)} not found`);
  writeFileSync(path, text.replace(from, to));
}
// Back to the committed tree; with `state`, the full row's state.
function reset({ state = true } = {}) {
  git('checkout', '--', '.');
  git('clean', '-fdq');
  rmSync(join(REPO, '.mutation'), { recursive: true, force: true });
  if (state) {
    mkdirSync(join(REPO, '.mutation'), { recursive: true });
    for (const f of STATE_FILES) cpSync(join(WORK, 'baseline', f), join(REPO, '.mutation', f));
  }
  shim(null);
}

const mutantKey = (file, m) => JSON.stringify([file, m.mutatorName, m.location, m.replacement]);
function freshStatuses() {
  const rel = '.mutation/fresh.json';
  rmSync(join(REPO, rel), { force: true });
  const r = spawnSync(process.execPath, ['node_modules/.bin/stryker', 'run', 'stryker.config.json', '--incremental', '--incrementalFile', rel, '--force'], { cwd: REPO, encoding: 'utf8', maxBuffer: 1 << 28 });
  if (r.status !== 0 && r.status !== 1) throw new Error(`fresh Stryker run failed (${r.status}): ${String(r.stdout).slice(-2000)}`);
  const statuses = new Map();
  for (const [file, entry] of Object.entries(readJSON(rel).files)) for (const m of entry.mutants) statuses.set(mutantKey(file, m), m.status);
  return statuses;
}
// Spec §7.1 equivalence: every kill outside a deferral is a kill in a fresh run of the same tree.
function equivalence() {
  const att = readJSON('.mutation/attestation.json');
  if (att.deferrals.some((d) => d.paths.includes('*'))) return { compared: 0, mismatches: [] };
  const deferred = new Set(att.deferrals.flatMap((d) => d.paths));
  const fresh = freshStatuses();
  const mismatches = [];
  let compared = 0;
  for (const [file, entry] of Object.entries(readJSON('.mutation/incremental.json').files)) {
    if (deferred.has(file)) continue;
    for (const m of entry.mutants) {
      if (m.status !== 'Killed') continue;
      compared++;
      const status = fresh.get(mutantKey(file, m)) ?? 'absent';
      if (status !== 'Killed' && status !== 'Timeout') mismatches.push(`${file}:${m.location.start.line} ${m.mutatorName} ${JSON.stringify(m.replacement)} → fresh ${status}`);
    }
  }
  return { compared, mismatches };
}
function checkEquivalence(row) {
  const e = equivalence();
  check(`${row}: equivalence (${e.compared} kills compared)`, e.compared > 0 && e.mismatches.length === 0, e.mismatches.join('; ') || 'nothing compared');
}
// Real reports for Plan 2's gate tests: projectRoot rewritten to ".", reportSha256 recomputed.
function exportRow(row) {
  const dir = join(REAL, row);
  mkdirSync(dir, { recursive: true });
  const report = readJSON('.mutation/incremental.json');
  report.projectRoot = '.';
  const bytes = `${JSON.stringify(report)}\n`;
  const att = readJSON('.mutation/attestation.json');
  att.reportSha256 = createHash('sha256').update(bytes).digest('hex');
  writeFileSync(join(dir, 'incremental.json'), bytes);
  writeFileSync(join(dir, 'attestation.json'), `${JSON.stringify(att, null, 2)}\n`);
}
const mutantsAt = (file, line) => (readJSON('.mutation/incremental.json').files[file]?.mutants ?? []).filter((m) => m.location.start.line === line);
const deferralReasons = () => readJSON('.mutation/attestation.json').deferrals.map((d) => d.reason);

async function editRow(row, apply, { args = ['run', '--mode', 'incremental'], env = {}, expect = () => [], equivalent = true } = {}) {
  reset();
  apply();
  const p = await plan();
  const r = await cli(args, { env });
  check(`${row}: exit 0`, r.code === 0, `exit ${r.code}: ${r.stderr.slice(-800)}`);
  if (r.code !== 0) return { p, r };
  for (const [label, ok, detail] of expect(p, r)) check(`${row}: ${label}`, ok, detail);
  if (equivalent) checkEquivalence(row);
  exportRow(row);
  return { p, r };
}

const A_LINE3 = "  if (score > top) return 'over';";
const preserveA = () => edit('src/a.ts', A_LINE3, "  if (top < score) return 'over';");

const ROWS = [
  ['cold', async () => {
    reset({ state: false });
    const p = await plan();
    check('cold: plan is cold with "no usable state"', p.cold && p.deferrals.some((d) => d.reason === 'no usable state'), JSON.stringify(p.deferrals));
    const r = await cli(['run', '--mode', 'incremental']);
    check('cold: exit 0, no invocation, pending_full=true', r.code === 0 && calls().length === 0 && r.output.pending_full === 'true', `${r.code} ${calls().length} ${JSON.stringify(r.output)}`);
  }],
  ['full', async () => {
    reset({ state: false });
    const r = await cli(['run', '--mode', 'full']);
    check('full: exit 0 with one invocation', r.code === 0 && calls().length === 1, `exit ${r.code}: ${r.stderr.slice(-800)}`);
    if (r.code !== 0) throw new Error('the full row failed; every other row needs its state');
    const att = readJSON('.mutation/attestation.json');
    check('full: no deferrals, lastFullAt = completedAt', att.deferrals.length === 0 && att.lastFullAt === att.completedAt, JSON.stringify(att.deferrals));
    check('full: the base fixture has no survivors (score 100)', att.score === 100, `score ${att.score}`);
    mkdirSync(join(WORK, 'baseline'), { recursive: true });
    for (const f of STATE_FILES) cpSync(join(REPO, '.mutation', f), join(WORK, 'baseline', f));
    exportRow('full');
  }],
  ['no-change', async () => {
    reset();
    const r = await cli(['run', '--mode', 'incremental']);
    check('no-change: exit 0, no invocation', r.code === 0 && calls().length === 0, `${r.code} ${calls().length}`);
  }],
  ['a-preserving', () => editRow('a-preserving', preserveA, {
    expect: () => [['no deferral', deferralReasons().length === 0, deferralReasons()]],
  })],
  ['a-survivor', () => editRow('a-survivor', () => edit('src/a.ts', A_LINE3, "  if (score >= top + 1) return 'over';"), {
    expect: () => [['a Survived mutant on line 3', mutantsAt('src/a.ts', 3).some((m) => m.status === 'Survived'), JSON.stringify(mutantsAt('src/a.ts', 3).map((m) => m.status))]],
  })],
  ['test-b', () => editRow('test-b', () => edit('tests/b.test.ts', "test('outside the range'", "test('another inside case', () => expect(inRange(70, range(50, 90))).toBe(true));\ntest('outside the range'"), {
    expect: (p) => [['scope [src/a.ts, src/b.ts]', JSON.stringify(p.scope) === '["src/a.ts","src/b.ts"]', JSON.stringify(p.scope)]],
  })],
  ['limits', () => editRow('limits', () => edit('src/limits.ts', 'LIMIT = 100;', 'LIMIT = 10 * 10;'), {
    expect: (p) => [
      ['forced has src/limits.ts and a src/c.ts range', p.forced.includes('src/limits.ts') && p.forced.some((f) => f.startsWith('src/c.ts:')), JSON.stringify(p.forced)],
      ['no deferral', deferralReasons().length === 0, deferralReasons()],
    ],
  })],
  ['helper-make', () => editRow('helper-make', () => edit('tests/helpers/make.ts', '({ lo, hi })', '({ hi, lo })'), {
    expect: (p) => [
      ['forced has src/a.ts and src/b.ts ranges', p.forced.some((f) => f.startsWith('src/a.ts')) && p.forced.some((f) => f.startsWith('src/b.ts')), JSON.stringify(p.forced)],
      ['vitest and node:assert are packages, not open importers', p.openImporters.length === 0, JSON.stringify(p.openImporters)],
    ],
  })],
  ['types-only', async () => {
    const { r } = await editRow('types-only', () => edit('src/types.ts', 'hi: number }', 'hi: number; }'), {
      expect: (_p, res) => [
        ['one invocation, no deferral, pending_full=false', calls().length === 1 && deferralReasons().length === 0 && res.output.pending_full === 'false', `${calls().length} ${deferralReasons()} ${res.output.pending_full}`],
      ],
    });
    if (r.code === 0) {
      const next = await plan();
      check('types-only: the next plan has no changes', next.changes.length === 0, JSON.stringify(next.changes));
    }
  }],
  ['delete-c-test', () => editRow('delete-c-test', () => rmSync(join(REPO, 'tests/c.test.ts')), {
    expect: (_p, r) => [
      ['deferral "no reachable tests: src/c.ts"', deferralReasons().includes('no reachable tests: src/c.ts'), deferralReasons()],
      ['cause unreachable', r.output.pending_cause === 'unreachable', r.output.pending_cause],
    ],
  })],
  ['e-residual', () => editRow('e-residual', () => edit('src/e.ts', '  const r = n + 1;', '  const r = 1 + n;'), {
    expect: () => [['no forced range inside dec (lines 6-9)', !calls().flat().some((a) => /src\/e\.ts:[6-9]/.test(a)), JSON.stringify(calls())]],
  })],
  ['e-flip', () => editRow('e-flip', () => edit('src/e.ts', '  return r;', '  return 2;'), {
    expect: () => [['line 2 ArithmeticOperator now Survived', mutantsAt('src/e.ts', 2).some((m) => m.mutatorName === 'ArithmeticOperator' && m.status === 'Survived'), JSON.stringify(mutantsAt('src/e.ts', 2).map((m) => [m.mutatorName, m.status]))]],
  })],
  ['e-flip-control', async () => {
    const { r } = await editRow('e-flip-control', () => edit('src/e.ts', '  return r;', '  return 2;'), { env: { MUTATION_TEST_DISABLE_RESIDUAL: '1' }, equivalent: false });
    if (r.code !== 0) return;
    const e = equivalence();
    check('e-flip-control: equivalence fails for exactly src/e.ts:2 ArithmeticOperator', e.mismatches.length === 1 && e.mismatches[0].startsWith('src/e.ts:2 ArithmeticOperator'), e.mismatches.join('; ') || 'no mismatch');
  }],
  ['lockfile', () => editRow('lockfile', () => writeFileSync(join(REPO, 'package-lock.json'), `${readFileSync(join(REPO, 'package-lock.json'), 'utf8')}\n`), {
    equivalent: false,
    expect: (_p, r) => [
      ['deferral "global input changed: package-lock.json"', deferralReasons().includes('global input changed: package-lock.json'), deferralReasons()],
      ['no invocation, pending_full=true', calls().length === 0 && r.output.pending_full === 'true', `${calls().length} ${r.output.pending_full}`],
    ],
  })],
  ['time-budget', () => editRow('time-budget', () => { preserveA(); shim({ sleepMs: 20000 }); }, {
    args: ['run', '--mode', 'incremental', '--pr', '--max-minutes', '0.05'],
    equivalent: false,
    expect: (_p, r) => [
      ['deferral "time budget exceeded"', deferralReasons().includes('time budget exceeded'), deferralReasons()],
      ['route=sweep (never red for speed)', r.output.route === 'sweep', r.output.route],
    ],
  })],
  ['sighup', async () => {
    reset();
    preserveA();
    shim({ sleepMs: 30000 });
    const before = stateText();
    const r = await cli(['run', '--mode', 'incremental'], { signalAfterMs: 5000 });
    check('sighup: exit 130', r.code === 130, `exit ${r.code}`);
    check('sighup: state unchanged, no lock, no work/', stateText() === before && !existsSync(join(REPO, '.mutation/lock')) && !existsSync(join(REPO, '.mutation/work')), 'state or lock or work/ left behind');
  }],
  ['stryker-killed', async () => {
    reset();
    preserveA();
    shim({ die: true });
    const before = stateText();
    const r = await cli(['run', '--mode', 'incremental']);
    check('stryker-killed: exit 4, state unchanged', r.code === 4 && stateText() === before, `exit ${r.code}`);
  }],
  ['full-no-tests', async () => {
    reset();
    shim({ noTests: true });
    const before = stateText();
    const r = await cli(['run', '--mode', 'full']);
    check('full-no-tests: exit 4, nothing committed', r.code === 4 && stateText() === before, `exit ${r.code}`);
  }],
  ['publish-fetch', async () => {
    reset();
    const pub = await cli(['publish-state', '--kind', 'inc']);
    check('publish-fetch: publish-state exit 0', pub.code === 0, pub.stderr.slice(-500));
    const clone = join(WORK, 'clone');
    rmSync(clone, { recursive: true, force: true });
    execFileSync('git', ['clone', '-q', REPO, clone]);
    execFileSync('git', ['remote', 'set-url', 'origin', BARE], { cwd: clone });
    symlinkSync(join(REPO, 'node_modules'), join(clone, 'node_modules'));
    // A symlink does not match the `node_modules/` directory pattern; exclude it so it is no input.
    writeFileSync(join(clone, '.git/info/exclude'), 'node_modules\n');
    const got = await cli(['fetch-state'], { dir: clone });
    check('publish-fetch: fetch-state exit 0 with remote/inc', got.code === 0 && existsSync(join(clone, '.mutation/remote/inc/incremental.json')), got.stderr.slice(-500));
    const run = await cli(['run', '--mode', 'incremental', '--also-state', '.mutation/remote/inc'], { dir: clone });
    const att = run.code === 0 ? readJSON('.mutation/attestation.json', clone) : null;
    check('publish-fetch: the clone adopts it (warm, nothing to run, no deferrals)', run.code === 0 && calls().length === 0 && att.deferrals.length === 0, `exit ${run.code} calls ${calls().length} ${JSON.stringify(att?.deferrals)}`);
  }],
  ['seed', async () => {
    reset({ state: false });
    const from = join(WORK, 'baseline/incremental.json');
    const r = await cli(['seed', '--from', from]);
    check('seed: exit 0 with "seeded from"', r.code === 0 && deferralReasons().includes(`seeded from ${from}`), `exit ${r.code} ${r.stderr.slice(-300)}`);
    const again = await cli(['seed', '--from', from, '--replace']);
    const kept = existsSync(join(REPO, '.mutation/replaced')) ? readdirSync(join(REPO, '.mutation/replaced')) : [];
    check('seed: --replace keeps the old pair', again.code === 0 && kept.length === 1, `exit ${again.code} kept ${kept}`);
  }],
  ['threshold', async () => {
    reset();
    edit('stryker.config.json', '"break": null', '"break": 100');
    edit('tests/a.test.ts', "test('at top is ok', () => assert.equal(grade(90, 50, 90), 'ok'));\n", '');
    const r = await cli(['run', '--mode', 'full']);
    check('threshold: a survivor below break 100 is exit 1', r.code === 1, `exit ${r.code}`);
    const again = await cli(['run', '--mode', 'incremental']);
    check('threshold: an unchanged re-run is exit 1 again', again.code === 1, `exit ${again.code}`);
  }],
];

async function setup() {
  mkdirSync(WORK, { recursive: true });
  cpSync(FIXTURE, REPO, { recursive: true });
  mkdirSync(join(REPO, 'tools/mutation-incremental'), { recursive: true });
  cpSync(join(TEMPLATE, 'cli.mjs'), join(REPO, 'tools/mutation-incremental/cli.mjs'));
  cpSync(join(TEMPLATE, 'lib'), join(REPO, 'tools/mutation-incremental/lib'), { recursive: true });
  writeFileSync(join(WORK, 'shim.mjs'), SHIM);
  const config = readJSON('mutation-incremental.json');
  config.stryker.command = [process.execPath, join(WORK, 'shim.mjs')];
  writeFileSync(join(REPO, 'mutation-incremental.json'), `${JSON.stringify(config, null, 2)}\n`);
  execFileSync('npm', ['ci', '--no-audit', '--no-fund'], { cwd: REPO, stdio: 'inherit' });
  git('init', '-q');
  git('config', 'user.email', 'e2e@example.com');
  git('config', 'user.name', 'e2e');
  git('add', '-A');
  git('commit', '-qm', 'fixture');
  execFileSync('git', ['init', '-q', '--bare', BARE]);
  git('remote', 'add', 'origin', BARE);
}

await setup();
for (const [row, run] of ROWS) {
  if (ONLY.size > 0 && row !== 'full' && !ONLY.has(row)) continue;
  const started = Date.now();
  try {
    await run();
  } catch (e) {
    check(`${row}: ran without an error`, false, e.stack);
    if (row === 'full') break;
  }
  console.log(`  (${row}: ${((Date.now() - started) / 1000).toFixed(1)} s)`);
}
const failed = results.filter((r) => !r.ok);
writeFileSync(join(WORK, 'results.json'), `${JSON.stringify(results, null, 2)}\n`);
console.log(`\n${results.length - failed.length}/${results.length} checks passed; work dir ${WORK}`);
process.exitCode = failed.length > 0 ? 1 : 0;
