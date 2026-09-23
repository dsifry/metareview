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
