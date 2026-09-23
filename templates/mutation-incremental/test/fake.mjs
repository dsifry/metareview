import { existsSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { main } from '../lib/main.mjs';
import { makeRepo } from './helpers.mjs';

const FAKE = readFileSync(new URL('./fake-stryker.mjs', import.meta.url), 'utf8');

// src/a.ts: line 2 has a kill (by tests/a.test.ts), line 3 a survivor it covers.
export const A = 'export function clamp(n, lo, hi) {\n  if (n < lo) return lo;\n  if (n > hi) return hi;\n  return n;\n}\n';
const mutant = (id, line, status, killedBy) => ({
  id, mutatorName: 'ConditionalExpression', replacement: 'true', status, killedBy, coveredBy: ['t1'],
  location: { start: { line, column: 7 }, end: { line, column: 13 } },
});
export function reportFor(source = A) {
  return {
    schemaVersion: '1',
    thresholds: { high: 80, low: 60 },
    files: { 'src/a.ts': { language: 'typescript', source, mutants: [mutant('1', 2, 'Killed', ['t1']), mutant('2', 3, 'Survived', [])] } },
    testFiles: { 'tests/a.test.ts': { tests: [{ id: 't1', name: 'clamps low' }] } },
  };
}

// A throwaway repository whose Stryker is the fake. .fake/ (the fake, its steps, its call log and
// GITHUB_OUTPUT) is gitignored, so it never enters a snapshot.
export function runRepo({ config = {}, stryker = {}, files = {}, steps = [{ report: reportFor() }] } = {}) {
  const r = makeRepo({
    config: { stryker: { command: [process.execPath, '.fake/stryker.mjs'], configFile: 'stryker.config.json', extraArgs: [] }, ...config },
    stryker,
    files: {
      '.gitignore': 'node_modules/\n.mutation/\n.fake/\n',
      '.fake/stryker.mjs': FAKE,
      'src/a.ts': A,
      'tests/a.test.ts': "import { clamp } from '../src/a';\n",
      ...files,
    },
  });
  const log = join(r.top, '.fake/calls.jsonl');
  // Replacing the steps also restarts the call log, so step 0 is the next invocation.
  r.steps = (list) => {
    writeFileSync(join(r.top, '.fake/steps.json'), JSON.stringify(list));
    rmSync(log, { force: true });
  };
  r.calls = () => (existsSync(log) ? readFileSync(log, 'utf8').split('\n').filter(Boolean).map((l) => JSON.parse(l)) : []);
  r.read = (rel) => JSON.parse(readFileSync(join(r.top, rel), 'utf8'));
  r.steps(steps);
  return r;
}

const capture = () => ({ text: '', write(s) { this.text += String(s); } });

// Runs the CLI in-process with GITHUB_OUTPUT pointed at .fake/output, returned parsed.
export async function cli(r, args, { killGraceMs = 200 } = {}) {
  const stdout = capture();
  const stderr = capture();
  const outFile = join(r.top, '.fake/output');
  writeFileSync(outFile, '');
  const saved = process.env.GITHUB_OUTPUT;
  process.env.GITHUB_OUTPUT = outFile;
  try {
    const code = await main(args, { stdout, stderr, cwd: r.top, killGraceMs });
    const lines = readFileSync(outFile, 'utf8').split('\n').filter(Boolean);
    const output = Object.fromEntries(lines.map((l) => [l.slice(0, l.indexOf('=')), l.slice(l.indexOf('=') + 1)]));
    return { code, stdout: stdout.text, stderr: stderr.text, output };
  } finally {
    if (saved === undefined) delete process.env.GITHUB_OUTPUT;
    else process.env.GITHUB_OUTPUT = saved;
  }
}

// A full run that leaves a usable state; later invocations echo their input file.
export async function warm(r) {
  const res = await cli(r, ['run', '--mode', 'full']);
  if (res.code !== 0) throw new Error(`warm-up full run failed (${res.code}): ${res.stderr}`);
  r.steps([{ report: 'input' }]);
}
