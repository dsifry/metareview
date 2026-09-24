import { join } from 'node:path';
import { spawnGroup } from './proc.mjs';
import { ATTESTATION_FILE, REPORT_FILE } from './state.mjs';
import { viewFiles } from './views.mjs';

const VIEW_VARS = ['MUTATION_VIEW', 'MUTATION_VIEW_PATTERNS', 'MUTATION_VIEW_FILES'];

// Spec §11.1: after state is committed, run the project's verifier once per view (once without
// views), no shell, cwd = top-level, own process group. Any failure (non-zero exit, a command that
// cannot be spawned, exceeding timeoutMinutes) is one returned line naming the view; the run then
// exits 1. An interrupt stops the loop; the caller exits 130.
export async function runVerify(config, { views, snapshot, pr, interrupt, graceMs, onOutput }) {
  if (config.verify === null) return [];
  const base = {
    ...process.env,
    MUTATION_REPORT: join(config.stateDir, REPORT_FILE),
    MUTATION_ATTESTATION: join(config.stateDir, ATTESTATION_FILE),
    MUTATION_RUN_KIND: pr ? 'pr' : 'other',
  };
  for (const name of VIEW_VARS) delete base[name];
  const runs = views === null
    ? [{ label: 'verify', env: base }]
    : Object.entries(views).map(([name, patterns]) => ({
      label: `verify (view ${name})`,
      env: { ...base, MUTATION_VIEW: name, MUTATION_VIEW_PATTERNS: JSON.stringify(patterns), MUTATION_VIEW_FILES: JSON.stringify(viewFiles({ [name]: patterns }, snapshot)[name]) },
    }));
  const timeoutMs = config.verify.timeoutMinutes === null ? null : config.verify.timeoutMinutes * 60_000;
  const failures = [];
  for (const run of runs) {
    const r = await spawnGroup(config.verify.command, { cwd: config.top, env: run.env, timeoutMs, graceMs, interrupt, onOutput });
    if (r.interrupted) break;
    if (r.error) failures.push(`${run.label}: cannot run ${config.verify.command[0]}: ${r.error.message}`);
    else if (r.timedOut) failures.push(`${run.label}: exceeded verify.timeoutMinutes (${config.verify.timeoutMinutes})`);
    else if (r.code !== 0) failures.push(`${run.label}: exit ${r.code}`);
  }
  return failures;
}
