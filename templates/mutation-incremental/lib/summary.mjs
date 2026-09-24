import { appendFileSync } from 'node:fs';
import { join } from 'node:path';
import { UsageError } from './errors.mjs';
import { loadHarnessConfig, summaryLine } from './inputs.mjs';
import { canonicalPendingFull, primaryCandidate, readCandidate } from './state.mjs';
import { deferralKey, pendingCause, routeFor } from './deferrals.mjs';

const JOBS = ['pr', 'pr-full', 'main', 'full'];

// Reasons and paths are rendered as code spans (review advisory); a backtick cannot close one early.
const code = (text) => `\`${String(text).replaceAll('`', "'")}\``;

function scoreText(score, thresholdBreak) {
  const value = score === null ? 'n/a' : score.toFixed(2);
  if (thresholdBreak === null) return `${value} (no thresholds.break)`;
  return `${value}, ${score !== null && score < thresholdBreak ? 'below' : 'at or above'} thresholds.break ${thresholdBreak}`;
}

const deferralLine = (d) => `${code(d.reason)} on ${d.paths.map(code).join(', ')}`;

function mainLines(remotes, thresholdBreak) {
  const [inc, full] = remotes;
  const main = [inc, full].find((c) => c.usable);
  const lines = main === undefined
    ? ['- main: main state unavailable']
    : [`- main (${main === inc ? 'mutation-state/inc' : 'mutation-state/full'}): score ${scoreText(main.attestation.score, thresholdBreak)}${main.attestation.deferrals.length > 0 ? ' (includes pending kills)' : ''}`];
  if (full.usable) lines.push(`- mutation-state/full: completed ${full.attestation.completedAt}, last full run ${full.attestation.lastFullAt}`);
  return lines;
}

// Spec §5.7 step 5 and §11.2: what the run did, what is pending and why, and (on PRs) main's score.
export function summaryMarkdown({ job, exitCode, config, state, remotes, pr }) {
  const lines = [`### Mutation testing (${job})`, ''];
  if (exitCode === '') return `${[...lines, '- run step did not execute or did not finish (see earlier step)'].join('\n')}\n`;
  lines.push(`- exit code: ${code(exitCode)}`);
  const committed = exitCode === '0' || exitCode === '1';
  if (!state.usable) lines.push('- no usable state');
  else {
    const att = state.attestation;
    const pending = att.deferrals.length > 0;
    lines.push(`- pending full run: ${pending}`);
    lines.push(`- score: ${scoreText(att.score, config.thresholdBreak)}${pending ? ' (includes pending kills)' : ''}${committed ? '' : ' — previous state (this run committed nothing)'}`);
    const counted = att.deferrals.filter((d) => !d.inherited);
    for (const d of counted) lines.push(`- deferred: ${deferralLine(d)}`);
    for (const d of att.deferrals.filter((x) => x.inherited)) lines.push(`- inherited from main, cleared by main's full run: ${deferralLine(d)}`);
    const route = committed ? routeFor(config.pendingOnPr, pendingCause(att.deferrals).cause, pr) : 'verdict';
    if (route === 'sweep') lines.push('- verdict deferred to pr-full');
    if (route === 'fail') {
      const mainKeys = new Set(remotes.filter((c) => c.usable).flatMap((c) => c.attestation.deferrals.map(deferralKey)));
      for (const d of counted.filter((x) => x.reason.startsWith('no reachable tests: '))) {
        lines.push(`- ${code(d.reason)}: add a test or remove the module${mainKeys.has(deferralKey(d)) ? ' (also on main)' : ''}`);
      }
    }
  }
  if (job === 'pr') lines.push(...mainLines(remotes, config.thresholdBreak));
  return `${lines.join('\n')}\n`;
}

// `summary --job pr|pr-full|main|full --exit-code <n> [--pr]`: appends to $GITHUB_STEP_SUMMARY (or
// prints). The workflow runs it with `if: always()` and `|| true`, so it never fails a job.
export async function summaryCommand(io, options) {
  if (!JOBS.includes(options.job)) throw new UsageError('summary needs --job pr|pr-full|main|full');
  const { config } = loadHarnessConfig(io, options, false);
  const text = summaryMarkdown({
    job: options.job,
    exitCode: options.exitCode ?? '',
    config,
    state: primaryCandidate(config),
    remotes: ['inc', 'full'].map((kind) => readCandidate(join(config.stateDir, 'remote', kind), { label: `mutation-state/${kind}`, primary: false })),
    pr: options.pr,
  });
  const file = process.env.GITHUB_STEP_SUMMARY;
  if (file) appendFileSync(file, text);
  else io.stdout.write(text);
  io.stderr.write(summaryLine({ command: 'summary', pendingFull: canonicalPendingFull(config) }));
  return 0;
}
