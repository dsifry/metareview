import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { UsageError } from './errors.mjs';
import { indexReport } from './report.mjs';
import { importerTests, openImporters, reachesFrom } from './graph.mjs';
import { lineDiff, hunkIntersects } from './diff.mjs';
import { canonicalDeferral, dedupeDeferrals, sortDeferrals } from './deferrals.mjs';

// Spec §11.4: larger edited files use the whole-file rule.
export const RESIDUAL_MAX_BYTES = 1 << 20;
const BAD_PATH = /[,{}[\]()!*?:\n]/;
const WEAK = new Set(['Survived', 'NoCoverage']);
const cmp = (a, b) => (a < b ? -1 : a > b ? 1 : 0);

const PUBLIC_KEYS = ['baseline', 'cold', 'pendingFull', 'scope', 'forced', 'forcedCount', 'deferrals', 'changes', 'openImporters', 'unclassified'];
export const publicPlan = (plan) => Object.fromEntries(PUBLIC_KEYS.map((k) => [k, plan[k]]));

function coldPlan(open) {
  const deferrals = [{ reason: 'no usable state', paths: ['*'] }];
  return { baseline: null, cold: true, pendingFull: true, scope: [], forced: [], forcedCount: 0, deferrals, added: deferrals, changes: [], openImporters: open, unclassified: [], invocation1: [], invocation2: [] };
}

