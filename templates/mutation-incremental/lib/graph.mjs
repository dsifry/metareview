import { existsSync, readFileSync, realpathSync } from 'node:fs';
import { builtinModules } from 'node:module';
import { dirname, join, posix } from 'node:path';

export const SOURCE_EXTS = ['.ts', '.tsx', '.mts', '.cts', '.js', '.jsx', '.mjs', '.cjs'];
const TS_EXTS = ['.ts', '.tsx', '.mts', '.cts'];
const MAX_BYTES = 1 << 20;
// String-literal specifiers only. `import type` / `export type` are elided at runtime (spec F4),
// and `vi.mock('./x')` replaces a module rather than depending on it.
// `=` is excluded so that, in code without semicolons, `export type X = …` cannot swallow the next
// statement's `from` (a real import/export clause never contains `=`).
const FROM = /(?:^|[^\w$.])(?:import|export)\s+(type\s+)?[^'"`;=]*?\bfrom\s*(['"])([^'"\n]+)\2/g;
const OTHERS = [
  /(?:^|[^\w$.])import\s*(['"])([^'"\n]+)\1/g,
  /(?:^|[^\w$.])import\s*\(\s*(['"])([^'"\n]+)\1\s*\)/g,
  /(?:^|[^\w$.])require\s*\(\s*(['"])([^'"\n]+)\1\s*\)/g,
];

export function specifiers(source) {
  const out = new Set();
  for (const m of source.matchAll(FROM)) if (!m[1]) out.add(m[3]);
  for (const re of OTHERS) for (const m of source.matchAll(re)) out.add(m[2]);
  return [...out];
}

function resolveFile(base, files) {
  const dir = base === '.' ? '' : `${base}/`; // a directory specifier at the repository root
  const candidates = [base, ...SOURCE_EXTS.map((e) => base + e), ...SOURCE_EXTS.map((e) => `${dir}index${e}`)];
  const js = base.match(/^(.*)\.[mc]?jsx?$/);
  if (js) candidates.push(...TS_EXTS.map((e) => js[1] + e));
  return candidates.find((c) => files[c]?.digest.startsWith('sha256:')) ?? null;
}

// Spec §5.4: a bare specifier is a package when node_modules holds it and its realpath still has a
// node_modules segment (npm, pnpm, yarn node-modules). Workspace links into the repo's own sources,
// and `npm link` targets elsewhere, are unresolved, so the importing file is an open importer.
function isInstalledPackage(spec, fromFile, top) {
  const name = spec.startsWith('@') ? spec.split('/').slice(0, 2).join('/') : spec.split('/')[0];
  let dir = dirname(join(top, fromFile));
  for (;;) {
    const candidate = join(dir, 'node_modules', name);
    if (existsSync(candidate)) return realpathSync(candidate).split(/[\\/]/).includes('node_modules');
    if (dir === top) return false;
    dir = dirname(dir);
  }
}

function classify(spec, fromFile, ctx) {
  let base = null;
  // '.' and '..' name a directory (its index), like './' and '../'.
  const relative = spec === '.' || spec === '..' || spec.startsWith('./') || spec.startsWith('../');
  if (relative) {
    base = posix.normalize(posix.join(posix.dirname(fromFile), spec)).replace(/(.)\/$/, '$1');
  } else {
    const key = Object.keys(ctx.aliases).filter((k) => spec.startsWith(k)).sort((a, b) => b.length - a.length)[0];
    if (key !== undefined) base = posix.normalize(ctx.aliases[key] + spec.slice(key.length));
  }
  if (base !== null) {
    // Spec §5.4: a relative or alias specifier that reaches no regular file in the snapshot (outside
    // the repo, gitignored or generated, excluded) is unresolved, so its file is an open importer.
    const to = base.startsWith('../') ? null : resolveFile(base, ctx.files);
    if (to) return { edge: to };
    // An alias key can also prefix a package name ("@" and "@testing-library/react"); an alias
    // specifier that reaches no file is still a package when one is installed under that name.
    if (relative) return { unresolved: true };
  }
  if (spec.startsWith('node:') || builtinModules.includes(spec.split('/')[0])) return {};
  return isInstalledPackage(spec, fromFile, ctx.top) ? {} : { unresolved: true };
}

const addTo = (map, key, value) => {
  if (!map.has(key)) map.set(key, new Set());
  map.get(key).add(value);
};

export function buildGraph(files, { top, aliases }) {
  const forward = new Map();
  const reverse = new Map();
  const open = new Map();
  const ctx = { files, aliases, top };
  for (const [path, info] of Object.entries(files)) {
    if (!info.digest.startsWith('sha256:') || !SOURCE_EXTS.some((e) => path.endsWith(e))) continue;
    const source = readFileSync(join(top, path), 'utf8');
    if (source.length > MAX_BYTES) {
      open.set(path, ['<source over 1 MiB>']); // its imports are unknown: an open importer
      continue;
    }
    for (const spec of specifiers(source)) {
      const r = classify(spec, path, ctx);
      if (r.edge) {
        addTo(forward, path, r.edge);
        addTo(reverse, r.edge, path);
      } else if (r.unresolved) {
        if (!open.has(path)) open.set(path, []);
        open.get(path).push(spec);
      }
    }
  }
  return { forward, reverse, open };
}

function closure(map, start) {
  const seen = new Set();
  const queue = [start];
  while (queue.length > 0) {
    for (const next of map.get(queue.pop()) ?? []) {
      if (!seen.has(next)) {
        seen.add(next);
        queue.push(next);
      }
    }
  }
  return seen;
}

// Files `start` imports, transitively, plus `start` itself.
export function reachesFrom(graph, start) {
  const out = closure(graph.forward, start);
  out.add(start);
  return out;
}

// Spec §5.4: tests that reach the target, plus every test that reaches an open importer
// (it may import the target through a specifier we could not resolve).
export function importerTests(graph, files, target) {
  const tests = new Set();
  const addTests = (paths) => {
    for (const p of paths) if (files[p].category === 'test') tests.add(p);
  };
  addTests(closure(graph.reverse, target));
  for (const opener of graph.open.keys()) addTests([opener, ...closure(graph.reverse, opener)]);
  return tests;
}

export function openImporters(graph) {
  return [...graph.open.entries()]
    .map(([path, specs]) => ({ path, specifiers: [...new Set(specs)].sort() }))
    .sort((a, b) => (a.path < b.path ? -1 : 1));
}
