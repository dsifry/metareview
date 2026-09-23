import { spawnSync } from 'node:child_process';
import { UsageError } from './errors.mjs';
import { matchList } from './glob.mjs';
import { duplicateKey } from './json.mjs';

const NAME = /^[A-Za-z0-9._-]+$/;
const VIEWS_TIMEOUT_MS = 60_000; // review advisory: a hanging views command must not stall the job
const fail = (message) => {
  throw new UsageError(`views: ${message}`);
};

const defaultRun = (argv, cwd) => spawnSync(argv[0], argv.slice(1), { cwd, encoding: 'utf8', timeout: VIEWS_TIMEOUT_MS });

// Spec §11.3: views are read-time only; completeness is checked at plan time before any invocation.
export function resolveViews(config, snapshot, { run = defaultRun } = {}) {
  if (!config.views) return null;
  let map = config.views.inline;
  if (map === undefined) {
    const r = run(config.views.command, config.top);
    if (r.status !== 0) fail(`views command ${config.views.command.join(' ')} failed (exit ${r.status}${r.error ? `: ${r.error.message}` : ''})`);
    try {
      map = JSON.parse(r.stdout);
    } catch {
      fail('views command did not print JSON');
    }
    const dup = duplicateKey(r.stdout);
    if (dup !== null) fail(`duplicate view name ${JSON.stringify(dup)}`);
  }
  if (map === null || typeof map !== 'object' || Array.isArray(map)) fail('views must be an object of name → patterns');
  for (const [name, patterns] of Object.entries(map)) {
    if (!NAME.test(name)) fail(`invalid view name ${JSON.stringify(name)} (use [A-Za-z0-9._-]+)`);
    if (!Array.isArray(patterns) || !patterns.every((p) => typeof p === 'string')) fail(`view ${name} must be an array of patterns`);
  }
  const entries = Object.keys(map).sort().map((name) => [name, map[name]]);
  for (const [path, info] of Object.entries(snapshot.files)) {
    for (const [name, patterns] of entries) {
      if (info.category !== 'mutate' && matchList(path, patterns)) fail(`view ${name} matches ${path}, which is not a mutate file`);
    }
    if (info.category === 'mutate' && !entries.some(([, patterns]) => matchList(path, patterns))) {
      fail(`mutate file ${path} belongs to no view (assign it in the views source before it has mutants)`);
    }
  }
  return Object.fromEntries(entries);
}

export function viewFiles(views, snapshot) {
  const mutate = Object.keys(snapshot.files).filter((p) => snapshot.files[p].category === 'mutate').sort();
  return Object.fromEntries(Object.entries(views).map(([name, patterns]) => [name, mutate.filter((p) => matchList(p, patterns))]));
}

// Per-view counts are computed per view and never summed (spec §11.3). Pending = kills covered by
// a deferral; a kill covered by both a counted and an inherited deferral is counted (§11.2).
export function viewSummaries(report, views, classified) {
  if (!views) return null;
  const out = {};
  for (const [name, patterns] of Object.entries(views)) {
    const s = { pendingCounted: 0, pendingInherited: 0 };
    for (const [file, entry] of Object.entries(report.files)) {
      if (!matchList(file, patterns)) continue;
      const covering = classified.filter((d) => d.paths.includes('*') || d.paths.includes(file));
      for (const m of entry.mutants ?? []) {
        s[m.status] = (s[m.status] ?? 0) + 1;
        if (m.status !== 'Killed' || covering.length === 0) continue;
        if (covering.some((d) => !d.inherited)) s.pendingCounted++;
        else s.pendingInherited++;
      }
    }
    out[name] = s;
  }
  return out;
}
