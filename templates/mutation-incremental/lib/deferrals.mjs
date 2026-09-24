import { UsageError } from './errors.mjs';
// Deferrals are {reason, paths}; paths are file paths or ["*"] (spec §4). Reasons and paths are
// canonical (fixed formats, byte-ordered paths) so equality comparisons are exact (spec §11.2).
const cmp = (a, b) => (a < b ? -1 : a > b ? 1 : 0);

export const canonicalDeferral = (d) => ({ reason: d.reason, paths: [...d.paths].sort(cmp) });

export const deferralKey = (d) => `${d.reason}\u0000${[...d.paths].sort(cmp).join('\u0001')}`;

// Previous then new, de-duplicated by reason and sorted paths (spec §5.5).
export function dedupeDeferrals(prev, next) {
  const seen = new Set();
  const out = [];
  for (const d of [...prev, ...next]) {
    const key = deferralKey(d);
    if (!seen.has(key)) {
      seen.add(key);
      out.push(canonicalDeferral(d));
    }
  }
  return out;
}

// Plan JSON order (spec §5.4): by reason, then first path.
export const sortDeferrals = (list) => [...list].sort((a, b) => cmp(a.reason, b.reason) || cmp(a.paths[0], b.paths[0]));


// The single table of canonical reasons (spec §11.2; review advisory "reason → named key").
// `named` is the path or runtime key the reason is about, for the inheritance digest check (ii).
const REASONS = [
  { re: /^global input changed: (.+)$/, cause: 'global', named: true },
  { re: /^support (.+) has no resolvable importer$/, cause: 'global', named: true },
  { re: /^support (.+) deleted$/, cause: 'global', named: true },
  { re: /^unclassified (.+) has no resolvable importer$/, cause: 'global', named: true },
  { re: /^no usable state$/, cause: 'global', named: false },
  { re: /^seeded from (.+)$/, cause: 'global', named: false },
  { re: /^no reachable tests: (.+)$/, cause: 'unreachable', named: true },
  { re: /^residual off: (.+)$/, cause: 'other', named: true },
  { re: /^time budget exceeded$/, cause: 'timeout', named: false },
  { re: /^blocked by deferred scope$/, cause: 'timeout', named: false },
  { re: /^forced set \d+ exceeds budget \d+$/, cause: 'timeout', named: false },
];

export function reasonInfo(reason) {
  for (const r of REASONS) {
    const m = reason.match(r.re);
    if (m) return { cause: r.cause, named: r.named ? m[1] : null };
  }
  throw new UsageError(`unknown deferral reason ${JSON.stringify(reason)} (harness bug)`);
}

const digestIn = (source, name) => (name.startsWith('cmd:') || name.startsWith('env:') ? source.runtime?.[name] : source.files?.[name]?.digest) ?? null;

function sameInputs(d, att, snapshot) {
  const { named } = reasonInfo(d.reason);
  const names = [...(named === null ? [] : [named]), ...d.paths.filter((p) => p !== '*')];
  return names.every((n) => digestIn(snapshot, n) === digestIn(att, n));
}

// Spec §11.2: a deferral is inherited only when main's published state carries the identical
// deferral (i) about inputs this run has not changed (ii). A cold run's "no usable state" and any
// deferral this run's own invocations produced are always counted.
export function classifyDeferrals({ deferrals, alsoAttestations = [], snapshot, producedKeys = new Set(), cold = false }) {
  return deferrals.map((d) => {
    const key = deferralKey(d);
    const forceCounted = producedKeys.has(key) || (cold && d.reason === 'no usable state');
    const inherited = !forceCounted && alsoAttestations.some((att) => (att.deferrals ?? []).some((x) => deferralKey(x) === key) && sameInputs(d, att, snapshot));
    return { ...canonicalDeferral(d), inherited };
  });
}

// Spec §11.2 Cause: global > unreachable > other > timeout > none, over counted deferrals only.
export function pendingCause(classified) {
  const counted = classified.filter((d) => !d.inherited);
  const infos = counted.map((d) => reasonInfo(d.reason));
  if (counted.some((d) => d.paths.length === 1 && d.paths[0] === '*')) return { cause: 'global', counted };
  const cause = ['unreachable', 'other', 'timeout'].find((c) => infos.some((i) => i.cause === c)) ?? 'none';
  return { cause, counted };
}

// Spec §11.2 Routing. Without --pr, or under allow, the exit code alone decides ("verdict"). Under
// full every cause but none routes to the sweep; under full-on-global only unreachable fails the PR,
// so a PR's verdict never depends on runner speed.
export function routeFor(pendingOnPr, cause, pr) {
  if (!pr || pendingOnPr === 'allow' || cause === 'none') return 'verdict';
  return pendingOnPr === 'full-on-global' && cause === 'unreachable' ? 'fail' : 'sweep';
}
