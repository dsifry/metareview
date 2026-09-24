import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { sha256 } from './snapshot.mjs';

export const TOOL = 'metareview-mutation-incremental';
export const TOOL_VERSION = '0.13.0';
// Spec §11.6: bumped only when planning or attestation semantics change; usability checks this,
// not toolVersion, so patch releases keep state.
export const STATE_VERSION = 1;
export const ATTESTATION_FILE = 'attestation.json';
export const REPORT_FILE = 'incremental.json';

const isObject = (v) => v !== null && typeof v === 'object' && !Array.isArray(v);

export function readCandidate(dir, { label, primary }) {
  const out = { label, dir, primary, usable: false, reason: '', attestation: null, report: null };
  let text;
  try {
    text = readFileSync(join(dir, ATTESTATION_FILE), 'utf8');
  } catch {
    out.reason = 'missing attestation';
    return out;
  }
  let att;
  try {
    att = JSON.parse(text);
  } catch {
    att = null;
  }
  if (!isObject(att)) {
    out.reason = 'unparseable attestation';
    return out;
  }
  out.attestation = att;
  if (att.schemaVersion !== 1 || att.tool !== TOOL || !isObject(att.files) || !isObject(att.runtime) || !Array.isArray(att.deferrals)) {
    out.reason = 'not a harness attestation';
    return out;
  }
  if (att.stateVersion !== STATE_VERSION) {
    out.reason = 'stateVersion mismatch';
    return out;
  }
  let bytes;
  try {
    bytes = readFileSync(join(dir, REPORT_FILE));
  } catch {
    out.reason = 'missing report';
    return out;
  }
  if (sha256(bytes) !== att.reportSha256) {
    out.reason = 'report hash mismatch';
    return out;
  }
  try {
    out.report = JSON.parse(bytes.toString('utf8'));
  } catch {
    out.reason = 'unparseable report';
    return out;
  }
  out.usable = true;
  return out;
}

// Sorted by path, then category (a path appears twice only when its category changed).
const sortChanges = (list) => {
  const byKey = new Map(list.map((c) => [`${c.path}\u0000${c.category}`, c]));
  return [...byKey.keys()].sort().map((k) => byKey.get(k));
};

// Spec §5.4 Change set. A path whose category changed appears under both categories: as deleted
// under the old one and new under the current one.
export function changeSet(attestation, snapshot) {
  const out = [];
  const old = attestation.files;
  for (const [path, info] of Object.entries(snapshot.files)) {
    const prev = old[path];
    if (!prev) out.push({ path, category: info.category, kind: 'new' });
    else if (prev.category !== info.category) {
      out.push({ path, category: prev.category, kind: 'deleted' });
      out.push({ path, category: info.category, kind: 'new' });
    } else if (prev.digest !== info.digest) out.push({ path, category: info.category, kind: 'changed' });
  }
  for (const [path, prev] of Object.entries(old)) if (!snapshot.files[path]) out.push({ path, category: prev.category, kind: 'deleted' });
  const oldRuntime = attestation.runtime;
  const keys = new Set([...Object.keys(oldRuntime), ...Object.keys(snapshot.runtime)]);
  for (const key of keys) {
    if (!(key in oldRuntime)) out.push({ path: key, category: 'global', kind: 'new' });
    else if (!(key in snapshot.runtime)) out.push({ path: key, category: 'global', kind: 'deleted' });
    else if (oldRuntime[key] !== snapshot.runtime[key]) out.push({ path: key, category: 'global', kind: 'changed' });
  }
  return sortChanges(out);
}

// Spec §5.4 Adopt: change-based, never time-based.
export function adopt(candidates, snapshot) {
  const ranked = candidates
    .map((c, order) => ({ c, order }))
    .filter(({ c }) => c.usable)
    .map((r) => ({ ...r, changes: changeSet(r.c.attestation, snapshot), deferred: r.c.attestation.deferrals.length > 0 ? 1 : 0 }))
    .sort((x, y) => x.deferred - y.deferred || x.changes.length - y.changes.length || Number(y.c.primary) - Number(x.c.primary) || x.order - y.order);
  return ranked.length === 0 ? null : { ...ranked[0].c, changes: ranked[0].changes };
}

export const primaryCandidate = (config) => readCandidate(config.stateDir, { label: config.stateDirRaw, primary: true });

// Spec §4: a pending full run is a state that is not usable, or one whose attestation has deferrals.
export function canonicalPendingFull(config) {
  const state = primaryCandidate(config);
  return !state.usable || state.attestation.deferrals.length > 0;
}