export function computePlan({ config, snapshot, graph, chosen, disableResidual = false, unbudgeted = false, readText }) {
  const read = readText ?? ((f) => readFileSync(join(config.top, f), 'utf8'));
  const open = openImporters(graph);
  if (!chosen) return coldPlan(open);

  const reportFiles = chosen.report.files;
  const R = indexReport(chosen.report);
  const byFile = new Map();
  for (const m of R.mutants) {
    if (!byFile.has(m.file)) byFile.set(m.file, []);
    byFile.get(m.file).push(m);
  }
  const own = (f) => byFile.get(f) ?? [];
  const inR = (f) => Object.hasOwn(reportFiles, f);
  // A path Stryker may be given: a regular file still in the mutate category (deleted, symlinked or
  // re-categorised files drop out, spec §5.4 Normalise).
  const present = (f) => snapshot.files[f]?.category === 'mutate' && snapshot.files[f].digest.startsWith('sha256:');
  const hits = (ids, set) => ids.some((id) => set.has(id));
  const idsOf = (tests) => new Set([...tests].flatMap((t) => R.testIds[t] ?? []));
  const importerIds = (p) => idsOf(importerTests(graph, snapshot.files, p));
  const changes = chosen.changes;

  const edited = new Set();           // forced whole-file (whole mode, or residual fallback)
  const residualEdited = new Map();   // file -> lineDiff result (unforced scope + mapped closure)
  const diffs = new Map();            // every changed mutate file with an attested source
  const scope = new Set();
  const forced = new Map();           // file -> Map(mutant id -> [start, end] in current lines)
  const added = [];

  // Step 1 (spec §5.4, §11.4): edited mutate files.
  for (const c of changes) {
    if (c.category !== 'mutate' || c.kind === 'deleted') continue;
    let d = null;
    if (inR(c.path) && present(c.path)) {
      const text = read(c.path);
      // Stryker embeds every file's source in the report (spec F3). null above the size or edit cap.
      if (Buffer.byteLength(text) <= RESIDUAL_MAX_BYTES) d = lineDiff(reportFiles[c.path].source, text);
    }
    if (d) diffs.set(c.path, d);
    if (config.editedFiles === 'residual' && d) residualEdited.set(c.path, d);
    else edited.add(c.path);
  }

  const addForced = (m) => {
    if (edited.has(m.file)) return;
    let start = m.startLine;
    let end = m.endLine;
    const d = residualEdited.get(m.file);
    if (d) {
      // Mutants touching a changed hunk are rebuilt and re-run by the unforced scope invocation.
      if (d.hunks.some((h) => hunkIntersects(start, end, h))) return;
      start = d.oldToNew[start];
      end = d.oldToNew[end];
    }
    if (!forced.has(m.file)) forced.set(m.file, new Map());
    forced.get(m.file).set(m.id, [start, end]);
  };

  // Step 2: tests.
  const weak = (f) => !inR(f) || own(f).some((m) => WEAK.has(m.status));
  const mutateFiles = Object.keys(snapshot.files).filter((f) => snapshot.files[f].category === 'mutate');
  for (const c of changes) {
    if (c.category !== 'test') continue;
    if (c.kind !== 'new' && R.testIds[c.path]) {
      const ids = new Set(R.testIds[c.path]);
      for (const m of R.mutants) if (hits(m.killedBy, ids) || hits(m.coveredBy, ids)) scope.add(m.file);
    }
    if (c.kind === 'deleted') continue;
    const reached = reachesFrom(graph, c.path);
    for (const f of reached) if (snapshot.files[f].category === 'mutate' && weak(f)) scope.add(f);
    if ([...reached].some((f) => graph.open.has(f))) {
      for (const f of mutateFiles) if (!inR(f)) scope.add(f);
      for (const f of byFile.keys()) if (weak(f)) scope.add(f);
    }
  }

  // Step 3: support.
  for (const c of changes) {
    if (c.category !== 'support') continue;
    if (c.kind === 'deleted') {
      added.push({ reason: `support ${c.path} deleted`, paths: ['*'] });
      continue;
    }
    const tests = importerTests(graph, snapshot.files, c.path);
    if (tests.size === 0) {
      added.push({ reason: `support ${c.path} has no resolvable importer`, paths: ['*'] });
      continue;
    }
    const ids = idsOf(tests);
    for (const m of R.mutants) if (hits(m.killedBy, ids) || hits(m.coveredBy, ids)) addForced(m);
  }

  // Step 4: residual (spec §5.4 step 4, §11.4.3, §11.5).
  const killedBy = (T, skip) => R.mutants.filter((m) => m.status === 'Killed' && m.file !== skip && hits(m.killedBy, T));
  // Mutants touching a changed hunk of a residual-edited file are re-run by the unforced scope and
  // the forced changed lines, so they are neither forced by the closure nor recorded under "off".
  const rerunInScope = (m) => residualEdited.get(m.file)?.hunks.some((h) => hunkIntersects(m.startLine, m.endLine, h)) === true;
  const applyResidual = (origin, targets) => {
    const live = targets.filter((m) => !rerunInScope(m));
    if (disableResidual || live.length === 0) return;
    if (config.residualMode === 'off') {
      added.push({ reason: `residual off: ${origin}`, paths: [...new Set(live.map((m) => m.file))] });
      return;
    }
    for (const m of live) addForced(m);
  };
  for (const c of changes) {
    if (c.category === 'mutate') {
      const Y = c.path;
      const mine = own(Y);
      const d = diffs.get(Y);
      const touching = residualEdited.has(Y) ? mine.filter((m) => d.hunks.some((h) => hunkIntersects(m.startLine, m.endLine, h))) : mine;
      const T = new Set(touching.flatMap((m) => m.coveredBy));
      const outsideEveryMutant = mine.length === 0 || !d || d.hunks.some((h) => !mine.some((m) => hunkIntersects(m.startLine, m.endLine, h)));
      // Spec §5.4 step 4 / §11.5: only a *changed* Y falls back to (or adds) its importer tests.
      if (c.kind === 'changed' && outsideEveryMutant) for (const id of importerIds(Y)) T.add(id);
      applyResidual(Y, killedBy(T, residualEdited.has(Y) ? null : Y));
    } else if (c.category === 'unclassified' && c.kind !== 'deleted') {
      const tests = importerTests(graph, snapshot.files, c.path);
      if (tests.size === 0) added.push({ reason: `unclassified ${c.path} has no resolvable importer`, paths: ['*'] });
      else applyResidual(c.path, killedBy(idsOf(tests), null));
    }
  }

  // Step 5: global and runtime inputs.
  for (const c of changes) if (c.category === 'global') added.push({ reason: `global input changed: ${c.path}`, paths: ['*'] });

  // Normalise.
  for (const f of [...edited]) if (!present(f)) edited.delete(f);
  for (const f of residualEdited.keys()) if (present(f)) scope.add(f);
  for (const f of [...scope]) if (!present(f) || edited.has(f)) scope.delete(f);
  for (const f of [...forced.keys()]) if (!present(f)) forced.delete(f);
  const whole = new Set();
  for (const f of forced.keys()) {
    if (scope.has(f) && !residualEdited.has(f)) {
      whole.add(f);
      scope.delete(f);
    }
  }
  const N = R.mutants.length;
  let b = Math.floor(config.budget.maxForcedShare * N);
  if (config.budget.maxForcedMutants !== null && config.budget.maxForcedMutants < b) b = config.budget.maxForcedMutants;
  const forcedIds = new Set();
  for (const [f, ms] of forced) for (const id of whole.has(f) ? own(f).map((m) => m.id) : ms.keys()) forcedIds.add(`${f}\u0000${id}`);
  const forcedCount = forcedIds.size;
  if (!unbudgeted && config.residualMode === 'bounded' && forcedCount > b) {
    added.push({ reason: `forced set ${forcedCount} exceeds budget ${b}`, paths: [...forced.keys()] });
    for (const f of whole) scope.add(f);
    forced.clear();
    whole.clear();
  }

  // Forced spans per file: the closure's mutants plus, for residual-edited files, the new-side lines
  // of every changed hunk (mandatory and budget-exempt: added after the budget check).
  const spansOf = new Map();
  for (const [f, ms] of forced) if (!whole.has(f)) spansOf.set(f, [...ms.values()]);
  // A mutant strictly enclosing a changed hunk (or containing a pure insertion) has changed text,
  // so Stryker re-runs it from the scope. Any other mutant touching a hunk may keep its own text
  // unchanged (Stryker diffs characters), so its current span is forced.
  const strictlySpans = (m, h) => h.oldEnd < h.oldStart || (m.startLine < h.oldStart && m.endLine > h.oldEnd);
  const containing = (d, line) => d.hunks.find((h) => h.oldStart <= line && line <= h.oldEnd);
  for (const [f, d] of residualEdited) {
    const spans = d.hunks.filter((h) => h.newEnd >= h.newStart).map((h) => [h.newStart, h.newEnd]);
    for (const m of own(f)) {
      if (!d.hunks.some((h) => hunkIntersects(m.startLine, m.endLine, h) && !strictlySpans(m, h))) continue;
      const start = d.oldToNew[m.startLine] ?? containing(d, m.startLine).newStart;
      spans.push([start, Math.max(start, d.oldToNew[m.endLine] ?? containing(d, m.endLine).newEnd)]);
    }
    if (spans.length > 0) spansOf.set(f, [...(spansOf.get(f) ?? []), ...spans]);
  }
  const entries = [...edited, ...whole].map((f) => ({ file: f, start: 0, text: f }));
  for (const [f, spans] of spansOf) {
    let cur = null;
    for (const [s, e] of spans.sort((x, y) => x[0] - y[0] || x[1] - y[1])) {
      if (cur && s <= cur.end + 1) cur.end = Math.max(cur.end, e);
      else {
        cur = { file: f, start: s, end: e };
        entries.push(cur);
      }
    }
  }
  const forcedStrings = entries
    .sort((x, y) => cmp(x.file, y.file) || x.start - y.start)
    .map((x) => x.text ?? `${x.file}:${x.start}-${x.end}`);
  for (const f of [...scope, ...entries.map((x) => x.file)]) {
    if (BAD_PATH.test(f)) throw new UsageError(`${JSON.stringify(f)} cannot be passed to Stryker --mutate (contains , { } [ ] ( ) ! * ? : or a newline)`);
  }

  const addedCanon = dedupeDeferrals([], added);
  const deferrals = sortDeferrals(dedupeDeferrals(chosen.attestation.deferrals.map(canonicalDeferral), addedCanon));
  const scopeList = [...scope].sort(cmp);
  return {
    baseline: chosen.label,
    cold: false,
    pendingFull: deferrals.length > 0,
    scope: scopeList,
    forced: forcedStrings,
    forcedCount,
    deferrals,
    added: sortDeferrals(addedCanon),
    changes,
    openImporters: open,
    unclassified: changes.filter((c) => c.category === 'unclassified' && c.kind !== 'deleted').map((c) => c.path).sort(cmp),
    invocation1: scopeList,
    invocation2: forcedStrings,
  };
}
