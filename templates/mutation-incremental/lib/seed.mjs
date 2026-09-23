import { copyFileSync, existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { UsageError } from './errors.mjs';
import { canonicalJSON } from './json.mjs';
import { loadHarnessConfig, summaryLine } from './inputs.mjs';
import { acquireLock } from './lock.mjs';
import { takeSnapshot } from './snapshot.mjs';
import { resolveViews, viewSummaries } from './views.mjs';
import { dedupeDeferrals, sortDeferrals } from './deferrals.mjs';
import { buildAttestation, commitState, WORK_DIR } from './attest.mjs';
import { parseReport } from './engine.mjs';
import { ATTESTATION_FILE, REPORT_FILE, primaryCandidate } from './state.mjs';

const cmp = (a, b) => (a < b ? -1 : a > b ? 1 : 0);
// Spec §11.6: a mutant in several reports takes the strongest status.
const RANK = { Killed: 4, Timeout: 3, Survived: 2, NoCoverage: 1 };
const rank = (status) => RANK[status] ?? 0;
const position = (m) => [m.location.start.line, m.location.start.column, m.location.end.line, m.location.end.column];
const byPosition = (a, b) => {
  const pa = position(a);
  const pb = position(b);
  return pa.reduce((acc, v, i) => acc || v - pb[i], 0) || cmp(a.mutatorName, b.mutatorName) || cmp(String(a.replacement), String(b.replacement));
};
const union = (a, b) => [...new Set([...a, ...b])].sort((x, y) => Number(x) - Number(y));

// Spec §11.6: tests are re-keyed by (file, name) and renumbered; mutants are matched by
// (file, mutatorName, location, replacement), merged (status by precedence, test lists unioned),
// and renumbered in byte order of file, then location. A path in several reports must have the
// same source.
export function mergeReports(reports) {
  const tests = new Map();
  const testFileEntries = new Map();
  const localKeys = reports.map((report) => {
    const local = new Map();
    for (const [file, entry] of Object.entries(report.testFiles)) {
      if (!testFileEntries.has(file)) testFileEntries.set(file, entry);
      for (const t of entry.tests) {
        const key = `${file}\u0000${t.name}`;
        local.set(String(t.id), key);
        if (!tests.has(key)) tests.set(key, { file, test: t });
      }
    }
    return local;
  });
  const keys = [...tests.keys()].sort(cmp);
  const newId = new Map(keys.map((k, i) => [k, String(i)]));
  const files = new Map();
  reports.forEach((report, i) => {
    const remap = (ids) => (ids ?? []).map((id) => newId.get(localKeys[i].get(String(id))));
    for (const [path, entry] of Object.entries(report.files)) {
      if (!files.has(path)) files.set(path, { entry, mutants: new Map() });
      const file = files.get(path);
      if (file.entry.source !== entry.source) throw new UsageError(`seed: ${path} has a different source in the given reports`);
      for (const m of entry.mutants) {
        const key = JSON.stringify([m.mutatorName, position(m), m.replacement]);
        const merged = { ...m, killedBy: remap(m.killedBy), coveredBy: remap(m.coveredBy) };
        const prev = file.mutants.get(key);
        if (prev === undefined) {
          file.mutants.set(key, merged);
          continue;
        }
        if (rank(m.status) > rank(prev.status)) prev.status = m.status;
        prev.killedBy = union(prev.killedBy, merged.killedBy);
        prev.coveredBy = union(prev.coveredBy, merged.coveredBy);
      }
    }
  });
  let next = 0;
  const outFiles = {};
  for (const path of [...files.keys()].sort(cmp)) {
    const { entry, mutants } = files.get(path);
    outFiles[path] = { ...entry, mutants: [...mutants.values()].sort(byPosition).map((m) => ({ ...m, id: String(next++) })) };
  }
  const outTests = {};
  for (const key of keys) {
    const { file, test } = tests.get(key);
    outTests[file] ??= { ...testFileEntries.get(file), tests: [] };
    outTests[file].tests.push({ ...test, id: newId.get(key) });
  }
  return { ...reports[0], files: outFiles, testFiles: outTests };
}

function readReport(top, path) {
  let bytes;
  try {
    bytes = readFileSync(resolve(top, path));
  } catch (e) {
    throw new UsageError(`seed: cannot read ${path}: ${e.message}`);
  }
  const report = parseReport(bytes);
  if (report === null) throw new UsageError(`seed: ${path} is not a mutation-testing report (no files object)`);
  return report;
}

// Spec §5.8: --replace first copies the current pair into replaced/<UTC timestamp>/.
function keepCurrent(stateDir) {
  const dir = join(stateDir, 'replaced', new Date().toISOString().replaceAll(':', '-'));
  mkdirSync(dir, { recursive: true });
  for (const name of [ATTESTATION_FILE, REPORT_FILE]) {
    if (existsSync(join(stateDir, name))) copyFileSync(join(stateDir, name), join(dir, name));
  }
}

// Spec §5.8, §11.6: bootstrap from full reports. The kills stay pending ("seeded from <report>" on
// ["*"]) until the first full run on main. Never exits 1: it prints the score instead.
export async function seedCommand(io, options) {
  if (options.from.length === 0) throw new UsageError('seed needs --from <report> (repeatable)');
  const { top, config } = loadHarnessConfig(io, options, false);
  mkdirSync(config.stateDir, { recursive: true });
  const release = acquireLock(config.stateDir);
  try {
    if (primaryCandidate(config).usable && !options.replace) throw new UsageError(`usable state exists in ${config.stateDirRaw}; pass --replace to seed over it`);
    const merged = mergeReports(options.from.map((p) => readReport(top, p)));
    const snapshot = takeSnapshot(config);
    const views = resolveViews(config, snapshot);
    if (options.replace) keepCurrent(config.stateDir);
    const work = join(config.stateDir, WORK_DIR);
    mkdirSync(work, { recursive: true });
    const output = join(work, REPORT_FILE);
    const bytes = Buffer.from(canonicalJSON(merged));
    writeFileSync(output, bytes);
    const deferrals = sortDeferrals(dedupeDeferrals([], options.from.map((p) => ({ reason: `seeded from ${p}`, paths: ['*'] })))).map((d) => ({ ...d, inherited: false }));
    const attestation = buildAttestation({
      config,
      files: snapshot.files,
      runtime: snapshot.runtime,
      reportBytes: bytes,
      report: merged,
      mode: 'seed',
      completedAt: new Date().toISOString(),
      lastFullAt: null,
      deferrals,
      views,
      summaries: viewSummaries(merged, views, deferrals),
    });
    commitState(config.stateDir, output, attestation);
    io.stdout.write(`seed: score=${attestation.score} thresholdBreak=${attestation.thresholdBreak}\n`);
    io.stderr.write(summaryLine({ command: 'seed', deferrals: deferrals.length, pendingFull: true }));
    return 0;
  } finally {
    release();
  }
}
