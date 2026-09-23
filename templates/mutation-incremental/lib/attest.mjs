import { copyFileSync, mkdirSync, renameSync, rmSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { canonicalJSON } from './json.mjs';
import { fileDigest, sha256 } from './snapshot.mjs';
import { ATTESTATION_FILE, REPORT_FILE, STATE_VERSION, TOOL, TOOL_VERSION } from './state.mjs';

export const WORK_DIR = 'work';
export const CHANGED_DURING_RUN = 'changed-during-run';
const SCORED = ['Killed', 'Timeout', 'Survived', 'NoCoverage'];

// Stryker's definition (spec §5.5): (Killed + Timeout) / (Killed + Timeout + Survived + NoCoverage)
// × 100, or null when that denominator is 0.
export function mutationScore(report) {
  const n = Object.fromEntries(SCORED.map((s) => [s, 0]));
  for (const entry of Object.values(report.files)) {
    for (const m of entry.mutants ?? []) if (Object.hasOwn(n, m.status)) n[m.status]++;
  }
  const denominator = n.Killed + n.Timeout + n.Survived + n.NoCoverage;
  return denominator === 0 ? null : ((n.Killed + n.Timeout) / denominator) * 100;
}

// Spec §5.5 with §11.2 (inherited flags on deferrals), §11.3 (views) and §11.6 (stateVersion).
export function buildAttestation({ config, files, runtime, reportBytes, report, mode, completedAt, lastFullAt, deferrals, views, summaries }) {
  const score = mutationScore(report);
  const attestation = {
    schemaVersion: 1,
    tool: TOOL,
    toolVersion: TOOL_VERSION,
    stateVersion: STATE_VERSION,
    engine: 'stryker',
    engineVersion: config.engineVersion,
    mode,
    completedAt,
    lastFullAt,
    report: REPORT_FILE,
    reportSha256: sha256(reportBytes),
    score,
    // A null score or an absent threshold never breaks.
    thresholdBreak: score !== null && config.thresholdBreak !== null && score < config.thresholdBreak,
    lists: config.lists,
    exclusions: config.exclusions,
    files,
    runtime,
    deferrals,
  };
  if (views !== null) Object.assign(attestation, { views, viewSummaries: summaries });
  return attestation;
}

// Spec §5.5: a path whose digest at the end of the run differs from the start-of-run snapshot is
// recorded as changed-during-run, so it never matches and is re-planned next time.
export function markChangedDuringRun(config, files) {
  return Object.fromEntries(Object.entries(files).map(([path, info]) => [path, fileDigest(config, path) === info.digest ? info : { ...info, digest: CHANGED_DURING_RUN }]));
}

// Spec §5.6 step 7: the attestation goes to work/, the output is renamed over incremental.json,
// then the attestation over attestation.json. A crash between the renames leaves a pair whose
// hashes disagree, which is cold next time and never trusted.
export function commitState(stateDir, outputPath, attestation) {
  const work = join(stateDir, WORK_DIR);
  mkdirSync(work, { recursive: true });
  const pending = join(work, ATTESTATION_FILE);
  writeFileSync(pending, canonicalJSON(attestation));
  const canonical = join(stateDir, REPORT_FILE);
  if (outputPath !== canonical) renameSync(outputPath, canonical);
  renameSync(pending, join(stateDir, ATTESTATION_FILE));
  rmSync(work, { recursive: true, force: true });
}

// Spec §5.4 Adopt: run copies an adopted state's two files into stateDir (temp file + rename),
// report first, so an interrupted copy leaves hashes that disagree.
export function installAdopted(stateDir, fromDir) {
  const work = join(stateDir, WORK_DIR);
  mkdirSync(work, { recursive: true });
  for (const name of [REPORT_FILE, ATTESTATION_FILE]) {
    const tmp = join(work, `adopt-${name}`);
    copyFileSync(join(fromDir, name), tmp);
    renameSync(tmp, join(stateDir, name));
  }
}
